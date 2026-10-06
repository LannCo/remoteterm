// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ subscribe: vi.fn(), badges: vi.fn() }));

vi.mock("@/layout/index", () => ({
    getLayoutModelForStaticTab: vi.fn(() => null),
    LayoutTreeActionType: {},
    newLayoutNode: vi.fn(),
}));
vi.mock("./services", () => ({ ObjectService: {}, ClientService: {} }));
vi.mock("./wps", () => ({ waveEventSubscribeSingle: h.subscribe, peekFileSubject: vi.fn() }));
vi.mock("./badge", () => ({ setupBadgesSubscription: h.badges }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));

import { initBuilderWaveEventSubs, initGlobalWaveEventSubs, refocusNode, setNodeFocus } from "./global";

function subscribedEvents(): string[] {
    return h.subscribe.mock.calls.map((call) => call[0].eventType).sort();
}

describe("builder subscriptions", () => {
    beforeEach(() => {
        h.subscribe.mockClear();
        h.badges.mockClear();
    });

    it("subscribe builder windows to object, config and blockfile events and badges, not userinput", () => {
        initBuilderWaveEventSubs();
        expect(subscribedEvents()).toEqual(["blockfile", "config", "waveobj:update"]);
        expect(h.badges).toHaveBeenCalledTimes(1);
    });

    it("keep the userinput subscription for main windows", () => {
        initGlobalWaveEventSubs({ windowId: "win-1" } as RemoteTermInitOpts);
        expect(subscribedEvents()).toEqual(["blockfile", "config", "userinput", "waveobj:update"]);
        expect(h.badges).toHaveBeenCalledTimes(1);
    });
});

describe("focus helpers without a layout model", () => {
    it("do nothing instead of throwing", () => {
        expect(() => setNodeFocus("node-1")).not.toThrow();
        expect(() => refocusNode("block-1")).not.toThrow();
    });
});
