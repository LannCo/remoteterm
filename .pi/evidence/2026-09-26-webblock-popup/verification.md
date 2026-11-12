# Task 7 — Live verification with real Electron 41 (headless)

**Superseded by `49186e11`** — this run predates the will-redirect guard,
menu-removal, and origin-title fixes and does not cover them. See
`docs/qa-review/pr67-fixes-applied.md` for their (non-live) verification.

Verifies `emain/emain-popup.ts` + `emain/emain-websecurity.ts` against real
Electron/Chromium behaviour, per Task 7 of
`.pi/plans/2026-09-26-webblock-popup-support.md` (remoteterm-daily) and the
"Security-hardening parity checklist" in that plan.

## Deviation from the plan (as instructed)

Per the orchestrator's brief, this run does **not** use `task dev` or the
RemoteTerm app (`remoteterm-daily`'s dev process is live and must not be
touched or restarted). It instead extends the standalone harness from Task 1
(`harness/probe-main.cjs`) with a new harness, `harness/verify-main.cjs`,
that:

- bundles the **real** `emain/emain-popup.ts` and `emain/emain-websecurity.ts`
  with esbuild (`electron` external, CJS output) instead of using fakes;
- wires them up exactly the way the app does: `installGuestWindowOpenHandler`
  on a `<webview>` guest's `did-attach-webview` (mirrors
  `emain/emain-tabview.ts:325-339`), the global `will-attach-webview` refusal
  keyed on `isAppWebContentsId` (mirrors `emain/emain.ts:161-171`), and
  `installPermissionHandlers` on session creation (mirrors
  `emain/emain.ts`'s `electronApp.on("session-created", installPermissionHandlers)`,
  needed for assertion j — omitting it left Chromium's own default permission
  decision in effect instead of the app's);
- drives `popup-fixture.html` / `popup-child.html` (Task 1) via
  `executeJavaScript`, plus a few extra probes the fixture's buttons can't
  express (nested-cap race, webview-in-popup, clipboard, blocked navigation,
  guest-destroy cascade).

Nothing under `emain/` or `frontend/` was edited. `remoteterm-daily` was not
read or written.

## Command used

```
cd /media/owner/Workspace/remoteterm/remoteterm-webpopup
node_modules/.bin/esbuild emain/emain-popup.ts --bundle --platform=node --format=cjs --external:electron \
  --outfile=<scratch>/emain-popup.bundle.cjs
node_modules/.bin/esbuild emain/emain-websecurity.ts --bundle --platform=node --format=cjs --external:electron \
  --outfile=<scratch>/emain-websecurity.bundle.cjs
env -u DISPLAY -u WAYLAND_DISPLAY xvfb-run -a -s "-screen 0 1600x1000x24" \
  node_modules/.bin/electron --no-sandbox .pi/evidence/2026-09-26-webblock-popup/harness/verify-main.cjs
```

where `<scratch>` is
`/tmp/claude-1000/-media-owner-Workspace-remoteterm/65b2b635-6ca7-4df9-a139-9105022c901d/scratchpad`.

`--no-sandbox` was required, consistent with `dispositions.md`'s finding for
Task 1: without it, nothing progressed past `new BrowserWindow()` under this
container's Xvfb. **This disables Chromium's OS-level sandbox process-wide.**
`getLastWebPreferences()` for the popup does report `sandbox: true` (see
assertion g1 below), which is Electron's record of the *requested*
per-renderer sandbox preference — but with `--no-sandbox` in effect at the
process level, the OS-level sandbox itself is **not verified by this run**.

Raw run logs: `harness/verify-run.log`. Exit code 0 both runs.

## Results

| # | Assertion | Expected | Observed | Result |
|---|---|---|---|---|
| a1 | `window.open` with features creates exactly one `BrowserWindow` | 1 | 1 | PASS |
| a2 | Child page reports `window.opener is SET` | `window.opener is SET` | `window.opener is SET` | PASS |
| a3 | Opener's `window.open()` call returns a `WindowProxy` (non-null) | true | fixture log: `open(features) returned WindowProxy` | PASS |
| b | Child's `postMessage` reaches opener's `message` listener | true | fixture log includes `message from popup: {"ok":true,"from":"child"}` | PASS |
| c1 | Child's `window.close()` closes the `BrowserWindow` | true | true (`closed` event fired) | PASS |
| c2 | Opener-side `popup.closed` becomes true | true | fixture log includes `window.closed observed true` | PASS |
| d1 | Plain `window.open(url)`: no window, exactly one `sendToTab` call | 0 windows, +1 sendToTab | 0 windows, +1 sendToTab (disposition `foreground-tab`) | PASS |
| d2 | `noopener`+geometry call: no window, exactly one `sendToTab` call | 0 windows, +1 sendToTab | 0 windows, +1 sendToTab (disposition `new-window`, `noopener` downgrades it) | PASS |
| d3 | `target=_blank` link: no window, exactly one `sendToTab` call | 0 windows, +1 sendToTab | 0 windows, +1 sendToTab (disposition `foreground-tab`) | PASS |
| e1 | `about:blank` then navigate creates exactly one `BrowserWindow` | 1 | 1 | PASS |
| e2 | Popup navigates to child page, `window.opener` still SET | SET, url has `popup-child.html` | `window.opener is SET`, url ends `popup-child.html` | PASS |
| f1 | `popup.webContents.session === guest.session` | true | true | PASS |
| f2 | `popup.webContents.session !== session.defaultSession` | true | true | PASS |
| f3 | `session.storagePath` ends `Partitions/webblock` | true | `.../electron-verify-userdata/Partitions/webblock` | PASS |
| g1 | Effective webPreferences (excl. preload) match the hardened set | `{nodeIntegration:false, nodeIntegrationInSubFrames:false, contextIsolation:true, sandbox:true, webSecurity:true, webviewTag:false}` | identical | PASS |
| g1b | `getLastWebPreferences()` exposes the applied preload path | preload path present, equal to harness preload | **key absent entirely** — full raw object has no `preload` or `preloads` field at all (keys: `allowRunningInsecureContent, contextIsolation, disableDialogs, disablePopups, enableBlinkFeatures, experimentalFeatures, javascript, nodeIntegration, nodeIntegrationInSubFrames, safeDialogs, safeDialogsMessage, sandbox, webSecurity, webviewTag`) | **FAIL** |
| g1c | Indirect proof preload ran, via its `console.log` reaching the `console-message` event | fires | did not fire (listener attached after the popup's first navigation; second navigation's message, if any, also not observed) | **FAIL (inconclusive)** |
| g2 | In the popup's page context, `typeof require/process/module` all `"undefined"` | `["undefined","undefined","undefined"]` | `["undefined","undefined","undefined"]` | PASS |
| h0 | `livePopupCount(root)` is 0 before the nested-cap test | 0 | 0 | PASS |
| h1 | 5 nested feature-opens from a child leave exactly 4 live popups total (cap = `MaxLivePopupsPerOpener`) | 4 | root popup + 3 of 5 nested succeeded = 4; `livePopupCount(root)` = 4 | PASS |
| h2 | Main-process log shows `[popup] cap reached` at least once | true | true | PASS |
| h3 | `livePopupCount(root)` returns to 0 after destroying the h1 popups | 0 | 0 | PASS |
| h4 | 6 **synchronous** `window.open()` feature calls in one `executeJavaScript`, from 0 popups, create at most 4 (race check) | ≤4 | renderer-reported 4 truthy returns, 4 actual `BrowserWindow`s created, `livePopupCount(root)` = 4 | PASS |
| i | Inserting `<webview src="https://example.com">` into a popup's DOM attaches no guest | no `did-attach-webview` | `did-attach-webview` fired = false, `will-attach-webview` fired = false | PASS |
| j | `navigator.clipboard.readText()` in the popup rejects | rejects | `rejected: Failed to execute 'readText' on 'Clipboard': Read permission denied.` (main-process log: `[permission] denied clipboard-read for webContents=14`) | PASS |
| k1 | Popup `location.href = "file:///etc/hostname"` — URL unchanged | unchanged | unchanged (stayed on `popup-child.html`) | PASS |
| k2 | `will-navigate` fires for the attempt, and `[popup] blocked navigation` is logged | fires + logged | **`will-navigate` never fired at all** (`willNavigateFired=false`); log line never appears | **FAIL** |
| l0 | `livePopupCount(root)` is 0 before the guest-destroy test | 0 | 0 | PASS |
| l1 | 2 popups are live before destroying the guest | 2 | `livePopupCount`=2, actually open=2 | PASS |
| l2 | `livePopupCount(root)` returns to 0 after the guest is destroyed | 0 | 0 | PASS |
| l3 | Both popup `BrowserWindow`s are actually closed after the guest is destroyed | 0 still open | 0 still open | PASS |
| m | `isAppWebContentsId(popup.webContents.id)` is false | false | false | PASS |

**28 PASS / 3 FAIL** (g1b, g1c, k2). No assertion was skipped or faked; every
row above is a real Electron 41 result from the two runs in
`harness/verify-run.log` (first run hit a harness bug — see below — and was
re-run after fixing it; results shown are from the corrected, final run).

## The three FAILs, in detail

### g1b / g1c — `getLastWebPreferences()` does not expose the popup's preload path in this Electron version

The plan's checklist (Task 7 assertion g) assumes
`webContents.getLastWebPreferences().preload` reports the applied preload
path. Empirically, in Electron 41.1.0, the object it returns has **no
`preload` or `preloads` field at all** — see the raw dump in the results
table. I added a second check (g1c), listening for the preload script's own
`console.log("[verify-preload] loaded")` via the popup's `console-message`
event, as an indirect way to confirm the preload actually ran; that also
never fired, most likely because the listener was attached after the
popup's very first (`about:blank`) navigation already ran the preload, and
Electron's `console-message` event may not surface preload-context (as
opposed to page-context) console output at all in a sandboxed,
context-isolated renderer — I could not confirm which.

