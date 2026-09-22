// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LannCo/remoteterm/pkg/blocklogger"
	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/panichandler"
	"github.com/LannCo/remoteterm/pkg/remote"
	"github.com/LannCo/remoteterm/pkg/remote/conncontroller"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/shellexec"
	"github.com/LannCo/remoteterm/pkg/util/envutil"
	"github.com/LannCo/remoteterm/pkg/util/fileutil"
	"github.com/LannCo/remoteterm/pkg/util/shellutil"
	"github.com/LannCo/remoteterm/pkg/util/utilfn"
	"github.com/LannCo/remoteterm/pkg/utilds"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshclient"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/LannCo/remoteterm/pkg/wslconn"
)

const (
	ConnType_Local = "local"
	ConnType_Wsl   = "wsl"
	ConnType_Ssh   = "ssh"
)

const (
	LocalConnVariant_GitBash = "gitbash"
)

type ShellController struct {
	Lock *sync.Mutex

	// shared fields
	ControllerType string
	TabId          string
	BlockId        string
	ConnName       string
	BlockDef       *remotetermobj.BlockDef
	RunLock        *atomic.Bool
	ProcStatus     string
	ProcExitCode   int
	VersionTs      utilds.VersionTs

	// for shell/cmd
	ShellProc    *shellexec.ShellProc
	ShellInputCh chan *BlockInputUnion
}

// Constructor that returns the Controller interface
func MakeShellController(tabId string, blockId string, controllerType string, connName string) Controller {
	return &ShellController{
		Lock:           &sync.Mutex{},
		ControllerType: controllerType,
		TabId:          tabId,
		BlockId:        blockId,
		ConnName:       connName,
		ProcStatus:     Status_Init,
		RunLock:        &atomic.Bool{},
	}
}

// Implement Controller interface methods

func (sc *ShellController) Start(ctx context.Context, blockMeta remotetermobj.MetaMapType, rtOpts *remotetermobj.RuntimeOpts, force bool) error {
	// Get the block data
	blockData, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, sc.BlockId)
	if err != nil {
		return fmt.Errorf("error getting block: %w", err)
	}

	// Use the existing run method which handles all the start logic
	go sc.run(ctx, blockData, blockData.Meta, rtOpts, force)
	return nil
}

func (sc *ShellController) Stop(graceful bool, newStatus string, destroy bool) {
	sc.Lock.Lock()
	defer sc.Lock.Unlock()

	if sc.ShellProc == nil || sc.ProcStatus == Status_Done || sc.ProcStatus == Status_Init {
		if newStatus != sc.ProcStatus {
			sc.ProcStatus = newStatus
			sc.sendUpdate_nolock()
		}
		return
	}

	sc.ShellProc.Close()
	if graceful {
		doneCh := sc.ShellProc.DoneCh
		sc.Lock.Unlock() // Unlock before waiting
		<-doneCh
		sc.Lock.Lock() // Re-lock after waiting
	}

	// Update status
	sc.ProcStatus = newStatus
	sc.sendUpdate_nolock()
}

func (sc *ShellController) getRuntimeStatus_nolock() BlockControllerRuntimeStatus {
	var rtn BlockControllerRuntimeStatus
	rtn.Version = sc.VersionTs.GetVersionTs()
	rtn.BlockId = sc.BlockId
	rtn.ShellProcStatus = sc.ProcStatus
	rtn.ShellProcConnName = sc.ConnName
	rtn.ShellProcExitCode = sc.ProcExitCode
	return rtn
}

func (sc *ShellController) GetRuntimeStatus() *BlockControllerRuntimeStatus {
	var rtn BlockControllerRuntimeStatus
	sc.WithLock(func() {
		rtn = sc.getRuntimeStatus_nolock()
	})
	return &rtn
}

func (sc *ShellController) GetConnName() string {
	return sc.ConnName
}

func (sc *ShellController) SendInput(inputUnion *BlockInputUnion) error {
	var shellInputCh chan *BlockInputUnion
	sc.WithLock(func() {
		shellInputCh = sc.ShellInputCh
	})
	if shellInputCh == nil {
		return fmt.Errorf("no shell input chan")
	}
	shellInputCh <- inputUnion
	return nil
}

func (sc *ShellController) WithLock(f func()) {
	sc.Lock.Lock()
	defer sc.Lock.Unlock()
	f()
}

type RunShellOpts struct {
	TermSize remotetermobj.TermSize `json:"termsize,omitempty"`
}

