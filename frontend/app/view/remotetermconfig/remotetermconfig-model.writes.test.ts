// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

// ---------------------------------------------------------------------------
// Mock atom store — mirrors the pattern in remotetermconfig-model.widgets.test.ts
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

const b64d = (s: string) => Buffer.from(s, "base64").toString("utf-8");
const b64e = (s: string) => Buffer.from(s, "utf-8").toString("base64");

type Disk = { [path: string]: any };
type ModelOpts = { failWrite?: string; failInfo?: string; secrets?: { [name: string]: string } };

// fullConfigAtom is deliberately never updated after a write: in the real app the config
// watcher round-trip lands later, so back-to-back actions see the pre-write snapshot.
async function makeModel(disk: Disk, opts: ModelOpts = {}) {
    const { RemoteTermConfigViewModel } = await import("./remotetermconfig-model");
    const fullConfigAtom = createMockAtom({
        widgets: { "w@a": { "display:order": 1, label: "a" }, "w@b": { "display:order": 2, label: "b" } },
        backgrounds: { "bg@x": { "display:name": "X", bg: "red", "bg:opacity": 0.3, "bg:blendmode": "normal" } },
    });
    const secrets: { [name: string]: string } = { ...(opts.secrets ?? {}) };
    const rpc = {
        FileInfoCommand: vi.fn(async (_c: any, p: any) => {
            if (opts.failInfo === p.info.path) throw new Error("EIO");
            return { notfound: disk[p.info.path] == null };
        }),
        FileReadCommand: vi.fn(async (_c: any, p: any) => ({
            data64: b64e(JSON.stringify(disk[p.info.path] ?? {})),
        })),
        FileWriteCommand: vi.fn(async (_c: any, p: any) => {
            await new Promise((r) => setTimeout(r, 5));
            if (opts.failWrite === p.info.path) throw new Error("EACCES");
            disk[p.info.path] = JSON.parse(b64d(p.data64));
        }),
        SetMetaCommand: vi.fn(async () => {}),
        GetSecretsNamesCommand: vi.fn(async () => Object.keys(secrets)),
        GetSecretsLinuxStorageBackendCommand: vi.fn(async () => "libsecret"),
        SetSecretsCommand: vi.fn(async (_c: any, data: { [name: string]: string | null }) => {
            for (const [name, value] of Object.entries(data)) {
                if (value == null) {
                    delete secrets[name];
                } else {
                    secrets[name] = value;
                }
            }
        }),
    };
    const model = new (RemoteTermConfigViewModel as any)({
        blockId: "b",
        nodeModel: { isFocused: createMockAtom(false) } as any,
        tabModel: { tabId: "t" } as any,
        waveEnv: {
            electron: { getConfigDir: () => "/config", getPlatform: () => "linux" },
            isWindows: () => false,
            getBlockMetaKeyAtom: vi.fn(() => createMockAtom(null)),
            getTabMetaKeyAtom: vi.fn(() => createMockAtom(null)),
            atoms: { fullConfigAtom, allConnStatus: createMockAtom([]), workspaceId: createMockAtom("ws") },
            rpc,
        } as any,
    });
    // let the constructor's loadFile() settle so it doesn't overwrite atoms mid-test
    await new Promise((r) => setTimeout(r, 0));
    return { model, rpc, secrets };
}

function tabMetaCalls(rpc: any) {
    return rpc.SetMetaCommand.mock.calls.filter((c: any) => c[1].oref === "tab:t");
}

