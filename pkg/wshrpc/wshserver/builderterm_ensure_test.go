// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func TestEnsureBuilderTabCreatesOneTerminal(t *testing.T) {
	home := setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	if rtn.AppId != "draft/demo" || rtn.TabId == "" {
		t.Fatalf("rtn = %+v", rtn)
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || tabs[0].OID != rtn.TabId {
		t.Fatalf("builder tabs = %d, want exactly the returned one", len(tabs))
	}
	blocks := tabBlocks(t, rtn.TabId)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	meta := blocks[0].Meta
	if meta.GetString(remotetermobj.MetaKey_CmdCwd, "") != appDirFor(home, "demo") || meta.GetString(remotetermobj.MetaKey_View, "") != "term" || meta.GetString(remotetermobj.MetaKey_Connection, "") != "local" {
		t.Errorf("block meta = %v", meta)
	}
	if durable, ok := meta[remotetermobj.MetaKey_TermDurable].(bool); !ok || durable {
		t.Errorf("term:durable = %#v, want false", meta[remotetermobj.MetaKey_TermDurable])
	}
	actions := pendingActions(t, rtn.TabId)
	if len(actions) != 1 || actions[0].ActionType != rtcore.LayoutActionDataType_Insert || actions[0].BlockId != blocks[0].OID || !actions[0].Focused {
		t.Errorf("pending actions = %+v", actions)
	}
}

func TestEnsureBuilderTabIsIdempotent(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	first := ensureTab(t, builderId, "draft/demo")
	rows := countRows(t)
	second := ensureTab(t, builderId, "draft/demo")
	if second.TabId != first.TabId {
		t.Fatalf("second Ensure returned %s, want %s", second.TabId, first.TabId)
	}
	if countRows(t) != rows {
		t.Fatalf("second Ensure wrote rows: %v -> %v", rows, countRows(t))
	}
}

func TestEnsureBuilderTabConcurrentCallsCreateOneTab(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	var wg sync.WaitGroup
	results := make([]string, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rtn, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"})
			if err == nil {
				results[i] = rtn.TabId
			}
		}()
	}
	wg.Wait()
	if results[0] == "" || results[0] != results[1] {
		t.Fatalf("results = %v, want the same tab twice", results)
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || len(tabBlocks(t, tabs[0].OID)) != 1 {
		t.Fatalf("got %d tabs", len(tabs))
	}
}

func TestEnsureBuilderTabRejectsBadAppsWithoutWrites(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rows := countRows(t)
	for _, appId := range []string{"", "draft", "draft/../x", "Draft/demo", "draft/missing"} {
		if _, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: appId}); err == nil {
			t.Errorf("app id %q accepted", appId)
		}
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
}

func TestEnsureBuilderTabUsesAppIdArgumentNotRtInfo(t *testing.T) {
	home := setupBuilderApps(t, "demo", "other")
	builderId := uuid.NewString()
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": "draft/other"})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
	rtn := ensureTab(t, builderId, "draft/demo")
	if got := tabBlocks(t, rtn.TabId)[0].Meta.GetString(remotetermobj.MetaKey_CmdCwd, ""); got != appDirFor(home, "demo") {
		t.Fatalf("cmd:cwd = %q, want the demo folder", got)
	}
}

func TestEnsureBuilderTabReplacesTabForAnotherApp(t *testing.T) {
	setupBuilderApps(t, "demo", "demo2")
	builderId := uuid.NewString()
	first := ensureTab(t, builderId, "draft/demo")
	firstBlock := tabBlocks(t, first.TabId)[0]
	second := ensureTab(t, builderId, "draft/demo2")
	if second.TabId == first.TabId || second.AppId != "draft/demo2" {
		t.Fatalf("second = %+v", second)
	}
	found, _ := rtstore.DBExistsORef(context.Background(), remotetermobj.MakeORef(remotetermobj.OType_Tab, first.TabId))
	if found {
		t.Fatal("the tab for the previous app survived")
	}
	if !wpsEvents.blockClosed(firstBlock.OID) {
		t.Error("the previous app's shell was not closed")
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || tabs[0].OID != second.TabId {
		t.Fatalf("builder tabs = %d", len(tabs))
	}
}

func TestEnsureBuilderTabRollsBackWhenQueueFails(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	failQueue(t)
	rows := countRows(t)
	if _, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"}); err == nil {
		t.Fatal("expected the queue failure")
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 0 {
		t.Fatalf("a half-created tab was left: %d", len(tabs))
	}
}

func TestEnsureBuilderTabRejectsNonElectronCallers(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rows := countRows(t)
	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeControllerRouteId(uuid.NewString()),
		wshutil.MakeTabRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(builderId),
		wshutil.MakeBuilderRouteId(uuid.NewString()),
		"",
	} {
		if _, err := WshServerImpl.EnsureBuilderTabCommand(sourceCtx(source), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"}); err == nil {
			t.Errorf("source %q accepted", source)
		}
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
}

func TestEnsureBuilderTabReturnsWhenRpcContextIsDone(t *testing.T) {
	setupBuilderApps(t, "demo")
	ctx, cancelFn := context.WithCancel(electronCtx())
	cancelFn()
	rows := countRows(t)
	_, err := WshServerImpl.EnsureBuilderTabCommand(ctx, wshrpc.CommandEnsureBuilderTabData{BuilderId: uuid.NewString(), AppId: "draft/demo"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if countRows(t) != rows {
		t.Fatal("a cancelled Ensure wrote rows")
	}
}

func TestEnsureBuilderTabWaitsForBuilderLock(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	err := runWhileBuilderLocked(t, builderId, func() error {
		_, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 {
		t.Fatalf("builder tabs after the lock was released = %d, want 1", len(tabs))
	}
}

func TestEnsureBuilderTabBroadcastsUpdates(t *testing.T) {
	setupBuilderApps(t, "demo")
	wpsEvents.reset()
	rtn := ensureTab(t, uuid.NewString(), "draft/demo")
	tab, _ := rtstore.DBMustGet[*remotetermobj.Tab](context.Background(), rtn.TabId)
	seen := map[string]bool{}
	for _, update := range wpsEvents.objUpdates() {
		seen[update.OType+":"+update.OID] = true
	}
	for _, want := range []string{"tab:" + rtn.TabId, "layout:" + tab.LayoutState, "block:" + tab.BlockIds[0]} {
		if !seen[want] {
			t.Errorf("no waveobj:update for %s", want)
		}
	}
}
