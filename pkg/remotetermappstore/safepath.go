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
	for i, component := range components {
		info, err := os.Lstat(component)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot inspect %s: %w", component, err)
		}
		// Go reports Windows junctions as ModeIrregular, not ModeSymlink.
		if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return fmt.Errorf("refusing to use %s: it is a symbolic link or junction", component)
		}
		if i < len(components)-1 && !info.IsDir() {
			return fmt.Errorf("refusing to use %s: %s is not a directory", target, component)
		}
	}
	return nil
}

// openAppRoot confines later operations to appDir. The Lstat/SameFile comparison closes the
// window between CheckNoSymlinks and OpenRoot, in which appDir could have been swapped for a
// symlink that OpenRoot would then follow.
func openAppRoot(appDir string) (*os.Root, error) {
	if err := CheckNoSymlinks(appDir); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(appDir)
	if err != nil {
		return nil, err
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	pathInfo, err := os.Lstat(appDir)
	if err != nil || !os.SameFile(rootInfo, pathInfo) {
		root.Close()
		return nil, fmt.Errorf("refusing to use %s: it changed while being opened", appDir)
	}
	return root, nil
}

// The file is opened through the root, so a symlink or FIFO swapped in after the checks cannot
// redirect or stall the read: the post-open Stat must describe a regular file and the same file
// that the earlier Lstat saw.
func readRegularFileInRoot(root *os.Root, rel string, maxSize int64) ([]byte, int64, error) {
	before, err := root.Lstat(rel)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat file: %w", err)
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|nonBlockFlag, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat file: %w", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(before, info) {
		return nil, 0, fmt.Errorf("%s is not a regular file", filepath.Base(rel))
	}
	if info.Size() > maxSize {
		return nil, 0, fmt.Errorf("%s is larger than %d bytes", filepath.Base(rel), maxSize)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read file: %w", err)
	}
	if int64(len(data)) > maxSize {
		return nil, 0, fmt.Errorf("%s is larger than %d bytes", filepath.Base(rel), maxSize)
	}
	return data, info.ModTime().UnixMilli(), nil
}

// Streams a regular file between roots with no size cap: the regular-file and SameFile checks
// already keep /dev/zero-style sources out, and apps may hold large static assets.
func copyRegularFileBetweenRoots(srcRoot *os.Root, dstRoot *os.Root, rel string) error {
	before, err := srcRoot.Lstat(rel)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	src, err := srcRoot.OpenFile(rel, os.O_RDONLY|nonBlockFlag, 0)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(before, info) {
		return fmt.Errorf("%s is not a regular file", filepath.Base(rel))
	}
	perm := info.Mode().Perm()
	dst, err := dstRoot.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("failed to copy %s: %w", filepath.Base(rel), err)
	}
	// The create mode is filtered by the umask; chmod restores the source's bits.
	if err := dst.Chmod(perm); err != nil {
		dst.Close()
		return fmt.Errorf("failed to set mode on %s: %w", filepath.Base(rel), err)
	}
	return dst.Close()
}

func createFileExclusiveInRoot(root *os.Root, rel string, contents []byte) error {
	f, err := root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return fmt.Errorf("failed to write %s: %w", filepath.Base(rel), err)
	}
	return f.Close()
}

// Overwrites open without O_TRUNC and truncate only after the post-open Stat confirms the
// target is the regular file the Lstat saw; O_TRUNC would destroy whatever was swapped in.
func writeFileInRoot(root *os.Root, rel string, contents []byte) error {
	before, err := root.Lstat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return createFileExclusiveInRoot(root, rel, contents)
	}
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filepath.Base(rel), err)
	}
	f, err := root.OpenFile(rel, os.O_WRONLY|nonBlockFlag, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filepath.Base(rel), err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("failed to stat %s: %w", filepath.Base(rel), err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(before, info) {
		f.Close()
		return fmt.Errorf("refusing to overwrite %s: not a regular file", filepath.Base(rel))
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return fmt.Errorf("failed to truncate %s: %w", filepath.Base(rel), err)
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return fmt.Errorf("failed to write %s: %w", filepath.Base(rel), err)
	}
	return f.Close()
}

func readRegularFileCapped(path string, maxSize int64) ([]byte, int64, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open %s: %w", filepath.Dir(path), err)
	}
	defer root.Close()
	return readRegularFileInRoot(root, filepath.Base(path), maxSize)
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
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filepath.Dir(path), err)
	}
	defer root.Close()
	return writeFileInRoot(root, filepath.Base(path), contents)
}