This is **not evidence the preload was wrong or missing**: assertion g2
(`typeof require/process/module` all `"undefined"` in the popup's page
context) passed, which is consistent with `contextIsolation`+`sandbox` being
correctly applied. But it means this harness could not positively confirm
*which* preload path Electron actually loaded for the popup, only that
`buildPopupWindowOptions`'s hardened webPreferences (sandbox, contextIsolation,
nodeIntegration, etc.) took effect. A future check for this specific claim
would need a preload that calls `contextBridge.exposeInMainWorld` with a
value the page can then report back, or `webContents.debugger` attached to
the `Page` domain — both out of scope for this pass.

### k2 — the `will-navigate` guard never fires for `location.href = "file:///..."`; something else blocks it

`hardenCreatedPopup` installs `wc.on("will-navigate", ...)` to reject
navigations `isAllowedPopupUrl` disallows, and logs `[popup] blocked
navigation to <url>` when it does. Setting `location.href` to a `file://` URL
from *inside* the popup's own page script left the URL unchanged (k1: PASS —
the popup did **not** navigate to the local file), but the raw instrumentation
added for this run shows **`will-navigate` never fired at all**, and the
`[popup] blocked navigation` log line never appears.

The observed behaviour (URL unchanged) is correct and matches the plan's
security intent, but not for the reason the plan's checklist states. The
most likely explanation is that Chromium's own renderer-process URL-commit
restrictions refuse an http(s)-origin, sandboxed renderer's attempt to
self-navigate to `file://` before it ever becomes a `will-navigate` event —
i.e. Chromium is the actual enforcement point here, not
`emain-popup.ts`'s guard. That guard may still matter for other blocked-URL
shapes (e.g. `javascript:` or a scheme Chromium doesn't itself refuse at the
process level) that this run did not separately probe. This is a real,
reproducible finding, not a flake — worth a note in the code (or a follow-up
probe against a scheme Chromium *does* let reach `will-navigate`, e.g. a
second http(s) origin that isAllowedPopupUrl would still allow, to confirm
the listener is wired at all) before treating that log line as a security
control test can rely on.

