// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"

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