// only call when holding the lock
func (sc *ShellController) sendUpdate_nolock() {
	rtStatus := sc.getRuntimeStatus_nolock()
	log.Printf("sending blockcontroller update %#v\n", rtStatus)
	wps.Broker.Publish(wps.WaveEvent{
		Event: wps.Event_ControllerStatus,
		Scopes: []string{
			remotetermobj.MakeORef(remotetermobj.OType_Tab, sc.TabId).String(),
			remotetermobj.MakeORef(remotetermobj.OType_Block, sc.BlockId).String(),
		},
		Data: rtStatus,
	})
}

func (sc *ShellController) UpdateControllerAndSendUpdate(updateFn func() bool) {
	var sendUpdate bool
	sc.WithLock(func() {
		sendUpdate = updateFn()
	})
	if sendUpdate {
		rtStatus := sc.GetRuntimeStatus()
		log.Printf("sending blockcontroller update %#v\n", rtStatus)
		wps.Broker.Publish(wps.WaveEvent{
			Event: wps.Event_ControllerStatus,
			Scopes: []string{
				remotetermobj.MakeORef(remotetermobj.OType_Tab, sc.TabId).String(),
				remotetermobj.MakeORef(remotetermobj.OType_Block, sc.BlockId).String(),
			},
			Data: rtStatus,
		})
	}
}

func (sc *ShellController) resetTerminalState(logCtx context.Context) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	wfile, statErr := filestore.WFS.Stat(ctx, sc.BlockId, remotetermbase.BlockFile_Term)
	if statErr == fs.ErrNotExist {
		return
	}
	if statErr != nil {
		log.Printf("error statting term file: %v\n", statErr)
		return
	}
	if wfile.Size == 0 {
		return
	}
	blocklogger.Debugf(logCtx, "[conndebug] resetTerminalState: resetting terminal state\n")
	resetSeq := shellutil.GetTerminalResetSeq()
	resetSeq += "\r\n"
	err := HandleAppendBlockFile(sc.BlockId, remotetermbase.BlockFile_Term, []byte(resetSeq))
	if err != nil {
		log.Printf("error appending to blockfile (terminal reset): %v\n", err)
	}
}

func (sc *ShellController) writeMutedMessageToTerminal(msg string) {
	if sc.BlockId == "" {
		return
	}
	fullMsg := "\x1b[90m" + msg + "\x1b[0m\r\n"
	err := HandleAppendBlockFile(sc.BlockId, remotetermbase.BlockFile_Term, []byte(fullMsg))
	if err != nil {
		log.Printf("error writing muted message to terminal (blockid=%s): %v", sc.BlockId, err)
	}
}

// [All the other existing private methods remain exactly the same - I'm not including them all here for brevity, but they would all be copied over with sc. replacing bc. throughout]

func (sc *ShellController) DoRunShellCommand(logCtx context.Context, rc *RunShellOpts, blockMeta remotetermobj.MetaMapType) error {
	blocklogger.Debugf(logCtx, "[conndebug] DoRunShellCommand\n")
	shellProc, err := sc.setupAndStartShellProcess(logCtx, rc, blockMeta)
	if err != nil {
		return err
	}
	return sc.manageRunningShellProcess(shellProc, rc, blockMeta)
}

// [Continue with all other methods, replacing bc with sc throughout...]

func (sc *ShellController) LockRunLock() bool {
	rtn := sc.RunLock.CompareAndSwap(false, true)
	if rtn {
		log.Printf("block %q run() lock\n", sc.BlockId)
	}
	return rtn
}

func (sc *ShellController) UnlockRunLock() {
	sc.RunLock.Store(false)
	log.Printf("block %q run() unlock\n", sc.BlockId)
}

