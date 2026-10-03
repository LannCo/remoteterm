# Builder by hand: design spec (v2)

Date: 2026-10-03. Branch: `feat/builder-by-hand` off `origin/main` 3e126798.
v2 folds in the requirements audit (B1-B2, M1-M5, m1-m9) and security review (F1-F9) of v1; the disposition table is at the end.

## Problem

The Tsunami app builder (Builder window: Preview / Code / Files / Secrets / Config tabs) was designed around a built-in AI chat that wrote `app.go`. This fork removed that AI (`1a7f1c83`). What remains cannot produce a running app:

1. **Builds cannot succeed.** With no `go.mod` in the app, the build requires `github.com/LannCo/remoteterm/tsunami v0.12.4`, which does not exist under that module path. A replace directive is only added when the user sets `tsunami:sdkreplacepath` (`tsunami/build/build.go:388-392`).
2. **Go is not found for GUI launches.** `FindGoExecutable` (`build.go:134`) checks `PATH` plus fixed system directories; desktop launches do not inherit PATH additions from shell rc files. The advertised minimum (Go 1.22, `build.go:31`) is also below the SDK's own `go 1.25.6` (`tsunami/go.mod:3`).
3. **New apps start empty.** "Create new" only sets the app id.
4. **Outside edits are invisible.** The frontend subscribes to `rtapp:appgoupdated`, but nothing publishes it since the AI tooling was deleted.
5. **Dead space.** An "AI features are disabled" placeholder takes 50% of the window width.

## Goal

A user can create a Tsunami app in the builder, write it by hand in the Code tab, or point any agent harness (Claude Code, Hermes, Codex, an editor) at the app folder, and see it build and preview. No AI code is added to the app.

## Success criteria (each has a named observable)

- S1. Fresh install, Go >= the SDK's `go` line installed where a login shell finds it, no settings: Create new app, then the builder status reaches `running` and Preview loads the starter app.
- S2. One Code-tab save produces exactly one build: one `building` status transition, starting from either `running` or `error`.
- S3. With live rebuild on (D4), an external write to a relevant file (root `*.go`, `static/**` except `static/tw.css`) starts a build within 2 s of the last write in a burst. With it off, the builder shows "Changed on disk" with a Rebuild button within 2 s, and no build starts.
- S4. Unsaved Code-tab edits are never replaced without the user choosing to.
- S5. The app folder contains `AGENTS.md`, `CLAUDE.md`, `TSUNAMI_GUIDE.md`; every SDK identifier they reference exists; the last build's output is readable at `<app>/.tsunami/build.log`.
- S6. "Open terminal" creates a running local shell block whose cwd is the app folder, in the last-focused main window.
- S7. Missing Go, a Go older than the SDK requires, a missing SDK bundle, and a missing scaffold each produce a distinct message naming the fix, shown in the Preview error view and Build panel.
- S8. A packaged build contains `resources/tsunamisdk/go.mod` and builds the starter app.

## Non-goals

- Any built-in AI, model calls, or chat UI.
- Editing files other than `app.go` in the Code tab.
- Publishing a LannCo `tsunami/vX` module tag (release process; to be filed as an issue).
- Fixing `tsunami/demo/*/go.mod` replace paths.
- Changing the user's local launch or rebuild scripts.
- Detecting hostile pre-existing agent files (`.claude/`, foreign `AGENTS.md`) in imported apps.

## Design

### D1. Bundled SDK

