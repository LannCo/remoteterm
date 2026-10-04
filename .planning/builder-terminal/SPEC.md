# Builder terminal panel: design spec (v5)

Date: 2026-10-04. Branch: `feat/builder-terminal` off local `main` 0fee7150 (builder-by-hand).
v5 folds in four audit rounds (requirements and security); disposition tables at the end. In this spec `Cmd:` is the keymodel notation: Cmd on macOS, Alt on Linux/Windows (`frontend/util/keyutil.ts:78-84`).

## Problem

The Tsunami app builder window was laid out as "build on the left, preview on the right": a Wave AI chat panel on the left, the app panel (Preview / Code / Config/Data / Files) and build output on the right. This fork removed the chat (`1a7f1c83`), and builder-by-hand removed the placeholder and the horizontal split (`872f5fd0`). The builder is now a single column, and the only way to drive an app with an agent harness is "Open terminal", which puts a shell in a *main* window, away from the preview.

## Goal

The left side of the builder window becomes a tiled terminal area, so one or more agent harnesses (Claude Code, Codex, Hermes, a plain shell) can work on the app side by side with its live preview. It behaves like a tab in the main window: split, resize, magnify, focus, close.

## Success criteria (each has a named observable)

- S1. First open of an app in a builder window shows a terminal panel on the left with exactly one running local shell, focused; `pwd` in it prints the app dir. (A renderer reload keeps existing panes; S1 is about first open.)
- S2. "Open terminal" (app panel header, and the empty-state button) adds one new shell pane to the builder's terminal panel, cwd = app folder, and focuses it. It never touches main windows.
- S3. New-pane and split keybindings, header split buttons and context-menu splits, with a builder pane as source, add a local shell pane with cwd = app folder, focused. Two splits in quick succession both appear. (Deviation from main windows: a builder split never copies the source pane's connection or cwd.)
- S4. With the terminal side focused, `Cmd:w` closes the focused pane, including the last one: the pane's block row is deleted and its shell PID (from `echo $$`) is gone within 5 s. After the last pane closes, the panel shows an empty state with an "Open terminal" button, builder focus moves to the app side, and the window stays open. With the app side focused, `Cmd:w` closes the builder window. Ctrl+W is never bound in the builder (it stays readline word-delete).
- S5. Terminal panel width is resizable and persists for the window's lifetime via `builder:layout` key `terminal`.
- S6. Closing the builder window (title-bar close and `Cmd:w` from the app side), and switching it to another app (`switchBuilderApp`), each delete the builder tab, its blocks and layout state from the DB, and every pane's shell PID is gone within 5 s. Processes an agent detached with `setsid`/`nohup` are out of scope.
- S7. After a backend crash or quit that skipped S6 cleanup, the next server start deletes every leftover builder tab and its blocks, logging `[startup] builder sweep: removed N tabs` before the `WAVESRV-ESTART` line (`cmd/server/main-server.go:344`).
- S8. Builder tabs never appear in any workspace's `tabids`, tab bar, or the workspace switcher.
- S9. Writes to `app.go` from two panes within one rebuild debounce window (300 ms, `pkg/buildercontroller/appwatcher.go:34`), with live rebuild on, produce exactly one `building` transition.
- S10. A tab that belongs to a workspace is never deleted by builder cleanup or the startup sweep, whatever its meta says.

## Non-goals

- Auto-launching a harness command in new panes (plain shell; the user starts the harness).
- Persisting panes or shells across window close (decided with the user: shells die with the window).
- Arbitrating concurrent edits to the same file by different agents.
- Remote connections in builder panes (D6: switcher hidden, server rejects non-local `connection` for builder-tab blocks).
- Isolation between builders, or between a builder and main windows, against a pane or renderer acting deliberately through generic RPCs (`CreateBlockCommand`, `DeleteBlockCommand`, `SetRTInfoCommand`, `EventSub`). The 16-pane cap and the `targetaction` allowlist are UI hygiene, not security controls.
- `wsh` workspace-scope (`-b ws`) and tab-routed commands (e.g. `wsh focusblock`, which routes to `tab:<TABID>`; the builder registers route `builder:<id>`) from a builder pane.
- Honouring a requested cwd for terminals created in the builder (e.g. File Browser "Open Terminal Here"): builder terminals always start in the app folder.
- The app-folder cwd is a convenience, not a security boundary: a pane shell can `cd` anywhere.
- Preview device emulation (sub-project 2, separate spec): built-in device profiles, optional user-downloaded WebKit/Gecko engines.

## Trust model

- Renderers and the Electron main process hold the authkey and are trusted: any of them can already delete any tab or block through existing RPCs (`pkg/web/ws.go:232`, `/wave/service`). Builder invariants are not defended against a compromised renderer.
- Pane shells are not given the authkey (`pkg/authkey/authkey.go:34`); `wsh` reaches the server over wshrpc with a block-scoped JWT, as an unrestricted RPC client. The adversary for this design is a pane acting through its issued JWT and `wsh` (e.g. an agent steered into running `wsh` commands). The goal: such a pane cannot, through *builder-specific* mechanisms, cause silent deletion of tabs it does not own, kill sibling panes, or keep a builder tab alive past teardown.
- Out of scope: same-uid OS capabilities. A pane runs as the user and can read the DB (including the JWT signing key, `pkg/remotetermobj/wtype.go:305`), `/proc/<pid>/environ` of the server, or `kill` processes; any of these defeats every builder invariant. D2's caller check is defence in depth and accident prevention against that class, not a boundary.
- The Preview `<webview>` gets no authkey, no preload, and cannot reach builder IPC (`emain/authkey.ts:12-26`, `emain/emain-websecurity.ts:88`, `emain/emain.ts:149-155`). D6 adds a dedicated preview partition.
- `EnsureBuilderTabCommand`, `OpenBuilderTerminalCommand` and `DeleteBuilderCommand` check the RPC source (`wshutil.GetRpcSourceFromContext`). Leaf links (pane `wsh` JWTs) have `Source` stamped by the router (`pkg/wshutil/wshrouter.go:566-568`, `proc:<uuid>`); websocket links (Electron, renderers) and router-kind JWT links (minted only for remote connservers, `pkg/remote/conncontroller/conncontroller.go:942-946`) are trusted router links whose source is self-asserted; a same-uid process on a remote host that can read a connserver token is in the out-of-scope same-uid class. So a pane using `wsh` cannot call them (D2). The other builder RPCs (`StartBuilderCommand`, `RequestBuilderRebuildCommand`, `StopBuilderCommand`, `WatchBuilderAppCommand`) take any builderId with no caller check; pre-existing, recorded not fixed.
- Pre-existing, recorded not fixed: pane JWTs live 365 days and are not revoked on block delete (`pkg/remotetermjwt/wavejwt.go:132-134`); `REMOTETERM_JWT` is in the pane env; `SetRTInfoCommand` accepts any oref (builder-by-hand R5). This spec stops trusting rtinfo for the builder tab's lifecycle and pane cwd. Other builder features still follow rtinfo `builder:appid` (build controller, Open folder, the renderer's app id on reload); a pane rewriting it can desync them from the terminals, which D5 detects and shows as an error.

