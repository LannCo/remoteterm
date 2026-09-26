// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { EventEmitter } from "events";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthKey, configureAuthKeyRequestInjection } from "./authkey";
import {
    applyGuestWebPreferences,
    hardenWebviewAttach,
    installPermissionHandlers,
    isAllowedPopupUrl,
    isAppWebContentsId,
    isPermissionAllowed,
    registerAppWebContents,
    WebBlockPartition,
} from "./emain-websecurity";

const { ipcHandlers } = vi.hoisted(() => {
    process.env.WAVE_SERVER_WEB_ENDPOINT = "127.0.0.1:61001";
    process.env.WAVE_SERVER_WS_ENDPOINT = "127.0.0.1:61002";
    return { ipcHandlers: new Map<string, (event: any) => void>() };
});
vi.mock("electron", () => ({
    ipcMain: { on: (channel: string, fn: (event: any) => void) => ipcHandlers.set(channel, fn) },
}));

let nextId = 1000;
function makeWebContents(): any {
    const wc = new EventEmitter() as any;
    wc.id = nextId++;
    return wc;
}

const PreloadPath = "/app/preload/preload-webview.cjs";

beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => {});
});

describe("app webContents registry", () => {
    it("tracks registered webContents until they are destroyed", () => {
        const wc = makeWebContents();
        expect(isAppWebContentsId(wc.id)).toBe(false);
        registerAppWebContents(wc);
        expect(isAppWebContentsId(wc.id)).toBe(true);
        wc.emit("destroyed");
        expect(isAppWebContentsId(wc.id)).toBe(false);
    });

    it("treats a missing id as not app-owned", () => {
        expect(isAppWebContentsId(undefined)).toBe(false);
        expect(isAppWebContentsId(null)).toBe(false);
    });
});

describe("auth key injection", () => {
    function captureListener() {
        let filter: any;
        let listener: (details: any, cb: (resp: any) => void) => void;
        const session: any = {
            webRequest: {
                onBeforeSendHeaders: (f: any, l: any) => {
                    filter = f;
                    listener = l;
                },
            },
        };
        configureAuthKeyRequestInjection(session);
        return { filter, listener };
    }

    function send(listener: any, webContentsId: number | undefined) {
        let result: any;
        listener({ webContentsId, requestHeaders: { Accept: "*/*" } }, (resp: any) => (result = resp));
        return result.requestHeaders;
    }

    it("filters on the server endpoints only", () => {
        const { filter } = captureListener();
        expect(filter.urls).toEqual(["http://127.0.0.1:61001/*", "ws://127.0.0.1:61002/*"]);
    });

    it("adds X-AuthKey for app-owned webContents", () => {
        const wc = makeWebContents();
        registerAppWebContents(wc);
        const { listener } = captureListener();
        expect(send(listener, wc.id)["X-AuthKey"]).toBe(AuthKey);
    });

    it("does not add X-AuthKey for web-block guests", () => {
        const guest = makeWebContents();
        const { listener } = captureListener();
        expect(send(listener, guest.id)).toEqual({ Accept: "*/*" });
    });

    it("adds X-AuthKey for requests with no webContents (main process)", () => {
        // Only Electron's own main-process net.request/fetch calls surface a null/undefined
        // webContentsId here - any request a web page can trigger always carries that page's
        // own (non-app) webContents id, so this can't be spoofed by a web block.
        const { listener } = captureListener();
        expect(send(listener, undefined)["X-AuthKey"]).toBe(AuthKey);
    });

    it("stops adding X-AuthKey once the app webContents is destroyed", () => {
        const wc = makeWebContents();
        registerAppWebContents(wc);
        wc.emit("destroyed");
        const { listener } = captureListener();
        expect(send(listener, wc.id)["X-AuthKey"]).toBeUndefined();
    });

    it("only returns the key over IPC to app-owned senders", () => {
        const app = makeWebContents();
        registerAppWebContents(app);
        const guest = makeWebContents();
        const handler = ipcHandlers.get("get-auth-key");
        const appEvent: any = { sender: app };
        const guestEvent: any = { sender: guest };
        handler(appEvent);
        handler(guestEvent);
        expect(appEvent.returnValue).toBe(AuthKey);
        expect(guestEvent.returnValue).toBeNull();
    });
});