- Single source of truth for the bundle contents: `build.CopySdkBundle(srcDir, dstDir string) error` in `tsunami/build`. Copies `go.mod`, `go.sum`, and non-test `*.go` files from the SDK runtime packages (`app/ engine/ vdom/ rpctypes/ util/ tsunamibase/ ui/`, recursively). Skips `build/`, `cmd/`, `demo/`, `frontend/`, `templates/`, `*_test.go`, symlinks.
- Exposed as a subcommand of the existing tsunami CLI (`tsunami/cmd`): `sdkbundle <dst>`. Taskfile task `build:tsunamisdk` runs it into `dist/tsunamisdk`; wired everywhere `build:tsunamiscaffold` is (dev and package tasks).
- `electron-builder.config.cjs`: exclude from `files`, add to `extraResources` as `tsunamisdk`, mirroring `tsunamiscaffold`.
- `remotetermapputil.GetTsunamiSdkPath()` returns `<app resources>/tsunamisdk`.
- Effective replace path, resolved per build: `tsunami:sdkreplacepath` if set; else the bundle if `<bundle>/go.mod` exists; else fail before tidy with the D6 SDK message.
- Go floor: the minimum Go version is read from the effective SDK's `go.mod` `go` directive (full version, e.g. 1.25.6), replacing the hard-coded minor 22 for builder builds. `GOTOOLCHAIN=local` is set for every builder `go` invocation so an older toolchain fails with our message instead of downloading another.
- AppImage note: the resources path can change per launch; the replace is rewritten every build, so this is harmless. Documented in `AGENTS.md` ("do not edit `go.mod`").

### D2. Go discovery

New resolution in `FindGoExecutable`, first hit wins:

1. `tsunami:gopath` setting (existing; honoured by the controller).
2. `exec.LookPath("go")`.
3. Existing fixed directories.
4. Per-user locations: `~/.local/go/bin/go`, `/usr/lib/go/bin/go`, newest `~/sdk/go*/bin/go`, `/snap/bin/go`, `~/.local/share/mise/shims/go`, `~/.asdf/shims/go`.
5. Login-shell probe, Linux and macOS only:
   - Shell = basename of `$SHELL`; allowlist `bash`, `zsh`, `fish` run with `-l -i -c`; `sh`, `dash`, `ksh` with `-l -c`; anything else skipped.
   - Script: `command -v go`. Stdin `/dev/null`; own process group (`Setpgid`); 3 s timeout kills the group; `cmd.WaitDelay` 1 s; stdout capped at 64 KiB.
   - Accept only the last non-empty line, an absolute path to an existing regular executable file.
   - Singleflight; success cached for the process lifetime; failure cached 30 s.

Whatever is found, the result is canonicalised: run `<found> env GOROOT` (2 s timeout, `GOTOOLCHAIN=local`) and use `$GOROOT/bin/go` and `$GOROOT/bin/gofmt`. This resolves shims and `/snap/bin` wrappers to the real toolchain.

`ResolveGoFmtPath` (called on every Code-tab save) uses only the cached result and never triggers a probe.

### D3. Starter files

- Embedded with `//go:embed` in new package `pkg/remotetermappstore/starter`:
  - `app.go`: about 40 lines, `package main`, `AppMeta` with title and short description, an `App` component with a counter button and a text input bound to an atom. No `init()`.
  - `AGENTS.md` (under 120 lines): build model (root `*.go` + `static/`; scaffold supplies `main`; no `init()`, use `AppInit() error`), files the build writes back that must not be edited (`go.mod`, `go.sum`, `manifest.json`, `static/tw.css`, `bin/`), the last build log at `.tsunami/build.log`, required `AppMeta` and `App`, Tailwind usage, pointer to `TSUNAMI_GUIDE.md`. No shell commands, no `go get`, no network instructions.
  - `CLAUDE.md`: the single line `@AGENTS.md`.
  - `TSUNAMI_GUIDE.md`: the Tsunami Framework Guide from `e9bc34a0^:pkg/aiusechat/tsunami/system.md`, imports rewritten to the LannCo path, chat-assistant framing removed, references to symbols absent from the current SDK corrected or cut. Companion docs (`graphing.md`, `global-keyboard-handling.md`) appended only if their symbols exist.
- `SeedApp(appId) ([]string, error)` in `remotetermappstore`, exposed as RPC `SeedBuilderAppCommand{appid}`:
  - `ValidateAppId`, then the safe-path checks in D7, create the app dir (0755) if missing.
  - Each file written with `os.OpenFile(O_CREATE|O_EXCL|O_WRONLY, 0644)` so an existing file or a dangling symlink is never written through.
  - Returns the files written.
