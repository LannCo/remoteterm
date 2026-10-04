// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as keyutil from "@/util/keyutil";
import { atom } from "jotai";
import { beforeAll, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    api: {
        closeBuilderWindow: vi.fn(),
        closeTab: vi.fn(() => Promise.resolve(true)),
        openBuilderTerminal: vi.fn(() => Promise.resolve("")),
        registerGlobalWebviewKeys: vi.fn(),
        setKeyboardChordMode: vi.fn(),
    },
    layoutModel: null as any,
    windowType: "tab",
    blockCount: 2,
    bcms: new Map<string, any>(),
}));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    return {
        atoms: {
            staticTabId: atom("tab-1"),
            modalOpen: atom(false),
            workspaceId: atom("ws-1"),
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
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: vi.fn(),
    getLayoutModelForStaticTab: vi.fn(() => h.layoutModel),
    NavigateDirection: { Up: 0, Right: 1, Down: 2, Left: 3 },
}));
vi.mock("./windowtype", () => ({
    isBuilderWindow: () => h.windowType === "builder",
    isTabWindow: () => h.windowType === "tab",
}));

import { appHandleKeyDown, registerGlobalKeys, uxCloseBlock } from "./keymodel";

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
        } else if (part.startsWith("c{")) {
            ev.code = part.slice(2, -1);
        } else {
            ev.key = part;
        }
    }
    return ev as WaveKeyboardEvent;
}

function makeLayoutModel(focusedBlockId: string) {
    const focused = focusedBlockId == null ? undefined : { id: `node-${focusedBlockId}`, data: { blockId: focusedBlockId } };
    return {
        focusedNode: atom(focused),
        ephemeralNode: atom(undefined),
        getNodeByBlockId: vi.fn((blockId: string) => (blockId === focusedBlockId ? focused : null)),
        closeNode: vi.fn(() => Promise.resolve()),
        closeFocusedNode: vi.fn(() => Promise.resolve()),
        switchNodeFocusInDirection: vi.fn(),
        switchNodeFocusByBlockNum: vi.fn(),
        magnifyNodeToggle: vi.fn(),
        addEphemeralNodeToLayout: vi.fn(),
        focusFirstNode: vi.fn(),
        focusNode: vi.fn(),
    };
}

describe("keymodel without a layout model or with an empty tree", () => {
    beforeAll(() => {
        keyutil.setKeyUtilPlatform("linux");
        registerGlobalKeys();
    });

    for (const [label, makeModel] of [
        ["no layout model", () => null],
        ["an empty tree", () => makeLayoutModel(null)],
    ] as const) {
        it(`does not throw with ${label}`, () => {
            h.windowType = "tab";
            h.layoutModel = makeModel();
            for (const desc of ["Escape", "Cmd:m", "Ctrl:Shift:ArrowLeft", "Cmd:f", "Ctrl:Shift:c{Digit1}"]) {
                expect(() => appHandleKeyDown(linuxKey(desc)), desc).not.toThrow();
            }
            expect(() => uxCloseBlock("b1")).not.toThrow();
        });
    }
});
