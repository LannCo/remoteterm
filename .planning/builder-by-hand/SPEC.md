# Builder by hand: design spec

Date: 2026-10-03. Branch: `feat/builder-by-hand` off `origin/main` 3e126798.

## Problem

The Tsunami app builder (Builder window: Preview / Code / Files / Secrets / Config tabs) was designed around a built-in AI chat that wrote `app.go`. This fork removed that AI (`1a7f1c83`). What remains cannot produce a running app:

1. **Builds cannot succeed.** With no `go.mod` in the app, the build requires `github.com/LannCo/remoteterm/tsunami v0.12.4`, which does not exist under that module path anywhere. A replace directive is only added when the user sets `tsunami:sdkreplacepath`.
2. **Go is not found for GUI launches.** `FindGoExecutable` (`tsunami/build/build.go:134`) checks `PATH` plus a few system directories. Desktop launches (hotkey, menu) do not inherit PATH additions made in shell rc files, so per-user Go installs are invisible.
3. **New apps start empty.** "Create new" only sets the app id; no file exists, the Preview tab says "No App to Preview", Config says "App Not Running".
4. **Outside edits are invisible.** The frontend still subscribes to `rtapp:appgoupdated`, but its only publisher was the deleted AI tooling. An editor or agent writing files in the app folder causes no reload and no rebuild.
5. **Dead space.** An "AI features are disabled" placeholder takes 50% of the window width (minimum 20%).

## Goal

A user can create a Tsunami app in the builder, write it by hand in the Code tab, or point any agent harness (Claude Code, Hermes, Codex, an editor) at the app folder, and see it build and preview live. No AI code is added to the app.

## Success criteria

- S1. Fresh install, Go installed somewhere a login shell can find it, no settings touched: Create new app, the starter app builds and appears in Preview.
- S2. Editing `app.go` in the Code tab and saving rebuilds once (not twice).
- S3. An external process writing `app.go` (or another root `*.go` file, or a file under `static/`) causes the Code tab to show the new content and the app to rebuild, within about 2 s.
- S4. If the Code tab has unsaved edits when the file changes on disk, the edits are not silently lost.
- S5. The app folder contains instructions an agent harness picks up (`AGENTS.md`, `CLAUDE.md`) plus a Tsunami API guide whose referenced SDK symbols exist.
- S6. One action in the builder opens a terminal in the app folder.
- S7. When Go or the SDK cannot be found, the error says what is missing and how to fix it.
- S8. Packaged releases work, not just source checkouts.

## Non-goals

- Any built-in AI, model calls, or chat UI.
- Editing files other than `app.go` in the Code tab (agents may still write extra `*.go` files; they compile but are not shown).
- Publishing a `tsunami/vX` Go module tag under the LannCo path (release process; tracked separately).
- Fixing the `tsunami/demo/*/go.mod` replace paths (demos are not used as seeds).
- Changing the user's local launch or rebuild scripts.

## Design

### D1. Bundled SDK (fixes problem 1, S1, S8)

- New Taskfile task `build:tsunamisdk` copies the SDK module to `dist/tsunamisdk`: `go.mod`, `go.sum`, and non-test `*.go` files from `app/ engine/ vdom/ rpctypes/ util/ tsunamibase/ ui/`. Excludes `build/`, `cmd/`, `demo/`, `frontend/`, `templates/`, `*_test.go`. Wired everywhere `build:tsunamiscaffold` is (dev and package tasks).
- `electron-builder.config.cjs`: exclude `dist/tsunamisdk` from `files`, add it to `extraResources` as `tsunamisdk`, mirroring `tsunamiscaffold`.
- New `remotetermapputil.GetTsunamiSdkPath()`: returns `<app resources>/tsunamisdk`.
- Build controller: effective replace path = `tsunami:sdkreplacepath` if set, else the bundled path if `<bundled>/go.mod` exists, else empty (current behaviour, which fails at tidy).
- Existing app `go.mod` files: the replace is (re)applied on every build because `AddReplace` runs whenever the path is non-empty, so old apps pick it up.
- Module sync: a Go test asserts the bundled file list rule covers every non-test package the scaffold template and SDK import (so a new SDK package is not silently left out). Concretely: `go list -deps` from a scratch app using the bundle must succeed offline with a warm module cache; test skips with a reason if `go` is unavailable.

