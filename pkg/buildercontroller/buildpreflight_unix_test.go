// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package buildercontroller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/utilds"
)

func runBuildForPreflightTest(t *testing.T, builderId string, appId string) *BuildResult {
	t.Helper()
	bc := makeBuilderController(builderId)
	bc.appId = appId
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(100)
	resultCh := make(chan *BuildResult, 1)
	go bc.buildAndRun(context.Background(), appId, nil, resultCh)
	select {
	case result := <-resultCh:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("the build did not finish within 2s (blocked on a FIFO?)")
		return nil
	}
}

func TestBuildFailsFastOnFifoNamedAsGoFile(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "fifo")
	if err := syscall.Mkfifo(filepath.Join(appDir, "zz.go"), 0644); err != nil {
		t.Fatal(err)
	}
	result := runBuildForPreflightTest(t, "test-preflight-fifo", "draft/fifo")
	if result.Success || !strings.Contains(result.ErrorMessage, "refusing to build: zz.go") {
		t.Fatalf("result = %+v, want a refusal naming zz.go", result)
	}
	if !strings.Contains(result.BuildOutput, "[error] refusing to build: zz.go") {
		t.Fatalf("build output = %q, want an [error] line naming zz.go", result.BuildOutput)
	}
}

func TestBuildPreflightRefusesSpecialFilesAndSymlinks(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, appDir string, outside string)
		want  string
	}{
		{"symlinked go.mod", func(t *testing.T, appDir string, outside string) {
			mustSymlink(t, filepath.Join(outside, "go.mod"), filepath.Join(appDir, "go.mod"))
		}, "go.mod"},
		{"symlinked go.sum", func(t *testing.T, appDir string, outside string) {
			mustSymlink(t, filepath.Join(outside, "go.sum"), filepath.Join(appDir, "go.sum"))
		}, "go.sum"},
		{"symlinked manifest.json", func(t *testing.T, appDir string, outside string) {
			mustSymlink(t, filepath.Join(outside, "manifest.json"), filepath.Join(appDir, "manifest.json"))
		}, "manifest.json"},
		{"symlinked static", func(t *testing.T, appDir string, outside string) {
			mustSymlink(t, outside, filepath.Join(appDir, "static"))
		}, "static"},
		{"symlinked static/tw.css", func(t *testing.T, appDir string, outside string) {
			mustMkdir(t, filepath.Join(appDir, "static"))
			mustSymlink(t, filepath.Join(outside, "tw.css"), filepath.Join(appDir, "static", "tw.css"))
		}, "static/tw.css"},
		{"FIFO deep in static", func(t *testing.T, appDir string, outside string) {
			mustMkdir(t, filepath.Join(appDir, "static", "img"))
			if err := syscall.Mkfifo(filepath.Join(appDir, "static", "img", "x"), 0644); err != nil {
				t.Fatal(err)
			}
		}, "static/img/x"},
		{"symlinked bin", func(t *testing.T, appDir string, outside string) {
			mustSymlink(t, outside, filepath.Join(appDir, "bin"))
		}, "bin"},
		{"symlinked bin/app", func(t *testing.T, appDir string, outside string) {
			mustMkdir(t, filepath.Join(appDir, "bin"))
			mustSymlink(t, filepath.Join(outside, "tool"), filepath.Join(appDir, "bin", "app"))
		}, "bin/app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, _ := setupBuilderTest(t)
			appDir := makeTestApp(t, home, "pre")
			outside := t.TempDir()
			for _, name := range []string{"go.mod", "go.sum", "manifest.json", "tw.css", "tool"} {
				if err := os.WriteFile(filepath.Join(outside, name), []byte("outside"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			tc.setup(t, appDir, outside)
			err := checkAppBuildInputs(appDir)
			if err == nil || !strings.Contains(err.Error(), "refusing to build: "+tc.want+" ") {
				t.Fatalf("checkAppBuildInputs = %v, want a refusal naming %s", err, tc.want)
			}
		})
	}
}

func TestBuildPreflightAcceptsAPlainApp(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "plain")
	for _, rel := range []string{"go.mod", "go.sum", "manifest.json", "helper.go", "static/tw.css", "static/img/logo.png", "bin/app"} {
		mustMkdir(t, filepath.Dir(filepath.Join(appDir, rel)))
		if err := os.WriteFile(filepath.Join(appDir, rel), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Files the build never reads may be anything.
	if err := syscall.Mkfifo(filepath.Join(appDir, "notes.fifo"), 0644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, t.TempDir(), filepath.Join(appDir, "docs"))
	if err := checkAppBuildInputs(appDir); err != nil {
		t.Fatalf("checkAppBuildInputs refused a plain app: %v", err)
	}
}

func mustSymlink(t *testing.T, target string, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
}
