// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { getRTInfoCommand, fileSubject } = vi.hoisted(() => ({
    getRTInfoCommand: vi.fn(),
    fileSubject: {
        subscribe: vi.fn(() => ({ unsubscribe: vi.fn() })),
        release: vi.fn(),
    },
}));

vi.mock("@xterm/xterm", () => ({ Terminal: class MockTerminal {} }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class MockFitAddon {} }));
vi.mock("@xterm/addon-image", () => ({ ImageAddon: class MockImageAddon {} }));
vi.mock("@xterm/addon-search", () => ({ SearchAddon: class MockSearchAddon {} }));
vi.mock("@xterm/addon-serialize", () => ({ SerializeAddon: class MockSerializeAddon {} }));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class MockWebLinksAddon {} }));
vi.mock("@xterm/addon-webgl", () => ({ WebglAddon: class MockWebglAddon {} }));
vi.mock("@/store/global", () => ({
    globalStore: { get: vi.fn(() => undefined), set: vi.fn(), sub: vi.fn(() => () => {}) },
    getApi: vi.fn(() => ({})),
    getBlockMetaKeyAtom: vi.fn(),
    getOverrideConfigAtom: vi.fn(() => vi.fn()),
    getSettingsKeyAtom: vi.fn(() => vi.fn()),
    isDev: false,
    openLink: vi.fn(),
    setBlockUploadState: vi.fn(),
    WOS: { makeORef: vi.fn((otype: string, oid: string) => `${otype}:${oid}`) },
    fetchWaveFile: vi.fn(),
}));
vi.mock("@/store/services", () => ({ BlockService: {} }));
vi.mock("@/app/store/badge", () => ({ setBadge: vi.fn() }));
vi.mock("@/app/store/wps", () => ({ getFileSubject: vi.fn(() => fileSubject) }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: { GetRTInfoCommand: getRTInfoCommand } }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/util/platformutil", () => ({ PLATFORM: "linux", PlatformMacOS: false }));
vi.mock("@/util/util", () => ({ base64ToArray: vi.fn(), fireAndForget: vi.fn(), isSshConnName: vi.fn() }));
vi.mock("debug", () => ({ default: () => vi.fn() }));
vi.mock("throttle-debounce", () => ({ debounce: vi.fn((_: any, fn: any) => fn) }));

import { TermWrap } from "./termwrap";

// A TermWrap shell with only the fields initTerminal/dispose/runProcessIdleTimeout touch, so
// the lifecycle can be driven without a real xterm instance.
function makeTermWrapShell(): TermWrap {
    const tw = Object.create(TermWrap.prototype) as TermWrap;
    const disposable = () => ({ dispose: vi.fn() });
    Object.assign(tw, {
        blockId: "block-1",
        disposed: false,
        idleTimeoutId: null,
        idleCallbackId: null,
        _visibilityChangeHandler: null,
        _writtenImageHashes: new Set(),
        promptMarkers: [],
        toDispose: [],
        heldData: [],
        loaded: false,
        webglContextLossDisposable: null,
        terminal: { onData: vi.fn(disposable), onSelectionChange: vi.fn(disposable), dispose: vi.fn() },
        onSearchResultsDidChange: null,
        processAndCacheData: vi.fn(),
        loadInitialTerminalData: vi.fn(async () => {}),
    });
    return tw;
}

describe("TermWrap lifecycle", () => {
    let idleCallbacks: Map<number, () => void>;
    let nextIdleId: number;

    beforeEach(() => {
        vi.useFakeTimers();
        idleCallbacks = new Map();
        nextIdleId = 1;
        vi.stubGlobal("requestIdleCallback", (cb: () => void) => {
            const id = nextIdleId++;
            idleCallbacks.set(id, cb);
            return id;
        });
        vi.stubGlobal("cancelIdleCallback", (id: number) => idleCallbacks.delete(id));
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.unstubAllGlobals();
        vi.restoreAllMocks();
    });

    function runIdleCallbacks() {
        const pending = [...idleCallbacks.values()];
        idleCallbacks.clear();
        pending.forEach((cb) => cb());
    }

    it("stops the idle cache loop once disposed while the timeout is pending", () => {
        const tw = makeTermWrapShell();
        tw.runProcessIdleTimeout();
        vi.advanceTimersByTime(5000);
        runIdleCallbacks();
        expect(tw.processAndCacheData).toHaveBeenCalledTimes(1);

        tw.dispose();
        vi.advanceTimersByTime(20000);
        runIdleCallbacks();
        expect(tw.processAndCacheData).toHaveBeenCalledTimes(1);
        expect(vi.getTimerCount()).toBe(0);
    });

    it("drops an idle callback that was already queued when dispose ran", () => {
        const tw = makeTermWrapShell();
        tw.runProcessIdleTimeout();
        vi.advanceTimersByTime(5000);
        expect(idleCallbacks.size).toBe(1);

        tw.dispose();
        expect(idleCallbacks.size).toBe(0);
        expect(tw.processAndCacheData).not.toHaveBeenCalled();
        expect(vi.getTimerCount()).toBe(0);
    });

    it("does not register the visibility listener or start the idle loop when disposed mid-init", async () => {
        const tw = makeTermWrapShell();
        let resolveRTInfo: (v: unknown) => void;
        getRTInfoCommand.mockReturnValueOnce(new Promise((r) => (resolveRTInfo = r)));
        const addListener = vi.spyOn(document, "addEventListener");

        const init = tw.initTerminal();
        tw.dispose();
        resolveRTInfo(null);
        await init;

        expect(tw.loadInitialTerminalData).not.toHaveBeenCalled();
        expect(addListener.mock.calls.some(([type]) => type === "visibilitychange")).toBe(false);
        expect(tw._visibilityChangeHandler).toBeNull();
        expect(vi.getTimerCount()).toBe(0);
    });

    it("does not register the visibility listener when disposed while initial data loads", async () => {
        const tw = makeTermWrapShell();
        getRTInfoCommand.mockResolvedValueOnce(null);
        let resolveLoad: () => void;
        tw.loadInitialTerminalData = vi.fn(() => new Promise<void>((r) => (resolveLoad = r)));
        const addListener = vi.spyOn(document, "addEventListener");

        const init = tw.initTerminal();
        await vi.waitFor(() => expect(tw.loadInitialTerminalData).toHaveBeenCalled());
        tw.dispose();
        resolveLoad();
        await init;

        expect(tw.loaded).toBe(false);
        expect(addListener.mock.calls.some(([type]) => type === "visibilitychange")).toBe(false);
        expect(vi.getTimerCount()).toBe(0);
    });

    it("registers the listener and starts the loop when init completes normally", async () => {
        const tw = makeTermWrapShell();
        getRTInfoCommand.mockResolvedValueOnce(null);

        await tw.initTerminal();

        expect(tw.loaded).toBe(true);
        expect(tw._visibilityChangeHandler).not.toBeNull();
        expect(vi.getTimerCount()).toBe(1);

        const removeListener = vi.spyOn(document, "removeEventListener");
        tw.dispose();
        expect(removeListener).toHaveBeenCalledWith("visibilitychange", expect.any(Function));
        expect(vi.getTimerCount()).toBe(0);
    });
});
