// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// execCommand is a package-level seam so GPU collector tests can inject
// canned CLI output instead of shelling out to real vendor tools — none of
// the three vendors' tools can be assumed present on any given dev/CI host.
var execCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// rocmLookPath, rocmGlobPaths and rocmStatPath are package-level seams so
// rocm-smi discovery tests can stub PATH lookup, filesystem globbing and
// existence checks without depending on this machine's actual ROCm install.
var rocmLookPath = exec.LookPath
var rocmGlobPaths = filepath.Glob
var rocmStatPath = os.Stat

var rocmVersionPattern = regexp.MustCompile(`rocm-(\d+(?:\.\d+)*)`)

func rocmVersionParts(path string) []int {
	m := rocmVersionPattern.FindStringSubmatch(path)
	if m == nil {
		return nil
	}
	segs := strings.Split(m[1], ".")
	parts := make([]int, len(segs))
	for i, s := range segs {
		// rocmVersionPattern guarantees s is \d+, so the only possible
		// Atoi failure is overflow, which clamps to max int and still
		// sorts correctly — nothing meaningful to handle here.
		parts[i], _ = strconv.Atoi(s)
	}
	return parts
}

// rocmVersionLess compares version segments numerically, not lexicographically:
// a string compare would rank "rocm-7.2.4" above "rocm-7.10.0" (the byte '2' > '1').
func rocmVersionLess(a, b []int) bool {
	for i := 0; i < len(a) || i < len(b); i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			return av < bv
		}
	}
	return false
}

// rocmSmiPathUsable guards against filepath.Glob matching a dangling
// symlink: rocm-smi itself is typically a symlink under /opt/rocm-X/bin,
// and the numerically-highest match on paper may be a broken or stale
// install, so each candidate is verified to actually resolve before it's
// trusted.
func rocmSmiPathUsable(path string) bool {
	info, err := rocmStatPath(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0111 != 0
}

// highestVersionedRocmSmiPath sorts candidates newest-version-first, then
// returns the first one that actually exists and is executable — not just
// the numerically-highest match blindly.
func highestVersionedRocmSmiPath(matches []string) (string, bool) {
	sorted := append([]string(nil), matches...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return rocmVersionLess(rocmVersionParts(sorted[j]), rocmVersionParts(sorted[i]))
	})
	for _, m := range sorted {
		if rocmSmiPathUsable(m) {
			return m, true
		}
	}
	return "", false
}

// resolveRocmSmiPath tries PATH first (works on hosts where rocm-smi is
// properly linked), then falls back to globbing every side-by-side
// /opt/rocm-* install — ROCm installs commonly live side by side under
// /opt/rocm-X.Y.Z, and that bin dir is often not on the server process's PATH.
func resolveRocmSmiPath() (string, error) {
	if path, err := rocmLookPath("rocm-smi"); err == nil {
		return path, nil
	}
	matches, err := rocmGlobPaths("/opt/rocm-*/bin/rocm-smi")
	if err != nil || len(matches) == 0 {
		return "", errors.New("rocm-smi not found on PATH or under /opt/rocm-*/bin")
	}
	path, ok := highestVersionedRocmSmiPath(matches)
	if !ok {
		return "", errors.New("rocm-smi not found on PATH or under /opt/rocm-*/bin")
	}
	return path, nil
}

type gpuReading struct {
	index     int
	util      float64
	vram      float64
	vramTotal float64
	temp      float64
}

func gpuMetaFor(index int) map[string]MetricMeta {
	label := "GPU " + strconv.Itoa(index)
	return map[string]MetricMeta{
		fmt.Sprintf("gpu:%d:util", index): {Label: label + " %", Unit: "%", Color: gpuColorFor(index), MinY: 0, MaxY: 100, DecimalPlaces: 0},
		fmt.Sprintf("gpu:%d:vram", index): {Label: label + " VRAM", Unit: "MB", Color: gpuColorFor(index), MinY: 0, MaxYKey: fmt.Sprintf("gpu:%d:vramtotal", index), DecimalPlaces: 0},
		fmt.Sprintf("gpu:%d:temp", index): {Label: label + " Temp", Unit: "°C", Color: gpuColorFor(index), MinY: 0, MaxY: 110, DecimalPlaces: 0},
	}
}

// gpuColorFor is resolved on the frontend from a fixed palette indexed by
// GPU number (see sysinfo.tsx's _plotColors array, Task 9) — the backend
// only needs to emit a stable per-index token here; frontend Task 9 maps
// index -> concrete color, this string is illustrative and gets overridden
// by the frontend's own palette lookup keyed on the "gpu:N:" prefix.
func gpuColorFor(index int) string {
	return "var(--sysinfo-gpu-color)"
}

