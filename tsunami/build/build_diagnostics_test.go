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
	"time"
)

func skipWithoutShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts as go and node")
	}
}

func fakeTool(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tool")
	writeExecutable(t, path, script)
	return path
}

func setSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("ANTHROPIC_API_KEY", "sk-should-not-leak")
	t.Setenv("OPENAI_API_KEY", "sk-should-not-leak")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/should-not-leak")
}

func assertNoSecrets(t *testing.T, envDump string) {
	t.Helper()
	for _, leak := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "SSH_AUTH_SOCK", "should-not-leak"} {
		if strings.Contains(envDump, leak) {
			t.Errorf("child environment contains %q:\n%s", leak, envDump)
		}
	}
}

func TestTailwindFailurePrintsItsOutput(t *testing.T) {
	skipWithoutShell(t)
	node := fakeTool(t, "#!/bin/sh\necho 'Error: Cannot find native binding @tailwindcss/oxide-darwin-arm64' >&2\nexit 1\n")
	oc := MakeOutputCapture()
	err := generateAppTailwindCss(t.TempDir(), false, BuildOpts{NodePath: node, OutputCapture: oc})
	if err == nil {
		t.Fatal("a failing Tailwind run reported success")
	}
	oc.Flush()
	if joined := strings.Join(oc.GetLines(), "\n"); !strings.Contains(joined, "Cannot find native binding") {
		t.Fatalf("the error points at the output but it was not printed; captured:\n%s", joined)
	}
}

