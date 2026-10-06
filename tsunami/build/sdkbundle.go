// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Only the packages an app build imports go into the bundle; build/, cmd/, demo/,
// frontend/ and templates/ belong to the toolchain side and stay out.
var SdkBundlePackageDirs = []string{"app", "engine", "vdom", "rpctypes", "util", "tsunamibase", "ui"}

// CopySdkBundle is the single definition of the bundle contents: the Taskfile, the
// packaged app and the starter compile test all go through it, so they cannot drift.
func CopySdkBundle(srcDir string, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create SDK bundle directory %s: %w", dstDir, err)
	}
	if err := copyBundleFile(filepath.Join(srcDir, "go.mod"), filepath.Join(dstDir, "go.mod"), true); err != nil {
		return err
	}
	if err := copyBundleFile(filepath.Join(srcDir, "go.sum"), filepath.Join(dstDir, "go.sum"), false); err != nil {
		return err
	}
	for _, pkgDir := range SdkBundlePackageDirs {
		if err := copyBundlePackageDir(srcDir, dstDir, pkgDir); err != nil {
			return err
		}
	}
	return nil
}

func copyBundlePackageDir(srcDir string, dstDir string, pkgDir string) error {
	root := filepath.Join(srcDir, pkgDir)
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("SDK package directory %s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("SDK package path %s is not a directory", root)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		return copyBundleFile(path, filepath.Join(dstDir, rel), true)
	})
}

func copyBundleFile(srcPath string, dstPath string, required bool) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		if !required && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", dstPath, err)
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", dstPath, err)
	}
	return nil
}
