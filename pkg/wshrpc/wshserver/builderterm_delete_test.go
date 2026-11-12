// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/buildercontroller"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func builderTabCount(t *testing.T, builderId string) int {
	t.Helper()
	tabs, err := rtcore.FindBuilderTabs(context.Background(), builderId)
	if err != nil {
		t.Fatal(err)
	}
	return len(tabs)
}

func TestDeleteBuilderCommandRemovesEveryOwnerTab(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	block := tabBlocks(t, rtn.TabId)[0]
	if _, err := rtcore.CreateBuilderTab(context.Background(), builderId, "draft/other"); err != nil {
		t.Fatal(err)
	}
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId); err != nil {
		t.Fatal(err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left", n)
	}
	if !wpsEvents.blockClosed(block.OID) {
		t.Error("no BlockClose for the builder's shell")
	}
}

func TestDeleteBuilderCommandCompletesWhenCallerContextIsCancelled(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	ctx, cancelFn := context.WithCancel(electronCtx())
	cancelFn()
	if err := WshServerImpl.DeleteBuilderCommand(ctx, builderId); err != nil {
		t.Fatal(err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left after a cancelled caller", n)
	}
}

func TestDeleteBuilderCommandAcceptsElectronAndOwnRendererOnly(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeControllerRouteId(uuid.NewString()),
		wshutil.MakeTabRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(uuid.NewString()),
		"",
	} {
		if err := WshServerImpl.DeleteBuilderCommand(sourceCtx(source), builderId); err == nil {
			t.Errorf("source %q accepted", source)
		}
	}
	if n := builderTabCount(t, builderId); n != 1 {
		t.Fatalf("a refused caller removed tabs (%d left)", n)
	}
	if err := WshServerImpl.DeleteBuilderCommand(sourceCtx(wshutil.MakeBuilderRouteId(builderId)), builderId); err != nil {
		t.Fatalf("the builder's own renderer was refused: %v", err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left", n)
	}
}

func TestDeleteBuilderCommandBroadcastsTabDelete(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	wpsEvents.reset()
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId); err != nil {
		t.Fatal(err)
	}
	for _, update := range wpsEvents.objUpdates() {
		if update.OType == remotetermobj.OType_Tab && update.OID == rtn.TabId && update.UpdateType == remotetermobj.UpdateType_Delete {
			return
		}
	}
	t.Fatal("no waveobj:update delete for the builder tab")
}

func TestDeleteBuilderCommandDeletesController(t *testing.T) {
	builderId := uuid.NewString()
	buildercontroller.GetOrCreateController(builderId)
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId); err != nil {
		t.Fatal(err)
	}
	if buildercontroller.GetController(builderId) != nil {
		t.Fatal("the builder controller survived")
	}
}

func TestDeleteBuilderCommandWaitsForBuilderLock(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	err := runWhileBuilderLocked(t, builderId, func() error {
		return WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left after the lock was released", n)
	}
}

func TestBuilderLockSerialisesDeleteAndEnsures(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	base := countRows(t)
	ensureTab(t, builderId, "draft/demo")
	ensureOp := func() {
		WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"})
	}
	deleteOp := func() {
		WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId)
	}
	lock := getBuilderLock(builderId)
	lock.Lock()
	var wg sync.WaitGroup
	for _, op := range []func(){ensureOp, deleteOp, ensureOp} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op()
		}()
	}
	time.Sleep(50 * time.Millisecond)
	lock.Unlock()
	wg.Wait()

	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) > 1 {
		t.Fatalf("%d builder tabs after interleaved Ensure/Delete", len(tabs))
	}
	// Every surviving tab owns exactly one block and one layout state; anything else is a leaked row.
	k := len(tabs)
	if got, want := countRows(t), [3]int{base[0] + k, base[1] + k, base[2] + k}; got != want {
		t.Fatalf("rows after the interleave = %v, want %v (%d tabs)", got, want, k)
	}
	final := ensureTab(t, builderId, "draft/demo")
	tabs, _ = rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || tabs[0].OID != final.TabId || len(tabBlocks(t, final.TabId)) != 1 {
		t.Fatalf("after a final Ensure: %d tabs", len(tabs))
	}
	if got, want := countRows(t), [3]int{base[0] + 1, base[1] + 1, base[2] + 1}; got != want {
		t.Fatalf("rows after the final Ensure = %v, want %v", got, want)
	}
}

func TestDeleteBuilderCommandLeavesOtherBuilder(t *testing.T) {
	setupBuilderApps(t, "demo", "demo2")
	builderA := uuid.NewString()
	builderB := uuid.NewString()
	ensureTab(t, builderA, "draft/demo")
	rtnB := ensureTab(t, builderB, "draft/demo2")
	blockB := tabBlocks(t, rtnB.TabId)[0]
	wpsEvents.reset()
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderA); err != nil {
		t.Fatal(err)
	}
	if builderTabCount(t, builderB) != 1 || len(tabBlocks(t, rtnB.TabId)) != 1 {
		t.Fatal("closing one builder touched the other's terminals")
	}
	if wpsEvents.blockClosed(blockB.OID) {
		t.Fatal("closing one builder closed the other's shell")
	}
}

func TestDeleteBlockCommandOnLastBuilderPaneKeepsTab(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	block := tabBlocks(t, rtn.TabId)[0]
	if err := WshServerImpl.DeleteBlockCommand(context.Background(), wshrpc.CommandDeleteBlockData{BlockId: block.OID}); err != nil {
		t.Fatalf("DeleteBlockCommand = %v", err)
	}
	if builderTabCount(t, builderId) != 1 || len(tabBlocks(t, rtn.TabId)) != 0 {
		t.Fatal("expected the builder tab kept with no panes")
	}
	if !wpsEvents.blockClosed(block.OID) {
		t.Error("no BlockClose for the closed pane")
	}
	removed := false
	for _, action := range pendingActions(t, rtn.TabId) {
		if action.ActionType == rtcore.LayoutActionDataType_Remove && action.BlockId == block.OID {
			removed = true
		}
	}
	if !removed {
		t.Error("no layout delete action queued for the closed pane")
	}
}
