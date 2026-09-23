# code-auditor report (round 3)
**Target:** `git diff bab19e28 4d744a2f` on `qa/fleet-2026-09-22` (second fix wave)
**Started:** 2026-09-22
**Status:** COMPLETE

## Findings
<!-- appended one at a time, as found -->

### R3-CA-1 [Medium] R2-CA-6 fix is a no-op: spin click after typing still saves old value + step
- **Location:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1078-1080` (render-phase resync) vs `:1093-1098` (`bump`), tree `4d744a2f`; commit `14d9489d`.
- **Issue:** mousedown on the arrow blurs the input, `commit()` sets `dirty=false`, and React flushes that discrete update before mouseup. On that render `!dirty && local !== String(value)` resets `local` to the stale `value`, so `bump`'s closure sees `local === String(value)` and `bumpNumberInput(local, value, …)` is identical to the old `value + delta`. The unit tests only exercise the pure helper, never the component.
- **Evidence:** verbatim `NumberControl` source bundled with esbuild, driven in headless Chromium via CDP real mouse events (type "50" over 10, press arrow, release after 100 ms; parent applies `onChange` after a simulated round-trip). (Harness was in the scratch worktree, since removed; rebuild by bundling lines 1053-1145 of each tree with a parent that applies `onChange` after a delay.)
  - `4d744a2f`: round-trip 5 ms -> saves `[50,51]`; 30 ms -> `[50,51]`; 200 ms -> `[50,11]`, input shows 11.
  - `bab19e28` (pre-fix): identical results for all three.
  The fix only "works" when the config round-trip beats the button press, which is exactly when the old code already worked. The commit message claim ("the same sequence saves 51") is false whenever the round-trip is slower than the press.
- **Also:** the two writes (`50` then `11`/`51`) are separate `SetConfigCommand` RPCs; `pkg/wshutil/wshrpc.go:434` runs each request on its own goroutine and `configWriteLock` (`pkg/rtconfig/settingsconfig.go:717-719`) only serialises each read-modify-write, not arrival order, so the stored value can end as the typed one while the field shows the stepped one until the next config event.
- **Fix:** keep the committed draft authoritative until `value` catches up, e.g. hold a `pendingRef` set in `commit()` and have the render-phase resync skip while `pendingRef.current !== value`, and base `bump` on `pendingRef.current ?? value`. Better still, cancel the blur commit when focus moves to the component's own spin button (`onBlur` checking `e.relatedTarget` inside the control) so only one write is sent, which also removes the ordering race. Add a component-level test (a DOM env is not installed; the CDP harness above is one option).

### R3-CA-2 [Medium] Stale SingletonLock with a reused pid blocks the new build with no escape
- **Location:** `emain/emain-platform.ts:183-191` (`checkLegacyInstanceRunning`), `:213-220` (Quit-only dialog), tree `4d744a2f`.
- **Issue:** a lock is stale exactly when the legacy app died uncleanly (crash, power loss, SIGKILL). After a reboot, low pids are reassigned, so the recorded pid is commonly alive as an unrelated process. `process.kill(pid, 0)` then succeeds (same uid) or throws `EPERM` (another uid, e.g. a root daemon); both fall through to `confirmedRunning: true` (EPERM is deliberately tested, `emain/emain-platform.test.ts:334`). The dialog offers only "Quit" and its detail omits `block.reason`, so the user is told to quit a WaveTerm that is not running, on every launch, until that unrelated process exits. Nothing is lost, but the app is unusable and the user has no way to find the lock file from the UI.
- **Fix:** confirm the pid is the legacy Electron before treating it as live: on Linux compare `readlinkSync(/proc/<pid>/exe)` (or `/proc/<pid>/cmdline`) against a WaveTerm/Electron binary; on macOS, or when that check is inconclusive, and for `EPERM`, return `confirmedRunning: false` so the Quit / Migrate anyway dialog applies. Include `block.reason` (lock path and pid) in the confirmed-running dialog's detail. Chromium's own ProcessSingleton does the equivalent executable check before honouring a lock (UNVERIFIED: from memory of `process_singleton_posix.cc` `IsChromeProcess`, not re-read here).

### R3-CA-3 [Medium] macOS: data-root merge never applies after a blocked launch; permanent abort dialog
- **Location:** `emain/emain-platform.ts:113` (`EmainDataEntryNames`), `:440` (data `canMergeIntoDest`), `emain/emain.ts:265` (`requestSingleInstanceLock` before the block check), tree `4d744a2f`.
- **Issue:** on macOS the data destination is `envPaths("remoteterm").data` = `~/Library/Application Support/remoteterm` (`node_modules/env-paths/index.js:13`), which is also the parent of Electron userData (`app.setName("remoteterm/electron")`, `emain-platform.ts:24,27`). A launch blocked by a running WaveTerm reaches `requestSingleInstanceLock()` (creates `electron/SingletonLock`) and emain-log before quitting, so the next launch sees `[electron, logs, rtapp.log]`; `electron` is not in the allowlist, so the data root aborts with "merge manually" on every launch. Commit `0b33097c` states the lock check covers "Linux and macOS" and that the launch after WaveTerm quits "completes instead of aborting"; that holds on Linux only (Linux userData sits under the config root, which always merges). Not exercised: the tests run with Linux paths.
- **Fix:** on darwin, also allow `electron` in the data-root allowlist (it is already in `UnmergeableDirNames`, so the legacy profile stays put and the new one is kept). The DB guard is unaffected: the server always creates `db/` and `wave.lock`. Add a darwin-path test.

### R3-CA-4 [Low] A data-root merge that fails part-way is never resumed
- **Location:** `emain/emain-platform.ts:358-372` (`mergeDataRoot`), `:440`, tree `4d744a2f`.
- **Issue:** on a throw inside `mergeTree` the marker is correctly not written and nothing is overwritten or deleted, so there is no data loss. But the entries already moved (e.g. `db/`) make the next launch's allowlist check fail, so the data root switches to the permanent "merge manually" abort while the server runs against the half-merged destination. The config root re-merges and converges. A realistic trigger is new to the merge path: renaming a directory to a *different* parent needs write permission on that directory itself (to update `..`), so a root-owned subdir (e.g. left by a `sudo` run) fails here though the whole-root rename (same parent) succeeded.
- **Fix:** let the data root resume a merge it started: write an in-progress marker (e.g. `.migrating-from-waveterm` holding the source) into the destination before the first rename, and treat its presence as `canMergeIntoDest` true; remove it when the completion marker is written.

### R3-CA-5 [Low] Staged-hunk revert's partial-success error is only logged
- **Location:** `pkg/wshrpc/wshremote/git.go:343-356`, `frontend/app/view/sourcecontrol/sourcecontrol-model.ts:387-400`, tree `4d744a2f`.
- **Issue:** the Go side is right: if the working tree rejects the reverse patch, the hunk is unstaged and the re-edited lines survive as an unstaged change, which is what a user would expect from "revert" minus the conflicting part. But `revertHunk` catches the error with `console.error` only; the user sees the hunk move from the staged to the unstaged list with no message, and may think the revert succeeded. The model already has `errorAtom` (`:32`), used for status errors (`:264,270`).
- **Fix:** set a user-visible error in the catch ("Hunk unstaged; the working tree changed since staging and was left as is") and make sure the following `fetchStatus()` does not clear it (it resets `errorAtom` on success, `:264`).

### R3-CA-6 [Info] Dev builds are blocked by a running production WaveTerm
- **Location:** `emain/emain-platform.ts:143` (`LegacyElectronUserDataPath`), tree `4d744a2f`.
- **Issue:** the legacy app used `waveterm/electron` in dev and prod (`git show 456d97c6^:emain/emain-platform.ts:17`), so the check cannot tell which one holds the lock. A dev RemoteTerm migrating `waveterm-dev` roots is held back (Quit only) by a running prod WaveTerm whose roots it never touches. Developer-only; acceptable if documented in the comment at `:141-142`.
- **Fix:** mention it in that comment, or skip the confirmed-running hard block in dev and fall through to the Quit / Migrate anyway choice.

### R3-CA-7 [Info] After "Migrate anyway", the startup log line records the pre-move data dir
- **Location:** `emain/emain.ts:63,69-73`, tree `4d744a2f`.
- **Issue:** with a legacy combined home (`~/.waveterm` with `wave.lock`) held back, module-level `remoteTermDataDir` resolves to `~/.waveterm`; "Migrate anyway" then moves it to `~/.remoteterm`. The only use of the cached value is the startup log line, so it is misleading but harmless: the server env and cwd call `getRemoteTermDataDir()` afresh (`emain/emain-remotetermsrv.ts:71-76`), and winston's open fd follows the rename on POSIX (Windows never reaches this path).
- **Fix:** log the data/config dirs again after a Migrate anyway, or move that log line after `resolveLegacyInstanceBlock()`.

## Verified OK
<!-- appended as checked -->

- **R2-CA-1 ordering:** `resolveLegacyInstanceBlock` awaits `app.whenReady()` before any `showMessageBox` (`emain-platform.ts:208`), runs before `runRemoteTermSrv` (`emain.ts:274-287`), and quits with `setUserConfirmedQuit(true)`. No dialog before ready. The check is memoised and only reached for a root that actually needs migrating (marker/absent/skip checks come first, `:258-269`). Hostnames containing `-` parse correctly (`lastIndexOf`). ENOENT and ESRCH proceed.
- **R2-CA-1 "Migrate anyway" cached paths:** server env/cwd, launch settings reads and the `get-data-dir`/`get-config-dir` IPC call the getters afresh; only the emain.ts startup log line is stale (R3-CA-7). In the non-combined-home case the destination paths are identical before and after the move.
- **R2-CA-9 DB safety:** a data destination merges only if every top-level entry is `logs` or `rtapp.log`; the server always creates `db/` and `wave.lock` there, so a destination the server has used can never be merged into. Merge never overwrites (`existsSync` guard before `renameSync`), never deletes files, removes only emptied source subdirs, and writes the marker only after `mergeTree` returns. Symlinks: `Dirent.isDirectory()` is lstat-based, so source symlinks are moved as links, never followed; a symlinked destination dir would be recursed into via `statSync` (follows), which only matters if the user has placed one there.
- **R2-CA-9 config merge:** `canMergeIntoDest: () => true` is safe for config (no overwrite; `electron/` kept on the destination side, excluded from the user-facing report); re-runs converge after a partial failure.
- **R2-CA-2:** `secrets.enc` qualifies the config root (`emain-platform.ts:399-404`); message updated.
- **R2-CA-4 Go semantics:** unstage (`git apply -R --cached`) then working tree (`git apply -R`); partial state keeps the user's re-edit as an unstaged change. Correct; only the surfacing is missing (R3-CA-5).
- **R2-CA-3 / R2-CA-5:** `reorderWidget` reads neighbour orders inside the queued `persistWidgetPatch` step; `applyBackgroundToTab` clears then sets `errorMessageAtom` on RPC failure. Both match their commit messages.
- **R2-CONF-1:** electron-vite 5.0.0's asset plugin (`node_modules/electron-vite/dist/chunks/lib-q6ns0vZr.js:593-650`, `apply: 'build'`, which covers main in dev as well) emits the PNG into `dist/main/...` and exports `join(import.meta.dirname, <file>)`; `electron-builder.config.cjs:21-26` packages `dist/**`. electron-vite documents exactly this pattern for a packed icon (`nativeImage.createFromPath(icon?asset)` for a Tray, https://electron-vite.org/guide/assets). UNVERIFIED: Electron's own ASAR docs (https://www.electronjs.org/docs/latest/tutorial/asar-archives) do not name `nativeImage`/`BrowserWindow({icon})` among asar-capable APIs; not exercised against a packaged build here.
- **Colour fixes (daa44e2c):** tests assert `text-primary` on message text; `cn()` removal matches its Miscellanea note.
- **Tooling on `4d744a2f` (scratch worktree):** `npx vitest run emain/ frontend/app/view/remotetermconfig/` 78/78 pass; `go test ./pkg/wshrpc/wshremote/ ./pkg/rtconfig/` ok; `npx tsc --noEmit` has errors only in untouched files (`frontend/preview/previews/processviewer.preview.tsx`, `frontend/app/view/term/term.tsx:314`), none in changed files.

## Completion
**Status:** COMPLETE
**Tally:** Critical 0, High 0, Medium 3 (R3-CA-1, -2, -3), Low 2 (R3-CA-4, -5), Info 2 (R3-CA-6, -7).
**Not checked:** packaged-asar icon load (no packaging/launch allowed); macOS runtime (R3-CA-3 is from code and env-paths source).
