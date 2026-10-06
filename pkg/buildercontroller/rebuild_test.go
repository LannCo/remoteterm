// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s", timeout, what)
}

func waitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestRequestRebuildCoalescesIntoOneFollowUp(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-coalesce")
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	var calls atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}

	begin := time.Now()
	bc.RequestRebuild("draft/demo", nil)
	if elapsed := time.Since(begin); elapsed > 200*time.Millisecond {
		t.Fatalf("RequestRebuild blocked for %v", elapsed)
	}
	waitSignal(t, started, "the first build")
	for i := 0; i < 5; i++ {
		bc.RequestRebuild("draft/demo", nil)
	}
	release <- struct{}{}
	waitSignal(t, started, "the follow-up build")
	release <- struct{}{}
	waitUntil(t, 2*time.Second, func() bool { return !bc.isBuilding() }, "the build loop to finish")
	time.Sleep(100 * time.Millisecond)
	if n := calls.Load(); n != 2 {
		t.Fatalf("%d builds ran, want 2 (the first and one coalesced follow-up)", n)
	}
}

func TestRequestRebuildRecordsInputHash(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-hash")
	done := make(chan struct{}, 1)
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		done <- struct{}{}
	}
	bc.RequestRebuild("draft/demo", nil)
	waitSignal(t, done, "the build")
	want, err := ComputeAppInputHash(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := bc.getLastBuildInputHash(); got != want {
		t.Fatalf("last build input hash %q, want %q", got, want)
	}
}

func TestRebuildStopsPreviousProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the sleep command")
	}
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-stop-previous")
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	bc.process = &BuilderProcess{Cmd: cmd, WaitCh: exited}

	sawProcess := make(chan bool, 1)
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		sawProcess <- bc.hasProcess()
	}
	bc.RequestRebuild("draft/demo", nil)
	select {
	case had := <-sawProcess:
		if had {
			t.Fatal("the previous app process was still attached when the new build started")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the build did not start")
	}
	waitSignal(t, exited, "the previous app process to exit")
}

func TestDeleteControllerDuringBuildLeavesNoProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the sleep command")
	}
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "teardown-build")
	bc := GetOrCreateController("test-teardown-build")
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls atomic.Int32
	attached := make(chan *exec.Cmd, 1)
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		// what a successful buildAndRun leaves behind: a running app attached to the controller
		appProcess := exec.Command("sleep", "30")
		if err := appProcess.Start(); err != nil {
			t.Error(err)
			return
		}
		attachTestProcess(bc, appProcess)
		attached <- appProcess
	}
	bc.RequestRebuild("draft/teardown-build", nil)
	waitSignal(t, started, "the first build")
	bc.RequestRebuild("draft/teardown-build", nil)

	deleted := make(chan struct{})
	go func() {
		DeleteController("test-teardown-build")
		close(deleted)
	}()
	waitUntil(t, 2*time.Second, func() bool { return bc.isClosed() }, "the controller to be marked closed")
	bc.RequestRebuild("draft/teardown-build", nil)
	release <- struct{}{}
	waitSignal(t, deleted, "DeleteController to return")

	var appProcess *exec.Cmd
	select {
	case appProcess = <-attached:
	case <-time.After(2 * time.Second):
		t.Fatal("the build did not attach its app process")
	}
	waitUntil(t, 2*time.Second, func() bool { return !bc.hasProcess() }, "the app process to be detached after teardown")
	time.Sleep(200 * time.Millisecond)
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d builds ran, want 1: a queued build ran after teardown", n)
	}
	exited := make(chan struct{})
	go func() {
		appProcess.Wait()
		close(exited)
	}()
	waitSignal(t, exited, "the app process to be killed")
}

func attachTestProcess(bc *BuilderController, cmd *exec.Cmd) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.process = &BuilderProcess{Cmd: cmd}
}

