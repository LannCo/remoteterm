// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"fmt"
	"log"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/util/utilfn"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/google/uuid"
)

func CreateSubBlock(ctx context.Context, blockId string, blockDef *remotetermobj.BlockDef) (*remotetermobj.Block, error) {
	if blockDef == nil {
		return nil, fmt.Errorf("blockDef is nil")
	}
	if blockDef.Meta == nil || blockDef.Meta.GetString(remotetermobj.MetaKey_View, "") == "" {
		return nil, fmt.Errorf("no view provided for new block")
	}
	blockData, err := createSubBlockObj(ctx, blockId, blockDef)
	if err != nil {
		return nil, fmt.Errorf("error creating sub block: %w", err)
	}
	return blockData, nil
}

func createSubBlockObj(ctx context.Context, parentBlockId string, blockDef *remotetermobj.BlockDef) (*remotetermobj.Block, error) {
	return rtstore.WithTxRtn(ctx, func(tx *rtstore.TxWrap) (*remotetermobj.Block, error) {
		parentBlock, _ := rtstore.DBGet[*remotetermobj.Block](tx.Context(), parentBlockId)
		if parentBlock == nil {
			return nil, fmt.Errorf("parent block not found: %q", parentBlockId)
		}
		blockId := uuid.NewString()
		blockData := &remotetermobj.Block{
			OID:         blockId,
			ParentORef:  remotetermobj.MakeORef(remotetermobj.OType_Block, parentBlockId).String(),
			RuntimeOpts: nil,
			Meta:        blockDef.Meta,
		}
		rtstore.DBInsert(tx.Context(), blockData)
		parentBlock.SubBlockIds = append(parentBlock.SubBlockIds, blockId)
		rtstore.DBUpdate(tx.Context(), parentBlock)
		return blockData, nil
	})
}

func CreateBlock(ctx context.Context, tabId string, blockDef *remotetermobj.BlockDef, rtOpts *remotetermobj.RuntimeOpts) (rtnBlock *remotetermobj.Block, rtnErr error) {
	var blockCreated bool
	var newBlockOID string
	defer func() {
		if rtnErr == nil {
			return
		}
		// if there was an error, and we created the block, clean it up since the function failed
		if blockCreated && newBlockOID != "" {
			deleteBlockObj(ctx, newBlockOID)
			filestore.WFS.DeleteZone(ctx, newBlockOID)
		}
	}()
	if blockDef == nil {
		return nil, fmt.Errorf("blockDef is nil")
	}
	if blockDef.Meta == nil || blockDef.Meta.GetString(remotetermobj.MetaKey_View, "") == "" {
		return nil, fmt.Errorf("no view provided for new block")
	}
	blockData, err := createBlockObj(ctx, tabId, blockDef, rtOpts)
	if err != nil {
		return nil, fmt.Errorf("error creating block: %w", err)
	}
	blockCreated = true
	newBlockOID = blockData.OID
	// upload the files if present
	if len(blockDef.Files) > 0 {
		for fileName, fileDef := range blockDef.Files {
			err := filestore.WFS.MakeFile(ctx, newBlockOID, fileName, fileDef.Meta, wshrpc.FileOpts{})
			if err != nil {
				return nil, fmt.Errorf("error making blockfile %q: %w", fileName, err)
			}
			err = filestore.WFS.WriteFile(ctx, newBlockOID, fileName, []byte(fileDef.Content))
			if err != nil {
				return nil, fmt.Errorf("error writing blockfile %q: %w", fileName, err)
			}
		}
	}
	return blockData, nil
}

func createBlockObj(ctx context.Context, tabId string, blockDef *remotetermobj.BlockDef, rtOpts *remotetermobj.RuntimeOpts) (*remotetermobj.Block, error) {
	return rtstore.WithTxRtn(ctx, func(tx *rtstore.TxWrap) (*remotetermobj.Block, error) {
		tab, _ := rtstore.DBGet[*remotetermobj.Tab](tx.Context(), tabId)
		if tab == nil {
			return nil, fmt.Errorf("tab not found: %q", tabId)
		}
		blockId := uuid.NewString()
		blockData := &remotetermobj.Block{
			OID:         blockId,
			ParentORef:  remotetermobj.MakeORef(remotetermobj.OType_Tab, tabId).String(),
			RuntimeOpts: rtOpts,
			Meta:        blockDef.Meta,
		}
		rtstore.DBInsert(tx.Context(), blockData)
		tab.BlockIds = append(tab.BlockIds, blockId)
		rtstore.DBUpdate(tx.Context(), tab)
		return blockData, nil
	})
}

