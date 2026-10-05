// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateBundle(t *testing.T) {
	t.Helper()
	SetBundledDir("")
	t.Cleanup(func() { SetBundledDir("") })
}

func makeBundle(t *testing.T, goVersion string, withModCache bool) string {
	t.Helper()
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, BundledToolchainDirName, "bin", "go"), fakeGoScript(goVersion))
	if withModCache {
		if err := os.MkdirAll(filepath.Join(dir, BundledModCacheDirName), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBundledGoBeatsGoOnPath(t *testing.T) {
	isolateGoDiscovery(t)
	isolateBundle(t)
	pathDir := t.TempDir()
	writeExecutable(t, filepath.Join(pathDir, "go"), fakeGoScript("1.27.0"))
	t.Setenv("PATH", pathDir)
	bundle := makeBundle(t, "1.26.2", false)
	SetBundledDir(bundle)

	got, err := FindGoExecutable("1.25.6")
	want := filepath.Join(bundle, BundledToolchainDirName, "bin", "go")
	if err != nil || got != want {
		t.Fatalf("FindGoExecutable = %q, %v; want the bundled %q", got, err, want)
	}
}

func TestBundledGoBelowTheFloorYieldsToPath(t *testing.T) {
	isolateGoDiscovery(t)
	isolateBundle(t)
	pathDir := t.TempDir()
	pathGo := filepath.Join(pathDir, "go")
	writeExecutable(t, pathGo, fakeGoScript("1.27.0"))
	t.Setenv("PATH", pathDir)
	SetBundledDir(makeBundle(t, "1.26.2", false))

	got, err := FindGoExecutable("1.27.0")
	if err != nil || got != pathGo {
		t.Fatalf("FindGoExecutable = %q, %v; want %q", got, err, pathGo)
	}
}

func TestMissingOrUnsetBundleFallsBackToNormalDiscovery(t *testing.T) {
	isolateGoDiscovery(t)
	isolateBundle(t)
	pathDir := t.TempDir()
	pathGo := filepath.Join(pathDir, "go")
	writeExecutable(t, pathGo, fakeGoScript("1.26.0"))
	t.Setenv("PATH", pathDir)

	SetBundledDir(t.TempDir())
	got, err := FindGoExecutable("1.25.6")
	if err != nil || got != pathGo {
		t.Fatalf("empty bundle dir: FindGoExecutable = %q, %v; want %q", got, err, pathGo)
	}
}

func TestSetBundledDirDropsACachedAnswer(t *testing.T) {
	isolateGoDiscovery(t)
	isolateBundle(t)
	pathDir := t.TempDir()
	pathGo := filepath.Join(pathDir, "go")
	writeExecutable(t, pathGo, fakeGoScript("1.26.0"))
	t.Setenv("PATH", pathDir)
	if got, err := FindGoExecutable("1.25.6"); err != nil || got != pathGo {
		t.Fatalf("first lookup = %q, %v", got, err)
	}
	bundle := makeBundle(t, "1.26.2", false)
	SetBundledDir(bundle)
	got, err := FindGoExecutable("1.25.6")
	if want := filepath.Join(bundle, BundledToolchainDirName, "bin", "go"); err != nil || got != want {
		t.Fatalf("after SetBundledDir = %q, %v; want %q", got, err, want)
	}
}

func envValue(env []string, key string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func TestGoBuildEnvForBundledToolchain(t *testing.T) {
	isolateBundle(t)
	t.Setenv("GOPROXY", "")
	os.Unsetenv("GOPROXY")
	t.Setenv("GOROOT", "/somewhere/else")
	t.Setenv("GOTOOLCHAIN", "auto")
	t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent")
	bundle := makeBundle(t, "1.26.2", true)
	SetBundledDir(bundle)
	bundledGo := filepath.Join(bundle, BundledToolchainDirName, "bin", "go")

	env := goBuildEnv(bundledGo)
	if v, _ := envValue(env, "GOROOT"); v != filepath.Join(bundle, BundledToolchainDirName) {
		t.Errorf("GOROOT = %q", v)
	}
	if v, _ := envValue(env, "GOTOOLCHAIN"); v != "local" {
		t.Errorf("GOTOOLCHAIN = %q", v)
	}
	proxy, _ := envValue(env, "GOPROXY")
	cacheURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(bundle, BundledModCacheDirName))}).String()
	if proxy != cacheURL+",https://proxy.golang.org,direct" {
		t.Errorf("GOPROXY = %q", proxy)
	}
	for _, key := range []string{"ANTHROPIC_API_KEY", "SSH_AUTH_SOCK"} {
		if _, ok := envValue(env, key); ok {
			t.Errorf("%s reached the go command", key)
		}
	}
}

