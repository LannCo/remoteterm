# round2-code-auditor report
**Target:** fix wave `35cd6146..75b60911` on `qa/fleet-2026-09-22` (CA-1..CA-12, TC-1)
**Started:** 2026-09-22
**Status:** COMPLETE

## Fix verification table
<!-- appended as checked -->

| ID | Commit | Defect gone? | Neighbour paths | Notes |
|---|---|---|---|---|
| CA-1 | 9f786c0b | Yes for toggle/opacity/blend (patch built inside the queued step from the fresh read, `remotetermconfig-model.ts:778-803,847-861,954-961`) | Queue still continues after failure; `null` patch aborts cleanly | Residual R2-CA-5 (reorder neighbour orders from snapshot) |
| CA-2 | 1559811c | Yes: empty rejected, clamped, commit on blur/Enter (`generalcontent.tsx:1053-1085`) | Unmount-without-blur loses the typed value, same as existing `TextControl` (`:1000-1035`); accepted as consistent | Residual R2-CA-6 (bump vs pending edit) |
| CA-3 | 27a9c131 | Yes: `configWriteLock` spans read-modify-write (`settingsconfig.go:717-776`); no nested lock; only caller of the unlocked writer holds the lock | Same-key RPC ordering still arbitrary (goroutine per request) | New tests fail on `35cd6146` (verified) |
| CA-4 | 282c9246 | Partly: dual-writer prevented (`wavebase.go:183-203`); migration still moves live dirs | POSIX fd closed on flock error (`wavebase-posix.go:21-24`); both locks released in reverse (`wavebase.go:172-180`); stale unlocked `wave.lock` is simply taken; not created in fresh dirs; `GOOS=windows go vet` clean; Windows `TryLock` failure leaks the mutex handle (pre-existing, process exits) | R2-CA-1 |
| CA-5 | 6563e25c | Yes: bare `~/.waveterm` fallback removed (`emain-platform.ts:313-336`); prod unchanged since suffixed == bare in prod | Override path semantics unchanged | |
| CA-6 | 7e781c9e | Mostly: any top-level `*.json` or `presets/` qualifies; skips now logged | Electron-only root correctly skipped | R2-CA-2 (`secrets.enc`), R2-CA-9 |
| CA-7 | ec0ab99c | Yes: both entrypoints go through `doReconnectJobExclusive`; `restartStreaming` has no other callers (`jobcontroller.go:2071,2168`) | No deadlock/lock-order issue found; lock held across bounded RPCs | R2-CA-7. New test fails on `35cd6146` ("2 restartStreaming calls in flight") |
| CA-8 | 9c212461 | Yes (`jobcontroller.go:2075`) | | Test fails on `35cd6146` (verified) |
| CA-9 | a117aff3 | Yes: deferred `unregisterJobStream` on every pre-loop return (`jobcontroller.go:1633-1638,1708`) | | R2-CA-8 (low confidence). Test fails on `35cd6146` |
| CA-10 | 79781c86 | Yes: `go test -race -count=5` clean for conncontroller, jobcontroller, rtconfig, remotetermbase | | |
| CA-11 FE | dfe1daa8 | Yes: descending loop (`sourcecontrol-model.ts:631-634`) | Hunk count still from `fetchDiffCached`; a stale cache with fewer hunks leaves the extras (pre-existing, round 1 noted) | R2-CA-10 |
| CA-11 Go | eb94ed5d | Yes: forward hunk + `git apply -R` (`git.go:339-343,1126-1135`). Scratch-repo checks passed for no-newline-at-EOF, adding a trailing newline, CRLF content | Staged path targets worktree only | R2-CA-4. Unequal-count tests fail on `35cd6146` ("corrupt patch") |
| CA-12 | 9f786c0b | Partly: add form stays open and nothing is applied on write failure (`remotetermconfig-model.ts:990-1030`) | | R2-CA-3 (`applyBackgroundToTab` unhandled) |
| TC-1 | 35891279 | Yes: asserts attempted/skipped sets and scheduler cleanup; no longer `t.Parallel` while mutating hooks | `resumeReconnectTestHook` (`jobcontroller.go:162,933-938`) follows the existing production test-hook pattern (`isConnectedTestHook`, `NeedsInteractiveAuthTestHook` in the same `var` block); nil in production, one branch. Acceptable | |

## New findings
<!-- appended one at a time, as found -->

