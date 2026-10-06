// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ModCacheOpts struct {
	GoPath       string
	SdkDir       string
	OutDir       string
	MinGoVersion string
	// Proxy is where the dependencies are fetched from at staging time; empty means the
	// public Go proxy.
	Proxy string
	// ExtraEnv is applied last; tests use it to point at a local cache.
	ExtraEnv []string
}

const seedModulePath = "tsunami/seed/modcache"

// seedModuleFiles describes an app that imports every package the SDK bundle holds. Resolving
// it downloads exactly what a real app's go mod tidy downloads, because the same SDK
// go.mod and the same replace directive are in play.
func seedModuleFiles(sdkDir string, goVersion string) (string, string) {
	goMod := fmt.Sprintf("module %s\n\ngo %s\n\nrequire %s v0.0.0\n\nreplace %s => %q\n",
		seedModulePath, goVersion, TsunamiSdkModulePath, TsunamiSdkModulePath, sdkDir)
	var imports strings.Builder
	for _, pkg := range SdkBundlePackageDirs {
		fmt.Fprintf(&imports, "\t_ %q\n", TsunamiSdkModulePath+"/"+pkg)
	}
	mainGo := "package main\n\nimport (\n" + imports.String() + ")\n\nfunc main() {}\n"
	return goMod, mainGo
}

// StageModuleCache fills OutDir with the SDK's dependencies laid out as a Go module proxy
// (what GOPROXY=file://OutDir serves). The packaged app lists it ahead of the real proxy,
// so a first build needs no network for them.
func StageModuleCache(ctx context.Context, opts ModCacheOpts) error {
	if opts.GoPath == "" || opts.SdkDir == "" || opts.OutDir == "" {
		return errors.New("GoPath, SdkDir and OutDir are required")
	}
	goVersion := opts.MinGoVersion
	if goVersion == "" {
		var err error
		goVersion, err = ReadSdkGoVersion(opts.SdkDir)
		if err != nil {
			return err
		}
	}
	work, err := os.MkdirTemp("", "tsunami-modcache-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	app := filepath.Join(work, "app")
	modCache := filepath.Join(work, "mod")
	goMod, mainGo := seedModuleFiles(opts.SdkDir, goVersion)
	for name, content := range map[string]string{"go.mod": goMod, "main.go": mainGo} {
		if err := os.MkdirAll(app, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(app, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	if err := seedGoSum(app, opts.SdkDir); err != nil {
		return err
	}

	proxy := opts.Proxy
	if proxy == "" {
		proxy = defaultGoProxy
	}
	extra := []string{
		"GOTOOLCHAIN=local", "GOMODCACHE=" + modCache, "GOCACHE=" + filepath.Join(work, "gocache"),
		"GOFLAGS=-modcacherw", "GOPROXY=" + proxy,
	}
	extra = append(extra, opts.ExtraEnv...)
	cmd := exec.CommandContext(ctx, opts.GoPath, "mod", "tidy")
	cmd.Dir = app
	cmd.Env = AllowlistedEnv(os.Environ(), extra...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy for the seed app failed: %w\n%s", err, output)
	}

	if err := os.RemoveAll(opts.OutDir); err != nil {
		return err
	}
	if _, err := copyProxyLayout(filepath.Join(modCache, "cache", "download"), opts.OutDir); err != nil {
		return err
	}
	return nil
}

// Of what go keeps under cache/download, a proxy serves the version lists and each
// version's .info, .mod and .zip; the rest (locks, .ziphash, the sumdb tiles) is the
// cache's own bookkeeping.
func copyProxyLayout(srcDir string, dstDir string) (int, error) {
	copied := 0
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "sumdb" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(filepath.Dir(rel)) != "@v" || !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		ext := filepath.Ext(name)
		if name != "list" && ext != ".info" && ext != ".mod" && ext != ".zip" {
			return nil
		}
		if err := copyFile(path, filepath.Join(dstDir, rel)); err != nil {
			return err
		}
		copied++
		return nil
	})
	if err != nil {
		return 0, err
	}
	if copied == 0 {
		return 0, fmt.Errorf("no module files found under %s", srcDir)
	}
	return copied, nil
}