- Callers: `handleCreateNew` (`app-selection-modal.tsx`) seeds before opening the app; the Preview empty state gains a "Create starter app" button calling the same RPC.

### D4. Live rebuild

**Backend watcher** (`pkg/buildercontroller`, new file):

- RPC `WatchBuilderAppCommand{builderid}`. The app id is read from the builder's rtInfo (`BuilderAppId`), never from the request. The builder id must have rtInfo. One watcher per builder; starting a new one closes the old one under the controller lock; a global cap of 8 watchers. Stopped in the existing builder teardown path.
- fsnotify on the app root and every directory under `static/`, walked with `filepath.WalkDir` without following symlinks, skipping `node_modules`, `.git`, dot-directories, capped at 1000 directories. A new directory under `static/` is added and then scanned, and its files count as changes.
- Relevant: root `*.go`; files under `static/` except `static/tw.css`. Ignored: `go.mod`, `go.sum`, `manifest.json`, `bin/`, dotfiles and dot-directories (including `.tsunami/`), names ending `~`, `.swp`, `.swx`, `.tmp`, all-digit names, Chmod-only events.
- Debounce 300 ms trailing.
- Root removed or renamed: poll for the directory every 1 s, re-add and rescan when it returns; after 10 s publish the "unavailable" state.
- Watcher state is published to the frontend as `rtapp:watchstatus` scoped to the builder id: `active` or `unavailable` with a reason.

**Input snapshot**: the controller computes a hash over the relevant files (path + content) at the start of every build and keeps it as `lastBuildInputHash`.

**On a debounced change**:

1. Compute the current input hash. If it equals `lastBuildInputHash`, stop (this is the echo of a save that already triggered a build).
2. Publish `rtapp:appgoupdated` scoped to the app id.
3. If setting `builder:liverebuild` is true, call `RequestRebuild()`. If false, the frontend shows "Changed on disk" with a Rebuild button that calls `RequestRebuild` via RPC.

**`RequestRebuild()`** (controller): non-blocking. If a build is running, set `rebuildPending`; when that build ends, exactly one follow-up build starts. Otherwise start a build in a goroutine. The previous app process is stopped before the new one runs. Exposed as RPC `RequestBuilderRebuildCommand{builderid}` returning immediately, so no RPC waits on a build and the 5 s RPC timeout is irrelevant.

**`builder:liverebuild` setting**: bool, default `false`. Off means external writes never execute code without a click, so an agent sandboxed to the app folder cannot run code outside its sandbox by writing a file. The builder's app panel header has a toggle bound to the setting so turning it on is one click, labelled "Rebuild on external changes".

**Frontend**:

- `saveAppFile` keeps formatting and writing, then calls `RequestBuilderRebuildCommand` instead of `debouncedRestart`. It records the content it wrote as `lastWrittenContent`.
- Handler for `rtapp:appgoupdated` reads `app.go` (it does not call `loadAppFile`, which starts builds) and applies the pure function `decideReload(editor, original, disk, lastWritten)`:
  - disk missing: set `appGoMissingAtom`; no atom changes; Preview shows "app.go is missing" with "Create starter app".
  - disk == editor: set original = disk (covers our own save and identical external writes).
  - editor == original (clean): set both to disk.
  - otherwise (dirty, disk differs): set `diskChangedAtom` = disk; Code tab shows "app.go changed on disk." with "Load disk version" and "Keep my edits".
- `loadAppFile` keeps its start-on-open behaviour for opening an app; the initial start goes through `RequestBuilderRebuildCommand` too.

**Build log**: every build writes its output and final status line to `<app>/.tsunami/build.log` (truncate per build, dot-directory so the watcher ignores it).

### D5. Terminal and folder actions

