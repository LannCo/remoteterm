// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

const TsunamiSdkModulePath = "github.com/LannCo/remoteterm/tsunami"

type goModParams struct {
	ModulePath     string
	GoVersion      string
	MinGoVersion   string
	SdkVersion     string
	SdkReplacePath string
}

// The replace target is rewritten on every build because packaged resource paths
// (AppImage mounts in particular) change between launches; AddReplace updates an
// existing replace in place, so a stale one never survives.
func makeGoModContent(existing []byte, params goModParams) ([]byte, error) {
	var modFile *modfile.File
	if existing != nil {
		parsed, err := modfile.Parse("go.mod", existing, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to parse existing go.mod: %w", err)
		}
		modFile = parsed
		if params.MinGoVersion != "" && (modFile.Go == nil || CompareGoVersions(modFile.Go.Version, params.MinGoVersion) < 0) {
			if err := modFile.AddGoStmt(params.MinGoVersion); err != nil {
				return nil, fmt.Errorf("failed to raise go version: %w", err)
			}
		}
	} else {
		modFile = &modfile.File{}
		if err := modFile.AddModuleStmt(params.ModulePath); err != nil {
			return nil, fmt.Errorf("failed to add module statement: %w", err)
		}
		if err := modFile.AddGoStmt(params.GoVersion); err != nil {
			return nil, fmt.Errorf("failed to add go version: %w", err)
		}
		if err := modFile.AddRequire(TsunamiSdkModulePath, params.SdkVersion); err != nil {
			return nil, fmt.Errorf("failed to add require directive: %w", err)
		}
	}
	if params.SdkReplacePath != "" {
		if err := modFile.AddReplace(TsunamiSdkModulePath, "", params.SdkReplacePath, ""); err != nil {
			return nil, fmt.Errorf("failed to add replace directive: %w", err)
		}
	}
	modFile.Cleanup()
	return modFile.Format()
}

// A first build has no go.sum, so go mod tidy would ask the checksum database (network)
// about every module. The SDK ships the sums its own go.mod was verified with; seeding
// them keeps verification while letting the first build run from the bundled module
// cache. Existing lines win and nothing is invented: a module the SDK does not list is
// still checked against sum.golang.org.
func seedGoSum(tempDir string, sdkDir string) error {
	if sdkDir == "" {
		return nil
	}
	seed, err := os.ReadFile(filepath.Join(sdkDir, "go.sum"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read the SDK go.sum: %w", err)
	}
	sumPath := filepath.Join(tempDir, "go.sum")
	existing, err := os.ReadFile(sumPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read go.sum: %w", err)
	}
	merged := mergeGoSum(existing, seed)
	if merged == nil {
		return nil
	}
	if err := os.WriteFile(sumPath, merged, 0644); err != nil {
		return fmt.Errorf("failed to write go.sum: %w", err)
	}
	return nil
}

func mergeGoSum(existing []byte, seed []byte) []byte {
	seen := make(map[string]bool)
	var out []string
	for _, source := range [][]byte{existing, seed} {
		for _, line := range strings.Split(string(source), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || seen[line] {
				continue
			}
			seen[line] = true
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return []byte(strings.Join(out, "\n") + "\n")
}
