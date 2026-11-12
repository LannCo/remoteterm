// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

func tabIdsOf(tabs []*remotetermobj.Tab) []string {
	ids := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		ids = append(ids, tab.OID)
	}
	return ids
}

func TestCreateBuilderTab(t *testing.T) {
	ctx := context.Background()
	builderId := uuid.NewString()
	tab, err := CreateBuilderTab(ctx, builderId, "draft/demo")
	if err != nil {
		t.Fatal(err)
	}
	got, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if err != nil || got == nil {
		t.Fatalf("tab not stored: %v", err)
	}
	if got.Name != BuilderTabName || len(got.BlockIds) != 0 {
		t.Errorf("tab = %+v", got)
	}
	if got.Meta.GetString(rtstore.MetaKey_BuilderOwner, "") != builderId || got.Meta.GetString(rtstore.MetaKey_BuilderAppId, "") != "draft/demo" {
		t.Errorf("tab meta = %v", got.Meta)
	}
	if !objExists(t, remotetermobj.OType_LayoutState, got.LayoutState) {
		t.Error("layout state not stored")
	}
	wsId, err := rtstore.DBFindWorkspaceForTabId(ctx, tab.OID)
	if err != nil || wsId != "" {
		t.Errorf("builder tab joined workspace %q (%v)", wsId, err)
	}
	if !rtstore.IsBuilderTab(ctx, tab.OID) {
		t.Error("IsBuilderTab = false for a new builder tab")
	}
	if _, err := CreateBuilderTab(ctx, "", "draft/demo"); err == nil {
		t.Error("empty builder id accepted")
	}
	if _, err := CreateBuilderTab(ctx, builderId, ""); err == nil {
		t.Error("empty app id accepted")
	}
}

func TestFindBuilderTabs(t *testing.T) {
	ctx := context.Background()
	ownerA := uuid.NewString()
	tabA := makeTestBuilderTab(t, ownerA)
	tabB := makeTestBuilderTab(t, uuid.NewString())
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: ownerA})
	insertTestWorkspace(t, spoofed.OID)

	byOwner, err := FindBuilderTabs(ctx, ownerA)
	if err != nil || !slices.Equal(tabIdsOf(byOwner), []string{tabA.OID}) {
		t.Fatalf("FindBuilderTabs(owner) = %v, %v; want [%s]", tabIdsOf(byOwner), err, tabA.OID)
	}
	all, err := FindBuilderTabs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	ids := tabIdsOf(all)
	if !slices.Contains(ids, tabA.OID) || !slices.Contains(ids, tabB.OID) || slices.Contains(ids, spoofed.OID) {
		t.Fatalf("FindBuilderTabs(\"\") = %v", ids)
	}
}

func TestDeleteBuilderTabDeletesBlocksLayoutAndTab(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := makeTestBuilderTab(t, owner)
	b1 := addTestTermBlock(t, tab.OID)
	b2 := addTestTermBlock(t, tab.OID)

	if err := DeleteBuilderTab(ctx, tab.OID, owner); err != nil {
		t.Fatal(err)
	}
	if objExists(t, remotetermobj.OType_Tab, tab.OID) || objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Error("tab or layout state survived")
	}
	for _, b := range []*remotetermobj.Block{b1, b2} {
		if objExists(t, remotetermobj.OType_Block, b.OID) {
			t.Errorf("block %s survived", b.OID)
		}
		if !blockCloses.closed(b.OID) {
			t.Errorf("no BlockClose for %s", b.OID)
		}
	}
	if err := DeleteBuilderTab(ctx, tab.OID, owner); err != nil {
		t.Errorf("second delete = %v, want nil", err)
	}
}

func TestDeleteBuilderTabRefusesWorkspaceTabAndOtherOwner(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: owner})
	insertTestWorkspace(t, spoofed.OID)
	if err := DeleteBuilderTab(ctx, spoofed.OID, ""); err == nil {
		t.Error("a workspace tab carrying builder:owner was accepted")
	}
	if !objExists(t, remotetermobj.OType_Tab, spoofed.OID) {
		t.Fatal("workspace tab deleted")
	}

	tab := makeTestBuilderTab(t, owner)
	block := addTestTermBlock(t, tab.OID)
	if err := DeleteBuilderTab(ctx, tab.OID, uuid.NewString()); err == nil {
		t.Error("owner mismatch accepted")
	}
	if !objExists(t, remotetermobj.OType_Tab, tab.OID) || !objExists(t, remotetermobj.OType_Block, block.OID) {
		t.Fatal("owner mismatch deleted something")
	}
}

