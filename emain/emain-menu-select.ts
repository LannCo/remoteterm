// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Kept apart from emain-menu.ts so the decisions can be tested without Electron.

// On macOS the app stays alive with no main window, and the menu then owns Cmd+N and Cmd+T so
// they can open one. A focused builder window uses Cmd+N for a terminal pane; a native
// accelerator would consume the key before the page sees it, so the hidden items step aside.
export function wantsHiddenNewWindowItems(numRemoteTermWindows: number, isBuilderWindowFocused: boolean): boolean {
    return numRemoteTermWindows === 0 && !isBuilderWindowFocused;
}

// The macOS menu bar is global: a File > Close click arrives from whichever window is focused,
// but the last-focused main window is remembered after focus has moved on. Popups and builder
// windows close themselves; everything else falls back to the main window.
export function pickCloseTarget<T>(
    clickedWindow: T,
    focusedMainWindow: T,
    isPopup: (window: T) => boolean,
    isBuilder: (window: T) => boolean
): T {
    if (clickedWindow != null && (isPopup(clickedWindow) || isBuilder(clickedWindow))) {
        return clickedWindow;
    }
    return focusedMainWindow ?? null;
}
