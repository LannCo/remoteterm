# mechanical-lint report

**Target:** `qa/fleet-2026-09-22` @ `58ea9e51`, files changed vs `origin/main` (merge-base `6d6128e5`, 468 files)
**Started:** 2026-09-22T19:51:56Z
**Status:** COMPLETE

## Toolchain

All present: `npx tsc`, `npx vitest`, `npx eslint` (`eslint.config.js`), `npx prettier` (`prettier.config.cjs`), `/home/owner/go-toolchain/bin/go` (`gofmt`, `go vet`, `go test`), `/home/owner/go/bin/golangci-lint` (v2 config `.golangci.yml`), `shellcheck`, `shfmt`. No tool configs changed vs `origin/main`.

Method: every finding at HEAD was compared against the same check run on an `origin/main` scratch worktree (`git worktree add`, node_modules symlinked), matched by pre-rename path via `git diff -M`, rule and message with Wave/RemoteTerm identifiers normalised. golangci-lint was run with `--max-issues-per-linter=0 --max-same-issues=0`: the default caps (50/linter, 3/same) truncate the list and make a HEAD-vs-origin diff meaningless.

## Baseline

HEAD `58ea9e51`, 461 changed files still present (130 JS/TS, 231 Go, 6 shell).

| Check                                               | HEAD                                                         | origin/main                                    | Introduced by branch                                                                                                                                                          |
| --------------------------------------------------- | ------------------------------------------------------------ | ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `tsc --noEmit`                                      | 18 errors, exit 2                                            | 19 errors                                      | 0                                                                                                                                                                             |
| `vitest run`                                        | 23 files, 178/178 pass                                       | -                                              | -                                                                                                                                                                             |
| `eslint` (changed files)                            | 0 errors, 74 warnings                                        | 101 warnings, 2 errors (same files)            | 0                                                                                                                                                                             |
| `prettier --check` (changed files)                  | 98 files                                                     | 73 of those 94 pre-existing files already fail | 25 files (4 new + 21 modified)                                                                                                                                                |
| `gofmt -l`                                          | 3 files, all generated (excluded)                            | 38 files                                       | 0 (branch already fixed 35)                                                                                                                                                   |
| `go vet ./...`                                      | exit 0                                                       | -                                              | 0                                                                                                                                                                             |
| `golangci-lint` (uncapped)                          | 733 (errcheck 613, staticcheck 103, govet 14, ineffassign 3) | 725                                            | 8 real (see Judgment calls); 5 are artefacts (4 SA4023 "related information" lines whose text embeds the renamed module path, 2 govet hits in `node_modules/flatted/golang/`) |
| `go test ./...`                                     | 28 packages ok, exit 0                                       | -                                              | -                                                                                                                                                                             |
| `shellcheck` / `shfmt` (6 shellintegration scripts) | 48 findings, 6 shfmt diffs                                   | identical counts                               | 0                                                                                                                                                                             |

## Auto-fixes

- **prettier -w, 27 files** (`68692897`): only files that pass at `origin/main` and fail at HEAD, i.e. drift the branch introduced (longer identifiers pushing lines past width, import order after module renames). Includes `emain/emain-platform.ts`, `emain/emain-tabview.ts`, `emain/emain-web.ts`, `electron-builder.config.cjs`, `frontend/app/view/remotetermconfig/remotetermconfig{.tsx,-model.ts,-model.widgets.test.ts}`, three `onboarding-upgrade-v01*.tsx`, `CLAUDE.md`, `.github/copilot-instructions.md` (table padding). Full list in the commit.
- **Reverted:** `RENAME_PLAN.md`, `RENAME_ALLOWLIST.md`. Prettier's markdown pass corrupts inline code spans that wrap across lines (`RENAME_PLAN.md:102-114` turned `` `WCLOUD_*` `` into `WCLOUD*\*` and dedented a list item; `RENAME_ALLOWLIST.md:27` dedented a wrapped `` `task generate` `` span) and is not idempotent on `RENAME_PLAN.md`. Left unformatted.
- **eslint --fix:** not applied. All 29 fixable warnings (`prefer-const`, 21 of them in `frontend/app/view/vdom/vdom-model.tsx`) exist identically at `origin/main`.
- **gofmt -w:** nothing to do. The only `gofmt -l` hits are the excluded generated files `pkg/remotetermobj/metaconsts.go`, `pkg/rtconfig/metaconsts.go`, `pkg/wshrpc/wshclient/wshclient.go`.
- Not reformatted on purpose: 73 changed files that already fail prettier at `origin/main` (e.g. `frontend/app/store/wshclientapi.ts`, generated). Reformatting them would widen the fork's diff against upstream for no behavioural gain; counted under Pre-existing.

