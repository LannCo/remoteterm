// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"fmt"
	"testing"
)

type fakeCollector struct {
	name       string
	probeCalls int
	probeOk    bool
	collectFn  func() (map[string]float64, error)
	meta       map[string]MetricMeta
}

func (f *fakeCollector) Name() string                         { return f.name }
func (f *fakeCollector) Probe() bool                          { f.probeCalls++; return f.probeOk }
func (f *fakeCollector) Describe() map[string]MetricMeta      { return f.meta }
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

func TestGetHeavyIntervalSecondsDefaultsWhenUnset(t *testing.T) {
	got := getHeavyIntervalSeconds("connection-with-no-config-entry")
	if got != DefaultHeavyIntervalSeconds {
		t.Fatalf("expected default %d for an unconfigured connection, got %d", DefaultHeavyIntervalSeconds, got)
	}
}

func collectorNames(cs []Collector) []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name()
	}
	return names
}