func (sc *ShellController) run(logCtx context.Context, bdata *remotetermobj.Block, blockMeta map[string]any, rtOpts *remotetermobj.RuntimeOpts, force bool) {
	blocklogger.Debugf(logCtx, "[conndebug] ShellController.run() %q\n", sc.BlockId)
	runningShellCommand := false
	ok := sc.LockRunLock()
	if !ok {
		log.Printf("block %q is already executing run()\n", sc.BlockId)
		return
	}
	defer func() {
		if !runningShellCommand {
			sc.UnlockRunLock()
		}
	}()
	curStatus := sc.GetRuntimeStatus()
	controllerName := bdata.Meta.GetString(remotetermobj.MetaKey_Controller, "")
	if controllerName != BlockController_Shell && controllerName != BlockController_Cmd {
		log.Printf("unknown controller %q\n", controllerName)
		return
	}
	runOnce := getBoolFromMeta(blockMeta, remotetermobj.MetaKey_CmdRunOnce, false)
	runOnStart := getBoolFromMeta(blockMeta, remotetermobj.MetaKey_CmdRunOnStart, true)
	if ((runOnStart || runOnce) && curStatus.ShellProcStatus == Status_Init) || force {
		if getBoolFromMeta(blockMeta, remotetermobj.MetaKey_CmdClearOnStart, false) {
			err := HandleTruncateBlockFile(sc.BlockId)
			if err != nil {
				log.Printf("error truncating term blockfile: %v\n", err)
			}
		}
		if runOnce {
			ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancelFn()
			metaUpdate := map[string]any{
				remotetermobj.MetaKey_CmdRunOnce:    false,
				remotetermobj.MetaKey_CmdRunOnStart: false,
			}
			err := rtstore.UpdateObjectMeta(ctx, remotetermobj.MakeORef(remotetermobj.OType_Block, sc.BlockId), metaUpdate, false)
			if err != nil {
				log.Printf("error updating block meta (in blockcontroller.run): %v\n", err)
				return
			}
		}
		runningShellCommand = true
		go func() {
			defer func() {
				panichandler.PanicHandler("blockcontroller:run-shell-command", recover())
			}()
			defer sc.UnlockRunLock()
			var termSize remotetermobj.TermSize
			if rtOpts != nil {
				termSize = rtOpts.TermSize
			} else {
				termSize = getTermSize(bdata)
			}
			err := sc.DoRunShellCommand(logCtx, &RunShellOpts{TermSize: termSize}, bdata.Meta)
			if err != nil {
				debugLog(logCtx, "error running shell: %v\n", err)
			}
		}()
	}
}

// [Include all the remaining private methods with bc replaced by sc]

type ConnUnion struct {
	ConnName   string
	ConnType   string
	SshConn    *conncontroller.SSHConn
	WslConn    *wslconn.WslConn
	WshEnabled bool
	ShellPath  string
	ShellOpts  []string
	ShellType  string
	HomeDir    string
}

func (bc *ShellController) getConnUnion(logCtx context.Context, remoteName string, blockMeta remotetermobj.MetaMapType) (ConnUnion, error) {
	rtn := ConnUnion{ConnName: remoteName}
	wshEnabled := !blockMeta.GetBool(remotetermobj.MetaKey_CmdNoWsh, false)
	if strings.HasPrefix(remoteName, "wsl://") {
		wslName := strings.TrimPrefix(remoteName, "wsl://")
		wslConn := wslconn.GetWslConn(wslName)
		if wslConn == nil {
			return ConnUnion{}, fmt.Errorf("wsl connection not found: %s", remoteName)
		}
		connStatus := wslConn.DeriveConnStatus()
		if connStatus.Status != conncontroller.Status_Connected {
			return ConnUnion{}, fmt.Errorf("wsl connection %s not connected, cannot start shellproc", remoteName)
		}
		rtn.ConnType = ConnType_Wsl
		rtn.WslConn = wslConn
		rtn.WshEnabled = wshEnabled && wslConn.WshEnabled.Load()
	} else if conncontroller.IsLocalConnName(remoteName) {
		rtn.ConnType = ConnType_Local
		rtn.WshEnabled = wshEnabled
	} else {
		opts, err := remote.ParseOpts(remoteName)
		if err != nil {
			return ConnUnion{}, fmt.Errorf("invalid ssh remote name (%s): %w", remoteName, err)
		}
		conn := conncontroller.MaybeGetConn(opts)
		if conn == nil {
			return ConnUnion{}, fmt.Errorf("ssh connection not found: %s", remoteName)
		}
		connStatus := conn.DeriveConnStatus()
		if connStatus.Status != conncontroller.Status_Connected {
			return ConnUnion{}, fmt.Errorf("ssh connection %s not connected, cannot start shellproc", remoteName)
		}
		rtn.ConnType = ConnType_Ssh
		rtn.SshConn = conn
		rtn.WshEnabled = wshEnabled && conn.WshEnabled.Load()
	}
	err := rtn.getRemoteInfoAndShellType(blockMeta)
	if err != nil {
		return ConnUnion{}, err
	}
	return rtn, nil
}

