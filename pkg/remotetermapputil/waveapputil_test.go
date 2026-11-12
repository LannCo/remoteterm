// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/tsunami/build"
)

func TestBundledGoFollowsTheResourcesPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in go is a shell script")
	}
	resources := t.TempDir()
	setResourcesPath(t, resources)
	bundledGo := filepath.Join(resources, build.BundledToolchainDirName, "bin", "go")
	if err := os.MkdirAll(filepath.Dir(bundledGo), 0755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'go version go1.26.2 darwin/arm64'; fi\n"
	if err := os.WriteFile(bundledGo, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	result := build.CheckGoVersion("", "1.25.6")
	if result.GoStatus != build.GoStatus_Ok || result.GoPath != bundledGo {
		t.Fatalf("CheckGoVersion = %+v, want the Go bundled under the resources path %s", result, bundledGo)
	}
}

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