## Harness bug found and fixed during this run (not a product-code issue)

The first run of `verify-main.cjs` threw `Error: An object could not be
cloned` for every assertion using the `openPopupViaScript` helper (h, i, j,
k, and one of l's setup calls), because the script's last-evaluated
expression was the return value of `window.open(...)` — a `WindowProxy`,
which Electron's `executeJavaScript` cannot structured-clone back across the
IPC boundary. Fixed by appending `; undefined;` to the injected script so its
completion value is serialisable. This was purely a harness authoring bug in
this session's own code, not in `emain-popup.ts`; noted here because the
first run's partial failures (and one contaminated `l0` count from leftover,
undestroyed popups) are visible in `harness/verify-run.log`'s earlier lines
and should be read as "harness broke", not "product broke".

Also fixed before any use: the harness did not initially call
`installPermissionHandlers` on session creation (the app does this via
`electronApp.on("session-created", installPermissionHandlers)` in
`emain/emain.ts`). Without it, assertion j's clipboard read *resolved*
instead of rejecting, because Chromium's own default permission decision was
in effect rather than the app's gated one. Adding the missing wiring (which
mirrors real app startup, not a change to `emain-popup.ts`/`emain-websecurity.ts`
themselves) made assertion j pass for the right reason.

## Not covered by this run

