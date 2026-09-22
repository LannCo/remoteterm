// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/util/shellutil"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshclient"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:          "wsh",
		Short:        "CLI tool to control RemoteTerm",
		Long:         `wsh is a small utility that lets you do cool things with RemoteTerm, right from the command line`,
		SilenceUsage: true,
	}
)

var WrappedStdin io.Reader = os.Stdin
var WrappedStdout io.Writer = &WrappedWriter{dest: os.Stdout}
var WrappedStderr io.Writer = &WrappedWriter{dest: os.Stderr}
var RpcClient *wshutil.WshRpc
var RpcContext wshrpc.RpcContext
var RpcClientRouteId string
var UsingTermWshMode bool
var blockArg string
var WshExitCode int

type WrappedWriter struct {
	dest io.Writer
}

func (w *WrappedWriter) Write(p []byte) (n int, err error) {
	if !UsingTermWshMode {
		return w.dest.Write(p)
	}
	count := 0
	for _, b := range p {
		if b == '\n' {
			count++
		}
	}
	if count == 0 {
		return w.dest.Write(p)
	}
	buf := make([]byte, len(p)+count) // Each '\n' adds one extra byte for '\r'
	writeIdx := 0
	for _, b := range p {
		if b == '\n' {
			buf[writeIdx] = '\r'
			buf[writeIdx+1] = '\n'
			writeIdx += 2
		} else {
			buf[writeIdx] = b
			writeIdx++
		}
	}
	return w.dest.Write(buf)
}

func WriteStderr(fmtStr string, args ...interface{}) {
	WrappedStderr.Write([]byte(fmt.Sprintf(fmtStr, args...)))
}

func WriteStdout(fmtStr string, args ...interface{}) {
	WrappedStdout.Write([]byte(fmt.Sprintf(fmtStr, args...)))
}

func OutputHelpMessage(cmd *cobra.Command) {
	cmd.SetOutput(WrappedStderr)
	cmd.Help()
	WriteStderr("\n")
}

func preRunSetupRpcClient(cmd *cobra.Command, args []string) error {
	jwtToken := getEnvNewOrLegacy(wshutil.WaveJwtTokenVarName, wshutil.LegacyWaveJwtTokenVarName)
	if jwtToken == "" {
		return fmt.Errorf("wsh must be run inside a Wave-managed SSH session (%s not found)", wshutil.WaveJwtTokenVarName)
	}
	err := setupRpcClient(nil, jwtToken)
	if err != nil {
		return err
	}
	return nil
}

func getIsTty() bool {
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) != 0 {
		return true
	}
	return false
}

type RunEFnType = func(*cobra.Command, []string) error

func resolveBlockArg() (*remotetermobj.ORef, error) {
	oref := blockArg
	if oref == "" {
		oref = "this"
	}
	fullORef, err := resolveSimpleId(oref)
	if err != nil {
		return nil, fmt.Errorf("resolving blockid: %w", err)
	}
	return fullORef, nil
}

func setupRpcClientWithToken(swapTokenStr string) (wshrpc.CommandAuthenticateRtnData, error) {
	var rtn wshrpc.CommandAuthenticateRtnData
	token, err := shellutil.UnpackSwapToken(swapTokenStr)
	if err != nil {
		return rtn, fmt.Errorf("error unpacking token: %w", err)
	}
	if token.RpcContext == nil {
		return rtn, fmt.Errorf("no rpccontext in token")
	}
	if token.RpcContext.SockName == "" {
		return rtn, fmt.Errorf("no sockname in token")
	}
	RpcContext = *token.RpcContext
	RpcClient, err = wshutil.SetupDomainSocketRpcClient(token.RpcContext.SockName, nil, "wshcmd")
	if err != nil {
		return rtn, fmt.Errorf("error setting up domain socket rpc client: %w", err)
	}
	rtn, err = wshclient.AuthenticateTokenCommand(RpcClient, wshrpc.CommandAuthenticateTokenData{Token: token.Token}, &wshrpc.RpcOpts{Route: wshutil.ControlRoute})
	if err != nil {
		return rtn, err
	}
	RpcClientRouteId = rtn.RouteId
	return rtn, nil
}

