// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

type DestroyableWindow = { isDestroyed(): boolean };

// The quake window is the primary main window in this app (emain-window.ts), so it is
// a valid target; the caller reveals a hidden one through the quake show path.
export function pickTerminalWindow<T extends DestroyableWindow>(lastFocused: T, all: T[]): T {
    if (lastFocused != null && !lastFocused.isDestroyed()) {
        return lastFocused;
    }
    return all.find((win) => !win.isDestroyed()) ?? null;
}

type BuilderWindowLike = { builderId: string; builderAppId?: string };

// One window per app keeps one controller, one watcher and one build per app folder.
export function findBuilderWindowForApp<T extends BuilderWindowLike>(
    windows: T[],
    appId: string,
    excludeBuilderId: string
): T {
    if (!appId) {
        return null;
    }
    return windows.find((win) => win.builderId !== excludeBuilderId && win.builderAppId === appId) ?? null;
}
