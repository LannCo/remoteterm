# GPU / Temperature Sysinfo Collectors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sysinfo widget's two hardcoded CPU/Mem collection functions with a pluggable collector registry, add GPU (NVIDIA/AMD/Intel) and CPU temperature metrics, and let the frontend multi-select any combination of whatever metrics are actually available on a given connection.

**Architecture:** Backend: a `Collector` interface (`Name`, `Probe`, `Describe`, `Collect`) with one implementation per metric family, registered once per `RunSysInfoLoop` connection. Fast tier (CPU/Mem) ticks every 1s; heavy tier (Temp/GPU) ticks on a per-connection configurable interval, republishing cached values between heavy ticks so the wire format (`TimeSeriesData{Ts, Values, Errors}`) stays a continuous 1s stream. Two new RPCs (`GetSysInfoMetricsCommand` for discovery, `SysInfoReprobeCommand` for manual re-detection) replace the frontend's hardcoded `PlotTypes`/`DefaultPlotMeta` maps. Frontend: `sysinfo:metrics` block-meta array replaces the old preset-name string; context menu becomes grouped checkboxes built from the discovery RPC.

**Tech Stack:** Go (`gopsutil/v4`, `os/exec` for vendor CLI shellouts), TypeScript/React/Jotai, existing `wshrpc` codegen pipeline (`task generate`).

**Spec:** `.pi/specs/2026-09-24-gpu-sysinfo-collectors-design.md`

## Global Constraints

- Go: no custom enum types, string constants only (`.kilocode/rules/rules.md`).
- Go: `Make`-prefixed constructors, not `New`; const decls at file top; `lock.Lock(); defer lock.Unlock()` pattern for any shared state.
- Never run `go build`; VSCode/gopls problems are the compile signal this repo trusts (this plan cannot self-verify Go compilation — flag any uncertain signature explicitly rather than assert it silently).
- TypeScript: `@/...` imports across packages, relative only within a directory; named exports only; 4-space indent; `== null`/`!= null` not `=== undefined`; jotai hooks only at component top level, never inline in JSX.
- Tailwind v4 for new UI; reuse existing tokens, never invent new inline styles/CSS vars without a reason.
- After any `wshrpctypes.go` change: run `task generate`, never hand-edit `frontend/types/gotypes.d.ts` or `frontend/app/store/wshclientapi.ts`.
- No live-reload-triggering action against the `remoteterm-daily` worktree's running `task dev` without asking first — this plan's tasks are Go/TS source edits, which the running dev server will pick up automatically; ask before committing a task if unsure whether a change is disruptive.
- JSON field naming: lowercase, no underscores, matching every existing `ConnKeywords`/`TimeSeriesData` field.

## Review Focus

- **Host with no supported GPU at all** (the common case — most connections). Every GPU collector's `Probe()` must return `false` cleanly, `Describe()`/`Collect()` must never be called for an unprobed-false collector, and the frontend menu must show zero GPU entries, not an empty/greyed placeholder group. Test: registry test with all three GPU collectors probing false, discovery RPC returns only cpu/mem/temp keys.
- **A single transient collector failure** (e.g. one `rocm-smi` call times out momentarily). Must not surface the error badge (only 3+ consecutive failures do) and must not crash the shared per-connection loop or stall other collectors' publication. Test: collector returns an error once, twice, succeeds on the third call — assert no `Errors` entry appears and the loop keeps running.
- **Multiple sysinfo widgets open on the same connection with different `sysinfo:metrics` selections.** They all read the same published `TimeSeriesData.Values`/`Errors` map (one shared event stream per connection, unchanged from today) — each widget must independently filter to its own selected subset without affecting what the others see. Test: two `SysinfoViewModel` instances with different `sysinfo:metrics` arrays reading the same fixture event, each renders only its own keys.
- **Heavy-tier metrics between polls** — a widget opened mid-cycle (heavy tier hasn't ticked yet for this loop instance) must not show a hole/NaN spike for GPU/temp series; the loop must publish a valid (even if slightly stale) heavy value on every fast tick once the first heavy collection has happened, and must not publish partial/zero heavy data before the first heavy tick completes. Test: loop test asserting tick 0 (before first heavy interval elapses) has no GPU/temp keys at all (collector hasn't run yet) rather than zeroes, and tick N (interval elapsed) has real values that persist unchanged through subsequent fast ticks until the next heavy tick.
- **Malformed/empty vendor CLI output** (driver present but tool prints nothing, or a version with a different CSV/JSON shape). Each parser must fail closed — treat as a normal per-tick collection failure (counts toward the 3-strike threshold), never panic, never silently emit garbage numbers (e.g. `0` parsed from an empty string). Test: each of the three GPU parsers gets an empty-string and a garbage-string fixture, both must return an error, not a zero-valued success.

---

## File Structure

**Backend (Go):**
- Create `pkg/wshrpc/wshremote/sysinfo_collectors.go` — `Collector` interface, `MetricMeta` type, `cpuCollector`, `memCollector`, `tempCollector`.
- Create `pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go` — `nvidiaGpuCollector`, `amdGpuCollector`, `intelGpuCollector`, and the shared per-vendor CLI-runner injection point used by tests.
- Modify `pkg/wshrpc/wshremote/sysinfo.go` — replace `getCpuData`/`getMemData`/`generateSingleServerData`/`RunSysInfoLoop` with the registry-driven, two-tier version; add failure-counter/`Errors` tracking.
- Modify `pkg/wshrpc/wshrpctypes.go` — add `Errors` field to `TimeSeriesData`; add `MetricMeta` type; add `SysInfoReprobeCommand`/`GetSysInfoMetricsCommand` to `WshRpcInterface` plus their request types.
- Modify `pkg/wshrpc/wshserver/wshserver.go` — implement the two new RPC handlers.
- Modify `pkg/rtconfig/settingsconfig.go` — add `SysInfoHeavyInterval` field to `ConnKeywords` (`json:"sysinfo:heavyinterval,omitempty"`).
- Create `pkg/wshrpc/wshremote/sysinfo_collectors_test.go`, `pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go`, `pkg/wshrpc/wshremote/sysinfo_test.go`.

**Frontend (TypeScript):**
- Modify `frontend/app/view/sysinfo/sysinfo.tsx` — remove `PlotTypes`/`DefaultPlotMeta`/`defaultCpuMeta`/`defaultMemMeta`/`plotTypeSelectedAtom`; add `sysinfo:metrics` array handling, discovery-RPC-driven `plotMetaAtom`, checkbox context menu, error badges, fixed `"Sysinfo"` title.
- Modify `frontend/app/theme.scss` — add `--sysinfo-temp-color`.
- Modify `frontend/app/view/remotetermconfig/connectionscontent.tsx` — clickable connection rows revealing an inline heavy-interval field.
- Create `frontend/app/view/sysinfo/sysinfo.test.tsx`.

---

### Task 1: Collector interface + CPU/Mem collectors (Go)

**Files:**
- Create: `pkg/wshrpc/wshremote/sysinfo_collectors.go`
- Create: `pkg/wshrpc/wshremote/sysinfo_collectors_test.go`
- Reference (not modified yet): `pkg/wshrpc/wshremote/sysinfo.go` (current `getCpuData`/`getMemData`, lines 21-47)

**Interfaces:**
- Produces: `type MetricMeta struct{...}`, `type Collector interface{ Name() string; Probe() bool; Describe() map[string]MetricMeta; Collect() (map[string]float64, error) }`, `MakeCpuCollector() Collector`, `MakeMemCollector() Collector`.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/wshrpc/wshremote/sysinfo_collectors_test.go
package wshremote

import "testing"

func TestCpuCollectorProbeAndDescribe(t *testing.T) {
	c := MakeCpuCollector()
	if !c.Probe() {
		t.Fatal("cpu collector must always probe true")
	}
	meta := c.Describe()
	if _, ok := meta["cpu"]; !ok {
		t.Fatal("expected 'cpu' key in cpu collector metadata")
	}
	if meta["cpu"].Label == "" || meta["cpu"].Unit != "%" {
		t.Fatalf("unexpected cpu meta: %+v", meta["cpu"])
	}
}

func TestCpuCollectorCollectReturnsAggregateAndPerCore(t *testing.T) {
	c := MakeCpuCollector()
	c.Probe()
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := values["cpu"]; !ok {
		t.Fatal("expected aggregate 'cpu' value")
	}
	found := false
	for k := range values {
		if k != "cpu" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected at least one per-core cpu:N value on a real host")
	}
}

func TestMemCollectorProbeAndCollect(t *testing.T) {
	c := MakeMemCollector()
	if !c.Probe() {
		t.Fatal("mem collector must always probe true")
	}
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"mem:total", "mem:used", "mem:free", "mem:available"} {
		if _, ok := values[key]; !ok {
			t.Fatalf("expected %q in mem collector output", key)
		}
	}
	meta := c.Describe()
	if meta["mem:used"].MaxYKey != "mem:total" {
		t.Fatalf("expected mem:used to reference mem:total as its dynamic max, got %+v", meta["mem:used"])
	}
}
```

- [ ] **Step 2: Confirm the tests fail to compile (no implementation exists yet)**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go vet ./pkg/wshrpc/wshremote/...`
Expected: compile errors referencing `MakeCpuCollector`/`MakeMemCollector` undefined.

- [ ] **Step 3: Write the collector interface and CPU/Mem implementations**

