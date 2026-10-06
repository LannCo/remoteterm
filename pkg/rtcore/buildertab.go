// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"fmt"
	"log"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

const BuilderTabName = "builder"

// The owner meta is written here, at insert: rtstore.UpdateObjectMeta refuses builder: keys on tabs.
func CreateBuilderTab(ctx context.Context, builderId string, appId string) (*remotetermobj.Tab, error) {
	if builderId == "" || appId == "" {
		return nil, fmt.Errorf("a builder tab needs a builder id and an app id")
	}
	layoutState := &remotetermobj.LayoutState{OID: uuid.NewString()}
	tab := &remotetermobj.Tab{
		OID:         uuid.NewString(),
		Name:        BuilderTabName,
		BlockIds:    []string{},
		LayoutState: layoutState.OID,
		Meta: remotetermobj.MetaMapType{
			rtstore.MetaKey_BuilderOwner: builderId,
			rtstore.MetaKey_BuilderAppId: appId,
		},
	}
	err := rtstore.WithTx(ctx, func(tx *rtstore.TxWrap) error {
		if err := rtstore.DBInsert(tx.Context(), tab); err != nil {
			return err
		}
		return rtstore.DBInsert(tx.Context(), layoutState)
	})
	if err != nil {
		return nil, fmt.Errorf("error creating builder tab: %w", err)
	}
	return tab, nil
}

func FindBuilderTabs(ctx context.Context, builderId string) ([]*remotetermobj.Tab, error) {
	tabIds, err := rtstore.DBFindTabIdsByBuilderOwner(ctx, builderId)
	if err != nil {
		return nil, fmt.Errorf("error finding builder tabs: %w", err)
	}
	rtn := make([]*remotetermobj.Tab, 0, len(tabIds))
	for _, tabId := range tabIds {
		if !rtstore.IsBuilderTab(ctx, tabId) {
			continue
		}
		tab, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tabId)
		if err != nil || tab == nil {
			continue
		}
		rtn = append(rtn, tab)
	}
	return rtn, nil
}

// A missing tab is a no-op. If any block cannot be deleted, the tab and its layout state are kept,
// so a later delete or the startup sweep can retry. Creators that skip the builder lock (wsh's
// CreateBlockCommand) can add a block mid-teardown; the final transaction re-reads the tab and keeps
// it in that case, rather than leaving the new block without a tab.
func DeleteBuilderTab(ctx context.Context, tabId string, expectedOwner string) error {
	tab, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil {
		return fmt.Errorf("error getting tab %s: %w", tabId, err)
	}
	if tab == nil {
		return nil
	}
	if !rtstore.IsBuilderTab(ctx, tabId) {
		return fmt.Errorf("tab %s is not a builder tab", tabId)
	}
	if expectedOwner != "" && tab.Meta.GetString(rtstore.MetaKey_BuilderOwner, "") != expectedOwner {
		return fmt.Errorf("tab %s belongs to another builder", tabId)
	}
	var firstErr error
	for _, blockId := range tab.BlockIds {
		err := DeleteBlock(ctx, blockId, false)
		if err == nil {
			continue
		}
		log.Printf("DeleteBuilderTab: error deleting block %s of tab %s: %v\n", blockId, tabId, err)
		if firstErr == nil {
			firstErr = fmt.Errorf("error deleting block %s: %w", blockId, err)
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return rtstore.WithTx(ctx, func(tx *rtstore.TxWrap) error {
		current, err := rtstore.DBGet[*remotetermobj.Tab](tx.Context(), tabId)
		if err != nil {
			return err
		}
		if current == nil {
			return nil
		}
		if len(current.BlockIds) > 0 {
			return fmt.Errorf("tab %s gained blocks while it was being deleted", tabId)
		}
		if err := rtstore.DBDelete(tx.Context(), remotetermobj.OType_LayoutState, current.LayoutState); err != nil {
			return err
		}
		return rtstore.DBDelete(tx.Context(), remotetermobj.OType_Tab, tabId)
	})
}

// Builder ids are fresh per window and a backend exit quits the app, so at startup every builder
// tab is an orphan. Per-tab errors are logged and do not stop the sweep.
func SweepBuilderTabs(ctx context.Context) int {
	tabs, err := FindBuilderTabs(ctx, "")
	if err != nil {
		log.Printf("[startup] builder sweep: %v\n", err)
	}
	removed := 0
	for _, tab := range tabs {
		if err := DeleteBuilderTab(ctx, tab.OID, ""); err != nil {
			log.Printf("[startup] builder sweep: could not remove tab %s: %v\n", tab.OID, err)
			continue
		}
		removed++
	}
	log.Printf("[startup] builder sweep: removed %d tabs\n", removed)
	return removed
}
