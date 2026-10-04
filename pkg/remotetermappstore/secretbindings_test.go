// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func setDataDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	orig := remotetermbase.DataHome_VarCache
	remotetermbase.DataHome_VarCache = dataDir
	t.Cleanup(func() { remotetermbase.DataHome_VarCache = orig })
	return dataDir
}

func TestSecretBindingsLiveUnderDataDir(t *testing.T) {
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	bindings := map[string]string{"API_KEY": "my-api-key"}
	if err := WriteAppSecretBindings("draft/demo", bindings); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataDir, "builder", "secret-bindings", "draft", "demo.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("bindings not at %s: %v", want, err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "secret-bindings.json")); err == nil {
		t.Fatal("bindings were written into the app folder")
	}
	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil || !maps.Equal(got, bindings) {
		t.Fatalf("read back %v, %v", got, err)
	}
}

func TestLegacyAppDirBindingsIgnored(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(appDir, "secret-bindings.json"), []byte(`{"API_KEY":"stolen"}`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no bindings", got, err)
	}
}

func TestPublishCopiesBindings(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bindings := map[string]string{"API_KEY": "my-api-key"}
	if err := WriteAppSecretBindings("draft/demo", bindings); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft("draft/demo"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("local/demo")
	if err != nil || !maps.Equal(got, bindings) {
		t.Fatalf("published bindings %v, %v", got, err)
	}
}

func TestSecretBindingsNeedDataDir(t *testing.T) {
	home := setupAppStoreTest(t)
	makeAppDir(t, home, "draft", "demo")
	orig := remotetermbase.DataHome_VarCache
	remotetermbase.DataHome_VarCache = ""
	t.Cleanup(func() { remotetermbase.DataHome_VarCache = orig })
	if err := WriteAppSecretBindings("draft/demo", map[string]string{"A": "b"}); err == nil {
		t.Fatal("expected an error with no data dir")
	}
}

func readBindingsOrFail(t *testing.T, appId string) map[string]string {
	t.Helper()
	got, err := ReadAppSecretBindings(appId)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestBindingsFollowRevertMakeDraftRenameAndDelete(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "local", "demo")
	published := map[string]string{"API_KEY": "published"}
	if err := WriteAppSecretBindings("local/demo", published); err != nil {
		t.Fatal(err)
	}

	if _, err := MakeDraftFromLocal("local/demo"); err != nil {
		t.Fatal(err)
	}
	if got := readBindingsOrFail(t, "draft/demo"); !maps.Equal(got, published) {
		t.Fatalf("make-draft bindings %v", got)
	}

	if err := WriteAppSecretBindings("draft/demo", map[string]string{"API_KEY": "edited"}); err != nil {
		t.Fatal(err)
	}
	if err := RevertDraft("draft/demo"); err != nil {
		t.Fatal(err)
	}
	if got := readBindingsOrFail(t, "draft/demo"); !maps.Equal(got, published) {
		t.Fatalf("reverted bindings %v", got)
	}

	if err := RenameLocalApp("demo", "renamed"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"local/renamed", "draft/renamed"} {
		if got := readBindingsOrFail(t, id); !maps.Equal(got, published) {
			t.Fatalf("%s bindings after rename %v", id, got)
		}
	}
	for _, id := range []string{"local/demo", "draft/demo"} {
		if got := readBindingsOrFail(t, id); len(got) != 0 {
			t.Fatalf("%s kept bindings after rename: %v", id, got)
		}
	}

	if err := DeleteApp("local/renamed"); err != nil {
		t.Fatal(err)
	}
	if got := readBindingsOrFail(t, "local/renamed"); len(got) != 0 {
		t.Fatalf("bindings survived delete: %v", got)
	}
	if got := readBindingsOrFail(t, "draft/renamed"); !maps.Equal(got, published) {
		t.Fatalf("deleting local removed the draft's bindings: %v", got)
	}
}

func TestPublishWithoutDraftBindingsClearsStaleLocalBindings(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "draft", "demo")
	if err := WriteAppSecretBindings("local/demo", map[string]string{"API_KEY": "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft("draft/demo"); err != nil {
		t.Fatal(err)
	}
	if got := readBindingsOrFail(t, "local/demo"); len(got) != 0 {
		t.Fatalf("stale bindings survived publish: %v", got)
	}
}
