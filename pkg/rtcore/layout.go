// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

const (
	LayoutActionDataType_Insert          = "insert"
	LayoutActionDataType_InsertAtIndex   = "insertatindex"
	LayoutActionDataType_Remove          = "delete"
	LayoutActionDataType_ClearTree       = "clear"
	LayoutActionDataType_Replace         = "replace"
	LayoutActionDataType_SplitHorizontal = "splithorizontal"
	LayoutActionDataType_SplitVertical   = "splitvertical"
	LayoutActionDataType_CleanupOrphaned = "cleanuporphaned"
)

type PortableLayout []struct {
	IndexArr []int                   `json:"indexarr"`
	Size     *uint                   `json:"size,omitempty"`
	BlockDef *remotetermobj.BlockDef `json:"blockdef"`
	Focused  bool                    `json:"focused"`
}

func GetStarterLayout() PortableLayout {
	return PortableLayout{
		{IndexArr: []int{0}, BlockDef: &remotetermobj.BlockDef{
			Meta: remotetermobj.MetaMapType{
				remotetermobj.MetaKey_View:       "term",
				remotetermobj.MetaKey_Controller: "shell",
			},
		}, Focused: true},
		{IndexArr: []int{1}, BlockDef: &remotetermobj.BlockDef{
			Meta: remotetermobj.MetaMapType{
				remotetermobj.MetaKey_View: "sysinfo",
			},
		}},
		{IndexArr: []int{1, 1}, BlockDef: &remotetermobj.BlockDef{
			Meta: remotetermobj.MetaMapType{
				remotetermobj.MetaKey_View: "web",
				remotetermobj.MetaKey_Url:  "https://github.com/LannCo/remoteterm",
			},
		}},
		{IndexArr: []int{1, 2}, BlockDef: &remotetermobj.BlockDef{
			Meta: remotetermobj.MetaMapType{
				remotetermobj.MetaKey_View: "preview",
				remotetermobj.MetaKey_File: "~",
			},
		}},
	}
}

func GetNewTabLayout(connName string) PortableLayout {
	blockMeta := remotetermobj.MetaMapType{
		remotetermobj.MetaKey_View:       "term",
		remotetermobj.MetaKey_Controller: "shell",
	}
	if connName != "" {
		blockMeta[remotetermobj.MetaKey_Connection] = connName
	}
	return PortableLayout{
		{IndexArr: []int{0}, BlockDef: &remotetermobj.BlockDef{
			Meta: blockMeta,
		}, Focused: true},
	}
}

func GetLayoutIdForTab(ctx context.Context, tabId string) (string, error) {
	tabObj, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil {
		return "", fmt.Errorf("unable to get layout id for given tab id %s: %w", tabId, err)
	}
	return tabObj.LayoutState, nil
}

func QueueLayoutAction(ctx context.Context, layoutStateId string, actions ...remotetermobj.LayoutActionData) error {
	layoutStateObj, err := rtstore.DBGet[*remotetermobj.LayoutState](ctx, layoutStateId)
	if err != nil {
		return fmt.Errorf("unable to get layout state for given id %s: %w", layoutStateId, err)
	}

	for i := range actions {
		if actions[i].ActionId == "" {
			actions[i].ActionId = uuid.New().String()
		}
	}

	if layoutStateObj.PendingBackendActions == nil {
		layoutStateObj.PendingBackendActions = &actions
	} else {
		*layoutStateObj.PendingBackendActions = append(*layoutStateObj.PendingBackendActions, actions...)
	}

	err = rtstore.DBUpdate(ctx, layoutStateObj)
	if err != nil {
		return fmt.Errorf("unable to update layout state with new actions: %w", err)
	}
	return nil
}

func QueueLayoutActionForTab(ctx context.Context, tabId string, actions ...remotetermobj.LayoutActionData) error {
	layoutStateId, err := GetLayoutIdForTab(ctx, tabId)
	if err != nil {
		return err
	}

	return QueueLayoutAction(ctx, layoutStateId, actions...)
}

func ApplyPortableLayout(ctx context.Context, tabId string, layout PortableLayout) error {
	actions := make([]remotetermobj.LayoutActionData, len(layout)+1)
	actions[0] = remotetermobj.LayoutActionData{ActionType: LayoutActionDataType_ClearTree}
	for i := 0; i < len(layout); i++ {
		layoutAction := layout[i]

		blockData, err := CreateBlock(ctx, tabId, layoutAction.BlockDef, &remotetermobj.RuntimeOpts{})
		if err != nil {
			return fmt.Errorf("unable to create block to apply portable layout to tab %s: %w", tabId, err)
		}

		actions[i+1] = remotetermobj.LayoutActionData{
			ActionType: LayoutActionDataType_InsertAtIndex,
			BlockId:    blockData.OID,
			IndexArr:   &layoutAction.IndexArr,
			NodeSize:   layoutAction.Size,
			Focused:    layoutAction.Focused,
		}
	}

	err := QueueLayoutActionForTab(ctx, tabId, actions...)
	if err != nil {
		return fmt.Errorf("unable to queue layout actions for portable layout: %w", err)
	}

	return nil
}

func BootstrapStarterLayout(ctx context.Context) error {
	ctx, cancelFn := context.WithTimeout(ctx, 2*time.Second)
	defer cancelFn()
	client, err := rtstore.DBGetSingleton[*remotetermobj.Client](ctx)
	if err != nil {
		log.Printf("unable to find client: %v\n", err)
		return fmt.Errorf("unable to find client: %w", err)
	}

	if len(client.WindowIds) < 1 {
		return fmt.Errorf("error bootstrapping layout, no windows exist")
	}

	windowId := client.WindowIds[0]

	window, err := rtstore.DBMustGet[*remotetermobj.Window](ctx, windowId)
	if err != nil {
		return fmt.Errorf("error getting window: %w", err)
	}

	workspace, err := rtstore.DBMustGet[*remotetermobj.Workspace](ctx, window.WorkspaceId)
	if err != nil {
		return fmt.Errorf("error getting workspace: %w", err)
	}

	tabId := workspace.ActiveTabId

	starterLayout := GetStarterLayout()
	err = ApplyPortableLayout(ctx, tabId, starterLayout)
	if err != nil {
		return fmt.Errorf("error applying starter layout: %w", err)
	}

	return nil
}
