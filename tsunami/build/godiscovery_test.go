// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
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

func TestProbeRejectsRelativeOutput(t *testing.T) {
	isolateGoDiscovery(t)
	shell := filepath.Join(t.TempDir(), "zsh")
	writeExecutable(t, shell, "#!/bin/sh\necho go\n")
	got, err := probeLoginShellForGo(shell)
	if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
		t.Fatalf("probeLoginShellForGo = %q, %v; want a not-absolute error", got, err)
	}
}

func readPidFile(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture did not record a background pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("bad pid file %q: %v", data, err)
	}
	return pid
}

func TestProbeTimesOutWithBackgroundChild(t *testing.T) {
	isolateGoDiscovery(t)
	goProbeTimeout = 300 * time.Millisecond
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "bgpid")
	shell := filepath.Join(dir, "bash")
	// The test PATH is an empty temp dir, so the script sets its own to reach sleep.
	writeExecutable(t, shell, "#!/bin/sh\nPATH=/usr/bin:/bin\nsleep 30 &\necho $! > '"+pidFile+"'\nsleep 30\n")
	start := time.Now()
	_, err := probeLoginShellForGo(shell)
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timed-out error, got %v", err)
	}
	if elapsed < goProbeTimeout {
		t.Fatalf("probe returned after %v, before the timeout; the fixture did not hang", elapsed)
	}
	if elapsed > goProbeTimeout+500*time.Millisecond {
		t.Fatalf("probe took %v; the process group was not killed", elapsed)
	}
	requireProcessGone(t, readPidFile(t, pidFile))
}

func TestProbeSucceedsAndReapsBackgroundJobAfterCleanExit(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	fakeGo := filepath.Join(dir, "real", "go")
	writeExecutable(t, fakeGo, plainFakeGo)
	pidFile := filepath.Join(dir, "bgpid")
	shell := filepath.Join(dir, "bash")
	writeExecutable(t, shell, "#!/bin/sh\nPATH=/usr/bin:/bin\nsleep 30 &\necho $! > '"+pidFile+"'\necho '"+fakeGo+"'\n")
	got, err := probeLoginShellForGo(shell)
	if err != nil || got != fakeGo {
		t.Fatalf("probeLoginShellForGo = %q, %v; want %q", got, err, fakeGo)
	}
	requireProcessGone(t, readPidFile(t, pidFile))
}

func TestProbeKeepsLastLineAfterLargeOutput(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	fakeGo := filepath.Join(dir, "real", "go")
	writeExecutable(t, fakeGo, plainFakeGo)
	shell := filepath.Join(dir, "bash")
	writeExecutable(t, shell, "#!/bin/sh\nPATH=/usr/bin:/bin\nhead -c 102400 /dev/zero | tr '\\0' x\necho\necho '"+fakeGo+"'\n")
	got, err := probeLoginShellForGo(shell)
	if err != nil || got != fakeGo {
		t.Fatalf("probeLoginShellForGo = %q, %v; want %q", got, err, fakeGo)
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
	setFailedAgo := func(ago time.Duration) {
		goCache.lock.Lock()
		defer goCache.lock.Unlock()
		goCache.failedAt = time.Now().Add(-ago)
	}
	setFailedAgo(29 * time.Second)
	FindGoExecutable("")
	if n := countRuns(); n != 1 {
		t.Fatalf("probe ran %d times 29s after the failure, want 1", n)
	}
	setFailedAgo(31 * time.Second)
	FindGoExecutable("")
	if n := countRuns(); n != 2 {
		t.Fatalf("probe ran %d times after the TTL expired, want 2", n)
	}
}

func TestFindGoConcurrentCallersShareOneProbe(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	fakeGo := filepath.Join(dir, "real", "go")
	writeExecutable(t, fakeGo, plainFakeGo)
	counter := filepath.Join(dir, "count")
	shell := filepath.Join(dir, "bash")
	writeExecutable(t, shell, "#!/bin/sh\necho x >> '"+counter+"'\necho '"+fakeGo+"'\n")
	t.Setenv("SHELL", shell)
	const callers = 8
	results := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = FindGoExecutable("")
		}()
	}
	wg.Wait()
	for i := 0; i < callers; i++ {
		if errs[i] != nil || results[i] != fakeGo {
			t.Fatalf("caller %d: FindGoExecutable = %q, %v; want %q", i, results[i], errs[i], fakeGo)
		}
	}
	data, _ := os.ReadFile(counter)
	if n := strings.Count(string(data), "x"); n != 1 {
		t.Fatalf("probe ran %d times for %d concurrent callers, want 1", n, callers)
	}
}

func TestFindGoRediscoversWhenCachedPathIsDeleted(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first", "go")
	second := filepath.Join(dir, "second", "go")
	writeExecutable(t, first, plainFakeGo)
	writeExecutable(t, second, plainFakeGo)
	goSearchPaths = func(string) []string { return []string{first, second} }
	got, err := FindGoExecutable("")
	if err != nil || got != first {
		t.Fatalf("FindGoExecutable = %q, %v; want %q", got, err, first)
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	got, err = FindGoExecutable("")
	if err != nil || got != second {
		t.Fatalf("after the cached Go was deleted, FindGoExecutable = %q, %v; want %q", got, err, second)
	}
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if _, err := FindGoExecutable(""); err == nil {
		t.Fatal("expected not found once every Go is deleted")
	}
	if fmtPath := GetCachedGoFmtPath(); fmtPath != "" {
		t.Fatalf("GetCachedGoFmtPath = %q after a failed rediscovery, want empty", fmtPath)
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
