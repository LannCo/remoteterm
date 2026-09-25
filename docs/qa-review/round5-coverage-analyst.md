# coverage-analyst report

**Target:** Round-5 negative controls on RemoteTerm's fourth fix wave — 11 fix commits `60f923d2..72b21bd9` (HEAD `72b21bd9`), plus re-verification of tests changed to correct previously-wrong assertions.
**Started:** 2026-09-22T00:00:00Z
**Status:** COMPLETE

## Findings
<!-- appended one at a time, as found -->

## Verified OK
<!-- appended as checked -->

- Frontend fix chain confirmed red-on-revert, cumulative newest-first per [[remoteterm_coverage_tooling]] where hunks overlapped:
  - `cb1f7477` (fractional spin rounding): reverting `bumpNumberInput`'s `.toFixed(decimals)` call fails 3/3 target tests (`generalcontent-numbercontrol.test.tsx`, `generalcontent.test.ts`) with the exact `1.1500000000000001` float-noise regression the commit message describes.
  - `dae0b3f2` (Reset ordering): manually reverted (patch context broken by the later prettier commit `72b21bd9`'s reflow of `FieldControl`), fails the targeted "Reset after queued spin clicks" test with `[1.05, null, 1.1, 1.15]` vs expected `[1.05, 1.1, 1.15, null]` — Reset lands ahead of the queue exactly as pre-fix.
  - `53b80984` (external-writer echo): manually reverted (same prettier-shifted region), fails its targeted "external change... shown" test; also collaterally fails `dae0b3f2`'s Reset test since that test's assertions depend on the `sentRef`/`inFlightRef` state `53b80984` introduced — expected coupling between two fixes touching the same component, not a false positive (confirmed independently earlier that `dae0b3f2` alone, with `53b80984`'s code intact, fails only its own test).
  - `b79cd8c8` (stage/unstage/stage-hunk error banner): reverting the 3 `globalStore.set(actionErrorAtom, ...)` calls fails exactly the 3 targeted tests in `sourcecontrol-revert-error.test.tsx`.
  - `1bdf3091` (24x24 dismiss target): reverting the `className` fails exactly the 1 targeted "24x24 minimum target" test.
  - `935afc22` (focus-on-dismiss for 3 banners): reverting all three touched files (`remotetermconfig.tsx`, `action-error.tsx`, `sourcecontrol.tsx`) fails all 3 targeted focus-retention tests across `remotetermconfig-dismiss-focus.test.tsx` and `sourcecontrol-revert-error.test.tsx`.
  - `90f7f449` (onboarding alias rename): reverting `alias rt="wsh"` back to `alias wave="wsh"` fails the targeted test.
  - `ae7d436b` (test-only, closes round-4's R4-COV-1): no production commit exists for this fix; verified by removing the two `onMouseDown={(e) => e.preventDefault()}` calls from `generalcontent.tsx` directly (mirroring the commit's own stated control) — both new `document.activeElement` assertions fail exactly as the commit message claims (31/31 → 29/31, both failures on the two new focus lines). R4-COV-1 is now closed.
- Migration fix chain confirmed red-on-revert, newest-first (`149d275f` → `7d066154` → `bbdde6bb`):
  - `149d275f` (never move the shared legacy profile from under a running dev build): reverting `legacyInstanceBlocksRoot`/`otherFlavour` fails exactly the 2 targeted "blocks only the root holding the shared profile" tests (linux/darwin).
  - `7d066154` (stock Electron holder offers Migrate anyway in dev): manually reverted the `holderIsDev` branch (patch context broken by `149d275f`'s refactor of the same function) back to always-`confirmedRunning:true`; fails its own targeted "dev build is still blocked... with Migrate anyway" test on both platforms, plus 4 collateral failures in `149d275f`'s tests that build on the same `holderIsDev`/`otherFlavour` mechanism — expected coupling (`149d275f`'s isolated revert above, on top of `7d066154` intact, showed only its own 2 tests fail).
  - `bbdde6bb` (quit instead of starting server over a failed migration): reverting `resolveIncompleteMigrationBlock`, `recordIncompleteMigration`, the db/-conflict guard in `mergeDataRoot`, and its `emain.ts` wiring fails exactly the 7 targeted tests across `emain-platform.test.ts` and `emain-startup-order.test.ts` (the failed-move abort test, both interrupted-merge tests, the db/-conflict refusal test, and both startup-order wiring tests).
- Changed-test guard re-check (per brief): all three still guard what they claim.
  - `emain-platform.test.ts:877` "never overwrites while resuming" — now separated from the db/-conflict case (own test at `:898`, added by `bbdde6bb`). It still asserts `notes.txt` in the destination is not clobbered (`dest-notes` kept) and the legacy copy survives separately (`legacy-notes` kept), while `resolveIncompleteMigrationBlock()` still resolves `true` because a kept-copy conflict is recoverable (`recordMigrationFailure`, not `recordIncompleteMigration`). The never-overwrite rule is intact; it no longer conflates "kept a copy" with "must quit."
  - `emain-platform.test.ts:612` "a dev build is still blocked by a stock Electron holder, with Migrate anyway" — asserts `showMessageBox` buttons are `["Quit", "Migrate anyway"]` (not Quit-only), closing the R4-CA-7 dead end; both flows (Quit-default and Migrate-anyway) are exercised in the same test.
  - `emain-startup-order.test.ts` — the `isAwaitedGuardThatReturns(stmt, guardName)` helper is now parameterised (was hardcoded to one guard name pre-`bbdde6bb`), reused for both `resolveLegacyInstanceBlock` and `resolveIncompleteMigrationBlock`; still an AST check against the real `emain.ts` source (not a mock), and still asserts strict ordering (`legacyIdx < guardIdx < srvIdx`). Startup order is intact.
- Full vitest suite run 3x from a clean worktree: 303/303 passed every run, 33/33 test files, no flake observed.

## Completion
**Status:** COMPLETE
**Findings:** 0 (none by severity — all 11 fix commits confirmed red-on-revert; all 3 changed-test guards re-verified intact; no flake across 3 full-suite runs)
**Scope covered:** all 11 fix commits in `60f923d2..72b21bd9` (frontend: `53b80984`, `dae0b3f2`, `cb1f7477`, `b79cd8c8`, `1bdf3091`, `935afc22`, `ae7d436b`, `90f7f449`; migration: `bbdde6bb`, `7d066154`, `149d275f`). `cd064807` (docs) and `72b21bd9` (prettier-only) excluded — no behaviour to control.
**Not checked:** did not re-audit round-1 through round-4 fixes (out of scope per brief — those rounds' own reports already re-verified them); did not attempt macOS-native execution (environment limitation noted in round-4 memory, unchanged).
