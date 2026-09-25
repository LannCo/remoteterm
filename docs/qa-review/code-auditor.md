# code-auditor report
**Target:** `qa/fleet-2026-09-22` vs `origin/main` (logic commits; rename ignored). Line numbers relative to HEAD `93aaefc3`.
**Started:** 2026-09-22
**Status:** IN PROGRESS

## What works
<!-- appended as checked -->

## Findings

### CA-1 [Medium] Widget write queue serialises writes but not the snapshot; same-widget toggle+drag reverts the toggle
- **Confidence:** high (code path), medium (user-visible frequency)
- **Location:** `frontend/app/view/remotetermconfig/remotetermconfig-model.ts:811-838` (snapshot), `:775-780` (queue)
- **Trigger:** toggle visibility on widget X, then drag/keyboard-move X (or click the toggle twice) before the config watcher pushes the new `fullConfig`.
- **Symptom:** `toggleWidgetHidden`/`reorderWidget` build the full replacement entry from `widgetsMapAtom` (merged fullConfig) at call time, before entering the queue. The second write spreads the stale widget (`display:hidden` = old value) and overwrites the first: the toggle is silently undone. Double-click on the switch writes `hidden=true` twice (both read `false`); `widgetscontent.tsx:241` also reads the confirmed value, so the second click is a no-op. The comment at `:767-770` claims the queue prevents exactly this toggle+drag race; the test at `remotetermconfig-model.widgets.test.ts:239` only covers two *different* widgets.
- **Fix:** compute the patch inside the queued step, from the freshly read raw file entry falling back to `widgetsMapAtom[key]` (e.g. pass a `(current) => next` updater into `writeWidgetPatch`). Same for `updateBackgroundOpacity`/`updateBackgroundBlendMode` (`:923-935`), which share the pattern (opacity then blend-mode on one preset reverts the opacity). Add a same-key test.

