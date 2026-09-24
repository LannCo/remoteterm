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
		if attempt == 4 {
			return map[string]float64{"gpu:0:util": 5}, nil
		}
		return nil, fmt.Errorf("fail %d", attempt)
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
	_, errMsg = tracker.record(recovering)
	if errMsg != "" {
		t.Fatalf("expected first failure after recovery to be strike 1 (no error), got %q", errMsg)
	}
}

func TestHeavyValuesCarryForwardOnSubThresholdFailure(t *testing.T) {
	attempt := 0
	gpu := &fakeCollector{name: "gpu-amd", probeOk: true, collectFn: func() (map[string]float64, error) {
		attempt++
		if attempt == 1 {
			return map[string]float64{"gpu:0:util": 42}, nil
		}
		return nil, fmt.Errorf("transient %d", attempt)
	}}
	state := &sysInfoLoopState{
		heavyCollectors: []Collector{gpu},
		tracker:         makeFailureTracker(),
		heavyInterval:   1,
	}
	values, _ := collectTick(state)
	if values["gpu:0:util"] != 42 {
		t.Fatalf("expected tick 1 value 42, got %v", values)
	}
	values, errs := collectTick(state)
	if v, ok := values["gpu:0:util"]; !ok || v != 42 {
		t.Fatalf("expected tick 1 value carried forward after one failure, got %v", values)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no errors on strike 1, got %v", errs)
	}
}

func TestGetHeavyIntervalSecondsDefaultsWhenUnset(t *testing.T) {
	got := getHeavyIntervalSeconds("connection-with-no-config-entry")
	if got != DefaultHeavyIntervalSeconds {
		t.Fatalf("expected default %d for an unconfigured connection, got %d", DefaultHeavyIntervalSeconds, got)
	}
}

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

func TestDescribeActiveCollectorsMergesFastAndHeavy(t *testing.T) {
	cpu := &fakeCollector{name: "cpu", meta: map[string]MetricMeta{"cpu": {Label: "CPU"}}}
	gpu := &fakeCollector{name: "gpu-amd", meta: map[string]MetricMeta{"gpu:0:util": {Label: "GPU"}}}
	state := &sysInfoLoopState{fastCollectors: []Collector{cpu}, heavyCollectors: []Collector{gpu}}
	registerLoopState("describe-test-conn", state)
	defer unregisterLoopState("describe-test-conn", state)
	meta := DescribeActiveCollectors("describe-test-conn")
	if len(meta) != 2 || meta["cpu"].Label != "CPU" || meta["gpu:0:util"].Label != "GPU" {
		t.Fatalf("expected cpu and gpu metadata, got %v", meta)
	}
}

func TestUnregisterLoopStateKeepsNewerLoopEntry(t *testing.T) {
	older := &sysInfoLoopState{}
	newer := &sysInfoLoopState{}
	registerLoopState("unregister-test-conn", older)
	registerLoopState("unregister-test-conn", newer)
	unregisterLoopState("unregister-test-conn", older)
	if got := lookupLoopState("unregister-test-conn"); got != newer {
		t.Fatalf("expected the newer loop's entry to survive the older loop exiting")
	}
	unregisterLoopState("unregister-test-conn", newer)
	if got := lookupLoopState("unregister-test-conn"); got != nil {
		t.Fatalf("expected entry removed once its own loop exits")
	}
}

func TestPartitionCollectorsSplitsByHeavyName(t *testing.T) {
	fast, heavy := partitionCollectors([]Collector{
		&fakeCollector{name: "cpu"}, &fakeCollector{name: "temp"},
		&fakeCollector{name: "mem"}, &fakeCollector{name: "gpu-nvidia"},
	})
	if fmt.Sprint(collectorNames(fast)) != "[cpu mem]" || fmt.Sprint(collectorNames(heavy)) != "[temp gpu-nvidia]" {
		t.Fatalf("unexpected partition fast=%v heavy=%v", collectorNames(fast), collectorNames(heavy))
	}
}