// returns the wrapped stdin and a new rpc client (that wraps the stdin input and stdout output)
func setupRpcClient(serverImpl wshutil.ServerImpl, jwtToken string) error {
	rpcCtx, err := wshutil.ExtractUnverifiedRpcContext(jwtToken)
	if err != nil {
		return fmt.Errorf("error extracting rpc context from %s: %v", wshutil.WaveJwtTokenVarName, err)
	}
	RpcContext = *rpcCtx
	sockName, err := wshutil.ExtractUnverifiedSocketName(jwtToken)
	if err != nil {
		return fmt.Errorf("error extracting socket name from %s: %v", wshutil.WaveJwtTokenVarName, err)
	}
	RpcClient, err = wshutil.SetupDomainSocketRpcClient(sockName, serverImpl, "wshcmd")
	if err != nil {
		return fmt.Errorf("error setting up domain socket rpc client: %v", err)
	}
	authRtn, err := wshclient.AuthenticateCommand(RpcClient, jwtToken, &wshrpc.RpcOpts{Route: wshutil.ControlRoute})
	if err != nil {
		return fmt.Errorf("error authenticating: %v", err)
	}
	RpcClientRouteId = authRtn.RouteId
	blockId := getEnvNewOrLegacy("REMOTETERM_BLOCKID", "WAVETERM_BLOCKID")
	if blockId != "" {
		peerInfo := fmt.Sprintf("domain:block:%s", blockId)
		wshclient.SetPeerInfoCommand(RpcClient, peerInfo, &wshrpc.RpcOpts{Route: wshutil.ControlRoute})
	}
	// note we don't modify WrappedStdin here (just use os.Stdin)
	return nil
}

func isFullORef(orefStr string) bool {
	_, err := remotetermobj.ParseORef(orefStr)
	return err == nil
}

func resolveSimpleId(id string) (*remotetermobj.ORef, error) {
	if isFullORef(id) {
		orefObj, err := remotetermobj.ParseORef(id)
		if err != nil {
			return nil, fmt.Errorf("error parsing full ORef: %v", err)
		}
		return &orefObj, nil
	}
	blockId := getEnvNewOrLegacy("REMOTETERM_BLOCKID", "WAVETERM_BLOCKID")
	if blockId == "" {
		return nil, fmt.Errorf("no REMOTETERM_BLOCKID env var set")
	}
	rtnData, err := wshclient.ResolveIdsCommand(RpcClient, wshrpc.CommandResolveIdsData{
		BlockId: blockId,
		Ids:     []string{id},
	}, &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return nil, fmt.Errorf("error resolving ids: %v", err)
	}
	oref, ok := rtnData.ResolvedIds[id]
	if !ok {
		return nil, fmt.Errorf("id not found: %q", id)
	}
	return &oref, nil
}

// getEnvNewOrLegacy reads a session-scoped env var written by the app into a spawned shell's
// environment, preferring the new REMOTETERM_ name but falling back to the deprecated WAVETERM_
// name so wsh keeps working inside a shell pane spawned by a pre-rename app instance.
func getEnvNewOrLegacy(newName string, legacyName string) string {
	if val := os.Getenv(newName); val != "" {
		return val
	}
	return os.Getenv(legacyName)
}

func getTabIdFromEnv() string {
	return getEnvNewOrLegacy("REMOTETERM_TABID", "WAVETERM_TABID")
}

// Execute executes the root command.
func Execute() {
	defer func() {
		r := recover()
		if r != nil {
			WriteStderr("[panic] %v\n", r)
			debug.PrintStack()
			wshutil.DoShutdown("", 1, true)
		} else {
			wshutil.DoShutdown("", WshExitCode, false)
		}
	}()
	rootCmd.PersistentFlags().StringVarP(&blockArg, "block", "b", "", "for commands which require a block id")
	err := rootCmd.Execute()
	if err != nil {
		wshutil.DoShutdown("", 1, true)
		return
	}
}
