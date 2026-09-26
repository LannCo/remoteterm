// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { session } from "electron";
import type {
    BaseWindow,
    BrowserWindow,
    BrowserWindowConstructorOptions,
    DidCreateWindowDetails,
    HandlerDetails,
    Rectangle,
    WebContents,
    WindowOpenHandlerResponse,
} from "electron";
import { applyGuestWebPreferences, isAllowedPopupUrl, isAppWebContentsId } from "./emain-websecurity";

export type WindowOpenRoute = "popup" | "tab" | "deny";

export const MaxLivePopupsPerOpener = 4;
const DefaultPopupWidth = 520;
const DefaultPopupHeight = 640;
const MinPopupWidth = 200;
const MinPopupHeight = 150;
const FalseyFeatureValues = new Set(["0", "no", "false"]);

// Every key the page's features string is allowed to influence. openGuestWindow spreads
// every parsed feature key straight into BrowserWindowConstructorOptions (Electron 41.1.0
// guest-window-manager.ts), so a key outside this set (frame, transparent, alwaysOnTop,
// webContents, kiosk, ...) lets the page dictate native window chrome or, via webContents,
// orphan the child onto a fresh default-session window (H-1/H-1b).
const AllowedPopupFeatureKeys = new Set([
    "popup",
    "width",
    "height",
    "left",
    "top",
    "innerwidth",
    "innerheight",
    "screenx",
    "screeny",
    "noopener",
    "noreferrer",
    "resizable",
    "scrollbars",
    "status",
    "toolbar",
    "menubar",
    "location",
]);

export function parseWindowFeatures(features: string | undefined): Record<string, string | boolean> {
    const rtn: Record<string, string | boolean> = {};
    if (!features) {
        return rtn;
    }
    for (const rawPart of features.split(",")) {
        const part = rawPart.trim();
        if (part === "") {
            continue;
        }
        const eq = part.indexOf("=");
        if (eq < 0) {
            rtn[part.toLowerCase()] = true;
            continue;
        }
        const key = part.slice(0, eq).trim().toLowerCase();
        const value = part.slice(eq + 1).trim();
        rtn[key] = FalseyFeatureValues.has(value.toLowerCase()) ? false : value;
    }
    return rtn;
}

function featureNumber(parsed: Record<string, string | boolean>, ...keys: string[]): number | undefined {
    for (const key of keys) {
        const v = parsed[key];
        if (typeof v !== "string") {
            continue;
        }
        const n = parseInt(v, 10);
        if (!Number.isNaN(n)) {
            return n;
        }
    }
    return undefined;
}

export function requestedPopupGeometry(features: string | undefined): {
    width?: number;
    height?: number;
    x?: number;
    y?: number;
} {
    const parsed = parseWindowFeatures(features);
    return {
        width: featureNumber(parsed, "width", "innerwidth"),
        height: featureNumber(parsed, "height", "innerheight"),
        x: featureNumber(parsed, "left", "screenx"),
        y: featureNumber(parsed, "top", "screeny"),
    };
}

function hasDisallowedFeatureKey(parsed: Record<string, string | boolean>): boolean {
    return Object.keys(parsed).some((key) => !AllowedPopupFeatureKeys.has(key));
}

// Chromium reports "new-window" only when window.open() was given a features string
// (its NEW_POPUP disposition); plain window.open(url) and target=_blank are tabs.
export function classifyWindowOpen(details: Pick<HandlerDetails, "url" | "disposition" | "features">): WindowOpenRoute {
    if (details.disposition !== "new-window") {
        return "tab";
    }
    // A shift-clicked link also arrives as "new-window" but with no renderer-created child
    // and an empty features string. Left unhandled, Electron builds a fresh BrowserWindow
    // on the default session that hardenCreatedPopup then destroys, so the user sees
    // nothing happen (M-1). Plain window.open(url) already arrives as foreground-tab, so
    // this only catches the browser-initiated case.
    if (details.features == null || details.features.trim() === "") {
        return "tab";
    }
    const parsed = parseWindowFeatures(details.features);
    if (hasDisallowedFeatureKey(parsed)) {
        return "tab";
    }
    if (parsed["noopener"] === true || parsed["noreferrer"] === true) {
        return "tab";
    }
    if (!isAllowedPopupUrl(details.url)) {
        return "deny";
    }
    return "popup";
}

function clamp(n: number, lo: number, hi: number): number {
    return Math.min(Math.max(n, lo), hi);
}

export function computePopupBounds(
    requested: { width?: number; height?: number; x?: number; y?: number },
    parentBounds: Rectangle,
    workArea: Rectangle
): Rectangle {
    const width = clamp(requested.width ?? DefaultPopupWidth, MinPopupWidth, workArea.width);
    const height = clamp(requested.height ?? DefaultPopupHeight, MinPopupHeight, workArea.height);
    const defaultX = parentBounds.x + Math.round((parentBounds.width - width) / 2);
    const defaultY = parentBounds.y + Math.round((parentBounds.height - height) / 2);
    const x = clamp(requested.x ?? defaultX, workArea.x, workArea.x + workArea.width - width);
    const y = clamp(requested.y ?? defaultY, workArea.y, workArea.y + workArea.height - height);
    return { x, y, width, height };
}

