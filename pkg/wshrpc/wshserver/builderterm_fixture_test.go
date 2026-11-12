// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
)

type wpsRecorder struct {
	lock   sync.Mutex
	events []wps.WaveEvent
}

var wpsEvents = &wpsRecorder{}

func (r *wpsRecorder) SendEvent(routeId string, ev wps.WaveEvent) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.events = append(r.events, ev)
}

func (r *wpsRecorder) reset() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.events = nil
}

func (r *wpsRecorder) objUpdates() []remotetermobj.WaveObjUpdate {
	r.lock.Lock()
	defer r.lock.Unlock()
	var rtn []remotetermobj.WaveObjUpdate
	for _, ev := range r.events {
		if ev.Event != wps.Event_WaveObjUpdate {
			continue
		}
		if update, ok := ev.Data.(remotetermobj.WaveObjUpdate); ok {
			rtn = append(rtn, update)
		}
	}
	return rtn
}

func (r *wpsRecorder) blockClosed(blockId string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	for _, ev := range r.events {
		if ev.Event == wps.Event_BlockClose && ev.Data == blockId {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wshserver-test-*")
	if err != nil {
		fmt.Printf("cannot create a test data dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, remotetermbase.WaveDBDir), 0755); err != nil {
		fmt.Printf("cannot create the db dir: %v\n", err)
		os.Exit(1)
	}
	remotetermbase.DataHome_VarCache = dir
	if err := rtstore.InitWStore(); err != nil {
		fmt.Printf("wstore: %v\n", err)
		os.Exit(1)
	}
	if err := filestore.InitFilestore(); err != nil {
		fmt.Printf("filestore: %v\n", err)
		os.Exit(1)
	}
	wps.Broker.Subscribe("wshserver-test", wps.SubscriptionRequest{Event: wps.Event_WaveObjUpdate, AllScopes: true})
	wps.Broker.Subscribe("wshserver-test", wps.SubscriptionRequest{Event: wps.Event_BlockClose, AllScopes: true})
	wps.Broker.SetClient(wpsEvents)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func electronCtx() context.Context {
	return wshutil.MakeRpcSourceContextForTest(context.Background(), wshutil.ElectronRoute)
}

func sourceCtx(source string) context.Context {
	return wshutil.MakeRpcSourceContextForTest(context.Background(), source)
}

func setupBuilderApps(t *testing.T, names ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range names {
		if err := os.MkdirAll(appDirFor(home, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func appDirFor(home string, name string) string {
	return filepath.Join(home, "waveapps", "draft", name)
}

// countRows returns the number of tab, block and layout rows.
func countRows(t *testing.T) [3]int {
	t.Helper()
	ctx := context.Background()
	tabs, err1 := rtstore.DBGetCount[*remotetermobj.Tab](ctx)
	blocks, err2 := rtstore.DBGetCount[*remotetermobj.Block](ctx)
	layouts, err3 := rtstore.DBGetCount[*remotetermobj.LayoutState](ctx)
	if err := errors.Join(err1, err2, err3); err != nil {
		t.Fatal(err)
	}
	return [3]int{tabs, blocks, layouts}
}

func tabBlocks(t *testing.T, tabId string) []*remotetermobj.Block {
	t.Helper()
	ctx := context.Background()
	tab, err := rtstore.DBMustGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil {
		t.Fatalf("tab %s: %v", tabId, err)
	}
	var rtn []*remotetermobj.Block
	for _, blockId := range tab.BlockIds {
		block, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, blockId)
		if err != nil {
			t.Fatalf("block %s: %v", blockId, err)
		}
		rtn = append(rtn, block)
	}
	return rtn
}

func pendingActions(t *testing.T, tabId string) []remotetermobj.LayoutActionData {
	t.Helper()
	ctx := context.Background()
	tab, err := rtstore.DBMustGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil {
		t.Fatalf("tab %s: %v", tabId, err)
	}
	layout, err := rtstore.DBMustGet[*remotetermobj.LayoutState](ctx, tab.LayoutState)
	if err != nil {
		t.Fatalf("layout %s: %v", tab.LayoutState, err)
	}
	if layout.PendingBackendActions == nil {
		return nil
	}
	return *layout.PendingBackendActions
}

func ensureTab(t *testing.T, builderId string, appId string) *wshrpc.CommandEnsureBuilderTabRtnData {
	t.Helper()
	rtn, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: appId})
	if err != nil {
		t.Fatalf("EnsureBuilderTabCommand(%s, %s): %v", builderId, appId, err)
	}
	return rtn
}

// runWhileBuilderLocked holds the builder's lock, starts op, and checks that op neither returns nor writes
// a row within 100 ms; then it releases the lock and returns op's result. A handler that skipped the lock
// fails here deterministically.
func runWhileBuilderLocked(t *testing.T, builderId string, op func() error) error {
	t.Helper()
	lock := getBuilderLock(builderId)
	lock.Lock()
	rows := countRows(t)
	done := make(chan error, 1)
	go func() {
		done <- op()
	}()
	select {
	case err := <-done:
		lock.Unlock()
		t.Fatalf("returned while the builder lock was held (err: %v)", err)
	case <-time.After(100 * time.Millisecond):
	}
	if got := countRows(t); got != rows {
		lock.Unlock()
		t.Fatalf("rows changed while the builder lock was held: %v -> %v", rows, got)
	}
	lock.Unlock()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("did not finish within 5 s of the lock being released")
	}
	return nil
}

func failQueue(t *testing.T) {
	t.Helper()
	orig := queueBuilderLayoutAction
	queueBuilderLayoutAction = func(ctx context.Context, tabId string, actions ...remotetermobj.LayoutActionData) error {
		return errors.New("queue unavailable")
	}
	t.Cleanup(func() { queueBuilderLayoutAction = orig })
}