## Design

### D1. Builder tab (backend)

- **Create:** `rtcore.CreateBuilderTab(ctx, builderId, appId)` inserts, in one transaction, a `Tab` (`Name: "builder"`, `BlockIds: []`, new `LayoutState`, `Meta{"builder:owner": builderId, "builder:appid": appId}`) and its `LayoutState`. It joins no workspace. The meta is written at insert.
- **Is-builder-tab predicate** `rtstore.IsBuilderTab(ctx, tabId)` (every delete path, the cascade skip, the local-only guard): `Meta["builder:owner"]` non-empty AND `DBFindWorkspaceForTabId(tab)` returns `""` with nil error. A lookup error counts as "not a builder tab" (fail safe: refuse to delete). It lives in `rtstore` (no import cycle; `rtcore` reuses it). Inside a transaction it is called with `tx.Context()`: the DB allows one open connection (`wstore_dbsetup.go:55`), so an outer context would deadlock until timeout.
- **Delete:** `rtcore.DeleteBuilderTab(ctx, tabId, expectedOwner)`:
  - Missing tab: no-op, nil error.
  - Refuses (error, deletes nothing) unless the predicate holds and, when `expectedOwner != ""`, `builder:owner == expectedOwner`.
  - Deletes each block with `DeleteBlock(ctx, id, false)` (publishes `Event_BlockClose`; controllers are destroyed asynchronously), logging and continuing past per-block errors. If every block was deleted, deletes the `LayoutState`, then the `Tab`. If any block delete failed, keeps the tab and layout state (so the sweep or a later delete retries) and returns the first error.