- Remove the placeholder panel and its `chat` layout key from `builder-workspace.tsx`.
- App panel header: app folder path (left-truncated, tooltip shows full path, copy button), "Open terminal", "Open folder", and the D4 live-rebuild toggle.
- "Open terminal": builder renderer calls a new no-argument IPC `open-builder-terminal`. The main process identifies the calling builder window, picks the last-focused main RemoteTerm window excluding the quake window (falling back to any non-quake main window); if none exists, it returns an error the builder shows as a notice. It calls backend RPC `OpenBuilderTerminalCommand{builderid, tabid}` with that window's active tab, then focuses the window.
- Backend `OpenBuilderTerminalCommand`: derives the app dir from the builder's rtInfo, applies D7 checks to the dir, and creates the block with `CreateBlockCommand` semantics: `meta: {view: "term", controller: "shell", connection: "local", "cmd:cwd": <dir>}`. No path ever comes from the renderer.
- "Open folder": new no-argument IPC `open-builder-folder`, resolved the same way server-side, then `shell.openPath` on a directory only (reject if not a directory).

### D6. Error messages

Raised in `verifyEnvironment` / the controller pre-build check (not inside `FindGoExecutable`, whose message `CheckGoVersion` swallows):

- Go not found: `Go toolchain not found. Install Go <min> or newer, or set "tsunami:gopath" in Settings to the full path of the go binary.`
- Go too old: `Go <found> is older than <min>, which the Tsunami SDK requires. Install a newer Go, or set "tsunami:gopath" to one.`
- SDK missing: `Tsunami SDK not found at <path>. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`
- Scaffold missing: `Tsunami scaffold not found at <path>. Rebuild with "task build:tsunamiscaffold", or set "tsunami:scaffoldpath" in Settings.`

`<min>` comes from D1. Messages surface through the builder status `errormsg` and are written to `.tsunami/build.log`.

### D7. App folder safety

Applied to `SeedApp`, the existing `ReadAppFile`/`WriteAppFile`, the watcher, and the terminal/folder actions:

- Every path component from `~/waveapps` down to the target is `Lstat`ed; any symlink is rejected.
- Reads require a regular file and cap at 2 MiB.
- Writes of new files use `O_EXCL`; overwrites of existing files require the existing target to be a regular file (checked with `Lstat` immediately before writing).

### D8. Secret bindings out of the app folder

`secret-bindings.json` moves from the app dir to `<data dir>/builder/secret-bindings/<ns>/<name>.json`, so a process that can write the app folder cannot bind the user's stored secrets into agent-written code. Read/write/publish paths in `waveappstore.go` change accordingly; publish copies the bindings file to the published id's location. A legacy `secret-bindings.json` in an app dir is ignored (the project is a pre-release POC; no migration).

### D9. One builder window per app

When an app is selected in a builder window and another builder window already has that app, the other window is focused and the selecting window is closed. This keeps one controller, one watcher, and one build per app folder.

## Testing

Go unit tests (new):

- `CopySdkBundle`: copies the expected set, skips excluded dirs, tests, and symlinks.
- Starter compile: `CopySdkBundle` into a temp dir, then a scratch module (scaffold `app-main.go.tmpl` + starter `app.go` + replace to the temp bundle) builds with `GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`. Skips with a stated reason if `go` is missing or older than the SDK's `go` line or the module cache is cold.
- Guide symbol check: every `app.X`, `vdom.X`, `ui.X` in `TSUNAMI_GUIDE.md` and `AGENTS.md` is an exported identifier in the SDK package (parsed with `go/parser`).
- Go floor: minimum read from the bundle `go.mod`; version comparison table (1.25.5 < 1.25.6, 1.26.0 >= 1.25.6, `go1.25rc1`).
- Go discovery: setting, PATH, extra dirs with fake executables in a temp HOME; probe with fake `$SHELL` scripts (noise then path; relative path; hang past timeout with a background child holding stdout; unknown shell skipped); failure cache honoured; GOROOT canonicalisation with a fake `go` printing a GOROOT.
- Seed: writes all files into empty dir; never overwrites; refuses a dangling symlink at `app.go`; refuses a symlinked app dir; rejects invalid ids.
- D7: read cap, non-regular file rejection, symlinked parent rejection.
- Watcher: relevant/ignored table; burst debounced to one event; echo suppression via input hash; new `static/` subdir scanned; root removal then recreation resumes; cap of 8; stop releases fds.
- `RequestRebuild`: request while building yields exactly one follow-up build; previous process stopped.
- Secret bindings: stored under the data dir; app-dir file ignored; publish copies bindings.

