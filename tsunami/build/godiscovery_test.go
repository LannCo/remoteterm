// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func isolateGoDiscovery(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("discovery fixtures use shell scripts")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "")
	origSearch := goSearchPaths
	origTimeout := goProbeTimeout
	goSearchPaths = func(string) []string { return nil }
	resetGoDiscoveryCache()
	t.Cleanup(func() {
		goSearchPaths = origSearch
		goProbeTimeout = origTimeout
		resetGoDiscoveryCache()
	})
	return home
}

func writeExecutable(t *testing.T, path string, script string) {
	t.Helper()
	writeTestFile(t, path, script)
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func fakeGoScript(version string) string {
	return "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'go version go" + version + " linux/amd64'; exit 0; fi\nexit 1\n"
}

var plainFakeGo = fakeGoScript("1.26.0")

func TestFindGoUsesPath(t *testing.T) {
	isolateGoDiscovery(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "go"), plainFakeGo)
	t.Setenv("PATH", binDir)
	got, err := FindGoExecutable("")
	if err != nil || got != filepath.Join(binDir, "go") {
		t.Fatalf("FindGoExecutable = %q, %v", got, err)
	}
}

func TestFindGoUsesFirstExecutableSearchPath(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	notExec := filepath.Join(dir, "b", "go")
	writeTestFile(t, notExec, plainFakeGo)
	first := filepath.Join(dir, "c", "go")
	second := filepath.Join(dir, "d", "go")
	writeExecutable(t, first, plainFakeGo)
	writeExecutable(t, second, plainFakeGo)
	goSearchPaths = func(string) []string {
		return []string{filepath.Join(dir, "a", "go"), notExec, first, second}
	}
	got, err := FindGoExecutable("")
	if err != nil || got != first {
		t.Fatalf("FindGoExecutable = %q, %v; want %q", got, err, first)
	}
}

func TestDefaultGoSearchPathsOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix search list")
	}
	home := t.TempDir()
	for _, v := range []string{"go1.9.2", "go1.26.1", "go1.25.6"} {
		writeExecutable(t, filepath.Join(home, "sdk", v, "bin", "go"), plainFakeGo)
	}
	want := []string{
		"/opt/homebrew/bin/go",
		"/usr/local/bin/go",
		"/usr/local/go/bin/go",
		"/usr/bin/go",
		filepath.Join(home, ".local", "go", "bin", "go"),
		"/usr/lib/go/bin/go",
		filepath.Join(home, "sdk", "go1.26.1", "bin", "go"),
		"/snap/bin/go",
		filepath.Join(home, ".local", "share", "mise", "shims", "go"),
		filepath.Join(home, ".asdf", "shims", "go"),
	}
	if got := defaultGoSearchPaths(home); !slices.Equal(got, want) {
		t.Fatalf("defaultGoSearchPaths:\n got %v\nwant %v", got, want)
	}
}

func TestProbeTakesLastLineAndPassesLoginFlags(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	fakeGo := filepath.Join(dir, "real", "go")
	writeExecutable(t, fakeGo, plainFakeGo)
	argsFile := filepath.Join(dir, "args")
	shell := filepath.Join(dir, "shells", "bash")
	writeExecutable(t, shell, "#!/bin/sh\nprintf '%s ' \"$@\" > '"+argsFile+"'\necho 'welcome to the machine'\necho\necho '"+fakeGo+"'\n")
	t.Setenv("SHELL", shell)
	got, err := FindGoExecutable("")
	if err != nil || got != fakeGo {
		t.Fatalf("FindGoExecutable = %q, %v; want %q", got, err, fakeGo)
	}
	args, _ := os.ReadFile(argsFile)
	if strings.TrimSpace(string(args)) != "-l -i -c command -v go" {
		t.Fatalf("bash probe args = %q", args)
	}

	resetGoDiscoveryCache()
	shShell := filepath.Join(dir, "shells2", "sh")
	writeExecutable(t, shShell, "#!/bin/sh\nprintf '%s ' \"$@\" > '"+argsFile+"'\necho '"+fakeGo+"'\n")
	t.Setenv("SHELL", shShell)
	if _, err := FindGoExecutable(""); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(argsFile)
	if strings.TrimSpace(string(args)) != "-l -c command -v go" {
		t.Fatalf("sh probe args = %q", args)
	}
}

func TestProbeRejectsRelativePath(t *testing.T) {
	isolateGoDiscovery(t)
	shell := filepath.Join(t.TempDir(), "zsh")
	writeExecutable(t, shell, "#!/bin/sh\necho go\n")
	got, err := probeLoginShellForGo(shell)
	if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
		t.Fatalf("probeLoginShellForGo = %q, %v; want a not-absolute error", got, err)
	}
}

