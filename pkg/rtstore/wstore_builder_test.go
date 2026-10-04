// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rtstore-test-*")
	if err != nil {
		fmt.Printf("cannot create a test data dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, remotetermbase.WaveDBDir), 0755); err != nil {
		fmt.Printf("cannot create the db dir: %v\n", err)
		os.Exit(1)
	}
	remotetermbase.DataHome_VarCache = dir
	if err := InitWStore(); err != nil {
		fmt.Printf("wstore: %v\n", err)
		os.Exit(1)
	}
	if err := filestore.InitFilestore(); err != nil {
		fmt.Printf("filestore: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func insertTestTab(t *testing.T, meta remotetermobj.MetaMapType) *remotetermobj.Tab {
	t.Helper()
	tab := &remotetermobj.Tab{OID: uuid.NewString(), Name: "t", BlockIds: []string{}, LayoutState: uuid.NewString(), Meta: meta}
	if err := DBInsert(context.Background(), tab); err != nil {
		t.Fatalf("insert tab: %v", err)
	}
	return tab
}

func insertTestWorkspace(t *testing.T, tabIds ...string) *remotetermobj.Workspace {
	t.Helper()
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: tabIds}
	if err := DBInsert(context.Background(), ws); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	return ws
}

func insertTestBlock(t *testing.T, parentORef string, meta remotetermobj.MetaMapType) *remotetermobj.Block {
	t.Helper()
	block := &remotetermobj.Block{OID: uuid.NewString(), ParentORef: parentORef, Meta: meta}
	if err := DBInsert(context.Background(), block); err != nil {
		t.Fatalf("insert block: %v", err)
	}
	return block
}

func builderMeta(owner string) remotetermobj.MetaMapType {
	return remotetermobj.MetaMapType{MetaKey_BuilderOwner: owner, MetaKey_BuilderAppId: "draft/demo"}
}

func tabORef(tabId string) remotetermobj.ORef {
	return remotetermobj.MakeORef(remotetermobj.OType_Tab, tabId)
}

func blockORef(blockId string) remotetermobj.ORef {
	return remotetermobj.MakeORef(remotetermobj.OType_Block, blockId)
}

func TestIsBuilderTab(t *testing.T) {
	ctx := context.Background()
	builderTab := insertTestTab(t, builderMeta(uuid.NewString()))
	if !IsBuilderTab(ctx, builderTab.OID) {
		t.Error("a tab with builder:owner and no workspace is a builder tab")
	}
	spoofed := insertTestTab(t, builderMeta(uuid.NewString()))
	insertTestWorkspace(t, spoofed.OID)
	if IsBuilderTab(ctx, spoofed.OID) {
		t.Error("a workspace tab carrying builder:owner must not count as a builder tab")
	}
	plain := insertTestTab(t, nil)
	if IsBuilderTab(ctx, plain.OID) {
		t.Error("a tab without builder:owner is not a builder tab")
	}
	if IsBuilderTab(ctx, uuid.NewString()) {
		t.Error("a missing tab is not a builder tab")
	}
}

func TestDBFindTabIdsByBuilderOwner(t *testing.T) {
	ctx := context.Background()
	ownerA := uuid.NewString()
	tabA := insertTestTab(t, builderMeta(ownerA))
	tabB := insertTestTab(t, builderMeta(uuid.NewString()))
	plain := insertTestTab(t, remotetermobj.MetaMapType{"tab:background": "red"})

	got, err := DBFindTabIdsByBuilderOwner(ctx, ownerA)
	if err != nil || !slices.Equal(got, []string{tabA.OID}) {
		t.Fatalf("by owner = %v, %v; want [%s]", got, err, tabA.OID)
	}
	all, err := DBFindTabIdsByBuilderOwner(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(all, tabA.OID) || !slices.Contains(all, tabB.OID) || slices.Contains(all, plain.OID) {
		t.Fatalf("any owner = %v; want %s and %s, not %s", all, tabA.OID, tabB.OID, plain.OID)
	}
}

func TestUpdateObjectMetaRejectsBuilderKeysOnTabs(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := insertTestTab(t, builderMeta(owner))
	for _, patch := range []remotetermobj.MetaMapType{
		{MetaKey_BuilderOwner: "someone-else"},
		{MetaKey_BuilderAppId: "draft/other"},
		{"builder:*": true},
		{MetaKey_BuilderOwner: nil},
		{"builder:anything": "x", "tab:background": "red"},
	} {
		if err := UpdateObjectMeta(ctx, tabORef(tab.OID), patch, false); err == nil {
			t.Errorf("patch %v was accepted", patch)
		}
	}
	got, _ := DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if got.Meta.GetString(MetaKey_BuilderOwner, "") != owner || got.Meta.GetString("tab:background", "") != "" {
		t.Fatalf("tab meta changed: %v", got.Meta)
	}
	if err := UpdateObjectMeta(ctx, tabORef(tab.OID), remotetermobj.MetaMapType{"tab:background": "red"}, false); err != nil {
		t.Fatalf("ordinary tab meta rejected: %v", err)
	}
	got, _ = DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if got.Meta.GetString("tab:background", "") != "red" || got.Meta.GetString(MetaKey_BuilderOwner, "") != owner {
		t.Fatalf("tab meta after ordinary write = %v", got.Meta)
	}
}

func TestUpdateObjectMetaDoesNotGuardBuilderKeysOnBlocks(t *testing.T) {
	block := insertTestBlock(t, tabORef(insertTestTab(t, nil).OID).String(), remotetermobj.MetaMapType{"view": "term"})
	if err := UpdateObjectMeta(context.Background(), blockORef(block.OID), remotetermobj.MetaMapType{"builder:note": "x"}, false); err != nil {
		t.Fatalf("block meta write rejected: %v", err)
	}
}

func TestUpdateObjectMetaLocalOnlyForBuilderBlocks(t *testing.T) {
	ctx := context.Background()
	builderTab := insertTestTab(t, builderMeta(uuid.NewString()))
	block := insertTestBlock(t, tabORef(builderTab.OID).String(), remotetermobj.MetaMapType{"view": "term"})
	subBlock := insertTestBlock(t, blockORef(block.OID).String(), remotetermobj.MetaMapType{"view": "term"})

	for _, conn := range []string{"user@host", "wsl://Ubuntu"} {
		patch := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: conn}
		if err := UpdateObjectMeta(ctx, blockORef(block.OID), patch, false); !errors.Is(err, ErrBuilderLocalOnly) {
			t.Errorf("block connection %q: err = %v, want ErrBuilderLocalOnly", conn, err)
		}
		if err := UpdateObjectMeta(ctx, blockORef(subBlock.OID), patch, false); !errors.Is(err, ErrBuilderLocalOnly) {
			t.Errorf("sub-block connection %q: err = %v, want ErrBuilderLocalOnly", conn, err)
		}
	}
	for _, conn := range []any{"local", "", "local:zsh", nil} {
		patch := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: conn}
		if err := UpdateObjectMeta(ctx, blockORef(block.OID), patch, false); err != nil {
			t.Errorf("local connection %v rejected: %v", conn, err)
		}
	}
	if err := UpdateObjectMeta(ctx, blockORef(block.OID), remotetermobj.MetaMapType{"term:fontsize": 12}, false); err != nil {
		t.Errorf("non-connection write rejected: %v", err)
	}

	wsTab := insertTestTab(t, nil)
	insertTestWorkspace(t, wsTab.OID)
	wsBlock := insertTestBlock(t, tabORef(wsTab.OID).String(), remotetermobj.MetaMapType{"view": "term"})
	if err := UpdateObjectMeta(ctx, blockORef(wsBlock.OID), remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: "user@host"}, false); err != nil {
		t.Errorf("remote connection rejected for a workspace block: %v", err)
	}
}

func TestUpdateObjectMetaConnectionOnMissingBlockIsNotFound(t *testing.T) {
	patch := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: "user@host"}
	err := UpdateObjectMeta(context.Background(), blockORef(uuid.NewString()), patch, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (the connection check must not poison the transaction)", err)
	}
}