### CA-2 [Medium] General tab number fields write every keystroke, unclamped; clearing the field writes 0
- **Confidence:** high
- **Location:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1069-1072`
- **Trigger:** select-all + Backspace (or type a partial value) in any number field, e.g. `window:maxtabcachesize` (min 1), `term:scrollback`, `window:tilegapsize`.
- **Symptom:** `Number("")` is `0` and finite, so clearing the input immediately persists `0` via `SetConfigCommand`; every intermediate keystroke is also persisted, and `min`/`max` are only enforced in `bump()` (`:1052-1057`), never on typed input. `window:maxtabcachesize: 0` feeds `setMaxTabCacheSize` (`emain/emain-tabview.ts:238-241`) unguarded, evicting every cached tab view. `term:fontsize` is safe only because `term-model.ts:269` clamps.
- **Fix:** mirror `TextControl`: keep a local string state, commit on blur/Enter, reject empty, clamp to `[min,max]` before `onChange`.

### CA-3 [Low] Concurrent `SetConfigCommand` calls can lose a key; frontend comment asserts no queue is needed
- **Confidence:** high (code path), low (likelihood: needs two writes within one read+atomic-write window)
- **Location:** `pkg/rtconfig/settingsconfig.go:711-746` (unlocked read, lock only in `WriteWaveHomeConfigFile` `:439-441`); `pkg/wshutil/wshrpc.go:434-439` (each request in its own goroutine); claim at `remotetermconfig-model.ts:685-687` and `generalcontent.tsx:1151-1152`.
- **Trigger:** two `setGeneralSetting` calls in flight at once (CA-2 per-keystroke writes, fast +/- clicks, toggle then select).
- **Symptom:** read-A, read-B, write-A, write-B: B's map lacks A's change, so A is lost; same-key writes can also land out of order (final value an intermediate keystroke). Server code is pre-existing, but the branch's new visual tab is the first UI that issues rapid single-key writes and documents that no serialisation is needed.
- **Fix:** hold `configWriteLock` across the read-modify-write in `SetBaseConfigValue`/`SetConnectionsConfigValue` (helper with `defer`), or serialise `setGeneralSetting` through a queue like widgets. Server-side lock is the better fix; correct the comments.

### CA-4 [High] Data-dir migration can move a running legacy instance's live data, and the lock rename removes the only guard
- **Confidence:** medium-high (code path certain; needs old and new builds running concurrently)
- **Location:** `emain/emain-platform.ts:121-185,251` (migration, no liveness check); `pkg/remotetermbase/wavebase.go:134-135` (`WaveLockFile = "remoteterm.lock"`, `DomainSocketBaseName = "remoteterm.sock"`, renamed in `668a8eda`); `emain/emain-platform.ts:17` (`app.setName("remoteterm/electron")`, so `requestSingleInstanceLock` at `emain/emain.ts:264` no longer collides with the old app's)
- **Trigger:** a pre-rename (waveterm-path) instance is still running, e.g. the currently-running daily driver, and the renamed build is started (second window via launcher/keybind, or `task dev` from the new tree).
- **Symptom:** the shim `renameSync`s the old instance's data root (SQLite DB, filestore, `wave.lock`) out from under it. The old server's `flock` is on `wave.lock`; the new server locks `remoteterm.lock`, so both servers open the same `db/` concurrently. With the pre-rename name, the new server would have failed to take the lock and exited. Result: two writers on one SQLite DB, and the old instance's later path-based opens recreate an empty `~/.local/share/waveterm*`.
- **Fix:** before migrating a root, try to take a non-blocking exclusive lock on `<source>/wave.lock` (same flock semantics as `wavebase-posix.go:18`); if it is held, skip migration and show a "quit the running RemoteTerm/Wave instance first" dialog and exit. Keep checking the legacy lock name in the Go `AcquireWaveLock` path for one release (try-lock both `wave.lock` and `remoteterm.lock`).

### CA-5 [Medium] Dev builds now fall back to the prod legacy home `~/.waveterm`
- **Confidence:** high (code path), medium (requires a legacy combined-home install)
- **Location:** `emain/emain-platform.ts:298-306` (bare `LegacyRemoteTermHomeDirName` fallback, added in `d6666d49`); upstream `getWaveHomeDir` only ever checked `~/.waveterm-dev` in dev.
- **Trigger:** dev build (`isDev`), no `~/.remoteterm-dev`/`~/.waveterm-dev` with `wave.lock`, and `~/.waveterm/wave.lock` exists.
- **Symptom:** dev instance uses the production legacy home for config and data. Migration (`:219-228`) moves only the suffixed path, so this persists. Combined with CA-4's lock rename, a prod legacy instance and the dev build can both hold "their" lock on the same DB. Commit message frames this as a fallback for "a genuinely old, pre-suffix install", but dev builds never read that dir before.
- **Fix:** drop the bare fallback in dev (`if (!isDev) home = path.join(homeDir, LegacyRemoteTermHomeDirName)`), or remove it entirely since prod's suffixed name is already the bare name.

### CA-6 [Low] Config-root migration silently skipped when legacy config dir has no `settings.json`
- **Confidence:** medium
- **Location:** `emain/emain-platform.ts:205`, `:138-140`
- **Trigger:** `~/.config/waveterm*/` holds `connections.json`/`widgets.json`/`backgrounds.json`/secrets but no `settings.json`.
- **Symptom:** `validateSource` returns false, the function returns without recording a failure, `getRemoteTermConfigDir` then creates an empty new root and the user's connections/widgets vanish with no dialog. Next launch the dest exists, so even adding `settings.json` later hits the "not empty, merge manually" abort.
- **Fix:** validate on "directory contains any `*.json`" (or any entry) rather than `settings.json` specifically.

### CA-7 [Medium] Route-up stream restart can run `restartStreaming` concurrently with an in-flight `doReconnectJob`
- **Confidence:** medium (both paths proven; the concrete outcome depends on job-manager handling of two overlapping `JobPrepareConnect`s)
- **Location:** `pkg/jobcontroller/jobcontroller.go:573-593` (added in `91814977`); `:1998-2010` (`ReconnectJob` uses `reconnectConnGroup`, `ReconnectJobRoute` uses `reconnectRouteGroup`); `jobcontroller_test.go:312-363` asserts the two groups do *not* exclude each other.
- **Trigger:** a `ReconnectJob` (conn reconnect, `durableshellcontroller.go:184`, `blockcontroller.go:263`, `wshserver.go:1466`) runs while the job's previous output loop has exited (`jobStreamHealth.active=false`, e.g. after an earlier failed restart). Its `RemoteReconnectToJobManagerCommand` re-registers the job route, which publishes route-up.
- **Symptom:** `handleRouteEvent` sets the job Connected (`:563`), sees no active stream, and spawns `ReconnectJobRoute` -> `doReconnectJob`, whose `CheckJobConnected` now passes, so it calls `restartStreaming` (`:2032`). Meanwhile the original call returns from `WaitForRegister` (`:2115`) and calls `restartStreaming` (`:2128`). `restartStreaming` has no per-job lock: the second call closes the first's fresh reader (`:2288-2290`) and both send prepare/start to the job manager. If the manager's last-applied prepare belongs to the closed reader, the terminal is back in Connected-but-no-stream with health `active=true` for the other stream, so later reconnects skip (`:2021-2024`). This is the same class as the StartJob race fixed in `58bad6e2`, on the reconnect path. The comment at `:579-581` ("can't race a concurrent doReconnectJob") is contradicted by the test above.
- **Fix:** serialise `restartStreaming` per jobId (a keyed mutex, or run both entrypoints through one singleflight key for the restart step), or have `handleRouteEvent` skip the restart when a `reconnectConnGroup` call for that job is in flight. Add a test with both entrypoints racing on one job.

### CA-8 [Low] Connected-but-no-stream restart failure marks Disconnected without notifying the block
- **Confidence:** high
- **Location:** `pkg/jobcontroller/jobcontroller.go:2033-2036` (compare `:2129-2133`, which does send)
- **Trigger:** `restartStreaming` fails on the Connected-but-no-stream branch.
- **Symptom:** `SetJobConnStatus` only mutates the in-memory map (`:1435-1443`); without `sendBlockJobStatusEventByJob(ctx, job)` the frontend keeps showing Connected while the backend considers it Disconnected, until some other event refreshes it.
- **Fix:** add `sendBlockJobStatusEventByJob(ctx, job)` after `:2035`.

### CA-9 [Low] `StartJob` error paths after `registerNewJobStream` leak the reader and leave health `active=true`
- **Confidence:** high (path), low (impact)
- **Location:** `pkg/jobcontroller/jobcontroller.go:1610` then early returns at `:1617-1619` and `:1649-1661`
- **Trigger:** `MakeFile` or `RemoteStartJobCommand` fails.
- **Symptom:** the stream reader is never closed (broker entry leaks per failed start; pre-existing for `jobReaders`), and since `58bad6e2` `jobStreamHealth` also claims an active stream for a job with no output loop. The job is marked Done so reconnect is unlikely, but the health entry is wrong for anything that consults it.
- **Fix:** on those error returns, `reader.Close()` and delete the `jobStreamIds`/`jobReaders`/`jobStreamHealth` entries (small `unregisterJobStream` helper with defer-based cleanup).

### CA-10 [Low] `go test -race ./pkg/remote/conncontroller/` fails: new test reads `KeepAliveInFlight` unlocked
- **Confidence:** high (reproduced: `go test -race -count=1 ./pkg/remote/conncontroller/` -> FAIL, 2 races)
- **Location:** `pkg/remote/conncontroller/conncontroller_test.go:444-450` (added in `ace80192`) reads `cm.KeepAliveInFlight` while the `SendKeepAlive` goroutine clears it under `cm.lock` (`connmonitor.go:116-120,141`). Second race: `TestCloseInvoluntary_Hysteresis_SuppressesEventOnReconnect` (`:2435`) vs a leaked hysteresis goroutine from an earlier test (`conncontroller.go:471`) touching a package-level test hook reset at `:1878`.
- **Symptom:** production code is correctly locked; the tests are racy, so the package cannot be run under `-race` in CI, which hides real races in this concurrency-heavy package. Non-race run passes.
- **Fix:** read via a locked accessor (`cm.lock.Lock(); v := cm.KeepAliveInFlight; cm.lock.Unlock()` in a test helper) or make the field `atomic.Bool`; make the hysteresis goroutine exit before test cleanup (wait on a done channel) or guard the hook with a mutex.

### CA-11 [Medium] "Revert file" in review mode reverts only about half the hunks
- **Confidence:** high
- **Location:** `frontend/app/view/sourcecontrol/sourcecontrol-model.ts:627-639` (loop `for i = 0..hunkCount-1` -> `revertHunk(path, i, staged)`); server re-diffs on every call and indexes into the *current* hunk list: `pkg/wshrpc/wshremote/git.go:321-341`.
- **Trigger:** click revert on a file with >= 2 hunks in review mode (`review-mode.tsx:60`).
- **Symptom:** after reverting hunk 0, the old hunk 1 becomes index 0, so `i=1` reverts the original hunk 2, and so on; once `i` passes the shrinking count the server returns "hunk index out of range", which `revertHunk` swallows (`:395-397`). With 3 hunks, the middle hunk survives; with 2, the second survives. The UI then refreshes as if the revert completed. `518ef5b9` reworked this function (hunk count now from the possibly-stale cache) but kept the ascending loop; the unit test (`review-mode.test.ts:524-532`) mocks `revertHunk` and asserts indices `0, 1`, which enshrines the bug.
- **Fix:** iterate descending (`for (let i = hunkCount - 1; i >= 0; i--)`) so earlier indices stay valid, or better, add a whole-file revert RPC (`git checkout -- path` / `git restore [--staged] -- path`) and stop looping. Fix the test to drive a fake server that re-indexes.
- **Secondary (low confidence):** the commit message's claim that `changeDirectory()` covers every cwd change (so the cwd-prefixed key could go) is not strictly true: `cwd` also derives from block `cmd:cwd` meta (`:139-146,153-165`) without wiping `diffCacheAtom`. Because `getFocusedTerminalCwd` is read via untracked `globalStore.get`, in practice this is rare. The simpler correct fix would have been making `invalidateDiffCache` match the cwd-prefixed key.

### CA-12 [Low] Background quick-add applies the new key to the tab even when the write failed
- **Confidence:** high
- **Location:** `frontend/app/view/remotetermconfig/remotetermconfig-model.ts:937-963,979-996`
- **Trigger:** `writeBackgroundPatch` fails (read error, malformed `backgrounds.json`, write RPC error).
- **Symptom:** `persistBackgroundPatch` swallows the failure (error banner only), `addBackground` still returns the key, `submitBackgroundAdd` closes the form (typed name/CSS lost) and sets `tab:background` to a key that does not exist. `applyBackgroundToTab` rejections from tile clicks (`backgroundscontent.tsx:197,204`) are also unhandled.
- **Fix:** have `writeBackgroundPatch` return success (boolean) and propagate it; only close the form and apply on success; wrap `applyBackgroundToTab` in try/catch setting `errorMessageAtom`.

## Carried items verdicts
1. **`cmd/server/main-server.go:147-159` `os.Unsetenv` errors ignored: intentional, not a bug.** On Unix `syscall.Unsetenv` cannot fail for these constant, non-empty, `=`-free names; upstream (`origin/main` `main-server.go:145-151`) has the identical pattern. Minor: legacy `WAVETERM_VERSION` is no longer unset, but nothing reads either version var back (only writers at `shellutil.go:231`, `blockcontroller.go:616`), so harmless.
2. **`pkg/jobcontroller/jobcontroller.go:2414` `PanicHandler` return ignored: intentional.** Every goroutine-top `defer` in the package (`:430,456,506,586,626,1682,...`) and `wshrpc.go:436` ignores it; the return exists for RPC handlers that convert a panic into an error response. Nothing to propagate to in a detached goroutine.
3. **`pkg/remote/conncontroller/connmonitor.go:209` `SendKeepAlive()` return ignored: harmless.** `:132-150` always returns `nil` (failures are logged inside the goroutine and surface via the stall path). Info-level cleanup: drop the `error` return so the signature stops implying a checked failure.
4. **`frontend/app/view/preview/preview-streaming.tsx:36` unused `zoomIn/zoomOut/resetTransform`: not a missing-use bug.** `ImageZoomControls` (`:14-30`) gets the same functions from `useControls()`, which reads the `TransformWrapper` context, so the buttons work. Identical to upstream (branch diff on this file is the one-line `remotetermutil` import rename). Info-level cleanup: replace the render-prop with plain children.

## What works
- Widget/background raw-file reads correctly distinguish missing file (`{}`) from read/parse failure (`null` aborts the write), so a transient RPC failure cannot wipe a populated `widgets.json`/`backgrounds.json` (`remotetermconfig-model.ts:731-763,845-877`).
- Write queues continue after a rejected write (both `then` arms), so one failure does not wedge later writes.
- Raw-JSON vs visual tab: switching to Visual confirms/discards unsaved JSON edits (`remotetermconfig.tsx:250-253`), so visual writes refreshing `fileContentAtom` cannot silently clobber unsaved raw edits.
- `58bad6e2` correctly closes the StartJob route-up window; test `TestRegisterNewJobStreamSeedsHealthSynchronously` covers it.
- `91814977` correctly moves `SetJobConnStatus(Connected)` after a successful `restartStreaming`, and the deferred-log closure fix in `wshrouter.go` is right.
- JWT dual-write/fallback (`d6666d49`) covers the env and swap-token map paths.
- `go vet` clean on `cmd/wsh/...`, `cmd/server`, `pkg/jobcontroller`, `pkg/remote/...`, `pkg/rtconfig`; `go test` passes for jobcontroller and conncontroller (non-race); vitest 51/51 for `remotetermconfig` + `sourcecontrol`.

## Checked-clean areas
- `c41677c7`: every `(rtnErr error)` -> `error` demotion checked; the only bare `return` (`wshcmd-getmeta.go`) became `return nil`, equivalent since nothing assigned `rtnErr`.
- `55829c31`: `downloadFile` is synchronous with its own try/catch (`preview-model.tsx:928-938`), so dropping `fireAndForget` is correct; removed Go helpers had no callers (vet/test compile).
- `ace80192`: `getIntConfig` rejects `<=0`; ticker is effectively fixed at 1s; no remaining `degraded` consumers in frontend or Go.
- Config-path sentinel `WAVECONFIGPATH` consistent between `remotetermconfig.tsx:319` and `monaco/schemaendpoints.ts`.
- `ConnectionQuickAddRegex` vs Go `userHostRe`: TS is stricter (Go also allows `\` in the user part), so nothing the UI accepts is rejected server-side.
- Stale comments (Info): `remotetermconfig-model.ts:957` and `remotetermconfig-model.widgets.test.ts:70` cite `pkg/wconfig/defaultconfig/...`, now `pkg/rtconfig/defaultconfig/...`.

## Completion
**Status:** COMPLETE
**Tally:** High 1 (CA-4), Medium 5 (CA-1, CA-2, CA-5, CA-7, CA-11), Low 6 (CA-3, CA-6, CA-8, CA-9, CA-10, CA-12). Carried items: 4/4 judged not-a-bug (two Info cleanups).
**Not checked:** runtime behaviour (no app launch per brief). CA-4 and CA-7 are code-path proofs; their user-visible outcome (SQLite dual-writer effects, job-manager handling of overlapping prepare/start) was not reproduced. Electron's userData creation timing vs the config-root migration (possible "dest not empty" abort in prod Linux, since `~/.config/remoteterm/electron` sits inside the config dest) is UNVERIFIED and not listed as a finding.
