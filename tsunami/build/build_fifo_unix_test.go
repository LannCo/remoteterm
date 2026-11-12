// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package build

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func returnsWithin(t *testing.T, what string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not return within 2s (blocked on a FIFO?)", what)
		return nil
	}
}

func makeFifoAppDir(t *testing.T, fifoRel string) string {
	t.Helper()
	appDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(appDir, "static"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "static", "logo.png"), []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(appDir, fifoRel), 0644); err != nil {
		t.Fatal(err)
	}
	return appDir
}

func TestCopyFilesFromAppFSFailsFastOnFifo(t *testing.T) {
	for _, fifoRel := range []string{"zz.go", "static/x", "go.mod"} {
		t.Run(fifoRel, func(t *testing.T) {
			appDir := makeFifoAppDir(t, fifoRel)
			err := returnsWithin(t, "copyFilesFromAppFS", func() error {
				_, err := copyFilesFromAppFS(NewDirFS(appDir), appDir, t.TempDir(), false, MakeOutputCapture())
				return err
			})
			if err == nil {
				t.Fatalf("copying an app with a FIFO at %s succeeded", fifoRel)
			}
		})
	}
}

func TestParseAppFileDoesNotBlockOnFifo(t *testing.T) {
	appDir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(appDir, MainAppFileName), 0644); err != nil {
		t.Fatal(err)
	}
	returnsWithin(t, "parseAndValidateAppFile", func() error {
		_, err := parseAndValidateAppFile(NewDirFS(appDir))
		return err
	})
}

func TestCopyFilesFromAppFSCopiesRegularFiles(t *testing.T) {
	appDir := makeFifoAppDir(t, "notes.fifo")
	dest := t.TempDir()
	stats, err := copyFilesFromAppFS(NewDirFS(appDir), appDir, dest, false, MakeOutputCapture())
	if err != nil {
		t.Fatal(err)
	}
	if stats.GoFiles != 1 || stats.StaticFiles != 1 {
		t.Fatalf("stats = %+v, want 1 go file and 1 static file", stats)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "static", "logo.png")); err != nil || string(got) != "png" {
		t.Fatalf("static/logo.png = %q, %v", got, err)
	}
}