// Must delete all blocks individually first.
// Also deletes LayoutState.
// recursive: if true, will recursively close parent tab, window, workspace, if they are empty.
// Returns new active tab id, error.
func DeleteBlock(ctx context.Context, blockId string, recursive bool) error {
	block, err := rtstore.DBGet[*remotetermobj.Block](ctx, blockId)
	if err != nil {
		return fmt.Errorf("error getting block: %w", err)
	}
	if block == nil {
		return nil
	}
	if len(block.SubBlockIds) > 0 {
		for _, subBlockId := range block.SubBlockIds {
			err := DeleteBlock(ctx, subBlockId, recursive)
			if err != nil {
				return fmt.Errorf("error deleting subblock %s: %w", subBlockId, err)
			}
		}
	}
	parentBlockCount, err := deleteBlockObj(ctx, blockId)
	if err != nil {
		return fmt.Errorf("error deleting block: %w", err)
	}
	log.Printf("DeleteBlock: parentBlockCount: %d", parentBlockCount)
	parentORef := remotetermobj.ParseORefNoErr(block.ParentORef)

	if recursive && parentORef.OType == remotetermobj.OType_Tab && parentBlockCount == 0 {
		// if parent tab has no blocks, delete the tab
		log.Printf("DeleteBlock: parent tab has no blocks, deleting tab %s", parentORef.OID)
		parentWorkspaceId, err := rtstore.DBFindWorkspaceForTabId(ctx, parentORef.OID)
		if err != nil {
			return fmt.Errorf("error finding workspace for tab to delete %s: %w", parentORef.OID, err)
		}
		newActiveTabId, err := DeleteTab(ctx, parentWorkspaceId, parentORef.OID, true)
		if err != nil {
			return fmt.Errorf("error deleting tab %s: %w", parentORef.OID, err)
		}
		SendActiveTabUpdate(ctx, parentWorkspaceId, newActiveTabId)
	}
	sendBlockCloseEvent(blockId)
	return nil
}

// returns the updated block count for the parent object
func deleteBlockObj(ctx context.Context, blockId string) (int, error) {
	return rtstore.WithTxRtn(ctx, func(tx *rtstore.TxWrap) (int, error) {
		block, err := rtstore.DBGet[*remotetermobj.Block](tx.Context(), blockId)
		if err != nil {
			return -1, fmt.Errorf("error getting block: %w", err)
		}
		if block == nil {
			return -1, fmt.Errorf("block not found: %q", blockId)
		}
		if len(block.SubBlockIds) > 0 {
			return -1, fmt.Errorf("block has subblocks, must delete subblocks first")
		}
		parentORef := remotetermobj.ParseORefNoErr(block.ParentORef)
		parentBlockCount := -1
		if parentORef != nil {
			if parentORef.OType == remotetermobj.OType_Tab {
				tab, _ := rtstore.DBGet[*remotetermobj.Tab](tx.Context(), parentORef.OID)
				if tab != nil {
					tab.BlockIds = utilfn.RemoveElemFromSlice(tab.BlockIds, blockId)
					rtstore.DBUpdate(tx.Context(), tab)
					parentBlockCount = len(tab.BlockIds)
				}
			} else if parentORef.OType == remotetermobj.OType_Block {
				parentBlock, _ := rtstore.DBGet[*remotetermobj.Block](tx.Context(), parentORef.OID)
				if parentBlock != nil {
					parentBlock.SubBlockIds = utilfn.RemoveElemFromSlice(parentBlock.SubBlockIds, blockId)
					rtstore.DBUpdate(tx.Context(), parentBlock)
					parentBlockCount = len(parentBlock.SubBlockIds)
				}
			}
		}
		rtstore.DBDelete(tx.Context(), remotetermobj.OType_Block, blockId)

		// Clean up block runtime info
		blockORef := remotetermobj.MakeORef(remotetermobj.OType_Block, blockId)
		rtstore.DeleteRTInfo(blockORef)

		return parentBlockCount, nil
	})
}

func sendBlockCloseEvent(blockId string) {
	waveEvent := wps.WaveEvent{
		Event: wps.Event_BlockClose,
		Scopes: []string{
			remotetermobj.MakeORef(remotetermobj.OType_Block, blockId).String(),
		},
		Data: blockId,
	}
	wps.Broker.Publish(waveEvent)
}
