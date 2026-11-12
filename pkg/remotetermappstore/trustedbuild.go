// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

const maxTrustedBuildHashSize = 256

// The record names the inputs of the last build the user started by hand. It lives beside the
// secret bindings, outside the app folder, so a process that can write the folder cannot mark
// its own edit as trusted.
func GetTrustedBuildPath(appId string) (string, error) {
	if err := ValidateAppId(appId); err != nil {
		return "", fmt.Errorf("invalid appId: %w", err)
	}
	dataDir := remotetermbase.GetWaveDataDir()
	if dataDir == "" {
		return "", fmt.Errorf("data directory is not set")
	}
	appNS, appName, _ := ParseAppId(appId)
	return filepath.Join(dataDir, "builder", "trusted-builds", appNS, appName+".hash"), nil
}

// An unreadable or missing record is the empty string: nothing is trusted.
func ReadTrustedBuildHash(appId string) string {
	path, err := GetTrustedBuildPath(appId)
	if err != nil {
		return ""
	}
	data, _, err := readRegularFileCapped(path, maxTrustedBuildHashSize)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func WriteTrustedBuildHash(appId string, hash string) error {
	path, err := GetTrustedBuildPath(appId)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create trusted build directory: %w", err)
	}
	// Written beside the target and renamed over it, so a reader never sees half a record.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".trusted-*")
	if err != nil {
		return fmt.Errorf("failed to write trusted build record: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(hash + "\n"); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write trusted build record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to write trusted build record: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to write trusted build record: %w", err)
	}
	return nil
}

func DeleteTrustedBuildHash(appId string) error {
	path, err := GetTrustedBuildPath(appId)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove trusted build record: %w", err)
	}
	return nil
}
