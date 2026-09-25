# R2 refute: reachability and trigger

Commit `75b60911`. Lens: can a concrete user action or process state reach the cited line? Read-only; the only runtime evidence is a read-only `ls` of `~/.config/*remoteterm*` / `~/.local/share/remoteterm-dev` and of other worktrees' `dist/` (not `remoteterm-daily`).

## R2-CA-1
VERDICT: HOLDS

Trigger: a legacy instance (upstream Wave Terminal, or any pre-rename fork build) is running on `~/.config/waveterm[-dev]` + `~/.local/share/waveterm[-dev]`. The user starts the new build.

- `emain/emain.ts` imports `./emain-platform`. `emain/emain-platform.ts:275` `performDataDirMigration()` runs as a top-level statement when the module is evaluated.
- `emain-platform.ts:225-253` → `migrateDataRoot`. The only gates are the marker check (`:134`), source existence (`:137`), `sourceSkipReason` (`:141`, which checks only that `wave.lock` *exists*, `:192-193`, and does not probe the flock) and dest emptiness (`:146`). Then `renameSync` at `:170`.
- The single-instance lock is only requested later, at `emain/emain.ts:264`, and the rename has already happened by then. That lock would not protect this case anyway: the legacy app's Electron userData (`~/.config/Wave*`) differs from the new one (`~/.config/RemoteTerm (Dev)`, observed on disk), so the two apps never contend.
- On Linux and macOS, `rename(2)` succeeds while the old process holds open fds and a flock on `wave.lock`. The flock follows the inode into the new directory, so `pkg/remotetermbase/wavebase.go:188-200` `AcquireWaveLock` then fails with "legacy lock ... is held" and the new server refuses to start. By then the old instance's roots have been moved.
- On Windows the rename of a dir with open handles fails. It is recorded as a failure and nothing moves, so the bug does not trigger there.

Severity: Medium. The old instance keeps running on the moved inodes; sqlite is WAL (`pkg/rtstore/wstore_dbsetup.go:51`), so the db, WAL and shm files stay open by fd. Its path-based writes to the old location fail: config uses AtomicWriteFile into a missing directory. The new app cannot start its backend until the old instance exits. I found no data destruction, only a broken concurrent run.

## R2-CA-2
VERDICT: REFUTED

A secrets-only legacy config root cannot be produced by a legacy server. The legacy server startup (`main:cmd/server/main-server.go:218,225`, and at HEAD `cmd/server/main-server.go:226,233`) unconditionally runs `EnsureWaveConfigDir` and then `EnsureWavePresetsDir`, which creates `<configdir>/presets/`. `secrets.enc` is written only by that same server at runtime (`pkg/secretstore/secretstore.go:202-205`), so any root that contains `secrets.enc` also contains `presets/`. `legacyConfigSkipReason` (`emain-platform.ts:206-209`) accepts `presets/`, so the root is migrated. The only way to trigger the claim is for the user to delete `presets/` by hand. An override root skips the check entirely (`:122-133`).

## R2-CA-3
VERDICT: HOLDS (Low)

Trigger: click any background tile in Settings → Backgrounds (`backgroundscontent.tsx:201,208`). The `onClick` handler discards the promise returned by `model.applyBackgroundToTab` (`remotetermconfig-model.ts:946-952`), which awaits `SetMetaCommand` with no try/catch. The server can reject only via `rtstore.UpdateObjectMeta` (`pkg/wshrpc/wshserver/wshserver.go:171-179`), which fails on a DB error or a missing tab. The RPC layer can also reject if the backend link drops. I found no global `unhandledrejection` handler in `frontend/` or `emain/`. The trigger is environmental only (backend or DB failure); normal use never rejects. Sibling writers set `errorMessageAtom`, so this is an inconsistency, not a common-path bug.

## R2-CA-4
VERDICT: HOLDS (Medium, pre-existing)

