// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/tsunami/build"
)

type builderEventRecorder struct {
	lock   sync.Mutex
	events []wps.WaveEvent
}

func (r *builderEventRecorder) SendEvent(routeId string, event wps.WaveEvent) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.events = append(r.events, event)
}

func (r *builderEventRecorder) snapshot() []wps.WaveEvent {
	r.lock.Lock()
	defer r.lock.Unlock()
	return append([]wps.WaveEvent(nil), r.events...)
}

func recordBuilderEvents(t *testing.T, builderId string) *builderEventRecorder {
	t.Helper()
	rec := &builderEventRecorder{}
	prev := wps.Broker.GetClient()
	wps.Broker.SetClient(rec)
	routeId := "test-route-" + builderId
	scope := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId).String()
	wps.Broker.Subscribe(routeId, wps.SubscriptionRequest{Event: wps.Event_BuilderStatus, Scopes: []string{scope}})
	wps.Broker.Subscribe(routeId, wps.SubscriptionRequest{Event: wps.Event_BuilderOutput, Scopes: []string{scope}})
	t.Cleanup(func() {
		wps.Broker.UnsubscribeAll(routeId)
		wps.Broker.SetClient(prev)
	})
	return rec
}

func describeEvents(events []wps.WaveEvent) string {
	var parts []string
	for _, event := range events {
		parts = append(parts, fmt.Sprintf("%s %+v", event.Event, event.Data))
	}
	return strings.Join(parts, "\n")
}

// After an app switch deletes a controller mid-build, the old build must not start its app
// or publish into the builder scope that the new controller now owns.
func TestDeletedControllerBuildStaysSilentAndStartsNoApp(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appA := makeTestApp(t, home, "orphan-a")
	if err := os.WriteFile(filepath.Join(appA, "manifest.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	makeTestApp(t, home, "orphan-b")
	const builderId = "test-orphan"
	rec := recordBuilderEvents(t, builderId)

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseBuild := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseBuild)
	oldCtxErr := make(chan error, 1)
	origCompile, origStart := compileApp, startAppProcess
	compileApp = func(ctx context.Context, appNS string, appPath string, oc *build.OutputCapture) (string, error) {
		if filepath.Base(appPath) != "orphan-a" {
			return "", errors.New("fake toolchain: no build for orphan-b")
		}
		started <- struct{}{}
		// Stands in for a build step that does not watch its context.
		<-release
		oldCtxErr <- ctx.Err()
		oc.Printf("output line from the old build")
		binPath, err := GetBuilderAppExecutablePath(appPath)
		if err != nil {
			return "", err
		}
		return binPath, os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755)
	}
	var startsLock sync.Mutex
	var starts []string
	startAppProcess = func(cmd *exec.Cmd) error {
		startsLock.Lock()
		defer startsLock.Unlock()
		starts = append(starts, cmd.Path)
		return errors.New("test: app processes are not started")
	}
	t.Cleanup(func() { compileApp, startAppProcess = origCompile, origStart })

	old := GetOrCreateController(builderId)
	old.RequestRebuild("draft/orphan-a", nil)
	waitSignal(t, started, "the old build")
	DeleteController(builderId)

	fresh := GetOrCreateController(builderId)
	t.Cleanup(func() { DeleteController(builderId) })
	fresh.RequestRebuild("draft/orphan-b", nil)
	waitUntil(t, 2*time.Second, func() bool {
		return !fresh.isBuilding() && fresh.GetStatus().Status == BuilderStatus_Error
	}, "the fresh controller's build")
	time.Sleep(200 * time.Millisecond)
	before := rec.snapshot()
	if len(before) == 0 {
		t.Fatal("the fresh controller published nothing")
	}

	releaseBuild()
	waitUntil(t, 2*time.Second, func() bool { return !old.isBuilding() && !old.isStopping() }, "the old build and its Stop to finish")
	time.Sleep(300 * time.Millisecond)

	if after := rec.snapshot(); len(after) != len(before) {
		t.Fatalf("the deleted controller published %d events after the delete:\n%s", len(after)-len(before), describeEvents(after[len(before):]))
	}
	select {
	case err := <-oldCtxErr:
		if err == nil {
			t.Fatal("DeleteController did not cancel the running build's context")
		}
	default:
		t.Fatal("the old build never finished")
	}
	startsLock.Lock()
	gotStarts := append([]string(nil), starts...)
	startsLock.Unlock()
	if len(gotStarts) != 0 {
		t.Fatalf("the deleted controller's build started its app: %v", gotStarts)
	}
	status := fresh.GetStatus()
	if status.Status != BuilderStatus_Error || !strings.Contains(status.ErrorMsg, "orphan-b") {
		t.Fatalf("fresh controller status = %q / %q, want its own error", status.Status, status.ErrorMsg)
	}
	var lastStatus wshrpc.BuilderStatusData
	for _, event := range before {
		if data, ok := event.Data.(wshrpc.BuilderStatusData); ok {
			lastStatus = data
		}
	}
	if !strings.Contains(lastStatus.ErrorMsg, "orphan-b") {
		t.Fatalf("last published status = %+v, want the fresh controller's", lastStatus)
	}
}
