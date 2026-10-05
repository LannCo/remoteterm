// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

type BuilderWindowLike = { builderId: string; builderAppId?: string; tearingDown?: boolean };

// One window per app keeps one controller, one watcher and one build per app folder. A window that is
// tearing down is hidden and about to be destroyed, so it never counts as the window for an app.
export function findBuilderWindowForApp<T extends BuilderWindowLike>(
    windows: T[],
    appId: string,
    excludeBuilderId: string
): T {
    if (!appId) {
        return null;
    }
    return (
        windows.find((win) => win.builderId !== excludeBuilderId && !win.tearingDown && win.builderAppId === appId) ??
        null
    );
}

export const OpenPathGraceMs = 250;

// shell.openPath settles only when the opener process exits, and a file manager keeps running,
// so awaiting it left the renderer's promise pending for as long as the window stayed open. A
// failure that arrives inside the grace period is returned to the caller; a later one is logged.
export async function openPathDetached(
    openPath: (target: string) => Promise<string>,
    target: string,
    logError: (message: string) => void,
    graceMs: number = OpenPathGraceMs
): Promise<string> {
    let settled: Promise<string>;
    try {
        settled = openPath(target).then(
            (err) => err ?? "",
            (e) => `Could not open the folder: ${e instanceof Error ? e.message : String(e)}`
        );
    } catch (e) {
        return `Could not open the folder: ${e instanceof Error ? e.message : String(e)}`;
    }
    let timer: ReturnType<typeof setTimeout>;
    const grace = new Promise<null>((resolve) => {
        timer = setTimeout(() => resolve(null), graceMs);
    });
    const early = await Promise.race([settled, grace]);
    clearTimeout(timer);
    if (early != null) {
        return early;
    }
    void settled.then((err) => {
        if (err) {
            logError(`Failed to open ${target}: ${err}`);
        }
    });
    return "";
}

export const MaxBuilderTargetLen = 64;
export const BuilderTeardownTimeoutMs = 20000;

const InvalidTargetMessage = "Invalid terminal target.";

export type ParsedBuilderTerminalTarget = {
    targetblockid: string;
    targetaction: string;
    error?: string;
};

// The server validates the action and the block, but the IPC argument is still untyped data from a
// renderer, so only short strings are forwarded.
export function parseBuilderTerminalTarget(target: unknown): ParsedBuilderTerminalTarget {
    const rtn: ParsedBuilderTerminalTarget = { targetblockid: "", targetaction: "" };
    if (target == null) {
        return rtn;
    }
    if (typeof target !== "object") {
        return { ...rtn, error: InvalidTargetMessage };
    }
    const fields = target as Record<string, unknown>;
    for (const key of ["targetblockid", "targetaction"] as const) {
        const val = fields[key];
        if (val == null) {
            continue;
        }
        if (typeof val !== "string" || val.length > MaxBuilderTargetLen) {
            return { targetblockid: "", targetaction: "", error: InvalidTargetMessage };
        }
        rtn[key] = val;
    }
    return rtn;
}

export type TeardownWindow = {
    tearingDown?: boolean;
    isDestroyed(): boolean;
    hide(): void;
};

export type BuilderTeardownSteps = {
    deleteBuilder: () => Promise<unknown>;
    deleteRtInfo: () => Promise<unknown>;
    destroyWindow: () => void;
    logError: (message: string, err: unknown) => void;
};

// The teardown can take up to BuilderTeardownTimeoutMs, so the window is hidden at once and a second close
// request meanwhile (Alt+W again, set-builder-window-appid) is ignored. The builder's terminals are deleted
// while its rtinfo still exists, and the window goes last whatever failed, so a dead server cannot keep it open.
export async function runBuilderTeardown(win: TeardownWindow, steps: BuilderTeardownSteps): Promise<void> {
    if (win.tearingDown || win.isDestroyed()) {
        return;
    }
    win.tearingDown = true;
    win.hide();
    try {
        await steps.deleteBuilder();
    } catch (e) {
        steps.logError("Error deleting builder:", e);
    }
    try {
        await steps.deleteRtInfo();
    } catch (e) {
        steps.logError("Error deleting builder rtinfo:", e);
    }
    if (!win.isDestroyed()) {
        steps.destroyWindow();
    }
}
