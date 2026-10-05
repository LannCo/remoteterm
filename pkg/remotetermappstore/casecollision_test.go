// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedAppRejectsCaseOnlyCollision(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "draft", "MyApp")
	_, err := SeedApp("draft/myapp")
	if err == nil || !strings.Contains(err.Error(), "MyApp") {
		t.Fatalf("SeedApp(draft/myapp) = %v, want a collision error naming MyApp", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, "waveapps", "draft", "myapp")); !os.IsNotExist(statErr) {
		t.Fatalf("colliding folder was created: %v", statErr)
	}
}

func TestSeedAppAllowsExactExistingName(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "draft", "MyApp")
	if _, err := SeedApp("draft/MyApp"); err != nil {
		t.Fatalf("re-seeding the same spelling: %v", err)
	}
}

func TestSeedAppCollisionIsPerNamespace(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "local", "MyApp")
	if _, err := SeedApp("draft/myapp"); err != nil {
		t.Fatalf("same name in another namespace: %v", err)
	}
}

func TestWriteAppFileRejectsCaseOnlyCollision(t *testing.T) {
	home := setupAppStoreTest(t)
	makeAppDir(t, home, "draft", "MyApp")
	if err := WriteAppFile("draft/myapp", "app.go", []byte("x")); err == nil {
		t.Fatal("write to a case-variant of an existing app succeeded")
	}
	if err := WriteAppFile("draft/MyApp", "app.go", []byte("x")); err != nil {
		t.Fatalf("write to the exact app: %v", err)
	}
}

func TestRenameLocalAppRejectsCaseOnlyCollision(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "local", "alpha")
	makeAppDir(t, home, "local", "Beta")
	err := RenameLocalApp("alpha", "beta")
	if err == nil || !strings.Contains(err.Error(), "Beta") {
		t.Fatalf("RenameLocalApp(alpha, beta) = %v, want a collision error naming Beta", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "waveapps", "local", "alpha")); statErr != nil {
		t.Fatalf("source folder moved: %v", statErr)
	}
}

func TestRenameLocalAppRejectsCollisionInDraftNamespace(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "local", "alpha")
	makeAppDir(t, home, "draft", "Beta")
	if err := RenameLocalApp("alpha", "beta"); err == nil {
		t.Fatal("rename onto a case-variant draft succeeded")
	}
}

func TestRenameLocalAppAllowsCaseChangeOfItself(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "local", "alpha")
	// On a case-insensitive volume the existing "already exists" check still applies; the
	// collision check must not be what refuses a rename of an app to its own case-variant.
	err := RenameLocalApp("alpha", "Alpha")
	if err != nil && strings.Contains(err.Error(), "differ only by case") {
		t.Fatalf("self rename rejected as a collision: %v", err)
	}
}

func TestMakeDraftFromLocalRejectsCaseOnlyCollision(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	localDir := makeAppDir(t, home, "local", "demo")
	if err := os.WriteFile(filepath.Join(localDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	makeAppDir(t, home, "draft", "Demo")
	if _, err := MakeDraftFromLocal("local/demo"); err == nil {
		t.Fatal("draft created next to a case-variant draft")
	}
}

func TestPublishDraftRejectsCaseOnlyCollision(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	draftDir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(draftDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	makeAppDir(t, home, "local", "Demo")
	if _, err := PublishDraft("draft/demo"); err == nil {
		t.Fatal("publish created a case-variant local app")
	}
}