func TestRequestRebuildAfterSaveBuildsOnSavingControllerOnly(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "saved")
	saver := GetOrCreateController("test-save-saver")
	bystander := GetOrCreateController("test-save-bystander")
	t.Cleanup(func() {
		DeleteController("test-save-saver")
		DeleteController("test-save-bystander")
	})
	var saverCalls, bystanderCalls atomic.Int32
	saver.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { saverCalls.Add(1) }
	bystander.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { bystanderCalls.Add(1) }

	setBuilderRtInfoForTest(t, "test-save-saver", "draft/saved", nil)
	setBuilderRtInfoForTest(t, "test-save-bystander", "draft/saved", nil)
	if err := RequestRebuildAfterSave("", "draft/saved"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if saverCalls.Load() != 0 || bystanderCalls.Load() != 0 {
		t.Fatal("a save without a builder id started a build")
	}

	if err := RequestRebuildAfterSave("test-save-saver", "draft/saved"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*time.Second, func() bool { return saverCalls.Load() == 1 && !saver.isBuilding() }, "the saving builder's build")
	time.Sleep(100 * time.Millisecond)
	if n := saverCalls.Load(); n != 1 {
		t.Fatalf("saving builder ran %d builds, want exactly 1", n)
	}
	if n := bystanderCalls.Load(); n != 0 {
		t.Fatalf("a builder that did not save ran %d builds", n)
	}
	want, err := ComputeAppInputHash(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := saver.getLastBuildInputHash(); got != want {
		t.Fatalf("saving builder hash %q, want %q", got, want)
	}
	if got := bystander.getLastBuildInputHash(); got != "" {
		t.Fatalf("a builder on the same app was stamped with %q", got)
	}
}

func TestRequestRebuildDuringStopDoesNotBuild(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "stopping")
	bc := makeBuilderController("test-stopping")
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}
	bc.RequestRebuild("draft/stopping", nil)
	waitSignal(t, started, "the first build")

	stopped := make(chan struct{})
	go func() {
		bc.Stop()
		close(stopped)
	}()
	waitUntil(t, 2*time.Second, func() bool { return bc.isStopping() }, "Stop to begin")
	for i := 0; i < 3; i++ {
		bc.RequestRebuild("draft/stopping", nil)
	}
	close(release)
	begin := time.Now()
	waitSignal(t, stopped, "Stop to return")
	t.Logf("Stop returned %v after release", time.Since(begin))
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d builds ran, want 1: a request during Stop started a build", n)
	}
	if bc.isStopping() {
		t.Fatal("the stopping state outlived Stop")
	}

	bc.RequestRebuild("draft/stopping", nil)
	waitSignal(t, started, "a build requested after Stop")
}

func TestBuildLoopPanicSetsErrorStatus(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "panics")
	bc := makeBuilderController("test-panic")
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		panic("boom")
	}
	bc.RequestRebuild("draft/panics", nil)
	waitUntil(t, 2*time.Second, func() bool { return !bc.isBuilding() }, "the build loop to be abandoned")
	waitUntil(t, 2*time.Second, func() bool { return bc.GetStatus().Status == BuilderStatus_Error }, "the error status")
	if msg := bc.GetStatus().ErrorMsg; !strings.Contains(msg, "boom") {
		t.Fatalf("error message %q does not mention the panic", msg)
	}
	lines := bc.GetOutput()
	if len(lines) == 0 || !strings.HasPrefix(lines[len(lines)-1], "[error] build crashed") {
		t.Fatalf("build output = %q; want a final [error] line", lines)
	}

	stopped := make(chan struct{})
	go func() {
		bc.Stop()
		close(stopped)
	}()
	waitSignal(t, stopped, "Stop after a panicked build")
}

