// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/tsunami/build"
)

const ScaffoldCheckFileName = "app-main.go.tmpl"

type TsunamiBuildEnv struct {
	ScaffoldPath   string
	SdkReplacePath string
	MinGoVersion   string
	GoPath         string
}

func ResolveTsunamiSdkPath(settingPath string) (string, error) {
	sdkPath := settingPath
	if sdkPath == "" {
		sdkPath = GetTsunamiSdkPath()
	}
	if _, err := os.Stat(filepath.Join(sdkPath, "go.mod")); err != nil {
		return "", fmt.Errorf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, sdkPath)
	}
	return sdkPath, nil
}

// Each missing prerequisite gets its own message naming the fix; checked here, before
// the build starts, because the build itself reports them as generic failures.
func PrepareTsunamiBuild(settings rtconfig.SettingsType) (*TsunamiBuildEnv, error) {
	return prepareTsunamiBuild(settings, build.CheckGoVersion)
}

func prepareTsunamiBuild(settings rtconfig.SettingsType, checkGo func(customGoPath string, minGoVersion string) build.GoVersionCheckResult) (*TsunamiBuildEnv, error) {
	sdkPath, err := ResolveTsunamiSdkPath(settings.TsunamiSdkReplacePath)
	if err != nil {
		return nil, err
	}
	minGoVersion, err := build.ReadSdkGoVersion(sdkPath)
	if err != nil {
		return nil, fmt.Errorf("Tsunami SDK at %s has an unusable go.mod: %w", sdkPath, err)
	}
	scaffoldPath := scaffoldPathFromSettings(settings)
	if _, err := os.Stat(filepath.Join(scaffoldPath, ScaffoldCheckFileName)); err != nil {
		return nil, fmt.Errorf(`Tsunami scaffold not found at %s. Rebuild with "task build:tsunamiscaffold", or set "tsunami:scaffoldpath" in Settings.`, scaffoldPath)
	}
	result := checkGo(settings.TsunamiGoPath, minGoVersion)
	switch result.GoStatus {
	case build.GoStatus_NotFound:
		return nil, fmt.Errorf(`Go toolchain not found. Install Go %s or newer, or set "tsunami:gopath" in Settings to the full path of the go binary.`, minGoVersion)
	case build.GoStatus_BadVersion:
		return nil, fmt.Errorf(`Go %s is older than %s, which the Tsunami SDK requires. Install a newer Go, or set "tsunami:gopath" to one.`, result.Version, minGoVersion)
	case build.GoStatus_Error:
		return nil, fmt.Errorf("%s", result.ErrorString)
	}
	return &TsunamiBuildEnv{
		ScaffoldPath:   scaffoldPath,
		SdkReplacePath: sdkPath,
		MinGoVersion:   minGoVersion,
		GoPath:         result.GoPath,
	}, nil
}
