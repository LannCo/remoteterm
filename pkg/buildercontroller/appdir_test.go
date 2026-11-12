// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
)

func setBuilderAppId(t *testing.T, builderId string, appId string) {
	t.Helper()
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": appId})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
}

func TestResolveBuilderAppDir(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	setBuilderAppId(t, "test-appdir", "draft/demo")
	got, err := ResolveBuilderAppDir("test-appdir")
	if err != nil || got != appDir {
		t.Fatalf("ResolveBuilderAppDir = %q, %v; want %q", got, err, appDir)
	}

	if _, err := ResolveBuilderAppDir("test-appdir-unknown"); err == nil {
		t.Error("expected an error for a builder without rtinfo")
	}

	setBuilderAppId(t, "test-appdir-missing", "draft/nothere")
	if _, err := ResolveBuilderAppDir("test-appdir-missing"); err == nil {
		t.Error("expected an error for a missing app folder")
	}
}

func TestResolveBuilderAppDirRejectsFile(t *testing.T) {
	home, _ := setupBuilderTest(t)
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "waveapps", "draft", "afile"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	setBuilderAppId(t, "test-appdir-file", "draft/afile")
	if _, err := ResolveBuilderAppDir("test-appdir-file"); err == nil {
		t.Fatal("a regular file was accepted as an app folder")
	}
}

func TestResolveBuilderAppDirRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	home, _ := setupBuilderTest(t)
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, "waveapps", "draft", "linked")); err != nil {
		t.Fatal(err)
	}
	setBuilderAppId(t, "test-appdir-link", "draft/linked")
	if _, err := ResolveBuilderAppDir("test-appdir-link"); err == nil {
		t.Fatal("a symlinked app folder was accepted")
	}
}

func TestMakeBuilderTerminalBlockDef(t *testing.T) {
	def := MakeBuilderTerminalBlockDef("/home/u/waveapps/draft/demo")
	want := map[string]string{
		"view":       "term",
		"controller": "shell",
		"connection": "local",
		"cmd:cwd":    "/home/u/waveapps/draft/demo",
	}
	for key, value := range want {
		if def.Meta[key] != value {
			t.Errorf("meta[%q] = %v, want %q", key, def.Meta[key], value)
		}
	}
}

func TestResolveAppDirForAppIdIgnoresRtInfo(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	setBuilderAppId(t, "test-appdir-other", "draft/other")
	got, err := ResolveAppDirForAppId("draft/demo")
	if err != nil || got != appDir {
		t.Fatalf("ResolveAppDirForAppId = %q, %v; want %q", got, err, appDir)
	}
}

func TestResolveAppDirForAppIdRejectsBadIdsAndFolders(t *testing.T) {
	home, _ := setupBuilderTest(t)
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "waveapps", "draft", "afile"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, appId := range []string{"", "demo", "draft/../../etc", "Draft/demo", "draft/nothere", "draft/afile"} {
		if got, err := ResolveAppDirForAppId(appId); err == nil {
			t.Errorf("ResolveAppDirForAppId(%q) = %q, want an error", appId, got)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, "waveapps", "draft", "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveAppDirForAppId("draft/linked"); err == nil {
		t.Error("a symlinked app folder was accepted")
	}
}

func TestMakeBuilderTerminalBlockDefPinsDurableOff(t *testing.T) {
	def := MakeBuilderTerminalBlockDef("/home/u/waveapps/draft/demo")
	durable, ok := def.Meta[remotetermobj.MetaKey_TermDurable].(bool)
	if !ok || durable {
		t.Fatalf("meta[%q] = %#v, want false", remotetermobj.MetaKey_TermDurable, def.Meta[remotetermobj.MetaKey_TermDurable])
	}
}
