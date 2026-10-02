// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";
import { classifyWindowOpen, computePopupBounds, parseWindowFeatures } from "./emain-popup";

vi.mock("electron", () => ({
    session: { defaultSession: { id: "default" } },
}));

beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => {});
});

const Popup = { url: "https://accounts.example.test/auth", disposition: "new-window" as const };

describe("parseWindowFeatures", () => {
    it("parses key=value pairs and bare tokens", () => {
        expect(parseWindowFeatures("width=480, height=600,noopener,popup=yes,menubar=0")).toEqual({
            width: "480",
            height: "600",
            noopener: true,
            popup: "yes",
            menubar: false,
        });
    });

    it("handles empty and undefined", () => {
        expect(parseWindowFeatures("")).toEqual({});
        expect(parseWindowFeatures(undefined)).toEqual({});
    });
});

describe("classifyWindowOpen", () => {
    it("routes a new-window disposition with an opener to a popup", () => {
        expect(classifyWindowOpen({ ...Popup, features: "width=480,height=600" })).toBe("popup");
        expect(classifyWindowOpen({ url: "about:blank", disposition: "new-window", features: "width=1" })).toBe("popup");
    });

    it("routes tab dispositions to the existing pane path", () => {
        for (const disposition of ["foreground-tab", "background-tab", "default", "other"] as const) {
            expect(classifyWindowOpen({ url: Popup.url, disposition, features: "" })).toBe("tab");
        }
    });

    it("routes noopener/noreferrer popups to the pane path", () => {
        expect(classifyWindowOpen({ ...Popup, features: "noopener,width=480" })).toBe("tab");
        expect(classifyWindowOpen({ ...Popup, features: "noreferrer" })).toBe("tab");
        expect(classifyWindowOpen({ ...Popup, features: "noopener=0,width=480" })).toBe("popup");
    });

    it("denies popups to non-web schemes", () => {
        for (const url of ["file:///tmp/x.html", "javascript:alert(1)", "chrome://gpu", "data:text/html,x"]) {
            expect(classifyWindowOpen({ url, disposition: "new-window", features: "width=1" })).toBe("deny");
        }
    });

    it("routes a features string with any key outside the allowlist to the pane path", () => {
        const dangerous = [
            "popup,frame=no,transparent=yes,fullscreen=yes,alwaysOnTop=yes,closable=no,opacity=0.02",
            "width=480,alwaysOnTop=yes",
            "width=480,AlwaysOnTop=yes",
            "width=480,alwaysontop=1",
            "width=480,webContents=1",
            "kiosk",
        ];
        for (const features of dangerous) {
            expect(classifyWindowOpen({ ...Popup, features })).toBe("tab");
        }
    });

    it("routes an allowlisted-only features string to popup", () => {
        const allowlistedOnly =
            "popup,width=480,height=600,left=10,top=20,innerWidth=1,innerHeight=1,screenX=1,screenY=1," +
            "resizable=yes,scrollbars=yes,status=yes,toolbar=yes,menubar=yes,location=yes,directories=no,copyhistory=no";
        expect(classifyWindowOpen({ ...Popup, features: allowlistedOnly })).toBe("popup");
    });

    it("routes Google Identity Services' real sign-in popup to the popup path (captured from rtapp.log)", () => {
        const gsiFeatures =
            "toolbar=no,location=no,directories=no,status=no,menubar=no,scrollbars=no,resizable=no," +
            "copyhistory=no,width=590,height=830,top=305,left=985";
        const gsiUrl =
            "https://accounts.google.com/o/oauth2/v2/auth?gsiwebsdk=gis_attributes&client_id=example.apps.googleusercontent.com" +
            "&scope=openid%20profile%20email&redirect_uri=gis_transform&response_type=code&display=popup";
        expect(
            classifyWindowOpen({ url: gsiUrl, disposition: "new-window", features: gsiFeatures })
        ).toBe("popup");
    });

    it("routes a featureless new-window (shift-click) to the pane path", () => {
        expect(classifyWindowOpen({ ...Popup, features: "" })).toBe("tab");
        expect(classifyWindowOpen({ ...Popup, features: "   " })).toBe("tab");
        expect(classifyWindowOpen({ ...Popup, features: undefined })).toBe("tab");
    });
});

