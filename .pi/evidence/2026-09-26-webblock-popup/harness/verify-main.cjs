"use strict";
/**
 * Task 7 live verification harness (real Electron 41, headless).
 *
 * Extends the Task 1 probe pattern: loads the real emain/emain-popup.ts
 * (bundled with esbuild, electron external) into a standalone Electron main
 * process, wires it up the same way emain-tabview.ts does in the app
 * (installGuestWindowOpenHandler on a <webview> guest's did-attach-webview),
 * and installs the same global will-attach-webview refusal emain.ts installs
 * (emain.ts:161-171). Drives popup-fixture.html / popup-child.html (Task 1)
 * plus a few assertions the fixture's buttons can't express (nested-cap
 * race, webview-in-popup, clipboard, blocked navigation, guest-destroy
 * cascade) via executeJavaScript. Never touches remoteterm-daily or task dev.
 */

const path = require("path");
const http = require("http");
const fs = require("fs");

// Every console.log in this process -- ours and the bundled emain-popup /
// emain-websecurity code's -- shares this stdout, so capturing it here is
// sufficient to check for main-process log lines like "[popup] blocked
// navigation" (assertion k) or "[popup] cap reached" (assertion h).
const mainLogLines = [];
const origConsoleLog = console.log.bind(console);
console.log = (...args) => {
    mainLogLines.push(args.map((a) => (typeof a === "string" ? a : JSON.stringify(a))).join(" "));
    origConsoleLog(...args);
};

const { app, BrowserWindow, session, screen, webContents } = require("electron");

const SCRATCH = "/tmp/claude-1000/-media-owner-Workspace-remoteterm/65b2b635-6ca7-4df9-a139-9105022c901d/scratchpad";
const SCRATCH_USERDATA = path.join(SCRATCH, "electron-verify-userdata");
app.setPath("userData", SCRATCH_USERDATA);

const EVIDENCE_DIR = path.resolve(__dirname, "..");
const PRELOAD_PATH = path.join(__dirname, "verify-preload.cjs");

const popupMod = require(path.join(SCRATCH, "emain-popup.bundle.cjs"));
const secMod = require(path.join(SCRATCH, "emain-websecurity.bundle.cjs"));

const results = [];
function record(id, assertion, expected, observed, pass) {
    results.push({ id, assertion, expected, observed, pass });
    console.log(`[verify-result] ${id} ${pass ? "PASS" : "FAIL"} | ${assertion} | expected=${JSON.stringify(expected)} observed=${JSON.stringify(observed)}`);
}

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitForLoad(wc) {
    if (wc.isLoading()) {
        await new Promise((resolve) => wc.once("did-finish-load", resolve));
    }
    await wait(80);
}

function allWindowIds() {
    return new Set(BrowserWindow.getAllWindows().map((w) => w.id));
}

async function diffNewWindows(action) {
    const before = allWindowIds();
    await action();
    await wait(500);
    return BrowserWindow.getAllWindows().filter((w) => !before.has(w.id));
}

// Mirrors emain-tabview.ts's window.open() -> child BrowserWindow wiring.
async function openPopupViaScript(wc, name, features) {
    const newWins = await diffNewWindows(() =>
        wc.executeJavaScript(
            // The trailing `undefined;` matters: without it the script's completion
            // value is the WindowProxy window.open() returns, and Electron's
            // executeJavaScript cannot structured-clone a WindowProxy back across
            // the IPC boundary ("An object could not be cloned").
            `window.open(new URL("popup-child.html", location.href).href, ${JSON.stringify(name)}, ${JSON.stringify(features)}); undefined;`,
            true
        )
    );
    return newWins[0] ?? null;
}

async function destroyAll(wins) {
    for (const w of wins) {
        if (w && !w.isDestroyed()) {
            w.destroy();
        }
    }
    await wait(200);
}

const MIME = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8" };
function startStaticServer(rootDir) {
    return new Promise((resolve, reject) => {
        const server = http.createServer((req, res) => {
            const urlPath = decodeURIComponent((req.url || "/").split("?")[0]);
            const rel = urlPath === "/" ? "/popup-fixture.html" : urlPath;
            const filePath = path.join(rootDir, rel);
            if (!filePath.startsWith(rootDir)) {
                res.writeHead(403);
                res.end();
                return;
            }
            fs.readFile(filePath, (err, data) => {
                if (err) {
                    res.writeHead(404);
                    res.end("not found: " + rel);
                    return;
                }
                const type = MIME[path.extname(filePath)] || "application/octet-stream";
                res.writeHead(200, { "Content-Type": type });
                res.end(data);
            });
        });
        server.on("error", reject);
        server.listen(0, "127.0.0.1", () => resolve(server));
    });
}