func (bc *ShellController) setupAndStartShellProcess(logCtx context.Context, rc *RunShellOpts, blockMeta remotetermobj.MetaMapType) (*shellexec.ShellProc, error) {
	// create a circular blockfile for the output
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	fsErr := filestore.WFS.MakeFile(ctx, bc.BlockId, remotetermbase.BlockFile_Term, nil, wshrpc.FileOpts{MaxSize: DefaultTermMaxFileSize, Circular: true})
	if fsErr != nil && fsErr != fs.ErrExist {
		return nil, fmt.Errorf("error creating blockfile: %w", fsErr)
	}
	if fsErr == fs.ErrExist {
		// reset the terminal state
		bc.resetTerminalState(logCtx)
	}
	bcInitStatus := bc.GetRuntimeStatus()
	if bcInitStatus.ShellProcStatus == Status_Running {
		return nil, nil
	}
	// TODO better sync here (don't let two starts happen at the same times)
	remoteName := blockMeta.GetString(remotetermobj.MetaKey_Connection, "")
	connUnion, err := bc.getConnUnion(logCtx, remoteName, blockMeta)
	if err != nil {
		return nil, err
	}
	blocklogger.Infof(logCtx, "[conndebug] remoteName: %q, connType: %s, wshEnabled: %v, shell: %q, shellType: %s\n", remoteName, connUnion.ConnType, connUnion.WshEnabled, connUnion.ShellPath, connUnion.ShellType)
	var cmdStr string
	var cmdOpts shellexec.CommandOptsType
	if bc.ControllerType == BlockController_Shell {
		cmdOpts.Interactive = true
		cmdOpts.Login = true
		cmdOpts.Cwd = blockMeta.GetString(remotetermobj.MetaKey_CmdCwd, "")
		if cmdOpts.Cwd != "" {
			cwdPath, err := remotetermbase.ExpandHomeDir(cmdOpts.Cwd)
			if err != nil {
				return nil, err
			}
			cmdOpts.Cwd = cwdPath
		}
	} else if bc.ControllerType == BlockController_Cmd {
		var cmdOptsPtr *shellexec.CommandOptsType
		cmdStr, cmdOptsPtr, err = createCmdStrAndOpts(bc.BlockId, blockMeta, remoteName)
		if err != nil {
			return nil, err
		}
		cmdOpts = *cmdOptsPtr
	} else {
		return nil, fmt.Errorf("unknown controller type %q", bc.ControllerType)
	}
	var shellProc *shellexec.ShellProc
	swapToken := makeSwapToken(ctx, logCtx, bc.BlockId, blockMeta, remoteName, connUnion.ShellType)
	cmdOpts.SwapToken = swapToken
	blocklogger.Debugf(logCtx, "[conndebug] created swaptoken: %s\n", swapToken.Token)
	if connUnion.ConnType == ConnType_Wsl {
		wslConn := connUnion.WslConn
		if !connUnion.WshEnabled {
			shellProc, err = shellexec.StartWslShellProcNoWsh(ctx, rc.TermSize, cmdStr, cmdOpts, wslConn)
			if err != nil {
				return nil, err
			}
		} else {
			sockName := wslConn.GetDomainSocketName()
			rpcContext := wshrpc.RpcContext{
				ProcRoute: true,
				SockName:  sockName,
				BlockId:   bc.BlockId,
				Conn:      wslConn.GetName(),
			}
			jwtStr, err := wshutil.MakeClientJWTToken(rpcContext)
			if err != nil {
				return nil, fmt.Errorf("error making jwt token: %w", err)
			}
			swapToken.RpcContext = &rpcContext
			remotetermbase.SetDualEnv(swapToken.Env, wshutil.WaveJwtTokenVarName, wshutil.LegacyWaveJwtTokenVarName, jwtStr)
			shellProc, err = shellexec.StartWslShellProc(ctx, rc.TermSize, cmdStr, cmdOpts, wslConn)
			if err != nil {
				wslConn.SetWshError(err)
				wslConn.WshEnabled.Store(false)
				blocklogger.Infof(logCtx, "[conndebug] error starting wsl shell proc with wsh: %v\n", err)
				blocklogger.Infof(logCtx, "[conndebug] attempting install without wsh\n")
				shellProc, err = shellexec.StartWslShellProcNoWsh(ctx, rc.TermSize, cmdStr, cmdOpts, wslConn)
				if err != nil {
					return nil, err
				}
			}
		}
	} else if connUnion.ConnType == ConnType_Ssh {
		conn := connUnion.SshConn
		if !connUnion.WshEnabled {
			shellProc, err = shellexec.StartRemoteShellProcNoWsh(ctx, rc.TermSize, cmdStr, cmdOpts, conn)
			if err != nil {
				return nil, err
			}
		} else {
			sockName := conn.GetDomainSocketName()
			rpcContext := wshrpc.RpcContext{
				ProcRoute: true,
				SockName:  sockName,
				BlockId:   bc.BlockId,
				Conn:      conn.Opts.String(),
			}
			jwtStr, err := wshutil.MakeClientJWTToken(rpcContext)
			if err != nil {
				return nil, fmt.Errorf("error making jwt token: %w", err)
			}
			swapToken.RpcContext = &rpcContext
			remotetermbase.SetDualEnv(swapToken.Env, wshutil.WaveJwtTokenVarName, wshutil.LegacyWaveJwtTokenVarName, jwtStr)
			shellProc, err = shellexec.StartRemoteShellProc(ctx, logCtx, rc.TermSize, cmdStr, cmdOpts, conn)
			if err != nil {
				conn.SetWshError(err)
				conn.WshEnabled.Store(false)
				blocklogger.Infof(logCtx, "[conndebug] error starting remote shell proc with wsh: %v\n", err)
				blocklogger.Infof(logCtx, "[conndebug] attempting install without wsh\n")
				shellProc, err = shellexec.StartRemoteShellProcNoWsh(ctx, rc.TermSize, cmdStr, cmdOpts, conn)
				if err != nil {
					return nil, err
				}
			}
		}
	} else if connUnion.ConnType == ConnType_Local {
		if connUnion.WshEnabled {
			sockName := remotetermbase.GetDomainSocketName()
			rpcContext := wshrpc.RpcContext{
				ProcRoute: true,
				SockName:  sockName,
				BlockId:   bc.BlockId,
			}
			jwtStr, err := wshutil.MakeClientJWTToken(rpcContext)
			if err != nil {
				return nil, fmt.Errorf("error making jwt token: %w", err)
			}
			swapToken.RpcContext = &rpcContext
			remotetermbase.SetDualEnv(swapToken.Env, wshutil.WaveJwtTokenVarName, wshutil.LegacyWaveJwtTokenVarName, jwtStr)
		}
		cmdOpts.ShellPath = connUnion.ShellPath
		cmdOpts.ShellOpts = getLocalShellOpts(blockMeta)
		shellProc, err = shellexec.StartLocalShellProc(logCtx, rc.TermSize, cmdStr, cmdOpts, remoteName)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unknown connection type for conn %q: %s", remoteName, connUnion.ConnType)
	}
	bc.UpdateControllerAndSendUpdate(func() bool {
		bc.ShellProc = shellProc
		bc.ProcStatus = Status_Running
		return true
	})
	return shellProc, nil
}