describe("computePopupBounds", () => {
    const parent = { x: 100, y: 100, width: 1200, height: 800 };
    const work = { x: 0, y: 0, width: 1920, height: 1080 };

    it("uses defaults centred over the parent when nothing is requested", () => {
        expect(computePopupBounds({}, parent, work)).toEqual({ x: 440, y: 180, width: 520, height: 640 });
    });

    it("honours requested size and position", () => {
        expect(computePopupBounds({ width: 400, height: 300, x: 50, y: 60 }, parent, work)).toEqual({
            x: 50,
            y: 60,
            width: 400,
            height: 300,
        });
    });

    it("clamps size and position into the work area", () => {
        const r = computePopupBounds({ width: 5000, height: 5000, x: 5000, y: -50 }, parent, work);
        expect(r.width).toBe(work.width);
        expect(r.height).toBe(work.height);
        expect(r.x + r.width).toBeLessThanOrEqual(work.x + work.width);
        expect(r.y).toBeGreaterThanOrEqual(work.y);
    });

    it("enforces a minimum size", () => {
        const r = computePopupBounds({ width: 1, height: 1 }, parent, work);
        expect(r.width).toBe(200);
        expect(r.height).toBe(150);
    });
});

import { session } from "electron";
import { EventEmitter } from "events";
import {
    buildPopupWindowOptions,
    handleGuestWindowOpen,
    hardenCreatedPopup,
    installGuestWindowOpenHandler,
    livePopupCount,
    isLivePopup,
    MaxLivePopupsPerOpener,
    popupWindowTitle,
} from "./emain-popup";
import { registerAppWebContents } from "./emain-websecurity";

const PreloadPath = "/app/preload/preload-webview.cjs";
const GuestSession = { id: "persist:webblock" };
let nextId = 5000;

function fakeWebContents(sessionObj: any = GuestSession): any {
    const wc = new EventEmitter() as any;
    wc.id = nextId++;
    wc.session = sessionObj;
    wc.destroyed = false;
    wc.isDestroyed = () => wc.destroyed;
    wc.windowOpenHandler = null;
    wc.setWindowOpenHandler = (h: any) => (wc.windowOpenHandler = h);
    wc.prefs = { sandbox: true, contextIsolation: true, nodeIntegration: false, nodeIntegrationInSubFrames: false };
    wc.getLastWebPreferences = () => wc.prefs;
    return wc;
}

function fakeWindow(sessionObj: any = GuestSession): any {
    const win = new EventEmitter() as any;
    win.webContents = fakeWebContents(sessionObj);
    win.destroyed = false;
    win.isDestroyed = () => win.destroyed;
    win.destroy = () => {
        win.destroyed = true;
        win.emit("closed");
    };
    win.menu = { items: [] };
    win.removeMenu = vi.fn(() => (win.menu = null));
    win.title = "";
    win.setTitle = (t: string) => (win.title = t);
    win.getBounds = () => ({ x: 100, y: 100, width: 1200, height: 800 });
    return win;
}

function ctxFor(root: any, parent: any = fakeWindow()) {
    const sent: any[] = [];
    return {
        ctx: {
            rootGuestId: root.id,
            sendToTab: (guestId: number, details: any) => sent.push({ guestId, details }),
            getParentWindow: () => parent,
            getWorkArea: () => ({ x: 0, y: 0, width: 1920, height: 1080 }),
            preloadPath: PreloadPath,
        },
        sent,
        parent,
    };
}

const PopupDetails: any = {
    url: "https://accounts.example.test/auth",
    frameName: "auth",
    features: "width=480,height=600",
    disposition: "new-window",
    referrer: { url: "", policy: "default" },
};

describe("buildPopupWindowOptions", () => {
    it("hardens webPreferences identically to a webview guest, pins the preload, and pins the opener's session", () => {
        const root = fakeWebContents();
        const { ctx, parent } = ctxFor(root);
        const opts = buildPopupWindowOptions(PopupDetails, root, ctx);
        expect(opts.webPreferences).toEqual({
            preload: PreloadPath,
            session: root.session,
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
        expect(opts.parent).toBe(parent);
        expect(opts.autoHideMenuBar).toBe(true);
        expect(opts).toMatchObject({ width: 480, height: 600 });
    });

    it("pins native window options a hostile features string could otherwise control", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const opts = buildPopupWindowOptions(PopupDetails, root, ctx);
        expect(opts).toMatchObject({
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
            minWidth: 200,
            minHeight: 150,
            maxWidth: 1920,
            maxHeight: 1080,
        });
        expect(opts.webPreferences).not.toHaveProperty("webContents");
    });

    it("omits a destroyed parent", () => {
        const root = fakeWebContents();
        const { ctx, parent } = ctxFor(root);
        parent.destroyed = true;
        expect(buildPopupWindowOptions(PopupDetails, root, ctx).parent).toBeUndefined();
    });
});

describe("handleGuestWindowOpen", () => {
    it("allows a popup with override options", () => {
        const root = fakeWebContents();
        const { ctx, sent } = ctxFor(root);
        const resp = handleGuestWindowOpen(root, PopupDetails, ctx);
        expect(resp.action).toBe("allow");
        expect(resp.overrideBrowserWindowOptions?.webPreferences?.sandbox).toBe(true);
        expect(resp.outlivesOpener).toBeUndefined();
        expect(sent).toEqual([]);
    });

    it("forwards tab routes to the renderer and denies", () => {
        const root = fakeWebContents();
        const { ctx, sent } = ctxFor(root);
        const details = { ...PopupDetails, disposition: "foreground-tab", features: "" };
        expect(handleGuestWindowOpen(root, details, ctx)).toEqual({ action: "deny" });
        expect(sent).toEqual([{ guestId: root.id, details }]);
    });

    it("denies non-web popup urls without forwarding", () => {
        const root = fakeWebContents();
        const { ctx, sent } = ctxFor(root);
        expect(handleGuestWindowOpen(root, { ...PopupDetails, url: "file:///x" }, ctx)).toEqual({ action: "deny" });
        expect(sent).toEqual([]);
    });

    it("denies when the opener is destroyed", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        root.destroyed = true;
        expect(handleGuestWindowOpen(root, PopupDetails, ctx)).toEqual({ action: "deny" });
    });
});