- **Find:** `rtcore.FindBuilderTabs(ctx, builderId)` selects tabs by `json_extract(data, '$.meta."builder:owner"')` in SQL (`builderId == ""` means any non-empty owner), then filters by the predicate.
- **Cascade skip:** in `DeleteBlock(recursive=true)`, when the parent tab satisfies the predicate, skip the "last block -> delete tab" cascade. Today that path calls `DeleteTab("")`, errors, and returns *before* `sendBlockCloseEvent`, leaking the shell (`pkg/rtcore/block.go:142-156`). With the skip, `DeleteBlock` returns nil and publishes BlockClose. Independently, `sendBlockCloseEvent` moves to immediately after `deleteBlockObj` succeeds, so no later cascade error (any tab) can skip it.
- **Reserved meta:** `rtstore.UpdateObjectMeta` (`pkg/rtstore/wstore.go:57`) rejects any key with prefix `builder:` (covers `builder:*` section-clears and nil deletes) on `tab:` orefs, so `SetMetaCommand` and `ObjectService.UpdateObjectMeta` are both covered. Only tab meta carries builder keys; block meta is not guarded. `CreateBuilderTab` writes at insert and does not go through it.
- **Workspace membership:** `UpdateWorkspaceTabIds` (`pkg/rtcore/workspace.go:453-461`) rejects a tab id whose tab has non-empty `builder:owner`, and rejects on a tab lookup error (fail closed).
- **Local only:** `rtstore.UpdateObjectMeta` (block orefs) and `rtcore.CreateBlock` / `CreateSubBlock` reject a `connection` value other than `""` / `"local"` when the block's tab (resolved through parent blocks, as `DBFindTabForBlockId`; for `CreateSubBlock` from the parent block id) satisfies the predicate. The check runs only when the incoming patch or block def contains a `connection` key, so ordinary meta writes do no lookups. This covers `SetMetaCommand`, the conn picker, `wsh ssh`/`wsh wsl`, `CreateBlockCommand`, `CreateSubBlockCommand` and `ObjectService`. It is a UX guard (unroutable SSH prompts), not a security boundary.
- **Workspace env:** `makeSwapToken` omits `WORKSPACEID` when the workspace id is empty (`pkg/blockcontroller/blockcontroller.go:618-630`).
- **Block def:** `MakeBuilderTerminalBlockDef` (`pkg/buildercontroller/appdir.go:38-47`) also pins the durable-shell meta key to false (exact key confirmed in the plan).

### D2. Serialisation and RPCs

