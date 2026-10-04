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
