// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	goProbeWaitDelay      = 1 * time.Second
	goProbeMaxOutput      = 64 * 1024
	goProbeScript         = "command -v go"
	goDiscoveryFailureTTL = 30 * time.Second
	goEnvTimeout          = 2 * time.Second
)

var goProbeTimeout = 3 * time.Second

var goSearchPaths = defaultGoSearchPaths

type goDiscoveryCache struct {
	findLock  sync.Mutex
	lock      sync.Mutex
	floor     string
	goPath    string
	gofmtPath string
	failErr   error
	failedAt  time.Time
}

// GoTooOldError is returned only when every Go found is older than the floor; it names
// the newest one so the message can say what was found.
type GoTooOldError struct {
	GoPath     string
	Version    string
	MinVersion string
}

func (e *GoTooOldError) Error() string {
	return fmt.Sprintf("the newest Go found (%s at %s) is older than %s", e.Version, e.GoPath, e.MinVersion)
}

var goCache = &goDiscoveryCache{}

type cappedWriter struct {
	buf bytes.Buffer
	max int
}

// Writes past the cap are reported as accepted so a chatty rc file sees no EPIPE.
func (w *cappedWriter) Write(p []byte) (int, error) {
	remain := w.max - w.buf.Len()
	if remain > 0 {
		if len(p) > remain {
			w.buf.Write(p[:remain])
		} else {
			w.buf.Write(p)
		}
	}
	return len(p), nil
}

// FindGoExecutable holds findLock for the whole discovery so concurrent callers share
// one login-shell probe instead of each starting their own. Results are cached per
// floor; in practice the floor is the SDK's go line and never changes.
func FindGoExecutable(minGoVersion string) (string, error) {
	goCache.findLock.Lock()
	defer goCache.findLock.Unlock()
	if hit, goPath, failErr := goCache.lookup(minGoVersion); hit {
		return goPath, failErr
	}
	found, err := discoverGo(minGoVersion)
	if err != nil {
		goCache.setFailure(minGoVersion, err)
		return "", err
	}
	goPath, gofmtPath := canonicalizeGoPath(found)
	goCache.setSuccess(minGoVersion, goPath, gofmtPath)
	return goPath, nil
}

// GetCachedGoFmtPath never discovers; Code-tab saves call it and must not wait on a probe.
func GetCachedGoFmtPath() string {
	goCache.lock.Lock()
	defer goCache.lock.Unlock()
	return goCache.gofmtPath
}

func (c *goDiscoveryCache) lookup(floor string) (bool, string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.floor != floor {
		return false, "", nil
	}
	if c.goPath != "" {
		return true, c.goPath, nil
	}
	if c.failErr != nil && time.Since(c.failedAt) < goDiscoveryFailureTTL {
		return true, "", c.failErr
	}
	return false, "", nil
}

func (c *goDiscoveryCache) setFailure(floor string, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.floor = floor
	c.goPath = ""
	c.failErr = err
	c.failedAt = time.Now()
}

func (c *goDiscoveryCache) setSuccess(floor string, goPath string, gofmtPath string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.floor = floor
	c.goPath = goPath
	c.gofmtPath = gofmtPath
	c.failErr = nil
}

func resetGoDiscoveryCache() {
	goCache.lock.Lock()
	defer goCache.lock.Unlock()
	goCache.floor = ""
	goCache.goPath = ""
	goCache.gofmtPath = ""
	goCache.failErr = nil
	goCache.failedAt = time.Time{}
}

func goExeName() string {
	if runtime.GOOS == "windows" {
		return "go.exe"
	}
	return "go"
}

func gofmtExeName() string {
	if runtime.GOOS == "windows" {
		return "gofmt.exe"
	}
	return "gofmt"
}

// Candidates are tried in order and each must meet the floor; a too-old Go early in
// PATH must not hide a newer one installed elsewhere.
func discoverGo(minGoVersion string) (string, error) {
	var newestTooOld *GoTooOldError
	accept := func(candidate string) bool {
		goVer, err := readGoVersion(candidate)
		if err != nil {
			return false
		}
		if minGoVersion == "" || CompareGoVersions(goVer, minGoVersion) >= 0 {
			return true
		}
		if newestTooOld == nil || CompareGoVersions(goVer, newestTooOld.Version) > 0 {
			newestTooOld = &GoTooOldError{GoPath: candidate, Version: goVer, MinVersion: minGoVersion}
		}
		return false
	}
	if goPath, err := exec.LookPath(goExeName()); err == nil {
		if absPath, err := filepath.Abs(goPath); err == nil && accept(absPath) {
			return absPath, nil
		}
	}
	home, _ := os.UserHomeDir()
	for _, candidate := range goSearchPaths(home) {
		if isExecutableFile(candidate) && accept(candidate) {
			return candidate, nil
		}
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		goPath, err := probeLoginShellForGo(os.Getenv("SHELL"))
		if err == nil && accept(goPath) {
			return goPath, nil
		}
		if err != nil {
			log.Printf("go discovery: %v", err)
		}
	}
	if newestTooOld != nil {
		return "", newestTooOld
	}
	return "", fmt.Errorf("go command not found in PATH, common installation locations, or the login shell")
}

