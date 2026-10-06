// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { newLayoutNode } from "./layoutNode";
import {
    LayoutNode,
    LayoutTreeActionType,
    LayoutTreeInsertNodeAction,
    LayoutTreeSplitHorizontalAction,
    LayoutTreeSplitVerticalAction,
} from "./types";

// A queued split whose target was closed meanwhile still carries a live block; dropping the action
// would leave that block (and its shell) with no node, so it is inserted instead.
export function makeBackendSplitAction(
    action: LayoutActionData,
    targetNode: LayoutNode
): LayoutTreeSplitHorizontalAction | LayoutTreeSplitVerticalAction | LayoutTreeInsertNodeAction {
    const newNode = newLayoutNode(undefined, action.nodesize, undefined, { blockId: action.blockid });
    if (targetNode == null) {
        const insertAction: LayoutTreeInsertNodeAction = {
            type: LayoutTreeActionType.InsertNode,
            node: newNode,
            magnified: false,
            focused: action.focused,
        };
        return insertAction;
    }
    const position = action.position as "before" | "after";
    if (action.actiontype === LayoutTreeActionType.SplitVertical) {
        const verticalAction: LayoutTreeSplitVerticalAction = {
            type: LayoutTreeActionType.SplitVertical,
            targetNodeId: targetNode.id,
            newNode,
            position,
            focused: action.focused,
        };
        return verticalAction;
    }
    const horizontalAction: LayoutTreeSplitHorizontalAction = {
        type: LayoutTreeActionType.SplitHorizontal,
        targetNodeId: targetNode.id,
        newNode,
        position,
        focused: action.focused,
    };
    return horizontalAction;
}
