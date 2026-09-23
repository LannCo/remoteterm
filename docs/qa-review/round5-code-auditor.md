# code-auditor round-5 report
**Target:** RemoteTerm `qa/fleet-2026-09-22`, `git diff 60f923d2 72b21bd9` (fourth fix wave), tree = HEAD `72b21bd9` unless stated
**Started:** 2026-09-22 (UTC)
**Status:** COMPLETE

## Findings

### R5-CA-1 [Low] NumberControl stays stale when another writer sets a value this control wrote earlier
- **Location:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1100` (tree `72b21bd9`)
- **Issue:** the new external-change rule clears `pending` only if `!sentRef.current.includes(value)`. `sentRef` holds every value written since `pending` was last clear, so another writer (other window, `wsh setconfig`) setting the key to one of those earlier values is taken for a late echo of our own write. `pending` is reconciled only on a prop change, so if no later event moves `value`, the field shows its own last write indefinitely.
- **Evidence (runtime):** scratch probe in the repo's own harness (`generalcontent-numbercontrol.test.tsx` `setup({ delayMs: 20, eventLagMs: 200 })`, the lag the fixer's "late echo" test uses): two Increase clicks write `[13, 14]`, then `external(13)`; stored value settles at 13, the input shows `"14"` (Expected "13", Received "14") 400 ms after the file settled. The next spin steps from the stale 14.
- **Impact:** display drift, then a write computed from the wrong base value. Needs a second writer to write a recently-sent value within the watcher lag, so it's rare. It's the same bug class as R4-CA-1, narrowed but still open.
- **Fix:** tell an echo apart from an external write by the order it arrives in, not by its value: count acknowledged writes and match echoes to them in order (an echo can only be for a write that has been acknowledged), or have `setGeneralSetting` return a per-write sequence/mtime that the watcher event carries. Also re-evaluate `pending` when `inFlightRef` drops to 0 (the resolve callback currently triggers no render on success, see line 1117).

### R5-CA-2 [Info] `migrationIncomplete` is never reset across the "Migrate anyway" re-run
- **Location:** `emain/emain-platform.ts:136`, `:344` (tree `72b21bd9`); code-read, no runtime repro
- **Issue:** `resolveLegacyInstanceBlock()` re-runs `performDataDirMigration()` after "Migrate anyway". An incomplete failure recorded in the first pass keeps `migrationIncomplete` true and stays in `migrationFailures`, even if the second pass completes that root. Since 149d275f the first pass can now attempt some roots while others are blocked (prod build, dev holder), which makes this reachable.
- **Impact:** only if a failure is transient between two passes a few seconds apart: one extra quit that shows a stale error, then the next launch proceeds. No data risk.
- **Fix:** clear `migrationFailures` and `migrationIncomplete` at the top of `performDataDirMigration()`. The re-run re-derives every root's state, and roots that already completed return early on their marker.

## Verified OK

**Fix commits against `60f923d2` (red-before):** I put the new `generalcontent-numbercontrol.test.tsx` over `60f923d2`'s `generalcontent.tsx` in a scratch worktree. The three new behavioural tests fail there: external change after acknowledgement (53b80984), Reset after queued clicks (dae0b3f2), fractional steps (cb1f7477). The in-flight-external and late-echo tests pass on both trees. They pin behaviour and aren't regressions. Each commit message matches its diff.

**Regression re-checks at HEAD (all 14 interaction tests plus 176 tests across emain/, remotetermconfig/, sourcecontrol/, onboarding/ green):**
- A typed draft plus a spin click sends one write (mouse and keyboard); focus stays in the input.
- Rapid clicks write `[13,14,15]` in order with `maxInFlight` 1. The queue moved to `FieldControl` (`generalcontent.tsx:1263-1272`) is per key and survives NumberControl re-renders. Reset shares it, so it goes last.
- A failed write reverts to the stored value. `setGeneralSetting` returns false rather than throwing, so the queue's `.catch` doesn't swallow a result.
- `decimalPlaces` handles exponent forms (`1e-7`, `1.5e-7`, `1e+21`). The regex always matches (an empty match at `$`) and is linear. A typed draft keeps its own precision.

**Migration (`emain/emain-platform.ts`, `emain/emain.ts`):**
- Never overwrites: `mergeTree` still renames only missing entries. The new `db/` check (`:472`) runs before the in-progress marker is written and touches neither side.
- Marker semantics are unchanged. An incomplete failure writes no completion marker, and the resume marker persists.
- Identity check: a non-legacy exe gives "unknown" (Migrate anyway). A prod exe is confirmed. A stock `electron`/`Electron` holder is never confirmed (`:271`).
- `legacyInstanceBlocksRoot` (`:283`) blocks the Linux config root and the macOS data root for a prod build with a dev holder. The `-dev` roots never contain `<appData>/waveterm/electron`, so dev builds are unaffected. The unknown-state path (no `otherFlavour`) blocks every root.
- `emain.ts:275-284`: both guards run before `runRemoteTermSrv`, after `requestSingleInstanceLock`.
- tsc is clean for the touched files. eslint reports only 5 warnings in untouched lines (`emain.ts:14,22,23,46`, `emain-platform.ts:767`).

**Open decision: sticky abort / ENOENT still starting the server. I judge it safe:**
- *Legacy-home abort* (`:401-405`, no `canMergeIntoDest`): the source is untouched. The Go server never creates `~/.remoteterm`; `wsh` uses `~/.waveterm` only on remote hosts (`pkg/remotetermbase/wavebase.go:144`), and a `~/.waveterm` without `wave.lock` is skipped. If `~/.remoteterm` has `wave.lock`, the new home is used and the legacy one is reported in the post-start error box on every launch (`emain.ts:297`). Otherwise `getRemoteTermHomeDir` (`:645`) falls back to running in place on the legacy home. Nothing destructive happens and the data isn't silently ignored.
- *Data-root abort* reaches the same branch (not listed in the fixer's note). It needs a non-allowlisted entry already in the destination, meaning the new build's own server state. Starting adds nothing new, the source is untouched, and the error is reported each launch. A quit launch writes only `logs`, `rtapp.log`, and on macOS the Electron userData, all allowlisted (`:561-573`), so this wave's quits can't create that state.
- *ENOENT without a marker* (`:417-426`): the source existed at `:365`, so ENOENT means a concurrent migrator renamed it. The rename is atomic, so whichever process wins the single-instance lock starts against a complete destination. A concurrent merge that hits ENOENT inside `mergeTree` goes through `recordIncompleteMigration` and quits.

**Source control / a11y:** stage, unstage and hunk failures set `actionErrorAtom` (`sourcecontrol-model.ts:351,369,386`). Focus moves before clear (`action-error.tsx:30-33`, `remotetermconfig.tsx:125-129`). `tabIndex={-1}` on the SCM container means a blank-area click now puts focus inside it. This helps the container-scoped Ctrl+Shift+R listener (`sourcecontrol.tsx:635-647`) and doesn't fight `block.tsx`'s focus-within check. The onboarding alias change (`rt="wsh"`) is text only.

## Completion
**Status:** COMPLETE
Tally: Critical 0, High 0, Medium 0, Low 1, Info 1.
Not checked: Windows behaviour of the new incomplete-migration quit (the `windows-userdata` root); runtime CDP focus ordering (covered by the repo's happy-dom tests, not re-driven); whether RPC responses can arrive after the watcher event in the real server (that ordering would widen R5-CA-1; UNVERIFIED).