Trigger: in Source Control, select a staged file and click the gutter glyph, whose hover text is "Revert hunk N". The chain:
- `DiffGutter.tsx:41-43,60-61` chooses `onRevertHunk` when `isStaged`.
- `sourcecontrol.tsx:903` (or `file-diff-section.tsx:282`) calls `model.revertHunk(path, i, staged=true)`.
- `sourcecontrol-model.ts:392` calls `GitRevertHunkCommand{staged:true}`.
- `git.go:326-327` takes the hunk from `git diff --cached`.
- `git.go:342` → `applyPatchReverse` (`git.go:1126-1127`) runs `git apply -R` with no `--cached`/`--index`, so only the worktree changes.

The index keeps the staged change, and the file now shows as staged plus a reverse unstaged edit. If the worktree has diverged from the index in that region, the apply fails and the error goes only to `console.error` (`sourcecontrol-model.ts:396`). `revertFileFromReview(path, true)` (`:627-633`) repeats the same result per hunk. The content is still recoverable from the index.

## R2-CA-5
VERDICT: HOLDS (Low)

Trigger: in Settings → Widgets, click "move up" on one row and then "move up" on the row it just passed, before the config watcher round-trip refreshes `widgetsMapAtom`. For example, with A0 B1 C2 D3:
1. Move D up. `widgetscontent.tsx:248-263` gives `[A,B,D,C]`, and `reorderWidget` (`remotetermconfig-model.ts:826-851`) computes D=(1+2)/2=1.5.
2. Move C up. This gives `[A,B,C,D]`; the next neighbour D is read from the stale `widgetsMap` as 3, so C=(1+3)/2=2.

The persisted order is A0 B1 D1.5 C2, so the second move is lost, and the list snaps back when `localKeys` resyncs. `newOrder` is computed at enqueue time from the snapshot. Only the widget object is re-read inside the queue (`:847-850`); the neighbour orders are not. The window is one fsnotify/config-event round-trip, which is reachable with fast clicks or keyboard Tab+Enter on adjacent buttons. A single drag followed by another drag is slower and rarely hits it.

## R2-CA-6
VERDICT: HOLDS (Low)

Trigger: in Settings → General, type into a number field (for example 10 → 50), then click the ▲ spin button without pressing Enter or Tab. Mousedown on the `<button>` (`generalcontent.tsx:1115-1119`) blurs the input, so `commit` (`:1075-1083`) runs and calls `onChange(50)`. The click then runs `bump(step)` (`:1085-1088`). That uses the `value` prop, which is still 10 because the config has not round-tripped, so it calls `onChange(11)` after `onChange(50)`. The final value is 11, the typed value is overwritten, and the user expected 51. Keyboard (Tab to the button, then Enter) is slower and usually misses the window.

## R2-CA-9
VERDICT: HOLDS (Medium, tiny population)

On-disk state after running any build with the original shim (`0bc74b18` onward, which includes `35cd6146`; branches `daily-driver/combined-2026-09-21`, `feat/rename-phase1-*`, `qa/*`; none of them in `main`):
- **Case A.** The legacy config root had `settings.json`. It was migrated and the marker written, and the new code returns at `:134`. Unaffected. This is the user's own machine: `~/.config/remoteterm-dev/.migrated-from-waveterm` says `moved-from:~/.config/waveterm-dev`.
- **Case B.** The legacy config root had no `settings.json`, so the old `validateSource` skipped it silently. That build's server then ran `EnsureWavePresetsDir` (`cmd/server/main-server.go:233`), so the new-path config dir now contains `presets/`, and there is no marker.

New code on Case B:
1. The marker is absent (`:134`).
2. The source exists (`:137`).
3. `legacyConfigSkipReason` now passes, because the legacy root has `presets/` and any `*.json` (`:206-209`).
4. The dest exists and is not empty (it has `presets/`), so the `:161-165` abort is recorded with no marker written.
5. `emain.ts:283-290` shows the error dialog.

