// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package utilfn

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGoMod(t *testing.T, dir string, modulePath string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.25\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestChdirToModuleRootSkipsNestedModules(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeGoMod(t, root, "example.com/outer")
	nested := filepath.Join(root, "sub", "inner")
	writeGoMod(t, nested, "example.com/inner")
	start := filepath.Join(nested, "deeper")
	if err := os.MkdirAll(start, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(start)

	if err := ChdirToModuleRoot("example.com/outer"); err != nil {
		t.Fatalf("ChdirToModuleRoot: %v", err)
	}
	got, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("cwd = %s, want %s", got, root)
	}
}

func TestChdirToModuleRootNotFound(t *testing.T) {
	start := t.TempDir()
	t.Chdir(start)
	if err := ChdirToModuleRoot("example.com/does-not-exist"); err == nil {
		t.Fatal("expected error when module root is absent")
	}
}
