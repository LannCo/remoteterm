// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { sortByDisplayOrder } from "@/app/workspace/widgetfilter";
import { beforeEach, describe, expect, it, vi } from "vitest";

// ---------------------------------------------------------------------------
// Mock atom store — mirrors the pattern in sourcecontrol/review-mode.test.ts
// ---------------------------------------------------------------------------

interface MockAtom {
    _key: string;
    _type: "primitive" | "derived";
    _value?: any;
    _fn?: (get: any) => any;
}

const atomStore = new Map<string, MockAtom>();
let atomCounter = 0;

function createMockAtom(initOrFn?: any): MockAtom {
    const key = `atom_${atomCounter++}`;
    if (typeof initOrFn === "function") {
        const derived: MockAtom = { _key: key, _type: "derived", _fn: initOrFn };
        atomStore.set(key, derived);
        return derived;
    }
    const val = initOrFn ?? null;
    const primitive: MockAtom = { _key: key, _type: "primitive", _value: val };
    atomStore.set(key, primitive);
    return primitive;
}

function mockAtomGet(atom: MockAtom | any): any {
    if (atom && atom._type === "primitive") return atom._value;
    if (atom && atom._type === "derived" && atom._fn) return atom._fn(mockAtomGet);
    return undefined;
}

function mockAtomSet(atom: MockAtom | any, value: any): void {
    if (atom && atom._type === "primitive") {
        atom._value = value;
    }
}

vi.mock("@/app/store/jotaiStore", () => ({
    globalStore: {
        get: vi.fn((atom: any) => mockAtomGet(atom)),
        set: vi.fn((atom: any, value: any) => mockAtomSet(atom, value)),
        sub: vi.fn(() => vi.fn()),
    },
}));

vi.mock("jotai", () => ({
    atom: vi.fn((initOrFn?: any) => createMockAtom(initOrFn)),
}));

vi.mock("@/app/store/wshrpcutil", () => ({
    TabRpcClient: {},
}));

vi.mock("@/app/view/remotetermconfig/remotetermconfig", () => ({ RemoteTermConfigView: {} }));
vi.mock("@/app/view/remotetermconfig/widgetscontent", () => ({ WidgetsContent: {} }));
vi.mock("@/app/view/remotetermconfig/backgroundscontent", () => ({ BackgroundsContent: {} }));
vi.mock("@/app/view/remotetermconfig/connectionscontent", () => ({ ConnectionsContent: {} }));
vi.mock("@/app/view/remotetermconfig/generalcontent", () => ({ GeneralContent: {} }));
vi.mock("@/app/view/remotetermconfig/secretscontent", () => ({ SecretsContent: {} }));

// ---------------------------------------------------------------------------
// Fixture: the real shipped default widget set (pkg/wconfig/defaultconfig/widgets.json)
// ---------------------------------------------------------------------------

function makeDefaultWidgetsMap(): { [key: string]: any } {
    return {
        "defwidget@terminal": {
            "display:order": -6,
            icon: "square-terminal",
            label: "terminal",
            blockdef: { meta: { view: "term", controller: "shell" } },
        },
        "defwidget@sourcecontrol": {
            "display:order": -5,
            icon: "code-branch",
            label: "source\ncontrol",
            blockdef: { meta: { view: "sourcecontrol" } },
        },
        "defwidget@files": {
            "display:order": -4,
            icon: "folder",
            label: "files",
            blockdef: { meta: { view: "preview" } },
        },
        "defwidget@web": {
            "display:order": -3,
            icon: "globe",
            label: "web",
            blockdef: { meta: { view: "web" } },
        },
        "defwidget@sysinfo": {
            "display:order": -2,
            icon: "chart-line",
            label: "sysinfo",
            blockdef: { meta: { view: "sysinfo" } },
        },
        "defwidget@processviewer": {
            "display:order": -1,
            icon: "list-tree",
            label: "processes",
            blockdef: { meta: { view: "processviewer" } },
        },
    };
}

function base64ToString(b64: string): string {
    return Buffer.from(b64, "base64").toString("utf-8");
}

function stringToBase64(str: string): string {
    return Buffer.from(str, "utf-8").toString("base64");
}

