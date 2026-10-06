// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"fmt"
	"os"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
)

// The folder always comes from the builder's rtinfo, never from the renderer, so a
// compromised page cannot point a terminal or a file manager somewhere else.
func ResolveBuilderAppDir(builderId string) (string, error) {
	appId, _, err := GetBuilderRebuildInputs(builderId)
	if err != nil {
		return "", err
	}
	return ResolveAppDirForAppId(appId)
}

// Builder terminals resolve their folder from the app id stored on their tab, never from rtinfo,
// which any pane can rewrite through SetRTInfoCommand.
func ResolveAppDirForAppId(appId string) (string, error) {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return "", err
	}
	if err := remotetermappstore.CheckNoSymlinks(appDir); err != nil {
		return "", err
	}
	info, err := os.Lstat(appDir)
	if err != nil {
		return "", fmt.Errorf("app folder %s is not available: %w", appDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("app folder %s is not a directory", appDir)
	}
	return appDir, nil
}
