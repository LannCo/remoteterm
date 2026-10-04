# Builder terminal panel: design spec (v1)

Date: 2026-10-04. Branch: `feat/builder-terminal` off local `main` 0fee7150 (builder-by-hand).

## Problem

The Tsunami app builder window was laid out as "build on the left, preview on the right": a Wave AI chat panel on the left, the app panel (Preview / Code / Config/Data / Files) and build output on the right. This fork removed the chat (`1a7f1c83`), and builder-by-hand removed the placeholder and the horizontal split (`872f5fd0`). The builder is now a single column, and the only way to drive an app with an agent harness is "Open terminal", which puts a shell in a *main* window, away from the preview.

## Goal

The left side of the builder window becomes a tiled terminal area, so one or more agent harnesses (Claude Code, Codex, Hermes, a plain shell) can work on the app side by side with its live preview. It behaves like a tab in the main window: split, resize, magnify, focus, close.

## Success criteria (each has a named observable)

- S1. Opening an app in the builder shows a terminal panel on the left with one running local shell whose cwd is the app folder (`pwd` prints the app dir).
- S2. "Open terminal" (app panel header) adds a new shell pane to the builder's terminal panel, cwd = app folder. It no longer touches main windows.
- S3. Split / new-pane keybindings with the terminal side focused add panes whose cwd is the app folder.
- S4. With the terminal side focused, `Cmd:w` (Ctrl+W on Linux) closes the focused pane; with the app side focused, it closes the builder window. Closing the last pane shows an empty state with an "Open terminal" button; the window stays open.
- S5. Terminal panel width is resizable and persists for the window's lifetime via `builder:layout` key `terminal`.
- S6. Closing the builder window, or switching to another app, kills every shell in its terminal panel (no surviving shell processes) and deletes its tab, blocks and layout state from the DB.
- S7. After a backend crash or quit that skipped S6 cleanup, the next server start deletes every leftover builder-owned tab and its blocks.
- S8. Builder terminal tabs never appear in any workspace's tab bar, the workspace switcher, or `ws.TabIds`.
- S9. Two panes running concurrently can each write app files; with live rebuild on, the preview rebuilds (existing coalescing; no new build storm).

## Non-goals