func TestDeleteBuilderTabKeepsTabWhenABlockFails(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := makeTestBuilderTab(t, owner)
	good := addTestTermBlock(t, tab.OID)
	bad := addTestTermBlock(t, tab.OID)
	// A sub-block id that no longer exists makes deleteBlockObj refuse the parent.
	bad.SubBlockIds = []string{uuid.NewString()}
	if err := rtstore.DBUpdate(ctx, bad); err != nil {
		t.Fatal(err)
	}

	if err := DeleteBuilderTab(ctx, tab.OID, owner); err == nil {
		t.Fatal("expected an error from the failing block")
	}
	if !objExists(t, remotetermobj.OType_Tab, tab.OID) || !objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Fatal("tab or layout state deleted although a block failed")
	}
	if objExists(t, remotetermobj.OType_Block, good.OID) || !objExists(t, remotetermobj.OType_Block, bad.OID) {
		t.Fatal("expected the good block gone and the failing block kept")
	}

	bad.SubBlockIds = nil
	if err := rtstore.DBUpdate(ctx, bad); err != nil {
		t.Fatal(err)
	}
	SweepBuilderTabs(ctx)
	if objExists(t, remotetermobj.OType_Tab, tab.OID) {
		t.Fatal("the sweep did not remove the tab after the block was fixed")
	}
}

func TestDeleteBuilderTabKeepsTabThatGainedABlock(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := makeTestBuilderTab(t, owner)
	block := addTestTermBlock(t, tab.OID)
	var late *remotetermobj.Block
	// A creator that does not hold the builder lock adds a pane while the teardown is running.
	blockCloses.setHook(func(blockId string) {
		if blockId != block.OID || late != nil {
			return
		}
		late = addTestTermBlock(t, tab.OID)
	})
	t.Cleanup(func() { blockCloses.setHook(nil) })

	if err := DeleteBuilderTab(ctx, tab.OID, owner); err == nil {
		t.Fatal("expected an error: the tab gained a block during the teardown")
	}
	blockCloses.setHook(nil)
	if late == nil {
		t.Fatal("the hook never ran")
	}
	if !objExists(t, remotetermobj.OType_Tab, tab.OID) || !objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Fatal("the tab was deleted under a live block")
	}
	if !objExists(t, remotetermobj.OType_Block, late.OID) {
		t.Fatal("the late block is gone")
	}
	SweepBuilderTabs(ctx)
	if objExists(t, remotetermobj.OType_Tab, tab.OID) || objExists(t, remotetermobj.OType_Block, late.OID) {
		t.Fatal("the sweep did not remove the tab and its late block")
	}
}

func TestSweepBuilderTabsRemovesBuilderTabsOnly(t *testing.T) {
	ctx := context.Background()
	makeTestBuilderTab(t, uuid.NewString())
	withBlock := makeTestBuilderTab(t, uuid.NewString())
	block := addTestTermBlock(t, withBlock.OID)
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: uuid.NewString()})
	insertTestWorkspace(t, spoofed.OID)

	before, err := FindBuilderTabs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var logBuf bytes.Buffer
	origLog := log.Writer()
	log.SetOutput(&logBuf)
	removed := SweepBuilderTabs(ctx)
	log.SetOutput(origLog)

	if removed != len(before) {
		t.Errorf("removed = %d, want %d", removed, len(before))
	}
	wantLine := fmt.Sprintf("[startup] builder sweep: removed %d tabs", len(before))
	if !strings.Contains(logBuf.String(), wantLine) {
		t.Errorf("log %q lacks %q", logBuf.String(), wantLine)
	}
	after, _ := FindBuilderTabs(ctx, "")
	if len(after) != 0 {
		t.Errorf("builder tabs left: %v", tabIdsOf(after))
	}
	if !blockCloses.closed(block.OID) {
		t.Error("no BlockClose for a swept block")
	}
	if !objExists(t, remotetermobj.OType_Tab, spoofed.OID) {
		t.Error("the sweep deleted a workspace tab")
	}
}
