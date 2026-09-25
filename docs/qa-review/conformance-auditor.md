# Conformance Audit: RemoteTerm rename branch vs RENAME_PLAN.md

Branch: `qa/fleet-2026-09-22`, HEAD `93aaefc3`.
Source of truth: `RENAME_PLAN.md` (1656 lines, Prerequisites "RESOLVED 2026-09-20" section +
Phases 1-6) and `RENAME_ALLOWLIST.md` (committed residual-sweep artefact, dated 2026-09-22).

**Findings:** CRITICAL: 0 | HIGH: 3 | MEDIUM: 1 | LOW: 0 | OK: 9

## Traceability Matrix
| ID | Claim (short) | Source (file:line) | Code (file:line) | Status | Severity |
| -- | ------------- | ------------------- | ----------------- | ------ | -------- |
| CONF-1 | Prerequisites decision 5: remote-host `.waveterm` state renames | RENAME_PLAN.md:439-443 | pkg/remotetermbase/wavebase.go:139,141,142,264,569; pkg/wshrpc/wshremote/wshremote.go:134 | MISSING | HIGH |
| CONF-2 | Phase 2 verification: no path-getter call at module scope outside emain-platform.ts | RENAME_PLAN.md:814-818, 893-895 | emain/emain-log.ts:96-97,106 | PARTIAL | MEDIUM |
| CONF-3 | Prerequisites decision 8: new RemoteTerm artwork for build/icon.* | RENAME_PLAN.md:457-465 | build/icon.icns, build/icon.ico, build/icons/*.png (unchanged) | MISSING | HIGH |
| CONF-4 | Phase 1 step 1: user-visible strings rebranded | RENAME_PLAN.md:504-540 | frontend/app/view/remotetermconfig/generalcontent.tsx:66,75,120,196,560,758,767,775 | MISSING | HIGH |
| OK-1 | Package rename table (11 packages) | RENAME_PLAN.md:412-429 | `pkg/remotetermbase`, `remotetermobj`, `remotetermapp`, `remotetermapputil`, `remotetermappstore`, `remotetermjwt`, `rtcore`, `rtconfig`, `rtstore` all present; `wshrpc`/`wshutil` correctly unchanged | OK | - |
| OK-2 | Module path -> github.com/LannCo/remoteterm, tsunami moves with root | RENAME_PLAN.md:412,435-438 | go.mod:1, tsunami/go.mod:1 | OK | - |
| OK-3 | `db_wave_file` frozen | RENAME_PLAN.md:433-434 | pkg/filestore/blockstore_dbops.go (9 literals), db/migrations-filestore/000001_init.*.sql | OK | - |
| OK-4 | `wavesrv`/route dual-accept for one release | RENAME_PLAN.md:444-446 | pkg/wshutil/wshrouter.go:25-29,619-620 (`LegacyDefaultRoute` normalized in `getLinkForRoute`); electron-builder.config.cjs, emain-platform.ts, emain-remotetermsrv.ts binary rename | OK | - |
| OK-5 | Override env vars (CONFIG/DATA/HOME) read-new-fallback-old | RENAME_PLAN.md:706-709 | emain/emain-platform.ts:53-58 | OK | - |
| OK-6 | Session-scoped vars dual-write (TABID/BLOCKID/WORKSPACEID/CLIENTID/CONN/JOBID/JWT/PUBLICKEY) | RENAME_PLAN.md:710-723 | pkg/remotetermbase/wavebase.go:59-93 (`SetDualEnv`/`GetEnvNewOrLegacy`), pkg/blockcontroller/blockcontroller.go:615-633, pkg/jobcontroller/jobcontroller.go:1624-1625 | OK | - |
| OK-7 | `feature:waveappbuilder` -> `feature:rtappbuilder` | RENAME_PLAN.md:447-451 (Prereq 7) | pkg/rtconfig/metaconsts.go:21, settingsconfig.go:52, frontend/app/view/remotetermconfig/generalcontent.tsx:143, widgets.tsx:437, gotypes.d.ts:1419 | OK | - |
| OK-8 | `envutil` prefix guard widened to both prefixes | RENAME_PLAN.md:729-732 | pkg/util/envutil/envutil.go:121 | OK | - |
| OK-9 | Shell-integration script content + output filenames renamed (44 occurrences) | RENAME_PLAN.md:734-739, 247-256 | pkg/util/shellutil/shellintegration/*.sh (44 REMOTETERM_ hits matching plan's per-file count), shellutil.go:287-292,419-425 (`remoteterm.fish`/`remotetermpwsh.ps1`) | OK | - |

## Gaps

#### [HIGH] Remote-host `.waveterm` state was resolved "renames" but never renamed
- **ID:** CONF-1
- **Category:** missing
- **Source says:** "5. **Remote-host `.waveterm` state:** **renames.** Expect the self-healing reinstall path (one permission prompt per host on first post-rename connect) rather than a hard break..." — RENAME_PLAN.md:439-443
- **Code does:** `const RemoteWaveHomeDirName = ".waveterm"`, `const RemoteFullWshBinPath = "~/.waveterm/bin/wsh"`, `const RemoteFullDomainSocketPath = "~/.waveterm/wave-remote.sock"` — pkg/remotetermbase/wavebase.go:139,141,142; two more inline `.waveterm` literals at wavebase.go:264 (`~/.waveterm/client/%s/waveterm.sock`) and :569 (`filepath.Join(homeDir, ".waveterm", "jobs")`); pkg/wshrpc/wshremote/wshremote.go:134 hardcodes `"~/.waveterm/bin/wsh"` directly (not even via the constant).
- **Impact:** Every SSH/WSL host this app installs to keeps writing/reading `.waveterm`, `wsh`, and the domain socket under the old name indefinitely — the rebrand never reaches remote hosts at all, not even the one-time reinstall friction the plan explicitly signed up for. RENAME_ALLOWLIST.md's Phase 6 sweep (category d) only excuses this residue "if the decision was 'do not migrate remote hosts'" — the recorded decision is the opposite, so under the plan's own classification scheme this is a genuine miss, not accepted residue, and the committed `hits_strict.txt`/`hits_loose.txt` sweep artefacts don't surface it because none of these five sites contain the literal substring "wave" in a way the sweep's file-level grep would flag as unexpected (the constant *names* still say "Wave*", matching plenty of allowlisted Go-generated-type residue, masking the fact that the *values* were supposed to change too).
- **Recommendation:** Fix the code — rename the four constants' values (and the two literal duplicates at wavebase.go:264/569, plus wshremote.go:134's inline copy) to the new remote-host directory/socket names per decision 5, or, if the repo owner has since decided against this migration, correct RENAME_PLAN.md's Prerequisites decision 5 and RENAME_ALLOWLIST.md's category (d) note to record "do not migrate" instead of leaving a "renames" decision unimplemented.

#### [MEDIUM] Phase 2's own verification gate does not hold against the shipped tree
- **ID:** CONF-2
- **Category:** partial
- **Source says:** "Add to Phase 2 verification (below) a check that no file imported by `emain.ts` calls `getWaveDataDir()`/`getWaveConfigDir()` at module scope going forward (`grep -n "getWaveDataDir()\|getWaveConfigDir()" emain/*.ts`, confirm every hit outside `emain-platform.ts` itself is inside a function body, not at top level)" — RENAME_PLAN.md:814-818, restated as a required Phase 2 verification step at :893-895.
- **Code does:** `emain/emain-log.ts:96-97` calls `rotateLogIfNeeded()` (which itself calls `getRemoteTermDataDir()` at emain-log.ts:69) inside a top-level `try` block, and `emain/emain-log.ts:106` calls `getRemoteTermDataDir()` directly inside the top-level `loggerTransports` array initializer — both are module-scope calls, not calls made from inside an on-demand function body called later.
- **Impact:** Running the plan's own verification command as written today would report a failing gate. The tree is not actually broken at runtime — `emain-log.ts:8` imports `getRemoteTermDataDir` from `emain-platform.ts`, and ESM import evaluation runs `emain-platform.ts`'s own top-level `performDataDirMigration()` (emain-platform.ts:251) before `emain-log.ts`'s top-level code executes — but that safety comes from import-graph ordering the plan's grep-based gate was never designed to verify, not from the module-scope discipline the gate is checking for. The gate's pass/fail signal is unreliable for its stated purpose; a future refactor that reorders `emain-log.ts`'s imports could silently reintroduce the exact race the plan spent three iterations designing this placement to avoid, with no gate to catch it.
- **Recommendation:** Fix the code (move the three `getRemoteTermDataDir()` calls in emain-log.ts into function bodies invoked only after `emain.ts`'s own import chain completes) or fix the plan's verification wording to explicitly permit "module-scope call in a file that itself imports the getter from emain-platform.ts, transitively guaranteeing ordering" as a second passing case.

#### [HIGH] App icon/logo artwork unchanged; residual-sweep documentation omits it
- **ID:** CONF-3
- **Category:** missing
- **Source says:** "**App icon/logo artwork:** **ship new RemoteTerm artwork everywhere**... extended to every load-bearing path named above... **and** `build/icon.icns`, `build/icon.ico`, `build/icons/{16,32,48,64,128,256,512}x512.png`... No rights-determination branch needed. Blocking for Phase 1, Phase 5..., and Phase 6" — RENAME_PLAN.md:457-465 (Prerequisites decision 8).
- **Code does:** `build/icon.icns` is 460965 bytes (Mac OS X icon, `ic12` type) on this branch — byte-identical to the pre-rename size. A commit that does replace these exact files (`673d1d72`, "chore(rename): RemoteTerm logo assets + Phase 1 UI chrome rename", `build/icon.icns` 460965→75780 bytes, plus `build/icon.ico` and all seven `build/icons/*.png`) exists in the repo's object store but is only reachable from `fork/feat/files-widget` and `fork/odds-and-ends` — `git branch -a --contains 673d1d72` does not list `qa/fleet-2026-09-22`, and `git merge-base qa/fleet-2026-09-22 673d1d72` returns the pre-rename commit `6d6128e5`, confirming it was never merged into this branch's history.
- **Impact:** The actual dock icon, Windows taskbar/installer icon, and Linux AppImage/deb icon a real user sees remains unchanged Wave Terminal artwork — the plan's own text calls this "the most visually obvious 'this is still Wave' signal... the one most likely to draw a trademark objection" under Apache-2.0 §6. Separately, RENAME_ALLOWLIST.md's Phase 6 "flagged, deliberately NOT actioned" section (the artefact whose job is to enumerate exactly this kind of known gap) lists `assets/wave-{dark,light}.png`, `public/logos/wave-*.png`, and `tsunami/frontend/public/wave-logo-256.png` as undone — correctly, matching their unchanged on-disk sizes — but does not mention `build/icon.*` at all, even though it's in the same undone state and is the single item the plan calls "blocking" for three separate phases. The residual-sweep's own documentation under-reports this gap.
- **Recommendation:** Fix the code — merge (or cherry-pick) the icon portion of `673d1d72` into this branch, and add `build/icon.icns`/`build/icon.ico`/`build/icons/*.png` to RENAME_ALLOWLIST.md's flagged-not-actioned list in the meantime so the gap is at least visible to whoever reads that artefact next.

#### [HIGH] Settings > General descriptions still read "Wave"/"Wave Terminal"
- **ID:** CONF-4
- **Category:** missing (post-sweep regression)
- **Source says:** Phase 1 step 1 covers "About modal, quit-confirm dialog, app menu, onboarding screens... window title... change display text only" as in-scope user-visible strings, and Phase 1's verification (RENAME_PLAN.md:627-639) requires a case-insensitive `wave` grep across the touched surfaces to return only allowlisted hits.
- **Code does:** `frontend/app/view/remotetermconfig/generalcontent.tsx` (the General-settings tab's own description strings, rendered directly in the Settings UI) still reads: line 66 `"Shows a confirmation dialog before quitting Wave Terminal. Requires an app restart."`; line 75 `"...opens your most recent Wave window."`; line 120 `"...shown when Wave is running under architecture translation..."`; line 196 `"...instead of Wave's overlay..."`; line 560 `"Opens web links inside Wave's web widget..."`; lines 758/767/775/783 (four RTApp-Builder-adjacent settings) `"...building Wave apps"` / `"...Wave apps"` (x3).
- **Impact:** A user opening Settings > General in an otherwise fully-rebranded app (the containing directory and every component/type in it were renamed `waveconfig`→`remotetermconfig`) reads "Wave"/"Wave Terminal" in eight description strings — exactly the "we shipped that, right?" surface Phase 1 exists to close. This is not covered by any RENAME_ALLOWLIST.md category: it postdates the Phase 6 residual sweep entirely — `git log` shows `716bd7c8` (the sweep commit, which produced the committed `hits_strict.txt` this audit diffed against) landed before `aa157a1a` ("fix(remotetermconfig): feature:waveappbuilder -> feature:rtappbuilder in general settings"), the commit that introduced or left these strings in their current form — so the sweep's "done" declaration predates this file's current content and never re-ran against it.
- **Recommendation:** Fix the code — reword the eight strings to "RemoteTerm"/"RTApp", and re-run the Phase 6 `hits_strict.txt`/`hits_loose.txt` sweep against current HEAD before the next "done" declaration, since at least one commit has landed since the committed baseline was captured.

## Residuals
No additional un-allowlisted, un-flagged residue found beyond CONF-1/3/4 above in a fresh
`git ls-files -z '*.go' '*.ts' '*.tsx' '*.md' '*.yml' '*.yaml' '*.json' | xargs -0 grep -ilZ wave`
diffed against the committed `hits_strict.txt` — the only other deltas were `docs/qa-review/mechanical-lint.md` (this audit's sibling report, not project content) and the three `remotetermconfig/` files covered by CONF-4/OK-7.

## Ambiguities
- `wsh wavepath` subcommand verb (RENAME_PLAN.md:1281-1291, Phase 4 step 12): the plan explicitly requires an owner decision ("decide explicitly whether the subcommand verb itself renames... do not let it default silently") but no resolution is recorded anywhere in the Prerequisites' "RESOLVED" section. Code still has `Use: "wavepath {config|data|log}"` (cmd/wsh/cmd/wshcmd-wavepath.go:18) — consistent with "still undecided, default to unchanged," but that reading isn't stated anywhere, so I can't confirm it's the *intended* default versus an open item nobody has returned to.

## Verified Conformant
Package/module rename (11-package table + tsunami), `db_wave_file` freeze, `wavesrv`
binary+route dual-accept, override-var read fallback, session-scoped-var dual-write
(`SetDualEnv`/`GetEnvNewOrLegacy`), `feature:rtappbuilder` key end-to-end (Go + generated TS +
UI), `envutil` prefix guard, shell-integration script rename (all 44 occurrences across the
correct 6-file set), CNAME→`docs.rterm.dev`, and the per-root data-dir migration shim (3 local
roots + Windows `userData` 4th root, per-root override short-circuit with distinct
`no-migration-needed:override` marker, marker-written-after-move gate placed as a genuinely-first
top-level statement, ENOENT concurrency handling) are all implemented closely and correctly
against the plan's (unusually detailed) corrected text — this is a materially more careful
implementation than the plan's own round-1/round-2 audit findings suggested was likely. The
`getRemoteTermHomeDir()` legacy `"wave.lock"` literal (emain-platform.ts:295,299,306) is
correctly frozen per the plan's explicit instruction, not a residual miss — initially flagged
during this audit and disconfirmed by tracing `WaveLockFile`'s actual write-target
(pkg/remotetermbase/wavebase-posix.go:18) against the plan's own "keep matching `wave.lock`
forever" requirement (RENAME_PLAN.md:701-704).
