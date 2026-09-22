// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"runtime"
	"sync"
	"time"

	"github.com/LannCo/remoteterm/pkg/authkey"
	"github.com/LannCo/remoteterm/pkg/blockcontroller"
	"github.com/LannCo/remoteterm/pkg/blocklogger"
	"github.com/LannCo/remoteterm/pkg/filebackup"
	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/jobcontroller"
	"github.com/LannCo/remoteterm/pkg/panichandler"
	"github.com/LannCo/remoteterm/pkg/remote/fileshare/wshfs"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/service"
	"github.com/LannCo/remoteterm/pkg/util/envutil"
	"github.com/LannCo/remoteterm/pkg/util/shellutil"
	"github.com/LannCo/remoteterm/pkg/util/sigutil"
	"github.com/LannCo/remoteterm/pkg/web"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshclient"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshremote"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshserver"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/joho/godotenv"

	"net/http"
	_ "net/http/pprof"
)

// these are set at build time
var WaveVersion = "0.0.0"
var BuildTime = "0"

const BackupCleanupTick = 2 * time.Minute
const BackupCleanupInterval = 4 * time.Hour

var shutdownOnce sync.Once

func init() {
	envFilePath := os.Getenv("REMOTETERM_ENVFILE")
	if envFilePath != "" {
		log.Printf("applying env file: %s\n", envFilePath)
		_ = godotenv.Load(envFilePath)
	}
}

func doShutdown(reason string) {
	shutdownOnce.Do(func() {
		log.Printf("shutting down: %s\n", reason)
		ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelFn()
		go blockcontroller.StopAllBlockControllersForShutdown()
		// TODO deal with flush in progress
		clearTempFiles()
		filestore.WFS.FlushCache(ctx)
		watcher := rtconfig.GetWatcher()
		if watcher != nil {
			watcher.Close()
		}
		time.Sleep(500 * time.Millisecond)
		log.Printf("shutdown complete\n")
		os.Exit(0)
	})
}

// watch stdin, kill server if stdin is closed
func stdinReadWatch() {
	defer func() {
		panichandler.PanicHandler("stdinReadWatch", recover())
	}()
	buf := make([]byte, 1024)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			doShutdown(fmt.Sprintf("stdin closed/error (%v)", err))
			break
		}
	}
}

func startConfigWatcher() {
	watcher := rtconfig.GetWatcher()
	if watcher != nil {
		watcher.Start()
	}
}

func backupCleanupLoop() {
	defer func() {
		panichandler.PanicHandler("backupCleanupLoop", recover())
	}()
	var nextCleanup int64
	for {
		if time.Now().Unix() > nextCleanup {
			nextCleanup = time.Now().Add(BackupCleanupInterval).Unix()
			err := filebackup.CleanupOldBackups()
			if err != nil {
				log.Printf("error cleaning up old backups: %v\n", err)
			}
		}
		time.Sleep(BackupCleanupTick)
	}
}

func createMainWshClient() {
	rpc := wshserver.GetMainRpcClient()
	wshutil.DefaultRouter.RegisterTrustedLeaf(rpc, wshutil.DefaultRoute)
	wps.Broker.SetClient(wshutil.DefaultRouter)
	localInitialEnv := envutil.PruneInitialEnv(envutil.SliceToMap(os.Environ()))
	sockName := remotetermbase.GetDomainSocketName()
	remoteImpl := wshremote.MakeRemoteRpcServerImpl(nil, wshutil.DefaultRouter, wshclient.GetBareRpcClient(), true, localInitialEnv, sockName)
	localConnWsh := wshutil.MakeWshRpc(wshrpc.RpcContext{Conn: wshrpc.LocalConnName}, remoteImpl, "conn:local")
	go wshremote.RunSysInfoLoop(localConnWsh, wshrpc.LocalConnName)
	wshutil.DefaultRouter.RegisterTrustedLeaf(localConnWsh, wshutil.MakeConnectionRouteId(wshrpc.LocalConnName))
	wshfs.RpcClient = localConnWsh
	wshfs.RpcClientRouteId = wshutil.MakeConnectionRouteId(wshrpc.LocalConnName)
}

