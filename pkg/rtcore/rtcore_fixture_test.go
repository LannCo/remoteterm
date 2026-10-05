// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/google/uuid"
)

// Publish is synchronous, so a hook set with setHook runs on the deleting goroutine, between that
// block's delete and the rest of the caller's work (pattern: jobcontroller_reconnect_test.go:43-75).
type blockCloseRecorder struct {
	lock     sync.Mutex
	blockIds []string
	hook     func(blockId string)
}

var blockCloses = &blockCloseRecorder{}

func (r *blockCloseRecorder) SendEvent(routeId string, ev wps.WaveEvent) {
	if ev.Event != wps.Event_BlockClose {
		return
	}
	blockId, ok := ev.Data.(string)
	if !ok {
		return
	}
	if hook := r.record(blockId); hook != nil {
		hook(blockId)
	}
}

func (r *blockCloseRecorder) record(blockId string) func(string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.blockIds = append(r.blockIds, blockId)
	return r.hook
}

func (r *blockCloseRecorder) setHook(hook func(blockId string)) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.hook = hook
}

func (r *blockCloseRecorder) closed(blockId string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	for _, id := range r.blockIds {
		if id == blockId {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rtcore-test-*")
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
	wps.Broker.Subscribe("rtcore-test", wps.SubscriptionRequest{Event: wps.Event_BlockClose, AllScopes: true})
	wps.Broker.SetClient(blockCloses)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func insertTestTab(t *testing.T, meta remotetermobj.MetaMapType) *remotetermobj.Tab {
	t.Helper()
	ctx := context.Background()
	layout := &remotetermobj.LayoutState{OID: uuid.NewString()}
	tab := &remotetermobj.Tab{OID: uuid.NewString(), Name: "t", BlockIds: []string{}, LayoutState: layout.OID, Meta: meta}
	if err := rtstore.DBInsert(ctx, tab); err != nil {
		t.Fatalf("insert tab: %v", err)
	}
	if err := rtstore.DBInsert(ctx, layout); err != nil {
		t.Fatalf("insert layout: %v", err)
	}
	return tab
}

func insertTestWorkspace(t *testing.T, tabIds ...string) *remotetermobj.Workspace {
	t.Helper()
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: tabIds}
	if err := rtstore.DBInsert(context.Background(), ws); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	return ws
}

func makeTestBuilderTab(t *testing.T, builderId string) *remotetermobj.Tab {
	t.Helper()
	tab, err := CreateBuilderTab(context.Background(), builderId, "draft/demo")
	if err != nil {
		t.Fatalf("CreateBuilderTab: %v", err)
	}
	return tab
}

func addTestTermBlock(t *testing.T, tabId string) *remotetermobj.Block {
	t.Helper()
	blockDef := &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
		remotetermobj.MetaKey_View:       "term",
		remotetermobj.MetaKey_Controller: "shell",
	}}
	block, err := CreateBlock(context.Background(), tabId, blockDef, nil)
	if err != nil {
		t.Fatalf("CreateBlock: %v", err)
	}
	return block
}

func objExists(t *testing.T, otype string, oid string) bool {
	t.Helper()
	found, err := rtstore.DBExistsORef(context.Background(), remotetermobj.MakeORef(otype, oid))
	if err != nil {
		t.Fatalf("exists %s:%s: %v", otype, oid, err)
	}
	return found
}
