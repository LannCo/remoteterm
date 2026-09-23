# Round-2 refutation: reproduction lens

Base: `75b60911`. Scratch worktree: `scratchpad/repro-r2` (detached). Saved tests: `scratchpad/repro-tests-r2/`.
`S=/tmp/claude-1000/-media-owner-Workspace-remoteterm/6b9bb981-3b3e-4d9c-9a93-b16272c3469e/scratchpad`.

## R2-CA-1

VERDICT: HOLDS

Test: `$S/repro-tests-r2/R2-CA-1_2_9_migration.test.ts` (describe "R2-CA-1"). A child `flock -x wave.lock sleep 30` holds the legacy lock (temp HOME/XDG, mocked `electron`), then `emain-platform` is imported.

Command: `npx vitest run emain/r2-repro-migration.test.ts -t R2-CA-1`

```
R2-CA-1 moved dir exists: true lock still held at new path: true
× does NOT move a data root whose wave.lock is held by a live process   -> expected false to be true
× does NOT move a legacy home whose wave.lock is held by a live process -> expected false to be true
```

Both the XDG data root and `~/.waveterm` are renamed while the lock is held. The flock follows the inode, so after the move the lock is still held at the new path; the Go legacy-lock check would then refuse to start, as the claim says. The running old instance has already lost its path by then.

## R2-CA-2

VERDICT: HOLDS

Test: same file, describe "R2-CA-2". The legacy `$XDG_CONFIG_HOME/waveterm` holds only `secrets.enc`. `secretstore.go:80` stores it in the config dir.

```
× migrates a legacy config root containing only secrets.enc -> expected false to be true
```

`legacyConfigSkipReason` accepts only `*.json` or `presets/`, so the root is skipped and `secrets.enc` stays behind.

## R2-CA-3

VERDICT: HOLDS

Test: `$S/repro-tests-r2/R2-CA-3_5_model.test.ts` ("R2-CA-3"). Uses the widgets-test harness with `SetMetaCommand` rejecting.

Command: `npx vitest run frontend/app/view/remotetermconfig/r2-repro-model.test.ts`

```
R2-CA-3 rejected: Error: rpc down errorMessageAtom: null
× applyBackgroundToTab surfaces an RPC failure instead of rejecting
```

The model rejects and sets no error. `backgroundscontent.tsx:201,208` calls `onClick={() => model.applyBackgroundToTab(...)}` and drops the promise, so the rejection goes unhandled. That last step comes from reading the code; there is no DOM in this repo to test it.

## R2-CA-4

VERDICT: HOLDS

Test: `$S/repro-tests-r2/R2-CA-4_revert_staged_test.go`. Uses the existing `makeRevertTestRepo` in a temp repo: stage `b->B`, then `GitRevertHunkCommand{Staged:true}`.

Command: `go test ./pkg/wshrpc/wshremote/ -run TestR2Repro -v`

```
worktree="a\nb\nc\n"
staged diff:   -b +B
unstaged diff: -B +b
--- FAIL: staged hunk still in index after revert
```

`applyPatchReverse` runs `git apply -R` without `--index`/`--cached`. The worktree is reverted, the index keeps `B`, and the worktree now holds an inverse unstaged change. The UI reaches this through `sourcecontrol-model.ts:392` and `review-mode.tsx:60` with `staged=true`.

## R2-CA-5

VERDICT: HOLDS (the second move lands in the wrong place; nothing is dropped)

Test: `R2-CA-3_5_model.test.ts` ("R2-CA-5"). Move 1 drags terminal to index 1. Move 2, on the updated local keys, drags files to index 1. Both fire before the config watcher round-trip.

```
written: terminal -4.5, files -5.5
sorted: ["files","source\ncontrol","terminal",...]   expected ["source\ncontrol","files","terminal",...]
✓ control: same moves with fullConfigAtom refreshed between them -> correct order
```

Move 2 reads terminal's order from the stale `fullConfigAtom` value (-6), not the queued -4.5, so files sorts ahead of sourcecontrol.

## R2-CA-6

VERDICT: NOT-REPRODUCED (reading: HOLDS)