type nvidiaGpuCollector struct {
	indices []int
}

func MakeNvidiaGpuCollector() Collector {
	return &nvidiaGpuCollector{}
}

func (n *nvidiaGpuCollector) Name() string {
	return "gpu-nvidia"
}

func parseNvidiaSmiCsv(output []byte) ([]gpuReading, error) {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return nil, errors.New("nvidia-smi returned no output")
	}
	lines := strings.Split(text, "\n")
	readings := make([]gpuReading, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, ",")
		if len(fields) != 5 {
			return nil, fmt.Errorf("unexpected nvidia-smi CSV row shape: %q", line)
		}
		idx, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			return nil, fmt.Errorf("bad gpu index in row %q: %w", line, err)
		}
		util, err := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("bad utilization in row %q: %w", line, err)
		}
		vram, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			return nil, fmt.Errorf("bad vram in row %q: %w", line, err)
		}
		vramTotal, err := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		if err != nil {
			return nil, fmt.Errorf("bad vram total in row %q: %w", line, err)
		}
		temp, err := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		if err != nil {
			return nil, fmt.Errorf("bad temp in row %q: %w", line, err)
		}
		readings = append(readings, gpuReading{index: idx, util: util, vram: vram, vramTotal: vramTotal, temp: temp})
	}
	return readings, nil
}

func (n *nvidiaGpuCollector) Probe() bool {
	out, err := execCommand("nvidia-smi", "--query-gpu=index,utilization.gpu,memory.used,memory.total,temperature.gpu", "--format=csv,noheader,nounits")
	if err != nil {
		return false
	}
	readings, err := parseNvidiaSmiCsv(out)
	if err != nil || len(readings) == 0 {
		return false
	}
	indices := make([]int, len(readings))
	for i, r := range readings {
		indices[i] = r.index
	}
	n.indices = indices
	return true
}

func (n *nvidiaGpuCollector) Describe() map[string]MetricMeta {
	meta := make(map[string]MetricMeta)
	for _, idx := range n.indices {
		for k, v := range gpuMetaFor(idx) {
			meta[k] = v
		}
	}
	return meta
}

func (n *nvidiaGpuCollector) Collect() (map[string]float64, error) {
	out, err := execCommand("nvidia-smi", "--query-gpu=index,utilization.gpu,memory.used,memory.total,temperature.gpu", "--format=csv,noheader,nounits")
	if err != nil {
		return nil, err
	}
	readings, err := parseNvidiaSmiCsv(out)
	if err != nil {
		return nil, err
	}
	values := make(map[string]float64)
	for _, r := range readings {
		values[fmt.Sprintf("gpu:%d:util", r.index)] = r.util
		values[fmt.Sprintf("gpu:%d:vram", r.index)] = r.vram
		values[fmt.Sprintf("gpu:%d:vramtotal", r.index)] = r.vramTotal
		values[fmt.Sprintf("gpu:%d:temp", r.index)] = r.temp
	}
	return values, nil
}

const (
	rocmKeyUtil      = "GPU use (%)"
	rocmKeyVramUsed  = "VRAM Total Used Memory (B)"
	rocmKeyVramTotal = "VRAM Total Memory (B)"
	rocmKeyTemp      = "Temperature (Sensor edge) (C)"
)

type amdGpuCollector struct {
	indices []int

	mu          sync.Mutex
	rocmSmiPath string
}

func MakeAmdGpuCollector() Collector {
	return &amdGpuCollector{}
}

func (a *amdGpuCollector) Name() string {
	return "gpu-amd"
}

// resolvedRocmSmiPath caches the discovered rocm-smi path on first success so
// later ticks don't re-glob /opt/rocm-*/bin on every Probe/Collect call.
func (a *amdGpuCollector) resolvedRocmSmiPath() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rocmSmiPath != "" {
		return a.rocmSmiPath, nil
	}
	path, err := resolveRocmSmiPath()
	if err != nil {
		return "", err
	}
	a.rocmSmiPath = path
	return path, nil
}

