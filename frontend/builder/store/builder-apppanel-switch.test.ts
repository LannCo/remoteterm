// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    order: [] as string[],
    rpc: { DeleteBuilderCommand: vi.fn(), SetRTInfoCommand: vi.fn() },
    api: { setBuilderWindowAppId: vi.fn(), doRefresh: vi.fn() },
    markSwitching: vi.fn(),
    setState: vi.fn(),
    tabAtom: null as any,
    getWaveObjectAtom: vi.fn(),
}));

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: h.rpc }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn(() => () => {}) }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app") },
        getApi: () => h.api,
        getSettingsKeyAtom: vi.fn(() => atom(false)),
        WOS: {
            makeORef: (otype: string, oid: string) => `${otype}:${oid}`,
            getWaveObjectAtom: h.getWaveObjectAtom,
        },
    };
});
vi.mock("@/builder/store/builder-term-model", async () => {
    const { atom } = await import("jotai");
    const tabIdAtom = atom("tab-1");
    h.tabAtom = atom(null);
    h.getWaveObjectAtom.mockImplementation(() => h.tabAtom);
    return {
        BuilderTermModel: {
            getInstance: () => ({ markSwitching: h.markSwitching, setState: h.setState, tabIdAtom }),
        },
    };
});

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderAppPanelModel } from "./builder-apppanel-model";

describe("switchBuilderApp", () => {
    afterEach(() => {
        vi.useRealTimers();
    });

    it("marks the panel as switching first and waits for Electron before reloading", async () => {
        vi.useFakeTimers();
        h.markSwitching.mockImplementation(() => h.order.push("switching"));
        h.rpc.DeleteBuilderCommand.mockImplementation(async () => h.order.push("delete"));
        h.rpc.SetRTInfoCommand.mockImplementation(async () => h.order.push("rtinfo"));
        let releaseAppId: () => void;
        h.api.setBuilderWindowAppId.mockImplementation(
            () =>
                new Promise<boolean>((resolve) => {
                    releaseAppId = () => {
                        h.order.push("appid");
                        resolve(true);
                    };
                })
        );
        h.api.doRefresh.mockImplementation(() => h.order.push("refresh"));

        const done = BuilderAppPanelModel.getInstance().switchBuilderApp();
        await vi.advanceTimersByTimeAsync(1000);
        expect(h.order).toEqual(["switching", "delete", "rtinfo"]);
        releaseAppId();
        await vi.advanceTimersByTimeAsync(1000);
        await done;
        expect(h.order).toEqual(["switching", "delete", "rtinfo", "appid", "refresh"]);
        expect(h.setState).not.toHaveBeenCalled();
    });

    it("returns to the terminals when the teardown fails and the tab still exists", async () => {
        vi.spyOn(console, "error").mockImplementation(() => {});
        h.rpc.DeleteBuilderCommand.mockRejectedValueOnce(new Error("server gone"));
        h.api.doRefresh.mockClear();
        h.setState.mockClear();
        globalStore.set(h.tabAtom, { otype: "tab", oid: "tab-1", blockids: ["b1"] });
        const model = BuilderAppPanelModel.getInstance();
        await model.switchBuilderApp();
        expect(h.getWaveObjectAtom).toHaveBeenCalledWith("tab:tab-1");
        expect(h.setState).toHaveBeenCalledWith("ready");
        expect(globalStore.get(model.errorAtom)).toContain("server gone");
        expect(h.api.doRefresh).not.toHaveBeenCalled();
    });

    it("offers Retry when the teardown fails and the tab is gone", async () => {
        vi.spyOn(console, "error").mockImplementation(() => {});
        h.rpc.DeleteBuilderCommand.mockRejectedValueOnce(new Error("server gone"));
        h.setState.mockClear();
        globalStore.set(h.tabAtom, null);
        await BuilderAppPanelModel.getInstance().switchBuilderApp();
        expect(h.setState).toHaveBeenCalledWith("vanished");
    });
});
