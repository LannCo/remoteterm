// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func setResourcesPath(t *testing.T, path string) {
	t.Helper()
	orig := remotetermbase.AppResourcesPath_VarCache
	remotetermbase.AppResourcesPath_VarCache = path
	t.Cleanup(func() { remotetermbase.AppResourcesPath_VarCache = orig })
}

func TestGetTsunamiSdkPath(t *testing.T) {
	resources := t.TempDir()
	setResourcesPath(t, resources)
	want := filepath.Join(resources, "tsunamisdk")
	if got := GetTsunamiSdkPath(); got != want {
		t.Fatalf("GetTsunamiSdkPath() = %q, want %q", got, want)
	}
}

func TestResolveGoFmtPathNeverProbes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell is a shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "probed")
	shell := filepath.Join(dir, "bash")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n: > '"+marker+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", t.TempDir())
	if _, err := ResolveGoFmtPath(); err == nil {
		t.Fatal("expected an error before any discovery has run")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("ResolveGoFmtPath started a login-shell probe")
	}
}
