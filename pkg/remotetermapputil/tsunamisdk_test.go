// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/tsunami/build"
)

func makeBuildFixture(t *testing.T) (string, string) {
	t.Helper()
	resources := t.TempDir()
	setResourcesPath(t, resources)
	sdk := filepath.Join(resources, "tsunamisdk")
	scaffold := filepath.Join(resources, "tsunamiscaffold")
	for dir, files := range map[string]map[string]string{
		sdk:      {"go.mod": "module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n"},
		scaffold: {"app-main.go.tmpl": "package main\n"},
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return sdk, scaffold
}

func fakeCheckGo(status string, version string) func(string, string) build.GoVersionCheckResult {
	return func(customGoPath string, minGoVersion string) build.GoVersionCheckResult {
		return build.GoVersionCheckResult{GoStatus: status, GoPath: "/fake/go", Version: version}
	}
}

func TestResolveTsunamiSdkPath(t *testing.T) {
	sdk, _ := makeBuildFixture(t)
	if got, err := ResolveTsunamiSdkPath(""); err != nil || got != sdk {
		t.Fatalf("bundle: got %q, %v", got, err)
	}
	custom := t.TempDir()
	if err := os.WriteFile(filepath.Join(custom, "go.mod"), []byte("module x\n\ngo 1.25.6\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveTsunamiSdkPath(custom); err != nil || got != custom {
		t.Fatalf("setting: got %q, %v", got, err)
	}
	if err := os.Remove(filepath.Join(sdk, "go.mod")); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, sdk)
	if _, err := ResolveTsunamiSdkPath(""); err == nil || err.Error() != want {
		t.Fatalf("missing bundle: got %v\nwant %s", err, want)
	}
}

func TestPrepareTsunamiBuildMessages(t *testing.T) {
	sdk, scaffold := makeBuildFixture(t)
	settings := rtconfig.SettingsType{}

	env, err := prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_Ok, "1.26.3"))
	if err != nil {
		t.Fatal(err)
	}
	if env.SdkReplacePath != sdk || env.ScaffoldPath != scaffold || env.MinGoVersion != "1.25.6" || env.GoPath != "/fake/go" {
		t.Fatalf("env = %+v", env)
	}

	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_NotFound, ""))
	wantNotFound := `Go toolchain not found. Install Go 1.25.6 or newer, or set "tsunami:gopath" in Settings to the full path of the go binary.`
	if err == nil || err.Error() != wantNotFound {
		t.Fatalf("not found: got %v", err)
	}

	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_BadVersion, "1.25.5"))
	wantOld := `Go 1.25.5 is older than 1.25.6, which the Tsunami SDK requires. Install a newer Go, or set "tsunami:gopath" to one.`
	if err == nil || err.Error() != wantOld {
		t.Fatalf("too old: got %v", err)
	}

	if err := os.Remove(filepath.Join(scaffold, "app-main.go.tmpl")); err != nil {
		t.Fatal(err)
	}
	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_Ok, "1.26.3"))
	wantScaffold := fmt.Sprintf(`Tsunami scaffold not found at %s. Rebuild with "task build:tsunamiscaffold", or set "tsunami:scaffoldpath" in Settings.`, scaffold)
	if err == nil || err.Error() != wantScaffold {
		t.Fatalf("scaffold: got %v", err)
	}

	if err := os.Remove(filepath.Join(sdk, "go.mod")); err != nil {
		t.Fatal(err)
	}
	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_NotFound, ""))
	if err == nil || err.Error() != fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, sdk) {
		t.Fatalf("SDK check must come first, got %v", err)
	}
}
