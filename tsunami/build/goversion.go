// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"
	"go/version"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/modfile"
)

var goVersionOutputRe = regexp.MustCompile(`go(1\.\d+(?:\.\d+)?(?:(?:rc|beta)\d+)?)`)

func ReadSdkGoVersion(sdkDir string) (string, error) {
	goModPath := filepath.Join(sdkDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", goModPath, err)
	}
	modFile, err := modfile.ParseLax(goModPath, data, nil)
	if err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", goModPath, err)
	}
	if modFile.Go == nil || modFile.Go.Version == "" {
		return "", fmt.Errorf("%s has no go directive", goModPath)
	}
	return modFile.Go.Version, nil
}

func ParseGoVersionOutput(out string) (string, bool) {
	matches := goVersionOutputRe.FindStringSubmatch(out)
	if len(matches) < 2 {
		return "", false
	}
	return matches[1], true
}

// go/version understands release candidates and toolchain suffixes, which a
// hand-rolled minor-number comparison got wrong.
func CompareGoVersions(a string, b string) int {
	return version.Compare(normalizeGoVersion(a), normalizeGoVersion(b))
}

func normalizeGoVersion(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "go") {
		return v
	}
	return "go" + v
}

// GOTOOLCHAIN=local makes an older Go fail with our version message instead of
// silently downloading a newer toolchain.
func goCmdEnv() []string {
	return AllowlistedEnv(os.Environ(), "GOTOOLCHAIN=local")
}
