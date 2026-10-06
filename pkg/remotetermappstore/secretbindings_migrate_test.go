// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const legacyBindingsJSON = `{"API_KEY":"my-api-key"}`

func writeLegacyBindings(t *testing.T, appDir string, contents string) string {
	t.Helper()
	path := filepath.Join(appDir, "secret-bindings.json")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newBindingsPath(t *testing.T, appId string) string {
	t.Helper()
	path, err := GetSecretBindingsPath(appId)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLegacyBindingsMigrateOnFirstRead(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	legacy := writeLegacyBindings(t, appDir, legacyBindingsJSON)

	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"API_KEY": "my-api-key"}; !maps.Equal(got, want) {
		t.Fatalf("migrated bindings %v, want %v", got, want)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file survived the move: %v", err)
	}
	newPath := newBindingsPath(t, "draft/demo")
	info, err := os.Stat(newPath)
	if err != nil {
		t.Fatalf("migrated file missing: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("migrated file mode %o, want 600", perm)
		}
		dirInfo, err := os.Stat(filepath.Dir(newPath))
		if err != nil {
			t.Fatal(err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("migrated directory mode %o, want 700", perm)
		}
	}
	if again := readBindingsOrFail(t, "draft/demo"); !maps.Equal(again, got) {
		t.Fatalf("second read %v", again)
	}
}

func TestNewBindingsWinOverLegacy(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if err := WriteAppSecretBindings("draft/demo", map[string]string{"API_KEY": "current"}); err != nil {
		t.Fatal(err)
	}
	legacy := writeLegacyBindings(t, appDir, `{"API_KEY":"stale"}`)
	got := readBindingsOrFail(t, "draft/demo")
	if got["API_KEY"] != "current" {
		t.Fatalf("legacy overrode the current bindings: %v", got)
	}
	if _, err := os.Lstat(legacy); err != nil {
		t.Fatalf("legacy file touched although the new location exists: %v", err)
	}
}

func TestLegacySymlinkNotMigrated(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	outside := filepath.Join(t.TempDir(), "other.json")
	if err := os.WriteFile(outside, []byte(`{"API_KEY":"outside"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(appDir, "secret-bindings.json")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no bindings from a symlinked legacy file", got, err)
	}
	if _, err := os.Lstat(newBindingsPath(t, "draft/demo")); !os.IsNotExist(err) {
		t.Fatalf("a symlinked legacy file was migrated: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("symlink target removed: %v", err)
	}
}

func TestLegacyUnparseableNotMigrated(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	legacy := writeLegacyBindings(t, appDir, "not json")
	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no bindings", got, err)
	}
	if _, err := os.Lstat(legacy); err != nil {
		t.Fatalf("an unparseable legacy file should be left where it is: %v", err)
	}
	if _, err := os.Lstat(newBindingsPath(t, "draft/demo")); !os.IsNotExist(err) {
		t.Fatalf("an unparseable legacy file was migrated: %v", err)
	}
}

func TestLegacyOversizeNotMigrated(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	big := make([]byte, maxSecretBindingsSize+1)
	for i := range big {
		big[i] = ' '
	}
	writeLegacyBindings(t, appDir, string(big)+"{}")
	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no bindings", got, err)
	}
	if _, err := os.Lstat(newBindingsPath(t, "draft/demo")); !os.IsNotExist(err) {
		t.Fatalf("an oversize legacy file was migrated: %v", err)
	}
}

func TestLegacyBindingsFollowPublish(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeLegacyBindings(t, appDir, legacyBindingsJSON)
	if _, err := PublishDraft("draft/demo"); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"API_KEY": "my-api-key"}
	for _, id := range []string{"draft/demo", "local/demo"} {
		if got := readBindingsOrFail(t, id); !maps.Equal(got, want) {
			t.Fatalf("%s bindings after publish %v", id, got)
		}
	}
	localDir := filepath.Join(home, "waveapps", "local", "demo")
	if _, err := os.Lstat(filepath.Join(localDir, "secret-bindings.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy file travelled into the published folder: %v", err)
	}
}

func TestLegacyBindingsFollowRename(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "local", "demo")
	writeLegacyBindings(t, appDir, legacyBindingsJSON)
	if err := RenameLocalApp("demo", "renamed"); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"API_KEY": "my-api-key"}
	if got := readBindingsOrFail(t, "local/renamed"); !maps.Equal(got, want) {
		t.Fatalf("bindings after rename %v", got)
	}
	if got := readBindingsOrFail(t, "local/demo"); len(got) != 0 {
		t.Fatalf("old id kept bindings: %v", got)
	}
}

func TestWriteBindingsSupersedesLegacy(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	legacy := writeLegacyBindings(t, appDir, legacyBindingsJSON)
	if err := WriteAppSecretBindings("draft/demo", map[string]string{"OTHER": "x"}); err != nil {
		t.Fatal(err)
	}
	if got := readBindingsOrFail(t, "draft/demo"); got["OTHER"] != "x" || len(got) != 1 {
		t.Fatalf("bindings after write %v", got)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file survived a write: %v", err)
	}
}
