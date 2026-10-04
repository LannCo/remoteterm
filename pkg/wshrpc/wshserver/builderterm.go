// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/LannCo/remoteterm/pkg/buildercontroller"
	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

const (
	MaxBuilderTermBlocks = 16
	BuilderWriteTimeout  = 15 * time.Second

	BuilderTargetAction_SplitRight = "splitright"
	BuilderTargetAction_SplitLeft  = "splitleft"
	BuilderTargetAction_SplitUp    = "splitup"
	BuilderTargetAction_SplitDown  = "splitdown"
)

var (
	builderLocksLock sync.Mutex
	builderLocks     = make(map[string]*sync.Mutex)
)

// Tests replace it to make queueing fail; nothing in production does, but the rollback paths depend on it.
var queueBuilderLayoutAction = rtcore.QueueLayoutActionForTab

// Panes reach the server through wsh on leaf links, whose source the router stamps as proc:<id>, so
// they cannot pass. Electron and the builder's own renderer are trusted links that assert their route.
func checkBuilderCaller(source string, builderId string, allowRenderer bool) error {
	parsed, err := uuid.Parse(builderId)
	if err != nil || parsed.String() != builderId {
		return fmt.Errorf("invalid builder id %q", builderId)
	}
	if source == wshutil.ElectronRoute {
		return nil
	}
	if allowRenderer && source == wshutil.MakeBuilderRouteId(builderId) {
		return nil
	}
	return fmt.Errorf("builder terminal commands are not available to %q", source)
}

// Entries are never removed: there is one per builder window per process, and the caller check stops
// panes minting ids. Removing them would let a waiter hold a lock that a newcomer no longer sees.
func getBuilderLock(builderId string) *sync.Mutex {
	builderLocksLock.Lock()
	defer builderLocksLock.Unlock()
	lock := builderLocks[builderId]
	if lock == nil {
		lock = &sync.Mutex{}
		builderLocks[builderId] = lock
	}
	return lock
}

func withBuilderLock(builderId string, fn func() error) error {
	lock := getBuilderLock(builderId)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

// Writes and their rollbacks run detached from the RPC context, so an expiring request cannot strand a
// half-created tab. The update map it carries is not goroutine-safe: use the context from one goroutine.
func makeBuilderWriteContext() (context.Context, context.CancelFunc) {
	ctx, cancelFn := context.WithTimeout(context.Background(), BuilderWriteTimeout)
	return remotetermobj.ContextWithUpdates(ctx), cancelFn
}

func isValidBuilderTargetAction(targetAction string) bool {
	switch targetAction {
	case "", BuilderTargetAction_SplitRight, BuilderTargetAction_SplitLeft, BuilderTargetAction_SplitUp, BuilderTargetAction_SplitDown:
		return true
	}
	return false
}

// The split strings and positions match CreateBlockCommand (wshserver.go), so wsh users and the builder
// get the same geometry.
func makeBuilderLayoutAction(blockId string, targetBlockId string, targetAction string) remotetermobj.LayoutActionData {
	action := remotetermobj.LayoutActionData{BlockId: blockId, Focused: true}
	switch targetAction {
	case BuilderTargetAction_SplitRight:
		action.ActionType = rtcore.LayoutActionDataType_SplitHorizontal
		action.Position = "after"
	case BuilderTargetAction_SplitLeft:
		action.ActionType = rtcore.LayoutActionDataType_SplitHorizontal
		action.Position = "before"
	case BuilderTargetAction_SplitUp:
		action.ActionType = rtcore.LayoutActionDataType_SplitVertical
		action.Position = "before"
	case BuilderTargetAction_SplitDown:
		action.ActionType = rtcore.LayoutActionDataType_SplitVertical
		action.Position = "after"
	default:
		action.ActionType = rtcore.LayoutActionDataType_Insert
		return action
	}
	action.TargetBlockId = targetBlockId
	return action
}

// Sent once per write phase, after success and after a rollback. Without it the renderer never sees new
// panes' layout actions or the tab's deletion. Updates are kept per object, last one wins, for every
// committed store call; a store transaction that fails adds none. So when a write phase undoes its own
// earlier writes (deleting a half-created tab or block), the broadcast carries the deletes for them.
func sendBuilderUpdates(writeCtx context.Context) {
	wps.Broker.SendUpdateEvents(remotetermobj.ContextGetUpdatesRtn(writeCtx))
}

// Electron fills both fields from the calling window (its builder id and its app id), never from the page.
func (ws *WshServer) EnsureBuilderTabCommand(ctx context.Context, data wshrpc.CommandEnsureBuilderTabData) (*wshrpc.CommandEnsureBuilderTabRtnData, error) {
	if err := checkBuilderCaller(wshutil.GetRpcSourceFromContext(ctx), data.BuilderId, false); err != nil {
		return nil, err
	}
	if err := remotetermappstore.ValidateAppId(data.AppId); err != nil {
		return nil, fmt.Errorf("invalid app id %q: %w", data.AppId, err)
	}
	var rtn *wshrpc.CommandEnsureBuilderTabRtnData
	err := withBuilderLock(data.BuilderId, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		writeCtx, cancelFn := makeBuilderWriteContext()
		defer cancelFn()
		defer sendBuilderUpdates(writeCtx)
		var err error
		rtn, err = ensureBuilderTab(writeCtx, data.BuilderId, data.AppId)
		return err
	})
	if err != nil {
		return nil, err
	}
	return rtn, nil
}

