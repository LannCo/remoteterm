// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const cleanupFakeGoScript = `#!/bin/sh
case "$1" in
version) echo "go version go1.26.3 linux/amd64" ;;
mod) exit 0 ;;
build)
	if [ -e "$(dirname "$0")/fail" ]; then exit 1; fi
	mkdir -p "$(dirname "$3")"
	printf '#!/bin/sh\necho "<AppManifest>{}</AppManifest>"\n' > "$3"
	chmod +x "$3"
	;;
esac
`

// leftoverBuildDirs lists the build temp directories under the TMPDIR the test set.
func leftoverBuildDirs(t *testing.T, tmp string) []string {
	t.Helper()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "tsunami-build-") {
			found = append(found, e.Name())
		}
	}
	return found
}

// fakeBuildOpts sets TMPDIR to a fresh directory and returns it with BuildOpts that run
// a whole build against a stand-in go and node, so no toolchain is needed.
func fakeBuildOpts(t *testing.T) (string, BuildOpts) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts as the go and node binaries")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	tools := t.TempDir()
	fakeGo := filepath.Join(tools, "go")
	fakeNode := filepath.Join(tools, "node")
	write := func(path, content string, mode os.FileMode) {
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(fakeGo, cleanupFakeGoScript, 0755)
	write(fakeNode, "#!/bin/sh\nexit 0\n", 0755)

	work := t.TempDir()
	appDir := filepath.Join(work, "app")
	scaffold := filepath.Join(work, "scaffold")
	for _, dir := range []string{appDir, filepath.Join(scaffold, "dist"), filepath.Join(scaffold, "nm")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(appDir, "app.go"), "package main\n", 0644)
	for _, name := range []string{"app-main.go.tmpl", "app-init.go.tmpl", "tailwind.css", "package.json"} {
		write(filepath.Join(scaffold, name), "x", 0644)
	}

	return tmp, BuildOpts{
		AppPath:       appDir,
		AppNS:         "test",
		ScaffoldPath:  scaffold,
		SdkVersion:    "v0.0.1",
		GoPath:        fakeGo,
		NodePath:      fakeNode,
		OutputFile:    filepath.Join(work, "out", "app-bin"),
		OutputCapture: MakeOutputCapture(),
	}
}

func TestTsunamiBuildOutputLeavesNoTempDirOnSuccess(t *testing.T) {
	tmp, opts := fakeBuildOpts(t)
	if err := TsunamiBuildOutput(opts); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if _, err := os.Stat(opts.OutputFile); err != nil {
		t.Fatalf("the build produced no output file: %v", err)
	}
	if left := leftoverBuildDirs(t, tmp); len(left) != 0 {
		t.Fatalf("temp directories left behind: %v", left)
	}
}

func TestTsunamiBuildOutputLeavesNoTempDirOnFailure(t *testing.T) {
	tmp, opts := fakeBuildOpts(t)
	// The build scrubs its environment, so the stand-in go is told to fail by a file beside it.
	if err := os.WriteFile(filepath.Join(filepath.Dir(opts.GoPath), "fail"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	err := TsunamiBuildOutput(opts)
	if err == nil || !strings.Contains(err.Error(), "compilation failed") {
		t.Fatalf("want a compilation failure, got %v", err)
	}
	if left := leftoverBuildDirs(t, tmp); len(left) != 0 {
		t.Fatalf("temp directories left behind: %v", left)
	}
}

func TestTsunamiBuildOutputKeepsTempDirWhenAsked(t *testing.T) {
	tmp, opts := fakeBuildOpts(t)
	opts.KeepTemp = true
	if err := TsunamiBuildOutput(opts); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if left := leftoverBuildDirs(t, tmp); len(left) != 1 {
		t.Fatalf("KeepTemp should leave exactly one temp directory, found %v", left)
	}
}

// TsunamiBuildInternal hands the temp directory to its caller; this pins the contract that
// TsunamiBuildOutput exists to honour, and proves the checks above can see a leftover.
func TestTsunamiBuildInternalLeavesCleanupToCaller(t *testing.T) {
	tmp, opts := fakeBuildOpts(t)
	env, err := TsunamiBuildInternal(opts)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if left := leftoverBuildDirs(t, tmp); len(left) != 1 {
		t.Fatalf("expected one temp directory before cleanup, found %v", left)
	}
	env.cleanupTempDir(false, false)
	if left := leftoverBuildDirs(t, tmp); len(left) != 0 {
		t.Fatalf("cleanupTempDir left %v", left)
	}
}