A DOM is needed (focus/blur ordering). Reading `generalcontent.tsx:1062-1130`: with a typed but uncommitted value, pressing the spin button blurs the input first, so `commit()` writes the typed value. The click then runs `bump()`, which calls `onChange(value ± step)` using the stale `value` prop. `setGeneralSetting` (`remotetermconfig-model.ts:693`) is not optimistic, so the prop has not updated yet. Two `SetConfigCommand` writes follow, and the last one (old ± step) overwrites the typed value.

## R2-CA-9

VERDICT: HOLDS (scoped to legacy config roots with `*.json`/`presets/` but no `settings.json`)

Test: `R2-CA-1_2_9_migration.test.ts` ("R2-CA-9"). Pre-fix module: `git show 35cd6146:emain/emain-platform.ts` (saved as `R2-CA-9_emain-platform-prefix-35cd6146.ts`).

Sequence:
1. Legacy config has `connections.json` + `electron/`. The pre-fix build skips it (its gate needs `settings.json`).
2. Electron userData lands in `~/.config/remoteterm/electron` (`app.setName("remoteterm/electron")`).
3. The fixed build is launched twice.

```
first:  ["config root migration aborted: .../.config/remoteterm already exists and is not empty. ..."]
second: [same]
× new build does not fail on every launch
✓ settings.json legacy config: pre-fix migrated it, new build is a no-op
```

The abort repeats on every launch. Users whose legacy config had `settings.json` are unaffected.

## R2-A11Y-1

VERDICT: HOLDS

Script: `$S/repro-tests-r2/R2-A11Y-1_contrast.py` (WCAG relative luminance).

```
--color-background rgb(34,34,34): 4.12:1
black/25 over bg: 4.50:1
pure black: 5.43:1
```

`--color-error` is rgb(229,77,46) (`tailwindsetup.css:26`), used at text-xs/text-caption (12px/11px). The config view root is `bg-background` (`remotetermconfig.tsx:36`), so 4.12:1 is below 4.5:1.

## R2-A11Y-2

VERDICT: REFUTED for save/delete errors; HOLDS only for the storage-backend warning (:355)

Test: `$S/repro-tests-r2/R2-A11Y-2_errordisplay.test.tsx` (SSR, `renderToStaticMarkup`, same Proxy-model pattern as `remotetermconfig-a11y.test.tsx`).

```
× :355 / :371 / :403 -> <div class="flex ... text-error"><i aria-hidden ...><span>MSG   (no role/aria-live)
```

The inner `ErrorDisplay` has no live region, as claimed. However, the secret save/delete/add/load failures (`remotetermconfig-model.ts:520-653`) are written to the shared `model.errorMessageAtom`. The outer view renders that atom inside `<span role="alert">` (`remotetermconfig.tsx:280-282`) whenever any file is selected, including secrets, so those failures are announced (they are also shown twice on screen). Only `storageBackendErrorAtom` (set in `checkStorageBackend`, `:496-503`) is outside any live region. That is a load-time warning, not the result of a user action.

## R2-CONF-1

VERDICT: HOLDS

Reading the image: `public/logos/wave-logo-dark.png` is the upstream Wave Terminal mark (two green wave shapes on black). Its only commit is upstream `8508a402` ("try adding the wave logo to header"). It is referenced at `emain/emain-window.ts:193` and `emain/emain-builder.ts:63` (Linux `BrowserWindow.icon`). The packaged icons in `build/icons` were rebranded in `e7a30ef8`, but these runtime references were not. Whether a given shell uses `_NET_WM_ICON` or the `.desktop` icon varies by DE. Where `_NET_WM_ICON` is used (most X11 WMs and taskbars), the Wave art shows.

## R2-CONF-2

VERDICT: HOLDS (dev-build-only doc drift; production behaviour matches the plan)

`RENAME_PLAN.md:839-842` says to reuse `getWaveHomeDir()`'s "legacy `~/.waveterm` fallback for pre-v0.8 installs" alongside the migration, "not replace it". At `815835c0^` the fork's code had that bare fallback (`home = path.join(homeDir, LegacyWaveHomeDirName)`). `6563e25c` (an ancestor of HEAD) removed it for dev builds. In production the suffixed legacy dir is `~/.waveterm`, so the fallback survives there.

The plan's rationale is also factually wrong. Upstream `getWaveHomeDir` at `146ceb1e` only checks `.${waveDirName}` (suffixed) and has no pre-v0.8 fallback; the "pre-v0.8" comment refers to the `wave.lock` check. The plan text needs correcting, not the code.
