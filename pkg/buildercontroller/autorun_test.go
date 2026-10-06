// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
)

// Trust records live in the data dir the whole package shares, so each test takes its own app
// name and clears the record on both sides.
func makeAutoRunFixture(t *testing.T, name string) (string, *BuilderController, *atomic.Int32) {
	t.Helper()
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, name)
	appId := "draft/" + name
	remotetermappstore.DeleteTrustedBuildHash(appId)
	t.Cleanup(func() { remotetermappstore.DeleteTrustedBuildHash(appId) })
	bc := makeBuilderController("test-autorun-" + name)
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }
	return appDir, bc, &builds
}

func waitBuildsIdle(t *testing.T, bc *BuilderController) {
	t.Helper()
	waitUntil(t, 2*time.Second, func() bool { return !bc.isBuilding() }, "the build loop to finish")
}

func requireDeclined(t *testing.T, bc *BuilderController, appId string, builds *atomic.Int32, what string) {
	t.Helper()
	before := builds.Load()
	err := bc.RequestAutoRun(appId, nil)
	if !errors.Is(err, ErrAutoRunDeclined) {
		t.Fatalf("%s: RequestAutoRun = %v, want ErrAutoRunDeclined", what, err)
	}
	if !strings.Contains(err.Error(), AutoRunDeclinedCode) {
		t.Fatalf("%s: error %q lacks the code the frontend matches on", what, err)
	}
	time.Sleep(50 * time.Millisecond)
	if builds.Load() != before || bc.isBuilding() {
		t.Fatalf("%s: a declined auto-run started a build", what)
	}
}

func requireAccepted(t *testing.T, bc *BuilderController, appId string, builds *atomic.Int32, what string) {
	t.Helper()
	before := builds.Load()
	if err := bc.RequestAutoRun(appId, nil); err != nil {
		t.Fatalf("%s: RequestAutoRun = %v", what, err)
	}
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == before+1 }, what+": the build")
	waitBuildsIdle(t, bc)
}

func TestAutoRunDeclinedForAppNeverStartedByHand(t *testing.T) {
	_, bc, builds := makeAutoRunFixture(t, "ar-fresh")
	requireDeclined(t, bc, "draft/ar-fresh", builds, "fresh app")
}

