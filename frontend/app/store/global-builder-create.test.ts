// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    layoutModel: null as any,
    createBlock: vi.fn(),
    deleteBlock: vi.fn(),
    openBuilderTerminal: vi.fn(),
}));

vi.mock("@/layout/index", () => ({
    getLayoutModelForStaticTab: vi.fn(() => h.layoutModel),
    LayoutTreeActionType: {
        InsertNode: "insert",
        SplitHorizontal: "splithorizontal",
        SplitVertical: "splitvertical",
        ReplaceNode: "replace",
    },
    newLayoutNode: (_dir: unknown, _size: unknown, _children: unknown, data: unknown) => ({ id: "node-new", data }),
}));
vi.mock("./services", () => ({
    ObjectService: { CreateBlock: h.createBlock, DeleteBlock: h.deleteBlock },
    ClientService: {},
}));
vi.mock("./wps", () => ({ waveEventSubscribeSingle: vi.fn(), peekFileSubject: vi.fn() }));
vi.mock("./badge", () => ({ setupBadgesSubscription: vi.fn() }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));

import { BuilderNoticeAtom } from "./builder-terminal";
import { createBlock, createBlockSplitHorizontally, createBlockSplitVertically, hideBlockKeepAlive, replaceBlock } from "./global";
import { globalStore } from "./jotaiStore";
import { setWaveWindowType } from "./windowtype";

const termDef: BlockDef = { meta: { view: "term", controller: "shell", "cmd:cwd": "/elsewhere", connection: "user@host" } };
const webDef: BlockDef = { meta: { view: "web", url: "https://example.com" } };

function makeLayoutModel() {
    return {
        treeReducer: vi.fn(),
        getNodeByBlockId: vi.fn(() => ({ id: "node-1" })),
        newEphemeralNode: vi.fn(),
    };
}

beforeEach(() => {
    vi.clearAllMocks();
    h.layoutModel = makeLayoutModel();
    h.createBlock.mockResolvedValue("new-block");
    h.openBuilderTerminal.mockResolvedValue("");
    (window as any).api = { openBuilderTerminal: h.openBuilderTerminal };
    globalStore.set(BuilderNoticeAtom, "");
});

describe("block creation in a builder window", () => {
    beforeEach(() => setWaveWindowType("builder"));

    it("sends term blocks to open-builder-terminal, ignoring the def's cwd and connection", async () => {
        expect(await createBlock(termDef)).toBeNull();
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "", targetaction: "" });
        expect(await createBlockSplitHorizontally(termDef, "b1", "after")).toBeNull();
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitright" });
        await createBlockSplitHorizontally(termDef, "b1", "before");
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitleft" });
        await createBlockSplitVertically(termDef, "b1", "after");
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitdown" });
        await createBlockSplitVertically(termDef, "b1", "before");
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitup" });
        expect(h.createBlock).not.toHaveBeenCalled();
        expect(h.layoutModel.treeReducer).not.toHaveBeenCalled();
    });

    it("shows an Open error as the builder notice", async () => {
        h.openBuilderTerminal.mockResolvedValueOnce("too many terminals in this builder (max 16)");
        await createBlock(termDef);
        expect(globalStore.get(BuilderNoticeAtom)).toBe("too many terminals in this builder (max 16)");
    });

    it("treats replaceBlock with a term def as a no-op", async () => {
        expect(await replaceBlock("b1", termDef, true)).toBeNull();
        expect(h.openBuilderTerminal).not.toHaveBeenCalled();
        expect(h.createBlock).not.toHaveBeenCalled();
        expect(h.deleteBlock).not.toHaveBeenCalled();
    });

    it("creates other views through the object service into the builder tab", async () => {
        expect(await createBlock(webDef)).toBe("new-block");
        expect(h.createBlock).toHaveBeenCalledWith(webDef, expect.anything());
        expect(h.layoutModel.treeReducer).toHaveBeenCalledTimes(1);
        expect(h.openBuilderTerminal).not.toHaveBeenCalled();
    });

    it("creates nothing before the terminal panel has a layout", async () => {
        h.layoutModel = null;
        await expect(createBlock(webDef)).rejects.toThrow();
        expect(h.createBlock).not.toHaveBeenCalled();
    });

    it("closes keep-alive blocks instead of hiding them", () => {
        expect(hideBlockKeepAlive("b1")).toBe(false);
    });
});

describe("block creation in a main window", () => {
    beforeEach(() => setWaveWindowType("tab"));

    it("keeps creating terminals through the object service", async () => {
        expect(await createBlock(termDef)).toBe("new-block");
        expect(h.createBlock).toHaveBeenCalledWith(termDef, expect.anything());
        expect(h.openBuilderTerminal).not.toHaveBeenCalled();
    });
});
