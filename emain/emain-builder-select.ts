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

export const BuilderTeardownTimeoutMs = 20000;

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