```go
// pkg/wshrpc/wshremote/sysinfo_collectors.go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"strconv"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

const BytesPerGB = 1073741824

type MetricMeta struct {
	Label         string  `json:"label"`
	Unit          string  `json:"unit"`
	Color         string  `json:"color"`
	MinY          float64 `json:"miny"`
	MaxY          float64 `json:"maxy"`
	MaxYKey       string  `json:"maxykey,omitempty"`
	DecimalPlaces int     `json:"decimalplaces"`
}

type Collector interface {
	Name() string
	Probe() bool
	Describe() map[string]MetricMeta
	Collect() (map[string]float64, error)
}

type cpuCollector struct {
	coreCount int
}

func MakeCpuCollector() Collector {
	return &cpuCollector{}
}

func (c *cpuCollector) Name() string {
	return "cpu"
}

func (c *cpuCollector) Probe() bool {
	percentArr, err := cpu.Percent(0, true)
	if err == nil {
		c.coreCount = len(percentArr)
	}
	return true
}

func (c *cpuCollector) Describe() map[string]MetricMeta {
	meta := make(map[string]MetricMeta)
	meta["cpu"] = MetricMeta{Label: "CPU %", Unit: "%", Color: "var(--sysinfo-cpu-color)", MinY: 0, MaxY: 100, DecimalPlaces: 0}
	for i := 0; i < c.coreCount; i++ {
		meta["cpu:"+strconv.Itoa(i)] = MetricMeta{
			Label: "Core " + strconv.Itoa(i), Unit: "%", Color: "var(--sysinfo-cpu-color)", MinY: 0, MaxY: 100, DecimalPlaces: 0,
		}
	}
	return meta
}

func (c *cpuCollector) Collect() (map[string]float64, error) {
	values := make(map[string]float64)
	percentArr, err := cpu.Percent(0, false)
	if err != nil {
		return nil, err
	}
	if len(percentArr) > 0 {
		values["cpu"] = percentArr[0]
	}
	percentArr, err = cpu.Percent(0, true)
	if err != nil {
		return nil, err
	}
	for idx, percent := range percentArr {
		values["cpu:"+strconv.Itoa(idx)] = percent
	}
	return values, nil
}

type memCollector struct{}

func MakeMemCollector() Collector {
	return &memCollector{}
}

func (m *memCollector) Name() string {
	return "mem"
}

func (m *memCollector) Probe() bool {
	return true
}

func (m *memCollector) Describe() map[string]MetricMeta {
	return map[string]MetricMeta{
		"mem:total":     {Label: "Memory Total", Unit: "GB", Color: "var(--sysinfo-mem-color)", MinY: 0, MaxY: 0, MaxYKey: "", DecimalPlaces: 1},
		"mem:used":      {Label: "Memory Used", Unit: "GB", Color: "var(--sysinfo-mem-color)", MinY: 0, MaxYKey: "mem:total", DecimalPlaces: 1},
		"mem:free":      {Label: "Memory Free", Unit: "GB", Color: "var(--sysinfo-mem-color)", MinY: 0, MaxYKey: "mem:total", DecimalPlaces: 1},
		"mem:available": {Label: "Memory Available", Unit: "GB", Color: "var(--sysinfo-mem-color)", MinY: 0, MaxYKey: "mem:total", DecimalPlaces: 1},
	}
}

func (m *memCollector) Collect() (map[string]float64, error) {
	memData, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	return map[string]float64{
		"mem:total":     float64(memData.Total) / BytesPerGB,
		"mem:available": float64(memData.Available) / BytesPerGB,
		"mem:used":      float64(memData.Used) / BytesPerGB,
		"mem:free":      float64(memData.Free) / BytesPerGB,
	}, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -run 'TestCpuCollector|TestMemCollector' -v`