func TestTailwindRunsWithAnAllowlistedEnvironment(t *testing.T) {
	skipWithoutShell(t)
	setSecrets(t)
	dump := filepath.Join(t.TempDir(), "env.txt")
	node := fakeTool(t, "#!/bin/sh\nenv > '"+dump+"'\nexit 0\n")
	if err := generateAppTailwindCss(t.TempDir(), false, BuildOpts{NodePath: node, OutputCapture: MakeOutputCapture()}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecrets(t, string(data))
	if !strings.Contains(string(data), "ELECTRON_RUN_AS_NODE=1") {
		t.Errorf("Electron-as-node needs ELECTRON_RUN_AS_NODE=1:\n%s", data)
	}
	if !strings.Contains(string(data), "PATH=") {
		t.Errorf("PATH was dropped:\n%s", data)
	}
}

func TestGoCommandsRunWithAnAllowlistedEnvironment(t *testing.T) {
	skipWithoutShell(t)
	setSecrets(t)
	t.Setenv("GOTOOLCHAIN", "auto")
	dumpDir := t.TempDir()
	fakeGo := fakeTool(t, `#!/bin/sh
case "$1" in
mod) env > '`+dumpDir+`/tidy.txt' ;;
build) env > '`+dumpDir+`/build.txt'; mkdir -p "$(dirname "$3")"; : > "$3" ;;
esac
`)
	tempDir := t.TempDir()
	writeTestFile(t, filepath.Join(tempDir, "app.go"), "package main\n")
	opts := BuildOpts{MinGoVersion: "1.25.6", SdkVersion: "v0.0.1", OutputCapture: MakeOutputCapture()}
	buildEnv := &BuildEnv{GoPath: fakeGo, GoVersion: "1.26.2"}
	if err := createGoMod(tempDir, "test", "app", buildEnv, opts, false); err != nil {
		t.Fatal(err)
	}
	if _, err := runGoBuild(tempDir, buildEnv, opts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tidy.txt", "build.txt"} {
		data, err := os.ReadFile(filepath.Join(dumpDir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		assertNoSecrets(t, string(data))
		if !strings.Contains(string(data), "GOTOOLCHAIN=local") || strings.Contains(string(data), "GOTOOLCHAIN=auto") {
			t.Errorf("%s: GOTOOLCHAIN must be pinned to local:\n%s", name, data)
		}
	}
}

func deadlineCtx(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestTailwindDeadlineIsNamedInTheError(t *testing.T) {
	skipWithoutShell(t)
	node := fakeTool(t, "#!/bin/sh\nsleep 5\n")
	err := generateAppTailwindCss(t.TempDir(), false, BuildOpts{NodePath: node, Ctx: deadlineCtx(t, 200*time.Millisecond), OutputCapture: MakeOutputCapture(), started: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "tailwind") {
		t.Fatalf("want a Tailwind timeout error, got %v", err)
	}
}

func TestGoBuildDeadlineIsNamedInTheError(t *testing.T) {
	skipWithoutShell(t)
	fakeGo := fakeTool(t, "#!/bin/sh\nsleep 5\n")
	tempDir := t.TempDir()
	writeTestFile(t, filepath.Join(tempDir, "app.go"), "package main\n")
	_, err := runGoBuild(tempDir, &BuildEnv{GoPath: fakeGo}, BuildOpts{Ctx: deadlineCtx(t, 200*time.Millisecond), OutputCapture: MakeOutputCapture(), started: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "compilation") {
		t.Fatalf("want a compilation timeout error, got %v", err)
	}
}

func TestGoModTidyDeadlineIsNamedInTheError(t *testing.T) {
	skipWithoutShell(t)
	fakeGo := fakeTool(t, "#!/bin/sh\nsleep 5\n")
	err := createGoMod(t.TempDir(), "test", "app", &BuildEnv{GoPath: fakeGo, GoVersion: "1.26.2"},
		BuildOpts{MinGoVersion: "1.25.6", SdkVersion: "v0.0.1", Ctx: deadlineCtx(t, 200*time.Millisecond), OutputCapture: MakeOutputCapture(), started: time.Now()}, false)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "go mod tidy") {
		t.Fatalf("want a go mod tidy timeout error, got %v", err)
	}
}

func TestTimeoutMessageNamesTheElapsedBudget(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	opts := BuildOpts{Ctx: ctx, started: time.Now().Add(-62 * time.Second)}
	err := opts.timeoutError("compilation")
	if err == nil || !strings.Contains(err.Error(), "after 62s") || !strings.Contains(err.Error(), "Rebuild") {
		t.Fatalf("got %v", err)
	}
}

func TestOrdinaryFailuresAreNotReportedAsTimeouts(t *testing.T) {
	skipWithoutShell(t)
	fakeGo := fakeTool(t, "#!/bin/sh\nexit 1\n")
	tempDir := t.TempDir()
	writeTestFile(t, filepath.Join(tempDir, "app.go"), "package main\n")
	_, err := runGoBuild(tempDir, &BuildEnv{GoPath: fakeGo}, BuildOpts{Ctx: deadlineCtx(t, time.Minute), OutputCapture: MakeOutputCapture(), started: time.Now()})
	if err == nil || strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "compilation failed") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildTimeoutIsLongerForTheColdCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("GOCACHE", cache)
	if got := BuildTimeout(); got != ColdBuildTimeout {
		t.Fatalf("empty cache: %v, want the cold deadline %v", got, ColdBuildTimeout)
	}
	for i := 0; i < 256; i++ {
		if err := os.MkdirAll(filepath.Join(cache, hex2(i)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if got := BuildTimeout(); got != ColdBuildTimeout {
		t.Fatalf("the empty hex directories go creates on first open are not a warm cache; got %v", got)
	}
	writeTestFile(t, filepath.Join(cache, "3f", "abcdef-d"), "x")
	if got := BuildTimeout(); got != WarmBuildTimeout {
		t.Fatalf("populated cache: %v, want the warm deadline %v", got, WarmBuildTimeout)
	}
	if ColdBuildTimeout <= WarmBuildTimeout {
		t.Fatalf("the cold deadline (%v) must exceed the warm one (%v)", ColdBuildTimeout, WarmBuildTimeout)
	}
}

func TestBuildTimeoutWithCacheOffIsCold(t *testing.T) {
	t.Setenv("GOCACHE", "off")
	if got := BuildTimeout(); got != ColdBuildTimeout {
		t.Fatalf("GOCACHE=off: %v", got)
	}
}

func hex2(i int) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[i>>4], digits[i&15]})
}

func TestSeedGoSumMergesTheSdkSumsWithoutDroppingTheApps(t *testing.T) {
	sdk := t.TempDir()
	writeTestFile(t, filepath.Join(sdk, "go.sum"), "example.com/a v1.0.0 h1:aaa=\nexample.com/a v1.0.0/go.mod h1:bbb=\n")
	tempDir := t.TempDir()
	writeTestFile(t, filepath.Join(tempDir, "go.sum"), "example.com/a v1.0.0/go.mod h1:bbb=\nexample.com/own v2.0.0 h1:ccc=\n")
	if err := seedGoSum(tempDir, sdk); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(tempDir, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := map[string]int{
		"example.com/a v1.0.0 h1:aaa=":        1,
		"example.com/a v1.0.0/go.mod h1:bbb=": 1,
		"example.com/own v2.0.0 h1:ccc=":      1,
	}
	if len(lines) != len(want) {
		t.Fatalf("go.sum has %d lines, want %d:\n%s", len(lines), len(want), data)
	}
	for _, line := range lines {
		want[line]--
	}
	for line, n := range want {
		if n != 0 {
			t.Errorf("line %q appears %d times too few/many", line, -n)
		}
	}
}

func TestSeedGoSumToleratesMissingFiles(t *testing.T) {
	tempDir := t.TempDir()
	if err := seedGoSum(tempDir, t.TempDir()); err != nil {
		t.Fatalf("an SDK without go.sum must not fail the build: %v", err)
	}
	if err := seedGoSum(tempDir, ""); err != nil {
		t.Fatalf("no SDK replace path must not fail the build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "go.sum")); err == nil {
		t.Error("an empty go.sum was created from nothing")
	}
}

func TestCreateGoModSeedsGoSumBeforeTidy(t *testing.T) {
	skipWithoutShell(t)
	sdk := t.TempDir()
	writeTestFile(t, filepath.Join(sdk, "go.sum"), "example.com/a v1.0.0 h1:aaa=\n")
	tempDir := t.TempDir()
	seen := filepath.Join(t.TempDir(), "seen")
	fakeGo := fakeTool(t, "#!/bin/sh\ncp go.sum '"+seen+"'\n")
	err := createGoMod(tempDir, "test", "app", &BuildEnv{GoPath: fakeGo, GoVersion: "1.26.2"},
		BuildOpts{MinGoVersion: "1.25.6", SdkVersion: "v0.0.1", SdkReplacePath: sdk, OutputCapture: MakeOutputCapture()}, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(seen)
	if err != nil || !strings.Contains(string(data), "example.com/a v1.0.0 h1:aaa=") {
		t.Fatalf("go mod tidy ran without the SDK's sums: %q, %v", data, err)
	}
}
