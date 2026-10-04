// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/tsunami/build"
)

const (
	DefaultTsunamiSdkVersion = "v0.12.4"
	TsunamiSdkDirName        = "tsunamisdk"
)

func GetTsunamiScaffoldPath() string {
	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	scaffoldPath := settings.TsunamiScaffoldPath
	if scaffoldPath == "" {
		scaffoldPath = filepath.Join(remotetermbase.GetWaveAppResourcesPath(), "tsunamiscaffold")
	}
	return scaffoldPath
}

func GetTsunamiSdkPath() string {
	return filepath.Join(remotetermbase.GetWaveAppResourcesPath(), TsunamiSdkDirName)
}

func ResolveGoFmtPath() (string, error) {
	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	goPath := settings.TsunamiGoPath

	if goPath == "" {
		gofmtPath := build.GetCachedGoFmtPath()
		if gofmtPath == "" {
			return "", fmt.Errorf("go toolchain has not been located yet (the first build locates it)")
		}
		return gofmtPath, nil
	}

	goDir := filepath.Dir(goPath)
	gofmtName := "gofmt"
	if runtime.GOOS == "windows" {
		gofmtName = "gofmt.exe"
	}
	gofmtPath := filepath.Join(goDir, gofmtName)

	info, err := os.Stat(gofmtPath)
	if err != nil {
		return "", fmt.Errorf("gofmt not found at %s: %w", gofmtPath, err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("gofmt path is a directory: %s", gofmtPath)
	}

	if info.Mode()&0111 == 0 {
		return "", fmt.Errorf("gofmt is not executable: %s", gofmtPath)
	}

	return gofmtPath, nil
}

func FormatGoCode(contents []byte) []byte {
	gofmtPath, err := ResolveGoFmtPath()
	if err != nil {
		return contents
	}

	cmd := exec.Command(gofmtPath)
	cmd.Stdin = bytes.NewReader(contents)
	formattedOutput, err := cmd.Output()
	if err != nil {
		return contents
	}

	return formattedOutput
}
