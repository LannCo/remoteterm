// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package remotetermbase

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func withDataDir(t *testing.T, dir string) {
	t.Helper()
	saved := DataHome_VarCache
	DataHome_VarCache = dir
	t.Cleanup(func() { DataHome_VarCache = saved })
}

func holdFlock(t *testing.T, fileName string) *os.File {
	t.Helper()
	fd, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(fd.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		fd.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { fd.Close() })
	return fd
}

// A pre-rename server holds wave.lock; emain then renames its data dir to the new name. The new
// server must not be able to open the same data dir.
func TestAcquireWaveLockRefusesWhenLegacyLockHeldInMovedDir(t *testing.T) {
	legacy := filepath.Join(t.TempDir(), "waveterm")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	holdFlock(t, filepath.Join(legacy, LegacyWaveLockFile))
	dest := filepath.Join(filepath.Dir(legacy), "remoteterm")
	if err := os.Rename(legacy, dest); err != nil {
		t.Fatalf("rename with held lock: %v", err)
	}
	withDataDir(t, dest)
	lock, err := AcquireWaveLock()
	if err == nil {
		lock.Close()
		t.Fatalf("acquired %s while a legacy instance holds %s in the same dir", WaveLockFile, LegacyWaveLockFile)
	}
	lock2, err := AcquireWaveLock()
	if err == nil {
		lock2.Close()
		t.Fatalf("failed acquire leaked a lock state that let a retry succeed")
	}
}

func TestAcquireWaveLockTakesUnheldLegacyLockAndReleasesBoth(t *testing.T) {
	dir := t.TempDir()
	legacyName := filepath.Join(dir, LegacyWaveLockFile)
	if err := os.WriteFile(legacyName, nil, 0600); err != nil {
		t.Fatal(err)
	}
	withDataDir(t, dir)
	lock, err := AcquireWaveLock()
	if err != nil {
		t.Fatalf("acquire with unheld legacy lock: %v", err)
	}
	fd, err := os.OpenFile(legacyName, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	if err := unix.Flock(int(fd.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
		t.Fatalf("legacy lock was not held by the new server")
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := unix.Flock(int(fd.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("legacy lock not released on Close: %v", err)
	}
	unix.Flock(int(fd.Fd()), unix.LOCK_UN)
	lock2, err := AcquireWaveLock()
	if err != nil {
		t.Fatalf("re-acquire after Close: %v", err)
	}
	lock2.Close()
}

func TestAcquireWaveLockDoesNotCreateLegacyLock(t *testing.T) {
	dir := t.TempDir()
	withDataDir(t, dir)
	lock, err := AcquireWaveLock()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := os.Stat(filepath.Join(dir, LegacyWaveLockFile)); !os.IsNotExist(err) {
		t.Fatalf("legacy lock file created in a fresh data dir (stat err: %v)", err)
	}
}
