// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
)

func setBuilderRtInfoForTest(t *testing.T, builderId string, appId string, env map[string]any) {
	t.Helper()
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": appId, "builder:env": env})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
}

func overrideWatchSeams(t *testing.T, live bool) *atomic.Int32 {
	t.Helper()
	origLive, origPublish := liveRebuildEnabled, publishAppGoUpdated
	var published atomic.Int32
	liveRebuildEnabled = func() bool { return live }
	publishAppGoUpdated = func(appId string) { published.Add(1) }
	t.Cleanup(func() {
		liveRebuildEnabled, publishAppGoUpdated = origLive, origPublish
	})
	return &published
}

func TestHandleAppFilesChangedSuppressesEcho(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "echo")
	published := overrideWatchSeams(t, true)
	bc := makeBuilderController("test-echo")
	setBuilderRtInfoForTest(t, "test-echo", "draft/echo", map[string]any{"FOO": "bar"})
	var builds atomic.Int32
	var gotEnv atomic.Value
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		gotEnv.Store(builderEnv)
		builds.Add(1)
	}

	hash, err := ComputeAppInputHash(appDir)
	if err != nil {
		t.Fatal(err)
	}
	bc.setLastBuildInputHash(hash)
	bc.setLastAnnouncedHash(hash)
	bc.handleAppFilesChanged("draft/echo")
	if published.Load() != 0 || builds.Load() != 0 {
		t.Fatalf("echo of our own build: %d events, %d builds; want 0 and 0", published.Load(), builds.Load())
	}

	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // outside\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.handleAppFilesChanged("draft/echo")
	if published.Load() != 1 {
		t.Fatalf("%d appgoupdated events for an outside change, want 1", published.Load())
	}
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the live rebuild")
	if env, _ := gotEnv.Load().(map[string]string); env["FOO"] != "bar" {
		t.Fatalf("live rebuild env = %v, want the builder's rtinfo env", env)
	}
}

// An outside edit that a build folds into its input hash before the watcher's debounce
// fires must still reach the editor, or the editor keeps older content as clean and the
// next save overwrites the edit.
func TestOutsideEditFoldedIntoBuildIsStillAnnounced(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "folded")
	published := overrideWatchSeams(t, true)
	bc := GetOrCreateController("test-folded")
	t.Cleanup(func() { DeleteController("test-folded") })
	setBuilderRtInfoForTest(t, "test-folded", "draft/folded", nil)
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }

	// Opening the window starts a build and then the watcher.
	bc.RequestRebuild("draft/folded", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 && !bc.isBuilding() }, "the first build")
	if status := bc.StartWatching("draft/folded"); status.Status != WatchStatus_Active {
		t.Fatalf("status = %+v", status)
	}
	bc.StopWatching()
	bc.handleAppFilesChanged("draft/folded")
	time.Sleep(100 * time.Millisecond)
	if published.Load() != 0 || builds.Load() != 1 {
		t.Fatalf("a change event that changed nothing: %d events, %d builds; want 0 and 1", published.Load(), builds.Load())
	}

	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // agent\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// A pending build (live on) or a Rebuild click (live off) starts inside the debounce.
	bc.RequestRebuild("draft/folded", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 2 && !bc.isBuilding() }, "the build")
	bc.handleAppFilesChanged("draft/folded")
	bc.handleAppFilesChanged("draft/folded")
	if n := published.Load(); n != 1 {
		t.Fatalf("%d appgoupdated events for an outside edit folded into a build, want 1", n)
	}
	time.Sleep(100 * time.Millisecond)
	if n := builds.Load(); n != 2 {
		t.Fatalf("%d builds, want 2: the second build already included the edit", n)
	}

	// The Code-tab save path.
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // saved\n"), 0644); err != nil {
		t.Fatal(err)
	}
	RequestRebuildAfterSave("test-folded", "draft/folded", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 3 && !bc.isBuilding() }, "the save's build")
	bc.handleAppFilesChanged("draft/folded")
	time.Sleep(100 * time.Millisecond)
	if n := published.Load(); n != 1 {
		t.Fatalf("%d appgoupdated events after a Code-tab save, want still 1", n)
	}
	if n := builds.Load(); n != 3 {
		t.Fatalf("%d builds after a Code-tab save, want 3", n)
	}
}

func TestSaveWithLiveRebuildOnYieldsOneBuild(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "save")
	overrideWatchSeams(t, true)
	bc := makeBuilderController("test-save")
	setBuilderRtInfoForTest(t, "test-save", "draft/save", nil)
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }

	// The Code-tab save path: write app.go, then request the rebuild on the saving builder.
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // saved\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.RequestRebuild("draft/save", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 && !bc.isBuilding() }, "the save's build")
	// The watcher then sees the save's own fsnotify event.
	bc.handleAppFilesChanged("draft/save")
	time.Sleep(200 * time.Millisecond)
	if n := builds.Load(); n != 1 {
		t.Fatalf("%d builds for one save with live rebuild on, want 1", n)
	}
}

func TestHandleAppFilesChangedWithLiveRebuildOff(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "manual")
	published := overrideWatchSeams(t, false)
	bc := makeBuilderController("test-manual")
	setBuilderRtInfoForTest(t, "test-manual", "draft/manual", nil)
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // outside\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.handleAppFilesChanged("draft/manual")
	time.Sleep(200 * time.Millisecond)
	if published.Load() != 1 || builds.Load() != 0 {
		t.Fatalf("live rebuild off: %d events, %d builds; want 1 and 0", published.Load(), builds.Load())
	}
}