### D2. Go discovery (problem 2, S1, S7)

Order in `FindGoExecutable`, first hit wins:

1. `tsunami:gopath` setting (already honoured by the controller; unchanged).
2. `exec.LookPath("go")`.
3. Existing fixed directories (unchanged).
4. Additional per-user locations: `~/go/bin/go` is NOT included (that is GOBIN, not a toolchain). Included: `~/.local/go/bin/go`, `/usr/lib/go/bin/go`, `/snap/bin/go`, newest `~/sdk/go*/bin/go`, `~/.local/share/mise/shims/go`, `~/.asdf/shims/go`.
5. Login-shell probe (Linux and macOS only): run the user's `$SHELL` (fallback `/bin/sh`) with `-l -i -c 'command -v go'`, stdin closed, 3 s timeout, environment as inherited. Take the last non-empty stdout line; accept only an absolute path to an existing executable regular file. Cache a success for the life of the process; do not cache a failure (the user may install Go and retry).

On failure the error reads: `Go toolchain not found. Install Go 1.22 or newer, or set "tsunami:gopath" in Settings to the full path of the go binary.`

### D3. Starter files (problem 3, S1, S5)

New backend operation `SeedApp(appId)`, exposed as RPC `SeedBuilderAppCommand{appid}`:

- Validates the app id (existing `ValidateAppId`), creates the app dir if missing.
- Writes each starter file only if it does not exist. Never overwrites. Returns the list of files written.
- Starter files (embedded with `//go:embed` in a new package `pkg/remotetermappstore/starter`):
  - `app.go`: a small working app (title, short description, a counter button and a text input using an atom), about 40 lines, `package main`, imports `github.com/LannCo/remoteterm/tsunami/app` and `.../vdom`.
  - `AGENTS.md`: under 120 lines. How the app is built and run (root `*.go` + `static/`, scaffold supplies `main`, builder rebuilds on save), files the build writes back (`go.mod`, `go.sum`, `manifest.json`, `static/tw.css`, `bin/`) and must not be hand-edited, the required `AppMeta` and `App` component, Tailwind classes available, how to see build errors (Build panel in the builder), and a pointer to `TSUNAMI_GUIDE.md` for the API.
  - `CLAUDE.md`: the single line `@AGENTS.md` (Claude Code import syntax).
  - `TSUNAMI_GUIDE.md`: the Tsunami Framework Guide recovered from `e9bc34a0^:pkg/aiusechat/tsunami/system.md`, with imports rewritten to `github.com/LannCo/remoteterm/tsunami`, wording that addresses a chat assistant removed, and stale API references corrected against the current SDK. The two companion docs (`graphing.md`, `global-keyboard-handling.md`) are appended as sections only if their referenced symbols exist.
- Callers:
  - `handleCreateNew` in `app-selection-modal.tsx` calls `SeedBuilderAppCommand` after choosing the id, before opening the app.
  - The Preview tab empty state ("No App to Preview") gains a "Create starter app" button that calls the same RPC. This covers existing empty drafts.

### D4. File watcher (problem 4, S2, S3, S4)

Backend, one watcher per builder window while it has an app selected:

