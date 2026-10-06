// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