func grabAndRemoveEnvVars() error {
	err := authkey.SetAuthKeyFromEnv()
	if err != nil {
		return fmt.Errorf("setting auth key: %v", err)
	}
	err = remotetermbase.CacheAndRemoveEnvVars()
	if err != nil {
		return err
	}
	// Remove WAVETERM/REMOTETERM env vars that leak from prod => dev. Unset both names of each
	// dual-written session-scoped var (see remotetermbase.SetDualEnv) since a leaked pre-rename
	// process could still have the old name set.
	os.Unsetenv(remotetermbase.WaveClientIdVarName)
	os.Unsetenv(remotetermbase.LegacyWaveClientIdVarName)
	os.Unsetenv(remotetermbase.WaveWorkspaceIdVarName)
	os.Unsetenv(remotetermbase.LegacyWaveWorkspaceIdVarName)
	os.Unsetenv(remotetermbase.WaveTabIdVarName)
	os.Unsetenv(remotetermbase.LegacyWaveTabIdVarName)
	os.Unsetenv(remotetermbase.WaveBlockIdVarName)
	os.Unsetenv(remotetermbase.LegacyWaveBlockIdVarName)
	os.Unsetenv(remotetermbase.WaveConnVarName)
	os.Unsetenv(remotetermbase.LegacyWaveConnVarName)
	os.Unsetenv(remotetermbase.WaveJwtTokenVarName)
	os.Unsetenv(remotetermbase.LegacyWaveJwtTokenVarName)
	os.Unsetenv(remotetermbase.WaveVersionVarName)

	return nil
}

func clearTempFiles() error {
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	client, err := rtstore.DBGetSingleton[*remotetermobj.Client](ctx)
	if err != nil {
		return fmt.Errorf("error getting client: %v", err)
	}
	filestore.WFS.DeleteZone(ctx, client.TempOID)
	return nil
}

func maybeStartPprofServer() {
	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	if settings.DebugPprofMemProfileRate != nil {
		runtime.MemProfileRate = *settings.DebugPprofMemProfileRate
		log.Printf("set runtime.MemProfileRate to %d\n", runtime.MemProfileRate)
	}
	if settings.DebugPprofPort == nil {
		return
	}
	pprofPort := *settings.DebugPprofPort
	if pprofPort < 1 || pprofPort > 65535 {
		log.Printf("[error] debug:pprofport must be between 1 and 65535, got %d\n", pprofPort)
		return
	}
	go func() {
		addr := fmt.Sprintf("localhost:%d", pprofPort)
		log.Printf("starting pprof server on %s\n", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("[error] pprof server failed: %v\n", err)
		}
	}()
}

