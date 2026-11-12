"use strict";
/**
 * Standalone Electron harness for Task 1 of
 * .pi/plans/2026-09-26-webblock-popup-support.md (remoteterm-daily).
 *
 * Loads popup-fixture.html into a <webview> guest, installs a
 * setWindowOpenHandler on the guest that logs the HandlerDetails Electron
 * reports for each window.open() shape, and denies every one (so nothing
 * actually opens). Deliberately does not touch remoteterm-daily, `task dev`,
 * or the app's own webview-attach code: this is a minimal reproduction to
 * confirm the disposition strings Electron 41 actually reports.
 *
 * Kept structured for reuse: a later task extends this to load the real
 * emain/emain-popup.ts (bundled with esbuild) in place of the inline
 * setWindowOpenHandler below, to verify opener/postMessage/window.closed
 * end-to-end.
 */

const path = require("path");
const http = require("http");
const fs = require("fs");
const { app, BrowserWindow } = require("electron");

// Off-limits: must not touch ~/.config. Scratch dir per task brief.
const SCRATCH_USERDATA =
  "/tmp/claude-1000/-media-owner-Workspace-remoteterm/65b2b635-6ca7-4df9-a139-9105022c901d/scratchpad/electron-probe-userdata";
app.setPath("userData", SCRATCH_USERDATA);

const EVIDENCE_DIR = path.resolve(__dirname, "..");

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
};

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

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function main() {
  await app.whenReady();

  const server = await startStaticServer(EVIDENCE_DIR);
  const port = server.address().port;
  const baseUrl = `http://127.0.0.1:${port}`;
  console.log("[popup-probe] serving " + baseUrl + " from " + EVIDENCE_DIR);

  console.log("[popup-probe] creating BrowserWindow");
  const win = new BrowserWindow({
    show: false,
    webPreferences: {
      webviewTag: true,
      nodeIntegration: false,
      contextIsolation: true,
    },
  });
  console.log("[popup-probe] BrowserWindow created");

  // Registered BEFORE loadURL: did-attach-webview can fire while the host
  // page's own load is still in flight, so a listener attached after
  // `await win.loadURL(...)` can miss it entirely.
  const guestPromise = new Promise((resolve) => {
    win.webContents.on("did-attach-webview", (_event, guestWebContents) => {
      console.log("[popup-probe] did-attach-webview fired");
      guestWebContents.setWindowOpenHandler((details) => {
        console.log(
          "[popup-probe]",
          JSON.stringify({
            url: details.url,
            disposition: details.disposition,
            features: details.features,
            frameName: details.frameName,
          }),
        );
        return { action: "deny" };
      });
      resolve(guestWebContents);
    });
  });

  const hostHtml =
    `<!doctype html><meta charset="utf-8"><body style="margin:0">` +
    `<webview id="guest" src="${baseUrl}/popup-fixture.html" partition="persist:webblock" ` +
    `allowpopups style="width:1200px;height:900px;display:flex;"></webview>` +
    `</body>`;
  console.log("[popup-probe] loading host page");
  await win.loadURL("data:text/html," + encodeURIComponent(hostHtml));
  console.log("[popup-probe] host page loaded");

  const timeoutPromise = wait(15000).then(() => {
    throw new Error("timed out waiting for did-attach-webview");
  });
  const guest = await Promise.race([guestPromise, timeoutPromise]);

  if (guest.isLoadingMainFrame()) {
    await new Promise((resolve) => guest.once("did-finish-load", resolve));
  }

  async function click(selector) {
    await guest.executeJavaScript(
      `document.querySelector(${JSON.stringify(selector)}).click();`,
      true,
    );
    await wait(400);
  }

  async function evalOpen(code) {
    await guest.executeJavaScript(code, true);
    await wait(400);
  }

  console.log("[popup-probe] --- driving fixture triggers ---");
  await click("#feat");
  await click("#plain");
  await click("#noopener");
  await click("#blank");
  await click('a[target="_blank"]');

  console.log("[popup-probe] --- driving extra probes ---");
  // window.open(url, "_blank", "popup") — a features string with no size,
  // just the "popup" token. Not one of the fixture's buttons.
  await evalOpen(
    `window.open(new URL("popup-child.html", location.href).href, "_blank", "popup");`,
  );
  // window.open(url, "x", "noopener") — named target, noopener, no size.
  await evalOpen(
    `window.open(new URL("popup-child.html", location.href).href, "x", "noopener");`,
  );

  console.log("[popup-probe] --- done, quitting ---");
  server.close();
  win.destroy();
  app.quit();
}

main().catch((err) => {
  console.error("[popup-probe] fatal", err);
  process.exitCode = 1;
  app.quit();
});
