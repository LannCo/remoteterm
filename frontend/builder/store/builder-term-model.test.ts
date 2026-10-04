// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    log: [] as string[],
    objs: new Map<string, any>(),
    objAtoms: new Map<string, any>(),
    api: { ensureBuilderTab: vi.fn(), doRefresh: vi.fn() },
    deleteLayoutModelForTab: vi.fn(),
    pinGate: null as { oref: string; gate: Promise<void> } | null,
}));

vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    const objAtom = (oref: string) => {
        let objectAtom = h.objAtoms.get(oref);
        if (objectAtom == null) {
            objectAtom = atom(null);
            h.objAtoms.set(oref, objectAtom);
        }
        return objectAtom;
    };
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app"), staticTabId: atom(null) },
        getApi: () => h.api,
        WOS: {
            makeORef: (otype: string, oid: string) => `${otype}:${oid}`,
            getWaveObjectAtom: objAtom,
            loadAndPinWaveObject: async (oref: string) => {
                h.log.push(`pin:${oref}`);
                if (h.pinGate?.oref === oref) {
                    await h.pinGate.gate;
                }
                const val = h.objs.get(oref) ?? null;
                globalStore.set(objAtom(oref), val);
                return val;
            },
            updateWaveObject: (update: WaveObjUpdate) => {
                globalStore.set(
                    objAtom(`${update.otype}:${update.oid}`),
                    update.updatetype === "delete" ? null : update.obj
                );
            },
        },
    };
});
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: h.deleteLayoutModelForTab,
    getLayoutModelForStaticTab: () => {
        h.log.push("layoutmodel");
        return null;
    },
}));

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { atoms, WOS } from "@/store/global";
import { BuilderTermMismatchMessage, BuilderTermModel } from "./builder-term-model";

function deleteTabUpdate(): WaveObjUpdate {
    return { updatetype: "delete", otype: "tab", oid: "tab-1" } as WaveObjUpdate;
}