Expected: PASS (this runs on the real host's real CPU/mem, no mocking needed here — behavior-preserving port of already-working code).

- [ ] **Step 5: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/wshrpc/wshremote/sysinfo_collectors.go pkg/wshrpc/wshremote/sysinfo_collectors_test.go
git commit -m "feat(sysinfo): add Collector interface, port CPU/Mem to it"
```

---

### Task 2: CPU temperature collector (Go)

**Files:**
- Modify: `pkg/wshrpc/wshremote/sysinfo_collectors.go` (remove the Task 1 placeholder line, add `tempCollector`)
- Modify: `pkg/wshrpc/wshremote/sysinfo_collectors_test.go`

**Interfaces:**
- Consumes: `Collector`, `MetricMeta` from Task 1.
- Produces: `MakeTempCollector() Collector`, emitting key `cpu:temp`.

- [ ] **Step 1: Write the failing test**

```go
func TestTempCollectorProbeIsHonest(t *testing.T) {
	c := MakeTempCollector()
	probed := c.Probe()
	values, err := c.Collect()
	if !probed {
		if err == nil && len(values) != 0 {
			t.Fatal("collector reported unavailable but still returned values")
		}
		t.Skip("no temperature sensors on this host/CI runner — Probe() correctly returned false")
	}
	if err != nil {
		t.Fatalf("probed true but Collect failed: %v", err)
	}
	if _, ok := values["cpu:temp"]; !ok {
		t.Fatal("expected cpu:temp key when probed true")
	}
}
```

- [ ] **Step 2: Confirm it fails to compile**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go vet ./pkg/wshrpc/wshremote/...`
Expected: `MakeTempCollector` undefined.

- [ ] **Step 3: Implement `tempCollector`**

Add to the bottom of `sysinfo_collectors.go`:

```go
type tempCollector struct {
	available bool
}

func MakeTempCollector() Collector {
	return &tempCollector{}
}

func (t *tempCollector) Name() string {
	return "temp"
}

// Probe reads sensors once to confirm at least one usable reading exists.
// Picking "the highest reading among sensor keys containing core/package,
// else the first entry" is a heuristic — sensor key naming varies by
// platform/driver (lm-sensors' coretemp module on Linux commonly exposes
// "coretemp_package_id_0"/"coretemp_core_0" style keys, but this is not
// guaranteed on every host) and there is no single correct "the" CPU
// temperature on multi-socket or heterogeneous-core hardware.
func (t *tempCollector) Probe() bool {
	stats, err := host.SensorsTemperatures()
	if err != nil || len(stats) == 0 {
		t.available = false
		return false
	}
	t.available = true
	return true
}

func (t *tempCollector) Describe() map[string]MetricMeta {
	if !t.available {
		return map[string]MetricMeta{}
	}
	return map[string]MetricMeta{
		"cpu:temp": {Label: "CPU Temp", Unit: "°C", Color: "var(--sysinfo-temp-color)", MinY: 0, MaxY: 100, DecimalPlaces: 1},
	}
}

func pickCpuTemperature(stats []host.TemperatureStat) (float64, bool) {
	best := 0.0
	found := false
	for _, s := range stats {
		key := strings.ToLower(s.SensorKey)
		if strings.Contains(key, "core") || strings.Contains(key, "package") {
			if !found || s.Temperature > best {
				best = s.Temperature
				found = true
			}
		}
	}
	if found {
		return best, true
	}
	return stats[0].Temperature, true
}

func (t *tempCollector) Collect() (map[string]float64, error) {
	stats, err := host.SensorsTemperatures()
	if err != nil {
		return nil, err
	}
	if len(stats) == 0 {
		return nil, errors.New("no temperature sensors reported")
	}
	temp, _ := pickCpuTemperature(stats)
	return map[string]float64{"cpu:temp": temp}, nil
}
```

Add `"errors"`, `"strings"`, and `"github.com/shirou/gopsutil/v4/host"` to the file's import block alongside the existing `"strconv"`/`"github.com/shirou/gopsutil/v4/cpu"`/`"github.com/shirou/gopsutil/v4/mem"`.

- [ ] **Step 4: Run the tests**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -run TestTempCollector -v`
Expected: PASS or SKIP (skip is a legitimate outcome on a CI runner/VM with no exposed sensors — the test itself asserts `Probe()` is honest about that, not that sensors always exist).

- [ ] **Step 5: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/wshrpc/wshremote/sysinfo_collectors.go pkg/wshrpc/wshremote/sysinfo_collectors_test.go
git commit -m "feat(sysinfo): add CPU temperature collector"
```

---

### Task 3: NVIDIA GPU collector (Go)

**Files:**
- Create: `pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go`
- Create: `pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go`

**Interfaces:**
- Consumes: `Collector`, `MetricMeta` from Task 1.
- Produces: `MakeNvidiaGpuCollector() Collector`, a package-level `execCommand` var (function type `func(name string, args ...string) ([]byte, error)`) that all three GPU collectors in this file share and that tests override — this is the seam that lets tests inject canned CLI output instead of shelling out for real.

This collector uses `nvidia-smi --query-gpu=index,utilization.gpu,memory.used,memory.total,temperature.gpu --format=csv,noheader,nounits`, a stable, documented nvidia-smi flag combination that emits one plain CSV row per GPU with no header/units to strip, e.g.:
```
0, 45, 2048, 8192, 62
1, 12, 512, 8192, 51
```

- [ ] **Step 1: Write the failing tests**

```go
// pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go
package wshremote

import (
	"errors"
	"testing"
)

func withFakeExec(t *testing.T, fn func(name string, args ...string) ([]byte, error)) {
	t.Helper()
	orig := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = orig })
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
		"gpu:0:util": 45, "gpu:0:vram": 2048, "gpu:0:temp": 62,
		"gpu:1:util": 12, "gpu:1:vram": 512, "gpu:1:temp": 51,
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
```

- [ ] **Step 2: Confirm the tests fail to compile**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go vet ./pkg/wshrpc/wshremote/...`
Expected: `execCommand`/`MakeNvidiaGpuCollector` undefined.

- [ ] **Step 3: Implement the shared exec seam and the NVIDIA collector**

```go
// pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// execCommand is a package-level seam so GPU collector tests can inject
// canned CLI output instead of shelling out to real vendor tools — none of
// the three vendors' tools can be assumed present on any given dev/CI host.
var execCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

type gpuReading struct {
	index int
	util  float64
	vram  float64
	temp  float64
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
		temp, err := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		if err != nil {
			return nil, fmt.Errorf("bad temp in row %q: %w", line, err)
		}
		readings = append(readings, gpuReading{index: idx, util: util, vram: vram, temp: temp})
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
		values[fmt.Sprintf("gpu:%d:temp", r.index)] = r.temp
	}
	return values, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -run TestNvidia -v`
Expected: PASS (all four tests use the fake `execCommand`, no real NVIDIA hardware needed — this machine is AMD-only, confirming these tests must not depend on real hardware).

- [ ] **Step 5: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go
git commit -m "feat(sysinfo): add NVIDIA GPU collector"
```

---

### Task 4: AMD GPU collector (Go)

**Files:**
- Modify: `pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go`
- Modify: `pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go`

**Interfaces:**
- Consumes: `execCommand`, `gpuReading`, `gpuMetaFor` from Task 3.
- Produces: `MakeAmdGpuCollector() Collector`.

**Verification caveat, read before implementing:** `rocm-smi`'s JSON field names have changed across ROCm releases. The parser below targets a commonly-seen shape (`rocm-smi --showuse --showmeminfo vram --showtemp --json`, keys like `"GPU use (%)"`, `"VRAM Total Used Memory (B)"`, `"Temperature (Sensor edge) (C)"` under a `"cardN"` object per GPU) but this has NOT been confirmed against a real installed `rocm-smi` on this session's own AMD 7900 XTX machine. Before trusting this in production, run `rocm-smi --showuse --showmeminfo vram --showtemp --json` for real on that machine (the owner's own desktop, so this is directly testable) and diff the actual keys against what the parser expects; adjust the field-name constants at the top of the AMD section if they differ. This is a real verification step, not busywork — do it as part of this task, not a "later."

- [ ] **Step 1: Write the failing tests**

```go
func TestAmdCollectorParsesRocmSmiJson(t *testing.T) {
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
	if values["gpu:0:temp"] != 62.0 {
		t.Errorf("gpu:0:temp = %v, want 62.0", values["gpu:0:temp"])
	}
}

func TestAmdCollectorProbeFalseOnMissingCommand(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("exec: \"rocm-smi\": executable file not found in $PATH")
	})
	c := MakeAmdGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false when rocm-smi is not installed")
	}
}

func TestAmdCollectorRejectsMalformedJson(t *testing.T) {
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
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(""), nil
	})
	c := MakeAmdGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false on empty output")
	}
}
```

- [ ] **Step 2: Confirm it fails to compile**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go vet ./pkg/wshrpc/wshremote/...`
Expected: `MakeAmdGpuCollector` undefined.

- [ ] **Step 3: Implement the AMD collector**

```go
import "encoding/json"
// (add to existing import block in sysinfo_collectors_gpu.go)

const (
	rocmKeyUtil = "GPU use (%)"
	rocmKeyVramUsed = "VRAM Total Used Memory (B)"
	rocmKeyVramTotal = "VRAM Total Memory (B)"
	rocmKeyTemp = "Temperature (Sensor edge) (C)"
)

type amdGpuCollector struct {
	indices []int
}

func MakeAmdGpuCollector() Collector {
	return &amdGpuCollector{}
}

func (a *amdGpuCollector) Name() string {
	return "gpu-amd"
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
		temp, err := strconv.ParseFloat(fields[rocmKeyTemp], 64)
		if err != nil {
			return nil, fmt.Errorf("bad temp for %q: %w", cardKey, err)
		}
		readings = append(readings, gpuReading{index: idx, util: util, vram: vramBytes / (1024 * 1024), temp: temp})
	}
	return readings, nil
}

func (a *amdGpuCollector) Probe() bool {
	out, err := execCommand("rocm-smi", "--showuse", "--showmeminfo", "vram", "--showtemp", "--json")
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
	out, err := execCommand("rocm-smi", "--showuse", "--showmeminfo", "vram", "--showtemp", "--json")
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
		values[fmt.Sprintf("gpu:%d:temp", r.index)] = r.temp
	}
	return values, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -run TestAmd -v`
Expected: PASS against the fake exec fixtures.

- [ ] **Step 5: Verify against real rocm-smi on this machine (the owner's own AMD 7900 XTX box)**

Run: `rocm-smi --showuse --showmeminfo vram --showtemp --json`
Compare the real output's key names against `rocmKeyUtil`/`rocmKeyVramUsed`/`rocmKeyVramTotal`/`rocmKeyTemp` above. If they differ, update the constants and re-run Step 4's tests (the fixtures in the test file should also be updated to match the real shape, so the test suite stays honest about what it's actually verifying).

- [ ] **Step 6: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go
git commit -m "feat(sysinfo): add AMD GPU collector, verified against real rocm-smi"
```

---

### Task 5: Intel GPU collector (Go) — utilization only, documented limitation

**Files:**
- Modify: `pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go`
- Modify: `pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go`

**Known limitation, by design:** unlike `nvidia-smi`/`rocm-smi`, Intel's `intel_gpu_top` does not reliably expose VRAM or temperature in a stable, documented way across Intel GPU generations (integrated vs Arc discrete) — those are commonly read from different sysfs paths that vary by driver (i915 vs Xe) and aren't in scope here. This collector emits **`gpu:N:util` only**; `gpu:N:vram`/`gpu:N:temp` are simply absent from its `Describe()`/`Collect()` output for Intel GPUs, which the existing sparse-per-collector-output design already handles cleanly (the frontend menu only shows keys a collector actually described).

**Verification caveat, same as Task 4:** `intel_gpu_top`'s JSON output shape (`-J` flag) is not independently confirmed against a real run in this plan — there is no Intel GPU on this session's own dev machine to test against. The parser targets the commonly-documented top-level `"engines"` object with per-engine `"busy"` percentage under an entry resembling `"Render/3D"`. **Before trusting this collector, run `intel_gpu_top -J -s 1000 -o -` for 1-2 seconds on a real Intel-GPU host (or find current upstream documentation/source for the exact schema) and adjust the field path if it differs** — this is flagged rather than guessed with false confidence.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Confirm it fails to compile**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go vet ./pkg/wshrpc/wshremote/...`
Expected: `MakeIntelGpuCollector` undefined.

- [ ] **Step 3: Implement the Intel collector**

```go
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
```

Note: this collector assumes a single Intel GPU (`gpu:0` only) — `intel_gpu_top`'s single-engine-summary JSON shape doesn't cleanly enumerate multiple discrete Intel GPUs the way `nvidia-smi`'s per-index CSV does. If multi-Intel-GPU support is ever needed, that's a real follow-up, not silently assumed solved here.

- [ ] **Step 4: Run the tests**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -run TestIntel -v`
Expected: PASS against fake exec fixtures.

- [ ] **Step 5: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/wshrpc/wshremote/sysinfo_collectors_gpu.go pkg/wshrpc/wshremote/sysinfo_collectors_gpu_test.go
git commit -m "feat(sysinfo): add Intel GPU collector (utilization only, documented limitation)"
```

---

### Task 6: Registry + two-tier loop + failure tracking (Go)

**Files:**
- Modify: `pkg/wshrpc/wshremote/sysinfo.go` (replace lines 21-72 entirely)
- Create: `pkg/wshrpc/wshremote/sysinfo_test.go`

**Interfaces:**
- Consumes: `Collector` interface and all `Make*Collector()` constructors from Tasks 1-5; `wshrpc.TimeSeriesData` (Task 7 adds `Errors` field — this task's code references it, so Task 6 and Task 7 must land together or Task 6's code temporarily won't compile; sequence them adjacently).
- Produces: `MakeCollectorRegistry() []Collector`, `RunSysInfoLoop(client, connName)` (signature unchanged from today, callers in `cmd/wsh/cmd/wshcmd-connserver.go` need no changes), `ReprobeCollectors(connName string) error` (package-level, keyed by connName, used by Task 7's `SysInfoReprobeCommand` handler).

- [ ] **Step 1: Write the failing tests**

```go
// pkg/wshrpc/wshremote/sysinfo_test.go
package wshremote

import "testing"

type fakeCollector struct {
	name       string
	probeCalls int
	probeOk    bool
	collectFn  func() (map[string]float64, error)
	meta       map[string]MetricMeta
}

func (f *fakeCollector) Name() string { return f.name }
func (f *fakeCollector) Probe() bool  { f.probeCalls++; return f.probeOk }
func (f *fakeCollector) Describe() map[string]MetricMeta { return f.meta }
func (f *fakeCollector) Collect() (map[string]float64, error) { return f.collectFn() }

func TestRunLoopIterationSkipsUnprobedCollectors(t *testing.T) {
	unavailable := &fakeCollector{name: "gpu-nvidia", probeOk: false}
	available := &fakeCollector{name: "cpu", probeOk: true, collectFn: func() (map[string]float64, error) {
		return map[string]float64{"cpu": 10}, nil
	}}
	collectors := []Collector{unavailable, available}
	active := probeAll(collectors)
	if len(active) != 1 || active[0].Name() != "cpu" {
		t.Fatalf("expected only the probed-true collector to remain active, got %v", collectorNames(active))
	}
}

func TestFailureCounterTripsErrorsAfterThreeStrikes(t *testing.T) {
	callCount := 0
	flaky := &fakeCollector{name: "gpu-amd", probeOk: true, collectFn: func() (map[string]float64, error) {
		callCount++
		return nil, fmt.Errorf("transient failure %d", callCount)
	}}
	tracker := makeFailureTracker()
	for i := 0; i < 2; i++ {
		values, errMsg := tracker.record(flaky)
		if len(values) != 0 {
			t.Fatalf("expected no values on failed collect, tick %d", i)
		}
		if errMsg != "" {
			t.Fatalf("expected no Errors entry before 3 strikes, got %q on tick %d", errMsg, i)
		}
	}
	_, errMsg := tracker.record(flaky)
	if errMsg == "" {
		t.Fatal("expected an Errors entry on the 3rd consecutive failure")
	}
}

func TestFailureCounterClearsOnRecovery(t *testing.T) {
	attempt := 0
	recovering := &fakeCollector{name: "gpu-amd", probeOk: true, collectFn: func() (map[string]float64, error) {
		attempt++
		if attempt <= 3 {
			return nil, fmt.Errorf("fail %d", attempt)
		}
		return map[string]float64{"gpu:0:util": 5}, nil
	}}
	tracker := makeFailureTracker()
	for i := 0; i < 3; i++ {
		tracker.record(recovering)
	}
	values, errMsg := tracker.record(recovering)
	if errMsg != "" {
		t.Fatalf("expected error to clear on recovery, got %q", errMsg)
	}
	if values["gpu:0:util"] != 5 {
		t.Fatalf("expected recovered value, got %v", values)
	}
}

func collectorNames(cs []Collector) []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name()
	}
	return names
}
```

Add `"fmt"` to the test file's imports.

- [ ] **Step 2: Confirm it fails to compile**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go vet ./pkg/wshrpc/wshremote/...`
Expected: `probeAll`/`makeFailureTracker` undefined.

