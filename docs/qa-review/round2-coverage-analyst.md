# round2-coverage-analyst report
**Target:** Round-2 coverage/test-strength check on RemoteTerm `qa/fleet-2026-09-22` @ `75b60911`, verifying round-1 gaps (COV-1..6, TC-1) and negative-controlling every fix's test in `35cd6146..75b60911`.
**Started:** 2026-09-22T00:00:00Z (session clock)
**Status:** IN PROGRESS

## Gap status
<!-- appended one at a time, as checked -->

### R2-COV-1 COV-1/COV-2 (handleRouteEvent, doReconnectJob) — mostly closed
`go test ./pkg/jobcontroller/... -coverprofile=...` then `go tool cover -func`:
- `handleRouteEvent` (jobcontroller.go:566): 0.0% -> **84.6%**
- `doReconnectJob` (jobcontroller.go:2051): 4.3% -> **74.3%**
- `doReconnectJobExclusive` (jobcontroller.go:2044): **100.0%** (new, per-job serialisation from `ec0ab99c`)
- `handleRouteUpEvent`/`handleRouteDownEvent` (jobcontroller.go:558,562): still **0.0%** — these remain the thin wrappers round-1 called out as "trivial, same story"; still true, `handleRouteEvent` (the logic they call into) is now well covered.
- Source: `pkg/jobcontroller/jobcontroller_reconnect_test.go` (532 new lines, commit `7dd1f1b5`). Package coverage rose 24.5% -> 45.0%.
- **Status:** substantially closed. Residual gap (thin wrapper functions) is cosmetic, not a finding.

### R2-COV-2 COV-3 (shellexec/wavebase security-sensitive logging) — partially closed
- `RedactSecret` (`pkg/util/shellutil/tokenswap.go:103`): 0.0% -> **100.0%**, `pkg/util/shellutil/tokenswap_test.go:12` `TestRedactSecret`.
- `GetMapValNewOrLegacy` (`pkg/remotetermbase/wavebase.go:111`): **still 0.0%**. The new `pkg/remotetermbase/wavebase-posix_test.go` (106 lines, commit `282c9246`) tests `AcquireWaveLock`'s legacy-`wave.lock` refusal instead — a different fix in the same package, not this function. Grepped the full diff `35cd6146..75b60911` for `GetMapValNewOrLegacy`; zero test references anywhere in the tree.
- `pkg/shellexec` package: still **0.0% of statements**, still zero `_test.go` files (`find pkg/shellexec -name '*_test.go'` empty). The `Debugf`-gated redaction call sites at shellexec.go (StartWslShellProc/StartRemoteShellProc) from `bb79c27d` remain untested by any Go test, only indirectly by the now-tested `RedactSecret` helper they call.
- **Status:** the redaction *primitive* is now unit-tested (good — it's the highest-leverage single fix, catches a reverted-to-Infof regression only if the call site is also inspected, which it isn't). The dual-write JWT fallback (`GetMapValNewOrLegacy`) and the shellexec.go call sites remain genuinely uncovered, exactly as round 1 found. Not a regression, just not fully addressed.

### R2-COV-3 COV-4 (grabAndRemoveEnvVars) — closed
`cmd/server/main-server.go:135` `grabAndRemoveEnvVars`: 0.0% -> **95.0%**. `cmd/server/main-server_test.go` (59 new lines). Package coverage 0.0% -> 11.1%.
- **Status:** closed.

### R2-COV-5 COV-5 (emain-platform.ts migration shim) — substantially closed
`npx vitest run --coverage emain/emain-platform.test.ts ...` (istanbul, lcov, `coverage/lcov.info`, `SF:emain/emain-platform.ts`): 0/210 lines -> **149/221 (67.4%)**, 0/31 funcs -> **16/34 (47.1%)**, 0/126 branches -> **86/128 (67.2%)**. Source: `emain/emain-platform.test.ts` (244 new lines). Test names cover exactly the scenarios round 1 asked for: dev-suffix fallback (`combined-home fallback > dev build ignores the production ~/.waveterm` / `migrates and uses ~/.waveterm-dev`), per-root legacy validation via marker files (`migrates a root holding only connections.json/widgets.json/presets/ai.json`, `skips an Electron-only root`), override short-circuit (`REMOTETERM_DATA_HOME`/`WAVETERM_DATA_HOME override short-circuits`), and partial-failure surfacing (`aborts and reports when the destination is not empty`, `ENOENT from rename > treats the move as done when another process left the marker` / `reports a failure when no marker was left`).
- **Status:** closed for the scenarios round 1 named. ~53% of functions/33% of branches remain uncovered per the raw numbers — did not enumerate which, out of scope for this pass (round 1 raised the file at 0%, not a specific residual branch list), flagging as residual rather than a new finding.

