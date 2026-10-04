// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
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
