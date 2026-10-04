// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type watchRecorder struct {
	lock     sync.Mutex
	changes  int
	statuses []string
	reasons  []string
}

func (r *watchRecorder) onChange() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.changes++
}

func (r *watchRecorder) onStatus(status string, reason string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.statuses = append(r.statuses, status)
	r.reasons = append(r.reasons, reason)
}

func (r *watchRecorder) lastReason() string {
	r.lock.Lock()
	defer r.lock.Unlock()
	if len(r.reasons) == 0 {
		return ""
	}
	return r.reasons[len(r.reasons)-1]
}

func (r *watchRecorder) changeCount() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.changes
}

func (r *watchRecorder) lastStatus() string {
	r.lock.Lock()
	defer r.lock.Unlock()
	if len(r.statuses) == 0 {
		return ""
	}
	return r.statuses[len(r.statuses)-1]
}

func shortWatchTimings(t *testing.T) {
	t.Helper()
	origDebounce, origPoll, origUnavailable, origMaxDirs := watchDebounce, rootPollInterval, rootUnavailableAfter, maxWatchedDirs
	watchDebounce = 100 * time.Millisecond
	rootPollInterval = 50 * time.Millisecond
	rootUnavailableAfter = 300 * time.Millisecond
	t.Cleanup(func() {
		watchDebounce, rootPollInterval, rootUnavailableAfter, maxWatchedDirs = origDebounce, origPoll, origUnavailable, origMaxDirs
	})
}

func startTestWatcher(t *testing.T, appDir string) *watchRecorder {
	t.Helper()
	rec, _ := startTestWatcherWithHandle(t, appDir)
	return rec
}

func startTestWatcherWithHandle(t *testing.T, appDir string) (*watchRecorder, *AppWatcher) {
	t.Helper()
	rec := &watchRecorder{}
	w, err := MakeAppWatcher(appDir, rec.onChange, rec.onStatus)
	if err != nil {
		t.Fatalf("MakeAppWatcher: %v", err)
	}
	t.Cleanup(w.Close)
	return rec, w
}

func writeAppFileForTest(t *testing.T, appDir string, rel string, content string) {
	t.Helper()
	path := filepath.Join(appDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherBurstDebouncedToOneChange(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "burst")
	rec := startTestWatcher(t, appDir)
	for i := 0; i < 5; i++ {
		writeAppFileForTest(t, appDir, "app.go", fmt.Sprintf("package main // %d\n", i))
		time.Sleep(20 * time.Millisecond)
	}
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() >= 1 }, "a change notification")
	time.Sleep(300 * time.Millisecond)
	if n := rec.changeCount(); n != 1 {
		t.Fatalf("%d change notifications for one burst, want 1", n)
	}
}

func TestWatcherIgnoresGeneratedAndTempFiles(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "ignored")
	writeAppFileForTest(t, appDir, "static/keep.txt", "x")
	rec := startTestWatcher(t, appDir)
	for _, rel := range []string{"go.mod", "go.sum", "manifest.json", "static/tw.css", ".tsunami/build.log", "app.go~", ".app.go.swp", "4913", "bin/app"} {
		writeAppFileForTest(t, appDir, rel, "generated")
	}
	time.Sleep(500 * time.Millisecond)
	if n := rec.changeCount(); n != 0 {
		t.Fatalf("%d change notifications for ignored files, want 0", n)
	}
}

func TestWatcherAtomicRenameSaveFiresOnce(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "rename")
	rec := startTestWatcher(t, appDir)
	writeAppFileForTest(t, appDir, "app.go.tmp.4242", "package main // new\n")
	time.Sleep(3 * watchDebounce)
	if n := rec.changeCount(); n != 0 {
		t.Fatalf("%d change notifications for the temp file alone, want 0", n)
	}
	if err := os.Rename(filepath.Join(appDir, "app.go.tmp.4242"), filepath.Join(appDir, "app.go")); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() >= 1 }, "a change notification")
	time.Sleep(300 * time.Millisecond)
	if n := rec.changeCount(); n != 1 {
		t.Fatalf("%d change notifications for one rename-save, want 1", n)
	}
}

func TestWatcherWatchesNewStaticSubdir(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "subdir")
	writeAppFileForTest(t, appDir, "static/keep.txt", "x")
	rec := startTestWatcher(t, appDir)
	writeAppFileForTest(t, appDir, "static/img/a.png", "a")
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() >= 1 }, "a change for the new directory")
	time.Sleep(300 * time.Millisecond)
	before := rec.changeCount()
	writeAppFileForTest(t, appDir, "static/img/b.png", "b")
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() > before }, "a change inside the new directory")
}

