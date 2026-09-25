// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import electron from "electron";

function broadcastNativeThemeChanged(shouldUseDarkColors: boolean): void {
    for (const wc of electron.webContents.getAllWebContents()) {
        if (wc.isDestroyed()) {
            continue;
        }
        wc.send("native-theme-change", shouldUseDarkColors);
    }
}

export function registerNativeThemeListener(): void {
    electron.nativeTheme.on("updated", () => {
        broadcastNativeThemeChanged(electron.nativeTheme.shouldUseDarkColors);
    });
}