describe("RemoteTermConfigViewModel — sidebar widgets (reorder / toggle / persist)", () => {
    let model: any;
    let fullConfigAtom: MockAtom;
    let rawWidgetsFileOnDisk: { [key: string]: any } | null; // null = file not found yet
    let fileWriteCommand: ReturnType<typeof vi.fn>;

    beforeEach(async () => {
        vi.clearAllMocks();
        vi.resetModules();
        atomStore.clear();
        atomCounter = 0;
        rawWidgetsFileOnDisk = null; // fresh install: no user widgets.json yet

        const { RemoteTermConfigViewModel } = await import("./remotetermconfig-model");

        fullConfigAtom = createMockAtom({ widgets: makeDefaultWidgetsMap() });

        fileWriteCommand = vi.fn(async (_client: any, params: { info: { path: string }; data64: string }) => {
            rawWidgetsFileOnDisk = JSON.parse(base64ToString(params.data64));
        });

        model = new (RemoteTermConfigViewModel as any)({
            blockId: "test-block",
            nodeModel: { isFocused: createMockAtom(false) } as any,
            tabModel: { tabId: "test-tab" } as any,
            waveEnv: {
                electron: {
                    getConfigDir: () => "/config",
                    getPlatform: () => "linux",
                },
                isWindows: () => false,
                getBlockMetaKeyAtom: vi.fn(() => createMockAtom(null)) as any,
                getTabMetaKeyAtom: vi.fn(() => createMockAtom(null)) as any,
                atoms: {
                    fullConfigAtom,
                    allConnStatus: createMockAtom([]),
                    workspaceId: createMockAtom("ws-1"),
                },
                rpc: {
                    FileInfoCommand: vi.fn(async (_client: any, params: { info: { path: string } }) => {
                        if (params.info.path === "/config/widgets.json") {
                            return { notfound: rawWidgetsFileOnDisk == null };
                        }
                        return { notfound: true };
                    }),
                    FileReadCommand: vi.fn(async () => ({
                        data64: stringToBase64(JSON.stringify(rawWidgetsFileOnDisk ?? {})),
                    })),
                    FileWriteCommand: fileWriteCommand,
                } as any,
            } as any,
        });
    });

    it("moving the first widget down one slot assigns it an order between its new neighbors, and it sorts into the new position", async () => {
        // terminal(-6), sourcecontrol(-5), files(-4), web(-3), sysinfo(-2), processviewer(-1)
        // Drag "terminal" from index 0 to index 1 -> new local order:
        // sourcecontrol, terminal, files, web, sysinfo, processviewer
        const newOrderedKeys = [
            "defwidget@sourcecontrol",
            "defwidget@terminal",
            "defwidget@files",
            "defwidget@web",
            "defwidget@sysinfo",
            "defwidget@processviewer",
        ];

        await model.reorderWidget("defwidget@terminal", 1, newOrderedKeys);

        expect(fileWriteCommand).toHaveBeenCalledTimes(1);
        expect(rawWidgetsFileOnDisk).not.toBeNull();

        const writtenOrder = rawWidgetsFileOnDisk["defwidget@terminal"]["display:order"];
        // neighbors in the NEW arrangement are sourcecontrol(-5, prev) and files(-4, next)
        expect(writtenOrder).toBeGreaterThan(-5);
        expect(writtenOrder).toBeLessThan(-4);

        const merged = { ...makeDefaultWidgetsMap(), ...rawWidgetsFileOnDisk };
        const sortedKeys = sortByDisplayOrder(merged).map((w) => w.label);
        expect(sortedKeys).toEqual(["source\ncontrol", "terminal", "files", "web", "sysinfo", "processes"]);
    });

    it("moving a widget to the very front (no prev neighbor) still sorts it before everything else", async () => {
        const newOrderedKeys = [
            "defwidget@processviewer",
            "defwidget@terminal",
            "defwidget@sourcecontrol",
            "defwidget@files",
            "defwidget@web",
            "defwidget@sysinfo",
        ];

        await model.reorderWidget("defwidget@processviewer", 0, newOrderedKeys);

        const writtenOrder = rawWidgetsFileOnDisk["defwidget@processviewer"]["display:order"];
        expect(writtenOrder).toBeLessThan(-6);

        const merged = { ...makeDefaultWidgetsMap(), ...rawWidgetsFileOnDisk };
        const sortedKeys = sortByDisplayOrder(merged).map((w) => w.label);
        expect(sortedKeys[0]).toBe("processes");
    });

    it("toggling visibility flips display:hidden and leaves every other widget key untouched", async () => {
        await model.toggleWidgetHidden("defwidget@web");

        expect(fileWriteCommand).toHaveBeenCalledTimes(1);
        expect(rawWidgetsFileOnDisk["defwidget@web"]["display:hidden"]).toBe(true);
        expect(Object.keys(rawWidgetsFileOnDisk)).toEqual(["defwidget@web"]);

        // Toggling again (simulating the config round-trip having merged the patch back
        // into fullConfigAtom, as the real backend/watcher would do) should flip it back.
        fullConfigAtom._value = { widgets: { ...makeDefaultWidgetsMap(), ...rawWidgetsFileOnDisk } };
        await model.toggleWidgetHidden("defwidget@web");

        expect(rawWidgetsFileOnDisk["defwidget@web"]["display:hidden"]).toBe(false);
    });

    it("a reorder and a toggle fired back-to-back both land in the written file instead of one clobbering the other", async () => {
        // Fire both without awaiting the first -- this is exactly what a fast drag-drop
        // immediately followed by a visibility click does in the real UI.
        const reorderPromise = model.reorderWidget("defwidget@terminal", 5, [
            "defwidget@sourcecontrol",
            "defwidget@files",
            "defwidget@web",
            "defwidget@sysinfo",
            "defwidget@processviewer",
            "defwidget@terminal",
        ]);
        const togglePromise = model.toggleWidgetHidden("defwidget@sysinfo");

        await Promise.all([reorderPromise, togglePromise]);

        expect(fileWriteCommand).toHaveBeenCalledTimes(2);
        expect(rawWidgetsFileOnDisk).toHaveProperty("defwidget@terminal");
        expect(rawWidgetsFileOnDisk).toHaveProperty("defwidget@sysinfo");
        expect(rawWidgetsFileOnDisk["defwidget@sysinfo"]["display:hidden"]).toBe(true);
        expect(rawWidgetsFileOnDisk["defwidget@terminal"]["display:order"]).toBeGreaterThan(-1);
    });

    it("reorder/toggle do not set hasEditedAtom -- matches the Backgrounds page's auto-persist design, Save is not meant to gate these", async () => {
        await model.reorderWidget("defwidget@terminal", 1, [
            "defwidget@sourcecontrol",
            "defwidget@terminal",
            "defwidget@files",
            "defwidget@web",
            "defwidget@sysinfo",
            "defwidget@processviewer",
        ]);
        await model.toggleWidgetHidden("defwidget@web");

        expect(model.hasChanges()).toBe(false);
    });
});