async function main() {
    await app.whenReady();

    // Mirrors emain.ts:161-171 exactly: global refusal of any will-attach-webview
    // on a webContents that isn't app-owned, running the same hardenWebviewAttach.
    app.on("web-contents-created", (_event, contents) => {
        contents.on("will-attach-webview", (event, webPreferences, params) => {
            const ok = secMod.isAppWebContentsId(contents.id) && secMod.hardenWebviewAttach(webPreferences, params, PRELOAD_PATH);
            console.log(`[verify] will-attach-webview on contents=${contents.id} appOwned=${secMod.isAppWebContentsId(contents.id)} allowed=${ok}`);
            if (!ok) {
                event.preventDefault();
            }
        });
    });

    // Mirrors emain.ts's `electronApp.on("session-created", installPermissionHandlers)`.
    // Without this, the persist:webblock session never gets the gated permission
    // handler, and Chromium's own default (not our hardening) decides clipboard/etc.
    app.on("session-created", secMod.installPermissionHandlers);

    const server = await startStaticServer(EVIDENCE_DIR);
    const port = server.address().port;
    const baseUrl = `http://127.0.0.1:${port}`;
    console.log("[verify] serving " + baseUrl + " from " + EVIDENCE_DIR);

    const host = new BrowserWindow({
        show: false,
        webPreferences: { webviewTag: true, nodeIntegration: false, contextIsolation: true },
    });
    secMod.registerAppWebContents(host.webContents);
    console.log("[verify] host webContents id =", host.webContents.id, "registered app-owned");

    const guestPromise = new Promise((resolve) => {
        host.webContents.on("did-attach-webview", (_event, guestWc) => {
            console.log("[verify] did-attach-webview on host, guest id =", guestWc.id);
            resolve(guestWc);
        });
    });

    const hostHtml =
        `<!doctype html><meta charset="utf-8"><body style="margin:0">` +
        `<webview id="guest" src="${baseUrl}/popup-fixture.html" partition="persist:webblock" ` +
        `allowpopups style="width:1200px;height:900px;display:flex;"></webview>` +
        `</body>`;
    await host.loadURL("data:text/html," + encodeURIComponent(hostHtml));

    const guest = await Promise.race([
        guestPromise,
        wait(15000).then(() => {
            throw new Error("timed out waiting for did-attach-webview on host");
        }),
    ]);
    await waitForLoad(guest);

    const sentToTab = [];
    const rootGuestId = guest.id;
    const ctx = {
        rootGuestId,
        sendToTab: (guestId, details) => {
            sentToTab.push({ guestId, details });
            console.log("[verify] sendToTab", guestId, details.url, details.disposition);
        },
        getParentWindow: () => (host.isDestroyed() ? null : host),
        getWorkArea: (parent) => {
            const display = parent == null ? screen.getPrimaryDisplay() : screen.getDisplayMatching(parent.getBounds());
            return display.workArea;
        },
        preloadPath: PRELOAD_PATH,
    };
    popupMod.installGuestWindowOpenHandler(guest, ctx);
    console.log("[verify] installGuestWindowOpenHandler installed on guest", rootGuestId);

    async function click(wc, selector) {
        await wc.executeJavaScript(`document.querySelector(${JSON.stringify(selector)}).click();`, true);
    }
    async function readLog(wc) {
        return wc.executeJavaScript(`document.getElementById("log") ? document.getElementById("log").textContent : ""`, true);
    }
    async function readOpenerText(wc) {
        return wc.executeJavaScript(`document.getElementById("opener") ? document.getElementById("opener").textContent : ""`, true);
    }

    let savedPopupIdForM = null;

    // --- a: "feat" -> exactly one child BrowserWindow, opener SET, WindowProxy returned ---
    let childA = null;
    try {
        const newWins = await diffNewWindows(() => click(guest, "#feat"));
        const count = newWins.length;
        childA = newWins[0] ?? null;
        record("a1", 'window.open with features creates exactly one BrowserWindow', 1, count, count === 1);
        if (childA) {
            savedPopupIdForM = childA.webContents.id;
            await waitForLoad(childA.webContents);
            const openerText = await readOpenerText(childA.webContents);
            record("a2", "child page reports window.opener is SET", "window.opener is SET", openerText, openerText === "window.opener is SET");
        } else {
            record("a2", "child page reports window.opener is SET", "window.opener is SET", "(no child window)", false);
        }
        const fixtureLog = await readLog(guest);
        const gotProxy = fixtureLog.includes("open(features) returned WindowProxy");
        record("a3", "opener's window.open() call returned a WindowProxy (non-null)", true, fixtureLog.trim(), gotProxy);
    } catch (e) {
        record("a", "feat popup creation", "success", "threw: " + e.message, false);
    }

    // --- b: child's postMessage reaches opener's message listener ---
    try {
        if (childA && !childA.isDestroyed()) {
            await click(childA.webContents, "#post");
            await wait(300);
            const fixtureLog = await readLog(guest);
            const got = fixtureLog.includes('message from popup: {"ok":true,"from":"child"}');
            record("b", "child postMessage reaches opener's message listener", true, fixtureLog.trim(), got);
        } else {
            record("b", "child postMessage reaches opener's message listener", true, "(no live child popup)", false);
        }
    } catch (e) {
        record("b", "child postMessage reaches opener's message listener", true, "threw: " + e.message, false);
    }

    // --- c: child's window.close() closes the window; opener-side popup.closed becomes true ---
    try {
        if (childA && !childA.isDestroyed()) {
            const closedPromise = new Promise((resolve) => childA.once("closed", () => resolve(true)));
            await click(childA.webContents, "#close");
            const closed = await Promise.race([closedPromise, wait(2000).then(() => false)]);
            record("c1", "window.close() from popup closes the BrowserWindow", true, closed, closed === true);
            await wait(600); // fixture polls popup.closed every 250ms
            const fixtureLog = await readLog(guest);
            const observedClosed = fixtureLog.includes("window.closed observed true");
            record("c2", "opener-side popup.closed becomes true", true, fixtureLog.trim(), observedClosed);
        } else {
            record("c1", "window.close() from popup closes the BrowserWindow", true, "(no live child popup)", false);
            record("c2", "opener-side popup.closed becomes true", true, "(no live child popup)", false);
        }
    } catch (e) {
        record("c", "window.close()/popup.closed", true, "threw: " + e.message, false);
    }

    // --- d: plain / noopener+geometry / target=_blank each create no window, one sendToTab call ---
    async function tabRouteCase(id, label, action) {
        const before = sentToTab.length;
        const newWins = await diffNewWindows(action);
        const after = sentToTab.length;
        record(
            `${id}`,
            `${label}: no window created, exactly one sendToTab call`,
            "0 windows, +1 sendToTab",
            `${newWins.length} windows, +${after - before} sendToTab`,
            newWins.length === 0 && after - before === 1
        );
    }
    try {
        await tabRouteCase("d1", '"open plain"', () => click(guest, "#plain"));
        await tabRouteCase("d2", '"open with noopener"', () => click(guest, "#noopener"));
        await tabRouteCase("d3", "target=_blank link", () => click(guest, 'a[target="_blank"]'));
    } catch (e) {
        record("d", "tab-routed window.open cases", true, "threw: " + e.message, false);
    }

    // --- e: about:blank then navigate; opener still SET ---
    let childE = null;
    try {
        const newWins = await diffNewWindows(() => click(guest, "#blank"));
        childE = newWins[0] ?? null;
        record("e1", '"open about:blank then navigate" creates exactly one BrowserWindow', 1, newWins.length, newWins.length === 1);
        if (childE) {
            // Indirect proof the preload actually ran: getLastWebPreferences() (used in
            // assertion g) does not expose a "preload" field at all in this Electron
            // version, so a console message from the preload script is the only direct
            // evidence available that ctx.preloadPath was actually applied.
            childE.webContents.on("console-message", (event, level, message) => {
                if (message.includes("[verify-preload] loaded")) {
                    childE._sawPreloadConsoleMessage = true;
                }
            });
            await wait(600); // fixture navigates after 300ms
            await waitForLoad(childE.webContents);
            const openerText = await readOpenerText(childE.webContents);
            const url = childE.webContents.getURL();
            record("e2", "popup navigates to child page with window.opener still SET", "window.opener is SET, url=popup-child.html", `url=${url}, openerText=${openerText}`, openerText === "window.opener is SET" && url.includes("popup-child.html"));
        } else {
            record("e2", "popup navigates to child page with window.opener still SET", "window.opener is SET", "(no child window)", false);
        }
    } catch (e) {
        record("e", "about:blank then navigate", true, "threw: " + e.message, false);
    }

    // --- f: session identity ---
    try {
        if (childE && !childE.isDestroyed()) {
            const sameAsGuest = childE.webContents.session === guest.session;
            const notDefault = childE.webContents.session !== session.defaultSession;
            const storagePath = childE.webContents.session.storagePath || "";
            const endsRight = storagePath.endsWith("Partitions/webblock") || storagePath.includes("Partitions" + path.sep + "webblock");
            record("f1", "popup webContents.session === guest.session", true, sameAsGuest, sameAsGuest === true);
            record("f2", "popup webContents.session !== session.defaultSession", true, notDefault, notDefault === true);
            record("f3", "session.storagePath ends in Partitions/webblock", true, storagePath, endsRight);
        } else {
            record("f", "popup session identity", true, "(no live popup from assertion e)", false);
        }
    } catch (e) {
        record("f", "popup session identity", true, "threw: " + e.message, false);
    }

    // --- g: effective webPreferences + page-context require/process/module undefined ---
    try {
        if (childE && !childE.isDestroyed()) {
            const prefs = childE.webContents.getLastWebPreferences();
            console.log("[verify] raw getLastWebPreferences() keys:", Object.keys(prefs).sort().join(","));
            console.log("[verify] raw getLastWebPreferences():", JSON.stringify(prefs));
            const expected = {
                nodeIntegration: false,
                nodeIntegrationInSubFrames: false,
                contextIsolation: true,
                sandbox: true,
                webSecurity: true,
                webviewTag: false,
            };
            const observed = {
                nodeIntegration: prefs.nodeIntegration,
                nodeIntegrationInSubFrames: prefs.nodeIntegrationInSubFrames,
                contextIsolation: prefs.contextIsolation,
                sandbox: prefs.sandbox,
                webSecurity: prefs.webSecurity,
                webviewTag: prefs.webviewTag,
            };
            const allMatch = Object.keys(expected).every((k) => expected[k] === observed[k]);
            record("g1", "popup effective webPreferences (excl. preload) match hardened set", expected, observed, allMatch);

            const hasPreloadKey = Object.prototype.hasOwnProperty.call(prefs, "preload");
            const preloadsField = prefs.preloads; // Electron 41 may report an array here instead.
            const sawPreloadConsoleMessage = childE._sawPreloadConsoleMessage === true;
            record(
                "g1b",
                "getLastWebPreferences() exposes the applied preload path directly",
                PRELOAD_PATH,
                `hasOwnProperty('preload')=${hasPreloadKey}, prefs.preload=${JSON.stringify(prefs.preload)}, prefs.preloads=${JSON.stringify(preloadsField)}`,
                prefs.preload === PRELOAD_PATH || (Array.isArray(preloadsField) && preloadsField.includes(PRELOAD_PATH))
            );
            record(
                "g1c",
                "indirect proof the preload actually ran: its console.log reached the webContents console-message event",
                true,
                sawPreloadConsoleMessage,
                sawPreloadConsoleMessage
            );

            const typesResult = await childE.webContents.executeJavaScript(
                `[typeof require, typeof process, typeof module]`,
                true
            );
            const wantTypes = ["undefined", "undefined", "undefined"];
            const typesMatch = JSON.stringify(typesResult) === JSON.stringify(wantTypes);
            record("g2", "popup page context: typeof require/process/module all undefined", wantTypes, typesResult, typesMatch);
        } else {
            record("g", "popup webPreferences / page globals", true, "(no live popup from assertion e)", false);
        }
    } catch (e) {
        record("g", "popup webPreferences / page globals", true, "threw: " + e.message, false);
    } finally {
        if (childE && !childE.isDestroyed()) {
            childE.destroy();
            await wait(200);
        }
    }

    // --- h: nested cap + synchronous race check ---
    try {
        record("h0", "livePopupCount(root) is 0 before nested-cap test", 0, popupMod.livePopupCount(rootGuestId), popupMod.livePopupCount(rootGuestId) === 0);

        const childH = await openPopupViaScript(guest, "hRoot", "width=320,height=320");
        if (childH) {
            await waitForLoad(childH.webContents);
        }
        const nestedWins = [];
        for (let i = 0; i < 5; i++) {
            if (!childH || childH.isDestroyed()) break;
            const w = await openPopupViaScript(childH.webContents, `hNested${i}`, "width=320,height=320");
            nestedWins.push(w);
            await wait(150);
        }
        const successfulNested = nestedWins.filter(Boolean).length;
        const liveNow = popupMod.livePopupCount(rootGuestId);
        const totalLive = 1 + successfulNested; // childH + successful nested, as long as none were closed
        record(
            "h1",
            "5 nested feature-opens from the child leave exactly 4 live popups total (cap = MaxLivePopupsPerOpener)",
            4,
            `childH=${childH ? "created" : "null"}, successfulNested=${successfulNested}, totalLiveWindowsTracked=${totalLive}, livePopupCount(root)=${liveNow}`,
            totalLive === 4 && liveNow === 4
        );
        const capLogged = mainLogLines.some((l) => l.includes("[popup] cap reached"));
        record("h2", "main-process log shows [popup] cap reached at least once", true, capLogged, capLogged);

        await destroyAll([childH, ...nestedWins]);
        record("h3", "livePopupCount(root) returns to 0 after destroying the h1 popups", 0, popupMod.livePopupCount(rootGuestId), popupMod.livePopupCount(rootGuestId) === 0);

        // Race check: 6 synchronous window.open() feature calls in ONE executeJavaScript.
        const before = allWindowIds();
        const raceResultsRaw = await guest.executeJavaScript(
            `(() => {
                const out = [];
                for (let i = 0; i < 6; i++) {
                    const w = window.open(new URL("popup-child.html", location.href).href, "race" + i, "width=300,height=300");
                    out.push(!!w);
                }
                return out;
            })();`,
            true
        );
        await wait(600);
        const raceWins = BrowserWindow.getAllWindows().filter((w) => !before.has(w.id));
        const truthyCount = raceResultsRaw.filter(Boolean).length;
        record(
            "h4",
            "6 synchronous window.open() feature calls from 0 popups create at most 4 (race check)",
            "<=4",
            `renderer-reported truthy=${truthyCount}, actual BrowserWindows created=${raceWins.length}, livePopupCount(root)=${popupMod.livePopupCount(rootGuestId)}`,
            raceWins.length <= 4 && popupMod.livePopupCount(rootGuestId) === raceWins.length
        );
        await destroyAll(raceWins);
    } catch (e) {
        record("h", "nested cap / race check", true, "threw: " + (e && e.stack || e), false);
    }

    // --- i: <webview> inserted into a popup's DOM attaches no guest ---
    try {
        const childI = await openPopupViaScript(guest, "iTest", "width=320,height=320");
        if (childI) {
            await waitForLoad(childI.webContents);
            let attachFired = false;
            let willAttachFired = false;
            childI.webContents.on("did-attach-webview", () => (attachFired = true));
            childI.webContents.on("will-attach-webview", () => (willAttachFired = true));
            await childI.webContents.executeJavaScript(
                `document.body.insertAdjacentHTML("beforeend", '<webview src="https://example.com"></webview>');`,
                true
            );
            await wait(800);
            record(
                "i",
                "inserting <webview> into a popup's DOM attaches no guest (no did-attach-webview)",
                false,
                `did-attach-webview fired=${attachFired}, will-attach-webview fired=${willAttachFired}`,
                attachFired === false
            );
            childI.destroy();
        } else {
            record("i", "inserting <webview> into a popup's DOM attaches no guest", false, "(could not create popup for this test)", false);
        }
    } catch (e) {
        record("i", "webview-in-popup", false, "threw: " + e.message, false);
    }

    // --- j: navigator.clipboard.readText() rejects in the popup ---
    try {
        const childJ = await openPopupViaScript(guest, "jTest", "width=320,height=320");
        if (childJ) {
            await waitForLoad(childJ.webContents);
            const outcome = await childJ.webContents.executeJavaScript(
                `navigator.clipboard.readText().then(() => "resolved").catch((e) => "rejected: " + e.message)`,
                true
            );
            const rejected = typeof outcome === "string" && outcome.startsWith("rejected");
            record("j", "navigator.clipboard.readText() rejects in the popup", "rejected: ...", outcome, rejected);
            childJ.destroy();
        } else {
            record("j", "navigator.clipboard.readText() rejects in the popup", "rejected: ...", "(could not create popup for this test)", false);
        }
    } catch (e) {
        record("j", "clipboard read in popup", "rejected", "threw: " + e.message, false);
    }

    // --- k: popup location.href = file:/// is blocked ---
    try {
        const childK = await openPopupViaScript(guest, "kTest", "width=320,height=320");
        if (childK) {
            await waitForLoad(childK.webContents);
            // Raw instrumentation to tell apart "our will-navigate handler fired and
            // blocked it" from "Chromium refused the http(s)->file:// navigation on its
            // own, before our handler ever saw it" (attached in addition to, not instead
            // of, hardenCreatedPopup's own listener, which was installed at popup-creation
            // time inside the bundle).
            let willNavigateFired = false;
            let willNavigateUrl = null;
            childK.webContents.on("will-navigate", (_event, url) => {
                willNavigateFired = true;
                willNavigateUrl = url;
            });
            const urlBefore = childK.webContents.getURL();
            await childK.webContents.executeJavaScript(`location.href = "file:///etc/hostname"; "issued";`, true);
            await wait(600);
            const urlAfter = childK.webContents.getURL();
            const blockedLogged = mainLogLines.some((l) => l.includes("[popup] blocked navigation"));
            record("k1", "URL unchanged after attempted file:// navigation", urlBefore, urlAfter, urlAfter === urlBefore);
            record(
                "k2",
                'will-navigate fires for the http(s)->file:// attempt, and our handler logs "[popup] blocked navigation"',
                "will-navigate fires with the file:// url, and the log line appears",
                `will-navigate fired=${willNavigateFired}, url seen=${willNavigateUrl}, "[popup] blocked navigation" logged=${blockedLogged}`,
                blockedLogged
            );
            childK.destroy();
        } else {
            record("k1", "URL unchanged after attempted file:// navigation", true, "(could not create popup for this test)", false);
            record("k2", 'will-navigate fires and logs "[popup] blocked navigation"', true, "(could not create popup for this test)", false);
        }
    } catch (e) {
        record("k", "blocked file:// navigation", true, "threw: " + e.message, false);
    }

    // --- n: dangerous feature strings (H-1) create no window, exactly one sendToTab call ---
    async function dangerousFeatureCase(id, features) {
        const before = sentToTab.length;
        const newWins = await diffNewWindows(() =>
            guest.executeJavaScript(
                `window.open(new URL("popup-child.html", location.href).href, "x", ${JSON.stringify(features)}); undefined;`,
                true
            )
        );
        const after = sentToTab.length;
        record(
            id,
            `dangerous features "${features}" create no BrowserWindow and make exactly one sendToTab call`,
            "0 windows, +1 sendToTab",
            `${newWins.length} windows, +${after - before} sendToTab`,
            newWins.length === 0 && after - before === 1
        );
    }
    try {
        await dangerousFeatureCase(
            "n1",
            "popup,frame=no,transparent=yes,alwaysOnTop=yes,fullscreen=yes,skipTaskbar=yes,closable=no,show=no"
        );
        await dangerousFeatureCase("n2", "popup,alwaysontop=yes");
        await dangerousFeatureCase("n3", "popup,webContents=1");
    } catch (e) {
        record("n", "dangerous feature strings routed to pane", true, "threw: " + e.message, false);
    }

    // --- o: direct pin check on buildPopupWindowOptions, then a real allowed popup's native state ---
    try {
        const oDetails = {
            url: `${baseUrl}/popup-child.html`,
            frameName: "oDirect",
            features: "width=320,height=320",
            disposition: "new-window",
        };
        const oOpts = popupMod.buildPopupWindowOptions(oDetails, guest, ctx);
        const expectedPins = {
            frame: true,
            transparent: false,
            fullscreen: false,
            alwaysOnTop: false,
            skipTaskbar: false,
            closable: true,
            show: true,
            kiosk: false,
            modal: false,
        };
        const pinsMatch = Object.keys(expectedPins).every((k) => oOpts[k] === expectedPins[k]);
        record("o1", "buildPopupWindowOptions pins native window options for an allowed popup", expectedPins, oOpts, pinsMatch);

        const childO = await openPopupViaScript(guest, "oReal", "width=320,height=320");
        if (childO) {
            await waitForLoad(childO.webContents);
            const state = {
                isAlwaysOnTop: childO.isAlwaysOnTop(),
                isClosable: childO.isClosable(),
                isVisible: childO.isVisible(),
                isFullScreen: childO.isFullScreen(),
            };
            const expected = { isAlwaysOnTop: false, isClosable: true, isVisible: true, isFullScreen: false };
            const ok = Object.keys(expected).every((k) => state[k] === expected[k]);
            record("o2", "a real allowed popup has the pinned native window state", expected, state, ok);
            childO.destroy();
            await wait(200);
        } else {
            record("o2", "a real allowed popup has the pinned native window state", true, "(could not create popup)", false);
        }
    } catch (e) {
        record("o", "pin checks on buildPopupWindowOptions / real popup", true, "threw: " + (e && e.stack || e), false);
    }

    // --- p: shift-click a plain <a href> (M-1) routes to the pane, no window, one sendToTab ---
    async function shiftClick(wc, selector) {
        const rect = await wc.executeJavaScript(
            `(() => { const r = document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect(); return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) }; })();`,
            true
        );
        wc.sendInputEvent({ type: "mouseDown", x: rect.x, y: rect.y, button: "left", clickCount: 1, modifiers: ["shift"] });
        wc.sendInputEvent({ type: "mouseUp", x: rect.x, y: rect.y, button: "left", clickCount: 1, modifiers: ["shift"] });
    }
    try {
        const beforeWins = allWindowIds();
        const beforeSent = sentToTab.length;
        const beforeDefaultIds = new Set(
            webContents.getAllWebContents().filter((wc) => wc.session === session.defaultSession).map((wc) => wc.id)
        );
        await shiftClick(guest, "#plainlink");
        await wait(600);
        const afterWins = BrowserWindow.getAllWindows().filter((w) => !beforeWins.has(w.id));
        const afterSent = sentToTab.length;
        const afterDefaultIds = webContents.getAllWebContents().filter((wc) => wc.session === session.defaultSession).map((wc) => wc.id);
        const newDefaultIds = afterDefaultIds.filter((id) => !beforeDefaultIds.has(id));
        const sawEntry = sentToTab[beforeSent];
        record(
            "p1",
            "shift-clicking a plain <a href> creates no BrowserWindow and makes exactly one sendToTab call",
            "0 windows, +1 sendToTab",
            `${afterWins.length} windows, +${afterSent - beforeSent} sendToTab, disposition=${sawEntry ? sawEntry.details.disposition : null}, features=${JSON.stringify(sawEntry ? sawEntry.details.features : null)}`,
            afterWins.length === 0 && afterSent - beforeSent === 1
        );
        record(
            "p2",
            "no new webContents appear on session.defaultSession as a result of the shift-click",
            [],
            newDefaultIds,
            newDefaultIds.length === 0
        );
    } catch (e) {
        record("p", "shift-click routing", true, "threw: " + (e && e.stack || e), false);
    }

    // --- q: preload effectiveness via contextBridge, replacing the failed g1b/g1c ---
    try {
        const childQ = await openPopupViaScript(guest, "qTest", "width=320,height=320");
        if (childQ) {
            await waitForLoad(childQ.webContents);
            const val = await childQ.webContents.executeJavaScript(`window.__verifyPreload`, true);
            record("q", "popup page sees window.__verifyPreload set by the applied preload via contextBridge", "ok", val, val === "ok");
            childQ.destroy();
            await wait(200);
        } else {
            record("q", "preload effectiveness via contextBridge", "ok", "(could not create popup)", false);
        }
    } catch (e) {
        record("q", "preload effectiveness via contextBridge", "ok", "threw: " + e.message, false);
    }

    // --- r: will-navigate guard wiring, and behaviour for schemes Chromium may hand to the browser ---
    try {
        const childR = await openPopupViaScript(guest, "rTest", "width=320,height=320");
        if (childR) {
            await waitForLoad(childR.webContents);
            const listenerCount = childR.webContents.listenerCount("will-navigate");
            record("r1", "popup webContents has a will-navigate listener installed", ">=1", listenerCount, listenerCount >= 1);

            async function tryScheme(id, url) {
                let fired = false;
                let seenUrl = null;
                const onWillNavigate = (_e, u) => {
                    fired = true;
                    seenUrl = u;
                };
                childR.webContents.on("will-navigate", onWillNavigate);
                const beforeLog = mainLogLines.length;
                await childR.webContents
                    .executeJavaScript(`location.href = ${JSON.stringify(url)}; "issued";`, true)
                    .catch(() => {});
                await wait(500);
                childR.webContents.removeListener("will-navigate", onWillNavigate);
                const blockedLogged = mainLogLines.slice(beforeLog).some((l) => l.includes("[popup] blocked navigation"));
                record(
                    id,
                    `renderer navigation to ${url}: will-navigate fires and the guard blocks it`,
                    "will-navigate fires and is blocked",
                    `will-navigate fired=${fired}, url seen=${seenUrl}, guard blocked=${blockedLogged}`,
                    fired && blockedLogged
                );
            }
            await tryScheme("r2", "ssh://example.invalid/");
            await tryScheme("r3", "mailto:x@example.invalid");
            childR.destroy();
            await wait(200);
        } else {
            record("r", "will-navigate guard wiring", true, "(could not create popup)", false);
        }
    } catch (e) {
        record("r", "will-navigate guard wiring", true, "threw: " + (e && e.stack || e), false);
    }

    // --- s: reviewer's L-2 -- nested opens from one executeJavaScript all get the router, stay within cap ---
    try {
        await wait(200);
        const beforeWins = allWindowIds();
        await guest.executeJavaScript(
            `(() => {
                const w = window.open("about:blank", "", "popup,width=300");
                w.open(location.href, "", "popup,width=300");
                return true;
            })();`,
            true
        );
        await wait(700);
        const newWins = BrowserWindow.getAllWindows().filter((w) => !beforeWins.has(w.id));
        const routerInstalled = newWins.every((w) => w.webContents.listenerCount("did-create-window") >= 1);
        record(
            "s1",
            "every BrowserWindow created by the nested window.open chain has the did-create-window router installed",
            true,
            newWins.map((w) => ({ id: w.id, didCreateWindowListeners: w.webContents.listenerCount("did-create-window") })),
            routerInstalled
        );
        const liveCount = popupMod.livePopupCount(rootGuestId);
        record(
            "s2",
            "live popups created by the nested chain stay within MaxLivePopupsPerOpener",
            `<= ${popupMod.MaxLivePopupsPerOpener}`,
            liveCount,
            liveCount <= popupMod.MaxLivePopupsPerOpener
        );

        const windowWcIds = new Set(BrowserWindow.getAllWindows().map((w) => w.webContents.id));
        const webblockOrphans = webContents
            .getAllWebContents()
            .filter((wc) => wc.session === guest.session && wc.id !== guest.id && !windowWcIds.has(wc.id));
        record(
            "s3",
            "no webblock-session webContents exists without an owning BrowserWindow (besides the guest webview)",
            0,
            webblockOrphans.map((wc) => wc.id),
            webblockOrphans.length === 0
        );

        await destroyAll(newWins);
    } catch (e) {
        record("s", "nested-open router / cap / orphan check", true, "threw: " + (e && e.stack || e), false);
    }

    // --- l: destroying the guest closes all its popups; livePopupCount returns to 0 ---
    try {
        await wait(200);
        record("l0", "livePopupCount(root) is 0 before guest-destroy test", 0, popupMod.livePopupCount(rootGuestId), popupMod.livePopupCount(rootGuestId) === 0);
        const childL1 = await openPopupViaScript(guest, "lTest1", "width=320,height=320");
        const childL2 = await openPopupViaScript(guest, "lTest2", "width=320,height=320");
        const liveBefore = popupMod.livePopupCount(rootGuestId);
        const stillOpenBefore = [childL1, childL2].filter((w) => w && !w.isDestroyed()).length;
        record("l1", "2 popups are live before destroying the guest", 2, `livePopupCount=${liveBefore}, actuallyOpen=${stillOpenBefore}`, liveBefore === 2 && stillOpenBefore === 2);

        const guestDestroyed = new Promise((resolve) => guest.once("destroyed", () => resolve(true)));
        await host.webContents.executeJavaScript(`document.getElementById("guest").remove();`, true);
        await Promise.race([guestDestroyed, wait(2000)]);
        await wait(400);

        const liveAfter = popupMod.livePopupCount(rootGuestId);
        record("l2", "livePopupCount(root) returns to 0 after the guest is destroyed", 0, liveAfter, liveAfter === 0);

        const stillOpenAfter = [childL1, childL2].filter((w) => w && !w.isDestroyed());
        record(
            "l3",
            "both popup BrowserWindows are actually closed/destroyed after the guest is destroyed",
            "0 still open",
            `${stillOpenAfter.length} still open (ids: ${stillOpenAfter.map((w) => w.id).join(",")})`,
            stillOpenAfter.length === 0
        );
        await destroyAll([childL1, childL2]);
    } catch (e) {
        record("l", "guest-destroy cascade", true, "threw: " + (e && e.stack || e), false);
    }

    // --- m: isAppWebContentsId(popup.webContents.id) is false ---
    try {
        const idToCheck = savedPopupIdForM;
        if (idToCheck != null) {
            const isApp = secMod.isAppWebContentsId(idToCheck);
            record("m", "isAppWebContentsId(popup.webContents.id) is false", false, isApp, isApp === false);
        } else {
            record("m", "isAppWebContentsId(popup.webContents.id) is false", false, "(no popup id captured)", false);
        }
    } catch (e) {
        record("m", "isAppWebContentsId(popup.webContents.id) is false", false, "threw: " + e.message, false);
    }

    console.log("[verify] --- done, quitting ---");
    console.log("[verify-results-json] " + JSON.stringify(results));
    server.close();
    if (!host.isDestroyed()) {
        host.destroy();
    }
    app.quit();
}

main().catch((err) => {
    console.error("[verify] fatal", err);
    console.log("[verify-results-json] " + JSON.stringify(results));
    process.exitCode = 1;
    app.quit();
});