func (bc *ShellController) manageRunningShellProcess(shellProc *shellexec.ShellProc, rc *RunShellOpts, blockMeta remotetermobj.MetaMapType) error {
	shellInputCh := make(chan *BlockInputUnion, 32)
	bc.ShellInputCh = shellInputCh

	go func() {
		// handles regular output from the pty (goes to the blockfile and xterm)
		defer func() {
			panichandler.PanicHandler("blockcontroller:shellproc-pty-read-loop", recover())
		}()
		defer func() {
			log.Printf("[shellproc] pty-read loop done\n")
			shellProc.Close()
			bc.WithLock(func() {
				// so no other events are sent
				bc.ShellInputCh = nil
			})
			shellProc.Cmd.Wait()
			exitCode := shellProc.Cmd.ExitCode()
			blockData := bc.getBlockData_noErr()
			if blockData != nil && blockData.Meta.GetString(remotetermobj.MetaKey_Controller, "") == BlockController_Cmd {
				termMsg := fmt.Sprintf("\r\nprocess finished with exit code = %d\r\n\r\n", exitCode)
				HandleAppendBlockFile(bc.BlockId, remotetermbase.BlockFile_Term, []byte(termMsg))
			}
			// to stop the inputCh loop
			time.Sleep(100 * time.Millisecond)
			close(shellInputCh) // don't use bc.ShellInputCh (it's nil)
		}()
		buf := make([]byte, 4096)
		for {
			nr, err := shellProc.Cmd.Read(buf)
			if nr > 0 {
				err := HandleAppendBlockFile(bc.BlockId, remotetermbase.BlockFile_Term, buf[:nr])
				if err != nil {
					log.Printf("error appending to blockfile: %v\n", err)
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("error reading from shell: %v\n", err)
				break
			}
		}
	}()
	go func() {
		// handles input from the shellInputCh, sent to pty
		// use shellInputCh instead of bc.ShellInputCh (because we want to be attached to *this* ch.  bc.ShellInputCh can be updated)
		defer func() {
			panichandler.PanicHandler("blockcontroller:shellproc-input-loop", recover())
		}()
		for ic := range shellInputCh {
			if len(ic.InputData) > 0 {
				shellProc.Cmd.Write(ic.InputData)
			}
			if ic.TermSize != nil {
				updateTermSize(shellProc, bc.BlockId, *ic.TermSize)
			}
		}
	}()
	go func() {
		defer func() {
			panichandler.PanicHandler("blockcontroller:shellproc-wait-loop", recover())
		}()
		// wait for the shell to finish
		var exitCode int
		defer func() {
			bc.UpdateControllerAndSendUpdate(func() bool {
				if bc.ProcStatus == Status_Running {
					bc.ProcStatus = Status_Done
				}
				bc.ProcExitCode = exitCode
				return true
			})
			log.Printf("[shellproc] shell process wait loop done\n")
		}()
		waitErr := shellProc.Cmd.Wait()
		exitCode = shellProc.Cmd.ExitCode()
		shellProc.SetWaitErrorAndSignalDone(waitErr)
		bc.resetTerminalState(context.Background())
		exitSignal := shellProc.Cmd.ExitSignal()
		var baseMsg string
		if bc.ControllerType == BlockController_Shell {
			baseMsg = "shell terminated - press enter to close"
		} else {
			baseMsg = "command exited - press enter to close"
		}
		msg := baseMsg
		if exitSignal != "" {
			msg = fmt.Sprintf("shell terminated (signal %s) - press enter to close", exitSignal)
		} else if exitCode != 0 {
			msg = fmt.Sprintf("shell terminated (exit code %d) - press enter to close", exitCode)
		}
		bc.writeMutedMessageToTerminal("[" + msg + "]")
		go checkCloseOnExit(bc.BlockId, exitCode)
	}()
	return nil
}

func (union *ConnUnion) getRemoteInfoAndShellType(blockMeta remotetermobj.MetaMapType) error {
	if !union.WshEnabled {
		return nil
	}
	if union.ConnType == ConnType_Ssh || union.ConnType == ConnType_Wsl {
		connRoute := wshutil.MakeConnectionRouteId(union.ConnName)
		remoteInfo, err := wshclient.RemoteGetInfoCommand(wshclient.GetBareRpcClient(), &wshrpc.RpcOpts{Route: connRoute, Timeout: 2000})
		if err != nil {
			// weird error, could flip the wshEnabled flag and allow it to go forward, but the connection should have already been vetted
			return fmt.Errorf("unable to obtain remote info from connserver: %w", err)
		}
		// TODO allow overriding remote shell path
		union.ShellPath = remoteInfo.Shell
		union.HomeDir = remoteInfo.HomeDir
	} else {
		shellPath, err := getLocalShellPath(blockMeta)
		if err != nil {
			return err
		}
		union.ShellPath = shellPath
		union.HomeDir = remotetermbase.GetHomeDir()
	}
	union.ShellType = shellutil.GetShellTypeFromShellPath(union.ShellPath)
	return nil
}

func checkCloseOnExit(blockId string, exitCode int) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	blockData, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, blockId)
	if err != nil {
		log.Printf("error getting block data: %v\n", err)
		return
	}
	closeOnExit := blockData.Meta.GetBool(remotetermobj.MetaKey_CmdCloseOnExit, false)
	closeOnExitForce := blockData.Meta.GetBool(remotetermobj.MetaKey_CmdCloseOnExitForce, false)
	if !closeOnExitForce && !(closeOnExit && exitCode == 0) {
		return
	}
	delayMs := blockData.Meta.GetFloat(remotetermobj.MetaKey_CmdCloseOnExitDelay, 2000)
	if delayMs < 0 {
		delayMs = 0
	}
	time.Sleep(time.Duration(delayMs) * time.Millisecond)
	rpcClient := wshclient.GetBareRpcClient()
	err = wshclient.DeleteBlockCommand(rpcClient, wshrpc.CommandDeleteBlockData{BlockId: blockId}, nil)
	if err != nil {
		log.Printf("error deleting block data (close on exit): %v\n", err)
	}
}