func TestProbeTimesOutWithBackgroundChild(t *testing.T) {
	isolateGoDiscovery(t)
	goProbeTimeout = 300 * time.Millisecond
	shell := filepath.Join(t.TempDir(), "bash")
	// The test PATH is an empty temp dir, so the script sets its own to reach sleep.
	writeExecutable(t, shell, "#!/bin/sh\nPATH=/usr/bin:/bin\n(sleep 30) &\nsleep 30\n")
	start := time.Now()
	_, err := probeLoginShellForGo(shell)
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timed-out error, got %v", err)
	}
	if elapsed < goProbeTimeout {
		t.Fatalf("probe returned after %v, before the timeout; the fixture did not hang", elapsed)
	}
	if elapsed > goProbeTimeout+goProbeWaitDelay+time.Second {
		t.Fatalf("probe took %v; the process group was not killed", elapsed)
	}
}

func TestProbeSkipsUnsetMissingAndUnknownShells(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	unknown := filepath.Join(dir, "tcsh")
	writeExecutable(t, unknown, "#!/bin/sh\n: > '"+marker+"'\n")
	for _, shell := range []string{"", "bash", filepath.Join(dir, "missing", "bash"), unknown} {
		resetGoDiscoveryCache()
		t.Setenv("SHELL", shell)
		start := time.Now()
		if _, err := FindGoExecutable(""); err == nil {
			t.Fatalf("SHELL=%q: expected not found", shell)
		}
		if time.Since(start) > time.Second {
			t.Fatalf("SHELL=%q: discovery took %v", shell, time.Since(start))
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a shell outside the allowlist was executed")
	}
}

func TestFindGoCachesFailureFor30Seconds(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	shell := filepath.Join(dir, "bash")
	writeExecutable(t, shell, "#!/bin/sh\necho x >> '"+counter+"'\n")
	t.Setenv("SHELL", shell)
	countRuns := func() int {
		data, _ := os.ReadFile(counter)
		return strings.Count(string(data), "x")
	}
	FindGoExecutable("")
	FindGoExecutable("")
	if n := countRuns(); n != 1 {
		t.Fatalf("probe ran %d times within the failure TTL, want 1", n)
	}
	goCache.lock.Lock()
	goCache.failedAt = time.Now().Add(-31 * time.Second)
	goCache.lock.Unlock()
	FindGoExecutable("")
	if n := countRuns(); n != 2 {
		t.Fatalf("probe ran %d times after the TTL expired, want 2", n)
	}
}

func TestFindGoCanonicalisesGoroot(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	goroot := filepath.Join(dir, "goroot")
	writeExecutable(t, filepath.Join(goroot, "bin", "go"), plainFakeGo)
	writeExecutable(t, filepath.Join(goroot, "bin", "gofmt"), plainFakeGo)
	shimDir := filepath.Join(dir, "shims")
	writeExecutable(t, filepath.Join(shimDir, "go"), "#!/bin/sh\nif [ \"$1\" = env ] && [ \"$2\" = GOROOT ]; then echo '"+goroot+"'; exit 0; fi\nif [ \"$1\" = version ]; then echo 'go version go1.26.0 linux/amd64'; exit 0; fi\nexit 1\n")
	t.Setenv("PATH", shimDir)
	if got := GetCachedGoFmtPath(); got != "" {
		t.Fatalf("GetCachedGoFmtPath before discovery = %q, want empty", got)
	}
	got, err := FindGoExecutable("")
	if err != nil || got != filepath.Join(goroot, "bin", "go") {
		t.Fatalf("FindGoExecutable = %q, %v", got, err)
	}
	if fmtPath := GetCachedGoFmtPath(); fmtPath != filepath.Join(goroot, "bin", "gofmt") {
		t.Fatalf("GetCachedGoFmtPath = %q", fmtPath)
	}
}

func TestFindGoSkipsTooOldCandidate(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	binDir := filepath.Join(dir, "path")
	writeExecutable(t, filepath.Join(binDir, "go"), fakeGoScript("1.24.0"))
	t.Setenv("PATH", binDir)
	newer := filepath.Join(dir, "newer", "go")
	writeExecutable(t, newer, fakeGoScript("1.26.1"))
	goSearchPaths = func(string) []string { return []string{newer} }
	got, err := FindGoExecutable("1.25.6")
	if err != nil || got != newer {
		t.Fatalf("FindGoExecutable = %q, %v; want the newer %q", got, err, newer)
	}
}

func TestFindGoReportsNewestTooOld(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	binDir := filepath.Join(dir, "path")
	writeExecutable(t, filepath.Join(binDir, "go"), fakeGoScript("1.24.0"))
	t.Setenv("PATH", binDir)
	older := filepath.Join(dir, "older", "go")
	writeExecutable(t, older, fakeGoScript("1.25.1"))
	goSearchPaths = func(string) []string { return []string{older} }

	_, err := FindGoExecutable("1.25.6")
	var tooOld *GoTooOldError
	if !errors.As(err, &tooOld) || tooOld.Version != "1.25.1" || tooOld.GoPath != older {
		t.Fatalf("err = %v; want GoTooOldError naming 1.25.1 at %s", err, older)
	}

	resetGoDiscoveryCache()
	res := CheckGoVersion("", "1.25.6")
	if res.GoStatus != GoStatus_BadVersion || res.Version != "1.25.1" || res.GoPath != older {
		t.Fatalf("CheckGoVersion = %+v; want badversion 1.25.1", res)
	}
}