### R2-CA-1 [Medium] CA-4 only half-fixed: migration still renames a live legacy instance's dirs; the guard fires afterwards
- **Tree:** `75b60911`. **Confidence:** high (code path).
- **Location:** `emain/emain-platform.ts:238-253` (data/legacy-home roots gated only by `lockFileSkipReason` = "`wave.lock` exists", `:192-194`), `:271` (`performDataDirMigration()` at module load); new guard `pkg/remotetermbase/wavebase.go:183-203`, called from `cmd/server/main-server.go:243`.
- **Symptom:** with a pre-rename instance running, the new build still `renameSync`s its data root (and config root) out from under it. The new `AcquireWaveLock` then sees the held `wave.lock` in the moved dir and the server exits (`main-server.go:245-246`), so the two-writer SQLite outcome is gone. What remains: the running old instance now lives in a renamed dir (its `wave.sock` path, config watcher and any path-based opens point at the old, now-missing path), and the new build fails to start after the damage is done. Round 1's fix asked for the try-lock *before* migrating and a "quit the running instance" dialog.
- **Fix:** in `lockFileSkipReason` (or a new pre-check in `migrateDataRoot`), probe whether `wave.lock` is held before moving. Node has no flock, but emain already spawns the Go server; a tiny `wsh`/server subcommand (or `flock -n` on Linux/macOS via `child_process.spawnSync` with an argv array) can report held/unheld. If held: skip all roots and show the dialog. Keep the Go-side guard as defence in depth.

### R2-CA-2 [Low] CA-6: a legacy config dir holding only `secrets.enc` is still skipped
- **Tree:** `75b60911`. **Confidence:** high.
- **Location:** `emain/emain-platform.ts:198-210` (`legacyConfigSkipReason` accepts only top-level `*.json` or `presets/`); secrets live in the config dir as `secrets.enc` (`pkg/secretstore/secretstore.go:26,80`).
- **Symptom:** a user whose only customisation is stored secrets has the config root skipped (now logged, still not migrated); the new build creates a fresh config root and the secrets are orphaned. Round-1 CA-6 named secrets explicitly.
- **Fix:** also accept `secrets.enc` (or: any entry other than `electron/`).

### R2-CA-3 [Low] CA-12 half-fixed: `applyBackgroundToTab` rejections are still unhandled
- **Tree:** `75b60911`. **Confidence:** high.
- **Location:** `frontend/app/view/remotetermconfig/remotetermconfig-model.ts:946-952` (no try/catch); callers `backgroundscontent.tsx:201,208` (tile `onClick`, promise dropped) and `remotetermconfig-model.ts:1030` (after the form has already closed).
- **Symptom:** a failing `SetMetaCommand` produces an unhandled promise rejection and no error banner; the quick-add path closes the form, then the apply fails silently. Round-1 CA-12 listed this as part of the fix.
- **Fix:** wrap the RPC in try/catch and set `errorMessageAtom`; return a boolean like the patch writers.

### R2-CA-4 [Low] CA-11 staged "revert" rewrites the working tree only; the staged change stays in the index
- **Tree:** `75b60911` (pre-existing at `35cd6146` `pkg/wshrpc/wshremote/git.go:326-341`, but only now reachable for unequal hunks). **Confidence:** high (reproduced in scratch repo).
- **Location:** `pkg/wshrpc/wshremote/git.go:327-343`: `Staged` selects `git diff --cached` but `applyPatchReverse` (`:1126-1135`) runs `git apply -R` against the worktree.
- **Symptom:** reverting a staged hunk (worktree == index) leaves `git diff --cached` unchanged and adds an equal-and-opposite unstaged change (verified: both `--cached --stat` and plain `--stat` show `1 +- 1` afterwards). If the worktree has further edits on that hunk, `git apply -R` fails ("patch does not apply") and `revertHunk` swallows it (`sourcecontrol-model.ts:395-397`). Review-mode "Revert file" passes `file.staged` (`review-mode.tsx:60`), so staged files hit this.
- **Fix:** for `Staged`, run `git apply -R --index` (reverts index and worktree together, fails cleanly if they diverge) or `git apply -R --cached` followed by the worktree apply; surface the error to the UI instead of `console.error`.

### R2-CA-5 [Low] CA-1 residual: `reorderWidget` still computes `newOrder` from the enqueue-time snapshot
- **Tree:** `75b60911`. **Confidence:** medium.
- **Location:** `frontend/app/view/remotetermconfig/remotetermconfig-model.ts:825-846`: `prevOrder`/`nextOrder`/`newOrder` come from `widgetsMapAtom` and the caller's `orderedKeys` before the queue; only the base widget object is re-read inside the step.
- **Symptom:** two moves landing within one config-watcher round-trip (keyboard up-up on one widget, or moving B next to A right after moving A) compute from stale orders: the second press repeats the first order (lost move) or places B relative to A's pre-move order. No data loss; the fix's toggle+drag case is correct.
- **Fix:** move the neighbour lookup into the `makeUpdates` callback, reading `display:order` via `getLatestWidget(raw, k)` for the neighbours.