describe("BuilderTermModel", () => {
    beforeEach(() => {
        BuilderTermModel.resetInstance();
        h.log.length = 0;
        h.pinGate = null;
        h.objAtoms.clear();
        h.objs.clear();
        h.objs.set("tab:tab-1", { otype: "tab", oid: "tab-1", version: 1, layoutstate: "layout-1", blockids: ["b1"] });
        h.objs.set("layout:layout-1", { otype: "layout", oid: "layout-1", version: 1 });
        h.api.ensureBuilderTab.mockReset();
        h.api.doRefresh.mockReset();
        h.deleteLayoutModelForTab.mockReset();
        h.api.ensureBuilderTab.mockImplementation(async () => {
            h.log.push("ensure");
            return { tabid: "tab-1", appid: "draft/app" };
        });
        globalStore.set(atoms.staticTabId, null);
        globalStore.set(atoms.builderAppId, "draft/app");
        BuilderFocusManager.getInstance().setAppFocused();
    });

    it("pins the tab and its layout before setting the static tab, and builds no layout model", async () => {
        const unsub = globalStore.sub(atoms.staticTabId, () =>
            h.log.push(`static:${globalStore.get(atoms.staticTabId)}`)
        );
        const model = BuilderTermModel.getInstance();
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        await model.bootstrap();
        unsub();
        expect(h.log).toEqual(["ensure", "pin:tab:tab-1", "pin:layout:layout-1", "static:tab-1"]);
        expect(globalStore.get(model.stateAtom)).toBe("ready");
        expect(globalStore.get(model.tabIdAtom)).toBe("tab-1");
        expect(globalStore.get(model.ensureOkAtom)).toBe(true);
    });

    it("runs the bootstrap once when asked twice", async () => {
        const model = BuilderTermModel.getInstance();
        await Promise.all([model.bootstrap(), model.bootstrap()]);
        expect(h.api.ensureBuilderTab).toHaveBeenCalledTimes(1);
    });

    it("mounts nothing when Electron's app id differs from the renderer's", async () => {
        h.api.ensureBuilderTab.mockResolvedValue({ tabid: "tab-1", appid: "draft/other" });
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        expect(globalStore.get(model.stateAtom)).toBe("mismatch");
        expect(globalStore.get(atoms.staticTabId)).toBeNull();
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(h.log.some((entry) => entry.startsWith("pin:"))).toBe(false);
        expect(BuilderTermMismatchMessage).toBe("Terminal app and builder app differ; reopen the builder");
    });

    it("shows an Ensure error and retries in place", async () => {
        h.api.ensureBuilderTab.mockResolvedValueOnce({ error: "Could not start the terminals: boom" });
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        expect(globalStore.get(model.stateAtom)).toBe("error");
        expect(globalStore.get(model.errorAtom)).toBe("Could not start the terminals: boom");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        model.retry();
        await vi.waitFor(() => expect(globalStore.get(model.stateAtom)).toBe("ready"));
        expect(h.api.doRefresh).not.toHaveBeenCalled();
    });

    it("keeps Open terminal disabled when loading the tab fails after Ensure", async () => {
        h.objs.delete("tab:tab-1");
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        expect(globalStore.get(model.stateAtom)).toBe("error");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(globalStore.get(atoms.staticTabId)).toBeNull();
    });

    it("reloads the renderer if a different static tab is already set", async () => {
        globalStore.set(atoms.staticTabId, "tab-old");
        await BuilderTermModel.getInstance().bootstrap();
        expect(h.api.doRefresh).toHaveBeenCalledTimes(1);
        expect(globalStore.get(atoms.staticTabId)).toBe("tab-old");
    });

    it("drops the layout model and offers a reload when the tab is deleted", async () => {
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        WOS.updateWaveObject(deleteTabUpdate());
        expect(globalStore.get(model.stateAtom)).toBe("vanished");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(h.deleteLayoutModelForTab).toHaveBeenCalledWith("tab-1");
        model.retry();
        expect(h.api.doRefresh).toHaveBeenCalledTimes(1);
    });

    it("shows switching, not the reload state, when the app is being switched", async () => {
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        model.markSwitching();
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        WOS.updateWaveObject(deleteTabUpdate());
        expect(globalStore.get(model.stateAtom)).toBe("switching");
        expect(h.deleteLayoutModelForTab).toHaveBeenCalledWith("tab-1");
    });

    function makeGate() {
        let release: () => void;
        const gate = new Promise<void>((resolve) => {
            release = resolve;
        });
        return { gate, release };
    }

    it("stays switching when the app switch starts while Ensure is in flight", async () => {
        const { gate, release } = makeGate();
        h.api.ensureBuilderTab.mockImplementation(async () => {
            await gate;
            return { tabid: "tab-1", appid: "draft/app" };
        });
        const model = BuilderTermModel.getInstance();
        const done = model.bootstrap();
        expect(globalStore.get(model.stateAtom)).toBe("loading");
        model.markSwitching();
        release();
        await done;
        expect(globalStore.get(model.stateAtom)).toBe("switching");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(globalStore.get(atoms.staticTabId)).toBeNull();
        expect(globalStore.get(model.tabIdAtom)).toBeNull();
    });

    it("stays switching when the app switch starts while the tab is being pinned", async () => {
        const { gate, release } = makeGate();
        h.pinGate = { oref: "layout:layout-1", gate };
        const model = BuilderTermModel.getInstance();
        const done = model.bootstrap();
        await vi.waitFor(() => expect(h.log).toContain("pin:layout:layout-1"));
        model.markSwitching();
        release();
        await done;
        expect(globalStore.get(model.stateAtom)).toBe("switching");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(globalStore.get(atoms.staticTabId)).toBeNull();
        expect(globalStore.get(model.tabIdAtom)).toBeNull();
    });

    it("does not report a load error over a state that moved on while pinning", async () => {
        const { gate, release } = makeGate();
        h.pinGate = { oref: "tab:tab-1", gate };
        h.objs.delete("tab:tab-1");
        const model = BuilderTermModel.getInstance();
        const done = model.bootstrap();
        await vi.waitFor(() => expect(h.log).toContain("pin:tab:tab-1"));
        model.markSwitching();
        release();
        await done;
        expect(globalStore.get(model.stateAtom)).toBe("switching");
    });

    it("is vanished, not ready, when the tab was deleted while the layout was being pinned", async () => {
        const { gate, release } = makeGate();
        h.pinGate = { oref: "layout:layout-1", gate };
        const model = BuilderTermModel.getInstance();
        const done = model.bootstrap();
        await vi.waitFor(() => expect(h.log).toContain("pin:layout:layout-1"));
        WOS.updateWaveObject(deleteTabUpdate());
        release();
        await done;
        expect(globalStore.get(model.stateAtom)).toBe("vanished");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        model.retry();
        expect(h.api.doRefresh).toHaveBeenCalledTimes(1);
    });

    it("moves builder focus to the terminal at the first pane and to the app at zero panes", () => {
        const model = BuilderTermModel.getInstance();
        const focus = BuilderFocusManager.getInstance();
        model.handlePaneCount(0);
        expect(focus.getFocusType()).toBe("app");
        model.handlePaneCount(1);
        expect(focus.getFocusType()).toBe("terminal");
        expect(model.hasPanes()).toBe(true);
        focus.setAppFocused();
        model.handlePaneCount(2);
        expect(focus.getFocusType()).toBe("app");
        focus.setTerminalFocused();
        model.handlePaneCount(0);
        expect(focus.getFocusType()).toBe("app");
        expect(model.hasPanes()).toBe(false);
        model.handlePaneCount(1);
        expect(focus.getFocusType()).toBe("terminal");
    });
});
