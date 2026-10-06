// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/service/objectservice"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/google/uuid"
)

func termDef(connection string) *remotetermobj.BlockDef {
	return &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
		remotetermobj.MetaKey_View:       "term",
		remotetermobj.MetaKey_Controller: "shell",
		remotetermobj.MetaKey_Connection: connection,
	}}
}

func TestReservedTabMetaThroughSetMetaAndObjectService(t *testing.T) {
	setupBuilderApps(t, "demo")
	ctx := context.Background()
	rtn := ensureTab(t, uuid.NewString(), "draft/demo")
	tabRef := remotetermobj.MakeORef(remotetermobj.OType_Tab, rtn.TabId)
	for _, patch := range []remotetermobj.MetaMapType{
		{rtstore.MetaKey_BuilderOwner: uuid.NewString()},
		{"builder:*": true},
		{rtstore.MetaKey_BuilderOwner: nil},
	} {
		if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: tabRef, Meta: patch}); err == nil {
			t.Errorf("SetMetaCommand accepted %v", patch)
		}
		if _, err := (&objectservice.ObjectService{}).UpdateObjectMeta(remotetermobj.UIContext{}, tabRef.String(), patch); err == nil {
			t.Errorf("ObjectService.UpdateObjectMeta accepted %v", patch)
		}
	}
	if !rtstore.IsBuilderTab(ctx, rtn.TabId) {
		t.Fatal("the builder tab lost its owner")
	}
	if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: tabRef, Meta: remotetermobj.MetaMapType{"tab:background": "red"}}); err != nil {
		t.Errorf("ordinary tab meta rejected: %v", err)
	}
}

func TestBuilderBlocksStayLocalThroughRpcs(t *testing.T) {
	setupBuilderApps(t, "demo")
	ctx := context.Background()
	rtn := ensureTab(t, uuid.NewString(), "draft/demo")
	block := tabBlocks(t, rtn.TabId)[0]
	blockRef := remotetermobj.MakeORef(remotetermobj.OType_Block, block.OID)
	remote := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: "user@host"}

	if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: blockRef, Meta: remote}); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Errorf("SetMetaCommand = %v, want ErrBuilderLocalOnly", err)
	}
	if _, err := WshServerImpl.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{TabId: rtn.TabId, BlockDef: termDef("user@host")}); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Errorf("CreateBlockCommand = %v, want ErrBuilderLocalOnly", err)
	}
	if _, err := WshServerImpl.CreateSubBlockCommand(ctx, wshrpc.CommandCreateSubBlockData{ParentBlockId: block.OID, BlockDef: termDef("user@host")}); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Errorf("CreateSubBlockCommand = %v, want ErrBuilderLocalOnly", err)
	}
	if len(tabBlocks(t, rtn.TabId)) != 1 {
		t.Fatal("a rejected block was added")
	}

	layout := &remotetermobj.LayoutState{OID: uuid.NewString()}
	normal := &remotetermobj.Tab{OID: uuid.NewString(), BlockIds: []string{}, LayoutState: layout.OID}
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: []string{normal.OID}}
	for _, obj := range []remotetermobj.WaveObj{layout, normal, ws} {
		if err := rtstore.DBInsert(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	created, err := WshServerImpl.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{TabId: normal.OID, BlockDef: termDef("user@host")})
	if err != nil {
		t.Fatalf("remote block rejected in a workspace tab: %v", err)
	}
	if _, err := WshServerImpl.CreateSubBlockCommand(ctx, wshrpc.CommandCreateSubBlockData{ParentBlockId: created.OID, BlockDef: termDef("user@host")}); err != nil {
		t.Fatalf("remote sub-block rejected in a workspace tab: %v", err)
	}
	if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: *created, Meta: remote}); err != nil {
		t.Fatalf("remote connection rejected for a workspace block: %v", err)
	}
}