- **Plan row 10 (popup minimises with the parent window)** — not meaningfully
  testable headless; there is no real window manager under Xvfb to observe
  minimize-with-parent semantics. Left for the owner to check manually.
- **Plan row 13 (real Google sign-in / GIS popup completes and closes
  itself)** — requires a live network round-trip to Google's own OAuth
  endpoints and a real Google account; out of scope for an offline harness.
  Left for the owner.
- **OS-level sandbox** — this run used `--no-sandbox` process-wide (required
  to get Electron running at all under this container's Xvfb, per
  `dispositions.md`). `webPreferences.sandbox: true` was confirmed as the
  *requested* per-renderer setting (assertion g1), but the actual OS sandbox
  enforcement is not exercised or verified by this run.

## Files created (this task only)

- `.pi/evidence/2026-09-26-webblock-popup/harness/verify-main.cjs`
- `.pi/evidence/2026-09-26-webblock-popup/harness/verify-preload.cjs`
- `.pi/evidence/2026-09-26-webblock-popup/harness/verify-run.log`
- `.pi/evidence/2026-09-26-webblock-popup/verification.md` (this file)
- `<scratch>/emain-popup.bundle.cjs`, `<scratch>/emain-websecurity.bundle.cjs` (esbuild output, scratch dir, not committed)

## Re-run after security fixes

Re-run against the three security-review fixes to `emain/emain-popup.ts`
(commits `053aa30b` H-1/H-1b feature allowlist + native-option pinning,
`5a40cc2f` M-1 featureless-`new-window` routing + popup session pinning,
`24b5b85c` Info-1 tautological-preload-check removal). Both bundles were
rebuilt from the fixed source immediately before this run:

```
cd /media/owner/Workspace/remoteterm/remoteterm-webpopup
node_modules/.bin/esbuild emain/emain-popup.ts --bundle --platform=node --format=cjs --external:electron \
  --outfile=<scratch>/emain-popup.bundle.cjs
node_modules/.bin/esbuild emain/emain-websecurity.ts --bundle --platform=node --format=cjs --external:electron \
  --outfile=<scratch>/emain-websecurity.bundle.cjs
env -u DISPLAY -u WAYLAND_DISPLAY xvfb-run -a -s "-screen 0 1600x1000x24" \
  node_modules/.bin/electron --no-sandbox .pi/evidence/2026-09-26-webblock-popup/harness/verify-main.cjs
```

Same scratch dir, same `--no-sandbox` caveat as the first run (see above).
Exit code 0. Raw log overwrites `harness/verify-run.log`.

New assertions n-s were added to `harness/verify-main.cjs`, and the fixture
(`popup-fixture.html`) gained one plain `<a id="plainlink">` for the
shift-click case (p); the preload (`verify-preload.cjs`) now also calls
`contextBridge.exposeInMainWorld("__verifyPreload", "ok")` for assertion q.
The new assertions were inserted before the guest-destroy block (l/m), not
after m, because m's predecessor test destroys the guest — the first attempt
to run n-s after m threw `Object has been destroyed` on every one of them.

### Rows a-m: re-run status

All of a1-m are unchanged from the first run: **28 PASS / 3 FAIL**, the same
three FAILs (g1b, g1c, k2), for the same reasons already documented above.
None of the three fixes touch the code paths those rows exercise (session
identity, sandboxing, clipboard, the nested-cap race, guest-destroy cascade),
so no regression and no improvement was expected or observed there. g1b/g1c
are superseded by assertion q below; k2 is not superseded, but r2/r3 add
positive evidence the `will-navigate` guard itself is wired (see notes under
row r below).

### New assertions (n-s)

