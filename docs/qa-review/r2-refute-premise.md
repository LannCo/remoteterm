# Round-2 refute: premises and intent

Commit `75b60911`. Fix wave `35cd6146..75b60911`; branch `origin/main..35cd6146`; "upstream" = `origin/main` (LannCo/remoteterm).

## R2-CA-1
VERDICT: HOLDS

- `emain-platform.ts:236-253` (data and legacy-home `migrateDataRoot` calls) at `75b60911`: the only gate on the source is `lockFileSkipReason` (`:192-194`), which checks that `wave.lock` *exists*, not that it is held. The config root (`:222-228`) has no liveness check at all. `renameSync` at `:169` runs regardless. Cite drift: `:271` is the `catch`; the call is `performDataDirMigration()` at `:275`.
- `emain.ts:264` `requestSingleInstanceLock()` runs after the import-time migration and under the new app name, so it cannot see a legacy instance either.
- Intent: commit `282c9246` (fix wave) documents the design choice: the lock lives in Go "because Node has no portable flock", and its test `TestAcquireWaveLockRefusesWhenLegacyLockHeldInMovedDir` explicitly renames a dir with a held lock first. So the Go check was designed to stop dual DB open, not to stop the move. The claim's premise (move happens before any liveness check) is correct and acknowledged by the design.
- Origin: rename-without-liveness from branch (`0bc74b18`); fix wave's Go check is partial by design.
- Severity: Low. Requires a legacy Wave instance still running while the new build first launches; the new server then refuses to start (visible), open fds in the old instance survive the rename on POSIX, but the old instance's later path-based writes (config writes, new files) fail.

## R2-CA-2
VERDICT: HOLDS

- `pkg/secretstore/secretstore.go:26,80,203`: `secrets.enc` is read/written at `filepath.Join(remotetermbase.GetWaveConfigDir(), "secrets.enc")`: the **config** root, so the claim is correctly placed.
- `emain-platform.ts:199-210` `legacyConfigSkipReason` qualifies a root only on a top-level `*.json` file or a `presets/` directory; a root holding only `secrets.enc` (plus `electron/`) returns "has no config files" and is skipped.
- Origin: branch (`0bc74b18` gated on `settings.json`, `35cd6146:205`); fix wave `7e781c9e` widened the predicate but still omits `secrets.enc`. Not documented as intended (commit message lists `connections.json`, `widgets.json`, presets only).
- Severity: Low-Medium. Narrow population (secrets saved but no config file ever written) but the loss is of encrypted credentials, and it is silent apart from a console log.

## R2-CA-3
VERDICT: HOLDS

- `remotetermconfig-model.ts:946-952`: `applyBackgroundToTab` awaits `SetMetaCommand` with no try/catch and no `errorMessageAtom` write (contrast `:940-942` in the neighbouring method).
- `backgroundscontent.tsx:201,208`: `onClick={() => model.applyBackgroundToTab(...)}`, the promise is dropped; `BackgroundTile` (`:37-60`) passes `onClick` straight to the `<button>`. No `fireAndForget` wrapper.
- `git grep 'unhandledrejection|unhandledRejection'` over `frontend` and `emain` at `75b60911`: no matches, so nothing global catches it.
- Origin: branch (`b8ea6c6f`, not present at `origin/main`).
- Severity: Low (local RPC failure is rare; consequence is a silent no-op plus console error).

## R2-CA-4
VERDICT: HOLDS

- `pkg/wshrpc/wshremote/git.go:327-343`: for `Staged`, the diff is `git diff --cached` (HEAD vs index), then `applyPatchReverse` (`:1126-1135`) runs `git apply -R` with no `--index`/`--cached`, which touches the working tree only. The index keeps the staged change.
- `DiffGutter.tsx:39-62`: the revert glyph is shown **only** for staged hunks, so this is the only revert path. Whether "revert" means discard or unstage, the result is wrong (the index keeps the change, or the working tree loses it).
- Origin: upstream. `origin/main:git.go:321-342` has the same `diff --cached` + plain `git apply` (hand-inverted patch). Fix wave `eb94ed5d` swapped to `apply -R` but kept working-tree-only semantics. The claim's "predates the branch" is correct.
- Severity: Medium as stated; not a branch regression.

## R2-CA-5
VERDICT: HOLDS

- `remotetermconfig-model.ts:826-852`: `prevOrder`/`nextOrder` read from `widgetsMapAtom` (`:827,833-834`) *before* `persistWidgetPatch`; only `latest` inside the queued closure is fresh. `newOrder` is captured at enqueue time.
- `widgetscontent.tsx:248-264` passes the optimistic `localKeys` as `orderedKeys`, so keys are fresh but their `display:order` values come from the stale atom. Concrete loss: X0,Y1,Z2; move Z up gives Z=0.5 (UI X,Z,Y); before the watcher round-trip, move Y up gives UI X,Y,Z, neighbours X=0, Z=**2 (stale)**, so Y=1. Result on disk: X0,Z0.5,Y1, which is X,Z,Y. The second move is lost when the list resyncs.
- The same widget moved twice in a row does not lose (its own stale order is not used as a neighbour), so the claim is right for moves involving different widgets.
- Origin: branch (`650af55e`). Fix wave `9f786c0b` moved the *patch* into the queue but not the neighbour computation; its comment at `:774-777` states the intent the code misses.
- Severity: Low.

## R2-CA-6
VERDICT: HOLDS

