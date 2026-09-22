# Frontend claims: reproduction lens

Worktree: `scratchpad/repro-fe` @ `93aaefc3` (detached). No DOM library is installed in this repo (no jsdom/happy-dom/@testing-library), so DOM tests borrow jsdom 27.4.0 read-only from `/media/owner/Workspace/snoppet/node_modules/jsdom` through a scratch harness (`repro-dom.ts`), with real React 19 `createRoot` + real jotai stores. Nothing was installed or committed.

`S` = `/tmp/claude-1000/-media-owner-Workspace-remoteterm/6b9bb981-3b3e-4d9c-9a93-b16272c3469e/scratchpad`. Saved regression tests: `S/repro-tests/`. Frontend commands run from `S/repro-fe`.

## CA-1

VERDICT: **HOLDS** (widgets and backgrounds)

- Test: `S/repro-tests/CA-1_CA-12_stale-snapshot-and-failed-write.test.ts` (model mocks copied from `remotetermconfig-model.widgets.test.ts`; in-memory disk, `fullConfigAtom` not refreshed between calls, as happens before the config watcher fires)
- Command: `npx vitest run frontend/app/view/remotetermconfig/repro-ca1-ca12.test.ts`
- Output:
  ```
  CA-1 widgets on disk: {"w@a":{"display:order":3,"label":"a"}}          <- display:hidden:true lost
  CA-1 bg on disk: {"bg@x":{...,"bg:opacity":0.3,"bg:blendmode":"multiply"}}  <- opacity 0.9 lost
  x CA-1 widgets: expected undefined to be true
  x CA-1 backgrounds: expected 0.3 to be 0.9
  ```
- Mechanism: `toggleWidgetHidden`/`reorderWidget` (`remotetermconfig-model.ts:811,835`) and `updateBackgroundOpacity`/`BlendMode` (`:924,931`) build the whole-key patch from `widgetsMapAtom`/`backgroundsMapAtom` at call time; the queue serialises the read of the file but the second patch replaces the whole key with its stale copy.

## CA-2

VERDICT: **HOLDS (with correction)** - no clamp, clearing saves 0, negatives/over-max saved; but 0 specifically does **not** reach emain.

- Frontend test: `S/repro-tests/CA-2_numbercontrol-unclamped.test.tsx` (renders real `GeneralContent`, types into the Max tab cache size input)
- Command: `npx vitest run frontend/app/view/remotetermconfig/repro-ca2.test.tsx`
- Output: `CA-2 saved values for keystrokes ['', '0', '-5', '500']: [0,0,-5,500]` (schema min 1, max 50) -> test fails.
- Go test: `S/repro-tests/CA-2_go_omitempty_test.go` (merge default 10 with user value, `ReUnmarshal` into `SettingsType`, marshal as sent to clients)
- Command: `go test ./pkg/rtconfig/ -run TestReproCA2 -v`
- Output:
  ```
  user value 0 -> key in fullConfig JSON sent to emain: <nil> (present=false)
  user value -5 -> key in fullConfig JSON sent to emain: -5 (present=true)
  ```
- Correction: `WindowMaxTabCacheSize int ...,omitempty` (`pkg/rtconfig/settingsconfig.go:118`) drops 0, so `emain.ts:315` never calls `setMaxTabCacheSize(0)`. Negative values do reach it; with `MaxCacheSize=-5`, `checkAndEvictCache` (`emain-tabview.ts:288`) loops `sorted.length + 5` times and reads `sorted[i].remoteTermTabId` past the array end (TypeError) once the cache has any entry. By reading only: emain reads the value only at startup (`emain.ts:315`).

## CA-11

VERDICT: **HOLDS**

