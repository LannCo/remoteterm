// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeGo(t *testing.T, path string, versionLine string, markerPath string) {
	t.Helper()
	script := "#!/bin/sh\n"
	if markerPath != "" {
		script += "printf '%s' \"$GOTOOLCHAIN\" > '" + markerPath + "'\n"
	}
	script += "if [ \"$1\" = version ]; then echo '" + versionLine + "'; exit 0; fi\nexit 1\n"
	writeTestFile(t, path, script)
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func TestReadSdkGoVersion(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n")
	got, err := ReadSdkGoVersion(dir)
	if err != nil || got != "1.25.6" {
		t.Fatalf("ReadSdkGoVersion = %q, %v; want 1.25.6", got, err)
	}

	noGoLine := t.TempDir()
	writeTestFile(t, filepath.Join(noGoLine, "go.mod"), "module example.com/x\n")
	if _, err := ReadSdkGoVersion(noGoLine); err == nil {
		t.Fatal("expected an error for a go.mod without a go directive")
	}

	if _, err := ReadSdkGoVersion(t.TempDir()); err == nil {
		t.Fatal("expected an error for a missing go.mod")
	}

	repo, err := ReadSdkGoVersion("..")
	if err != nil || repo == "" {
		t.Fatalf("ReadSdkGoVersion(..) = %q, %v", repo, err)
	}
}

func TestCompareGoVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.25.5", "1.25.6", -1},
		{"1.26.0", "1.25.6", 1},
		{"1.25.6", "1.25.6", 0},
		{"go1.25rc1", "1.25.6", -1},
		{"1.25rc1", "1.25.0", -1},
		{"1.9", "1.10", -1},
		{"go1.26.1", "go1.26.0", 1},
	}
	for _, tc := range cases {
		if got := CompareGoVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareGoVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseGoVersionOutput(t *testing.T) {
	cases := map[string]string{
		"go version go1.26.3 linux/amd64":          "1.26.3",
		"go version go1.25rc1 darwin/arm64":        "1.25rc1",
		"go version go1.22 linux/amd64":            "1.22",
		"go version devel go1.27-abcdef Tue linux": "1.27",
	}
	for in, want := range cases {
		got, ok := ParseGoVersionOutput(in)
		if !ok || got != want {
			t.Errorf("ParseGoVersionOutput(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := ParseGoVersionOutput("not a version"); ok {
		t.Error("expected ok=false for unparseable output")
	}
}

func TestCheckGoVersionAgainstFloor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go uses a shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "gotoolchain")
	newGo := filepath.Join(dir, "new", "go")
	writeFakeGo(t, newGo, "go version go1.26.0 linux/amd64", marker)
	res := CheckGoVersion(newGo, "1.25.6")
	if res.GoStatus != GoStatus_Ok || res.Version != "1.26.0" {
		t.Fatalf("new go: status %q version %q (%s)", res.GoStatus, res.Version, res.ErrorString)
	}
	toolchain, err := os.ReadFile(marker)
	if err != nil || string(toolchain) != "local" {
		t.Fatalf("GOTOOLCHAIN seen by go = %q, %v; want local", toolchain, err)
	}

	oldGo := filepath.Join(dir, "old", "go")
	writeFakeGo(t, oldGo, "go version go1.25.5 linux/amd64", "")
	res = CheckGoVersion(oldGo, "1.25.6")
	if res.GoStatus != GoStatus_BadVersion || res.Version != "1.25.5" {
		t.Fatalf("old go: status %q version %q", res.GoStatus, res.Version)
	}

	res = CheckGoVersion(filepath.Join(dir, "missing", "go"), "1.25.6")
	if res.GoStatus != GoStatus_NotFound {
		t.Fatalf("missing custom go: status %q, want notfound", res.GoStatus)
	}
}

func TestVerifyEnvironmentUsesMinGoVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go uses a shell script")
	}
	oldGo := filepath.Join(t.TempDir(), "go")
	writeFakeGo(t, oldGo, "go version go1.25.5 linux/amd64", "")
	_, err := verifyEnvironment(false, BuildOpts{SdkReplacePath: "/sdk", GoPath: oldGo, MinGoVersion: "1.25.6"})
	if err == nil || !strings.Contains(err.Error(), "1.25.6") {
		t.Fatalf("expected an error naming 1.25.6, got %v", err)
	}
}