func ensureBuilderTab(ctx context.Context, builderId string, appId string) (*wshrpc.CommandEnsureBuilderTabRtnData, error) {
	tabs, err := rtcore.FindBuilderTabs(ctx, builderId)
	if err != nil {
		return nil, err
	}
	var current *remotetermobj.Tab
	for _, tab := range tabs {
		if current == nil && tab.Meta.GetString(rtstore.MetaKey_BuilderAppId, "") == appId {
			current = tab
			continue
		}
		if err := rtcore.DeleteBuilderTab(ctx, tab.OID, builderId); err != nil {
			return nil, fmt.Errorf("error removing the terminals of a previous app: %w", err)
		}
	}
	if current != nil {
		return &wshrpc.CommandEnsureBuilderTabRtnData{TabId: current.OID, AppId: appId}, nil
	}
	appDir, err := buildercontroller.ResolveAppDirForAppId(appId)
	if err != nil {
		return nil, err
	}
	tab, err := rtcore.CreateBuilderTab(ctx, builderId, appId)
	if err != nil {
		return nil, err
	}
	if _, err := addBuilderTermBlock(ctx, tab.OID, appDir, "", ""); err != nil {
		if delErr := rtcore.DeleteBuilderTab(ctx, tab.OID, builderId); delErr != nil {
			log.Printf("EnsureBuilderTabCommand: could not roll back tab %s: %v\n", tab.OID, delErr)
		}
		return nil, err
	}
	return &wshrpc.CommandEnsureBuilderTabRtnData{TabId: tab.OID, AppId: appId}, nil
}

// rtcore.CreateBlock and the queue are called directly, not through CreateBlockCommand, which
// broadcasts only on success and cannot roll back.
func addBuilderTermBlock(ctx context.Context, tabId string, appDir string, targetBlockId string, targetAction string) (string, error) {
	blockData, err := rtcore.CreateBlock(ctx, tabId, buildercontroller.MakeBuilderTerminalBlockDef(appDir), nil)
	if err != nil {
		return "", fmt.Errorf("error creating terminal: %w", err)
	}
	err = queueBuilderLayoutAction(ctx, tabId, makeBuilderLayoutAction(blockData.OID, targetBlockId, targetAction))
	if err != nil {
		if delErr := rtcore.DeleteBlock(ctx, blockData.OID, false); delErr != nil {
			log.Printf("builder terminal: could not roll back block %s: %v\n", blockData.OID, delErr)
		}
		return "", fmt.Errorf("error queuing layout action: %w", err)
	}
	return blockData.OID, nil
}