func TestWatcherRootRemovedThenRecreated(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "gone")
	rec := startTestWatcher(t, appDir)
	if err := os.RemoveAll(appDir); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 3*time.Second, func() bool { return rec.lastStatus() == WatchStatus_Unavailable }, "the unavailable status")
	before := rec.changeCount()
	writeAppFileForTest(t, appDir, "app.go", "package main // back\n")
	waitUntil(t, 3*time.Second, func() bool { return rec.lastStatus() == WatchStatus_Active }, "the active status after recreation")
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() > before }, "a rescan change after recreation")
}

func TestWatcherCapOfEight(t *testing.T) {
	home, _ := setupBuilderTest(t)
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers leaked from earlier tests", n)
	}
	var watchers []*AppWatcher
	for i := 0; i < MaxAppWatchers; i++ {
		w, err := MakeAppWatcher(makeTestApp(t, home, fmt.Sprintf("cap%d", i)), func() {}, func(string, string) {})
		if err != nil {
			t.Fatalf("watcher %d: %v", i, err)
		}
		watchers = append(watchers, w)
	}
	extraDir := makeTestApp(t, home, "cap-extra")
	if _, err := MakeAppWatcher(extraDir, func() {}, func(string, string) {}); err == nil || !strings.Contains(err.Error(), "limit 8") {
		t.Fatalf("ninth watcher: got %v, want a limit error", err)
	}
	watchers[0].Close()
	extra, err := MakeAppWatcher(extraDir, func() {}, func(string, string) {})
	if err != nil {
		t.Fatalf("watcher after closing one: %v", err)
	}
	extra.Close()
	for _, w := range watchers[1:] {
		w.Close()
	}
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers still counted after Close", n)
	}
}

func TestWatcherCloseReleasesFds(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("counts /proc/self/fd")
	}
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "fds")
	countFds := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	before := countFds()
	for i := 0; i < 3; i++ {
		w, err := MakeAppWatcher(appDir, func() {}, func(string, string) {})
		if err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
	if after := countFds(); after > before {
		t.Fatalf("open fds went from %d to %d after closing watchers", before, after)
	}
}

// A slow Add must not stall the goroutine that reads Events: on Windows fsnotify's Add
// waits for a reply from the backend reader, which may be blocked sending to Events.
func TestWatcherKeepsDrainingEventsWhileAddIsInFlight(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "slowadd")
	writeAppFileForTest(t, appDir, "static/keep.txt", "x")
	rec, w := startTestWatcherWithHandle(t, appDir)

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	releaseAdd := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseAdd)
	w.lock.Lock()
	realAdd := w.addFn
	w.addFn = func(dir string) error {
		if strings.HasSuffix(dir, "slow") {
			close(entered)
			<-release
		}
		return realAdd(dir)
	}
	w.lock.Unlock()

	if err := os.Mkdir(filepath.Join(appDir, "static", "slow"), 0755); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, entered, "the Add of the new directory")
	writeAppFileForTest(t, appDir, "static/other.png", "o")
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() >= 1 }, "a change while the Add is still blocked")
	releaseAdd()
}

func TestWatcherRootReturnsOverDirCapReleasesSlot(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "regrow")
	maxWatchedDirs = 3
	rec := startTestWatcher(t, appDir)
	staging := filepath.Join(home, "staging")
	for _, sub := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(staging, "static", sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(appDir); err != nil {
		t.Fatal(err)
	}
	// Renamed into place so the poller sees the whole tree at once.
	if err := os.Rename(staging, appDir); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 3*time.Second, func() bool { return activeWatcherCount() == 0 }, "the slot to be released")
	if rec.lastStatus() != WatchStatus_Unavailable || !strings.Contains(rec.lastReason(), "more than 3") {
		t.Fatalf("status %q reason %q, want unavailable over the cap", rec.lastStatus(), rec.lastReason())
	}
}

func TestWatcherDirCapOnCreatedDirReleasesSlot(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "growcap")
	writeAppFileForTest(t, appDir, "static/keep.txt", "x")
	maxWatchedDirs = 3
	rec := startTestWatcher(t, appDir)
	if err := os.MkdirAll(filepath.Join(appDir, "static", "a", "b"), 0755); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 3*time.Second, func() bool { return activeWatcherCount() == 0 }, "the slot to be released")
	if rec.lastStatus() != WatchStatus_Unavailable || !strings.Contains(rec.lastReason(), "more than 3") {
		t.Fatalf("status %q reason %q, want unavailable over the cap", rec.lastStatus(), rec.lastReason())
	}
}