- [ ] **Step 3: Rewrite `sysinfo.go`**

```go
// pkg/wshrpc/wshremote/sysinfo.go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"log"
	"time"

	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshclient"
	"github.com/LannCo/remoteterm/pkg/wshutil"
)

const DefaultHeavyIntervalSeconds = 5

func MakeCollectorRegistry() []Collector {
	return []Collector{
		MakeCpuCollector(),
		MakeMemCollector(),
		MakeTempCollector(),
		MakeNvidiaGpuCollector(),
		MakeAmdGpuCollector(),
		MakeIntelGpuCollector(),
	}
}

// heavyCollectorNames are ticked on the slower, configurable interval;
// everything else (cpu, mem) ticks every second, matching today's behavior.
var heavyCollectorNames = map[string]bool{
	"temp":       true,
	"gpu-nvidia": true,
	"gpu-amd":    true,
	"gpu-intel":  true,
}

func probeAll(collectors []Collector) []Collector {
	active := make([]Collector, 0, len(collectors))
	for _, c := range collectors {
		if c.Probe() {
			active = append(active, c)
		}
	}
	return active
}

type failureTracker struct {
	counts map[string]int
}

func makeFailureTracker() *failureTracker {
	return &failureTracker{counts: make(map[string]int)}
}

// record runs the collector, returning its values on success. On failure it
// increments that collector's consecutive-failure count and only returns a
// non-empty errMsg once the count reaches 3 — a single transient failure
// (the common case) never surfaces anything. A success resets the count and
// clears any previously-surfaced error.
func (ft *failureTracker) record(c Collector) (values map[string]float64, errMsg string) {
	values, err := c.Collect()
	if err == nil {
		ft.counts[c.Name()] = 0
		return values, ""
	}
	ft.counts[c.Name()]++
	if ft.counts[c.Name()] >= 3 {
		return map[string]float64{}, err.Error()
	}
	return map[string]float64{}, ""
}

func getHeavyIntervalSeconds(connName string) int {
	config := rtconfig.GetWatcher().GetFullConfig()
	connSettings, ok := config.Connections[connName]
	if !ok || connSettings.SysInfoHeavyInterval <= 0 {
		return DefaultHeavyIntervalSeconds
	}
	return connSettings.SysInfoHeavyInterval
}

type sysInfoLoopState struct {
	connName        string
	client          *wshutil.WshRpc
	fastCollectors  []Collector
	heavyCollectors []Collector
	tracker         *failureTracker
	lastHeavyValues map[string]float64
	lastHeavyErrors map[string]string
	heavyEverRan    bool
	heavyInterval   int
	tickCount       int
}

var activeLoopStates = make(map[string]*sysInfoLoopState)

func ReprobeCollectors(connName string) error {
	state, ok := activeLoopStates[connName]
	if !ok {
		return nil
	}
	all := append(append([]Collector{}, state.fastCollectors...), state.heavyCollectors...)
	registry := MakeCollectorRegistry()
	active := probeAll(registry)
	fast := make([]Collector, 0)
	heavy := make([]Collector, 0)
	for _, c := range active {
		if heavyCollectorNames[c.Name()] {
			heavy = append(heavy, c)
		} else {
			fast = append(fast, c)
		}
	}
	state.fastCollectors = fast
	state.heavyCollectors = heavy
	state.heavyInterval = getHeavyIntervalSeconds(connName)
	_ = all
	return nil
}

func generateSingleServerData(state *sysInfoLoopState) {
	now := time.Now()
	values := make(map[string]float64)
	errorsMap := make(map[string]string)

	for _, c := range state.fastCollectors {
		v, errMsg := state.tracker.record(c)
		for k, val := range v {
			values[k] = val
		}
		if errMsg != "" {
			errorsMap[c.Name()] = errMsg
		}
	}

	runHeavyThisTick := state.heavyInterval <= 1 || state.tickCount%state.heavyInterval == 0
	if runHeavyThisTick && len(state.heavyCollectors) > 0 {
		heavyValues := make(map[string]float64)
		heavyErrors := make(map[string]string)
		for _, c := range state.heavyCollectors {
			v, errMsg := state.tracker.record(c)
			for k, val := range v {
				heavyValues[k] = val
			}
			if errMsg != "" {
				heavyErrors[c.Name()] = errMsg
			}
		}
		state.lastHeavyValues = heavyValues
		state.lastHeavyErrors = heavyErrors
		state.heavyEverRan = true
	}
	if state.heavyEverRan {
		for k, v := range state.lastHeavyValues {
			values[k] = v
		}
		for k, v := range state.lastHeavyErrors {
			errorsMap[k] = v
		}
	}

	state.tickCount++

	tsData := wshrpc.TimeSeriesData{Ts: now.UnixMilli(), Values: values, Errors: errorsMap}
	event := wps.WaveEvent{
		Event:   wps.Event_SysInfo,
		Scopes:  []string{state.connName},
		Data:    tsData,
		Persist: 1024,
	}
	wshclient.EventPublishCommand(state.client, event, &wshrpc.RpcOpts{NoResponse: true})
}

func RunSysInfoLoop(client *wshutil.WshRpc, connName string) {
	defer func() {
		log.Printf("sysinfo loop ended conn:%s\n", connName)
		delete(activeLoopStates, connName)
	}()
	registry := MakeCollectorRegistry()
	active := probeAll(registry)
	fast := make([]Collector, 0)
	heavy := make([]Collector, 0)
	for _, c := range active {
		if heavyCollectorNames[c.Name()] {
			heavy = append(heavy, c)
		} else {
			fast = append(fast, c)
		}
	}
	state := &sysInfoLoopState{
		connName:        connName,
		client:          client,
		fastCollectors:  fast,
		heavyCollectors: heavy,
		tracker:         makeFailureTracker(),
		heavyInterval:   getHeavyIntervalSeconds(connName),
	}
	activeLoopStates[connName] = state
	for {
		generateSingleServerData(state)
		time.Sleep(1 * time.Second)
	}
}
```

Note on `ReprobeCollectors`: this rebuilds and re-probes the full registry, replacing the loop's active collector sets in place on the shared `activeLoopStates` map entry — the running `for` loop in `RunSysInfoLoop` reads `state.fastCollectors`/`state.heavyCollectors` fresh each tick via the shared `*sysInfoLoopState` pointer, so a reprobe takes effect on the very next tick without restarting the loop or losing `state.lastHeavyValues`'s continuity for collectors that remain active. This map is only ever mutated from the single goroutine running each connection's loop plus RPC-handler goroutines calling `ReprobeCollectors` — Task 7 must wrap `activeLoopStates` access in a mutex (`sync.Mutex`) since RPC handlers run on different goroutines than the loop; this plan's Task 6 code above is intentionally incomplete on that point and Task 7 adds the lock, per this repo's `lock.Lock(); defer lock.Unlock()` convention — do not skip it.