- **Caller check:** a pure function `checkBuilderCaller(source, builderId, allowRenderer bool) error`. `EnsureBuilderTabCommand` and `OpenBuilderTerminalCommand` accept only `wshutil.ElectronRoute` (`"electron"`, `pkg/wshutil/wshrouter.go:30`; set by `emain/emain-wsh.ts:15`). `DeleteBuilderCommand` also accepts `wshutil.MakeBuilderRouteId(builderid)` (`wshrouter.go:139`; the renderer's `switchBuilderApp` calls it via `TabRpcClient`). Everything else (`proc:`, `controller:`, `conn:`, `tab:`, `feblock:`, other builders, empty) is rejected. `builderid` must parse as a UUID. A test-only exported helper in `wshutil` builds a context with a given source, so the handlers are testable.
- **Lock:** a keyed mutex per builderId (map guarded by its own mutex; helper funcs with `Lock(); defer Unlock()`), held by Ensure, Open and the tab teardown in `DeleteBuilderCommand`. Entries are never removed (one per builder window per process lifetime; the caller check stops panes minting ids). After acquiring, Ensure and Open return `ctx.Err()` if their RPC context is done.
- **Write context and broadcast:** after validation, the write phase of Ensure and Open, and the teardown in `DeleteBuilderCommand`, run on a detached, time-bounded context (`context.WithTimeout(context.Background(), 15s)`) wrapped with `remotetermobj.ContextWithUpdates`, on a single goroutine (the update map is not goroutine-safe). Rollbacks use the same context, so an expiring RPC context cannot strand a half-created tab. Each calls `wps.Broker.SendUpdateEvents(remotetermobj.ContextGetUpdatesRtn(ctx))` once before returning, on success and after rollback (failed transactions contribute no updates, `pkg/rtstore/wstore_dbsetup.go:58-80`). Without it the renderer never sees new panes' layout actions or the tab's deletion. Open calls `rtcore.CreateBlock` and `QueueLayoutActionForTab` directly rather than `CreateBlockCommand` (which broadcasts on success only and cannot roll back). The startup sweep is exempt (no clients yet).
- **App dir:** new `buildercontroller.ResolveAppDirForAppId(appId)`: `GetAppDir` (validates the id and bounds the dir to the apps root, `pkg/remotetermappstore/waveappstore.go:79-86`) plus the symlink and `Lstat` checks of `ResolveBuilderAppDir`. It never reads rtinfo. `ResolveBuilderAppDir` keeps serving its existing callers.
- **`EnsureBuilderTabCommand{builderid, appid} -> {tabid, appid}`** (under the lock). Electron IPC fills both fields from the calling window (`bw.builderId`, `bw.builderAppId`):
  - `appid` empty or invalid -> error.
  - `FindBuilderTabs(builderid)`: if one has `builder:appid == appid`, return it. Others (stale app) are deleted with `DeleteBuilderTab(…, builderid)` first.
  - `ResolveAppDirForAppId(appid)`; failure -> error, nothing created.
  - `CreateBuilderTab`, then one block from `MakeBuilderTerminalBlockDef(appDir)` with an `insert` layout action (`focused: true`). Any failure after the tab insert deletes the tab before returning the error.
- **`OpenBuilderTerminalCommand{builderid, targetblockid?, targetaction?}`** replaces `{builderid, tabid}` (under the lock). Validate fully, then write:
  - `targetaction` in `""`, `splitright`, `splitleft`, `splitup`, `splitdown` (the strings `CreateBlockCommand` uses, `pkg/wshrpc/wshserver/wshserver.go:226-286`); anything else, including `replace`, rejected.
  - Tab: exactly one `FindBuilderTabs(builderid)` result, else error "builder terminal not ready". Open never creates or deletes tabs.
  - App dir: `ResolveAppDirForAppId` of the tab's own `builder:appid` (server-written), never rtinfo.
  - When `targetaction != ""`: `targetblockid` required and must be in `tab.BlockIds` (direct child).
  - Cap: reject with "too many terminals in this builder (max 16)" when the tab has 16 blocks with `view: "term"`.
  - Create the block, queue the layout action (`insert`, or the split on `targetblockid`) with `focused: true`; if queueing fails, delete the new block and return the error.
- **`DeleteBuilderCommand`**: the tab teardown runs on a detached context (`context.WithTimeout(context.Background(), 15s)`), not the RPC context, under the lock: `DeleteBuilderTab(t, builderid)` for each `FindBuilderTabs(builderid)`. It keeps deleting the builder controller as today. No tombstone: `switchBuilderApp` reuses the builderId after calling it.

### D3. Startup sweep

In `cmd/server/main-server.go`, after `InitJobController` and `InitBlockController` (so BlockClose reaches their handlers) and before `go StartupReconnectDurableShells` (lines 308-310), and therefore before the websocket, web and unix RPC listeners (lines 323-346): `DeleteBuilderTab(ctx, t, "")` for each `FindBuilderTabs(ctx, "")`. Per-tab errors are logged and do not stop startup. Logs `[startup] builder sweep: removed N tabs`. Builder ids are fresh UUIDs per window (`emain/emain-builder.ts:37`), so every such tab is an orphan; a backend exit quits the app (`emain/emain-remotetermsrv.ts:79-83`), so no live window owns one.

### D4. Electron main process

- New IPC `ensure-builder-tab` -> `EnsureBuilderTabCommand{builderid: bw.builderId, appid: bw.builderAppId}`; returns `{tabid, appid}` or `{error}`. The window is in `builderWindows` with `builderAppId` set before `builder-init` is sent (`emain/emain-builder.ts:84-87`, `emain/emain-ipc.ts:492-497`); the plan confirms the panel cannot call the IPC earlier.
- `switchBuilderApp` awaits `setBuilderWindowAppId(null)` before reloading (`builder-apppanel-model.ts:315-317`).
- `open-builder-terminal` keeps deriving `builderId` from the calling window, stops picking or focusing a main window, and forwards `targetblockid` / `targetaction` after checking each is a string of at most 64 chars (else error).
- `destroyBuilderWindow` (`emain/emain-ipc.ts:226`): awaits `DeleteBuilderCommand` in try/catch (continuing on error), then deletes builder rtinfo, then destroys the window. The `closed` handler's no-response `DeleteBuilderCommand` stays as an idempotent fallback.
- App switch has one path, `switchBuilderApp` (`frontend/builder/store/builder-apppanel-model.ts:308-324`: `DeleteBuilderCommand`, clear app id, reload). `app-selection-modal.tsx` only runs in a renderer with no app selected and needs no change.

### D5. Renderer init and terminal panel

- **Subscriptions:** `initBuilder` adds `waveobj:update`, `config`, `blockfile`, the badges subscription, and `subscribeToConnEvents()` (`frontend/app/store/global.ts:55-96`, `:753-790`). No `userinput` (local-only panes).
- **`staticTabIdAtom`** becomes a `PrimitiveAtom<string>`; only the builder bootstrap writes it in production code. `uiContext` reads `staticTabIdAtom` at call time instead of the init-time closure (`frontend/app/store/global-atoms.ts:19-25`); unchanged for main windows.
- **Bootstrap** (`BuilderTermPanel`, once the window has an app id):
  1. `ensure-builder-tab` IPC -> `{tabid, appid}`. Error: message + Retry; app column unaffected. If the returned `appid` differs from `atoms.builderAppId` (rtinfo was rewritten), show "Terminal app and builder app differ; reopen the builder" instead of mounting.
  2. `loadAndPinWaveObject` the tab, then load and pin its `LayoutState`.
  3. If `staticTabId` is unset, set it. If it is set to a different id (cannot happen without an app switch, which reloads), reload the renderer.
  4. Mount the tile layout for that tab inside `TabModelContext`, with `TileLayoutContents.onNodeDelete = ObjectService.DeleteBlock(blockId)` (as `tabcontent.tsx:38-46`; the D1 cascade skip keeps the tab).
  No code obtains a layout model for the builder tab before step 3. Callers of `getLayoutModelForStaticTab()` reachable in builder windows (`frontend/app/store/keymodel.ts:67-71, 154, 175, 191, 217, 295-315`, `frontend/app/store/focusManager.ts:16-20, 35`) return early on a null model.
- **Tab vanishes while mounted** (tab atom becomes null): unmount the tile layout, `deleteLayoutModelForTab`, show the Retry state; Retry reloads the renderer. `switchBuilderApp` sets a "switching" flag before `DeleteBuilderCommand` that shows a neutral "Switching app…" state instead of Retry.
- **Layout:** `builder-workspace.tsx` restores the horizontal `PanelGroup`: left `BuilderTermPanel` (`layout.terminal`, default 40, min 20), resize handle, right column (existing app/build vertical group). A saved layout without `terminal` uses 40.
- **Empty state:** when the layout has no visible leaves, show "No terminals" and an "Open terminal" button.
- **Header button:** the app panel's "Open terminal" is disabled until bootstrap step 1 has succeeded.
- **Reload:** a renderer reload (same builderId and app) re-runs the bootstrap; Ensure returns the existing tab; panes and shells survive.
- **Preview partition:** the Preview `<webview>` (`frontend/builder/tabs/builder-previewtab.tsx:215-224`) gets an explicit in-memory partition `builder-preview-<builderId>`, so web blocks in the builder tab (default `persist:webblock`) and other builders' previews do not share its cookies or storage. `hardenWebviewAttach` keeps explicit partitions (`emain/emain-websecurity.ts:92-94`); permission handlers reach every session (`emain/emain.ts:147`). Preview storage no longer survives an app restart.

### D6. Focus, keys, menus, block creation

- **Focus:** `BuilderFocusType` becomes `"app" | "terminal"`. Initial focus is `"terminal"` once the first pane mounts, else `"app"`. Focus-capture/click inside the terminal panel sets `"terminal"`; the app-panel handlers set `"app"`. Reaching zero panes sets `"app"`. The focused side gets the accent border.
- **Keys** (builder windows; "as tab" = the tab-window handler in `keymodel.ts` run against the builder tab's layout model):

  | Binding (keymodel names) | Focus terminal | Focus app |
  |---|---|---|
  | `Cmd:w` | close focused pane (close rule) | close builder window (existing) |
  | new pane (`Cmd:n`) | Open terminal, append | none |
  | split right / down (`Cmd:d`, `Shift:Cmd:d`), split chord (`Ctrl:Shift:s` + arrow) | Open terminal, matching `targetaction` on focused block | none |
  | focus moves (`Ctrl:Shift:Arrow*`), magnify (`Cmd:m`), block number (`Ctrl:Shift:c{Digit}`) | as tab | none |
  | term search (`Cmd:f`, `Escape`) | as tab (block-level handler) | none |
  | `Cmd:t`, `Shift:Cmd:w`, `Cmd:[`, `Cmd:]`, `Cmd:1-9`, `F2`, `Ctrl:Shift:i`, `Ctrl:Shift:x`, `Cmd:g` | not bound | not bound |

  Exact key strings come from `keymodel.ts` in the plan. Preview webview key interception (`emain/emain-ipc.ts:64, 393-395`) receives only `Cmd:w`: terminal-side bindings are not added to the list `registerBuilderGlobalKeys` forwards to `registerGlobalWebviewKeys` (`keymodel.ts:699-706`).
- **Close rule:** in builder windows, `uxCloseBlock` / `genericClose` always close through the layout model's `closeNode` (which calls `onNodeDelete`), including the last block; they never call `simpleCloseStaticTab` / `closeTab` (`keymodel.ts:130-176`). Keep-alive blocks in a builder tab are closed, never hidden (`global.ts:627-645`).
- **Block creation, central intercept** in `createBlock`, `createBlockSplitHorizontally`, `createBlockSplitVertically`, `replaceBlock` (`frontend/app/store/global.ts:377-461`), when `isBuilderWindow()` (`frontend/app/store/windowtype.ts:11`):
  - block def with `view: "term"` -> `open-builder-terminal` (append, or the matching split on the target block); the def's meta (cwd, controller, connection) is ignored. These functions return the new block id today; in the builder they return `null` and show any error as a notice (callers checked in the plan).
  - `replaceBlock` with a term def -> no-op.
  - any other view (Open URL, File Browser, internal links, stickers) -> existing `ObjectService` path into the builder tab.
  Blocks created by `wsh` from a pane land in the builder tab via `CreateBlockCommand`; `wsh term` blocks count toward the cap.
- **Split focus and stale targets:** the frontend's backend-action split handlers honour `action.focused` (`frontend/layout/lib/layoutModel.ts:513-569`), as `insert` does; if the target node no longer exists, they insert instead of dropping the action.
- **Connections:** in builder tabs the block header connection button and `Cmd:g` are hidden/unbound.

### D7. Failure handling

- Ensure fails: panel error + Retry; app column unaffected.
- Open fails (not ready, cap, bad target, backend error): error string shown as a notice; no rows remain.
- Shell exits: pane behaves as in a main window.
- Switch app or close window with running panes: shells killed without confirmation (same as closing a main-window tab).
- `DeleteBuilderCommand` never completes (crash, quit ordering, IPC error): D3 sweep on next start.
- A late Ensure arriving after window teardown can create a tab no window shows; no shell starts (shells start when a pane mounts). The sweep removes it next start. Accepted.

## Testing

- **Go unit** (`pkg/rtcore`, `pkg/rtstore`, `pkg/wshrpc/wshserver`, `pkg/blockcontroller`, `pkg/buildercontroller`, `cmd/server` sweep helper):
  - `CreateBuilderTab`: tab + layout state, no workspace, both meta keys.
  - `DeleteBuilderTab`: deletes blocks, layout state, tab; BlockClose per block; refuses a workspace tab carrying `builder:owner`; refuses owner mismatch; idempotent; a failing block leaves the tab present and a later sweep removes it.
  - Caller check: Ensure and Open accept the Electron source only; Delete accepts Electron and `builder:<same id>`; `proc:`/`controller:`/`tab:`/other-builder sources rejected with zero rows changed.
  - Broadcast: Ensure, Open and Delete teardown each publish `waveobj:update` for the LayoutState / tab changes they make (including the tab delete).
  - `FindBuilderTabs`: by owner and any-owner; excludes workspace tabs.
  - `DeleteBlock(recursive=true)` on the last block of a builder tab: nil, BlockClose published, tab remains.
  - `UpdateObjectMeta` rejects `builder:owner`, `builder:*`, nil `builder:owner` on tab orefs, via both `SetMetaCommand` and `ObjectService.UpdateObjectMeta`; other keys unaffected.
  - `DeleteBlock`: BlockClose published even when a later cascade step errors.
  - `UpdateWorkspaceTabIds` rejects a builder tab id.
  - Non-local `connection` rejected for builder-tab blocks and their sub-blocks via `SetMetaCommand`, `CreateBlockCommand`, `CreateSubBlockCommand`; accepted for normal tabs.
  - Ensure: idempotent; concurrent Ensure creates one tab; invalid app id or bad app dir creates nothing; app dir from the `appid` argument even when rtinfo `builder:appid` differs; first call creates exactly one terminal block with `cmd:cwd` = app dir and durable off; different appid replaces the tab; failure after tab insert leaves no tab.
  - Open: appends; split on a direct-child target; sub-block target, foreign target, missing target and `replace` rejected with zero new rows; 17th term block rejected; no tab -> "not ready", zero rows; queue failure removes the new block; cwd from tab meta even when rtinfo appid differs.
  - `DeleteBuilderCommand`: removes all owner tabs regardless of rtinfo; completes when the caller's RPC context is cancelled early.
  - Keyed lock: Delete interleaved with two waiting Ensures under `-race` yields one tab.
  - Sweep: removes builder tabs only; workspace tab with spoofed `builder:owner` survives; logs the count; helper runs between controller init and durable reconnect.
  - `makeSwapToken`: no `WORKSPACEID` for a builder pane.
- **Vitest:** layout default/merge without `terminal`; focus switching including zero-pane -> app; key table routing by focus (including `Cmd:w` both ways, unbound keys, webview key list contains only `Cmd:w`); last-pane close calls `closeNode` not `closeTab`; `onNodeDelete` wired to `ObjectService.DeleteBlock`; create intercept (term -> IPC with action, non-term -> ObjectService, `replaceBlock` term no-op); bootstrap order (pin before `staticTabId`, no layout model before step 3); appid mismatch error state; header button disabled before Ensure; tab-vanished state driven by a delete update; split handlers honour `focused` and fall back to insert; main windows never write `staticTabIdAtom`.
- `tsc --noEmit`; `go test ./pkg/...`; tsunami package list; full vitest; `go test -race` on touched packages.
- **E2E, isolated only:** nested Xvfb with `DISPLAY` unset for everything not launched on it, scratch HOME and XDG dirs, `REMOTETERM_CONFIG_HOME`, `REMOTETERM_DATA_HOME`, `REMOTETERM_ISOLATED_PROFILE=1`, scratch `GOCACHE`/`GOMODCACHE`, private Vite `cacheDir`; never the live display or the user's data dirs. Keys sent are the Linux bindings (Alt for `Cmd:`). Checks: S1; S2; S3 (including two rapid splits); S4 (Alt+W closes pane with PID and row gone; last pane -> empty state, window open; Ctrl+W in a shell deletes a word); S9 with a barrier (both panes run `while [ ! -e $T/go ]; do :; done; echo >> app.go`, then the test creates `$T/go`); S6 via title-bar close, via Alt+W from the app side, and via `switchBuilderApp`; S7 (kill server mid-session, restart, log line before `WAVESRV-ESTART`, rows gone); S8; S10 (workspace tab with `builder:owner` injected by direct DB write survives restart).

## Risks

- Layout save vs. backend queue race (`persistToBackend` rewriting `pendingbackendactions`, `layoutModel.ts:581-598`): unconfirmed. The S3 rapid-split E2E check detects it; if it reproduces, the plan adds a merge-by-action-id fix.
- `focusedNodeId` is not cleared when the root node is deleted (`layoutTree.ts:363-364`); the plan checks `getFocusedBlockInStaticTab` against an empty tree.
- Bell badges on builder panes are never auto-cleared (no `BadgeAutoClearing` in the builder tree). Cosmetic; accepted.

## Audit disposition: round 1 (v1)

| Finding | Disposition | Where |
|---|---|---|
| Req B1 last-pane close routed to `closeTab` | Accepted | D6, S4 |
| Req B2 layout model built before `staticTabId`/LayoutState | Accepted (reordered again in round 2) | D5 |
| Req M1 `uiContext` closure; create family | Accepted | D5, D6 |
| Req M2 Cmd is Alt on Linux | Accepted | header, S4, E2E |
| Req M3 SSH prompts unroutable | Accepted: builder panes local only | D1, D6 |
| Req M4 subscription inventory | Accepted | D5 |
| Req M5 key map scope; nonexistent menu items | Accepted | D6 |
| Req m1 split focus | Accepted | D6, S3 |
| Req m2 webview keys global | Accepted | D6 |
| Req m3 reload | Accepted | S1, D5 |
| Req m4 Open creating tab -> two panes | Superseded: Open never creates tabs | D2 |
| Req m5 keep-alive hidden blocks | Accepted | D5, D6 |
| Req m6 / Sec M4 sweep ordering | Accepted | D3 |
| Req m7 cascade test | Accepted | D1 |
| Req m8 observables | Accepted | S3, S6, S9 |
| Req m9 queue race | Risk + E2E detector | Risks |
| Sec H1 spoofable `builder:owner` | Accepted; guard moved to `UpdateObjectMeta` in round 2 | D1, S10 |
| Sec H2 rtinfo deleted before teardown; app switch | Accepted | D1, D2, D4 |
| Sec M1 cross-builder targeting | Accepted | D1, D2 |
| Sec M2 validate-before-write, `replace` | Accepted | D2 |
| Sec M3 Ensure/Open/Delete races | Accepted (lock entries kept, round 2) | D2, D7 |
| Sec L1 pane flood | Accepted as UI hygiene | D2, non-goals |
| Sec L2 async kill | Accepted | S6 |
| Sec L3 cwd not a boundary | Accepted | non-goals |
| Sec L4 pre-existing weaknesses | Recorded | trust model |
| Sec L5 empty `WORKSPACEID` | Accepted | D1 |

## Audit disposition: round 2 (v2)

| Finding | Disposition | Where |
|---|---|---|
| Req M1 / Sec M-B lock entry removal race | Accepted: entries never removed | D2 |
| Req M2 layout model built before LayoutState loaded | Accepted: pin tab + LayoutState before setting `staticTabId` | D5 |
| Req M3 / Sec L-B rtinfo appid drives deletion | Accepted: appid from Electron window state; Open never deletes; cwd from tab meta | D2, D4 |
| Req M4 modal is not a switch path | Accepted | D4 |
| Req M5 `onNodeDelete` not wired | Accepted; PID + row check added to S4 | D5, S4 |
| Req M6 local-only and cap enforced only on Open; cap contradiction | Accepted: server rejects non-local connection; cap = term blocks | D1, D2, D6 |
| Req m1 / Sec L-D sub-block targets; Open rollback | Accepted: direct child; rollback; insert fallback | D2, D6 |
| Req m2 webview key list | Accepted: only `Cmd:w` | D6 |
| Req m3 zero panes + `Cmd:w`; initial focus | Accepted | D6, S4 |
| Req m4 intercept drops cwd; return value | Accepted: documented non-goal; returns null | Non-goals, D6 |
| Req m5 tab-routed wsh | Non-goal | Non-goals |
| Req m6 / Sec L-C workspace TabIds; other meta writers | Accepted | D1 |
| Req m7 tab vanishes while mounted | Accepted | D5 |
| Req m8 listener wording | Accepted | D3 |
| Req m9 testability | Accepted: log line, debounce value, switch path | S6, S7, S9 |
| Sec M-A guard bypass via ObjectService | Accepted for `UpdateObjectMeta`; `UpdateObject` left: renderers trusted (trust model) | D1, trust model |
| Sec M-C teardown on RPC ctx | Accepted: detached ctx, best effort, try/catch in main | D1, D2, D4 |
| Sec M-D shared web partition | Accepted: preview gets `builder-preview` partition | D5 |
| Sec L-A generic RPC cross-builder | Non-goal, stated | Non-goals |
| Sec L-E cap inconsistency | Accepted | D2, D6 |
| Sec L-F `PrimitiveAtom`, reload loop | Accepted; reload cannot loop (fresh renderer, lock fix) | D5 |
| Sec L-G durable default | Accepted: pinned false | D1 |
| Sec L-H IPC arg typing | Accepted | D4 |

## Audit disposition: round 3 (v3)

| Finding | Disposition | Where |
|---|---|---|
| Req M2 / Sec M1 builder RPCs callable by panes | Accepted: RPC source allowlist | D2, trust model |
| Req M1 no update broadcast from Ensure/Open/teardown | Accepted | D2, tests |
| Sec M2 tab deleted after failed block delete | Accepted: keep tab on any block failure | D1 |
| Sec L1 Ensure app dir via rtinfo-reading resolver | Accepted: `ResolveAppDirForAppId`; rtinfo desync recorded | D2, trust model |
| Sec L2 / Req m3 local-only gaps (sub-blocks, other writers) | Accepted: check in `UpdateObjectMeta` and `CreateBlock`/`CreateSubBlock` | D1 |
| Sec L3 / Req m4 shared preview partition | Accepted: per-builder partition | D5 |
| Sec L4 block-oref meta guard has no consumer | Accepted: tab orefs only | D1 |
| Sec L5 BlockClose skipped on cascade error | Accepted: publish right after delete | D1 |
| Sec L6 lock not context-aware | Accepted: `ctx.Err()` after acquire | D2 |
| Sec I1 unawaited `setBuilderWindowAppId`; IPC timing | Accepted | D4 |
| Req m1 renderer vs window app id | Accepted: Ensure returns appid; mismatch error | D2, D5 |
| Req m2 header Open before Ensure | Accepted: disabled until step 1 | D5 |
| Req m5 dead `builder:tabid` rtinfo | Accepted: dropped | D2 |
| Req m6 S6 close paths; S9 timing | Accepted | Testing |

## Audit disposition: round 4 (v4)

| Finding | Disposition | Where |
|---|---|---|
| Major 1 source stamping wording; same-uid adversary | Accepted: wording corrected; adversary narrowed; same-uid out of scope | Trust model |
| Minor 1 Ensure rollback on RPC ctx | Accepted: detached write context | D2 |
| Minor 2 guard placement, tx context, patch-only check | Accepted | D1 |
| Minor 3 broadcast citation; Open via `CreateBlockCommand` | Accepted: Open uses `rtcore.CreateBlock` + `QueueLayoutActionForTab` | D2 |
| Minor 4 caller-check test seam | Accepted: pure function + test helper | D2 |
| Minor 5 other builder RPCs unchecked | Recorded as pre-existing | Trust model |
| Minor 6 Retry flash on app switch | Accepted: switching flag | D5 |

Verdict after round 4: no blockers; the single major was wording. Spec ready to plan.
