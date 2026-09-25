// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func withFakeExec(t *testing.T, fn func(name string, args ...string) ([]byte, error)) {
	t.Helper()
	orig := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = orig })
}

func withRocmPathSeams(t *testing.T, lookPath func(string) (string, error), glob func(string) ([]string, error)) {
	t.Helper()
	origLookPath := rocmLookPath
	origGlob := rocmGlobPaths
	rocmLookPath = lookPath
	rocmGlobPaths = glob
	t.Cleanup(func() {
		rocmLookPath = origLookPath
		rocmGlobPaths = origGlob
	})
}

func withRocmStatSeam(t *testing.T, stat func(string) (os.FileInfo, error)) {
	t.Helper()
	orig := rocmStatPath
	rocmStatPath = stat
	t.Cleanup(func() { rocmStatPath = orig })
}

// fakeFileInfo is a minimal os.FileInfo for stubbing rocmStatPath without
// touching the real filesystem.
type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

func alwaysUsableStat(string) (os.FileInfo, error) {
	return fakeFileInfo{mode: 0o755}, nil
}

// withRocmFound stubs discovery to succeed trivially via PATH, for tests
// whose subject is Collect/Probe parsing rather than path resolution itself.
// The stub path is distinctive (not the bare "rocm-smi" literal) so a
// regression that hardcodes the unresolved command name would still exec
// successfully in withFakeExec but be caught by any test asserting on it.
func withRocmFound(t *testing.T) {
	t.Helper()
	withRocmPathSeams(t,
		func(string) (string, error) { return "/stub/rocm-smi", nil },
		func(string) ([]string, error) { return nil, nil },
	)
}

func TestNvidiaCollectorParsesMultiGpuCsv(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte("0, 45, 2048, 8192, 62\n1, 12, 512, 8192, 51\n"), nil
	})
	c := MakeNvidiaGpuCollector()
	if !c.Probe() {
		t.Fatal("expected probe true with fake exec returning valid output")
	}
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]float64{
		"gpu:0:util":      45, "gpu:0:vram": 2048, "gpu:0:vramtotal": 8192, "gpu:0:temp": 62,
		"gpu:1:util":      12, "gpu:1:vram": 512, "gpu:1:vramtotal": 8192, "gpu:1:temp": 51,
	}
	for k, v := range want {
		if values[k] != v {
			t.Errorf("values[%q] = %v, want %v", k, values[k], v)
		}
	}
}

func TestNvidiaCollectorProbeFalseWhenCommandMissing(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("exec: \"nvidia-smi\": executable file not found in $PATH")
	})
	c := MakeNvidiaGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false when nvidia-smi is not installed")
	}
}

func TestNvidiaCollectorRejectsMalformedOutput(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte("not,csv,at,all,garbage\n"), nil
	})
	c := MakeNvidiaGpuCollector()
	c.Probe()
	if _, err := c.Collect(); err == nil {
		t.Fatal("expected an error on malformed CSV, got nil")
	}
}

func TestNvidiaCollectorRejectsEmptyOutput(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(""), nil
	})
	c := MakeNvidiaGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false on empty output (no GPUs reported)")
	}
}

func TestAmdCollectorParsesRocmSmiJson(t *testing.T) {
	withRocmFound(t)
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(`{
			"card0": {
				"GPU use (%)": "45",
				"VRAM Total Used Memory (B)": "2147483648",
				"VRAM Total Memory (B)": "17179869184",
				"Temperature (Sensor edge) (C)": "62.0"
			}
		}`), nil
	})
	c := MakeAmdGpuCollector()
	if !c.Probe() {
		t.Fatal("expected probe true with fake exec returning valid JSON")
	}
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["gpu:0:util"] != 45 {
		t.Errorf("gpu:0:util = %v, want 45", values["gpu:0:util"])
	}
	wantVramMb := 2147483648.0 / (1024 * 1024)
	if values["gpu:0:vram"] != wantVramMb {
		t.Errorf("gpu:0:vram = %v, want %v", values["gpu:0:vram"], wantVramMb)
	}
	wantVramTotalMb := 17179869184.0 / (1024 * 1024)
	if values["gpu:0:vramtotal"] != wantVramTotalMb {
		t.Errorf("gpu:0:vramtotal = %v, want %v", values["gpu:0:vramtotal"], wantVramTotalMb)
	}
	if values["gpu:0:temp"] != 62.0 {
		t.Errorf("gpu:0:temp = %v, want 62.0", values["gpu:0:temp"])
	}
}

func TestAmdCollectorProbeFalseOnMissingCommand(t *testing.T) {
	withRocmPathSeams(t,
		func(string) (string, error) { return "", errors.New("exec: \"rocm-smi\": executable file not found in $PATH") },
		func(string) ([]string, error) { return nil, nil },
	)
	c := MakeAmdGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false when rocm-smi is not installed anywhere (PATH or /opt/rocm-*/bin)")
	}
}

