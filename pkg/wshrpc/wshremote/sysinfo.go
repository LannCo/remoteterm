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

// TODO(task 7): guard activeLoopStates with a sync.Mutex; RPC handlers calling
// ReprobeCollectors run on different goroutines than RunSysInfoLoop.
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
