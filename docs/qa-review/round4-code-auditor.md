# code-auditor report (round 4)
**Target:** `git diff 816fb3d8 cd464aff` on `qa/fleet-2026-09-22` (third fix wave)
**Started:** 2026-09-22 (UTC)
**Status:** IN PROGRESS

## Findings
<!-- appended one at a time, as found -->

### R4-CA-1 [Medium] NumberControl shows a stale value permanently when the stored value moves past its pending write
- **Location:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1083-1088` (tree `cd464aff`)
- **Issue:** `pending` is cleared only when `value === pending` (1083) or when the write fails (1100-1102). While `pending != null` the render-phase resync (1086) is suppressed. The config watcher re-reads the whole file when each fsnotify event fires (`pkg/rtconfig/filewatcher.go:169-173`, `ReadFullConfig()`), so if any other write to the same key lands between our write and the watcher's read, `value` jumps straight past `pending`, `pending` is never matched, and the field keeps showing the control's own last write for as long as it stays mounted.
- **Evidence:** scratch happy-dom test driving the verbatim `GeneralContent` (userEvent clicks), stub model whose events re-read a shared "file" after 10 ms. `window:zoom` = 1, click Increase (write 1.05, RPC 30 ms), external write of 2 at 32 ms, wait 300 ms: `cd464aff` -> file `2`, field `1.05`; `816fb3d8` -> file `2`, field `2`. Same with the control's own Reset button (see R4-CA-2): Increase, then Reset 25 ms later, event lag 40 ms: `cd464aff` field `1.05`, stored unset (default 0.25); `816fb3d8` field `0.25`. Regression introduced by `b7221ec6`.
- **Impact:** the settings UI displays a value that is not in effect; the next spin click then steps from the stale `pending` (1127), writing a value derived from it and silently overwriting the other writer. Triggers: the field's own Reset button, another window's config page, the raw JSON editor, `wsh setconfig`.
- **Fix:** stop treating "prop equals pending" as the only acknowledgement. E.g. on a successful resolve of the *last queued* write, record the prop value seen then; clear `pending` when `value` equals `pending` or changes from that snapshot. Add the two scenarios above as regression tests.

### R4-CA-2 [Low] The field's Reset button bypasses the per-control write queue, so queued spin writes undo a reset
- **Location:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1241` and `:1345` (Reset via `FieldRow`), queue at `:1093-1097` (tree `cd464aff`)
- **Issue:** `b7221ec6` serialises same-key writes inside `NumberControl` (`writeQueueRef`), and `cd464aff` documents at `remotetermconfig-model.ts:690-693` that "callers issuing several writes to the same key must serialise them". `FieldControl.reset` is a second same-key writer outside that queue: it is sent immediately, ahead of spin writes still queued.
- **Evidence:** same harness as R4-CA-1, RPC 50 ms: three Increase clicks then Reset. `cd464aff` RPC order `[1.05, null, 1.1, 1.1500000000000001]`, final stored `1.1500000000000001`; the user's last action (Reset) is lost. `816fb3d8`: `[1.05, 1.05, 1.05, null]`, stored unset.
- **Impact:** a reset the user saw take effect gets overwritten; with R4-CA-1 it can also leave the field showing a value that differs from the stored one.
- **Fix:** route the reset through the same queue (e.g. give `NumberControl` an `onReset` it enqueues, or lift the queue to `FieldControl` keyed per setting) and clear `pending` when the reset is enqueued.

