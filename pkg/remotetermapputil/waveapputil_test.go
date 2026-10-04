// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func setResourcesPath(t *testing.T, path string) {
	t.Helper()
	orig := remotetermbase.AppResourcesPath_VarCache
	remotetermbase.AppResourcesPath_VarCache = path
	t.Cleanup(func() { remotetermbase.AppResourcesPath_VarCache = orig })
}

func TestGetTsunamiSdkPath(t *testing.T) {
	resources := t.TempDir()
	setResourcesPath(t, resources)
	want := filepath.Join(resources, "tsunamisdk")
	if got := GetTsunamiSdkPath(); got != want {
		t.Fatalf("GetTsunamiSdkPath() = %q, want %q", got, want)
	}
}