func getLocalShellPath(blockMeta remotetermobj.MetaMapType) (string, error) {
	shellPath := blockMeta.GetString(remotetermobj.MetaKey_TermLocalShellPath, "")
	if shellPath != "" {
		return shellPath, nil
	}

	connName := blockMeta.GetString(remotetermobj.MetaKey_Connection, "")
	if strings.HasPrefix(connName, "local:") {
		variant := strings.TrimPrefix(connName, "local:")
		if variant == LocalConnVariant_GitBash {
			if runtime.GOOS != "windows" {
				return "", fmt.Errorf("connection \"local:gitbash\" is only supported on Windows")
			}
			fullConfig := rtconfig.GetWatcher().GetFullConfig()
			gitBashPath := shellutil.FindGitBash(&fullConfig, false)
			if gitBashPath == "" {
				return "", fmt.Errorf("connection \"local:gitbash\": git bash not found on this system, please install Git for Windows or set term:localshellpath to specify the git bash location")
			}
			return gitBashPath, nil
		}
		return "", fmt.Errorf("unsupported local connection type: %q", connName)
	}

	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	if settings.TermLocalShellPath != "" {
		return settings.TermLocalShellPath, nil
	}
	return shellutil.DetectLocalShellPath(), nil
}

func getLocalShellOpts(blockMeta remotetermobj.MetaMapType) []string {
	if blockMeta.HasKey(remotetermobj.MetaKey_TermLocalShellOpts) {
		opts := blockMeta.GetStringList(remotetermobj.MetaKey_TermLocalShellOpts)
		return append([]string{}, opts...)
	}
	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	if len(settings.TermLocalShellOpts) > 0 {
		return append([]string{}, settings.TermLocalShellOpts...)
	}
	return nil
}

