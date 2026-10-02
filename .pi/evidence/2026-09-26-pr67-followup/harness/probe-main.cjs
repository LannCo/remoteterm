"use strict";
/**
 * PR #67 follow-up probe (real Electron 41, headless under Xvfb).
 *
 * Settles two findings the PR #67 security-auditor left unverified:
 *
 *  SEC-2: what an about:blank popup's effective webPreferences are when the
 *         opener is hardened vs deliberately weakened (sandbox: false), and
 *         whether hardenCreatedPopup lets the weakened case live.
 *  SEC-3: whether the 4-live-popup cap can be overrun when several renderer
 *         processes sharing one root guest call window.open() at the same
 *         instant (the allow decision runs in -will-add-new-contents, the
 *         count increments later in did-create-window).
 *
 * Loads the real emain/emain-popup.ts + emain-websecurity.ts bundled with
 * esbuild (electron external) from $PR67_BUNDLE_DIR. Never touches the app,
 * its userData, or the live display; run it via xvfb-run with DISPLAY unset.
 */

const path = require("path");
const http = require("http");
const fs = require("fs");

// Written with fs.writeSync: app.exit() at the end otherwise drops buffered stdout.
const mainLogLines = [];
console.log = (...args) => {
    const line = args.map((a) => (typeof a === "string" ? a : JSON.stringify(a))).join(" ");
    mainLogLines.push(line);
    fs.writeSync(1, line + "\n");
};

const { app, BrowserWindow, screen } = require("electron");

const BUNDLE_DIR = process.env.PR67_BUNDLE_DIR;
if (!BUNDLE_DIR) {
    throw new Error("PR67_BUNDLE_DIR must point at the esbuild output directory");
}
app.setPath("userData", path.join(BUNDLE_DIR, "electron-probe-userdata"));

const FIXTURE_DIR = path.resolve(__dirname, "../../2026-09-26-webblock-popup");
const PRELOAD_PATH = path.resolve(FIXTURE_DIR, "harness/verify-preload.cjs");
const RACE_TRIALS = Number(process.env.PR67_RACE_TRIALS || 30);

const popupMod = require(path.join(BUNDLE_DIR, "emain-popup.bundle.cjs"));
const secMod = require(path.join(BUNDLE_DIR, "emain-websecurity.bundle.cjs"));

const results = [];
function record(id, assertion, observed, pass) {
    results.push({ id, assertion, observed, pass });
    console.log(`[probe-result] ${id} ${pass ? "PASS" : "FAIL"} | ${assertion} | ${JSON.stringify(observed)}`);
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

function newWindowsSince(before) {
    return BrowserWindow.getAllWindows().filter((w) => !before.has(w.id) && !w.isDestroyed());
}

async function destroyAll(wins) {
    for (const w of wins) {
        if (w && !w.isDestroyed()) {
            w.destroy();
        }
    }
    await wait(250);
}

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
                res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
                res.end(data);
            });
        });
        server.on("error", reject);
        // Reached as both 127.0.0.1 and localhost: two distinct sites, so site
        // isolation puts the root guest and cross-site children in separate processes.
        server.listen(0, "127.0.0.1", () => resolve(server));
    });
}

const weakenedHosts = new Set();

async function makeGuest(baseUrl, { weaken }) {
    const host = new BrowserWindow({
        show: false,
        webPreferences: { webviewTag: true, nodeIntegration: false, contextIsolation: true },
    });
    secMod.registerAppWebContents(host.webContents);
    if (weaken) {
        weakenedHosts.add(host.webContents.id);
    }
    const guestPromise = new Promise((resolve) => host.webContents.once("did-attach-webview", (_e, wc) => resolve(wc)));
    const hostHtml =
        `<!doctype html><meta charset="utf-8"><body style="margin:0">` +
        `<webview src="${baseUrl}/popup-fixture.html" partition="persist:webblock" allowpopups ` +
        `style="width:1000px;height:700px;display:flex;"></webview></body>`;
    await host.loadURL("data:text/html," + encodeURIComponent(hostHtml));
    const guest = await Promise.race([
        guestPromise,
        wait(15000).then(() => {
            throw new Error("timed out waiting for did-attach-webview");
        }),
    ]);
    await waitForLoad(guest);
    const ctx = {
        rootGuestId: guest.id,
        sendToTab: () => {},
        getParentWindow: () => (host.isDestroyed() ? null : host),
        getWorkArea: (parent) =>
            (parent == null ? screen.getPrimaryDisplay() : screen.getDisplayMatching(parent.getBounds())).workArea,
        preloadPath: PRELOAD_PATH,
    };
    popupMod.installGuestWindowOpenHandler(guest, ctx);
    return { host, guest, ctx };
}

function prefSummary(wc) {
    const p = wc.getLastWebPreferences();
    if (p == null) {
        return null;
    }
    return { sandbox: p.sandbox, contextIsolation: p.contextIsolation, nodeIntegration: p.nodeIntegration };
}

async function probeAboutBlank(baseUrl, label, weaken) {
    const { host, guest } = await makeGuest(baseUrl, { weaken });
    const before = allWindowIds();
    const opened = await guest.executeJavaScript(`!!window.open("about:blank", "ab", "width=300,height=300")`, true);
    await wait(600);
    const created = BrowserWindow.getAllWindows().filter((w) => !before.has(w.id));
    const child = created[0];
    const observed = {
        rendererGotWindow: opened,
        openerPrefs: prefSummary(guest),
        openerPid: guest.getOSProcessId(),
        windowsCreated: created.length,
        childSurvived: child != null && !child.isDestroyed(),
    };
    if (child != null && !child.isDestroyed()) {
        observed.childPrefs = prefSummary(child.webContents);
        observed.childPid = child.webContents.getOSProcessId();
        observed.childSharesOpenerProcess = observed.childPid === observed.openerPid;
        observed.childUrl = child.webContents.getURL();
    }
    observed.destroyLogged = mainLogLines.some((l) => l.includes("[popup] destroying child window") && l.includes("webPreferences"));
    await destroyAll([...created, host]);
    return observed;
}

