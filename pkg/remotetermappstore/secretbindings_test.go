// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"maps"
	"os"
	"path/filepath"
	"runtime"
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

func unsetDataDir(t *testing.T) {
	t.Helper()
	orig := remotetermbase.DataHome_VarCache
	remotetermbase.DataHome_VarCache = ""
	t.Cleanup(func() { remotetermbase.DataHome_VarCache = orig })
}

func TestSecretBindingsNeedDataDir(t *testing.T) {
	home := setupAppStoreTest(t)
	makeAppDir(t, home, "draft", "demo")
	unsetDataDir(t)
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

func writeBindingsFile(t *testing.T, dataDir string, appId string, contents string) string {
	t.Helper()
	ns, name, _ := ParseAppId(appId)
	path := filepath.Join(dataDir, "builder", "secret-bindings", ns, name+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A non-empty directory where the file should be makes every remove of it fail on
// every platform and for root, which a read-only parent does not.
func blockBindingsRemoval(t *testing.T, dataDir string, appId string) string {
	t.Helper()
	ns, name, _ := ParseAppId(appId)
	path := filepath.Join(dataDir, "builder", "secret-bindings", ns, name+".json")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A regular file where the namespace directory should be makes every write fail.
func blockBindingsNamespace(t *testing.T, dataDir string, ns string) {
	t.Helper()
	root := filepath.Join(dataDir, "builder", "secret-bindings")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ns), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be absent", path)
	}
}

func TestDeleteAppKeepsFolderWhenBindingsRemovalFails(t *testing.T) {
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	blocked := blockBindingsRemoval(t, dataDir, "draft/demo")
	if err := DeleteApp("draft/demo"); err == nil {
		t.Fatal("expected an error when the bindings cannot be removed")
	}
	mustExist(t, appDir)

	if err := os.RemoveAll(blocked); err != nil {
		t.Fatal(err)
	}
	writeBindingsFile(t, dataDir, "draft/demo", `{"API_KEY":"stale"}`)
	if err := DeleteApp("draft/demo"); err != nil {
		t.Fatal(err)
	}
	mustNotExist(t, appDir)
	if got := readBindingsOrFail(t, "draft/demo"); len(got) != 0 {
		t.Fatalf("stale bindings survived the retry: %v", got)
	}
}

func TestRenameRestoresBindingsWhenFolderRenameFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a read-only directory that is enforced against the caller")
	}
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	makeAppDir(t, home, "local", "demo")
	makeAppDir(t, home, "draft", "demo")
	published := map[string]string{"API_KEY": "published"}
	drafted := map[string]string{"API_KEY": "drafted"}
	if err := WriteAppSecretBindings("local/demo", published); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppSecretBindings("draft/demo", drafted); err != nil {
		t.Fatal(err)
	}
	// Local renames, then the draft rename fails: both folders and both bindings must end where they began.
	draftNS := filepath.Join(home, "waveapps", "draft")
	if err := os.Chmod(draftNS, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(draftNS, 0755) })

	if err := RenameLocalApp("demo", "renamed"); err == nil {
		t.Fatal("expected the draft folder rename to fail")
	}
	mustExist(t, filepath.Join(home, "waveapps", "local", "demo"))
	mustNotExist(t, filepath.Join(home, "waveapps", "local", "renamed"))
	if got := readBindingsOrFail(t, "local/demo"); !maps.Equal(got, published) {
		t.Fatalf("local bindings after failed rename: %v", got)
	}
	if got := readBindingsOrFail(t, "draft/demo"); !maps.Equal(got, drafted) {
		t.Fatalf("draft bindings after failed rename: %v", got)
	}
	for _, id := range []string{"local/renamed", "draft/renamed"} {
		if got := readBindingsOrFail(t, id); len(got) != 0 {
			t.Fatalf("%s gained bindings: %v", id, got)
		}
	}
	mustExist(t, filepath.Join(dataDir, "builder", "secret-bindings", "local", "demo.json"))
}

func TestRenameChangesNothingWhenBindingsMoveFails(t *testing.T) {
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	makeAppDir(t, home, "local", "demo")
	published := map[string]string{"API_KEY": "published"}
	if err := WriteAppSecretBindings("local/demo", published); err != nil {
		t.Fatal(err)
	}
	// The new id's bindings file is a non-empty directory, so the move cannot land.
	blockBindingsRemoval(t, dataDir, "local/renamed")
	if err := RenameLocalApp("demo", "renamed"); err == nil {
		t.Fatal("expected the bindings move to fail")
	}
	mustExist(t, filepath.Join(home, "waveapps", "local", "demo"))
	mustNotExist(t, filepath.Join(home, "waveapps", "local", "renamed"))
	if got := readBindingsOrFail(t, "local/demo"); !maps.Equal(got, published) {
		t.Fatalf("bindings after failed rename: %v", got)
	}
}