func main() {
	log.SetFlags(0) // disable timestamp since electron's winston logger already wraps with timestamp
	log.SetPrefix("[wavesrv] ")
	remotetermbase.WaveVersion = WaveVersion
	remotetermbase.BuildTime = BuildTime
	wshutil.DefaultRouter = wshutil.NewWshRouter()
	wshutil.DefaultRouter.SetAsRootRouter()

	err := grabAndRemoveEnvVars()
	if err != nil {
		log.Printf("[error] %v\n", err)
		return
	}
	err = service.ValidateServiceMap()
	if err != nil {
		log.Printf("error validating service map: %v\n", err)
		return
	}
	err = remotetermbase.EnsureWaveDataDir()
	if err != nil {
		log.Printf("error ensuring wave home dir: %v\n", err)
		return
	}
	err = remotetermbase.EnsureWaveDBDir()
	if err != nil {
		log.Printf("error ensuring wave db dir: %v\n", err)
		return
	}
	err = remotetermbase.EnsureWaveConfigDir()
	if err != nil {
		log.Printf("error ensuring wave config dir: %v\n", err)
		return
	}

	// TODO: rather than ensure this dir exists, we should let the editor recursively create parent dirs on save
	err = remotetermbase.EnsureWavePresetsDir()
	if err != nil {
		log.Printf("error ensuring wave presets dir: %v\n", err)
		return
	}
	err = remotetermbase.EnsureWaveCachesDir()
	if err != nil {
		log.Printf("error ensuring wave caches dir: %v\n", err)
		return
	}
	waveLock, err := remotetermbase.AcquireWaveLock()
	if err != nil {
		log.Printf("error acquiring wave lock (another instance of Wave is likely running): %v\n", err)
		return
	}
	defer func() {
		err = waveLock.Close()
		if err != nil {
			log.Printf("error releasing wave lock: %v\n", err)
		}
	}()
	log.Printf("wave version: %s (%s)\n", WaveVersion, BuildTime)
	log.Printf("wave data dir: %s\n", remotetermbase.GetWaveDataDir())
	log.Printf("wave config dir: %s\n", remotetermbase.GetWaveConfigDir())
	err = filestore.InitFilestore()
	if err != nil {
		log.Printf("error initializing filestore: %v\n", err)
		return
	}
	err = rtstore.InitWStore()
	if err != nil {
		log.Printf("error initializing rtstore: %v\n", err)
		return
	}
	go func() {
		defer func() {
			panichandler.PanicHandler("InitCustomShellStartupFiles", recover())
		}()
		err := shellutil.InitCustomShellStartupFiles()
		if err != nil {
			log.Printf("error initializing wsh and shell-integration files: %v\n", err)
		}
	}()
	firstLaunch, err := rtcore.EnsureInitialData()
	if err != nil {
		log.Printf("error ensuring initial data: %v\n", err)
		return
	}
	if firstLaunch {
		log.Printf("first launch detected")
	}
	err = clearTempFiles()
	if err != nil {
		log.Printf("error clearing temp files: %v\n", err)
		return
	}
	err = rtcore.InitMainServer()
	if err != nil {
		log.Printf("error initializing mainserver: %v\n", err)
		return
	}

	err = shellutil.FixupWaveZshHistory()
	if err != nil {
		log.Printf("error fixing up wave zsh history: %v\n", err)
	}
	createMainWshClient()
	sigutil.InstallShutdownSignalHandlers(doShutdown)
	sigutil.InstallSIGUSR1Handler()
	rtconfig.MigratePresetsBackgrounds()
	startConfigWatcher()
	maybeStartPprofServer()
	go stdinReadWatch()
	go backupCleanupLoop()
	blocklogger.InitBlockLogger()
	jobcontroller.InitJobController()
	blockcontroller.InitBlockController()
	go blockcontroller.StartupReconnectDurableShells(context.Background())
	err = rtcore.InitBadgeStore()
	if err != nil {
		log.Printf("error initializing badge store: %v\n", err)
		return
	}
	go func() {
		defer func() {
			panichandler.PanicHandler("GetSystemSummary", recover())
		}()
		remotetermbase.GetSystemSummary()
	}()

	webListener, err := web.MakeTCPListener("web")
	if err != nil {
		log.Printf("error creating web listener: %v\n", err)
		return
	}
	wsListener, err := web.MakeTCPListener("websocket")
	if err != nil {
		log.Printf("error creating websocket listener: %v\n", err)
		return
	}
	go web.RunWebSocketServer(wsListener)
	unixListener, err := web.MakeUnixListener()
	if err != nil {
		log.Printf("error creating unix listener: %v\n", err)
		return
	}
	go func() {
		if BuildTime == "" {
			BuildTime = "0"
		}
		// use fmt instead of log here to make sure it goes directly to stderr
		fmt.Fprintf(os.Stderr, "WAVESRV-ESTART ws:%s web:%s version:%s buildtime:%s\n", wsListener.Addr(), webListener.Addr(), WaveVersion, BuildTime)
	}()
	go wshutil.RunWshRpcOverListener(unixListener, nil)
	web.RunWebServer(webListener) // blocking
	runtime.KeepAlive(waveLock)
}
