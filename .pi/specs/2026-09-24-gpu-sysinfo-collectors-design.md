# GPU / temperature stats in the sysinfo panel — design spec

Status: approved by owner 2026-09-24, ready for implementation planning.

## Intent

The sysinfo widget (`frontend/app/view/sysinfo/sysinfo.tsx` +
`pkg/wshrpc/wshremote/sysinfo.go`) currently plots CPU% and memory only, via
a hardcoded set of named presets (`CPU`, `Mem`, `CPU + Mem`, `All CPU`),
single-select via a context-menu radio. Owner wants GPU utilization/VRAM/temp
(NVIDIA, AMD, Intel) and CPU temperature added, with **true multi-select**
of any combination of available metrics — not more fixed presets. This
requires restructuring the backend from two hardcoded collection functions
into a pluggable collector registry, since a fixed-preset model can't
express "whatever metrics are available on this host, user picks any
subset."

Scope covers both local and remote (SSH) connections — `RunSysInfoLoop`
already runs once per connection (the Go binary executes on whichever host
the connection targets, local or remote-agent), so collectors need no
separate remote-specific code path, only host-appropriate detection.

Source mockup: `ModalSysinfoGPU.dc.html` from the Wave Config design canvas
(`https://claude.ai/artifact/9vUo8edtrgGJ6rvKSePxy1`) — shown as reference
for the GPU series' visual shape and the "new menu entries" framing, not
followed literally for menu structure (multi-select checkboxes replace the
mockup's fixed-preset radio list per owner's scope decision below).

## Explicitly out of scope

- Fully generic user-defined telemetry (arbitrary user-supplied commands as
  metric sources). Considered and rejected for this round: no stated need
  beyond vendor GPUs + a temp reading, and it's a security-sensitive
  expansion (remote arbitrary command execution) that would need its own
  scoping pass if ever wanted.
