// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
)

const appStaticDir = "static"

var buildRootFiles = []string{"go.mod", "go.sum", "manifest.json"}

// The build copies these back into the app folder by path when it finishes.
var buildMovedBackFiles = []string{"go.mod", "go.sum", "manifest.json", "static/tw.css"}

// The Tsunami build reads and writes the app folder by path (os.DirFS, os.Create, go build
// -o): a FIFO stalls it for good and a symlink carries its reads and writes outside the
// folder. Checking everything it touches through the app root first turns both into a
// build error that names the path. An agent could still swap a file in after this check;
// it narrows the window to the build itself rather than closing it.
func checkAppBuildInputs(appDir string) error {
	root, err := remotetermappstore.OpenAppRoot(appDir)
	if err != nil {
		return fmt.Errorf("cannot use the app folder: %w", err)
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("cannot list the app folder: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") && !slices.Contains(buildRootFiles, name) {
			continue
		}
		if !entry.Type().IsRegular() {
			return refuseBuildPath(name, "is not a regular file")
		}
	}
	if err := checkStaticDir(root); err != nil {
		return err
	}
	if err := checkMovedBackFiles(root); err != nil {
		return err
	}
	return checkBinDir(root)
}

// The move-back opens these with os.Create, which writes through a hard link to the file's
// other names, wherever they are.
func checkMovedBackFiles(root *os.Root) error {
	for _, rel := range buildMovedBackFiles {
		info, err := root.Lstat(rel)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("cannot inspect %s: %w", rel, err)
		}
		if remotetermappstore.HasOtherHardLinks(info) {
			return fmt.Errorf("refusing to build: %s in the app folder has other hard links, and the build would write through them", rel)
		}
	}
	return nil
}

// fs.WalkDir follows a symlink at its starting point, so static itself is checked with
// Lstat before the walk; below it, entries are typed without being followed.
func checkStaticDir(root *os.Root) error {
	info, err := root.Lstat(appStaticDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", appStaticDir, err)
	}
	if !info.IsDir() {
		return refuseBuildPath(appStaticDir, "is not a directory")
	}
	return fs.WalkDir(root.FS(), appStaticDir, func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || d.Type().IsRegular() {
			return nil
		}
		return refuseBuildPath(rel, "is not a regular file or directory")
	})
}

// The build writes the binary to bin/app and then runs it, so a symlinked bin or bin/app
// would put an executable of the agent's choosing outside the folder.
func checkBinDir(root *os.Root) error {
	binary := "bin/app"
	if runtime.GOOS == "windows" {
		binary = "bin/app.exe"
	}
	for _, rel := range []string{"bin", binary} {
		info, err := root.Lstat(rel)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot inspect %s: %w", rel, err)
		}
		if rel == "bin" && !info.IsDir() {
			return refuseBuildPath(rel, "is not a directory")
		}
		if rel == binary && !info.Mode().IsRegular() {
			return refuseBuildPath(rel, "is not a regular file")
		}
	}
	return nil
}

func refuseBuildPath(rel string, problem string) error {
	return fmt.Errorf("refusing to build: %s in the app folder %s (symbolic links, pipes and devices are not allowed)", rel, problem)
}
