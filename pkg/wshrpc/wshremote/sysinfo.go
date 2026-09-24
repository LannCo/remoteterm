// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"log"
	"sync"
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

// sysInfoLoopState is shared between RunSysInfoLoop's goroutine and RPC
// handler goroutines (ReprobeCollectors, DescribeActiveCollectors). mu guards
// the fields ReprobeCollectors swaps in; every other field is touched only by
// the loop goroutine and needs no lock.
type sysInfoLoopState struct {
	connName string
	client   *wshutil.WshRpc

	mu              sync.Mutex
	fastCollectors  []Collector
	heavyCollectors []Collector
	heavyInterval   int
	// Queued by ReprobeCollectors and applied by the loop, because
	// lastHeavyValues/lastHeavyErrors belong to the loop goroutine.
	staleValueKeys  []string
	staleCollectors []string

	tracker         *failureTracker
	lastHeavyValues map[string]float64
	lastHeavyErrors map[string]string
	heavyEverRan    bool
	tickCount       int
}

type tickCollectors struct {
	fast            []Collector
	heavy           []Collector
	heavyInterval   int
	staleValueKeys  []string
	staleCollectors []string
}

func (s *sysInfoLoopState) takeTickCollectors() tickCollectors {
	s.mu.Lock()
	defer s.mu.Unlock()
	tc := tickCollectors{
		fast:            s.fastCollectors,
		heavy:           s.heavyCollectors,
		heavyInterval:   s.heavyInterval,
		staleValueKeys:  s.staleValueKeys,
		staleCollectors: s.staleCollectors,
	}
	s.staleValueKeys = nil
	s.staleCollectors = nil
	return tc
}

func (s *sysInfoLoopState) activeCollectors() []Collector {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append(append([]Collector{}, s.fastCollectors...), s.heavyCollectors...)
}

// replaceCollectors swaps in a freshly probed collector set and queues the
// metric keys and collector names of heavy collectors that are gone, so their
// carried-forward values and errors stop being published.
func (s *sysInfoLoopState) replaceCollectors(fast, heavy []Collector, heavyInterval int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	newKeys := make(map[string]bool)
	newNames := make(map[string]bool)
	for _, c := range heavy {
		newNames[c.Name()] = true
		for k := range c.Describe() {
			newKeys[k] = true
		}
	}
	for _, c := range s.heavyCollectors {
		if !newNames[c.Name()] {
			s.staleCollectors = append(s.staleCollectors, c.Name())
		}
		for k := range c.Describe() {
			if !newKeys[k] {
				s.staleValueKeys = append(s.staleValueKeys, k)
			}
		}
	}
	s.fastCollectors = fast
	s.heavyCollectors = heavy
	s.heavyInterval = heavyInterval
}

var activeLoopStateMu sync.Mutex
var activeLoopStates = make(map[string]*sysInfoLoopState)

func registerLoopState(connName string, state *sysInfoLoopState) {
	activeLoopStateMu.Lock()
	defer activeLoopStateMu.Unlock()
	activeLoopStates[connName] = state
}

// unregisterLoopState only removes the entry if it still points at state, so a
// loop exiting after a newer loop for the same connection registered cannot
// remove the newer loop's entry.
func unregisterLoopState(connName string, state *sysInfoLoopState) {
	activeLoopStateMu.Lock()
	defer activeLoopStateMu.Unlock()
	if activeLoopStates[connName] == state {
		delete(activeLoopStates, connName)
	}
}

func lookupLoopState(connName string) *sysInfoLoopState {
	activeLoopStateMu.Lock()
	defer activeLoopStateMu.Unlock()
	return activeLoopStates[connName]
}

func partitionCollectors(active []Collector) (fast, heavy []Collector) {
	fast = make([]Collector, 0)
	heavy = make([]Collector, 0)
	for _, c := range active {
		if heavyCollectorNames[c.Name()] {
			heavy = append(heavy, c)
		} else {
			fast = append(fast, c)
		}
	}
	return fast, heavy
}

func ReprobeCollectors(connName string) error {
	state := lookupLoopState(connName)
	if state == nil {
		return nil
	}
	// Probing shells out to vendor CLIs, so it runs without holding any lock.
	fast, heavy := partitionCollectors(probeAll(MakeCollectorRegistry()))
	state.replaceCollectors(fast, heavy, getHeavyIntervalSeconds(connName))
	return nil
}

// DescribeActiveCollectors returns wshremote.MetricMeta; wshrpc.MetricMeta is a
// hand-kept duplicate (import cycle), converted in the wshserver handler.
func DescribeActiveCollectors(connName string) map[string]MetricMeta {
	state := lookupLoopState(connName)
	if state == nil {
		return map[string]MetricMeta{}
	}
	meta := make(map[string]MetricMeta)
	for _, c := range state.activeCollectors() {
		for k, v := range c.Describe() {
			meta[k] = v
		}
	}
	return meta
}

func collectTick(state *sysInfoLoopState) (map[string]float64, map[string]string) {
	tc := state.takeTickCollectors()
	for _, k := range tc.staleValueKeys {
		delete(state.lastHeavyValues, k)
	}
	for _, name := range tc.staleCollectors {
		delete(state.lastHeavyErrors, name)
		delete(state.tracker.counts, name)
	}

	values := make(map[string]float64)
	errorsMap := make(map[string]string)

	for _, c := range tc.fast {
		v, errMsg := state.tracker.record(c)
		for k, val := range v {
			values[k] = val
		}
		if errMsg != "" {
			errorsMap[c.Name()] = errMsg
		}
	}

	runHeavyThisTick := tc.heavyInterval <= 1 || state.tickCount%tc.heavyInterval == 0
	if runHeavyThisTick && len(tc.heavy) > 0 {
		// Seeded from the previous tick so a sub-threshold failure keeps the
		// collector's last values on screen instead of blanking its chart.
		heavyValues := make(map[string]float64, len(state.lastHeavyValues))
		for k, v := range state.lastHeavyValues {
			heavyValues[k] = v
		}
		heavyErrors := make(map[string]string)
		for _, c := range tc.heavy {
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
	return values, errorsMap
}

func generateSingleServerData(state *sysInfoLoopState) {
	now := time.Now()
	values, errorsMap := collectTick(state)
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
	fast, heavy := partitionCollectors(probeAll(MakeCollectorRegistry()))
	state := &sysInfoLoopState{
		connName:        connName,
		client:          client,
		fastCollectors:  fast,
		heavyCollectors: heavy,
		tracker:         makeFailureTracker(),
		heavyInterval:   getHeavyIntervalSeconds(connName),
	}
	registerLoopState(connName, state)
	defer func() {
		log.Printf("sysinfo loop ended conn:%s\n", connName)
		unregisterLoopState(connName, state)
	}()
	for {
		generateSingleServerData(state)
		time.Sleep(1 * time.Second)
	}
}