func TestAmdCollectorRejectsMalformedJson(t *testing.T) {
	withRocmFound(t)
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte("not json at all"), nil
	})
	c := MakeAmdGpuCollector()
	c.Probe()
	if _, err := c.Collect(); err == nil {
		t.Fatal("expected an error on malformed JSON, got nil")
	}
}

func TestAmdCollectorRejectsEmptyOutput(t *testing.T) {
	withRocmFound(t)
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(""), nil
	})
	c := MakeAmdGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false on empty output")
	}
}

func TestResolveRocmSmiPathPrefersPathLookup(t *testing.T) {
	withRocmPathSeams(t,
		func(string) (string, error) { return "/usr/bin/rocm-smi", nil },
		func(string) ([]string, error) {
			t.Fatal("glob should not be called when LookPath succeeds")
			return nil, nil
		},
	)
	path, err := resolveRocmSmiPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/usr/bin/rocm-smi" {
		t.Errorf("path = %q, want /usr/bin/rocm-smi", path)
	}
}

func TestResolveRocmSmiPathFallsBackToOptGlobWhenPathLookupFails(t *testing.T) {
	withRocmPathSeams(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(pattern string) ([]string, error) {
			if pattern != "/opt/rocm-*/bin/rocm-smi" {
				t.Errorf("glob pattern = %q, want /opt/rocm-*/bin/rocm-smi", pattern)
			}
			return []string{"/opt/rocm-7.2.4/bin/rocm-smi"}, nil
		},
	)
	withRocmStatSeam(t, alwaysUsableStat)
	path, err := resolveRocmSmiPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/opt/rocm-7.2.4/bin/rocm-smi" {
		t.Errorf("path = %q, want /opt/rocm-7.2.4/bin/rocm-smi", path)
	}
}

func TestResolveRocmSmiPathPicksHighestVersionWhenMultipleGlobMatches(t *testing.T) {
	withRocmPathSeams(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string) ([]string, error) {
			return []string{
				"/opt/rocm-7.2.4/bin/rocm-smi",
				"/opt/rocm-7.10.0/bin/rocm-smi",
				"/opt/rocm-6.4.1/bin/rocm-smi",
			}, nil
		},
	)
	withRocmStatSeam(t, alwaysUsableStat)
	path, err := resolveRocmSmiPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 7.10.0 is the highest version numerically, but the lexicographically
	// largest string is 7.2.4 ('2' > '1') — this pins the numeric compare.
	if path != "/opt/rocm-7.10.0/bin/rocm-smi" {
		t.Errorf("path = %q, want /opt/rocm-7.10.0/bin/rocm-smi (highest version, not lexicographically largest)", path)
	}
}

func TestResolveRocmSmiPathErrorsWhenNothingFound(t *testing.T) {
	withRocmPathSeams(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string) ([]string, error) { return nil, nil },
	)
	if _, err := resolveRocmSmiPath(); err == nil {
		t.Fatal("expected an error when rocm-smi is not found via PATH or glob")
	}
}

func TestResolveRocmSmiPathSurfacesGlobErrorWithoutPanicking(t *testing.T) {
	withRocmPathSeams(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string) ([]string, error) { return nil, errors.New("glob: malformed pattern") },
	)
	if _, err := resolveRocmSmiPath(); err == nil {
		t.Fatal("expected an error when glob itself fails, got nil")
	}
}

func TestResolveRocmSmiPathFallsBackWhenHighestVersionIsBrokenSymlink(t *testing.T) {
	withRocmPathSeams(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string) ([]string, error) {
			return []string{
				"/opt/rocm-6.4.1/bin/rocm-smi",
				"/opt/rocm-7.10.0/bin/rocm-smi",
			}, nil
		},
	)
	withRocmStatSeam(t, func(path string) (os.FileInfo, error) {
		if path == "/opt/rocm-7.10.0/bin/rocm-smi" {
			return nil, os.ErrNotExist
		}
		return fakeFileInfo{mode: 0o755}, nil
	})
	path, err := resolveRocmSmiPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/opt/rocm-6.4.1/bin/rocm-smi" {
		t.Errorf("path = %q, want fallback to lower version /opt/rocm-6.4.1/bin/rocm-smi since the higher version doesn't resolve", path)
	}
}

