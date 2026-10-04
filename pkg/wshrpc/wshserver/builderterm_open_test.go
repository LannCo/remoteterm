// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func openTerm(builderId string, targetBlockId string, targetAction string) error {
	return WshServerImpl.OpenBuilderTerminalCommand(electronCtx(), wshrpc.CommandOpenBuilderTerminalData{
		BuilderId:     builderId,
		TargetBlockId: targetBlockId,
		TargetAction:  targetAction,
	})
}

func termCount(t *testing.T, tabId string) int {
	t.Helper()
	count := 0
	for _, block := range tabBlocks(t, tabId) {
		if block.Meta.GetString(remotetermobj.MetaKey_View, "") == "term" {
			count++
		}
	}
	return count
}

func TestOpenBuilderTerminalAppends(t *testing.T) {
	home := setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	if err := openTerm(builderId, "", ""); err != nil {
		t.Fatal(err)
	}
	blocks := tabBlocks(t, rtn.TabId)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	added := blocks[1]
	if added.Meta.GetString(remotetermobj.MetaKey_CmdCwd, "") != appDirFor(home, "demo") {
		t.Errorf("cwd = %q", added.Meta.GetString(remotetermobj.MetaKey_CmdCwd, ""))
	}
	actions := pendingActions(t, rtn.TabId)
	last := actions[len(actions)-1]
	if last.ActionType != rtcore.LayoutActionDataType_Insert || last.BlockId != added.OID || !last.Focused {
		t.Errorf("last action = %+v", last)
	}
}

func TestOpenBuilderTerminalSplitsADirectChild(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	first := tabBlocks(t, rtn.TabId)[0]
	if err := openTerm(builderId, first.OID, "splitdown"); err != nil {
		t.Fatal(err)
	}
	actions := pendingActions(t, rtn.TabId)
	last := actions[len(actions)-1]
	if last.ActionType != rtcore.LayoutActionDataType_SplitVertical || last.Position != "after" || last.TargetBlockId != first.OID || !last.Focused {
		t.Errorf("last action = %+v", last)
	}
}