- Started by RPC `WatchBuilderAppCommand{builderid, appid}` from the builder frontend when the app id is set; replaces any previous watch for that builder; stopped when the builder window closes (existing builder teardown path) or a new app is selected.
- fsnotify on the app root and `static/` (added when it appears, recursively for subdirectories of `static/`). Creates the app dir if missing so an agent can be pointed at it.
- Relevant changes: root `*.go`; anything under `static/` except `static/tw.css`. Ignored: `go.mod`, `go.sum`, `manifest.json`, `bin/`, dotfiles, editor temp files (names ending `~`, `.swp`, `.swx`, `.tmp`, or purely numeric names such as vim's `4913`), Chmod-only events.
- Debounce: 300 ms trailing per watcher, so one burst of writes yields one event.
- On fire: publish `rtapp:appgoupdated` scoped to the app id (existing constant, existing subscriber).
- Frontend handler (`builder-apppanel-model.ts`):
  - Reads `app.go` from disk.
  - If the editor is clean (`code === original`): replace both atoms with disk content.
  - If dirty and disk content differs from `original`: leave both atoms alone and set a `diskChangedAtom` holding the disk content. The Code tab shows a bar: "app.go changed on disk." with "Load disk version" (discard edits, adopt disk content as both atoms) and "Keep my edits" (dismiss; the next save overwrites disk).
  - If disk content equals `original`: no-op apart from the restart below.
  - In every case calls the existing `debouncedRestart` (800 ms trailing). A Code-tab save already calls it; the watcher event for the same save lands inside the window, so S2 holds with one build.
- If the watcher cannot start (fsnotify error, inotify limit), log it, return the error to the frontend, and the builder shows a non-blocking notice "Live reload unavailable: <reason>"; saves still rebuild as today.

### D5. Terminal and folder actions (S6)

- Remove the placeholder panel and its `chat` layout key from `builder-workspace.tsx`; the right-hand content takes the full width. Saved layouts containing a `chat` value are ignored.
- The app panel header gains:
  - The app folder path (truncated from the left, full path in a tooltip) with a copy button.
  - "Open terminal": asks the main process to open a new terminal block with `cmd:cwd` set to the app folder in the active tab of the most recently focused main window, then focuses that window. If no main window exists, a new one is created first.
  - "Open folder": existing `open-native-path` IPC.
- The main-process side reuses existing block-creation RPC (`CreateBlockCommand` with `BlockDef{meta:{view:"term","cmd:cwd":<dir>}}`) against that window's active tab id.

### D6. Error messages (S7)

- Go not found: D2 text.
- SDK bundle missing and no setting: `Tsunami SDK not found at <path>. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`
- Scaffold missing: `Tsunami scaffold not found at <path>. Rebuild with "task build:tsunamiscaffold".` (checked before invoking the build).
- These surface through the existing builder status `errormsg` path (Preview error view and Build panel).

## Testing

- Go unit tests (new, none exist today for these packages):
  - Watcher: relevant/ignored path filter (table test); debounce collapses a burst to one publish; `static/` subdirectory added after start is watched; stop releases fds.
  - Seed: writes all files into an empty dir; never overwrites an existing file; rejects invalid ids; returns written list.
  - Go discovery: setting, PATH, extra dirs (fake executables in a temp HOME), shell probe (fake `$SHELL` script printing noise then a path; one that hangs past timeout; one that prints a relative path).
  - SDK path resolution: setting beats bundle; bundle used when `go.mod` present; empty otherwise.
  - Guide symbol check: every `app.X`, `vdom.X`, `ui.X` identifier referenced in `TSUNAMI_GUIDE.md` and `AGENTS.md` is an exported name in the current SDK package (parsed with `go/parser`).
  - Starter compile check: scratch module with the scaffold `main` + starter `app.go` + replace to `tsunami/` builds with `GOFLAGS=-mod=mod GOPROXY=off`; skips with reason if `go` missing or the module cache is cold.
- Frontend vitest: the reload decision (clean replace / dirty keep + flag / identical no-op) extracted as a pure function and table-tested.
- End-to-end, isolated: nested Xvfb, scratch HOME and XDG dirs, `REMOTETERM_CONFIG_HOME`/`REMOTETERM_DATA_HOME`/`REMOTETERM_ISOLATED_PROFILE=1`, never the live display or the user's data dirs. Create app, seed, wait for running status, preview loads; write `app.go` from a shell; confirm rebuild and reload; Open terminal creates a term block with the right cwd.
- Existing suites stay green: `go test ./pkg/...`, `cd tsunami && go test ./...`, `npx vitest run`, `npx tsc --noEmit`.

## Risks

- R1. Shell probe runs user rc files. Mitigated by timeout, closed stdin, last-line parsing, absolute-path and executable checks; only used after every cheaper option fails.
- R2. First build needs the Go module cache (SDK deps). Offline first builds fail; error from `go mod tidy` surfaces in the Build panel. Accepted.
- R3. The recovered guide may describe APIs that changed. Mitigated by the symbol check test; anything that cannot be verified is cut, not guessed.
- R4. inotify watch limits on large `static/` trees. Mitigated by the D4 failure notice and unchanged save-triggered rebuilds.
