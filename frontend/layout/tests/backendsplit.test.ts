// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { makeBackendSplitAction } from "../lib/backendsplit";
import { newLayoutNode } from "../lib/layoutNode";
import { insertNode, splitHorizontal, splitVertical } from "../lib/layoutTree";
import {
    LayoutTreeActionType,
    LayoutTreeInsertNodeAction,
    LayoutTreeSplitHorizontalAction,
    LayoutTreeSplitVerticalAction,
} from "../lib/types";
import { newLayoutTreeState } from "./model";

function backendAction(actiontype: string, position: string, focused = true): LayoutActionData {
    return {
        actiontype,
        actionid: "action-1",
        blockid: "b2",
        focused,
        magnified: false,
        ephemeral: false,
        targetblockid: "b1",
        position,
    };
}

describe("makeBackendSplitAction", () => {
    it("keeps focused on a horizontal split and focuses the new node", () => {
        const target = newLayoutNode(undefined, undefined, undefined, { blockId: "b1" });
        const state = newLayoutTreeState(target);
        const treeAction = makeBackendSplitAction(backendAction("splithorizontal", "after"), target);
        expect(treeAction.type).toBe(LayoutTreeActionType.SplitHorizontal);
        splitHorizontal(state, treeAction as LayoutTreeSplitHorizontalAction);
        const newNode = (treeAction as LayoutTreeSplitHorizontalAction).newNode;
        expect(newNode.data.blockId).toBe("b2");
        expect(state.focusedNodeId).toBe(newNode.id);
    });

    it("keeps focused on a vertical split", () => {
        const target = newLayoutNode(undefined, undefined, undefined, { blockId: "b1" });
        const state = newLayoutTreeState(target);
        const treeAction = makeBackendSplitAction(backendAction("splitvertical", "before"), target);
        expect(treeAction.type).toBe(LayoutTreeActionType.SplitVertical);
        expect((treeAction as LayoutTreeSplitVerticalAction).position).toBe("before");
        splitVertical(state, treeAction as LayoutTreeSplitVerticalAction);
        expect(state.focusedNodeId).toBe((treeAction as LayoutTreeSplitVerticalAction).newNode.id);
    });

    it("does not focus when the backend did not ask", () => {
        const target = newLayoutNode(undefined, undefined, undefined, { blockId: "b1" });
        const state = newLayoutTreeState(target);
        const treeAction = makeBackendSplitAction(backendAction("splithorizontal", "after", false), target);
        splitHorizontal(state, treeAction as LayoutTreeSplitHorizontalAction);
        expect(state.focusedNodeId).toBeUndefined();
    });

    it("inserts the block when the target node no longer exists", () => {
        const other = newLayoutNode(undefined, undefined, undefined, { blockId: "b3" });
        const state = newLayoutTreeState(other);
        const treeAction = makeBackendSplitAction(backendAction("splitvertical", "after"), null);
        expect(treeAction.type).toBe(LayoutTreeActionType.InsertNode);
        insertNode(state, treeAction as LayoutTreeInsertNodeAction);
        const inserted = (treeAction as LayoutTreeInsertNodeAction).node;
        expect(inserted.data.blockId).toBe("b2");
        expect(state.focusedNodeId).toBe(inserted.id);
    });
});