func TestGoBuildEnvKeepsTheUsersProxyBehindTheBundledCache(t *testing.T) {
	isolateBundle(t)
	t.Setenv("GOPROXY", "https://goproxy.corp.example")
	bundle := makeBundle(t, "1.26.2", true)
	SetBundledDir(bundle)
	proxy, _ := envValue(goBuildEnv(filepath.Join(bundle, BundledToolchainDirName, "bin", "go")), "GOPROXY")
	if !strings.HasPrefix(proxy, "file://") || !strings.HasSuffix(proxy, ",https://goproxy.corp.example") {
		t.Errorf("GOPROXY = %q", proxy)
	}
}

func TestGoBuildEnvEscapesCommasInTheBundlePath(t *testing.T) {
	isolateBundle(t)
	os.Unsetenv("GOPROXY")
	dir := filepath.Join(t.TempDir(), "My Apps, Two|Three")
	writeExecutable(t, filepath.Join(dir, BundledToolchainDirName, "bin", "go"), fakeGoScript("1.26.2"))
	if err := os.MkdirAll(filepath.Join(dir, BundledModCacheDirName), 0755); err != nil {
		t.Fatal(err)
	}
	SetBundledDir(dir)
	proxy, _ := envValue(goBuildEnv(filepath.Join(dir, BundledToolchainDirName, "bin", "go")), "GOPROXY")
	entries := strings.Split(proxy, ",")
	if len(entries) != 3 || strings.Contains(proxy, "|") {
		t.Fatalf("a comma or pipe in the path split the GOPROXY list: %q", proxy)
	}
	parsed, err := url.Parse(entries[0])
	if err != nil || parsed.Path != filepath.ToSlash(filepath.Join(dir, BundledModCacheDirName)) {
		t.Fatalf("the cache URL does not decode back to the directory: %q (%v)", entries[0], err)
	}
}

func TestGoBuildEnvLeavesOtherToolchainsAlone(t *testing.T) {
	isolateBundle(t)
	os.Unsetenv("GOPROXY")
	os.Unsetenv("GOROOT")
	SetBundledDir(makeBundle(t, "1.26.2", true))
	other := filepath.Join(t.TempDir(), "go")
	env := goBuildEnv(other)
	if v, ok := envValue(env, "GOROOT"); ok {
		t.Errorf("GOROOT=%q set for a toolchain that is not ours", v)
	}
	if v, _ := envValue(env, "GOTOOLCHAIN"); v != "local" {
		t.Errorf("GOTOOLCHAIN = %q", v)
	}
	if proxy, _ := envValue(env, "GOPROXY"); !strings.HasPrefix(proxy, "file://") {
		t.Errorf("the offline cache must serve any toolchain, got GOPROXY=%q", proxy)
	}
}

func TestGoBuildEnvWithoutABundleChangesNothingButTheAllowlist(t *testing.T) {
	isolateBundle(t)
	os.Unsetenv("GOPROXY")
	os.Unsetenv("GOROOT")
	env := goBuildEnv("/usr/local/go/bin/go")
	if _, ok := envValue(env, "GOPROXY"); ok {
		t.Error("GOPROXY was set although there is no bundled cache")
	}
	if _, ok := envValue(env, "GOROOT"); ok {
		t.Error("GOROOT was set although there is no bundled toolchain")
	}
}