### R2-COV-6 COV-6 (remotetermconfig-model.ts secrets + backgrounds + read-failure branches) — substantially closed
`SF:frontend/app/view/remotetermconfig/remotetermconfig-model.ts`: 195/518 lines (37.6%) -> **331/530 (62.5%)**, 16/69 funcs (23.2%) -> **41/76 (53.9%)**, 39/193 branches (20.2%) -> **87/207 (42.0%)**. Source: `remotetermconfig-model.writes.test.ts` (325 new lines, commit `ecbb5a04`).
- Secrets (round-1's named gap, lines 508-651 pre-change): `saveSecret writes.../surfaces an RPC failure.../does nothing without a selected secret`, `deleteSecret sends a null value.../surfaces an RPC failure`, `addNewSecret rejects empty, invalid and duplicate names.../stores a trimmed valid name.../surfaces an RPC failure` — 8 tests, all in `remotetermconfig-model.writes.test.ts` under "RemoteTermConfigViewModel — secrets".
- Backgrounds read/write (round-1's named gap): `readRawBackgroundsFile > treats a missing file as empty...`, `background opacity then blend mode in quick succession keeps both`, `quick-add applies/does not apply the background to the tab when the write succeeds/fails`.
- Missing-file-vs-read-failure distinction (round-1's specifically flagged highest-value/lowest-effort item): **directly hit** — `readRawBackgroundsFile > returns null on a read failure and aborts the write instead of treating it as empty` and `> returns null for a non-object file and aborts the write`, both distinct from the `treats a missing file as empty` case. This is the exact three-way split (missing/error/malformed) round 1 asked for, applied to backgrounds; round 1's suggestion to also add it to the *widgets* read path was not done (only backgrounds), a minor scope note not a gap since backgrounds was the more-uncovered target.
- **Status:** closed for secrets, backgrounds read/write, and the read-failure-vs-missing-file distinction. Residual: widgets-path read-failure-vs-notfound split (round 1's secondary ask) still untested; branch coverage overall still only 42%, real residual surface remains but no new named gap found in the round-1-scoped review.

### R2-COV-4 TC-1 (TestHandleSystemResumeSmoke asserts nothing) — closed
The no-op test was replaced (not just supplemented) by `TestHandleSystemResumeFastPathFiltering` (`pkg/jobcontroller/jobcontroller_test.go:511-583`, commit `35891279`). It now asserts real outcomes via test hooks (`hasRunningDurableJobsTestHook`, `NeedsInteractiveAuthTestHook`, `resumeReconnectTestHook`): the eligible (disconnected, unsuppressed, has durable jobs, no interactive auth) connection's name arrives on the `attempted` channel; suppressed/no-jobs/interactive/already-healthy connections are explicitly asserted absent (`t.Fatalf` if seen), and scheduler-map cleanup is checked per connection. `HandleSystemResume` itself: 61.8% -> **75.7%** function coverage.
- **Status:** closed. Flagged separately under Flake risks (200ms empty-channel-drain window).

## Negative-control table
<!-- appended one at a time, as checked -->
All reverts/restores done in throwaway worktree `/tmp/claude-1000/-media-owner-Workspace-remoteterm/6b9bb981-3b3e-4d9c-9a93-b16272c3469e/scratchpad/r2-mut` @ `75b60911` (`node_modules` symlinked from QA worktree), one commit at a time: `git show <commit> -- <prod files> | git apply -R`, run test, `git checkout HEAD -- <prod files>` to restore before the next.

| ID | Commit | Prod file(s) reverted | Test run | Reverted result | Restored result |
|---|---|---|---|---|---|
| CA-9 | `a117aff3` | `pkg/jobcontroller/jobcontroller.go` | `TestStartJobFailureReleasesStream` | **FAIL** — "stream health still active/reader still registered/stream id still registered after failed start" | PASS |
| CA-7 | `ec0ab99c` | `pkg/jobcontroller/jobcontroller.go` | `TestReconnectEntrypointsSerializeRestartStreaming` | **FAIL** — "2 restartStreaming calls in flight at once for one job" | PASS |
| CA-8 | `9c212461` | `pkg/jobcontroller/jobcontroller.go` | `TestConnectedNoStreamRestartFailurePublishesStatus` | **FAIL** — "last published status = connected, stored status = disconnected" | PASS |
| CA-4 | `282c9246` | `pkg/remotetermbase/wavebase-{posix,win}.go`, `wavebase.go` | `TestAcquireWaveLock*` (3 tests) | **FAIL** — build failure, `undefined: LegacyWaveLockFile` (stronger than a runtime fail: the test can't even compile without the fix) | PASS |
| CA-3 | `27a9c131` | `pkg/rtconfig/settingsconfig.go` | `TestSetBaseConfigValueConcurrentWritersKeepAllKeys`, `TestSetConnectionsConfigValueConcurrentWritersKeepAllConns` (`-race`) | **FAIL** — "key ... lost to a concurrent writer" (both) | PASS |
| CA-11a | `eb94ed5d` | `pkg/wshrpc/wshremote/git.go` | `TestGitRevertHunk*` (3 tests) | **PARTIAL FAIL** — `TestGitRevertHunkWithUnequalLineCounts` and `...AfterEarlierHunk` FAIL ("corrupt patch"); `TestGitRevertHunkDescendingClearsEveryHunk` still PASSES with the fix reverted — see flag below | 3/3 PASS |
| CA-11b | `dfe1daa8` | `frontend/app/view/sourcecontrol/sourcecontrol-model.ts` | `review-mode.test.ts` | **FAIL** — "revertFileFromReview > invalidates diff cache and reverts every hunk from last to first" mismatched call order | PASS (31/31) |
| CA-1/CA-12 | `9f786c0b` | `frontend/app/view/remotetermconfig/remotetermconfig-model.ts` | `remotetermconfig-model.writes.test.ts` + `.widgets.test.ts` | **FAIL** — 4 tests: "toggle then drag...", "opacity then blend mode...", "quick-add does not apply background on write failure" (+1 more) | 22/22 PASS |
| CA-2 | `1559811c` | `generalcontent.tsx` | `generalcontent.test.ts` | **FAIL** — 3 `parseNumberInput` tests, `TypeError: parseNumberInput is not a function` | 12/12 PASS |
| CONF-4 | `2c50d488` | `generalcontent.tsx` | `generalcontent.test.ts` | **FAIL** — "no FieldSchema...uses the upstream Wave product name": `app:confirmquit` still said "Wave" | 12/12 PASS |
| A11Y-1/2/3 | `3d3f5f05` | `secretscontent.tsx` | `remotetermconfig-a11y.test.tsx` | **FAIL** — 4 SecretsContent tests, "no `<label for>` with text Name/Value" | 8/8 PASS |
| A11Y-4/5 | `87056ec3` | `backgroundscontent.tsx`, `connectionscontent.tsx`, `remotetermconfig.tsx` | `remotetermconfig-a11y.test.tsx` | **FAIL** — 3 tests: connections/background error not `role=alert`, keychain row still has `opacity-70` | 8/8 PASS |
| A11Y-6 | `efe415fe` | `widgetscontent.tsx` | `remotetermconfig-a11y.test.tsx` | **FAIL** — "Widget order move buttons...24x24px": button classes missing `w-6` | 8/8 PASS |
| CA-5 | `6563e25c` | `emain/emain-platform.ts` | `emain-platform.test.ts` | **FAIL** — exactly "dev build ignores the production ~/.waveterm" (1/16) | 16/16 PASS |
| CA-6 | `7e781c9e` | `emain/emain-platform.ts` | `emain-platform.test.ts` | **FAIL** — 5/16 "legacy config root validation" tests | 16/16 PASS |

**14/14 fix items have at least one test that goes red when the fix is reverted and green when restored.** One item (CA-11a) has a test in its group that does not discriminate — see below.

## Flake risks
<!-- appended as found -->

### R2-FLAKE-1 [LOW] TestGitRevertHunkDescendingClearsEveryHunk doesn't discriminate the eb94ed5d fix
`pkg/wshrpc/wshremote/git_revert_test.go:79` passes identically whether `eb94ed5d`'s `git.go` fix is present or reverted (see CA-11a row above). Not a flake in the sense of nondeterminism — it's deterministic-green either way — but it means only 2 of the 3 new tests in this file are load-bearing for the "unequal add/remove line count" bug; this one is redundant coverage of the "descending order" scenario the *pre-existing* revert logic already handled. Not a false-negative risk for regressions in the specific unequal-line-count fix (the other two tests catch that), just an assertion strength note.

### R2-FLAKE-2 [LOW] TestHandleSystemResumeFastPathFiltering has a fixed 100ms drain window
`pkg/jobcontroller/jobcontroller_test.go:566` sleeps 100ms after seeing the expected reconnect to give "any wrongly spawned" goroutines time to report on a channel before draining it. On a heavily loaded CI runner a wrongly-spawned reconnect goroutine that takes >100ms to reach the hook could pass this run and only get caught on a subsequent one — asymmetric risk (only masks true positives, doesn't produce false failures), so it would show as an intermittent miss of a real regression rather than a flaky red. Low risk given the goroutines here do no real I/O.

### R2-FLAKE-3 [LOW] TestReconnectEntrypointsSerializeRestartStreaming relies on a real 300ms sleep to force the concurrency window
`pkg/jobcontroller/jobcontroller_reconnect_test.go:216,271` — `fakeJobManager.JobPrepareConnectCommand` sleeps `prepareHold` (300ms) so two concurrently-launched `ReconnectJob`/`ReconnectJobRoute` calls are both in flight when the assertion checks `maxInflight`. This is a genuine concurrency test (confirmed to fail red when the serialisation fix is reverted, see CA-7 above), but the 300ms margin is a guess at "long enough to guarantee overlap under CI scheduling pressure" — not inherently flaky, but if CI is ever heavily oversubscribed a goroutine scheduling delay >300ms before the second call reaches the hold could produce a false pass (masking a would-be regression) rather than a false fail. `go test -count=1` reruns won't surface this; only scheduler pressure would.

### R2-FLAKE-4 [INFO] conncontroller_test.go carries many pre-existing sleep-based waits, largely unchanged this round
`pkg/remote/conncontroller/conncontroller_test.go` has ~25 `time.Sleep` calls (5ms-100ms) and extensive `t.Parallel()` use; only 2 sleep lines are new in the `79781c86` "make the package pass go test -race" commit (the rest predate round 2). Package coverage moved only 47.1% -> 47.9% this round — this file was primarily a race-condition fix, not a coverage addition, so out of round-1-gap scope; flagging as a pre-existing flake surface, not a regression introduced this round.

## Completion
**Status:** COMPLETE

**Gap status tally:** COV-1/COV-2 substantially closed (handleRouteEvent 0->84.6%, doReconnectJob 4.3->74.3%; thin wrapper functions still 0%, cosmetic). COV-3 partially closed (`RedactSecret` now 100%; `GetMapValNewOrLegacy` still 0%, `pkg/shellexec` still has zero test files — genuine residual gap, not a regression). COV-4 closed (`grabAndRemoveEnvVars` 0->95%). COV-5 substantially closed (`emain-platform.ts` 0->67.4% lines / 47.1% funcs / 67.2% branches, all round-1-named scenarios present). COV-6 substantially closed (`remotetermconfig-model.ts` 37.6->62.5% lines / 23.2->53.9% funcs / 20.2->42.0% branches; secrets, backgrounds read/write, and the missing-file-vs-read-failure distinction all directly tested). TC-1 closed (`TestHandleSystemResumeSmoke` replaced by an assertion-bearing test).

**Negative-control tally:** 14/14 fix items (CA-1/CA-12, CA-2 through CA-9, CA-11a/b, CONF-4, A11Y-1/2/3, A11Y-4/5, A11Y-6) have at least one test confirmed red-on-revert, green-on-restore, in a throwaway worktree at `75b60911` (removed after use, QA worktree left untouched — confirmed via `git status`). One test (`TestGitRevertHunkDescendingClearsEveryHunk`) does not discriminate its nominal fix commit (`eb94ed5d`) but the other two tests in the same file/commit do, so CA-11 as a whole is still validated.

**Measurement basis:** `go test <pkgs> -cover` / `-coverprofile` + `go tool cover -func` for Go; `npx vitest run --coverage` (istanbul, lcov at `coverage/lcov.info`) for frontend, parsed via `SF:`/`LH:`/`LF:`/`FNH:`/`FNF:`/`BRH:`/`BRF:` per-file records. All commands confirmed non-empty test counts and exit status before use (96 vitest tests / 7 files in the frontend scope run; Go packages listed all returned `ok` with non-zero statement counts except `pkg/shellexec`, confirmed genuinely test-file-less via `find`, not a tool artefact).

**Could not check:** did not re-run the full existing suite for every reverted commit (only the tests named per-commit in the brief) — a revert could theoretically also break an unrelated pre-existing test not checked here, but that's outside the negative-control question asked. Did not investigate whether `pkg/shellexec`'s and `GetMapValNewOrLegacy`'s continued absence of tests (COV-3 residual) was a deliberate scope cut by this round's fix wave or an oversight — flagging as still-open per round 1, not re-litigating severity.
