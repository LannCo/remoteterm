# Builder Terminal Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Tsunami app builder window a tiled terminal area on the left, backed by a server-owned builder tab, so agent harnesses can work beside the live preview, with shells that die with the window and a startup sweep for anything left behind.

**Architecture:** The backend gains a builder-tab lifecycle in `pkg/rtcore` (create, find, delete, sweep) guarded by an `rtstore.IsBuilderTab` predicate, plus three caller-checked RPCs (`EnsureBuilderTabCommand`, a rewritten `OpenBuilderTerminalCommand`, a teardown in `DeleteBuilderCommand`) serialised by a per-builder lock and broadcasting their object updates. Electron forwards the window's own builder id and app id. The renderer bootstraps the tab, pins it, sets a now-writable `staticTabIdAtom`, and mounts the ordinary tile layout in a new left panel; key handling, block creation and close rules branch on `isBuilderWindow()` and on a two-sided builder focus.

**Tech Stack:** Go 1.26.2 (repo toolchain `golang-1.26.2/bin/go`), SQLite via `sqlx`/`txwrap`, Electron + electron-vite 5, React 19, Jotai, `react-resizable-panels`, Tailwind v4, Vitest 3 (`happy-dom` where a DOM is needed), go-task (`./node_modules/.bin/task`).

**Spec:** `/media/owner/Workspace/remoteterm/remoteterm-builder-terminal/.planning/builder-terminal/SPEC.md` (v5). Read it alongside this plan. Where this plan differs, "Plan deviations from spec" below says how and why; "Spec items confirmed in the plan" answers every "confirmed in the plan" item.

## Global Constraints

Every task's requirements include this section.

- Repo root (worktree): `/media/owner/Workspace/remoteterm/remoteterm-builder-terminal`, branch `feat/builder-terminal`. Read `CLAUDE.md` and `.kilocode/rules/rules.md` before writing code. Skill guides: `.kilocode/skills/add-rpc/SKILL.md` (Tasks 6, 7), `.kilocode/skills/electron-api/SKILL.md` (Task 10).
- Go: string constants, never custom enum types; constructors `Make...`, never `New...`; consts at the top of the file; `Printf`, not `Println`; locking only through small helpers using `lock.Lock(); defer lock.Unlock()`; early returns. Never run `go build`; `go test` and `go vet` on specific packages are fine.
- Inside an `rtstore.WithTx`/`WithTxRtn` callback, every nested store call takes `tx.Context()`. The DB allows one open connection (`pkg/rtstore/wstore_dbsetup.go:55`), so an outer context deadlocks until it times out.
- TypeScript: `@/` imports across directories (aliases in `tsconfig.json`: `@/app/*`, `@/builder/*`, `@/util/*`, `@/layout/*`, `@/store/*`, `@/view/*`, `@/element/*`), relative only within a directory; named exports only; 4-space indent; writable atoms typed `PrimitiveAtom<T>` (cast `atom(null) as PrimitiveAtom<T>`); hooks at the top of components before any early return; `== null`, never `=== undefined`; `cursor-pointer` on every clickable; never `cursor-help` or `cursor-not-allowed`. Accent buttons: `bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer`. Models are singletons (`private static instance`, `private constructor`, `static getInstance()`), atoms on the model, `globalStore.get/set`, no hooks in models.
- Comments explain why, never what. Never remove existing comments (moving one with its code is fine).
- Formatting: new `.ts`/`.tsx` files follow the repo Prettier config (`prettier.config.cjs`, 120 columns, 4-space indent from `.editorconfig`); Task 17 checks them. Do not reformat existing files wholesale; several (`keymodel.ts`, `global.ts`, `emain-ipc.ts`) were not Prettier-clean before this branch. New files start with `// Copyright 2026, Command Line Inc.` and `// SPDX-License-Identifier: Apache-2.0`; do not change the year in existing files.
- Generated files are never hand-edited: `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`. After changing `pkg/wshrpc/wshrpctypes*.go`, run `./node_modules/.bin/task generate` from the repo root (it uses `golang-1.26.2/bin/go` through the Taskfile `GO` var, `Taskfile.yml:16-17`).
- Reserved names, exact: tab meta keys `builder:owner`, `builder:appid` (prefix `builder:` reserved on tab orefs); block meta `term:durable` pinned `false` for builder terminals; rtinfo key `builder:layout` gains `terminal` (default 40, min 20); preview partition `builder-preview-<builderId>`; route ids `electron` and `builder:<builderId>`.
- Limits, exact: 16 term blocks per builder tab; Open `targetaction` in `""`, `splitright`, `splitleft`, `splitup`, `splitdown`; Electron IPC target strings at most 64 chars; write context timeout 15 s; Electron `DeleteBuilderCommand` RPC timeout 20 s.
- Messages, verbatim: `builder terminal not ready`; `too many terminals in this builder (max 16)`; `Terminal app and builder app differ; reopen the builder`; `Switching app…`; `No terminals`; `Open terminal`; `Retry`; log line `[startup] builder sweep: removed N tabs` (N substituted).
- Safety, binding on every task: never start the app, Electron, `task dev`, `npm run dev`, `electron-vite dev/preview`, or anything on a display (Task 18 alone launches, on a nested Xvfb, under its own recipe). Never write to `~/.config`, `~/.local/share`, `~/waveapps`, dotfiles, or a running RemoteTerm. Go tests use temp dirs (`os.MkdirTemp`, `t.TempDir()`) and `t.Setenv("HOME", ...)`.
- Test commands, from the repo root with the repo toolchain first on `PATH`:
  ```bash
  export PATH=$PWD/golang-1.26.2/bin:$PATH
  go test ./pkg/<pkg>/... -count=1
  go test -race ./pkg/<pkg>/... -count=1
  (cd tsunami && go test ./app/... ./build/... ./engine/... ./rpctypes/... ./util/... ./vdom/... -count=1)
  npx vitest run <path>
  npx tsc --noEmit
  ./node_modules/.bin/task generate
  ```
- Commits: stage only the files the task names (`git add <paths>`); the worktree has untracked `golang-1.26.2` and `zig-0.14.0`, never add them. Commit messages put user-facing impact first. End every commit message with the line `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Do not push.
- Prose in British English except identifiers. No em-dashes anywhere (use `-`, commas, colons or parentheses).
- Baseline (HEAD 26227a42, before Task 1): `go test ./pkg/... -count=1` is all `ok` (or `[no test files]`), and `./node_modules/.bin/task generate` changes nothing (`git status --short` lists only the untracked `golang-1.26.2` and `zig-0.14.0`). Steps that say "no new failures" or "no other changes" compare against this.
- Expected-failure steps name which tests or builds must fail and why. Quoted messages are exact when they come from this plan's code (test messages, `builder terminal not ready`, and so on); compiler, TypeScript and library messages are given as examples, so a different wording of the same failure is not a mismatch.
- If a step's expected output differs from what you see, stop and report the difference with the command output instead of adapting silently. If a cited line has shifted, find the code by the quoted text and say so in your report.

## Plan deviations from spec

1. **Local-only check accepts every local connection name.** The spec allows `""`/`"local"`. The plan also accepts `local:<shell>` (local shell profiles, `pkg/remote/conncontroller/conncontroller.go:199-201`) and `nil` (a meta delete). Rejecting `local:zsh` would break the local shell picker without adding any safety. `wsl://...` and SSH names are rejected.
2. **`UpdateWorkspaceTabIds` rejects a builder-owned tab only when it would newly join the workspace.** A tab already in that workspace's `TabIds` (only possible through a direct DB write, the S10 setup) can still be reordered. Real builder tabs are never in a workspace, so S8 holds; a lookup error still fails closed.
3. **The sweep logic lives in `rtcore.SweepBuilderTabs`**, which logs the line. `cmd/server/main-server.go` calls it through `runBuilderSweep()`; the ordering test parses `main()` with `go/ast`.
4. **`makeSwapToken` is fixed by extracting `addTabAndWorkspaceEnv`.** `makeSwapToken` also reads the config watcher and filestore (`pkg/blockcontroller/shellcontroller.go:759-761`), so the test drives the extracted helper against a real temp DB.
5. **Test seam for queue failures.** `pkg/wshrpc/wshserver` gets `var queueBuilderLayoutAction = rtcore.QueueLayoutActionForTab`. Nothing in production makes queueing fail, and a missing LayoutState makes `QueueLayoutAction` nil-deref (`pkg/rtcore/layout.go:86-104`, pre-existing, not fixed here), so the spec's "queue failure removes the new block" and "failure after tab insert leaves no tab" tests override the var.
6. **Key names.** The spec writes `Shift:Cmd:w`; keymodel binds `Cmd:Shift:w` (`frontend/app/store/keymodel.ts:494`). Neither is bound in builder windows. Tab windows also bind vim focus aliases `Ctrl:Shift:h/j/k/l` (`keymodel.ts:544-575`); the builder binds them with the other focus moves ("as tab"). `Cmd:i` (refocus) is not bound in builder windows.
7. **Split chord prefix.** keymodel starts a chord before any handler runs (`keymodel.ts:358-362`), so `Ctrl:Shift:s` is consumed in both foci; with app focus the follow-up arrow returns unhandled and does nothing. Changing the shared chord machinery is out of proportion.
8. **Held `Cmd:w` auto-repeat is ignored in builder windows** (Review Focus 1). Without it, holding Alt+W closes every pane, moves focus to the app side, then closes the window.
9. **Ensure deletes every non-matching tab of the builder, also when a matching one exists** ("Others ... are deleted first" read as all of them).
10. **Empty state overlays a mounted tile layout.** `TabContent` unmounts `TileLayout` at zero blocks (`frontend/app/tab/tabcontent.tsx:57-58`); the builder panel keeps it mounted and overlays "No terminals" when `layoutModel.numLeafs` is 0, so the model keeps processing backend actions.
11. **Builder ids must be canonical UUID strings** (`uuid.Parse` and `parsed.String() == builderId`). Electron mints them with `randomUUID()` (`emain/emain-builder.ts:37`), which is canonical.
12. **The header "Open terminal" button is enabled only while the panel is ready** (Ensure succeeded, its `appid` matched, the tab and layout loaded); it is disabled again on an error, while switching, and when the tab vanishes. In any other state Open would add a pane nobody sees. The spec says "until bootstrap step 1 has succeeded"; this is stricter.
13. **Retry on an Ensure error re-runs the bootstrap in place** (no layout model exists yet). Retry after the tab vanished reloads the renderer, as the spec says.
14. **`openBuilderTerminal` (ElectronApi) takes an optional `{ targetblockid, targetaction }`**, and a new `ensureBuilderTab()` returns `{ tabid?, appid?, error? }`. The old pick-a-main-window helpers (`pickTerminalWindow`, `bringWindowToFront`) lose their only caller and are deleted with their tests.
15. **`data-builder-focus` attribute** on the builder workspace root (`"app"` or `"terminal"`), the E2E observable for which side is focused.
16. **In the local-only check, the parent-chain lookup never fails the transaction; a DB error inside `IsBuilderTab` still fails it closed (the write is refused).** Inside `UpdateObjectMeta`'s transaction a nested store error marks the shared `TxWrap` failed (`txwrap.WithTx` sets `txWrap.Err`). A broken or missing parent chain is not a DB error, so the block-to-tab walk uses a quiet lookup that treats it as "not a builder block" and lets the write's own result (for example `ErrNotFound`) stand. A real read error inside `IsBuilderTab` does mark the transaction failed, so the write is refused rather than allowed.
17. **Accepted risk (D7): a stale tab that cannot be deleted blocks that builder until restart.** If a block of a previous app's tab cannot be deleted, `DeleteBuilderTab` keeps the tab (D1). Ensure then fails for that builder (it deletes stale tabs before returning or creating one), and Open reports `builder terminal not ready` (it requires exactly one tab). The startup sweep removes the tab on the next start.
18. **E2E substitutions.** Xvfb has no window manager, so Task 18 closes the window with the renderer's `window.close()`, which takes the same BrowserWindow `close`/`closed` path as a title-bar close (`emain/emain-builder.ts:89-127`), not `destroyBuilderWindow`. "Switch App" is an item in a native context menu (`frontend/builder/builder-apppanel.tsx:282-285`), so the E2E replays `switchBuilderApp`'s calls from the builder renderer (same RPCs, same `builder:<id>` route); the "Switching app…" state is covered by unit tests (Tasks 14, 15). Native context menus cannot be driven there either; the context-menu split is covered by the `createBlockSplit*` intercept unit test, because both menus call those functions (`frontend/app/view/term/term-model.ts:894-913`, `frontend/app/block/blockframe-header.tsx:138-165`).

## Spec items confirmed in the plan

- **Durable meta key:** `remotetermobj.MetaKey_TermDurable = "term:durable"` (`pkg/remotetermobj/metaconsts.go:111`). Local blocks are never durable anyway (`pkg/jobcontroller/jobcontroller.go:2611-2615`), so the pin is defence in depth. Task 4.
- **keymodel key strings** (from `frontend/app/store/keymodel.ts:451-697`): close `Cmd:w`; new pane `Cmd:n`; split right `Cmd:d`; split down `Shift:Cmd:d`; chord `Ctrl:Shift:s` then `ArrowUp`/`ArrowDown`/`ArrowLeft`/`ArrowRight`; focus moves `Ctrl:Shift:ArrowUp/Down/Left/Right` and `Ctrl:Shift:k/j/h/l`; magnify `Cmd:m`; block number `Ctrl:Shift:c{Digit1..9}` and `Ctrl:Shift:c{Numpad1..9}`; search `Cmd:f`, `Escape`. Unbound in builder: `Cmd:t`, `Cmd:Shift:w`, `Cmd:[`, `Shift:Cmd:[`, `Cmd:]`, `Shift:Cmd:]`, `Cmd:1..9`, `F2`, `Ctrl:Shift:i`, `Ctrl:Shift:x`, `Cmd:g`, `Cmd:i`. Task 16.
- **`getLayoutModelForStaticTab()` callers reachable in builder windows**, all made null-tolerant (Task 11): `keymodel.ts:67-71` (`getFocusedBlockInStaticTab`), `:146-159` (`uxCloseBlock`), `:161-177` (`genericClose`), `:190-196` (`switchBlockInDirection`), `:371-385` (block dispatch, now also in builder), `:498-510` (`Cmd:m`); `focusManager.ts:16-20, 34-48`; `global.ts:493-496` (`setNodeFocus`), `:720-740` (`refocusNode`, called from `blockframe-header.tsx:193`), and the create family `global.ts:377-460`. Already null-safe: `global.ts:627-656, 658-718`, `tabrpcclient.ts:21-24, 64-67, 78-81`, `keymodel.ts:179-188`. Not reachable in builder windows: `keymodel.ts:244-261` (returns early for builders), `:263-320` (tab-window `Cmd:n`/splits, replaced by the builder table), `app.tsx:292`, `widgets.tsx:59, 98`.
- **Builder-init gating of `ensure-builder-tab`:** `createBuilderWindow` sets `builderAppId` (`emain/emain-builder.ts:86`) before pushing the window (`:129`); `builder-init` is sent only for a window found by `getBuilderWindowByWebContentsId` (`emain/emain-ipc.ts:492-497`), so only after that push. The app-selection modal awaits `setBuilderWindowAppId` before it sets `atoms.builderAppId` (`frontend/builder/app-selection-modal.tsx:138-148`, `154-172`), and `BuilderWorkspace` (which holds the panel) renders only once `builderAppId` is a draft id (`frontend/builder/builder-app.tsx:37-55`). The panel cannot call the IPC before Electron knows the app id.
- **Create-family callers and their return values:** no caller uses the returned block id (`keymodel.ts:299, 309, 319, 581`; `blockframe-header.tsx:149, 163`; `term-model.ts:829, 901, 911, 935`; `termsticker.tsx:91`; `previewutil.ts:55, 70`; `launcher.tsx:127`; `app.tsx:129`; `connectiondropdown.tsx:180`; `widgets.tsx` and `webview.tsx:765` through `WaveEnv.createBlock`, `frontend/app/remotetermenv/remotetermenvimpl.ts:34`). Returning `null` from the builder branch is safe. Task 13.
- **Risk: `focusedNodeId` not cleared on root delete.** Confirmed harmless for the atom (`focusedNode` resolves through `findNode`, which returns `undefined` for an empty tree, `frontend/layout/lib/layoutNode.ts:113-122`) but `getFocusedBlockInStaticTab` then throws on `focusedNode.data` (`keymodel.ts:69-70`). Task 11 adds optional chaining and a test.
- **Risk: layout save vs. backend queue race** (`frontend/layout/lib/layoutModel.ts:581-599`). Not fixed pre-emptively; Task 18's two-rapid-splits check is the detector, and its report must say whether it reproduced.
- **Cites re-checked** (current lines): `wstore.go:57-75` `UpdateObjectMeta`; `block.go:119-157` `DeleteBlock`, cascade at `:142-154`, `sendBlockCloseEvent` at `:155`; `workspace.go:453-461` `UpdateWorkspaceTabIds`; `blockcontroller.go:618-630` workspace env; `appdir.go:38-47` block def; `wshserver.go:1164-1229` builder RPCs; `main-server.go:308-310` controller init and durable reconnect, `:323-346` listeners, `:344` `WAVESRV-ESTART`; `global.ts:55-96` and `:753-790` subscriptions; `global-atoms.ts:19-25` and `:66` atoms; `layoutModel.ts:515-570` split handlers; `builder-apppanel-model.ts:308-324` `switchBuilderApp`; `builder-previewtab.tsx:215-223` webview; `emain-ipc.ts:226-251` `destroyBuilderWindow`, `:547-571` `open-builder-terminal`; `emain-websecurity.ts:91-93` partition default.

## Review Focus

Five conditions the spec implies but no spec-derived test exercises; each has a pinning test in the named task.

1. **Holding Alt+W in the terminal side** (key auto-repeat): a person expects panes to close, not the builder window once they run out. Repeated `Cmd:w` keydowns are ignored in builder windows. Test: Task 16, `ignores auto-repeated Cmd:w in both foci` in `frontend/builder/store/builder-keys.test.ts`.
2. **The app folder is renamed or deleted while the builder is open, then "Open terminal" is pressed:** a clear error notice and no new rows, never a pane in some other directory. Test: Task 7, `TestOpenBuilderTerminalAppDirGone`.
3. **A shell exits and the person presses Enter on the last pane** (`term-model.ts:719-722` calls `closeBlock`, which sends `DeleteBlockCommand` at `:754-756`; that handler deletes recursively, `wshserver.go:467-490`): the builder tab survives, the pane goes, and the empty state shows. Test: Task 8, `TestDeleteBlockCommandOnLastBuilderPaneKeepsTab`.
4. **Two builder windows open on different apps:** closing or switching one never touches the other's tab or shells. Test: Task 8, `TestDeleteBuilderCommandLeavesOtherBuilder`.
5. **"Open terminal" clicked repeatedly near the 16-pane cap** (double-click, or an agent loop): the lock makes the count check and the insert atomic, so the tab ends with exactly 16 term blocks. Test: Task 7, `TestOpenBuilderTerminalConcurrentAtCap`.

---

## File structure

| Path | Status | Responsibility |
|---|---|---|
| `pkg/rtstore/wstore_builder.go` | Create | Builder meta key consts, `IsBuilderTab`, owner lookup SQL, reserved-key and local-only checks |
| `pkg/rtstore/wstore.go` | Modify `:57-75` | `UpdateObjectMeta` calls the two checks |
| `pkg/rtstore/wstore_builder_test.go` | Create | DB fixture (`TestMain`) and tests for the above |
| `pkg/rtcore/buildertab.go` | Create | `CreateBuilderTab`, `FindBuilderTabs`, `DeleteBuilderTab`, `SweepBuilderTabs` |
| `pkg/rtcore/block.go` | Modify `:20-157` | Local-only guard in `CreateBlock`/`CreateSubBlock`; BlockClose right after delete; cascade skip |
| `pkg/rtcore/workspace.go` | Modify `:453-461` | Builder tabs cannot join a workspace |
| `pkg/rtcore/rtcore_fixture_test.go`, `buildertab_test.go`, `block_builder_test.go` | Create | DB fixture with a BlockClose recorder; tests |
| `pkg/blockcontroller/blockcontroller.go` | Modify `:608-630` | `addTabAndWorkspaceEnv`, no empty `WORKSPACEID` |
| `pkg/blockcontroller/builderenv_test.go` | Create | DB fixture and env test |
| `pkg/buildercontroller/appdir.go` | Modify | `ResolveAppDirForAppId`; durable pin in the block def |
| `pkg/buildercontroller/appdir_test.go` | Modify | Tests for both |
| `pkg/wshutil/wshrpc.go`, `wshrpc_test.go` | Modify / Create | `MakeRpcSourceContextForTest` |
| `pkg/wshrpc/wshrpctypes_builder.go` | Modify | `EnsureBuilderTabCommand` types; new `CommandOpenBuilderTerminalData` |
| `pkg/wshrpc/wshserver/builderterm.go` | Create | Caller check, keyed lock, write context, Ensure/Open/Delete handlers |
| `pkg/wshrpc/wshserver/wshserver.go` | Modify `:1164-1170`, `:1208-1222` | Old Delete/Open handlers removed (moved) |
| `pkg/wshrpc/wshserver/builderterm_unit_test.go`, `builderterm_fixture_test.go`, `builderterm_test.go` | Create | Pure tests; DB fixture; RPC tests |
| `cmd/server/main-server.go`, `main-server_sweep_test.go` | Modify / Create | `runBuilderSweep()` between controller init and durable reconnect; AST order test |
| `emain/emain-builder-select.ts`, `.test.ts` | Modify | `parseBuilderTerminalTarget`, `runBuilderTeardown`; old picker helpers removed |
| `emain/emain-ipc.ts`, `emain/preload.ts`, `frontend/types/custom.d.ts`, `frontend/preview/mock/preview-electron-api.ts` | Modify | `ensure-builder-tab`, new `open-builder-terminal`, teardown order |
| `frontend/app/store/global-atoms.ts` | Modify `:14-126` | `staticTabIdAtom` writable; `uiContext` reads it |
| `frontend/app/store/global.ts` | Modify | Builder subscriptions; create intercept; null-safe callers; keep-alive off in builder |
| `frontend/remoteterm.ts` | Modify `:235-289` | Builder subscriptions |
| `frontend/layout/lib/layoutModelHooks.ts` | Modify `:45-48` | `null` model without a static tab |
| `frontend/layout/lib/backendsplit.ts`, `frontend/layout/tests/backendsplit.test.ts` | Create | Backend split action translation (focused, insert fallback) |
| `frontend/layout/lib/layoutModel.ts` | Modify `:515-570` | Uses it |
| `frontend/app/store/keymodel.ts` | Modify | Null-safe callers; builder close rule; builder key tables; block dispatch |
| `frontend/app/store/focusManager.ts` | Modify | Null-safe |
| `frontend/app/store/builder-terminal.ts`, `.test.ts` | Create | Shared notice atom, term-def check, split mapping, `openBuilderTerminal`, connection-UI gate |
| `frontend/builder/store/builder-focusmanager.ts` | Modify | `"app" | "terminal"` |
| `frontend/builder/store/builder-term-model.ts`, `.test.ts` | Create | Bootstrap, states, pane-count focus |
| `frontend/builder/store/builder-layout.ts`, `.test.ts` | Create | Layout defaults and merge |
| `frontend/builder/builder-termcontents.tsx`, `.test.tsx` | Create | Tile contents with `onNodeDelete` |
| `frontend/builder/builder-termpanel.tsx` | Create | Panel UI and states |
| `frontend/builder/builder-workspace.tsx` | Modify | Horizontal split, focus borders, `data-builder-focus` |
| `frontend/builder/builder-appheader.tsx` | Modify `:132-159` (button `:138-144`) | Button disabled until the panel is ready |
| `frontend/builder/store/builder-apppanel-model.ts`, `.test.ts` | Modify | Shared notice; `openTerminal`; `switchBuilderApp`; preview partition helper |
| `frontend/builder/tabs/builder-previewtab.tsx` | Modify `:215-223` | Preview partition |
| `frontend/builder/store/builder-keys.ts`, `.test.ts` | Create | Builder key tables |
| `frontend/app/block/blockframe-header.tsx` | Modify `:272-280` | No connection button in builder windows |
| `frontend/app/store/*.test.ts` (several), `frontend/app/static-tab-writers.test.ts` | Create | Store, keymodel, intercept and source-scan tests |

---
### Task 1: Builder-tab predicate and meta guards (`pkg/rtstore`)

**Files:**
- Create: `pkg/rtstore/wstore_builder.go`
- Modify: `pkg/rtstore/wstore.go:57-75` (`UpdateObjectMeta`)
- Test: `pkg/rtstore/wstore_builder_test.go` (new; first DB-backed test in this package)

**Interfaces:**
- Consumes: existing `DBGet`, `DBFindWorkspaceForTabId` (`wstore_dbops.go:393-410`), `WithTxRtn`.
- Produces (package `rtstore`):
  - consts `MetaKey_BuilderOwner = "builder:owner"`, `MetaKey_BuilderAppId = "builder:appid"`, `BuilderMetaPrefix = "builder:"`
  - `var ErrBuilderLocalOnly error`
  - `IsBuilderTab(ctx context.Context, tabId string) bool`
  - `DBFindTabIdsByBuilderOwner(ctx context.Context, builderId string) ([]string, error)` (`""` = any non-empty owner)
  - `CheckNoReservedTabMeta(meta remotetermobj.MetaMapType) error`
  - `CheckBuilderTabConnection(ctx context.Context, tabId string, meta remotetermobj.MetaMapType) error`
  - `CheckBuilderBlockConnection(ctx context.Context, blockId string, meta remotetermobj.MetaMapType) error`

The DB fixture follows `pkg/jobcontroller/jobcontroller_reconnect_test.go:34-40, 98-136` (temp data dir, `remotetermbase.DataHome_VarCache`, `InitWStore`, `filestore.InitFilestore`) but runs once in `TestMain`, as `pkg/buildercontroller/buildercontroller_test.go:21-31` does for its data dir.

- [ ] **Step 1: Write the failing tests**

Create `pkg/rtstore/wstore_builder_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rtstore-test-*")
	if err != nil {
		fmt.Printf("cannot create a test data dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, remotetermbase.WaveDBDir), 0755); err != nil {
		fmt.Printf("cannot create the db dir: %v\n", err)
		os.Exit(1)
	}
	remotetermbase.DataHome_VarCache = dir
	if err := InitWStore(); err != nil {
		fmt.Printf("wstore: %v\n", err)
		os.Exit(1)
	}
	if err := filestore.InitFilestore(); err != nil {
		fmt.Printf("filestore: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func insertTestTab(t *testing.T, meta remotetermobj.MetaMapType) *remotetermobj.Tab {
	t.Helper()
	tab := &remotetermobj.Tab{OID: uuid.NewString(), Name: "t", BlockIds: []string{}, LayoutState: uuid.NewString(), Meta: meta}
	if err := DBInsert(context.Background(), tab); err != nil {
		t.Fatalf("insert tab: %v", err)
	}
	return tab
}

func insertTestWorkspace(t *testing.T, tabIds ...string) *remotetermobj.Workspace {
	t.Helper()
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: tabIds}
	if err := DBInsert(context.Background(), ws); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	return ws
}

func insertTestBlock(t *testing.T, parentORef string, meta remotetermobj.MetaMapType) *remotetermobj.Block {
	t.Helper()
	block := &remotetermobj.Block{OID: uuid.NewString(), ParentORef: parentORef, Meta: meta}
	if err := DBInsert(context.Background(), block); err != nil {
		t.Fatalf("insert block: %v", err)
	}
	return block
}

func builderMeta(owner string) remotetermobj.MetaMapType {
	return remotetermobj.MetaMapType{MetaKey_BuilderOwner: owner, MetaKey_BuilderAppId: "draft/demo"}
}

func tabORef(tabId string) remotetermobj.ORef {
	return remotetermobj.MakeORef(remotetermobj.OType_Tab, tabId)
}

func blockORef(blockId string) remotetermobj.ORef {
	return remotetermobj.MakeORef(remotetermobj.OType_Block, blockId)
}

func TestIsBuilderTab(t *testing.T) {
	ctx := context.Background()
	builderTab := insertTestTab(t, builderMeta(uuid.NewString()))
	if !IsBuilderTab(ctx, builderTab.OID) {
		t.Error("a tab with builder:owner and no workspace is a builder tab")
	}
	spoofed := insertTestTab(t, builderMeta(uuid.NewString()))
	insertTestWorkspace(t, spoofed.OID)
	if IsBuilderTab(ctx, spoofed.OID) {
		t.Error("a workspace tab carrying builder:owner must not count as a builder tab")
	}
	plain := insertTestTab(t, nil)
	if IsBuilderTab(ctx, plain.OID) {
		t.Error("a tab without builder:owner is not a builder tab")
	}
	if IsBuilderTab(ctx, uuid.NewString()) {
		t.Error("a missing tab is not a builder tab")
	}
}

func TestDBFindTabIdsByBuilderOwner(t *testing.T) {
	ctx := context.Background()
	ownerA := uuid.NewString()
	tabA := insertTestTab(t, builderMeta(ownerA))
	tabB := insertTestTab(t, builderMeta(uuid.NewString()))
	plain := insertTestTab(t, remotetermobj.MetaMapType{"tab:background": "red"})

	got, err := DBFindTabIdsByBuilderOwner(ctx, ownerA)
	if err != nil || !slices.Equal(got, []string{tabA.OID}) {
		t.Fatalf("by owner = %v, %v; want [%s]", got, err, tabA.OID)
	}
	all, err := DBFindTabIdsByBuilderOwner(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(all, tabA.OID) || !slices.Contains(all, tabB.OID) || slices.Contains(all, plain.OID) {
		t.Fatalf("any owner = %v; want %s and %s, not %s", all, tabA.OID, tabB.OID, plain.OID)
	}
}

func TestUpdateObjectMetaRejectsBuilderKeysOnTabs(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := insertTestTab(t, builderMeta(owner))
	for _, patch := range []remotetermobj.MetaMapType{
		{MetaKey_BuilderOwner: "someone-else"},
		{MetaKey_BuilderAppId: "draft/other"},
		{"builder:*": true},
		{MetaKey_BuilderOwner: nil},
		{"builder:anything": "x", "tab:background": "red"},
	} {
		if err := UpdateObjectMeta(ctx, tabORef(tab.OID), patch, false); err == nil {
			t.Errorf("patch %v was accepted", patch)
		}
	}
	got, _ := DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if got.Meta.GetString(MetaKey_BuilderOwner, "") != owner || got.Meta.GetString("tab:background", "") != "" {
		t.Fatalf("tab meta changed: %v", got.Meta)
	}
	if err := UpdateObjectMeta(ctx, tabORef(tab.OID), remotetermobj.MetaMapType{"tab:background": "red"}, false); err != nil {
		t.Fatalf("ordinary tab meta rejected: %v", err)
	}
	got, _ = DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if got.Meta.GetString("tab:background", "") != "red" || got.Meta.GetString(MetaKey_BuilderOwner, "") != owner {
		t.Fatalf("tab meta after ordinary write = %v", got.Meta)
	}
}

func TestUpdateObjectMetaDoesNotGuardBuilderKeysOnBlocks(t *testing.T) {
	block := insertTestBlock(t, tabORef(insertTestTab(t, nil).OID).String(), remotetermobj.MetaMapType{"view": "term"})
	if err := UpdateObjectMeta(context.Background(), blockORef(block.OID), remotetermobj.MetaMapType{"builder:note": "x"}, false); err != nil {
		t.Fatalf("block meta write rejected: %v", err)
	}
}

func TestUpdateObjectMetaLocalOnlyForBuilderBlocks(t *testing.T) {
	ctx := context.Background()
	builderTab := insertTestTab(t, builderMeta(uuid.NewString()))
	block := insertTestBlock(t, tabORef(builderTab.OID).String(), remotetermobj.MetaMapType{"view": "term"})
	subBlock := insertTestBlock(t, blockORef(block.OID).String(), remotetermobj.MetaMapType{"view": "term"})

	for _, conn := range []string{"user@host", "wsl://Ubuntu"} {
		patch := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: conn}
		if err := UpdateObjectMeta(ctx, blockORef(block.OID), patch, false); !errors.Is(err, ErrBuilderLocalOnly) {
			t.Errorf("block connection %q: err = %v, want ErrBuilderLocalOnly", conn, err)
		}
		if err := UpdateObjectMeta(ctx, blockORef(subBlock.OID), patch, false); !errors.Is(err, ErrBuilderLocalOnly) {
			t.Errorf("sub-block connection %q: err = %v, want ErrBuilderLocalOnly", conn, err)
		}
	}
	for _, conn := range []any{"local", "", "local:zsh", nil} {
		patch := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: conn}
		if err := UpdateObjectMeta(ctx, blockORef(block.OID), patch, false); err != nil {
			t.Errorf("local connection %v rejected: %v", conn, err)
		}
	}
	if err := UpdateObjectMeta(ctx, blockORef(block.OID), remotetermobj.MetaMapType{"term:fontsize": 12}, false); err != nil {
		t.Errorf("non-connection write rejected: %v", err)
	}

	wsTab := insertTestTab(t, nil)
	insertTestWorkspace(t, wsTab.OID)
	wsBlock := insertTestBlock(t, tabORef(wsTab.OID).String(), remotetermobj.MetaMapType{"view": "term"})
	if err := UpdateObjectMeta(ctx, blockORef(wsBlock.OID), remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: "user@host"}, false); err != nil {
		t.Errorf("remote connection rejected for a workspace block: %v", err)
	}
}

func TestUpdateObjectMetaConnectionOnMissingBlockIsNotFound(t *testing.T) {
	patch := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: "user@host"}
	err := UpdateObjectMeta(context.Background(), blockORef(uuid.NewString()), patch, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (the connection check must not poison the transaction)", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go test ./pkg/rtstore/... -count=1`
Expected: the build fails because the new names do not exist yet (for example `undefined: MetaKey_BuilderOwner`).

- [ ] **Step 3: Implement**

Create `pkg/rtstore/wstore_builder.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
)

const (
	MetaKey_BuilderOwner = "builder:owner"
	MetaKey_BuilderAppId = "builder:appid"
	BuilderMetaPrefix    = "builder:"
)

const maxBlockParentDepth = 5

var ErrBuilderLocalOnly = errors.New("builder terminals are local only: remote connections are not available in a builder window")

// IsBuilderTab reports whether tabId is a builder-owned tab: it carries builder:owner and belongs to
// no workspace. A lookup error counts as "not a builder tab", so callers that delete refuse.
// Inside a transaction pass tx.Context(): the DB has one connection, so an outer context would block.
func IsBuilderTab(ctx context.Context, tabId string) bool {
	tab, err := DBGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil || tab == nil {
		return false
	}
	if tab.Meta.GetString(MetaKey_BuilderOwner, "") == "" {
		return false
	}
	wsId, err := DBFindWorkspaceForTabId(ctx, tabId)
	if err != nil {
		return false
	}
	return wsId == ""
}

// builderId "" matches any non-empty owner. Results still need IsBuilderTab: a workspace tab can carry the key.
func DBFindTabIdsByBuilderOwner(ctx context.Context, builderId string) ([]string, error) {
	return WithTxRtn(ctx, func(tx *TxWrap) ([]string, error) {
		if builderId == "" {
			query := `SELECT oid FROM db_tab WHERE COALESCE(json_extract(data, '$.meta."builder:owner"'), '') <> ''`
			return tx.SelectStrings(query), nil
		}
		query := `SELECT oid FROM db_tab WHERE json_extract(data, '$.meta."builder:owner"') = ?`
		return tx.SelectStrings(query, builderId), nil
	})
}

func CheckNoReservedTabMeta(meta remotetermobj.MetaMapType) error {
	for key := range meta {
		if strings.HasPrefix(key, BuilderMetaPrefix) {
			return fmt.Errorf("tab meta key %q is reserved", key)
		}
	}
	return nil
}

func isLocalConnMetaValue(val any) bool {
	if val == nil {
		return true
	}
	connName, ok := val.(string)
	if !ok {
		return false
	}
	return connName == "" || connName == "local" || strings.HasPrefix(connName, "local:")
}

// The connection checks look anything up only when the patch sets a connection. They keep SSH
// prompts out of builder windows, which cannot show them; they are not a security boundary.
func CheckBuilderTabConnection(ctx context.Context, tabId string, meta remotetermobj.MetaMapType) error {
	connVal, ok := meta[remotetermobj.MetaKey_Connection]
	if !ok || isLocalConnMetaValue(connVal) {
		return nil
	}
	if !IsBuilderTab(ctx, tabId) {
		return nil
	}
	return ErrBuilderLocalOnly
}

func CheckBuilderBlockConnection(ctx context.Context, blockId string, meta remotetermobj.MetaMapType) error {
	connVal, ok := meta[remotetermobj.MetaKey_Connection]
	if !ok || isLocalConnMetaValue(connVal) {
		return nil
	}
	tabId := dbFindTabForBlockIdQuiet(ctx, blockId)
	if tabId == "" {
		return nil
	}
	return CheckBuilderTabConnection(ctx, tabId, meta)
}

// Unlike DBFindTabForBlockId this never returns an error. Inside a transaction a nested error marks
// the shared TxWrap failed, which would roll back the caller's write for an unrelated reason.
func dbFindTabForBlockIdQuiet(ctx context.Context, blockId string) string {
	tabId, _ := WithTxRtn(ctx, func(tx *TxWrap) (string, error) {
		for range maxBlockParentDepth {
			parentORef := tx.GetString(`SELECT json_extract(data, '$.parentoref') FROM db_block WHERE oid = ?`, blockId)
			oref, err := remotetermobj.ParseORef(parentORef)
			if err != nil {
				return "", nil
			}
			if oref.OType == remotetermobj.OType_Tab {
				return oref.OID, nil
			}
			if oref.OType != remotetermobj.OType_Block {
				return "", nil
			}
			blockId = oref.OID
		}
		return "", nil
	})
	return tabId
}
```

In `pkg/rtstore/wstore.go`, replace `UpdateObjectMeta` (lines 57-75) with:

```go
func UpdateObjectMeta(ctx context.Context, oref remotetermobj.ORef, meta remotetermobj.MetaMapType, mergeSpecial bool) error {
	return WithTx(ctx, func(tx *TxWrap) error {
		if oref.IsEmpty() {
			return fmt.Errorf("empty object reference")
		}
		if oref.OType == remotetermobj.OType_Tab {
			if err := CheckNoReservedTabMeta(meta); err != nil {
				return err
			}
		}
		if oref.OType == remotetermobj.OType_Block {
			if err := CheckBuilderBlockConnection(tx.Context(), oref.OID, meta); err != nil {
				return err
			}
		}
		obj, _ := DBGetORef(tx.Context(), oref)
		if obj == nil {
			return ErrNotFound
		}
		objMeta := remotetermobj.GetMeta(obj)
		if objMeta == nil {
			objMeta = make(map[string]any)
		}
		newMeta := remotetermobj.MergeMeta(objMeta, meta, mergeSpecial)
		remotetermobj.SetMeta(obj, newMeta)
		DBUpdate(tx.Context(), obj)
		return nil
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/rtstore/... -count=1 && go test -race ./pkg/rtstore/... -count=1`
Expected: `ok  	github.com/LannCo/remoteterm/pkg/rtstore` twice.

- [ ] **Step 5: Run the dependants**

Run: `go test ./pkg/rtcore/... ./pkg/jobcontroller/... ./pkg/wshrpc/... ./pkg/service/... -count=1`
Expected: no new failures compared with the baseline (every line `ok` or `[no test files]`).

- [ ] **Step 6: Commit**

```bash
git add pkg/rtstore/wstore_builder.go pkg/rtstore/wstore.go pkg/rtstore/wstore_builder_test.go
git commit -m "feat(builder): reserve builder tab meta and keep builder blocks local

Tab meta keys under builder: can no longer be written through SetMeta or
the object service, and a builder tab's blocks reject remote connections.

Miscellanea: rtstore.IsBuilderTab predicate and owner lookup; first
DB-backed tests in pkg/rtstore.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Builder tab lifecycle (`pkg/rtcore`)

**Files:**
- Create: `pkg/rtcore/buildertab.go`
- Test: `pkg/rtcore/rtcore_fixture_test.go` (new DB fixture, shared by Task 3), `pkg/rtcore/buildertab_test.go`

**Interfaces:**
- Consumes (Task 1): `rtstore.MetaKey_BuilderOwner`, `rtstore.MetaKey_BuilderAppId`, `rtstore.IsBuilderTab`, `rtstore.DBFindTabIdsByBuilderOwner`.
- Produces (package `rtcore`):
  - `const BuilderTabName = "builder"`
  - `CreateBuilderTab(ctx context.Context, builderId string, appId string) (*remotetermobj.Tab, error)`
  - `FindBuilderTabs(ctx context.Context, builderId string) ([]*remotetermobj.Tab, error)` (`""` = every builder tab)
  - `DeleteBuilderTab(ctx context.Context, tabId string, expectedOwner string) error` (`expectedOwner ""` = any owner)
  - `SweepBuilderTabs(ctx context.Context) int` (logs `[startup] builder sweep: removed N tabs`)
  - test helpers in package `rtcore` (`_test.go`): `blockCloses` recorder with `closed(blockId string) bool` and `setHook(func(blockId string))`; `insertTestTab`, `insertTestWorkspace`, `makeTestBuilderTab`, `addTestTermBlock`, `objExists`

- [ ] **Step 1: Write the DB fixture**

Create `pkg/rtcore/rtcore_fixture_test.go`. It follows `pkg/jobcontroller/jobcontroller_reconnect_test.go:98-136` (temp data dir, wstore, filestore, a `wps.Broker` client that records events), run once from `TestMain`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/google/uuid"
)

// Publish is synchronous, so a hook set with setHook runs on the deleting goroutine, between that
// block's delete and the rest of the caller's work (pattern: jobcontroller_reconnect_test.go:43-75).
type blockCloseRecorder struct {
	lock     sync.Mutex
	blockIds []string
	hook     func(blockId string)
}

var blockCloses = &blockCloseRecorder{}

func (r *blockCloseRecorder) SendEvent(routeId string, ev wps.WaveEvent) {
	if ev.Event != wps.Event_BlockClose {
		return
	}
	blockId, ok := ev.Data.(string)
	if !ok {
		return
	}
	if hook := r.record(blockId); hook != nil {
		hook(blockId)
	}
}

func (r *blockCloseRecorder) record(blockId string) func(string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.blockIds = append(r.blockIds, blockId)
	return r.hook
}

func (r *blockCloseRecorder) setHook(hook func(blockId string)) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.hook = hook
}

func (r *blockCloseRecorder) closed(blockId string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	for _, id := range r.blockIds {
		if id == blockId {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rtcore-test-*")
	if err != nil {
		fmt.Printf("cannot create a test data dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, remotetermbase.WaveDBDir), 0755); err != nil {
		fmt.Printf("cannot create the db dir: %v\n", err)
		os.Exit(1)
	}
	remotetermbase.DataHome_VarCache = dir
	if err := rtstore.InitWStore(); err != nil {
		fmt.Printf("wstore: %v\n", err)
		os.Exit(1)
	}
	if err := filestore.InitFilestore(); err != nil {
		fmt.Printf("filestore: %v\n", err)
		os.Exit(1)
	}
	wps.Broker.Subscribe("rtcore-test", wps.SubscriptionRequest{Event: wps.Event_BlockClose, AllScopes: true})
	wps.Broker.SetClient(blockCloses)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func insertTestTab(t *testing.T, meta remotetermobj.MetaMapType) *remotetermobj.Tab {
	t.Helper()
	ctx := context.Background()
	layout := &remotetermobj.LayoutState{OID: uuid.NewString()}
	tab := &remotetermobj.Tab{OID: uuid.NewString(), Name: "t", BlockIds: []string{}, LayoutState: layout.OID, Meta: meta}
	if err := rtstore.DBInsert(ctx, tab); err != nil {
		t.Fatalf("insert tab: %v", err)
	}
	if err := rtstore.DBInsert(ctx, layout); err != nil {
		t.Fatalf("insert layout: %v", err)
	}
	return tab
}

func insertTestWorkspace(t *testing.T, tabIds ...string) *remotetermobj.Workspace {
	t.Helper()
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: tabIds}
	if err := rtstore.DBInsert(context.Background(), ws); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	return ws
}

func makeTestBuilderTab(t *testing.T, builderId string) *remotetermobj.Tab {
	t.Helper()
	tab, err := CreateBuilderTab(context.Background(), builderId, "draft/demo")
	if err != nil {
		t.Fatalf("CreateBuilderTab: %v", err)
	}
	return tab
}

func addTestTermBlock(t *testing.T, tabId string) *remotetermobj.Block {
	t.Helper()
	blockDef := &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
		remotetermobj.MetaKey_View:       "term",
		remotetermobj.MetaKey_Controller: "shell",
	}}
	block, err := CreateBlock(context.Background(), tabId, blockDef, nil)
	if err != nil {
		t.Fatalf("CreateBlock: %v", err)
	}
	return block
}

func objExists(t *testing.T, otype string, oid string) bool {
	t.Helper()
	found, err := rtstore.DBExistsORef(context.Background(), remotetermobj.MakeORef(otype, oid))
	if err != nil {
		t.Fatalf("exists %s:%s: %v", otype, oid, err)
	}
	return found
}
```

- [ ] **Step 2: Write the failing tests**

Create `pkg/rtcore/buildertab_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

func tabIdsOf(tabs []*remotetermobj.Tab) []string {
	ids := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		ids = append(ids, tab.OID)
	}
	return ids
}

func TestCreateBuilderTab(t *testing.T) {
	ctx := context.Background()
	builderId := uuid.NewString()
	tab, err := CreateBuilderTab(ctx, builderId, "draft/demo")
	if err != nil {
		t.Fatal(err)
	}
	got, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if err != nil || got == nil {
		t.Fatalf("tab not stored: %v", err)
	}
	if got.Name != BuilderTabName || len(got.BlockIds) != 0 {
		t.Errorf("tab = %+v", got)
	}
	if got.Meta.GetString(rtstore.MetaKey_BuilderOwner, "") != builderId || got.Meta.GetString(rtstore.MetaKey_BuilderAppId, "") != "draft/demo" {
		t.Errorf("tab meta = %v", got.Meta)
	}
	if !objExists(t, remotetermobj.OType_LayoutState, got.LayoutState) {
		t.Error("layout state not stored")
	}
	wsId, err := rtstore.DBFindWorkspaceForTabId(ctx, tab.OID)
	if err != nil || wsId != "" {
		t.Errorf("builder tab joined workspace %q (%v)", wsId, err)
	}
	if !rtstore.IsBuilderTab(ctx, tab.OID) {
		t.Error("IsBuilderTab = false for a new builder tab")
	}
	if _, err := CreateBuilderTab(ctx, "", "draft/demo"); err == nil {
		t.Error("empty builder id accepted")
	}
	if _, err := CreateBuilderTab(ctx, builderId, ""); err == nil {
		t.Error("empty app id accepted")
	}
}

func TestFindBuilderTabs(t *testing.T) {
	ctx := context.Background()
	ownerA := uuid.NewString()
	tabA := makeTestBuilderTab(t, ownerA)
	tabB := makeTestBuilderTab(t, uuid.NewString())
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: ownerA})
	insertTestWorkspace(t, spoofed.OID)

	byOwner, err := FindBuilderTabs(ctx, ownerA)
	if err != nil || !slices.Equal(tabIdsOf(byOwner), []string{tabA.OID}) {
		t.Fatalf("FindBuilderTabs(owner) = %v, %v; want [%s]", tabIdsOf(byOwner), err, tabA.OID)
	}
	all, err := FindBuilderTabs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	ids := tabIdsOf(all)
	if !slices.Contains(ids, tabA.OID) || !slices.Contains(ids, tabB.OID) || slices.Contains(ids, spoofed.OID) {
		t.Fatalf("FindBuilderTabs(\"\") = %v", ids)
	}
}

func TestDeleteBuilderTabDeletesBlocksLayoutAndTab(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := makeTestBuilderTab(t, owner)
	b1 := addTestTermBlock(t, tab.OID)
	b2 := addTestTermBlock(t, tab.OID)

	if err := DeleteBuilderTab(ctx, tab.OID, owner); err != nil {
		t.Fatal(err)
	}
	if objExists(t, remotetermobj.OType_Tab, tab.OID) || objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Error("tab or layout state survived")
	}
	for _, b := range []*remotetermobj.Block{b1, b2} {
		if objExists(t, remotetermobj.OType_Block, b.OID) {
			t.Errorf("block %s survived", b.OID)
		}
		if !blockCloses.closed(b.OID) {
			t.Errorf("no BlockClose for %s", b.OID)
		}
	}
	if err := DeleteBuilderTab(ctx, tab.OID, owner); err != nil {
		t.Errorf("second delete = %v, want nil", err)
	}
}

func TestDeleteBuilderTabRefusesWorkspaceTabAndOtherOwner(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: owner})
	insertTestWorkspace(t, spoofed.OID)
	if err := DeleteBuilderTab(ctx, spoofed.OID, ""); err == nil {
		t.Error("a workspace tab carrying builder:owner was accepted")
	}
	if !objExists(t, remotetermobj.OType_Tab, spoofed.OID) {
		t.Fatal("workspace tab deleted")
	}

	tab := makeTestBuilderTab(t, owner)
	block := addTestTermBlock(t, tab.OID)
	if err := DeleteBuilderTab(ctx, tab.OID, uuid.NewString()); err == nil {
		t.Error("owner mismatch accepted")
	}
	if !objExists(t, remotetermobj.OType_Tab, tab.OID) || !objExists(t, remotetermobj.OType_Block, block.OID) {
		t.Fatal("owner mismatch deleted something")
	}
}

func TestDeleteBuilderTabKeepsTabWhenABlockFails(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := makeTestBuilderTab(t, owner)
	good := addTestTermBlock(t, tab.OID)
	bad := addTestTermBlock(t, tab.OID)
	// A sub-block id that no longer exists makes deleteBlockObj refuse the parent.
	bad.SubBlockIds = []string{uuid.NewString()}
	if err := rtstore.DBUpdate(ctx, bad); err != nil {
		t.Fatal(err)
	}

	if err := DeleteBuilderTab(ctx, tab.OID, owner); err == nil {
		t.Fatal("expected an error from the failing block")
	}
	if !objExists(t, remotetermobj.OType_Tab, tab.OID) || !objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Fatal("tab or layout state deleted although a block failed")
	}
	if objExists(t, remotetermobj.OType_Block, good.OID) || !objExists(t, remotetermobj.OType_Block, bad.OID) {
		t.Fatal("expected the good block gone and the failing block kept")
	}

	bad.SubBlockIds = nil
	if err := rtstore.DBUpdate(ctx, bad); err != nil {
		t.Fatal(err)
	}
	SweepBuilderTabs(ctx)
	if objExists(t, remotetermobj.OType_Tab, tab.OID) {
		t.Fatal("the sweep did not remove the tab after the block was fixed")
	}
}

func TestDeleteBuilderTabKeepsTabThatGainedABlock(t *testing.T) {
	ctx := context.Background()
	owner := uuid.NewString()
	tab := makeTestBuilderTab(t, owner)
	block := addTestTermBlock(t, tab.OID)
	var late *remotetermobj.Block
	// A creator that does not hold the builder lock adds a pane while the teardown is running.
	blockCloses.setHook(func(blockId string) {
		if blockId != block.OID || late != nil {
			return
		}
		late = addTestTermBlock(t, tab.OID)
	})
	t.Cleanup(func() { blockCloses.setHook(nil) })

	if err := DeleteBuilderTab(ctx, tab.OID, owner); err == nil {
		t.Fatal("expected an error: the tab gained a block during the teardown")
	}
	blockCloses.setHook(nil)
	if late == nil {
		t.Fatal("the hook never ran")
	}
	if !objExists(t, remotetermobj.OType_Tab, tab.OID) || !objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Fatal("the tab was deleted under a live block")
	}
	if !objExists(t, remotetermobj.OType_Block, late.OID) {
		t.Fatal("the late block is gone")
	}
	SweepBuilderTabs(ctx)
	if objExists(t, remotetermobj.OType_Tab, tab.OID) || objExists(t, remotetermobj.OType_Block, late.OID) {
		t.Fatal("the sweep did not remove the tab and its late block")
	}
}

func TestSweepBuilderTabsRemovesBuilderTabsOnly(t *testing.T) {
	ctx := context.Background()
	makeTestBuilderTab(t, uuid.NewString())
	withBlock := makeTestBuilderTab(t, uuid.NewString())
	block := addTestTermBlock(t, withBlock.OID)
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: uuid.NewString()})
	insertTestWorkspace(t, spoofed.OID)

	before, err := FindBuilderTabs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var logBuf bytes.Buffer
	origLog := log.Writer()
	log.SetOutput(&logBuf)
	removed := SweepBuilderTabs(ctx)
	log.SetOutput(origLog)

	if removed != len(before) {
		t.Errorf("removed = %d, want %d", removed, len(before))
	}
	wantLine := fmt.Sprintf("[startup] builder sweep: removed %d tabs", len(before))
	if !strings.Contains(logBuf.String(), wantLine) {
		t.Errorf("log %q lacks %q", logBuf.String(), wantLine)
	}
	after, _ := FindBuilderTabs(ctx, "")
	if len(after) != 0 {
		t.Errorf("builder tabs left: %v", tabIdsOf(after))
	}
	if !blockCloses.closed(block.OID) {
		t.Error("no BlockClose for a swept block")
	}
	if !objExists(t, remotetermobj.OType_Tab, spoofed.OID) {
		t.Error("the sweep deleted a workspace tab")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go test ./pkg/rtcore/... -count=1`
Expected: the build fails because `CreateBuilderTab` and its siblings do not exist yet (for example `undefined: CreateBuilderTab`).

- [ ] **Step 4: Implement**

Create `pkg/rtcore/buildertab.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"fmt"
	"log"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

const BuilderTabName = "builder"

// The owner meta is written here, at insert: rtstore.UpdateObjectMeta refuses builder: keys on tabs.
func CreateBuilderTab(ctx context.Context, builderId string, appId string) (*remotetermobj.Tab, error) {
	if builderId == "" || appId == "" {
		return nil, fmt.Errorf("a builder tab needs a builder id and an app id")
	}
	layoutState := &remotetermobj.LayoutState{OID: uuid.NewString()}
	tab := &remotetermobj.Tab{
		OID:         uuid.NewString(),
		Name:        BuilderTabName,
		BlockIds:    []string{},
		LayoutState: layoutState.OID,
		Meta: remotetermobj.MetaMapType{
			rtstore.MetaKey_BuilderOwner: builderId,
			rtstore.MetaKey_BuilderAppId: appId,
		},
	}
	err := rtstore.WithTx(ctx, func(tx *rtstore.TxWrap) error {
		if err := rtstore.DBInsert(tx.Context(), tab); err != nil {
			return err
		}
		return rtstore.DBInsert(tx.Context(), layoutState)
	})
	if err != nil {
		return nil, fmt.Errorf("error creating builder tab: %w", err)
	}
	return tab, nil
}

func FindBuilderTabs(ctx context.Context, builderId string) ([]*remotetermobj.Tab, error) {
	tabIds, err := rtstore.DBFindTabIdsByBuilderOwner(ctx, builderId)
	if err != nil {
		return nil, fmt.Errorf("error finding builder tabs: %w", err)
	}
	rtn := make([]*remotetermobj.Tab, 0, len(tabIds))
	for _, tabId := range tabIds {
		if !rtstore.IsBuilderTab(ctx, tabId) {
			continue
		}
		tab, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tabId)
		if err != nil || tab == nil {
			continue
		}
		rtn = append(rtn, tab)
	}
	return rtn, nil
}

// A missing tab is a no-op. If any block cannot be deleted, the tab and its layout state are kept,
// so a later delete or the startup sweep can retry. Creators that skip the builder lock (wsh's
// CreateBlockCommand) can add a block mid-teardown; the final transaction re-reads the tab and keeps
// it in that case, rather than leaving the new block without a tab.
func DeleteBuilderTab(ctx context.Context, tabId string, expectedOwner string) error {
	tab, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil {
		return fmt.Errorf("error getting tab %s: %w", tabId, err)
	}
	if tab == nil {
		return nil
	}
	if !rtstore.IsBuilderTab(ctx, tabId) {
		return fmt.Errorf("tab %s is not a builder tab", tabId)
	}
	if expectedOwner != "" && tab.Meta.GetString(rtstore.MetaKey_BuilderOwner, "") != expectedOwner {
		return fmt.Errorf("tab %s belongs to another builder", tabId)
	}
	var firstErr error
	for _, blockId := range tab.BlockIds {
		err := DeleteBlock(ctx, blockId, false)
		if err == nil {
			continue
		}
		log.Printf("DeleteBuilderTab: error deleting block %s of tab %s: %v\n", blockId, tabId, err)
		if firstErr == nil {
			firstErr = fmt.Errorf("error deleting block %s: %w", blockId, err)
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return rtstore.WithTx(ctx, func(tx *rtstore.TxWrap) error {
		current, err := rtstore.DBGet[*remotetermobj.Tab](tx.Context(), tabId)
		if err != nil {
			return err
		}
		if current == nil {
			return nil
		}
		if len(current.BlockIds) > 0 {
			return fmt.Errorf("tab %s gained blocks while it was being deleted", tabId)
		}
		if err := rtstore.DBDelete(tx.Context(), remotetermobj.OType_LayoutState, current.LayoutState); err != nil {
			return err
		}
		return rtstore.DBDelete(tx.Context(), remotetermobj.OType_Tab, tabId)
	})
}

// Builder ids are fresh per window and a backend exit quits the app, so at startup every builder
// tab is an orphan. Per-tab errors are logged and do not stop the sweep.
func SweepBuilderTabs(ctx context.Context) int {
	tabs, err := FindBuilderTabs(ctx, "")
	if err != nil {
		log.Printf("[startup] builder sweep: %v\n", err)
	}
	removed := 0
	for _, tab := range tabs {
		if err := DeleteBuilderTab(ctx, tab.OID, ""); err != nil {
			log.Printf("[startup] builder sweep: could not remove tab %s: %v\n", tab.OID, err)
			continue
		}
		removed++
	}
	log.Printf("[startup] builder sweep: removed %d tabs\n", removed)
	return removed
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/rtcore/... -count=1 && go test -race ./pkg/rtcore/... -count=1`
Expected: `ok  	github.com/LannCo/remoteterm/pkg/rtcore` twice.

- [ ] **Step 6: Commit**

```bash
git add pkg/rtcore/buildertab.go pkg/rtcore/rtcore_fixture_test.go pkg/rtcore/buildertab_test.go
git commit -m "feat(builder): server-owned builder tabs with delete and startup sweep

A builder window's terminals will live in a tab that joins no workspace;
deleting it removes every block (each publishes BlockClose, so shells
stop), and a sweep removes every such tab left behind by a crash.

Miscellanea: rtcore.CreateBuilderTab/FindBuilderTabs/DeleteBuilderTab/
SweepBuilderTabs; DB-backed rtcore test fixture with a BlockClose recorder.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Block deletion cascade, workspace membership and local-only block creation (`pkg/rtcore`)

**Files:**
- Modify: `pkg/rtcore/block.go:20-32` (`CreateSubBlock`), `:54-72` (`CreateBlock`), `:119-157` (`DeleteBlock`)
- Modify: `pkg/rtcore/workspace.go:453-461` (`UpdateWorkspaceTabIds`)
- Test: `pkg/rtcore/block_builder_test.go`

**Interfaces:**
- Consumes (Task 1): `rtstore.IsBuilderTab`, `rtstore.CheckBuilderTabConnection`, `rtstore.CheckBuilderBlockConnection`, `rtstore.ErrBuilderLocalOnly`, `rtstore.MetaKey_BuilderOwner`. (Task 2): `makeTestBuilderTab`, `addTestTermBlock`, `insertTestTab`, `insertTestWorkspace`, `objExists`, `blockCloses` from the rtcore test fixture.
- Produces: `DeleteBlock(ctx, blockId, recursive)` keeps the signature; it now publishes BlockClose immediately after the block row is deleted, and with `recursive == true` it never deletes a builder tab. `CreateBlock`/`CreateSubBlock` return `rtstore.ErrBuilderLocalOnly` for a non-local `connection` in a builder tab. `UpdateWorkspaceTabIds` refuses a builder-owned tab that is not already in the workspace.

- [ ] **Step 1: Write the failing tests**

Create `pkg/rtcore/block_builder_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

func TestDeleteBlockLastBlockOfBuilderTabKeepsTab(t *testing.T) {
	ctx := context.Background()
	tab := makeTestBuilderTab(t, uuid.NewString())
	block := addTestTermBlock(t, tab.OID)
	if err := DeleteBlock(ctx, block.OID, true); err != nil {
		t.Fatalf("DeleteBlock = %v, want nil", err)
	}
	if !blockCloses.closed(block.OID) {
		t.Error("no BlockClose for the last builder pane")
	}
	got, _ := rtstore.DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if got == nil || len(got.BlockIds) != 0 {
		t.Fatalf("builder tab = %+v, want present with no blocks", got)
	}
	if !objExists(t, remotetermobj.OType_LayoutState, tab.LayoutState) {
		t.Error("layout state deleted")
	}
}

func TestDeleteBlockPublishesBlockCloseWhenCascadeFails(t *testing.T) {
	ctx := context.Background()
	orphan := insertTestTab(t, nil)
	block := addTestTermBlock(t, orphan.OID)
	if err := DeleteBlock(ctx, block.OID, true); err == nil {
		t.Fatal("expected the cascade to fail for a tab that belongs to no workspace")
	}
	if objExists(t, remotetermobj.OType_Block, block.OID) {
		t.Error("block row survived")
	}
	if !blockCloses.closed(block.OID) {
		t.Fatal("a cascade error skipped BlockClose, so the shell would keep running")
	}
}

func TestUpdateWorkspaceTabIdsRejectsBuilderTab(t *testing.T) {
	ctx := context.Background()
	normal := insertTestTab(t, nil)
	ws := insertTestWorkspace(t, normal.OID)
	builderTab := makeTestBuilderTab(t, uuid.NewString())
	if err := UpdateWorkspaceTabIds(ctx, ws.OID, []string{normal.OID, builderTab.OID}); err == nil {
		t.Fatal("a builder tab joined a workspace")
	}
	got, _ := rtstore.DBGet[*remotetermobj.Workspace](ctx, ws.OID)
	if !slices.Equal(got.TabIds, []string{normal.OID}) {
		t.Fatalf("workspace tabids = %v", got.TabIds)
	}
	if !rtstore.IsBuilderTab(ctx, builderTab.OID) {
		t.Error("builder tab lost its builder status")
	}
}

func TestUpdateWorkspaceTabIdsKeepsReorderingWithSpoofedTab(t *testing.T) {
	ctx := context.Background()
	normal := insertTestTab(t, nil)
	spoofed := insertTestTab(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: uuid.NewString()})
	ws := insertTestWorkspace(t, normal.OID, spoofed.OID)
	if err := UpdateWorkspaceTabIds(ctx, ws.OID, []string{spoofed.OID, normal.OID}); err != nil {
		t.Fatalf("reorder rejected: %v", err)
	}
	got, _ := rtstore.DBGet[*remotetermobj.Workspace](ctx, ws.OID)
	if !slices.Equal(got.TabIds, []string{spoofed.OID, normal.OID}) {
		t.Fatalf("workspace tabids = %v", got.TabIds)
	}
}

func TestCreateBlockRejectsRemoteConnectionInBuilderTab(t *testing.T) {
	ctx := context.Background()
	tab := makeTestBuilderTab(t, uuid.NewString())
	remoteDef := func() *remotetermobj.BlockDef {
		return &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
			remotetermobj.MetaKey_View:       "term",
			remotetermobj.MetaKey_Controller: "shell",
			remotetermobj.MetaKey_Connection: "user@host",
		}}
	}
	localDef := func() *remotetermobj.BlockDef {
		return &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
			remotetermobj.MetaKey_View:       "term",
			remotetermobj.MetaKey_Controller: "shell",
			remotetermobj.MetaKey_Connection: "local",
		}}
	}

	if _, err := CreateBlock(ctx, tab.OID, remoteDef(), nil); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Fatalf("CreateBlock remote = %v, want ErrBuilderLocalOnly", err)
	}
	got, _ := rtstore.DBGet[*remotetermobj.Tab](ctx, tab.OID)
	if len(got.BlockIds) != 0 {
		t.Fatalf("a rejected block was added: %v", got.BlockIds)
	}
	parent, err := CreateBlock(ctx, tab.OID, localDef(), nil)
	if err != nil {
		t.Fatalf("CreateBlock local = %v", err)
	}
	if _, err := CreateSubBlock(ctx, parent.OID, remoteDef()); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Fatalf("CreateSubBlock remote = %v, want ErrBuilderLocalOnly", err)
	}
	if _, err := CreateSubBlock(ctx, parent.OID, localDef()); err != nil {
		t.Fatalf("CreateSubBlock local = %v", err)
	}

	wsTab := insertTestTab(t, nil)
	insertTestWorkspace(t, wsTab.OID)
	if _, err := CreateBlock(ctx, wsTab.OID, remoteDef(), nil); err != nil {
		t.Fatalf("remote block rejected in a workspace tab: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go test ./pkg/rtcore/... -count=1 -run 'TestDeleteBlock|TestUpdateWorkspaceTabIds|TestCreateBlockRejects'`
Expected: FAIL. `TestDeleteBlockLastBlockOfBuilderTabKeepsTab` fails with `DeleteBlock = error deleting tab ...: workspace not found: "", want nil`; `TestDeleteBlockPublishesBlockCloseWhenCascadeFails` fails with `a cascade error skipped BlockClose`; `TestUpdateWorkspaceTabIdsRejectsBuilderTab` fails with `a builder tab joined a workspace`; `TestCreateBlockRejectsRemoteConnectionInBuilderTab` fails with `CreateBlock remote = <nil>`. `TestUpdateWorkspaceTabIdsKeepsReorderingWithSpoofedTab` passes already.

- [ ] **Step 3: Implement the local-only guard**

In `pkg/rtcore/block.go`, `CreateSubBlock`: after the `no view provided for new block` check (line 25-26) insert:

```go
	if err := rtstore.CheckBuilderBlockConnection(ctx, blockId, blockDef.Meta); err != nil {
		return nil, err
	}
```

In `CreateBlock`: after the `no view provided for new block` check (lines 70-72) insert:

```go
	if err := rtstore.CheckBuilderTabConnection(ctx, tabId, blockDef.Meta); err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Implement the BlockClose move and the cascade skip**

In `DeleteBlock`, replace lines 135-156 (from `parentBlockCount, err := deleteBlockObj(ctx, blockId)` through the final `sendBlockCloseEvent(blockId)`) with:

```go
	parentBlockCount, err := deleteBlockObj(ctx, blockId)
	if err != nil {
		return fmt.Errorf("error deleting block: %w", err)
	}
	// Published before any cascade step, so a later error cannot leave the block's controller (and its shell) running.
	sendBlockCloseEvent(blockId)
	log.Printf("DeleteBlock: parentBlockCount: %d", parentBlockCount)
	parentORef := remotetermobj.ParseORefNoErr(block.ParentORef)

	// A builder tab outlives its last pane: the builder panel shows an empty state, and the builder deletes the tab.
	if recursive && parentORef != nil && parentORef.OType == remotetermobj.OType_Tab && parentBlockCount == 0 && !rtstore.IsBuilderTab(ctx, parentORef.OID) {
		// if parent tab has no blocks, delete the tab
		log.Printf("DeleteBlock: parent tab has no blocks, deleting tab %s", parentORef.OID)
		parentWorkspaceId, err := rtstore.DBFindWorkspaceForTabId(ctx, parentORef.OID)
		if err != nil {
			return fmt.Errorf("error finding workspace for tab to delete %s: %w", parentORef.OID, err)
		}
		newActiveTabId, err := DeleteTab(ctx, parentWorkspaceId, parentORef.OID, true)
		if err != nil {
			return fmt.Errorf("error deleting tab %s: %w", parentORef.OID, err)
		}
		SendActiveTabUpdate(ctx, parentWorkspaceId, newActiveTabId)
	}
	return nil
}
```

- [ ] **Step 5: Implement the workspace guard**

In `pkg/rtcore/workspace.go`, replace `UpdateWorkspaceTabIds` (lines 453-461) with:

```go
func UpdateWorkspaceTabIds(ctx context.Context, workspaceId string, tabIds []string) error {
	ws, _ := rtstore.DBGet[*remotetermobj.Workspace](ctx, workspaceId)
	if ws == nil {
		return fmt.Errorf("workspace not found: %q", workspaceId)
	}
	if err := checkNoNewBuilderTabs(ctx, ws, tabIds); err != nil {
		return err
	}
	ws.TabIds = tabIds
	rtstore.DBUpdate(ctx, ws)
	return nil
}

// A tab already in this workspace can carry builder:owner only through a direct DB write; it stays
// reorderable and is never treated as a builder tab. Any other tab with an owner must stay out, and a
// failed lookup refuses the update.
func checkNoNewBuilderTabs(ctx context.Context, ws *remotetermobj.Workspace, tabIds []string) error {
	for _, tabId := range tabIds {
		if slices.Contains(ws.TabIds, tabId) {
			continue
		}
		tab, err := rtstore.DBGet[*remotetermobj.Tab](ctx, tabId)
		if err != nil {
			return fmt.Errorf("cannot check tab %s: %w", tabId, err)
		}
		if tab != nil && tab.Meta.GetString(rtstore.MetaKey_BuilderOwner, "") != "" {
			return fmt.Errorf("tab %s belongs to a builder window and cannot join a workspace", tabId)
		}
	}
	return nil
}
```

(`slices` is already imported in `workspace.go`, line 13.)

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./pkg/rtcore/... -count=1 && go test -race ./pkg/rtcore/... -count=1`
Expected: `ok  	github.com/LannCo/remoteterm/pkg/rtcore` twice.

- [ ] **Step 7: Run the dependants**

Run: `go test ./pkg/... -count=1 2>&1 | grep -v '^ok\|no test files'`
Expected: no new failures compared with the baseline: no output. Here `grep -v` exiting 1 with no output is the success case (nothing but `ok` and `[no test files]` lines).

- [ ] **Step 8: Commit**

```bash
git add pkg/rtcore/block.go pkg/rtcore/workspace.go pkg/rtcore/block_builder_test.go
git commit -m "fix(blocks): closing a pane always stops its shell; builder tabs stay out of workspaces

Deleting a block now publishes BlockClose before any tab cascade, so a
cascade error can no longer leave the shell running. Closing the last
pane of a builder tab keeps the tab, builder tabs cannot be added to a
workspace, and builder panes cannot be created with a remote connection.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Pane environment, app-dir resolver and durable pin (`pkg/blockcontroller`, `pkg/buildercontroller`)

**Files:**
- Modify: `pkg/blockcontroller/blockcontroller.go:608-630` (`makeSwapToken`; new `addTabAndWorkspaceEnv`)
- Test: `pkg/blockcontroller/builderenv_test.go` (new; adds a `TestMain` DB fixture to this package)
- Modify: `pkg/buildercontroller/appdir.go:14-47`
- Test: `pkg/buildercontroller/appdir_test.go` (append)

**Interfaces:**
- Consumes (Task 1): `rtstore.MetaKey_BuilderOwner` (test data only).
- Produces:
  - `blockcontroller.addTabAndWorkspaceEnv(ctx context.Context, env map[string]string, blockId string)` (package-private)
  - `buildercontroller.ResolveAppDirForAppId(appId string) (string, error)` (never reads rtinfo; `ResolveBuilderAppDir` delegates to it)
  - `buildercontroller.MakeBuilderTerminalBlockDef(appDir string)` now also sets `remotetermobj.MetaKey_TermDurable` (`"term:durable"`) to `false`

- [ ] **Step 1: Write the failing tests**

Create `pkg/blockcontroller/builderenv_test.go` (fixture pattern as `pkg/jobcontroller/jobcontroller_reconnect_test.go:98-136`, run once from `TestMain`; this package had no `TestMain`, and its existing tests do not touch the store):

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "blockcontroller-test-*")
	if err != nil {
		fmt.Printf("cannot create a test data dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, remotetermbase.WaveDBDir), 0755); err != nil {
		fmt.Printf("cannot create the db dir: %v\n", err)
		os.Exit(1)
	}
	remotetermbase.DataHome_VarCache = dir
	if err := rtstore.InitWStore(); err != nil {
		fmt.Printf("wstore: %v\n", err)
		os.Exit(1)
	}
	if err := filestore.InitFilestore(); err != nil {
		fmt.Printf("filestore: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func insertEnvTestTabAndBlock(t *testing.T, meta remotetermobj.MetaMapType) (*remotetermobj.Tab, *remotetermobj.Block) {
	t.Helper()
	ctx := context.Background()
	tab := &remotetermobj.Tab{OID: uuid.NewString(), BlockIds: []string{}, LayoutState: uuid.NewString(), Meta: meta}
	if err := rtstore.DBInsert(ctx, tab); err != nil {
		t.Fatal(err)
	}
	block := &remotetermobj.Block{
		OID:        uuid.NewString(),
		ParentORef: remotetermobj.MakeORef(remotetermobj.OType_Tab, tab.OID).String(),
		Meta:       remotetermobj.MetaMapType{"view": "term"},
	}
	if err := rtstore.DBInsert(ctx, block); err != nil {
		t.Fatal(err)
	}
	return tab, block
}

func TestAddTabAndWorkspaceEnvOmitsEmptyWorkspaceId(t *testing.T) {
	ctx := context.Background()
	builderTab, builderBlock := insertEnvTestTabAndBlock(t, remotetermobj.MetaMapType{rtstore.MetaKey_BuilderOwner: uuid.NewString()})
	env := map[string]string{}
	addTabAndWorkspaceEnv(ctx, env, builderBlock.OID)
	if env[remotetermbase.WaveTabIdVarName] != builderTab.OID || env[remotetermbase.LegacyWaveTabIdVarName] != builderTab.OID {
		t.Errorf("tab id env = %v", env)
	}
	for _, name := range []string{remotetermbase.WaveWorkspaceIdVarName, remotetermbase.LegacyWaveWorkspaceIdVarName} {
		if val, ok := env[name]; ok {
			t.Errorf("%s set to %q for a builder pane", name, val)
		}
	}

	wsTab, wsBlock := insertEnvTestTabAndBlock(t, nil)
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: []string{wsTab.OID}}
	if err := rtstore.DBInsert(ctx, ws); err != nil {
		t.Fatal(err)
	}
	wsEnv := map[string]string{}
	addTabAndWorkspaceEnv(ctx, wsEnv, wsBlock.OID)
	if wsEnv[remotetermbase.WaveWorkspaceIdVarName] != ws.OID || wsEnv[remotetermbase.LegacyWaveWorkspaceIdVarName] != ws.OID {
		t.Errorf("workspace env = %v, want %s", wsEnv, ws.OID)
	}
}
```

Append to `pkg/buildercontroller/appdir_test.go`:

```go
func TestResolveAppDirForAppIdIgnoresRtInfo(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	setBuilderAppId(t, "test-appdir-other", "draft/other")
	got, err := ResolveAppDirForAppId("draft/demo")
	if err != nil || got != appDir {
		t.Fatalf("ResolveAppDirForAppId = %q, %v; want %q", got, err, appDir)
	}
}

func TestResolveAppDirForAppIdRejectsBadIdsAndFolders(t *testing.T) {
	home, _ := setupBuilderTest(t)
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "waveapps", "draft", "afile"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, appId := range []string{"", "demo", "draft/../../etc", "Draft/demo", "draft/nothere", "draft/afile"} {
		if got, err := ResolveAppDirForAppId(appId); err == nil {
			t.Errorf("ResolveAppDirForAppId(%q) = %q, want an error", appId, got)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, "waveapps", "draft", "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveAppDirForAppId("draft/linked"); err == nil {
		t.Error("a symlinked app folder was accepted")
	}
}

func TestMakeBuilderTerminalBlockDefPinsDurableOff(t *testing.T) {
	def := MakeBuilderTerminalBlockDef("/home/u/waveapps/draft/demo")
	durable, ok := def.Meta[remotetermobj.MetaKey_TermDurable].(bool)
	if !ok || durable {
		t.Fatalf("meta[%q] = %#v, want false", remotetermobj.MetaKey_TermDurable, def.Meta[remotetermobj.MetaKey_TermDurable])
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go test ./pkg/blockcontroller/... ./pkg/buildercontroller/... -count=1`
Expected: both packages fail to build because `addTabAndWorkspaceEnv` and `ResolveAppDirForAppId` do not exist yet.

- [ ] **Step 3: Implement the env helper**

In `pkg/blockcontroller/blockcontroller.go`, inside `makeSwapToken`, replace lines 618-630 (from `tabId, err := rtstore.DBFindTabForBlockId(ctx, blockId)` through the closing brace of `if tabId != "" { ... }`) with:

```go
	addTabAndWorkspaceEnv(ctx, token.Env, blockId)
```

and add after `makeSwapToken`:

```go
// A builder pane's tab joins no workspace; an empty workspace id is left unset rather than exported empty.
func addTabAndWorkspaceEnv(ctx context.Context, env map[string]string, blockId string) {
	tabId, err := rtstore.DBFindTabForBlockId(ctx, blockId)
	if err != nil {
		log.Printf("error finding tab for block: %v\n", err)
		return
	}
	remotetermbase.SetDualEnv(env, remotetermbase.WaveTabIdVarName, remotetermbase.LegacyWaveTabIdVarName, tabId)
	if tabId == "" {
		return
	}
	wsId, err := rtstore.DBFindWorkspaceForTabId(ctx, tabId)
	if err != nil {
		log.Printf("error finding workspace for tab: %v\n", err)
		return
	}
	if wsId == "" {
		return
	}
	remotetermbase.SetDualEnv(env, remotetermbase.WaveWorkspaceIdVarName, remotetermbase.LegacyWaveWorkspaceIdVarName, wsId)
}
```

- [ ] **Step 4: Implement the resolver and the durable pin**

Replace `pkg/buildercontroller/appdir.go` lines 14-47 (keep the existing comment above `ResolveBuilderAppDir`) with:

```go
// The folder always comes from the builder's rtinfo, never from the renderer, so a
// compromised page cannot point a terminal or a file manager somewhere else.
func ResolveBuilderAppDir(builderId string) (string, error) {
	appId, _, err := GetBuilderRebuildInputs(builderId)
	if err != nil {
		return "", err
	}
	return ResolveAppDirForAppId(appId)
}

// Builder terminals resolve their folder from the app id stored on their tab, never from rtinfo,
// which any pane can rewrite through SetRTInfoCommand.
func ResolveAppDirForAppId(appId string) (string, error) {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return "", err
	}
	if err := remotetermappstore.CheckNoSymlinks(appDir); err != nil {
		return "", err
	}
	info, err := os.Lstat(appDir)
	if err != nil {
		return "", fmt.Errorf("app folder %s is not available: %w", appDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("app folder %s is not a directory", appDir)
	}
	return appDir, nil
}

func MakeBuilderTerminalBlockDef(appDir string) *remotetermobj.BlockDef {
	return &remotetermobj.BlockDef{
		Meta: remotetermobj.MetaMapType{
			remotetermobj.MetaKey_View:        "term",
			remotetermobj.MetaKey_Controller:  "shell",
			remotetermobj.MetaKey_Connection:  "local",
			remotetermobj.MetaKey_CmdCwd:      appDir,
			remotetermobj.MetaKey_TermDurable: false,
		},
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/blockcontroller/... ./pkg/buildercontroller/... -count=1 && go test -race ./pkg/blockcontroller/... -count=1`
Expected: `ok` for both packages, then `ok` for `pkg/blockcontroller` under `-race`.

The test drives the helper, so also check that `makeSwapToken` really uses it and kept no lookup of its own:

```bash
awk '/^func makeSwapToken\(/,/^}/' pkg/blockcontroller/blockcontroller.go | grep -c 'addTabAndWorkspaceEnv(ctx, token.Env, blockId)'
awk '/^func makeSwapToken\(/,/^}/' pkg/blockcontroller/blockcontroller.go | grep -c 'DBFindTabForBlockId\|DBFindWorkspaceForTabId'
grep -c 'DBFindTabForBlockId\|DBFindWorkspaceForTabId' pkg/blockcontroller/blockcontroller.go
awk '/^func addTabAndWorkspaceEnv\(/,/^}/' pkg/blockcontroller/blockcontroller.go | grep -c 'DBFindTabForBlockId\|DBFindWorkspaceForTabId'
```

Expected: `1`, `0`, `2`, `2` (the two lookups exist only inside the helper).

- [ ] **Step 6: Commit**

```bash
git add pkg/blockcontroller/blockcontroller.go pkg/blockcontroller/builderenv_test.go pkg/buildercontroller/appdir.go pkg/buildercontroller/appdir_test.go
git commit -m "feat(builder): builder panes start in the stored app folder, never durable

A builder terminal's folder is resolved from an app id the server keeps,
not from rtinfo a pane could rewrite; builder terminals are pinned
non-durable; and a pane outside any workspace no longer gets an empty
REMOTETERM_WORKSPACEID.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Caller check, keyed lock and write context (`pkg/wshutil`, `pkg/wshrpc/wshserver`)

**Files:**
- Modify: `pkg/wshutil/wshrpc.go` (add `MakeRpcSourceContextForTest` after `GetRpcSourceFromContext`, lines 85-91)
- Test: `pkg/wshutil/wshrpc_test.go` (new)
- Create: `pkg/wshrpc/wshserver/builderterm.go`
- Test: `pkg/wshrpc/wshserver/builderterm_unit_test.go` (new; pure, no DB)

**Interfaces:**
- Consumes: `wshutil.ElectronRoute` (`pkg/wshutil/wshrouter.go:30`), `wshutil.MakeBuilderRouteId` (`:139-141`), `rtcore.LayoutActionDataType_*` (`pkg/rtcore/layout.go:17-26`).
- Produces:
  - `wshutil.MakeRpcSourceContextForTest(ctx context.Context, source string) context.Context`
  - package `wshserver`: consts `MaxBuilderTermBlocks = 16`, `BuilderWriteTimeout = 15 * time.Second`, `BuilderTargetAction_SplitRight/SplitLeft/SplitUp/SplitDown` (`"splitright"`, `"splitleft"`, `"splitup"`, `"splitdown"`); `checkBuilderCaller(source string, builderId string, allowRenderer bool) error`; `getBuilderLock(builderId string) *sync.Mutex`; `withBuilderLock(builderId string, fn func() error) error`; `makeBuilderWriteContext() (context.Context, context.CancelFunc)`; `isValidBuilderTargetAction(targetAction string) bool`; `makeBuilderLayoutAction(blockId string, targetBlockId string, targetAction string) remotetermobj.LayoutActionData`

The source a handler sees is `RpcResponseHandler.source` (`pkg/wshutil/wshrpc.go:85-91`, `:624-656`); the router overwrites it for leaf links (`pkg/wshutil/wshrouter.go:566-568`), and the frontend and Electron clients send their own route id (`frontend/app/store/wshclient.ts:33, 54, 83`; `emain/emain-wsh.ts:15`).

- [ ] **Step 1: Write the failing tests**

Create `pkg/wshutil/wshrpc_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshutil

import (
	"context"
	"testing"
)

func TestMakeRpcSourceContextForTest(t *testing.T) {
	if got := GetRpcSourceFromContext(context.Background()); got != "" {
		t.Fatalf("source of a bare context = %q", got)
	}
	ctx := MakeRpcSourceContextForTest(context.Background(), ElectronRoute)
	if got := GetRpcSourceFromContext(ctx); got != ElectronRoute {
		t.Fatalf("source = %q, want %q", got, ElectronRoute)
	}
	if GetIsCanceledFromContext(ctx) {
		t.Fatal("a test context reports cancelled")
	}
}
```

Create `pkg/wshrpc/wshserver/builderterm_unit_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func TestCheckBuilderCaller(t *testing.T) {
	builderId := uuid.NewString()
	other := uuid.NewString()
	cases := []struct {
		source        string
		allowRenderer bool
		ok            bool
	}{
		{wshutil.ElectronRoute, false, true},
		{wshutil.ElectronRoute, true, true},
		{wshutil.MakeBuilderRouteId(builderId), true, true},
		{wshutil.MakeBuilderRouteId(builderId), false, false},
		{wshutil.MakeBuilderRouteId(other), true, false},
		{wshutil.MakeProcRouteId(uuid.NewString()), true, false},
		{wshutil.MakeControllerRouteId(uuid.NewString()), true, false},
		{wshutil.MakeConnectionRouteId("user@host"), true, false},
		{wshutil.MakeTabRouteId(uuid.NewString()), true, false},
		{wshutil.MakeFeBlockRouteId(uuid.NewString()), true, false},
		{"", true, false},
	}
	for _, tc := range cases {
		err := checkBuilderCaller(tc.source, builderId, tc.allowRenderer)
		if (err == nil) != tc.ok {
			t.Errorf("checkBuilderCaller(%q, allowRenderer=%v) = %v, want ok=%v", tc.source, tc.allowRenderer, err, tc.ok)
		}
	}
	for _, bad := range []string{"", "not-a-uuid", strings.ToUpper(builderId), "{" + builderId + "}", "urn:uuid:" + builderId} {
		if err := checkBuilderCaller(wshutil.ElectronRoute, bad, true); err == nil {
			t.Errorf("builder id %q accepted", bad)
		}
	}
}

func TestBuilderLockIsPerBuilder(t *testing.T) {
	a := uuid.NewString()
	if getBuilderLock(a) != getBuilderLock(a) {
		t.Fatal("the same builder got two locks")
	}
	if getBuilderLock(a) == getBuilderLock(uuid.NewString()) {
		t.Fatal("two builders share a lock")
	}
}

func TestWithBuilderLockSerialises(t *testing.T) {
	builderId := uuid.NewString()
	var inside, maxInside atomic.Int32
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			withBuilderLock(builderId, func() error {
				n := inside.Add(1)
				for {
					m := maxInside.Load()
					if n <= m || maxInside.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				inside.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()
	if maxInside.Load() != 1 {
		t.Fatalf("%d callers inside the lock at once", maxInside.Load())
	}
}

func TestMakeBuilderWriteContextIsDetachedAndTracksUpdates(t *testing.T) {
	ctx, cancelFn := makeBuilderWriteContext()
	defer cancelFn()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > BuilderWriteTimeout || time.Until(deadline) < BuilderWriteTimeout-time.Second {
		t.Fatalf("deadline = %v (ok=%v), want about %v from now", deadline, ok, BuilderWriteTimeout)
	}
	if remotetermobj.ContextGetUpdates(ctx) == nil {
		t.Fatal("write context does not collect object updates")
	}
}

func TestIsValidBuilderTargetAction(t *testing.T) {
	for _, ok := range []string{"", "splitright", "splitleft", "splitup", "splitdown"} {
		if !isValidBuilderTargetAction(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"replace", "SPLITRIGHT", "split", "insert"} {
		if isValidBuilderTargetAction(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestMakeBuilderLayoutAction(t *testing.T) {
	insert := makeBuilderLayoutAction("b2", "", "")
	if insert.ActionType != rtcore.LayoutActionDataType_Insert || insert.BlockId != "b2" || !insert.Focused || insert.TargetBlockId != "" {
		t.Errorf("insert = %+v", insert)
	}
	cases := map[string][2]string{
		"splitright": {rtcore.LayoutActionDataType_SplitHorizontal, "after"},
		"splitleft":  {rtcore.LayoutActionDataType_SplitHorizontal, "before"},
		"splitup":    {rtcore.LayoutActionDataType_SplitVertical, "before"},
		"splitdown":  {rtcore.LayoutActionDataType_SplitVertical, "after"},
	}
	for action, want := range cases {
		got := makeBuilderLayoutAction("b2", "b1", action)
		if got.ActionType != want[0] || got.Position != want[1] || got.TargetBlockId != "b1" || got.BlockId != "b2" || !got.Focused {
			t.Errorf("%s = %+v, want type %s position %s", action, got, want[0], want[1])
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go test ./pkg/wshutil/... ./pkg/wshrpc/wshserver/... -count=1`
Expected: both packages fail to build because `MakeRpcSourceContextForTest` and `checkBuilderCaller` do not exist yet.

- [ ] **Step 3: Implement the test helper**

In `pkg/wshutil/wshrpc.go`, after `GetRpcSourceFromContext` (ends line 91), add:

```go
// MakeRpcSourceContextForTest returns a context whose RPC source is source, as if the request had
// arrived on that route. Production code never calls it; handler tests use it to drive caller checks.
func MakeRpcSourceContextForTest(ctx context.Context, source string) context.Context {
	return withRespHandler(ctx, &RpcResponseHandler{
		source:   source,
		canceled: &atomic.Bool{},
		done:     &atomic.Bool{},
	})
}
```

(`sync/atomic` is already imported by `wshrpc.go`; `RpcResponseHandler` uses `*atomic.Bool`.)

- [ ] **Step 4: Implement the primitives**

Create `pkg/wshrpc/wshserver/builderterm.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

const (
	MaxBuilderTermBlocks = 16
	BuilderWriteTimeout  = 15 * time.Second

	BuilderTargetAction_SplitRight = "splitright"
	BuilderTargetAction_SplitLeft  = "splitleft"
	BuilderTargetAction_SplitUp    = "splitup"
	BuilderTargetAction_SplitDown  = "splitdown"
)

var (
	builderLocksLock sync.Mutex
	builderLocks     = make(map[string]*sync.Mutex)
)

// Panes reach the server through wsh on leaf links, whose source the router stamps as proc:<id>, so
// they cannot pass. Electron and the builder's own renderer are trusted links that assert their route.
func checkBuilderCaller(source string, builderId string, allowRenderer bool) error {
	parsed, err := uuid.Parse(builderId)
	if err != nil || parsed.String() != builderId {
		return fmt.Errorf("invalid builder id %q", builderId)
	}
	if source == wshutil.ElectronRoute {
		return nil
	}
	if allowRenderer && source == wshutil.MakeBuilderRouteId(builderId) {
		return nil
	}
	return fmt.Errorf("builder terminal commands are not available to %q", source)
}

// Entries are never removed: there is one per builder window per process, and the caller check stops
// panes minting ids. Removing them would let a waiter hold a lock that a newcomer no longer sees.
func getBuilderLock(builderId string) *sync.Mutex {
	builderLocksLock.Lock()
	defer builderLocksLock.Unlock()
	lock := builderLocks[builderId]
	if lock == nil {
		lock = &sync.Mutex{}
		builderLocks[builderId] = lock
	}
	return lock
}

func withBuilderLock(builderId string, fn func() error) error {
	lock := getBuilderLock(builderId)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

// Writes and their rollbacks run detached from the RPC context, so an expiring request cannot strand a
// half-created tab. The update map it carries is not goroutine-safe: use the context from one goroutine.
func makeBuilderWriteContext() (context.Context, context.CancelFunc) {
	ctx, cancelFn := context.WithTimeout(context.Background(), BuilderWriteTimeout)
	return remotetermobj.ContextWithUpdates(ctx), cancelFn
}

func isValidBuilderTargetAction(targetAction string) bool {
	switch targetAction {
	case "", BuilderTargetAction_SplitRight, BuilderTargetAction_SplitLeft, BuilderTargetAction_SplitUp, BuilderTargetAction_SplitDown:
		return true
	}
	return false
}

// The split strings and positions match CreateBlockCommand (wshserver.go), so wsh users and the builder
// get the same geometry.
func makeBuilderLayoutAction(blockId string, targetBlockId string, targetAction string) remotetermobj.LayoutActionData {
	action := remotetermobj.LayoutActionData{BlockId: blockId, Focused: true}
	switch targetAction {
	case BuilderTargetAction_SplitRight:
		action.ActionType = rtcore.LayoutActionDataType_SplitHorizontal
		action.Position = "after"
	case BuilderTargetAction_SplitLeft:
		action.ActionType = rtcore.LayoutActionDataType_SplitHorizontal
		action.Position = "before"
	case BuilderTargetAction_SplitUp:
		action.ActionType = rtcore.LayoutActionDataType_SplitVertical
		action.Position = "before"
	case BuilderTargetAction_SplitDown:
		action.ActionType = rtcore.LayoutActionDataType_SplitVertical
		action.Position = "after"
	default:
		action.ActionType = rtcore.LayoutActionDataType_Insert
		return action
	}
	action.TargetBlockId = targetBlockId
	return action
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/wshutil/... ./pkg/wshrpc/wshserver/... -count=1 && go test -race ./pkg/wshrpc/wshserver/... -count=1`
Expected: `ok` for `pkg/wshutil` and `pkg/wshrpc/wshserver`, then `ok` under `-race`.

- [ ] **Step 6: Commit**

```bash
git add pkg/wshutil/wshrpc.go pkg/wshutil/wshrpc_test.go pkg/wshrpc/wshserver/builderterm.go pkg/wshrpc/wshserver/builderterm_unit_test.go
git commit -m "feat(builder): caller check and per-builder lock for builder terminal RPCs

Only Electron (and, for teardown, the builder's own renderer) will be
able to create or remove builder terminals; panes using wsh are refused.

Miscellanea: wshutil.MakeRpcSourceContextForTest for handler tests;
keyed lock, detached write context and layout action mapping in
wshserver/builderterm.go.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: `EnsureBuilderTabCommand` and RPC-level guard coverage (`pkg/wshrpc`)

**Files:**
- Modify: `pkg/wshrpc/wshrpctypes_builder.go:11-34` (interface), append types after line 194
- Generated (via `task generate`, never by hand): `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`
- Modify: `pkg/wshrpc/wshserver/builderterm.go` (Task 5 file)
- Test: `pkg/wshrpc/wshserver/builderterm_fixture_test.go` (new DB fixture, used by Tasks 7 and 8), `pkg/wshrpc/wshserver/builderterm_ensure_test.go`, `pkg/wshrpc/wshserver/builderterm_guard_test.go`

**Interfaces:**
- Consumes: Task 1 (`rtstore.MetaKey_BuilderAppId`, `rtstore.ErrBuilderLocalOnly`, `rtstore.IsBuilderTab`); Task 2 (`rtcore.CreateBuilderTab`, `rtcore.FindBuilderTabs`, `rtcore.DeleteBuilderTab`); Task 4 (`buildercontroller.ResolveAppDirForAppId`, `buildercontroller.MakeBuilderTerminalBlockDef`); Task 5 (`checkBuilderCaller`, `withBuilderLock`, `makeBuilderWriteContext`, `makeBuilderLayoutAction`, `wshutil.MakeRpcSourceContextForTest`).
- Produces:
  - `wshrpc.CommandEnsureBuilderTabData{BuilderId string "builderid"; AppId string "appid"}`, `wshrpc.CommandEnsureBuilderTabRtnData{TabId string "tabid"; AppId string "appid"}`
  - `(*WshServer).EnsureBuilderTabCommand(ctx, data) (*wshrpc.CommandEnsureBuilderTabRtnData, error)`; generated `RpcApi.EnsureBuilderTabCommand(client, data, opts?)` (TS) and `wshclient.EnsureBuilderTabCommand` (Go)
  - package `wshserver`: `var queueBuilderLayoutAction = rtcore.QueueLayoutActionForTab` (test seam); `sendBuilderUpdates(writeCtx context.Context)`; `ensureBuilderTab(ctx, builderId, appId string) (*wshrpc.CommandEnsureBuilderTabRtnData, error)`; `addBuilderTermBlock(ctx context.Context, tabId string, appDir string, targetBlockId string, targetAction string) (string, error)`
  - test helpers (package `wshserver`, `_test.go`): `wpsEvents` (`reset()`, `objUpdates() []remotetermobj.WaveObjUpdate`, `blockClosed(blockId string) bool`), `electronCtx()`, `sourceCtx(source string)`, `setupBuilderApps(t, names ...string) string` (returns HOME), `appDirFor(home, name string) string`, `countRows(t) [3]int`, `tabBlocks(t, tabId) []*remotetermobj.Block`, `pendingActions(t, tabId) []remotetermobj.LayoutActionData`, `ensureTab(t, builderId, appId) *wshrpc.CommandEnsureBuilderTabRtnData`, `failQueue(t)`, `runWhileBuilderLocked(t, builderId string, op func() error) error`

- [ ] **Step 1: Add the RPC types and generate bindings**

In `pkg/wshrpc/wshrpctypes_builder.go`, add to `WshRpcBuilderInterface` after `OpenBuilderTerminalCommand` (line 26):

```go
	EnsureBuilderTabCommand(ctx context.Context, data CommandEnsureBuilderTabData) (*CommandEnsureBuilderTabRtnData, error)
```

and append after `CommandGetBuilderAppDirData` (ends line 194):

```go

type CommandEnsureBuilderTabData struct {
	BuilderId string `json:"builderid"`
	AppId     string `json:"appid"`
}

type CommandEnsureBuilderTabRtnData struct {
	TabId string `json:"tabid"`
	AppId string `json:"appid"`
}
```

Run: `./node_modules/.bin/task generate`
Expected: exits 0. Compared with the baseline (where `task generate` changes nothing), `git status --short` now lists exactly `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go` and your `wshrpctypes_builder.go` as modified, besides the untracked toolchains; no other changes. `grep -n EnsureBuilderTabCommand frontend/app/store/wshclientapi.ts pkg/wshrpc/wshclient/wshclient.go` shows one function in each. If any other file changed, stop and report.

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go vet ./pkg/wshrpc/wshserver/`
Expected: exit 0. Handlers are bound by reflection (`pkg/wshrpc/wshrpcmeta.go:91-110`), so the missing method is not a compile error yet; the tests below catch it.

- [ ] **Step 2: Write the DB fixture**

Create `pkg/wshrpc/wshserver/builderterm_fixture_test.go` (pattern: `pkg/jobcontroller/jobcontroller_reconnect_test.go:98-136`, once from `TestMain`; the recorder subscribes to `waveobj:update` and `blockclose`):

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
)

type wpsRecorder struct {
	lock   sync.Mutex
	events []wps.WaveEvent
}

var wpsEvents = &wpsRecorder{}

func (r *wpsRecorder) SendEvent(routeId string, ev wps.WaveEvent) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.events = append(r.events, ev)
}

func (r *wpsRecorder) reset() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.events = nil
}

func (r *wpsRecorder) objUpdates() []remotetermobj.WaveObjUpdate {
	r.lock.Lock()
	defer r.lock.Unlock()
	var rtn []remotetermobj.WaveObjUpdate
	for _, ev := range r.events {
		if ev.Event != wps.Event_WaveObjUpdate {
			continue
		}
		if update, ok := ev.Data.(remotetermobj.WaveObjUpdate); ok {
			rtn = append(rtn, update)
		}
	}
	return rtn
}

func (r *wpsRecorder) blockClosed(blockId string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	for _, ev := range r.events {
		if ev.Event == wps.Event_BlockClose && ev.Data == blockId {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wshserver-test-*")
	if err != nil {
		fmt.Printf("cannot create a test data dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, remotetermbase.WaveDBDir), 0755); err != nil {
		fmt.Printf("cannot create the db dir: %v\n", err)
		os.Exit(1)
	}
	remotetermbase.DataHome_VarCache = dir
	if err := rtstore.InitWStore(); err != nil {
		fmt.Printf("wstore: %v\n", err)
		os.Exit(1)
	}
	if err := filestore.InitFilestore(); err != nil {
		fmt.Printf("filestore: %v\n", err)
		os.Exit(1)
	}
	wps.Broker.Subscribe("wshserver-test", wps.SubscriptionRequest{Event: wps.Event_WaveObjUpdate, AllScopes: true})
	wps.Broker.Subscribe("wshserver-test", wps.SubscriptionRequest{Event: wps.Event_BlockClose, AllScopes: true})
	wps.Broker.SetClient(wpsEvents)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func electronCtx() context.Context {
	return wshutil.MakeRpcSourceContextForTest(context.Background(), wshutil.ElectronRoute)
}

func sourceCtx(source string) context.Context {
	return wshutil.MakeRpcSourceContextForTest(context.Background(), source)
}

func setupBuilderApps(t *testing.T, names ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range names {
		if err := os.MkdirAll(appDirFor(home, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func appDirFor(home string, name string) string {
	return filepath.Join(home, "waveapps", "draft", name)
}

// countRows returns the number of tab, block and layout rows.
func countRows(t *testing.T) [3]int {
	t.Helper()
	ctx := context.Background()
	tabs, err1 := rtstore.DBGetCount[*remotetermobj.Tab](ctx)
	blocks, err2 := rtstore.DBGetCount[*remotetermobj.Block](ctx)
	layouts, err3 := rtstore.DBGetCount[*remotetermobj.LayoutState](ctx)
	if err := errors.Join(err1, err2, err3); err != nil {
		t.Fatal(err)
	}
	return [3]int{tabs, blocks, layouts}
}

func tabBlocks(t *testing.T, tabId string) []*remotetermobj.Block {
	t.Helper()
	ctx := context.Background()
	tab, err := rtstore.DBMustGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil {
		t.Fatalf("tab %s: %v", tabId, err)
	}
	var rtn []*remotetermobj.Block
	for _, blockId := range tab.BlockIds {
		block, err := rtstore.DBMustGet[*remotetermobj.Block](ctx, blockId)
		if err != nil {
			t.Fatalf("block %s: %v", blockId, err)
		}
		rtn = append(rtn, block)
	}
	return rtn
}

func pendingActions(t *testing.T, tabId string) []remotetermobj.LayoutActionData {
	t.Helper()
	ctx := context.Background()
	tab, err := rtstore.DBMustGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil {
		t.Fatalf("tab %s: %v", tabId, err)
	}
	layout, err := rtstore.DBMustGet[*remotetermobj.LayoutState](ctx, tab.LayoutState)
	if err != nil {
		t.Fatalf("layout %s: %v", tab.LayoutState, err)
	}
	if layout.PendingBackendActions == nil {
		return nil
	}
	return *layout.PendingBackendActions
}

func ensureTab(t *testing.T, builderId string, appId string) *wshrpc.CommandEnsureBuilderTabRtnData {
	t.Helper()
	rtn, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: appId})
	if err != nil {
		t.Fatalf("EnsureBuilderTabCommand(%s, %s): %v", builderId, appId, err)
	}
	return rtn
}

// runWhileBuilderLocked holds the builder's lock, starts op, and checks that op neither returns nor writes
// a row within 100 ms; then it releases the lock and returns op's result. A handler that skipped the lock
// fails here deterministically.
func runWhileBuilderLocked(t *testing.T, builderId string, op func() error) error {
	t.Helper()
	lock := getBuilderLock(builderId)
	lock.Lock()
	rows := countRows(t)
	done := make(chan error, 1)
	go func() {
		done <- op()
	}()
	select {
	case err := <-done:
		lock.Unlock()
		t.Fatalf("returned while the builder lock was held (err: %v)", err)
	case <-time.After(100 * time.Millisecond):
	}
	if got := countRows(t); got != rows {
		lock.Unlock()
		t.Fatalf("rows changed while the builder lock was held: %v -> %v", rows, got)
	}
	lock.Unlock()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("did not finish within 5 s of the lock being released")
	}
	return nil
}

func failQueue(t *testing.T) {
	t.Helper()
	orig := queueBuilderLayoutAction
	queueBuilderLayoutAction = func(ctx context.Context, tabId string, actions ...remotetermobj.LayoutActionData) error {
		return errors.New("queue unavailable")
	}
	t.Cleanup(func() { queueBuilderLayoutAction = orig })
}
```

- [ ] **Step 3: Write the failing Ensure tests**

Create `pkg/wshrpc/wshserver/builderterm_ensure_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func TestEnsureBuilderTabCreatesOneTerminal(t *testing.T) {
	home := setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	if rtn.AppId != "draft/demo" || rtn.TabId == "" {
		t.Fatalf("rtn = %+v", rtn)
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || tabs[0].OID != rtn.TabId {
		t.Fatalf("builder tabs = %d, want exactly the returned one", len(tabs))
	}
	blocks := tabBlocks(t, rtn.TabId)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	meta := blocks[0].Meta
	if meta.GetString(remotetermobj.MetaKey_CmdCwd, "") != appDirFor(home, "demo") || meta.GetString(remotetermobj.MetaKey_View, "") != "term" || meta.GetString(remotetermobj.MetaKey_Connection, "") != "local" {
		t.Errorf("block meta = %v", meta)
	}
	if durable, ok := meta[remotetermobj.MetaKey_TermDurable].(bool); !ok || durable {
		t.Errorf("term:durable = %#v, want false", meta[remotetermobj.MetaKey_TermDurable])
	}
	actions := pendingActions(t, rtn.TabId)
	if len(actions) != 1 || actions[0].ActionType != rtcore.LayoutActionDataType_Insert || actions[0].BlockId != blocks[0].OID || !actions[0].Focused {
		t.Errorf("pending actions = %+v", actions)
	}
}

func TestEnsureBuilderTabIsIdempotent(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	first := ensureTab(t, builderId, "draft/demo")
	rows := countRows(t)
	second := ensureTab(t, builderId, "draft/demo")
	if second.TabId != first.TabId {
		t.Fatalf("second Ensure returned %s, want %s", second.TabId, first.TabId)
	}
	if countRows(t) != rows {
		t.Fatalf("second Ensure wrote rows: %v -> %v", rows, countRows(t))
	}
}

func TestEnsureBuilderTabConcurrentCallsCreateOneTab(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	var wg sync.WaitGroup
	results := make([]string, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rtn, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"})
			if err == nil {
				results[i] = rtn.TabId
			}
		}()
	}
	wg.Wait()
	if results[0] == "" || results[0] != results[1] {
		t.Fatalf("results = %v, want the same tab twice", results)
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || len(tabBlocks(t, tabs[0].OID)) != 1 {
		t.Fatalf("got %d tabs", len(tabs))
	}
}

func TestEnsureBuilderTabRejectsBadAppsWithoutWrites(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rows := countRows(t)
	for _, appId := range []string{"", "draft", "draft/../x", "Draft/demo", "draft/missing"} {
		if _, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: appId}); err == nil {
			t.Errorf("app id %q accepted", appId)
		}
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
}

func TestEnsureBuilderTabUsesAppIdArgumentNotRtInfo(t *testing.T) {
	home := setupBuilderApps(t, "demo", "other")
	builderId := uuid.NewString()
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": "draft/other"})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
	rtn := ensureTab(t, builderId, "draft/demo")
	if got := tabBlocks(t, rtn.TabId)[0].Meta.GetString(remotetermobj.MetaKey_CmdCwd, ""); got != appDirFor(home, "demo") {
		t.Fatalf("cmd:cwd = %q, want the demo folder", got)
	}
}

func TestEnsureBuilderTabReplacesTabForAnotherApp(t *testing.T) {
	setupBuilderApps(t, "demo", "demo2")
	builderId := uuid.NewString()
	first := ensureTab(t, builderId, "draft/demo")
	firstBlock := tabBlocks(t, first.TabId)[0]
	second := ensureTab(t, builderId, "draft/demo2")
	if second.TabId == first.TabId || second.AppId != "draft/demo2" {
		t.Fatalf("second = %+v", second)
	}
	found, _ := rtstore.DBExistsORef(context.Background(), remotetermobj.MakeORef(remotetermobj.OType_Tab, first.TabId))
	if found {
		t.Fatal("the tab for the previous app survived")
	}
	if !wpsEvents.blockClosed(firstBlock.OID) {
		t.Error("the previous app's shell was not closed")
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || tabs[0].OID != second.TabId {
		t.Fatalf("builder tabs = %d", len(tabs))
	}
}

func TestEnsureBuilderTabRollsBackWhenQueueFails(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	failQueue(t)
	rows := countRows(t)
	if _, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"}); err == nil {
		t.Fatal("expected the queue failure")
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 0 {
		t.Fatalf("a half-created tab was left: %d", len(tabs))
	}
}

func TestEnsureBuilderTabRejectsNonElectronCallers(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rows := countRows(t)
	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeControllerRouteId(uuid.NewString()),
		wshutil.MakeTabRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(builderId),
		wshutil.MakeBuilderRouteId(uuid.NewString()),
		"",
	} {
		if _, err := WshServerImpl.EnsureBuilderTabCommand(sourceCtx(source), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"}); err == nil {
			t.Errorf("source %q accepted", source)
		}
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
}

func TestEnsureBuilderTabReturnsWhenRpcContextIsDone(t *testing.T) {
	setupBuilderApps(t, "demo")
	ctx, cancelFn := context.WithCancel(electronCtx())
	cancelFn()
	rows := countRows(t)
	_, err := WshServerImpl.EnsureBuilderTabCommand(ctx, wshrpc.CommandEnsureBuilderTabData{BuilderId: uuid.NewString(), AppId: "draft/demo"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if countRows(t) != rows {
		t.Fatal("a cancelled Ensure wrote rows")
	}
}

func TestEnsureBuilderTabWaitsForBuilderLock(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	err := runWhileBuilderLocked(t, builderId, func() error {
		_, err := WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 {
		t.Fatalf("builder tabs after the lock was released = %d, want 1", len(tabs))
	}
}

func TestEnsureBuilderTabBroadcastsUpdates(t *testing.T) {
	setupBuilderApps(t, "demo")
	wpsEvents.reset()
	rtn := ensureTab(t, uuid.NewString(), "draft/demo")
	tab, _ := rtstore.DBMustGet[*remotetermobj.Tab](context.Background(), rtn.TabId)
	seen := map[string]bool{}
	for _, update := range wpsEvents.objUpdates() {
		seen[update.OType+":"+update.OID] = true
	}
	for _, want := range []string{"tab:" + rtn.TabId, "layout:" + tab.LayoutState, "block:" + tab.BlockIds[0]} {
		if !seen[want] {
			t.Errorf("no waveobj:update for %s", want)
		}
	}
}
```

Create `pkg/wshrpc/wshserver/builderterm_guard_test.go` (the Task 1 and Task 3 guards, through each RPC entry point the spec names):

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/service/objectservice"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/google/uuid"
)

func termDef(connection string) *remotetermobj.BlockDef {
	return &remotetermobj.BlockDef{Meta: remotetermobj.MetaMapType{
		remotetermobj.MetaKey_View:       "term",
		remotetermobj.MetaKey_Controller: "shell",
		remotetermobj.MetaKey_Connection: connection,
	}}
}

func TestReservedTabMetaThroughSetMetaAndObjectService(t *testing.T) {
	setupBuilderApps(t, "demo")
	ctx := context.Background()
	rtn := ensureTab(t, uuid.NewString(), "draft/demo")
	tabRef := remotetermobj.MakeORef(remotetermobj.OType_Tab, rtn.TabId)
	for _, patch := range []remotetermobj.MetaMapType{
		{rtstore.MetaKey_BuilderOwner: uuid.NewString()},
		{"builder:*": true},
		{rtstore.MetaKey_BuilderOwner: nil},
	} {
		if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: tabRef, Meta: patch}); err == nil {
			t.Errorf("SetMetaCommand accepted %v", patch)
		}
		if _, err := (&objectservice.ObjectService{}).UpdateObjectMeta(remotetermobj.UIContext{}, tabRef.String(), patch); err == nil {
			t.Errorf("ObjectService.UpdateObjectMeta accepted %v", patch)
		}
	}
	if !rtstore.IsBuilderTab(ctx, rtn.TabId) {
		t.Fatal("the builder tab lost its owner")
	}
	if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: tabRef, Meta: remotetermobj.MetaMapType{"tab:background": "red"}}); err != nil {
		t.Errorf("ordinary tab meta rejected: %v", err)
	}
}

func TestBuilderBlocksStayLocalThroughRpcs(t *testing.T) {
	setupBuilderApps(t, "demo")
	ctx := context.Background()
	rtn := ensureTab(t, uuid.NewString(), "draft/demo")
	block := tabBlocks(t, rtn.TabId)[0]
	blockRef := remotetermobj.MakeORef(remotetermobj.OType_Block, block.OID)
	remote := remotetermobj.MetaMapType{remotetermobj.MetaKey_Connection: "user@host"}

	if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: blockRef, Meta: remote}); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Errorf("SetMetaCommand = %v, want ErrBuilderLocalOnly", err)
	}
	if _, err := WshServerImpl.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{TabId: rtn.TabId, BlockDef: termDef("user@host")}); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Errorf("CreateBlockCommand = %v, want ErrBuilderLocalOnly", err)
	}
	if _, err := WshServerImpl.CreateSubBlockCommand(ctx, wshrpc.CommandCreateSubBlockData{ParentBlockId: block.OID, BlockDef: termDef("user@host")}); !errors.Is(err, rtstore.ErrBuilderLocalOnly) {
		t.Errorf("CreateSubBlockCommand = %v, want ErrBuilderLocalOnly", err)
	}
	if len(tabBlocks(t, rtn.TabId)) != 1 {
		t.Fatal("a rejected block was added")
	}

	layout := &remotetermobj.LayoutState{OID: uuid.NewString()}
	normal := &remotetermobj.Tab{OID: uuid.NewString(), BlockIds: []string{}, LayoutState: layout.OID}
	ws := &remotetermobj.Workspace{OID: uuid.NewString(), TabIds: []string{normal.OID}}
	for _, obj := range []remotetermobj.WaveObj{layout, normal, ws} {
		if err := rtstore.DBInsert(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	created, err := WshServerImpl.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{TabId: normal.OID, BlockDef: termDef("user@host")})
	if err != nil {
		t.Fatalf("remote block rejected in a workspace tab: %v", err)
	}
	if _, err := WshServerImpl.CreateSubBlockCommand(ctx, wshrpc.CommandCreateSubBlockData{ParentBlockId: created.OID, BlockDef: termDef("user@host")}); err != nil {
		t.Fatalf("remote sub-block rejected in a workspace tab: %v", err)
	}
	if err := WshServerImpl.SetMetaCommand(ctx, wshrpc.CommandSetMetaData{ORef: *created, Meta: remote}); err != nil {
		t.Fatalf("remote connection rejected for a workspace block: %v", err)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./pkg/wshrpc/wshserver/... -count=1`
Expected: the test build fails because `WshServer` has no `EnsureBuilderTabCommand` method yet.

- [ ] **Step 5: Implement Ensure**

In `pkg/wshrpc/wshserver/builderterm.go`, add to the imports `"log"`, `"github.com/LannCo/remoteterm/pkg/buildercontroller"`, `"github.com/LannCo/remoteterm/pkg/remotetermappstore"`, `"github.com/LannCo/remoteterm/pkg/rtstore"`, `"github.com/LannCo/remoteterm/pkg/wps"`, `"github.com/LannCo/remoteterm/pkg/wshrpc"`, add after the `builderLocks` var block:

```go
// Tests replace it to make queueing fail; nothing in production does, but the rollback paths depend on it.
var queueBuilderLayoutAction = rtcore.QueueLayoutActionForTab
```

and append:

```go
// Sent once per write phase, after success and after a rollback. Without it the renderer never sees new
// panes' layout actions or the tab's deletion. Updates are kept per object, last one wins, for every
// committed store call; a store transaction that fails adds none. So when a write phase undoes its own
// earlier writes (deleting a half-created tab or block), the broadcast carries the deletes for them.
func sendBuilderUpdates(writeCtx context.Context) {
	wps.Broker.SendUpdateEvents(remotetermobj.ContextGetUpdatesRtn(writeCtx))
}

// Electron fills both fields from the calling window (its builder id and its app id), never from the page.
func (ws *WshServer) EnsureBuilderTabCommand(ctx context.Context, data wshrpc.CommandEnsureBuilderTabData) (*wshrpc.CommandEnsureBuilderTabRtnData, error) {
	if err := checkBuilderCaller(wshutil.GetRpcSourceFromContext(ctx), data.BuilderId, false); err != nil {
		return nil, err
	}
	if err := remotetermappstore.ValidateAppId(data.AppId); err != nil {
		return nil, fmt.Errorf("invalid app id %q: %w", data.AppId, err)
	}
	var rtn *wshrpc.CommandEnsureBuilderTabRtnData
	err := withBuilderLock(data.BuilderId, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		writeCtx, cancelFn := makeBuilderWriteContext()
		defer cancelFn()
		defer sendBuilderUpdates(writeCtx)
		var err error
		rtn, err = ensureBuilderTab(writeCtx, data.BuilderId, data.AppId)
		return err
	})
	if err != nil {
		return nil, err
	}
	return rtn, nil
}

func ensureBuilderTab(ctx context.Context, builderId string, appId string) (*wshrpc.CommandEnsureBuilderTabRtnData, error) {
	tabs, err := rtcore.FindBuilderTabs(ctx, builderId)
	if err != nil {
		return nil, err
	}
	var current *remotetermobj.Tab
	for _, tab := range tabs {
		if current == nil && tab.Meta.GetString(rtstore.MetaKey_BuilderAppId, "") == appId {
			current = tab
			continue
		}
		if err := rtcore.DeleteBuilderTab(ctx, tab.OID, builderId); err != nil {
			return nil, fmt.Errorf("error removing the terminals of a previous app: %w", err)
		}
	}
	if current != nil {
		return &wshrpc.CommandEnsureBuilderTabRtnData{TabId: current.OID, AppId: appId}, nil
	}
	appDir, err := buildercontroller.ResolveAppDirForAppId(appId)
	if err != nil {
		return nil, err
	}
	tab, err := rtcore.CreateBuilderTab(ctx, builderId, appId)
	if err != nil {
		return nil, err
	}
	if _, err := addBuilderTermBlock(ctx, tab.OID, appDir, "", ""); err != nil {
		if delErr := rtcore.DeleteBuilderTab(ctx, tab.OID, builderId); delErr != nil {
			log.Printf("EnsureBuilderTabCommand: could not roll back tab %s: %v\n", tab.OID, delErr)
		}
		return nil, err
	}
	return &wshrpc.CommandEnsureBuilderTabRtnData{TabId: tab.OID, AppId: appId}, nil
}

// rtcore.CreateBlock and the queue are called directly, not through CreateBlockCommand, which
// broadcasts only on success and cannot roll back.
func addBuilderTermBlock(ctx context.Context, tabId string, appDir string, targetBlockId string, targetAction string) (string, error) {
	blockData, err := rtcore.CreateBlock(ctx, tabId, buildercontroller.MakeBuilderTerminalBlockDef(appDir), nil)
	if err != nil {
		return "", fmt.Errorf("error creating terminal: %w", err)
	}
	err = queueBuilderLayoutAction(ctx, tabId, makeBuilderLayoutAction(blockData.OID, targetBlockId, targetAction))
	if err != nil {
		if delErr := rtcore.DeleteBlock(ctx, blockData.OID, false); delErr != nil {
			log.Printf("builder terminal: could not roll back block %s: %v\n", blockData.OID, delErr)
		}
		return "", fmt.Errorf("error queuing layout action: %w", err)
	}
	return blockData.OID, nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./pkg/wshrpc/... -count=1 && go test -race ./pkg/wshrpc/wshserver/... -count=1`
Expected: `ok` for `pkg/wshrpc`, `pkg/wshrpc/wshremote`, `pkg/wshrpc/wshserver`; then `ok` under `-race`.

- [ ] **Step 7: Type-check the frontend**

Run: `npx tsc --noEmit`
Expected: exit 0 (the generated TS only adds a function and two types).

- [ ] **Step 8: Commit**

```bash
git add pkg/wshrpc/wshrpctypes_builder.go pkg/wshrpc/wshclient/wshclient.go frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts pkg/wshrpc/wshserver/builderterm.go pkg/wshrpc/wshserver/builderterm_fixture_test.go pkg/wshrpc/wshserver/builderterm_ensure_test.go pkg/wshrpc/wshserver/builderterm_guard_test.go
git commit -m "feat(builder): EnsureBuilderTabCommand creates the builder's terminal tab

The first call for a builder window creates its tab with one local shell
in the app folder; later calls return the same tab, a different app
replaces it, and any failure leaves nothing behind. Only Electron may
call it.

Miscellanea: RPC types and generated bindings; wshserver DB test fixture;
guard coverage through SetMeta, ObjectService and the create RPCs.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: `OpenBuilderTerminalCommand` rewrite (`pkg/wshrpc`, Electron call site)

**Files:**
- Modify: `pkg/wshrpc/wshrpctypes_builder.go:187-190` (`CommandOpenBuilderTerminalData`)
- Generated (via `task generate`): `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`
- Modify: `pkg/wshrpc/wshserver/wshserver.go:1208-1222` (delete the old handler), `pkg/wshrpc/wshserver/builderterm.go` (new handler)
- Modify: `emain/emain-ipc.ts:24-29, 35-42` (imports), `:547-571` (`open-builder-terminal` handler)
- Modify: `emain/emain-builder-select.ts:4-33` (delete `pickTerminalWindow`, `bringWindowToFront` and their types), `emain/emain-builder-select.test.ts:5, 11-29, 52-82`
- Test: `pkg/wshrpc/wshserver/builderterm_open_test.go`

**Interfaces:**
- Consumes: Task 5 (`isValidBuilderTargetAction`, `MaxBuilderTermBlocks`, `withBuilderLock`, `makeBuilderWriteContext`); Task 6 (`sendBuilderUpdates`, `addBuilderTermBlock`, `queueBuilderLayoutAction`, test helpers `ensureTab`, `electronCtx`, `sourceCtx`, `setupBuilderApps`, `appDirFor`, `countRows`, `tabBlocks`, `pendingActions`, `failQueue`, `termDef`).
- Produces:
  - `wshrpc.CommandOpenBuilderTerminalData{BuilderId string "builderid"; TargetBlockId string "targetblockid,omitempty"; TargetAction string "targetaction,omitempty"}` (the `tabid` field is gone)
  - `(*WshServer).OpenBuilderTerminalCommand(ctx, data) error`, errors verbatim: `builder terminal not ready`, `too many terminals in this builder (max 16)`
  - package `wshserver`: `openBuilderTerminal(ctx context.Context, data wshrpc.CommandOpenBuilderTerminalData) error`, `countTermBlocks(ctx context.Context, blockIds []string) (int, error)`
  - Electron `open-builder-terminal` no longer touches main windows; it sends `{ builderid }` only (Task 10 adds the target)

- [ ] **Step 1: Change the RPC type and regenerate**

In `pkg/wshrpc/wshrpctypes_builder.go`, replace `CommandOpenBuilderTerminalData` (lines 187-190) with:

```go
type CommandOpenBuilderTerminalData struct {
	BuilderId     string `json:"builderid"`
	TargetBlockId string `json:"targetblockid,omitempty"`
	TargetAction  string `json:"targetaction,omitempty"`
}
```

Run: `./node_modules/.bin/task generate`
Expected: exit 0; `frontend/types/gotypes.d.ts` now declares `CommandOpenBuilderTerminalData` with `builderid`, `targetblockid?`, `targetaction?` and no `tabid`; no other changes besides the three generated files and `wshrpctypes_builder.go`.

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go vet ./pkg/wshrpc/wshserver/`
Expected: vet fails because the old handler in `wshserver.go` still reads `data.TabId`, which the type no longer has.

- [ ] **Step 2: Write the failing tests**

Create `pkg/wshrpc/wshserver/builderterm_open_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func openTerm(builderId string, targetBlockId string, targetAction string) error {
	return WshServerImpl.OpenBuilderTerminalCommand(electronCtx(), wshrpc.CommandOpenBuilderTerminalData{
		BuilderId:     builderId,
		TargetBlockId: targetBlockId,
		TargetAction:  targetAction,
	})
}

func termCount(t *testing.T, tabId string) int {
	t.Helper()
	count := 0
	for _, block := range tabBlocks(t, tabId) {
		if block.Meta.GetString(remotetermobj.MetaKey_View, "") == "term" {
			count++
		}
	}
	return count
}

func TestOpenBuilderTerminalAppends(t *testing.T) {
	home := setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	if err := openTerm(builderId, "", ""); err != nil {
		t.Fatal(err)
	}
	blocks := tabBlocks(t, rtn.TabId)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	added := blocks[1]
	if added.Meta.GetString(remotetermobj.MetaKey_CmdCwd, "") != appDirFor(home, "demo") {
		t.Errorf("cwd = %q", added.Meta.GetString(remotetermobj.MetaKey_CmdCwd, ""))
	}
	actions := pendingActions(t, rtn.TabId)
	last := actions[len(actions)-1]
	if last.ActionType != rtcore.LayoutActionDataType_Insert || last.BlockId != added.OID || !last.Focused {
		t.Errorf("last action = %+v", last)
	}
}

func TestOpenBuilderTerminalSplitsADirectChild(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	first := tabBlocks(t, rtn.TabId)[0]
	if err := openTerm(builderId, first.OID, "splitdown"); err != nil {
		t.Fatal(err)
	}
	actions := pendingActions(t, rtn.TabId)
	last := actions[len(actions)-1]
	if last.ActionType != rtcore.LayoutActionDataType_SplitVertical || last.Position != "after" || last.TargetBlockId != first.OID || !last.Focused {
		t.Errorf("last action = %+v", last)
	}
}

func TestOpenBuilderTerminalRejectsBadTargetsWithoutWrites(t *testing.T) {
	setupBuilderApps(t, "demo", "demo2")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	first := tabBlocks(t, rtn.TabId)[0]
	sub, err := rtcore.CreateSubBlock(context.Background(), first.OID, termDef("local"))
	if err != nil {
		t.Fatal(err)
	}
	other := ensureTab(t, uuid.NewString(), "draft/demo2")
	foreign := tabBlocks(t, other.TabId)[0]
	rows := countRows(t)
	cases := []struct{ target, action string }{
		{sub.OID, "splitright"},
		{foreign.OID, "splitright"},
		{uuid.NewString(), "splitright"},
		{first.OID, "replace"},
		{"", "splitright"},
	}
	for _, tc := range cases {
		if err := openTerm(builderId, tc.target, tc.action); err == nil {
			t.Errorf("target %q action %q accepted", tc.target, tc.action)
		}
	}
	if countRows(t) != rows {
		t.Fatalf("rows changed: %v -> %v", rows, countRows(t))
	}
}

func TestOpenBuilderTerminalCapsTermBlocks(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	for range 7 {
		if err := openTerm(builderId, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	// Terminals that wsh creates through CreateBlockCommand count toward the cap too.
	for range 8 {
		if _, err := WshServerImpl.CreateBlockCommand(context.Background(), wshrpc.CommandCreateBlockData{TabId: rtn.TabId, BlockDef: termDef("local")}); err != nil {
			t.Fatal(err)
		}
	}
	if got := termCount(t, rtn.TabId); got != MaxBuilderTermBlocks {
		t.Fatalf("term blocks = %d, want %d", got, MaxBuilderTermBlocks)
	}
	rows := countRows(t)
	err := openTerm(builderId, "", "")
	if err == nil || err.Error() != "too many terminals in this builder (max 16)" {
		t.Fatalf("17th Open = %v", err)
	}
	if countRows(t) != rows {
		t.Fatal("a rejected Open wrote rows")
	}
}

func TestOpenBuilderTerminalWithoutTabIsNotReady(t *testing.T) {
	setupBuilderApps(t, "demo")
	rows := countRows(t)
	err := openTerm(uuid.NewString(), "", "")
	if err == nil || err.Error() != "builder terminal not ready" {
		t.Fatalf("err = %v, want builder terminal not ready", err)
	}
	if countRows(t) != rows {
		t.Fatal("Open without a tab wrote rows")
	}
}

func TestOpenBuilderTerminalRemovesBlockWhenQueueFails(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	failQueue(t)
	rows := countRows(t)
	if err := openTerm(builderId, "", ""); err == nil {
		t.Fatal("expected the queue failure")
	}
	if countRows(t) != rows || len(tabBlocks(t, rtn.TabId)) != 1 {
		t.Fatalf("rows %v -> %v; the new block was not removed", rows, countRows(t))
	}
}

func TestOpenBuilderTerminalUsesTabAppIdNotRtInfo(t *testing.T) {
	home := setupBuilderApps(t, "demo", "other")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": "draft/other"})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
	if err := openTerm(builderId, "", ""); err != nil {
		t.Fatal(err)
	}
	blocks := tabBlocks(t, rtn.TabId)
	if got := blocks[len(blocks)-1].Meta.GetString(remotetermobj.MetaKey_CmdCwd, ""); got != appDirFor(home, "demo") {
		t.Fatalf("cwd = %q, want the demo folder", got)
	}
}

func TestOpenBuilderTerminalRejectsNonElectronCallers(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	rows := countRows(t)
	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeControllerRouteId(uuid.NewString()),
		wshutil.MakeTabRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(builderId),
		"",
	} {
		err := WshServerImpl.OpenBuilderTerminalCommand(sourceCtx(source), wshrpc.CommandOpenBuilderTerminalData{BuilderId: builderId})
		if err == nil {
			t.Errorf("source %q accepted", source)
		}
	}
	if countRows(t) != rows {
		t.Fatal("a refused caller wrote rows")
	}
}

func TestOpenBuilderTerminalBroadcastsLayoutUpdate(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	tab, _ := rtstore.DBMustGet[*remotetermobj.Tab](context.Background(), rtn.TabId)
	wpsEvents.reset()
	if err := openTerm(builderId, "", ""); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, update := range wpsEvents.objUpdates() {
		if update.OType == remotetermobj.OType_LayoutState && update.OID == tab.LayoutState {
			found = true
		}
	}
	if !found {
		t.Fatal("no waveobj:update for the layout state")
	}
}

func TestOpenBuilderTerminalWaitsForBuilderLock(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	if err := runWhileBuilderLocked(t, builderId, func() error { return openTerm(builderId, "", "") }); err != nil {
		t.Fatal(err)
	}
	if n := len(tabBlocks(t, rtn.TabId)); n != 2 {
		t.Fatalf("blocks after the lock was released = %d, want 2", n)
	}
}

func TestOpenBuilderTerminalAppDirGone(t *testing.T) {
	home := setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	if err := os.RemoveAll(appDirFor(home, "demo")); err != nil {
		t.Fatal(err)
	}
	rows := countRows(t)
	err := openTerm(builderId, "", "")
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("err = %v, want the app folder reported as not available", err)
	}
	if countRows(t) != rows {
		t.Fatal("Open wrote rows for a missing app folder")
	}
}

func TestOpenBuilderTerminalConcurrentAtCap(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	for range MaxBuilderTermBlocks - 3 {
		if err := openTerm(builderId, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			openTerm(builderId, "", "")
		}()
	}
	wg.Wait()
	if got := termCount(t, rtn.TabId); got != MaxBuilderTermBlocks {
		t.Fatalf("term blocks = %d after concurrent Opens, want %d", got, MaxBuilderTermBlocks)
	}
}
```

- [ ] **Step 3: Implement**

Delete `OpenBuilderTerminalCommand` from `pkg/wshrpc/wshserver/wshserver.go` (lines 1208-1222, the function and its trailing blank line). Append to `pkg/wshrpc/wshserver/builderterm.go` (add `"slices"` to its imports):

```go
func (ws *WshServer) OpenBuilderTerminalCommand(ctx context.Context, data wshrpc.CommandOpenBuilderTerminalData) error {
	if err := checkBuilderCaller(wshutil.GetRpcSourceFromContext(ctx), data.BuilderId, false); err != nil {
		return err
	}
	if !isValidBuilderTargetAction(data.TargetAction) {
		return fmt.Errorf("invalid target action %q", data.TargetAction)
	}
	if data.TargetAction != "" && data.TargetBlockId == "" {
		return fmt.Errorf("a split needs a target terminal")
	}
	return withBuilderLock(data.BuilderId, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		writeCtx, cancelFn := makeBuilderWriteContext()
		defer cancelFn()
		defer sendBuilderUpdates(writeCtx)
		return openBuilderTerminal(writeCtx, data)
	})
}

// Open never creates or deletes tabs; the folder comes from the app id stored on the tab, never rtinfo.
func openBuilderTerminal(ctx context.Context, data wshrpc.CommandOpenBuilderTerminalData) error {
	tabs, err := rtcore.FindBuilderTabs(ctx, data.BuilderId)
	if err != nil {
		return err
	}
	if len(tabs) != 1 {
		return fmt.Errorf("builder terminal not ready")
	}
	tab := tabs[0]
	appDir, err := buildercontroller.ResolveAppDirForAppId(tab.Meta.GetString(rtstore.MetaKey_BuilderAppId, ""))
	if err != nil {
		return err
	}
	if data.TargetAction != "" && !slices.Contains(tab.BlockIds, data.TargetBlockId) {
		return fmt.Errorf("the split target is not a terminal in this builder")
	}
	termCount, err := countTermBlocks(ctx, tab.BlockIds)
	if err != nil {
		return err
	}
	if termCount >= MaxBuilderTermBlocks {
		return fmt.Errorf("too many terminals in this builder (max %d)", MaxBuilderTermBlocks)
	}
	_, err = addBuilderTermBlock(ctx, tab.OID, appDir, data.TargetBlockId, data.TargetAction)
	return err
}

func countTermBlocks(ctx context.Context, blockIds []string) (int, error) {
	blocks, err := rtstore.DBSelectMap[*remotetermobj.Block](ctx, blockIds)
	if err != nil {
		return 0, fmt.Errorf("error reading terminals: %w", err)
	}
	count := 0
	for _, block := range blocks {
		if block.Meta.GetString(remotetermobj.MetaKey_View, "") == "term" {
			count++
		}
	}
	return count, nil
}
```

- [ ] **Step 4: Run the Go tests**

Run: `go test ./pkg/wshrpc/... -count=1 && go test -race ./pkg/wshrpc/wshserver/... -count=1`
Expected: `ok` for every `pkg/wshrpc` package; `ok` under `-race`.

- [ ] **Step 5: Fix the Electron call site**

Run: `npx tsc --noEmit`
Expected: tsc fails in `emain/emain-ipc.ts` at the `OpenBuilderTerminalCommand` call, because `tabid` is no longer part of `CommandOpenBuilderTerminalData` (the wording of the TypeScript error may vary).

In `emain/emain-ipc.ts`, replace the `open-builder-terminal` handler (lines 547-571) with:

```ts
    electron.ipcMain.handle("open-builder-terminal", async (event): Promise<string> => {
        const bw = getBuilderWindowByWebContentsId(event.sender.id);
        if (bw == null) {
            return "This action is only available in a builder window.";
        }
        try {
            await RpcApi.OpenBuilderTerminalCommand(ElectronWshClient, { builderid: bw.builderId });
        } catch (e) {
            return `Could not open a terminal: ${e instanceof Error ? e.message : String(e)}`;
        }
        return "";
    });
```

Then remove the imports that lost their only use: `bringWindowToFront` and `pickTerminalWindow` from the `./emain-builder-select` import (lines 24-29), and `focusedRemoteTermWindow`, `getAllRemoteTermWindows`, `getQuakeWindow`, `revealQuakeWindow` from the `./emain-window` import (lines 35-42). Check first: `grep -n "focusedRemoteTermWindow\|getAllRemoteTermWindows\|getQuakeWindow\|revealQuakeWindow\|pickTerminalWindow\|bringWindowToFront" emain/emain-ipc.ts` must list only the import lines.

In `emain/emain-builder-select.ts`, delete lines 4-33 (`DestroyableWindow`, `pickTerminalWindow`, `RevealableWindow`, `bringWindowToFront` and their comments; nothing else uses them: `grep -rn "pickTerminalWindow\|bringWindowToFront" emain frontend` lists only these files). In `emain/emain-builder-select.test.ts`, drop `bringWindowToFront` and `pickTerminalWindow` from the import (line 5) and delete the `describe("pickTerminalWindow", ...)` (lines 11-29) and `describe("bringWindowToFront", ...)` (lines 52-82) blocks, plus the `makeWin` helper (lines 7-9) if nothing else uses it.

- [ ] **Step 6: Run the frontend checks**

Run: `npx tsc --noEmit && npx vitest run emain/emain-builder-select.test.ts`
Expected: tsc exit 0; vitest `findBuilderWindowForApp` and `openPathDetached` suites pass.

- [ ] **Step 7: Commit**

```bash
git add pkg/wshrpc/wshrpctypes_builder.go pkg/wshrpc/wshclient/wshclient.go frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts pkg/wshrpc/wshserver/wshserver.go pkg/wshrpc/wshserver/builderterm.go pkg/wshrpc/wshserver/builderterm_open_test.go emain/emain-ipc.ts emain/emain-builder-select.ts emain/emain-builder-select.test.ts
git commit -m "feat(builder): Open terminal adds a pane to the builder, not a main window

OpenBuilderTerminalCommand now appends or splits a local shell inside
the builder's own terminal tab, in the app folder stored on that tab,
capped at 16 terminals; bad targets and refused callers change nothing.

Miscellanea: RPC type drops tabid for targetblockid/targetaction;
regenerated bindings; Electron no longer picks a main window, and the
unused picker helpers are removed.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: `DeleteBuilderCommand` teardown (`pkg/wshrpc/wshserver`)

**Files:**
- Modify: `pkg/wshrpc/wshserver/wshserver.go:1164-1170` (delete the old handler), `pkg/wshrpc/wshserver/builderterm.go` (new handler)
- Test: `pkg/wshrpc/wshserver/builderterm_delete_test.go`

**Interfaces:**
- Consumes: Task 2 (`rtcore.FindBuilderTabs`, `rtcore.DeleteBuilderTab`), Task 3 (cascade skip, exercised through `DeleteBlockCommand`), Task 5 (`checkBuilderCaller` with `allowRenderer = true`, `getBuilderLock`, `withBuilderLock`, `makeBuilderWriteContext`), Task 6 (`sendBuilderUpdates`, test helpers).
- Produces: `(*WshServer).DeleteBuilderCommand(ctx, builderId string) error` (signature unchanged). It accepts the `electron` route and `builder:<builderId>`, deletes every builder tab of that builder on a detached 15 s context under the builder lock, broadcasts the updates, and still calls `buildercontroller.DeleteController(builderId)`. Package `wshserver`: `deleteBuilderTabs(ctx context.Context, builderId string) error`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/wshrpc/wshserver/builderterm_delete_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/buildercontroller"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func builderTabCount(t *testing.T, builderId string) int {
	t.Helper()
	tabs, err := rtcore.FindBuilderTabs(context.Background(), builderId)
	if err != nil {
		t.Fatal(err)
	}
	return len(tabs)
}

func TestDeleteBuilderCommandRemovesEveryOwnerTab(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	block := tabBlocks(t, rtn.TabId)[0]
	if _, err := rtcore.CreateBuilderTab(context.Background(), builderId, "draft/other"); err != nil {
		t.Fatal(err)
	}
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId); err != nil {
		t.Fatal(err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left", n)
	}
	if !wpsEvents.blockClosed(block.OID) {
		t.Error("no BlockClose for the builder's shell")
	}
}

func TestDeleteBuilderCommandCompletesWhenCallerContextIsCancelled(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	ctx, cancelFn := context.WithCancel(electronCtx())
	cancelFn()
	if err := WshServerImpl.DeleteBuilderCommand(ctx, builderId); err != nil {
		t.Fatal(err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left after a cancelled caller", n)
	}
}

func TestDeleteBuilderCommandAcceptsElectronAndOwnRendererOnly(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeControllerRouteId(uuid.NewString()),
		wshutil.MakeTabRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(uuid.NewString()),
		"",
	} {
		if err := WshServerImpl.DeleteBuilderCommand(sourceCtx(source), builderId); err == nil {
			t.Errorf("source %q accepted", source)
		}
	}
	if n := builderTabCount(t, builderId); n != 1 {
		t.Fatalf("a refused caller removed tabs (%d left)", n)
	}
	if err := WshServerImpl.DeleteBuilderCommand(sourceCtx(wshutil.MakeBuilderRouteId(builderId)), builderId); err != nil {
		t.Fatalf("the builder's own renderer was refused: %v", err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left", n)
	}
}

func TestDeleteBuilderCommandBroadcastsTabDelete(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	wpsEvents.reset()
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId); err != nil {
		t.Fatal(err)
	}
	for _, update := range wpsEvents.objUpdates() {
		if update.OType == remotetermobj.OType_Tab && update.OID == rtn.TabId && update.UpdateType == remotetermobj.UpdateType_Delete {
			return
		}
	}
	t.Fatal("no waveobj:update delete for the builder tab")
}

func TestDeleteBuilderCommandDeletesController(t *testing.T) {
	builderId := uuid.NewString()
	buildercontroller.GetOrCreateController(builderId)
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId); err != nil {
		t.Fatal(err)
	}
	if buildercontroller.GetController(builderId) != nil {
		t.Fatal("the builder controller survived")
	}
}

func TestDeleteBuilderCommandWaitsForBuilderLock(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	ensureTab(t, builderId, "draft/demo")
	err := runWhileBuilderLocked(t, builderId, func() error {
		return WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := builderTabCount(t, builderId); n != 0 {
		t.Fatalf("%d builder tabs left after the lock was released", n)
	}
}

func TestBuilderLockSerialisesDeleteAndEnsures(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	base := countRows(t)
	ensureTab(t, builderId, "draft/demo")
	ensureOp := func() {
		WshServerImpl.EnsureBuilderTabCommand(electronCtx(), wshrpc.CommandEnsureBuilderTabData{BuilderId: builderId, AppId: "draft/demo"})
	}
	deleteOp := func() {
		WshServerImpl.DeleteBuilderCommand(electronCtx(), builderId)
	}
	lock := getBuilderLock(builderId)
	lock.Lock()
	var wg sync.WaitGroup
	for _, op := range []func(){ensureOp, deleteOp, ensureOp} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op()
		}()
	}
	time.Sleep(50 * time.Millisecond)
	lock.Unlock()
	wg.Wait()

	tabs, _ := rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) > 1 {
		t.Fatalf("%d builder tabs after interleaved Ensure/Delete", len(tabs))
	}
	// Every surviving tab owns exactly one block and one layout state; anything else is a leaked row.
	k := len(tabs)
	if got, want := countRows(t), [3]int{base[0] + k, base[1] + k, base[2] + k}; got != want {
		t.Fatalf("rows after the interleave = %v, want %v (%d tabs)", got, want, k)
	}
	final := ensureTab(t, builderId, "draft/demo")
	tabs, _ = rtcore.FindBuilderTabs(context.Background(), builderId)
	if len(tabs) != 1 || tabs[0].OID != final.TabId || len(tabBlocks(t, final.TabId)) != 1 {
		t.Fatalf("after a final Ensure: %d tabs", len(tabs))
	}
	if got, want := countRows(t), [3]int{base[0] + 1, base[1] + 1, base[2] + 1}; got != want {
		t.Fatalf("rows after the final Ensure = %v, want %v", got, want)
	}
}

func TestDeleteBuilderCommandLeavesOtherBuilder(t *testing.T) {
	setupBuilderApps(t, "demo", "demo2")
	builderA := uuid.NewString()
	builderB := uuid.NewString()
	ensureTab(t, builderA, "draft/demo")
	rtnB := ensureTab(t, builderB, "draft/demo2")
	blockB := tabBlocks(t, rtnB.TabId)[0]
	wpsEvents.reset()
	if err := WshServerImpl.DeleteBuilderCommand(electronCtx(), builderA); err != nil {
		t.Fatal(err)
	}
	if builderTabCount(t, builderB) != 1 || len(tabBlocks(t, rtnB.TabId)) != 1 {
		t.Fatal("closing one builder touched the other's terminals")
	}
	if wpsEvents.blockClosed(blockB.OID) {
		t.Fatal("closing one builder closed the other's shell")
	}
}

func TestDeleteBlockCommandOnLastBuilderPaneKeepsTab(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	rtn := ensureTab(t, builderId, "draft/demo")
	block := tabBlocks(t, rtn.TabId)[0]
	if err := WshServerImpl.DeleteBlockCommand(context.Background(), wshrpc.CommandDeleteBlockData{BlockId: block.OID}); err != nil {
		t.Fatalf("DeleteBlockCommand = %v", err)
	}
	if builderTabCount(t, builderId) != 1 || len(tabBlocks(t, rtn.TabId)) != 0 {
		t.Fatal("expected the builder tab kept with no panes")
	}
	if !wpsEvents.blockClosed(block.OID) {
		t.Error("no BlockClose for the closed pane")
	}
	removed := false
	for _, action := range pendingActions(t, rtn.TabId) {
		if action.ActionType == rtcore.LayoutActionDataType_Remove && action.BlockId == block.OID {
			removed = true
		}
	}
	if !removed {
		t.Error("no layout delete action queued for the closed pane")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go test ./pkg/wshrpc/wshserver/... -count=1 -run 'TestDeleteBuilderCommand|TestBuilderLockSerialises|TestDeleteBlockCommandOnLast'`
Expected: FAIL. `TestDeleteBuilderCommandRemovesEveryOwnerTab` (`2 builder tabs left`), `TestDeleteBuilderCommandCompletesWhenCallerContextIsCancelled` (`1 builder tabs left after a cancelled caller`), `TestDeleteBuilderCommandAcceptsElectronAndOwnRendererOnly` (`source "proc:..." accepted`), `TestDeleteBuilderCommandBroadcastsTabDelete` (`no waveobj:update delete for the builder tab`) and `TestDeleteBuilderCommandWaitsForBuilderLock` (`returned while the builder lock was held`) fail. The others pass already: the old handler deletes no tabs, so the lock test cannot see two, and `TestDeleteBlockCommandOnLastBuilderPaneKeepsTab` passes because of Task 3. They pin behaviour the new handler must keep.

- [ ] **Step 3: Implement**

Delete `DeleteBuilderCommand` from `pkg/wshrpc/wshserver/wshserver.go` (lines 1164-1170 and the blank line after). Append to `pkg/wshrpc/wshserver/builderterm.go`:

```go
// The builder's own renderer may call this too: switchBuilderApp tears down before reloading.
func (ws *WshServer) DeleteBuilderCommand(ctx context.Context, builderId string) error {
	if err := checkBuilderCaller(wshutil.GetRpcSourceFromContext(ctx), builderId, true); err != nil {
		return err
	}
	err := withBuilderLock(builderId, func() error {
		writeCtx, cancelFn := makeBuilderWriteContext()
		defer cancelFn()
		defer sendBuilderUpdates(writeCtx)
		return deleteBuilderTabs(writeCtx, builderId)
	})
	buildercontroller.DeleteController(builderId)
	return err
}

// The RPC context is deliberately unused: a window that is closing must not leave shells behind. There is no
// tombstone, because switchBuilderApp reuses the builder id after calling this.
func deleteBuilderTabs(ctx context.Context, builderId string) error {
	tabs, err := rtcore.FindBuilderTabs(ctx, builderId)
	if err != nil {
		return err
	}
	var firstErr error
	for _, tab := range tabs {
		err := rtcore.DeleteBuilderTab(ctx, tab.OID, builderId)
		if err == nil {
			continue
		}
		log.Printf("DeleteBuilderCommand: %v\n", err)
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/wshrpc/wshserver/... -count=1 && go test -race ./pkg/wshrpc/wshserver/... -count=1`
Expected: `ok` twice.

- [ ] **Step 5: Commit**

```bash
git add pkg/wshrpc/wshserver/wshserver.go pkg/wshrpc/wshserver/builderterm.go pkg/wshrpc/wshserver/builderterm_delete_test.go
git commit -m "feat(builder): closing or switching a builder removes its terminals

DeleteBuilderCommand now deletes the builder's terminal tab, every pane
and its layout (each shell gets BlockClose) on a detached context, even
if the caller has already gone, and leaves other builders alone. Panes
cannot call it through wsh.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Startup sweep (`cmd/server`)

**Files:**
- Modify: `cmd/server/main-server.go:50-51` (const), `:308-310` (call), new helper `runBuilderSweep` after `backupCleanupLoop` (ends line 119)
- Test: `cmd/server/main-server_sweep_test.go` (new)

**Interfaces:**
- Consumes (Task 2): `rtcore.SweepBuilderTabs(ctx context.Context) int` (it logs `[startup] builder sweep: removed N tabs`).
- Produces: `const BuilderSweepTimeout = 30 * time.Second`; `runBuilderSweep()` called synchronously in `main()` after `jobcontroller.InitJobController()` and `blockcontroller.InitBlockController()` and before `go blockcontroller.StartupReconnectDurableShells(...)`, so before every listener (`main-server.go:323-346`) and before `WAVESRV-ESTART` (`:344`).

`main()` cannot be run from a test, so its order is checked on the parsed source, as `emain/emain-startup-order.test.ts` does for Electron.

- [ ] **Step 1: Write the failing test**

Create `cmd/server/main-server_sweep_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func callName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			return x.Name + "." + e.Sel.Name
		}
	}
	return ""
}

// mainCallIndex maps each call made directly in main() (not inside a closure) to the index of the
// top-level statement that first makes it.
func mainCallIndex(t *testing.T) (map[string]int, []ast.Stmt) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "main-server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var mainFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "main" {
			mainFn = fn
		}
	}
	if mainFn == nil {
		t.Fatal("main() not found")
	}
	index := map[string]int{}
	for i, stmt := range mainFn.Body.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := callName(call.Fun)
			if _, seen := index[name]; name != "" && !seen {
				index[name] = i
			}
			return true
		})
	}
	return index, mainFn.Body.List
}

func TestBuilderSweepRunsBetweenControllerInitAndReconnect(t *testing.T) {
	index, stmts := mainCallIndex(t)
	sweep, ok := index["runBuilderSweep"]
	if !ok {
		t.Fatal("runBuilderSweep is not called in main()")
	}
	if _, isExpr := stmts[sweep].(*ast.ExprStmt); !isExpr {
		t.Fatal("runBuilderSweep must run synchronously, not in a goroutine")
	}
	for _, before := range []string{"jobcontroller.InitJobController", "blockcontroller.InitBlockController"} {
		if i, ok := index[before]; !ok || i >= sweep {
			t.Errorf("%s (stmt %d) must run before the sweep (stmt %d)", before, i, sweep)
		}
	}
	for _, after := range []string{"blockcontroller.StartupReconnectDurableShells", "web.MakeTCPListener", "web.MakeUnixListener"} {
		if i, ok := index[after]; !ok || i <= sweep {
			t.Errorf("%s (stmt %d) must run after the sweep (stmt %d)", after, i, sweep)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `export PATH=$PWD/golang-1.26.2/bin:$PATH && go test ./cmd/server/... -count=1 -run TestBuilderSweep`
Expected: FAIL, `runBuilderSweep is not called in main()`.

- [ ] **Step 3: Implement**

In `cmd/server/main-server.go`, after `const BackupCleanupInterval = 4 * time.Hour` (line 51) add:

```go
const BuilderSweepTimeout = 30 * time.Second
```

After `backupCleanupLoop` (ends line 119) add:

```go
// Runs after the job and block controllers subscribe to BlockClose, so swept shells are torn down, and
// before any listener opens, so no window can see or race the orphaned tabs.
func runBuilderSweep() {
	ctx, cancelFn := context.WithTimeout(context.Background(), BuilderSweepTimeout)
	defer cancelFn()
	rtcore.SweepBuilderTabs(ctx)
}
```

In `main()`, between `blockcontroller.InitBlockController()` (line 309) and `go blockcontroller.StartupReconnectDurableShells(context.Background())` (line 310) insert:

```go
	runBuilderSweep()
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/server/... -count=1`
Expected: `ok  	github.com/LannCo/remoteterm/cmd/server`.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main-server.go cmd/server/main-server_sweep_test.go
git commit -m "feat(builder): remove leftover builder terminals at server start

After a crash or a quit that skipped cleanup, the next start deletes
every builder tab and its blocks before any window connects, logging
\"[startup] builder sweep: removed N tabs\". Workspace tabs are never
touched.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Electron IPC (`ensure-builder-tab`, `open-builder-terminal` targets, teardown order)

**Files:**
- Modify: `emain/emain-builder-select.ts` (add `MaxBuilderTargetLen`, `BuilderTeardownTimeoutMs`, `parseBuilderTerminalTarget`, `runBuilderTeardown`)
- Test: `emain/emain-builder-select.test.ts` (append)
- Modify: `emain/emain-ipc.ts:226-251` (`destroyBuilderWindow`), the `open-builder-terminal` handler (as left by Task 7), new `ensure-builder-tab` handler next to it
- Modify: `emain/emain-builder.ts:15-19` (`BuilderWindowType` gains `tearingDown?: boolean`)
- Modify: `emain/preload.ts:68` (and add `ensureBuilderTab`)
- Modify: `frontend/types/custom.d.ts:71-75` (new global types after `BuilderInitOpts`), `:126` (`ElectronApi`)
- Modify: `frontend/preview/mock/preview-electron-api.ts:55`

**Interfaces:**
- Consumes: Task 6 `RpcApi.EnsureBuilderTabCommand(client, {builderid, appid})` returning `{tabid, appid}`; Task 7 `RpcApi.OpenBuilderTerminalCommand(client, {builderid, targetblockid?, targetaction?})`; Task 8 `DeleteBuilderCommand` accepts the `electron` route.
- Produces:
  - global TS types `BuilderTerminalTarget = { targetblockid?: string; targetaction?: string }`, `BuilderTabInfo = { tabid?: string; appid?: string; error?: string }`
  - `ElectronApi.ensureBuilderTab(): Promise<BuilderTabInfo>` (IPC `ensure-builder-tab`), `ElectronApi.openBuilderTerminal(target?: BuilderTerminalTarget): Promise<string>` (IPC `open-builder-terminal`; `""` on success, else an error message)
  - `emain-builder-select.ts`: `MaxBuilderTargetLen = 64`, `BuilderTeardownTimeoutMs = 20000`, `parseBuilderTerminalTarget(target: unknown): ParsedBuilderTerminalTarget` (`{ targetblockid: string; targetaction: string; error?: string }`), `TeardownWindow = { tearingDown?: boolean; isDestroyed(): boolean; hide(): void }`, `runBuilderTeardown(win: TeardownWindow, steps: BuilderTeardownSteps): Promise<void>`
  - `destroyBuilderWindow` returns at once if the window is already tearing down or destroyed; otherwise it marks it, hides it, awaits `DeleteBuilderCommand` (errors logged, up to 20 s), deletes the builder rtinfo, and destroys the window if nothing else has. The `closed` handler's no-response `DeleteBuilderCommand` (`emain/emain-builder.ts:116-127`) stays as the fallback for title-bar closes.

`ensure-builder-tab` reads `bw.builderId` and `bw.builderAppId` from the calling window only. The panel cannot call it before the window knows its app id (see "Spec items confirmed in the plan").

- [ ] **Step 1: Write the failing tests**

First, from the repo root, make sure Vitest cannot write into a `node_modules` shared with another worktree (a symlinked one shares its `.vite` cache with the live tree; see `llm-wiki/wiki/concepts/remoteterm-fork-engineering-gotchas.md`, "A worktree that symlinks the live `node_modules`"):

```bash
test -L node_modules && { echo "shared node_modules: stop"; exit 1; }
```

Expected: no output. If it prints, stop and report.

Append to `emain/emain-builder-select.test.ts` and add `parseBuilderTerminalTarget`, `runBuilderTeardown` to its import from `./emain-builder-select`:

```ts
describe("parseBuilderTerminalTarget", () => {
    it("treats a missing target as append", () => {
        const empty = { targetblockid: "", targetaction: "" };
        expect(parseBuilderTerminalTarget(undefined)).toEqual(empty);
        expect(parseBuilderTerminalTarget(null)).toEqual(empty);
        expect(parseBuilderTerminalTarget({})).toEqual(empty);
    });

    it("passes short strings through", () => {
        expect(parseBuilderTerminalTarget({ targetblockid: "b1", targetaction: "splitright" })).toEqual({
            targetblockid: "b1",
            targetaction: "splitright",
        });
        expect(parseBuilderTerminalTarget({ targetblockid: "x".repeat(64) }).error).toBeUndefined();
    });

    it("rejects non-strings and strings over 64 characters", () => {
        for (const bad of [
            { targetblockid: 5 },
            { targetaction: ["splitright"] },
            { targetblockid: "x".repeat(65) },
            "splitright",
            7,
        ]) {
            expect(parseBuilderTerminalTarget(bad)).toEqual({
                targetblockid: "",
                targetaction: "",
                error: "Invalid terminal target.",
            });
        }
    });
});

function makeTeardownWindow() {
    let destroyed = false;
    return {
        tearingDown: false,
        hide: vi.fn(),
        isDestroyed: () => destroyed,
        destroy: vi.fn(() => {
            destroyed = true;
        }),
    };
}

describe("runBuilderTeardown", () => {
    it("hides the window, deletes the builder, then its rtinfo, then destroys the window", async () => {
        const order: string[] = [];
        const win = makeTeardownWindow();
        win.hide.mockImplementation(() => order.push("hide"));
        await runBuilderTeardown(win, {
            deleteBuilder: async () => {
                order.push("delete");
            },
            deleteRtInfo: async () => {
                order.push("rtinfo");
            },
            destroyWindow: () => {
                order.push("destroy");
                win.destroy();
            },
            logError: vi.fn(),
        });
        expect(order).toEqual(["hide", "delete", "rtinfo", "destroy"]);
    });

    it("waits for the delete before going on", async () => {
        const order: string[] = [];
        let finish: () => void;
        const win = makeTeardownWindow();
        const done = runBuilderTeardown(win, {
            deleteBuilder: () =>
                new Promise<void>((resolve) => {
                    finish = () => {
                        order.push("delete");
                        resolve();
                    };
                }),
            deleteRtInfo: async () => {
                order.push("rtinfo");
            },
            destroyWindow: () => order.push("destroy"),
            logError: vi.fn(),
        });
        await Promise.resolve();
        expect(order).toEqual([]);
        finish();
        await done;
        expect(order).toEqual(["delete", "rtinfo", "destroy"]);
    });

    it("still destroys the window when both RPCs fail", async () => {
        const order: string[] = [];
        const logError = vi.fn();
        await runBuilderTeardown(makeTeardownWindow(), {
            deleteBuilder: async () => {
                order.push("delete");
                throw new Error("server gone");
            },
            deleteRtInfo: async () => {
                order.push("rtinfo");
                throw new Error("server gone");
            },
            destroyWindow: () => order.push("destroy"),
            logError,
        });
        expect(order).toEqual(["delete", "rtinfo", "destroy"]);
        expect(logError).toHaveBeenCalledTimes(2);
    });

    it("runs the teardown and destroys the window once when asked twice at the same time", async () => {
        const win = makeTeardownWindow();
        const deleteBuilder = vi.fn(() => new Promise<void>((resolve) => setTimeout(resolve, 10)));
        const steps = {
            deleteBuilder,
            deleteRtInfo: vi.fn(async () => {}),
            destroyWindow: () => win.destroy(),
            logError: vi.fn(),
        };
        await Promise.all([runBuilderTeardown(win, steps), runBuilderTeardown(win, steps)]);
        expect(deleteBuilder).toHaveBeenCalledTimes(1);
        expect(steps.deleteRtInfo).toHaveBeenCalledTimes(1);
        expect(win.hide).toHaveBeenCalledTimes(1);
        expect(win.destroy).toHaveBeenCalledTimes(1);
    });

    it("leaves a window alone that is already destroyed, or destroyed during the teardown", async () => {
        const gone = makeTeardownWindow();
        gone.destroy();
        const steps = {
            deleteBuilder: vi.fn(async () => {}),
            deleteRtInfo: vi.fn(async () => {}),
            destroyWindow: vi.fn(),
            logError: vi.fn(),
        };
        await runBuilderTeardown(gone, steps);
        expect(steps.deleteBuilder).not.toHaveBeenCalled();
        expect(gone.hide).not.toHaveBeenCalled();

        const closing = makeTeardownWindow();
        await runBuilderTeardown(closing, { ...steps, deleteBuilder: async () => closing.destroy() });
        expect(steps.destroyWindow).not.toHaveBeenCalled();
    });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx vitest run emain/emain-builder-select.test.ts`
Expected: the new suites fail because `parseBuilderTerminalTarget` and `runBuilderTeardown` are not exported yet (they throw on the first call); the older suites still pass.

- [ ] **Step 3: Implement the helpers**

Append to `emain/emain-builder-select.ts`:

```ts
export const MaxBuilderTargetLen = 64;
export const BuilderTeardownTimeoutMs = 20000;

const InvalidTargetMessage = "Invalid terminal target.";

export type ParsedBuilderTerminalTarget = {
    targetblockid: string;
    targetaction: string;
    error?: string;
};

// The server validates the action and the block, but the IPC argument is still untyped data from a
// renderer, so only short strings are forwarded.
export function parseBuilderTerminalTarget(target: unknown): ParsedBuilderTerminalTarget {
    const rtn: ParsedBuilderTerminalTarget = { targetblockid: "", targetaction: "" };
    if (target == null) {
        return rtn;
    }
    if (typeof target !== "object") {
        return { ...rtn, error: InvalidTargetMessage };
    }
    const fields = target as Record<string, unknown>;
    for (const key of ["targetblockid", "targetaction"] as const) {
        const val = fields[key];
        if (val == null) {
            continue;
        }
        if (typeof val !== "string" || val.length > MaxBuilderTargetLen) {
            return { targetblockid: "", targetaction: "", error: InvalidTargetMessage };
        }
        rtn[key] = val;
    }
    return rtn;
}

export type TeardownWindow = {
    tearingDown?: boolean;
    isDestroyed(): boolean;
    hide(): void;
};

export type BuilderTeardownSteps = {
    deleteBuilder: () => Promise<unknown>;
    deleteRtInfo: () => Promise<unknown>;
    destroyWindow: () => void;
    logError: (message: string, err: unknown) => void;
};

// The teardown can take up to BuilderTeardownTimeoutMs, so the window is hidden at once and a second close
// request meanwhile (Alt+W again, set-builder-window-appid) is ignored. The builder's terminals are deleted
// while its rtinfo still exists, and the window goes last whatever failed, so a dead server cannot keep it open.
export async function runBuilderTeardown(win: TeardownWindow, steps: BuilderTeardownSteps): Promise<void> {
    if (win.tearingDown || win.isDestroyed()) {
        return;
    }
    win.tearingDown = true;
    win.hide();
    try {
        await steps.deleteBuilder();
    } catch (e) {
        steps.logError("Error deleting builder:", e);
    }
    try {
        await steps.deleteRtInfo();
    } catch (e) {
        steps.logError("Error deleting builder rtinfo:", e);
    }
    if (!win.isDestroyed()) {
        steps.destroyWindow();
    }
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npx vitest run emain/emain-builder-select.test.ts`
Expected: all suites pass.

- [ ] **Step 5: Wire the IPC**

In `frontend/types/custom.d.ts`, after `BuilderInitOpts` (ends line 75) add:

```ts
    type BuilderTerminalTarget = {
        targetblockid?: string;
        targetaction?: string;
    };

    type BuilderTabInfo = {
        tabid?: string;
        appid?: string;
        error?: string;
    };
```

and replace the `openBuilderTerminal` line in `ElectronApi` (line 126) with:

```ts
        ensureBuilderTab: () => Promise<BuilderTabInfo>; // ensure-builder-tab
        openBuilderTerminal: (target?: BuilderTerminalTarget) => Promise<string>; // open-builder-terminal
```

In `emain/preload.ts`, replace line 68 (`openBuilderTerminal: () => ipcRenderer.invoke("open-builder-terminal"),`) with:

```ts
    ensureBuilderTab: () => ipcRenderer.invoke("ensure-builder-tab"),
    openBuilderTerminal: (target?: BuilderTerminalTarget) => ipcRenderer.invoke("open-builder-terminal", target),
```

In `frontend/preview/mock/preview-electron-api.ts`, replace line 55 with:

```ts
    ensureBuilderTab: () => Promise.resolve({ error: "Not available in the preview." }),
    openBuilderTerminal: (_target?: BuilderTerminalTarget) => Promise.resolve(""),
```

In `emain/emain-builder.ts`, add a field to `BuilderWindowType` (lines 15-19):

```ts
export type BuilderWindowType = BrowserWindow & {
    builderId: string;
    builderAppId?: string;
    savedInitOpts: BuilderInitOpts;
    tearingDown?: boolean;
};
```

In `emain/emain-ipc.ts`, add `BuilderTeardownTimeoutMs`, `parseBuilderTerminalTarget`, `runBuilderTeardown` to the `./emain-builder-select` import. Replace `destroyBuilderWindow` (lines 226-251) with:

```ts
async function destroyBuilderWindow(bw: BuilderWindowType) {
    const builderId = bw.builderId;
    await runBuilderTeardown(bw, {
        deleteBuilder: async () => {
            if (!builderId) {
                return;
            }
            await RpcApi.DeleteBuilderCommand(ElectronWshClient, builderId, { timeout: BuilderTeardownTimeoutMs });
        },
        deleteRtInfo: async () => {
            if (!builderId) {
                return;
            }
            await RpcApi.SetRTInfoCommand(ElectronWshClient, {
                oref: `builder:${builderId}`,
                data: {} as ObjRTInfo,
                delete: true,
            });
        },
        destroyWindow: () => {
            const wc = bw.webContents;
            if (wc.isDevToolsOpened()) {
                wc.closeDevTools();
            }
            for (const guest of electron.webContents.getAllWebContents()) {
                if (guest.getType() === "webview" && guest.hostWebContents?.id === wc.id) {
                    if (guest.isDevToolsOpened()) {
                        guest.closeDevTools();
                    }
                }
            }
            bw.destroy();
        },
        logError: (message, err) => console.error(message, err),
    });
}
```

Replace the `open-builder-terminal` handler (Task 7 version) with these two handlers:

```ts
    electron.ipcMain.handle("ensure-builder-tab", async (event): Promise<BuilderTabInfo> => {
        const bw = getBuilderWindowByWebContentsId(event.sender.id);
        if (bw == null) {
            return { error: "This action is only available in a builder window." };
        }
        if (!bw.builderAppId) {
            return { error: "No app is open in this builder window." };
        }
        try {
            const rtn = await RpcApi.EnsureBuilderTabCommand(ElectronWshClient, {
                builderid: bw.builderId,
                appid: bw.builderAppId,
            });
            return { tabid: rtn.tabid, appid: rtn.appid };
        } catch (e) {
            return { error: `Could not start the terminals: ${e instanceof Error ? e.message : String(e)}` };
        }
    });

    electron.ipcMain.handle("open-builder-terminal", async (event, target: unknown): Promise<string> => {
        const bw = getBuilderWindowByWebContentsId(event.sender.id);
        if (bw == null) {
            return "This action is only available in a builder window.";
        }
        const parsed = parseBuilderTerminalTarget(target);
        if (parsed.error) {
            return parsed.error;
        }
        try {
            await RpcApi.OpenBuilderTerminalCommand(ElectronWshClient, {
                builderid: bw.builderId,
                targetblockid: parsed.targetblockid,
                targetaction: parsed.targetaction,
            });
        } catch (e) {
            return `Could not open a terminal: ${e instanceof Error ? e.message : String(e)}`;
        }
        return "";
    });
```

- [ ] **Step 6: Run the checks**

Run: `npx tsc --noEmit && npx vitest run emain/`
Expected: tsc exit 0; every `emain/*.test.ts` passes.

- [ ] **Step 7: Commit**

```bash
git add emain/emain-builder-select.ts emain/emain-builder-select.test.ts emain/emain-ipc.ts emain/emain-builder.ts emain/preload.ts frontend/types/custom.d.ts frontend/preview/mock/preview-electron-api.ts
git commit -m "feat(builder): Electron bridges the builder terminal panel

The builder window can now ask for its terminal tab (with its own
builder and app ids, never the page's), open panes with a split target,
and on close hides the window at once, waits for the terminals to be
deleted, then removes it; a second close request meanwhile is ignored.

Miscellanea: ensure-builder-tab IPC; target strings capped at 64
characters; teardown ordering helper with tests.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 11: Renderer store foundation (writable static tab, `uiContext`, builder subscriptions, null-safe layout callers)

**Files:**
- Modify: `frontend/app/store/global-atoms.ts:14-25, 65-66`
- Modify: `frontend/types/custom.d.ts:19` (`staticTabId` type)
- Modify: `frontend/app/store/global.ts:55-94` (subscriptions), `:493-496` (`setNodeFocus`), `:720-740` (`refocusNode`), exports `:853-912`
- Modify: `frontend/remoteterm.ts:252` (builder subscriptions)
- Modify: `frontend/layout/lib/layoutModelHooks.ts:45-48`
- Modify: `frontend/app/store/keymodel.ts:67-71, 146-196, 244-261, 371-385, 498-510, 633-660`
- Modify: `frontend/app/store/focusManager.ts:4, 15-48`
- Modify: `frontend/builder/store/builder-apppanel-model.ts:50` (`configUnsubFn`), `:130-138` (its own `config` subscription), `:542-545` (its cleanup in `dispose`)
- Test: `frontend/app/store/global-atoms.test.ts` (append), `frontend/app/store/global-builder-subs.test.ts` (new), `frontend/layout/tests/layoutModelHooks.test.ts` (new), `frontend/app/store/keymodel-builder.test.ts` (new), `frontend/app/store/focusManager.test.ts` (new), `frontend/builder/store/builder-apppanel-subs.test.ts` (new)

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `atoms.staticTabId: PrimitiveAtom<string>` (`GlobalAtomsType.staticTabId: jotai.PrimitiveAtom<string>`); only the builder bootstrap (Task 14) writes it in production code
  - `atoms.uiContext` reads `staticTabId` at call time
  - `initBuilderWaveEventSubs(): void` exported from `@/app/store/global` (`waveobj:update`, `config`, `blockfile`, badges; no `userinput`); `BuilderAppPanelModel` no longer subscribes to `config` itself (it did so only because builder windows had no global subscription; `configUnsubFn` has no other user, `grep -rn configUnsubFn frontend` lists only that file), so a builder window has one `config` subscription
  - `FocusManager.blockFocusAtom` depends on `atoms.staticTabId`, so it recomputes once a builder sets its tab
  - `getLayoutModelForStaticTab()` returns `null` while `staticTabId` is unset
  - keymodel (module-private, used by Task 16): `magnifyFocusedNode()`, `activateSearch(event)`, `deactivateSearch()` hoisted to module level and null-safe; `getFocusedBlockInStaticTab()`, `switchBlockInDirection()`, `globalRefocus()`, `uxCloseBlock()`, `genericClose()` return early without a layout model
  - test helpers local to `keymodel-builder.test.ts`: `h` (hoisted state), `linuxKey(desc: string): WaveKeyboardEvent`, `makeLayoutModel(focusedBlockId: string)`

- [ ] **Step 1: Write the failing tests**

Append to `frontend/app/store/global-atoms.test.ts` (and add `import { globalStore } from "./jotaiStore";` and `initGlobalAtoms` to the `./global-atoms` import):

```ts
describe("uiContext", () => {
    it("reads the static tab id when it is called, so a builder can set it after init", () => {
        vi.spyOn(console, "log").mockImplementation(() => {});
        initGlobalAtoms({ windowId: "win-1", builderId: "builder-1", platform: "linux", environment: "renderer" } as GlobalInitOptions);
        const atoms = getAtoms();
        expect(globalStore.get(atoms.uiContext).activetabid).toBeUndefined();
        globalStore.set(atoms.staticTabId, "tab-9");
        expect(globalStore.get(atoms.uiContext)).toEqual({ windowid: "win-1", activetabid: "tab-9" });
    });

    it("is unchanged for main windows", () => {
        initGlobalAtoms({ windowId: "win-2", tabId: "tab-2", platform: "linux", environment: "renderer" } as GlobalInitOptions);
        expect(globalStore.get(getAtoms().uiContext)).toEqual({ windowid: "win-2", activetabid: "tab-2" });
    });
});
```

(Its `vitest` import gains `vi`.)

Create `frontend/app/store/global-builder-subs.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ subscribe: vi.fn(), badges: vi.fn() }));

vi.mock("@/layout/index", () => ({
    getLayoutModelForStaticTab: vi.fn(() => null),
    LayoutTreeActionType: {},
    newLayoutNode: vi.fn(),
}));
vi.mock("./services", () => ({ ObjectService: {}, ClientService: {} }));
vi.mock("./wps", () => ({ waveEventSubscribeSingle: h.subscribe, peekFileSubject: vi.fn() }));
vi.mock("./badge", () => ({ setupBadgesSubscription: h.badges }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));

import { initBuilderWaveEventSubs, initGlobalWaveEventSubs, refocusNode, setNodeFocus } from "./global";

function subscribedEvents(): string[] {
    return h.subscribe.mock.calls.map((call) => call[0].eventType).sort();
}

describe("builder subscriptions", () => {
    beforeEach(() => {
        h.subscribe.mockClear();
        h.badges.mockClear();
    });

    it("subscribe builder windows to object, config and blockfile events and badges, not userinput", () => {
        initBuilderWaveEventSubs();
        expect(subscribedEvents()).toEqual(["blockfile", "config", "waveobj:update"]);
        expect(h.badges).toHaveBeenCalledTimes(1);
    });

    it("keep the userinput subscription for main windows", () => {
        initGlobalWaveEventSubs({ windowId: "win-1" } as RemoteTermInitOpts);
        expect(subscribedEvents()).toEqual(["blockfile", "config", "userinput", "waveobj:update"]);
        expect(h.badges).toHaveBeenCalledTimes(1);
    });
});

describe("focus helpers without a layout model", () => {
    it("do nothing instead of throwing", () => {
        expect(() => setNodeFocus("node-1")).not.toThrow();
        expect(() => refocusNode("block-1")).not.toThrow();
    });
});
```

Create `frontend/layout/tests/layoutModelHooks.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ getWaveObjectAtom: vi.fn() }));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    return {
        atoms: { staticTabId: atom(null) },
        globalStore,
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}`, getWaveObjectAtom: h.getWaveObjectAtom },
    };
});
vi.mock("@/app/hook/useDimensions", () => ({ useOnResize: vi.fn() }));
vi.mock("../lib/layoutModel", () => ({ LayoutModel: vi.fn() }));

import { getLayoutModelForStaticTab } from "../lib/layoutModelHooks";

describe("getLayoutModelForStaticTab", () => {
    it("returns null before a static tab is set, without creating a tab:null object", () => {
        expect(getLayoutModelForStaticTab()).toBeNull();
        expect(h.getWaveObjectAtom).not.toHaveBeenCalled();
    });
});
```

Create `frontend/app/store/keymodel-builder.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as keyutil from "@/util/keyutil";
import { atom } from "jotai";
import { beforeAll, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    api: {
        closeBuilderWindow: vi.fn(),
        closeTab: vi.fn(() => Promise.resolve(true)),
        openBuilderTerminal: vi.fn(() => Promise.resolve("")),
        registerGlobalWebviewKeys: vi.fn(),
        setKeyboardChordMode: vi.fn(),
    },
    layoutModel: null as any,
    windowType: "tab",
    blockCount: 2,
    bcms: new Map<string, any>(),
}));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    return {
        atoms: {
            staticTabId: atom("tab-1"),
            modalOpen: atom(false),
            workspaceId: atom("ws-1"),
            controlShiftDelayAtom: atom(false),
            newTabDropdownOpen: atom(false),
        },
        createBlock: vi.fn(),
        createBlockSplitHorizontally: vi.fn(),
        createBlockSplitVertically: vi.fn(),
        getAllBlockComponentModels: vi.fn(() => []),
        getApi: () => h.api,
        getBlockComponentModel: (blockId: string) => h.bcms.get(blockId),
        getBlockMetaKeyAtom: vi.fn(() => atom(null)),
        getFocusedBlockId: vi.fn(),
        getSettingsKeyAtom: vi.fn(() => atom(false)),
        globalStore,
        refocusNode: vi.fn(),
        replaceBlock: vi.fn(),
        WOS: {
            makeORef: (otype: string, oid: string) => `${otype}:${oid}`,
            getWaveObjectAtom: () => atom({ blockids: Array.from({ length: h.blockCount }, (_, i) => `b${i + 1}`) }),
        },
    };
});
vi.mock("@/app/store/focusManager", () => ({ FocusManager: { getInstance: vi.fn() } }));
vi.mock("@/app/store/services", () => ({ UserInputService: {} }));
vi.mock("@/app/store/tab-model", () => ({ getActiveTabModel: vi.fn(() => null) }));
vi.mock("@/app/workspace/workspace-layout-model", () => ({ WorkspaceLayoutModel: {} }));
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: vi.fn(),
    getLayoutModelForStaticTab: vi.fn(() => h.layoutModel),
    NavigateDirection: { Up: 0, Right: 1, Down: 2, Left: 3 },
}));
vi.mock("./windowtype", () => ({
    isBuilderWindow: () => h.windowType === "builder",
    isTabWindow: () => h.windowType === "tab",
}));

import { appHandleKeyDown, registerGlobalKeys, uxCloseBlock } from "./keymodel";

function linuxKey(desc: string): WaveKeyboardEvent {
    const ev: any = { type: "keydown", key: "", code: "", cmd: false, alt: false, option: false, meta: false, control: false, shift: false };
    for (const part of desc.split(":")) {
        if (part === "Cmd") {
            ev.cmd = true;
            ev.alt = true;
        } else if (part === "Shift") {
            ev.shift = true;
        } else if (part === "Ctrl") {
            ev.control = true;
        } else if (part.startsWith("c{")) {
            ev.code = part.slice(2, -1);
        } else {
            ev.key = part;
        }
    }
    return ev as WaveKeyboardEvent;
}

function makeLayoutModel(focusedBlockId: string) {
    const focused = focusedBlockId == null ? undefined : { id: `node-${focusedBlockId}`, data: { blockId: focusedBlockId } };
    return {
        focusedNode: atom(focused),
        ephemeralNode: atom(undefined),
        getNodeByBlockId: vi.fn((blockId: string) => (blockId === focusedBlockId ? focused : null)),
        closeNode: vi.fn(() => Promise.resolve()),
        closeFocusedNode: vi.fn(() => Promise.resolve()),
        switchNodeFocusInDirection: vi.fn(),
        switchNodeFocusByBlockNum: vi.fn(),
        magnifyNodeToggle: vi.fn(),
        addEphemeralNodeToLayout: vi.fn(),
        focusFirstNode: vi.fn(),
        focusNode: vi.fn(),
    };
}

describe("keymodel without a layout model or with an empty tree", () => {
    beforeAll(() => {
        keyutil.setKeyUtilPlatform("linux");
        registerGlobalKeys();
    });

    for (const [label, makeModel] of [
        ["no layout model", () => null],
        ["an empty tree", () => makeLayoutModel(null)],
    ] as const) {
        it(`does not throw with ${label}`, () => {
            h.windowType = "tab";
            h.layoutModel = makeModel();
            for (const desc of ["Escape", "Cmd:m", "Ctrl:Shift:ArrowLeft", "Cmd:f", "Ctrl:Shift:c{Digit1}"]) {
                expect(() => appHandleKeyDown(linuxKey(desc)), desc).not.toThrow();
            }
            expect(() => uxCloseBlock("b1")).not.toThrow();
        });
    }
});
```

Create `frontend/app/store/focusManager.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom } from "jotai";
import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ layoutModel: null as any }));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    return { atoms: { staticTabId: atom(null) }, getBlockComponentModel: vi.fn() };
});
vi.mock("@/layout/index", () => ({ getLayoutModelForStaticTab: () => h.layoutModel }));

import { atoms } from "@/app/store/global";
import { FocusManager } from "./focusManager";
import { globalStore } from "./jotaiStore";

describe("FocusManager.blockFocusAtom", () => {
    it("recomputes once a builder window sets its static tab", () => {
        const focusAtom = FocusManager.getInstance().blockFocusAtom;
        expect(globalStore.get(focusAtom)).toBeNull();
        h.layoutModel = { focusedNode: atom({ id: "node-b1", data: { blockId: "b1" } }) };
        globalStore.set(atoms.staticTabId, "tab-1");
        expect(globalStore.get(focusAtom)).toBe("b1");
    });
});
```

Create `frontend/builder/store/builder-apppanel-subs.test.ts` (the `@/layout/index` mock is unused until Task 14 makes the model import `builder-term-model.ts`; it is there so this file keeps loading then):

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ subscribe: vi.fn((_sub: { eventType: string }) => () => {}) }));

vi.mock("@/app/store/wshclientapi", () => ({
    // Every RPC resolves to an empty object; initialize() tolerates that, and only its subscriptions matter here.
    RpcApi: new Proxy({}, { get: () => async () => ({}) }),
}));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: h.subscribe }));
vi.mock("@/layout/index", () => ({ deleteLayoutModelForTab: vi.fn() }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    const settingAtom = atom(false);
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app"), staticTabId: atom(null), fullConfigAtom: atom(null) },
        getApi: vi.fn(() => ({})),
        getSettingsKeyAtom: vi.fn(() => settingAtom),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}` },
    };
});

import { BuilderAppPanelModel } from "./builder-apppanel-model";

describe("BuilderAppPanelModel.initialize", () => {
    it("leaves the config subscription to initBuilderWaveEventSubs", async () => {
        vi.spyOn(console, "error").mockImplementation(() => {});
        await BuilderAppPanelModel.getInstance().initialize();
        const events = h.subscribe.mock.calls.map((call) => call[0].eventType);
        expect(events).toContain("builderstatus");
        expect(events).not.toContain("config");
    });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx vitest run frontend/app/store/global-atoms.test.ts frontend/app/store/global-builder-subs.test.ts frontend/layout/tests/layoutModelHooks.test.ts frontend/app/store/keymodel-builder.test.ts frontend/app/store/focusManager.test.ts frontend/builder/store/builder-apppanel-subs.test.ts`
Expected: FAIL. `uiContext` reads `undefined` after the set (the closure ignores `staticTabId`); `initBuilderWaveEventSubs` is not exported; `setNodeFocus`/`refocusNode` throw (they dereference a null layout model); `getLayoutModelForStaticTab` throws or the `not.toHaveBeenCalled` assertion fails (it builds a `tab:null` object); the keymodel cases fail their `not.toThrow` assertions; the FocusManager test reads `null` after the tab is set (the atom has no dependency to recompute on); the apppanel test finds a `config` subscription.

- [ ] **Step 3: Make `staticTabId` writable and `uiContext` live**

In `frontend/app/store/global-atoms.ts`, delete the `staticTabIdAtom` lines 65-66 (keep the comment by moving it) and replace lines 19-25 (`const uiContextAtom = ...`) with:

```ts
    // this is *the* tab that this tabview represents.  it should never change.
    // Builder windows have no tab at init; their terminal panel sets it once, after loading its tab.
    const staticTabIdAtom = atom(initOpts.tabId) as PrimitiveAtom<string>;
    const uiContextAtom = atom((get) => {
        const uiContext: UIContext = {
            windowid: initOpts.windowId,
            activetabid: get(staticTabIdAtom),
        };
        return uiContext;
    }) as Atom<UIContext>;
```

In `frontend/types/custom.d.ts` line 19, change `staticTabId: jotai.Atom<string>;` to:

```ts
        staticTabId: jotai.PrimitiveAtom<string>; // set at init in main windows; set once by the builder terminal panel
```

- [ ] **Step 4: Builder subscriptions**

In `frontend/app/store/global.ts`, replace `initGlobalWaveEventSubs` (lines 55-94) with the three functions below (the handler bodies and their comments are unchanged, only regrouped):

```ts
function subscribeToSharedWaveEvents() {
    waveEventSubscribeSingle({
        eventType: "waveobj:update",
        handler: (event) => {
            // console.log("waveobj:update wave event handler", event);
            WOS.updateWaveObject(event.data);
        },
    });
    waveEventSubscribeSingle({
        eventType: "config",
        handler: (event) => {
            // console.log("config wave event handler", event);
            globalStore.set(atoms.fullConfigAtom, event.data.fullconfig);
        },
    });
    waveEventSubscribeSingle({
        eventType: "blockfile",
        handler: (event) => {
            // console.log("blockfile event update", event);
            const fileSubject = peekFileSubject(event.data.zoneid, event.data.filename);
            if (fileSubject != null) {
                fileSubject.next(event.data);
            }
        },
    });
    setupBadgesSubscription();
}

function initGlobalWaveEventSubs(initOpts: RemoteTermInitOpts) {
    subscribeToSharedWaveEvents();
    waveEventSubscribeSingle({
        eventType: "userinput",
        handler: (event) => {
            const connName = event.data?.connname;
            if (connName) {
                modalsModel.upsertUserInputPrompt(connName, "UserInputPrompt", { ...event.data });
            } else {
                console.log("[PW-EVENT] userinput event has no connName, using empty key", event.data);
                modalsModel.upsertUserInputPrompt("", "UserInputPrompt", { ...event.data });
            }
        },
        scope: initOpts.windowId,
    });
}

// The config event keeps settings such as the live-rebuild toggle current in builder windows. Builder panes
// are local only, so there is no connection prompt to route and no userinput subscription.
function initBuilderWaveEventSubs() {
    subscribeToSharedWaveEvents();
}
```

Add `initBuilderWaveEventSubs,` to the export list (after `initGlobalWaveEventSubs,`).

In `frontend/remoteterm.ts`, replace line 252 (`await loadConnStatus();` inside `initBuilder`) with:

```ts
    await loadConnStatus();
    await loadBadges();
    initBuilderWaveEventSubs();
    subscribeToConnEvents();
```

and add `initBuilderWaveEventSubs` to the `@/store/global` import (lines 22-30).

Builder windows now get `config` from `initBuilderWaveEventSubs`, so remove `BuilderAppPanelModel`'s own subscription, which existed only because they had none (its comment says so; the comment's point now lives on `initBuilderWaveEventSubs` above). In `frontend/builder/store/builder-apppanel-model.ts`: delete the field `configUnsubFn: (() => void) | null = null;` (line 50); delete lines 130-138 (the three-line comment `// The builder window loads the config once at startup ...` and the `this.configUnsubFn = waveEventSubscribeSingle({ eventType: "config", ... });` call, plus the blank line before it); and delete the `if (this.configUnsubFn) { ... }` block in `dispose()` (lines 542-545). Check: `grep -rn configUnsubFn frontend` prints nothing afterwards.

- [ ] **Step 5: Null-safe layout callers**

In `frontend/layout/lib/layoutModelHooks.ts`, replace `getLayoutModelForStaticTab` (lines 45-48) with:

```ts
export function getLayoutModelForStaticTab() {
    const tabId = globalStore.get(atoms.staticTabId);
    if (tabId == null) {
        return null;
    }
    return getLayoutModelForTabById(tabId);
}
```

In `frontend/app/store/global.ts`, replace `setNodeFocus` (lines 493-496) with:

```ts
function setNodeFocus(nodeId: string) {
    const layoutModel = getLayoutModelForStaticTab();
    layoutModel?.focusNode(nodeId);
}
```

and in `refocusNode` (lines 720-740) replace

```ts
    const layoutModel = getLayoutModelForStaticTab();
    const layoutNodeId = layoutModel.getNodeByBlockId(blockId);
```

with

```ts
    const layoutModel = getLayoutModelForStaticTab();
    if (layoutModel == null) {
        return;
    }
    const layoutNodeId = layoutModel.getNodeByBlockId(blockId);
```

In `frontend/app/store/focusManager.ts`, change line 4 to `import { atoms, getBlockComponentModel } from "@/app/store/global";` and replace the constructor body and `refocusNode` (lines 15-48) with:

```ts
    private constructor() {
        this.blockFocusAtom = atom((get) => {
            // Read so the atom recomputes when a builder window sets its tab after init.
            get(atoms.staticTabId);
            const layoutModel = getLayoutModelForStaticTab();
            if (layoutModel == null) {
                return null;
            }
            const lnode = get(layoutModel.focusedNode);
            return lnode?.data?.blockId;
        });
    }

    static getInstance(): FocusManager {
        if (!FocusManager.instance) {
            FocusManager.instance = new FocusManager();
        }
        return FocusManager.instance;
    }

    nodeFocusWithin(): boolean {
        return focusedBlockId() != null;
    }

    refocusNode() {
        const layoutModel = getLayoutModelForStaticTab();
        if (layoutModel == null) {
            return;
        }
        const lnode = globalStore.get(layoutModel.focusedNode);
        if (lnode == null || lnode.data?.blockId == null) {
            return;
        }
        layoutModel.focusNode(lnode.id);
        const blockId = lnode.data.blockId;
        const bcm = getBlockComponentModel(blockId);
        const ok = bcm?.viewModel?.giveFocus?.();
        if (!ok) {
            const inputElem = document.getElementById(`${blockId}-dummy-focus`);
            inputElem?.focus();
        }
    }
```

In `frontend/app/store/keymodel.ts`:

Replace `getFocusedBlockInStaticTab` (lines 67-71) with:

```ts
// focusedNodeId is not cleared when the root node is deleted, so focusedNode can be undefined
// while the tree is empty (a builder with no panes).
function getFocusedBlockInStaticTab(): string {
    const layoutModel = getLayoutModelForStaticTab();
    if (layoutModel == null) {
        return null;
    }
    const focusedNode = globalStore.get(layoutModel.focusedNode);
    return focusedNode?.data?.blockId;
}
```

In `uxCloseBlock`, replace

```ts
    const layoutModel = getLayoutModelForStaticTab();
    const node = layoutModel.getNodeByBlockId(blockId);
```

with

```ts
    const layoutModel = getLayoutModelForStaticTab();
    const node = layoutModel?.getNodeByBlockId(blockId);
```

In `genericClose`, replace

```ts
    const layoutModel = getLayoutModelForStaticTab();
    fireAndForget(layoutModel.closeFocusedNode.bind(layoutModel));
```

with

```ts
    const layoutModel = getLayoutModelForStaticTab();
    if (layoutModel == null) {
        return;
    }
    fireAndForget(layoutModel.closeFocusedNode.bind(layoutModel));
```

In `switchBlockInDirection` (lines 190-196), insert after `const layoutModel = getLayoutModelForStaticTab();`:

```ts
    if (layoutModel == null) {
        return;
    }
```

In `globalRefocus` (lines 244-261), insert the same three lines after its `const layoutModel = getLayoutModelForStaticTab();`.

In `appHandleKeyDown`, replace

```ts
        const layoutModel = getLayoutModelForStaticTab();
        const focusedNode = globalStore.get(layoutModel.focusedNode);
        const blockId = focusedNode?.data?.blockId;
```

with

```ts
        const layoutModel = getLayoutModelForStaticTab();
        const focusedNode = layoutModel == null ? null : globalStore.get(layoutModel.focusedNode);
        const blockId = focusedNode?.data?.blockId;
```

Add these module-level functions after `handleSplitVertical` (ends line 320):

```ts
function magnifyFocusedNode() {
    const layoutModel = getLayoutModelForStaticTab();
    if (layoutModel == null) {
        return;
    }
    const focusedNode = globalStore.get(layoutModel.focusedNode);
    if (focusedNode == null) {
        return;
    }
    const ephemeralNode = globalStore.get(layoutModel.ephemeralNode);
    if (ephemeralNode?.id === focusedNode.id) {
        layoutModel.addEphemeralNodeToLayout();
        return;
    }
    layoutModel.magnifyNodeToggle(focusedNode.id);
}

function activateSearch(event: WaveKeyboardEvent): boolean {
    const bcm = getBlockComponentModel(getFocusedBlockInStaticTab());
    const viewModel = bcm?.viewModel;
    if (viewModel == null) {
        return false;
    }
    // Ctrl+f is reserved in most shells
    if (event.control && viewModel.viewType == "term") {
        return false;
    }
    if (viewModel.searchAtoms) {
        if (globalStore.get(viewModel.searchAtoms.isOpen)) {
            // Already open — increment the focusInput counter so this block's
            // SearchComponent focuses its own input (avoids a global DOM query
            // that could target the wrong block when multiple searches are open).
            const cur = globalStore.get(viewModel.searchAtoms.focusInput) as number;
            globalStore.set(viewModel.searchAtoms.focusInput, cur + 1);
        } else {
            globalStore.set(viewModel.searchAtoms.isOpen, true);
        }
        return true;
    }
    return false;
}

function deactivateSearch(): boolean {
    const bcm = getBlockComponentModel(getFocusedBlockInStaticTab());
    const searchAtoms = bcm?.viewModel?.searchAtoms;
    if (searchAtoms && globalStore.get(searchAtoms.isOpen)) {
        globalStore.set(searchAtoms.isOpen, false);
        return true;
    }
    return false;
}
```

(The comment inside `activateSearch` is the existing one from lines 641-643, moved with its code; it contains an em-dash that predates this plan, so leave it as it is.)

Then, inside `registerGlobalKeys`, replace the `Cmd:m` handler (lines 498-510) with:

```ts
    globalKeyMap.set("Cmd:m", () => {
        magnifyFocusedNode();
        return true;
    });
```

and delete the nested `function activateSearch(...)` and `function deactivateSearch()` (lines 633-660); the `globalKeyMap.set("Cmd:f", activateSearch);` and `Escape` handler below them now use the module-level functions.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `npx vitest run frontend/app/store/ frontend/layout/ frontend/builder/`
Expected: all pass, including the existing store, layout and builder suites.

- [ ] **Step 7: Type-check**

Run: `npx tsc --noEmit`
Expected: exit 0.

- [ ] **Step 8: Commit**

```bash
git add frontend/app/store/global-atoms.ts frontend/types/custom.d.ts frontend/app/store/global.ts frontend/remoteterm.ts frontend/layout/lib/layoutModelHooks.ts frontend/app/store/keymodel.ts frontend/app/store/focusManager.ts frontend/builder/store/builder-apppanel-model.ts frontend/app/store/global-atoms.test.ts frontend/app/store/global-builder-subs.test.ts frontend/layout/tests/layoutModelHooks.test.ts frontend/app/store/keymodel-builder.test.ts frontend/app/store/focusManager.test.ts frontend/builder/store/builder-apppanel-subs.test.ts
git commit -m "feat(builder): renderer groundwork for a terminal tab in builder windows

Builder windows now receive object, config, blockfile, badge and
connection events, and every keyboard and focus path that reaches the
layout tolerates a window with no tab yet or with an empty tree.

Miscellanea: staticTabIdAtom is a PrimitiveAtom and uiContext reads it
when called; getLayoutModelForStaticTab returns null without a static
tab; search and magnify handlers hoisted to module level; the builder
app panel model drops its own config subscription; FocusManager's block
focus atom recomputes when the static tab is set.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 12: Backend split actions honour `focused` and fall back to insert (`frontend/layout`)

**Files:**
- Create: `frontend/layout/lib/backendsplit.ts`
- Modify: `frontend/layout/lib/layoutModel.ts:515-570` (the `SplitHorizontal` and `SplitVertical` cases of `handleBackendAction`), imports at the top
- Test: `frontend/layout/tests/backendsplit.test.ts`

**Interfaces:**
- Consumes: `newLayoutNode` (`frontend/layout/lib/layoutNode.ts:16-28`), `LayoutTreeActionType` and action types (`frontend/layout/lib/types.ts:82-90, 158-160, 219-234`), reducers `splitHorizontal`/`splitVertical`/`insertNode` (`frontend/layout/lib/layoutTree.ts:279-297, 454-540`), global `LayoutActionData` (generated).
- Produces: `makeBackendSplitAction(action: LayoutActionData, targetNode: LayoutNode): LayoutTreeSplitHorizontalAction | LayoutTreeSplitVerticalAction | LayoutTreeInsertNodeAction`. A queued split carries `focused` (Open always sends `true`, so the new pane takes focus, S3); a split whose target node is gone becomes a focused-as-requested insert instead of being dropped.

- [ ] **Step 1: Write the failing test**

Create `frontend/layout/tests/backendsplit.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { makeBackendSplitAction } from "../lib/backendsplit";
import { newLayoutNode } from "../lib/layoutNode";
import { insertNode, splitHorizontal, splitVertical } from "../lib/layoutTree";
import {
    LayoutTreeActionType,
    LayoutTreeInsertNodeAction,
    LayoutTreeSplitHorizontalAction,
    LayoutTreeSplitVerticalAction,
} from "../lib/types";
import { newLayoutTreeState } from "./model";

function backendAction(actiontype: string, position: string, focused = true): LayoutActionData {
    return {
        actiontype,
        actionid: "action-1",
        blockid: "b2",
        focused,
        magnified: false,
        ephemeral: false,
        targetblockid: "b1",
        position,
    };
}

describe("makeBackendSplitAction", () => {
    it("keeps focused on a horizontal split and focuses the new node", () => {
        const target = newLayoutNode(undefined, undefined, undefined, { blockId: "b1" });
        const state = newLayoutTreeState(target);
        const treeAction = makeBackendSplitAction(backendAction("splithorizontal", "after"), target);
        expect(treeAction.type).toBe(LayoutTreeActionType.SplitHorizontal);
        splitHorizontal(state, treeAction as LayoutTreeSplitHorizontalAction);
        const newNode = (treeAction as LayoutTreeSplitHorizontalAction).newNode;
        expect(newNode.data.blockId).toBe("b2");
        expect(state.focusedNodeId).toBe(newNode.id);
    });

    it("keeps focused on a vertical split", () => {
        const target = newLayoutNode(undefined, undefined, undefined, { blockId: "b1" });
        const state = newLayoutTreeState(target);
        const treeAction = makeBackendSplitAction(backendAction("splitvertical", "before"), target);
        expect(treeAction.type).toBe(LayoutTreeActionType.SplitVertical);
        expect((treeAction as LayoutTreeSplitVerticalAction).position).toBe("before");
        splitVertical(state, treeAction as LayoutTreeSplitVerticalAction);
        expect(state.focusedNodeId).toBe((treeAction as LayoutTreeSplitVerticalAction).newNode.id);
    });

    it("does not focus when the backend did not ask", () => {
        const target = newLayoutNode(undefined, undefined, undefined, { blockId: "b1" });
        const state = newLayoutTreeState(target);
        const treeAction = makeBackendSplitAction(backendAction("splithorizontal", "after", false), target);
        splitHorizontal(state, treeAction as LayoutTreeSplitHorizontalAction);
        expect(state.focusedNodeId).toBeUndefined();
    });

    it("inserts the block when the target node no longer exists", () => {
        const other = newLayoutNode(undefined, undefined, undefined, { blockId: "b3" });
        const state = newLayoutTreeState(other);
        const treeAction = makeBackendSplitAction(backendAction("splitvertical", "after"), null);
        expect(treeAction.type).toBe(LayoutTreeActionType.InsertNode);
        insertNode(state, treeAction as LayoutTreeInsertNodeAction);
        const inserted = (treeAction as LayoutTreeInsertNodeAction).node;
        expect(inserted.data.blockId).toBe("b2");
        expect(state.focusedNodeId).toBe(inserted.id);
    });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npx vitest run frontend/layout/tests/backendsplit.test.ts`
Expected: the suite fails to load because `../lib/backendsplit` does not exist yet.

- [ ] **Step 3: Implement**

Create `frontend/layout/lib/backendsplit.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { newLayoutNode } from "./layoutNode";
import {
    LayoutNode,
    LayoutTreeActionType,
    LayoutTreeInsertNodeAction,
    LayoutTreeSplitHorizontalAction,
    LayoutTreeSplitVerticalAction,
} from "./types";

// A queued split whose target was closed meanwhile still carries a live block; dropping the action
// would leave that block (and its shell) with no node, so it is inserted instead.
export function makeBackendSplitAction(
    action: LayoutActionData,
    targetNode: LayoutNode
): LayoutTreeSplitHorizontalAction | LayoutTreeSplitVerticalAction | LayoutTreeInsertNodeAction {
    const newNode = newLayoutNode(undefined, action.nodesize, undefined, { blockId: action.blockid });
    if (targetNode == null) {
        const insertAction: LayoutTreeInsertNodeAction = {
            type: LayoutTreeActionType.InsertNode,
            node: newNode,
            magnified: false,
            focused: action.focused,
        };
        return insertAction;
    }
    const position = action.position as "before" | "after";
    if (action.actiontype === LayoutTreeActionType.SplitVertical) {
        const verticalAction: LayoutTreeSplitVerticalAction = {
            type: LayoutTreeActionType.SplitVertical,
            targetNodeId: targetNode.id,
            newNode,
            position,
            focused: action.focused,
        };
        return verticalAction;
    }
    const horizontalAction: LayoutTreeSplitHorizontalAction = {
        type: LayoutTreeActionType.SplitHorizontal,
        targetNodeId: targetNode.id,
        newNode,
        position,
        focused: action.focused,
    };
    return horizontalAction;
}
```

In `frontend/layout/lib/layoutModel.ts`, add `import { makeBackendSplitAction } from "./backendsplit";` with the other `./` imports, and replace the two cases `case LayoutTreeActionType.SplitHorizontal: { ... }` and `case LayoutTreeActionType.SplitVertical: { ... }` (lines 515-570) with:

```ts
            case LayoutTreeActionType.SplitHorizontal:
            case LayoutTreeActionType.SplitVertical: {
                if (action.position != "before" && action.position != "after") {
                    console.error(
                        "Cannot apply eventbus layout action",
                        action.actiontype,
                        "invalid position",
                        action.position
                    );
                    break;
                }
                const targetNode = this?.getNodeByBlockId(action.targetblockid);
                if (!targetNode) {
                    console.warn("Split target is gone, inserting the new block instead", action.targetblockid);
                }
                this.treeReducer(makeBackendSplitAction(action, targetNode), false);
                break;
            }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npx vitest run frontend/layout/ && npx tsc --noEmit`
Expected: all layout suites pass; tsc exit 0.

- [ ] **Step 5: Commit**

```bash
git add frontend/layout/lib/backendsplit.ts frontend/layout/lib/layoutModel.ts frontend/layout/tests/backendsplit.test.ts
git commit -m "fix(layout): server-side splits focus the new pane and survive a closed target

A split queued by the server now focuses the new pane when asked, and if
the pane it targeted was closed in the meantime the new pane is inserted
instead of being dropped (which left a running shell with no pane).

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 13: Builder terminal client helpers and the central create intercept (`frontend/app/store`)

**Files:**
- Create: `frontend/app/store/builder-terminal.ts`
- Test: `frontend/app/store/builder-terminal.test.ts`
- Modify: `frontend/app/store/global.ts:377-460` (create family), `:627-645` (`hideBlockKeepAlive`), imports `:30-38`
- Test: `frontend/app/store/global-builder-create.test.ts`
- Modify: `frontend/builder/store/builder-apppanel-model.ts:40` (`noticeAtom`), `:190-193` (`openTerminal`; it was `:200-203` before Task 11 removed 10 lines above it), imports
- Test: `frontend/builder/store/builder-apppanel-model.test.ts` (append)

**Interfaces:**
- Consumes: Task 10 `ElectronApi.openBuilderTerminal(target?: BuilderTerminalTarget): Promise<string>`; Task 11 `getLayoutModelForStaticTab()` may return `null`; `isBuilderWindow()` (`frontend/app/store/windowtype.ts:11-13`).
- Produces (`@/app/store/builder-terminal`):
  - `type BuilderTermAction = "" | "splitright" | "splitleft" | "splitup" | "splitdown"`
  - `BuilderNoticeAtom: PrimitiveAtom<string>` (also `BuilderAppPanelModel.getInstance().noticeAtom`, so the app header's notice strip shows Open errors)
  - `isTermBlockDef(blockDef: BlockDef): boolean`
  - `splitActionFor(direction: "horizontal" | "vertical", position: "before" | "after"): BuilderTermAction`
  - `openBuilderTerminal(action: BuilderTermAction, targetBlockId: string): Promise<string>` (sets the notice to the error, or clears it)
  - `showConnectionUi(): boolean` (false in builder windows; used by Task 16)
- Produces (`@/app/store/global`): in builder windows `createBlock`, `createBlockSplitHorizontally`, `createBlockSplitVertically` route `view: "term"` defs to `openBuilderTerminal` and return `null`; `replaceBlock` with a term def is a no-op returning `null`; other views go through `ObjectService` into the builder tab, and throw before creating anything if the window has no layout yet; `hideBlockKeepAlive` returns `false` in builder windows (keep-alive blocks are closed, never hidden).

- [ ] **Step 1: Write the failing tests**

Create `frontend/app/store/builder-terminal.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { BuilderNoticeAtom, isTermBlockDef, openBuilderTerminal, showConnectionUi, splitActionFor } from "./builder-terminal";
import { globalStore } from "./jotaiStore";
import { setWaveWindowType } from "./windowtype";

describe("splitActionFor", () => {
    it("maps the layout split directions to the server's target actions", () => {
        expect(splitActionFor("horizontal", "after")).toBe("splitright");
        expect(splitActionFor("horizontal", "before")).toBe("splitleft");
        expect(splitActionFor("vertical", "after")).toBe("splitdown");
        expect(splitActionFor("vertical", "before")).toBe("splitup");
    });
});

describe("isTermBlockDef", () => {
    it("matches only term views", () => {
        expect(isTermBlockDef({ meta: { view: "term", controller: "shell" } })).toBe(true);
        expect(isTermBlockDef({ meta: { view: "web" } })).toBe(false);
        expect(isTermBlockDef(null)).toBe(false);
    });
});

describe("openBuilderTerminal", () => {
    const open = vi.fn();

    beforeEach(() => {
        open.mockReset();
        (window as any).api = { openBuilderTerminal: open };
    });

    it("sends the target and clears the notice on success", async () => {
        globalStore.set(BuilderNoticeAtom, "old notice");
        open.mockResolvedValue("");
        expect(await openBuilderTerminal("splitdown", "b1")).toBe("");
        expect(open).toHaveBeenCalledWith({ targetblockid: "b1", targetaction: "splitdown" });
        expect(globalStore.get(BuilderNoticeAtom)).toBe("");
    });

    it("appends with an empty target and shows the error as the notice", async () => {
        open.mockResolvedValue("builder terminal not ready");
        expect(await openBuilderTerminal("", null)).toBe("builder terminal not ready");
        expect(open).toHaveBeenCalledWith({ targetblockid: "", targetaction: "" });
        expect(globalStore.get(BuilderNoticeAtom)).toBe("builder terminal not ready");
    });
});

describe("showConnectionUi", () => {
    it("is off in builder windows only", () => {
        setWaveWindowType("builder");
        expect(showConnectionUi()).toBe(false);
        setWaveWindowType("tab");
        expect(showConnectionUi()).toBe(true);
    });
});
```

Create `frontend/app/store/global-builder-create.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    layoutModel: null as any,
    createBlock: vi.fn(),
    deleteBlock: vi.fn(),
    openBuilderTerminal: vi.fn(),
}));

vi.mock("@/layout/index", () => ({
    getLayoutModelForStaticTab: vi.fn(() => h.layoutModel),
    LayoutTreeActionType: {
        InsertNode: "insert",
        SplitHorizontal: "splithorizontal",
        SplitVertical: "splitvertical",
        ReplaceNode: "replace",
    },
    newLayoutNode: (_dir: unknown, _size: unknown, _children: unknown, data: unknown) => ({ id: "node-new", data }),
}));
vi.mock("./services", () => ({
    ObjectService: { CreateBlock: h.createBlock, DeleteBlock: h.deleteBlock },
    ClientService: {},
}));
vi.mock("./wps", () => ({ waveEventSubscribeSingle: vi.fn(), peekFileSubject: vi.fn() }));
vi.mock("./badge", () => ({ setupBadgesSubscription: vi.fn() }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));

import { BuilderNoticeAtom } from "./builder-terminal";
import { createBlock, createBlockSplitHorizontally, createBlockSplitVertically, hideBlockKeepAlive, replaceBlock } from "./global";
import { globalStore } from "./jotaiStore";
import { setWaveWindowType } from "./windowtype";

const termDef: BlockDef = { meta: { view: "term", controller: "shell", "cmd:cwd": "/elsewhere", connection: "user@host" } };
const webDef: BlockDef = { meta: { view: "web", url: "https://example.com" } };

function makeLayoutModel() {
    return {
        treeReducer: vi.fn(),
        getNodeByBlockId: vi.fn(() => ({ id: "node-1" })),
        newEphemeralNode: vi.fn(),
    };
}

beforeEach(() => {
    vi.clearAllMocks();
    h.layoutModel = makeLayoutModel();
    h.createBlock.mockResolvedValue("new-block");
    h.openBuilderTerminal.mockResolvedValue("");
    (window as any).api = { openBuilderTerminal: h.openBuilderTerminal };
    globalStore.set(BuilderNoticeAtom, "");
});

describe("block creation in a builder window", () => {
    beforeEach(() => setWaveWindowType("builder"));

    it("sends term blocks to open-builder-terminal, ignoring the def's cwd and connection", async () => {
        expect(await createBlock(termDef)).toBeNull();
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "", targetaction: "" });
        expect(await createBlockSplitHorizontally(termDef, "b1", "after")).toBeNull();
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitright" });
        await createBlockSplitHorizontally(termDef, "b1", "before");
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitleft" });
        await createBlockSplitVertically(termDef, "b1", "after");
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitdown" });
        await createBlockSplitVertically(termDef, "b1", "before");
        expect(h.openBuilderTerminal).toHaveBeenLastCalledWith({ targetblockid: "b1", targetaction: "splitup" });
        expect(h.createBlock).not.toHaveBeenCalled();
        expect(h.layoutModel.treeReducer).not.toHaveBeenCalled();
    });

    it("shows an Open error as the builder notice", async () => {
        h.openBuilderTerminal.mockResolvedValueOnce("too many terminals in this builder (max 16)");
        await createBlock(termDef);
        expect(globalStore.get(BuilderNoticeAtom)).toBe("too many terminals in this builder (max 16)");
    });

    it("treats replaceBlock with a term def as a no-op", async () => {
        expect(await replaceBlock("b1", termDef, true)).toBeNull();
        expect(h.openBuilderTerminal).not.toHaveBeenCalled();
        expect(h.createBlock).not.toHaveBeenCalled();
        expect(h.deleteBlock).not.toHaveBeenCalled();
    });

    it("creates other views through the object service into the builder tab", async () => {
        expect(await createBlock(webDef)).toBe("new-block");
        expect(h.createBlock).toHaveBeenCalledWith(webDef, expect.anything());
        expect(h.layoutModel.treeReducer).toHaveBeenCalledTimes(1);
        expect(h.openBuilderTerminal).not.toHaveBeenCalled();
    });

    it("creates nothing before the terminal panel has a layout", async () => {
        h.layoutModel = null;
        await expect(createBlock(webDef)).rejects.toThrow();
        expect(h.createBlock).not.toHaveBeenCalled();
    });

    it("closes keep-alive blocks instead of hiding them", () => {
        expect(hideBlockKeepAlive("b1")).toBe(false);
    });
});

describe("block creation in a main window", () => {
    beforeEach(() => setWaveWindowType("tab"));

    it("keeps creating terminals through the object service", async () => {
        expect(await createBlock(termDef)).toBe("new-block");
        expect(h.createBlock).toHaveBeenCalledWith(termDef, expect.anything());
        expect(h.openBuilderTerminal).not.toHaveBeenCalled();
    });
});
```

Append to `frontend/builder/store/builder-apppanel-model.test.ts` (add `import { BuilderNoticeAtom } from "@/app/store/builder-terminal";` to its imports):

```ts
describe("BuilderAppPanelModel notice", () => {
    it("shares the builder notice atom, so Open terminal errors from any path show in the header", () => {
        expect(BuilderAppPanelModel.getInstance().noticeAtom).toBe(BuilderNoticeAtom);
    });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx vitest run frontend/app/store/builder-terminal.test.ts frontend/app/store/global-builder-create.test.ts frontend/builder/store/builder-apppanel-model.test.ts`
Expected: FAIL. `builder-terminal.test.ts`, `global-builder-create.test.ts` and `builder-apppanel-model.test.ts` fail to load because `builder-terminal.ts` does not exist yet.

- [ ] **Step 3: Implement the helper module**

Create `frontend/app/store/builder-terminal.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom, PrimitiveAtom } from "jotai";
import { globalStore } from "./jotaiStore";
import { isBuilderWindow } from "./windowtype";

export type BuilderTermAction = "" | "splitright" | "splitleft" | "splitup" | "splitdown";

// BuilderAppPanelModel.noticeAtom is this atom. It lives here so global.ts can report errors from the
// create intercept without importing the builder model, which imports global.ts.
export const BuilderNoticeAtom = atom("") as PrimitiveAtom<string>;

export function isTermBlockDef(blockDef: BlockDef): boolean {
    return blockDef?.meta?.view === "term";
}

export function splitActionFor(direction: "horizontal" | "vertical", position: "before" | "after"): BuilderTermAction {
    if (direction === "horizontal") {
        return position === "before" ? "splitleft" : "splitright";
    }
    return position === "before" ? "splitup" : "splitdown";
}

export async function openBuilderTerminal(action: BuilderTermAction, targetBlockId: string): Promise<string> {
    const api = (window as any).api as ElectronApi;
    const err = (await api.openBuilderTerminal({ targetblockid: targetBlockId ?? "", targetaction: action })) ?? "";
    globalStore.set(BuilderNoticeAtom, err);
    return err;
}

// Builder panes are local only (the server rejects remote connections), so the switcher is hidden there.
export function showConnectionUi(): boolean {
    return !isBuilderWindow();
}
```

- [ ] **Step 4: Implement the intercept**

In `frontend/app/store/global.ts`:

Add imports: `LayoutModel` to the `@/layout/index` import (lines 6-11); `import { isTermBlockDef, openBuilderTerminal, splitActionFor } from "./builder-terminal";`; and change `import { isPreviewWindow } from "./windowtype";` to `import { isBuilderWindow, isPreviewWindow } from "./windowtype";`.

Replace the create family (lines 377-460, `createBlockSplitHorizontally` through `replaceBlock`) with:

```ts
// The layout model is checked before any block is created, so a failure cannot leave an orphan block.
function requireStaticLayoutModel(): LayoutModel {
    const layoutModel = getLayoutModelForStaticTab();
    if (layoutModel == null) {
        throw new Error("this window has no layout yet");
    }
    return layoutModel;
}

// In builder windows a terminal is always a local shell in the app folder, created by the server under
// the builder's lock; the def's cwd, controller and connection are ignored. Callers ignore the return value.
async function createBlockSplitHorizontally(
    blockDef: BlockDef,
    targetBlockId: string,
    position: "before" | "after"
): Promise<string> {
    if (isBuilderWindow() && isTermBlockDef(blockDef)) {
        await openBuilderTerminal(splitActionFor("horizontal", position), targetBlockId);
        return null;
    }
    const layoutModel = requireStaticLayoutModel();
    const rtOpts: RuntimeOpts = { termsize: { rows: 25, cols: 80 } };
    const newBlockId = await ObjectService.CreateBlock(blockDef, rtOpts);
    const targetNodeId = layoutModel.getNodeByBlockId(targetBlockId)?.id;
    if (targetNodeId == null) {
        throw new Error(`targetNodeId not found for blockId: ${targetBlockId}`);
    }
    const splitAction: LayoutTreeSplitHorizontalAction = {
        type: LayoutTreeActionType.SplitHorizontal,
        targetNodeId: targetNodeId,
        newNode: newLayoutNode(undefined, undefined, undefined, { blockId: newBlockId }),
        position: position,
        focused: true,
    };
    layoutModel.treeReducer(splitAction);
    return newBlockId;
}

async function createBlockSplitVertically(
    blockDef: BlockDef,
    targetBlockId: string,
    position: "before" | "after"
): Promise<string> {
    if (isBuilderWindow() && isTermBlockDef(blockDef)) {
        await openBuilderTerminal(splitActionFor("vertical", position), targetBlockId);
        return null;
    }
    const layoutModel = requireStaticLayoutModel();
    const rtOpts: RuntimeOpts = { termsize: { rows: 25, cols: 80 } };
    const newBlockId = await ObjectService.CreateBlock(blockDef, rtOpts);
    const targetNodeId = layoutModel.getNodeByBlockId(targetBlockId)?.id;
    if (targetNodeId == null) {
        throw new Error(`targetNodeId not found for blockId: ${targetBlockId}`);
    }
    const splitAction: LayoutTreeSplitVerticalAction = {
        type: LayoutTreeActionType.SplitVertical,
        targetNodeId: targetNodeId,
        newNode: newLayoutNode(undefined, undefined, undefined, { blockId: newBlockId }),
        position: position,
        focused: true,
    };
    layoutModel.treeReducer(splitAction);
    return newBlockId;
}

async function createBlock(blockDef: BlockDef, magnified = false, ephemeral = false): Promise<string> {
    if (isBuilderWindow() && isTermBlockDef(blockDef)) {
        await openBuilderTerminal("", null);
        return null;
    }
    const layoutModel = requireStaticLayoutModel();
    const rtOpts: RuntimeOpts = { termsize: { rows: 25, cols: 80 } };
    const blockId = await ObjectService.CreateBlock(blockDef, rtOpts);
    if (ephemeral) {
        layoutModel.newEphemeralNode(blockId);
        return blockId;
    }
    const insertNodeAction: LayoutTreeInsertNodeAction = {
        type: LayoutTreeActionType.InsertNode,
        node: newLayoutNode(undefined, undefined, undefined, { blockId }),
        magnified,
        focused: true,
    };
    layoutModel.treeReducer(insertNodeAction);
    return blockId;
}

async function replaceBlock(blockId: string, blockDef: BlockDef, focus: boolean): Promise<string> {
    if (isBuilderWindow() && isTermBlockDef(blockDef)) {
        return null;
    }
    const layoutModel = requireStaticLayoutModel();
    const rtOpts: RuntimeOpts = { termsize: { rows: 25, cols: 80 } };
    const newBlockId = await ObjectService.CreateBlock(blockDef, rtOpts);
    setTimeout(() => {
        fireAndForget(() => ObjectService.DeleteBlock(blockId));
    }, 300);
    const targetNodeId = layoutModel.getNodeByBlockId(blockId)?.id;
    if (targetNodeId == null) {
        throw new Error(`targetNodeId not found for blockId: ${blockId}`);
    }
    const replaceNodeAction: LayoutTreeReplaceNodeAction = {
        type: LayoutTreeActionType.ReplaceNode,
        targetNodeId: targetNodeId,
        newNode: newLayoutNode(undefined, undefined, undefined, { blockId: newBlockId }),
        focused: focus,
    };
    layoutModel.treeReducer(replaceNodeAction);
    return newBlockId;
}
```

In `hideBlockKeepAlive` (line 627), insert as the first statement:

```ts
    // Nothing in a builder window can show a hidden block again, so keep-alive views close there.
    if (isBuilderWindow()) return false;
```

- [ ] **Step 5: Share the notice and route the header button**

In `frontend/builder/store/builder-apppanel-model.ts`, add `import { BuilderNoticeAtom, openBuilderTerminal } from "@/app/store/builder-terminal";`, change line 40 to:

```ts
    noticeAtom: PrimitiveAtom<string> = BuilderNoticeAtom;
```

and replace `openTerminal` (lines 190-193 after Task 11) with:

```ts
    async openTerminal() {
        await openBuilderTerminal("", null);
    }
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `npx vitest run frontend/app/store/ frontend/builder/ && npx tsc --noEmit`
Expected: all pass; tsc exit 0.

- [ ] **Step 7: Commit**

```bash
git add frontend/app/store/builder-terminal.ts frontend/app/store/builder-terminal.test.ts frontend/app/store/global.ts frontend/app/store/global-builder-create.test.ts frontend/builder/store/builder-apppanel-model.ts frontend/builder/store/builder-apppanel-model.test.ts
git commit -m "feat(builder): every new terminal in a builder window opens in its panel

In builder windows, any request to create a terminal (new pane, splits,
context menus, Open Terminal Here) asks the server for a local shell in
the app folder, and errors show in the app header's notice strip. Other
views still open in the builder's tab; keep-alive widgets close instead
of hiding.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 14: Builder terminal model, focus sides, layout defaults and tile contents (`frontend/builder`)

**Files:**
- Modify: `frontend/builder/store/builder-focusmanager.ts` (whole file, 30 lines)
- Create: `frontend/builder/store/builder-term-model.ts`, `frontend/builder/store/builder-term-model.test.ts`
- Create: `frontend/builder/store/builder-layout.ts`, `frontend/builder/store/builder-layout.test.ts`
- Create: `frontend/builder/builder-termcontents.tsx`, `frontend/builder/builder-termcontents.test.tsx`
- Modify: `frontend/builder/store/builder-apppanel-model.ts:298-314` (`switchBuilderApp`; `:308-324` before Task 11; Task 13 adds one import line and removes one line from `openTerminal`, so the number holds), imports
- Test: `frontend/builder/store/builder-apppanel-switch.test.ts` (new), `frontend/app/static-tab-writers.test.ts` (new)

**Interfaces:**
- Consumes: Task 10 `ElectronApi.ensureBuilderTab(): Promise<BuilderTabInfo>`, `ElectronApi.doRefresh()`; Task 11 `atoms.staticTabId: PrimitiveAtom<string>`, `deleteLayoutModelForTab(tabId)` (`frontend/layout/lib/layoutModelHooks.ts:50-52`); `WOS.loadAndPinWaveObject`, `WOS.getWaveObjectAtom`, `WOS.makeORef` (`frontend/app/store/wos.ts:202-223, 51`).
- Produces:
  - `BuilderFocusType = "app" | "terminal"`; `BuilderFocusManager.setTerminalFocused()` (plus existing `setAppFocused()`, `getFocusType()`, `focusType` atom)
  - `BuilderTermModel` singleton (`getInstance()`, `resetInstance()`): atoms `stateAtom: PrimitiveAtom<BuilderTermState>` (`"idle" | "loading" | "ready" | "error" | "mismatch" | "vanished" | "switching"`), `errorAtom`, `tabIdAtom`, `ensureOkAtom` (true exactly while `stateAtom` is `"ready"`; every state change goes through `setState`); methods `bootstrap()`, `retry()`, `markSwitching()`, `setState(state)`, `setError(message)`, `onTabValue(tabId, tab)`, `handlePaneCount(count)`, `hasPanes()`; const `BuilderTermMismatchMessage = "Terminal app and builder app differ; reopen the builder"`
  - `BuilderLayout = { terminal: number; app: number; build: number }`, `DefaultBuilderLayout = { terminal: 40, app: 80, build: 20 }`, `MinTerminalPercent = 20`, `mergeBuilderLayout(saved: Record<string, number>): BuilderLayout`
  - `makeBuilderTileContents(tabId: string, gapSizePx: number): TileLayoutContents` (`onNodeDelete` = `ObjectService.DeleteBlock(blockId)`, as `frontend/app/tab/tabcontent.tsx:38-40`)
  - `switchBuilderApp` marks the panel "switching" before `DeleteBuilderCommand`, awaits `setBuilderWindowAppId(null)` before reloading, and on any failure leaves "Switching app…": back to `"ready"` if the builder tab object still exists, otherwise `"vanished"` (Retry reloads)

Bootstrap order (spec D5): Ensure through IPC, then pin the tab and its LayoutState, then set `staticTabId`. The order matters because `getLayoutModelForTab` subscribes a new model to its LayoutState only when the tab is the static tab at creation time (`frontend/layout/lib/layoutModelHooks.ts:25-33`); the model never calls `getLayoutModelForStaticTab()` (the panel does, in Task 15, only once the state is `ready`).

- [ ] **Step 1: Write the failing tests**

Create `frontend/builder/store/builder-term-model.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    log: [] as string[],
    objs: new Map<string, any>(),
    objAtoms: new Map<string, any>(),
    api: { ensureBuilderTab: vi.fn(), doRefresh: vi.fn() },
    deleteLayoutModelForTab: vi.fn(),
}));

vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    const objAtom = (oref: string) => {
        let objectAtom = h.objAtoms.get(oref);
        if (objectAtom == null) {
            objectAtom = atom(null);
            h.objAtoms.set(oref, objectAtom);
        }
        return objectAtom;
    };
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app"), staticTabId: atom(null) },
        getApi: () => h.api,
        WOS: {
            makeORef: (otype: string, oid: string) => `${otype}:${oid}`,
            getWaveObjectAtom: objAtom,
            loadAndPinWaveObject: async (oref: string) => {
                h.log.push(`pin:${oref}`);
                const val = h.objs.get(oref) ?? null;
                globalStore.set(objAtom(oref), val);
                return val;
            },
            updateWaveObject: (update: WaveObjUpdate) => {
                globalStore.set(objAtom(`${update.otype}:${update.oid}`), update.updatetype === "delete" ? null : update.obj);
            },
        },
    };
});
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: h.deleteLayoutModelForTab,
    getLayoutModelForStaticTab: () => {
        h.log.push("layoutmodel");
        return null;
    },
}));

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { atoms, WOS } from "@/store/global";
import { BuilderTermMismatchMessage, BuilderTermModel } from "./builder-term-model";

function deleteTabUpdate(): WaveObjUpdate {
    return { updatetype: "delete", otype: "tab", oid: "tab-1" } as WaveObjUpdate;
}

describe("BuilderTermModel", () => {
    beforeEach(() => {
        BuilderTermModel.resetInstance();
        h.log.length = 0;
        h.objAtoms.clear();
        h.objs.clear();
        h.objs.set("tab:tab-1", { otype: "tab", oid: "tab-1", version: 1, layoutstate: "layout-1", blockids: ["b1"] });
        h.objs.set("layout:layout-1", { otype: "layout", oid: "layout-1", version: 1 });
        h.api.ensureBuilderTab.mockReset();
        h.api.doRefresh.mockReset();
        h.deleteLayoutModelForTab.mockReset();
        h.api.ensureBuilderTab.mockImplementation(async () => {
            h.log.push("ensure");
            return { tabid: "tab-1", appid: "draft/app" };
        });
        globalStore.set(atoms.staticTabId, null);
        globalStore.set(atoms.builderAppId, "draft/app");
        BuilderFocusManager.getInstance().setAppFocused();
    });

    it("pins the tab and its layout before setting the static tab, and builds no layout model", async () => {
        const unsub = globalStore.sub(atoms.staticTabId, () => h.log.push(`static:${globalStore.get(atoms.staticTabId)}`));
        const model = BuilderTermModel.getInstance();
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        await model.bootstrap();
        unsub();
        expect(h.log).toEqual(["ensure", "pin:tab:tab-1", "pin:layout:layout-1", "static:tab-1"]);
        expect(globalStore.get(model.stateAtom)).toBe("ready");
        expect(globalStore.get(model.tabIdAtom)).toBe("tab-1");
        expect(globalStore.get(model.ensureOkAtom)).toBe(true);
    });

    it("runs the bootstrap once when asked twice", async () => {
        const model = BuilderTermModel.getInstance();
        await Promise.all([model.bootstrap(), model.bootstrap()]);
        expect(h.api.ensureBuilderTab).toHaveBeenCalledTimes(1);
    });

    it("mounts nothing when Electron's app id differs from the renderer's", async () => {
        h.api.ensureBuilderTab.mockResolvedValue({ tabid: "tab-1", appid: "draft/other" });
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        expect(globalStore.get(model.stateAtom)).toBe("mismatch");
        expect(globalStore.get(atoms.staticTabId)).toBeNull();
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(h.log.some((entry) => entry.startsWith("pin:"))).toBe(false);
        expect(BuilderTermMismatchMessage).toBe("Terminal app and builder app differ; reopen the builder");
    });

    it("shows an Ensure error and retries in place", async () => {
        h.api.ensureBuilderTab.mockResolvedValueOnce({ error: "Could not start the terminals: boom" });
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        expect(globalStore.get(model.stateAtom)).toBe("error");
        expect(globalStore.get(model.errorAtom)).toBe("Could not start the terminals: boom");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        model.retry();
        await vi.waitFor(() => expect(globalStore.get(model.stateAtom)).toBe("ready"));
        expect(h.api.doRefresh).not.toHaveBeenCalled();
    });

    it("keeps Open terminal disabled when loading the tab fails after Ensure", async () => {
        h.objs.delete("tab:tab-1");
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        expect(globalStore.get(model.stateAtom)).toBe("error");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(globalStore.get(atoms.staticTabId)).toBeNull();
    });

    it("reloads the renderer if a different static tab is already set", async () => {
        globalStore.set(atoms.staticTabId, "tab-old");
        await BuilderTermModel.getInstance().bootstrap();
        expect(h.api.doRefresh).toHaveBeenCalledTimes(1);
        expect(globalStore.get(atoms.staticTabId)).toBe("tab-old");
    });

    it("drops the layout model and offers a reload when the tab is deleted", async () => {
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        WOS.updateWaveObject(deleteTabUpdate());
        expect(globalStore.get(model.stateAtom)).toBe("vanished");
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        expect(h.deleteLayoutModelForTab).toHaveBeenCalledWith("tab-1");
        model.retry();
        expect(h.api.doRefresh).toHaveBeenCalledTimes(1);
    });

    it("shows switching, not the reload state, when the app is being switched", async () => {
        const model = BuilderTermModel.getInstance();
        await model.bootstrap();
        model.markSwitching();
        expect(globalStore.get(model.ensureOkAtom)).toBe(false);
        WOS.updateWaveObject(deleteTabUpdate());
        expect(globalStore.get(model.stateAtom)).toBe("switching");
        expect(h.deleteLayoutModelForTab).toHaveBeenCalledWith("tab-1");
    });

    it("moves builder focus to the terminal at the first pane and to the app at zero panes", () => {
        const model = BuilderTermModel.getInstance();
        const focus = BuilderFocusManager.getInstance();
        model.handlePaneCount(0);
        expect(focus.getFocusType()).toBe("app");
        model.handlePaneCount(1);
        expect(focus.getFocusType()).toBe("terminal");
        expect(model.hasPanes()).toBe(true);
        focus.setAppFocused();
        model.handlePaneCount(2);
        expect(focus.getFocusType()).toBe("app");
        focus.setTerminalFocused();
        model.handlePaneCount(0);
        expect(focus.getFocusType()).toBe("app");
        expect(model.hasPanes()).toBe(false);
        model.handlePaneCount(1);
        expect(focus.getFocusType()).toBe("terminal");
    });
});
```

Create `frontend/builder/store/builder-layout.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { DefaultBuilderLayout, mergeBuilderLayout } from "./builder-layout";

describe("mergeBuilderLayout", () => {
    it("uses the defaults when nothing is saved", () => {
        expect(mergeBuilderLayout(null)).toEqual({ terminal: 40, app: 80, build: 20 });
        expect(DefaultBuilderLayout.terminal).toBe(40);
    });

    it("gives a layout saved before the terminal panel a 40% terminal", () => {
        expect(mergeBuilderLayout({ app: 70, build: 30 })).toEqual({ terminal: 40, app: 70, build: 30 });
    });

    it("keeps a saved terminal width", () => {
        expect(mergeBuilderLayout({ terminal: 55, app: 80, build: 20 }).terminal).toBe(55);
    });

    it("ignores values that are not usable percentages", () => {
        for (const bad of [Number.NaN, 0, 150, -5, "30" as unknown as number]) {
            expect(mergeBuilderLayout({ terminal: bad }).terminal).toBe(40);
        }
    });
});
```

Create `frontend/builder/builder-termcontents.test.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ deleteBlock: vi.fn(() => Promise.resolve()) }));

vi.mock("@/app/block/block", () => ({ Block: () => null }));
vi.mock("@/store/services", () => ({ ObjectService: { DeleteBlock: h.deleteBlock } }));

import { makeBuilderTileContents } from "./builder-termcontents";

describe("makeBuilderTileContents", () => {
    it("deletes the pane's block when its node is deleted", async () => {
        const contents = makeBuilderTileContents("tab-1", 3);
        expect(contents.tabId).toBe("tab-1");
        expect(contents.gapSizePx).toBe(3);
        await contents.onNodeDelete({ blockId: "b1" } as TabLayoutData);
        expect(h.deleteBlock).toHaveBeenCalledWith("b1");
    });
});
```

Create `frontend/builder/store/builder-apppanel-switch.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    order: [] as string[],
    rpc: { DeleteBuilderCommand: vi.fn(), SetRTInfoCommand: vi.fn() },
    api: { setBuilderWindowAppId: vi.fn(), doRefresh: vi.fn() },
    markSwitching: vi.fn(),
    setState: vi.fn(),
    tabAtom: null as any,
    getWaveObjectAtom: vi.fn(),
}));

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: h.rpc }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn(() => () => {}) }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app") },
        getApi: () => h.api,
        getSettingsKeyAtom: vi.fn(() => atom(false)),
        WOS: {
            makeORef: (otype: string, oid: string) => `${otype}:${oid}`,
            getWaveObjectAtom: h.getWaveObjectAtom,
        },
    };
});
vi.mock("@/builder/store/builder-term-model", async () => {
    const { atom } = await import("jotai");
    const tabIdAtom = atom("tab-1");
    h.tabAtom = atom(null);
    h.getWaveObjectAtom.mockImplementation(() => h.tabAtom);
    return {
        BuilderTermModel: {
            getInstance: () => ({ markSwitching: h.markSwitching, setState: h.setState, tabIdAtom }),
        },
    };
});

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderAppPanelModel } from "./builder-apppanel-model";

describe("switchBuilderApp", () => {
    afterEach(() => {
        vi.useRealTimers();
    });

    it("marks the panel as switching first and waits for Electron before reloading", async () => {
        vi.useFakeTimers();
        h.markSwitching.mockImplementation(() => h.order.push("switching"));
        h.rpc.DeleteBuilderCommand.mockImplementation(async () => h.order.push("delete"));
        h.rpc.SetRTInfoCommand.mockImplementation(async () => h.order.push("rtinfo"));
        let releaseAppId: () => void;
        h.api.setBuilderWindowAppId.mockImplementation(
            () =>
                new Promise<boolean>((resolve) => {
                    releaseAppId = () => {
                        h.order.push("appid");
                        resolve(true);
                    };
                })
        );
        h.api.doRefresh.mockImplementation(() => h.order.push("refresh"));

        const done = BuilderAppPanelModel.getInstance().switchBuilderApp();
        await vi.advanceTimersByTimeAsync(1000);
        expect(h.order).toEqual(["switching", "delete", "rtinfo"]);
        releaseAppId();
        await vi.advanceTimersByTimeAsync(1000);
        await done;
        expect(h.order).toEqual(["switching", "delete", "rtinfo", "appid", "refresh"]);
        expect(h.setState).not.toHaveBeenCalled();
    });

    it("returns to the terminals when the teardown fails and the tab still exists", async () => {
        vi.spyOn(console, "error").mockImplementation(() => {});
        h.rpc.DeleteBuilderCommand.mockRejectedValueOnce(new Error("server gone"));
        h.api.doRefresh.mockClear();
        h.setState.mockClear();
        globalStore.set(h.tabAtom, { otype: "tab", oid: "tab-1", blockids: ["b1"] });
        const model = BuilderAppPanelModel.getInstance();
        await model.switchBuilderApp();
        expect(h.getWaveObjectAtom).toHaveBeenCalledWith("tab:tab-1");
        expect(h.setState).toHaveBeenCalledWith("ready");
        expect(globalStore.get(model.errorAtom)).toContain("server gone");
        expect(h.api.doRefresh).not.toHaveBeenCalled();
    });

    it("offers Retry when the teardown fails and the tab is gone", async () => {
        vi.spyOn(console, "error").mockImplementation(() => {});
        h.rpc.DeleteBuilderCommand.mockRejectedValueOnce(new Error("server gone"));
        h.setState.mockClear();
        globalStore.set(h.tabAtom, null);
        await BuilderAppPanelModel.getInstance().switchBuilderApp();
        expect(h.setState).toHaveBeenCalledWith("vanished");
    });
});
```

Create `frontend/app/static-tab-writers.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import fs from "fs";
import path from "path";
import { describe, expect, it } from "vitest";

const FrontendRoot = path.join(import.meta.dirname, "..");
// The preview harness (frontend/preview) fakes tab switching on its own mock atoms; it is not a window.
const ExcludedDirs = new Set([path.join(FrontendRoot, "preview")]);
const WritePatterns = [
    /\.set\(\s*[\w.]*atoms\.staticTabId\b/,
    /useSetAtom\(\s*[\w.]*atoms\.staticTabId\b/,
    /useAtom\(\s*[\w.]*atoms\.staticTabId\b/,
];

function sourceFiles(dir: string): string[] {
    const rtn: string[] = [];
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
            if (!ExcludedDirs.has(full)) {
                rtn.push(...sourceFiles(full));
            }
            continue;
        }
        if (/\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)) {
            rtn.push(full);
        }
    }
    return rtn;
}

describe("staticTabIdAtom writers", () => {
    it("are limited to the builder terminal bootstrap, so main windows never change their tab", () => {
        const writers = sourceFiles(FrontendRoot)
            .filter((file) => WritePatterns.some((pattern) => pattern.test(fs.readFileSync(file, "utf8"))))
            .map((file) => path.relative(FrontendRoot, file));
        expect(writers).toEqual([path.join("builder", "store", "builder-term-model.ts")]);
    });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx vitest run frontend/builder/ frontend/app/static-tab-writers.test.ts`
Expected: FAIL. The model, layout and tile-contents suites fail to load (their modules do not exist yet); the switch test fails its order assertion (`markSwitching` is never called); the writers test finds `[]`.

- [ ] **Step 3: Implement the focus sides**

Replace the body of `frontend/builder/store/builder-focusmanager.ts` after the imports (lines 7-30) with:

```ts
export type BuilderFocusType = "app" | "terminal";

export class BuilderFocusManager {
    private static instance: BuilderFocusManager | null = null;

    focusType: PrimitiveAtom<BuilderFocusType> = atom<BuilderFocusType>("app");

    private constructor() {}

    static getInstance(): BuilderFocusManager {
        if (!BuilderFocusManager.instance) {
            BuilderFocusManager.instance = new BuilderFocusManager();
        }
        return BuilderFocusManager.instance;
    }

    setAppFocused() {
        globalStore.set(this.focusType, "app");
    }

    setTerminalFocused() {
        globalStore.set(this.focusType, "terminal");
    }

    getFocusType(): BuilderFocusType {
        return globalStore.get(this.focusType);
    }
}
```

- [ ] **Step 4: Implement the layout defaults and the tile contents**

Create `frontend/builder/store/builder-layout.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type BuilderLayout = {
    terminal: number;
    app: number;
    build: number;
};

export const DefaultBuilderLayout: BuilderLayout = { terminal: 40, app: 80, build: 20 };
export const MinTerminalPercent = 20;

// builder:layout lives in rtinfo, which older builds wrote without "terminal" and which any pane can
// rewrite, so each value is checked before it sizes a panel.
export function mergeBuilderLayout(saved: Record<string, number>): BuilderLayout {
    const rtn: BuilderLayout = { ...DefaultBuilderLayout };
    if (saved == null) {
        return rtn;
    }
    for (const key of Object.keys(DefaultBuilderLayout) as (keyof BuilderLayout)[]) {
        const val = saved[key];
        if (typeof val === "number" && Number.isFinite(val) && val > 0 && val < 100) {
            rtn[key] = val;
        }
    }
    return rtn;
}
```

Create `frontend/builder/builder-termcontents.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Block } from "@/app/block/block";
import type { ContentRenderer, NodeModel, PreviewRenderer } from "@/layout/index";
import type { TileLayoutContents } from "@/layout/lib/types";
import * as services from "@/store/services";

// As TabContent (app/tab/tabcontent.tsx): closing a pane deletes its block. The server keeps a builder
// tab when its last block goes, so the panel can show its empty state.
export function makeBuilderTileContents(tabId: string, gapSizePx: number): TileLayoutContents {
    const renderContent: ContentRenderer = (nodeModel: NodeModel) => {
        return <Block key={nodeModel.blockId} nodeModel={nodeModel} preview={false} />;
    };
    const renderPreview: PreviewRenderer = (nodeModel: NodeModel) => {
        return <Block key={nodeModel.blockId} nodeModel={nodeModel} preview={true} />;
    };
    function onNodeDelete(data: TabLayoutData) {
        return services.ObjectService.DeleteBlock(data.blockId);
    }
    return { renderContent, renderPreview, tabId, onNodeDelete, gapSizePx };
}
```

- [ ] **Step 5: Implement the model**

Create `frontend/builder/store/builder-term-model.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { deleteLayoutModelForTab } from "@/layout/index";
import { atoms, getApi, WOS } from "@/store/global";
import { fireAndForget } from "@/util/util";
import { atom, type PrimitiveAtom } from "jotai";

export type BuilderTermState = "idle" | "loading" | "ready" | "error" | "mismatch" | "vanished" | "switching";

export const BuilderTermMismatchMessage = "Terminal app and builder app differ; reopen the builder";

export class BuilderTermModel {
    private static instance: BuilderTermModel | null = null;

    stateAtom: PrimitiveAtom<BuilderTermState> = atom<BuilderTermState>("idle");
    errorAtom: PrimitiveAtom<string> = atom<string>("");
    tabIdAtom = atom<string>(null) as PrimitiveAtom<string>;
    ensureOkAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    bootstrapping = false;
    paneCount = 0;
    tabUnsubFn: (() => void) | null = null;

    private constructor() {}

    static getInstance(): BuilderTermModel {
        if (!BuilderTermModel.instance) {
            BuilderTermModel.instance = new BuilderTermModel();
        }
        return BuilderTermModel.instance;
    }

    static resetInstance(): void {
        BuilderTermModel.instance?.tabUnsubFn?.();
        BuilderTermModel.instance = null;
    }

    async bootstrap(): Promise<void> {
        const state = globalStore.get(this.stateAtom);
        if (this.bootstrapping || (state !== "idle" && state !== "error")) {
            return;
        }
        this.bootstrapping = true;
        try {
            await this.runBootstrap();
        } finally {
            this.bootstrapping = false;
        }
    }

    // The tab and its LayoutState are pinned before staticTabId is set: the first layout model for the
    // static tab subscribes to that LayoutState when it is created (layoutModelHooks.ts), so nothing may
    // build one before both are loaded.
    async runBootstrap(): Promise<void> {
        this.setState("loading");
        const result = await getApi().ensureBuilderTab();
        if (result?.error || !result?.tabid) {
            this.setError(result?.error || "Could not start the terminals.");
            return;
        }
        if (result.appid !== globalStore.get(atoms.builderAppId)) {
            this.setState("mismatch");
            return;
        }
        try {
            const tab = await WOS.loadAndPinWaveObject<Tab>(WOS.makeORef("tab", result.tabid));
            if (tab == null) {
                throw new Error("the terminal tab was not found");
            }
            await WOS.loadAndPinWaveObject<LayoutState>(WOS.makeORef("layout", tab.layoutstate));
        } catch (e) {
            this.setError(`Could not load the terminals: ${e?.message ?? String(e)}`);
            return;
        }
        const staticTabId = globalStore.get(atoms.staticTabId);
        if (staticTabId != null && staticTabId !== result.tabid) {
            // Only an app switch changes the tab, and it reloads the renderer; reload rather than mix two tabs.
            getApi().doRefresh();
            return;
        }
        if (staticTabId == null) {
            globalStore.set(atoms.staticTabId, result.tabid);
        }
        globalStore.set(this.tabIdAtom, result.tabid);
        this.watchTab(result.tabid);
        this.setState("ready");
    }

    // The header's Open terminal button follows ensureOkAtom, so it is true exactly while the panel is
    // ready: an Open in any other state would add a pane nobody can see.
    setState(state: BuilderTermState) {
        globalStore.set(this.stateAtom, state);
        globalStore.set(this.ensureOkAtom, state === "ready");
    }

    setError(message: string) {
        globalStore.set(this.errorAtom, message);
        this.setState("error");
    }

    // Before the layout exists a failed bootstrap can simply run again; once a tab was mounted, only a
    // fresh renderer gets a clean layout model.
    retry() {
        if (globalStore.get(this.stateAtom) === "error") {
            fireAndForget(() => this.bootstrap());
            return;
        }
        getApi().doRefresh();
    }

    markSwitching() {
        this.setState("switching");
    }

    watchTab(tabId: string) {
        this.tabUnsubFn?.();
        const tabAtom = WOS.getWaveObjectAtom<Tab>(WOS.makeORef("tab", tabId));
        this.tabUnsubFn = globalStore.sub(tabAtom, () => this.onTabValue(tabId, globalStore.get(tabAtom)));
    }

    onTabValue(tabId: string, tab: Tab) {
        if (tab != null) {
            return;
        }
        const state = globalStore.get(this.stateAtom);
        if (state !== "ready" && state !== "switching") {
            return;
        }
        deleteLayoutModelForTab(tabId);
        this.setState(state === "ready" ? "vanished" : "switching");
    }

    handlePaneCount(count: number) {
        const focusManager = BuilderFocusManager.getInstance();
        if (count === 0) {
            focusManager.setAppFocused();
        } else if (this.paneCount === 0) {
            focusManager.setTerminalFocused();
        }
        this.paneCount = count;
    }

    hasPanes(): boolean {
        return this.paneCount > 0;
    }
}
```

- [ ] **Step 6: Switch-app ordering**

In `frontend/builder/store/builder-apppanel-model.ts`, add `import { BuilderTermModel } from "@/builder/store/builder-term-model";` and replace `switchBuilderApp` (lines 298-314 after Tasks 11 and 13) with:

```ts
    async switchBuilderApp() {
        const builderId = globalStore.get(atoms.builderId);
        // Set first: DeleteBuilderCommand removes the tab, and the panel must not offer Retry meanwhile.
        BuilderTermModel.getInstance().markSwitching();
        try {
            await RpcApi.DeleteBuilderCommand(TabRpcClient, builderId);
            await new Promise((resolve) => setTimeout(resolve, 500));
            await RpcApi.SetRTInfoCommand(TabRpcClient, {
                oref: WOS.makeORef("builder", builderId),
                data: { "builder:appid": null },
            });
            await getApi().setBuilderWindowAppId(null);
            await new Promise((resolve) => setTimeout(resolve, 100));
            getApi().doRefresh();
        } catch (err) {
            console.error("Failed to switch builder app:", err);
            globalStore.set(this.errorAtom, `Failed to switch builder app: ${err.message || "Unknown error"}`);
            // Leave "Switching app…": back to the terminals if the tab survived, otherwise to Retry (a reload).
            const termModel = BuilderTermModel.getInstance();
            const tabId = globalStore.get(termModel.tabIdAtom);
            const tab = tabId == null ? null : globalStore.get(WOS.getWaveObjectAtom<Tab>(WOS.makeORef("tab", tabId)));
            termModel.setState(tab != null ? "ready" : "vanished");
        }
    }
```

The existing `builder-apppanel-model.test.ts` now loads `builder-term-model.ts`; add `vi.mock("@/builder/store/builder-term-model", () => ({ BuilderTermModel: { getInstance: () => ({ markSwitching: vi.fn() }) } }));` next to its other `vi.mock` calls so it keeps its narrow mock set.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `npx vitest run frontend/builder/ frontend/app/ && npx tsc --noEmit`
Expected: all pass; tsc exit 0.

- [ ] **Step 8: Commit**

```bash
git add frontend/builder/store/builder-focusmanager.ts frontend/builder/store/builder-term-model.ts frontend/builder/store/builder-term-model.test.ts frontend/builder/store/builder-layout.ts frontend/builder/store/builder-layout.test.ts frontend/builder/builder-termcontents.tsx frontend/builder/builder-termcontents.test.tsx frontend/builder/store/builder-apppanel-model.ts frontend/builder/store/builder-apppanel-model.test.ts frontend/builder/store/builder-apppanel-switch.test.ts frontend/app/static-tab-writers.test.ts
git commit -m "feat(builder): terminal panel model with safe bootstrap and switching state

The builder's terminal panel will load its tab before any layout exists,
report an Ensure failure with Retry, refuse to mount when Electron and
the page disagree on the app, notice when its tab is deleted, and show a
neutral state while the app is being switched.

Miscellanea: BuilderTermModel; builder focus gains a terminal side;
builder:layout defaults with a 40% terminal; tile contents wired to
ObjectService.DeleteBlock; source scan pinning the only staticTabId
writer.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 15: Terminal panel UI, workspace split, header button and preview partition (`frontend/builder`)

**Files:**
- Create: `frontend/builder/builder-termpanel.tsx`, `frontend/builder/builder-termpanel.test.tsx`
- Modify: `frontend/builder/builder-workspace.tsx` (whole file, 112 lines); Test: `frontend/builder/builder-workspace.test.tsx` (new)
- Modify: `frontend/builder/builder-appheader.tsx:4-9` (imports), `:132-159` (`BuilderAppHeader`; the button is `:138-144`); Test: `frontend/builder/builder-appheader.test.tsx` (new)
- Modify: `frontend/builder/store/builder-apppanel-model.ts` (add `getBuilderPreviewPartition`), `frontend/builder/store/builder-apppanel-model.test.ts` (append)
- Modify: `frontend/builder/tabs/builder-previewtab.tsx:4` (import), `:215-223` (`<webview>`)

**Interfaces:**
- Consumes: Task 13 `openBuilderTerminal`; Task 14 `BuilderTermModel` (`stateAtom`, `errorAtom`, `tabIdAtom`, `ensureOkAtom`, `bootstrap()`, `retry()`, `setState()`, `markSwitching()`, `handlePaneCount()`, `hasPanes()`), `BuilderTermMismatchMessage`, `BuilderFocusManager.setTerminalFocused()`, `mergeBuilderLayout`, `MinTerminalPercent`, `BuilderLayout`, `makeBuilderTileContents`; `TileLayout`, `getLayoutModelForStaticTab` (`@/layout/index`); `TabModelContext`, `getTabModelByTabId` (`frontend/app/store/tab-model.ts:54-80`).
- Produces:
  - `BuilderTermPanel` (named export): runs the bootstrap on mount; renders `Starting terminals…`, the error with `Retry`, `BuilderTermMismatchMessage`, `Switching app…`, `The terminal tab is gone.` with `Retry`, or the tile layout inside `TabModelContext` with a `No terminals` / `Open terminal` overlay when `layoutModel.numLeafs` is 0; focus-capture or mouse-down inside sets builder focus to `"terminal"` when there are panes; accent border when the terminal side is focused
  - `BuilderWorkspace`: horizontal `PanelGroup` (terminal `layout.terminal`, default 40, min 20 | app column), root `data-builder-focus="app"|"terminal"`, layout saved to rtinfo `builder:layout` with `terminal`; focus-capture or mouse-down anywhere in the app column (app panel or build panel) sets builder focus to `"app"`
  - Header "Open terminal" disabled until `ensureOkAtom` is true
  - `getBuilderPreviewPartition(builderId: string): string` = `builder-preview-<builderId>`; the Preview `<webview>` uses it

- [ ] **Step 1: Write the failing tests**

Create `frontend/builder/builder-termpanel.test.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { atom, Provider } from "jotai";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    layoutModel: null as any,
    open: vi.fn(() => Promise.resolve("")),
}));

vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return {
        atoms: { builderAppId: atom("draft/app"), staticTabId: atom(null), settingsAtom: atom({}) },
        getApi: () => ({ getCursorPoint: vi.fn() }),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}`, getWaveObjectAtom: () => atom(null) },
    };
});
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: vi.fn(),
    getLayoutModelForStaticTab: () => h.layoutModel,
    TileLayout: () => null,
}));
vi.mock("@/app/store/tab-model", async () => {
    const { createContext } = await import("react");
    return { getTabModelByTabId: vi.fn(), TabModelContext: createContext(undefined) };
});
vi.mock("@/builder/builder-termcontents", () => ({ makeBuilderTileContents: vi.fn(() => ({})) }));
vi.mock("@/app/store/builder-terminal", () => ({ openBuilderTerminal: h.open }));

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { BuilderTermMismatchMessage, BuilderTermModel, type BuilderTermState } from "@/builder/store/builder-term-model";
import { BuilderTermPanel } from "./builder-termpanel";

function renderPanel(state: BuilderTermState, extra?: () => void) {
    const model = BuilderTermModel.getInstance();
    vi.spyOn(model, "bootstrap").mockResolvedValue(undefined);
    globalStore.set(model.stateAtom, state);
    extra?.();
    render(
        <Provider store={globalStore}>
            <BuilderTermPanel />
        </Provider>
    );
    return model;
}

describe("BuilderTermPanel", () => {
    beforeEach(() => {
        BuilderTermModel.resetInstance();
        h.layoutModel = null;
        h.open.mockClear();
        BuilderFocusManager.getInstance().setAppFocused();
    });

    afterEach(() => {
        cleanup();
        vi.restoreAllMocks();
    });

    it("shows a neutral state while the app is being switched", () => {
        renderPanel("switching");
        expect(screen.getByText("Switching app…")).toBeTruthy();
        expect(screen.queryByText("Retry")).toBeNull();
    });

    it("shows the mismatch message without mounting or retrying", () => {
        renderPanel("mismatch");
        expect(screen.getByText(BuilderTermMismatchMessage)).toBeTruthy();
        expect(screen.queryByText("Retry")).toBeNull();
    });

    it("shows an Ensure error with a Retry button", () => {
        const model = renderPanel("error", () => globalStore.set(BuilderTermModel.getInstance().errorAtom, "Could not start the terminals: boom"));
        const retry = vi.spyOn(model, "retry").mockImplementation(() => {});
        expect(screen.getByText("Could not start the terminals: boom")).toBeTruthy();
        fireEvent.click(screen.getByText("Retry"));
        expect(retry).toHaveBeenCalledTimes(1);
    });

    it("shows the empty state over the layout when the tab has no panes, and opens a terminal from it", () => {
        h.layoutModel = { numLeafs: atom(0) };
        renderPanel("ready", () => globalStore.set(BuilderTermModel.getInstance().tabIdAtom, "tab-1"));
        expect(screen.getByText("No terminals")).toBeTruthy();
        fireEvent.click(screen.getByText("Open terminal"));
        expect(h.open).toHaveBeenCalledWith("", null);
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("app");
    });

    it("hides the empty state and takes terminal focus once a pane exists", async () => {
        const leafs = atom(0);
        h.layoutModel = { numLeafs: leafs };
        renderPanel("ready", () => globalStore.set(BuilderTermModel.getInstance().tabIdAtom, "tab-1"));
        await act(async () => {
            globalStore.set(leafs, 1);
        });
        expect(screen.queryByText("No terminals")).toBeNull();
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("terminal");
    });
});
```

Append to `frontend/builder/store/builder-apppanel-model.test.ts` (and import `getBuilderPreviewPartition` from `./builder-apppanel-model`):

```ts
describe("getBuilderPreviewPartition", () => {
    it("gives each builder its own in-memory partition", () => {
        expect(getBuilderPreviewPartition("b1")).toBe("builder-preview-b1");
        expect(getBuilderPreviewPartition("b1").startsWith("persist:")).toBe(false);
        expect(getBuilderPreviewPartition("b2")).not.toBe(getBuilderPreviewPartition("b1"));
    });
});
```

Create `frontend/builder/builder-appheader.test.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, render, screen } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn(() => () => {}) }));
vi.mock("@/layout/index", () => ({ deleteLayoutModelForTab: vi.fn() }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    // One atom for every key: a component given a new atom on each render re-renders forever.
    const settingAtom = atom(false);
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app"), staticTabId: atom(null) },
        getApi: vi.fn(),
        getSettingsKeyAtom: vi.fn(() => settingAtom),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}` },
    };
});

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderTermModel } from "@/builder/store/builder-term-model";
import { BuilderAppHeader } from "./builder-appheader";

describe("BuilderAppHeader Open terminal button", () => {
    afterEach(() => {
        cleanup();
    });

    it("is disabled until the terminal panel is ready, and again when it leaves ready", async () => {
        const model = BuilderTermModel.getInstance();
        model.setState("loading");
        render(
            <Provider store={globalStore}>
                <BuilderAppHeader />
            </Provider>
        );
        const button = screen.getByText("Open terminal").closest("button");
        expect(button.disabled).toBe(true);
        await act(async () => {
            model.setState("ready");
        });
        expect(button.disabled).toBe(false);
        await act(async () => {
            model.markSwitching();
        });
        expect(button.disabled).toBe(true);
    });
});
```

Create `frontend/builder/builder-workspace.test.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/store/wshclientapi", () => ({
    RpcApi: { GetRTInfoCommand: vi.fn(async () => ({})), SetRTInfoCommand: vi.fn(async () => {}) },
}));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return { atoms: { builderId: atom("builder-1") } };
});
vi.mock("@/builder/builder-apppanel", async () => {
    const { createElement } = await import("react");
    return { BuilderAppPanel: () => createElement("div", null, "app panel") };
});
vi.mock("@/builder/builder-buildpanel", async () => {
    const { createElement } = await import("react");
    return { BuilderBuildPanel: () => createElement("div", null, "build output") };
});
vi.mock("@/builder/builder-termpanel", async () => {
    const { createElement } = await import("react");
    return { BuilderTermPanel: () => createElement("div", null, "terminals") };
});

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { BuilderWorkspace } from "./builder-workspace";

describe("BuilderWorkspace", () => {
    afterEach(() => {
        cleanup();
    });

    it("moves builder focus to the app side when the build panel is clicked", async () => {
        BuilderFocusManager.getInstance().setTerminalFocused();
        render(
            <Provider store={globalStore}>
                <BuilderWorkspace />
            </Provider>
        );
        fireEvent.mouseDown(await screen.findByText("build output"));
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("app");
        expect(document.querySelector("[data-builder-focus]").getAttribute("data-builder-focus")).toBe("app");
    });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx vitest run frontend/builder/builder-termpanel.test.tsx frontend/builder/store/builder-apppanel-model.test.ts frontend/builder/builder-appheader.test.tsx frontend/builder/builder-workspace.test.tsx`
Expected: FAIL: `builder-termpanel.test.tsx` fails to load (`./builder-termpanel` does not exist yet); the partition test fails (`getBuilderPreviewPartition` does not exist); the header test fails (throws, or the `disabled` assertion fails); the workspace test fails (throws, or the focus assertion fails).

- [ ] **Step 3: Implement the panel**

Create `frontend/builder/builder-termpanel.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { openBuilderTerminal } from "@/app/store/builder-terminal";
import { getTabModelByTabId, TabModelContext } from "@/app/store/tab-model";
import { makeBuilderTileContents } from "@/builder/builder-termcontents";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { BuilderTermMismatchMessage, BuilderTermModel } from "@/builder/store/builder-term-model";
import { getLayoutModelForStaticTab, TileLayout } from "@/layout/index";
import { atoms, getApi, WOS } from "@/store/global";
import { cn, fireAndForget } from "@/util/util";
import { atom, useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useMemo } from "react";

const ZeroAtom = atom(0);
const TileGapSizeAtom = atom((get) => get(atoms.settingsAtom)?.["window:tilegapsize"]);

type PanelMessageProps = {
    message: string;
    actionLabel?: string;
    onAction?: () => void;
};

const PanelMessage = memo(({ message, actionLabel, onAction }: PanelMessageProps) => {
    return (
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-4 text-center text-sm text-secondary">
            <div>{message}</div>
            {actionLabel && (
                <button
                    className="px-3 py-1 bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                    onClick={onAction}
                >
                    {actionLabel}
                </button>
            )}
        </div>
    );
});

PanelMessage.displayName = "PanelMessage";

// Rendered only once the model is ready, i.e. after the tab and its LayoutState are pinned and staticTabId
// is set, so the layout model created here subscribes to the right LayoutState.
const BuilderTermLayout = memo(({ tabId }: { tabId: string }) => {
    const tabAtom = useMemo(() => WOS.getWaveObjectAtom<Tab>(WOS.makeORef("tab", tabId)), [tabId]);
    const tileGapSize = useAtomValue(TileGapSizeAtom);
    const contents = useMemo(() => makeBuilderTileContents(tabId, tileGapSize), [tabId, tileGapSize]);
    const layoutModel = getLayoutModelForStaticTab();
    const numLeafs = useAtomValue(layoutModel?.numLeafs ?? ZeroAtom);

    useEffect(() => {
        BuilderTermModel.getInstance().handlePaneCount(numLeafs);
    }, [numLeafs]);

    return (
        <TabModelContext.Provider value={getTabModelByTabId(tabId)}>
            <div className="relative h-full w-full">
                <TileLayout key={tabId} contents={contents} tabAtom={tabAtom} getCursorPoint={getApi().getCursorPoint} />
                {numLeafs === 0 && (
                    <div className="absolute inset-0 bg-main-bg">
                        <PanelMessage
                            message="No terminals"
                            actionLabel="Open terminal"
                            onAction={() => fireAndForget(() => openBuilderTerminal("", null))}
                        />
                    </div>
                )}
            </div>
        </TabModelContext.Provider>
    );
});

BuilderTermLayout.displayName = "BuilderTermLayout";

export const BuilderTermPanel = memo(() => {
    const model = BuilderTermModel.getInstance();
    const state = useAtomValue(model.stateAtom);
    const errorMsg = useAtomValue(model.errorAtom);
    const tabId = useAtomValue(model.tabIdAtom);
    const focusType = useAtomValue(BuilderFocusManager.getInstance().focusType);

    useEffect(() => {
        fireAndForget(() => model.bootstrap());
    }, []);

    // With no panes the terminal side has nothing to act on, so a click there leaves Cmd:w closing the window.
    const handleFocusCapture = useCallback(() => {
        if (model.hasPanes()) {
            BuilderFocusManager.getInstance().setTerminalFocused();
        }
    }, [model]);

    let content: React.ReactNode;
    if (state === "ready" && tabId != null) {
        content = <BuilderTermLayout tabId={tabId} />;
    } else if (state === "error") {
        content = <PanelMessage message={errorMsg} actionLabel="Retry" onAction={() => model.retry()} />;
    } else if (state === "vanished") {
        content = <PanelMessage message="The terminal tab is gone." actionLabel="Retry" onAction={() => model.retry()} />;
    } else if (state === "mismatch") {
        content = <PanelMessage message={BuilderTermMismatchMessage} />;
    } else if (state === "switching") {
        content = <PanelMessage message="Switching app…" />;
    } else {
        content = <PanelMessage message="Starting terminals…" />;
    }

    return (
        <div
            data-builder-term-panel
            className={cn(
                "h-full w-full overflow-hidden border-2",
                focusType === "terminal" ? "border-accent" : "border-transparent"
            )}
            onFocusCapture={handleFocusCapture}
            onMouseDownCapture={handleFocusCapture}
        >
            {content}
        </div>
    );
});

BuilderTermPanel.displayName = "BuilderTermPanel";
```

- [ ] **Step 4: Implement the workspace split**

Replace `frontend/builder/builder-workspace.tsx` with:

```tsx
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { BuilderAppPanel } from "@/builder/builder-apppanel";
import { BuilderBuildPanel } from "@/builder/builder-buildpanel";
import { BuilderTermPanel } from "@/builder/builder-termpanel";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { type BuilderLayout, mergeBuilderLayout, MinTerminalPercent } from "@/builder/store/builder-layout";
import { atoms } from "@/store/global";
import { cn } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import { debounce } from "throttle-debounce";

const BuilderWorkspace = memo(() => {
    const builderId = useAtomValue(atoms.builderId);
    const [initialLayout, setInitialLayout] = useState<BuilderLayout>(null);
    const layoutRef = useRef<BuilderLayout>(null);
    const focusType = useAtomValue(BuilderFocusManager.getInstance().focusType);
    const isAppFocused = focusType === "app";

    useEffect(() => {
        const loadLayout = async () => {
            let saved: Record<string, number> = null;
            if (builderId) {
                try {
                    const rtInfo = await RpcApi.GetRTInfoCommand(TabRpcClient, {
                        oref: `builder:${builderId}`,
                    });
                    saved = rtInfo?.["builder:layout"] as Record<string, number>;
                } catch (error) {
                    console.error("Failed to load builder layout:", error);
                }
            }
            const layout = mergeBuilderLayout(saved);
            layoutRef.current = layout;
            setInitialLayout(layout);
        };

        loadLayout();
    }, [builderId]);

    const saveLayout = useCallback(
        debounce(500, (newLayout: BuilderLayout) => {
            if (!builderId) return;

            RpcApi.SetRTInfoCommand(TabRpcClient, {
                oref: `builder:${builderId}`,
                data: {
                    "builder:layout": newLayout,
                },
            }).catch((error) => {
                console.error("Failed to save builder layout:", error);
            });
        }),
        [builderId]
    );

    // Both panel groups report their sizes on mount; merging into a ref stops one report overwriting the
    // other with a stale copy of the layout.
    const updateLayout = useCallback(
        (patch: Partial<BuilderLayout>) => {
            layoutRef.current = { ...layoutRef.current, ...patch };
            saveLayout(layoutRef.current);
        },
        [saveLayout]
    );

    const handleHorizontalLayout = useCallback((sizes: number[]) => updateLayout({ terminal: sizes[0] }), [updateLayout]);

    const handleVerticalLayout = useCallback(
        (sizes: number[]) => updateLayout({ app: sizes[0], build: sizes[1] }),
        [updateLayout]
    );

    // The app panel sets app focus itself; this also covers the build panel below it.
    const handleAppColumnFocus = useCallback(() => {
        BuilderFocusManager.getInstance().setAppFocused();
    }, []);

    if (initialLayout == null) {
        return null;
    }

    return (
        <div className="flex-1 overflow-hidden" data-builder-focus={focusType}>
            <PanelGroup direction="horizontal" onLayout={handleHorizontalLayout}>
                <Panel defaultSize={initialLayout.terminal} minSize={MinTerminalPercent}>
                    <BuilderTermPanel />
                </Panel>
                <PanelResizeHandle className="w-0.5 bg-transparent hover:bg-gray-500/20 transition-colors" />
                <Panel defaultSize={100 - initialLayout.terminal} minSize={20}>
                    <div
                        className={cn(
                            "flex flex-col relative h-full",
                            isAppFocused ? "border-2 border-accent" : "border-2 border-transparent"
                        )}
                        style={{
                            borderBottomRightRadius: 8,
                        }}
                        onFocusCapture={handleAppColumnFocus}
                        onMouseDownCapture={handleAppColumnFocus}
                    >
                        <PanelGroup direction="vertical" onLayout={handleVerticalLayout}>
                            <Panel defaultSize={initialLayout.app} minSize={20}>
                                <BuilderAppPanel />
                            </Panel>
                            <PanelResizeHandle className="h-0.5 bg-transparent hover:bg-gray-500/20 transition-colors" />
                            <Panel
                                defaultSize={initialLayout.build}
                                minSize={20}
                                maxSize={50}
                                style={{ borderBottomRightRadius: 8 }}
                            >
                                <BuilderBuildPanel />
                            </Panel>
                        </PanelGroup>
                    </div>
                </Panel>
            </PanelGroup>
        </div>
    );
});

BuilderWorkspace.displayName = "BuilderWorkspace";

export { BuilderWorkspace };
```

- [ ] **Step 5: Header button, preview partition**

In `frontend/builder/store/builder-apppanel-model.ts`, add before `export class BuilderAppPanelModel`:

```ts
// In-memory (no "persist:" prefix) and per builder: the preview must not share cookies or storage with
// web blocks in the builder tab (persist:webblock) or with other builders' previews.
export function getBuilderPreviewPartition(builderId: string): string {
    return `builder-preview-${builderId}`;
}
```

In `frontend/builder/tabs/builder-previewtab.tsx`, change the import on line 4 to `import { BuilderAppPanelModel, getBuilderPreviewPartition } from "@/builder/store/builder-apppanel-model";` and add a `partition` attribute to the `<webview>` (lines 215-223):

```tsx
                <webview
                    ref={model.webviewRef}
                    src={lastKnownUrl}
                    partition={getBuilderPreviewPartition(builderId)}
                    className="w-full h-full"
                    style={{
                        visibility: isWebViewActive ? "visible" : "hidden",
                        pointerEvents: isWebViewActive ? "auto" : "none",
                    }}
                />
```

In `frontend/builder/builder-appheader.tsx`, add the imports `import { BuilderTermModel } from "@/builder/store/builder-term-model";` and `import { cn } from "@/util/util";`, then replace the "Open terminal" button inside `BuilderAppHeader` (lines 138-144) with:

```tsx
                <button
                    className={cn(
                        "shrink-0 flex items-center gap-1.5 px-2 py-0.5 text-xs rounded transition-colors",
                        terminalReady ? "hover:bg-secondary/10 cursor-pointer" : "opacity-50"
                    )}
                    disabled={!terminalReady}
                    onClick={() => model.openTerminal()}
                >
                    <i className="fa fa-terminal" />
                    Open terminal
                </button>
```

and add `const terminalReady = useAtomValue(BuilderTermModel.getInstance().ensureOkAtom);` as the second line of `BuilderAppHeader`, after `const model = BuilderAppPanelModel.getInstance();`.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `npx vitest run frontend/builder/ && npx tsc --noEmit`
Expected: all builder suites pass; tsc exit 0.

- [ ] **Step 7: Commit**

```bash
git add frontend/builder/builder-termpanel.tsx frontend/builder/builder-termpanel.test.tsx frontend/builder/builder-workspace.tsx frontend/builder/builder-workspace.test.tsx frontend/builder/builder-appheader.tsx frontend/builder/builder-appheader.test.tsx frontend/builder/store/builder-apppanel-model.ts frontend/builder/store/builder-apppanel-model.test.ts frontend/builder/tabs/builder-previewtab.tsx
git commit -m "feat(builder): terminal panel on the left of the builder window

The builder window opens with a resizable terminal area beside the app,
starting one shell in the app folder; panes split, close and focus as in
a main window, an empty panel offers Open terminal, and the header button
waits until the terminals are ready. The app preview now uses its own
in-memory browser storage per builder.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 16: Builder keys, close rule, block dispatch and connection UI (`frontend/app/store`, `frontend/builder`)

**Files:**
- Create: `frontend/builder/store/builder-keys.ts`, `frontend/builder/store/builder-keys.test.ts`
- Modify: `frontend/app/store/keymodel.ts` (imports `:4-31`; `uxCloseBlock` `:146-159`; `genericClose` `:161-177`; `appHandleKeyDown` dispatch condition `:371`; `registerBuilderGlobalKeys` `:699-706`)
- Test: `frontend/app/store/keymodel-builder-keys.test.ts` (new; a separate file so the tab-window keys registered by Task 11's suite are not in its key map; `happy-dom`, because the block hand-off reads `document.activeElement`)
- Modify: `frontend/app/block/blockframe-header.tsx:272` (connection button)

**Interfaces:**
- Consumes: Task 11 module-level `magnifyFocusedNode`, `activateSearch`, `deactivateSearch`, null-safe `getFocusedBlockInStaticTab`, `switchBlockInDirection`, `switchBlockByBlockNum`, `genericClose`; Task 13 `openBuilderTerminal`, `BuilderTermAction`, `showConnectionUi`; Task 14 `BuilderFocusManager` (`getFocusType()`, `"app" | "terminal"`).
- Produces:
  - `makeBuilderKeyTables(deps: BuilderKeyDeps): BuilderKeyTables` with `BuilderKeyDeps = { getFocusType; closeBuilderWindow; closeFocusedPane; openTerminal(action, targetBlockId); getFocusedBlockId; isFocusMoveDisabled; switchBlockInDirection(direction); switchBlockByBlockNum(blockNum); magnifyFocused; activateSearch(waveEvent); handleEscape }` and `BuilderKeyTables = { keyMap: Map<string, BuilderKeyHandler>; chordMap: Map<string, Map<string, BuilderKeyHandler>>; webviewKeys: string[] }`; `BuilderWebviewKeys = ["Cmd:w"]`
  - key table (spec D6), exact strings: `Cmd:w` (terminal: close focused pane; app: close window; auto-repeat ignored), `Cmd:n`, `Cmd:d`, `Shift:Cmd:d`, chord `Ctrl:Shift:s` + `ArrowUp`/`ArrowDown`/`ArrowLeft`/`ArrowRight`, `Ctrl:Shift:ArrowUp/Down/Left/Right`, `Ctrl:Shift:k/j/h/l`, `Cmd:m`, `Ctrl:Shift:c{Digit1..9}`, `Ctrl:Shift:c{Numpad1..9}`, `Cmd:f`, `Escape`; all but `Cmd:w` act only with terminal focus and return `false` otherwise
  - in builder windows `uxCloseBlock`/`genericClose` always close through the layout model (`closeNode`/`closeFocusedNode`), including the last pane, and never call `closeTab`; key events reach the focused block's `keyDownHandler` when the terminal side is focused; the block header hides the connection button

- [ ] **Step 1: Write the failing tests**

Create `frontend/builder/store/builder-keys.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { BuilderWebviewKeys, makeBuilderKeyTables } from "./builder-keys";

type Focus = "app" | "terminal";

function setup(focus: Focus, focusedBlockId: string = "b1") {
    const state = { focus, moveDisabled: false };
    const deps = {
        getFocusType: () => state.focus,
        closeBuilderWindow: vi.fn(),
        closeFocusedPane: vi.fn(),
        openTerminal: vi.fn(),
        getFocusedBlockId: () => focusedBlockId,
        isFocusMoveDisabled: () => state.moveDisabled,
        switchBlockInDirection: vi.fn(),
        switchBlockByBlockNum: vi.fn(),
        magnifyFocused: vi.fn(),
        activateSearch: vi.fn(() => true),
        handleEscape: vi.fn(() => true),
    };
    return { state, deps, tables: makeBuilderKeyTables(deps) };
}

function press(handler: (e: WaveKeyboardEvent) => boolean, repeat = false): boolean {
    return handler({ type: "keydown", repeat } as WaveKeyboardEvent);
}

describe("builder key tables", () => {
    it("closes the focused pane with Cmd:w on the terminal side and the window on the app side", () => {
        const { state, deps, tables } = setup("terminal");
        expect(press(tables.keyMap.get("Cmd:w"))).toBe(true);
        expect(deps.closeFocusedPane).toHaveBeenCalledTimes(1);
        expect(deps.closeBuilderWindow).not.toHaveBeenCalled();
        state.focus = "app";
        expect(press(tables.keyMap.get("Cmd:w"))).toBe(true);
        expect(deps.closeBuilderWindow).toHaveBeenCalledTimes(1);
    });

    it("ignores auto-repeated Cmd:w in both foci", () => {
        const { state, deps, tables } = setup("terminal");
        expect(press(tables.keyMap.get("Cmd:w"), true)).toBe(true);
        state.focus = "app";
        expect(press(tables.keyMap.get("Cmd:w"), true)).toBe(true);
        expect(deps.closeFocusedPane).not.toHaveBeenCalled();
        expect(deps.closeBuilderWindow).not.toHaveBeenCalled();
    });

    it("opens panes and splits through open-builder-terminal on the focused block", () => {
        const { deps, tables } = setup("terminal");
        press(tables.keyMap.get("Cmd:n"));
        expect(deps.openTerminal).toHaveBeenLastCalledWith("", null);
        press(tables.keyMap.get("Cmd:d"));
        expect(deps.openTerminal).toHaveBeenLastCalledWith("splitright", "b1");
        press(tables.keyMap.get("Shift:Cmd:d"));
        expect(deps.openTerminal).toHaveBeenLastCalledWith("splitdown", "b1");
        const chord = tables.chordMap.get("Ctrl:Shift:s");
        const expected: [string, string][] = [
            ["ArrowUp", "splitup"],
            ["ArrowDown", "splitdown"],
            ["ArrowLeft", "splitleft"],
            ["ArrowRight", "splitright"],
        ];
        for (const [key, action] of expected) {
            expect(press(chord.get(key))).toBe(true);
            expect(deps.openTerminal).toHaveBeenLastCalledWith(action, "b1");
        }
    });

    it("does not split without a focused pane", () => {
        const { deps, tables } = setup("terminal", null);
        expect(press(tables.keyMap.get("Cmd:d"))).toBe(true);
        expect(deps.openTerminal).not.toHaveBeenCalled();
    });

    it("leaves every key except Cmd:w to the app side when it is focused", () => {
        const { deps, tables } = setup("app");
        for (const [key, handler] of tables.keyMap) {
            if (key === "Cmd:w") {
                continue;
            }
            expect(press(handler), key).toBe(false);
        }
        for (const handler of tables.chordMap.get("Ctrl:Shift:s").values()) {
            expect(press(handler)).toBe(false);
        }
        expect(deps.openTerminal).not.toHaveBeenCalled();
        expect(deps.switchBlockInDirection).not.toHaveBeenCalled();
        expect(deps.magnifyFocused).not.toHaveBeenCalled();
        expect(deps.activateSearch).not.toHaveBeenCalled();
        expect(deps.handleEscape).not.toHaveBeenCalled();
    });

    it("runs focus moves, magnify, block numbers, search and Escape as a tab does", () => {
        const { state, deps, tables } = setup("terminal");
        press(tables.keyMap.get("Ctrl:Shift:ArrowLeft"));
        press(tables.keyMap.get("Ctrl:Shift:l"));
        expect(deps.switchBlockInDirection.mock.calls).toEqual([[3], [1]]);
        press(tables.keyMap.get("Cmd:m"));
        expect(deps.magnifyFocused).toHaveBeenCalledTimes(1);
        press(tables.keyMap.get("Ctrl:Shift:c{Digit3}"));
        press(tables.keyMap.get("Ctrl:Shift:c{Numpad9}"));
        expect(deps.switchBlockByBlockNum.mock.calls).toEqual([[3], [9]]);
        expect(press(tables.keyMap.get("Cmd:f"))).toBe(true);
        expect(press(tables.keyMap.get("Escape"))).toBe(true);
        state.moveDisabled = true;
        expect(press(tables.keyMap.get("Ctrl:Shift:ArrowUp"))).toBe(false);
    });

    it("leaves the tab-window keys unbound", () => {
        const { tables } = setup("terminal");
        const unbound = ["Cmd:t", "Cmd:Shift:w", "Cmd:[", "Shift:Cmd:[", "Cmd:]", "Shift:Cmd:]", "F2", "Ctrl:Shift:i", "Ctrl:Shift:x", "Cmd:g", "Cmd:i", "Ctrl:w"];
        for (let idx = 1; idx <= 9; idx++) {
            unbound.push(`Cmd:${idx}`);
        }
        for (const key of unbound) {
            expect(tables.keyMap.has(key), key).toBe(false);
        }
    });

    it("forwards only Cmd:w from the preview webview", () => {
        expect(setup("app").tables.webviewKeys).toEqual(["Cmd:w"]);
        expect(BuilderWebviewKeys).toEqual(["Cmd:w"]);
    });
});
```

Create `frontend/app/store/keymodel-builder-keys.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import * as keyutil from "@/util/keyutil";
import { atom } from "jotai";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    api: {
        closeBuilderWindow: vi.fn(),
        closeTab: vi.fn(() => Promise.resolve(true)),
        registerGlobalWebviewKeys: vi.fn(),
        setKeyboardChordMode: vi.fn(),
    },
    openTerminal: vi.fn(() => Promise.resolve("")),
    layoutModel: null as any,
    blockCount: 1,
    bcms: new Map<string, any>(),
    webviewKeys: null as string[],
}));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    return {
        atoms: {
            staticTabId: atom("tab-1"),
            modalOpen: atom(false),
            workspaceId: atom(null),
            controlShiftDelayAtom: atom(false),
            newTabDropdownOpen: atom(false),
        },
        createBlock: vi.fn(),
        createBlockSplitHorizontally: vi.fn(),
        createBlockSplitVertically: vi.fn(),
        getAllBlockComponentModels: vi.fn(() => []),
        getApi: () => h.api,
        getBlockComponentModel: (blockId: string) => h.bcms.get(blockId),
        getBlockMetaKeyAtom: vi.fn(() => atom(null)),
        getFocusedBlockId: vi.fn(),
        getSettingsKeyAtom: vi.fn(() => atom(false)),
        globalStore,
        refocusNode: vi.fn(),
        replaceBlock: vi.fn(),
        WOS: {
            makeORef: (otype: string, oid: string) => `${otype}:${oid}`,
            getWaveObjectAtom: () => atom({ blockids: Array.from({ length: h.blockCount }, (_, i) => `b${i + 1}`) }),
        },
    };
});
vi.mock("@/app/store/focusManager", () => ({ FocusManager: { getInstance: vi.fn() } }));
vi.mock("@/app/store/services", () => ({ UserInputService: {} }));
vi.mock("@/app/store/tab-model", () => ({ getActiveTabModel: vi.fn(() => null) }));
vi.mock("@/app/workspace/workspace-layout-model", () => ({ WorkspaceLayoutModel: {} }));
vi.mock("@/app/store/builder-terminal", () => ({ openBuilderTerminal: h.openTerminal }));
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: vi.fn(),
    getLayoutModelForStaticTab: vi.fn(() => h.layoutModel),
    NavigateDirection: { Up: 0, Right: 1, Down: 2, Left: 3 },
}));
vi.mock("./windowtype", () => ({ isBuilderWindow: () => true, isTabWindow: () => false }));

import { appHandleKeyDown, registerBuilderGlobalKeys, uxCloseBlock } from "./keymodel";

function linuxKey(desc: string): WaveKeyboardEvent {
    const ev: any = { type: "keydown", key: "", code: "", cmd: false, alt: false, option: false, meta: false, control: false, shift: false };
    for (const part of desc.split(":")) {
        if (part === "Cmd") {
            ev.cmd = true;
            ev.alt = true;
        } else if (part === "Shift") {
            ev.shift = true;
        } else if (part === "Ctrl") {
            ev.control = true;
        } else {
            ev.key = part;
        }
    }
    return ev as WaveKeyboardEvent;
}

function makeLayoutModel() {
    const focused = { id: "node-b1", data: { blockId: "b1" } };
    return {
        focusedNode: atom(focused),
        ephemeralNode: atom(undefined),
        getNodeByBlockId: vi.fn((blockId: string) => (blockId === "b1" ? focused : null)),
        closeNode: vi.fn(() => Promise.resolve()),
        closeFocusedNode: vi.fn(() => Promise.resolve()),
        switchNodeFocusInDirection: vi.fn(),
        switchNodeFocusByBlockNum: vi.fn(),
        magnifyNodeToggle: vi.fn(),
        focusFirstNode: vi.fn(),
        focusNode: vi.fn(),
    };
}

describe("builder keys in keymodel", () => {
    beforeAll(() => {
        keyutil.setKeyUtilPlatform("linux");
        registerBuilderGlobalKeys();
        h.webviewKeys = h.api.registerGlobalWebviewKeys.mock.calls[0][0];
    });

    beforeEach(() => {
        vi.clearAllMocks();
        h.layoutModel = makeLayoutModel();
        h.blockCount = 1;
        h.bcms.clear();
        BuilderFocusManager.getInstance().setTerminalFocused();
    });

    it("registers only Cmd:w with the preview webview", () => {
        expect(h.webviewKeys).toEqual(["Cmd:w"]);
    });

    it("closes the last pane through the layout model, never the tab, with Cmd:w on the terminal side", () => {
        expect(appHandleKeyDown(linuxKey("Cmd:w"))).toBe(true);
        expect(h.layoutModel.closeFocusedNode).toHaveBeenCalledTimes(1);
        expect(h.api.closeTab).not.toHaveBeenCalled();
        expect(h.api.closeBuilderWindow).not.toHaveBeenCalled();
    });

    it("closes the last pane from its header through closeNode, never closeTab", () => {
        uxCloseBlock("b1");
        expect(h.layoutModel.closeNode).toHaveBeenCalledWith("node-b1");
        expect(h.api.closeTab).not.toHaveBeenCalled();
    });

    it("closes the builder window with Cmd:w on the app side", () => {
        BuilderFocusManager.getInstance().setAppFocused();
        expect(appHandleKeyDown(linuxKey("Cmd:w"))).toBe(true);
        expect(h.api.closeBuilderWindow).toHaveBeenCalledTimes(1);
        expect(h.layoutModel.closeFocusedNode).not.toHaveBeenCalled();
    });

    it("splits the focused pane through open-builder-terminal", () => {
        expect(appHandleKeyDown(linuxKey("Cmd:d"))).toBe(true);
        expect(h.openTerminal).toHaveBeenCalledWith("splitright", "b1");
    });

    it("moves focus between panes as a tab does", () => {
        expect(appHandleKeyDown(linuxKey("Ctrl:Shift:ArrowLeft"))).toBe(true);
        expect(h.layoutModel.switchNodeFocusInDirection).toHaveBeenCalledWith(3);
    });

    it("hands other keys to the focused pane on the terminal side only, and leaves Ctrl:w unbound", () => {
        const keyDownHandler = vi.fn(() => false);
        h.bcms.set("b1", { viewModel: { keyDownHandler } });
        expect(appHandleKeyDown(linuxKey("Ctrl:w"))).toBe(false);
        expect(keyDownHandler).toHaveBeenCalledTimes(1);
        BuilderFocusManager.getInstance().setAppFocused();
        expect(appHandleKeyDown(linuxKey("Ctrl:w"))).toBe(false);
        expect(keyDownHandler).toHaveBeenCalledTimes(1);
    });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx vitest run frontend/builder/store/builder-keys.test.ts frontend/app/store/keymodel-builder-keys.test.ts`
Expected: FAIL. `builder-keys.test.ts` fails to load (`./builder-keys` does not exist yet). In the keymodel suite, the webview-keys and app-side `Cmd:w` cases pass already (that is today's builder behaviour), while the terminal-side `Cmd:w` (`closeFocusedNode` not called), `uxCloseBlock` on the last pane, `Cmd:d`, the focus move and the block hand-off fail.

- [ ] **Step 3: Implement the key tables**

Create `frontend/builder/store/builder-keys.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { BuilderTermAction } from "@/app/store/builder-terminal";
import type { BuilderFocusType } from "@/builder/store/builder-focusmanager";
import { NavigateDirection } from "@/layout/lib/types";

export type BuilderKeyHandler = (waveEvent: WaveKeyboardEvent) => boolean;

export type BuilderKeyDeps = {
    getFocusType: () => BuilderFocusType;
    closeBuilderWindow: () => void;
    closeFocusedPane: () => void;
    openTerminal: (action: BuilderTermAction, targetBlockId: string) => void;
    getFocusedBlockId: () => string;
    isFocusMoveDisabled: () => boolean;
    switchBlockInDirection: (direction: NavigateDirection) => void;
    switchBlockByBlockNum: (blockNum: number) => void;
    magnifyFocused: () => void;
    activateSearch: (waveEvent: WaveKeyboardEvent) => boolean;
    handleEscape: () => boolean;
};

export type BuilderKeyTables = {
    keyMap: Map<string, BuilderKeyHandler>;
    chordMap: Map<string, Map<string, BuilderKeyHandler>>;
    webviewKeys: string[];
};

// The preview webview forwards only these to the builder, so every other key stays with the app being built.
export const BuilderWebviewKeys = ["Cmd:w"];

const FocusMoveKeys: [string, NavigateDirection][] = [
    ["Ctrl:Shift:ArrowUp", NavigateDirection.Up],
    ["Ctrl:Shift:ArrowDown", NavigateDirection.Down],
    ["Ctrl:Shift:ArrowLeft", NavigateDirection.Left],
    ["Ctrl:Shift:ArrowRight", NavigateDirection.Right],
    ["Ctrl:Shift:k", NavigateDirection.Up],
    ["Ctrl:Shift:j", NavigateDirection.Down],
    ["Ctrl:Shift:h", NavigateDirection.Left],
    ["Ctrl:Shift:l", NavigateDirection.Right],
];

const SplitChordKeys: [string, BuilderTermAction][] = [
    ["ArrowUp", "splitup"],
    ["ArrowDown", "splitdown"],
    ["ArrowLeft", "splitleft"],
    ["ArrowRight", "splitright"],
];

export function makeBuilderKeyTables(deps: BuilderKeyDeps): BuilderKeyTables {
    const onTerminal = (handler: BuilderKeyHandler): BuilderKeyHandler => {
        return (waveEvent) => {
            if (deps.getFocusType() !== "terminal") {
                return false;
            }
            return handler(waveEvent);
        };
    };
    const split = (action: BuilderTermAction): BuilderKeyHandler =>
        onTerminal(() => {
            const blockId = deps.getFocusedBlockId();
            if (blockId == null) {
                return true;
            }
            deps.openTerminal(action, blockId);
            return true;
        });

    const keyMap = new Map<string, BuilderKeyHandler>();
    // A held key would close every pane, move focus to the app side and then close the window.
    keyMap.set("Cmd:w", (waveEvent) => {
        if (waveEvent.repeat) {
            return true;
        }
        if (deps.getFocusType() === "terminal") {
            deps.closeFocusedPane();
            return true;
        }
        deps.closeBuilderWindow();
        return true;
    });
    keyMap.set(
        "Cmd:n",
        onTerminal(() => {
            deps.openTerminal("", null);
            return true;
        })
    );
    keyMap.set("Cmd:d", split("splitright"));
    keyMap.set("Shift:Cmd:d", split("splitdown"));
    keyMap.set(
        "Cmd:m",
        onTerminal(() => {
            deps.magnifyFocused();
            return true;
        })
    );
    for (const [key, direction] of FocusMoveKeys) {
        keyMap.set(
            key,
            onTerminal(() => {
                if (deps.isFocusMoveDisabled()) {
                    return false;
                }
                deps.switchBlockInDirection(direction);
                return true;
            })
        );
    }
    for (let blockNum = 1; blockNum <= 9; blockNum++) {
        const handler = onTerminal(() => {
            deps.switchBlockByBlockNum(blockNum);
            return true;
        });
        keyMap.set(`Ctrl:Shift:c{Digit${blockNum}}`, handler);
        keyMap.set(`Ctrl:Shift:c{Numpad${blockNum}}`, handler);
    }
    keyMap.set(
        "Cmd:f",
        onTerminal((waveEvent) => deps.activateSearch(waveEvent))
    );
    keyMap.set(
        "Escape",
        onTerminal(() => deps.handleEscape())
    );

    const splitChord = new Map<string, BuilderKeyHandler>();
    for (const [key, action] of SplitChordKeys) {
        splitChord.set(key, split(action));
    }
    return {
        keyMap,
        chordMap: new Map([["Ctrl:Shift:s", splitChord]]),
        webviewKeys: [...BuilderWebviewKeys],
    };
}
```

- [ ] **Step 4: Wire keymodel**

In `frontend/app/store/keymodel.ts`, add imports:

```ts
import { openBuilderTerminal } from "@/app/store/builder-terminal";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { makeBuilderKeyTables } from "@/builder/store/builder-keys";
```

At the start of `uxCloseBlock` (line 146), before the existing comment, insert:

```ts
    // In a builder window the last pane closes like any other: the server keeps the builder tab and the
    // panel shows its empty state. Closing the tab would close nothing the user can see.
    if (isBuilderWindow()) {
        const layoutModel = getLayoutModelForStaticTab();
        const node = layoutModel?.getNodeByBlockId(blockId);
        if (node) {
            fireAndForget(() => layoutModel.closeNode(node.id));
        }
        return;
    }
```

At the start of `genericClose` (line 161) insert:

```ts
    if (isBuilderWindow()) {
        const layoutModel = getLayoutModelForStaticTab();
        if (layoutModel == null) {
            return;
        }
        fireAndForget(layoutModel.closeFocusedNode.bind(layoutModel));
        return;
    }
```

Add before `appHandleKeyDown`:

```ts
function isBuilderTerminalFocused(): boolean {
    return isBuilderWindow() && BuilderFocusManager.getInstance().getFocusType() === "terminal";
}
```

and in `appHandleKeyDown` change `if (isTabWindow()) {` (line 371) to:

```ts
    if (isTabWindow() || isBuilderTerminalFocused()) {
```

Replace `registerBuilderGlobalKeys` (lines 699-706) with:

```ts
function registerBuilderGlobalKeys() {
    const tables = makeBuilderKeyTables({
        getFocusType: () => BuilderFocusManager.getInstance().getFocusType(),
        closeBuilderWindow: () => getApi().closeBuilderWindow(),
        closeFocusedPane: () => genericClose(),
        openTerminal: (action, targetBlockId) => fireAndForget(() => openBuilderTerminal(action, targetBlockId)),
        getFocusedBlockId: () => getFocusedBlockInStaticTab(),
        isFocusMoveDisabled: () => !!globalStore.get(getSettingsKeyAtom("app:disablectrlshiftarrows")),
        switchBlockInDirection: (direction) => switchBlockInDirection(direction),
        switchBlockByBlockNum: (blockNum) => switchBlockByBlockNum(blockNum),
        magnifyFocused: () => magnifyFocusedNode(),
        activateSearch: (waveEvent) => activateSearch(waveEvent),
        handleEscape: () => modalsModel.popModal() || deactivateSearch(),
    });
    for (const [key, handler] of tables.keyMap) {
        globalKeyMap.set(key, handler);
    }
    for (const [key, chordKeys] of tables.chordMap) {
        globalChordMap.set(key, chordKeys);
    }
    getApi().registerGlobalWebviewKeys(tables.webviewKeys);
}
```

- [ ] **Step 5: Hide the connection button in builder windows**

In `frontend/app/block/blockframe-header.tsx`, add `import { showConnectionUi } from "@/app/store/builder-terminal";` and change line 272 from `{manageConnection && (` to:

```tsx
                {manageConnection && showConnectionUi() && (
```

(`Cmd:g`, the other route to the connection switcher, is not in the builder key table.)

- [ ] **Step 6: Run the tests to verify they pass**

Run: `npx vitest run frontend/builder/ frontend/app/store/ && npx tsc --noEmit`
Expected: all pass; tsc exit 0.

- [ ] **Step 7: Commit**

```bash
git add frontend/builder/store/builder-keys.ts frontend/builder/store/builder-keys.test.ts frontend/app/store/keymodel.ts frontend/app/store/keymodel-builder-keys.test.ts frontend/app/block/blockframe-header.tsx
git commit -m "feat(builder): terminal keys follow the focused side of the builder

With the terminal side focused, Alt+W (Cmd+W on macOS) closes the
focused pane, including the last; new pane, splits, focus moves,
magnify, block numbers and search work as in a main window. With the
app side focused, Alt+W closes the builder window and the other keys
stay with the app. Holding Alt+W no longer runs on into closing the
window, Ctrl+W stays readline's word delete, and builder panes show no
connection switcher.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 17: Full verification sweep

**Files:**
- No source changes. If a check fails, stop and report it with the output; do not fix inside this task.

**Interfaces:**
- Consumes: Tasks 1-16 committed on `feat/builder-terminal`.
- Produces: a short report (each command, exit status, pass/fail counts) for the reviewer.

- [ ] **Step 1: Go**

```bash
export PATH=$PWD/golang-1.26.2/bin:$PATH
go vet ./pkg/rtstore/ ./pkg/rtcore/ ./pkg/blockcontroller/ ./pkg/buildercontroller/ ./pkg/wshutil/ ./pkg/wshrpc/... ./cmd/server/
go test ./pkg/... -count=1
go test ./cmd/server/... -count=1
go test -race ./pkg/rtstore/... ./pkg/rtcore/... ./pkg/blockcontroller/... ./pkg/buildercontroller/... ./pkg/wshutil/... ./pkg/wshrpc/... -count=1
(cd tsunami && go test ./app/... ./build/... ./engine/... ./rpctypes/... ./util/... ./vdom/... -count=1)
```

Expected: vet prints nothing; every `go test` line is `ok` or `[no test files]`.

- [ ] **Step 2: Frontend and Electron**

```bash
npx tsc --noEmit
npx vitest run
```

Expected: tsc exit 0; vitest reports no failed tests.

- [ ] **Step 3: Generated files are current**

```bash
./node_modules/.bin/task generate
git status --short frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts pkg/wshrpc/wshclient/wshclient.go
```

Expected: no output from `git status` (regenerating changes nothing, so nobody hand-edited them).

- [ ] **Step 4: House rules on the diff**

```bash
git diff main...HEAD --name-only
git diff main...HEAD | grep -n '^+.*—' || true
git diff main...HEAD | grep -nE '^\+.*\b(func|function) New[A-Z]' || true
git diff main...HEAD | grep -nE '^\+.*(cursor-help|cursor-not-allowed)' || true
```

Expected: the file list matches the files named in Tasks 1-16 plus `.planning/builder-terminal/*`; the em-dash grep shows only the line moved verbatim in Task 11 (`// Already open — increment the focusInput counter ...`); the other two greps print nothing.

- [ ] **Step 5: Format new frontend files**

```bash
git diff main...HEAD --name-only --diff-filter=A -- '*.ts' '*.tsx' | xargs npx prettier --check
```

Expected: `All matched files use Prettier code style!`. If not, run the same with `--write`, rerun Step 2, and commit the formatting alone:

```bash
git diff main...HEAD --name-only --diff-filter=A -- '*.ts' '*.tsx' | xargs npx prettier --write
git add -u
git commit -m "style(builder): format new builder terminal files

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

(Existing files are not reformatted wholesale: several, such as `keymodel.ts` and `global.ts`, were not Prettier-clean before this branch.)

---

### Task 18: Isolated end-to-end run (nested Xvfb, scratch profile)

**Files:**
- Create (not committed): `.planning/builder-terminal/E2E-REPORT.md`; all other artefacts stay under the scratch root `$SCR`
- No source changes. A failing check is reported with its evidence, not fixed here.

**Interfaces:**
- Consumes: the whole branch after Task 17 passed.
- Produces: `E2E-REPORT.md` with a PASS/FAIL line and evidence for each check below, the safety preflight, the isolation proof and the cleanup record.

The recipe is the one that held for the builder-by-hand run, with its addendum applied: `llm-wiki/wiki/concepts/remoteterm-fork-engineering-gotchas.md` ("Isolated launch recipe: every part is required", lines 182-205, including the 2026-10-03 addendum that the build steps need scratch `GOCACHE`/`GOMODCACHE`) and `llm-wiki/raw/2026-10-03-remoteterm-builder-by-hand-e2e-report.md` ("Safety and preflight"). Read both before starting.

**Hard rules (stop rather than bend them):**
- Never use `DISPLAY=:0` or the user's running X/Wayland session; never run `xdotool`, `xwininfo`, `wmctrl` or anything else against a display other than the nested Xvfb, and then only with `DISPLAY` set explicitly for that one command. Every launch goes through `env -i`, so `DISPLAY` and `WAYLAND_DISPLAY` are absent everywhere except under `xvfb-run`, which sets `DISPLAY` for its own children only.
- Never read from or write to the user's `~/.config`, `~/.local/share`, `~/waveapps`, dotfiles, or a running RemoteTerm (the user's dev profile is `~/.config/remoteterm-dev` and `~/.local/share/remoteterm-dev`; it may be running). Use `REMOTETERM_CONFIG_HOME`/`REMOTETERM_DATA_HOME`, never `REMOTETERM_HOME` (a legacy combined-home variable with different semantics).
- Never run `task dev`, `electron-vite dev` or `npm run dev`.
- `REMOTETERM_ISOLATED_PROFILE=1` is set as the recipe requires. Note in the report that nothing in this worktree reads it (`grep -rn ISOLATED_PROFILE emain frontend pkg cmd` finds nothing here); isolation rests on the config and data homes, `--user-data-dir`, and the scratch `HOME`, XDG and `TMPDIR` dirs.
- No window manager runs under Xvfb, so there is no title bar; the "title-bar close" check uses the renderer's `window.close()`, which takes the same BrowserWindow `close`/`closed` path. "Switch App" lives in a native context menu that cannot be clicked there; the check replays `switchBuilderApp`'s calls from the builder renderer (same RPCs, same `builder:<id>` route). `BuilderTermModel` is a module singleton that the page does not expose on `window`, so the replay cannot call `markSwitching()` first; the report must say that the "Switching app…" state is covered by unit tests only (Tasks 14 and 15). Say both substitutions in the report.

**Shell state and process ownership (every step):**
- Shell variables do not survive between tool calls. Step 1 prints the scratch root; every later shell call, including each fenced block below, starts with `SCR=<that literal path>; source "$SCR/lib.sh" || exit 1` (written below as `SCR=/tmp/rtbt.XXXX`; substitute the real path). `lib.sh` refuses to load unless `SCR` is a `/tmp/rtbt.*` path and `REPO`, `PORT`, `START`, `NODEBIN` are restored from `$SCR/logs/env.txt`; it then restores `SID`, `EL`, `SRV`, `XDISP` from `$SCR/logs/run.env` once a launch has been recorded. Nothing else is carried between calls: each block recomputes what it needs (block ids, tab ids, focused pane) from the DB or the page.
- A process belongs to this run only if its environment has `HOME=$SCR/home` or its cwd is under `$SCR` (`own_pid`). `own_sid` checks that the recorded session leader still has `HOME=$SCR/home`. Every use of `SID` goes with `own_sid || exit 1`, and every `kill` targets either this run's session after `own_sid` passed or a single PID that passed `own_pid`. Pane shells run in their own sessions (creack/pty uses `Setsid`), so cleanup also scans `/proc` for this run's processes (`own_procs`, `stop_run`).
- A launch runs `launch.sh` in the background of a non-interactive shell, so it is not a process-group leader and the `setsid` inside it runs in that same process: its PID is the session id. `launch_run` records it and requires the session found through the debugging port and the scratch `--user-data-dir` to be the same one.

- [ ] **Step 1: Scratch root, baseline and `lib.sh`**

```bash
REPO=/media/owner/Workspace/remoteterm/remoteterm-builder-terminal
node -e 'process.exit(typeof WebSocket === "function" ? 0 : 1)' || { echo "ABORT: this node has no global WebSocket (cdp.mjs needs Node 22+)"; exit 1; }
SCR=$(mktemp -d /tmp/rtbt.XXXX) && chmod 700 "$SCR"
mkdir -p "$SCR"/{home,run,cfg,data,ud,gocache,gomod,zigcache,vitecache,tmp,t,logs}
chmod 700 "$SCR/run" "$SCR/tmp"
touch "$SCR/start-marker"
START="$(date '+%Y-%m-%d %H:%M:%S')"
NODEBIN=$(dirname "$(command -v node)")
PORT=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
printf 'REPO=%q\nSCR=%q\nPORT=%q\nSTART=%q\nNODEBIN=%q\n' "$REPO" "$SCR" "$PORT" "$START" "$NODEBIN" | tee "$SCR/logs/env.txt"
for d in ~/.config/remoteterm* ~/.local/share/remoteterm* ~/.config/RemoteTerm* ~/waveapps; do
    [ -e "$d" ] && find "$d" -maxdepth 3 -type f -printf '%T@ %s %p\n' 2>/dev/null
done | sort -k3 > "$SCR/logs/user-files-before.txt"
stat -c '%Y %n' "$REPO/node_modules/.vite" "$REPO/node_modules/.vite-temp" > "$SCR/logs/vite-before.txt" 2>&1
pgrep -a -f 'remoteterm|electron' > "$SCR/logs/live-processes-before.txt" || true
echo "SCR=$SCR"
```

Only `find`/`stat`/`pgrep` touch the user's paths, read-only, to record a baseline of the files (mtime, size, path) in the user's RemoteTerm profiles and `~/waveapps`. Note the printed `SCR` path; it is the literal every later call starts with.

Create `$SCR/lib.sh`:

```bash
# Sourced at the start of every shell call, after SCR=<literal path>: source "$SCR/lib.sh" || exit 1
need() { for v; do [ -n "${!v}" ] || { echo "ABORT: $v unset"; return 1; }; done; }
need SCR || return 1
case "$SCR" in /tmp/rtbt.?*) ;; *) echo "ABORT: SCR=$SCR is not a scratch root"; return 1 ;; esac
source "$SCR/logs/env.txt" || return 1
need SCR REPO PORT START NODEBIN || return 1
if [ -f "$SCR/logs/run.env" ]; then source "$SCR/logs/run.env" || return 1; fi
T="$SCR/t"
APPDIR="$SCR/home/waveapps/draft/e2e1"
DB="$SCR/data/db/waveterm.db"

# Ownership: a process belongs to this run if its environment has HOME=$SCR/home or its cwd is under $SCR.
# Pane shells start their own sessions (creack/pty Setsid), so the session alone does not find them.
own_pid() {
    [[ "$1" =~ ^[0-9]+$ ]] || return 1
    tr '\0' '\n' 2>/dev/null < "/proc/$1/environ" | grep -qxF "HOME=$SCR/home" && return 0
    case "$(readlink "/proc/$1/cwd" 2>/dev/null)" in "$SCR" | "$SCR"/*) return 0 ;; esac
    return 1
}
# Required next to every use of SID: the recorded session leader must still be this run's.
own_sid() {
    need SID || return 1
    tr '\0' '\n' 2>/dev/null < "/proc/$SID/environ" | grep -qxF "HOME=$SCR/home" && return 0
    echo "session $SID: leader gone or not this run's (no HOME=$SCR/home in its environment)"
    return 1
}
own_procs() {
    local d p
    for d in /proc/[0-9]*; do
        p=${d#/proc/}
        [ "$p" = "$$" ] || [ "$p" = "$BASHPID" ] && continue
        own_pid "$p" && echo "$p"
    done
}
show_own() { local l; l=$(own_procs | paste -sd,); if [ -n "$l" ]; then ps -o pid,sid,args -p "$l"; else echo "(no process of this run)"; fi; }

cdp() { node "$SCR/cdp.mjs" "$PORT" "$@"; }
bcdp() { cdp "RTApp Builder" "$@"; }
mcdp() { cdp "RemoteTerm" "$@"; }
dbq() { python3 "$SCR/db.py" "$DB" "$@"; }
builder_tabs() { dbq "SELECT oid, json_extract(data,'\$.blockids') FROM db_tab WHERE json_extract(data,'\$.meta.\"builder:owner\"') IS NOT NULL"; }
builder_block_ids() { builder_tabs | python3 -c 'import json,sys; rows = json.loads(sys.stdin.read()); print(" ".join(json.loads(rows[0][1])) if rows else "")'; }
block_exists() { dbq "SELECT count(*) FROM db_block WHERE oid = ?" "$1"; }
wait_file() { for _ in $(seq 100); do [ -s "$1" ] && return 0; sleep 0.1; done; echo "timeout waiting for $1"; return 1; }
wait_dead() {
    [[ "$1" =~ ^[0-9]+$ ]] || { echo "FAIL: no pid"; return 1; }
    for _ in $(seq 50); do kill -0 "$1" 2>/dev/null || return 0; sleep 0.1; done
    echo "pid $1 still alive after 5s"; return 1
}
wait_block_gone() { for _ in $(seq 50); do [ "$(block_exists "$1")" = "[[0]]" ] && return 0; sleep 0.1; done; echo "block $1 still in the DB after 5s"; return 1; }

# Session id of the launch that owns the debugging port and the scratch profile; empty if none.
session_of_port() { local pid; pid=$(pgrep -o -f "remote-debugging-port=$PORT .*--user-data-dir=$SCR/ud"); [ -n "$pid" ] && ps -o sid= -p "$pid" | tr -d ' '; }
save_run() { printf 'SID=%q\nEL=%q\nSRV=%q\nXDISP=%q\n' "$SID" "$EL" "$SRV" "$XDISP" > "$SCR/logs/run.env"; }
# Starts launch.sh in the background and records it. The background child is not a process-group
# leader (no job control in a non-interactive shell), so setsid in launch.sh runs in that same
# process and its PID becomes the session id; session_of_port cross-checks it.
launch_run() {  # $1 = log name
    bash "$SCR/launch.sh" "$SCR" > "$SCR/logs/$1.log" 2>&1 &
    local lpid=$!
    SID= EL= SRV= XDISP=
    for _ in $(seq 90); do
        SID=$(session_of_port)
        if [ -n "$SID" ]; then
            EL=$(pgrep -o -s "$SID" -x electron)
            SRV=$(pgrep -o -s "$SID" -f '^[^ ]*/bin/remotetermsrv\.')
            [ -n "$EL" ] && [ -n "$SRV" ] && curl -sf "http://127.0.0.1:$PORT/json/list" | grep -q '"page"' && break
        fi
        sleep 1
    done
    [ -n "$EL" ] && XDISP=$(tr '\0' '\n' 2>/dev/null < "/proc/$EL/environ" | sed -n 's/^DISPLAY=:\([0-9]*\).*/\1/p')
    save_run; cat "$SCR/logs/run.env"
    need SID EL SRV || { echo "ABORT: launch not up after 90 s; see logs/$1.log"; return 1; }
    [ "$SID" = "$lpid" ] || { echo "ABORT: port's session $SID differs from launch pid $lpid"; return 1; }
    own_sid || return 1
    need XDISP || return 1
    [ "$XDISP" != 0 ] || { echo "ABORT: Electron is on display :0"; return 1; }
}
# Journal lines since START that mention this run's scratch root or any PID recorded for it.
journal_check() {
    local pids pat=(-e "$SCR")
    pids=$({ cat "$SCR/logs/pids.txt" 2>/dev/null; own_procs; } | sort -u | paste -sd'|')
    [ -n "$pids" ] && pat+=(-e "(^|[^0-9])($pids)([^0-9]|\$)")
    journalctl --user --since "$START" --no-pager 2>/dev/null | grep -iE 'remoteterm|\.scope' | grep -E "${pat[@]}" || true
}
preflight() {  # $1 = launch number
    need SID EL SRV && own_sid || return 1
    { pgrep -s "$SID"; own_procs; } >> "$SCR/logs/pids.txt"
    ps -s "$SID" -o pid,ppid,args > "$SCR/logs/session$1.txt"
    { tr '\0' '\n' < "/proc/$EL/environ" | grep -E '^(DISPLAY|WAYLAND_DISPLAY|HOME|TMPDIR|XDG_|REMOTETERM_)'
      tr '\0' '\n' < "/proc/$SRV/environ" | grep -E '^(DISPLAY|HOME|REMOTETERM_)'; } > "$SCR/logs/preflight$1-environ.txt"
    find "/proc/$SRV/fd" -mindepth 1 -maxdepth 1 -printf '%l\n' | grep -v -e "$SCR" -e "$REPO" -e '^pipe:' -e '^socket:' -e '^anon_inode' -e '^/dev/' -e '^/proc/' -e '^/sys/' > "$SCR/logs/preflight$1-fds-outside.txt" || true
    journal_check > "$SCR/logs/preflight$1-journal.txt"
    head -50 "$SCR/logs/session$1.txt" "$SCR/logs/preflight$1-environ.txt" "$SCR/logs/preflight$1-fds-outside.txt" "$SCR/logs/preflight$1-journal.txt"
}
# Open fds of this run's processes that point into the real HOME (outside $SCR).
fd_scan() {
    local p
    for p in $({ [ -n "$SID" ] && pgrep -s "$SID"; own_procs; } | sort -u); do
        own_pid "$p" || { echo "$p not owned, skipped"; continue; }
        find "/proc/$p/fd" -mindepth 1 -maxdepth 1 -printf "$p %l\n" 2>/dev/null
    done | grep -F " $HOME/" | grep -vF " $SCR/" || true
}
# Stops this run: Electron first, then the session, then every remaining process that passes own_pid.
stop_run() {
    local p
    if [ -n "$SID" ] && own_sid; then
        own_pid "$EL" && kill -TERM "$EL" && sleep 5
        pkill -TERM -s "$SID"; sleep 5; pkill -KILL -s "$SID"
    fi
    show_own > "$SCR/logs/stragglers.txt"; cat "$SCR/logs/stragglers.txt"
    for p in $(own_procs); do kill -TERM "$p" 2>/dev/null; done; sleep 2
    for p in $(own_procs); do kill -KILL "$p" 2>/dev/null; done; sleep 1
    [ -z "$(own_procs)" ] && echo "no process of this run left" && return 0
    echo "ABORT: processes of this run remain:"; show_own; return 1
}

# Merges one setting into the scratch settings.json; the app's config watcher picks it up.
set_setting() {
    python3 - "$SCR/cfg/settings.json" "$1" "$2" <<'PY'
import json, os, sys
path, key, value = sys.argv[1], sys.argv[2], json.loads(sys.argv[3])
data = json.load(open(path)) if os.path.exists(path) else {}
data[key] = value
json.dump(data, open(path, "w"), indent=2)
PY
}
focused_block() { bcdp eval "document.activeElement?.closest('[data-blockid]')?.dataset.blockid ?? null"; }
builder_focus() { bcdp eval "document.querySelector('[data-builder-focus]')?.dataset.builderFocus ?? null"; }
dom_panes() { bcdp eval "document.querySelectorAll('[data-builder-term-panel] .block[data-blockid]').length"; }
# Types a command into the focused pane and presses Enter.
run_in_pane() { bcdp text "$1" && bcdp key Enter; }
# Focuses a pane by clicking its terminal area.
focus_pane() { bcdp click "[data-builder-term-panel] [data-blockid=\"$1\"] .block-content" || bcdp click "[data-builder-term-panel] [data-blockid=\"$1\"]"; }
# Call before an action that should add one pane.
mark_panes() { builder_block_ids | tr ' ' '\n' | sed '/^$/d' | sort > "$T/ids-prev"; }
# After that action: exactly one new block id; it is focused, the DB and DOM hold $2 panes,
# builder focus is "terminal", and the new pane's shell starts in the app folder.
check_new_pane() {  # $1 = label, $2 = expected pane count
    local ids count new dom foc bf
    for _ in $(seq 50); do
        ids=$(builder_block_ids); count=$(echo $ids | wc -w)
        [ "$count" -ge "$2" ] && break; sleep 0.2
    done
    sleep 1
    new=$(echo $ids | tr ' ' '\n' | sed '/^$/d' | sort | comm -13 "$T/ids-prev" -)
    dom=$(dom_panes); foc=$(focused_block); bf=$(builder_focus)
    echo "$1: db=$count dom=$dom new=[$new] focused=$foc builderfocus=$bf"
    [ "$(echo $new | wc -w)" = 1 ] && [ "$count" = "$2" ] && [ "$dom" = "$2" ] \
        && [ "$foc" = "\"$new\"" ] && [ "$bf" = '"terminal"' ] \
        && run_in_pane "pwd > $T/$1.pwd" && wait_file "$T/$1.pwd" && [ "$(cat "$T/$1.pwd")" = "$APPDIR" ] \
        && echo "$1 PASS" || echo "$1 FAIL"
}
```

(`main` windows are titled `RemoteTerm - <tab>`; builder windows `RTApp Builder (draft/e2e1)`, `frontend/remoteterm.ts:265`, `frontend/builder/builder-app.tsx:52`. The server's `argv[0]` is `<app base>/bin/remotetermsrv.<arch>`, `emain/emain-platform.ts:777-785`; no other process of the session has that as its first word.)

Check it loads:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
echo "lib ok: REPO=$REPO PORT=$PORT"
```

- [ ] **Step 2: Build with scratch caches**

Create `$SCR/e2e.vite.config.ts` so every Vite step uses a scratch cache dir:

```ts
import base from "/media/owner/Workspace/remoteterm/remoteterm-builder-terminal/electron.vite.config";

const cacheDir = process.env.E2E_VITE_CACHE;

export default {
    ...base,
    main: { ...base.main, cacheDir },
    preload: { ...base.preload, cacheDir },
    renderer: { ...base.renderer, cacheDir },
};
```

Then build (the build needs network for Go and npm modules; `DISPLAY` is not set):

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
set -o pipefail
cd "$REPO"
BUILD_ENV=(env -i PATH="$REPO/golang-1.26.2/bin:$NODEBIN:/usr/local/bin:/usr/bin:/bin" HOME="$SCR/home" TMPDIR="$SCR/tmp" \
    XDG_CONFIG_HOME="$SCR/home/.config" XDG_CACHE_HOME="$SCR/home/.cache" XDG_DATA_HOME="$SCR/home/.local/share" \
    GOCACHE="$SCR/gocache" GOMODCACHE="$SCR/gomod" ZIG_GLOBAL_CACHE_DIR="$SCR/zigcache" ZIG_LOCAL_CACHE_DIR="$SCR/zigcache" \
    E2E_VITE_CACHE="$SCR/vitecache")
"${BUILD_ENV[@]}" ./node_modules/.bin/task build:backend build:tsunamiscaffold build:tsunamisdk 2>&1 | tee "$SCR/logs/build-task.log" \
    || { echo "ABORT: task build failed"; exit 1; }
"${BUILD_ENV[@]}" node_modules/.bin/electron-vite build --mode production --config "$SCR/e2e.vite.config.ts" 2>&1 | tee "$SCR/logs/build-vite.log" \
    || { echo "ABORT: electron-vite build failed"; exit 1; }
for f in dist/bin/remotetermsrv.* dist/main/index.js dist/preload/index.cjs dist/frontend/index.html; do
    [ "$f" -nt "$SCR/start-marker" ] || { echo "ABORT: $f was not rebuilt by this run"; exit 1; }
done
ls -l dist/bin/remotetermsrv.* dist/main/index.js dist/frontend/index.html
git status --short
```

Expected: both builds succeed and every listed output is newer than `$SCR/start-marker`; `git status --short` lists only the untracked toolchains (and `.planning/builder-terminal/E2E-REPORT.md` once written). If `dist/preload/index.cjs` is not the preload's file name, take the real name from `ls dist/preload` and record it; do not drop the check. If `electron-vite` cannot load the wrapper config, stop and report the error; do not fall back to the repo config without saying so.

- [ ] **Step 3: Helpers**

Create `$SCR/cdp.mjs` (Chrome DevTools Protocol client using Node's built-in `fetch` and `WebSocket`; Node 22):

```js
// Usage: node cdp.mjs <port> <title-substring> <command> [args...]
// Commands: targets | eval <expr> | key <combo> | keys <combo>... | keyrepeat <combo> | text <string>
//           click <css-selector> | clicktext <button-text> | drag <css-selector> <dx> | shot <file.png>
import fs from "node:fs";

// A hung page or socket must not stall the run: give up after 30 s.
const watchdog = setTimeout(() => {
    console.error("cdp: timed out after 30 s");
    process.exit(3);
}, 30000);

const [port, match, cmd, ...args] = process.argv.slice(2);
const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
if (cmd === "targets") {
    console.log(JSON.stringify(targets.filter((t) => t.type === "page").map((t) => t.title)));
    process.exit(0);
}
const target = targets.find((t) => t.type === "page" && t.title.includes(match));
if (target == null) {
    console.error(`no page target matching ${JSON.stringify(match)}; pages: ${JSON.stringify(targets.map((t) => t.title))}`);
    process.exit(2);
}
const ws = new WebSocket(target.webSocketDebuggerUrl);
const pending = new Map();
let nextId = 0;
ws.onmessage = (msg) => {
    const data = JSON.parse(msg.data);
    if (data.id != null && pending.has(data.id)) {
        pending.get(data.id)(data);
        pending.delete(data.id);
    }
};
await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = reject;
});
function send(method, params = {}) {
    return new Promise((resolve) => {
        const id = ++nextId;
        pending.set(id, resolve);
        ws.send(JSON.stringify({ id, method, params }));
    });
}
async function evaluate(expression) {
    const res = await send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (res.result?.exceptionDetails) {
        throw new Error(res.result.exceptionDetails.exception?.description ?? JSON.stringify(res.result.exceptionDetails));
    }
    return res.result?.result?.value;
}
const Modifiers = { Alt: 1, Ctrl: 2, Meta: 4, Shift: 8 };
const NamedKeys = {
    Enter: ["Enter", "Enter", 13],
    Escape: ["Escape", "Escape", 27],
    ArrowUp: ["ArrowUp", "ArrowUp", 38],
    ArrowDown: ["ArrowDown", "ArrowDown", 40],
    ArrowLeft: ["ArrowLeft", "ArrowLeft", 37],
    ArrowRight: ["ArrowRight", "ArrowRight", 39],
};
function keyParams(combo) {
    const parts = combo.split("+");
    const name = parts.pop();
    const modifiers = parts.reduce((acc, part) => acc | Modifiers[part], 0);
    if (NamedKeys[name]) {
        const [key, code, keyCode] = NamedKeys[name];
        return { modifiers, key, code, windowsVirtualKeyCode: keyCode };
    }
    const upper = name.toUpperCase();
    const key = modifiers & Modifiers.Shift ? upper : name.toLowerCase();
    return { modifiers, key, code: `Key${upper}`, windowsVirtualKeyCode: upper.charCodeAt(0) };
}
async function pressKey(combo, autoRepeat = false) {
    const params = keyParams(combo);
    await send("Input.dispatchKeyEvent", { type: "rawKeyDown", autoRepeat, ...params });
    await send("Input.dispatchKeyEvent", { type: "keyUp", ...params });
}
async function centerOf(selectorExpr) {
    const rect = await evaluate(`(() => { const el = ${selectorExpr}; if (!el) return null; const r = el.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; })()`);
    if (rect == null) {
        throw new Error(`no element for ${selectorExpr}`);
    }
    return rect;
}
async function mouse(type, x, y) {
    await send("Input.dispatchMouseEvent", { type, x, y, button: "left", buttons: type === "mouseReleased" ? 0 : 1, clickCount: 1 });
}
try {
    if (cmd === "eval") {
        console.log(JSON.stringify(await evaluate(args[0])));
    } else if (cmd === "key") {
        await pressKey(args[0]);
    } else if (cmd === "keys") {
        await Promise.all(args.map((combo) => pressKey(combo)));
    } else if (cmd === "keyrepeat") {
        await pressKey(args[0], true);
    } else if (cmd === "text") {
        await send("Input.insertText", { text: args[0] });
    } else if (cmd === "click" || cmd === "clicktext") {
        const expr =
            cmd === "click"
                ? `document.querySelector(${JSON.stringify(args[0])})`
                : `[...document.querySelectorAll("button")].find((b) => b.textContent.trim() === ${JSON.stringify(args[0])} && b.getBoundingClientRect().width > 0)`;
        const { x, y } = await centerOf(expr);
        await mouse("mousePressed", x, y);
        await mouse("mouseReleased", x, y);
    } else if (cmd === "drag") {
        const { x, y } = await centerOf(`document.querySelector(${JSON.stringify(args[0])})`);
        const dx = Number(args[1]);
        await mouse("mousePressed", x, y);
        for (let step = 1; step <= 10; step++) {
            await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: x + (dx * step) / 10, y, button: "left", buttons: 1 });
        }
        await mouse("mouseReleased", x + dx, y);
    } else if (cmd === "shot") {
        const res = await send("Page.captureScreenshot", { format: "png" });
        fs.writeFileSync(args[0], Buffer.from(res.result.data, "base64"));
    } else {
        throw new Error(`unknown command ${cmd}`);
    }
} catch (e) {
    console.error(e.message);
    process.exitCode = 1;
}
clearTimeout(watchdog);
ws.close();
```

Create `$SCR/db.py` (read-only SQLite queries; the DB runs in WAL mode, so reads are safe while the app runs):

```python
import json
import sqlite3
import sys

conn = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
print(json.dumps(conn.execute(sys.argv[2], sys.argv[3:]).fetchall()))
```

- [ ] **Step 4: Launch and safety preflight**

Create `$SCR/launch.sh`:

```bash
#!/bin/bash
# DISPLAY and WAYLAND_DISPLAY are never passed in: env -i starts from nothing and xvfb-run sets DISPLAY for its children only.
# setsid runs in this process (it is not a group leader), so this PID becomes the session id that launch_run records.
[ -f "$1/logs/env.txt" ] || exit 1
source "$1/logs/env.txt" || exit 1
cd "$REPO" || exit 1
ulimit -c 0
exec env -i \
    PATH="$REPO/golang-1.26.2/bin:$NODEBIN:/usr/local/bin:/usr/bin:/bin" \
    HOME="$SCR/home" XDG_RUNTIME_DIR="$SCR/run" TMPDIR="$SCR/tmp" \
    XDG_CONFIG_HOME="$SCR/home/.config" XDG_DATA_HOME="$SCR/home/.local/share" \
    XDG_CACHE_HOME="$SCR/home/.cache" XDG_STATE_HOME="$SCR/home/.local/state" \
    REMOTETERM_CONFIG_HOME="$SCR/cfg" REMOTETERM_DATA_HOME="$SCR/data" \
    REMOTETERM_ISOLATED_PROFILE=1 REMOTETERM_NOCONFIRMQUIT=1 \
    GOCACHE="$SCR/gocache" GOMODCACHE="$SCR/gomod" E2E_VITE_CACHE="$SCR/vitecache" \
    setsid xvfb-run -a -s "-screen 0 1600x1000x24" dbus-run-session -- \
    node_modules/.bin/electron-vite preview --skipBuild --config "$SCR/e2e.vite.config.ts" -- \
    --remote-debugging-port="$PORT" --user-data-dir="$SCR/ud"
```

`TMPDIR` puts `xvfb-run`'s temporary directory (and the app's) under `$SCR/tmp`, so nothing of this run is left in `/tmp` except the Xvfb lock and socket handled in Step 11.

Launch (run 1), record it and run the preflight:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
launch_run launch1 || { echo "ABORT: run Step 11 cleanup and report"; exit 1; }
preflight 1
```

`launch_run` polls for up to 90 s until the session, Electron, the server and a page on `/json/list` are all up, then requires: the port's session equals the launch PID, `own_sid` passes, and `XDISP` is set and not `0`. Expected from `preflight 1`: `DISPLAY=:<XDISP>` for both processes, no `WAYLAND_DISPLAY`; `HOME=$SCR/home`, `TMPDIR=$SCR/tmp`; `REMOTETERM_CONFIG_HOME=$SCR/cfg`, `REMOTETERM_DATA_HOME=$SCR/data`; `preflight1-fds-outside.txt` empty; `preflight1-journal.txt` (journal lines since `START` naming `$SCR` or one of this run's PIDs) shows no unit or scope. Any failure here: run Step 11's cleanup in a new call and report; do not continue.

If a first-run feature-tour modal covers the main window, dismiss it through its own button with `mcdp clicktext ...` (the earlier run saw a "Durable SSH Sessions" tour).

- [ ] **Step 5: S1 (first open) and S5 (resize persists)**

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
dbq "SELECT count(*) FROM db_workspace" | tee "$SCR/logs/workspaces-before.txt"
mcdp eval "window.api.openBuilder()"
sleep 5
bcdp click 'input[placeholder="my-app"]'
bcdp text "e2e1" && bcdp key Enter
sleep 8
builder_tabs | tee "$SCR/logs/s1-tabs.txt"
```

Expected: exactly one builder tab with one block id. Then:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
B1=$(builder_block_ids)
[ "$(focused_block)" = "\"$B1\"" ] && echo "S1 focus PASS" || echo "S1 focus FAIL: $(focused_block)"
[ "$(builder_focus)" = '"terminal"' ] && echo "S1 builder focus PASS" || echo "S1 builder focus FAIL"
run_in_pane "pwd > $T/s1.pwd; echo \$\$ > $T/p1.pid" && wait_file "$T/p1.pid" && [ "$(cat "$T/s1.pwd")" = "$APPDIR" ] \
    && echo "S1 pwd PASS" || echo "S1 pwd FAIL: $(cat "$T/s1.pwd" 2>/dev/null)"
bcdp shot "$SCR/logs/s1.png"
```

S5:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
bcdp drag '[data-builder-focus] > [data-panel-group] > [data-resize-handle]' 200
sleep 1.5
bcdp eval "document.querySelector('[data-builder-focus] > [data-panel-group] > [data-panel]').dataset.panelSize"
bcdp eval "window.RpcApi.GetRTInfoCommand(window.TabRpcClient, {oref: 'builder:' + window.globalStore.get(window.globalAtoms.builderId)}).then((r) => r['builder:layout'])"
builder_block_ids > "$SCR/logs/s5-blocks-before-reload.txt"
bcdp eval "location.reload()"; sleep 8
bcdp eval "document.querySelector('[data-builder-focus] > [data-panel-group] > [data-panel]').dataset.panelSize"
builder_block_ids | diff "$SCR/logs/s5-blocks-before-reload.txt" - && echo "reload kept the blocks PASS" || echo "reload kept the blocks FAIL"
wait_dead_quick() { kill -0 "$1" 2>/dev/null; }
[[ "$(cat "$T/p1.pid" 2>/dev/null)" =~ ^[0-9]+$ ]] && kill -0 "$(cat "$T/p1.pid")" && echo "reload kept the shell PASS" || echo "reload kept the shell FAIL"
```

Expected: the panel size grows above 40 and `builder:layout.terminal` matches it (within 1); after the reload the size is the same, the builder tab's block ids are unchanged, and the p1 shell is alive (reload keeps panes).

- [ ] **Step 6: S2 (Open terminal), S3 (keys, header split, two rapid splits), S8 (no builder tab in workspaces)**

Header split buttons render only when `term:showsplitbuttons` is true (`frontend/app/block/blockframe-header.tsx:130, 137`), and it defaults to false (`pkg/rtconfig/defaultconfig/settings.json:5`). Turn it on first and require the button before any check:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
set_setting "term:showsplitbuttons" true
sleep 2
bcdp eval "!!document.querySelector('[data-builder-term-panel] button[title=\"Split Horizontally\"]')"
```

Expected: `true`. If it prints `false`, stop and report (the S3 header-split check cannot run).

Record the main windows' tabs, then add panes. `mark_panes` saves the block ids before each action; `check_new_pane` then requires exactly one new id (the set difference), that it is the focused pane, that the DB and the DOM both hold the expected count, that builder focus is `"terminal"`, and that the new pane's shell starts in the app folder. Pane counts: 1 after Step 5, then 2 (Open terminal), 3 (Alt+D), 4 (Shift+Alt+D), 5 (chord), 6 (Alt+N), 7 (header split), 9 (two rapid splits).

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
main_tab_counts() { dbq "SELECT oid, json_array_length(json_extract(data,'\$.blockids')) FROM db_tab WHERE json_extract(data,'\$.meta.\"builder:owner\"') IS NULL"; }
main_tab_counts > "$SCR/logs/main-tabs-before.txt"
mark_panes; bcdp clicktext "Open terminal"; check_new_pane s2 2
main_tab_counts | diff "$SCR/logs/main-tabs-before.txt" - && echo "S2 main windows untouched PASS" || echo "S2 main windows untouched FAIL"
mark_panes; bcdp key Alt+D; check_new_pane s3-altd 3
mark_panes; bcdp key Shift+Alt+D; check_new_pane s3-shiftaltd 4
mark_panes; bcdp key Ctrl+Shift+S; bcdp key ArrowDown; check_new_pane s3-chord 5
mark_panes; bcdp key Alt+N; check_new_pane s3-altn 6
FOCUSED=$(focused_block | tr -d '"')
mark_panes; bcdp click "[data-builder-term-panel] [data-blockid=\"$FOCUSED\"] button[title=\"Split Horizontally\"]"; check_new_pane s3-header 7
bcdp keys Alt+D Alt+D; sleep 3
echo "rapid: db=$(builder_block_ids | wc -w) dom=$(dom_panes)"
```

Expected: every step PASS; the rapid pair leaves 9 blocks in the DB and 9 panes in the DOM. If the DB has 9 blocks but the DOM shows 8 after 3 s, the layout-save race from the spec's Risks reproduced: record it (screenshot, both counts, and the LayoutState's `pendingbackendactions` via `bcdp eval` with `window.RpcApi`), mark S3-rapid FAIL, and continue.

S8:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
BT=$(builder_tabs | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())[0][0])')
[ -n "$BT" ] || { echo "ABORT: no builder tab"; exit 1; }
dbq "SELECT json_extract(data,'\$.tabids') FROM db_workspace" | grep -q "$BT" && echo "S8 FAIL (workspace tabids)" || echo "S8 tabids PASS"
mcdp eval "[...document.querySelectorAll('[data-tab-id],[data-tabid]')].map((e) => e.dataset.tabId ?? e.dataset.tabid)" | grep -q "$BT" && echo "S8 FAIL (tab bar)" || echo "S8 tab bar PASS"
dbq "SELECT count(*) FROM db_workspace" | diff "$SCR/logs/workspaces-before.txt" - && echo "S8 workspace count PASS" || echo "S8 workspace count FAIL"
```

Expected: all three PASS. Here `grep -q` exiting 1 (no match) is the passing case.

- [ ] **Step 7: S4 (close panes), Ctrl+W, held Alt+W**

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
FOCUSED=$(focused_block | tr -d '"')
run_in_pane "echo \$\$ > $T/p4.pid" && wait_file "$T/p4.pid" || { echo "S4 FAIL: no shell pid"; exit 1; }
bcdp key Alt+W
wait_block_gone "$FOCUSED" && wait_dead "$(cat "$T/p4.pid")" && echo "S4 close PASS" || echo "S4 close FAIL"
FOCUSED=$(focused_block | tr -d '"'); focus_pane "$FOCUSED"
bcdp text "echo hello world"; bcdp key Ctrl+W; bcdp text " > $T/ctrlw.txt"; bcdp key Enter
wait_file "$T/ctrlw.txt" && [ "$(cat "$T/ctrlw.txt")" = "hello" ] && echo "Ctrl+W PASS" || echo "Ctrl+W FAIL: $(cat "$T/ctrlw.txt" 2>/dev/null)"
```

Record every remaining pane's shell PID, then close them one at a time:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
for id in $(builder_block_ids); do
    focus_pane "$id" && run_in_pane "echo \$\$ > $T/pid-$id" && wait_file "$T/pid-$id" || echo "FAIL: no pid for pane $id"
done
for _ in $(seq 20); do
    [ -z "$(builder_block_ids)" ] && break
    bcdp key Alt+W; sleep 0.4
done
for f in "$T"/pid-*; do
    [ -e "$f" ] || { echo "FAIL: no pid files"; break; }
    wait_dead "$(cat "$f")" || echo "FAIL: shell of $f still alive"
done
builder_tabs; builder_focus
bcdp eval "!!document.evaluate(\"//*[text()='No terminals']\", document, null, 9, null).singleNodeValue"
cdp x targets
bcdp keyrepeat Alt+W; sleep 1; cdp x targets
bcdp clicktext "Open terminal"; sleep 2; builder_tabs
```

Expected: after the last Alt+W the builder tab still exists with `[]` blocks, every recorded PID is gone within 5 s (no FAIL line), builder focus is `"app"`, "No terminals" is shown, and the builder window is still listed by `targets`; the auto-repeated Alt+W leaves the window open; the empty-state "Open terminal" adds one pane.

- [ ] **Step 8: S9 (two writers, one rebuild)**

Wait for the app to be built and running, then let two panes append to `app.go` at the same moment:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
set_setting "builder:liverebuild" true
for _ in $(seq 120); do grep -q "status: running" "$APPDIR/.tsunami/build.log" 2>/dev/null && break; sleep 1; done
bcdp key Alt+D; sleep 2
DOT="document.querySelector('span.w-2.h-2.rounded-full')"
for _ in $(seq 60); do bcdp eval "$DOT?.className ?? ''" | grep -q bg-success && break; sleep 1; done
bcdp eval "window.__e2eTransitions = []; const dot = $DOT; new MutationObserver(() => window.__e2eTransitions.push(dot.className)).observe(dot, { attributes: true, attributeFilter: ['class'] }); !!dot"
wc -l < "$APPDIR/app.go" > "$T/app-lines-before"
for id in $(builder_block_ids); do
    focus_pane "$id" && run_in_pane "while [ ! -e $T/go ]; do :; done; echo >> app.go" || echo "FAIL: could not arm pane $id"
done
touch "$T/go"
# build.log is replaced on every build (pkg/remotetermappstore/buildlog.go:63-68): wait for one written after $T/go.
for _ in $(seq 120); do
    [ "$APPDIR/.tsunami/build.log" -nt "$T/go" ] && grep -q "status: running" "$APPDIR/.tsunami/build.log" && break
    sleep 1
done
sleep 3
echo "app.go lines: $(cat "$T/app-lines-before") -> $(wc -l < "$APPDIR/app.go")"
bcdp eval "window.__e2eTransitions"
```

Expected: the observer finds the dot (`true`); `app.go` grew by exactly two lines; a `build.log` newer than `$T/go` reports `status: running`; the transitions list contains exactly one class string with `bg-warning` (one `building` transition), followed by `bg-success`. More than one `bg-warning` is a FAIL. If the post-edit wait ran out (120 s) without a newer `build.log`, mark S9 FAIL with the log's content.

- [ ] **Step 9: S6 (teardown on close, on Alt+W from the app side, on app switch)**

For each of the three paths, run one call that records, acts and checks. The recording part is the same each time:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
TAB=$(builder_tabs | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())[0][0])')
[ -n "$TAB" ] || { echo "ABORT: no builder tab"; exit 1; }
IDS=$(builder_block_ids)
for id in $IDS; do
    focus_pane "$id" && run_in_pane "echo \$\$ > $T/s6-$id.pid" && wait_file "$T/s6-$id.pid" || echo "FAIL: no pid for pane $id"
done
# (a) BrowserWindow close, as from the title bar:
bcdp eval "window.close()"
sleep 5
builder_tabs | grep -q "$TAB" && echo "S6 FAIL: tab $TAB still present" || echo "S6 tab gone PASS"
for id in $IDS; do
    [ "$(block_exists "$id")" = "[[0]]" ] || echo "S6 FAIL: block $id left"
    wait_dead "$(cat "$T/s6-$id.pid" 2>/dev/null)" || echo "S6 FAIL: shell of $id"
done
cdp x targets
```

For (b), start with `mcdp eval "window.api.openBuilder('draft/e2e1')"; sleep 8`, then the recording part, then `bcdp clicktext "Code"; sleep 0.5; builder_focus` (expect `"app"`) and `bcdp key Alt+W` in place of `window.close()`, then the same checks.

For (c), start the same way, then the recording part, then check reachability and replay `switchBuilderApp`:

```bash
bcdp eval "typeof window.BuilderTermModel"
bcdp eval "(async () => { const id = window.globalStore.get(window.globalAtoms.builderId); await window.RpcApi.DeleteBuilderCommand(window.TabRpcClient, id); await new Promise((r) => setTimeout(r, 500)); await window.RpcApi.SetRTInfoCommand(window.TabRpcClient, { oref: 'builder:' + id, data: { 'builder:appid': null } }); await window.api.setBuilderWindowAppId(null); window.api.doRefresh(); return id; })()"
```

then the same checks.

Expected: all three paths PASS; after (a) and (b) the builder page is gone from `targets`; after (c) the window is still open and shows the app selection modal. For (c), `typeof window.BuilderTermModel` should print `"undefined"` (the model is not reachable): record in the report that `markSwitching()` was not exercised end to end and is covered by unit tests only. If it is reachable, call `window.BuilderTermModel.getInstance().markSwitching()` before the replay and record that instead. (The app panel's tab bar has a "Code" button, `frontend/builder/builder-apppanel.tsx`; any click inside the app column sets app focus.)

- [ ] **Step 10: S7 (sweep after a crash) and S10 (spoofed workspace tab survives)**

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
need SRV && own_sid || exit 1
bcdp eval "window.close()"; sleep 2
mcdp eval "window.api.openBuilder('draft/e2e1')"; sleep 8
builder_tabs | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())[0][0])' > "$SCR/logs/crash-tab.txt"
[ -s "$SCR/logs/crash-tab.txt" ] || { echo "ABORT: no builder tab to leave behind"; exit 1; }
own_pid "$SRV" && [ "$(ps -o sid= -p "$SRV" | tr -d ' ')" = "$SID" ] || { echo "ABORT: $SRV is not this run's server"; exit 1; }
kill -9 "$SRV"
for _ in $(seq 30); do [ -z "$(own_procs)" ] && break; sleep 1; done
show_own
```

Expected: `(no process of this run)`. If any remain, run `stop_run` in a new call and require it to end with "no process of this run left". Then, as one call:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
# The DB is written directly only while no process of this run is alive.
[ -z "$(own_procs)" ] || { echo "ABORT: processes of this run are alive"; show_own; exit 1; }
CRASH_TAB=$(cat "$SCR/logs/crash-tab.txt")
[ "$(dbq "SELECT count(*) FROM db_tab WHERE oid = ?" "$CRASH_TAB")" = "[[1]]" ] && echo "crash left the builder tab (expected)" || { echo "PRECONDITION FAIL: no leftover tab"; exit 1; }
WS_TAB=$(dbq "SELECT json_extract(data,'\$.tabids[0]') FROM db_workspace LIMIT 1" | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())[0][0])')
[ -n "$WS_TAB" ] || { echo "ABORT: no workspace tab to spoof"; exit 1; }
echo "$WS_TAB" > "$SCR/logs/spoofed-tab.txt"
python3 - "$DB" "$WS_TAB" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
conn.execute("""UPDATE db_tab SET data = json_set(data, '$.meta."builder:owner"', 'e2e-spoof') WHERE oid = ?""", (sys.argv[2],))
conn.commit()
PY
launch_run launch2 || { echo "ABORT: run Step 11 cleanup and report"; exit 1; }
preflight 2
# WAVESRV-ESTART is consumed by emain without logging it (emain/emain-remotetermsrv.ts:107-121); its markers stand in:
# "spawned remotetermsrv" (emain/emain-remotetermsrv.ts:86) and "remotetermsrv ready signal received" (emain/emain.ts:324).
awk '/spawned remotetermsrv/ { n = NR; s = 0; r = 0 }
     n && !r && /\[startup\] builder sweep: removed 1 tabs/ { s = NR }
     n && !r && /remotetermsrv ready signal received/ { r = NR }
     END { if (n && s && r && s < r) print "S7 order PASS (spawned " n ", sweep " s ", ready " r ")";
           else print "S7 order FAIL (spawned " n ", sweep " s ", ready " r ")" }' "$SCR/data/rtapp.log"
[ "$(dbq "SELECT count(*) FROM db_tab WHERE oid = ?" "$CRASH_TAB")" = "[[0]]" ] && echo "S7 crash tab swept PASS" || echo "S7 crash tab swept FAIL"
# The spoofed workspace tab now carries builder:owner, so it is the only row builder_tabs may return.
builder_tabs | python3 -c 'import json,sys; rows = json.loads(sys.stdin.read()); ok = [r[0] for r in rows] == [sys.argv[1]]; print(("S7 builder tabs PASS " if ok else "S7 builder tabs FAIL ") + json.dumps(rows))' "$WS_TAB"
dbq "SELECT json_extract(data,'\$.meta.\"builder:owner\"') FROM db_tab WHERE oid = ?" "$WS_TAB" | grep -q e2e-spoof && echo "S10 owner kept PASS" || echo "S10 owner kept FAIL"
dbq "SELECT json_extract(data,'\$.tabids') FROM db_workspace" | grep -q "$WS_TAB" && echo "S10 still in workspace PASS" || echo "S10 still in workspace FAIL"
```

Expected: `launch_run` and `preflight 2` pass as in Step 4 (record the preflight); in the second launch's part of `rtapp.log` (after its last "spawned remotetermsrv"), `[startup] builder sweep: removed 1 tabs` comes before "remotetermsrv ready signal received"; the crash tab is gone; `builder_tabs` returns exactly the spoofed tab; the spoofed tab still carries `e2e-spoof` and is still in its workspace's `tabids` (S10). The report says that the emain markers stand in for `WAVESRV-ESTART`, which emain consumes without logging.

- [ ] **Step 11: Cleanup and isolation proof**

First, while the app still runs, the final fd scan and the positive evidence that the scratch profile was used:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
{ [ -n "$SID" ] && pgrep -s "$SID"; own_procs; } >> "$SCR/logs/pids.txt"
fd_scan | tee "$SCR/logs/final-fds-in-home.txt"
find "$SCR/data/db" "$SCR/cfg" -type f -newer "$SCR/start-marker" -printf '%TT %s %p\n' | tee "$SCR/logs/scratch-profile-written.txt"
```

Expected: `final-fds-in-home.txt` is empty (no process of this run has a file under the real `$HOME` open); `scratch-profile-written.txt` lists at least `waveterm.db` (or its `-wal`) and `settings.json`.

Then stop everything this run started and check what is left:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
need XDISP || exit 1
stop_run || { echo "ABORT: leave $SCR in place and report the processes above"; exit 1; }
[ -n "$SID" ] && ps -s "$SID" -o pid,args
# Xvfb removes its own lock and socket on a clean exit. Remove leftovers only if the lock names a PID recorded for this run that is gone.
XLOCK="/tmp/.X$XDISP-lock"
if [ -e "$XLOCK" ]; then
    XPID=$(tr -dc '0-9' < "$XLOCK")
    if ! grep -qx "$XPID" "$SCR/logs/pids.txt"; then echo "$XLOCK names PID $XPID, not this run's; leaving it"
    elif kill -0 "$XPID" 2>/dev/null; then echo "Xvfb $XPID still running; leaving its lock and socket"
    else rm -f "$XLOCK" "/tmp/.X11-unix/X$XDISP" && echo "removed $XLOCK and /tmp/.X11-unix/X$XDISP"; fi
fi
for d in ~/.config/remoteterm* ~/.local/share/remoteterm* ~/.config/RemoteTerm* ~/waveapps; do
    [ -e "$d" ] && find "$d" -maxdepth 3 -type f -printf '%T@ %s %p\n' 2>/dev/null
done | sort -k3 > "$SCR/logs/user-files-after.txt"
diff "$SCR/logs/user-files-before.txt" "$SCR/logs/user-files-after.txt" > "$SCR/logs/user-files-diff.txt"; cat "$SCR/logs/user-files-diff.txt"
ls -d ~/waveapps/draft/e2e1 2>/dev/null && echo "ISOLATION FAIL: app folder created in the real HOME"
stat -c '%Y %n' "$REPO/node_modules/.vite" "$REPO/node_modules/.vite-temp" 2>&1 | diff "$SCR/logs/vite-before.txt" - && echo "vite stamps unchanged"
journal_check
```

Expected: "no process of this run left"; the session list is empty; `~/waveapps/draft/e2e1` does not exist; the `.vite` stamps are unchanged; `journal_check` prints nothing. A line in `user-files-diff.txt` is a lead to investigate, not proof of a leak: the user's live dev instance (see `live-processes-before.txt`) may be running and writing its own files. For each changed or new path, check `final-fds-in-home.txt` and the `preflight*-fds-outside.txt` lists; if no process of this run had it open, record it as the live instance's write; if one did, it is an isolation FAIL. Never touch those files.

Copy the evidence you cite (logs, PNGs) into the report or next to it. Then, in a final call:

```bash
SCR=/tmp/rtbt.XXXX; source "$SCR/lib.sh" || exit 1
[ -z "$(own_procs)" ] || { echo "ABORT: processes of this run are alive; not removing $SCR"; show_own; exit 1; }
chmod -R u+w "$SCR/gomod"
rm -rf "$SCR" && echo "removed $SCR"
```

(Go marks its module cache read-only, hence the `chmod`.) Nothing else in `/tmp` belongs to this run: `xvfb-run` and the app used `TMPDIR=$SCR/tmp`.

- [ ] **Step 12: Report**

Write `.planning/builder-terminal/E2E-REPORT.md`: the scratch root, launch command lines, PIDs and session ids (and that each session id equalled its launch PID); the preflight for both launches; one line per check (S1, S2, S3 including the rapid pair and whether the layout race reproduced, S4 including Ctrl+W and the held Alt+W, S5, S6 a/b/c, S7, S8, S9, S10) with PASS/FAIL and the evidence; the isolation proof (final fd scan, scratch-profile evidence, user-files diff with each changed path explained, journal); the cleanup record; the substitutions (no window manager, native Switch App menu with the `markSwitching` note, and the emain markers standing in for `WAVESRV-ESTART`). Do not commit it; the reviewer decides.

---

## Spec coverage

### Design bullets

| Spec item | Task(s) | Pinned by |
|---|---|---|
| D1 Create (`CreateBuilderTab`, meta at insert, no workspace) | 2 | `TestCreateBuilderTab` |
| D1 Is-builder-tab predicate (owner + no workspace, errors = not builder, `tx.Context()`) | 1 | `TestIsBuilderTab`, `TestUpdateObjectMetaConnectionOnMissingBlockIsNotFound` |
| D1 Delete (missing no-op, refusals, per-block errors keep tab, re-read in the final transaction) | 2 | `TestDeleteBuilderTab*`, incl. `TestDeleteBuilderTabKeepsTabThatGainedABlock` |
| D1 Find (SQL on owner, any-owner, predicate filter) | 1, 2 | `TestDBFindTabIdsByBuilderOwner`, `TestFindBuilderTabs` |
| D1 Cascade skip; BlockClose right after delete | 3 | `TestDeleteBlockLastBlockOfBuilderTabKeepsTab`, `TestDeleteBlockPublishesBlockCloseWhenCascadeFails` |
| D1 Reserved `builder:` tab meta in `UpdateObjectMeta` | 1, 6 | `TestUpdateObjectMetaRejectsBuilderKeysOnTabs`, `TestReservedTabMetaThroughSetMetaAndObjectService` |
| D1 Workspace membership guard | 3 | `TestUpdateWorkspaceTabIdsRejectsBuilderTab` (deviation 2) |
| D1 Local only (`UpdateObjectMeta`, `CreateBlock`, `CreateSubBlock`, patch-only) | 1, 3, 6 | `TestUpdateObjectMetaLocalOnlyForBuilderBlocks`, `TestCreateBlockRejectsRemoteConnectionInBuilderTab`, `TestBuilderBlocksStayLocalThroughRpcs` (deviation 1) |
| D1 Workspace env (`WORKSPACEID` omitted) | 4 | `TestAddTabAndWorkspaceEnvOmitsEmptyWorkspaceId` |
| D1 Block def pins `term:durable` false | 4 | `TestMakeBuilderTerminalBlockDefPinsDurableOff` |
| D2 Caller check (pure function, UUID, routes) + test helper | 5, 6, 7, 8 | `TestCheckBuilderCaller`, `TestMakeRpcSourceContextForTest`, `Test*RejectsNonElectronCallers`, `TestDeleteBuilderCommandAcceptsElectronAndOwnRendererOnly` |
| D2 Keyed lock, entries kept, `ctx.Err()` after acquire | 5, 6, 7, 8 | `TestBuilderLockIsPerBuilder`, `TestWithBuilderLockSerialises`, `TestEnsureBuilderTabWaitsForBuilderLock`, `TestOpenBuilderTerminalWaitsForBuilderLock`, `TestDeleteBuilderCommandWaitsForBuilderLock`, `TestEnsureBuilderTabReturnsWhenRpcContextIsDone`, `TestBuilderLockSerialisesDeleteAndEnsures` (row ledger) |
| D2 Detached 15 s write context, one goroutine, broadcast once | 5, 6, 7, 8 | `TestMakeBuilderWriteContextIsDetachedAndTracksUpdates`, `Test*BroadcastsUpdates`/`*LayoutUpdate`/`*TabDelete` |
| D2 `ResolveAppDirForAppId` | 4 | `TestResolveAppDirForAppId*` |
| D2 `EnsureBuilderTabCommand` | 6 | `TestEnsureBuilderTab*` |
| D2 `OpenBuilderTerminalCommand` (actions, not-ready, tab app dir, direct child, cap, rollback) | 7 | `TestOpenBuilderTerminal*` |
| D2 `DeleteBuilderCommand` teardown (detached, under lock, controller kept) | 8 | `TestDeleteBuilderCommand*` |
| D3 Startup sweep (order, per-tab errors, log line) | 2, 9 | `TestSweepBuilderTabsRemovesBuilderTabsOnly`, `TestBuilderSweepRunsBetweenControllerInitAndReconnect` |
| D4 `ensure-builder-tab` IPC (window's own ids; gating confirmed) | 10 | tsc; E2E Step 5 |
| D4 `switchBuilderApp` awaits `setBuilderWindowAppId(null)`; a failed switch offers Retry | 14 | both tests in `builder-apppanel-switch.test.ts` |
| D4 `open-builder-terminal` (no main window, 64-char strings) | 7, 10 | `parseBuilderTerminalTarget` tests; E2E Step 6 |
| D4 `destroyBuilderWindow` order (hidden at once, single teardown); `closed` fallback kept | 10 | `runBuilderTeardown` tests incl. the concurrent-calls test; E2E Step 9 |
| D4 One switch path; modal unchanged | 14 | confirmed in "Spec items confirmed" |
| D5 Subscriptions in `initBuilder` (no `userinput`; one `config` subscription) | 11 | `global-builder-subs.test.ts`, `builder-apppanel-subs.test.ts` |
| D5 `staticTabIdAtom` writable, only builder writes; `uiContext` live | 11, 14 | `uiContext` tests, `static-tab-writers.test.ts` |
| D5 Bootstrap steps 1-4 and ordering | 14, 15 | `pins the tab and its layout before setting the static tab...`, `builder-termcontents.test.tsx` |
| D5 Null-tolerant layout-model callers | 11 | `keymodel-builder.test.ts`, `layoutModelHooks.test.ts`, `global-builder-subs.test.ts`, `focusManager.test.ts` |
| D5 Tab vanishes; switching flag | 14, 15 | `drops the layout model and offers a reload...`, `shows switching...`, panel tests |
| D5 Layout (horizontal split, default 40, min 20) | 14, 15 | `builder-layout.test.ts`; E2E Step 5 |
| D5 Empty state | 15 | `shows the empty state over the layout...` |
| D5 Header button disabled until Ensure (stricter: only while ready, deviation 12) | 14, 15 | `ensureOkAtom` assertions in the model tests, `builder-appheader.test.tsx` |
| D5 Reload keeps panes | 6, 14 | `TestEnsureBuilderTabIsIdempotent`; E2E Step 5 |
| D5 Preview partition | 15 | `getBuilderPreviewPartition` test; existing `keeps an explicit partition` (`emain/emain-websecurity.test.ts:169-173`) |
| D6 Focus sides, initial focus, zero panes to app, borders | 14, 15 | `moves builder focus to the terminal...`, panel tests, `builder-workspace.test.tsx` |
| D6 Key table, webview key list | 16 | `builder-keys.test.ts`, `keymodel-builder-keys.test.ts` |
| D6 Close rule; keep-alive closed not hidden | 13, 16 | `closes the last pane ... never the tab`, `closes keep-alive blocks instead of hiding them` |
| D6 Central create intercept, `replaceBlock` no-op, return `null` | 13 | `global-builder-create.test.ts` |
| D6 Split focus and stale-target insert | 12 | `backendsplit.test.ts` |
| D6 Connections hidden / `Cmd:g` unbound | 13, 16 | `showConnectionUi` test, unbound-keys test |
| D7 Ensure fails: error + Retry | 14, 15 | `shows an Ensure error and retries in place`, panel Retry test |
| D7 Open fails: notice, no rows | 7, 13 | `Test*WithoutWrites`, `shows an Open error as the builder notice` |
| D7 Shell exits | 8 | `TestDeleteBlockCommandOnLastBuilderPaneKeepsTab` |
| D7 Close/switch kills shells | 8 | `TestDeleteBuilderCommandRemovesEveryOwnerTab`; E2E Step 9 |
| D7 Delete never completes / late Ensure | 2, 9 | sweep tests; E2E Step 10 |

### Success criteria

| Criterion | Task(s) | Unit evidence | E2E (Task 18) |
|---|---|---|---|
| S1 first open: one local shell, focused, `pwd` = app dir | 6, 14, 15 | `TestEnsureBuilderTabCreatesOneTerminal`, bootstrap tests | Step 5 |
| S2 Open terminal adds a focused pane in the app dir, main windows untouched | 7, 13, 15 | `TestOpenBuilderTerminalAppends` | Step 6 |
| S3 keys, header and menu splits; two rapid splits | 7, 12, 13, 16 | intercept and key-table tests, `backendsplit.test.ts` | Step 6 |
| S4 `Cmd:w` closes panes incl. last; empty state; app focus; window stays; Ctrl+W unbound | 3, 8, 14, 16 | cascade-skip, close-rule, key-table tests | Step 7 |
| S5 terminal width persists via `builder:layout.terminal` | 14, 15 | `builder-layout.test.ts` | Step 5 |
| S6 close, Alt+W from app side and app switch delete tab, blocks, layout; shells die | 8, 10, 14 | `TestDeleteBuilderCommand*`, teardown-order tests | Step 9 |
| S7 startup sweep logged before the server ready signal (emain markers stand in for `WAVESRV-ESTART`) | 2, 9 | sweep tests, AST order test | Step 10 |
| S8 builder tabs never in `tabids`, tab bar, switcher | 2, 3 | `TestCreateBuilderTab`, `TestUpdateWorkspaceTabIdsRejectsBuilderTab` | Step 6 |
| S9 two writers in one debounce window, one `building` transition | (existing debounce, `pkg/buildercontroller/appwatcher.go:34`) | none new | Step 8 |
| S10 workspace tab is never deleted by cleanup or sweep | 1, 2, 3 | `TestIsBuilderTab`, refusal and sweep tests | Step 10 |

### Spec Testing section

| Spec test bullet | Test |
|---|---|
| Go: CreateBuilderTab | `TestCreateBuilderTab` |
| Go: DeleteBuilderTab (BlockClose, refusals, idempotent, failing block then sweep) | `TestDeleteBuilderTabDeletesBlocksLayoutAndTab`, `TestDeleteBuilderTabRefusesWorkspaceTabAndOtherOwner`, `TestDeleteBuilderTabKeepsTabWhenABlockFails` |
| Go: caller check, zero rows | `TestCheckBuilderCaller`, `TestEnsureBuilderTabRejectsNonElectronCallers`, `TestOpenBuilderTerminalRejectsNonElectronCallers`, `TestDeleteBuilderCommandAcceptsElectronAndOwnRendererOnly` |
| Go: broadcast | `TestEnsureBuilderTabBroadcastsUpdates`, `TestOpenBuilderTerminalBroadcastsLayoutUpdate`, `TestDeleteBuilderCommandBroadcastsTabDelete` |
| Go: FindBuilderTabs | `TestFindBuilderTabs` |
| Go: DeleteBlock on last builder block | `TestDeleteBlockLastBlockOfBuilderTabKeepsTab` |
| Go: UpdateObjectMeta reserved keys via SetMeta and ObjectService | `TestUpdateObjectMetaRejectsBuilderKeysOnTabs`, `TestReservedTabMetaThroughSetMetaAndObjectService` |
| Go: BlockClose despite cascade error | `TestDeleteBlockPublishesBlockCloseWhenCascadeFails` |
| Go: UpdateWorkspaceTabIds | `TestUpdateWorkspaceTabIdsRejectsBuilderTab` |
| Go: non-local connection via SetMeta, CreateBlock, CreateSubBlock | `TestBuilderBlocksStayLocalThroughRpcs` |
| Go: Ensure cases | `TestEnsureBuilderTab*` (11 tests) |
| Go: Open cases | `TestOpenBuilderTerminal*` (12 tests) |
| Go: DeleteBuilderCommand regardless of rtinfo; cancelled caller | `TestDeleteBuilderCommandRemovesEveryOwnerTab`, `TestDeleteBuilderCommandCompletesWhenCallerContextIsCancelled` |
| Go: keyed lock under `-race` | `TestBuilderLockSerialisesDeleteAndEnsures` with its row ledger, and the three `*WaitsForBuilderLock` tests (Task 17 runs `-race`) |
| Go: sweep | `TestSweepBuilderTabsRemovesBuilderTabsOnly`, `TestBuilderSweepRunsBetweenControllerInitAndReconnect` |
| Go: makeSwapToken | `TestAddTabAndWorkspaceEnvOmitsEmptyWorkspaceId` |
| Vitest: layout default/merge | `builder-layout.test.ts` |
| Vitest: focus switching incl. zero panes | `moves builder focus to the terminal at the first pane...`, panel focus test |
| Vitest: key routing by focus, unbound keys, webview list | `builder-keys.test.ts`, `keymodel-builder-keys.test.ts` |
| Vitest: last-pane close uses `closeNode` | `closes the last pane from its header through closeNode, never closeTab` |
| Vitest: `onNodeDelete` wired | `builder-termcontents.test.tsx` |
| Vitest: create intercept | `global-builder-create.test.ts` |
| Vitest: bootstrap order; no layout model before step 3 | `pins the tab and its layout before setting the static tab, and builds no layout model` |
| Vitest: appid mismatch | `mounts nothing when Electron's app id differs...`, panel mismatch test |
| Vitest: header disabled before Ensure | `ensureOkAtom` assertions (Task 14), `builder-appheader.test.tsx` (Task 15) |
| Vitest: tab vanished via delete update | `drops the layout model and offers a reload when the tab is deleted` |
| Vitest: split handlers honour `focused`, insert fallback | `backendsplit.test.ts` |
| Vitest: main windows never write `staticTabIdAtom` | `static-tab-writers.test.ts` |
| tsc, go test, tsunami list, full vitest, `-race` | Task 17 |
| E2E checks | Task 18 |
