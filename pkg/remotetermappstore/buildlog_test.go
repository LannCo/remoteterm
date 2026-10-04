// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAppBuildLogCreatesAndTruncates(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := WriteAppBuildLog("draft/demo", []byte("a long first log\nstatus: error\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppBuildLog("draft/demo", []byte("short\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".tsunami", "build.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "short\n" {
		t.Fatalf("build log = %q, want only the latest contents", got)
	}
}

func TestWriteAppBuildLogRefusesSymlinkedTsunamiDir(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, ".tsunami")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppBuildLog("draft/demo", []byte("x")); err == nil {
		t.Fatal("write through a symlinked .tsunami succeeded")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("the symlink target was written to: %v", entries)
	}
}

func TestWriteAppBuildLogRefusesSymlinkedTsunamiDirInsideApp(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.Mkdir(filepath.Join(dir, "elsewhere"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("elsewhere", filepath.Join(dir, ".tsunami")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppBuildLog("draft/demo", []byte("x")); err == nil {
		t.Fatal("write through a symlinked .tsunami succeeded")
	}
	if _, err := os.Stat(filepath.Join(dir, "elsewhere", "build.log")); err == nil {
		t.Fatal("the build log was written through the symlink")
	}
}

func TestWriteAppBuildLogReplacesSymlinkedLogFile(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.Mkdir(filepath.Join(dir, ".tsunami"), 0755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("precious"), 0644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, ".tsunami", "build.log")
	if err := os.Symlink(victim, logPath); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppBuildLog("draft/demo", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(victim); string(got) != "precious" {
		t.Fatalf("the symlink target was overwritten: %q", got)
	}
	info, err := os.Lstat(logPath)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("build.log is not a regular file after the write: %v %v", info, err)
	}
}

func TestWriteAppBuildLogReplacesHardLinkedLogFile(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.Mkdir(filepath.Join(dir, ".tsunami"), 0755); err != nil {
		t.Fatal(err)
	}
	// Stands in for ~/.bashrc: outside the app folder, on the same filesystem.
	victim := filepath.Join(home, "victim")
	if err := os.WriteFile(victim, []byte("precious"), 0644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, ".tsunami", "build.log")
	if err := os.Link(victim, logPath); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := WriteAppBuildLog("draft/demo", []byte("compiler output\nstatus: error\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(victim); string(got) != "precious" {
		t.Fatalf("the hard link target was written: %q", got)
	}
	if got, _ := os.ReadFile(logPath); string(got) != "compiler output\nstatus: error\n" {
		t.Fatalf("build log = %q", got)
	}
}

func TestWriteAppBuildLogFailsWhenTsunamiIsAFile(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(dir, ".tsunami"), []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppBuildLog("draft/demo", []byte("x")); err == nil {
		t.Fatal("expected an error when .tsunami is a file")
	}
}
