// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

func TestDeleteBlockLastBlockOfBuilderTabKeepsTab(t *testing.T) {
	ctx := context.Background()
	tab := makeTestBuilderTab(t, uuid.NewString())
	block := addTestTermBlock(t, tab.OID)
	if err := DeleteBlock(ctx, block.OID, true); err != nil {
		t.Fatalf("DeleteBlock = %v, want nil", err)
	}
	if !blockCloses.closed(block.OID) {
		t.Error("no BlockClose for the last builder pane")
	}
	got, _ := rtstore.DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if got == nil || len(got.BlockIds) != 0 {
		t.Fatalf("builder tab = %+v, want present with no blocks", got)
	}
	if !objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Error("layout state deleted")
	}
}

func TestDeleteBlockPublishesBlockCloseWhenCascadeFails(t *testing.T) {
	ctx := context.Background()
	orphan := insertTestTab(t, nil)
	block := addTestTermBlock(t, orphan.OID)
	if err := DeleteBlock(ctx, block.OID, true); err == nil {
		t.Fatal("expected the cascade to fail for a tab that belongs to no workspace")
	}
	if objExists(t, remotetermobj.OType_Block, block.OID) {
		t.Error("block row survived")
	}
	if !blockCloses.closed(block.OID) {
		t.Fatal("a cascade error skipped BlockClose, so the shell would keep running")
	}
}

func TestUpdateWorkspaceTabIdsRejectsBuilderTab(t *testing.T) {
	ctx := context.Background()
	normal := insertTestTab(t, nil)
	ws := insertTestWorkspace(t, normal.OID)
	builderTab := makeTestBuilderTab(t, uuid.NewString())
	if err := UpdateWorkspaceTabIds(ctx, ws.OID, []string{normal.OID, builderTab.OID}); err == nil {
		t.Fatal("a builder tab joined a workspace")
	}
	got, _ := rtstore.DBGet[*remotetermobj.Workspace](ctx, ws.OID)
	if !slices.Equal(got.TabIds, []string{normal.OID}) {
		t.Fatalf("workspace tabids = %v", got.TabIds)
	}
	if !rtstore.IsBuilderTab(ctx, builderTab.OID) {
		t.Error("builder tab lost its builder status")
	}
}

func TestUpdateWorkspaceTabIdsKeepsReorderingWithSpoofedTab(t *testing.T) {
	ctx := context.Background()
	normal := insertTestTab(t, nil)
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: uuid.NewString()})
	ws := insertTestWorkspace(t, normal.OID, spoofed.OID)
	if err := UpdateWorkspaceTabIds(ctx, ws.OID, []string{spoofed.OID, normal.OID}); err != nil {
		t.Fatalf("reorder rejected: %v", err)
	}
	got, _ := rtstore.DBGet[*remotetermobj.Workspace](ctx, ws.OID)
	if !slices.Equal(got.TabIds, []string{spoofed.OID, normal.OID}) {
		t.Fatalf("workspace tabids = %v", got.TabIds)
	}
}

func TestCreateBlockRejectsRemoteConnectionInBuilderTab(t *testing.T) {
	ctx := context.Background()
	tab := makeTestBuilderTab(t, uuid.NewString())
	remoteDef := func() *remotetermobj.BlockDef {
		return &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
			remotetermobj.MetaKey_View:       "term",
			remotetermobj.MetaKey_Controller: "shell",
			remotetermobj.MetaKey_Connection: "user@host",
		}}
	}
	localDef := func() *remotetermobj.BlockDef {
		return &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
			remotetermobj.MetaKey_View:       "term",
			remotetermobj.MetaKey_Controller: "shell",
			remotetermobj.MetaKey_Connection: "local",
		}}
	}

	if _, err := CreateBlock(ctx, tab.OID, remoteDef(), nil); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Fatalf("CreateBlock remote = %v, want ErrBuilderLocalOnly", err)
	}
	got, _ := rtstore.DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if len(got.BlockIds) != 0 {
		t.Fatalf("a rejected block was added: %v", got.BlockIds)
	}
	parent, err := CreateBlock(ctx, tab.OID, localDef(), nil)
	if err != nil {
		t.Fatalf("CreateBlock local = %v", err)
	}
	if _, err := CreateSubBlock(ctx, parent.OID, remoteDef()); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Fatalf("CreateSubBlock remote = %v, want ErrBuilderLocalOnly", err)
	}
	if _, err := CreateSubBlock(ctx, parent.OID, localDef()); err != nil {
		t.Fatalf("CreateSubBlock local = %v", err)
	}

	wsTab := insertTestTab(t, nil)
	insertTestWorkspace(t, wsTab.OID)
	if _, err := CreateBlock(ctx, wsTab.OID, remoteDef(), nil); err != nil {
		t.Fatalf("remote block rejected in a workspace tab: %v", err)
	}
}
