// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func listBundleFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func TestCopySdkBundleCopiesOnlyRuntimePackages(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures need a Unix filesystem")
	}
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "go.mod"), "module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n")
	writeTestFile(t, filepath.Join(src, "go.sum"), "")
	writeTestFile(t, filepath.Join(src, "app", "app.go"), "package app\n")
	writeTestFile(t, filepath.Join(src, "app", "app_test.go"), "package app\n")
	writeTestFile(t, filepath.Join(src, "app", "sub", "sub.go"), "package sub\n")
	writeTestFile(t, filepath.Join(src, "engine", "engine.go"), "package engine\n")
	writeTestFile(t, filepath.Join(src, "engine", "render.md"), "notes\n")
	for _, dir := range []string{"vdom", "rpctypes", "util", "tsunamibase", "ui"} {
		writeTestFile(t, filepath.Join(src, dir, dir+".go"), "package "+dir+"\n")
	}
	for _, dir := range []string{"build", "cmd", "demo/todo", "frontend", "templates"} {
		writeTestFile(t, filepath.Join(src, dir, "x.go"), "package x\n")
	}
	if err := os.Symlink(filepath.Join(src, "app", "app.go"), filepath.Join(src, "app", "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "build"), filepath.Join(src, "app", "linkdir")); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "tsunamisdk")
	if err := CopySdkBundle(src, dst); err != nil {
		t.Fatalf("CopySdkBundle: %v", err)
	}

	want := []string{
		"app/app.go",
		"app/sub/sub.go",
		"engine/engine.go",
		"go.mod",
		"go.sum",
		"rpctypes/rpctypes.go",
		"tsunamibase/tsunamibase.go",
		"ui/ui.go",
		"util/util.go",
		"vdom/vdom.go",
	}
	got := listBundleFiles(t, dst)
	if !slices.Equal(got, want) {
		t.Fatalf("bundle files:\n got %v\nwant %v", got, want)
	}
}

func TestCopySdkBundleRequiresGoMod(t *testing.T) {
	src := t.TempDir()
	for _, dir := range SdkBundlePackageDirs {
		writeTestFile(t, filepath.Join(src, dir, dir+".go"), "package "+dir+"\n")
	}
	err := CopySdkBundle(src, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Fatalf("expected an error naming go.mod, got %v", err)
	}
}

func TestCopySdkBundleFromRepoSdk(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "tsunamisdk")
	if err := CopySdkBundle("..", dst); err != nil {
		t.Fatalf("CopySdkBundle(..): %v", err)
	}
	for _, rel := range []string{"go.mod", "go.sum", "app/defaultclient.go", "vdom/vdom.go", "engine/clientimpl.go"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Errorf("expected %s in bundle: %v", rel, err)
		}
	}
	for _, rel := range []string{"build", "cmd", "demo", "frontend", "templates"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err == nil {
			t.Errorf("%s must not be in the bundle", rel)
		}
	}
	for _, rel := range listBundleFiles(t, dst) {
		if strings.HasSuffix(rel, "_test.go") {
			t.Errorf("test file %s must not be in the bundle", rel)
		}
	}
}