- Aggregate/blended GPU metrics across multiple cards (e.g. "average GPU
  util"). Per-GPU keys only, matching how per-core CPU has no blended
  equivalent today. Multi-GPU is not assumed to be a rare case for this
  app's users.
- True per-widget poll intervals. The backend loop is one-per-connection,
  broadcasting one shared event stream to every sysinfo widget watching
  that connection; restructuring that into per-subscriber polling was
  considered and rejected as disproportionate — the underlying reasoning
  for wider GPU/temp intervals (heavier calls) holds per-connection, not
  per-widget, so a shared per-connection interval is the correct unit
  regardless of restructuring cost.
- App-wide toast/notification plumbing for collector failures. Failures
  surface inline on the affected widget instead (see Error handling).

## Architecture

### Collector interface

New file `pkg/wshrpc/wshremote/sysinfo_collectors.go`:

```go
type MetricValue struct {
    Value         float64
    Label         string
    Unit          string
    Color         string
    MinY          float64
    MaxY          float64
    DecimalPlaces int
}

type Collector interface {
    Name() string          // "cpu", "mem", "temp", "gpu-nvidia", "gpu-amd", "gpu-intel"
    Probe() bool            // detect once; true if usable on this host, cache the result
    Collect() map[string]MetricValue
}
```

One collector implementation per family: `cpuCollector`, `memCollector`,
`tempCollector` (host/CPU temperature via a sensors read), `nvidiaGpuCollector`,
`amdGpuCollector`, `intelGpuCollector`. All three GPU collectors shell out to
their vendor CLI (`nvidia-smi`, `rocm-smi`, `intel_gpu_top`) — no cgo/NVML
binding, uniform code path across vendors, easiest to extend to a future
vendor. Collectors that fail to detect their CLI/driver mark `Probe()` false
and are dropped from that connection's active set for the loop's lifetime
(re-probing is manual, see below — not automatic).

### Registry and loop

`RunSysInfoLoop(client, connName)` builds the collector list once at start,
calls `Probe()` on each, keeps only the ones that return true. Two tiers:

- **Fast tier** (`cpu`, `mem`): tick every 1s, unchanged from today.
- **Heavy tier** (`temp`, `gpu-nvidia`, `gpu-amd`, `gpu-intel`): tick on a
  slower, configurable interval (default 5s), grouped together under one
  interval setting rather than one knob per collector — the "heavier call,
  wider gap" reasoning applies to the whole tier, not GPU specifically.
  Between heavy-tier ticks, the loop republishes the heavy tier's
  last-collected values unchanged alongside the fresh fast-tier read, so the
  event stream stays continuous at 1s cadence even though heavy metrics
  refresh less often.

Values from both tiers are merged into one `map[string]float64` for
`TimeSeriesData.Values`, same wire shape as today — no RPC/event-type
schema break for the base event.

### Interval configuration

The heavy-tier interval is a per-connection setting, stored in
`connections.json` (`ConnKeywords`, e.g. `sysinfo:heavyinterval`, seconds,
default 5). Editable from two places, per owner's explicit "I'd expect it
in both" answer:

1. The sysinfo widget's gear/context menu (writes through to that
   connection's config even though the control lives on the widget).
2. The Connections settings page (`frontend/app/view/waveconfig/connectionscontent.tsx`),
   alongside other per-connection fields.

Realistic case for per-connection (not global): different remote hosts run
different GPU hardware/drivers — an older card's `rocm-smi` query can cost
meaningfully more than a fast local `nvidia-smi` read, so hosts legitimately
want different cadences. Storage cost is trivial (one more `ConnKeywords`
field).

### Manual re-probe

New RPC (e.g. `SysInfoReprobeCommand(connName)`) re-runs `Probe()` on every
registered collector for a live connection's already-running loop, without
restarting the loop or losing plot history. Exposed as a "Re-detect GPU"
action in the widget's gear menu. Automatic periodic re-probing was
considered and rejected — probe-once-at-start-and-cache is simpler and the
hot-plug/driver-installed-mid-session case is rare enough to handle via an
explicit user action instead of periodic background waste.

### Metric keys

- `cpu:temp` — new, host/CPU temperature.
- `gpu:N:util`, `gpu:N:vram`, `gpu:N:temp` — per GPU index `N`, one triplet
  per detected card, across all three vendor collectors uniformly. No
  vendor prefix in the key itself (the collector that produced it is
  Collector.Name(), not part of the wire key) — a host only ever has one
  vendor's cards in practice, so `gpu:0`, `gpu:1`, … stays unambiguous.
- Existing `cpu`, `cpu:N`, `mem:total`, `mem:used`, `mem:free`,
  `mem:available` keys unchanged.

### Metric discovery RPC

New RPC (e.g. `GetSysInfoMetricsCommand(connName)`) returns
`map[string]MetricMeta` (label, unit, color, min/max, decimal places) for
every metric key currently available on that connection (i.e. every probed
collector's output keys). Replaces the hardcoded `DefaultPlotMeta` map in
`sysinfo.tsx`. Frontend calls it when a sysinfo widget connects and again
after a manual re-probe, to pick up newly-available collectors.

### Frontend

- `sysinfo:type` (preset string, block meta) and the parallel `graph:metrics`
  field collapse into one `sysinfo:metrics` array in block meta — the list
  of currently-selected metric keys, written directly by
  `SetMetaCommand`, no preset indirection.
- Context menu ("Plot Type" submenu today) becomes a flat checkbox list
  (`type: "checkbox"`, not `"radio"`) grouped by family — CPU / Memory /
  Temperature / GPU 0 / GPU 1 / … — built from the discovery RPC's
  response, not the hardcoded `PlotTypes` object. A host with no GPU simply
  never shows GPU entries; nothing greyed-out or explained, they don't
  exist for that connection.
- `PlotTypes`, `DefaultPlotMeta`, `defaultCpuMeta`/`defaultMemMeta` helpers,
  and the `plotTypeSelectedAtom` preset-name atom are removed; `metrics`
  atom reads `sysinfo:metrics` directly instead of deriving from a preset
  function.
- Tab title (`viewName` atom) becomes the fixed string `"Sysinfo"` — no
  more dynamic preset-name title, since there's no single name for an
  arbitrary metric combination.
- Gear menu gains: the heavy-tier interval control (see above) and the
  "Re-detect GPU" action.

### Error handling

Each collector's `Collect()` call is wrapped by the loop with a per-collector
consecutive-failure counter. A collector that fails 3 consecutive ticks is
marked degraded: its keys stop appearing in fresh `Values` for that tick
(matching today's silent-drop-on-error pattern for a single failure), AND
a new `Errors map[string]string` field on `TimeSeriesData` (in
`pkg/wshrpc/wshrpctypes.go`, alongside `Values`) gets an entry
`{collectorName: lastErrorMessage}`. The counter resets and the entry
clears on the next successful `Collect()`.

Frontend reads `Errors` off the same sysinfo event stream and renders a
small warning badge with a tooltip (the error message) on the affected
metric's plot/legend entry — visible only on that widget, no app-wide
notification system needed (none exists yet in this repo; out of scope to
build one for this). A transient single-tick failure (the common case,
e.g. a momentary `rocm-smi` hiccup) never surfaces anything — only a
persistent run of 3+ triggers the badge, so noise stays low.

## Testing

- **Go, per collector**: unit tests against canned vendor-CLI output
  (`nvidia-smi`, `rocm-smi`, `intel_gpu_top` sample outputs as fixtures),
  injected via a command-runner interface (not real `exec.Command` calls in
  tests) so CI and this machine (AMD-only hardware) can exercise all three
  vendors' parsing without needing the actual GPU present. Cover: normal
  parse, malformed/empty output, command-not-found (maps to `Probe()`
  false), and the 3-consecutive-failure threshold crossing into the
  `Errors` field.
- **Go, registry**: one test asserting probe-once-and-cache behavior (a
  collector that starts unavailable doesn't get retried mid-loop without an
  explicit re-probe call) and one asserting the manual re-probe RPC updates
  the active collector set without disrupting the fast tier.
- **Frontend**: extend/replace the existing sysinfo tests for: checkbox
  menu construction from a discovery-RPC-shaped fixture, `sysinfo:metrics`
  array read/write via `SetMetaCommand`, empty-selection rendering, and the
  error-badge rendering off a fixture `TimeSeriesData.Errors` payload.

## Open items carried into the implementation plan (not decisions, just don't-forget)

- Exact RPC names (`SysInfoReprobeCommand`, `GetSysInfoMetricsCommand`) are
  illustrative; finalize against `wshrpctypes.go`'s existing naming
  conventions during planning.
- `connections.json`'s `ConnKeywords` struct (`pkg/remote/connutil.go` /
  `pkg/wconfig/settingsconfig.go` — confirm exact location, renamed during
  the 2026-09 rebrand) needs the new `sysinfo:heavyinterval` field added
  with its default and validation.
- `task generate` must run after the `wshrpctypes.go`/`TimeSeriesData`
  change to regenerate `frontend/types/gotypes.d.ts` and
  `frontend/app/store/wshclientapi.ts` — do not hand-edit those generated
  files.
