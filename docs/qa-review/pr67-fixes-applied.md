# PR #67 fixes applied (2026-09-26)

Source findings: `pr67-code-auditor.md` (findings 1, 2) and `security-auditor.md` (SEC-1).

## 1. [HIGH] SEC-1: popup redirect bypass

- `emain/emain-popup.ts` `hardenCreatedPopup`: the navigation guard is now one function, `blockDisallowedNavigation`, registered on both `will-navigate` and `will-redirect`. A server-side 3xx from an allowed https: popup URL to `file:`, `chrome:`, `javascript:` or `data:` is now cancelled.
- The guard reads `event.url` rather than the deprecated positional `url` argument (Electron 41 passes both).
- Test: `hardenCreatedPopup > blocks a server-side redirect to a disallowed scheme` emits `will-redirect` for four disallowed schemes and one allowed https URL. The existing will-navigate test now goes through the same `emitNav` helper, which passes the URL both ways, as Electron does.

## 2. [MEDIUM] Popup inherits the app menu

- `emain/emain-popup.ts`: `child.removeMenu()` replaces `child.setMenuBarVisibility(false)`, which only hid the menu and left its accelerators live on Linux/Windows.
- New export `isLivePopup(win)` checks the existing `livePopups` tracking.
- `emain/emain-menu.ts` covers macOS, where the app menu stays global:
  - New `asRemoteTermWindow(window)` helper using `instanceof RemoteTermBrowserWindow`. "Create Workspace" and the Alt/Cmd+Ctrl+1..9 workspace-switch handlers now use it instead of an unchecked cast. With a popup focused, switch is a no-op and create opens the workspace in a new visible window (the existing `createWorkspace(null)` path), with no TypeError and no orphaned workspace.
  - File > Close closes the focused window if it is a live popup, instead of closing the user's RemoteTerm window.
- Test: `hardenCreatedPopup > removes the inherited app menu so its accelerators cannot fire against the popup`. Electron's `BrowserWindow` has no `getMenu()`, so the fake window models menu state and the test asserts `removeMenu` ran and the menu is null. `isLivePopup` is asserted true while the popup is open and false after it closes. The emain-menu.ts guards have no unit test because the menu module has no test harness (it registers `ipcMain` handlers at import time).

## 3. [MEDIUM] Popup has no origin indicator

- `emain/emain-popup.ts`: `installOriginTitle` (called from `hardenCreatedPopup`) handles `page-title-updated` on the popup `BrowserWindow`, calls `preventDefault()`, and sets the title to `popupWindowTitle(currentUrl, pageTitle)`, i.e. `host - page title` (just `host` when the page title is empty; the whole URL when there is no host, e.g. `about:blank`). `did-navigate` updates the current URL and clears the old page title. The initial title is set from the creation URL straight away. The codebase had no existing window-title helper to reuse.
- The listener is on the window, not the webContents, because the window's own `page-title-updated` is the event whose `preventDefault()` stops Electron's automatic `setTitle`.
- Tests: `hardenCreatedPopup > prefixes the window title with the popup's current host, whatever title the page sets` covers the initial title, a spoofed "Sign in - Google Accounts" title, navigation to a different host:port, and a page title that impersonates a host. `popupWindowTitle` has its own test for the fallbacks.
- Not changed: the popup is still parented to the main window. The finding's fix section only asked for the title.

## Verification

- Mutation check: I removed each fix (the `will-redirect` line, `removeMenu()`, the `installOriginTitle` call, and the title `preventDefault()`) one at a time. Each time exactly the matching new test failed.
- `npx vitest run emain/emain-popup.test.ts emain/emain-websecurity.test.ts`: 49/49 passed.
- `npx vitest run` (full suite): 40 files, 415 tests passed.
- `npx tsc --noEmit`: exit 0.
- `npx eslint emain/emain-popup.ts emain/emain-popup.test.ts emain/emain-tabview.ts emain/emain-websecurity.ts emain/emain-websecurity.test.ts emain/emain-menu.ts`: exit 0, no output.
- Nothing was run against a live display or a real Electron instance.
