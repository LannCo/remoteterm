# Migration Plan: Wave Terminal internals -> RemoteTerm (full rebrand)

**Assessment:** Safe to start once the Prerequisites section's blocking decisions are resolved.
Phase 1's string-level edits and Phase 2's env-var/data-dir migration are lower-risk than
Phases 3-4 but are **not** simple cosmetic work — a round-1 plan/requirements audit (2026-09-20)
found the data-dir migration shim's original marker-validation and gate logic would have
silently no-op'd or data-raced on real installs, and Phase 1's original file list missed several
functionally-significant categories (`emain/` menu strings, the WaveApp Builder sub-feature,
`.github/FUNDING.yml`, the Apache-2.0 `NOTICE` file). Both are corrected in the phase text below;
do not execute from an earlier cached copy of this plan. Phases 3-4 (Go module path +
package/type renames, frontend internal module renames) are mechanical but touch roughly 204 Go
files plus 11 non-`.go` module/template files (`tsunami/`, previously missed) and a disputed
143-269 frontend files (re-run the count before dispatch, see Scope). **Phase 3 only** must be
done as a single atomic commit with a scripted, `git ls-files`-scoped sed pass, not hand-edited
— that is what dominates Phase 3's risk. **Phase 4 is explicitly not scripted the same way**:
step 10 requires file-by-file `@/` import-path substitution rather than a blind sed, since the
literal string also appears inside comments/doc-strings that need rewording, not just
path-substitution — do not apply Phase 3's "single scripted sed pass" method to Phase 4; an
earlier draft of this paragraph implied both phases share a method, which they don't. The
`frontend/types/gotypes.d.ts` regeneration
dependency (`task generate`) creates a hard ordering constraint between the Go rename and the
frontend rename that the phase order below is built around. Additional out-of-scope-until-decided
surfaces — remote-host `.waveterm` state, the `wavesrv` binary/route identifier, `tsunami`'s
module path — now have explicit resolution requirements in Prerequisites; none of them are
optional background reading.