After fixes: tsc 18 errors (unchanged, all pre-existing), vitest 178/178, eslint 0 errors / 74 warnings (unchanged), prettier clean on all branch-introduced files except the two reverted markdown files. No Go files touched, so Go results equal baseline.

## Hand fixes

_None._ No branch-introduced undefined names, broken imports, type errors or unreachable code: tsc, eslint and `go vet` findings on changed files all pre-date the branch.

## Judgment calls for fleet

golangci-lint findings the branch introduced (all errcheck). Each matches the surrounding upstream idiom; none is clearly a bug.

1. `cmd/server/main-server.go:147-159` - `os.Unsetenv` return ignored (13 calls vs 7 upstream; the branch added the 6 `Legacy*` names). Upstream ignores the same call. Fine unless the fleet wants a failure logged.
2. `pkg/jobcontroller/jobcontroller.go:2414` - `panichandler.PanicHandler` return ignored in the new `RestartStreaming` output-loop goroutine. Same as the 8 other call sites in the file (e.g. `:1682`).
3. `pkg/remote/conncontroller/connmonitor.go:209` - `cm.SendKeepAlive()` return ignored on input notify. `SendKeepAlive` (`:132`) always returns `nil` and logs its own errors, so the signature's `error` looks vestigial; worth a look alongside the configurable-thresholds change.

Pre-existing items worth a look while those files are open (not introduced here):

4. `cmd/wsh/cmd/wshcmd-connserver.go:475,489,502` - staticcheck SA4023 "comparison always true": `serverRunRouter`/`serverRunRouterDomainSocket`/`serverRunNormal` never return a nil interface, so the `err != nil` checks after them always fire or are typed-nil traps.
5. `pkg/util/utilfn/streamtolines.go:61` - same SA4023 pattern on `StreamToLines`.
6. `frontend/app/view/webview/webview.tsx:231,242,254` - empty `catch` blocks (`no-empty`).
7. `frontend/app/view/preview/preview-streaming.tsx:36` - four unused destructured values on one line; possible missing-use.

## Pre-existing

Counted, not fixed (all also present at `origin/main`):

- tsc: 18 errors - `frontend/preview/previews/processviewer.preview.tsx` (15, missing `numthreads`), `frontend/preview/mock/defaultconfig.ts:11`, `frontend/preview/mock/preview-electron-api.ts:4`, `frontend/app/view/term/term.tsx:314` (`overviewRuler`). `origin/main` has these plus one in `preview-directory.tsx` that the branch fixed.
- eslint (changed files): 74 warnings - 42 `no-unused-vars`, 29 `prefer-const`, 3 `no-empty`.
- prettier: 73 changed files already failing upstream.
- golangci-lint: 719 matched to `origin/main` (errcheck ~600, staticcheck ~100, govet 12, ineffassign 3); 5 more are artefacts (SA4023 "related information" lines carrying the renamed module path; `node_modules/flatted/golang/pkg/flatted/flatted.go:35,62`, which golangci-lint scans because the Go module root contains `node_modules`).
- shellcheck: 48 findings across `pkg/util/shellutil/shellintegration/*.sh`; shfmt diffs on all 6. Identical at `origin/main`. `fish_wavefish.sh` and `pwsh_wavepwsh.sh` are not POSIX shell, so shellcheck output there is noise.

## Completion

**Status:** COMPLETE

- Findings: 0 hand fixes; 1 auto-fix commit (27 files); 3 branch-introduced judgment calls, 4 pre-existing judgment calls.
- Lint and tests on changed files: no branch-introduced tsc/eslint/go vet/gofmt/golangci-lint errors remain except the 3 errcheck judgment calls; `RENAME_PLAN.md` and `RENAME_ALLOWLIST.md` still fail prettier by design.
- Not checked: runtime behaviour (out of scope; no app launch).
