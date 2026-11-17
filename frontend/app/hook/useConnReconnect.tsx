// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { BlockEnv } from "@/app/block/blockenv";
import { useWaveEnv } from "@/app/remotetermenv/remotetermenv";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import * as React from "react";

function rpcErrorMessage(e: unknown): string {
    if (e instanceof Error && e.message !== "") {
        return e.message;
    }
    const s = String(e);
    return s === "" || s === "undefined" ? "reconnect failed" : s;
}

/**
 * Shared click action for every reconnect button in the connection status
 * overlay (disconnected, gave-up, countdown, stalled, and the legacy shell).
 *
 * Always sends force: true. A plain Connect is refused by the backend while the
 * connection is connecting or connected — exactly the states a network flap
 * leaves behind (a hung dial holds the connect lock, a half-open TCP session
 * still reports connected until stall detection trips) — and the refusal was
 * previously only visible in the server log, so clicks appeared dead.
 * ForceReconnect degrades to a plain connect from disconnected/error, closes an
 * in-flight or half-open session involuntarily (password cache preserved), and
 * no-ops against a healthy connection.
 *
 * Exposes in-flight and error state so the overlay can acknowledge the click
 * immediately and surface a rejection instead of swallowing it.
 */
export function useConnReconnectAction(connName: string, blockId: string, connStatus?: ConnStatus) {
    const waveEnv = useWaveEnv<BlockEnv>();
    const inFlightRef = React.useRef(false);
    const [inFlight, setInFlight] = React.useState(false);
    const [error, setError] = React.useState<string | null>(null);

    // A stale error must not resurface next to a newer attempt's state: once a
    // connect is actually underway (or the connection is back), the rejection
    // it would have reported is obsolete.
    React.useEffect(() => {
        if (connStatus?.status === "connecting" || connStatus?.status === "connected") {
            setError(null);
        }
    }, [connStatus?.status]);

    // The block switched to a different connection: drop state from the old one.
    React.useEffect(() => {
        inFlightRef.current = false;
        setInFlight(false);
        setError(null);
    }, [connName]);

    const reconnect = React.useCallback(() => {
        if (inFlightRef.current) {
            return;
        }
        inFlightRef.current = true;
        setError(null);
        setInFlight(true);
        const prtn = waveEnv.rpc.ConnConnectCommand(
            TabRpcClient,
            { host: connName, logblockid: blockId, force: true },
            { timeout: 60000 }
        );
        prtn
            .catch((e: unknown) => {
                setError(rpcErrorMessage(e));
            })
            .finally(() => {
                inFlightRef.current = false;
                setInFlight(false);
            });
    }, [connName, blockId, waveEnv]);

    return { reconnect, inFlight, error };
}