func parseRocmSmiJson(output []byte) ([]gpuReading, error) {
	var raw map[string]map[string]string
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("rocm-smi JSON parse failed: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("rocm-smi returned no cards")
	}
	readings := make([]gpuReading, 0, len(raw))
	for cardKey, fields := range raw {
		idxStr := strings.TrimPrefix(cardKey, "card")
		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			return nil, fmt.Errorf("unexpected rocm-smi card key %q: %w", cardKey, err)
		}
		util, err := strconv.ParseFloat(fields[rocmKeyUtil], 64)
		if err != nil {
			return nil, fmt.Errorf("bad util for %q: %w", cardKey, err)
		}
		vramBytes, err := strconv.ParseFloat(fields[rocmKeyVramUsed], 64)
		if err != nil {
			return nil, fmt.Errorf("bad vram for %q: %w", cardKey, err)
		}
		vramTotalBytes, err := strconv.ParseFloat(fields[rocmKeyVramTotal], 64)
		if err != nil {
			return nil, fmt.Errorf("bad vram total for %q: %w", cardKey, err)
		}
		temp, err := strconv.ParseFloat(fields[rocmKeyTemp], 64)
		if err != nil {
			return nil, fmt.Errorf("bad temp for %q: %w", cardKey, err)
		}
		readings = append(readings, gpuReading{
			index:     idx,
			util:      util,
			vram:      vramBytes / (1024 * 1024),
			vramTotal: vramTotalBytes / (1024 * 1024),
			temp:      temp,
		})
	}
	return readings, nil
}

func (a *amdGpuCollector) Probe() bool {
	path, err := a.resolvedRocmSmiPath()
	if err != nil {
		return false
	}
	out, err := execCommand(path, "--showuse", "--showmeminfo", "vram", "--showtemp", "--json")
	if err != nil {
		return false
	}
	readings, err := parseRocmSmiJson(out)
	if err != nil || len(readings) == 0 {
		return false
	}
	indices := make([]int, len(readings))
	for i, r := range readings {
		indices[i] = r.index
	}
	a.indices = indices
	return true
}

func (a *amdGpuCollector) Describe() map[string]MetricMeta {
	meta := make(map[string]MetricMeta)
	for _, idx := range a.indices {
		for k, v := range gpuMetaFor(idx) {
			meta[k] = v
		}
	}
	return meta
}

func (a *amdGpuCollector) Collect() (map[string]float64, error) {
	path, err := a.resolvedRocmSmiPath()
	if err != nil {
		return nil, err
	}
	out, err := execCommand(path, "--showuse", "--showmeminfo", "vram", "--showtemp", "--json")
	if err != nil {
		return nil, err
	}
	readings, err := parseRocmSmiJson(out)
	if err != nil {
		return nil, err
	}
	values := make(map[string]float64)
	for _, r := range readings {
		values[fmt.Sprintf("gpu:%d:util", r.index)] = r.util
		values[fmt.Sprintf("gpu:%d:vram", r.index)] = r.vram
		values[fmt.Sprintf("gpu:%d:vramtotal", r.index)] = r.vramTotal
		values[fmt.Sprintf("gpu:%d:temp", r.index)] = r.temp
	}
	return values, nil
}

type intelEngineJson struct {
	Engines map[string]struct {
		Busy float64 `json:"busy"`
	} `json:"engines"`
}

type intelGpuCollector struct {
	available bool
}

func MakeIntelGpuCollector() Collector {
	return &intelGpuCollector{}
}

func (i *intelGpuCollector) Name() string {
	return "gpu-intel"
}

func parseIntelGpuTopJson(output []byte) (float64, error) {
	var parsed intelEngineJson
	if err := json.Unmarshal(output, &parsed); err != nil {
		return 0, fmt.Errorf("intel_gpu_top JSON parse failed: %w", err)
	}
	engine, ok := parsed.Engines["Render/3D"]
	if !ok {
		return 0, errors.New("intel_gpu_top output missing Render/3D engine")
	}
	return engine.Busy, nil
}

func (i *intelGpuCollector) Probe() bool {
	out, err := execCommand("intel_gpu_top", "-J", "-s", "1000", "-o", "-")
	if err != nil {
		return false
	}
	if _, err := parseIntelGpuTopJson(out); err != nil {
		return false
	}
	i.available = true
	return true
}

func (i *intelGpuCollector) Describe() map[string]MetricMeta {
	if !i.available {
		return map[string]MetricMeta{}
	}
	return map[string]MetricMeta{
		"gpu:0:util": {Label: "GPU 0 %", Unit: "%", Color: gpuColorFor(0), MinY: 0, MaxY: 100, DecimalPlaces: 0},
	}
}

func (i *intelGpuCollector) Collect() (map[string]float64, error) {
	out, err := execCommand("intel_gpu_top", "-J", "-s", "1000", "-o", "-")
	if err != nil {
		return nil, err
	}
	busy, err := parseIntelGpuTopJson(out)
	if err != nil {
		return nil, err
	}
	return map[string]float64{"gpu:0:util": busy}, nil
}
