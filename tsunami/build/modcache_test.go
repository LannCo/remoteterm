// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestSeedModuleImportsEveryBundledSdkPackage(t *testing.T) {
	sdk := t.TempDir()
	goMod, mainGo := seedModuleFiles(sdk, "1.25.6")
	mf, err := modfile.Parse("go.mod", []byte(goMod), nil)
	if err != nil {
		t.Fatalf("seed go.mod does not parse: %v\n%s", err, goMod)
	}
	if len(mf.Replace) != 1 || mf.Replace[0].New.Path != sdk || mf.Replace[0].Old.Path != TsunamiSdkModulePath {
		t.Errorf("replace = %+v", mf.Replace)
	}
	if mf.Go == nil || mf.Go.Version != "1.25.6" {
		t.Errorf("go directive = %+v", mf.Go)
	}
	for _, pkg := range SdkBundlePackageDirs {
		if !strings.Contains(mainGo, `_ "`+TsunamiSdkModulePath+"/"+pkg+`"`) {
			t.Errorf("seed main.go does not import %s", pkg)
		}
	}
}

func TestCopyProxyLayoutKeepsOnlyWhatAProxyServes(t *testing.T) {
	src := t.TempDir()
	for _, rel := range []string{
		"github.com/google/uuid/@v/list",
		"github.com/google/uuid/@v/v1.6.0.info",
		"github.com/google/uuid/@v/v1.6.0.mod",
		"github.com/google/uuid/@v/v1.6.0.zip",
		"github.com/google/uuid/@v/v1.6.0.ziphash",
		"github.com/google/uuid/@v/v1.6.0.lock",
		"github.com/google/uuid/@v/v1.6.0.partial",
		"sumdb/sum.golang.org/latest",
	} {
		writeTestFile(t, filepath.Join(src, filepath.FromSlash(rel)), rel)
	}
	dst := filepath.Join(t.TempDir(), "gomodcache")
	n, err := copyProxyLayout(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	got := listFiles(t, dst)
	want := []string{
		"github.com/google/uuid/@v/list",
		"github.com/google/uuid/@v/v1.6.0.info",
		"github.com/google/uuid/@v/v1.6.0.mod",
		"github.com/google/uuid/@v/v1.6.0.zip",
	}
	if len(got) != len(want) || n != len(want) {
		t.Fatalf("copied %d files %v, want %v", n, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestCopyProxyLayoutRefusesAnEmptyCache(t *testing.T) {
	if _, err := copyProxyLayout(t.TempDir(), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("an empty module cache was accepted; the bundle would silently ship nothing")
	}
}

// localGoOrSkip finds a Go toolchain and a module cache that already holds the SDK's
// dependencies, so the next two tests exercise the real go command without the network.
func localGoOrSkip(t *testing.T) (goBin string, sdkDir string, localProxy string, minGo string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file proxy paths differ on Windows")
	}
	goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		t.Skip("no go binary at runtime.GOROOT()/bin/go")
	}
	sdkSrc, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	minGo, err = ReadSdkGoVersion(sdkSrc)
	if err != nil {
		t.Fatal(err)
	}
	sdkDir = filepath.Join(t.TempDir(), "sdk")
	if err := CopySdkBundle(sdkSrc, sdkDir); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(goBin, "env", "GOMODCACHE").Output()
	if err != nil {
		t.Skipf("go env GOMODCACHE: %v", err)
	}
	download := filepath.Join(strings.TrimSpace(string(out)), "cache", "download")
	for _, probe := range []string{"github.com/google/uuid/@v/v1.6.0.zip", "github.com/outrigdev/goid/@v/v0.3.0.zip"} {
		if _, err := os.Stat(filepath.Join(download, filepath.FromSlash(probe))); err != nil {
			t.Skipf("the local module cache lacks %s", probe)
		}
	}
	return goBin, sdkDir, "file://" + download, minGo
}

func TestStageModuleCacheThenBuildTheSeedOffline(t *testing.T) {
	goBin, sdkDir, localProxy, minGo := localGoOrSkip(t)
	out := filepath.Join(t.TempDir(), "gomodcache")
	err := StageModuleCache(context.Background(), ModCacheOpts{
		GoPath: goBin, SdkDir: sdkDir, OutDir: out, MinGoVersion: minGo,
		Proxy: localProxy, ExtraEnv: []string{"GOSUMDB=off", "GOFLAGS=-modcacherw"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range []string{"github.com/google/uuid/@v/v1.6.0.zip", "github.com/outrigdev/goid/@v/v0.3.0.zip"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(probe))); err != nil {
			t.Errorf("staged cache lacks %s", probe)
		}
	}

	// A fresh app, a fresh module cache, GOPROXY=off after the bundled cache, and the
	// real checksum database left on (unreachable here): this is the friend's first build.
	app := t.TempDir()
	goMod, mainGo := seedModuleFiles(sdkDir, minGo)
	writeTestFile(t, filepath.Join(app, "go.mod"), goMod)
	writeTestFile(t, filepath.Join(app, "main.go"), mainGo)
	if err := seedGoSum(app, sdkDir); err != nil {
		t.Fatal(err)
	}
	env := AllowlistedEnv(os.Environ(),
		"GOTOOLCHAIN=local", "GOMODCACHE="+t.TempDir(), "GOFLAGS=-modcacherw", "GOSUMDB=sum.golang.org",
		"GOPROXY=file://"+out+",off", "GOCACHE="+t.TempDir(), "HTTPS_PROXY=http://127.0.0.1:9", "HTTP_PROXY=http://127.0.0.1:9")
	for _, args := range [][]string{{"mod", "tidy"}, {"build", "-o", filepath.Join(app, "bin"), "."}} {
		cmd := exec.Command(goBin, args...)
		cmd.Dir = app
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s offline from the staged cache: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
}
