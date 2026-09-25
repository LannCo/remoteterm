// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { RenderProcessGoneDetails } from "electron";

export interface TabLifecycleTarget {
    remoteTermTabId: string;
    isDestroyed: boolean;
    webContents: { reload: () => void };
}

// -3 is ERR_ABORTED, fired for routine navigation cancellation, not a real load failure.
const AbortedLoadErrorCode = -3;

export function handleTabRenderProcessGone(tabView: TabLifecycleTarget, details: RenderProcessGoneDetails): void {
    console.log(`[render-process-gone] tab=${tabView.remoteTermTabId} reason=${details.reason} ts=${Date.now()}`);
    if (tabView.isDestroyed) {
        return;
    }
    tabView.webContents.reload();
}

export function handleTabUnresponsive(tabView: TabLifecycleTarget): void {
    console.log(`[unresponsive] tab=${tabView.remoteTermTabId} ts=${Date.now()}`);
}

export function handleTabDidFailLoad(
    tabView: TabLifecycleTarget,
    errorCode: number,
    errorDescription: string,
    isMainFrame: boolean
): void {
    if (!isMainFrame || errorCode === AbortedLoadErrorCode) {
        return;
    }
    console.log(
        `[did-fail-load] tab=${tabView.remoteTermTabId} errorCode=${errorCode} errorDescription=${errorDescription} ts=${Date.now()}`
    );
    if (tabView.isDestroyed) {
        return;
    }
    tabView.webContents.reload();
}