// for "cmd" type blocks
func createCmdStrAndOpts(blockId string, blockMeta remotetermobj.MetaMapType, connName string) (string, *shellexec.CommandOptsType, error) {
	var cmdStr string
	var cmdOpts shellexec.CommandOptsType
	cmdStr = blockMeta.GetString(remotetermobj.MetaKey_Cmd, "")
	if cmdStr == "" {
		return "", nil, fmt.Errorf("missing cmd in block meta")
	}
	cmdOpts.Cwd = blockMeta.GetString(remotetermobj.MetaKey_CmdCwd, "")
	if cmdOpts.Cwd != "" {
		cwdPath, err := remotetermbase.ExpandHomeDir(cmdOpts.Cwd)
		if err != nil {
			return "", nil, err
		}
		cmdOpts.Cwd = cwdPath
	}
	useShell := blockMeta.GetBool(remotetermobj.MetaKey_CmdShell, true)
	if !useShell {
		if strings.Contains(cmdStr, " ") {
			return "", nil, fmt.Errorf("cmd should not have spaces if cmd:shell is false (use cmd:args)")
		}
		cmdArgs := blockMeta.GetStringList(remotetermobj.MetaKey_CmdArgs)
		// shell escape the args
		for _, arg := range cmdArgs {
			cmdStr = cmdStr + " " + utilfn.ShellQuote(arg, false, -1)
		}
	}
	cmdOpts.ForceJwt = blockMeta.GetBool(remotetermobj.MetaKey_CmdJwt, false)
	return cmdStr, &cmdOpts, nil
}

func (bc *ShellController) getBlockData_noErr() *remotetermobj.Block {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	blockData, err := rtstore.DBGet[*remotetermobj.Block](ctx, bc.BlockId)
	if err != nil {
		log.Printf("error getting block data (getBlockData_noErr): %v\n", err)
		return nil
	}
	return blockData
}

