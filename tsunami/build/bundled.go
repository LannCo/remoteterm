// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// Both live directly under the app's resources directory (see electron-builder.config.cjs).
	BundledToolchainDirName = "gotoolchain"
	BundledModCacheDirName  = "gomodcache"

	defaultGoProxy = "https://proxy.golang.org,direct"
)

var bundleState struct {
	lock    sync.Mutex
	dir     string
	resolve func() string
}

// SetBundledDir points the builder at the directory holding the packaged Go toolchain
// and module cache; "" (the default, and what a development checkout has) means there
// is none.
func SetBundledDir(dir string) {
	bundleState.lock.Lock()
	defer bundleState.lock.Unlock()
	bundleState.dir = dir
	bundleState.resolve = nil
}

// SetBundledDirResolver is SetBundledDir for a directory that is only known after start-up
// (the server learns its resources path from the environment); resolve runs on every use.
func SetBundledDirResolver(resolve func() string) {
	bundleState.lock.Lock()
	defer bundleState.lock.Unlock()
	bundleState.resolve = resolve
}

func bundledDir() string {
	bundleState.lock.Lock()
	resolve, dir := bundleState.resolve, bundleState.dir
	bundleState.lock.Unlock()
	if resolve != nil {
		return resolve()
	}
	return dir
}

func bundledGoRoot() string {
	dir := bundledDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, BundledToolchainDirName)
}

func bundledGoPath() string {
	root := bundledGoRoot()
	if root == "" {
		return ""
	}
	goPath := filepath.Join(root, "bin", goExeName())
	if !isExecutableFile(goPath) {
		return ""
	}
	return goPath
}

// A file:// URL is a proxy in the GOPROXY list, so the cache is read where it lies and
// copied into the user's own module cache; the bundle itself may be read-only. Commas
// and pipes would split the list, so they are escaped.
func bundledProxyURL() string {
	dir := bundledDir()
	if dir == "" {
		return ""
	}
	cache := filepath.Join(dir, BundledModCacheDirName)
	if info, err := os.Stat(cache); err != nil || !info.IsDir() {
		return ""
	}
	slashed := filepath.ToSlash(cache)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	proxyURL := (&url.URL{Scheme: "file", Path: slashed}).String()
	return strings.ReplaceAll(proxyURL, ",", "%2C")
}

func isWithin(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// goBuildEnv is the environment for go mod tidy and go build: the allowlist, the local
// toolchain pin, an explicit GOROOT when goPath is the bundled toolchain (so nothing
// depends on how the app was launched), and the bundled module cache ahead of the
// user's proxy list so a first build needs no network for the SDK's dependencies.
func goBuildEnv(goPath string) []string {
	extra := []string{"GOTOOLCHAIN=local"}
	if root := bundledGoRoot(); root != "" && isWithin(goPath, root) {
		extra = append(extra, "GOROOT="+root)
	}
	if cacheURL := bundledProxyURL(); cacheURL != "" {
		next := os.Getenv("GOPROXY")
		if next == "" {
			next = defaultGoProxy
		}
		extra = append(extra, "GOPROXY="+cacheURL+","+next)
	}
	return AllowlistedEnv(os.Environ(), extra...)
}