func TestHighestVersionedRocmSmiPath(t *testing.T) {
	tests := []struct {
		name    string
		matches []string
		want    string
	}{
		{
			name:    "single match",
			matches: []string{"/opt/rocm-7.2.4/bin/rocm-smi"},
			want:    "/opt/rocm-7.2.4/bin/rocm-smi",
		},
		{
			name: "unparseable version mixed with numeric — numeric wins",
			matches: []string{
				"/opt/rocm-dev/bin/rocm-smi",
				"/opt/rocm-7.2.4/bin/rocm-smi",
			},
			want: "/opt/rocm-7.2.4/bin/rocm-smi",
		},
		{
			name: "two unparseable versions — first wins (stable)",
			matches: []string{
				"/opt/rocm-dev/bin/rocm-smi",
				"/opt/rocm-staging/bin/rocm-smi",
			},
			want: "/opt/rocm-dev/bin/rocm-smi",
		},
		{
			name: "7.2 vs 7.2.0 — tie, first wins (stable)",
			matches: []string{
				"/opt/rocm-7.2/bin/rocm-smi",
				"/opt/rocm-7.2.0/bin/rocm-smi",
			},
			want: "/opt/rocm-7.2/bin/rocm-smi",
		},
		{
			name: "7.2.4 vs 7.2.4.1 — unequal length, more-specific version wins",
			matches: []string{
				"/opt/rocm-7.2.4/bin/rocm-smi",
				"/opt/rocm-7.2.4.1/bin/rocm-smi",
			},
			want: "/opt/rocm-7.2.4.1/bin/rocm-smi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withRocmStatSeam(t, alwaysUsableStat)
			got, ok := highestVersionedRocmSmiPath(tt.matches)
			if !ok {
				t.Fatalf("highestVersionedRocmSmiPath(%v) returned ok=false", tt.matches)
			}
			if got != tt.want {
				t.Errorf("highestVersionedRocmSmiPath(%v) = %q, want %q", tt.matches, got, tt.want)
			}
		})
	}
}

func TestAmdCollectorCachesResolvedPathAcrossCalls(t *testing.T) {
	const resolvedPath = "/opt/rocm-7.2.4/bin/rocm-smi"
	lookPathCalls := 0
	globCalls := 0
	withRocmPathSeams(t,
		func(string) (string, error) {
			lookPathCalls++
			return "", errors.New("not found")
		},
		func(string) ([]string, error) {
			globCalls++
			return []string{resolvedPath}, nil
		},
	)
	withRocmStatSeam(t, alwaysUsableStat)
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		if name != resolvedPath {
			t.Errorf("execCommand called with name %q, want resolved path %q", name, resolvedPath)
		}
		return []byte(`{"card0":{"GPU use (%)":"1","VRAM Total Used Memory (B)":"1","VRAM Total Memory (B)":"1","Temperature (Sensor edge) (C)":"1"}}`), nil
	})
	c := MakeAmdGpuCollector()
	if !c.Probe() {
		t.Fatal("expected probe true with a resolvable rocm-smi path")
	}
	if _, err := c.Collect(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := c.Collect(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lookPathCalls != 1 {
		t.Errorf("lookPath called %d times, want 1 (resolved path should be cached after first resolution)", lookPathCalls)
	}
	if globCalls != 1 {
		t.Errorf("glob called %d times, want 1 (resolved path should be cached after first resolution)", globCalls)
	}
}

// TestResolveRocmSmiPathFindsRealRocmInstallIfPresent exercises the real
// (unstubbed) LookPath/Glob/Stat seams against this specific host, skipping
// itself on machines with no /opt/rocm-*/bin/rocm-smi at all — checked by
// glob rather than a hardcoded version, so this doesn't silently go
// permanently-skip after a ROCm upgrade changes the installed version.
func TestResolveRocmSmiPathFindsRealRocmInstallIfPresent(t *testing.T) {
	matches, _ := filepath.Glob("/opt/rocm-*/bin/rocm-smi")
	if len(matches) == 0 {
		t.Skip("no /opt/rocm-*/bin/rocm-smi on this host")
	}
	path, err := resolveRocmSmiPath()
	if err != nil {
		t.Fatalf("resolveRocmSmiPath failed even though %v exist: %v", matches, err)
	}
	if _, lookErr := exec.LookPath("rocm-smi"); lookErr == nil {
		return
	}
	if !slices.Contains(matches, path) {
		t.Errorf("resolved path %q is not among the real glob matches %v", path, matches)
	}
}

func TestIntelCollectorParsesEngineBusy(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(`{"engines":{"Render/3D":{"busy":37.5}}}`), nil
	})
	c := MakeIntelGpuCollector()
	if !c.Probe() {
		t.Fatal("expected probe true with fake exec returning valid JSON")
	}
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["gpu:0:util"] != 37.5 {
		t.Errorf("gpu:0:util = %v, want 37.5", values["gpu:0:util"])
	}
	if _, ok := values["gpu:0:vram"]; ok {
		t.Error("Intel collector must not emit vram — not reliably available, by design")
	}
	if _, ok := values["gpu:0:temp"]; ok {
		t.Error("Intel collector must not emit temp — not reliably available, by design")
	}
}

func TestIntelCollectorProbeFalseOnMissingCommand(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("exec: \"intel_gpu_top\": executable file not found in $PATH")
	})
	c := MakeIntelGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false when intel_gpu_top is not installed")
	}
}

func TestIntelCollectorRejectsMalformedJson(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte("garbage"), nil
	})
	c := MakeIntelGpuCollector()
	c.Probe()
	if _, err := c.Collect(); err == nil {
		t.Fatal("expected an error on malformed JSON, got nil")
	}
}