- `generalcontent.tsx:1086-1089` `bump` computes from the `value` prop and calls `onChange`. `:1105` input `onBlur={commit}`; the spin buttons (`:1115-1130`) have no `onMouseDown` preventDefault and no focus handling.
- Sequence in Chromium/Electron: mousedown on the button moves focus, the input blurs, `commit()` calls `onChange(typed)`; then click, `bump` calls `onChange(value+delta)` with the still-stale prop (the prop updates only after the config round-trip). The second write wins and the typed value is overwritten.
- Origin: branch (the pre-fix `bump` at `35cd6146:1052-1057` has the same stale-`value` pattern); fix wave `1559811c` only added `setDirty(false)` and range parsing.
- Severity: Low.

## R2-CA-9
VERDICT: HOLDS

- Pre-fix build (`35cd6146:205`) skipped a legacy config root lacking `settings.json` without writing a marker. That build still creates `~/.config/remoteterm` (Electron userData `remoteterm/electron`, set at `:17`, gets its SingletonLock/prefs there), so the destination becomes non-empty and unmarked.
- Fixed build: `legacyConfigSkipReason` now qualifies the same root (for example `connections.json`), then `:146-165` finds the destination non-empty and unmarked, calls `recordMigrationFailure(... aborted ... merge manually)` and returns without a marker, so the same happens on every launch. `emain.ts:283-290` shows it as an error dialog each launch.
- Intent: the abort-on-non-empty-destination is deliberate (comment `:99-104` calls it "sticky"), and the fix wave's own comment at `:196-198` anticipates the Electron-only case, but not this transition from the pre-fix build. The dialog does give a manual fix.
- Origin: fix wave (`7e781c9e`) exposes branch-build users to the existing abort.
- Severity: Low. Population: people who ran an intermediate branch build (the user's daily driver) with a legacy config root lacking `settings.json`.

## R2-A11Y-1
VERDICT: HOLDS

- Tokens (`frontend/tailwindsetup.css:9,26`, identical at `origin/main`): `--color-error: rgb(229,77,46)` = `#E54D2E`; `--color-background: rgb(34,34,34)`.
- WCAG 2.x relative luminance, sRGB linearisation `((c/255+0.055)/1.055)^2.4`: L(error) = 0.2126·0.7834 + 0.7152·0.0742 + 0.0722·0.0240 = 0.2214; L(bg) = 0.0160. Ratio (0.2214+0.05)/(0.0160+0.05) = **4.12:1**. Over `bg-panel` (rgba(31,33,31,0.5) composited over 34) it is 4.15:1; over `#232323` modal bg it is 4.07:1.
- Sizes: `secretscontent.tsx:155` `text-caption` = 11px (`tailwindsetup.css:48`); `connectionscontent.tsx:111`, `backgroundscontent.tsx:114` `text-xs` = 12px (Tailwind default, not overridden); regular weight. Not large text, so **4.5:1** applies. Fails.
- Origin: token upstream; these usages are branch (`text-error` present at `35cd6146` in all three files). The fix wave (`87056ec3`) added `role="alert"` to two of them but kept the colour.
- Severity: Low-Medium (AA fail, margin ~0.4).

## R2-A11Y-2
VERDICT: REFUTED

- `secretscontent.tsx:14-25` `ErrorDisplay` indeed has no role/aria-live.
- But `:371` and `:403` render `model.errorMessageAtom`, the same atom rendered by the parent view at `remotetermconfig.tsx:280-282` as `<span role="alert">{errorMessage}</span>`. That banner sits inside `{selectedFile && ...}` (`:186`) unconditionally, and Secrets is a `selectedFile` (`remotetermconfig-model.ts:87` `visualComponent: SecretsContent`). Save/delete/add failures (`remotetermconfig-model.ts:579,600,653`) set that atom, so they **are** announced (and shown twice visually).
- Residual: `:355` renders `storageBackendErrorAtom`, which is not in the banner and is unannounced. It appears on opening the tab (a load-time state, not a status change after user action), so it is at most a weak 4.1.3 case.
- The claim's main premise (failed secret save/delete not announced) is false. Severity correction: Info (duplicate error rendering; the storage-backend warning lacks `role="status"`).

## R2-CONF-1
VERDICT: HOLDS

- `emain-window.ts:193` and `emain-builder.ts:63` (Linux) set the icon to `public/logos/wave-logo-dark.png`. Blob `c40e5790` is identical at `origin/main`; no branch commit touches `public/logos/`.
- Intent: `RENAME_ALLOWLIST.md:115-119` and `RENAME_PLAN.md:296-306` list `public/logos/wave-*.png` as Wave trademark art that is **blocking**, pending new artwork. It is not accepted as a final state. The fix wave `e7a30ef8` produced RemoteTerm renders for `build/icons/*` only, so the artwork now exists but the runtime path was missed.
- Origin: upstream path/art; left open by the fix wave.
- Severity: Low-Medium (trademark/branding; visible on X11 taskbars that use `_NET_WM_ICON`, for example Cinnamon). It was already known, so it is not a new discovery.

## R2-CONF-2
VERDICT: REFUTED

- `RENAME_PLAN.md:839-842` says the migration "reuses the existing `getWaveHomeDir()` backwards-compat pattern ... (it already has a legacy `~/.waveterm` fallback for pre-v0.8 installs) ... sit alongside it ... not replace it." It says nothing about dev builds.
- Premise check at `origin/main`: `emain-platform.ts:29-31` `waveDirName` is dev-suffixed (`waveterm-dev` in dev), and `getWaveHomeDir` (`:74-86`) uses `.${waveDirName}`. So upstream dev builds used `~/.waveterm-dev`, never bare `~/.waveterm`. The premise is false.
- `6563e25c` removed only a branch-introduced dev-to-bare fallback. The legacy compat layer is retained at `75b60911:331-333` (`LegacyRemoteTermHomeDirNameSuffixed`, which is `~/.waveterm` in prod and `~/.waveterm-dev` in dev), matching upstream and the plan.
- There is no contradiction between the plan and the code. Severity: none.