func TestOpenBuilderTerminalRejectsBadTargetsWithoutWrites(t *testing.T) {
	setupBuilderApps(t, "demo", "demo2")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	first := tabBlocks(t, rtn.TabId)[0]
	sub, err := rtcore.CreateSubBlock(context.Background(), first.OID, termDef("local"))
	if err != nil {
		t.Fatal(err)
	}
	other := ensureTab(t, uuid.NewString(), "draft/demo2")
	foreign := tabBlocks(t, other.TabId)[0]
	rows := countRows(t)
	cases := []struct{ target, action string }{
		{sub.OID, "splitright"},
		{foreign.OID, "splitright"},
		{uuid.NewString(), "splitright"},
		{first.OID, "replace"},
		{"", "splitright"},
	}
	for _, tc := range cases {
		if err := openTerm(builderId, tc.target, tc.action); err == nil {
			t.Errorf("target %q action %q accepted", tc.target, tc.action)
		}
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
}

func TestOpenBuilderTerminalCapsTermBlocks(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	for range 7 {
		if err := openTerm(builderId, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	// Terminals that wsh creates through CreateBlockCommand count toward the cap too.
	for range 8 {
		if _, err := WshServerImpl.CreateBlockCommand(context.Background(), wshrpc.CommandCreateBlockData{TabId: rtn.TabId, BlockDef: termDef("local")}); err != nil {
			t.Fatal(err)
		}
	}
	if got := termCount(t, rtn.TabId); got != MaxBuilderTermBlocks {
		t.Fatalf("term blocks = %d, want %d", got, MaxBuilderTermBlocks)
	}
	rows := countRows(t)
	err := openTerm(builderId, "", "")
	if err == nil || err.Error() != "too many terminals in this builder (max 16)" {
		t.Fatalf("17th Open = %v", err)
	}
	if countRows(t) != rows {
		t.Fatal("a rejected Open wrote rows")
	}
}

func TestOpenBuilderTerminalWithoutTabIsNotReady(t *testing.T) {
	setupBuilderApps(t, "demo")
	rows := countRows(t)
	err := openTerm(uuid.NewString(), "", "")
	if err == nil || err.Error() != "builder terminal not ready" {
		t.Fatalf("err = %v, want builder terminal not ready", err)
	}
	if countRows(t) != rows {
		t.Fatal("Open without a tab wrote rows")
	}
}

func TestOpenBuilderTerminalRemovesBlockWhenQueueFails(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	failQueue(t)
	rows := countRows(t)
	if err := openTerm(builderId, "", ""); err == nil {
		t.Fatal("expected the queue failure")
	}
	if countRows(t) != rows || len(tabBlocks(t, rtn.TabId)) != 1 {
		t.Fatalf("rows %v -> %v; the new block was not removed", rows, countRows(t))
	}
}

func TestOpenBuilderTerminalUsesTabAppIdNotRtInfo(t *testing.T) {
	home := setupBuilderApps(t, "demo", "other")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": "draft/other"})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
	if err := openTerm(builderId, "", ""); err != nil {
		t.Fatal(err)
	}
	blocks := tabBlocks(t, rtn.TabId)
	if got := blocks[len(blocks)-1].Meta.GetString(remotetermobj.MetaKey_CmdCwd, ""); got != appDirFor(home, "demo") {
		t.Fatalf("cwd = %q, want the demo folder", got)
	}
}

func TestOpenBuilderTerminalRejectsNonElectronCallers(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	rows := countRows(t)
	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeControllerRouteId(uuid.NewString()),
		wshutil.MakeTabRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(builderId),
		"",
	} {
		err := WshServerImpl.OpenBuilderTerminalCommand(sourceCtx(source), wshrpc.CommandOpenBuilderTerminalData{BuilderId: builderId})
		if err == nil {
			t.Errorf("source %q accepted", source)
		}
	}
	if countRows(t) != rows {
		t.Fatal("a refused caller wrote rows")
	}
}

func TestOpenBuilderTerminalBroadcastsLayoutUpdate(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	tab, _ := rtstore.DBMustGet[*remotetermobj.Tab](context.Background(), rtn.TabId)
	wpsEvents.reset()
	if err := openTerm(builderId, "", ""); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, update := range wpsEvents.objUpdates() {
		if update.OType == remotetermobj.OType_LayoutState && update.OID == tab.LayoutState {
			found = true
		}
	}
	if !found {
		t.Fatal("no waveobj:update for the layout state")
	}
}

func TestOpenBuilderTerminalWaitsForBuilderLock(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	if err := runWhileBuilderLocked(t, builderId, func() error { return openTerm(builderId, "", "") }); err != nil {
		t.Fatal(err)
	}
	if n := len(tabBlocks(t, rtn.TabId)); n != 2 {
		t.Fatalf("blocks after the lock was released = %d, want 2", n)
	}
}

func TestOpenBuilderTerminalAppDirGone(t *testing.T) {
	home := setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	if err := os.RemoveAll(appDirFor(home, "demo")); err != nil {
		t.Fatal(err)
	}
	rows := countRows(t)
	err := openTerm(builderId, "", "")
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("err = %v, want the app folder reported as not available", err)
	}
	if countRows(t) != rows {
		t.Fatal("Open wrote rows for a missing app folder")
	}
}

func TestOpenBuilderTerminalConcurrentAtCap(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	for range MaxBuilderTermBlocks - 3 {
		if err := openTerm(builderId, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			openTerm(builderId, "", "")
		}()
	}
	wg.Wait()
	if got := termCount(t, rtn.TabId); got != MaxBuilderTermBlocks {
		t.Fatalf("term blocks = %d after concurrent Opens, want %d", got, MaxBuilderTermBlocks)
	}
}