- Test: `S/repro-tests/CA-11_revert-file-ascending-indices_test.go` (temp git repo, 80-line file, 4 separated hunks; calls the real `GitRevertHunkCommand` for i = 0..hunkCount-1 exactly as `revertFileFromReview` does, errors swallowed like `revertHunk`'s catch; `GIT_CONFIG_GLOBAL=/dev/null`)
- Command: `go test ./pkg/wshrpc/wshremote/ -run TestReproCA11 -v`
- Output:
  ```
  revert index 0: err=<nil>
  revert index 1: err=<nil>
  revert index 2: err=hunk index 2 out of range
  revert index 3: err=hunk index 3 out of range
  hunks before=4 after=2
  CA-11 reproduced: 2 of 4 hunks survive 'Revert file'
  ```
- Existing test pins the bug: `review-mode.test.ts:531-532` asserts calls with indices 0 and 1.

## CA-12

VERDICT: **HOLDS**

- Test: same file as CA-1, case "CA-12" (`FileWriteCommand` throws `EACCES` for backgrounds.json)
- Output:
  ```
  CA-12 SetMeta calls: [..., {"oref":"tab:t","meta":{"bg:*":true,"tab:background":"bg@mine"}}]
  error: Failed to save backgrounds.json: EACCES
  x expected [...] to have a length of +0 but got 1
  ```
- Mechanism: `writeBackgroundPatch` swallows the error; `addBackground` returns the key unconditionally; `submitBackgroundAdd` applies it (`remotetermconfig-model.ts:991-995`), leaving the tab pointing at a non-existent key. The quick-add form is also closed, so the error only shows in the global banner.

## A11Y-1

VERDICT: **HOLDS** (nuance: placeholder fallback)

- Test: `S/repro-tests/A11Y-1_2_3_secrets-labels-invalid-focus.test.tsx` (renders real `SecretsContent` in add mode and detail mode)
- Command: `npx vitest run frontend/app/view/remotetermconfig/repro-a11y-secrets.test.tsx`
- Output: `{"addName":["","MY_SECRET_NAME"],"addValue":["","Enter secret value..."],"detailValue":["",""],"labelsWithFor":0}` -> `expected '' to be 'Name'`
- No label-derived name on any of the three fields. Browsers fall back to the placeholder, so the add-form fields get "MY_SECRET_NAME" / "Enter secret value..." (a poor name, 1.3.1 still fails); the detail-view Value textarea has an empty placeholder once revealed, so it has no name at all.

## A11Y-2

VERDICT: **HOLDS**

- Test: same file, case "A11Y-2" (name `1bad-name`)
- Output: `{"class":"border-error","ariaInvalid":null,"describedby":[],"alert":0}` -> `expected null to be 'true'`
- A static hint ("Must start with a letter...") is always shown but is not tied to the input and does not change on error; the invalid state is signalled only by the red border (plus a disabled submit button).

## A11Y-3

VERDICT: **HOLDS**

- Test: same file, case "A11Y-3" (spy on `HTMLTextAreaElement.prototype.focus`; focus Save, type, re-render)
- Output: `focus() calls: mount= 1 total= 5 activeElement after last re-render: TEXTAREA` -> `expected 5 to be 1`
- Each re-render passes a new callback ref, React calls it again, and `.focus()` pulls focus off the Save button back to the textarea.

## A11Y-4

VERDICT: **HOLDS** (location detail slightly off)

- Test: `S/repro-tests/A11Y-4_quickadd-error-live-region.test.tsx` (renders real `BackgroundsContent` and `ConnectionsContent` with a sentinel error; walks ancestors for `role=alert|status` / `aria-live`)
- Command: `npx vitest run frontend/app/view/remotetermconfig/repro-a11y4.test.tsx`
- Output: `{"bgRendered":true,"bgLive":null,"connRendered":true,"connLive":null}` -> fails.
- By reading: `remotetermconfig.tsx:280-303` error and validation banners contain no `role`/`aria-live` (the file has none anywhere). `generalcontent.tsx` has no banner of its own; its save errors go through `errorMessageAtom` into the `remotetermconfig.tsx` banner, so it is covered by that one. `secretscontent.tsx` `ErrorDisplay` (`:14-24`) also lacks a live role.

## A11Y-5

VERDICT: **NOT-REPRODUCED** (jsdom cannot measure). Reading verdict: **HOLDS**

- Tokens (`frontend/tailwindsetup.css`): `--color-muted` rgb(140,145,140), `--color-panel` rgba(31,33,31,0.5), `--color-background` rgb(34,34,34).
- Computed (WCAG relative luminance, row `opacity-70` compositing the row over the page background): muted on panel without opacity 5.0:1; with `opacity-70` text rgb(108,112,108) on rgb(33,34,33) = **3.17:1**, below 4.5:1 for 10px `text-xxs`. Assumes an opaque window background; transparency or a tab background image shifts it.
- The rows are a "Concept only" mock-up, but they are informational text, not an inactive UI component, so no 1.4.3 exemption applies.

## A11Y-6

VERDICT: **NOT-REPRODUCED** (jsdom has no layout). Reading verdict: **HOLDS** for WidgetOrderRow; **HOLDS, weaker** for NumberControl

- WidgetOrderRow move buttons (`widgetscontent.tsx:146-160`): `w-5 h-[13px]` = 20x13 px, stacked with no gap, so the spacing exception fails too (24px circles overlap). The only alternative is drag reorder, a path-based gesture rather than an equivalent single-pointer control, so the equivalent-control exception is doubtful.
- NumberControl spin buttons (`generalcontent.tsx:1076-1092`): 16x11 px box; the `::before` hit-area extension (`-inset-x-1.5`, `-top-2`/`-bottom-0.5` and mirror) gives about 28x21 px, still under 24 px tall, and the two expanded areas overlap. The equivalent control (the number input, `w-14`, `text-xs`, no padding) is about 56x16 px, so it does not meet 24 px either and does not qualify for the exception.
