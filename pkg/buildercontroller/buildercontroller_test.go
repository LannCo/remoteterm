// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/utilds"
)

// Secret bindings live in the data dir; without one a build fails before it would start
// its app, and the start check goes untested. It is set once here, before any test runs:
// writing it inside a test races with status goroutines left over from earlier tests.
func TestMain(m *testing.M) {
	dataDir, err := os.MkdirTemp("", "buildercontroller-data-*")
	if err != nil {
		fmt.Printf("cannot create a test data dir: %v\n", err)
		os.Exit(1)
	}
	remotetermbase.DataHome_VarCache = dataDir
	code := m.Run()
	os.RemoveAll(dataDir)
	os.Exit(code)
}

func setupBuilderTest(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	resources := t.TempDir()
	orig := remotetermbase.AppResourcesPath_VarCache
	remotetermbase.AppResourcesPath_VarCache = resources
	t.Cleanup(func() { remotetermbase.AppResourcesPath_VarCache = orig })
	return home, resources
}

func makeTestApp(t *testing.T, home string, appName string) string {
	t.Helper()
	appDir := filepath.Join(home, "waveapps", "draft", appName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return appDir
}

func TestBuildAndRunReportsMissingSdk(t *testing.T) {
	home, resources := setupBuilderTest(t)
	makeTestApp(t, home, "demo")
	bc := &BuilderController{
		builderId:    "test-missing-sdk",
		appId:        "draft/demo",
		status:       BuilderStatus_Building,
		outputBuffer: utilds.MakeMultiReaderLineBuffer(100),
	}
	resultCh := make(chan *BuildResult, 1)
	bc.buildAndRun(context.Background(), "draft/demo", nil, resultCh)

	want := fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, filepath.Join(resources, "tsunamisdk"))
	result := <-resultCh
	if result.Success || result.ErrorMessage != want {
		t.Fatalf("result = %+v\nwant error %s", result, want)
	}
	status := bc.GetStatus()
	if status.Status != BuilderStatus_Error || status.ErrorMsg != want {
		t.Fatalf("status = %q / %q", status.Status, status.ErrorMsg)
	}
	lines := bc.GetOutput()
	if len(lines) == 0 || lines[len(lines)-1] != "[error] "+want {
		t.Fatalf("build output = %q; want a final [error] line", lines)
	}
}

func TestBuildLogRecordsLatestBuildOnly(t *testing.T) {
	home, resources := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-build-log")
	bc.appId = "draft/demo"
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(100)
	bc.buildAndRun(context.Background(), "draft/demo", nil, nil)

	logPath := filepath.Join(appDir, ".tsunami", "build.log")
	first, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("build log not written: %v", err)
	}
	if !strings.Contains(string(first), "Tsunami SDK not found") || !strings.HasSuffix(string(first), "status: error\n") {
		t.Fatalf("first build log:\n%s", first)
	}

	sdk := filepath.Join(resources, "tsunamisdk")
	if err := os.MkdirAll(sdk, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "go.mod"), []byte("module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(100)
	bc.buildAndRun(context.Background(), "draft/demo", nil, nil)
	second, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(second), "Tsunami SDK not found") || !strings.Contains(string(second), "Tsunami scaffold not found") {
		t.Fatalf("second build log was not truncated or is wrong:\n%s", second)
	}
}

func TestBuildLogFailureDoesNotChangeOutcome(t *testing.T) {
	home, resources := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	if err := os.WriteFile(filepath.Join(appDir, ".tsunami"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	bc := makeBuilderController("test-build-log-fail")
	bc.appId = "draft/demo"
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(100)
	resultCh := make(chan *BuildResult, 1)
	bc.buildAndRun(context.Background(), "draft/demo", nil, resultCh)
	want := fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, filepath.Join(resources, "tsunamisdk"))
	if result := <-resultCh; result.ErrorMessage != want {
		t.Fatalf("result error %q, want %q", result.ErrorMessage, want)
	}
	if status := bc.GetStatus(); status.Status != BuilderStatus_Error || status.ErrorMsg != want {
		t.Fatalf("status %q / %q", status.Status, status.ErrorMsg)
	}
}

func TestBuildLogStatusLinesMatchAgentNotes(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	logPath := filepath.Join(appDir, ".tsunami", "build.log")

	writeBuildLog("draft/demo", []string{"building", "listening"}, runningBuildLogStatus(4321))
	running, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(running) != "building\nlistening\nstatus: running on port 4321\n" {
		t.Fatalf("running build log = %q", running)
	}

	writeBuildLog("draft/demo", []string{"main.go:3: boom"}, BuildLogStatusError)
	failed, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(failed) != "main.go:3: boom\nstatus: error\n" {
		t.Fatalf("error build log = %q", failed)
	}

	// AGENTS.md tells agents to look for exactly these lines.
	notes, err := os.ReadFile(filepath.Join("..", "remotetermappstore", "starter", "files", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"`status: running on port <n>`", "`" + BuildLogStatusError + "`"} {
		if !strings.Contains(string(notes), line) {
			t.Fatalf("AGENTS.md does not quote %s", line)
		}
	}
	if strings.Replace(runningBuildLogStatus(0), "0", "<n>", 1) != "status: running on port <n>" {
		t.Fatalf("running status line %q has drifted from AGENTS.md", runningBuildLogStatus(0))
	}
}
