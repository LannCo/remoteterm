// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import * as keyutil from "@/util/keyutil";
import { atom } from "jotai";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    api: {
        closeBuilderWindow: vi.fn(),
        closeTab: vi.fn(() => Promise.resolve(true)),
        registerGlobalWebviewKeys: vi.fn(),
        setKeyboardChordMode: vi.fn(),
    },
    openTerminal: vi.fn(() => Promise.resolve("")),
    layoutModel: null as any,
    blockCount: 1,
    bcms: new Map<string, any>(),
    webviewKeys: null as string[],
}));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    return {
        atoms: {
            staticTabId: atom("tab-1"),
            modalOpen: atom(false),
            workspaceId: atom(null),
            controlShiftDelayAtom: atom(false),
            newTabDropdownOpen: atom(false),
        },
        createBlock: vi.fn(),
        createBlockSplitHorizontally: vi.fn(),
        createBlockSplitVertically: vi.fn(),
        getAllBlockComponentModels: vi.fn(() => []),
        getApi: () => h.api,
        getBlockComponentModel: (blockId: string) => h.bcms.get(blockId),
        getBlockMetaKeyAtom: vi.fn(() => atom(null)),
        getFocusedBlockId: vi.fn(),
        getSettingsKeyAtom: vi.fn(() => atom(false)),
        globalStore,
        refocusNode: vi.fn(),
        replaceBlock: vi.fn(),
        WOS: {
            makeORef: (otype: string, oid: string) => `${otype}:${oid}`,
            getWaveObjectAtom: () => atom({ blockids: Array.from({ length: h.blockCount }, (_, i) => `b${i + 1}`) }),
        },
    };
});
vi.mock("@/app/store/focusManager", () => ({ FocusManager: { getInstance: vi.fn() } }));
vi.mock("@/app/store/services", () => ({ UserInputService: {} }));
vi.mock("@/app/store/tab-model", () => ({ getActiveTabModel: vi.fn(() => null) }));
vi.mock("@/app/workspace/workspace-layout-model", () => ({ WorkspaceLayoutModel: {} }));
vi.mock("@/app/store/builder-terminal", () => ({ openBuilderTerminal: h.openTerminal }));
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: vi.fn(),
    getLayoutModelForStaticTab: vi.fn(() => h.layoutModel),
    NavigateDirection: { Up: 0, Right: 1, Down: 2, Left: 3 },
}));
vi.mock("./windowtype", () => ({ isBuilderWindow: () => true, isTabWindow: () => false }));

import { appHandleKeyDown, registerBuilderGlobalKeys, uxCloseBlock } from "./keymodel";

function linuxKey(desc: string): WaveKeyboardEvent {
    const ev: any = { type: "keydown", key: "", code: "", cmd: false, alt: false, option: false, meta: false, control: false, shift: false };
    for (const part of desc.split(":")) {
        if (part === "Cmd") {
            ev.cmd = true;
            ev.alt = true;
        } else if (part === "Shift") {
            ev.shift = true;
        } else if (part === "Ctrl") {
            ev.control = true;
        } else {
            ev.key = part;
        }
    }
    return ev as WaveKeyboardEvent;
}

function makeLayoutModel() {
    const focused = { id: "node-b1", data: { blockId: "b1" } };
    return {
        focusedNode: atom(focused),
        ephemeralNode: atom(undefined),
        getNodeByBlockId: vi.fn((blockId: string) => (blockId === "b1" ? focused : null)),
        closeNode: vi.fn(() => Promise.resolve()),
        closeFocusedNode: vi.fn(() => Promise.resolve()),
        switchNodeFocusInDirection: vi.fn(),
        switchNodeFocusByBlockNum: vi.fn(),
        magnifyNodeToggle: vi.fn(),
        focusFirstNode: vi.fn(),
        focusNode: vi.fn(),
    };
}

describe("builder keys in keymodel", () => {
    beforeAll(() => {
        keyutil.setKeyUtilPlatform("linux");
        registerBuilderGlobalKeys();
        h.webviewKeys = h.api.registerGlobalWebviewKeys.mock.calls[0][0];
    });

    beforeEach(() => {
        vi.clearAllMocks();
        h.layoutModel = makeLayoutModel();
        h.blockCount = 1;
        h.bcms.clear();
        BuilderFocusManager.getInstance().setTerminalFocused();
    });

    it("registers only Cmd:w with the preview webview", () => {
        expect(h.webviewKeys).toEqual(["Cmd:w"]);
    });

    it("closes the last pane through the layout model, never the tab, with Cmd:w on the terminal side", () => {
        expect(appHandleKeyDown(linuxKey("Cmd:w"))).toBe(true);
        expect(h.layoutModel.closeFocusedNode).toHaveBeenCalledTimes(1);
        expect(h.api.closeTab).not.toHaveBeenCalled();
        expect(h.api.closeBuilderWindow).not.toHaveBeenCalled();
    });

    it("closes the last pane from its header through closeNode, never closeTab", () => {
        uxCloseBlock("b1");
        expect(h.layoutModel.closeNode).toHaveBeenCalledWith("node-b1");
        expect(h.api.closeTab).not.toHaveBeenCalled();
    });

    it("closes the builder window with Cmd:w on the app side", () => {
        BuilderFocusManager.getInstance().setAppFocused();
        expect(appHandleKeyDown(linuxKey("Cmd:w"))).toBe(true);
        expect(h.api.closeBuilderWindow).toHaveBeenCalledTimes(1);
        expect(h.layoutModel.closeFocusedNode).not.toHaveBeenCalled();
    });

    it("splits the focused pane through open-builder-terminal", () => {
        expect(appHandleKeyDown(linuxKey("Cmd:d"))).toBe(true);
        expect(h.openTerminal).toHaveBeenCalledWith("splitright", "b1");
    });

    it("moves focus between panes as a tab does", () => {
        expect(appHandleKeyDown(linuxKey("Ctrl:Shift:ArrowLeft"))).toBe(true);
        expect(h.layoutModel.switchNodeFocusInDirection).toHaveBeenCalledWith(3);
    });

    it("hands other keys to the focused pane on the terminal side only, and leaves Ctrl:w unbound", () => {
        const keyDownHandler = vi.fn(() => false);
        h.bcms.set("b1", { viewModel: { keyDownHandler } });
        expect(appHandleKeyDown(linuxKey("Ctrl:w"))).toBe(false);
        expect(keyDownHandler).toHaveBeenCalledTimes(1);
        BuilderFocusManager.getInstance().setAppFocused();
        expect(appHandleKeyDown(linuxKey("Ctrl:w"))).toBe(false);
        expect(keyDownHandler).toHaveBeenCalledTimes(1);
    });
});
