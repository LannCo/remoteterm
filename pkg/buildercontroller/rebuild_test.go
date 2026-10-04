// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"os/exec"
	"runtime"
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
	var appProcess *exec.Cmd
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		// what a successful buildAndRun leaves behind: a running app attached to the controller
		appProcess = exec.Command("sleep", "30")
		if err := appProcess.Start(); err != nil {
			t.Error(err)
			return
		}
		attachTestProcess(bc, appProcess)
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

	time.Sleep(200 * time.Millisecond)
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d builds ran, want 1: a queued build ran after teardown", n)
	}
	if bc.hasProcess() {
		t.Fatal("an app process is still attached after teardown")
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

func TestRecordAppInputHashUpdatesMatchingControllers(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "recorded")
	match := GetOrCreateController("test-record-match")
	other := GetOrCreateController("test-record-other")
	t.Cleanup(func() {
		DeleteController("test-record-match")
		DeleteController("test-record-other")
	})
	setTestAppId(match, "draft/recorded")
	setTestAppId(other, "draft/elsewhere")
	other.setLastBuildInputHash("untouched")

	RecordAppInputHash("draft/recorded")
	want, err := ComputeAppInputHash(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := match.getLastBuildInputHash(); got != want {
		t.Fatalf("matching controller hash %q, want %q", got, want)
	}
	if got := other.getLastBuildInputHash(); got != "untouched" {
		t.Fatalf("a controller for another app was updated: %q", got)
	}
}

func setTestAppId(bc *BuilderController, appId string) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.appId = appId
}