func readGoVersion(goPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), goVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, goPath, "version")
	cmd.Env = goCmdEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	goVer, ok := ParseGoVersionOutput(string(out))
	if !ok {
		return "", errors.New("unparseable go version output")
	}
	return goVer, nil
}

func defaultGoSearchPaths(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			`c:\go\bin\go.exe`,
			`c:\program files\go\bin\go.exe`,
		}
	}
	paths := []string{
		"/opt/homebrew/bin/go",
		"/usr/local/bin/go",
		"/usr/local/go/bin/go",
		"/usr/bin/go",
	}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".local", "go", "bin", "go"))
	}
	paths = append(paths, "/usr/lib/go/bin/go")
	if home != "" {
		if sdkGo := newestSdkGo(home); sdkGo != "" {
			paths = append(paths, sdkGo)
		}
	}
	paths = append(paths, "/snap/bin/go")
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".local", "share", "mise", "shims", "go"),
			filepath.Join(home, ".asdf", "shims", "go"),
		)
	}
	return paths
}

func newestSdkGo(home string) string {
	matches, err := filepath.Glob(filepath.Join(home, "sdk", "go*", "bin", goExeName()))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sdkVersion := func(goPath string) string {
		return filepath.Base(filepath.Dir(filepath.Dir(goPath)))
	}
	sort.Slice(matches, func(i, j int) bool {
		return CompareGoVersions(sdkVersion(matches[i]), sdkVersion(matches[j])) > 0
	})
	return matches[0]
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func loginShellArgs(shellPath string) ([]string, bool) {
	switch filepath.Base(shellPath) {
	case "bash", "zsh", "fish":
		return []string{"-l", "-i", "-c", goProbeScript}, true
	case "sh", "dash", "ksh":
		return []string{"-l", "-c", goProbeScript}, true
	}
	return nil, false
}

// The probe is the last resort because it runs the user's rc files; every bound
// here (timeout, group kill, output cap) exists because rc files can hang or spam.
func probeLoginShellForGo(shellPath string) (string, error) {
	if shellPath == "" {
		return "", fmt.Errorf("SHELL is not set")
	}
	if !filepath.IsAbs(shellPath) {
		return "", fmt.Errorf("SHELL %q is not an absolute path", shellPath)
	}
	args, ok := loginShellArgs(shellPath)
	if !ok {
		return "", fmt.Errorf("shell %q is not on the probe allowlist", filepath.Base(shellPath))
	}
	if !isExecutableFile(shellPath) {
		return "", fmt.Errorf("shell %q is not an executable file", shellPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), goProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shellPath, args...)
	output := &cappedWriter{max: goProbeMaxOutput}
	cmd.Stdout = output
	cmd.WaitDelay = goProbeWaitDelay
	setProbeProcessGroup(cmd)
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("login shell probe timed out after %v", goProbeTimeout)
	}
	if runErr != nil {
		return "", fmt.Errorf("login shell probe failed: %w", runErr)
	}
	line := lastNonEmptyLine(output.buf.String())
	if !filepath.IsAbs(line) {
		return "", fmt.Errorf("login shell probe returned %q, not an absolute path", line)
	}
	if !isExecutableFile(line) {
		return "", fmt.Errorf("login shell probe returned %q, which is not an executable file", line)
	}
	return line, nil
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// Shims (mise, asdf) and /snap/bin wrappers are not the toolchain; asking the found
// binary for its GOROOT gives the real go and a gofmt that sits beside it.
func canonicalizeGoPath(found string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), goEnvTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, found, "env", "GOROOT")
	cmd.Env = goCmdEnv()
	out, err := cmd.Output()
	if err == nil {
		goroot := strings.TrimSpace(string(out))
		if filepath.IsAbs(goroot) {
			rootGo := filepath.Join(goroot, "bin", goExeName())
			if isExecutableFile(rootGo) {
				rootFmt := filepath.Join(goroot, "bin", gofmtExeName())
				if !isExecutableFile(rootFmt) {
					rootFmt = ""
				}
				return rootGo, rootFmt
			}
		}
	}
	fmtPath := filepath.Join(filepath.Dir(found), gofmtExeName())
	if !isExecutableFile(fmtPath) {
		fmtPath = ""
	}
	return found, fmtPath
}