describe("RemoteTermConfigViewModel — queued widget/background writes", () => {
    beforeEach(() => {
        vi.resetModules();
        atomStore.clear();
        atomCounter = 0;
    });

    it("toggle then drag in quick succession keeps the toggle", async () => {
        const disk: Disk = {};
        const { model } = await makeModel(disk);
        await Promise.all([model.toggleWidgetHidden("w@a"), model.reorderWidget("w@a", 1, ["w@b", "w@a"])]);
        const written = disk["/config/widgets.json"]["w@a"];
        expect(written["display:hidden"]).toBe(true);
        expect(written["display:order"]).toBe(3);
    });

    it("two quick toggles of the same widget cancel out", async () => {
        const disk: Disk = {};
        const { model } = await makeModel(disk);
        await Promise.all([model.toggleWidgetHidden("w@a"), model.toggleWidgetHidden("w@a")]);
        expect(disk["/config/widgets.json"]["w@a"]["display:hidden"]).toBe(false);
    });

    it("background opacity then blend mode in quick succession keeps both", async () => {
        const disk: Disk = {};
        const { model } = await makeModel(disk);
        await Promise.all([
            model.updateBackgroundOpacity("bg@x", 0.9),
            model.updateBackgroundBlendMode("bg@x", "multiply"),
        ]);
        const written = disk["/config/backgrounds.json"]["bg@x"];
        expect(written["bg:opacity"]).toBe(0.9);
        expect(written["bg:blendmode"]).toBe("multiply");
    });

    it("quick-add applies the new background to the tab when the write succeeds", async () => {
        const disk: Disk = {};
        const { model, rpc } = await makeModel(disk);
        model.openBackgroundAdd();
        model.backgroundsAddNameAtom._value = "Mine";
        model.backgroundsAddBgAtom._value = "blue";
        await model.submitBackgroundAdd();
        expect(disk["/config/backgrounds.json"]["bg@mine"].bg).toBe("blue");
        expect(tabMetaCalls(rpc)).toHaveLength(1);
        expect(tabMetaCalls(rpc)[0][1].meta["tab:background"]).toBe("bg@mine");
    });

    it("quick-add does not apply the background to the tab when the write fails", async () => {
        const disk: Disk = {};
        const { model, rpc } = await makeModel(disk, { failWrite: "/config/backgrounds.json" });
        model.backgroundsAddNameAtom._value = "Mine";
        model.backgroundsAddBgAtom._value = "blue";
        await model.submitBackgroundAdd();
        expect(tabMetaCalls(rpc)).toHaveLength(0);
        expect(model.errorMessageAtom._value).toMatch(/Failed to save backgrounds\.json/);
        expect(model.backgroundsAddNameAtom._value).toBe("Mine");
    });
});

describe("RemoteTermConfigViewModel — readRawBackgroundsFile", () => {
    beforeEach(() => {
        vi.resetModules();
        atomStore.clear();
        atomCounter = 0;
    });

    it("treats a missing file as empty and writes only the touched key", async () => {
        const disk: Disk = {};
        const { model } = await makeModel(disk);
        expect(await model.readRawBackgroundsFile()).toEqual({});
        await model.updateBackgroundOpacity("bg@x", 0.5);
        expect(Object.keys(disk["/config/backgrounds.json"])).toEqual(["bg@x"]);
    });

    it("returns null on a read failure and aborts the write instead of treating it as empty", async () => {
        const disk: Disk = { "/config/backgrounds.json": { "bg@keep": { bg: "green" } } };
        const { model, rpc } = await makeModel(disk, { failInfo: "/config/backgrounds.json" });
        expect(await model.readRawBackgroundsFile()).toBeNull();
        await model.updateBackgroundOpacity("bg@x", 0.5);
        expect(rpc.FileWriteCommand).not.toHaveBeenCalled();
        expect(disk["/config/backgrounds.json"]).toEqual({ "bg@keep": { bg: "green" } });
        expect(model.errorMessageAtom._value).toMatch(/Failed to check backgrounds\.json/);
    });

    it("returns null for a non-object file and aborts the write", async () => {
        const disk: Disk = { "/config/backgrounds.json": [1, 2] };
        const { model, rpc } = await makeModel(disk);
        await model.updateBackgroundBlendMode("bg@x", "multiply");
        expect(rpc.FileWriteCommand).not.toHaveBeenCalled();
        expect(model.errorMessageAtom._value).toBe("backgrounds.json content is not a valid object");
    });

    it("merges into an existing file without dropping other keys", async () => {
        const disk: Disk = { "/config/backgrounds.json": { "bg@keep": { bg: "green" } } };
        const { model } = await makeModel(disk);
        await model.updateBackgroundOpacity("bg@x", 0.5);
        expect(disk["/config/backgrounds.json"]["bg@keep"]).toEqual({ bg: "green" });
        expect(disk["/config/backgrounds.json"]["bg@x"]["bg:opacity"]).toBe(0.5);
    });
});