- [ ] **Step 4: Run the tests**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -run 'TestRunLoop|TestFailureCounter' -v`
Expected: FAIL to compile at this step specifically because `wshrpc.TimeSeriesData` doesn't have an `Errors` field yet (Task 7 adds it) — this is expected and acceptable; note it in the commit message and proceed directly to Task 7 without a separate "fix" commit, since Tasks 6 and 7 are sequenced adjacently for exactly this reason.

- [ ] **Step 5: Commit (compile-broken intentionally, fixed by Task 7 immediately after)**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/wshrpc/wshremote/sysinfo.go pkg/wshrpc/wshremote/sysinfo_test.go
git commit -m "feat(sysinfo): registry-driven two-tier loop with failure tracking

Depends on Task 7's TimeSeriesData.Errors field to compile; sequenced
adjacently, not independently mergeable."
```

---

### Task 7: RPC schema — `Errors` field, discovery + reprobe commands (Go)

**Files:**
- Modify: `pkg/wshrpc/wshrpctypes.go`
- Modify: `pkg/wshrpc/wshserver/wshserver.go`
- Modify: `pkg/wshrpc/wshremote/sysinfo.go` (add the mutex noted in Task 6)

**Interfaces:**
- Produces: `wshrpc.TimeSeriesData.Errors map[string]string`, `wshrpc.MetricMeta` (re-exported/aliased from `wshremote.MetricMeta` or duplicated with matching JSON tags — see Step 3 for which), `SysInfoReprobeCommand(ctx, CommandSysInfoReprobeData) error`, `GetSysInfoMetricsCommand(ctx, CommandSysInfoMetricsData) (map[string]MetricMeta, error)`.

