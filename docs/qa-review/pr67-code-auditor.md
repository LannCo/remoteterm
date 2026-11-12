# code-auditor report: PR #67 (webblock-popup-support)

**Target:** `pr/webblock-popup-support` vs `origin/main` (e80880d0). Scope: `emain/emain-popup.ts`, `emain/emain-popup.test.ts`, `emain/emain-tabview.ts`, `emain/emain-websecurity.ts`, `emain/emain-websecurity.test.ts`.

(Kept separate from `code-auditor.md`, which holds the committed round-1 fleet report for the unrelated `qa/fleet-2026-09-22` branch — commit `35cd6146`. Do not overwrite that file.)

## Verdict: NEEDS_WORK — nothing Critical/High, isolation core holds

- Features-string allowlist: no features string reaches a `BrowserWindow` with weaker webPreferences than the `<webview>` guest gets.
- Session pinning: a popup cannot land on the default (`X-AuthKey`) session.
- Popup cap: checked on the only `allow` return, tracked on both Electron window-creation paths.
- `emain-tabview.ts`'s own `setWindowOpenHandler` does not bypass the new hardening.
- Checked against Electron v41.1.0 source (installed version): feature parser is case-sensitive, drops `resizable`; only `width/height/x/y` can pass through and our window options override all four, applied after webPreferences. Session comes from `opener.session` on our window-options path; backstop check runs synchronously in the same step as window creation, so a popup never runs unchecked. Nested popups get their own handler on `did-create-window`, routed through the same cap.
- Vitest 45/45, `tsc --noEmit` clean, eslint clean on the three changed modules.

## Findings

1. **[Medium] Popups inherit the app menu and its shortcuts** (`emain/emain-popup.ts:206`, `:281`). On Linux/Windows every new window gets the app menu; `autoHideMenuBar`/`setMenuBarVisibility(false)` only hide it, shortcuts still fire. Alt+Ctrl+1..9 workspace-switch throws a TypeError in main (popup has no `switchWorkspace`, `emain-menu.ts:65-67`). File > Close closes the user's RemoteTerm window, not the popup (`emain-menu.ts:138-142`). "Create Workspace" creates one that's never shown. **Fix:** `child.removeMenu()` in `hardenCreatedPopup`; on macOS guard menu handlers with `instanceof RemoteTermBrowserWindow`.
2. **[Medium] Popup shows no origin, stays always-on-top** (`emain/emain-popup.ts:200-227`). No URL bar, page sets its own title, `parent` keeps it on top. Any web-block page can open a fake "Sign in - Google" window the user can't distinguish from real. Old pane path showed the URL. **Fix:** on `page-title-updated`, override the title to `host - title`; refresh on navigation.
3. **[Low] No click required to open popups** (`emain/emain-popup.ts:247`). Electron has no built-in popup blocker; webview sets `allowpopups`. A page can open 4 windows on load and reopen one each time the user closes it. **Fix:** only route to popup if the guest had real user input in the last few seconds; else pane.
4. **[Low] `noopener=1`/`noopener=yes`/`noreferrer=1` not recognised** (`:69`, `:122`) — only bare `noopener` matches, so these variants get a real window instead of the pane. No isolation impact. **Fix:** parse per HTML-spec boolean semantics, add tests.
5. **[Low] Unused `WebBlockPartition` import** at `emain/emain-popup.test.ts:140` — already fixed in the working tree (mechanical-lint pass); also `frameName` destructure dropped at `emain-tabview.ts:362`. Both intentional (this session's own mechanical lint step), not stray.
6. **[Info] Allowlist check doesn't special-case `__proto__`** (`:53`, `:104`) — `popup,__proto__=x` still classifies as popup. No effect today (Electron's own parser also drops that key). **Fix:** build the parsed object with `Object.create(null)`.
7. **[Info] Popup cap relies on Chromium's message ordering**, undocumented in code — 6 back-to-back `window.open()` calls stop at 4 only because of that ordering (confirmed on Electron 41 per evidence row h4). **Fix:** one-line comment at `:247`.
8. **[Info] `.pi/evidence/2026-09-26-webblock-popup/verification.md` is stale**: cites SHAs not on this branch, stale line numbers (`emain.ts:161-171` vs current `:144-153`), points to an uncommitted `verify-run.log`, doesn't cover the latest commit `9a72e5c4`, re-implements tabview wiring rather than exercising it for real, ran with `--no-sandbox`, and contains absolute local paths.

## Not run
No live/display work performed. One harness check would confirm finding 1 directly: in `.pi/evidence/.../harness/verify-main.cjs`, set an application menu, open an allowed popup, assert `popup.getMenu() == null` (expected to fail today). Run under `env -u DISPLAY -u WAYLAND_DISPLAY xvfb-run` if pursued.

AGENT_VERDICT: NEEDS_WORK