func TestAutoRunAfterExplicitStartOfSameInputs(t *testing.T) {
	_, bc, builds := makeAutoRunFixture(t, "ar-same")
	bc.RequestUserRebuild("draft/ar-same", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the explicit build")
	waitBuildsIdle(t, bc)
	requireAccepted(t, bc, "draft/ar-same", builds, "same inputs")

	other := makeBuilderController("test-autorun-ar-same-2")
	var otherBuilds atomic.Int32
	other.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { otherBuilds.Add(1) }
	requireAccepted(t, other, "draft/ar-same", &otherBuilds, "a new controller after a restart")
}

func TestAutoRunDeclinedOnceAppGoChanges(t *testing.T) {
	appDir, bc, builds := makeAutoRunFixture(t, "ar-edit")
	bc.RequestUserRebuild("draft/ar-edit", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the explicit build")
	waitBuildsIdle(t, bc)

	appGo := filepath.Join(appDir, "app.go")
	if err := os.WriteFile(appGo, []byte("package main\n// edited by someone else\n"), 0644); err != nil {
		t.Fatal(err)
	}
	requireDeclined(t, bc, "draft/ar-edit", builds, "edited app.go")

	if err := os.WriteFile(appGo, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	requireAccepted(t, bc, "draft/ar-edit", builds, "app.go restored")
}

func TestAutoRunDeclinedWhenGoModOrGoSumChange(t *testing.T) {
	appDir, bc, builds := makeAutoRunFixture(t, "ar-mod")
	bc.RequestUserRebuild("draft/ar-mod", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the explicit build")
	waitBuildsIdle(t, bc)

	for _, name := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(appDir, name)
		if err := os.WriteFile(path, []byte("replace example.com/x => /somewhere\n"), 0644); err != nil {
			t.Fatal(err)
		}
		requireDeclined(t, bc, "draft/ar-mod", builds, name+" added")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		requireAccepted(t, bc, "draft/ar-mod", builds, name+" removed")
	}
}

func TestAutoRunDeclinedWhenStaticFileChanges(t *testing.T) {
	appDir, bc, builds := makeAutoRunFixture(t, "ar-static")
	bc.RequestUserRebuild("draft/ar-static", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the explicit build")
	waitBuildsIdle(t, bc)

	writeAppFileForTest(t, appDir, "static/logo.txt", "new asset")
	requireDeclined(t, bc, "draft/ar-static", builds, "new static file")
}

func TestStartRecordsTrust(t *testing.T) {
	_, bc, builds := makeAutoRunFixture(t, "ar-start")
	if err := bc.Start(context.Background(), "draft/ar-start", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the build")
	waitBuildsIdle(t, bc)
	if remotetermappstore.ReadTrustedBuildHash("draft/ar-start") == "" {
		t.Fatal("Start left no trust record")
	}
}

func TestSaveRecordsTrust(t *testing.T) {
	_, bc, builds := makeAutoRunFixture(t, "ar-save")
	DeleteController("test-autorun-ar-save")
	controllerMap["test-autorun-ar-save"] = bc
	t.Cleanup(func() { DeleteController("test-autorun-ar-save") })
	setBuilderRtInfoForTest(t, "test-autorun-ar-save", "draft/ar-save", nil)
	if err := RequestRebuildAfterSave("test-autorun-ar-save", "draft/ar-save"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the build")
	waitBuildsIdle(t, bc)
	requireAccepted(t, bc, "draft/ar-save", builds, "after a save")
}

func TestWatcherRebuildNeverRecordsTrust(t *testing.T) {
	appDir, bc, builds := makeAutoRunFixture(t, "ar-live")
	overrideWatchSeams(t, true)
	setBuilderRtInfoForTest(t, bc.builderId, "draft/ar-live", nil)
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n// outside edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.handleAppFilesChanged("draft/ar-live")
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the live rebuild")
	waitBuildsIdle(t, bc)
	if got := remotetermappstore.ReadTrustedBuildHash("draft/ar-live"); got != "" {
		t.Fatalf("a watcher-triggered rebuild recorded trust %q", got)
	}
	requireDeclined(t, bc, "draft/ar-live", builds, "after a live rebuild of an outside edit")
}

// The first build writes go.mod back into the app folder; without a refresh the next open
// would see changed inputs and decline.
func TestTrustFollowsBuildRewritingGoMod(t *testing.T) {
	appDir, bc, builds := makeAutoRunFixture(t, "ar-rewrite")
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		builds.Add(1)
		os.WriteFile(filepath.Join(appDir, "go.mod"), []byte("module tsunami/app/ar-rewrite\n"), 0644)
		os.WriteFile(filepath.Join(appDir, "go.sum"), []byte("example.com/x v1 h1:abc\n"), 0644)
	}
	bc.RequestUserRebuild("draft/ar-rewrite", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the explicit build")
	waitBuildsIdle(t, bc)
	requireAccepted(t, bc, "draft/ar-rewrite", builds, "after the build rewrote go.mod")
}

func TestTrustNotRefreshedWhenAppGoChangedDuringBuild(t *testing.T) {
	appDir, bc, builds := makeAutoRunFixture(t, "ar-race")
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		builds.Add(1)
		os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n// slipped in mid-build\n"), 0644)
	}
	bc.RequestUserRebuild("draft/ar-race", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the explicit build")
	waitBuildsIdle(t, bc)
	requireDeclined(t, bc, "draft/ar-race", builds, "an edit that landed during the build")
}

func TestAutoRunDeclinedWhenRecordIsMissing(t *testing.T) {
	_, bc, builds := makeAutoRunFixture(t, "ar-nodata")
	bc.RequestUserRebuild("draft/ar-nodata", nil)
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the explicit build")
	waitBuildsIdle(t, bc)
	if err := remotetermappstore.DeleteTrustedBuildHash("draft/ar-nodata"); err != nil {
		t.Fatal(err)
	}
	requireDeclined(t, bc, "draft/ar-nodata", builds, "a missing record")
}
