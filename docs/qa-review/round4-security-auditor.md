# round4-security-auditor report
**Target:** `git diff 816fb3d8 cd464aff` on `qa/fleet-2026-09-22` (RemoteTerm, third fix wave)
**Started:** 2026-09-22T00:00:00Z
**Status:** IN PROGRESS

## Findings
<!-- appended one at a time, as found -->

_None._

## Verified OK
<!-- appended as checked -->

- `emain/emain-platform.ts:196-225` (`checkLegacyInstanceRunning`): `pid` is derived from `parseInt` on the `SingletonLock` symlink target and gated by `Number.isInteger(pid) && pid > 0` (:212-215) *before* it reaches `readProcessExecutable(pid)` (:226) or `process.kill(pid, 0)` (:220) — a crafted lock target with junk after the last `-` is rejected at :213-215 and never reaches either call.
- `emain/emain-platform.ts:170-188` (`readProcessExecutable`, new): Linux path is `readlinkSync(\`/proc/${pid}/exe\`)` with `pid` already a validated positive integer — no shell, no traversal surface (string interpolation of a bounded integer). macOS path is `execFileSync("ps", ["-p", String(pid), "-o", "comm="], {...})` — fixed argv array via `execFileSync`, not a shell string; `child_process.execFileSync` does not invoke `/bin/sh` so no metacharacter injection is possible even if `pid` were attacker-influenced (it is not, by the check above). Both paths wrapped in try/catch, returning `null` on any read failure (deleted binary, EPERM cross-uid, unsupported platform) rather than throwing — confirmed this also fixes the round3 `Verified OK` note about the `EPERM` edge case: that path now falls through to `readProcessExecutable` failing too (a foreign-uid `/proc/<pid>/exe` is normally unreadable), landing on `confirmedRunning: false` (:228) instead of round3's observed false-positive `confirmedRunning: true`.
- `emain/emain-platform.ts:120-123` (`MigrationInProgressFileName`) and its only three call sites (:317, :414, :430): the marker's on-disk content (`merging-from:${spec.source}\n<timestamp>\n`, written at :415) is never read back anywhere in the diff — `resumeMerge` is decided purely by `existsSync` (:317), so a planted marker with arbitrary content cannot redirect `mergeTree`'s source/dest, which stay the fixed `spec.source`/`spec.dest` for that root. Confirmed by `grep -n MigrationInProgressFileName emain/*.ts` returning only the const, the `existsSync` check, and the write/unlink pair — no `readFileSync` on this path.
- Same three sites: `resumeMerge` (:317) does bypass `spec.sourceSkipReason?.()` (the `wave.lock`-presence check, :446-448) and `spec.canMergeIntoDest?.(destEntries)` (:346) when true. This is deliberate, not a gap: the code comment at :117-119 states the marker exists precisely because a first partial merge can already have moved `wave.lock` out of `spec.source` (so re-checking it would wrongly abort the resume) and can already have populated `spec.dest` with entries `canMergeIntoDest` wouldn't otherwise accept (the previously-merged legacy files). The bypass cannot cause data loss: `mergeTree` (:395-405, unchanged by this diff) still gates every individual move on `!existsSync(destPath)` before `renameSync`, so a resumed merge still cannot overwrite a same-named entry in `dest` (e.g. a live db `remoteterm` has since created) — it can only place additional, non-colliding legacy files there. No new precondition beyond the standing "attacker can already write to `spec.dest`" class carried from round1-3.
- `package.json`, `package-lock.json`: `happy-dom`, `@testing-library/dom`, `@testing-library/react`, `@testing-library/user-event` are all under `devDependencies` only (verified via `json.load`/inspection, not grep-on-text). `grep -rln "happy-dom\|@testing-library" frontend/ --include="*.ts" --include="*.tsx" | grep -v "\.test\."` returns no results — nothing outside `*.test.ts(x)` imports them, so they don't reach the Vite/Electron production bundle.
- `frontend/app/view/sourcecontrol/action-error.tsx` (new), `sourcecontrol-model.ts` (`actionErrorAtom`, `dismissActionError`): the new error banner renders `{error}` as JSX text content (:16), never via `dangerouslySetInnerHTML` or `innerHTML` — React escapes it, so arbitrary git stderr/exception text reaching `actionErrorAtom` (e.g. `Failed to revert hunk: ${e?.message}`, `sourcecontrol-model.ts:400`) cannot inject markup even though the message content is not attacker-controllable through any boundary this diff touches.
- `emain/emain-platform.ts:361-368` (`app.setName`/`ElectronUserDataPath` refactor): `["remoteterm", "electron"].join("/")` is a fixed literal array, not reachable from user input; purely a readability refactor of round1-3's already-reviewed userData path, no behaviour change.
- `frontend/app/onboarding/onboarding-command.tsx`, `generalcontent.tsx`, `remotetermconfig-model.ts`: diff is UI-only (spin-button/number-input focus and comment fixes per commit messages), no new data flow into `eval`, shell, `innerHTML`, or file paths; skimmed, no security-relevant surface.
- `RENAME_PLAN.md`, `public/logos/remoteterm-logo.png`: docs and a static asset, no action needed.

## Not re-verified this round
- Round3's `R3-SEC-1` (symlink-following in `mergeTree`/`mergeDataRoot`, config root's unconditional `canMergeIntoDest`) is untouched by this diff (only additions around it) — still open at the same Low severity, same standing precondition, not restated as a new finding.

## Completion
**Status:** COMPLETE
**Findings:** 0 (0 Critical, 0 High, 0 Medium, 0 Low)
**Not checked / out of scope:** No live-app runtime verification (live-system rule), no `go build`. Windows migration path not re-examined (unaffected by this diff — `checkLegacyInstanceRunning` still returns `null` on `win32` before reaching new code). Round3's `R3-SEC-1` left open, unchanged by this diff, not re-litigated.
