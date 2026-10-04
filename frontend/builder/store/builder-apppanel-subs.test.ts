// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ subscribe: vi.fn((_sub: { eventType: string }) => () => {}) }));

vi.mock("@/app/store/wshclientapi", () => ({
    // Every RPC resolves to an empty object; initialize() tolerates that, and only its subscriptions matter here.
    RpcApi: new Proxy({}, { get: () => async () => ({}) }),
}));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: h.subscribe }));
vi.mock("@/layout/index", () => ({ deleteLayoutModelForTab: vi.fn() }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    const settingAtom = atom(false);
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app"), staticTabId: atom(null), fullConfigAtom: atom(null) },
        getApi: vi.fn(() => ({})),
        getSettingsKeyAtom: vi.fn(() => settingAtom),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}` },
    };
});

import { BuilderAppPanelModel } from "./builder-apppanel-model";

describe("BuilderAppPanelModel.initialize", () => {
    it("leaves the config subscription to initBuilderWaveEventSubs", async () => {
        vi.spyOn(console, "error").mockImplementation(() => {});
        await BuilderAppPanelModel.getInstance().initialize();
        const events = h.subscribe.mock.calls.map((call) => call[0].eventType);
        expect(events).toContain("builderstatus");
        expect(events).not.toContain("config");
    });
});