func TestMakeDraftLeavesNoDraftWhenBindingsCopyFails(t *testing.T) {
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	makeAppDir(t, home, "local", "demo")
	if err := WriteAppSecretBindings("local/demo", map[string]string{"API_KEY": "published"}); err != nil {
		t.Fatal(err)
	}
	blockBindingsNamespace(t, dataDir, "draft")
	if _, err := MakeDraftFromLocal("local/demo"); err == nil {
		t.Fatal("expected an error when the bindings cannot be copied")
	}
	mustNotExist(t, filepath.Join(home, "waveapps", "draft", "demo"))
}

func TestCopyAndMoveBindingsKeepUnparseableSource(t *testing.T) {
	setupAppStoreTest(t)
	dataDir := setDataDir(t)
	const corrupt = "not json {"
	writeBindingsFile(t, dataDir, "draft/demo", corrupt)
	if err := copySecretBindings("draft/demo", "local/demo"); err != nil {
		t.Fatalf("copy of an unparseable source: %v", err)
	}
	if err := moveSecretBindings("local/demo", "local/other"); err != nil {
		t.Fatalf("move of an unparseable source: %v", err)
	}
	for id, want := range map[string]string{"draft/demo": corrupt, "local/other": corrupt} {
		ns, name, _ := ParseAppId(id)
		got, err := os.ReadFile(filepath.Join(dataDir, "builder", "secret-bindings", ns, name+".json"))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q, %v", id, got, err)
		}
	}
	mustNotExist(t, filepath.Join(dataDir, "builder", "secret-bindings", "local", "demo.json"))
}

func TestMoveBindingsWithMissingSourceClearsTarget(t *testing.T) {
	setupAppStoreTest(t)
	dataDir := setDataDir(t)
	target := writeBindingsFile(t, dataDir, "local/renamed", `{"API_KEY":"stale"}`)
	if err := moveSecretBindings("local/demo", "local/renamed"); err != nil {
		t.Fatal(err)
	}
	mustNotExist(t, target)
}

func TestOperationsFailBeforeFolderWorkWithoutDataDir(t *testing.T) {
	home := setupAppStoreTest(t)
	unsetDataDir(t)
	localDemo := makeAppDir(t, home, "local", "demo")
	draftDemo := makeAppDir(t, home, "draft", "demo")
	for dir, content := range map[string]string{localDemo: "local", draftDemo: "draft"} {
		if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	assertUnchanged := func(t *testing.T) {
		t.Helper()
		for dir, want := range map[string]string{localDemo: "local", draftDemo: "draft"} {
			got, err := os.ReadFile(filepath.Join(dir, "app.go"))
			if err != nil || string(got) != want {
				t.Fatalf("%s changed: %q, %v", dir, got, err)
			}
		}
		mustNotExist(t, filepath.Join(home, "waveapps", "local", "renamed"))
		mustNotExist(t, filepath.Join(home, "waveapps", "draft", "renamed"))
	}

	t.Run("publish", func(t *testing.T) {
		if _, err := PublishDraft("draft/demo"); err == nil {
			t.Fatal("expected an error")
		}
		assertUnchanged(t)
	})
	t.Run("revert", func(t *testing.T) {
		if err := RevertDraft("draft/demo"); err == nil {
			t.Fatal("expected an error")
		}
		assertUnchanged(t)
	})
	t.Run("rename", func(t *testing.T) {
		if err := RenameLocalApp("demo", "renamed"); err == nil {
			t.Fatal("expected an error")
		}
		assertUnchanged(t)
	})
	t.Run("delete", func(t *testing.T) {
		if err := DeleteApp("draft/demo"); err == nil {
			t.Fatal("expected an error")
		}
		assertUnchanged(t)
	})
	t.Run("make-draft", func(t *testing.T) {
		if err := os.RemoveAll(draftDemo); err != nil {
			t.Fatal(err)
		}
		defer makeAppDir(t, home, "draft", "demo")
		if _, err := MakeDraftFromLocal("local/demo"); err == nil {
			t.Fatal("expected an error")
		}
		mustNotExist(t, draftDemo)
	})
}