async function probeCapRace(baseUrl, crossBaseUrl) {
    const { host, guest, ctx } = await makeGuest(baseUrl, { weaken: false });
    const root = ctx.rootGuestId;
    let maxLiveObserved = 0;
    let maxWindowsObserved = 0;
    let overrunTrials = 0;
    const perTrial = [];
    const childUrl = `${crossBaseUrl}/popup-child.html`;

    for (let trial = 0; trial < RACE_TRIALS; trial++) {
        const openers = [];
        for (let i = 0; i < 3; i++) {
            const before = allWindowIds();
            await guest.executeJavaScript(`window.open(${JSON.stringify(childUrl)}, "seed${trial}_${i}", "width=300,height=300"); undefined;`, true);
            await wait(250);
            const w = newWindowsSince(before)[0];
            if (w != null) {
                await waitForLoad(w.webContents);
                openers.push(w);
            }
        }
        const seedLive = popupMod.livePopupCount(root);
        const renderers = [guest, ...openers.map((w) => w.webContents)];
        const pids = new Set(renderers.map((wc) => wc.getOSProcessId()));

        const before = allWindowIds();
        const fireAt = Date.now() + 400;
        const script = (tag) =>
            `(() => { const t = ${fireAt}; while (Date.now() < t) {} ` +
            `const out = []; for (let i = 0; i < 3; i++) { out.push(!!window.open(${JSON.stringify(childUrl)}, "${tag}_" + i, "width=300,height=300")); } return out; })()`;
        const returns = await Promise.all(renderers.map((wc, idx) => wc.executeJavaScript(script(`burst${trial}_${idx}`), true)));
        await wait(800);
        const burstWins = newWindowsSince(before);
        const live = popupMod.livePopupCount(root);
        const totalWindows = openers.filter((w) => !w.isDestroyed()).length + burstWins.length;
        maxLiveObserved = Math.max(maxLiveObserved, live);
        maxWindowsObserved = Math.max(maxWindowsObserved, totalWindows);
        if (live > popupMod.MaxLivePopupsPerOpener || totalWindows > popupMod.MaxLivePopupsPerOpener) {
            overrunTrials++;
        }
        perTrial.push({
            trial,
            seedLive,
            distinctRendererPids: pids.size,
            rendererTruthy: returns.flat().filter(Boolean).length,
            burstWindows: burstWins.length,
            live,
            totalWindows,
        });
        await destroyAll([...openers, ...burstWins]);
    }
    await destroyAll([host]);
    return { trials: RACE_TRIALS, overrunTrials, maxLiveObserved, maxWindowsObserved, perTrial };
}

async function main() {
    await app.whenReady();
    // Each probe tears its host down; without this Electron quits on the first teardown.
    app.on("window-all-closed", () => {});

    app.on("web-contents-created", (_event, contents) => {
        contents.on("will-attach-webview", (event, webPreferences, params) => {
            const ok = secMod.isAppWebContentsId(contents.id) && secMod.hardenWebviewAttach(webPreferences, params, PRELOAD_PATH);
            if (!ok) {
                event.preventDefault();
                return;
            }
            if (weakenedHosts.has(contents.id)) {
                // Deliberate SEC-2 fault injection: an opener that escaped hardening.
                webPreferences.sandbox = false;
            }
        });
    });
    app.on("session-created", secMod.installPermissionHandlers);

    const server = await startStaticServer(FIXTURE_DIR);
    const port = server.address().port;
    const baseUrl = `http://127.0.0.1:${port}`;
    const crossBaseUrl = `http://localhost:${port}`;

    try {
        const hardened = await probeAboutBlank(baseUrl, "hardened", false);
        record("A1", "about:blank popup from a hardened opener lives", hardened, hardened.childSurvived === true);
        const weakened = await probeAboutBlank(baseUrl, "weakened", true);
        record("A2", "about:blank popup from a sandbox:false opener is destroyed", weakened, weakened.childSurvived === false);
    } catch (e) {
        record("A", "about:blank probe", "threw: " + ((e && e.stack) || e), false);
    }

    try {
        const race = await probeCapRace(baseUrl, crossBaseUrl);
        const multiProcess = race.perTrial.every((t) => t.distinctRendererPids >= 2);
        record("R0", "cap-race burst used >= 2 renderer processes every trial", { multiProcess }, multiProcess);
        record(
            "R1",
            `concurrent cross-process window.open() bursts never exceed MaxLivePopupsPerOpener (${race.trials} trials)`,
            { overrunTrials: race.overrunTrials, maxLiveObserved: race.maxLiveObserved, maxWindowsObserved: race.maxWindowsObserved },
            race.overrunTrials === 0
        );
        console.log("[probe-race-detail] " + JSON.stringify(race.perTrial));
    } catch (e) {
        record("R", "cap-race probe", "threw: " + ((e && e.stack) || e), false);
    }

    const failed = results.filter((r) => !r.pass).length;
    console.log(`[probe-summary] ${results.length - failed}/${results.length} PASS`);
    server.close();
    app.exit(failed === 0 ? 0 : 1);
}

main().catch((e) => {
    console.log("[probe] fatal", (e && e.stack) || e);
    app.exit(2);
});