**Design note on the type duplication:** `wshrpc` cannot import `wshremote` (the RPC types package is lower-level than the collector implementation package — importing `wshremote`'s `MetricMeta` from `wshrpctypes.go` would very likely create an import cycle, since `wshremote` already imports `wshrpc` for `wshrpc.TimeSeriesData`/`wshrpc.RpcOpts`). Define `MetricMeta` directly in `wshrpctypes.go` with identical fields/JSON tags to `wshremote.MetricMeta` from Task 1, and have the `GetSysInfoMetricsCommand` handler convert `wshremote.MetricMeta` values into `wshrpc.MetricMeta` values field-by-field. This is a small, deliberate duplication in service of the existing package boundary, not an oversight — flag it with a comment in both places.

- [ ] **Step 1: Add `Errors` to `TimeSeriesData` and the new types/interface methods**

In `pkg/wshrpc/wshrpctypes.go`, modify the existing struct (around line 394):

```go
type TimeSeriesData struct {
	Ts     int64              `json:"ts"`
	Values map[string]float64 `json:"values"`
	Errors map[string]string  `json:"errors,omitempty"`
}
```

Add near it:

```go
// MetricMeta mirrors wshremote.MetricMeta field-for-field. Duplicated
// rather than imported to avoid an import cycle (wshremote already
// imports this package for TimeSeriesData/RpcOpts) — keep both structs'
// fields and json tags in sync by hand if either changes.
type MetricMeta struct {
	Label         string  `json:"label"`
	Unit          string  `json:"unit"`
	Color         string  `json:"color"`
	MinY          float64 `json:"miny"`
	MaxY          float64 `json:"maxy"`
	MaxYKey       string  `json:"maxykey,omitempty"`
	DecimalPlaces int     `json:"decimalplaces"`
}

type CommandSysInfoReprobeData struct {
	ConnName string `json:"connname"`
}

type CommandSysInfoMetricsData struct {
	ConnName string `json:"connname"`
}
```

In the `WshRpcInterface` block, add near `StreamCpuDataCommand` (around line 71):

```go
	SysInfoReprobeCommand(ctx context.Context, data CommandSysInfoReprobeData) error
	GetSysInfoMetricsCommand(ctx context.Context, data CommandSysInfoMetricsData) (map[string]MetricMeta, error)
```

- [ ] **Step 2: Regenerate bindings**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && task generate`
Expected: `frontend/types/gotypes.d.ts` and `frontend/app/store/wshclientapi.ts` update to include `TimeSeriesData.errors`, `MetricMeta`, `SysInfoReprobeCommand`, `GetSysInfoMetricsCommand`. Do not hand-edit either generated file — if `task generate` fails, fix the Go source and re-run, never patch the generated output directly.

- [ ] **Step 3: Implement the handlers in `wshserver.go`**

```go
// pkg/wshrpc/wshserver/wshserver.go — add near the existing
// StreamCpuDataCommand implementation

func (ws *WshServer) SysInfoReprobeCommand(ctx context.Context, data wshrpc.CommandSysInfoReprobeData) error {
	return wshremote.ReprobeCollectors(data.ConnName)
}

func (ws *WshServer) GetSysInfoMetricsCommand(ctx context.Context, data wshrpc.CommandSysInfoMetricsData) (map[string]wshrpc.MetricMeta, error) {
	descriptions := wshremote.DescribeActiveCollectors(data.ConnName)
	converted := make(map[string]wshrpc.MetricMeta, len(descriptions))
	for k, v := range descriptions {
		converted[k] = wshrpc.MetricMeta{
			Label: v.Label, Unit: v.Unit, Color: v.Color,
			MinY: v.MinY, MaxY: v.MaxY, MaxYKey: v.MaxYKey, DecimalPlaces: v.DecimalPlaces,
		}
	}
	return converted, nil
}
```

This calls a new `wshremote.DescribeActiveCollectors(connName string) map[string]wshremote.MetricMeta` — add it to `pkg/wshrpc/wshremote/sysinfo.go`:

```go
func DescribeActiveCollectors(connName string) map[string]MetricMeta {
	activeLoopStateMu.Lock()
	state, ok := activeLoopStates[connName]
	activeLoopStateMu.Unlock()
	if !ok {
		return map[string]MetricMeta{}
	}
	meta := make(map[string]MetricMeta)
	for _, c := range append(append([]Collector{}, state.fastCollectors...), state.heavyCollectors...) {
		for k, v := range c.Describe() {
			meta[k] = v
		}
	}
	return meta
}
```

Also add the mutex flagged as missing in Task 6 — modify `sysinfo.go`:

```go
var activeLoopStateMu sync.Mutex
var activeLoopStates = make(map[string]*sysInfoLoopState)
```

And wrap every existing read/write of `activeLoopStates` (in `RunSysInfoLoop`'s setup/defer and in `ReprobeCollectors`) with `activeLoopStateMu.Lock(); defer activeLoopStateMu.Unlock()` per this repo's synchronization convention — do not leave any inline lock/unlock pair, use the defer pattern throughout. Add `"sync"` to the file's imports.

- [ ] **Step 4: Write and run a test for the discovery/reprobe path**

```go
// pkg/wshrpc/wshremote/sysinfo_test.go — append

func TestDescribeActiveCollectorsEmptyForUnknownConn(t *testing.T) {
	meta := DescribeActiveCollectors("no-such-connection")
	if len(meta) != 0 {
		t.Fatalf("expected empty metadata for an unregistered connection, got %v", meta)
	}
}

func TestReprobeCollectorsNoOpForUnknownConn(t *testing.T) {
	if err := ReprobeCollectors("no-such-connection"); err != nil {
		t.Fatalf("expected nil error for an unregistered connection, got %v", err)
	}
}
```

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -v`
Expected: full package PASS, including Task 6's tests that were failing to compile before this task's `Errors` field landed.

- [ ] **Step 5: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/wshrpc/wshrpctypes.go pkg/wshrpc/wshserver/wshserver.go pkg/wshrpc/wshremote/sysinfo.go pkg/wshrpc/wshremote/sysinfo_test.go frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts
git commit -m "feat(sysinfo): Errors field, discovery + reprobe RPCs, mutex-guarded loop state"
```

---

### Task 8: Per-connection heavy interval config (Go)

**Files:**
- Modify: `pkg/rtconfig/settingsconfig.go`

**Interfaces:**
- Consumes: `ConnKeywords` struct (line 228 today).
- Produces: `ConnKeywords.SysInfoHeavyInterval int`, consumed by `getHeavyIntervalSeconds` (already written in Task 6).

- [ ] **Step 1: Add the field**

In `pkg/rtconfig/settingsconfig.go`, add to the `ConnKeywords` struct (near the other `Conn*` fields, around line 228):

```go
	SysInfoHeavyInterval int `json:"sysinfo:heavyinterval,omitempty"`
```

No test file exists yet for `settingsconfig.go`'s struct shape (it's a plain data struct, no behavior to unit test in isolation) — Task 6's `getHeavyIntervalSeconds` tests (added below) are what actually exercise this field's default-fallback behavior.

- [ ] **Step 2: Write a test for the default-fallback behavior this field enables**

```go
// pkg/wshrpc/wshremote/sysinfo_test.go — append

func TestGetHeavyIntervalSecondsDefaultsWhenUnset(t *testing.T) {
	got := getHeavyIntervalSeconds("connection-with-no-config-entry")
	if got != DefaultHeavyIntervalSeconds {
		t.Fatalf("expected default %d for an unconfigured connection, got %d", DefaultHeavyIntervalSeconds, got)
	}
}
```

This test relies on `rtconfig.GetWatcher().GetFullConfig()` returning a real (likely empty-for-this-connection) config in the test environment — if `rtconfig.GetWatcher()` isn't initialized in a bare `go test` run and panics, wrap `getHeavyIntervalSeconds` with a nil-watcher guard returning the default, and note that guard explicitly in the function's comment as handling the test-environment case, not a production code path.

- [ ] **Step 3: Run the test**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && go test ./pkg/wshrpc/wshremote/... -run TestGetHeavyIntervalSeconds -v`
Expected: PASS. If it panics on a nil watcher, apply the guard from Step 2's note and re-run.

- [ ] **Step 4: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add pkg/rtconfig/settingsconfig.go pkg/wshrpc/wshremote/sysinfo.go pkg/wshrpc/wshremote/sysinfo_test.go
git commit -m "feat(sysinfo): sysinfo:heavyinterval ConnKeywords field"
```

---

### Task 9: Frontend — multi-select sysinfo.tsx rewrite

**Files:**
- Modify: `frontend/app/view/sysinfo/sysinfo.tsx` (full rewrite of lines 32-328; `SingleLinePlot`/`SysinfoViewInner`/`SysinfoView` component bodies from line 330 onward stay structurally similar, adjusted for the new atoms)
- Modify: `frontend/app/theme.scss` (add `--sysinfo-temp-color` near line 116)
- Create: `frontend/app/view/sysinfo/sysinfo.test.tsx`

**Interfaces:**
- Consumes: `GetSysInfoMetricsCommand`, `SysInfoReprobeCommand` (Task 7, generated in `wshclientapi.ts`), `TimeSeriesData.errors` (Task 7).
- Produces: `SysinfoViewModel` with `metricsAtom` (replaces `plotTypeSelectedAtom`+old `metrics`), `plotMetaAtom` now populated from the discovery RPC instead of the static `DefaultPlotMeta`, `errorBadgesAtom`.

- [ ] **Step 1: Add the color token**

In `frontend/app/theme.scss`, next to the existing lines 115-116:

```scss
    --sysinfo-temp-color: #ff7043;
```

- [ ] **Step 2: Write the failing frontend tests**

```tsx
// frontend/app/view/sysinfo/sysinfo.test.tsx
import { describe, expect, it, vi } from "vitest";
import { getGpuColor, buildMetricsMenu } from "@/app/view/sysinfo/sysinfo";

describe("getGpuColor", () => {
    it("returns a distinct color per GPU index, cycling through the palette", () => {
        const c0 = getGpuColor(0);
        const c1 = getGpuColor(1);
        expect(c0).not.toEqual(c1);
        expect(getGpuColor(0)).toEqual(c0); // stable for the same index
    });
});

describe("buildMetricsMenu", () => {
    const fixtureMeta: Record<string, TimeSeriesMeta> = {
        cpu: { name: "CPU %", label: "%", color: "var(--sysinfo-cpu-color)" },
        "mem:used": { name: "Memory Used", label: "GB", color: "var(--sysinfo-mem-color)" },
        "gpu:0:util": { name: "GPU 0 %", label: "%", color: "#000" },
        "gpu:1:util": { name: "GPU 1 %", label: "%", color: "#000" },
    };

    it("groups GPU keys by index into their own submenu, leaves cpu/mem flat", () => {
        const menu = buildMetricsMenu(fixtureMeta, ["cpu"], vi.fn());
        const topLevelLabels = menu.map((item) => item.label);
        expect(topLevelLabels).toContain("cpu");
        expect(topLevelLabels).toContain("mem:used");
        const gpuGroups = menu.filter((item) => item.submenu);
        expect(gpuGroups.length).toBe(2); // GPU 0, GPU 1
    });

    it("marks currently-selected metrics as checked", () => {
        const menu = buildMetricsMenu(fixtureMeta, ["cpu", "mem:used"], vi.fn());
        const cpuItem = menu.find((item) => item.label === "cpu");
        const memItem = menu.find((item) => item.label === "mem:used");
        expect(cpuItem?.checked).toBe(true);
        expect(memItem?.checked).toBe(true);
    });

    it("produces an empty menu when no metrics are available (no GPU, no sensors)", () => {
        const menu = buildMetricsMenu({}, [], vi.fn());
        expect(menu.length).toBe(0);
    });

    it("computes independent checked-state for two different selections against the same discovery data", () => {
        // Regression guard for multiple sysinfo widgets on one connection: both read the
        // same discovery-RPC metadata (fixtureMeta) but each has its own sysinfo:metrics
        // block-meta array, so buildMetricsMenu must be a pure function of `selected` with
        // no shared state between calls.
        const menuA = buildMetricsMenu(fixtureMeta, ["cpu"], vi.fn());
        const menuB = buildMetricsMenu(fixtureMeta, ["mem:used"], vi.fn());
        expect(menuA.find((i) => i.label === "cpu")?.checked).toBe(true);
        expect(menuA.find((i) => i.label === "mem:used")?.checked).toBe(false);
        expect(menuB.find((i) => i.label === "cpu")?.checked).toBe(false);
        expect(menuB.find((i) => i.label === "mem:used")?.checked).toBe(true);
    });
});
```

- [ ] **Step 3: Confirm the tests fail**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx vitest run frontend/app/view/sysinfo/sysinfo.test.tsx`
Expected: FAIL — `getGpuColor`/`buildMetricsMenu` not exported yet.

- [ ] **Step 4: Rewrite `sysinfo.tsx`**

Replace the file's contents from the top through line 328 (everything before `const _plotColors = ...`) with:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { globalStore } from "@/app/store/jotaiStore";
import { makeORef } from "@/app/store/wos";
import * as util from "@/util/util";
import * as Plot from "@observablehq/plot";
import clsx from "clsx";
import dayjs from "dayjs";
import * as htl from "htl";
import * as jotai from "jotai";
import * as React from "react";

import { useDimensionsWithExistingRef } from "@/app/hook/useDimensions";
import type { MetaKeyAtomFnType, WaveEnv, WaveEnvSubset } from "@/app/remotetermenv/remotetermenv";
import { waveEventSubscribeSingle } from "@/app/store/wps";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { OverlayScrollbarsComponent, OverlayScrollbarsComponentRef } from "overlayscrollbars-react";

export type SysinfoEnv = WaveEnvSubset<{
    rpc: {
        EventReadHistoryCommand: WaveEnv["rpc"]["EventReadHistoryCommand"];
        SetMetaCommand: WaveEnv["rpc"]["SetMetaCommand"];
        GetSysInfoMetricsCommand: WaveEnv["rpc"]["GetSysInfoMetricsCommand"];
        SysInfoReprobeCommand: WaveEnv["rpc"]["SysInfoReprobeCommand"];
    };
    atoms: {
        fullConfigAtom: WaveEnv["atoms"]["fullConfigAtom"];
    };
    getConnStatusAtom: WaveEnv["getConnStatusAtom"];
    getBlockMetaKeyAtom: MetaKeyAtomFnType<"graph:numpoints" | "sysinfo:metrics" | "connection" | "count">;
}>;

const DefaultNumPoints = 120;
const _plotColors = ["#58C142", "#FFC107", "#FF5722", "#2196F3", "#9C27B0", "#00BCD4", "#FFEB3B", "#795548"];

export function getGpuColor(gpuIndex: number): string {
    return _plotColors[gpuIndex % _plotColors.length];
}

type DataItem = {
    ts: number;
    [k: string]: number;
};

function convertWaveEventToDataItem(event: Extract<WaveEvent, { event: "sysinfo" }>): DataItem {
    const eventData = event.data;
    if (eventData == null || eventData.ts == null || eventData.values == null) {
        return null;
    }
    const dataItem = { ts: eventData.ts };
    for (const key in eventData.values) {
        dataItem[key] = eventData.values[key];
    }
    return dataItem;
}

function gpuIndexFromKey(key: string): number | null {
    const match = key.match(/^gpu:(\d+):/);
    if (match == null) {
        return null;
    }
    return parseInt(match[1], 10);
}

export function buildMetricsMenu(
    availableMeta: Record<string, TimeSeriesMeta>,
    selected: string[],
    onToggle: (key: string) => void
): ContextMenuItem[] {
    const selectedSet = new Set(selected);
    const gpuGroups = new Map<number, string[]>();
    const flatKeys: string[] = [];

    for (const key of Object.keys(availableMeta)) {
        const gpuIdx = gpuIndexFromKey(key);
        if (gpuIdx != null) {
            if (!gpuGroups.has(gpuIdx)) {
                gpuGroups.set(gpuIdx, []);
            }
            gpuGroups.get(gpuIdx).push(key);
        } else {
            flatKeys.push(key);
        }
    }

    const menu: ContextMenuItem[] = flatKeys.map((key) => ({
        label: key,
        type: "checkbox",
        checked: selectedSet.has(key),
        click: () => onToggle(key),
    }));

    const sortedGpuIndices = Array.from(gpuGroups.keys()).sort((a, b) => a - b);
    for (const gpuIdx of sortedGpuIndices) {
        const keys = gpuGroups.get(gpuIdx);
        menu.push({
            label: `GPU ${gpuIdx}`,
            submenu: keys.map((key) => ({
                label: availableMeta[key]?.name ?? key,
                type: "checkbox",
                checked: selectedSet.has(key),
                click: () => onToggle(key),
            })),
        });
    }

    return menu;
}

class SysinfoViewModel implements ViewModel {
    viewType: string;
    termMode: jotai.Atom<string>;
    htmlElemFocusRef: React.RefObject<HTMLInputElement>;
    blockId: string;
    viewIcon: jotai.Atom<string>;
    viewText: jotai.Atom<string>;
    viewName: jotai.Atom<string>;
    dataAtom: jotai.PrimitiveAtom<Array<DataItem>>;
    addInitialDataAtom: jotai.WritableAtom<unknown, [DataItem[]], void>;
    addContinuousDataAtom: jotai.WritableAtom<unknown, [DataItem], void>;
    incrementCount: jotai.WritableAtom<unknown, [], Promise<void>>;
    loadingAtom: jotai.PrimitiveAtom<boolean>;
    numPoints: jotai.Atom<number>;
    metricsAtom: jotai.Atom<string[]>;
    connection: jotai.Atom<string>;
    manageConnection: jotai.Atom<boolean>;
    filterOutNowsh: jotai.Atom<boolean>;
    connStatus: jotai.Atom<ConnStatus>;
    availableMetaAtom: jotai.PrimitiveAtom<Record<string, TimeSeriesMeta>>;
    errorsAtom: jotai.Atom<Record<string, string>>;
    endIconButtons: jotai.Atom<IconButtonDecl[]>;
    env: SysinfoEnv;

    constructor({ blockId, waveEnv }: ViewModelInitType) {
        this.viewType = "sysinfo";
        this.blockId = blockId;
        this.env = waveEnv;
        this.addInitialDataAtom = jotai.atom(null, (get, set, points) => {
            const targetLen = get(this.numPoints) + 1;
            try {
                const newDataRaw = [...points];
                if (newDataRaw.length == 0) {
                    return;
                }
                const latestItemTs = newDataRaw[newDataRaw.length - 1]?.ts ?? 0;
                const cutoffTs = latestItemTs - 1000 * targetLen;
                const blankItemTemplate = { ...newDataRaw[newDataRaw.length - 1] };
                for (const key in blankItemTemplate) {
                    blankItemTemplate[key] = NaN;
                }

                const newDataFiltered = newDataRaw.filter((dataItem) => dataItem.ts >= cutoffTs);
                if (newDataFiltered.length == 0) {
                    return;
                }
                const newDataWithGaps: Array<DataItem> = [];
                if (newDataFiltered[0].ts > cutoffTs) {
                    const blankItemStart = { ...blankItemTemplate, ts: cutoffTs };
                    const blankItemEnd = { ...blankItemTemplate, ts: newDataFiltered[0].ts - 1 };
                    newDataWithGaps.push(blankItemStart);
                    newDataWithGaps.push(blankItemEnd);
                }
                newDataWithGaps.push(newDataFiltered[0]);
                for (let i = 1; i < newDataFiltered.length; i++) {
                    const prevIdxItem = newDataFiltered[i - 1];
                    const curIdxItem = newDataFiltered[i];
                    const timeDiff = curIdxItem.ts - prevIdxItem.ts;
                    if (timeDiff > 2000) {
                        const blankItemStart = { ...blankItemTemplate, ts: prevIdxItem.ts + 1, blank: 1 };
                        const blankItemEnd = { ...blankItemTemplate, ts: curIdxItem.ts - 1, blank: 1 };
                        newDataWithGaps.push(blankItemStart);
                        newDataWithGaps.push(blankItemEnd);
                    }
                    newDataWithGaps.push(curIdxItem);
                }
                set(this.dataAtom, newDataWithGaps);
            } catch (e) {
                console.log("Error adding data to sysinfo", e);
            }
        });
        this.addContinuousDataAtom = jotai.atom(null, (get, set, newPoint) => {
            const targetLen = get(this.numPoints) + 1;
            const data = get(this.dataAtom);
            try {
                const latestItemTs = newPoint?.ts ?? 0;
                const cutoffTs = latestItemTs - 1000 * targetLen;
                data.push(newPoint);
                const newData = data.filter((dataItem) => dataItem.ts >= cutoffTs);
                set(this.dataAtom, newData);
            } catch (e) {
                console.log("Error adding data to sysinfo", e);
            }
        });
        this.availableMetaAtom = jotai.atom({});
        this.manageConnection = jotai.atom(true);
        this.filterOutNowsh = jotai.atom(true);
        this.loadingAtom = jotai.atom(true);
        this.numPoints = jotai.atom((get) => {
            const metaNumPoints = get(this.env.getBlockMetaKeyAtom(blockId, "graph:numpoints"));
            if (metaNumPoints == null || metaNumPoints <= 0) {
                return DefaultNumPoints;
            }
            return metaNumPoints;
        });
        this.metricsAtom = jotai.atom((get) => {
            const metrics = get(this.env.getBlockMetaKeyAtom(blockId, "sysinfo:metrics"));
            if (metrics == null || !Array.isArray(metrics) || metrics.length === 0) {
                return ["cpu"];
            }
            return metrics;
        });
        this.errorsAtom = jotai.atom((get) => {
            const plotData = get(this.dataAtom);
            const last = plotData[plotData.length - 1];
            return (last as any)?.__errors ?? {};
        });
        this.viewIcon = jotai.atom((get) => {
            return "chart-line"; // should not be hardcoded
        });
        this.viewName = jotai.atom((get) => {
            return "Sysinfo";
        });
        this.incrementCount = jotai.atom(null, async (get, _set) => {
            const count = get(this.env.getBlockMetaKeyAtom(blockId, "count")) ?? 0;
            await this.env.rpc.SetMetaCommand(TabRpcClient, {
                oref: makeORef("block", this.blockId),
                meta: { count: count + 1 },
            });
        });
        this.connection = jotai.atom((get) => {
            const connValue = get(this.env.getBlockMetaKeyAtom(blockId, "connection"));
            if (util.isBlank(connValue)) {
                return "local";
            }
            return connValue;
        });
        this.dataAtom = jotai.atom([]);
        this.loadInitialData();
        this.loadAvailableMetrics();
        this.connStatus = jotai.atom((get) => {
            const connName = get(this.env.getBlockMetaKeyAtom(blockId, "connection"));
            const connAtom = this.env.getConnStatusAtom(connName);
            return get(connAtom);
        });
    }

    get viewComponent(): ViewComponent {
        return SysinfoView;
    }

    async loadAvailableMetrics() {
        try {
            const connName = globalStore.get(this.connection);
            const meta = await this.env.rpc.GetSysInfoMetricsCommand(TabRpcClient, { connname: connName });
            globalStore.set(this.availableMetaAtom, meta ?? {});
        } catch (e) {
            console.log("Error loading sysinfo metric metadata", e);
        }
    }

    async reprobe() {
        try {
            const connName = globalStore.get(this.connection);
            await this.env.rpc.SysInfoReprobeCommand(TabRpcClient, { connname: connName });
            await this.loadAvailableMetrics();
        } catch (e) {
            console.log("Error reprobing sysinfo collectors", e);
        }
    }

    async toggleMetric(key: string) {
        const current = globalStore.get(this.metricsAtom);
        const next = current.includes(key) ? current.filter((k) => k !== key) : [...current, key];
        await this.env.rpc.SetMetaCommand(TabRpcClient, {
            oref: makeORef("block", this.blockId),
            meta: { "sysinfo:metrics": next },
        });
    }

    async loadInitialData() {
        globalStore.set(this.loadingAtom, true);
        try {
            const numPoints = globalStore.get(this.numPoints);
            const connName = globalStore.get(this.connection);
            const initialData = await this.env.rpc.EventReadHistoryCommand(TabRpcClient, {
                event: "sysinfo",
                scope: connName,
                maxitems: numPoints,
            });
            if (initialData == null) {
                return;
            }
            this.getDefaultData();
            const initialDataItems: DataItem[] = initialData.map(convertWaveEventToDataItem);
            globalStore.set(this.addInitialDataAtom, initialDataItems);
        } catch (e) {
            console.log("Error loading initial data for sysinfo", e);
        } finally {
            globalStore.set(this.loadingAtom, false);
        }
    }

    getSettingsMenuItems(): ContextMenuItem[] {
        const availableMeta = globalStore.get(this.availableMetaAtom);
        const selected = globalStore.get(this.metricsAtom);
        const fullMenu: ContextMenuItem[] = [];
        fullMenu.push({
            label: "Metrics",
            submenu: buildMetricsMenu(availableMeta, selected, (key) => this.toggleMetric(key)),
        });
        fullMenu.push({
            label: "Re-detect GPU",
            click: () => this.reprobe(),
        });
        fullMenu.push({ type: "separator" });
        return fullMenu;
    }

    getDefaultData(): DataItem[] {
        const numPoints = globalStore.get(this.numPoints);
        const currentTime = Date.now() - 1000;
        const points: DataItem[] = [];
        for (let i = numPoints; i > -1; i--) {
            points.push({ ts: currentTime - i * 1000 });
        }
        return points;
    }
}
```

Then keep the file's existing `SysinfoView`, `SingleLinePlot`, and `SysinfoViewInner` function bodies (currently lines 347-570) largely as-is, with these targeted changes:

1. In `SysinfoViewInner` (currently around line 525), replace `const yvals = jotai.useAtomValue(model.metrics);` with `const yvals = jotai.useAtomValue(model.metricsAtom);` and replace `const plotMeta = jotai.useAtomValue(model.plotMetaAtom);` with `const plotMeta = jotai.useAtomValue(model.availableMetaAtom);`.
2. In the `SysinfoViewInner` render loop's `.map((yval, _idx) => ...)`, pass a new `errorMessage` prop to `SingleLinePlot`: `errorMessage={jotai.useAtomValue(model.errorsAtom)[collectorNameForKey(yval)]}` — add a small `collectorNameForKey(key: string): string` helper near `gpuIndexFromKey` that maps a metric key back to its owning collector name (`"cpu" | "cpu:N"` → `"cpu"`, `"mem:*"` → `"mem"`, `"cpu:temp"` → `"temp"`, `"gpu:N:*"` → `"gpu-nvidia"|"gpu-amd"|"gpu-intel"` — since the frontend can't know which vendor produced a given `gpu:N:*` key from the key alone, key the `errorsAtom` lookup on whichever of the three GPU collector names has a non-empty error and the metric is a `gpu:` key; if none matches, no badge. This is an acceptable approximation — the badge's job is "something is wrong with this metric's source," not precise vendor attribution).
3. In `SingleLinePlot`, accept the new `errorMessage?: string` prop and render a small badge when present:

```tsx
{errorMessage && (
    <div
        className="absolute top-1 right-1 z-10"
        title={errorMessage}
    >
        <i aria-hidden="true" className="fa-sharp fa-solid fa-triangle-exclamation text-warning text-xs" />
        <span className="sr-only">Metric error: {errorMessage}</span>
    </div>
)}
```
placed as a sibling of the plot's `containerRef` div, inside a wrapping `<div className="relative min-h-[100px]">` that replaces the current bare `<div ref={containerRef} className="min-h-[100px]" />` return.

- [ ] **Step 5: Run the tests**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx vitest run frontend/app/view/sysinfo/sysinfo.test.tsx`
Expected: PASS.

- [ ] **Step 6: Typecheck**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx tsc --noEmit`
Expected: no new errors beyond this repo's known pre-existing baseline (per prior sessions' handoffs, ~18 pre-existing preview/fixture errors unrelated to this work — compare the count before/after this task's changes, don't assume zero is the bar, compare against the baseline).

- [ ] **Step 7: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add frontend/app/view/sysinfo/sysinfo.tsx frontend/app/view/sysinfo/sysinfo.test.tsx frontend/app/theme.scss
git commit -m "feat(sysinfo): multi-select metrics UI, discovery-driven menu, error badges"
```

---

### Task 10: Frontend — per-connection heavy-interval field in Connections settings

**Files:**
- Modify: `frontend/app/view/remotetermconfig/connectionscontent.tsx`

**Interfaces:**
- Consumes: `model.applyBackgroundToTab`-style pattern is NOT reused (different page); instead reuses `SetConnectionsConfigCommand` already wired into this page's model (`RemoteTermConfigViewModel`) for the existing quick-add flow.
- Produces: a clickable `HostsList` row revealing an inline interval field, writing `sysinfo:heavyinterval` via the existing connections-config RPC.

**Context discovered during planning:** this page's `HostsList` (lines 159-207 today) is a flat, non-interactive status list — there is no existing per-connection detail/edit affordance to extend. This task adds one, scoped tightly to this single field (not a general per-connection settings editor, which is out of scope).

- [ ] **Step 1: Add expand state and the interval field to `HostsList`**

Modify `HostsListProps` and the row rendering (replacing lines 159-207):

```tsx
interface HostsListProps {
    names: string[];
    connStatusMap: Map<string, ConnStatus>;
    connKeywordsMap: Map<string, { "sysinfo:heavyinterval"?: number }>;
    onSetHeavyInterval: (name: string, seconds: number) => void;
}

const HostsList = memo(({ names, connStatusMap, connKeywordsMap, onSetHeavyInterval }: HostsListProps) => {
    const [expandedName, setExpandedName] = useState<string | null>(null);

    if (names.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center gap-2 py-12 text-center">
                <i aria-hidden="true" className="fa-sharp fa-solid fa-server text-3xl text-muted" />
                <div className="text-secondary">No connections found</div>
            </div>
        );
    }

    return (
        <div className="flex-1 min-h-0 flex flex-col gap-1.5 overflow-y-auto">
            <div className="grid grid-cols-[14px_20px_1fr_90px] gap-2.5 px-2 text-caption font-semibold uppercase tracking-wide text-muted">
                <span />
                <span />
                <span>Connection</span>
                <span>Last used</span>
            </div>
            {names.map((name) => {
                const status = connStatusMap.get(name);
                const expanded = expandedName === name;
                const currentInterval = connKeywordsMap.get(name)?.["sysinfo:heavyinterval"] ?? 5;
                return (
                    <div key={name} className="flex flex-col gap-1.5">
                        <button
                            type="button"
                            aria-expanded={expanded}
                            onClick={() => setExpandedName(expanded ? null : name)}
                            title={statusLabel(status)}
                            className="grid grid-cols-[14px_20px_1fr_90px] items-center gap-2.5 bg-panel border border-border/60 rounded-md px-2 py-2 text-left cursor-pointer hover:border-border"
                        >
                            <span
                                aria-hidden="true"
                                className={cn("w-1.5 h-1.5 rounded-full justify-self-center", statusDotClass(status))}
                            />
                            <span className="w-5 h-5 rounded flex items-center justify-center bg-surface text-secondary">
                                <i aria-hidden="true" className="fa-sharp fa-solid fa-server text-xxs" />
                            </span>
                            <span className="font-mono text-xs truncate">
                                {name}
                                <span className="sr-only"> — {statusLabel(status)}</span>
                            </span>
                            <span className="text-xs text-muted-foreground">
                                {formatRelativeTime(status?.lastconnecttime ?? 0)}
                            </span>
                        </button>
                        {expanded && (
                            <div className="flex items-center gap-2 px-3 py-2 bg-panel/60 border border-border/40 rounded-md text-caption">
                                <label htmlFor={`heavy-interval-${name}`} className="text-muted">
                                    GPU/temp poll interval (seconds)
                                </label>
                                <input
                                    id={`heavy-interval-${name}`}
                                    type="number"
                                    min={1}
                                    max={60}
                                    value={currentInterval}
                                    onChange={(e) => onSetHeavyInterval(name, Number(e.target.value))}
                                    className="w-16 bg-black/20 border border-border rounded px-1.5 py-0.5 text-xs"
                                />
                            </div>
                        )}
                    </div>
                );
            })}
        </div>
    );
});
HostsList.displayName = "HostsList";
```

- [ ] **Step 2: Wire the write handler and pass `connKeywordsMap` from `ConnectionsContent`**

Find the component that currently renders `<HostsList names={...} connStatusMap={...} />` inside `ConnectionsContent` (search this file for `<HostsList`) and add:

```tsx
const handleSetHeavyInterval = (name: string, seconds: number) => {
    if (!Number.isFinite(seconds) || seconds < 1) {
        return;
    }
    model.env.rpc.SetConnectionsConfigCommand(TabRpcClient, {
        host: name,
        metamaptype: { "sysinfo:heavyinterval": seconds },
    });
};
```

Pass `connKeywordsMap={model.connectionsConfigAtom /* or whatever atom already exposes per-connection ConnKeywords on this page — locate it by searching this file for how the Keychain/status rows already read connection config, reuse that same atom rather than adding a second one */}` and `onSetHeavyInterval={handleSetHeavyInterval}` to `<HostsList>`. The exact existing atom name for per-connection config on this page needs to be located by the implementer at this step — grep `connectionscontent.tsx` and `remotetermconfig-model.ts` for how `ConnKeywords` values already reach this component (the Keychain toggle and quick-add flow both must already read/write connection-scoped config somehow); reuse that mechanism rather than introducing a parallel one. If no such atom already exists on this page, add one on `RemoteTermConfigViewModel` following the same pattern as `backgroundsOrderedAtom`.

- [ ] **Step 3: Manual verification (no automated test for this task — see note)**

This task's correctness (does the field round-trip to `connections.json` and does the running `RunSysInfoLoop` pick it up) can't be meaningfully unit-tested without either mocking the full RPC round-trip or a live app — and Task 6/7's Go-side `getHeavyIntervalSeconds` already has direct unit coverage for the read side. Verify manually: open the Wave Config modal's Connections page in the running `task dev` instance, expand a connection row, change the interval field, confirm (via the existing `docs/qa-review`-style manual-verification convention this repo has used for prior UI work, e.g. the widgets-toggle live-verify from the earlier config-reskin arc) that `connections.json` on disk actually updates. Flag to the owner if this step reveals the atom-location assumption from Step 2 was wrong.

- [ ] **Step 4: Typecheck**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx tsc --noEmit`
Expected: no new errors beyond the known baseline.

- [ ] **Step 5: Commit**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git add frontend/app/view/remotetermconfig/connectionscontent.tsx
git commit -m "feat(sysinfo): expandable connection row for heavy-interval setting"
```

---

## Execution Handoff Note

This plan has 10 tasks. Tasks 3-5 (the three GPU vendor collectors) are independent of each other once Task 1 lands, and Tasks 9-10 (frontend) depend on Task 7's generated bindings but not on each other. Given the plan's mix of well-specified pure-logic tasks (1, 2, 6, 8) and two tasks carrying explicit unverified-assumption caveats (4's rocm-smi field names, 5's intel_gpu_top schema, 10's connection-config atom name), subagent-driven execution's per-task fresh review is worth the cost here specifically because those caveats are exactly the kind of thing a fresh reviewer either confirms was actually verified or catches as still-unverified before it ships.