### R4-CA-3 [Low, pre-existing] Fractional steps accumulate float error into the field and settings.json
- **Location:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1069-1072` (`bumpNumberInput`), `:1054-1065` (`parseNumberInput`); schema `window:zoom` step `0.05` at `:213-215` (tree `cd464aff`)
- **Issue:** `base + delta` is not rounded to the step's precision. Three Increase clicks from `window:zoom` = 1 write `1.05, 1.1, 1.1500000000000001` (R4-CA-2 harness, `cd464aff`); the input shows `1.1500000000000001` and that literal is stored. Same arithmetic at `816fb3d8` (masked there by the stepping bug), so not introduced by this wave, but newly reachable now that repeated steps work.
- **Fix:** round in `bumpNumberInput` to the step's decimal places, e.g. `Number((base + delta).toFixed(decimals(step)))`, and add `window:zoom` to the interaction test.

### R4-CA-4 [Low] Stage, unstage and stage-hunk failures are still console-only
- **Location:** `frontend/app/view/sourcecontrol/sourcecontrol-model.ts:338-388` (`stageFiles`, `unstageFiles`, `stageHunk`), vs `:398-401` (`revertHunk`) (tree `cd464aff`)
- **Issue:** `ed96fef8` surfaces revert failures through `actionErrorAtom`; the three sibling actions have the identical `catch { console.error; await fetchStatus() }` shape and the same symptom (the action silently does nothing, then the status poll clears `errorAtom`). The banner also keeps a message about the previous repository after `cwd`/connection changes (nothing clears `actionErrorAtom` except Dismiss).
- **Fix:** set `actionErrorAtom` in the three catches (`Failed to stage ...: ${e?.message ?? String(e)}`), and clear it where `cwd`/connection change.

### R4-CA-5 [Medium] A production build moves a running legacy dev build's Electron profile (and live SingletonLock) into its own userData
- **Location:** `emain/emain-platform.ts:231-236` (tree `cd464aff`), commit `b98702d6`
- **Issue:** the "other flavour holds the lock, so ours is not running" shortcut is sound for a dev build (the prod lock holder never uses the `-dev` roots), but not for a production build. The shared legacy userData `<appData>/waveterm/electron` sits *inside* the production roots being moved: the Linux config root `~/.config/waveterm` (see the comment at `:450-452`) and the macOS data root `~/Library/Application Support/waveterm`. When the holder is the legacy dev build (stock `electron`), a production RemoteTerm returns `null` and renames the root containing that live profile.
- **Evidence:** scratch test in the existing "running legacy instance" harness (`emain/emain-platform.test.ts`, tree `cd464aff`): Linux, `isPackaged=true`, lock `<host>-424242`, `/proc/424242/exe` = `.../node_modules/electron/dist/electron`, legacy config root with `settings.json` and `electron/Local Storage/x`. Result: `resolveLegacyInstanceBlock()` resolves `true` with no dialog; `~/.config/waveterm/electron/SingletonLock` gone; `~/.config/remoteterm/electron/SingletonLock` -> `<host>-424242`; the live profile is now under `remoteterm/electron`, which is this build's own userData (`:29-30`). The shipped test case `["production", "linux", ".../electron"]` at `emain-platform.test.ts:470` asserts this outcome as correct.
- **Impact:** the running legacy dev app loses its profile path mid-session. UNVERIFIED (Chromium behaviour, not run): the new build's `requestSingleInstanceLock()` (`emain.ts:265`) then finds a live-pid lock in its own userData and quits, notifying the legacy dev app instead. Reachable for anyone who runs a pre-rename dev build and installs the packaged app, which matches this project's own workflow.
- **Fix:** apply the other-flavour shortcut only when `isDev` (dev migrating, prod holder). For a production build a dev holder must still block (or at least get the Quit / Migrate anyway dialog). Flip the two `production` rows of the test to expect a block.

### R4-CA-6 [Medium] A resumed merge cannot recover the database: the server has already created a fresh one in the half-merged root
- **Location:** `emain/emain-platform.ts:346-347`, `:386-395`, `:422-427`; `emain/emain.ts:284` then `:291` (tree `cd464aff`), commit `9f4a84c3`
- **Issue:** after a merge throws, `mergeDataRoot` records a failure and returns; startup continues and `runRemoteTermSrv` (`emain.ts:284`) opens `<data>/db/filestore.db` with `mode=rwc` (`pkg/filestore/blockstore_dbsetup.go:52-54,67`), creating an empty DB in the destination before the failure dialog is even shown (`emain.ts:291`). On the resumed launch `mergeTree` keeps every destination file, so the fresh DB wins and the user's real `db/` stays in the legacy root with a "compare and merge them manually" message; SQLite files cannot be merged by hand. The commit's own example (a `db/` that cannot be moved) is exactly this case, and the shipped test `emain/emain-platform.test.ts:764-781` ("never overwrites while resuming") asserts the orphaned legacy DB as the expected result. The first resume test (`:726-762`) passes only because it models no server run between the two launches.
- **Impact:** a user who fixes the permission problem and relaunches, as the dialog asks, gets an empty workspace/history; the real data is reachable only by manual file surgery. The resume marker converges the state but does not recover the data it was added for.
- **Fix:** do not start the server over a half-merged root. Make a merge failure block like the legacy-instance gate (resolve false from a startup check before `runRemoteTermSrv`, show the failure, quit), so the resumed launch finds the destination as the failed launch left it. Update the `:764` test to model that. `RENAME_PLAN.md` ("Anything the server creates ... the root aborts", then "the merge is resumable ... overwrites nothing") should state the outcome for `db/` once decided.

### R4-CA-7 [Low] For dev builds the identity check matches any stock-Electron process, so a reused pid can still give a Quit-only dialog
- **Location:** `emain/emain-platform.ts:155-158`, `:237-240` (tree `cd464aff`), commit `0748f77e`
- **Issue:** matching is an exact basename compare (good: no substring match, `wave` would not hit), but the dev flavour's expected basename is `electron` / `Electron`, shared by every `electron .` dev process on the machine, including their renderer/GPU/zygote children and a new-build RemoteTerm dev instance. A stale legacy-dev lock whose pid has been reused by any of them is "confirmed running" and gets the Quit-only dialog, the exact dead end `0748f77e` set out to remove.
- **Impact:** narrow (stale lock plus pid reuse into an Electron process), clears once that process exits; the dialog does show the pid and path.
- **Fix:** for the dev flavour, return `confirmedRunning: false` (Quit / Migrate anyway) unless something more specific matches, e.g. `/proc/<pid>/cmdline` containing an app path whose `package.json` is the legacy one; keep the Quit-only dialog for the production basenames.

## Verified OK
<!-- appended as checked -->

- **NumberControl, real events (`b7221ec6`, `2239007a`).** Verbatim `GeneralContent` bundled with esbuild from each tree, driven in `chrome-headless-shell` (no `DISPLAY`) over CDP (`Input.dispatchMouseEvent` / `insertText` / `dispatchKeyEvent`), stub model with RPC delay then a 10 ms watcher event. `cd464aff` / `816fb3d8`:
  - draft `20` + Increase, RPC 5 / 30 / 200 ms: writes `[21]` each, shown 21, max in flight 1 / writes `[20, 13]`, shown 13, 2 in flight.
  - 5 Increase clicks 30 ms apart, RPC 200 ms: `[13..17]`, shown 17, max in flight 1 / `[13 x5]`, 5 in flight.
  - draft `30` + Decrease x2: `[29, 28]` / `[30, 11, 11]`.
  - ArrowUp x2 in the input, then Tab out through both spin buttons: no write until focus leaves the control, then one `[14]` (both trees).
  - Enter on draft `30`: one `[30]` (both trees).
  The fix commit's claims hold. Harness removed; no browser left running.
- **Unmount with writes queued:** queued writes still go out (user intent kept), no `console.error`, no React warning (happy-dom, `cd464aff`).
- **Failed write mid-queue:** `[1.05, 1.1 (fails), 1.15]` converges, field matches stored. A lone failed write reverts to the stored value (shipped test).
- **`setGeneralSetting` returns `Promise<boolean>`**; only `NumberControl` consumes the result; other callers ignore it (`generalcontent.tsx:1240-1241`). The two doc commits (`27603c42`, `cd464aff`) match the code.
- **Source control revert banner (`ed96fef8`):** `actionErrorAtom` set only in `revertHunk`'s catch (`sourcecontrol-model.ts:400`), not cleared by `fetchStatus` (which clears `errorAtom` only, `:268`), cleared by Dismiss; `role="alert"`, `cursor-pointer`, `displayName` set. The commit's "survives a later successful revert" is intended behaviour and matches the code.
- **Identity matching:** exact `path.basename` compare against `waveterm`/`Wave` (prod) and `electron`/`Electron` (dev); legacy names confirmed from `146ceb1e^:package.json` (`name: waveterm`, `productName: Wave`) and `electron-builder.config.cjs:15,82`. `(deleted)` suffix stripped; unreadable `/proc` (EACCES/EPERM) gets the Migrate-anyway dialog; `ps` uses `execFileSync` with an argv array and a 2 s timeout (no shell).
- **Dev build with a production holder** migrates the `-dev` roots only; the production holder's profile is untouched (correct direction of `b98702d6`; opposite direction is R4-CA-5).
- **macOS `electron/` allowance (`169fa9e1`):** only added when `<appData>/remoteterm/electron`'s parent equals the data destination, so Linux and `XDG_DATA_HOME` layouts still abort on `electron/`; server-created entries still abort.
- **Resume marker consistency:** marker written before the first move and removed after the completion marker. Crash after the completion marker leaves a stray `.migrating-from-waveterm` that is never read again (completion check at `:310` wins); harmless. `mergeTree` checks `existsSync` before every `renameSync`, so it overwrites nothing (barring a concurrent launch, which the plan already lists).
- **"Wave Terminal" strings (`1e50117f`):** both dialogs; log lines unchanged as the commit says.
- **Onboarding logo (`425180d8`):** `public/logos/remoteterm-logo.png` is a real 1563x1563 PNG; import and command text updated.
- **`emain-startup-order.test.ts`:** passes; it breaks loudly (not silently) on harmless refactors such as `const ok = await resolveLegacyInstanceBlock(); if (!ok) return;` or `appMain` becoming an arrow function, which is an acceptable trade. Nested functions are excluded from the search, so a guard hidden in a callback does not count.
- **`RENAME_PLAN.md` amendments** match the code, apart from the dev/prod scoping rule (R4-CA-5) and the resume outcome for `db/` (R4-CA-6).
- **Tooling (scratch worktree at `cd464aff`):** `npx vitest run` 32 files / 281 tests green; `go test ./pkg/rtconfig/` ok; `npx tsc --noEmit` errors only in files this wave does not touch (`processviewer.preview.tsx`, `term.tsx:314`, `preview/mock/*`); eslint on changed files: 2 warnings, both on lines not in the diff (`emain-platform.ts:702` unused `url`, `onboarding-command.tsx:6` unused `FakeTermBlock`).

## Completion
**Status:** COMPLETE
**Tally:** Critical 0, High 0, Medium 3 (R4-CA-1, R4-CA-5, R4-CA-6), Low 4 (R4-CA-2, R4-CA-3 pre-existing, R4-CA-4, R4-CA-7), Info 0.
**Not checked:** real macOS (`ps -o comm=` output format), Chromium's `requestSingleInstanceLock` behaviour after R4-CA-5's move (marked UNVERIFIED), the app itself (not launched per brief).
