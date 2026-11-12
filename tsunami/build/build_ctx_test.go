// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunGoBuildStopsWhenContextIsCancelled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the go binary")
	}
	// The sleep child keeps the output pipe open after the shell is killed, as go's
	// compile and link children do.
	fakeGo := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nsleep 5\n"), 0755); err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(200*time.Millisecond, cancel)

	begin := time.Now()
	_, err := runGoBuild(tempDir, &BuildEnv{GoPath: fakeGo}, BuildOpts{Ctx: ctx, OutputCapture: MakeOutputCapture()})
	elapsed := time.Since(begin)
	if err == nil {
		t.Fatal("a cancelled build reported success")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("runGoBuild returned %v after the context was cancelled, want under 3s", elapsed)
	}
}
