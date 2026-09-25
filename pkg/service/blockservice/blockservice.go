// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockservice

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/LannCo/remoteterm/pkg/blockcontroller"
	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/tsgen/tsgenmeta"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/google/uuid"
)

type BlockService struct{}

const DefaultTimeout = 2 * time.Second

var BlockServiceInstance = &BlockService{}

func (bs *BlockService) SendCommand_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "send command to block",
		ArgNames: []string{"blockid", "cmd"},
	}
}

func (bs *BlockService) GetControllerStatus(ctx context.Context, blockId string) (*blockcontroller.BlockControllerRuntimeStatus, error) {
	return blockcontroller.GetBlockControllerRuntimeStatus(blockId), nil
}

func (*BlockService) SaveTerminalState_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "save the terminal state to a blockfile",
		ArgNames: []string{"ctx", "blockId", "state", "stateType", "ptyOffset", "termSize", "decModes"},
	}
}

func (bs *BlockService) SaveTerminalState(ctx context.Context, blockId string, state string, stateType string, ptyOffset int64, termSize remotetermobj.TermSize, decModes string) error {
	_, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, blockId)
	if err != nil {
		return err
	}
	if stateType != "full" && stateType != "preview" {
		return fmt.Errorf("invalid state type: %q", stateType)
	}
	// ignore MakeFile error (already exists is ok)
	filestore.WFS.MakeFile(ctx, blockId, "cache:term:"+stateType, nil, wshrpc.FileOpts{})
	err = filestore.WFS.WriteFile(ctx, blockId, "cache:term:"+stateType, []byte(state))
	if err != nil {
		return fmt.Errorf("cannot save terminal state: %w", err)
	}
	fileMeta := wshrpc.FileMeta{
		"ptyoffset": ptyOffset,
		"termsize":  termSize,
	}
	// Always write decmodes (even empty string) so stale values are overwritten
	fileMeta["decmodes"] = decModes
	err = filestore.WFS.WriteMeta(ctx, blockId, "cache:term:"+stateType, fileMeta, true)
	if err != nil {
		return fmt.Errorf("cannot save terminal state meta: %w", err)
	}
	return nil
}

func (*BlockService) CleanupOrphanedBlocks_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "queue a layout action to cleanup orphaned blocks in the tab",
		ArgNames: []string{"ctx", "tabId"},
	}
}

func (bs *BlockService) CleanupOrphanedBlocks(ctx context.Context, tabId string) (remotetermobj.UpdatesRtnType, error) {
	ctx = remotetermobj.ContextWithUpdates(ctx)
	layoutAction := remotetermobj.LayoutActionData{
		ActionType: rtcore.LayoutActionDataType_CleanupOrphaned,
		ActionId:   uuid.NewString(),
	}
	err := rtcore.QueueLayoutActionForTab(ctx, tabId, layoutAction)
	if err != nil {
		return nil, fmt.Errorf("error queuing cleanup layout action: %w", err)
	}
	return remotetermobj.ContextGetUpdatesRtn(ctx), nil
}

func (*BlockService) SaveTerminalImages_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "save image manifest for terminal state restore",
		ArgNames: []string{"ctx", "blockId", "manifest"},
	}
}

func (bs *BlockService) SaveTerminalImages(ctx context.Context, blockId string, manifest string) error {
	_, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, blockId)
	if err != nil {
		return err
	}
	filestore.WFS.MakeFile(ctx, blockId, "cache:term:images", nil, wshrpc.FileOpts{})
	return filestore.WFS.WriteFile(ctx, blockId, "cache:term:images", []byte(manifest))
}

func (*BlockService) SaveImageAsset_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "save a single image asset (content-addressed)",
		ArgNames: []string{"ctx", "blockId", "name", "data"},
	}
}

var hexNamePattern = regexp.MustCompile(`^[0-9a-f]{1,32}$`)

func (bs *BlockService) SaveImageAsset(ctx context.Context, blockId string, name string, data string) error {
	if !hexNamePattern.MatchString(name) {
		return fmt.Errorf("invalid image asset name: %q", name)
	}
	_, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, blockId)
	if err != nil {
		return err
	}
	fileName := "cache:term:img:" + name
	filestore.WFS.MakeFile(ctx, blockId, fileName, nil, wshrpc.FileOpts{})
	return filestore.WFS.WriteFile(ctx, blockId, fileName, []byte(data))
}
