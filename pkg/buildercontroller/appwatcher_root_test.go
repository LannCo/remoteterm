// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"io/fs"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// kqueue lists the directory inside Add, so a file that vanishes mid-listing makes the Add of
// the root fail with ErrNotExist although the root is there.
func makeFakeAddWatcher(appDir string, addFn func(string) error) *AppWatcher {
	return &AppWatcher{
		appDir:      filepath.Clean(appDir),
		watchedDirs: make(map[string]bool),
		addFn:       addFn,
		closeCh:     make(chan struct{}),
	}
}

func vanishedFileErr(dir string) error {
	return &fs.PathError{Op: "lstat", Path: filepath.Join(dir, "swap.tmp"), Err: fs.ErrNotExist}
}

func TestAddTreeRetriesRootAddThatLostARace(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "kqrace")
	rootAddRetryDelay = time.Millisecond
	t.Cleanup(func() { rootAddRetryDelay = defaultRootAddRetryDelay })
	var calls atomic.Int32
	w := makeFakeAddWatcher(appDir, func(dir string) error {
		if dir == w0(appDir) && calls.Add(1) == 1 {
			return vanishedFileErr(dir)
		}
		return nil
	})
	if err := w.addTree(); err != nil {
		t.Fatalf("addTree: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("root Add called %d times, want 2 (one retry)", got)
	}
	if !w.watchedDirs[w.appDir] {
		t.Fatal("root is not recorded as watched after the retry succeeded")
	}
}

func TestAddTreeToleratesRootAddThatKeepsLosingTheRace(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "kqrace2")
	rootAddRetryDelay = time.Millisecond
	t.Cleanup(func() { rootAddRetryDelay = defaultRootAddRetryDelay })
	var calls atomic.Int32
	w := makeFakeAddWatcher(appDir, func(dir string) error {
		calls.Add(1)
		return vanishedFileErr(dir)
	})
	if err := w.addTree(); err != nil {
		t.Fatalf("addTree failed although the folder exists: %v", err)
	}
	if calls.Load() < 2 {
		t.Fatalf("root Add called %d times, want a retry", calls.Load())
	}
}

func TestAddWatchRootStillFailsWhenFolderIsGone(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := filepath.Join(home, "waveapps", "draft", "gone")
	rootAddRetryDelay = time.Millisecond
	t.Cleanup(func() { rootAddRetryDelay = defaultRootAddRetryDelay })
	w := makeFakeAddWatcher(appDir, func(dir string) error {
		return &fs.PathError{Op: "add", Path: dir, Err: fs.ErrNotExist}
	})
	if err := w.addWatch(w.appDir); err == nil {
		t.Fatal("a root that no longer exists was treated as watched")
	}
}

func TestAddWatchRootStillReportsOtherErrors(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "kqperm")
	w := makeFakeAddWatcher(appDir, func(dir string) error {
		return &fs.PathError{Op: "add", Path: dir, Err: fs.ErrPermission}
	})
	if err := w.addWatch(w.appDir); err == nil {
		t.Fatal("a permission error on the root was swallowed")
	}
}

// w0 spells the root the way addTree passes it to the Add function.
func w0(appDir string) string { return filepath.Clean(appDir) }
