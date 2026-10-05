// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "blockcontroller-test-*")
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
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func insertEnvTestTabAndBlock(t *testing.T, meta remotetermobj.MetaMapType) (*remotetermobj.Tab, *remotetermobj.Block) {
	t.Helper()
	ctx := context.Background()
	tab := &remotetermobj.Tab{OID: uuid.NewString(), BlockIds: []string{}, LayoutState: uuid.NewString(), Meta: meta}
	if err := rtstore.DBInsert(ctx, tab); err != nil {
		t.Fatal(err)
	}
	block := &remotetermobj.Block{
		OID:        uuid.NewString(),
		ParentORef: remotetermobj.MakeORef(remotetermobj.OType_Tab, tab.OID).String(),
		Meta:       remotetermobj.MetaMapType{"view": "term"},
	}
	if err := rtstore.DBInsert(ctx, block); err != nil {
		t.Fatal(err)
	}
	return tab, block
}

func TestAddTabAndWorkspaceEnvOmitsEmptyWorkspaceId(t *testing.T) {
	ctx := context.Background()
	builderTab, builderBlock := insertEnvTestTabAndBlock(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: uuid.NewString()})
	env := map[string]string{}
	addTabAndWorkspaceEnv(ctx, env, builderBlock.OID)
	if env[remotetermbase.WaveTabIdVarName] != builderTab.OID || env[remotetermbase.LegacyWaveTabIdVarName] != builderTab.OID {
		t.Errorf("tab id env = %v", env)
	}
	for _, name := range []string{remotetermbase.WaveWorkspaceIdVarName, remotetermbase.LegacyWaveWorkspaceIdVarName} {
		if val, ok := env[name]; ok {
			t.Errorf("%s set to %q for a builder pane", name, val)
		}
	}

	wsTab, wsBlock := insertEnvTestTabAndBlock(t, nil)
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: []string{wsTab.OID}}
	if err := rtstore.DBInsert(ctx, ws); err != nil {
		t.Fatal(err)
	}
	wsEnv := map[string]string{}
	addTabAndWorkspaceEnv(ctx, wsEnv, wsBlock.OID)
	if wsEnv[remotetermbase.WaveWorkspaceIdVarName] != ws.OID || wsEnv[remotetermbase.LegacyWaveWorkspaceIdVarName] != ws.OID {
		t.Errorf("workspace env = %v, want %s", wsEnv, ws.OID)
	}
}
