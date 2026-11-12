// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom, PrimitiveAtom } from "jotai";
import { globalStore } from "./jotaiStore";
import { isBuilderWindow } from "./windowtype";

export type BuilderTermAction = "" | "splitright" | "splitleft" | "splitup" | "splitdown";

// BuilderAppPanelModel.noticeAtom is this atom. It lives here so global.ts can report errors from the
// create intercept without importing the builder model, which imports global.ts.
export const BuilderNoticeAtom = atom("") as PrimitiveAtom<string>;

export function isTermBlockDef(blockDef: BlockDef): boolean {
    return blockDef?.meta?.view === "term";
}

export function splitActionFor(direction: "horizontal" | "vertical", position: "before" | "after"): BuilderTermAction {
    if (direction === "horizontal") {
        return position === "before" ? "splitleft" : "splitright";
    }
    return position === "before" ? "splitup" : "splitdown";
}

export async function openBuilderTerminal(action: BuilderTermAction, targetBlockId: string): Promise<string> {
    const api = (window as any).api as ElectronApi;
    const err = (await api.openBuilderTerminal({ targetblockid: targetBlockId ?? "", targetaction: action })) ?? "";
    globalStore.set(BuilderNoticeAtom, err);
    return err;
}

// Builder panes are local only (the server rejects remote connections), so the switcher is hidden there.
export function showConnectionUi(): boolean {
    return !isBuilderWindow();
}
