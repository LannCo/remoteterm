// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTrustedBuildHashRoundTrip(t *testing.T) {
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if got := ReadTrustedBuildHash("draft/demo"); got != "" {
		t.Fatalf("a fresh app has trusted hash %q", got)
	}
	if err := WriteTrustedBuildHash("draft/demo", "abc123"); err != nil {
		t.Fatal(err)
	}
	if got := ReadTrustedBuildHash("draft/demo"); got != "abc123" {
		t.Fatalf("read back %q", got)
	}
	if err := WriteTrustedBuildHash("draft/demo", "def456"); err != nil {
		t.Fatal(err)
	}
	if got := ReadTrustedBuildHash("draft/demo"); got != "def456" {
		t.Fatalf("overwrite read back %q", got)
	}
	path := filepath.Join(dataDir, "builder", "trusted-builds", "draft", "demo.hash")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("record not under the data dir: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("record mode %o, want 600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".trusted-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if entries, _ := os.ReadDir(appDir); len(entries) != 0 {
		t.Fatalf("something was written into the app folder: %v", entries)
	}
	if err := DeleteTrustedBuildHash("draft/demo"); err != nil {
		t.Fatal(err)
	}
	if got := ReadTrustedBuildHash("draft/demo"); got != "" {
		t.Fatalf("hash survived delete: %q", got)
	}
	if err := DeleteTrustedBuildHash("draft/demo"); err != nil {
		t.Fatalf("deleting a missing record: %v", err)
	}
}

func TestTrustedBuildHashNeedsDataDir(t *testing.T) {
	setupAppStoreTest(t)
	unsetDataDir(t)
	if err := WriteTrustedBuildHash("draft/demo", "abc"); err == nil {
		t.Fatal("write succeeded with no data dir")
	}
	if got := ReadTrustedBuildHash("draft/demo"); got != "" {
		t.Fatalf("read %q with no data dir", got)
	}
}

func TestTrustedBuildHashRejectsBadAppId(t *testing.T) {
	setupAppStoreTest(t)
	setDataDir(t)
	if err := WriteTrustedBuildHash("../escape", "abc"); err == nil {
		t.Fatal("a malformed app id was accepted")
	}
}

func TestDeleteAppClearsTrustedBuildHash(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "draft", "demo")
	if err := WriteTrustedBuildHash("draft/demo", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteApp("draft/demo"); err != nil {
		t.Fatal(err)
	}
	if got := ReadTrustedBuildHash("draft/demo"); got != "" {
		t.Fatalf("a deleted app left trust %q behind", got)
	}
}

func TestSeedingAFreshAppClearsStaleTrust(t *testing.T) {
	setupAppStoreTest(t)
	setDataDir(t)
	if err := WriteTrustedBuildHash("draft/fresh", "stale"); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/fresh"); err != nil {
		t.Fatal(err)
	}
	if got := ReadTrustedBuildHash("draft/fresh"); got != "" {
		t.Fatalf("a new app inherited trust %q", got)
	}
}
