// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const bundledStarterApp = `package main

import (
	"github.com/LannCo/remoteterm/tsunami/app"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

var AppMeta = app.AppMeta{Title: "Bundled", ShortDesc: "built from a bundled toolchain"}

var App = app.DefineComponent("App", func(_ any) any {
	return vdom.H("div", nil, "hello")
})
`

// A Linux stand-in for the packaged macOS resources directory: the running toolchain
// trimmed by the shipping rules, a module cache staged from the local one, and the SDK.
// The build then runs with no go on PATH, GOPROXY=off after the bundled cache, the
// checksum database unreachable and a cold module cache.
func TestBundledToolchainAndCacheBuildTheStarterOffline(t *testing.T) {
	goBin, _, localProxy, minGo := localGoOrSkip(t)
	resources := t.TempDir()
	goroot := filepath.Dir(filepath.Dir(goBin))
	if err := TrimToolchainDir(goroot, filepath.Join(resources, BundledToolchainDirName)); err != nil {
		t.Fatal(err)
	}
	sdkSrc, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	sdkDir := filepath.Join(resources, "tsunamisdk")
	if err := CopySdkBundle(sdkSrc, sdkDir); err != nil {
		t.Fatal(err)
	}
	err = StageModuleCache(context.Background(), ModCacheOpts{
		GoPath: goBin, SdkDir: sdkDir, OutDir: filepath.Join(resources, BundledModCacheDirName), MinGoVersion: minGo,
		Proxy: localProxy, ExtraEnv: []string{"GOSUMDB=off"},
	})
	if err != nil {
		t.Fatal(err)
	}

	scaffold := filepath.Join(resources, "tsunamiscaffold")
	for _, name := range []string{"app-main.go.tmpl", "app-init.go.tmpl", "tailwind.css"} {
		data, err := os.ReadFile(filepath.Join(sdkSrc, "templates", name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(scaffold, name), string(data))
	}
	writeTestFile(t, filepath.Join(scaffold, "package.json"), "{}")
	writeTestFile(t, filepath.Join(scaffold, "dist", "index.html"), "<!doctype html>\n")
	if err := os.MkdirAll(filepath.Join(scaffold, "nm"), 0755); err != nil {
		t.Fatal(err)
	}
	fakeNode := fakeTool(t, "#!/bin/sh\nmkdir -p \"$(dirname \"$7\")\"\n: > \"$7\"\n")

	appDir := filepath.Join(t.TempDir(), "app")
	writeTestFile(t, filepath.Join(appDir, "app.go"), bundledStarterApp)

	isolateGoDiscovery(t)
	isolateBundle(t)
	nogo := t.TempDir()
	t.Setenv("PATH", nogo+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "sum.golang.org")
	t.Setenv("GOFLAGS", "-modcacherw")
	t.Setenv("GOCACHE", t.TempDir())
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("GOROOT", "")
	os.Unsetenv("GOROOT")
	SetBundledDir(resources)

	check := CheckGoVersion("", minGo)
	wantGo := filepath.Join(resources, BundledToolchainDirName, "bin", "go")
	if check.GoStatus != GoStatus_Ok || check.GoPath != wantGo {
		t.Fatalf("discovery = %+v, want the bundled %s", check, wantGo)
	}

	out := filepath.Join(t.TempDir(), "bin", "app")
	err = TsunamiBuildOutput(BuildOpts{
		AppPath: appDir, AppNS: "test", ScaffoldPath: scaffold, SdkReplacePath: sdkDir, SdkVersion: "v0.12.4",
		MinGoVersion: minGo, NodePath: fakeNode, OutputFile: out, OutputCapture: MakeOutputCapture(), MoveFileBack: true,
	})
	if err != nil {
		t.Fatalf("offline build from the bundled toolchain and cache failed: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Mode()&0111 == 0 {
		t.Fatalf("no executable output: %v", err)
	}
	if runtime.GOOS != "windows" {
		manifest, err := os.ReadFile(filepath.Join(appDir, "manifest.json"))
		if err != nil || !strings.Contains(string(manifest), "Bundled") {
			t.Fatalf("manifest from the freshly built app: %q, %v", manifest, err)
		}
	}
}