func TestHandleAppFilesChangedIgnoredOnClosedController(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "closed")
	published := overrideWatchSeams(t, true)
	bc := makeBuilderController("test-closed")
	setBuilderRtInfoForTest(t, "test-closed", "draft/closed", nil)
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // late\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.markClosed()
	bc.handleAppFilesChanged("draft/closed")
	time.Sleep(100 * time.Millisecond)
	if published.Load() != 0 || builds.Load() != 0 {
		t.Fatalf("closed controller: %d events, %d builds; want 0 and 0", published.Load(), builds.Load())
	}
}

func TestHandleAppFilesChangedIgnoresStaleAppId(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "old")
	makeTestApp(t, home, "new")
	published := overrideWatchSeams(t, true)
	bc := makeBuilderController("test-stale")
	// The window was repointed at draft/new after the watcher on draft/old was created.
	setBuilderRtInfoForTest(t, "test-stale", "draft/new", nil)
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.handleAppFilesChanged("draft/old")
	time.Sleep(200 * time.Millisecond)
	if published.Load() != 0 || builds.Load() != 0 {
		t.Fatalf("stale app id: %d events, %d builds; want 0 and 0", published.Load(), builds.Load())
	}
}

func TestInstallWatcherOnControllerClosedMeanwhileReleasesSlot(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "racedelete")
	bc := makeBuilderController("test-racedelete")
	watcher, err := MakeAppWatcher(appDir, func() {}, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	// DeleteController closed the controller between StartWatching's check and the swap.
	bc.markClosed()
	status := bc.installWatcher(watcher)
	if status.Status != WatchStatus_Unavailable {
		t.Fatalf("status = %+v, want unavailable", status)
	}
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers active after installing on a closed controller, want 0", n)
	}
	if bc.watcher != nil {
		t.Fatal("a closed controller kept a watcher")
	}
	if status := bc.StartWatching("draft/racedelete"); status.Status != WatchStatus_Unavailable || activeWatcherCount() != 0 {
		t.Fatalf("StartWatching on a closed controller: %+v, %d active", status, activeWatcherCount())
	}
}

func TestReplacedWatcherCannotPublishStaleStatus(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "replace")
	var lock sync.Mutex
	var got []string
	orig := publishBuilderWatchStatus
	publishBuilderWatchStatus = func(builderId string, data wshrpc.BuilderWatchStatusData) {
		lock.Lock()
		defer lock.Unlock()
		got = append(got, data.Reason)
	}
	t.Cleanup(func() { publishBuilderWatchStatus = orig })
	bc := makeBuilderController("test-replace")
	if status := bc.StartWatching("draft/replace"); status.Status != WatchStatus_Active {
		t.Fatalf("first start: %+v", status)
	}
	old := bc.watcher
	if status := bc.StartWatching("draft/replace"); status.Status != WatchStatus_Active {
		t.Fatalf("second start: %+v", status)
	}
	t.Cleanup(bc.StopWatching)
	// The old watcher reports while it shuts down, after the new one is installed.
	old.onStatus(WatchStatus_Unavailable, "stale")
	bc.watcher.onStatus(WatchStatus_Unavailable, "current")
	lock.Lock()
	defer lock.Unlock()
	if len(got) != 1 || got[0] != "current" {
		t.Fatalf("published reasons = %v, want only [current]", got)
	}
}

func TestConcurrentStartWatchingKeepsOneWatcher(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "twice")
	bc := makeBuilderController("test-twice")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bc.StartWatching("draft/twice")
		}()
	}
	wg.Wait()
	if n := activeWatcherCount(); n != 1 {
		t.Fatalf("%d watchers active after concurrent StartWatching, want 1", n)
	}
	bc.StopWatching()
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers active after StopWatching, want 0", n)
	}
}

func TestStartWatchingReportsUnavailableOverDirCap(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "toomany")
	for _, sub := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(appDir, "static", sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	origMax := maxWatchedDirs
	maxWatchedDirs = 3
	t.Cleanup(func() { maxWatchedDirs = origMax })
	bc := makeBuilderController("test-dircap")
	status := bc.StartWatching("draft/toomany")
	if status.Status != WatchStatus_Unavailable || !strings.Contains(status.Reason, "more than 3") {
		t.Fatalf("status = %+v", status)
	}
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("a failed watcher kept its slot (%d active)", n)
	}
}

func TestStartWatchingRefusesSymlinkedAppFolder(t *testing.T) {
	home, _ := setupBuilderTest(t)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "waveapps", "draft", "linked")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	bc := makeBuilderController("test-symlink")
	status := bc.StartWatching("draft/linked")
	if status.Status != WatchStatus_Unavailable || !strings.Contains(status.Reason, "symbolic link") {
		t.Fatalf("status = %+v", status)
	}
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("a refused watcher kept its slot (%d active)", n)
	}
}

func TestStartWatchingSkipsSymlinkedStaticSubdir(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "linkstatic")
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(appDir, "static"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(appDir, "static", "out")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	rec := startTestWatcher(t, appDir)
	if err := os.WriteFile(filepath.Join(outside, "x.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if n := rec.changeCount(); n != 0 {
		t.Fatalf("%d change notifications from outside a symlinked directory, want 0", n)
	}
}

func TestDeleteControllerStopsWatcher(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "teardown")
	bc := GetOrCreateController("test-teardown")
	if status := bc.StartWatching("draft/teardown"); status.Status != WatchStatus_Active {
		t.Fatalf("status = %+v", status)
	}
	if n := activeWatcherCount(); n != 1 {
		t.Fatalf("%d watchers active, want 1", n)
	}
	DeleteController("test-teardown")
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers active after DeleteController, want 0", n)
	}
}
