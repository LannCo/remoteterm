// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

const MaxAppFileReadSize = 2 * 1024 * 1024

func GetWaveAppsRoot() string {
	return filepath.Join(remotetermbase.GetHomeDir(), "waveapps")
}

// A process that can write the app folder (an agent, an editor plugin) can plant
// symlinks; refusing every symlinked component keeps our reads and writes inside it.
// The check starts at ~/waveapps/<ns>: ~/waveapps itself is the user's choice (it may
// live on another disk) and is out of reach of anything confined to an app folder.
func CheckNoSymlinks(target string) error {
	root := GetWaveAppsRoot()
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path %s is outside %s", target, root)
	}
	var components []string
	if rel != "." {
		cur := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			cur = filepath.Join(cur, part)
			components = append(components, cur)
		}
	}
	for _, component := range components {
		info, err := os.Lstat(component)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot inspect %s: %w", component, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing to use %s: it is a symbolic link", component)
		}
	}
	return nil
}

func readRegularFileCapped(path string, maxSize int64) ([]byte, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	if info.Size() > maxSize {
		return nil, 0, fmt.Errorf("%s is larger than %d bytes", filepath.Base(path), maxSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read file: %w", err)
	}
	if int64(len(data)) > maxSize {
		return nil, 0, fmt.Errorf("%s is larger than %d bytes", filepath.Base(path), maxSize)
	}
	return data, info.ModTime().UnixMilli(), nil
}

func createFileExclusive(path string, contents []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return fmt.Errorf("failed to write %s: %w", filepath.Base(path), err)
	}
	return f.Close()
}

func writeAppFileSafe(path string, contents []byte) error {
	if err := CheckNoSymlinks(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return createFileExclusive(path, contents)
	}
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to overwrite %s: not a regular file", filepath.Base(path))
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filepath.Base(path), err)
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return fmt.Errorf("failed to write %s: %w", filepath.Base(path), err)
	}
	return f.Close()
}