This repeats on every launch until the user moves `~/.config/waveterm[-dev]` away. A merge that copies rather than moves leaves the source in place and does not stop it. Electron userData is not inside the config dest (it is at `~/.config/RemoteTerm (Dev)`), so `presets/` alone is what makes the dest non-empty, and it always exists. The trigger is deterministic for Case B users. Severity is Medium per affected user; the only users who ran a shim build are fork dev and QA users.

## R2-A11Y-1
VERDICT: HOLDS

The text is reachable by user input:
- `secretscontent.tsx:155`: type a name that fails `SecretNameRegex` (`:128`), such as `1abc`, in Add Secret.
- `connectionscontent.tsx:111`: a quick-add connection error (`connectionsQuickAddErrorAtom`).
- `backgroundscontent.tsx:114`: an add-background error inside the `bg-panel` form (`:76`).

The tokens are `--color-error` rgb(229,77,46) and `--color-background` rgb(34,34,34) (`frontend/tailwindsetup.css:9,26`). That gives ≈4.12:1, and `bg-panel` rgba(31,33,31,.5) over the same base lands at about the same value. All three are 12px text. A user-set tab background image can move the ratio either way.

## R2-A11Y-2
VERDICT: REFUTED (residual Low)

Save, delete, add and load failures set the shared `model.errorMessageAtom` (`remotetermconfig-model.ts:520-653`). The same atom renders the header banner `<span role="alert">{errorMessage}</span>` at `remotetermconfig.tsx:280-283`. That banner sits inside the `selectedFile &&` wrapper (`:186`), the same wrapper that mounts the Secrets `VisualComponent` (`:310-314`). Every `ErrorDisplay` at `secretscontent.tsx:371,403` is therefore paired with an announced alert.

The residual is `:355`, the `storageBackendErrorAtom` warning. It replaces the whole view after the async backend check (`remotetermconfig-model.ts:501-508`) and has no live region. It is view content found on navigation rather than an action-result status message, so it is marginal under 4.1.3.

## R2-CONF-1
VERDICT: REFUTED (as a user-visible defect)

The code paths are Linux only: every main window (`emain/emain-window.ts:187-193`) and the builder window (`emain/emain-builder.ts:61-64`). The macOS and Windows branches set no `icon`. The path is `getElectronAppBasePath()` (`emain-platform.ts:400-402` = `dist/`) plus `public/logos/wave-logo-dark.png`, which resolves to `dist/public/logos/...`. That file is never produced:
- The renderer `outDir` is `dist/frontend` (`electron.vite.config.ts:129`), and Vite copies `public/` there, giving `dist/frontend/logos/`. Built trees in `remoteterm-config-reskin/dist` confirm that `dist/public` is absent.
- electron-builder packages only `./dist` and `package.json` (`electron-builder.config.cjs:21-33`).

The icon path is dead in both dev and packaged builds (and was dead upstream too, `main:emain/emain-window.ts:193`), so Electron gets an empty image, and the WM falls back to the `.desktop` or build icon, which was rebranded in `e7a30ef8`. The running app does not show this Wave art. The residual is a cleanup note: the dead reference to a Wave-named asset. Anyone who "fixes" the path must swap the art too, because the file is still Wave's logo (blob `c40e5790` is identical to `main`).

## R2-CONF-2
VERDICT: REFUTED (misread)

`RENAME_PLAN.md:839-842` tells the migration to sit alongside the existing `getWaveHomeDir()` compat pattern and not replace it. That existing pattern (`main:emain/emain-platform.ts:74-87`) was `~/.${waveDirName}`, which is suffix-aware: `~/.waveterm` in prod and `~/.waveterm-dev` in dev. It never had a dev→bare `~/.waveterm` fallback. The plan's parenthetical "legacy `~/.waveterm`" describes that prod value. HEAD keeps the pattern exactly as `legacySuffixedHome` (`emain-platform.ts:331-334`), and `6563e25c` removed only the fork-introduced bare fallback, which was never upstream. No behaviour contradicts the plan. At most the plan's wording could be tightened.
