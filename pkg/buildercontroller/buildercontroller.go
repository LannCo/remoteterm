// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LannCo/remoteterm/pkg/panichandler"
	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/LannCo/remoteterm/pkg/remotetermapputil"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/tsunamiutil"
	"github.com/LannCo/remoteterm/pkg/utilds"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/tsunami/build"
)

const (
	BuilderStatus_Init     = "init"
	BuilderStatus_Building = "building"
	BuilderStatus_Running  = "running"
	BuilderStatus_Error    = "error"
	BuilderStatus_Stopped  = "stopped"

	BuildLogFileName    = remotetermappstore.BuildLogFile
	BuildLogStatusError = "status: error"
)

type BuilderProcess struct {
	Cmd         *exec.Cmd
	StdinWriter io.WriteCloser
	Port        int
	WaitCh      chan struct{}
	WaitRtn     error

	// Handed to the app as TSUNAMI_AUTHTOKEN and to the builder's own window, never broadcast.
	PreviewToken string
}

type BuildResult struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"errormessage,omitempty"`
	BuildOutput  string `json:"buildoutput"`
}

type BuilderController struct {
	lock          sync.Mutex
	builderId     string
	appId         string
	process       *BuilderProcess
	outputBuffer  *utilds.MultiReaderLineBuffer
	statusLock    sync.Mutex
	status        string
	statusVersion int
	port          int
	exitCode      int
	errorMsg      string

	// Atomic so the publish paths, some of which run under bc.lock, can read it lock-free.
	closed             atomic.Bool
	buildCancel        context.CancelFunc
	stopping           int
	building           bool
	rebuildPending     bool
	pendingAppId       string
	pendingEnv         map[string]string
	lastBuildInputHash string
	lastAnnouncedHash  string
	trust              buildTrust
	watcher            *AppWatcher
	runBuildFn         func(ctx context.Context, appId string, builderEnv map[string]string)
}

var (
	controllerMap = make(map[string]*BuilderController) // key is builderid
	mapLock       sync.Mutex
)

// Tests swap this to count or block the builds of controllers the package creates itself.
var runBuildAndRun = func(bc *BuilderController, ctx context.Context, appId string, builderEnv map[string]string) {
	bc.buildAndRun(ctx, appId, builderEnv, nil)
}

func GetOrCreateController(builderId string) *BuilderController {
	mapLock.Lock()
	defer mapLock.Unlock()

	bc := controllerMap[builderId]
	if bc != nil {
		return bc
	}

	bc = makeBuilderController(builderId)
	controllerMap[builderId] = bc

	return bc
}

func makeBuilderController(builderId string) *BuilderController {
	bc := &BuilderController{
		builderId: builderId,
		status:    BuilderStatus_Init,
	}
	runBuild := runBuildAndRun
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		runBuild(bc, ctx, appId, builderEnv)
	}
	return bc
}

func GetController(builderId string) *BuilderController {
	mapLock.Lock()
	defer mapLock.Unlock()
	return controllerMap[builderId]
}

func DeleteController(builderId string) {
	mapLock.Lock()
	bc := controllerMap[builderId]
	delete(controllerMap, builderId)
	mapLock.Unlock()

	if bc == nil {
		return
	}
	bc.markClosed()
	bc.StopWatching()
	// Stop waits for a running build, which can outlast the caller's RPC timeout; the
	// controller is already out of the map and closed, so the caller need not wait. The
	// app process is still killed once the build ends.
	go func() {
		defer func() {
			panichandler.PanicHandler(fmt.Sprintf("buildercontroller[%s].Stop", builderId), recover())
		}()
		bc.Stop()
	}()
}

func GetBuilderAppExecutablePath(appPath string) (string, error) {
	binDir := filepath.Join(appPath, "bin")

	binaryName := "app"
	if runtime.GOOS == "windows" {
		binaryName = "app.exe"
	}
	binPath := filepath.Join(binDir, binaryName)

	err := remotetermbase.TryMkdirs(binDir, 0755, "app bin directory")
	if err != nil {
		return "", fmt.Errorf("failed to create app bin directory: %w", err)
	}

	return binPath, nil
}

func Shutdown() {
	mapLock.Lock()
	controllers := make([]*BuilderController, 0, len(controllerMap))
	for _, bc := range controllerMap {
		controllers = append(controllers, bc)
	}
	mapLock.Unlock()

	for _, bc := range controllers {
		bc.markClosed()
		bc.StopWatching()
		bc.Stop()
	}
}

func (bc *BuilderController) waitForBuildDone(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !bc.isBuilding() {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (bc *BuilderController) Start(ctx context.Context, appId string, builderEnv map[string]string) error {
	bc.RequestUserRebuild(appId, builderEnv)
	return nil
}

// RequestRebuild never waits for a build, so the RPC that calls it returns at once and
// the RPC timeout never applies to a build. Requests that arrive during a build
// collapse into a single follow-up build. It is for rebuilds the user did not ask for
// (the watcher's live rebuild): it does not mark the inputs as trusted.
func (bc *BuilderController) RequestRebuild(appId string, builderEnv map[string]string) {
	bc.requestRebuild(appId, builderEnv, false)
}

func (bc *BuilderController) requestRebuild(appId string, builderEnv map[string]string, userInitiated bool) {
	if !bc.acceptsRequests() {
		return
	}
	if userInitiated {
		bc.trustCurrentInputs(appId)
	}
	bc.recordInputHash(appId)
	if !bc.queueBuild(appId, builderEnv) {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panichandler.PanicHandler(fmt.Sprintf("buildercontroller[%s].buildLoop", bc.builderId), r)
				bc.abandonBuildLoop(r)
			}
		}()
		bc.buildLoop()
	}()
}

func (bc *BuilderController) queueBuild(appId string, builderEnv map[string]string) bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.closed.Load() || bc.stopping > 0 {
		return false
	}
	bc.pendingAppId = appId
	bc.pendingEnv = builderEnv
	if bc.building {
		bc.rebuildPending = true
		return false
	}
	bc.building = true
	return true
}

func (bc *BuilderController) buildLoop() {
	for {
		appId, builderEnv, ok := bc.beginBuild()
		if !ok {
			return
		}
		bc.recordInputHash(appId)
		bc.runOneBuild(appId, builderEnv)
		bc.refreshTrustAfterBuild(appId)
		if !bc.endBuild() {
			return
		}
	}
}

func (bc *BuilderController) runOneBuild(appId string, builderEnv map[string]string) {
	buildCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	bc.setBuildCancel(cancel)
	defer bc.setBuildCancel(nil)
	bc.runBuildFn(buildCtx, appId, builderEnv)
}

func (bc *BuilderController) beginBuild() (string, map[string]string, bool) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.closed.Load() {
		bc.building = false
		return "", nil, false
	}
	bc.rebuildPending = false
	if bc.process != nil {
		log.Printf("BuilderController: stopping previous app %s for builder %s", bc.appId, bc.builderId)
		bc.stopProcess_nolock()
	}
	bc.appId = bc.pendingAppId
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(1000)
	bc.setStatus_nolock(BuilderStatus_Building, 0, 0, "")
	bc.publishOutputLine("", true)
	bc.outputBuffer.SetLineCallback(func(line string) {
		bc.publishOutputLine(line, false)
	})
	return bc.pendingAppId, bc.pendingEnv, true
}

func (bc *BuilderController) endBuild() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.rebuildPending && !bc.closed.Load() {
		return true
	}
	bc.building = false
	return false
}

// Without the error status the UI would stay on "building" for good, and the cleared
// building flag is what lets Stop() return instead of waiting for a loop that is gone.
func (bc *BuilderController) abandonBuildLoop(panicVal any) {
	bc.handleBuildError(fmt.Errorf("build crashed: %v", panicVal), nil)
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.building = false
	bc.rebuildPending = false
}

// Closing is permanent: a controller being torn down must not start a queued build
// or act on a late watcher callback, either of which would leave an orphan app process.
// Cancelling the running build matters once DeleteController stops waiting for it: the
// build would otherwise run to the end and start an app nobody will see.
func (bc *BuilderController) markClosed() {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.closed.Store(true)
	bc.rebuildPending = false
	if bc.buildCancel != nil {
		bc.buildCancel()
	}
}

func (bc *BuilderController) isClosed() bool {
	return bc.closed.Load()
}

// A controller closed between beginBuild and here gets its build cancelled at once.
func (bc *BuilderController) setBuildCancel(cancel context.CancelFunc) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.buildCancel = cancel
	if cancel != nil && bc.closed.Load() {
		cancel()
	}
}

// A request that arrived while Stop() waits would re-arm rebuildPending and keep the
// build loop going, so Stop() would never see building clear.
func (bc *BuilderController) beginStop() {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.stopping++
	bc.rebuildPending = false
}

func (bc *BuilderController) endStop() {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.stopping--
}

func (bc *BuilderController) isStopping() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.stopping > 0
}

func (bc *BuilderController) acceptsRequests() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return !bc.closed.Load() && bc.stopping == 0
}

func (bc *BuilderController) isBuilding() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.building
}

func (bc *BuilderController) hasProcess() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.process != nil
}

func (bc *BuilderController) getAppId() string {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.appId
}

// The hash is taken when a build is requested as well as when it starts: a save that
// lands during a running build is then recognised as ours, not as an outside change.
func (bc *BuilderController) recordInputHash(appId string) {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	hash, err := ComputeAppInputHash(appDir)
	if err != nil {
		return
	}
	bc.setLastBuildInputHash(hash)
}

// A Code-tab save calls this after writing app.go: the build is requested on the builder
// that saved, which records the input hash and guarantees a build follows, so the watcher
// can never take our own write for an outside change. Other builders on the same app are
// left alone; they have their own watchers and hashes.
// The rebuild uses the builder's rtinfo app id, like every other rebuild; a save naming a
// different app (a window that switched apps while the save was in flight) wrote its file
// but must not build the window's current app on that app's behalf.
func RequestRebuildAfterSave(builderId string, savedAppId string) error {
	if builderId == "" {
		return nil
	}
	appId, builderEnv, err := GetBuilderRebuildInputs(builderId)
	if err != nil {
		return err
	}
	if appId != savedAppId {
		return fmt.Errorf("builder %s is on %s, not the saved app %s", builderId, appId, savedAppId)
	}
	// A controller deleted by an app switch that timed out on the frontend must not leave
	// this window's saves building nothing.
	bc := GetOrCreateController(builderId)
	// The editor already holds what it just wrote, so the save itself is never announced.
	bc.recordAnnouncedHash(appId)
	bc.RequestUserRebuild(appId, builderEnv)
	return nil
}

// The app id and env come from the builder's rtinfo, never from a request: a watcher or
// a save can only ever act on the app the builder window was opened for.
func GetBuilderRebuildInputs(builderId string) (string, map[string]string, error) {
	rtInfo := rtstore.GetRTInfo(remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId))
	if rtInfo == nil {
		return "", nil, fmt.Errorf("builder rtinfo not found for builderid: %s", builderId)
	}
	if rtInfo.BuilderAppId == "" {
		return "", nil, fmt.Errorf("builder appid not set for builderid: %s", builderId)
	}
	return rtInfo.BuilderAppId, rtInfo.BuilderEnv, nil
}

func (bc *BuilderController) setLastBuildInputHash(hash string) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.lastBuildInputHash = hash
}

func (bc *BuilderController) getLastBuildInputHash() string {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.lastBuildInputHash
}

// The announced hash is what the editor has been told about, kept apart from the build
// hash: a build that starts inside the watcher's debounce folds an outside edit into the
// build hash, and gating the announcement on that hash would leave the editor showing
// older content as clean, ready to overwrite the edit on the next save.
func (bc *BuilderController) recordAnnouncedHash(appId string) {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	hash, err := ComputeAppInputHash(appDir)
	if err != nil {
		return
	}
	bc.setLastAnnouncedHash(hash)
}

func (bc *BuilderController) setLastAnnouncedHash(hash string) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.lastAnnouncedHash = hash
}

func (bc *BuilderController) getLastAnnouncedHash() string {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.lastAnnouncedHash
}

// Compare and set under one lock, so two change callbacks racing on the same edit
// announce it once.
func (bc *BuilderController) markAnnounced(hash string) bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if hash == bc.lastAnnouncedHash {
		return false
	}
	bc.lastAnnouncedHash = hash
	return true
}

// AGENTS.md in the starter files quotes these two status lines, so a change here
// has to change that file too.
func runningBuildLogStatus(port int) string {
	return fmt.Sprintf("status: running on port %d", port)
}

// Agents working in the app folder cannot see the Build panel; this file is how they
// read compile errors. Failing to write it must never change the build result.
func writeBuildLog(appId string, lines []string, statusLine string) {
	if appId == "" {
		return
	}
	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	buf.WriteString(statusLine)
	buf.WriteString("\n")
	if err := remotetermappstore.WriteAppBuildLog(appId, []byte(buf.String())); err != nil {
		log.Printf("BuilderController: cannot write build log for %s: %v", appId, err)
	}
}

func (bc *BuilderController) buildAndRun(ctx context.Context, appId string, builderEnv map[string]string, resultCh chan<- *BuildResult) {
	appNS, _, err := remotetermappstore.ParseAppId(appId)
	if err != nil {
		bc.handleBuildError(fmt.Errorf("failed to parse app id: %w", err), resultCh)
		return
	}

	appPath, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		bc.handleBuildError(fmt.Errorf("failed to get app directory: %w", err), resultCh)
		return
	}

	if err := checkAppBuildInputs(appPath); err != nil {
		bc.handleBuildError(err, resultCh)
		return
	}

	outputCapture := build.MakeOutputCapture()
	cachePath, err := compileApp(ctx, appNS, appPath, outputCapture)
	for _, line := range outputCapture.GetLines() {
		bc.outputBuffer.AddLine(line)
	}
	if err != nil {
		bc.handleBuildError(err, resultCh)
		return
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		bc.handleBuildError(fmt.Errorf("build output not found: %w", err), resultCh)
		return
	}

	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		bc.handleBuildError(fmt.Errorf("build output is not executable"), resultCh)
		return
	}

	previewToken, err := makePreviewToken()
	if err != nil {
		bc.handleBuildError(fmt.Errorf("failed to run app: %w", err), resultCh)
		return
	}
	process, err := bc.runBuilderApp(ctx, appId, cachePath, withPreviewToken(builderEnv, previewToken))
	if err != nil {
		bc.handleBuildError(fmt.Errorf("failed to run app: %w", err), resultCh)
		return
	}
	process.PreviewToken = previewToken

	bc.lock.Lock()
	bc.process = process
	bc.setStatus_nolock(BuilderStatus_Running, process.Port, 0, "")
	bc.lock.Unlock()

	writeBuildLog(appId, outputCapture.GetLines(), runningBuildLogStatus(process.Port))

	time.Sleep(1 * time.Second)

	if resultCh != nil {
		buildOutput := ""
		if bc.outputBuffer != nil {
			lines := bc.outputBuffer.GetLines()
			buildOutput = strings.Join(lines, "\n")
		}
		select {
		case resultCh <- &BuildResult{
			Success:     true,
			BuildOutput: buildOutput,
		}:
		default:
		}
	}

	go func() {
		<-process.WaitCh
		bc.lock.Lock()
		if bc.process == process {
			bc.process = nil
			exitCode := exitCodeFromWaitErr(process.WaitRtn)
			bc.setStatus_nolock(BuilderStatus_Stopped, 0, exitCode, "")
		}
		bc.lock.Unlock()
	}()
}

// startAppProcess is a seam so tests can see whether an app would have started.
var startAppProcess = func(cmd *exec.Cmd) error {
	return cmd.Start()
}

// compileApp runs the toolchain; tests replace it to stand in for a build without one.
var compileApp = func(ctx context.Context, appNS string, appPath string, outputCapture *build.OutputCapture) (string, error) {
	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	buildEnv, err := remotetermapputil.PrepareTsunamiBuild(settings)
	if err != nil {
		return "", err
	}

	cachePath, err := GetBuilderAppExecutablePath(appPath)
	if err != nil {
		return "", fmt.Errorf("failed to get builder executable path: %w", err)
	}

	nodePath := remotetermbase.GetWaveAppElectronExecPath()
	if nodePath == "" {
		return "", fmt.Errorf("electron executable path not set")
	}

	sdkVersion := settings.TsunamiSdkVersion
	if sdkVersion == "" {
		sdkVersion = remotetermapputil.DefaultTsunamiSdkVersion
	}

	err = build.TsunamiBuildOutput(build.BuildOpts{
		AppPath:        appPath,
		AppNS:          appNS,
		Verbose:        true,
		Open:           false,
		KeepTemp:       false,
		OutputFile:     cachePath,
		ScaffoldPath:   buildEnv.ScaffoldPath,
		SdkReplacePath: buildEnv.SdkReplacePath,
		MinGoVersion:   buildEnv.MinGoVersion,
		SdkVersion:     sdkVersion,
		NodePath:       nodePath,
		GoPath:         buildEnv.GoPath,
		OutputCapture:  outputCapture,
		MoveFileBack:   true,
		Ctx:            ctx,
	})
	if err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}
	return cachePath, nil
}

func (bc *BuilderController) runBuilderApp(ctx context.Context, appId string, appBinPath string, builderEnv map[string]string) (*BuilderProcess, error) {
	manifest, err := remotetermappstore.ReadAppManifest(appId)
	if err != nil {
		return nil, fmt.Errorf("failed to read app manifest: %w", err)
	}

	secretBindings, err := remotetermappstore.ReadAppSecretBindings(appId)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret bindings (ERR-SECRET): %w", err)
	}

	secretEnv, err := remotetermappstore.BuildAppSecretEnv(appId, manifest, secretBindings)
	if err != nil {
		return nil, fmt.Errorf("failed to build secret environment (ERR-SECRET): %w", err)
	}

	if builderEnv == nil {
		builderEnv = make(map[string]string)
	}
	for k, v := range secretEnv {
		builderEnv[k] = v
	}

	cmd := exec.Command(appBinPath)
	cmd.Env = build.AllowlistedEnv(os.Environ(), "TSUNAMI_CLOSEONSTDIN=1")

	if remotetermbase.IsDevMode() {
		cmd.Env = append(cmd.Env, "TSUNAMI_CORS="+tsunamiutil.DevModeCorsOrigins)
	}

	for key, value := range builderEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	// A build that finished after its controller was deleted must not start an app: no
	// panel shows it and nothing would stop it until the background Stop got round to it.
	if bc.closed.Load() || ctx.Err() != nil {
		return nil, fmt.Errorf("the builder was closed before the app started")
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	portChan := make(chan int, 1)
	portFound := false

	bc.outputBuffer.SetLineCallback(func(line string) {
		if !portFound {
			if port := build.ParseTsunamiPort(line); port > 0 {
				portFound = true
				portChan <- port
			}
		}
		bc.publishOutputLine(line, false)
	})

	err = startAppProcess(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	waitCh := make(chan struct{})
	process := &BuilderProcess{
		Cmd:         cmd,
		StdinWriter: stdinPipe,
		WaitCh:      waitCh,
	}

	go func() {
		process.WaitRtn = cmd.Wait()
		close(waitCh)
	}()

	go bc.outputBuffer.ReadAll(stdoutPipe)
	go bc.outputBuffer.ReadAll(stderrPipe)

	errChan := make(chan error, 1)
	go func() {
		<-process.WaitCh
		select {
		case <-portChan:
		default:
			errChan <- fmt.Errorf("process died before emitting port")
		}
	}()

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	select {
	case port := <-portChan:
		process.Port = port
		return process, nil
	case err := <-errChan:
		cmd.Process.Kill()
		return nil, err
	case <-timeout.C:
		cmd.Process.Kill()
		return nil, fmt.Errorf("timeout waiting for port")
	case <-ctx.Done():
		cmd.Process.Kill()
		return nil, fmt.Errorf("cancelled while waiting for app port: %w", ctx.Err())
	}
}

func (bc *BuilderController) handleBuildError(err error, resultCh chan<- *BuildResult) {
	appId, lines := bc.recordBuildError(err, resultCh)
	writeBuildLog(appId, lines, BuildLogStatusError)
}

func (bc *BuilderController) recordBuildError(err error, resultCh chan<- *BuildResult) (string, []string) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.setStatus_nolock(BuilderStatus_Error, 0, 1, err.Error())
	var lines []string
	if bc.outputBuffer != nil {
		bc.outputBuffer.AddLine("[error] " + err.Error())
		lines = bc.outputBuffer.GetLines()
	}

	if resultCh != nil {
		select {
		case resultCh <- &BuildResult{
			Success:      false,
			ErrorMessage: err.Error(),
			BuildOutput:  strings.Join(lines, "\n"),
		}:
		default:
		}
	}
	return bc.appId, lines
}

func (bc *BuilderController) Stop() error {
	bc.beginStop()
	defer bc.endStop()
	if err := bc.waitForBuildDone(context.Background()); err != nil {
		return err
	}

	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.stopProcess_nolock()
	bc.setStatus_nolock(BuilderStatus_Stopped, 0, 0, "")
	return nil
}

func (bc *BuilderController) stopProcess_nolock() {
	if bc.process == nil {
		return
	}

	if bc.process.Cmd.Process != nil {
		bc.process.Cmd.Process.Kill()
	}

	if bc.process.StdinWriter != nil {
		bc.process.StdinWriter.Close()
	}

	bc.process = nil
}

func (bc *BuilderController) GetStatus() wshrpc.BuilderStatusData {
	appId := bc.getAppId()
	bc.statusLock.Lock()
	defer bc.statusLock.Unlock()

	bc.statusVersion++
	statusData := wshrpc.BuilderStatusData{
		Status:   bc.status,
		Port:     bc.port,
		ExitCode: bc.exitCode,
		ErrorMsg: bc.errorMsg,
		Version:  bc.statusVersion,
	}

	if appId != "" {
		manifest, err := remotetermappstore.ReadAppManifest(appId)
		if err == nil && manifest != nil {
			wshrpcManifest := &wshrpc.AppManifest{
				AppMeta: wshrpc.AppMeta{
					Title:     manifest.AppMeta.Title,
					ShortDesc: manifest.AppMeta.ShortDesc,
				},
				ConfigSchema: manifest.ConfigSchema,
				DataSchema:   manifest.DataSchema,
				Secrets:      make(map[string]wshrpc.SecretMeta),
			}
			for k, v := range manifest.Secrets {
				wshrpcManifest.Secrets[k] = wshrpc.SecretMeta{
					Desc:     v.Desc,
					Optional: v.Optional,
				}
			}
			statusData.Manifest = wshrpcManifest
		}

		secretBindings, err := remotetermappstore.ReadAppSecretBindings(appId)
		if err == nil {
			statusData.SecretBindings = secretBindings
		}

		if manifest != nil && secretBindings != nil {
			_, err := remotetermappstore.BuildAppSecretEnv(appId, manifest, secretBindings)
			statusData.SecretBindingsComplete = (err == nil)
		}
	}

	return statusData
}

func (bc *BuilderController) GetOutput() []string {
	buf := bc.getOutputBuffer()
	if buf == nil {
		return []string{}
	}
	return buf.GetLines()
}

func (bc *BuilderController) getOutputBuffer() *utilds.MultiReaderLineBuffer {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.outputBuffer
}

func (bc *BuilderController) setStatus_nolock(status string, port int, exitCode int, errorMsg string) {
	bc.statusLock.Lock()
	bc.status = status
	bc.port = port
	bc.exitCode = exitCode
	bc.errorMsg = errorMsg
	bc.statusLock.Unlock()

	go bc.publishStatus()
}

// A deleted controller shares its builder oref scope with the controller that replaced it,
// so anything it published after closing (a late "running", "stopped" or output line) would
// land in the new controller's panel.
func (bc *BuilderController) publishStatus() {
	if bc.closed.Load() {
		return
	}
	status := bc.GetStatus()
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_BuilderStatus,
		Scopes: []string{remotetermobj.MakeORef(remotetermobj.OType_Builder, bc.builderId).String()},
		Data:   status,
	})
}

func (bc *BuilderController) publishOutputLine(line string, reset bool) {
	if bc.closed.Load() {
		return
	}
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_BuilderOutput,
		Scopes: []string{remotetermobj.MakeORef(remotetermobj.OType_Builder, bc.builderId).String()},
		Data: map[string]any{
			"lines": []string{line},
			"reset": reset,
		},
	})
}

func exitCodeFromWaitErr(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	if exitError, ok := waitErr.(*exec.ExitError); ok {
		return exitError.ExitCode()
	}
	return 1
}