### R2-CA-6 [Low] CA-2: clicking +/- while the field has an uncommitted typed value writes stale+step over it
- **Tree:** `75b60911`. **Confidence:** medium (standard blur-before-click ordering; not run).
- **Location:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1087-1090` (`bump` uses the `value` prop), `:1075-1085` (`commit` on blur).
- **Symptom:** type `50` in a field showing `10`, click "+": mousedown blurs the input, `commit` sends `50`, then `bump` sends `11` (prop still `10` until the watcher round-trip). Both `SetConfigCommand` RPCs run in separate goroutines (`pkg/wshutil/wshrpc.go:434-439`), so the server lock (CA-3) serialises them in arbitrary order: result is `11` or `50`. Same stale-`value` issue makes two fast "+" clicks increment once (pre-existing).
- **Fix:** have `bump` start from `parseNumberInput(local, ...) ?? value` and keep `local` as the pending value (set `local` to the bumped value) so consecutive bumps accumulate.

### R2-CA-7 [Info] CA-7 lock: wait is not ctx-aware and entries are never pruned
- **Tree:** `75b60911`. **Confidence:** high (code), impact low.
- **Location:** `pkg/jobcontroller/jobcontroller.go:2044-2049` (`lock.Lock()` ignores `ctx`); `:155` map never deleted from (`DeleteJob` `:2529-2537` does not touch it).
- **Symptom:** a waiter (e.g. the user-initiated `JobControllerReconnectJobCommand`, `wshserver.go:1466`) blocks for the holder's full duration, bounded by the holder's RPC timeouts (5s reconnect + 2s register + 1s loop-exit + 5s prepare + ...), then runs with a possibly expired ctx and fails at the first DB call. One `*sync.Mutex` leaks per job ever reconnected. No deadlock found: every reconnect entrypoint is on its own goroutine (`:596-603`, `:658`, `blockcontroller.go:252-263`), and nothing under the lock re-enters `ReconnectJob*`.
- **Fix:** optional: a 1-slot channel per job with `select { case ch <- struct{}{}: case <-ctx.Done(): return ctx.Err() }`; delete the entry in `DeleteJob`.

### R2-CA-8 [Info, low confidence] CA-9 cleanup deletes by jobId, not by the stream it registered
- **Tree:** `75b60911`. **Location:** `pkg/jobcontroller/jobcontroller.go:1525-1530` (`unregisterJobStream`), deferred at `:1633-1638`.
- **Symptom:** if a `restartStreaming` for the same job replaced the registration while `RemoteStartJobCommand` was in flight (30s timeout, `:1667`) and the start then returned an error, the defer deletes the *new* stream's `jobStreamIds`/`jobReaders`/`jobStreamHealth` entries. The job is marked Done on that path, so impact is cosmetic. UNVERIFIED that the interleaving is reachable (needs a route-up during a start that later reports failure).
- **Fix:** delete only if `jobStreamIds.Get(jobId) == streamMeta.Id`.

### R2-CA-9 [Info] CA-6 now surfaces a sticky "merge manually" failure for users who already ran the pre-fix build
- **Tree:** `75b60911`. **Location:** `emain/emain-platform.ts:146-165`.
- **Symptom:** a legacy config root that the old `settings.json` check skipped now qualifies; if the new build already created `~/.config/remoteterm*`, every launch records the non-empty-destination abort and shows it. This is the intended surfacing, but there is no way to dismiss it short of a manual merge. Noting for whoever owns the migration UX.

### R2-CA-10 [Info] CA-11 tests: the Go descending test cannot fail on the old code; the FE test still mocks the server
- `pkg/wshrpc/wshremote/git_revert_test.go:78-97` passes at `35cd6146` (run in a scratch export), because the ascending-index bug lived in TS. The FE guard (`review-mode.test.ts:503-537`) asserts call order on a mocked `revertHunk`, not the outcome against a re-indexing fake as round 1 suggested. Adequate as a regression pin; not an end-to-end check.

## Commit-message claims
<!-- appended as checked -->
Failure paths cited in tree `35cd6146`.

| Commit | Claim | Verdict | Evidence at `35cd6146` |
|---|---|---|---|
| 282c9246 | Renamed lock let a new server share the DB with a pre-rename server in a migrated dir | Accurate. Title/body do not claim the migration itself is now safe (see R2-CA-1) | `pkg/remotetermbase/wavebase.go:134`; `emain/emain-platform.ts:216,227` (gate on `wave.lock` existence only) |
| 9f786c0b | Toggle then drag (and opacity then blend) lost the first edit; failed add no longer applied | Accurate | `remotetermconfig-model.ts:831,838` (patch from snapshot before queue); `:992` form closed unconditionally |
| 6563e25c | "a dev build with no ~/.waveterm-dev ran against the production data and config" | Slight compression: also needs no `~/.remoteterm-dev/wave.lock` and a `wave.lock` in `~/.waveterm`. "Prod unchanged" is accurate | `emain/emain-platform.ts:302` then `wave.lock` check below it |
| 1559811c | Clearing saved 0; every keystroke saved; out-of-range not clamped | Accurate | `generalcontent.tsx:1070` (`Number("")` = 0, finite) |
| 9c212461 | Block kept showing "connected" | Accurate; test fails on old tree with `last published status = "connected", stored status = "disconnected"` | `jobcontroller.go:2032-2036` |
| 7e781c9e | "users with only connections.json, widgets.json or presets lost them, and the skip was silent" | Mild overstatement: data was orphaned in the legacy dir, not deleted (the function returns before any move). Otherwise accurate | `emain/emain-platform.ts:138,205` |
| 27a9c131 | Concurrent writers dropped keys; tests failed before the fix | Accurate; re-run on scratch export: both tests FAIL at iteration 0 | `settingsconfig.go:711-746,746,765` |
| ec0ab99c | Two prepares in flight at once for one job | Accurate; test fails on old tree | `jobcontroller.go:2000,2007,2032,2128` |
| dfe1daa8 | Ascending loop skipped hunks | Accurate | `sourcecontrol-model.ts:631` |
| eb94ed5d | Inverted patch kept forward header, "corrupt patch" on unequal counts | Accurate; reproduced ("corrupt patch at line 13") | `pkg/wshrpc/wshremote/git.go:340,1092` |
| a117aff3 | Reader stayed open, health active after failed start | Accurate; test fails on old tree | `jobcontroller.go:1610,1618,1660` |
| 79781c86 | Races were test-side | Consistent with round 1; post-fix `-race -count=5` clean | n/a (test-only) |
| 35891279 | Old smoke test asserted nothing and mutated a hook under `t.Parallel()` | Accurate per `coverage-analyst.md:72-75` | `jobcontroller_test.go:507-531` |

## Confirmed
<!-- appended as checked -->
- `go vet` clean on all touched Go packages, plus `GOOS=windows go vet ./pkg/remotetermbase/`.
- `go test -race -count=5` passes for `pkg/remote/conncontroller`, `pkg/jobcontroller`, `pkg/rtconfig`, `pkg/remotetermbase`; `-race -count=1` also passes for `pkg/wshrpc/wshremote`, `cmd/server`, `pkg/util/shellutil`.
- `npx vitest run` over `remotetermconfig`, `sourcecontrol`, `emain-platform.test.ts`: 7 files, 96 tests pass.
- `npx tsc --noEmit`: 18 errors, all in untouched files (`frontend/preview/**`, `term.tsx`); none in the fix diff.
- Old-tree failure of the new tests was checked by exporting `35cd6146` into the scratchpad and overlaying the three new Go test files: CA-3, CA-7, CA-8, CA-9 and the CA-11 unequal-count tests all fail there and pass at HEAD.
- a11y commits (`3d3f5f05`, `87056ec3`, `efe415fe`): hooks sit above the early return in `SecretDetailView`; the stable callback ref only re-runs on mount/unmount and clears `model.secretValueRef` on unmount. No logic regressions.
- `e7a30ef8` touches only binary icons under `build/`.

## Completion
**Status:** COMPLETE
**Tally:** Medium 1 (R2-CA-1), Low 5 (R2-CA-2..6), Info 4 (R2-CA-7..10). Fixes fully verified: CA-2, CA-3, CA-5, CA-7, CA-8, CA-9, CA-10, CA-11 (both halves, with R2-CA-4 as a pre-existing staged-path gap), TC-1. Partial: CA-1 (R2-CA-5), CA-4 (R2-CA-1), CA-6 (R2-CA-2), CA-12 (R2-CA-3).
**Not checked:** no app launch (per brief), so R2-CA-6's blur/click ordering and the post-migration behaviour of a live old instance (R2-CA-1) are code-path reasoning. Windows lock behaviour checked by vet and reading only.