**Confidence:** 4 (scope numbers below are from `grep`/`glob` run against the live tree
2026-09-20, not from the read-only inventory agent's summary alone; upstream migration-guide
research doesn't apply here since this is an internal rename, not a dependency/framework
upgrade, so there's no external doc to cross-check against — confidence capped at 4 rather
than 5 for that reason). Round-1 plan/requirements audit (2026-09-20) found and corrected 4
CRITICAL, 10 HIGH, and 9 MEDIUM findings against the original draft — primarily missed scope
(remote-host state, `wavesrv`, `emain/`, the WaveApp Builder sub-feature, `tsunami`'s separate
Go module) and unsound verification logic in Phases 2 and 3 (a migration shim that couldn't
actually validate real data, and exit criteria that would pass on a tree that doesn't compile).
**Round 2** (2026-09-20, same day) re-audited the round-1 rewrite itself and found 2 CRITICAL, 11
HIGH, and 11 MEDIUM further findings — some newly-introduced *interactions between* round 1's
independent section-by-section fixes (e.g. the data-dir migration's new env-var fallback and its
new abort-on-unmigratable-destination logic combined to make the app refuse to start for users
with an override var set), some genuine scope this document still hadn't covered (app icon/logo
trademark, the WaveApp Builder's Go-side engine content, `.kilocode/rules/rules.md`), and several
verification commands that were syntactically or scope-wise broken (a Phase 4 grep pattern that
matches zero files even before any rename work starts, `{{.GO}}` Taskfile-template syntax quoted
as a literal shell command). All are corrected in the phase text below. The corrections are
integrated into the phase text below, not appended as a separate errata section — treat this
document as the current, twice-corrected plan, not either the pre-audit or the post-round-1
draft.

### Scope

- Go files importing `github.com/wavetermdev/waveterm/...`: **204 files, 793 import-path
  occurrences** (`grep -rc "wavetermdev/waveterm" --glob '*.go'`). Of these, **16 are `_test.go`
  files.**
- Go files under `pkg/` alone: **212 files** (`glob pkg/**/*.go`; some have zero wave-string
  hits and don't need touching beyond the import path).
- Frontend `.ts`/`.tsx` files containing a case-insensitive `wave` token: figure under dispute
  — re-audit reproduced **143 files inside `frontend/`** (1,350 occurrences) against
  `grep -il wave --glob '*.{ts,tsx}' frontend/`, not the 269 originally cited here. Repo-wide
  (adding `emain/`, `tsunami/frontend/`, `docs/`) is 167. Neither matches 269; the Go figure
  from the same scan reproduced exactly, so this looks like a tooling artefact from the
  original scan rather than a transcription error, but the direction of the error overstates
  Phase 4's size. **Re-run the exact command before Phase 4 sizing/dispatch and record which
  directories were included**, so the eventual file-list the executor works from is generated
  fresh rather than trusted from this number. Of the reproduced set, **6 are `.test.ts(x)`
  files.**
- `emain/` (Electron main-process TypeScript, not currently assigned to any phase — see
  Prerequisites): **16 files, ~430 case-insensitive `wave` hits**, largest single-file counts
  `emain-window.ts` (116), `emain-tabview.ts` (71), `emain-platform.ts` (58),
  `emain-wavesrv.ts` (49), `emain.ts` (39), `emain-menu.ts` (29).
- `frontend/builder/` and `frontend/app/view/tsunami/` (the "WaveApp Builder" sub-feature —
  see Prerequisites): not currently assigned to any phase. Contains ~15+ user-facing strings
  (document titles, menu labels, headings, body copy) plus a WPS event-type literal
  (`"waveapp:appgoupdated"`) and a log-filename pattern in `emain/emain-log.ts`.
- `wavesrv` binary name + RPC route identifier (`DefaultRoute`/`ConnHostWaveSrv` in
  `pkg/wshutil/wshrouter.go` and `pkg/remote/connparse/connparse.go`) — not currently assigned
  to any phase; same class of decision as `wsh`'s binary name (Phase 4 step 12) but with an
  added wire-protocol dimension. See Prerequisites.
- Remote-host `.waveterm` state (`RemoteWaveHomeDirName`, `RemoteFullWshBinPath`,
  `RemoteFullDomainSocketPath`, `RemoteDomainSocketBaseName` in `pkg/wavebase/wavebase.go`,
  read by `pkg/wshrpc/wshremote/wshremote.go`, `pkg/remote/conncontroller/conncontroller.go`,
  and shell-integration scripts) — a separate constant family from the local data-dir Phase 2
  migrates, installed on every machine the user has SSHed into. Not assigned to any phase; no
  migration path exists for it. See Prerequisites.
- `tsunami/`: a second Go module (`github.com/wavetermdev/waveterm/tsunami`, root `go.mod`
  requires it at `v0.12.3` with a local `replace`), 8 demo-app `go.mod` files under
  `tsunami/demo/`, and 2 runtime-consumed Go source templates
  (`tsunami/templates/app-main.go.tmpl`, `app-init.go.tmpl`) copied into
  `dist/tsunamiscaffold/` at build time. None of these are `*.go` files, so Phase 3's
  `--include=*.go` verification grep is structurally blind to them. See Prerequisites and
  Phase 3 step 9.
- Files with any wave-token hit repo-wide (loose scan, prior inventory agent):
  **~495 files**, ranging from `pkg/wshrpc/wshserver/wshserver.go` (128 hits) down to
  single-hit files. Full list: `hits_loose.txt` / `hits_strict.txt` in the inventory agent's
  scratchpad (paths in the assigning message) — stale by file-count only, not by shape; re-run
  before Phase 3 execution. This loose scan predates the `emain/`, `tsunami/` module-file, and
  `frontend/builder/` gaps identified above — treat ~495 as a floor, not a ceiling.
- `wave://` custom URI scheme, Monaco-side: **2 files**
  (`frontend/app/monaco/monaco-react.tsx`, `schemaendpoints.ts`) — confirmed via independent
  re-check; no OS-level protocol-handler registration exists in `electron-builder.config.cjs`
  or `package.json`, so this is not also an OS-handler registration risk beyond what Phase 4
  step 11 already covers.
- CI/build files referencing wave-branded infra: **`Taskfile.yml`** (WCLOUD_* endpoints,
  `RELEASES_BUCKET: dl.waveterm.dev`, `waveterm_*.snap` glob, `~/.config/waveterm-dev` dev
  tasks, `APP_NAME: "Wave"` (line 7), `WINGET_PACKAGE: CommandLine.Wave` (line 15) — **also**
  `build:schema`'s `pkg/wconfig/*.go` source glob and `-ldflags -X main.WaveVersion=...` on two
  build targets, both of which Phase 3's package/identifier renames would silently break even
  though the file itself is out of scope this pass — see Phase 3 step 9's Taskfile
  cross-reference; see Phase 5 step 14 for which of these are deliberately deferred vs. carved
  out as hard exceptions), **`.github/workflows/bump-version.yml`**
  (`WAVE_BUILDER_APPID`/`WAVE_BUILDER_KEY` vars/secrets, `WAVETERM_VERSION` output var),
  **`.github/workflows/build-helper.yml`** (confirmed via grep, not yet read), **`CNAME`**
  (`docs.waveterm.dev`), **`.github/copilot-instructions.md`** (9 hits, same prose-only class
  as the `.kilocode`/`aiprompts` files Phase 1 step 4 covers), **`.github/FUNDING.yml`**
  (`github: wavetermdev`, routes this fork's Sponsor button at the upstream project — needs an
  explicit keep/remove/repoint decision, not silent inheritance).
- `pkg/wconfig/defaultconfig/settings.json`'s `"web:defaulturl"` default value points at
  `https://github.com/wavetermdev/waveterm` — a user-visible default (opened by a new Web
  widget), not a code identifier, and not currently in any phase's file list.
- Two `/tmp`-scoped diagnostic filename constants: `pkg/util/sigutil/sigusr1_notwindows.go`'s
  `DumpFilePath = "/tmp/waveterm-usr1-dump.log"` and
  `cmd/wsh/cmd/wshcmd-connserver.go`'s `/tmp/waveterm-connserver-%d.log` pattern. Cosmetic only
  (ephemeral `/tmp` files, not persisted or user-configured), but plain Go string literals with
  no compatibility concern — in scope for Phase 3's sed pass alongside the package renames,
  rather than left for Phase 6 to catch as a residual.
- Breaking changes from target: **0** — this is an internal rename with no upstream version
  target, so "breaking changes from target" in the usual sense doesn't apply. The functional
  break is self-inflicted: the on-disk data-dir migration in Phase 2 and the Go import-path
  rewrite in Phase 3.

### Prerequisites

**Hard blockers — Phase 3 cannot start until these are resolved.** Round-1 audit found two
genuine naming decisions sitting inside Phase 3 step 9's execution instructions with no
blocking flag (a `<owner>` placeholder in a `sed` command and eleven package directories
marked "or similar"). Both need to be nailed down here, in writing, before any subagent
touches Go code:

- **Exact target module path.** `go.mod`'s module line changes from
  `github.com/wavetermdev/waveterm` to a specific `github.com/<owner>/remoteterm` string — get
  the literal owner/org name from the repo owner. No placeholder may reach the `sed` script.
- **Complete old→new package name table**, not "or similar": `pkg/wavebase`, `pkg/waveobj`,
  `pkg/waveapp`, `pkg/waveapputil`, `pkg/waveappstore`, `pkg/wavejwt`, `pkg/wshrpc`,
  `pkg/wshutil`, `pkg/wcore`, `pkg/wconfig`, `pkg/wstore` — each needs a final directory/package
  name before the sed pass is written, not decided ad hoc while running it. (`pkg/waveai` is
  removed from this list — see Checked and Clear.)
  **Their exported `Wave*` type names (`WaveObj`, `WaveAIStreamRequest`, etc.) are explicitly
  pinned as do-not-rename this pass — a decision, not an oversight.** These types are generated
  into `frontend/types/gotypes.d.ts`/`frontend/app/store/wshclientapi.ts` and consumed *by name*
  in hand-written frontend code with no phase assigned to update those consumers: `WaveObj`
  alone is referenced in 6 hand-written files outside any phase's file list
  (`frontend/app/store/wos.ts`, `services.ts`, `frontend/app/waveenv/waveenv.ts`,
  `waveenvimpl.ts`, `frontend/preview/mock/mockwaveenv.ts`, `tabbar-mock.tsx` — 38 occurrences),
  and the same pattern likely recurs for the other exported types once resolved. Renaming the
  package/directory a type lives in does not require renaming the type's own identifier — package
  and type names are independent decisions in Go, and holding the type names fixed avoids a
  frontend-contamination problem the plan's own phase-dependency structure exists to prevent
  (Phase 3's atomic commit landing a tree whose frontend silently fails `tsc --noEmit`, with the
  fix surfacing only after the revert-costly Phase 3 commit is already merged). If the repo owner
  wants the exported type names renamed too, that decision must include an explicit new step
  (not implied by the package rename) enumerating every hand-written frontend consumer of every
  affected type name and updating it in the same atomic commit, with `npx tsc --noEmit` added as
  a Phase 3 gate — treat this as a materially larger, separate decision from the package rename,
  not a free extension of it.
- **`db_wave_file` is a live on-disk SQL table name, already created on every existing user's
  filestore database — it stays unchanged, full stop, and this must be recorded before Phase 3
  or Phase 6 run.** `db/migrations-filestore/000001_init.up.sql` defines `CREATE TABLE
  db_wave_file (...)`, queried by 9 live SQL string literals in
  `pkg/filestore/blockstore_dbops.go` (`SELECT`/`INSERT`/`DELETE ... FROM db_wave_file`). This
  is a plain Go string literal, not an import path or a Go identifier, so it is untouched by
  Phase 3's import-path sed and untouched by `gopls rename`'s package-clause operation — nothing
  in Phase 3's stated mechanism renames it, which is correct, but it **is** inside a tracked
  `.go` file containing the substring "wave", so it will surface in Phase 6's residual scan
  (`hits_strict.txt`). **Phase 6 step 18's classification scheme (a/b/c/d) has no bucket for
  "on-disk schema identifier that must never change because it already exists in every user's
  real database"** — an executor following step 18's literal remediation ("genuine miss: fix
  it, re-run the scan") would rename the string literal in `blockstore_dbops.go`, at which point
  every query in that file targets a table name (`db_remoteterm_file`) that doesn't exist in any
  pre-existing filestore database: `no such table: db_remoteterm_file`, a hard runtime failure
  on the file-storage subsystem for every user who upgrades — the same class of risk as
  `wave.lock`'s marker-check bug from round 1, in SQL schema rather than a Go/env-var
  identifier. **Add `db_wave_file` to `RENAME_ALLOWLIST.md` explicitly** (Phase 3 step 9's
  allowlist artefact) so Phase 6's sweep treats it as allowlisted (category a), not a genuine
  miss. If the repo owner genuinely wants this table renamed, it requires an actual database
  migration (`ALTER TABLE db_wave_file RENAME TO db_remoteterm_file`, added as a **new** numbered
  migration file, never an edit to `000001_init.up.sql`) coordinated with the corresponding
  query-string change in `blockstore_dbops.go` in the same commit — never a residual-sweep "fix".
  Checked: `db/migrations-wstore/*.sql` (the separate workspace/session/tab database) carries no
  equivalent wave-branded table/column names — this issue is isolated to the filestore schema.
- **`tsunami/` is a second Go module, not a subpackage of the root one, and a simple
  "exclude the `tsunami/` directory" rule does not correctly implement either choice.**
  `tsunami/go.mod` declares `module github.com/wavetermdev/waveterm/tsunami`; the root `go.mod`
  requires it at a pinned version (`v0.12.3`) and locally overrides it (`replace ... =>
  ./tsunami`). Eight demo apps under `tsunami/demo/*/go.mod` and two runtime-consumed templates
  (`tsunami/templates/app-main.go.tmpl`, `app-init.go.tmpl`) also declare or import this path,
  and the templates are copied into `dist/tsunamiscaffold/` at build time to scaffold new user
  apps. Decide: does `tsunami` rename in lockstep with the root module, or does it keep its own
  (possibly externally-consumed) path? Either way, the boundary is the **import path string**,
  not the directory:
  - **Two root-module files import the tsunami module from *outside* `tsunami/`**:
    `pkg/buildercontroller/buildercontroller.go` (imports `.../tsunami/build`) and
    `pkg/wshrpc/wshserver/wshserver.go`. If the decision is "tsunami keeps its own path," a
    directory-scoped exclusion (`tsunami/` held out of the sed) does not protect these two
    files — they are outside `tsunami/` and inside the ordinary 204-file sed scope, so a blanket
    sed still rewrites their import of a module that, by decision, didn't move, producing an
    import path that resolves to nothing under this branch.
  - **If the decision is "tsunami moves with the root," the 8 `tsunami/demo/*/app.go` files are
    separate Go modules but are still `*.go` files inside ordinary `git ls-files '*.go'` scope**
    — they get swept by the sed regardless of whether anyone remembers to also update their
    `go.mod` files, so both halves (source files via the blanket sed, `go.mod` files via the
    explicit list two paragraphs below) need to land together, not just the `go.mod` list on its
    own.
  - **Implement the exclusion (or inclusion) by import-path pattern, not by directory path**:
    the sed/grep boundary for tsunami-related content should match on the string
    `github.com/wavetermdev/waveterm/tsunami` wherever it occurs (43 tracked files carry it, 86
    occurrences total — inside `tsunami/` and in the two root-module files above), not on
    whether the containing file happens to live under `tsunami/`.
  This decision changes what Phase 3's `sed` is allowed to touch and is a separate question from
  the eleven root packages above.
- **Remote-host `.waveterm` state has no rename or migration decision.** `pkg/wavebase.go`
  defines `RemoteWaveHomeDirName = ".waveterm"`, `RemoteFullWshBinPath`,
  `RemoteFullDomainSocketPath`, and `RemoteDomainSocketBaseName` — these locate the `wsh`
  binary, domain socket, and shell-integration scripts (`.bashrc` snippet, `wave.fish`,
  `wavepwsh.ps1`, zsh integration) that this app has already installed on **every machine the
  user has ever SSHed into**, not just the local machine. This is a different constant family
  from the local data-dir constants Phase 2 migrates, and neither renaming these values nor
  leaving them alone is currently a decision anyone has made:
  - If Phase 3's blanket "rename every wave-branded identifier" sed touches these values,
    **the actual mechanism is more forgiving than "no migration path" suggests, but not free of
    friction**: `pkg/remote/conncontroller/conncontroller.go`'s `IsWshVersionUpToDate` treats a
    `wsh` binary missing from the (now-renamed) expected path as `"not-installed"`, which
    triggers `getPermissionToInstallWsh` + `InstallWsh` — i.e. the local client re-installs
    `wsh` at the new path after a permission prompt, self-healing rather than hard-failing.
    Round-3 audit corrected "remote command execution... break on next connect, with no
    migration path" (an earlier version of this item's wording) to: **expect one extra
    permission-prompt-and-reinstall per remote host on first post-rename connect, plus an
    orphaned old-path `.waveterm` tree left behind** — closer to the second bullet below than a
    hard break, though a *running* SSH-block terminal opened before the rename may still error
    until the session reconnects (unconfirmed; the shell-integration reinstall path for the
    `.bashrc` snippet/`wave.fish`/`wavepwsh.ps1` files was not fully traced).
  - If the values are deliberately left unchanged, every remote host keeps an orphaned
    `.waveterm` tree that nothing cleans up, and a fresh `.remoteterm` (or whatever the new
    name is) install gets pushed alongside it on next connect — silent residue on machines the
    user may not fully control.
  Decide which of these two outcomes is acceptable (or design a targeted remote-reinstall step)
  before Phase 3 executes, and record the decision here. Phase 6's cleanup sweep should treat
  orphaned remote `.waveterm` directories as expected, not as a miss, once this is decided.
  **Two locally-written shell-integration output filenames need the same decision as the
  `wave.lock`/`wave.sock` marker filenames in Phase 3 step 9**: `pkg/util/shellutil/
  shellutil.go` hardcodes the literal output filenames `"wave.fish"` and `"wavepwsh.ps1"` — the
  actual fish/PowerShell hook files written into the local data directory (the same root Phase 2
  migrates) that a user's shell sources on every new session. These are distinct from the six
  source-template files step 6/8 already cover (which handle those templates'
  `WAVETERM_`-prefixed *content*, not these two templates' own *output filenames*). Decide
  whether `"wave.fish"`/`"wavepwsh.ps1"` rename for newly-written installs going forward (same
  "old name for legacy detection, new name for new writes" framing as the lock/socket filenames
  a few lines away in the same source file) — resolve in Phase 3 step 9 alongside those.
- **`wavesrv` binary name and its RPC route identifier need the same explicit
  keep/rename decision Phase 4 step 12 already gives `wsh` — and the owner needs the
  coexistence consequence stated, not just the two options.** `wavesrv` is the compiled Go
  server binary the Electron main process spawns (`emain/emain-wavesrv.ts`,
  `emain/emain-platform.ts`'s `wavesrvBinName`) and is referenced by name in three places in
  `electron-builder.config.cjs`. It is *also* a wire-level identifier: `pkg/wshutil/wshrouter.go`
  `DefaultRoute = "wavesrv"` is written into outgoing RPC messages (`wshrouter.go`'s
  `rpcMsg.Route = DefaultRoute`), and `pkg/remote/connparse/connparse.go` `ConnHostWaveSrv =
  "wavesrv"` parses user-facing connection strings — both cross a version boundary the way the
  remote-host state above does: a previously-installed remote `wsh` binary, or a `wsh` process
  already running in an existing shell pane, encodes the *old* route string. Decide: does the
  binary name and the route-identifier string change together (in the same Phase 3 commit,
  updating `electron-builder.config.cjs`'s three match points, `emain-platform.ts`, and both Go
  route constants), or is `wavesrv` held out of scope like `wsh`? **If "rename together" is
  chosen, the router must accept both the old and new route strings for one release** — the
  same dual-write/dual-accept deprecation-window pattern Phase 2 step 6 already applies to the
  session-scoped env vars — otherwise an already-running `wsh` process addressing the server by
  the old route string silently stops routing correctly the moment the local server starts
  answering only to the new one. Record the decision, and this coexistence requirement if
  "rename together" is chosen, here.
- **The "WaveApp Builder" sub-feature needs its own product-naming decision, not a literal
  find/replace, and the decision covers more than the frontend.** `frontend/builder/` and
  `frontend/app/view/tsunami/` implement a distinct mini-app-builder feature with its own
  document titles, menu labels ("Stop WaveApp", "Remix WaveApp in Builder"), headings ("Create
  New WaveApp"), a WPS event-type literal (`"waveapp:appgoupdated"`), and a log-filename pattern
  in `emain/emain-log.ts` (`^waveapp\.(\d+)\.log$`). "WaveApp" reads as a compound product name,
  the same class of decision as `pkg/wavebase`'s final name, not a plain "Wave" substring —
  decide what this feature is called under the RemoteTerm brand before Phase 1/Phase 4 touch it.
  **The feature's Go-side engine is a separate, larger surface than the frontend pieces above
  and has no owning phase at all**: `tsunami/engine/`, `tsunami/app/`, `tsunami/vdom/`,
  `tsunami/rpctypes/`, `tsunami/ui/`, `tsunami/build/`, and `tsunami/cmd/` (20+ Go files)
  contain non-import-path "wave"/"waveapp" content — struct comments (`tsunami/engine/
  clientimpl.go:47-48`, "for waveapps, the icon to use"), and a hardcoded served-icon route
  (`clientimpl.go:170`, `/wave-logo-256.png`). Phase 3's `tsunami/` handling (above) is
  import-path-only; it does not cover renaming this content. Once the WaveApp Builder name is
  decided, a content-level rename pass over `tsunami/`'s Go source is also needed — assign it to
  Phase 3 (alongside the import-path work, since it's the same directory and the same Go
  toolchain) or to a dedicated step, but assign it explicitly; it is not covered by Phase 4's
  `frontend/`/`emain/` scope or by Phase 3's mechanical sed.
- **App icon/logo artwork is Wave Terminal's own trademark and has no phase — and the file set
  named here must include the ones electron-builder actually packages, not just the in-app UI
  assets.** No phase, step, or other Prerequisites item addresses `assets/wave-*.{png,svg,webp,
  ico}`, `assets/waveterm-logo-*.{png,svg,ico}`, `public/logos/wave-*.png`, `docs/static/img/
  logo/wave-*`, or `tsunami/frontend/public/wave-logo-256.png`. These are loaded at runtime as
  the actual window/taskbar icon (`emain/emain-window.ts:200`, `emain/emain-builder.ts:63`) and
  served as an icon route (`tsunami/engine/clientimpl.go:170`, `/wave-logo-256.png`). **A
  separate, more load-bearing set is missing from this list entirely**: `build/icon.icns`,
  `build/icon.ico`, and `build/icons/{16x16,32x32,48x48,64x64,128x128,256x256,512x512}.png` —
  electron-builder's zero-config default icon-directory convention (mac: `build/icon.icns`,
  Windows: `build/icon.ico`, Linux: `build/icons/*.png`), confirmed live by grepping
  `electron-builder.config.cjs` for an explicit `icon:` override (none exists, so
  electron-builder relies on this exact directory) and by `git log` on `build/icon.icns`
  showing it was last touched by an upstream commit titled "update app icon on Windows" — this
  is genuine Wave-branded art, not a placeholder. These 9 files are what actually produces the
  dock icon, the Windows taskbar/installer icon, and the Linux AppImage/deb package icon a real
  user sees — more load-bearing than the in-app `assets/wave-*` SVGs, since those are UI
  elements while `build/icon.*` is what the OS itself displays. An executor who ships new art
  or secures a rights determination for every path in the first list above can still ship a
  build whose dock/taskbar/installer icon is unchanged Wave artwork, because the file that
  actually controls that icon was never named. All of this is binary image content with no
  renameable string, so Phase 6's grep-based residual sweep will never catch a miss here either
  — the most visually obvious "this is still Wave" signal in the product once every text
  identifier is renamed, and the one most likely to draw a trademark objection: Apache-2.0 §6
  does not grant permission to use "the trade names, trademarks, service marks, or product
  names of the Licensor" beyond reasonable/customary attribution. A find/replace string pass
  does nothing here — new artwork or an explicit rights determination is design work, not a
  rename script, and needs to be scheduled. Decide: ship new RemoteTerm artwork for **all**
  paths named above, including `build/icon.*`, or continue using Wave Terminal's logo files
  under an explicit rights determination. Blocking for Phase 1 (window icon load paths) and
  Phase 5 (the `build/icon.*` set is packaging-time, not runtime, and belongs alongside Phase
  5's CI/build work if new art needs wiring into the build config) and Phase 6 (the residual
  sweep would otherwise silently pass — image files carry no renameable string content for a
  text-based grep to catch).
- **`emain/` (the Electron main-process TypeScript, 16 files, ~430 case-insensitive `wave`
  hits) is not currently assigned to any phase.** Phase 2 touches two files in `emain/` for
  env-var values only; Phase 4's title, file list, and verification grep are all
  `frontend/`-only. Decide whether `emain/`'s internal identifiers (`WaveBrowserWindow`,
  `createNewWaveWindow`, `getWaveWindowById`, `getWaveSrvPath`, `runWaveSrv`, `getWaveVersion`,
  etc.) are in scope for Phase 4 or deliberately deferred — Phase 6's exit criterion (residual
  grep returns only allowlisted hits) cannot pass with `emain/` unscoped, so this cannot be
  left implicit.
- **The `docs/` Docusaurus site keep/drop/rewrite decision (Phase 1 step 5) blocks Phase 1
  being called done and blocks Phase 5 step 16 (`CNAME`).** It was previously absent from this
  Prerequisites list despite carrying the same blocking weight as the items above — added here
  so it is not missed when Phase 1 is dispatched as the "independently shippable" warm-up
  phase. See Phase 1 step 5 and "Plan Is Wrong If" for the full reasoning (short version: this
  fork has 249 real commits to `docs/`, including fork-authored features, so "drop entirely" is
  not the low-cost default it might look like). If the decision is "keep and rebrand," fold
  `docs/package.json`'s `"name": "waveterm-docs"` field (and the corresponding
  `package-lock.json` workspace entry, regenerated via `npm install`) into the same Phase 1
  step 5 edit — it's npm workspace bookkeeping, not rendered content, and easy to miss under a
  "keep/drop/rewrite the docs *content*" framing.
- **A fourth path root — Electron's own `userData` directory on Windows — has no closing
  decision, only an "UNCERTAIN" flag inside step 6 that nothing forces to resolution.** Step 6's
  `app.setName("waveterm/electron")` bullet already identifies that on Windows, Electron's
  `userData` resolves under `%APPDATA%` while `envPaths`'s config root resolves under
  `%APPDATA%\waveterm\Config` — siblings, not parent/child, meaning this root does **not**
  automatically ride along with the config-root migration the way it does on Linux/macOS. That
  bullet correctly flags this as unconfirmed, but nothing elsewhere treats it as blocking:
  Estimated Effort's Phase 2 entry states flatly "three path roots need migrating (not one)"
  and Post-Migration Validation's success criteria name only the config and data roots — neither
  reflects the possible fourth. Resolve here, before Phase 2 executes on Windows: either commit
  to migrating Electron's `userData` as a fourth root, or explicitly accept and document that
  cookies, cached sessions, and window geometry are discarded on a Windows upgrade — do not
  leave this as an open "uncertain" that Estimated Effort and Post-Migration Validation both
  silently assume resolved in the "three roots" direction.
- **Windows code-signing identity (`electron-builder.config.cjs`'s `win.signtoolOptions`)
  hardcodes `publisherName`/`certificateSubjectName: "Command Line Inc"` and has no keep/repoint
  decision — this is a different legal mechanism from the NOTICE-file attribution Phase 1 step 3
  already resolves, not the same question asked twice.** The signing config is gated on
  `SM_CODE_SIGNING_CERT_SHA1_HASH` being set (currently inert without it), but if this fork ever
  provisions its own Windows signing certificate, leaving these two fields as "Command Line Inc"
  produces a signed binary whose Authenticode publisher metadata doesn't match the actual
  certificate subject — some signing tools reject this outright, and it's user-visible in
  Windows' own "Digital Signatures" property dialog. Fold into the existing CI-secrets
  confirmation below: also confirm whether this fork has, or plans to obtain, its own Windows
  code-signing certificate, and if so, update these two fields to match its actual subject name.
- Confirm with the repo owner whether `WAVE_BUILDER_APPID` / `WAVE_BUILDER_KEY` /
  `PUBLISHER_KEY_ID` / `SNAPCRAFT_LOGIN_CREDS` / `WINGET_BUMP_PAT` GitHub Actions secrets exist
  on *this* fork's repo settings (not visible to a code-only agent). If they don't, Phase 5's
  CI changes are cosmetic-only for now; if they do, renaming `WAVE_BUILDER_APPID` requires
  updating the corresponding repo secret at the same time as the workflow file, in the same PR
  merge window, or `bump-version.yml` breaks on the next run.
- Confirm `dl.waveterm.dev` / `docs.waveterm.dev` / `api-dev.waveterm.dev` /
  `wsapi-dev.waveterm.dev` / `ping-dev.waveterm.dev` are upstream-owned infrastructure this
  fork has no control over (the README's "Fork Notes" claims telemetry was fully removed, but
  `Taskfile.yml`'s `electron:dev`/`electron:start` tasks still set `WCLOUD_PING_ENDPOINT` /
  `WCLOUD_ENDPOINT` / `WCLOUD_WS_ENDPOINT` pointing at `*.waveterm.dev`). This is a
  pre-existing inconsistency, not something this rename plan introduces, but Phase 5 should
  not invent replacement RemoteTerm-branded endpoints that don't exist — flag and leave the
  vars pointed at upstream (or dead) until the repo owner decides whether that dev-telemetry
  path should be deleted outright, which is a separate piece of work from renaming.
- `package.json` (`name: "remoteterm"`, `productName: "RemoteTerm"`, `appId:
  "dev.remoteterm.app"`, `homepage: "https://remoteterm.dev"`) is already done — treat as the
  north star for every user-facing string added in Phase 1.
- **Zero live Go-touching branches at Phase 3 execution time.** `.claude/worktrees/` currently
  contains ten agent worktrees with full Go checkouts (nine found by the original audit, plus
  one more created 2026-09-20 for Phase 1 execution — count drifts as work happens, re-check
  live, don't trust this number either), and the repo currently has ~202 unrelated uncommitted
  files on the active branch. **A sibling worktree outside `.claude/worktrees/` also exists and
  must be checked**: `remoteterm-uat` at `/media/owner/Workspace/remoteterm/remoteterm-uat`
  (branch `integration/uat-prs-45-51`, PRs #45-51 merged in for manual UAT) — the `git
  worktree list` command surfaces this one too; don't scope the check to `.claude/worktrees/`
  only. "Plan Is Wrong If" already names this risk but defers checking it to execution time —
  it is already checkable now: run `git worktree list` immediately before Phase 3 starts and
  confirm every worktree other than the one about to do the rename is either gone or not
  mid-edit on `*.go` files, and confirm there are no open PRs touching `*.go`, not as an
  afterthought.

### All Prerequisites decisions — RESOLVED 2026-09-20 by repo owner

Every blocking item above is now decided. Phase 3 is unblocked. Recorded here as the single
source of truth for the sed script and every downstream step; the reasoning above each original
item still stands as rationale and stays in place.

1. **Module path:** `github.com/wavetermdev/waveterm` → **`github.com/LannCo/remoteterm`**
   (matches the actual fork remote).
2. **Package rename table** (full internal rename confirmed; `pkg/waveai` already removed, not
   in this table):
   | Old | New | Note |
   |---|---|---|
   | `pkg/wavebase` | `pkg/remotetermbase` | full prefix |
   | `pkg/waveobj` | `pkg/remotetermobj` | full prefix |
   | `pkg/waveapp` | `pkg/remotetermapp` | full prefix |
   | `pkg/waveapputil` | `pkg/remotetermapputil` | full prefix |
   | `pkg/waveappstore` | `pkg/remotetermappstore` | full prefix |
   | `pkg/wavejwt` | `pkg/remotetermjwt` | full prefix |
   | `pkg/wshrpc` | **unchanged** | `wsh`'s own RPC layer — `wsh` itself stays out of scope (Phase 4 step 12), so its RPC/util packages keep the same naming link rather than orphaning the reference |
   | `pkg/wshutil` | **unchanged** | same reasoning as `wshrpc` |
   | `pkg/wcore` | `pkg/rtcore` | short `w→rt` prefix |
   | `pkg/wconfig` | `pkg/rtconfig` | short `w→rt` prefix |
   | `pkg/wstore` | `pkg/rtstore` | short `w→rt` prefix |

   Exported `Wave*` type names (`WaveObj`, `WaveAIStreamRequest`, etc.) remain **pinned as
   do-not-rename this pass**, per the reasoning above — this is unchanged by the package-name
   decision; package and type names are independent in Go.
3. **`db_wave_file`:** not an owner decision — stays unchanged, added to `RENAME_ALLOWLIST.md`,
   per the reasoning above (this is a correctness constraint, not a judgment call).
4. **`tsunami/`:** **renames with the root module** → `github.com/LannCo/remoteterm/tsunami`.
   Implement by import-path string match (`github.com/wavetermdev/waveterm/tsunami`), not by
   directory boundary, per the reasoning above — this also resolves the two root-module files
   that import it from outside `tsunami/`.
5. **Remote-host `.waveterm` state:** **renames.** Expect the self-healing reinstall path (one
   permission prompt per host on first post-rename connect) rather than a hard break, per the
   round-3-corrected reasoning above. The two shell-integration output filenames
   (`wave.fish`/`wavepwsh.ps1`) rename for new writes going forward, alongside the
   `wave.lock`/`wave.sock` marker-filename decision in Phase 3 step 9.
6. **`wavesrv` binary + RPC route:** **renames together**, with the router accepting **both**
   the old (`wavesrv`) and new route strings for one release (same dual-accept pattern as the
   session-scoped env vars in Phase 2 step 6) — required, not optional, given the decision.
7. **"WaveApp Builder" feature naming:** renamed to **"RTApp Builder"**, mechanically:
   "WaveApp" → "RTApp" wherever it appears as the product-facing name — "Stop WaveApp" → "Stop
   RTApp", "Remix WaveApp in Builder" → "Remix RTApp in Builder", "Create New WaveApp" →
   "Create New RTApp", the WPS event literal `"waveapp:appgoupdated"` →
   `"rtapp:appgoupdated"`, and the `emain-log.ts` log-filename pattern `^waveapp\.(\d+)\.log$` →
   `^rtapp\.(\d+)\.log$`. The Go-side content-level rename pass over `tsunami/engine/`,
   `tsunami/app/`, `tsunami/vdom/`, `tsunami/rpctypes/`, `tsunami/ui/`, `tsunami/build/`,
   `tsunami/cmd/` (struct comments, the `/wave-logo-256.png` served-icon route) is assigned to
   **Phase 3**, alongside that phase's `tsunami/` import-path work — same directory, same Go
   toolchain, same commit.
8. **App icon/logo artwork:** **ship new RemoteTerm artwork everywhere**, using the mark already
   designed and approved (device+prompt glyph, `#58C142`) — extended to every load-bearing path
   named above: `assets/wave-*`, `assets/waveterm-logo-*`, `public/logos/wave-*`,
   `docs/static/img/logo/wave-*`, `tsunami/frontend/public/wave-logo-256.png`, **and**
   `build/icon.icns`, `build/icon.ico`, `build/icons/{16,32,48,64,128,256,512}x512.png` (the
   electron-builder default-icon-directory set — the actual dock/taskbar/installer icon, not
   just in-app UI assets). No rights-determination branch needed. Blocking for Phase 1, Phase 5
   (packaging-time icon set), and Phase 6 (image files carry no text for the residual grep to
   catch — this is verified by human eyeball, not by the sweep).
9. **`emain/`:** **in scope for Phase 4.** Its 16 files / ~430 `wave` hits fold into Phase 4's
   file list and verification grep alongside the `frontend/` work; Phase 6's exit criterion can
   now actually pass.
10. **`docs/` Docusaurus site:** **keep and rebrand.** Rewrite the 9 files carrying the upstream
    module path (`docs/docusaurus.config.ts` and the 8 `.mdx` files listed at Phase 1 step 5)
    plus `docs/package.json`'s `"name": "waveterm-docs"` workspace field (and the corresponding
    `package-lock.json` entry, regenerated via `npm install`) as part of Phase 1 step 5.
11. **Fourth path root — Windows Electron `userData`:** **migrate as a fourth root**, alongside
    the config/data/legacy roots in Phase 2 step 7, for full cross-platform consistency. Extend
    Phase 2's verification and Post-Migration Validation success criteria to name all four
    roots, not three.
12. **Windows code-signing identity:** **no cert yet — leave
    `publisherName`/`certificateSubjectName: "Command Line Inc"` as-is.** Inert without
    `SM_CODE_SIGNING_CERT_SHA1_HASH` set; revisit if/when this fork provisions its own Windows
    signing certificate.
13. **GitHub Actions secrets / upstream endpoints:** still deferred, per the earlier recorded
    decision (Phase 5 steps 13-14 area) — unchanged by this round of decisions.

### Steps

**DONE 2026-09-20** — branch `feat/rename-phase1-user-visible-strings` (7 commits, 85 files,
committed not pushed, worktree at `.claude/worktrees/agent-a5ab3d7aed99f28d5`). Verification grep
passed clean. Two gaps found during execution, worth a plan edit next time someone touches this
section: `.kilocode/rules/overview.md` was missing from step 4's file list (fixed in the commit,
not in this plan text); Phase 5 step 17's note to move a release-title string "to Phase 1" was
never reflected back into Phase 1's own step list — that string is still unhandled. Also
surfaced, not fixed (repo-owner attention, not rename work): `README.ko.md`/`README.zh-TW.md`
are stale relative to the current English README (describe a removed AI-chat-widget feature);
`ROADMAP.md`'s AI-feature roadmap contradicts `AGENTS.md`'s stated priority of removing AI
features.

**Phase 1: User-visible strings + docs (independently shippable except three blocking owner
decisions this phase depends on — see Prerequisites: the docs/ keep/drop/rewrite decision
gating step 5, the WaveApp Builder naming decision gating the strings inside step 1, and the
app icon/logo artwork decision, which Prerequisites already states is "blocking for Phase 1"
because of the window-icon load paths. An earlier version of this header named only the first
of the three.)**

1. About modal, quit-confirm dialog, app menu, onboarding screens
   (`frontend/app/onboarding/onboarding*.tsx` — 5+ hits each in `onboarding.tsx`,
   `onboarding-upgrade-v0140.tsx`, `onboarding-upgrade-minor.tsx`,
   `onboarding-starask.tsx`), `frontend/app/modals/about.tsx`, window title
   (`frontend/wave.ts`, `index.html`): change display text only. No renames of files, exports,
   or types in this step.
   - **Also in this step, not a separate pass:** `emain/emain-menu.ts:175` (`label: "About Wave
     Terminal"` — a second, distinct instance of the About-menu string, separate from the modal
     component listed above) and `emain/emain-platform.ts:50` (`title: "Wave has detected a
     performance issue"` — a native dialog title shown to users on a real code path). Both were
     missed by the original file list and by Phase 1's own verification grep (`emain/` wasn't
     in its path list) — confirmed round-1 finding, fixed here rather than left to Phase 6.
   - **WaveApp Builder naming decision required before editing `frontend/builder/` and
     `frontend/app/view/tsunami/`** (document titles, menu labels "Stop WaveApp"/"Remix WaveApp
     in Builder", headings "Create New WaveApp", body copy) — see Prerequisites. Once the
     RemoteTerm-branded name for this sub-feature is decided, update these strings in this step
     alongside the other user-visible text; do not defer to Phase 6.
   - **`cmd/wsh/cmd/*.go` Cobra `Use`/`Short`/`Long` help-text prose also belongs in this step**
     — the single most likely place a new user of the renamed CLI actually reads the product's
     own self-description, and previously in no phase's scope at all (not Phase 1's frontend/doc
     file list, not Phase 3's import-path sed or Go-aware package-rename tool, neither of which
     touches arbitrary string literals). Confirmed live examples: `wshcmd-root.go:23` `Short:
     "CLI tool to control Wave Terminal"`; `wshcmd-conn.go:18-19` `"manage Wave Terminal
     connections"` / `"...SSH and WSL connections"`; `wshcmd-secret.go:25` `"Manage secrets for
     Wave Terminal"`; `wshcmd-editconfig.go:19-20` `"edit Wave configuration files"`;
     `wshcmd-jobmanager.go:23` `"job manager for wave terminal"`;
     `wshcmd-connserver.go:36` `"remote server to power wave blocks"`; `wshcmd-file.go:109`
     `"show wave file information"`. String-literal edits only — same class of change as this
     step's other user-visible text, not a code-identifier rename, so it doesn't need to wait
     for Phase 3. **This is a `.go` file edit inside a phase whose verification otherwise states
     "touches zero `.go` files" — see the correction to Phase 1's verification below.** Verify
     with `wsh --help` and `wsh <each-subcommand> --help` manually; `tsc`/`go vet` cannot catch
     stale CLI help text.
   - **Risk:** onboarding screens are versioned per historical release
     (`onboarding-upgrade-v0121.tsx` etc.) — don't rename the *files*, since the version
     suffix is a historical marker, not a brand marker; only the strings inside change.
   - **Rollback position:** reversible.
2. GitHub issue template (`.github/ISSUE_TEMPLATE/bug-report.yml`, `config.yml`),
   `electron-builder.config.cjs` `NS*UsageDescription` entitlement strings (9 strings, all
   read "A CLI application running in Wave wants to..." — mechanical find/replace of "Wave"
   with "RemoteTerm"), `pkg/wconfig/defaultconfig/settings.json`'s `"web:defaulturl"` (currently
   `https://github.com/wavetermdev/waveterm`, the URL a new Web widget opens by default — decide
   whether it points at this fork's repo or `https://remoteterm.dev` per the `package.json`
   north star; this file is also excluded from Phase 3's Go-only sed pass, so it must be
   handled here or it's missed entirely).
   - **`.github/FUNDING.yml`** (`github: wavetermdev`) routes this fork's GitHub Sponsors button
     at the upstream project. Decide explicitly: keep (credits upstream), remove, or repoint to
     this fork's own sponsor account if one exists — do not leave it as silent inheritance.
   - **Risk:** none — static strings, no code path depends on their content.
   - **Rollback position:** reversible.
3. `README.md`, `README.ko.md`, `README.zh-TW.md`: rewrite the top-level product description,
   keep the existing `> **Fork:** This is a fork of [Wave Terminal](...)` attribution line
   as-is (Apache-2.0 requires retaining the "NOTICE" and copyright attributions of the
   original work — see Out of Scope below). `BUILD.md`, `CONTRIBUTING.md`,
   `ACKNOWLEDGEMENTS.md`, `ROADMAP.md`, `AGENTS.md`, project `CLAUDE.md`, and three previously
   unlisted root markdown files of the same class: `RELEASES.md`, `IMAGE-RENDERING-INVESTIGATION.md`,
   `REMOTE-IMAGE-PASTE-SPEC.md`.
   - **`RELEASES.md` needs the same "does this infrastructure actually exist for this fork"
     treatment already given to `Taskfile.yml`'s release-bucket strings and the GitHub Actions
     secrets, not a blind rebrand.** It documents a WinGet publishing PR flow, a Homebrew Cask
     (`Homebrew/homebrew-cask/.../wave.rb`), a separate `wavetermdev/chocolatey` repository, a
     Snapcraft Store listing, and a "Wave Release Bot" (`wave-releaser`) GitHub service account —
     external, upstream-owned package-registry identities this fork almost certainly does not
     control. Confirm with the repo owner which (if any) of this is real for this fork before
     rewriting it as though it is; if none of it exists, mark the relevant sections as
     aspirational/inherited-from-upstream rather than silently rebranding a pipeline that was
     never set up under the RemoteTerm name — a blind find/replace here would leave a document
     that reads as an accurate runbook for infrastructure that doesn't exist.
   - **`NOTICE` (repo root, 1 line: `Copyright 2025, Command Line Inc.`) must be read as part of
     this step, not skipped.** Apache-2.0 §4(d) requires any derivative work to retain a
     readable copy of the original NOTICE file's attribution — do not rename or remove
     "Command Line Inc." (that's the upstream copyright holder's name, not a product-name
     string). This file contains no "wave" substring, so no grep-based verification in this
     plan will ever surface it; it must be checked by direct read, once, here. Decide whether to
     add RemoteTerm's own copyright line as an addendum underneath the retained upstream notice
     (Apache's own guidance on derivative-work notices recommends this) and record that decision
     next to the FOSSA-link decision below.
   - **Apache-2.0 §4(b) ("modified files" notice) is a separate obligation from §4(d)'s NOTICE
     handling and needs its own explicit decision, not silent omission.** §4(b) requires "any
     modified files to carry prominent notices stating that You changed the files." Record the
     decision here rather than leaving it unconsidered: this migration satisfies §4(b) at the
     repository level via the retained `> **Fork:**` README attribution line plus intact git
     history showing the fork point, rather than via per-file headers — a deliberate choice, not
     an oversight.
   - **Risk:** `ACKNOWLEDGEMENTS.md` links a FOSSA report scoped to
     `git%2Bgithub.com%2Fwavetermdev%2Fwaveterm` — that link points at upstream's FOSSA
     project, not this fork's. Flag rather than silently leave broken: either drop the FOSSA
     badge/link (this fork likely has no FOSSA project of its own) or note it's inherited and
     may 404/show unrelated data.
   - **Rollback position:** reversible.
4. `.kilocode/skills/*/SKILL.md` (8 files), `.kilocode/rules/rules.md` (this repo's own root
     `CLAUDE.md` `@`-imports this file into every Claude Code session working here, making it
     the single most-loaded prose file in the repo for any agent on this project — it opens with
     "Wave Terminal is a modern terminal which provides graphical blocks..." and was previously
     missing from this step's file list entirely; every agent working on Phase 2 onward would
     otherwise keep reading a stale self-description throughout the migration), `aiprompts/*.md`
     (internal dev notes, ~15 files), and `.github/copilot-instructions.md` (9 hits — same
     category, AI-coding-agent-facing project description, previously omitted from this step's
     file list) — pure prose references to "Wave Terminal" describing the *product*; update
     product-name mentions, leave code-identifier mentions (`waveDirName`, `pkg/wavebase`, etc.)
     alone since those don't move until Phase 3/4. This step is prose-only and must not touch
     any string that also appears as a real Go/TS identifier — grep for the identifier forms
     first and exclude them.
   - **Risk:** these files describe *how the code works*, including package names that will
     become stale once Phase 3/4 land. Accept that staleness here (Phase 6 sweep fixes it) —
     don't try to pre-rename identifiers that don't exist yet.
   - **Rollback position:** reversible.
5. `docs/` (Docusaurus site) — **scope decision, not execution**: see "Plan Is Wrong If" and
   the dedicated section below. Do not touch `docs/` content in this phase; only decide
   keep/drop/rewrite before Phase 1 is called done, since a partial docs edit is worse than
   either full commitment. **This decision is a Prerequisite (see above), not optional
   background reading — Phase 1 is not "done" while it is outstanding, despite the phase header
   below calling the phase independently shippable.**

Verification for Phase 1: **do not grep whole directories and expect zero hits** — `emain/`,
`frontend/builder/`, and `frontend/app/view/tsunami/` are large, mostly Phase 2/4-owned
directories that this phase only touches two lines of (`emain-menu.ts`, `emain-platform.ts`)
plus the WaveApp Builder user-visible strings; a whole-directory grep against them will return
hundreds of unrelated hits and can never pass, which was itself a round-2 finding against an
earlier version of this verification block. Instead, verify precisely:
1. File:line checks — `grep -n 'About Wave Terminal' emain/emain-menu.ts` (expect line 175,
   now renamed) and `grep -n 'has detected a performance issue' emain/emain-platform.ts`
   (expect line 50, now renamed) both return the updated string, not the old one.
2. `grep -rni "wave" frontend/app/onboarding frontend/app/modals .github/ISSUE_TEMPLATE
   .github/copilot-instructions.md .github/FUNDING.yml .kilocode/skills .kilocode/rules
   electron-builder.config.cjs pkg/wconfig/defaultconfig/settings.json README*.md BUILD.md
   CONTRIBUTING.md ACKNOWLEDGEMENTS.md ROADMAP.md AGENTS.md CLAUDE.md RELEASES.md
   IMAGE-RENDERING-INVESTIGATION.md REMOTE-IMAGE-PASTE-SPEC.md aiprompts` (`-rni`, not `-rn` —
   an earlier version of this gate had no `-i`, so it could not fail on the capitalised brand
   strings this phase exists to change, e.g. "Wave Terminal", the nine `NS*UsageDescription`
   strings that all read "A CLI application running in **Wave** wants to..."; also not `-ril`
   from an even earlier version — the `-l`-only form reported filenames without line context,
   which is not enough to distinguish an intentional deferred reference from a miss; Phase 6's
   sweep was already case-insensitive, this brings Phase 1 in line with it) should return only:
   the intentional fork attribution line, code-identifier references inside
   `.kilocode`/`aiprompts` prose deferred to later phases, and the FOSSA link (flagged above).
3. The WaveApp Builder strings specifically edited in `frontend/builder/` and
   `frontend/app/view/tsunami/` per this step (document titles, the listed menu labels and
   headings) — check those specific strings by name, not the directories as a whole; the rest
   of those directories' content remains untouched until Phase 4 and is expected to still say
   "wave" at this point.
4. `wsh --help` and `wsh <each-subcommand> --help`, run manually, show the updated help text
   for every Cobra `Use`/`Short`/`Long` string edited above; separately, `grep -rni
   '"[^"]*Wave[^"]*"' cmd/wsh/cmd/*.go` should return only string literals not yet in scope
   (identifier/import-path content Phase 3 owns), never prose.
`NOTICE` is checked separately by direct read (step 3), not by this grep, since it contains no
"wave" substring to match. **This phase is not zero-`.go`-file, despite an earlier version of
this line claiming so** — the `cmd/wsh/cmd/*.go` Cobra help-text edits above are plain string
literals, not code compiled or built in any way this phase's edits could break, so no `go build`
is involved regardless, but "touches zero `.go` files" was never quite accurate once that bullet
was added and is corrected here.

**Phase 2: Env vars + data-dir migration shim**

6. `emain/emain-platform.ts`: rename `waveDirNamePrefix` from `"waveterm"` to `"remoteterm"`,
   **and separately rename the `envPaths("waveterm", {suffix: waveDirNameSuffix})` literal a
   few lines below it** — these are two distinct string literals in the same file
   (`waveDirNamePrefix` at line 29, the `envPaths(...)` call at line 33), and only renaming the
   first while missing the second leaves the *data* directory (which `envPaths(...)` resolves
   on macOS/Windows, and on Linux when `XDG_DATA_HOME` is unset) on the old branded path while
   the *config* directory moves — see step 7's path-root enumeration for why this split is the
   single largest blast-radius gap found in round-1 audit. Rename
   `WaveConfigHomeVarName`/`WaveDataHomeVarName`/`WaveHomeVarName` constants' *values* from
   `WAVETERM_CONFIG_HOME`/`WAVETERM_DATA_HOME`/`WAVETERM_HOME` to
   `REMOTETERM_CONFIG_HOME`/`REMOTETERM_DATA_HOME`/`REMOTETERM_HOME`, **with a fallback**: read
   the new name first, and if unset, fall back to the old name with a one-time deprecation
   `console.log` (matches the existing backwards-compat idiom already in this file). These
   three are override vars a user may already have set in a shell profile; renaming with no
   fallback silently reverts them to the default location with no error. Rename
   `WAVETERM_ENVFILE` and `WAVETERM_NOCONFIRMQUIT` (referenced in `Taskfile.yml`,
   `emain/*.ts`) to `REMOTETERM_ENVFILE` / `REMOTETERM_NOCONFIRMQUIT` in the same commit as
   their Taskfile usages, no fallback needed (these two are genuinely dev-only knobs with two
   readers/writers each, unlike the three overrides above).
   - **`app.setName("waveterm/electron")` on line 17 of `emain-platform.ts` also needs
     `waveterm` -> `remoteterm` in the electron-runtime-data string** (distinct from the
     `app.setName(isDev ? "RemoteTerm (Dev)" : "RemoteTerm")` call two lines later that sets
     the *display* name — this first one sets Electron's own internal `userData` subdirectory
     name and currently still says `waveterm/electron` even though `package.json` is already
     branded). **This is a fourth path root, alongside the three step 7 enumerates, not
     already covered by them**: on Linux and macOS this `userData` subtree (cookies, Cache,
     Local Storage, window state) nests inside whichever root ends up housing Electron's own
     data and rides along with that root's migration; on Windows, Electron's `userData`
     resolves under `%APPDATA%` while `envPaths`'s config root resolves under
     `%APPDATA%\waveterm\Config`, making `%APPDATA%\waveterm\electron` a *sibling*, not a
     child, of the migrated config root — **UNCERTAIN, could not confirm Windows `userData`
     resolution without running the app; treat as needs-confirming and either add it as an
     explicit fourth migrated root on Windows or state plainly that Electron runtime data
     (cookies, cached sessions, window position/size) is discarded on upgrade on Windows and
     name that loss explicitly, rather than leaving it implicitly covered by step 7's "three
     roots" framing.**
   - **Renaming `waveDirNamePrefix` silently breaks the legacy pre-v0.8 detection this same
     file already relies on, unless decoupled first.** `waveDirName` (derived from
     `waveDirNamePrefix`) is what `getWaveHomeDir()` uses to build the legacy combined home
     path (`path.join(homeDir, "." + waveDirName)`) — renaming the prefix makes that function
     look for `~/.remoteterm` and silently stop recognising the real legacy path, `~/.waveterm`.
     Before renaming `waveDirNamePrefix`, add a separate constant decoupled from it (e.g.
     `LegacyWaveHomeDirName = ".waveterm"`) and point the legacy-detection code at that constant
     instead. Freeze the `"wave.lock"` string literal used by that same legacy check explicitly
     — step 9's note that the marker filename's *value* may rename in Phase 3 applies to
     newly-created data going forward, never to this legacy-detection literal, which must keep
     matching `wave.lock` forever regardless of what Phase 3 decides for new installs.
   - **The `WAVETERM_*` family is larger than the vars listed above, and the fix differs by
     how each var reaches its reader.** Three groups, not one:
     1. **User-set override vars** (`WAVETERM_CONFIG_HOME`/`DATA_HOME`/`HOME`) — read-new,
        fall-back-to-old is correct here, since the *user* is the one who set the value and the
        app only ever reads it.
     2. **Session-scoped vars the app itself writes into every spawned shell**
        (`WAVETERM_TABID`/`WAVETERM_BLOCKID`/`WAVETERM_WORKSPACEID`/`WAVETERM_CLIENTID`/
        `WAVETERM_CONN`/`WAVETERM_JOBID`, set in `pkg/blockcontroller/blockcontroller.go` and
        `pkg/jobcontroller/jobcontroller.go`, plus `WAVETERM_AUTH_KEY`
        (`pkg/authkey/authkey.go`, `emain/authkey.ts`), `WAVETERM_JWT`, `WAVETERM_SWAPTOKEN`,
        `WAVETERM_PUBLICKEY`) — **a read-side fallback does not protect these.** A shell or
        tmux pane spawned by an already-running pre-rename app instance has the *old* names
        baked into its inherited environment for the rest of its life; a post-rename `wsh`
        that only reads the new name never finds them, no matter what fallback `wsh`'s own read
        path has, because the app already wrote the old name into that shell's environment
        before the rename happened. The correct mitigation is on the **write** side: for one
        deprecation window/release, the app writes **both** the old and new names when spawning
        a shell, and `wsh` reads new-name-first with old-name fallback as before. Treat writes
        of the old name during this window as expected, allowed residue, not a miss.
     3. **Genuinely dev-only knobs** (`WAVETERM_ENVFILE`, `WAVETERM_NOCONFIRMQUIT`,
        `WAVETERM_DEV`/`WAVETERM_DEV_VITE` in `frontend/util/isdev.ts`) — rename outright, no
        fallback needed, as originally specified.
     Enumerate the full family with `grep -rn 'WAVETERM_' --include='*.go' --include='*.ts'
     --include='*.tsx' .` (not `pkg/`-scoped), classify each into one of the three groups above,
     and apply the matching pattern. Also widen the literal-prefix guard
     `pkg/util/envutil/envutil.go`'s `strings.HasPrefix(key, "WAVETERM_")` to match **both**
     prefixes during the deprecation window (a guard that only recognises the new prefix would
     silently drop the dual-written old-name vars group 2 still needs), and update the shell
     integration files named in step 8 below — **the real path is
     `pkg/util/shellutil/shellintegration/`, not `shellintegration/` at the repo root, and
     there are six files carrying `WAVETERM_`, not four**: `zsh_zshrc.sh` (15 hits),
     `bash_bashrc.sh` (14), `fish_wavefish.sh` (5), `pwsh_wavepwsh.sh` (5), `zsh_zshenv.sh` (4),
     `zsh_zlogin.sh` (1) — 44 occurrences total, plus 2 more in `shellutil.go` itself. An
     earlier version of this plan named only three of these six files and the wrong root path;
     the omitted three carry 34 of the 44 occurrences.
7. Data-dir migration shim. **At least three separate path roots need migrating, not one**
   (plus a possible fourth on Windows — see step 6's `app.setName` bullet): the config dir
   (`waveDirNamePrefix`, e.g. `~/.config/waveterm` -> `~/.config/remoteterm`), the data dir
   (the `envPaths(...)` literal from step 6 — a *different* path on macOS/Windows and on Linux
   without `XDG_DATA_HOME`), and the legacy pre-v0.8 combined home dir
   (`getWaveHomeDir()`'s `~/.waveterm` fallback). Migrating only the config root while leaving
   the data root on the old path is a correctly-branded-but-broken half-migration: the sqlite
   workspace/tab/block-history DB, `wave.lock`, `bin/wsh`, and all shell-integration rc files
   live under the **data** dir, not the config dir — a user who hits this loses their entire
   session history and workspace layout while the app otherwise looks fully migrated.
   - **Marker validation must use a file each specific root actually contains** — the original
     "check for `wave.lock` or `config/`" instruction does not work for the config root: the
     legacy `wave.lock` file has only ever been written under the **data** dir
     (`pkg/wavebase/wavebase-posix.go`, `wavebase-win.go`), never the config dir, so a shim that
     gates the config-dir migration on `wave.lock`'s presence inside the config dir will find
     it on **zero** real installs and silently never migrate. Use per-root markers instead:
     validate the config-dir source with a file it actually contains (`settings.json`, which
     `pkg/wconfig/settingsconfig.go` always resolves against the config dir), and validate the
     data-dir source with `wave.lock` as originally proposed. **The Phase 2 verification fixture
     below must be corrected to match** — seeding a synthetic `wave.lock` into a fake config dir
     tests a shim that cannot work on real data; build the fixture by running the pre-rename
     build once against a scratch `HOME` so it creates the genuine tree shape, then run the
     post-rename build against that.
   - **The override-var short-circuit must be evaluated per root, independently — not once for
     the whole migration.** The first fix for this interaction (checking "does the whole
     migration have a same-path source/destination, and if so skip everything") is itself wrong:
     the config, data, and legacy-home roots have three *independent* override vars
     (`WAVETERM_CONFIG_HOME`, `WAVETERM_DATA_HOME`, `WAVETERM_HOME`), each with its own
     resolved source, destination, and marker file (`settings.json` for config, `wave.lock` for
     data). A user with only `WAVETERM_CONFIG_HOME` set has config source == destination but
     data source != destination — a single whole-migration "same path, skip and return" check
     would skip the **data** root's real migration too, silently reproducing the exact
     half-migration bug this plan has a dedicated Risks-table row for (settings appear intact,
     the user's entire session/workspace history is orphaned), just triggered by a different
     mechanism than the original gate-logic bug. **Fix, restated per root**: for each of the
     three roots independently, if that root's resolved source path and resolved destination
     path are the same real path (which happens only when that root's specific override var is
     set), write that root's completion marker and continue to the next root — this is not a
     migration-wide early return, it is a per-root skip inside the same loop/sequence that
     processes all three roots. The marker payload for a same-path skip is not "moved from X at
     T" (nothing moved) — use a distinct payload, e.g. `no-migration-needed:override`, so a
     human inspecting the marker later can tell the two cases apart. Add **two** fixture cases to
     Phase 2 verification below, not one: (a) all three override vars set (exercises "every root
     skips"), and (b) only `WAVETERM_CONFIG_HOME` set with the other two roots on their
     XDG/platform-default paths (exercises "one root skips, the other two migrate for real" —
     the case a whole-migration short-circuit would silently break).
   - **The migration gate cannot be "new path does not exist" as originally specified**, because
     the app itself creates the new path as a side effect before any check can run:
     `emain/emain.ts` calls the path getters at module-evaluation time
     (`getWaveDataDir()`/`getWaveConfigDir()`), and both getters end by calling
     `ensurePathExists()`, which `mkdirSync`s the directory. By the time any check "does the new
     path exist" runs, the app has usually already created it — the gate as specified fires on
     the vast majority of launches. **Use a marker file written after a successful move instead**
     (e.g. `<newdir>/.migrated-from-waveterm` containing the source path and timestamp).
     **Placement, precisely, and placed where nothing can call the getters first** (two earlier
     versions of this instruction were both defeated by an import-time side effect they didn't
     account for: v1 said "immediately after `app.setName(...)`," which doesn't survive
     `emain-platform.ts` having two `app.setName` calls with `paths`/`waveDirName` defined
     between them; v2 said "call it as the first statement of `emain.ts`, before that file's own
     calls to the getters at lines 56-57" — but `emain.ts`'s *own* first statement is not
     actually first: `emain.ts:23` imports `emain-log.ts`, and `emain-log.ts` calls
     `getWaveDataDir()` **at module scope during ESM import evaluation** (`emain-log.ts:68-75`'s
     `rotateLogIfNeeded()` and `:95-107`'s log-file/transport setup both call the getter and
     `mkdirSync` the destination before `emain.ts`'s own body ever runs, since ES module imports
     evaluate before the importing module's statements). Electron-vite preserves ESM evaluation
     order, so any module `emain.ts` imports gets a chance to call the getters first, and there
     is no "first statement of `emain.ts`" placement that is actually first): **the migration
     must be a top-level, unconditional, synchronous call inside `emain-platform.ts` itself**
     (not exported and called from elsewhere), placed immediately after the three
     `WAVETERM_*_HOME` constant declarations and before the `getWaveConfigDir`/`getWaveDataDir`
     function *definitions* are exported. This is the only placement that is provably first:
     every consumer of the getters — `emain-log.ts`, `emain.ts`, `emain-wavesrv.ts`, and any
     future importer — imports the getter functions *from this module*, so this module's own
     top-level code runs before any of theirs, regardless of import order elsewhere. Add to
     Phase 2 verification (below) a check that no file imported by `emain.ts` calls
     `getWaveDataDir()`/`getWaveConfigDir()` at module scope going forward
     (`grep -n "getWaveDataDir()\|getWaveConfigDir()" emain/*.ts`, confirm every hit outside
     `emain-platform.ts` itself is inside a function body, not at top level) — this plan
     previously *asserted* the getters were called-not-defined-first without testing it, twice;
     do not repeat that. Handle the destination-already-exists case explicitly
     (after the override-var short-circuit above has already ruled out the "same path" case):
     empty directory (created by the app's own `mkdirSync` before the shim ran) -> remove it,
     then move; non-empty without the migration marker -> abort loudly and tell the user to
     merge manually rather than attempting a move-onto-existing that `fs.renameSync` will
     reject (`ENOTEMPTY`/`EEXIST` on POSIX, `EPERM` on Windows).
   - **Concurrency:** the migration runs before
     `electronApp.requestSingleInstanceLock()` (`emain/emain.ts`, ~200 lines and one
     module-evaluation phase after the path getters), so two processes launched close together
     (double-click racing a `wsh`-triggered launch, or a desktop-file launch racing a stale
     process) can both enter the migration simultaneously. Wrap the move in try/catch; treat
     `ENOENT` on the source during the move as "another process already did this" and re-check
     for the completion marker rather than crashing module evaluation (an uncaught throw here
     kills the process before any window exists, with no visible error to the user). Add this to
     the step's risk list rather than leaving concurrent launches undiscussed.
   - **Remote hosts are explicitly out of scope for this local migration shim** — see
     Prerequisites' "Remote-host `.waveterm` state" item. Do not extend this step to cover
     remote-side `.waveterm` directories; that needs its own decision and, if a migration is
     warranted, its own design, because move-not-copy semantics don't translate cleanly to a
     machine the local process doesn't fully control.
   - Do **not** delete the old directory's *parent* — only the directory being moved, per root.
     This reuses the existing `getWaveHomeDir()` backwards-compat pattern already in the file
     (it already has a legacy `~/.waveterm` fallback for pre-v0.8 installs) — the new migration
     should sit alongside it as a second, newer compat layer, not replace it.
   - **Pre-step, gated, before any real-data test, with an explicit source for the pre-rename
     build:** Phase 2 edits the same working tree the pre-rename build would otherwise come
     from, and the plan already treats extra worktrees inside this repo as hazardous (nine
     already exist under `.claude/worktrees/` — see Phase 3's worktree note), so "get a
     pre-rename build" cannot mean "add a tenth worktree here." At the commit immediately
     before Phase 2's first edit, either (a) build once via `task package` and archive the
     resulting `dist/` output *outside* this repository (a scratch directory, not a worktree),
     or (b) clone the repo fresh into a scratch directory outside `/media/owner/Workspace/
     remoteterm/remoteterm` at that commit and build there. Either way, the repo owner takes a
     manual `cp -r` backup of all path roots using that pre-rename build, **and verifies the
     restore actually works** by launching the pre-rename build against the restored copy in a
     scratch `HOME` — an untested restore is not a rollback. This is a blocking precondition
     for the "Post-Migration Validation" data check below, not an afterthought read only after
     execution.
   - **Rollback position:** reversible with data work for the migration logic itself (a failed
     migration is recoverable manually — the source directory was moved, not deleted; `mv`
     back). But reverting Phase 2's *code* (via `git revert`) does **not** move data back: the
     reverted build reads the old path while the user's data now sits at the new path, which
     looks identical to data loss until someone notices and manually reverse-migrates. See the
     corrected "Rollback Strategy" section below — Phase 2 is not the same class of "free"
     revert as Phases 1/4/5/6.
8. `pkg/wavebase/wavebase.go` and Go-side equivalents of `WAVETERM_*` env var names (`grep`
   showed `pkg/wavebase/wavebase.go` in the top-20 hit files), plus
   `pkg/util/envutil/envutil.go`'s prefix guard and the six `pkg/util/shellutil/
   shellintegration/*.sh` files named in step 6 (path and count corrected there — not four
   files at a root-level `shellintegration/`, as an earlier version of this plan said) — same
   rename, same commit as step 6 if the Go side reads the same env var names (verify with
   `grep WAVETERM_ pkg/` before assuming; if the Go backend and Electron frontend both read
   `WAVETERM_CONFIG_HOME` they must rename in lockstep or the backend and frontend disagree
   on where data lives). Also rename `WAVETERM_SKIP_APP_DEPS` in `postinstall.cjs` (repo root)
   in this same commit — a real npm-lifecycle knob with no owning phase in an earlier version
   of this plan, found only by widening the residual-hit scan beyond `pkg/`/`emain/`/`cmd/`.
   - **Sequencing: not a blocker, either order works.** Phase 3's `sed` rewrites Go *import
     paths*, not string constants — `pkg/wavebase/wavebase.go`'s `WAVETERM_*` values survive a
     Phase 3 pass untouched regardless of order and need this edit either way. Doing it before
     Phase 3 keeps this commit's diff small and legible, which is the only reason to prefer that
     order; it is not a dependency. (The "Estimated Effort" section previously stated this step
     "must land before Phase 3" as a hard sequencing requirement — that contradicted this step's
     own text and has been corrected to match this paragraph.)

Verification for Phase 2: manual test (not `go build`) — run the dev binary via `task dev`
pointed at a scratch `HOME` (per project rule: never touch the user's live `~/.config`
without asking — use a throwaway `HOME=/tmp/...` for this verification, not the real one).
Build a realistic fixture rather than a synthetic one, obtained per step 7's pre-step
(a scratch-directory build, not a new in-repo worktree): run the **pre-rename** build once
against the scratch `HOME` to create a genuine config dir (containing `settings.json`) and data
dir (containing `wave.lock`), then run the **post-rename** build against that same `HOME` and
confirm both roots move to their new paths, the migration-completion marker is written, and the
app starts with settings and session history intact. Re-run the post-rename build a second time
against the same `HOME` and confirm the migration does not re-fire (marker-gated, not
existence-gated). Also confirm no file imported by `emain.ts` calls the path getters at module
scope (`grep -n "getWaveDataDir()\|getWaveConfigDir()" emain/*.ts`, every hit outside
`emain-platform.ts` itself must be inside a function body) — this is what step 7's placement fix
depends on and it must be checked directly, not assumed. **Second and third fixtures, required,
not optional, exercising the per-root override short-circuit (step 7)**: (b) repeat the two-build
sequence with `WAVETERM_CONFIG_HOME`/`WAVETERM_DATA_HOME`/`WAVETERM_HOME` **all three** explicitly
set in the scratch environment before the post-rename build's first launch, and confirm the app
starts normally (does not hit the "abort loudly" branch) and every root writes its
`no-migration-needed:override` marker immediately with no data move; (c) repeat again with
**only** `WAVETERM_CONFIG_HOME` set and the data/legacy-home roots left on their XDG/default
paths, and confirm the config root skips (override marker) while the data root still performs a
real move (`.migrated-from-waveterm` marker, data actually relocated) — this is the case a
whole-migration (rather than per-root) short-circuit would silently fail. `git ls-files -z |
xargs -0 grep -ln
WAVETERM_` (not a fixed path list, and not a raw filesystem walk — gitignore-scoped for the same
reason as Phase 3's sed) should return: `emain/`, `pkg/`, `cmd/`, `frontend/`,
`pkg/util/shellutil/shellintegration/`, and `postinstall.cjs` all clean of the old prefix outside
comments documenting legacy behaviour or the deliberately-retained fallback/dual-write reads from
step 6; **`Taskfile.yml` is expected to still show exactly the `WAVETERM_ENVFILE`/
`WAVETERM_NOCONFIRMQUIT` usages renamed in the same commit as step 6 — not a full-file skip**
(Phase 5 step 14's "skip Taskfile.yml entirely" decision covers *release-infra* strings only;
this pair of dev-only knobs is a narrow, explicit carve-out already required by step 6, and this
gate should not be read as contradicting that deferral); and an explicit, enumerated residue list
for what's still expected outside the renamed scope at this point in the plan — `bump-version.yml`
and `build-helper.yml` (Phase 5), `.kilocode/skills/add-wshcmd/SKILL.md` and `docs/docs/
connections.mdx`/`customwidgets.mdx` (Phase 1's prose/docs scope, not yet executed at this
verification point if phases run in order), and `.pi/specs/*` (historical planning docs, out of
scope entirely — not a rename target). The original verification command (`emain/ pkg/
Taskfile.yml` only, later widened to add `cmd/`/`frontend/`/`shellintegration/` with the wrong
path) missed all of the above; this version is scope-complete against a repo-wide scan, not a
fixed guess at which directories matter.

**Phase 3: Go module path + package rename (mechanical, atomic within itself)**

9. Single scripted pass, single commit: `go.mod` module path
   `github.com/wavetermdev/waveterm` -> the exact target string resolved in Prerequisites (no
   `<owner>` placeholder may reach the sed script), then a `sed`/`gofmt -r`-driven rewrite of
   every import statement across all files that match `wavetermdev/waveterm`. `gofmt -r` is
   *not* the right tool here (it rewrites expressions, not import path strings); use a scripted
   `sed 's#github.com/wavetermdev/waveterm#<resolved target>#g'`, or `gofmt`'s companion
   `goimports`/`gopls`-based rename tooling if available.
   - **File-list generation must be gitignore-aware, not a raw `**/*.go` glob.** The repo root
     currently holds nine live agent worktrees under `.claude/worktrees/` (each a full Go
     checkout with its own `go.mod`) and a vendored Go toolchain under `golang-1.26.2/`
     (including the stdlib's own deliberately-misformatted `gofmt` test data) — both are
     gitignored (`.gitignore`) and invisible to the 204-file count, but a raw filesystem
     `sed`/`gofmt` walk will hit them anyway: rewriting Go files inside nine other worktrees
     silently dirties nine other branches, and running `gofmt -l .` over the vendored toolchain
     makes "returns empty" fail for reasons that have nothing to do with this rename. Generate
     the file list with `git ls-files -z '*.go' | xargs -0 sed -i ...` and confirm the same way
     with `gofmt -l $(git ls-files '*.go')`, not bare `.`.
   - **`tsunami/` is a second Go module and needs its own explicit handling, matched by import
     path, not by directory** (see Prerequisites for the full reasoning): it has its own
     `go.mod` (`module github.com/wavetermdev/waveterm/tsunami`), is required by the root
     `go.mod` at a pinned version with a local `replace` override, has 8 demo-app `go.mod` files
     under `tsunami/demo/`, and has 2 runtime-consumed Go source templates
     (`tsunami/templates/app-main.go.tmpl`, `app-init.go.tmpl`) that get copied into
     `dist/tsunamiscaffold/` to scaffold new user apps. None of these 11 files are `*.go`, so
     they are invisible to a `--include=*.go` grep or a `*.go`-scoped sed. If `tsunami` moves
     with the root module: rewrite the root `go.mod`'s module line, `require`, and `replace`
     entries together, rewrite `tsunami/go.mod`'s own module line, and rewrite both `.go.tmpl`
     files, in addition to (not instead of) the ordinary `*.go` sed pass which already reaches
     `tsunami/*.go` and the two root-module files (`pkg/buildercontroller/buildercontroller.go`,
     `pkg/wshrpc/wshserver/wshserver.go`) that import the tsunami module from outside
     `tsunami/`. If `tsunami` keeps its own path: exclude by matching the import-path string
     `github.com/wavetermdev/waveterm/tsunami` specifically, not by excluding the `tsunami/`
     directory — a directory-scoped exclusion misses the two root-module files above, which sit
     outside `tsunami/` but import it. Also decide and execute, in this same phase, the
     content-level rename (identifiers, comments, string literals — not just the import path) of
     `tsunami/engine/`, `tsunami/app/`, `tsunami/vdom/`, `tsunami/rpctypes/`, `tsunami/ui/`,
     `tsunami/build/`, and `tsunami/cmd/` once the WaveApp Builder naming decision (Prerequisites)
     is made — this is separate work from the import-path sed above and was previously unassigned
     to any phase.
     **The 8 `tsunami/demo/*/go.mod` files are a different, lower-stakes case than the paragraph
     above treats them as — reclassify, don't rewrite as load-bearing.** They are already
     unbuildable for a reason unrelated to this rename: `tsunami/demo/todo/go.mod` (and its
     siblings) `replace`s the tsunami module to an absolute path on an upstream developer's own
     machine (`/Users/mike/work/waveterm/tsunami`), which does not exist in this repo or on any
     machine this fork's contributors use — nothing in the root module graph depends on these
     demo modules, and no gate in this plan can compile them: `go vet ./...` does not descend
     into nested modules (each demo has its own `go.mod`), so `cd tsunami && go vet ./...`
     (added to Phase 3 verification below) does not cover them either, and neither does anything
     else in this plan. Treat the 8 demo `go.mod` files as **cosmetic-only and unverifiable**:
     rewrite their module-path strings for consistency if touched at all, but do not treat
     rewriting them as satisfying any compile obligation, and do not let their presence inflate
     the load-bearing status of `tsunami/go.mod` and the two `.go.tmpl` templates, which remain
     genuinely load-bearing (referenced by the root module graph and copied into
     `dist/tsunamiscaffold/` at build time, respectively). Optionally repoint each demo's dead
     `replace` to `../..` in the same commit as a drive-by fix, independent of the rename.
   - **`pkg/wconfig/defaultconfig/settings.json`'s `web:defaulturl`** (handled in Phase 1 step 2)
     and any other non-`.go` file carrying the module path as a literal string (the schema file
     `schema/settings.json`'s `$id` is generated by `build:schema` and self-corrects once the
     module path changes — no separate action needed there) must not be swept up by a
     module-path sed scoped to `*.go`; they are handled by their owning phase instead.
   - **Package renames need a named tool and a recipe that matches how that tool actually
     works — a `git mv`-then-rename-tool sequence is not followable, because the tool moves the
     directory itself.** Renaming `pkg/wavebase` to its resolved new name is not a string
     substitution on the import-path prefix: it requires moving the directory, rewriting the
     `package` clause in every file inside it, rewriting every package-qualified selector that
     references it from *other* packages (e.g. `wavebase.GetWaveDataDir()` appearing anywhere
     outside `pkg/wavebase` itself), and rewriting the directory segment inside every import
     path that names it. A repo-wide scan for package-qualified selectors on the eleven
     packages' pre-rename names returns 2,071 occurrences across 101 files in `pkg/` alone,
     covering only 7 of the 11 — the true count including `cmd/` and `emain/` references is
     larger. A plain `sed` on the import-path prefix leaves every one of these unrewritten,
     producing a tree where files compile against a package name that no longer matches its
     directory — exactly the intermediate breakage the project's "no `go build`, trust VSCode's
     problems pane" convention cannot verify without opening every file.
     **`golang.org/x/tools/cmd/gomvpkg` is not a viable tool for this** — it is absent from the
     latest `x/tools` release, and its underlying engine (`golang.org/x/tools/refactor/rename`)
     is deprecated with its sibling CLI (`gorename`) removed upstream; do not name it as an
     option. **Use `gopls rename` on the package clause, supplying the new full import path**
     — per gopls's own documentation, this single operation both moves every file in the package
     to the new directory *and* rewrites every importer's path, which is the opposite order from
     "git mv first, then rename": running `git mv` before `gopls rename` leaves the tree
     non-type-checking (every importer's path now dangles) at exactly the moment a
     type-checking-driven tool needs a coherent tree to work from, so **do not `git mv` first**.
     Correct sequence per package: (1) confirm the tree currently type-checks (this is why
     package renames happen one at a time, not as one blanket operation across all eleven); (2)
     run `gopls rename` on that package's `package` clause with the resolved new import path,
     setting `renameMovesSubpackages: true` for `pkg/wshrpc` (which has subpackages
     `wshclient`/`wshserver`/`wshremote`) and `pkg/wconfig` (which has subpackage
     `defaultconfig`) — gopls does not move subpackages by default, and a parent-only rename
     leaves a half-moved tree; **(2.5) VERIFIED 2026-09-20 by live dry-run (`gopls rename -w`
     v0.23.0, tested on `pkg/wavejwt`→`pkg/remotetermjwt` in a disposable worktree, not just
     read from documentation): `gopls rename` moves the file(s) and rewrites every importer's
     import path and call-site selector correctly, but does NOT update the moved file's own
     `package <oldname>` declaration line — it leaves the physically-relocated file still
     declaring the old package name. This is silent and does not error at rename time; Go
     permits a directory name and its package's declared name to differ, so nothing complains
     until the mismatch collides with the now-updated importers expecting the new name
     (confirmed: `go build ./...` fails with "imported as `<old>` and not used" /
     "undefined: `<new>`" across every importer, until this one line is fixed). After each
     package's `gopls rename` call, and before moving to the next package, manually fix the
     package clause in the moved file(s): `sed -i 's/^package <oldname>$/package <newname>/'
     <path-to-moved-file(s)>` (every file gopls moved, if the package has more than one file —
     check with `gopls rename -l` first, or `git status` after the move). This is a required
     step the tool does not perform itself, not an edge case — it happened on the very first
     package tested. Also observed once during testing: combining `-w` with `-l` in the same
     invocation produced a destructive failure (the source file was deleted before the
     destination directory/file was successfully created, exit code 2, "no such file or
     directory") when the target directory didn't already exist — **do not combine `-w` and `-l`
     in one call**; run `-l` (or `-d` for a full diff) separately first to preview, then `-w`
     alone to write, and check `git status` immediately after each package to confirm the
     expected file(s) exist at both old (should be gone) and new (should exist) paths before
     proceeding to the next package.** (3) `git add -A` to record the tool's own file moves
     (there is nothing to `git mv`, the tool already moved the files) — expect `git status` to
     show this as a delete-plus-new-untracked-file pair rather than a tracked rename (gopls does
     not `git mv`), which is fine for the commit but means `git log --follow` on the new path
     is needed later to see pre-rename history, not a plain `git log`; (4) run `task generate` after any
     package touching `wshrpctypes.go`, since a partial-tree `go run` can fail for one package's
     rename and mask a problem in the next. gopls also refuses to rename `package main` and
     `_test` packages — for the 16 `_test.go` files in scope, expect to rename the package they
     belong to via its non-test files and confirm the test files picked up the new package name
     automatically (gopls keeps same-directory test files in sync); verify this rather than
     assume it. Squash all eleven packages' work into the single atomic commit this step
     requires — the per-package ordering above is about correctness of execution, not about
     creating eleven separate commits. **Pre-flight check, RESOLVED 2026-09-20: `gopls` was not
     installed anywhere on this machine (checked PATH, `$GOPATH/bin`, and the vendored
     `golang-1.26.2/bin`, which ships only `go`/`gofmt`); installed via `go install
     golang.org/x/tools/gopls@latest` using the vendored toolchain, resulting in
     `golang.org/x/tools/gopls v0.23.0` at `$HOME/go/bin/gopls`. Confirm this binary is still
     present at execution time — a fresh environment (new container, different machine) needs
     this install step repeated.** Package list and final names, resolved
     in Prerequisites: `pkg/wavebase`, `pkg/waveobj`, `pkg/waveapp`, `pkg/waveapputil`,
     `pkg/waveappstore`, `pkg/wavejwt`, `pkg/wshrpc`, `pkg/wshutil`, `pkg/wcore`, `pkg/wconfig`,
     `pkg/wstore`. **Their exported `Wave*` type names (`WaveObj`, `WaveAIStreamRequest`, etc.)
     are pinned as do-not-rename this pass** — see Prerequisites for the full reasoning
     (renaming them breaks 38 hand-written frontend references with no phase assigned to fix
     them); `gopls rename`'s package-clause operation does not rename these identifiers, so this
     carve-out requires no special handling here, only restraint from renaming them separately.
     **`pkg/waveai` is not in this list — confirmed removed** (`git log --diff-filter=D --
     'pkg/waveai*'` → `a08c2d74`); see "Checked and Clear", which is now the single source of
     truth for this — do not re-add it here or to Phase 4 step 10.
   - **Also decide and apply in this same commit (all resolved in Prerequisites): the
     `wavesrv` binary name / RPC route identifier** (`DefaultRoute`/`ConnHostWaveSrv` in
     `pkg/wshutil/wshrouter.go`/`pkg/remote/connparse/connparse.go`, plus
     `electron-builder.config.cjs`'s three `wavesrv` match points and `emain-platform.ts`'s
     `wavesrv` binary-name construction, if the decision is to rename it — if so, the router
     must accept both the old and new route strings for one release, per Prerequisites) **and
     the local on-disk marker/output filenames**: `WaveLockFile = "wave.lock"` /
     `DomainSocketBaseName = "wave.sock"` and, alongside them, the two shell-integration output
     filenames `pkg/util/shellutil/shellutil.go` hardcodes — `"wave.fish"`
     (`GetLocalWaveFishFilePath()`) and `"wavepwsh.ps1"` (`GetLocalWavePwshFilePath()`) — all
     four are in `pkg/wavebase`/`pkg/util/shellutil`, packages already being touched in this
     commit. If any of these filenames' *values* also rename (e.g. `wave.lock` ->
     `remoteterm.lock`, `wave.fish` -> `remoteterm.fish`), note explicitly that this only
     affects data written going forward — Phase 2's migration always runs before this phase
     touches these values, so its legacy-marker check keeps looking for the old `wave.lock`
     name regardless, and that divergence (old name to detect legacy data, new name for
     everything created after) is intentional, not a bug to reconcile.
   - **Windows packaging: `build/deb-postinstall.tpl` is Linux-only and out of this bullet's
     scope, but its own hardcoded `/opt/Wave/waveterm` paths need a Phase 5 decision** — see
     Phase 5 for the fix; noted here only because it's wired in via `electron-builder.config.cjs`
     the same file this bullet's `wavesrv` match points live in.
   - **Cross-check the Taskfile dependency this phase would otherwise silently break**, even
     though `Taskfile.yml` itself is out of scope this pass (Phase 5 steps 14/15): `task
     build:schema`'s `sources:` glob is `pkg/wconfig/*.go` — renaming that directory makes the
     glob match nothing, and `build:schema` is a dependency of `task generate` itself. Separately,
     `-ldflags -X main.WaveVersion={{.VERSION}}` binds `pkg/wavebase/wavebase.go`'s
     `WaveVersion` var by name on two build targets (`cmd/server`, `cmd/wsh`) — Go silently
     no-ops an `-X` flag targeting a symbol that no longer exists, so a renamed `WaveVersion`
     makes every build report version `0.0.0` with no compiler error, propagating into the About
     modal, `wsh version`, the remote version check, and `WAVETERM_VERSION`/`REMOTETERM_VERSION`
     in shells. Before running the package-rename sed, grep `Taskfile.yml` for every
     `pkg/wavebase`/`pkg/wconfig`/`main.WaveVersion`-shaped reference and either update the
     Taskfile in this same commit (narrower carve-out than the full Phase 5 deferral — this is
     "identifiers Phase 3 invalidates", not "branded infra strings", which is what the Taskfile
     skip decision was actually about) or explicitly pin `pkg/wconfig` and `WaveVersion` as
     do-not-rename this pass.
   - **Risk (applies only if the repo owner opts into the separate, larger decision to rename
     exported type names too — see the do-not-rename default above):** renaming exported Go
     type names (`WaveObj` etc.) that are also referenced by name in Go struct tags, JSON field
     names, or DB column mappings (`pkg/util/dbutil`, `db/migrations-wstore/*.sql`) would be a
     silent runtime break invisible to both `gofmt` and VSCode's problems pane. Before renaming
     any exported type, grep for its name as a *string literal* (not just as a Go identifier)
     across `pkg/`, `db/migrations-*/`, `frontend/types/gotypes.d.ts`, `Taskfile.yml`,
     `.github/workflows/`, `electron-builder.config.cjs`, **and `git ls-files '*_test.go'`** —
     an earlier version of this scope omitted test files, but at least one already does exactly
     what this risk warns about: `pkg/tsgen/tsgenevent_test.go:19`,
     `strings.Contains(waveEventTypeDecl, "WaveEventName")`, asserts against a type name as a
     hardcoded string literal. A Go-aware rename tool rewrites the identifier everywhere it's
     referenced as code but has no reason to touch a string literal that merely spells the same
     text for an unrelated assertion, so a renamed type silently breaks this kind of test —
     and this project's own convention of never running `go test` in CI or `Taskfile.yml` (only
     compiling test binaries, per Phase 3 verification item 5) means a broken assertion of this
     shape has no path to being caught by anything in this plan. If exported types are renamed,
     add a step that actually *runs* `go test ./...` once, manually, after the rename — not just
     compiles it — specifically because this is the one point where that project convention
     would otherwise let a real regression through permanently. Struct tags, generated TS types,
     and build-flag references all embed the Go type/symbol name as text, not just as a Go
     identifier a type-aware tool would catch.
   - **Risk:** `frontend/types/gotypes.d.ts` is generated from `pkg/wshrpc/wshrpctypes.go` via
     `task generate`. Any type rename inside `wshrpctypes.go` requires running `task generate`
     in the *same commit*, otherwise the frontend's checked-in generated types silently
     diverge from the Go source of truth until someone happens to run it — this is called out
     explicitly by the project's own CLAUDE.md/rules.md and is easy to miss because nothing
     enforces it at commit time.
   - **Rollback position:** point of no return once merged past this commit if any other
     branch or open PR still imports the old module path — this is why it must land as one
     atomic commit with nothing else in flight. Before this commit: fully reversible (git
     revert of a single commit). After this commit, reverting is still mechanically possible
     (revert is symmetric) but costly if other work has branched off it in the meantime — treat
     it as a stop-the-world change: land it when no other feature branches are open against Go
     code. Confirm this immediately before execution per Prerequisites' worktree/branch check —
     nine agent worktrees exist under `.claude/worktrees/` as of this writing; that count must
     be re-checked (not assumed) at execution time, since it changes daily.
   - **What NOT to rename in this step:** any `wavetermdev/waveterm`-shaped string that is a
     genuine external reference — e.g. a doc comment linking to the *upstream* GitHub repo for
     attribution/historical context, or (if present) a `replace` directive or comment citing
     upstream for a specific behavioural decision. Grep the hits for `// See
     https://github.com/wavetermdev/waveterm` or similar comment-only references before the
     blanket `sed` pass and exclude them — none were spotted in the files read directly during
     this planning pass, but the strict inventory list wasn't read in full; **treat this as
     unverified and re-check immediately before running the `sed` pass** (a `grep -B2
     "wavetermdev/waveterm"` on every hit, filtered to lines starting with `//` or inside
     `/* */`, is the check). **Persist this pre-sed check's output as a committed file**
     (e.g. `RENAME_ALLOWLIST.md`, alongside the sed script) rather than a one-off grep whose
     result only exists in a terminal scrollback — Phase 6's residual sweep needs a concrete,
     durable artefact to diff against, not a reconstruction of what Phase 3 excluded.

Verification for Phase 3, as an ordered gate (round-1 audit found the original grep+gofmt-only
criteria pass clean on trees that cannot actually compile — see the two compile signals added
below):
1. `task generate` **exits 0**. This is `go run` under the hood (`Taskfile.yml`'s `generate`
   task and its `build:schema` dependency both invoke `go run`), which is a real compile of the
   `cmd/generatets`, `cmd/generatego`, and `cmd/generateschema` packages and their transitive
   imports — treat a non-zero exit as the primary compile-failure signal for this phase, and
   note that invoking it via `task generate` (not `go build`/`go run` directly) does not violate
   the project's "no `go build`" rule, since the task already exists and is already run for
   other reasons.
2. Diff `frontend/types/gotypes.d.ts` — if package/type renames touched `wshrpctypes.go`, the
   diff must be non-empty and committed in this same phase, not deferred. **Also confirm
   `frontend/app/store/wshclientapi.ts`** (also generated by `task generate`, also covered by
   the project's "do not hand-edit generated files" rule in `rules.md`) reflects the same
   renames — it is generated from the same source and is excluded from Phase 4's file list for
   the same reason `gotypes.d.ts` is (see Phase 4 step 10).
3. `git ls-files -z '*.go' '*.mod' '*.tmpl' '*.json' | xargs -0 grep -l "wavetermdev/waveterm"`
   (not `--include=*.go`, and now including `*.json` — `schema/settings.json`'s `$id` field
   carries the module path and is generated by `build:schema`; it "self-corrects" only if
   regenerated *and committed* in this phase, and a scope that excludes `*.json` would report a
   stale, uncommitted `$id` as clean) returns only the allowlisted attribution references
   recorded in `RENAME_ALLOWLIST.md` above, plus anything deliberately excluded per the
   `tsunami/` decision.
4. `gofmt -l $(git ls-files '*.go')` returns empty — gitignore-scoped, not bare `.` (see the
   worktree/vendored-toolchain note above). Use whichever `go`/`gofmt` binary this project's own
   tooling uses — `Taskfile.yml` pins a vendored toolchain (`GO_DIR: "golang-1.26.2"`) and
   routes every Go invocation through its own `{{.GO}}` template variable; run this and item 5
   via `task`-wrapped invocations (e.g. add a scratch `task` target, or resolve `{{.GO}}`'s
   value from `Taskfile.yml` and invoke that binary directly) rather than a bare system `go`,
   since the system PATH may not have a matching version. `{{.GO}}` itself is Taskfile template
   syntax, not something to paste into a shell literally — an earlier version of this
   verification block did exactly that.
5. `go vet ./...` (via the project's pinned Go binary, per item 4's note — or `go test ./...
   -run XXX -count=1`, which compiles every test binary without running any test) — `task
   generate`'s `go run` only compiles the three generator `main` packages' transitive imports,
   which does **not** include the 16 `_test.go` files carrying the module path; without this
   step those files are unverified. **`go vet ./...` from the repo root covers the root module
   only — run it a second time from `tsunami/` (`cd tsunami && go vet ./...`) to cover the
   module that Prerequisites' tsunami decision and this step's import-path-pattern exclusion are
   most likely to get wrong.** If the project's "no `go build`" rule is read strictly enough to
   exclude `go vet`/`go test -run XXX` too, state that explicitly and add a Phase 6 item to run
   the suite once instead of silently leaving it unverified.
6. Push to a throwaway branch and confirm `build-macos-ci.yml` (which triggers on every branch
   push and runs `task package` -> `build:backend` -> real `go build` of `wavesrv` and `wsh`
   across 8 GOOS/GOARCH pairs) is green — this is Phase 3's actual full-build compile signal,
   and it runs automatically on push whether this checklist asks for it or not; naming it here
   moves it from an implicit side effect to an explicit, owned exit criterion, with the run URL
   recorded in the PR description. No manual `go build` is being introduced by this — the CI
   workflow already exists and already does this on every branch push.
No local `go build` is run directly by a human or an agent at any point in this phase — every
compile signal above is either a `task`-wrapped `go run`/`go vet` invocation the project already
uses, or CI that already runs on push regardless. **On any gate failure above: do not commit, do
not proceed to Phase 4, report the failing gate and its output to the repo owner** — this is the
phase whose rollback cost jumps sharply (see Rollback Strategy), so it is also the phase least
suited to an executor silently deciding how to proceed past a failed check. The same go/no-go
rule applies at every phase's verification gate in this plan, not only Phase 3's; stated once
here because Phase 3's consequences of proceeding past a failure are the most severe.

**Phase 4: Frontend internal module renames**

10. `frontend/wave.ts` -> `frontend/remoteterm.ts` (or equivalent; confirm naming with repo
    owner — filenames must stay lowercase per project TS convention), `frontend/util/waveutil.ts`,
    `frontend/types/waveevent.d.ts`, `frontend/app/waveenv/` directory,
    `frontend/app/view/waveconfig/`, all `@/` import paths referencing these files
    (search-and-replace of the import path string, mechanical, but must happen file-by-file
    rather than blind `sed` since `@/waveenv/...` also appears inside comments and doc strings
    that should be reworded, not just path-substituted). **`frontend/app/view/waveai/` does not
    exist — do not include it.** **`frontend/types/gotypes.d.ts` and
    `frontend/app/store/wshclientapi.ts` are excluded from this step entirely** — both are
    generated by `task generate` from Go source and the project's own `rules.md` forbids
    hand-editing either; they are handled in Phase 3 (their content already reflects the Phase 3
    renames by the time this phase starts, per the dependency below), not re-touched here even
    though both would otherwise match this step's `wave`-token file list. (Round-1 audit: the "Checked and Clear" resolution correctly
    records `pkg/waveai` as removed, but this step still carried a stale "confirm still exists"
    note pointing at a directory that a repo-wide glob confirms is not present; the only
    remaining `waveai`-named artefacts are `aiprompts/waveai-*.md` (Phase 1) and
    `docs/docs/img/waveai-model-dropdown.png` (gated on the Phase 1 docs scope decision).)
    - **`index.html:14`** (`<script type="module" src="frontend/wave.ts">`) — the
      Vite/electron-vite renderer entry point. Not a `@/` import and not a `.ts`/`.tsx` file, so
      neither this step's original file list nor its verification grep (`@/wave` pattern,
      `frontend/` only) would catch it; renaming `frontend/wave.ts` without updating this line
      breaks the renderer entry point in a way `tsc --noEmit` cannot see (HTML isn't typechecked)
      and vitest doesn't exercise (it doesn't build the renderer bundle). Update this line in the
      same commit as the file rename.
    - **`frontend/builder/` and `frontend/app/view/tsunami/`** (the "WaveApp Builder"
      sub-feature — naming decision resolved in Prerequisites) and **`emain/emain-log.ts`'s**
      `^waveapp\.(\d+)\.log$` filename pattern: rename per the Prerequisites decision, in this
      phase, not left to Phase 6's residual sweep to catch unplanned.
    - **`emain/` (16 files, ~430 case-insensitive `wave` hits) is now in this phase's scope**
      (see Prerequisites) alongside `frontend/`: this phase's title, file list, and verification
      previously covered `frontend/` only, which left `emain/`'s internal identifiers
      (`WaveBrowserWindow`, `createNewWaveWindow`, `getWaveWindowById`, `getWaveSrvPath`,
      `runWaveSrv`, `getWaveVersion`, etc.) with no owning phase — and made Phase 6's "residual
      sweep returns only allowlisted hits" exit criterion unachievable, since those ~430 hits
      would never be allowlistable third-party/attribution content. Rename `emain/`'s internal
      identifiers alongside `frontend/`'s in this phase.
    - **Depends on Phase 3 being complete**: `frontend/types/gotypes.d.ts` (Go-generated) must
      already reflect the Phase 3 renames before frontend code importing those types is
      touched, otherwise this phase's diff mixes "renamed because of the module rename" with
      "renamed because the generated types changed," which is exactly the kind of dependency
      inversion the plan format's verification checklist flags — Phase 4 cannot start until
      Phase 3's `task generate` output has landed.
    - **Rollback position:** reversible via `git revert` — a frontend-only rename with no
      persisted user data or remote-side state to reconcile (unlike Phases 2/3). The only
      caveat is re-running `git ls-files` scoped greps after a revert to confirm no stray
      `@/remoteterm`-style import survives a partial revert.
11. Custom `wave://` Monaco URI scheme (`frontend/app/monaco/monaco-react.tsx`,
    `schemaendpoints.ts`, 2 files only, confirmed by independent re-check — no OS-level
    protocol-handler registration exists anywhere else) -> `remoteterm://`. **RESOLVED
    2026-09-20 (round 2): no shim needed.** The persistence question this step originally
    flagged as UNCERTAIN is answered — there is no `localStorage`, `sessionStorage`, or
    `indexedDB` usage anywhere under `frontend/`; every `wave://` use is either an in-memory
    Monaco model URI (`monaco-react.tsx:11,177,178`) or a static schema-association URI
    (`schemaendpoints.ts:17,22,27,32`), neither persisted across app restarts. Rename directly,
    no compatibility shim required.
    - **Rollback position:** reversible (2 files, no persisted state to reconcile).
12. `cmd/wsh` CLI binary name and help text: the binary name itself (`wsh`) was not flagged by
    the requester as in-scope for renaming (env vars, `wshrpc`, Go packages under `pkg/wsh*`
    were flagged, but the *binary name* `wsh` is a user-facing CLI command name that scripts,
    aliases, and muscle memory depend on) — **treat the `wsh` binary name as out of scope
    unless the repo owner explicitly confirms otherwise**; only its help text / usage strings
    that say "Wave"/"WaveTerm" move in Phase 1 (moved there — see Phase 1's `cmd/wsh/cmd/*.go`
    Cobra help-text bullet, since this string-literal work doesn't depend on anything Phase 3/4
    produce and doesn't need to wait). **`wavesrv`'s binary name and RPC route identifier are a
    separate decision**, resolved in Prerequisites and executed in Phase 3 (it is a Go-side
    rename with a wire-protocol dimension, not a frontend one) — do not conflate the two or
    assume `wsh`'s "out of scope" treatment also covers `wavesrv`.
    - **`wsh wavepath` is a real Cobra subcommand verb (`wshcmd-wavepath.go:18`, `Use:
      "wavepath {config|data|log}"`) that sits ambiguously between "help/usage string" (which
      this step says moves) and "binary/command name" (which this step says stays)** — a user
      types `wsh wavepath data` to print a resolved path, exactly the kind of literal,
      scriptable command-name surface that justifies leaving the binary name itself alone.
      Decide explicitly whether the subcommand verb itself renames (e.g. to `wsh remotepath`,
      breaking any existing script that calls `wsh wavepath`) or stays (leaving one literal
      "wave" verb in an otherwise fully-renamed CLI) — do not let it default silently to either
      rule from this step's other two categories; record the decision here, in Prerequisites, or
      wherever the `wsh` binary-name decision itself gets recorded, alongside the `wavesrv`
      decision it already sits next to.
    - **Rollback position:** reversible (string-only edits; the binary name itself doesn't
      move, so there's no compatibility surface to unwind).

Verification for Phase 4: `npx tsc --noEmit` (per project rule: this is the TS equivalent of
"no `go build`" — the project already uses this as its typecheck gate via `task check:ts`,
so it is not introducing a new verification step, just using the one that already exists).
**The residual-import grep must match the actual import specifiers this step renames, not the
literal string `@/wave`** — an earlier version of this gate (`grep -rn "@/wave" frontend/
emain/`) matches zero files even before any rename work starts, because none of the real
specifiers begin with exactly that string: use `grep -rnE "@/(app/waveenv|util/waveutil|app/
view/waveconfig|types/waveevent)" frontend/ emain/ | grep -v node_modules`, which should return
zero hits outside anything explicitly deferred (none expected) once this phase's renames land.
Run `npm run test` (vitest) — the 6 flagged `.test.ts(x)` files touch code paths in this phase's
scope and must stay green. Run `npm run build:dev` (or `task build:frontend:dev`) — the only
check in this phase that exercises actual module resolution for the *renderer bundle and its
entry point* rather than just type-checking; `tsc --noEmit` alone would not have caught the
`index.html` entry-point break described above. On any gate failure: do not commit, do not
proceed to Phase 5; report to the repo owner (see Phase 3's verification for the general rule).

**Phase 5: CI/build**

13. `.github/workflows/bump-version.yml`: rename `WAVETERM_VERSION` output var to
    `REMOTETERM_VERSION` (internal to the workflow, zero external dependents — safe,
    mechanical). Leave `WAVE_BUILDER_APPID`/`WAVE_BUILDER_KEY` var/secret *names* alone unless
    the repo owner confirms the underlying GitHub App and repo secrets have also been
    renamed/recreated (renaming the reference without the secret existing under the new name
    breaks every future version bump silently until someone runs the workflow) — this is the
    Prerequisites item above, repeated here because it's a hard blocker for this specific
    step, not just a nice-to-know. **`build-helper.yml` has its own independent
    `WAVETERM_VERSION`** (not the same variable as `bump-version.yml`'s, just the same name —
    confirmed via direct read, lines 19 and 95) — fold this file's rename into this same step
    rather than leaving it to step 17's separate, still-unread pass; step 17 below is narrowed
    to cover only what's left in that file after this.
    - **Rollback position:** reversible (workflow-internal variable, zero external dependents).
14. **Restated for precision (round-2 audit found the original phrasing read as covering more
    than the repo owner actually decided): DECIDED 2026-09-20 by repo owner — skip
    `Taskfile.yml`'s release-infra strings this pass**, not the whole file. `RELEASES_BUCKET:
    dl.waveterm.dev/releases-w2`, `ARTIFACTS_BUCKET: waveterm-github-artifacts/staging-w2`, and
    the WCLOUD_* endpoint vars stay untouched — no release infra exists to rename toward.
    **Three narrow carve-outs are explicitly still in scope, already required elsewhere in this
    plan, and are not covered by this deferral**: (a) `WAVETERM_ENVFILE`/
    `WAVETERM_NOCONFIRMQUIT` renamed in the same commit as Phase 2 step 6's Electron-side
    rename (these are dev-only knobs, not release infra); (b) `pkg/wconfig/*.go`'s
    `build:schema` source glob and `main.WaveVersion`'s `-ldflags -X` target, cross-checked and
    fixed in the same commit as Phase 3 step 9's package rename (these are identifiers Phase 3
    invalidates, not release-infra strings); (c) `APP_NAME`/`WINGET_PACKAGE` and other
    packaging-metadata vars remain deferred with the release-bucket strings, since there is no
    real packaging pipeline to point them at yet. Revisit the full-file deferral once real
    RemoteTerm release infra exists — do not resurrect this step speculatively in a later phase
    without a fresh owner decision, and do not read carve-outs (a)/(b) above as reopening this
    decision, since both were already required by their respective phases before this
    clarification.
    - **Rollback position:** n/a — no edit in this phase's scope; the file is deliberately
      untouched here except for carve-outs (a)/(b), whose rollback positions are covered under
      Phase 2 step 6 and Phase 3 step 9 respectively.
15. **Also deferred with step 14** (same file, same owner decision, release-infra strings
    only): the stale `artifacts:snap:publish:*` `waveterm_{{.UP_VERSION}}_*.snap` glob (already
    inconsistent with `package.json`'s `"name": "remoteterm"` — the real snap artifact today
    would be `remoteterm_*.snap`). Pre-existing bug, independent of this rename; leave it for
    whoever touches `Taskfile.yml` next.
    - **Rollback position:** n/a — no edit in this phase's scope.
16. `CNAME`: `docs.waveterm.dev` -> only touch if Phase 1's docs-site scope decision
    (below) is "keep and rebrand." If the decision is "drop docs/ entirely," delete `CNAME`
    instead of renaming it. This step cannot execute before that scope decision is made.
    - **Rollback position:** reversible in git, **not in DNS propagation** — deleting or
      repointing `CNAME` is a real-world DNS change with a TTL burn once it's live; a `git
      revert` restores the file instantly but does not un-propagate whatever DNS state already
      resolved in the interim. Treat this step's real-world effect as slower to undo than its
      git history suggests.
17. `.github/workflows/build-helper.yml`: **narrowed by step 13 above**, which already covers
    this file's `WAVETERM_VERSION` (lines 19, 95). What's left in this file: a user-visible
    release title string at line 196 (`"Wave Terminal ... Release"`) — **move this string's
    rename to Phase 1** (it's user-facing prose, the same category Phase 1 step 1/2 already
    cover, not CI-mechanics work). **Round-3 audit independently re-grepped this file in full:
    only 3 wave hits exist total (lines 19, 95, 196), and all 3 are accounted for by step 13 and
    this step — the file is fully classified, not partially, despite an earlier version of this
    plan hedging that it wasn't.**
    - **Rollback position:** reversible (CI-config-only, no external dependents beyond the
      workflow itself).
18. `build/deb-postinstall.tpl` (wired in via `electron-builder.config.cjs`'s `deb: {
    afterInstall: "build/deb-postinstall.tpl" }`, and exercised by real CI — `build-helper.yml`
    references `make/*.deb` as a build artifact): hardcodes `/opt/Wave/waveterm` three times and
    `/opt/Wave/chrome-sandbox` once, and runs `update-alternatives --install '/usr/bin/waveterm'
    'waveterm' '/opt/Wave/waveterm' 100` on every `.deb` install. **This script is already
    broken today, independent of any further rename work**: `package.json`'s `productName` is
    already `"RemoteTerm"` (Prerequisites' north star), and electron-builder's `deb` target
    packages the app under `/opt/${productName}/` by its own convention — meaning the real
    install path on the *current* tree is already `/opt/RemoteTerm/`, not `/opt/Wave/`, so any
    `.deb` built from this tree today already creates an `update-alternatives` symlink pointing
    at a path the packaged files aren't actually installed under, silently leaving the
    `waveterm` CLI launcher non-functional. Fix both problems in this step: update
    `/opt/Wave/waveterm`/`/opt/Wave/chrome-sandbox` and the `update-alternatives` symlink name
    to match `productName`'s actual current value, and (separately) update them again to match
    the resolved `wsh`/`wavesrv`-adjacent binary-name decision from Prerequisites if that
    decision changes the launcher name further. Flag explicitly in the commit message that the
    first half of this fix is a pre-existing bug this rename plan is simply the first work
    positioned to catch, not new breakage introduced by the rename.
    - **Rollback position:** reversible (a build-time packaging script, no persisted state).

Verification for Phase 5: this is CI-config-only; there is no local build step that exercises
it. Verification is: push to a throwaway branch and confirm `build-macos-ci.yml` (which
triggers on every branch push) still runs green, since it depends on `task package` which
depends on everything upstream of it in Taskfile.yml. Do not merge Phase 5 without at least
one green CI run on the exact commit. On gate failure: do not merge, report to the repo owner
(see Phase 3's verification for the general rule).

**Two Phase 5 decisions RESOLVED 2026-09-22 by repo owner:**
- **Step 13, `WAVE_BUILDER_APPID`/`WAVE_BUILDER_KEY`: deferred**, not renamed. No RemoteTerm-named
  GitHub App or repo secret exists yet under a new name. Context for the deferral: the upstream
  Wave Terminal maintainer has been inactive for a while and this fork is not expected to keep
  tracking upstream `main` — it's becoming its own standalone, de-facto-maintained project. Bundle
  this rename into a future full-infra-standup pass alongside the other still-deferred
  release-infra strings (steps 14/15's `Taskfile.yml` carve-out) once a real RemoteTerm GitHub App
  + secrets are actually created — not before. `WAVETERM_VERSION`->`REMOTETERM_VERSION` (the
  workflow-internal output var, zero external dependents) was renamed in both
  `bump-version.yml` and `build-helper.yml` as planned; only the App/secret var *names* are held
  back.
- **Step 16, `CNAME`: renamed to `docs.rterm.dev`**, not `docs.remoteterm.dev`. Owner is buying
  `rterm.dev` as the project's real new domain for docs hosting — separate from `remoteterm.dev`,
  which stays the root/homepage domain already in place from Phase 1 (`package.json` `homepage`,
  `about.tsx`, README badges). This is a deliberate two-domain split, not an inconsistency — do
  not "fix" `remoteterm.dev` references toward `rterm.dev` without a fresh owner decision.
  Real-world DNS for `rterm.dev` may not exist yet; treat the committed `CNAME` value as staged
  ahead of DNS being live, same caution as this step's Rollback Position note above.

**Phase 6: Cleanup pass + residual-reference grep sweep**

18. Re-run the inventory scan against the post-Phase-5 tree using **this literal command**,
    inlined here rather than referenced from an external agent's method (round-2 audit found
    the original phrasing — "get it from the `waveterm-name-scan` agent's method if not already
    documented" — pointed at a session-scoped scratchpad that this fleet's own convention says
    does not survive session cleanup; by Phase 6 execution time, weeks after Phase 1, that
    scratchpad is very likely gone, leaving the plan's own finish-line gate unreproducible):
    `git ls-files -z | xargs -0 grep -ilZ wave | tr '\0' '\n' > hits_loose.txt` for the loose,
    case-insensitive, whole-file-match scan, and
    `git ls-files -z '*.go' '*.ts' '*.tsx' '*.md' '*.yml' '*.yaml' '*.json' | xargs -0 grep -ilZ
    'wave' | tr '\0' '\n' > hits_strict.txt` for the strict, source/doc/config-scoped variant —
    both `git ls-files`-based (not a raw filesystem walk, for the same gitignored-directory
    reason as Phase 3's sed). **Commit both output files alongside `RENAME_ALLOWLIST.md`** so
    this gate has a durable, versioned baseline rather than a re-derived one each time it runs.
    Anything still matching in either file is either: (a) a deliberately preserved
    upstream-attribution reference — check against `RENAME_ALLOWLIST.md`, the committed
    artefact Phase 3 step 9 produces, not a re-derived guess; (b) a genuine miss; (c) a
    `node_modules`/generated/vendored file that's expected to still say "wave" (e.g.
    `node_modules/bare-path/NOTICE` — an unrelated third-party package's own NOTICE file, not
    this project's; do not touch third-party package metadata); or (d) **expected,
    out-of-scope-by-decision residue**: orphaned remote-host `.waveterm` directories (per the
    Prerequisites decision on remote-host state — if the decision was "do not migrate remote
    hosts", their leftover files are not a miss), `wsh`/`wavesrv` names if either was
    deliberately held out of scope, and app-icon/logo image files if the Prerequisites decision
    was to keep using Wave Terminal's existing artwork under a rights determination rather than
    ship new art (image files carry no renameable string content, so this scan wouldn't find
    them as text hits anyway — noted here so the human review doesn't assume image-based
    branding was covered by this gate at all). Category (d) should be enumerated explicitly in
    the sweep's output alongside (a)-(c), not silently folded into "genuine miss."
    - **Rollback position:** n/a — this step is verification only, no edits of its own.
19. Remove any now-dead compatibility code: if Phase 2's data-dir migration shim was written
    as a permanent check-on-every-launch rather than a true one-time migration, this is the
    phase to confirm it's cheap enough to leave in permanently (recommended — removing it
    later has no clean trigger for "enough time has passed") rather than a note to strip it.
    - **Rollback position:** reversible (this step only removes code already established as
      dead; reverting restores it, at worst reintroducing an inert code path).

Verification for Phase 6: the residual-reference grep sweep (step 18's committed
`hits_loose.txt`/`hits_strict.txt`) returns only the allowlisted attribution/third-party hits
(category a, checked against `RENAME_ALLOWLIST.md`) and the explicitly-expected residue
(category d), both enumerated in the sweep's output and checked by a human, not just "count
went down." On gate failure (a genuine miss found): fix it in this phase, re-run the scan; do
not sign off the migration as complete with an open genuine-miss item.

### Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Package rename executed as a plain string-substitution sed instead of a Go-aware rename, leaving 2,000+ package-qualified selectors unrewritten while the module-path prefix looks clean; or executed with `gomvpkg` (removed from upstream `x/tools`) or `git mv`-before-`gopls rename` (inverts the tool's own model, which moves the directory itself) | Medium (this is the natural reading of "scripted sed pass" without the tool/sequence named precisely) | High (tree does not compile; the project's "no `go build`" convention delays discovery) | Use `gopls rename` on the package clause, correct order (rename first, `git add -A` after — no pre-`git mv`), `renameMovesSubpackages: true` for `wshrpc`/`wconfig` — step 9 |
| Data-dir migration's env-var read-fallback (step 6) and destination-exists abort logic (step 7) combine to make the app refuse to start for every user who has a `WAVETERM_CONFIG_HOME`/`DATA_HOME`/`HOME` override set, since source and destination resolve to the same path and trip the "non-empty, no marker" abort branch | Medium (affects a specific but real cohort — anyone who customised their data location) | High (app fails to launch post-upgrade for that cohort, with no obvious cause) | Same-path short-circuit checked before any move/abort logic runs, plus an override-var-set fixture in Phase 2 verification — step 7 |
| Data-dir migration marker check uses a file (`wave.lock`) that the config-dir root never actually contains, so the migration no-ops on every real install while a synthetic test fixture reports it passing | Medium (as originally specified, effectively certain) | High (users silently lose settings on upgrade with no error, no migration, no message) | Per-root markers (`settings.json` for config, `wave.lock` for data), fixture built from a real pre-rename install rather than a synthetic seed — step 7 |
| Migration gate ("new path doesn't exist") is falsified by the app's own `mkdirSync` side effect before the check runs, so the gate fires on the vast majority of launches and migration is silently skipped | Medium-High | High (same data-loss profile as the row above, plus a second failure mode: if the shim does fire, "move onto an already-created empty directory" throws `ENOTEMPTY`/`EEXIST`) | Marker-file-written-after-successful-move as the gate, checked before the path getters run; explicit destination-exists handling — step 7 |
| Data-dir migration absorbs an unrelated `~/.config/waveterm` from a real separate Wave Terminal install | Low-Medium | High (data corruption/mixing between two distinct apps) | Validate with the corrected per-root marker check (step 7) |
| Only the config-dir path root is migrated; the data-dir root (a second, distinct `envPaths("waveterm", ...)` literal) is missed, so the sqlite workspace/session DB and `wsh` binary are left on the old path | Medium | High (looks like a clean migration — settings appear intact — while the user's entire session/workspace history is orphaned) | Enumerate and migrate all three path roots explicitly, per-platform — steps 6/7 |
| Module-path rename lands partially (some imports rewritten, some not) because of a botched sed pass, or the sed/gofmt walk hits gitignored worktrees and the vendored Go toolchain instead of just this repo's tracked files | Medium | High (nothing compiles, and the project's "no `go build`" convention means this might not surface until someone opens every file in VSCode; a raw walk also dirties nine unrelated agent worktrees) | Single atomic commit, `git ls-files`-scoped sed/gofmt, `task generate`/`go vet` as real compile signals, exhaustive residual-grep before calling Phase 3 done — step 9 |
| Phase 3's `sed` is scoped to `*.go` files only, so `tsunami/`'s separate `go.mod`, its 8 demo `go.mod` files, and its 2 runtime-consumed `.go.tmpl` templates keep the old module path while `tsunami/*.go` files inside the sed scope get rewritten, breaking the module | Medium | High (tsunami becomes unresolvable; scaffolded user apps break at runtime; the plan's own `*.go`-scoped verification would report success on a broken tree) | Explicit `tsunami/` handling decision in Prerequisites; verification widened to include `go.mod`/`.tmpl` files, not just `*.go` — step 9 |
| `task generate` not re-run after a Go type rename in `wshrpctypes.go`, frontend silently drifts from backend | Medium | Medium (type errors surface eventually via `tsc`, not silently forever, but could ship broken in between) | Explicit step in Phase 3 to run `task generate` in the same commit as any `wshrpctypes.go` edit |
| `Taskfile.yml` (deliberately out of scope for branded-string changes) hard-codes `pkg/wconfig/*.go` as a build-schema source glob and `main.WaveVersion` as an `-ldflags -X` target — Phase 3 renaming either breaks the build silently (Go ignores an `-X` flag with no matching symbol) | Medium | Medium-High (every binary silently reports version `0.0.0`; `build:schema` glob matches nothing, no compiler error either way) | String-literal grep of `Taskfile.yml`/`.github/workflows/`/`electron-builder.config.cjs` before the Phase 3 rename; narrow carve-out distinguishing "identifiers Phase 3 invalidates" from "branded infra strings" — step 9 |
| Remote-host `.waveterm` state (wsh binary, domain socket, shell integration installed on every previously-connected SSH/WSL host) has no rename or migration decision; a blanket Phase 3 identifier rename triggers `IsWshVersionUpToDate`'s self-healing reinstall path rather than a hard break, but still costs a permission-prompt-and-reinstall per host plus an orphaned old-path tree | Medium | Medium (downgraded from High after round-3 correction — "no migration path" was inaccurate; the real cost is friction, not breakage, though a *running* SSH-block terminal opened pre-rename may still error until reconnect, unconfirmed) | Explicit decision recorded in Prerequisites before Phase 3 executes; Phase 6 sweep treats resulting residue as expected once decided, not a miss |
| `wavesrv`'s RPC route identifier (`DefaultRoute`/`ConnHostWaveSrv`) crosses the same version boundary as the remote-host state above — an already-running `wsh` process or previously-installed remote binary encodes the old route string | Medium | Medium-High (if renamed without a dual-accept window, an in-flight `wsh` process silently stops routing correctly the moment the local server switches to the new route string) | Router accepts both old and new route strings for one release if "rename together" is chosen — Prerequisites, Phase 3 step 9 |
| Exported Go type renames (`WaveObj` etc.) break 38+ hand-written frontend references across 6 files outside any phase's file list, with no gate catching it until Phase 4's `tsc --noEmit`, by which point the revert-costly Phase 3 commit is already merged | Medium (natural reading of "package rename" without an explicit type-name carve-out) | High (Phase 3 commit lands a tree whose frontend silently fails to typecheck; fix becomes ad hoc rather than planned) | Exported type names pinned as do-not-rename this pass by default — Prerequisites, Phase 3 step 9 |
| `db_wave_file` (a live SQL table name, already created on every existing user's filestore database) gets "fixed" by Phase 6's residual-sweep remediation, since it's a `.go` file containing the substring "wave" but Phase 6's classification scheme has no bucket for "on-disk schema identifier that must never change" | Low-Medium (depends on how literally an executor follows step 18's "genuine miss: fix it" instruction) | High (`no such table: db_remoteterm_file` — hard runtime failure of the file-storage subsystem for every user who upgrades) | Explicit Prerequisites decision + `RENAME_ALLOWLIST.md` entry so Phase 6 treats it as allowlisted, not a miss |
| App icon/logo artwork (including electron-builder's default-convention `build/icon.*` set, distinct from the in-app `assets/wave-*` files) ships unchanged after every text identifier is renamed, since binary image content carries no string for any grep-based gate to catch | Medium (silent by construction — no verification step in this plan can detect it) | Medium (visually obvious "still Wave" signal; the one most likely to draw a trademark objection under Apache-2.0 §6) | Explicit Prerequisites decision (new art vs. rights determination) naming the full file set including `build/icon.*`; Phase 6 sweep documents this as an expected blind spot, not a false-clean signal |
| WaveApp Builder naming, `emain/` scope, and `docs/` scope decisions (Prerequisites) are not made before Phase 1/3/4 execute, because nothing forces them to resolution before the phase that depends on each one starts | Medium (these are the kind of decision easy to skip past when a phase is dispatched as a "warm-up") | Medium-High (varies by decision — from Phase 6's exit criterion being unachievable with `emain/` unscoped, to shipping the wrong sub-feature name across dozens of strings) | Each decision is now listed in Prerequisites with an explicit blocking flag on the phase(s) it gates — Phase 1 header, step 9's tsunami/WaveApp-Builder bullets |
| CI secrets (`WAVE_BUILDER_KEY` etc.) renamed in workflow YAML without matching repo-secret rename | Medium | High (release automation breaks silently until next scheduled/manual run) | Prerequisites section — confirm with repo owner before Phase 5 step 13 |
| Release infra strings (`dl.waveterm.dev`, S3 bucket names) renamed without infra existing | Low (only happens if someone free-associates the rename onto real infra strings) | High (breaks releases) | Explicit "do not execute speculatively" call-out in Phase 5 steps 14/16 |
| `build/deb-postinstall.tpl` hardcodes `/opt/Wave/waveterm`, already mismatched against `package.json`'s current `productName` even before this rename touches anything further | Low-Medium (only affects `.deb` installs, but the pipeline is confirmed live in CI) | Medium (broken `update-alternatives` symlink leaves the `waveterm` CLI launcher non-functional post-install) | Fix both the pre-existing mismatch and the rename-driven update in the same commit — Phase 5 step 18 |

### Rollback Strategy

Phases 1, 4, 5, 6 are each individually revertible with a single `git revert` — none of them
touch a shared, hard-to-reverse resource once landed. **Phase 2 is not in this group**, despite
an earlier draft of this section grouping it there: reverting Phase 2's *code* with `git revert`
does not move user data back. The reverted build reads the old path while the user's data now
sits at the new path (the migration already moved it), which looks identical to data loss until
someone notices and manually reverse-migrates — no reverse-migration step exists anywhere in
this plan, and none is needed if Phase 2 is not reverted, but "revertible with a single `git
revert`" is the wrong claim to make about it. Phase 2's actual rollback position is: code-revert
is free, but does not undo the data move; a failed *migration itself* (not a reverted commit) is
recoverable manually since the source directory was moved, not deleted. Phase 3 is the one phase
where rollback stops being free in a different way: once merged, any commit that branches off it
(including subsequent phases) also needs reverting if Phase 3 is reverted, so treat Phase 3 as
the point after which "rollback" means "revert everything built on top of it too," not a true
point of no return but a point where the cost of rollback jumps sharply. Land Phase 3 only when
no other Go-touching branches are in flight (verify immediately before landing — see
Prerequisites).

### Checked and Clear

- `LICENSE` (Apache-2.0, standard boilerplate, no project-specific strings) — read in full,
  no rename needed.
- Repo-root `NOTICE` (`Copyright 2025, Command Line Inc.`) — read in full as part of Phase 1
  step 3; retained unchanged per Apache-2.0 §4(d) (this is the upstream copyright holder's
  attribution, not a product-name string). Flagged separately here because it contains no
  "wave" substring and so is invisible to every grep-based verification in this plan — the only
  way it gets checked is a direct read, which Phase 1 step 3 now does explicitly.
- `docs/node_modules/*`, `node_modules/bare-path/NOTICE`, and other vendored/third-party
  files that happen to contain "wave"-adjacent strings (from `counts.txt`'s tail) — these are
  third-party package contents, not this project's code; confirmed by path (`node_modules/`)
  and left untouched in every phase above.
- `pkg/waveai` — **RESOLVED 2026-09-20**: confirmed removed
  (`git log --diff-filter=D -- 'pkg/waveai*'` → `a08c2d74 remove old waveai backend code`).
  Removed from Phase 3's package-rename list and from Phase 4 step 10's file list (both
  previously still carried it after this resolution was recorded — round-1 audit finding,
  corrected here and at both step sites). **`frontend/app/view/waveai/` does not exist** — a
  repo-wide glob confirms no such directory; the earlier claim that it "remains" was checked
  against the resolution's own reasoning but not against the live tree, and was wrong. Only
  `aiprompts/waveai-*.md` (Phase 1, prose) and `docs/docs/img/waveai-model-dropdown.png` (gated
  on the Phase 1 docs scope decision) remain.
- Go test files (16 files importing the module path) — checked via targeted grep
  (`*_test.go` glob), included explicitly in Phase 3's sed scope, and now also explicitly
  compiled (not just included in the sed) via `go vet ./...` in Phase 3's verification gate —
  see Phase 3 step 9's verification, item 5.
- Config file schema/key names (`settings.json`, `connections.json`, `profiles.json`,
  `presets.json`, `backgrounds.json`) — checked for wave-branded JSON *keys* via
  `pkg/wconfig/metaconsts.go` and `pkg/wconfig/defaultconfig/*.json`; none found. The
  filenames themselves are already brand-neutral. (The `web:defaulturl` *value* inside
  `settings.json`, as opposed to its keys, does need a decision — see Phase 1 step 2.)
- macOS entitlements — no separate `.entitlements` file exists outside the `NS*UsageDescription`
  strings already covered in `electron-builder.config.cjs` (Phase 1 step 2); no
  `CFBundle*`/Info.plist-style wave strings found elsewhere.

### Plan Is Wrong If

- ~~`pkg/waveai` still exists under a different path~~ **RESOLVED**: confirmed removed
  (`a08c2d74`). No longer a live risk to this plan.
- **The `docs/` Docusaurus site is not actually "mostly unmodified upstream"** as the prior
  inventory agent characterized it. **RESOLVED, and the plan WAS wrong**:
  `git log --oneline -- docs/` shows 249 commits, including this fork's own low-PR-number
  features (`#24` SSH port forwarding, `#23` reconnect improvements, `#10` auto-reconnect
  detection) mixed into Docusaurus content, not a clean upstream mirror. Phase 1 step 5's
  "keep/drop/rewrite" decision now has a materially higher cost on "drop" than the original
  brief implied — dropping `docs/` would delete real fork-authored documentation, not just
  upstream boilerplate. Scope decision is still the repo owner's to make, but "drop entirely"
  should no longer be presented as the low-cost default option.
- **Other feature branches are open against Go code at the time Phase 3 is meant to land.**
  If so, landing the module-path rename forces every one of them to rebase through a
  793-occurrence diff. Check: `git branch -r` / open PR list immediately before Phase 3
  execution, not at planning time (this will have changed by then).
- **The GitHub Actions secrets named in `bump-version.yml` don't exist on this fork's repo at
  all** (plausible, since this looks like a personal fork rather than an org with release
  infrastructure). If so, Phase 5 steps 13-14 are moot — the workflow already fails/no-ops on
  this fork today, and renaming the references changes nothing functional, only readability.
  Check: repo owner needs to look at Settings -> Secrets and variables -> Actions; not
  checkable from inside the repo.

### Post-Migration Validation

- **Baseline metrics:** not applicable in the performance sense — this is a naming migration,
  not a behavioural one. The relevant "baseline" is: does the app launch, does existing user
  data (config + sessions) survive the Phase 2 migration, does `wsh` still work end-to-end
  against a running instance.
- **Success criteria:** app launches under the new name/paths; a pre-existing config **and**
  data directory pair (built by running the pre-rename binary once against a scratch `HOME` per
  the live-system rule, not a synthetic seed — see Phase 2 step 7) is migrated intact and
  readable, verified by launching against the *restored* backup as well as the live migrated
  copy; a second launch against the same `HOME` confirms the migration does not re-fire; `wsh`
  CLI round-trips a basic RPC call **against a disposable test remote — a scratch VM or
  container spun up specifically for this validation, not an already-connected host the repo
  owner relies on**, to confirm the Prerequisites decision on remote-host `.waveterm` state
  didn't silently break remote command execution. **This remote-connection check must be
  performed or explicitly approved by the repo owner, not run unattended by a subagent**: the
  Prerequisites section already treats remote-host `.waveterm` state as touching infrastructure
  outside this repo's control, and running rename-related install/remove commands against a
  real, currently-used SSH target without asking first is exactly the live-system hazard this
  project's own standing rules treat as a hard stop — do not substitute a real personal remote
  for the disposable one specified here. `npx tsc --noEmit`, `go vet ./...`, and
  `gofmt -l $(git ls-files '*.go')` all clean; CI green on `build-macos-ci.yml`.
- **Monitoring plan:** none automated — this is a local desktop app with no telemetry
  (per README's fork notes) and no production service to monitor. Manual smoke test by the
  repo owner after each phase lands, per the project's own "no go build, trust the problems
  pane" convention, extended here to "trust manual launch-and-click, not automated
  monitoring."
- **Hypercare period:** n/a (single-user local desktop app, not a deployed service).
- **Data validation:** after Phase 2, diff the migrated `~/.config/remoteterm` tree (and the
  separately-migrated data-dir root — see step 7) against the pre-migration backup (the
  migration moves rather than copies, so the gated pre-step backup in step 7 — taken and
  restore-verified by the repo owner, not by an agent, given the live-system-data-touching
  CLAUDE.md rule — is what this diffs against, not an ad hoc backup taken after the fact).

### Estimated Effort

- Phase 1 (user-visible strings + docs decision): M, up from S-M after round 2 — mechanical
  string edits, but the file list grew across both audit rounds (`emain/` menu/dialog strings,
  `NOTICE` read, `FUNDING.yml` decision, `copilot-instructions.md`, `.kilocode/rules/rules.md`,
  `settings.json`'s default URL, `RELEASES.md`/`IMAGE-RENDERING-INVESTIGATION.md`/
  `REMOTE-IMAGE-PASTE-SPEC.md`, the Apache-2.0 §4(b) decision, and the WaveApp Builder naming
  decision), so "mechanical string edits across ~30 files" understates it. Still no code-path
  risk, but genuinely more files and more owner-facing decisions than the phase's "S" origin
  suggested. The app icon/logo artwork decision (Prerequisites) is also gated through this
  phase for the window-icon load paths, though the artwork itself is design work outside this
  plan's own scope.
- Phase 2 (env vars + data-dir migration): L, up from M — round-1 audit found this phase
  under-sized: three path roots need migrating (not one), the marker-validation logic as
  originally specified doesn't work on any real install, the gate logic as specified is
  falsified by the app's own directory creation, and there's a concurrency race with the
  single-instance lock. This is genuinely the highest-judgment piece of the whole plan, more so
  than the file count suggests. Failure-likelihood: medium, concentrated in the migration
  validation/gate logic, now addressed in step 7's rewrite.
  Sequencing: **not a hard blocker on Phase 3** (see step 8) — preferred before Phase 3 for
  commit legibility only; the "must land before Phase 3 touches `pkg/wavebase`" framing
  previously stated here contradicted step 8's own text and has been removed.
- Phase 3 (Go module path + package rename): L, and round 2 found the package-rename portion
  specifically harder than the phase's single "L" tag implies — 204 `*.go` files, 793
  module-path occurrences, plus 11 additional non-`.go` files (`tsunami/go.mod` and its 8 demo
  `go.mod` files, 2 `.go.tmpl` templates) that are structurally invisible to a `*.go`-scoped
  sed/grep, plus the WaveApp Builder content-level rename inside `tsunami/engine/` and siblings
  once that naming decision lands. The module-path rewrite itself is a straightforward scripted
  sed; the eleven-package rename is not — it needs a Go-aware rename tool (`gopls rename` /
  `gomvpkg`) or an ordered per-package `git mv` + rewrite recipe (step 9), not a single blanket
  pass, because package-qualified selectors (2,071+ occurrences across just 7 of 11 packages in
  `pkg/` alone) are invisible to an import-path-only sed. One atomic commit, requires `task
  generate` + `go vet` (root and `tsunami/` separately) as real compile gates, requires
  coordination on Go/TS/build-flag string-literal cross-references. Failure-likelihood:
  medium-high if not scripted carefully, gitignore-aware, and tool-assisted for the package
  moves; low if done via a verified, `git ls-files`-scoped sed script plus a Go-aware rename
  tool plus the widened verification gate in step 9. Token-cost class: high (touches every Go
  file in the tree; a subagent executing this should work from a generated file list, not
  re-grep the whole tree per file).
  Sequencing: hard blocker for Phase 4; should land with no other Go branches in flight
  (re-verify at execution time per Prerequisites, not from this document's snapshot).
- Phase 4 (frontend internal module renames): M — candidate file count is under dispute (269
  cited originally, 143 reproduced in `frontend/` alone by round-1 audit; re-run the exact
  command before dispatch rather than trusting either number) but most files need only an
  import-path substitution, not a content rewrite. Scope grew to include `emain/` (16 files,
  ~430 hits), `index.html`'s entry-point reference, and the WaveApp Builder
  directories/log-pattern — all previously unowned. The `@/waveenv`, `@/util/waveutil` style
  path renames are mechanical once Phase 3's generated types have landed.
  Sequencing: hard-blocked on Phase 3 completion (`gotypes.d.ts` must be current).
- Phase 5 (CI/build): S-M — small file count (4-5 files) but two steps (13, 14) are gated on
  information only the repo owner has (do GitHub secrets/S3 buckets exist under new names).
  Failure-likelihood: low for the mechanical parts, but steps 13/14 executed prematurely have
  high impact (break release automation) — treat those two as separate, owner-gated sub-steps.
- Phase 6 (cleanup sweep): S-M, up from S — the sweep's exit criterion (residual grep returns
  only allowlisted hits) was not achievable against the original phase assignments, since
  `emain/` had no owning phase; now that Phase 4 covers `emain/`, the sweep itself is still
  low-complexity verification work, but it now also needs to check the persisted
  `RENAME_ALLOWLIST.md` artefact and classify remote-host residue explicitly (category d in
  step 18) rather than a bare count comparison.