export type PopupHostContext = {
    rootGuestId: number;
    sendToTab: (guestId: number, details: HandlerDetails) => void;
    getParentWindow: () => BaseWindow | null;
    getWorkArea: (parent: BaseWindow | null) => Rectangle;
    preloadPath: string;
};

const livePopups = new Map<number, Set<BrowserWindow>>();

export function livePopupCount(rootGuestId: number): number {
    return livePopups.get(rootGuestId)?.size ?? 0;
}

function trackPopup(rootGuestId: number, win: BrowserWindow): void {
    let set = livePopups.get(rootGuestId);
    if (set == null) {
        set = new Set();
        livePopups.set(rootGuestId, set);
    }
    set.add(win);
    win.once("closed", () => {
        set.delete(win);
        if (set.size === 0) {
            livePopups.delete(rootGuestId);
        }
    });
}

function liveParent(ctx: PopupHostContext): BaseWindow | null {
    const parent = ctx.getParentWindow();
    if (parent == null || parent.isDestroyed()) {
        return null;
    }
    return parent;
}

export function buildPopupWindowOptions(
    details: HandlerDetails,
    opener: WebContents,
    ctx: PopupHostContext
): BrowserWindowConstructorOptions {
    const parent = liveParent(ctx);
    const workArea = ctx.getWorkArea(parent);
    const parentBounds = parent?.getBounds() ?? workArea;
    const bounds = computePopupBounds(requestedPopupGeometry(details.features), parentBounds, workArea);
    const webPreferences: BrowserWindowConstructorOptions["webPreferences"] = {
        preload: ctx.preloadPath,
        session: opener.session,
    };
    applyGuestWebPreferences(webPreferences);
    const opts: BrowserWindowConstructorOptions = {
        ...bounds,
        minWidth: MinPopupWidth,
        minHeight: MinPopupHeight,
        maxWidth: workArea.width,
        maxHeight: workArea.height,
        autoHideMenuBar: true,
        // openGuestWindow spreads the page's parsed features before this object, so these
        // fields must be pinned here too, not just filtered by classifyWindowOpen's
        // allowlist: any parsed key we missed still can't reach the native window (H-1).
        frame: true,
        transparent: false,
        fullscreen: false,
        fullscreenable: true,
        kiosk: false,
        alwaysOnTop: false,
        skipTaskbar: false,
        closable: true,
        minimizable: true,
        focusable: true,
        show: true,
        opacity: 1,
        modal: false,
        webPreferences,
    };
    if (parent != null) {
        opts.parent = parent;
    }
    return opts;
}

export function handleGuestWindowOpen(
    opener: WebContents,
    details: HandlerDetails,
    ctx: PopupHostContext
): WindowOpenHandlerResponse {
    if (opener == null || opener.isDestroyed()) {
        return { action: "deny" };
    }
    const route = classifyWindowOpen(details);
    if (route === "tab") {
        ctx.sendToTab(ctx.rootGuestId, details);
        return { action: "deny" };
    }
    if (route === "deny") {
        console.log("[popup] denied", details.url);
        return { action: "deny" };
    }
    if (livePopupCount(ctx.rootGuestId) >= MaxLivePopupsPerOpener) {
        console.log("[popup] cap reached for guest", ctx.rootGuestId);
        return { action: "deny" };
    }
    return { action: "allow", overrideBrowserWindowOptions: buildPopupWindowOptions(details, opener, ctx) };
}

function destroyPopup(child: BrowserWindow, reason: string): false {
    console.log(`[popup] destroying child window: ${reason}`);
    if (!child.isDestroyed()) {
        child.destroy();
    }
    return false;
}

// Returns false when the popup was refused and destroyed. The session checks are the
// backstop for the X-AuthKey scoping: a popup that reached the default session would
// otherwise be one isAppWebContentsId bug away from the RPC key.
export function hardenCreatedPopup(
    child: BrowserWindow,
    opener: WebContents,
    details: DidCreateWindowDetails,
    ctx: PopupHostContext
): boolean {
    const wc = child.webContents;
    if (wc.session !== opener.session) {
        return destroyPopup(child, "session differs from opener");
    }
    if (wc.session === session.defaultSession) {
        return destroyPopup(child, "popup on default session");
    }
    if (isAppWebContentsId(wc.id)) {
        return destroyPopup(child, "popup webContents is app-owned");
    }
    child.setMenuBarVisibility(false);
    wc.on("will-navigate", (event, url) => {
        if (!isAllowedPopupUrl(url)) {
            console.log("[popup] blocked navigation to", url);
            event.preventDefault();
        }
    });
    installGuestWindowOpenHandler(wc, ctx);
    trackPopup(ctx.rootGuestId, child);
    return true;
}

export function installGuestWindowOpenHandler(guest: WebContents, ctx: PopupHostContext): void {
    guest.setWindowOpenHandler((details) => handleGuestWindowOpen(guest, details, ctx));
    guest.on("did-create-window", (child, details) => {
        hardenCreatedPopup(child, guest, details, ctx);
    });
    if (guest.id === ctx.rootGuestId) {
        guest.once("destroyed", () => livePopups.delete(ctx.rootGuestId));
    }
}