func resolveEnvMap(blockId string, blockMeta remotetermobj.MetaMapType, connName string) (map[string]string, error) {
	rtn := make(map[string]string)
	config := rtconfig.GetWatcher().GetFullConfig()
	connKeywords := config.Connections[connName]
	ckEnv := connKeywords.CmdEnv
	for k, v := range ckEnv {
		rtn[k] = v
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	_, envFileData, err := filestore.WFS.ReadFile(ctx, blockId, remotetermbase.BlockFile_Env)
	if err == fs.ErrNotExist {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading command env file: %w", err)
	}
	if len(envFileData) > 0 {
		envMap := envutil.EnvToMap(string(envFileData))
		for k, v := range envMap {
			rtn[k] = v
		}
	}
	cmdEnv := blockMeta.GetStringMap(remotetermobj.MetaKey_CmdEnv, true)
	for k, v := range cmdEnv {
		if v == remotetermobj.MetaMap_DeleteSentinel {
			delete(rtn, k)
			continue
		}
		rtn[k] = v
	}
	connEnv := blockMeta.GetConnectionOverride(connName).GetStringMap(remotetermobj.MetaKey_CmdEnv, true)
	for k, v := range connEnv {
		if v == remotetermobj.MetaMap_DeleteSentinel {
			delete(rtn, k)
			continue
		}
		rtn[k] = v
	}
	return rtn, nil
}

func getCustomInitScriptKeyCascade(shellType string) []string {
	if shellType == "bash" {
		return []string{remotetermobj.MetaKey_CmdInitScriptBash, remotetermobj.MetaKey_CmdInitScriptSh, remotetermobj.MetaKey_CmdInitScript}
	}
	if shellType == "zsh" {
		return []string{remotetermobj.MetaKey_CmdInitScriptZsh, remotetermobj.MetaKey_CmdInitScriptSh, remotetermobj.MetaKey_CmdInitScript}
	}
	if shellType == "pwsh" {
		return []string{remotetermobj.MetaKey_CmdInitScriptPwsh, remotetermobj.MetaKey_CmdInitScript}
	}
	if shellType == "fish" {
		return []string{remotetermobj.MetaKey_CmdInitScriptFish, remotetermobj.MetaKey_CmdInitScript}
	}
	return []string{remotetermobj.MetaKey_CmdInitScript}
}

func getCustomInitScript(logCtx context.Context, meta remotetermobj.MetaMapType, connName string, shellType string) string {
	initScriptVal, metaKeyName := getCustomInitScriptValue(meta, connName, shellType)
	if initScriptVal == "" {
		return ""
	}
	if !fileutil.IsInitScriptPath(initScriptVal) {
		blocklogger.Infof(logCtx, "[conndebug] inline initScript (size=%d) found in meta key: %s\n", len(initScriptVal), metaKeyName)
		return initScriptVal
	}
	blocklogger.Infof(logCtx, "[conndebug] initScript detected as a file %q from meta key: %s\n", initScriptVal, metaKeyName)
	initScriptVal, err := remotetermbase.ExpandHomeDir(initScriptVal)
	if err != nil {
		blocklogger.Infof(logCtx, "[conndebug] cannot expand home dir in Wave initscript file: %v\n", err)
		return fmt.Sprintf("echo \"cannot expand home dir in Wave initscript file, from key %s\";\n", metaKeyName)
	}
	fileData, err := os.ReadFile(initScriptVal)
	if err != nil {
		blocklogger.Infof(logCtx, "[conndebug] cannot open Wave initscript file: %v\n", err)
		return fmt.Sprintf("echo \"cannot open Wave initscript file, from key %s\";\n", metaKeyName)
	}
	if len(fileData) > MaxInitScriptSize {
		blocklogger.Infof(logCtx, "[conndebug] initscript file too large, size=%d, max=%d\n", len(fileData), MaxInitScriptSize)
		return fmt.Sprintf("echo \"initscript file too large, from key %s\";\n", metaKeyName)
	}
	if utilfn.HasBinaryData(fileData) {
		blocklogger.Infof(logCtx, "[conndebug] initscript file contains binary data\n")
		return fmt.Sprintf("echo \"initscript file contains binary data, from key %s\";\n", metaKeyName)
	}
	blocklogger.Infof(logCtx, "[conndebug] initscript file read successfully, size=%d\n", len(fileData))
	return string(fileData)
}

// returns (value, metakey)
func getCustomInitScriptValue(meta remotetermobj.MetaMapType, connName string, shellType string) (string, string) {
	keys := getCustomInitScriptKeyCascade(shellType)
	connMeta := meta.GetConnectionOverride(connName)
	if connMeta != nil {
		for _, key := range keys {
			if connMeta.HasKey(key) {
				return connMeta.GetString(key, ""), "blockmeta/[" + connName + "]/" + key
			}
		}
	}
	for _, key := range keys {
		if meta.HasKey(key) {
			return meta.GetString(key, ""), "blockmeta/" + key
		}
	}
	fullConfig := rtconfig.GetWatcher().GetFullConfig()
	connKeywords := fullConfig.Connections[connName]
	connKeywordsMap := make(map[string]any)
	err := utilfn.ReUnmarshal(&connKeywordsMap, connKeywords)
	if err != nil {
		log.Printf("error re-unmarshalling connKeywords: %v\n", err)
		return "", ""
	}
	ckMeta := remotetermobj.MetaMapType(connKeywordsMap)
	for _, key := range keys {
		if ckMeta.HasKey(key) {
			return ckMeta.GetString(key, ""), "connections.json/" + connName + "/" + key
		}
	}
	return "", ""
}

func updateTermSize(shellProc *shellexec.ShellProc, blockId string, termSize remotetermobj.TermSize) {
	err := setTermSizeInDB(blockId, termSize)
	if err != nil {
		log.Printf("error setting pty size: %v\n", err)
	}
	err = shellProc.Cmd.SetSize(termSize.Rows, termSize.Cols)
	if err != nil {
		log.Printf("error setting pty size: %v\n", err)
	}
}

func setTermSizeInDB(blockId string, termSize remotetermobj.TermSize) error {
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	ctx = remotetermobj.ContextWithUpdates(ctx)
	bdata, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, blockId)
	if err != nil {
		return fmt.Errorf("error getting block data: %v", err)
	}
	if bdata.RuntimeOpts == nil {
		bdata.RuntimeOpts = &remotetermobj.RuntimeOpts{}
	}
	bdata.RuntimeOpts.TermSize = termSize
	err = rtstore.DBUpdate(ctx, bdata)
	if err != nil {
		return fmt.Errorf("error updating block data: %v", err)
	}
	updates := remotetermobj.ContextGetUpdatesRtn(ctx)
	wps.Broker.SendUpdateEvents(updates)
	return nil
}
