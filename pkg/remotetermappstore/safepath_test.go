// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setupAppStoreTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func makeAppDir(t *testing.T, home string, ns string, name string) string {
	t.Helper()
	dir := filepath.Join(home, "waveapps", ns, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func skipWithoutSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures need a Unix filesystem")
	}
}

func TestReadAppFileCapsSize(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(dir, "ok.bin"), bytes.Repeat([]byte("a"), MaxAppFileReadSize), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), bytes.Repeat([]byte("a"), MaxAppFileReadSize+1), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "ok.bin"); err != nil {
		t.Fatalf("2 MiB file: %v", err)
	}
	if _, err := ReadAppFile("draft/demo", "big.bin"); err == nil {
		t.Fatal("expected an error for a file over 2 MiB")
	}
}

func TestReadAppFileMissingIsErrNotExist(t *testing.T) {
	home := setupAppStoreTest(t)
	makeAppDir(t, home, "draft", "demo")
	_, err := ReadAppFile("draft/demo", "app.go")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, want an os.ErrNotExist error", err)
	}
}

func TestReadAppFileRejectsNonRegular(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.Mkdir(filepath.Join(dir, "app.go"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "app.go"); err == nil {
		t.Fatal("expected an error reading a directory")
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "link.go"); err == nil {
		t.Fatal("expected an error reading through a symlink")
	}
}

func TestSymlinkedParentRejected(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.css"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "static")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "static/secret.css"); err == nil {
		t.Error("read through a symlinked parent succeeded")
	}
	if err := WriteAppFile("draft/demo", "static/new.css", []byte("x")); err == nil {
		t.Error("write through a symlinked parent succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.css")); err == nil {
		t.Error("a file was created outside the app folder")
	}
	if err := DeleteAppFile("draft/demo", "static/secret.css"); err == nil {
		t.Error("delete through a symlinked parent succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.css")); err != nil {
		t.Error("a file outside the app folder was deleted")
	}
}

func TestSymlinkedAppDirRejected(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, "waveapps", "draft", "demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "app.go"); err == nil {
		t.Error("read from a symlinked app folder succeeded")
	}
	if err := WriteAppFile("draft/demo", "x.go", []byte("x")); err == nil {
		t.Error("write into a symlinked app folder succeeded")
	}
	if _, err := os.Stat(filepath.Join(real, "x.go")); err == nil {
		t.Error("a file was created in the symlink target")
	}
}

func TestWriteAppFileRefusesDanglingSymlink(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	target := filepath.Join(t.TempDir(), "created-through-link")
	if err := os.Symlink(target, filepath.Join(dir, "app.go")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("package main\n")); err == nil {
		t.Fatal("write through a dangling symlink succeeded")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the symlink target was created")
	}
}

func TestWriteAppFileRefusesNonRegularTarget(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.Mkdir(filepath.Join(dir, "app.go"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("x")); err == nil {
		t.Fatal("overwriting a directory succeeded")
	}
}

func TestWriteAppFileCreatesAndOverwrites(t *testing.T) {
	home := setupAppStoreTest(t)
	makeAppDir(t, home, "draft", "demo")
	if err := WriteAppFile("draft/demo", "static/css/a.css", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "static/css/a.css", []byte("two")); err != nil {
		t.Fatal(err)
	}
	data, err := ReadAppFile("draft/demo", "static/css/a.css")
	if err != nil || string(data.Contents) != "two" {
		t.Fatalf("read back %q, %v", data, err)
	}
}

func TestSymlinkedWaveappsRootAllowed(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	realRoot := t.TempDir()
	if err := os.Symlink(realRoot, filepath.Join(home, "waveapps")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("package main\n")); err != nil {
		t.Fatalf("write through a symlinked ~/waveapps: %v", err)
	}
	data, err := ReadAppFile("draft/demo", "app.go")
	if err != nil || string(data.Contents) != "package main\n" {
		t.Fatalf("read through a symlinked ~/waveapps: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(realRoot, "draft", "demo", "app.go")); err != nil {
		t.Fatalf("app.go not under the real root: %v", err)
	}
}

func TestSymlinkedNamespaceRejected(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	realNs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "waveapps"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realNs, filepath.Join(home, "waveapps", "draft")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("x")); err == nil {
		t.Error("write through a symlinked namespace succeeded")
	}
	if _, err := ReadAppFile("draft/demo", "app.go"); err == nil {
		t.Error("read through a symlinked namespace succeeded")
	}
	if entries, _ := os.ReadDir(realNs); len(entries) != 0 {
		t.Fatalf("files created in the symlink target: %v", entries)
	}
}