Frontend vitest:

- `decideReload` table, including disk missing and disk == editor.

Packaging check:

- After `task build:tsunamisdk`, `dist/tsunamisdk/go.mod` exists; electron-builder config lists it in `extraResources` (static test on the config).

End-to-end, isolated (nested Xvfb, scratch HOME and XDG dirs, `REMOTETERM_CONFIG_HOME`, `REMOTETERM_DATA_HOME`, `REMOTETERM_ISOLATED_PROFILE=1`; never the live display or the user's data dirs): create app; status reaches `running`; save in Code tab gives one `building` transition; external write with live rebuild off shows "Changed on disk", with it on rebuilds; Open terminal creates a shell block with the app dir as cwd.

Existing suites stay green: `go test ./pkg/...`, `cd tsunami && go test ./...`, `npx vitest run`, `npx tsc --noEmit`.

## Risks

- R1. Shell probe runs user rc files. Mitigated by D2 bounds; used last.
- R2. First build needs the Go module cache for SDK dependencies; offline first builds fail with the `go mod tidy` error in the Build panel and build log.
- R3. The recovered guide may describe changed APIs. The symbol test catches identifiers; prose that cannot be verified is cut.
- R4. inotify limits. Mitigated by caps and the "Live reload unavailable" state; saves still rebuild.
- R5. `SetRTInfoCommand` lets a renderer set builder env vars passed to the app process. Pre-existing; out of scope.

## Audit disposition (v1 findings)

| Finding | Disposition |
|---|---|
| B1 Go floor | Accepted: D1 min from SDK `go` line, `GOTOOLCHAIN=local`, D6 too-old message |
| B2 no shell controller | Accepted: D5 `controller: "shell"` |
| M1 double build via loadAppFile | Accepted: D4 handler does not call `loadAppFile`; builds via `RequestRebuild` |
| M2 mid-build change dropped | Accepted: D4 backend coalescing, non-blocking RPC |
| M3 agents cannot see errors | Accepted: D4 `.tsunami/build.log` |
| M4 watcher lifetime | Accepted: D4 root poll, subdir scan |
| M5 / F5 probe hang, repeat, shims | Accepted: D2 bounds, caches, GOROOT canonicalisation |
| m1 error wiring | Accepted: D6 placement, scaffold setting named |
| m2 two windows one app | Accepted: D9 |
| m3 app.go deleted | Accepted: D4 `decideReload` missing case |
| m4 process leak | Accepted: `RequestRebuild` stops previous process |
| m5 / F4 app id source, watcher cap | Accepted: D4 rtInfo-derived, cap 8 |
| m6 false changed-on-disk | Accepted: `disk == editor` case |
| m7 testability | Accepted: observables in S1-S8, packaging check |
| m8 duplicate bundle rule | Accepted: `CopySdkBundle` single source, compile test uses it |
| m9 init rule, missing symbol | Accepted: AGENTS.md rule, symbol test |
| F1 auto-run is sandbox escape | Accepted: `builder:liverebuild` default off, one-click toggle |
| F2 secret bindings writable | Accepted: D8 |
| F3 symlinks, special files | Accepted: D7, O_EXCL seeding, watcher no-follow |
| F6 terminal path from renderer | Accepted: D5 no-argument IPC, server-side resolution, `connection: "local"` |
| F7 agent files | Partly: starter text constraints accepted; foreign-file notice is a non-goal |
| F8 toolchain download | Accepted: `GOTOOLCHAIN=local`; surfacing new `require` lines rejected (YAGNI) |
| F9 rebuild spam | Accepted via coalescing in `RequestRebuild` |
