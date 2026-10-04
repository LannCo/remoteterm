// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const (
	BuildLogDir  = ".tsunami"
	BuildLogFile = BuildLogDir + "/build.log"
)

// OpenAppRoot is for callers outside this package that read an app folder directly (the
// build input hash): they get the same symlink and swap-race protection as the app file API.
func OpenAppRoot(appDir string) (*os.Root, error) {
	return openAppRoot(appDir)
}

// ReadAppRootFile reads a regular file through root with a size cap. A symlink, FIFO or other
// special file is refused without being opened for blocking I/O.
func ReadAppRootFile(root *os.Root, rel string, maxSize int64) ([]byte, error) {
	data, _, err := readRegularFileInRoot(root, rel, maxSize)
	return data, err
}

// An agent that can write the app folder could plant `.tsunami -> ~/.config` or a
// `build.log` symlink, so the directory and the file both go through the in-root helpers.
func WriteAppBuildLog(appId string, contents []byte) error {
	if err := ValidateAppId(appId); err != nil {
		return fmt.Errorf("invalid appId: %w", err)
	}
	appDir, err := GetAppDir(appId)
	if err != nil {
		return err
	}
	root, err := openAppRoot(appDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(BuildLogDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", BuildLogDir, err)
	}
	info, err := root.Lstat(BuildLogDir)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", BuildLogDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to use %s: not a plain directory", BuildLogDir)
	}
	// Unlinking first and creating with O_EXCL means a build.log hard-linked to a file
	// outside the app folder only loses its link here; the outside file is never written.
	if err := root.Remove(BuildLogFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to replace %s: %w", BuildLogFile, err)
	}
	return createFileExclusiveInRoot(root, BuildLogFile, contents)
}
