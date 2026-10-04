# Builder terminal panel: design spec (v2)

Date: 2026-10-04. Branch: `feat/builder-terminal` off local `main` 0fee7150 (builder-by-hand).
v2 folds in the requirements audit (B1-B2, M1-M5, m1-m9) and security review (H1-H2, M1-M4, L1-L5) of v1; disposition table at the end. In this spec `Cmd:` is the keymodel notation: Cmd on macOS, Alt on Linux/Windows (`frontend/util/keyutil.ts:78-84`).

## Problem

The Tsunami app builder window was laid out as "build on the left, preview on the right": a Wave AI chat panel on the left, the app panel (Preview / Code / Config/Data / Files) and build output on the right. This fork removed the chat (`1a7f1c83`), and builder-by-hand removed the placeholder and the horizontal split (`872f5fd0`). The builder is now a single column, and the only way to drive an app with an agent harness is "Open terminal", which puts a shell in a *main* window, away from the preview.

## Goal

The left side of the builder window becomes a tiled terminal area, so one or more agent harnesses (Claude Code, Codex, Hermes, a plain shell) can work on the app side by side with its live preview. It behaves like a tab in the main window: split, resize, magnify, focus, close.

## Success criteria (each has a named observable)

- S1. First open of an app in a builder window shows a terminal panel on the left with exactly one running local shell; `pwd` in it prints the app dir. (A renderer reload keeps existing panes; S1 is about first open.)
- S2. "Open terminal" (app panel header, and the empty-state button) adds one new shell pane to the builder's terminal panel, cwd = app folder, and focuses it. It never touches main windows.
- S3. New-pane and split keybindings, header split buttons and context-menu splits, with a builder pane as source, add a local shell pane with cwd = app folder, focused. (Deviation from main windows, where a split copies the source pane's connection: builder splits are always local app-folder shells.)
- S4. With the terminal side focused, `Cmd:w` closes the focused pane, including the last one; with the app side focused, it closes the builder window. After the last pane closes, the panel shows an empty state with an "Open terminal" button and the window stays open. Ctrl+W is never bound in the builder (it stays readline word-delete).
- S5. Terminal panel width is resizable and persists for the window's lifetime via `builder:layout` key `terminal`.
- S6. Closing the builder window, or switching it to another app, deletes its builder tab, blocks and layout state from the DB, and every pane's shell PID (captured with `echo $$`) is gone within 5 s. Processes an agent detached with `setsid`/`nohup` are out of scope.
- S7. After a backend crash or quit that skipped S6 cleanup, the next server start deletes every leftover builder tab and its blocks before any client connects.
- S8. Builder tabs never appear in any workspace's `tabids`, tab bar, or the workspace switcher.
- S9. Writes from two panes within one rebuild debounce window, with live rebuild on, produce exactly one `building` transition.
- S10. A tab that belongs to a workspace is never deleted by builder cleanup or the startup sweep, whatever its meta says.

## Non-goals

- Auto-launching a harness command in new panes (plain shell; the user starts the harness).
- Persisting panes or shells across window close (decided with the user: shells die with the window).
- Arbitrating concurrent edits to the same file by different agents.
- Remote connections in builder panes (see D6: the connection switcher is hidden; a connection forced in via `wsh setmeta` is unsupported).
- `wsh` workspace-scope (`-b ws`) from a builder pane; it fails as for any workspace-less context.
- The app-folder cwd is a convenience, not a security boundary: a pane shell can `cd` anywhere.
- Preview device emulation (sub-project 2, separate spec): built-in device profiles, optional user-downloaded WebKit/Gecko engines.

## Trust model

- Any websocket client holding the authkey is a trusted router with unrestricted RPC (`pkg/web/ws.go:232`); `builderid` in an RPC is an argument, not a principal. So every builder invariant below is enforced server-side, not by "who called".
- The Preview `<webview>` gets no authkey, no preload, and cannot reach builder IPC (`emain/authkey.ts:12-26`, `emain/emain-websecurity.ts:88`, `emain/emain.ts:149-155`). Unchanged.
- A pane shell is an unrestricted RPC client (as in main windows). The realistic adversary for the invariants is a prompt-injected agent in a pane; the goal is that it cannot cause silent cross-tab damage through builder-specific mechanisms.
- Pre-existing, recorded not fixed: pane JWTs live 365 days and are not revoked on block delete (`pkg/remotetermjwt/wavejwt.go:132-134`); `REMOTETERM_JWT` is in the pane env; `SetRTInfoCommand` accepts any oref (builder-by-hand R5).

## Design

### D1. Builder tab (backend)

- **Create:** `rtcore.CreateBuilderTab(ctx, builderId, appId)` inserts, in one transaction, a `Tab` (`Name: "builder"`, `BlockIds: []`, new `LayoutState`, `Meta{"builder:owner": builderId, "builder:appid": appId}`) and its `LayoutState`. It joins no workspace. The meta is written at insert, not via `SetMetaCommand`.
- **Is-builder-tab predicate** (used by every delete path and the cascade skip): `Meta["builder:owner"]` non-empty AND the tab is in no workspace's `TabIds` (`DBFindWorkspaceForTabId` returns `""`).
- **Delete:** `rtcore.DeleteBuilderTab(ctx, tabId, expectedOwner)`:
  - Missing tab: no-op, nil error.
  - Refuses (error, deletes nothing) unless the predicate holds and, when `expectedOwner != ""`, `builder:owner == expectedOwner`.
  - Deletes each block with `DeleteBlock(ctx, id, false)` (publishes `Event_BlockClose`; controllers destroyed asynchronously), then the `LayoutState`, then the `Tab`.
- **Find by owner:** `rtcore.FindBuilderTabs(ctx, builderId)` returns tab ids whose meta `builder:owner == builderId` and that satisfy the predicate (DB scan of tabs; builder tabs are few). Teardown uses this, never a pointer the renderer can set.
- **Cascade skip:** in `DeleteBlock(recursive=true)`, when the parent tab satisfies the predicate, skip the "last block -> delete tab" cascade. Today that path calls `DeleteTab("")`, errors, and returns *before* `sendBlockCloseEvent`, leaking the shell (`pkg/rtcore/block.go:142-156`). With the skip, `DeleteBlock` returns nil and publishes BlockClose.
- **Reserved meta:** `SetMetaCommand` rejects any key with prefix `builder:` (including the `builder:*` section-clear form and nil deletes) on `tab:` and `block:` orefs, with an error naming the key. Server code writes these keys directly.
- **Workspace env:** `makeSwapToken` omits `WORKSPACEID` when the workspace id is empty (`pkg/blockcontroller/blockcontroller.go:618-630`) instead of exporting `""`.

### D2. Per-builder serialisation and RPCs

- A keyed mutex per builderId (helper funcs with `Lock(); defer Unlock()`) is held by Ensure, Open and the tab teardown in `DeleteBuilderCommand`. The entry is removed at the end of `DeleteBuilderCommand`.
- **`EnsureBuilderTabCommand{builderid} -> {tabid}`** (under the lock):
  - Read builder rtinfo `builder:appid`; empty -> error "no app selected".
  - `FindBuilderTabs(builderid)`: if one exists whose `builder:appid` equals the current app id, return it. Any found with a different app id (app switched without teardown) are deleted with `DeleteBuilderTab(…, builderid)` first.
  - Resolve the app dir (`ResolveBuilderAppDir`, builder-by-hand D7); on failure return the error, create nothing.
  - `CreateBuilderTab`, then one block from `MakeBuilderTerminalBlockDef(appDir)` with a layout `insert` action queued (the existing `CreateBlockCommand` mechanism); on any failure after the tab insert, delete the tab before returning the error.
  - Write rtinfo `builder:tabid` as a hint for the renderer only; nothing server-side trusts it.
- **`OpenBuilderTerminalCommand{builderid, targetblockid?, targetaction?}`** replaces `{builderid, tabid}` (under the lock):
  - Validate first, write after. `targetaction` must be one of `""`, `splitright`, `splitleft`, `splitup`, `splitdown` (exact strings as used by `CreateBlockCommand`, `pkg/wshrpc/wshserver/wshserver.go:226-286`); anything else, including `replace`, is rejected. When `targetaction != ""`, `targetblockid` is required and `DBFindTabForBlockId(targetblockid)` must equal the builder tab; otherwise rejected. Rejections create no rows.
  - Tab: as Ensure (finds or creates). If this call created the tab, return after Ensure's first pane (no second pane).
  - Cap: if the tab already has 16 blocks, reject with "too many terminals in this builder (max 16)".
  - Create the block from `MakeBuilderTerminalBlockDef(appDir)` and queue the layout action (`insert` for `""`, otherwise the split action on `targetblockid`) with `focused: true`.
- **`DeleteBuilderCommand`** additionally (under the lock): `DeleteBuilderTab(t, builderid)` for every `FindBuilderTabs(builderid)`, clear rtinfo `builder:tabid`, drop the lock entry. It keeps deleting the builder controller as today. It does not tombstone: the app-switch path reuses the builderId after calling it (`frontend/builder/store/builder-apppanel-model.ts:308-324`).

### D3. Startup sweep

In `cmd/server/main-server.go`, after `InitJobController` and `InitBlockController` (so BlockClose reaches their handlers) and before `go StartupReconnectDurableShells` and before any listener starts (~lines 308-310): for every tab with non-empty `builder:owner` that satisfies the D1 predicate, `DeleteBuilderTab(ctx, tabId, "")`. Errors are logged per tab and do not stop startup. Builder ids are fresh UUIDs per window (`emain/emain-builder.ts:37`), so every such tab is an orphan.

### D4. Electron main process

- `open-builder-terminal` IPC keeps deriving `builderId` from the calling window (never from args), stops picking or focusing a main window, and forwards `{builderid, targetblockid, targetaction}` from the renderer. Returns the error string on failure as today.
- `destroyBuilderWindow` awaits `DeleteBuilderCommand` (with a short timeout) *before* deleting builder rtinfo and destroying the window. The `closed` handler's existing no-response `DeleteBuilderCommand` stays as the fallback; it is idempotent.
- `app-selection-modal.tsx` app switch: covered by Ensure's app-id check (old tab deleted on the next Ensure). The renderer reloads on switch as today; if the modal path does not, it calls `DeleteBuilderCommand` before setting the new app id and reloads, so S6 holds on both paths.

### D5. Renderer init and terminal panel

- **Subscriptions:** `initBuilder` adds `waveobj:update`, `config`, `blockfile`, the badges subscription, and `subscribeToConnEvents()` (`frontend/app/store/global.ts:55-96`, `:753-790`). It does not add `userinput` (local-only panes, D6).
- **Tab bootstrap order** (in `BuilderTermPanel`, once `builder:appid` is set):
  1. `EnsureBuilderTabCommand` -> tab id. Error: show message + Retry; app column unaffected.
  2. If `staticTabId` is unset, set it to the tab id. If it is set to a *different* id (the app changed under this renderer), reload the renderer instead of continuing. `staticTabId` is set at most once per renderer lifetime.
  3. `loadAndPinWaveObject` the tab, then load its `LayoutState`.
  4. Only now mount the tile layout for that tab inside `TabModelContext`.
  No code calls `getLayoutModelForTab` / `getLayoutModelForStaticTab` for the builder before step 2. Callers of `getLayoutModelForStaticTab()` reachable from builder keys and focus (`frontend/app/store/keymodel.ts:67-71, 217, 295-315`, `frontend/app/store/focusManager.ts:16-20`) tolerate a null model.
- **`uiContext`:** derived from `staticTabIdAtom` at call time instead of the init-time closure (`frontend/app/store/global-atoms.ts:19-25`), so `ObjectService` calls from the builder carry the builder tab id.
- **Layout:** `builder-workspace.tsx` restores the horizontal `PanelGroup`: left `BuilderTermPanel` (`layout.terminal`, default 40, min 20), resize handle, right column (existing app/build vertical group). A saved layout without `terminal` uses 40.
- **Empty state:** when the tab's layout has no visible leaves, show "No terminals" and an "Open terminal" button (same IPC as the header button).
- **Reload:** a renderer reload (same builderId) re-runs the bootstrap; Ensure returns the existing tab; panes and shells survive.

### D6. Focus, keys, menus, block creation

- **Focus:** `BuilderFocusType` becomes `"app" | "terminal"`. Focus-capture/click inside the terminal panel sets `"terminal"`; the existing app-panel handlers set `"app"`. The focused side gets the accent border.
- **Keys** (builder windows; routed only when focus is `"terminal"` unless stated; "as tab" = the tab-window binding in `keymodel.ts` runs against the builder tab's layout model):

  | Binding (keymodel names) | Focus terminal | Focus app |
  |---|---|---|
  | `Cmd:w` | close focused pane (D6 close rule) | close builder window (existing) |
  | new pane (`Cmd:n`) | Open terminal, append | none |
  | split right / down (`Cmd:d`, `Shift:Cmd:d`), split chord (`Ctrl:Shift:s` + arrow) | Open terminal with matching `targetaction` on focused block | none |
  | focus moves (`Ctrl:Shift:Arrow*`), magnify (`Cmd:m`), block number (`Ctrl:Shift:c{Digit}`) | as tab | none |
  | term search (`Cmd:f`, `Escape`) | as tab (block-level handler) | none |
  | `Cmd:t`, `Shift:Cmd:w`, `Cmd:[`, `Cmd:]`, `Cmd:1-9`, `F2`, `Ctrl:Shift:i`, `Ctrl:Shift:x`, `Cmd:g` | not bound | not bound |

  Exact key strings are taken from `keymodel.ts` in the plan; the table fixes behaviour. Preview webview key interception (`emain/emain-ipc.ts:64, 393-395`) is not extended.
- **Close rule:** in builder windows, `uxCloseBlock` / `genericClose` always close the node through the layout model (`closeNode` -> `DeleteBlockCommand`), including the last block; they never call `simpleCloseStaticTab` / `closeTab` (`keymodel.ts:130-176`). Keep-alive blocks in a builder tab are closed, never hidden (`global.ts:627-645`).
- **Block creation, central intercept:** in `createBlock`, `createBlockSplitHorizontally`, `createBlockSplitVertically`, `replaceBlock` (`frontend/app/store/global.ts:377-461`), when the window is a builder:
  - a block def with `view: "term"` -> `open-builder-terminal` (append or matching split on the target block); the source pane's meta is not copied;
  - `replaceBlock` with a term def -> rejected (no-op);
  - any other view (Open URL, File Browser, internal links, stickers) -> existing `ObjectService` path into the builder tab (works once `uiContext` carries the tab id).
  Blocks created by `wsh` from a pane (e.g. `wsh view`) land in the builder tab via the existing `CreateBlockCommand`; they count toward the empty-state check but not the 16-pane cap.
- **Split focus:** the frontend's backend-action handlers for split actions honour `action.focused` (`frontend/layout/lib/layoutModel.ts:513-569`), as `insert` already does.
- **Connections:** in builder tabs the block header connection button and `Cmd:g` are hidden/unbound; panes are local.

### D7. Failure handling

- Ensure fails: panel error + Retry; app column unaffected.
- Open terminal fails (cap, bad target, backend error): the error string is shown as a notice; no rows created.
- Shell exits: pane behaves as in a main window.
- Switch app or close window with running panes: shells killed without confirmation (same as closing a main-window tab).
- `DeleteBuilderCommand` never reached (crash, quit ordering, IPC timeout): D3 sweep on next start.
- A late Ensure/Open arriving after window teardown can create a tab no window shows; no shell starts (shells start when a pane mounts). The sweep removes it next start. Accepted.

## Testing

- **Go unit** (`pkg/rtcore`, `pkg/wshrpc/wshserver`, `pkg/blockcontroller`, `cmd/server` sweep helper):
  - `CreateBuilderTab`: tab + layout state, no workspace, both meta keys.
  - `DeleteBuilderTab`: deletes blocks, layout state, tab; publishes BlockClose per block; refuses a workspace tab carrying `builder:owner`; refuses owner mismatch; idempotent on missing tab.
  - `DeleteBlock(recursive=true)` on the last block of a builder tab: returns nil, publishes BlockClose, tab remains.
  - `SetMetaCommand` rejects `builder:owner`, `builder:*`, nil `builder:owner` on tab and block orefs; other keys unaffected.
  - Ensure: idempotent; concurrent Ensure creates one tab; bad app dir creates nothing; first call creates exactly one terminal block with `cmd:cwd` = app dir; app id change replaces the tab; failure after tab insert leaves no tab.
  - Open: appends; split actions on an in-tab target; foreign `targetblockid`, missing target, and `replace` rejected with zero new rows; 17th pane rejected; Open with no tab yields exactly one pane.
  - `DeleteBuilderCommand`: removes all owner tabs even with `builder:tabid` rtinfo missing or pointing elsewhere; Ensure/Open/Delete interleavings under `-race` leave either zero tabs or one consistent tab.
  - Sweep: removes builder tabs only; a workspace tab with spoofed `builder:owner` survives; ordering helper is called between controller init and durable reconnect.
  - `makeSwapToken`: no `WORKSPACEID` for a builder pane.
- **Vitest:** layout default/merge without `terminal`; focus switching; key table routing by focus (including `Cmd:w` both ways and unbound keys); last-pane close calls `closeNode` not `closeTab`; create intercept (term -> IPC with action, non-term -> ObjectService, `replaceBlock` term rejected); bootstrap order (no layout model before `staticTabId`, reload on differing id); split handlers honour `focused`.
- `tsc --noEmit`; `go test ./pkg/...`; tsunami package list; full vitest; `go test -race` on touched packages.
- **E2E, isolated only:** nested Xvfb with `DISPLAY` unset for everything not launched on it, scratch HOME and XDG dirs, `REMOTETERM_CONFIG_HOME`, `REMOTETERM_DATA_HOME`, `REMOTETERM_ISOLATED_PROFILE=1`, scratch `GOCACHE`/`GOMODCACHE`, private Vite `cacheDir`; never the live display or the user's data dirs. Keys sent are the Linux bindings (Alt for `Cmd:`). Checks: S1; S2; S3 including two splits in quick succession both appearing; S4 (Alt+W closes pane, last pane -> empty state, window open; Ctrl+W in a shell deletes a word); S9; S6 (PIDs via `echo $$`, gone within 5 s; DB has no builder tab rows); S7 (kill server mid-session, restart, rows gone before first client); S8; S10 (workspace tab with injected `builder:owner` via direct DB write survives restart).

## Risks

- Layout save vs. backend queue race (`persistToBackend` rewriting `pendingbackendactions`, `layoutModel.ts:581-598`): unconfirmed. The E2E rapid-double-split check detects it; if it reproduces, the plan adds a merge-by-action-id fix.
- `getLayoutModelForStaticTab` null-tolerance may be needed at more call sites than listed; the plan audits all callers reachable from builder windows.
- Bell badges on builder panes are never auto-cleared (no `BadgeAutoClearing` in the builder tree). Cosmetic; accepted.

## Audit disposition (v1 findings)

| Finding | Disposition | Where |
|---|---|---|
| Req B1 last-pane close routed to `closeTab` | Accepted: builder close rule never uses `closeTab` | D6, S4, tests |
| Req B2 layout model built before `staticTabId`/LayoutState | Accepted: ordered bootstrap, set-once `staticTabId`, null-tolerant callers | D5 |
| Req M1 `uiContext` closure; create family | Accepted: `uiContext` from atom; central intercept (term -> IPC, other views -> tab) | D5, D6 |
| Req M2 Cmd is Alt on Linux | Accepted | header note, S4, E2E |
| Req M3 SSH prompts unroutable | Accepted by restricting builder panes to local | non-goals, D6 |
| Req M4 subscription inventory | Accepted with list | D5 |
| Req M5 key map scope; nonexistent menu items | Accepted: key table; dropped menu bullet | D6 |
| Req m1 split focus | Accepted | D6, S3 |
| Req m2 webview keys global | Accepted: not extended | D6 |
| Req m3 reload | Accepted | S1, D5 |
| Req m4 Open creating tab -> two panes | Accepted | D2 |
| Req m5 keep-alive hidden blocks | Accepted: closed not hidden; empty = no visible leaves | D5, D6 |
| Req m6 / Sec M4 sweep ordering | Accepted | D3 |
| Req m7 cascade test | Accepted | D1, tests |
| Req m8 observables | Accepted | S3, S6, S9 |
| Req m9 queue race | Risk + E2E detector | Risks |
| Sec H1 spoofable `builder:owner` | Accepted: reserved prefix in SetMeta; structural predicate | D1, S10 |
| Sec H2 rtinfo deleted before teardown; app switch | Accepted: owner scan, awaited delete, app-id check | D1, D2, D4 |
| Sec M1 cross-builder targeting | Accepted: owner == builderId everywhere; rtinfo hint untrusted | D1, D2 |
| Sec M2 validate-before-write, `replace` | Accepted: allowlist, pre-write checks | D2 |
| Sec M3 Ensure/Open/Delete races | Accepted: keyed lock; no tombstone (app switch reuses id); late-call residual accepted | D2, D7 |
| Sec L1 pane flood | Accepted: cap 16 | D2 |
| Sec L2 async kill | Accepted: S6 reworded with 5 s and detached-process exclusion | S6 |
| Sec L3 cwd not a boundary | Accepted | non-goals |
| Sec L4 pre-existing weaknesses | Recorded | trust model |
| Sec L5 empty `WORKSPACEID` | Accepted | D1 |
