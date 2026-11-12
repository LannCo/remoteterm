# PR #67 follow-up: SEC-2 and SEC-3

Follows `pr67-security-auditor.md`, which deferred SEC-2 (Medium) and SEC-3 (Low) as "safe today but unenforced/unverified".

Evidence harness: `.pi/evidence/2026-09-26-pr67-followup/harness/probe-main.cjs` (real Electron 41.1.0, Xvfb, loads the real `emain-popup.ts`/`emain-websecurity.ts` bundled with esbuild). Run:

```
B=<scratch>/bundle
node_modules/.bin/esbuild emain/emain-popup.ts --bundle --platform=node --format=cjs --external:electron --outfile=$B/emain-popup.bundle.cjs
node_modules/.bin/esbuild emain/emain-websecurity.ts --bundle --platform=node --format=cjs --external:electron --outfile=$B/emain-websecurity.bundle.cjs
env -u DISPLAY -u WAYLAND_DISPLAY PR67_BUNDLE_DIR=$B PR67_RACE_TRIALS=30 xvfb-run -a -s "-screen 0 1600x1000x24" \
  node_modules/electron/dist/electron --no-sandbox .pi/evidence/2026-09-26-pr67-followup/harness/probe-main.cjs
```

## SEC-2: about:blank popups inherit the opener's webPreferences — fixed

### Finding, confirmed live

Before the fix, with a webview guest deliberately weakened to `sandbox: false` after `hardenWebviewAttach`:

| | opener prefs | child `getLastWebPreferences()` | child pid == opener pid | child survived |
|---|---|---|---|---|
| A1 hardened opener | sandbox true | sandbox true | yes | yes |
| A2 weakened opener | **sandbox false** | **sandbox true** | yes | **yes** |

The about:blank child runs in the opener's renderer process, yet its own `getLastWebPreferences()` reports the hardened override from `buildPopupWindowOptions`. A check on the child's prefs alone would therefore be a false backstop: the only meaningful check for about:blank is on the opener.

### Change

- `emain/emain-popup.ts`: new `hasHardenedPreferences(wc)` (sandbox true, contextIsolation true, nodeIntegration / nodeIntegrationInSubFrames not true). `hardenCreatedPopup` destroys the popup if either the child's or the opener's prefs fail it, alongside the existing session and app-owned-id checks. `getLastWebPreferences` is a real Electron 41 `WebContents` method (Electron's own `guest-window-manager.ts` `makeWebPreferences` reads it) but is absent from `electron.d.ts`, so it is accessed through a narrowed type; a missing accessor or `null` prefs fails closed.
- Comments at `buildPopupWindowOptions` and at the `about:` entry of `AllowedPopupUrlProtocols` (`emain-websecurity.ts`) documenting that about:blank relies on opener hardening: `hardenWebviewAttach` (via `will-attach-webview`, `emain.ts:145`) for root guests, `buildPopupWindowOptions` for nested popup openers.

### Verification

- Live, after the fix: A1 PASS (hardened opener's about:blank popup lives), A2 PASS (weakened opener's popup destroyed; log `[popup] destroying child window: opener webPreferences are not hardened`; zero windows remain).
- Unit tests (`emain-popup.test.ts`): about:blank from a `sandbox: false` opener is destroyed and untracked even though the child reports hardened prefs; popups with weakened own prefs (contextIsolation false, nodeIntegration true, nodeIntegrationInSubFrames true, sandbox unset), a missing accessor, or `null` prefs are destroyed.

## SEC-3: check-then-track race on the 4-popup cap — not reproducible, code unchanged

### Why a unit test cannot settle it

At unit level `handleGuestWindowOpen` trivially returns `allow` repeatedly until `trackPopup` runs; the question is whether Electron can interleave two allow decisions before either `did-create-window`. In Electron 41 the allow decision runs in `-will-add-new-contents` (`IsWebContentsCreationOverridden`, during the renderer's sync `CreateNewWindow`) and `did-create-window` fires later from `-add-new-contents` (`AddNewContents`, on the renderer's `ShowCreatedWindow`). Only a real run answers it.

### Tests

1. Single renderer, 6 synchronous `window.open()` calls in one task: already covered by assertion h4 in `.pi/evidence/2026-09-26-webblock-popup/verification.md` (4 created, cap held).
2. New, cross-process (R0/R1): root guest on `127.0.0.1`, 3 popups seeded on `localhost` (separate site, separate renderer process; 2 distinct renderer pids confirmed every trial), bringing the count to 3. Then all 4 renderers busy-wait to a shared wall-clock instant and each fire 3 `window.open()` calls (12 concurrent attempts against 1 remaining slot).

Result, 30 trials after the SEC-2 fix (10 more before it, same outcome): every trial exactly 1 renderer-side truthy return, 1 new window, `livePopupCount` 4; overrun trials 0, max live 4.

Conclusion: no overrun observed in single-process or synchronised cross-process bursts. This is empirical, not a proof over every scheduling; the residual exposure, if any, stays what the auditor rated it (Low: extra popups would still be fully hardened). No code change.

## Commands (all green)

- `npx vitest run emain/emain-popup.test.ts emain/emain-websecurity.test.ts`: 51 passed.
- `npx tsc --noEmit`: clean.
- `npx eslint emain/emain-popup.ts emain/emain-popup.test.ts emain/emain-websecurity.ts`: clean.
- `npx vitest run`: 420 passed, 7 failed, all in `frontend/app/view/term/kitty-graphics-chunking.test.ts` (`@xterm/addon-image` patched-behaviour assertions, e.g. `onImageAdded is not a function`). That file imports nothing from `emain/` and this change touches only `emain/`; the failures are unrelated. Cause not investigated here (likely the `patches/@xterm+addon-image` state of this worktree's `node_modules`).
