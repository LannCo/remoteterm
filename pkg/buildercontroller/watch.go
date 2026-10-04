// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"log"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
)

var liveRebuildEnabled = func() bool {
	return rtconfig.GetWatcher().GetFullConfig().Settings.BuilderLiveRebuild
}

var publishAppGoUpdated = func(appId string) {
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_WaveAppAppGoUpdated,
		Scopes: []string{appId},
	})
}

// appId must come from the builder's rtinfo, never from a request.
func (bc *BuilderController) StartWatching(appId string) wshrpc.BuilderWatchStatusData {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return wshrpc.BuilderWatchStatusData{Status: WatchStatus_Unavailable, Reason: err.Error()}
	}
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.stopWatcher_nolock()
	watcher, err := MakeAppWatcher(appDir, func() {
		bc.handleAppFilesChanged(appId)
	}, bc.publishWatchStatus)
	if err != nil {
		return wshrpc.BuilderWatchStatusData{Status: WatchStatus_Unavailable, Reason: err.Error()}
	}
	bc.watcher = watcher
	return wshrpc.BuilderWatchStatusData{Status: WatchStatus_Active}
}

func (bc *BuilderController) StopWatching() {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.stopWatcher_nolock()
}

func (bc *BuilderController) stopWatcher_nolock() {
	if bc.watcher == nil {
		return
	}
	bc.watcher.Close()
	bc.watcher = nil
}

// Off by default: an agent sandboxed to the app folder must not be able to run code
// outside the sandbox just by writing a file. The frontend offers a Rebuild button.
// A save from the Code tab requests its own rebuild, which records the input hash, so
// the hash comparison here is what keeps that save from also triggering a live rebuild.
func (bc *BuilderController) handleAppFilesChanged(appId string) {
	if bc.isClosed() {
		return
	}
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	hash, err := ComputeAppInputHash(appDir)
	if err == nil && hash == bc.getLastBuildInputHash() {
		return
	}
	publishAppGoUpdated(appId)
	if !liveRebuildEnabled() {
		return
	}
	_, builderEnv, err := GetBuilderRebuildInputs(bc.builderId)
	if err != nil {
		log.Printf("BuilderController: live rebuild of %s skipped: %v\n", appId, err)
		return
	}
	bc.RequestRebuild(appId, builderEnv)
}

func (bc *BuilderController) publishWatchStatus(status string, reason string) {
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_BuilderWatchStatus,
		Scopes: []string{remotetermobj.MakeORef(remotetermobj.OType_Builder, bc.builderId).String()},
		Data:   wshrpc.BuilderWatchStatusData{Status: status, Reason: reason},
	})
}