| # | Assertion | Expected | Observed | Result |
|---|---|---|---|---|
| n1 | `window.open` with `popup,frame=no,transparent=yes,alwaysOnTop=yes,fullscreen=yes,skipTaskbar=yes,closable=no,show=no` creates no window, one `sendToTab` call | 0 windows, +1 sendToTab | 0 windows, +1 sendToTab | PASS |
| n2 | Same, with `popup,alwaysontop=yes` (lowercase) | 0 windows, +1 sendToTab | 0 windows, +1 sendToTab | PASS |
| n3 | Same, with `popup,webContents=1` (H-1b) | 0 windows, +1 sendToTab | 0 windows, +1 sendToTab | PASS |
| o1 | `buildPopupWindowOptions` pins `frame/transparent/fullscreen/alwaysOnTop/skipTaskbar/closable/show/kiosk/modal` for an allowed popup | all pinned values | all matched (full object dumped in `verify-run.log`) | PASS |
| o2 | A real allowed popup's actual `BrowserWindow` state | `isAlwaysOnTop=false, isClosable=true, isVisible=true, isFullScreen=false` | identical | PASS |
| p1 | Shift-clicking a plain `<a href>` (via `sendInputEvent` mouseDown/mouseUp, `modifiers:["shift"]`) creates no window, one `sendToTab` call | 0 windows, +1 sendToTab | 0 windows, +1 sendToTab; handler saw `disposition=new-window, features=""` | PASS |
| p2 | No new `webContents` appear on `session.defaultSession` from the shift-click | `[]` | `[]` | PASS |
| q | Popup page sees `window.__verifyPreload === "ok"`, set by the applied preload via `contextBridge.exposeInMainWorld` | `"ok"` | `"ok"` | PASS |
| r1 | Popup `webContents.listenerCount("will-navigate") >= 1` | `>=1` | `1` | PASS |
| r2 | Renderer sets `location.href = "ssh://example.invalid/"`: `will-navigate` fires and the guard blocks it | fires + blocked | `will-navigate fired=true, url seen=ssh://example.invalid/, guard blocked=true` | PASS |
| r3 | Renderer sets `location.href = "mailto:x@example.invalid"`: `will-navigate` fires and the guard blocks it | fires + blocked | `will-navigate fired=true, url seen=mailto:x@example.invalid, guard blocked=true` | PASS |
| s1 | Nested `window.open("about:blank",...); w.open(location.href,...)` in one `executeJavaScript`: every resulting `BrowserWindow` has a `did-create-window` listener installed | true for all | 2 windows created (ids 18, 19), each with 1 listener | PASS |
| s2 | Live popups from that nested chain stay within `MaxLivePopupsPerOpener` | `<=4` | `2` | PASS |
| s3 | No `webblock`-session `webContents` exists without an owning `BrowserWindow` (besides the guest `<webview>` itself) | `0` | `[]` (none found) | PASS |

**14/14 new assertions PASS.** No product-code defect surfaced by n-s: every
row confirms the fix behaves as specified (H-1 allowlist + pinning holds
under a live Electron window, M-1's shift-click routing and session pin both
hold, and the router/cap survive a synchronous nested-open chain with no
orphaned webContents).

### r2/r3 vs. the original k2 finding

r2 and r3 show `will-navigate` **does** fire and **is** blocked for
`ssh://` and `mailto:` targets, confirming `hardenCreatedPopup`'s
`will-navigate` listener is genuinely wired and does real work for schemes
Chromium hands to the browser layer. This narrows, but does not overturn,
the original k2 finding: the `file://` case in k2 still shows no
`will-navigate` event at all, which is consistent with the original
hypothesis that Chromium refuses that specific http(s)->`file://`
self-navigation at the renderer's own URL-commit layer, before the event
would ever reach `emain-popup.ts`. No code change made for k2 — the observed
behaviour (URL unchanged) is still correct, and r2/r3 are the requested
positive evidence that the guard itself is not simply dead.

### q vs. the original g1b/g1c finding

q directly confirms preload effectiveness by having the preload call
`contextBridge.exposeInMainWorld` and reading the exposed value back from the
popup's own page context, sidestepping `getLastWebPreferences()`'s missing
`preload` field (g1b) and the unobserved `console-message` event (g1c)
entirely. This is a strictly stronger check than either: g1b/g1c tried to
infer that the preload ran; q proves it by observing code the preload itself
injected into the page.

## Deviation

No product code was changed as part of this re-run; every one of the three
security fixes was applied and committed beforehand (`053aa30b`, `5a40cc2f`,
`24b5b85c`) per the unit-test-first flow, before this harness ever ran. This
re-run is verification only.
