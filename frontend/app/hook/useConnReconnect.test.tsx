// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    connConnectMock: vi.fn(),
}));

vi.mock("@/app/store/wshrpcutil", () => ({
    TabRpcClient: { clientid: "test-client" },
}));
vi.mock("@/app/remotetermenv/remotetermenv", () => ({
    useWaveEnv: () => ({ rpc: { ConnConnectCommand: h.connConnectMock } }),
}));

import { useConnReconnectAction } from "./useConnReconnect";

type HookProps = { connName: string; blockId: string; connStatus?: ConnStatus };

function renderReconnectHook(initial: HookProps) {
    return renderHook((props: HookProps) => useConnReconnectAction(props.connName, props.blockId, props.connStatus), {
        initialProps: initial,
    });
}

function deferredPromise() {
    let resolve!: () => void;
    let reject!: (e: Error) => void;
    const promise = new Promise<void>((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
}

beforeEach(() => {
    h.connConnectMock.mockReset();
    h.connConnectMock.mockReturnValue(Promise.resolve());
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe("useConnReconnectAction", () => {
    it("sends force:true with the block's logblockid (escalation past connecting/connected refusals)", () => {
        const { result } = renderReconnectHook({ connName: "host1", blockId: "block-1" });

        act(() => {
            result.current.reconnect();
        });

        expect(h.connConnectMock).toHaveBeenCalledTimes(1);
        const [client, data, opts] = h.connConnectMock.mock.calls[0];
        expect(data).toEqual({ host: "host1", logblockid: "block-1", force: true });
        expect(opts).toEqual({ timeout: 60000 });
        expect(client).toEqual({ clientid: "test-client" });
    });

    it("flags in-flight immediately and clears it when the RPC settles", async () => {
        const deferred = deferredPromise();
        h.connConnectMock.mockReturnValue(deferred.promise);
        const { result } = renderReconnectHook({ connName: "host1", blockId: "block-1" });

        act(() => {
            result.current.reconnect();
        });
        expect(result.current.inFlight).toBe(true);

        await act(async () => {
            deferred.resolve();
        });
        await waitFor(() => expect(result.current.inFlight).toBe(false));
    });

    it("debounces a second click while the first RPC is still pending", () => {
        const deferred = deferredPromise();
        h.connConnectMock.mockReturnValue(deferred.promise);
        const { result } = renderReconnectHook({ connName: "host1", blockId: "block-1" });

        act(() => {
            result.current.reconnect();
            result.current.reconnect();
        });

        expect(h.connConnectMock).toHaveBeenCalledTimes(1);
    });

    it("surfaces a rejection as error text and allows a follow-up click", async () => {
        const deferred = deferredPromise();
        h.connConnectMock.mockReturnValue(deferred.promise);
        const { result } = renderReconnectHook({ connName: "host1", blockId: "block-1" });

        act(() => {
            result.current.reconnect();
        });
        await act(async () => {
            deferred.reject(new Error('cannot connect to "host1" when status is "connecting"'));
        });
        await waitFor(() => expect(result.current.error).toBe('cannot connect to "host1" when status is "connecting"'));
        expect(result.current.inFlight).toBe(false);

        // After the failed attempt settles, clicking again must go through.
        act(() => {
            result.current.reconnect();
        });
        expect(h.connConnectMock).toHaveBeenCalledTimes(2);
    });

    it("clears the error once the connection is connecting or connected again", async () => {
        h.connConnectMock.mockReturnValue(Promise.reject(new Error("boom")));
        const { result, rerender } = renderReconnectHook({ connName: "host1", blockId: "block-1" });

        act(() => {
            result.current.reconnect();
        });
        await waitFor(() => expect(result.current.error).toBe("boom"));

        rerender({ connName: "host1", blockId: "block-1", connStatus: { status: "connecting" } as ConnStatus });
        await waitFor(() => expect(result.current.error).toBe(null));
    });

    it("drops in-flight and error state when the block switches connection", async () => {
        const deferred = deferredPromise();
        h.connConnectMock.mockReturnValue(deferred.promise);
        const { result, rerender } = renderReconnectHook({ connName: "host1", blockId: "block-1" });

        act(() => {
            result.current.reconnect();
        });
        expect(result.current.inFlight).toBe(true);

        rerender({ connName: "host2", blockId: "block-1" });
        await waitFor(() => expect(result.current.inFlight).toBe(false));
        expect(result.current.error).toBe(null);

        // The pending RPC of the old connection must not block a click for the new one.
        act(() => {
            result.current.reconnect();
        });
        expect(h.connConnectMock).toHaveBeenCalledTimes(2);
        const [, data] = h.connConnectMock.mock.calls[1];
        expect(data).toEqual({ host: "host2", logblockid: "block-1", force: true });
    });
});