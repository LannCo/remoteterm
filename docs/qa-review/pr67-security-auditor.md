# security-auditor report: PR #67 (webblock-popup-support)

**Target:** `pr/webblock-popup-support` vs `origin/main` (e80880d0).

(Kept separate from `security-auditor.md`, which holds the committed round-1 fleet report for the unrelated `qa/fleet-2026-09-22` branch — commit `35cd6146`. Do not overwrite that file.)

Summary: Critical 0, High 1, Medium 1, Low 2.

## Findings

**[HIGH] SEC-1 — Popup navigation guard listens for `will-navigate` only, not `will-redirect`.**
`emain/emain-popup.ts:282-287` (`hardenCreatedPopup`) only hooks `will-navigate`. Electron fires a separate `will-redirect` event for server-side 3xx redirects, and no `will-redirect` listener existed anywhere in the popup/webview/emain.ts navigation surface. A guest page could `window.open()` its own https: URL (passes `isAllowedPopupUrl`, `emain-websecurity.ts:68-77`), then 302-redirect it to `file:///...`, never re-checked — defeating the invariant at `emain-websecurity.ts:10` ("Popups never need file:... is hostile"). Impact: sandboxed popup renders arbitrary local file contents readable by the app's OS user. **Status: fixed** — see `pr67-fixes-applied.md`, commit `49186e11`.

**[MEDIUM] SEC-2 — `about:blank` popups get webPreferences copied from the opener, not the computed override.**
Electron's documented behaviour: for `window.open("about:blank", ...)`, the child's webPreferences are copied from the parent and cannot be overridden. Since `about:` is a deliberately-allowed popup scheme (`emain-websecurity.ts:11`, tested at `emain-popup.test.ts:37`), the hardened `webPreferences` computed in `buildPopupWindowOptions` (`emain-popup.ts:186-228`) is silently a no-op for this case. Currently safe only because every opener in the call graph ends up with identical sandbox/contextIsolation/nodeIntegration/preload/session — an architectural invariant, not something the code or tests enforce or comment on. **Status: not fixed this pass** — recommend a code comment documenting the reliance, plus a defensive runtime check in `hardenCreatedPopup` if Electron exposes an accessor for it.

**[LOW] SEC-3 — Possible check-then-track race on the 4-popup-per-opener cap** (`emain-popup.ts:157-176, 247-251`). Count increments only on `did-create-window`/`trackPopup`, not at the `"allow"` decision point in `handleGuestWindowOpen`. Could not verify without a live Electron run (would need: a headless harness firing `window.open()` in a burst, counting resulting `BrowserWindow`s). Low severity — every extra popup would still be fully hardened; this is a resource/UX control, not a security boundary. **Status: unverified, not fixed** — deferrable.

**[LOW/INFO] SEC-4 — The `"tab"` route forwards the guest's URL to the renderer with no scheme check** (`emain-popup.ts:109-129, 239-241`). Pre-existing behaviour, not introduced by this PR (the PR actually made the `"deny"` path more restrictive). The `webview-new-window` IPC consumer wasn't found in this worktree's `frontend/` tree, so downstream handling couldn't be assessed — scope-boundary note, not a new finding.

## Threat-model verification
1. Feature-string escape to nodeIntegration/no-sandbox/wrong-preload: verified clean for non-`about:blank` URLs (see SEC-2 for the one exception).
2. Popup landing in default (`X-AuthKey`) session: verified not reachable — two independent destroy checks pin popups off `defaultSession`; `configureAuthKeyRequestInjection` (`authkey.ts:21`) is wired only onto `electron.session.defaultSession` (`emain.ts:320`), so a guest-session popup wouldn't hit that hook even without the destroy checks.
3. TOCTOU between `BrowserWindow` creation and `did-create-window` hardening: verified low risk — sandbox/contextIsolation/nodeIntegration/session/preload are constructor-time properties already fixed before `did-create-window` runs.
4. webContentsId forgery/collision: verified not exploitable — ids are Electron-internal, never renderer-suppliable; `registerAppWebContents` removes an id from its registry synchronously on that object's `"destroyed"` event.
5. Exceeding the 4-live-popup cap: see SEC-3 (unverified, low severity).

Static review only (diff plus `emain-popup.test.ts`/`emain-websecurity.test.ts`) — no live display or live Electron instance used.

AGENT_VERDICT: NEEDS_WORK (pre-fix) → SEC-1 (the only High) is now fixed; SEC-2/3/4 remain as deferrable Medium/Low/Info.