func TestReplaceCollectorsPurgesRemovedHeavyCollectorState(t *testing.T) {
	gpuFails := false
	gpu := &fakeCollector{
		name: "gpu-amd",
		meta: map[string]MetricMeta{"gpu:0:util": {}, "gpu:1:util": {}},
		collectFn: func() (map[string]float64, error) {
			if gpuFails {
				return nil, fmt.Errorf("rocm-smi gone")
			}
			return map[string]float64{"gpu:0:util": 42, "gpu:1:util": 7}, nil
		},
	}
	temp := &fakeCollector{
		name:      "temp",
		meta:      map[string]MetricMeta{"temp:cpu": {}},
		collectFn: func() (map[string]float64, error) { return map[string]float64{"temp:cpu": 55}, nil },
	}
	state := &sysInfoLoopState{
		heavyCollectors: []Collector{gpu, temp},
		tracker:         makeFailureTracker(),
		heavyInterval:   1,
	}
	collectTick(state)
	gpuFails = true
	collectTick(state)
	collectTick(state)
	values, errs := collectTick(state)
	if values["gpu:0:util"] != 42 || errs["gpu-amd"] == "" {
		t.Fatalf("precondition: expected carried-forward gpu value and surfaced error, got values=%v errs=%v", values, errs)
	}

	state.replaceCollectors(nil, []Collector{temp}, 1)
	values, errs = collectTick(state)
	for _, k := range []string{"gpu:0:util", "gpu:1:util"} {
		if _, ok := values[k]; ok {
			t.Fatalf("expected %s purged after its collector was reprobed away, got %v", k, values)
		}
	}
	if _, ok := errs["gpu-amd"]; ok {
		t.Fatalf("expected gpu-amd error purged after reprobe, got %v", errs)
	}
	if values["temp:cpu"] != 55 {
		t.Fatalf("expected surviving collector's values kept, got %v", values)
	}
	if _, ok := state.tracker.counts["gpu-amd"]; ok {
		t.Fatalf("expected gpu-amd failure count reset after reprobe")
	}
}

func TestReplaceCollectorsPurgesKeysDroppedBySameNamedCollector(t *testing.T) {
	twoGpus := &fakeCollector{
		name:      "gpu-nvidia",
		meta:      map[string]MetricMeta{"gpu:0:util": {}, "gpu:1:util": {}},
		collectFn: func() (map[string]float64, error) { return map[string]float64{"gpu:0:util": 1, "gpu:1:util": 2}, nil },
	}
	oneGpu := &fakeCollector{
		name:      "gpu-nvidia",
		meta:      map[string]MetricMeta{"gpu:0:util": {}},
		collectFn: func() (map[string]float64, error) { return map[string]float64{"gpu:0:util": 3}, nil },
	}
	state := &sysInfoLoopState{heavyCollectors: []Collector{twoGpus}, tracker: makeFailureTracker(), heavyInterval: 1}
	collectTick(state)
	state.replaceCollectors(nil, []Collector{oneGpu}, 1)
	values, _ := collectTick(state)
	if _, ok := values["gpu:1:util"]; ok {
		t.Fatalf("expected unplugged gpu:1 key purged, got %v", values)
	}
	if values["gpu:0:util"] != 3 {
		t.Fatalf("expected gpu:0 from the new collector, got %v", values)
	}
}

func TestReplaceCollectorsConcurrentWithTicksIsRaceFree(t *testing.T) {
	mk := func() Collector {
		return &fakeCollector{
			name:      "gpu-amd",
			meta:      map[string]MetricMeta{"gpu:0:util": {}},
			collectFn: func() (map[string]float64, error) { return map[string]float64{"gpu:0:util": 1}, nil },
		}
	}
	state := &sysInfoLoopState{heavyCollectors: []Collector{mk()}, tracker: makeFailureTracker(), heavyInterval: 1}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			state.replaceCollectors(nil, []Collector{mk()}, 1+i%3)
			state.activeCollectors()
		}
	}()
	for i := 0; i < 200; i++ {
		collectTick(state)
	}
	<-done
}

func collectorNames(cs []Collector) []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name()
	}
	return names
}