func TestGetOutputDuringRebuildsIsRaceFree(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "race")
	bc := makeBuilderController("test-output-race")
	var calls atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		calls.Add(1)
	}
	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
				bc.GetOutput()
			}
		}
	}()
	for i := 0; i < 40; i++ {
		bc.RequestRebuild("draft/race", nil)
		waitUntil(t, 2*time.Second, func() bool { return !bc.isBuilding() }, "a build to finish")
	}
	close(stop)
	<-readerDone
	if calls.Load() == 0 {
		t.Fatal("no builds ran")
	}
}

// An app switch deletes the controller while a build may be running. The delete must
// return before the frontend's RPC timeout, the build's app must still be killed, and a
// later save from the same window must get a controller that builds.
func TestDeleteControllerDuringBlockedBuildReturnsAndSaveRecovers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the sleep command")
	}
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "switch")
	var freshBuilds atomic.Int32
	origRun := runBuildAndRun
	runBuildAndRun = func(bc *BuilderController, ctx context.Context, appId string, builderEnv map[string]string) {
		freshBuilds.Add(1)
	}
	t.Cleanup(func() { runBuildAndRun = origRun })

	setBuilderRtInfoForTest(t, "test-switch", "draft/switch", nil)
	bc := GetOrCreateController("test-switch")
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseBuild := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseBuild)
	appProcess := make(chan *exec.Cmd, 1)
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		started <- struct{}{}
		<-release
		cmd := exec.Command("sleep", "30")
		if err := cmd.Start(); err != nil {
			t.Error(err)
			return
		}
		attachTestProcess(bc, cmd)
		appProcess <- cmd
	}
	bc.RequestRebuild("draft/switch", nil)
	waitSignal(t, started, "the build")

	deleted := make(chan struct{})
	go func() {
		DeleteController("test-switch")
		close(deleted)
	}()
	waitSignal(t, deleted, "DeleteController to return while the build is blocked")
	if GetController("test-switch") != nil {
		t.Fatal("the deleted controller is still registered")
	}

	if err := RequestRebuildAfterSave("test-switch", "draft/switch"); err != nil {
		t.Fatal(err)
	}
	fresh := GetController("test-switch")
	t.Cleanup(func() { DeleteController("test-switch") })
	if fresh == nil || fresh == bc {
		t.Fatalf("a save after the delete got controller %p, want a fresh one (old %p)", fresh, bc)
	}
	waitUntil(t, 2*time.Second, func() bool { return freshBuilds.Load() == 1 && !fresh.isBuilding() }, "the save's build")

	releaseBuild()
	var cmd *exec.Cmd
	select {
	case cmd = <-appProcess:
	case <-time.After(2 * time.Second):
		t.Fatal("the blocked build did not finish after release")
	}
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	waitSignal(t, exited, "the deleted controller's app process to be killed")
	time.Sleep(100 * time.Millisecond)
	if n := freshBuilds.Load(); n != 1 {
		t.Fatalf("the fresh controller ran %d builds, want 1", n)
	}
}

func TestRequestRebuildAfterSaveUsesTheBuilderAppId(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "current")
	makeTestApp(t, home, "other")
	bc := GetOrCreateController("test-save-appid")
	t.Cleanup(func() { DeleteController("test-save-appid") })
	setBuilderRtInfoForTest(t, "test-save-appid", "draft/current", map[string]any{"FOO": "bar"})
	type buildCall struct {
		appId string
		env   map[string]string
	}
	builds := make(chan buildCall, 4)
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		builds <- buildCall{appId, builderEnv}
	}

	if err := RequestRebuildAfterSave("test-save-appid", "draft/other"); err == nil {
		t.Fatal("a save of another app reported a rebuild")
	}
	select {
	case call := <-builds:
		t.Fatalf("a save of draft/other built %s", call.appId)
	case <-time.After(200 * time.Millisecond):
	}

	if err := RequestRebuildAfterSave("test-save-appid", "draft/current"); err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-builds:
		if call.appId != "draft/current" || call.env["FOO"] != "bar" {
			t.Fatalf("build = %+v, want draft/current with the rtinfo env", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the save of the builder's app did not build")
	}
}