describe("hardenCreatedPopup", () => {
    const createdDetails: any = { url: PopupDetails.url, frameName: "auth", options: { webPreferences: { preload: PreloadPath } }, disposition: "new-window" };

    // Electron passes the URL both on the event object and as the deprecated positional arg.
    function emitNav(child: any, name: string, url: string) {
        const ev = { url, preventDefault: vi.fn() };
        child.webContents.emit(name, ev, url);
        return ev;
    }

    it("accepts a same-session popup, guards navigation, and tracks it", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const child = fakeWindow();
        expect(hardenCreatedPopup(child, root, createdDetails, ctx)).toBe(true);
        expect(child.destroyed).toBe(false);
        expect(livePopupCount(root.id)).toBe(1);
        expect(isLivePopup(child)).toBe(true);
        expect(emitNav(child, "will-navigate", "file:///etc/passwd").preventDefault).toHaveBeenCalled();
        expect(emitNav(child, "will-navigate", "https://ok.test/").preventDefault).not.toHaveBeenCalled();
        child.destroy();
        expect(livePopupCount(root.id)).toBe(0);
        expect(isLivePopup(child)).toBe(false);
    });

    it("blocks a server-side redirect to a disallowed scheme", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const child = fakeWindow();
        hardenCreatedPopup(child, root, createdDetails, ctx);
        for (const url of ["file:///etc/passwd", "chrome://gpu", "javascript:alert(1)", "data:text/html,x"]) {
            expect(emitNav(child, "will-redirect", url).preventDefault).toHaveBeenCalled();
        }
        expect(emitNav(child, "will-redirect", "https://accounts.example.test/cb").preventDefault).not.toHaveBeenCalled();
    });

    it("removes the inherited app menu so its accelerators cannot fire against the popup", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const child = fakeWindow();
        hardenCreatedPopup(child, root, createdDetails, ctx);
        expect(child.removeMenu).toHaveBeenCalled();
        expect(child.menu).toBeNull();
    });

    it("prefixes the window title with the popup's current host, whatever title the page sets", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const child = fakeWindow();
        hardenCreatedPopup(child, root, createdDetails, ctx);
        expect(child.title).toBe("accounts.example.test");
        const titleEv = { preventDefault: vi.fn() };
        child.emit("page-title-updated", titleEv, "Sign in - Google Accounts", true);
        expect(titleEv.preventDefault).toHaveBeenCalled();
        expect(child.title).toBe("accounts.example.test - Sign in - Google Accounts");
        child.webContents.emit("did-navigate", {}, "https://evil.test:8443/login", 200, "OK");
        expect(child.title).toBe("evil.test:8443");
        child.emit("page-title-updated", { preventDefault: vi.fn() }, "accounts.google.com", true);
        expect(child.title).toBe("evil.test:8443 - accounts.google.com");
    });

    it("destroys a popup that landed on a different session or the default session", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const other = fakeWindow({ id: "persist:other" });
        expect(hardenCreatedPopup(other, root, createdDetails, ctx)).toBe(false);
        expect(other.destroyed).toBe(true);
        // Same object on both sides so the "differs from opener" check passes and the
        // default-session check is the one that fires.
        const rootOnDefault = fakeWebContents(session.defaultSession);
        const onDefault = fakeWindow(session.defaultSession);
        expect(hardenCreatedPopup(onDefault, rootOnDefault, createdDetails, ctx)).toBe(false);
        expect(onDefault.destroyed).toBe(true);
    });

    it("destroys a popup with an app-owned webContents id", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const appOwned = fakeWindow();
        registerAppWebContents(appOwned.webContents);
        expect(hardenCreatedPopup(appOwned, root, createdDetails, ctx)).toBe(false);
        expect(appOwned.destroyed).toBe(true);
    });

    it("destroys an about:blank popup whose opener is not sandboxed, even though the child reports hardened prefs", () => {
        // Real Electron 41 (see .pi/evidence/2026-09-26-pr67-followup): the about:blank child
        // runs in the opener's process yet getLastWebPreferences reports the hardened override.
        const root = fakeWebContents();
        root.prefs = { ...root.prefs, sandbox: false };
        const { ctx } = ctxFor(root);
        const child = fakeWindow();
        const aboutBlank = { ...createdDetails, url: "about:blank" };
        expect(hardenCreatedPopup(child, root, aboutBlank, ctx)).toBe(false);
        expect(child.destroyed).toBe(true);
        expect(livePopupCount(root.id)).toBe(0);
    });

    it("destroys a popup whose own webPreferences are weakened or unreadable", () => {
        const weakenings = [{ contextIsolation: false }, { nodeIntegration: true }, { nodeIntegrationInSubFrames: true }, { sandbox: undefined }];
        for (const weaken of weakenings) {
            const root = fakeWebContents();
            const { ctx } = ctxFor(root);
            const child = fakeWindow();
            child.webContents.prefs = { ...child.webContents.prefs, ...weaken };
            expect(hardenCreatedPopup(child, root, createdDetails, ctx)).toBe(false);
            expect(child.destroyed).toBe(true);
        }
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const noAccessor = fakeWindow();
        delete noAccessor.webContents.getLastWebPreferences;
        expect(hardenCreatedPopup(noAccessor, root, createdDetails, ctx)).toBe(false);
        const nullPrefs = fakeWindow();
        nullPrefs.webContents.prefs = null;
        expect(hardenCreatedPopup(nullPrefs, root, createdDetails, ctx)).toBe(false);
        expect(livePopupCount(root.id)).toBe(0);
    });

    it("installs the router on the popup so nested opens route through the root", () => {
        const root = fakeWebContents();
        const { ctx, sent } = ctxFor(root);
        const child = fakeWindow();
        hardenCreatedPopup(child, root, createdDetails, ctx);
        expect(typeof child.webContents.windowOpenHandler).toBe("function");
        const tabDetails = { ...PopupDetails, disposition: "foreground-tab", features: "" };
        expect(child.webContents.windowOpenHandler(tabDetails)).toEqual({ action: "deny" });
        expect(sent).toEqual([{ guestId: root.id, details: tabDetails }]);
        expect(child.webContents.windowOpenHandler(PopupDetails).action).toBe("allow");
        const grandchild = fakeWindow();
        child.webContents.emit("did-create-window", grandchild, createdDetails);
        expect(livePopupCount(root.id)).toBe(2);
    });

    it("caps live popups per root opener", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        const wins: any[] = [];
        for (let i = 0; i < MaxLivePopupsPerOpener; i++) {
            const w = fakeWindow();
            hardenCreatedPopup(w, root, createdDetails, ctx);
            wins.push(w);
        }
        expect(handleGuestWindowOpen(root, PopupDetails, ctx)).toEqual({ action: "deny" });
        wins[0].destroy();
        expect(handleGuestWindowOpen(root, PopupDetails, ctx).action).toBe("allow");
    });
});

describe("popupWindowTitle", () => {
    it("falls back to the bare host, or the whole URL when there is no host", () => {
        expect(popupWindowTitle("https://a.test/x", "")).toBe("a.test");
        expect(popupWindowTitle("https://a.test/x", "   ")).toBe("a.test");
        expect(popupWindowTitle("about:blank", "Loading")).toBe("about:blank - Loading");
        expect(popupWindowTitle("not a url", "t")).toBe("not a url - t");
    });
});

describe("installGuestWindowOpenHandler", () => {
    it("wires setWindowOpenHandler and did-create-window on the guest and clears tracking on destroy", () => {
        const root = fakeWebContents();
        const { ctx } = ctxFor(root);
        installGuestWindowOpenHandler(root, ctx);
        expect(root.windowOpenHandler(PopupDetails).action).toBe("allow");
        const child = fakeWindow();
        root.emit("did-create-window", child, { url: PopupDetails.url, frameName: "auth", options: { webPreferences: { preload: PreloadPath } }, disposition: "new-window" });
        expect(livePopupCount(root.id)).toBe(1);
        root.emit("destroyed");
        expect(livePopupCount(root.id)).toBe(0);
    });
});
