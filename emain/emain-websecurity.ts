// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { Session, WebContents, WebPreferences } from "electron";

// Web blocks must never share the default session: the app's X-AuthKey is injected there.
export const WebBlockPartition = "persist:webblock";

const AllowedWebviewSrcProtocols = new Set(["http:", "https:", "file:", "about:"]);
// Popups never need file:; an OAuth or share popup that navigates to file: is hostile.
// about: popups inherit the opener's webPreferences, not buildPopupWindowOptions' override;
// hardenCreatedPopup destroys any popup whose opener is not hardened.
const AllowedPopupUrlProtocols = new Set(["http:", "https:", "about:"]);
const GuestAllowedPermissions = new Set(["fullscreen", "clipboard-sanitized-write"]);

const appWebContentsIds = new Set<number>();

export function registerAppWebContents(wc: WebContents): void {
    const id = wc.id;
    appWebContentsIds.add(id);
    wc.once("destroyed", () => appWebContentsIds.delete(id));
}

export function isAppWebContentsId(id: number | null | undefined): boolean {
    return id != null && appWebContentsIds.has(id);
}

export function isPermissionAllowed(wc: WebContents | null, permission: string): boolean {
    if (wc != null && isAppWebContentsId(wc.id)) {
        return true;
    }
    return GuestAllowedPermissions.has(permission);
}

export function installPermissionHandlers(session: Session): void {
    session.setPermissionRequestHandler((wc, permission, callback) => {
        const allowed = isPermissionAllowed(wc, permission);
        if (!allowed) {
            console.log(`[permission] denied ${permission} for webContents=${wc?.id}`);
        }
        callback(allowed);
    });
    session.setPermissionCheckHandler((wc, permission) => isPermissionAllowed(wc, permission));
}

function isAllowedWebviewSrc(src: string | undefined): boolean {
    if (!src) {
        return true;
    }
    try {
        return AllowedWebviewSrcProtocols.has(new URL(src).protocol);
    } catch {
        return false;
    }
}

export function applyGuestWebPreferences(webPreferences: WebPreferences): void {
    webPreferences.nodeIntegration = false;
    webPreferences.nodeIntegrationInSubFrames = false;
    webPreferences.nodeIntegrationInWorker = false;
    webPreferences.contextIsolation = true;
    webPreferences.sandbox = true;
    webPreferences.webSecurity = true;
    webPreferences.allowRunningInsecureContent = false;
    webPreferences.experimentalFeatures = false;
    webPreferences.webviewTag = false;
    delete webPreferences.enableBlinkFeatures;
}

export function isAllowedPopupUrl(url: string | undefined): boolean {
    if (!url) {
        return false;
    }
    try {
        return AllowedPopupUrlProtocols.has(new URL(url).protocol);
    } catch {
        return false;
    }
}

// Returns false when the attach must be refused.
export function hardenWebviewAttach(
    webPreferences: WebPreferences,
    params: Record<string, string>,
    webviewPreloadPath: string
): boolean {
    if (webPreferences.preload != null && webPreferences.preload !== webviewPreloadPath) {
        console.log(`[will-attach-webview] dropping unexpected preload ${webPreferences.preload}`);
        delete webPreferences.preload;
    }
    applyGuestWebPreferences(webPreferences);
    if (!webPreferences.partition) {
        webPreferences.partition = WebBlockPartition;
    }
    if (!isAllowedWebviewSrc(params.src)) {
        console.log(`[will-attach-webview] refusing src ${params.src}`);
        return false;
    }
    return true;
}