describe("permissions", () => {
    it("allows everything for app-owned webContents", () => {
        const wc = makeWebContents();
        registerAppWebContents(wc);
        expect(isPermissionAllowed(wc, "media")).toBe(true);
        expect(isPermissionAllowed(wc, "notifications")).toBe(true);
    });

    it("denies sensitive permissions for guests and unknown callers", () => {
        const guest = makeWebContents();
        for (const perm of ["media", "geolocation", "notifications", "clipboard-read", "hid", "serial", "usb"]) {
            expect(isPermissionAllowed(guest, perm)).toBe(false);
            expect(isPermissionAllowed(null, perm)).toBe(false);
        }
        expect(isPermissionAllowed(guest, "fullscreen")).toBe(true);
        expect(isPermissionAllowed(guest, "clipboard-sanitized-write")).toBe(true);
    });

    it("installs matching request and check handlers on a session", () => {
        let requestHandler: any;
        let checkHandler: any;
        const session: any = {
            setPermissionRequestHandler: (h: any) => (requestHandler = h),
            setPermissionCheckHandler: (h: any) => (checkHandler = h),
        };
        installPermissionHandlers(session);
        const guest = makeWebContents();
        const cb = vi.fn();
        requestHandler(guest, "media", cb);
        expect(cb).toHaveBeenCalledWith(false);
        expect(checkHandler(guest, "geolocation")).toBe(false);
        expect(checkHandler(guest, "fullscreen")).toBe(true);
    });
});

describe("hardenWebviewAttach", () => {
    it("moves unpartitioned web blocks off the default session", () => {
        const prefs: any = { preload: PreloadPath };
        expect(hardenWebviewAttach(prefs, { src: "https://example.com" }, PreloadPath)).toBe(true);
        expect(prefs.partition).toBe(WebBlockPartition);
        expect(prefs.preload).toBe(PreloadPath);
    });

    it("keeps an explicit partition", () => {
        const prefs: any = { partition: "tsunami:abc" };
        hardenWebviewAttach(prefs, { src: "http://localhost:1234/" }, PreloadPath);
        expect(prefs.partition).toBe("tsunami:abc");
    });

    it("forces secure web preferences and drops an unexpected preload", () => {
        const prefs: any = {
            preload: "/tmp/evil.js",
            nodeIntegration: true,
            nodeIntegrationInSubFrames: true,
            contextIsolation: false,
            sandbox: false,
            webSecurity: false,
            allowRunningInsecureContent: true,
            experimentalFeatures: true,
            webviewTag: true,
            enableBlinkFeatures: "Foo",
        };
        hardenWebviewAttach(prefs, { src: "https://example.com" }, PreloadPath);
        expect(prefs).toEqual({
            nodeIntegration: false,
            nodeIntegrationInSubFrames: false,
            nodeIntegrationInWorker: false,
            contextIsolation: true,
            sandbox: true,
            webSecurity: true,
            allowRunningInsecureContent: false,
            experimentalFeatures: false,
            webviewTag: false,
            partition: WebBlockPartition,
        });
    });

    it("allows web, file and about sources and refuses other schemes", () => {
        for (const src of ["", "about:blank", "http://x.test/", "https://x.test/", "file:///tmp/a.html"]) {
            expect(hardenWebviewAttach({} as any, { src }, PreloadPath)).toBe(true);
        }
        for (const src of ["javascript:alert(1)", "chrome://gpu", "devtools://devtools", "not a url"]) {
            expect(hardenWebviewAttach({} as any, { src }, PreloadPath)).toBe(false);
        }
    });
});

describe("applyGuestWebPreferences", () => {
    it("forces the same secure values hardenWebviewAttach forces", () => {
        const prefs: any = { nodeIntegration: true, sandbox: false, webviewTag: true, enableBlinkFeatures: "Foo" };
        applyGuestWebPreferences(prefs);
        expect(prefs).toEqual({
            nodeIntegration: false,
            nodeIntegrationInSubFrames: false,
            nodeIntegrationInWorker: false,
            contextIsolation: true,
            sandbox: true,
            webSecurity: true,
            allowRunningInsecureContent: false,
            experimentalFeatures: false,
            webviewTag: false,
        });
    });

    it("leaves partition and preload alone", () => {
        const prefs: any = { partition: "persist:x", preload: PreloadPath };
        applyGuestWebPreferences(prefs);
        expect(prefs.partition).toBe("persist:x");
        expect(prefs.preload).toBe(PreloadPath);
    });
});

describe("isAllowedPopupUrl", () => {
    it("allows http, https and about only", () => {
        for (const u of ["http://x.test/", "https://x.test/a?b", "about:blank"]) {
            expect(isAllowedPopupUrl(u)).toBe(true);
        }
        for (const u of ["file:///etc/passwd", "javascript:alert(1)", "chrome://gpu", "data:text/html,x", "", undefined, "not a url"]) {
            expect(isAllowedPopupUrl(u)).toBe(false);
        }
    });
});
