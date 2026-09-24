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
