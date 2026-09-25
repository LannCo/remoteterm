# round3-coverage-analyst report
**Target:** Negative-control check on round-2 fix wave, branch `qa/fleet-2026-09-22` HEAD `4d744a2f`, commits `bab19e28..4d744a2f` (9 fix commits: a98f1c7d d08c4a5f 0b33097c c69d42f0 8d27ba95 14d9489d 88ca9433 daa44e2c 8e3ae0c7)
**Started:** 2026-09-22T15:20:00Z
**Status:** IN PROGRESS

## Findings
<!-- appended one at a time, as found -->

### R3-COV-1 [MEDIUM] R2-CA-1 (`0b33097c`) — `emain.ts` wiring of `resolveLegacyInstanceBlock()` is untested
- **What's uncovered:** `emain/emain.ts:271-276` calls `resolveLegacyInstanceBlock()` before the server starts and quits if it returns `false`. No test imports `emain.ts`/`appMain` (only `emain-platform.test.ts` and `emain-window-icon.test.ts` exist under `emain/`).
- **Confirmed:** reverted only the `emain.ts` hunk of `0b33097c` (kept `emain-platform.ts` fixed) and ran the full suite (`npx vitest run`): 27 files, 243 tests, all green — nothing catches the missing wiring. Restored, re-ran, still 243 green (baseline unaffected).
- **Why it matters:** `resolveLegacyInstanceBlock()` itself is well covered (see below), but the actual bug this commit fixes — "new build renames legacy roots out from under a running instance" — is a `emain.ts` startup-sequencing bug. The unit tests only prove the helper function behaves correctly in isolation; they don't prove it's invoked, or invoked before the server starts. A future refactor that dropped or reordered this call would compile clean and pass the whole suite.
- **Suggested test:** `appMain` is not currently unit-testable (Electron app singleton side effects at module scope per R2-CA-1's own note). Two practical options: (1) extract the startup guard sequence (`resolveLegacyInstanceBlock` check + quit) into a small testable function `runStartupMigrationGuards()` called by `appMain`, unit test that in isolation; or (2) accept as a structural gap given Electron main-process entry points are conventionally covered by e2e/manual smoke rather than vitest, and say so explicitly rather than leave silent.
- **Estimated effort:** S (extract function) or accept as documented gap.
- **Certainty:** traced directly to a full-suite run showing 0 failures with the wiring reverted; not a tool artefact — the file was confirmed instrumented/run (`emain/emain-platform.test.ts` in the same directory ran and passed).

### R3-COV-2 [MEDIUM] R2-CA-6 (`14d9489d`) — spin-button click wiring untested; helper is
- **What's uncovered:** `frontend/app/view/remotetermconfig/generalcontent.tsx:1093-1098`, `NumberControl`'s `bump()` closure (the actual click handler on the spin buttons) calls `bumpNumberInput(local, value, delta, min, max)`. Only the pure helper `bumpNumberInput` is unit tested (`generalcontent.test.ts`); no test exercises `NumberControl`/`bump()` itself, so nothing proves the helper is actually wired into the click handler rather than just defined alongside it.
- **Confirmed:** manually reverted only the `bump()` body (restored the pre-fix `onChange(parseNumberInput(String(value + delta), min, max))`, leaving `bumpNumberInput` intact but unused) and ran both the file's own tests and the whole `remotetermconfig/` directory: 15/15 and 47/47 green respectively — the exact regression the brief asked about (wiring reverted, helper kept) passes clean. Restored, then also ran the standard full-commit revert (both hunks) as a second check: 3/15 red (`bumpNumberInput is not a function`), confirming the helper itself is tested, restored to green after.
- **Why it matters:** the fix commit message describes a UI interaction bug ("typing 50 over 10, clicking the up arrow"); the shipped test only proves the arithmetic is correct in isolation, not that `NumberControl` calls it. A future edit that reverted just the `bump()` line (e.g. an accidental merge conflict resolution, or someone "simplifying" `NumberControl` back to inline `onChange`) would pass every current test.
- **Suggested test:** a React Testing Library test on `NumberControl` (render with a typed draft in the input, fire the spin-up click, assert `onChange` receives the value stepped from the draft, not from the stale `value` prop) closes this. `NumberControl` is already exported-adjacent (declared with `memo` in the same file) so this is a component-level test, same style as the existing `remotetermconfig-a11y.test.tsx` file in the same directory.
- **Estimated effort:** S.
- **Certainty:** traced to two separate live test runs (partial revert: 47/47 green; full revert: 3/15 red), not a tool artefact.

### R3-COV-3 [LOW] R2-CONF-1 (`8e3ae0c7`) — `emain-window-icon.test.ts` checks source text only, but a real build-level control is practical and passes
- **What's uncovered:** the test regexes `emain-window.ts` source for a `?asset` import matching `build/icons/\d+x\d+\.png` and confirms the *source* PNG file exists at `build/icons/256x256.png` — it never runs electron-vite's asset pipeline, so it doesn't prove the `?asset` import actually resolves or that the emitted chunk lands where `emain-builder.ts`/`emain-window.ts` expect it at runtime.
- **Confirmed impractical-or-not:** ran `npx electron-vite build --mode development --outDir <scratch>` from the scratch worktree (not `go build`, no app launch, no daily-driver touch). Completed in **37.11s**. Verified `dist/main/chunks/256x256-<hash>.png` was emitted, is byte-identical (`cmp`) to `build/icons/256x256.png`, and the built `main` bundle references it by the same `chunks/256x256-<hash>.png` relative path. This is a real, currently-passing control — the brief's proposed stronger test is practical and would pass today. Build output was removed after inspection (`rm -rf` on the scratch outDir only); the QA worktree and scratch worktree `git status --short` both clean afterward.
- **Why it matters:** the current test is source-text pattern matching, which passes even if electron-vite's config changed in a way that breaks `?asset` emission (e.g. an `assetsInclude`/`outDir` misconfiguration) or if the built chunk path diverged from the source path assumption. A 37s build step is cheap enough to run in CI on every PR touching `emain/emain-window.ts` or `electron.vite.config.*`.
- **Suggested test:** add a CI-only (not vitest, since it needs a real build) smoke check: `electron-vite build --mode development --outDir <tmp>` then assert `dist/main/chunks/256x256-*.png` exists and is non-empty; or wire it as a vitest test using `execSync`/`child_process` if the ~37s cost is acceptable inside the vitest run. Either way, gate it on the same file paths this commit touched so it doesn't run on unrelated PRs.
- **Estimated effort:** S (script exists and was validated above; wiring into CI is the only remaining work).
- **Certainty:** traced to a live build run with byte-comparison confirmation, not a tool artefact.

## Verified OK
<!-- appended as checked -->

- **a98f1c7d** (secrets.enc-only migration root) — reverted cumulatively with `d08c4a5f`+`0b33097c` (all three touch overlapping regions of `emain/emain-platform.ts`; a direct isolated revert conflicts). "migrates a root holding only secrets.enc" red on revert, green on restore.
- **d08c4a5f** (merge into existing destination) — reverted cumulatively with `0b33097c`. "merges into a destination left by an earlier build's defaults and converges" and "never overwrites a destination file and reports the kept legacy copy once" red on revert, green on restore.
- **0b33097c** (block migration while legacy instance running) — reverted in isolation (newest of the three, no conflict). 10/29 tests red (`resolveLegacyInstanceBlock is not a function`), green on restore. See R3-COV-1 for the `emain.ts` wiring half of this commit, which is not covered by any test.
- **8d27ba95** (keep both of two rapid widget drags) — reverted in isolation. "two quick drags both land..." red on revert, green on restore.
- **c69d42f0** (surface failed tab-background apply) — reverted cumulatively with `8d27ba95` (both touch `remotetermconfig-model.ts`). "applying a background to the tab surfaces an RPC failure instead of rejecting" red on revert, green on restore.
- **14d9489d** (spin buttons step from typed number) — standard full-commit revert (helper + wiring): 3/15 red (`bumpNumberInput is not a function`), green on restore. See R3-COV-2 for the partial-revert gap.
- **88ca9433** (revert staged hunk unstages properly, `pkg/wshrpc/wshremote/git.go`) — reverted in isolation (Go). All 3 commit-specific tests (`TestGitRevertStagedHunk*`) red on revert with clear diff output showing the hunk still staged; green on restore. `TestGitRevertHunkDescendingClearsEveryHunk` stayed green through the revert (as round-2 baseline noted, it discriminates a different commit `eb94ed5d`, not this one) — expected, not a new gap.
- **daa44e2c** (readable colour for settings error messages) — reverted in isolation (3 prod files: `backgroundscontent.tsx`, `connectionscontent.tsx`, `secretscontent.tsx`). 3/8 a11y tests red on revert, green on restore.
- **8e3ae0c7** (Linux window icon) — standard full-commit revert (`emain-builder.ts` + `emain-window.ts`): 2/2 red (`public/logos`/`wave-logo` regex hits reappear), green on restore. See R3-COV-3 for the stronger build-level control.

## Completion
**Status:** COMPLETE

All 9 round-2 fix commits negative-controlled (revert prod-only hunk, confirm red, restore, confirm green). Three commits sharing `emain/emain-platform.ts` (a98f1c7d, d08c4a5f, 0b33097c) and two sharing `remotetermconfig-model.ts` (c69d42f0, 8d27ba95) required cumulative newest-first reverts since a direct isolated revert of the older commit conflicted against the newer commit's overlapping hunks (documented in [[remoteterm_coverage_tooling]] update below) — all still traced to commit-specific red tests.

**Findings by severity:** 1 LOW (R3-COV-3, icon build-control), 2 MEDIUM (R3-COV-1 emain.ts wiring untested, R3-COV-2 spin-button click wiring untested), 0 HIGH/CRITICAL.

**Could not check:** nothing outstanding — all three special-attention items from the brief were directly tested (R2-CONF-1 build-level control run and confirmed practical/passing; R2-CA-6 wiring-vs-helper isolation confirmed a real gap; R2-CA-1 emain.ts early-return confirmed untested via full-suite run).