describe("RemoteTermConfigViewModel — secrets", () => {
    beforeEach(() => {
        vi.resetModules();
        atomStore.clear();
        atomCounter = 0;
    });

    it("saveSecret writes the edited value and closes the view", async () => {
        const { model, rpc, secrets } = await makeModel({}, { secrets: { API_KEY: "old" } });
        model.selectedSecretAtom._value = "API_KEY";
        model.secretValueAtom._value = "new";
        await model.saveSecret();
        expect(rpc.SetSecretsCommand).toHaveBeenCalledWith({}, { API_KEY: "new" });
        expect(secrets.API_KEY).toBe("new");
        expect(model.selectedSecretAtom._value).toBeNull();
        expect(model.isLoadingAtom._value).toBe(false);
    });

    it("saveSecret surfaces an RPC failure and keeps the view open", async () => {
        const { model, rpc } = await makeModel({}, { secrets: { API_KEY: "old" } });
        rpc.SetSecretsCommand.mockRejectedValueOnce(new Error("locked"));
        model.selectedSecretAtom._value = "API_KEY";
        await model.saveSecret();
        expect(model.errorMessageAtom._value).toBe("Failed to save secret: locked");
        expect(model.selectedSecretAtom._value).toBe("API_KEY");
        expect(model.isLoadingAtom._value).toBe(false);
    });

    it("saveSecret does nothing without a selected secret", async () => {
        const { model, rpc } = await makeModel({});
        await model.saveSecret();
        expect(rpc.SetSecretsCommand).not.toHaveBeenCalled();
    });

    it("deleteSecret sends a null value and refreshes the list", async () => {
        const { model, rpc } = await makeModel({}, { secrets: { A: "1", B: "2" } });
        model.selectedSecretAtom._value = "A";
        await model.deleteSecret();
        expect(rpc.SetSecretsCommand).toHaveBeenCalledWith({}, { A: null });
        expect(model.secretNamesAtom._value).toEqual(["B"]);
        expect(model.selectedSecretAtom._value).toBeNull();
    });

    it("deleteSecret surfaces an RPC failure", async () => {
        const { model, rpc } = await makeModel({}, { secrets: { A: "1" } });
        rpc.SetSecretsCommand.mockRejectedValueOnce(new Error("denied"));
        model.selectedSecretAtom._value = "A";
        await model.deleteSecret();
        expect(model.errorMessageAtom._value).toBe("Failed to delete secret: denied");
        expect(model.selectedSecretAtom._value).toBe("A");
    });

    it("addNewSecret rejects empty, invalid and duplicate names without calling the RPC", async () => {
        const { model, rpc } = await makeModel({}, { secrets: { TAKEN: "x" } });
        model.secretNamesAtom._value = ["TAKEN"];

        model.newSecretNameAtom._value = "   ";
        await model.addNewSecret();
        expect(model.errorMessageAtom._value).toBe("Secret name cannot be empty");

        model.newSecretNameAtom._value = "1bad-name";
        await model.addNewSecret();
        expect(model.errorMessageAtom._value).toMatch(/^Invalid secret name/);

        model.newSecretNameAtom._value = "TAKEN";
        await model.addNewSecret();
        expect(model.errorMessageAtom._value).toBe('Secret "TAKEN" already exists');

        expect(rpc.SetSecretsCommand).not.toHaveBeenCalled();
    });

    it("addNewSecret stores a trimmed valid name, resets the form and refreshes the list", async () => {
        const { model, rpc } = await makeModel({});
        model.isAddingNewAtom._value = true;
        model.newSecretNameAtom._value = "  NEW_ONE ";
        model.newSecretValueAtom._value = "v";
        await model.addNewSecret();
        expect(rpc.SetSecretsCommand).toHaveBeenCalledWith({}, { NEW_ONE: "v" });
        expect(model.isAddingNewAtom._value).toBe(false);
        expect(model.newSecretNameAtom._value).toBe("");
        expect(model.secretNamesAtom._value).toEqual(["NEW_ONE"]);
        expect(model.errorMessageAtom._value).toBeNull();
    });

    it("addNewSecret surfaces an RPC failure and keeps the form open", async () => {
        const { model, rpc } = await makeModel({});
        rpc.SetSecretsCommand.mockRejectedValueOnce(new Error("no backend"));
        model.isAddingNewAtom._value = true;
        model.newSecretNameAtom._value = "OK_NAME";
        await model.addNewSecret();
        expect(model.errorMessageAtom._value).toBe("Failed to add secret: no backend");
        expect(model.isAddingNewAtom._value).toBe(true);
    });
});
