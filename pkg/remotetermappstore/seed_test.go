// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore/starter"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func TestSeedAppWritesStarterFiles(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	written, err := SeedApp("draft/fresh")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"app.go", "AGENTS.md", "CLAUDE.md", "TSUNAMI_GUIDE.md"}
	if !slices.Equal(written, want) {
		t.Fatalf("written = %v, want %v", written, want)
	}
	files, _ := starter.GetStarterFiles()
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(home, "waveapps", "draft", "fresh", f.Name))
		if err != nil || string(data) != string(f.Data) {
			t.Errorf("%s not written as embedded: %v", f.Name, err)
		}
	}
}

func TestSeedAppNeverOverwrites(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "mine")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main // mine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/mine")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(written, "app.go") {
		t.Fatal("app.go reported as written")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "app.go"))
	if string(data) != "package main // mine\n" {
		t.Fatalf("app.go was overwritten: %q", data)
	}
}

func TestSeedAppSkipsDanglingSymlink(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "linked")
	target := filepath.Join(t.TempDir(), "outside.go")
	if err := os.Symlink(target, filepath.Join(dir, "app.go")); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/linked")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(written, "app.go") {
		t.Fatal("app.go reported as written through a symlink")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the symlink target was created")
	}
}

func TestSeedAppRefusesSymlinkedAppDir(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, "waveapps", "draft", "evil")); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/evil"); err == nil {
		t.Fatal("seeding a symlinked app folder succeeded")
	}
	if entries, _ := os.ReadDir(real); len(entries) != 0 {
		t.Fatalf("files written into the symlink target: %v", entries)
	}
}

func TestSeedAppRejectsInvalidIds(t *testing.T) {
	setupAppStoreTest(t)
	setDataDir(t)
	for _, id := range []string{"", "nonamespace", "draft/../escape", "draft/has space", "draft/a/b"} {
		if _, err := SeedApp(id); err == nil {
			t.Errorf("SeedApp(%q) succeeded", id)
		}
	}
}

func TestSeedAppFillsEmptyAppGo(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "empty")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/empty")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(written, "app.go") {
		t.Fatalf("written = %v; an empty app.go should be filled", written)
	}
	files, _ := starter.GetStarterFiles()
	data, _ := os.ReadFile(filepath.Join(dir, "app.go"))
	if string(data) != string(files[0].Data) {
		t.Fatal("empty app.go was not replaced with the starter app")
	}
}

func TestSeedAppLeavesSymlinkToEmptyFile(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "emptylink")
	target := filepath.Join(t.TempDir(), "empty.go")
	if err := os.WriteFile(target, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "app.go")); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/emptylink")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(written, "app.go") {
		t.Fatal("wrote through a symlink to an empty file")
	}
	if data, _ := os.ReadFile(target); len(data) != 0 {
		t.Fatal("the symlink target was modified")
	}
}

func TestSeedAppClearsStaleBindingsForNewApp(t *testing.T) {
	setupAppStoreTest(t)
	setDataDir(t)
	if err := WriteAppSecretBindings("draft/reborn", map[string]string{"API_KEY": "old-binding"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/reborn"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("draft/reborn")
	if err != nil || len(got) != 0 {
		t.Fatalf("bindings after seeding a new app = %v, %v; want none", got, err)
	}
}

func TestSeedAppKeepsBindingsForExistingApp(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "draft", "kept")
	bindings := map[string]string{"API_KEY": "my-binding"}
	if err := WriteAppSecretBindings("draft/kept", bindings); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/kept"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("draft/kept")
	if err != nil || !maps.Equal(got, bindings) {
		t.Fatalf("bindings of an existing app changed: %v, %v", got, err)
	}
}

// A new app whose stale bindings cannot be cleared must not leave a folder behind: the next
// attempt would see an existing app and keep the stale bindings.
func TestSeedAppWithoutDataDirLeavesNoFolderForNewApp(t *testing.T) {
	home := setupAppStoreTest(t)
	orig := remotetermbase.DataHome_VarCache
	remotetermbase.DataHome_VarCache = ""
	t.Cleanup(func() { remotetermbase.DataHome_VarCache = orig })
	if _, err := SeedApp("draft/nodata"); err == nil {
		t.Fatal("seeding succeeded without a data dir")
	}
	if _, err := os.Lstat(filepath.Join(home, "waveapps", "draft", "nodata")); err == nil {
		t.Fatal("app folder was created although the bindings could not be cleared")
	}
}

func TestSeedAppThroughSymlinkedRootButNotNamespace(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	realRoot := t.TempDir()
	if err := os.Symlink(realRoot, filepath.Join(home, "waveapps")); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/seeded"); err != nil {
		t.Fatalf("seed through a symlinked ~/waveapps: %v", err)
	}
	if _, err := os.Stat(filepath.Join(realRoot, "draft", "seeded", "app.go")); err != nil {
		t.Fatalf("seeded app.go not under the real root: %v", err)
	}
	realNs := t.TempDir()
	if err := os.Symlink(realNs, filepath.Join(realRoot, "local")); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("local/seeded"); err == nil {
		t.Fatal("seed through a symlinked namespace succeeded")
	}
	if entries, _ := os.ReadDir(realNs); len(entries) != 0 {
		t.Fatalf("files created in the symlink target: %v", entries)
	}
}
