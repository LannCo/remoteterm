// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"sync/atomic"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
)

const watchClosedReason = "the builder was closed"

var liveRebuildEnabled = func() bool {
	return rtconfig.GetWatcher().GetFullConfig().Settings.BuilderLiveRebuild
}

var publishAppGoUpdated = func(appId string) {
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_WaveAppAppGoUpdated,
		Scopes: []string{appId},
	})
}

// appId must come from the builder's rtinfo, never from a request. The watcher is built
// outside bc.lock (it walks up to maxWatchedDirs directories) and swapped in under it.
func (bc *BuilderController) StartWatching(appId string) wshrpc.BuilderWatchStatusData {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return makeUnavailableStatus(err.Error())
	}
	if bc.isClosed() {
		return makeUnavailableStatus(watchClosedReason)
	}
	bc.StopWatching()
	// The editor has just loaded the app, so what is on disk now is the baseline; without
	// it the first change event would be announced even when it changed nothing.
	bc.recordAnnouncedHash(appId)
	var self atomic.Pointer[AppWatcher]
	watcher, err := MakeAppWatcher(appDir, func() {
		bc.handleAppFilesChanged(appId)
	}, func(status string, reason string) {
		bc.publishWatchStatusFrom(&self, status, reason)
	})
	if err != nil {
		return makeUnavailableStatus(err.Error())
	}
	self.Store(watcher)
	return bc.installWatcher(watcher)
}

func makeUnavailableStatus(reason string) wshrpc.BuilderWatchStatusData {
	return wshrpc.BuilderWatchStatusData{Status: WatchStatus_Unavailable, Reason: reason}
}

// A controller that DeleteController closed while its watcher was being built must not
// keep that watcher: nothing would ever close it, and it would hold one of the global
// slots and an inotify descriptor until restart.
func (bc *BuilderController) installWatcher(watcher *AppWatcher) wshrpc.BuilderWatchStatusData {
	replaced, status := bc.swapWatcher(watcher)
	if replaced != nil {
		replaced.Close()
	}
	return status
}

func (bc *BuilderController) swapWatcher(watcher *AppWatcher) (*AppWatcher, wshrpc.BuilderWatchStatusData) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.closed.Load() {
		return watcher, makeUnavailableStatus(watchClosedReason)
	}
	cur := bc.watcher
	if cur != nil && !cur.isClosed() && cur.appDir == watcher.appDir {
		// A concurrent StartWatching for the same app got there first.
		return watcher, wshrpc.BuilderWatchStatusData{Status: WatchStatus_Active}
	}
	bc.watcher = watcher
	return cur, wshrpc.BuilderWatchStatusData{Status: WatchStatus_Active}
}

func (bc *BuilderController) StopWatching() {
	if watcher := bc.takeWatcher(); watcher != nil {
		watcher.Close()
	}
}

func (bc *BuilderController) takeWatcher() *AppWatcher {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	watcher := bc.watcher
	bc.watcher = nil
	return watcher
}

// Off by default: an agent sandboxed to the app folder must not be able to run code
// outside the sandbox just by writing a file. The frontend offers a Rebuild button.
// A save from the Code tab requests its own rebuild, which records the input hash, so
// the hash comparison here is what keeps that save from also triggering a live rebuild.
func (bc *BuilderController) handleAppFilesChanged(appId string) {
	if bc.isClosed() {
		return
	}
	// The frontend can point a live builder at another app; a watcher left on the old
	// one must neither announce its changes nor build and run it.
	curAppId, builderEnv, err := GetBuilderRebuildInputs(bc.builderId)
	if err != nil || curAppId != appId {
		return
	}
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	hash, err := ComputeAppInputHash(appDir)
	if err != nil {
		// Without a hash an echo cannot be told from an outside change, so every event
		// counts as an outside change.
		publishAppGoUpdated(appId)
		if liveRebuildEnabled() {
			bc.RequestRebuild(appId, builderEnv)
		}
		return
	}
	if bc.markAnnounced(hash) {
		publishAppGoUpdated(appId)
	}
	if hash == bc.getLastBuildInputHash() || !liveRebuildEnabled() {
		return
	}
	bc.RequestRebuild(appId, builderEnv)
}

// A watcher that has been replaced can still report as it shuts down; its "unavailable"
// must not overwrite the "active" of the watcher that replaced it. self is empty only
// while the watcher is being built, before it can be anyone's predecessor.
func (bc *BuilderController) publishWatchStatusFrom(self *atomic.Pointer[AppWatcher], status string, reason string) {
	if w := self.Load(); w != nil && !bc.isCurrentWatcher(w) {
		return
	}
	publishBuilderWatchStatus(bc.builderId, wshrpc.BuilderWatchStatusData{Status: status, Reason: reason})
}

func (bc *BuilderController) isCurrentWatcher(w *AppWatcher) bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.watcher == w
}

var publishBuilderWatchStatus = func(builderId string, data wshrpc.BuilderWatchStatusData) {
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_BuilderWatchStatus,
		Scopes: []string{remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId).String()},
		Data:   data,
	})
}
