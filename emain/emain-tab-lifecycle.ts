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
export const MaxConsecutiveReloads = 6;
export const BaseReloadDelayMs = 1000;

interface ReloadState {
    failures: number;
    timer: ReturnType<typeof setTimeout>;
}

const reloadStates = new WeakMap<TabLifecycleTarget, ReloadState>();

// A crash and a failed load can both fire for one failure, so a pending reload absorbs later events.
function scheduleTabReload(tabView: TabLifecycleTarget, cause: string): void {
    if (tabView.isDestroyed) {
        return;
    }
    if (!reloadStates.has(tabView)) {
        reloadStates.set(tabView, { failures: 0, timer: null });
    }
    const state = reloadStates.get(tabView);
    if (state.timer != null) {
        return;
    }
    if (state.failures >= MaxConsecutiveReloads) {
        console.log(`[reload-giveup] tab=${tabView.remoteTermTabId} cause=${cause} failures=${state.failures}`);
        return;
    }
    const delayMs = BaseReloadDelayMs * 2 ** state.failures;
    state.failures++;
    console.log(
        `[reload-scheduled] tab=${tabView.remoteTermTabId} cause=${cause} attempt=${state.failures} delayMs=${delayMs}`
    );
    state.timer = setTimeout(() => {
        state.timer = null;
        if (tabView.isDestroyed) {
            return;
        }
        tabView.webContents.reload();
    }, delayMs);
}

export function handleTabLoadSucceeded(tabView: TabLifecycleTarget): void {
    const state = reloadStates.get(tabView);
    if (state == null) {
        return;
    }
    state.failures = 0;
}

export function handleTabRenderProcessGone(tabView: TabLifecycleTarget, details: RenderProcessGoneDetails): void {
    console.log(`[render-process-gone] tab=${tabView.remoteTermTabId} reason=${details.reason} ts=${Date.now()}`);
    scheduleTabReload(tabView, `render-process-gone:${details.reason}`);
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
    scheduleTabReload(tabView, `did-fail-load:${errorCode}`);
}