- Auto-launching a harness command in new panes (plain shell; user starts the harness).
- Persisting panes or shells across window close (decided: shells die with the window).
- Arbitrating concurrent edits to the same file by different agents.
- Hiding or restricting the per-pane connection switcher (user's choice, as in main windows).
- `wsh` commands that resolve the *workspace* from a builder pane (`ws` scope); they error as for any workspace-less context. Tab-scoped `wsh` works.
- Preview device emulation (sub-project 2, separate spec): built-in device profiles, optional user-downloaded WebKit/Gecko engines.

## Design

### D1. Builder-owned tab (backend)

- New `rtcore.CreateBuilderTab(ctx, builderId) (*Tab, error)`: inserts a `Tab` (`Name: "builder"`, `BlockIds: []`, new `LayoutState`) with `Meta["builder:owner"] = builderId`. It is added to no workspace. Insert of tab and layout state happens in one transaction.
- New `rtcore.DeleteBuilderTab(ctx, tabId) error`: refuses tabs without `builder:owner`; deletes every block with `DeleteBlock(ctx, id, false)` (fires `Event_BlockClose` -> controller destroyed -> shell killed), then the `LayoutState`, then the `Tab`. Idempotent: a missing tab is not an error.
- Builder rtinfo gains `builder:tabid` (string) on the `builder:<id>` oref.
- `DeleteBlock` recursive cascade: when the parent tab has `builder:owner`, the "last block deleted -> delete tab" cascade is skipped (the tab has no workspace; `DBFindWorkspaceForTabId` would fail). The frontend shows the empty state instead.

### D2. RPCs

- New `EnsureBuilderTabCommand{builderid} -> {tabid}`:
  - If builder rtinfo has `builder:tabid` and that tab exists with matching `builder:owner`, return it.
  - Otherwise resolve the app dir (`ResolveBuilderAppDir`, D7 checks from builder-by-hand); on failure return the error and create nothing.
  - Create the tab, create one block from `MakeBuilderTerminalBlockDef(appDir)` in it (with the layout insert action the existing `CreateBlockCommand` path queues), store `builder:tabid`, return.
  - Serialised per builderId so two concurrent calls create one tab.
- `OpenBuilderTerminalCommand` changes from `{builderid, tabid}` to `{builderid, targetblockid?, targetaction?}`:
  - Resolves the tab from builder rtinfo `builder:tabid` (ensuring it if absent), the app dir from builder rtinfo as today. No path or tab id comes from the renderer.
  - `targetaction` is one of the layout actions `CreateBlockCommand` already supports for splits (e.g. split right / split down relative to `targetblockid`); empty means append. `targetblockid`, when set, must be a block in this builder's tab, otherwise error.
- `DeleteBuilderCommand` additionally calls `DeleteBuilderTab` for the builder's `builder:tabid` (if any) and clears that rtinfo key. This covers window close and switch app.

### D3. Startup sweep

On server start, after the DB is open and before any RPC is served, delete every `Tab` whose meta has `builder:owner` via `DeleteBuilderTab`. Builder ids are fresh UUIDs per window (`emain/emain-builder.ts:37`) and never survive a restart, so every such tab is an orphan.

### D4. Electron main process

- `open-builder-terminal` IPC keeps deriving `builderId` from the calling window (never from IPC args), stops picking a main window, and forwards `{builderid, targetblockid, targetaction}` from the renderer. It no longer focuses any other window.
- `destroyBuilderWindow` calls `DeleteBuilderCommand` before deleting builder rtinfo (if not already the case), so Cmd:w-close and duplicate-app paths clean up the tab.

### D5. Renderer init and layout

- `initBuilder` adds the global event subscriptions it lacks (`blockfile`, `waveobj:update`, `config`, and any others the term view needs), excluding handlers that assume a tab window (to be enumerated in the plan).
- `atoms.staticTabId` becomes settable after init; the builder sets it to the ensured tab id. Existing code using it (`uxCloseBlock`, `forceRestartController`, term vdom creation, `getLayoutModelForStaticTab`) then works unchanged.
- `builder-workspace.tsx` restores the horizontal `PanelGroup`: left `BuilderTermPanel` (`layout.terminal`, default 40, min 20), resize handle, right column (existing app/build vertical group). `DefaultLayoutPercentages` gains `terminal: 40`; a saved layout lacking `terminal` uses 40.
- `BuilderTermPanel`:
  - No app selected: renders nothing in the panel body.
  - Calls `EnsureBuilderTabCommand` when the app id is set; on error shows the message and a Retry button.
  - On success renders the standard tile layout for that tab inside `TabModelContext` for the tab id.
  - Zero blocks: empty state with an "Open terminal" button (same action as the header button).

### D6. Focus and keys

- `BuilderFocusType` becomes `"app" | "terminal"`. Focus-capture/click inside the terminal panel sets `"terminal"`; the existing app-panel handlers set `"app"`. The focused side gets the accent border (as the app column does today).
- With focus `"terminal"`, `appHandleKeyDown` routes block/layout keybindings to the builder tab's layout model (today only for tab windows): close pane, split, new pane, focus moves, magnify.
- Split and new-pane keybindings, and the block header/context-menu split/new entries, go through `open-builder-terminal` with the matching `targetaction`, so new panes are always app-folder shells.
- `Cmd:w`: focus `"terminal"` -> close focused pane (`uxCloseBlock`); focus `"app"` -> close the builder window (existing).
- Block menu items that need a workspace (move to another tab or window, etc.) are hidden when the block's tab has `builder:owner`.

### D7. Failure handling

- Ensure fails: panel shows the error and Retry; the app column is unaffected.
- Shell exits: pane behaves as in a main window.
- Switch app or close window with running panes: shells are killed without a confirmation prompt (same as closing a main-window tab).
- Backend gone before `DeleteBuilderCommand` (quit ordering): D3 sweep cleans up next start.

## Testing

- Go unit (`pkg/rtcore`, `pkg/wshrpc/wshserver`, `pkg/buildercontroller` as touched):
  - `CreateBuilderTab` inserts tab + layout state, joins no workspace, sets `builder:owner`.
  - `DeleteBuilderTab` removes blocks, layout state and tab; publishes block-close for each block; refuses non-builder tabs; idempotent.
  - `DeleteBlock(recursive=true)` on the last block of a builder tab leaves the tab.
  - `EnsureBuilderTabCommand` idempotent (same tab on repeat; concurrent calls create one tab); rejects bad app dir without creating anything; first call creates exactly one terminal block with cwd = app dir.
  - `OpenBuilderTerminalCommand` targets the builder tab; rejects a `targetblockid` from another tab.
  - `DeleteBuilderCommand` deletes the tab and clears `builder:tabid`.
  - Startup sweep deletes only `builder:owner` tabs; ordinary tabs untouched.
  - `-race` on touched packages.
- Vitest: layout defaults/merge with missing `terminal`; focus type switching; `Cmd:w` routing by focus; split routing through the builder IPC; workspace-only menu items hidden for builder tabs.
- `tsc --noEmit`; full `go test ./pkg/...`; tsunami packages; full vitest.
- E2E, isolated only: nested Xvfb, `DISPLAY` unset for everything not launched on Xvfb, scratch HOME and XDG dirs, `REMOTETERM_CONFIG_HOME`, `REMOTETERM_DATA_HOME`, `REMOTETERM_ISOLATED_PROFILE=1`, scratch `GOCACHE`/`GOMODCACHE`, private Vite `cacheDir`; never the live display or the user's data dirs. Checks: S1 (`pwd` in first pane), S2 (second pane), S4 (Ctrl+W pane close, empty state, window survives), S9 (edit from a pane with live rebuild on -> `building` -> `running`), S6 (close window -> `ps` shows no shells from those panes; DB has no builder tab rows), S7 (kill server mid-session, restart, rows gone), S8 (no builder tab in any workspace's `tabids`).

## Risks

- `initGlobalWaveEventSubs` handlers may assume a tab window; mitigated by enumerating and excluding in the plan.
- Code paths that look up the workspace from a tab (`DBFindWorkspaceForTabId`) may be hit from builder panes beyond those listed; plan audits callers reachable from term blocks and the tile layout.
- `staticTabId` changing from fixed-at-init to settable may affect consumers that read it once; plan audits readers.
- The tile layout's pending-action queue must apply the initial insert when the builder first renders the tab.
