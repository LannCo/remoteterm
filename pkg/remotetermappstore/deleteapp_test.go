// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteAppRemovesFolderAndBindings(t *testing.T) {
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppSecretBindings("draft/demo", map[string]string{"K": "v"}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteApp("draft/demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(appDir); !os.IsNotExist(err) {
		t.Fatalf("app folder survived: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dataDir, "builder", "secret-bindings", "draft", "demo.json")); !os.IsNotExist(err) {
		t.Fatalf("bindings survived: %v", err)
	}
}

func TestDeleteAppMissingIsNotAnError(t *testing.T) {
	setupAppStoreTest(t)
	setDataDir(t)
	if err := DeleteApp("draft/nothere"); err != nil {
		t.Fatalf("deleting a missing app: %v", err)
	}
}

func TestDeleteAppRefusesSymlinkedNamespace(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	realNs := t.TempDir()
	victim := filepath.Join(realNs, "demo")
	if err := os.MkdirAll(victim, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "keep.txt"), []byte("precious"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "waveapps"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realNs, filepath.Join(home, "waveapps", "draft")); err != nil {
		t.Fatal(err)
	}
	if err := DeleteApp("draft/demo"); err == nil {
		t.Fatal("delete through a symlinked namespace succeeded")
	}
	if _, err := os.Stat(filepath.Join(victim, "keep.txt")); err != nil {
		t.Fatalf("file outside the app store was deleted: %v", err)
	}
}

func TestDeleteAppRefusesSymlinkedAppDir(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "keep.txt"), []byte("precious"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "waveapps", "draft", "demo")); err != nil {
		t.Fatal(err)
	}
	if err := DeleteApp("draft/demo"); err == nil {
		t.Fatal("delete of a symlinked app folder succeeded")
	}
	if _, err := os.Stat(filepath.Join(target, "keep.txt")); err != nil {
		t.Fatalf("symlink target was touched: %v", err)
	}
}
