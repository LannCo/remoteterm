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
