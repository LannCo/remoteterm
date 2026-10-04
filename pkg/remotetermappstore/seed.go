// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore/starter"
)

// O_EXCL means an existing file, or a symlink planted where a starter file would go,
// is left alone rather than written through. The one exception is an empty regular
// app.go (an editor or agent touched it), which counts as missing.
func SeedApp(appId string) ([]string, error) {
	if err := ValidateAppId(appId); err != nil {
		return nil, fmt.Errorf("invalid appId: %w", err)
	}
	appDir, err := GetAppDir(appId)
	if err != nil {
		return nil, err
	}
	if err := CheckNoSymlinks(appDir); err != nil {
		return nil, err
	}
	_, statErr := os.Lstat(appDir)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("cannot inspect %s: %w", appDir, statErr)
	}
	// Bindings are keyed by app id and outlive a deleted folder; a new app with an old
	// name must not inherit them. This runs before the folder exists: if it failed after
	// the mkdir, a retry would see an existing app and keep the stale bindings.
	if errors.Is(statErr, fs.ErrNotExist) {
		if err := deleteSecretBindings(appId); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create app directory: %w", err)
	}
	// MkdirAll follows a symlink created after the first check; openAppRoot checks again.
	root, err := openAppRoot(appDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files, err := starter.GetStarterFiles()
	if err != nil {
		return nil, err
	}
	written := make([]string, 0, len(files))
	for _, f := range files {
		err := createFileExclusiveInRoot(root, f.Name, f.Data)
		if errors.Is(err, fs.ErrExist) && f.Name == starter.AppGoFileName {
			filled, fillErr := fillEmptyFileInRoot(root, f.Name, f.Data)
			if fillErr != nil {
				return written, fmt.Errorf("failed to write %s: %w", f.Name, fillErr)
			}
			if filled {
				written = append(written, f.Name)
			}
			continue
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return written, fmt.Errorf("failed to write %s: %w", f.Name, err)
		}
		written = append(written, f.Name)
	}
	return written, nil
}

// Opens without O_TRUNC: the file is only written once the post-open Stat confirms it is the
// empty regular file the Lstat saw, so a symlink or FIFO swapped in meanwhile is left alone.
func fillEmptyFileInRoot(root *os.Root, rel string, contents []byte) (bool, error) {
	before, err := root.Lstat(rel)
	if err != nil {
		return false, err
	}
	if !before.Mode().IsRegular() || before.Size() != 0 {
		return false, nil
	}
	f, err := root.OpenFile(rel, os.O_WRONLY|nonBlockFlag, 0)
	if err != nil {
		return false, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return false, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(before, info) || info.Size() != 0 {
		f.Close()
		return false, nil
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}
