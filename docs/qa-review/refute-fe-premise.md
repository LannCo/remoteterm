# Refutation pass: frontend claims, premises and intent lens

Commit `93aaefc3`; base `origin/main` (merge-base `6d6128e5`). All `frontend/app/view/remotetermconfig/*` files are absent at `origin/main`; `secretscontent.tsx` and the error banners in `remotetermconfig.tsx` are renamed/restyled copies of `frontend/app/view/waveconfig/{secretscontent,waveconfig}.tsx` on main, so "introduced" is judged against those where relevant.

## CA-1
VERDICT: HOLDS

- `remotetermconfig-model.ts:810-839`: `reorderWidget` and `toggleWidgetHidden` both read `globalStore.get(this.widgetsMapAtom)` synchronously and build `{ ...widget, <one field> }` before calling `persistWidgetPatch`. The queue (`775-780`) only defers `writeWidgetPatch(updates)`; `updates` is already fixed. `writeWidgetPatch` (`790`) does `{ ...rawContent, ...updates }`, so the whole key is replaced with the enqueue-time snapshot.
- Jotai premise: `widgetsMapAtom` (`228-231`) is derived from `env.atoms.fullConfigAtom`, which changes only when the backend pushes a new config after the file write. So `globalStore.get` at enqueue returns the pre-toggle widget until that push lands; a drag enqueued in that window writes `display:hidden` back to its old value. Same shape for `updateBackgroundOpacity`/`updateBackgroundBlendMode` (`923-935`) via `backgroundsMapAtom` (`247-250`).
- Intent: the comment at `765-770` says the queue exists so "a toggle and a drag-drop landing close together can't race" and that "each write's read-then-merge waits for the previous write". The read-merge is per-file, not per-field, so the stated intent is not met. Unintended.
- Introduced by branch: yes.
- Severity: as filed. Window is config-watch latency, not only simultaneous input; slider drags on opacity make the backgrounds case easy to hit.

## CA-2
VERDICT: HOLDS (severity down)

- `generalcontent.tsx:1068-1071`: `onChange` does `Number(e.target.value)` and calls `onChange(next)` if finite. `Number("") === 0`, so clearing writes 0. `min`/`max` are only passed to the native input (no clamp on typed input); only `bump()` clamps (`1052-1057`).
- `write` (`1156`) calls `model.setGeneralSetting` → `SetConfigCommand` per call (`remotetermconfig-model.ts:688-696`), no debounce: every keystroke persists.
- Schema for `window:maxtabcachesize` sets `min: 1, max: 50` (`generalcontent.tsx:305-315`), so clamping was the intent; typed path bypasses it.
- `emain/emain.ts:315-316` → `emain/emain-tabview.ts:238-240` assigns without validation. Consequence checked: `checkAndEvictCache` (`276-291`) with 0 (or negative) evicts every non-active tab idle >1s; `tryEvictEntry` refuses the active tab. Result is loss of tab caching (slower switching, reloads), not a crash.
- Introduced by branch: UI yes; emain setter unguarded identically at `origin/main:emain/emain-tabview.ts:240`.
- Severity: Low (performance degradation, self-correcting on re-edit).

## CA-11
VERDICT: HOLDS (pre-existing)

- `sourcecontrol-model.ts:627-640`: loops `i = 0..hunkCount-1` calling `revertHunk(path, i, staged)`.
- Server premise: `pkg/wshrpc/wshremote/git.go:321-342` re-runs `git diff` on every call, re-parses hunks, and indexes into the fresh list. Reproduced the algorithm in a scratch repo (Python port of `extractHunkInversePatch`/`extractHunkLines`, same `git apply`): 3 modified hunks → i=0 reverts old hunk 0, i=1 reverts old hunk 2, i=2 "out of range"; 1 hunk survives. `revertHunk` swallows the error (`387-399`), so the UI reports nothing.
- Test `review-mode.test.ts:505-533` mocks `revertHunk` and asserts calls with `0` and `1`: it encodes the bug rather than catching it.
- Adjacent premise defect found: `extractHunkInversePatch` (`git.go:1092-1110`) swaps `+`/`-` body lines but keeps the `@@ -a,b +c,d @@` header unswapped. For any hunk whose added and removed counts differ, `git apply` fails with "corrupt patch" (reproduced: rc 128). Such hunks are never reverted, individually or via "Revert file", which also changes which hunks the index shift skips. Worth a separate finding.
- Introduced by branch: no. Identical loop at `origin/main:sourcecontrol-model.ts:610-619`; `git.go` diff vs main is import path + gofmt only.
- Severity: as filed (silent data-retention bug in a destructive action); inherited.

## CA-12
VERDICT: HOLDS

- `remotetermconfig-model.ts:979-996`: `submitBackgroundAdd` awaits `addBackground`, then `applyBackgroundToTab(key)` if `key` truthy.
- `addBackground` (`937-963`) returns `key` unconditionally after `await persistBackgroundPatch(...)`. `writeBackgroundPatch` (`891-913`) never throws: read failure returns early, write failure is caught and only sets `errorMessageAtom`. So the tab's `tab:background` is set to a key absent from `backgrounds.json`.
- Intent: no comment or test covers the failure path.
- Introduced by branch: yes.
- Severity: Low. Error banner is shown; tab points at a dangling key (renders no background).

## A11Y-1
VERDICT: HOLDS (narrowed)

- `secretscontent.tsx:134-135`, `152-153`, `216-217`: `<label>` siblings, no `htmlFor`, inputs have no `id`, not wrapped. Label association is absent (1.3.1 holds for all three).
- Premise "no accessible name" is partly false: per HTML-AAM accname for `input`/`textarea`, `placeholder` is used as fallback. Add form Name gets "MY_SECRET_NAME", Add form Value gets "Enter secret value...", detail Value gets "Enter new secret value..." only while `!secretShown` (`234`). After Reveal the placeholder is `""`, so 4.1.2 fails only there. Placeholder-derived "MY_SECRET_NAME" is a poor name but is a name.
- Introduced by branch: no; `origin/main:frontend/app/view/waveconfig/secretscontent.tsx:143,162,223` has the same unassociated labels.
- Severity: 1.3.1 as filed; 4.1.2 limited to the revealed-value state.

## A11Y-2
VERDICT: HOLDS

- `secretscontent.tsx:128` computes `isNameInvalid`; `138-141` toggles `border-error` only. No `aria-invalid`, no error text. The helper at `147-149` is static and always shown, not an error message, and is not linked via `aria-describedby`. The disabled Add button is the only other cue and is itself opacity-only.
- 3.3.1 wording ("item that is in error is identified and the error is described to the user in text") is not met; 1.4.1 fails as colour is the sole indicator of the error state.
- Introduced by branch: no; `origin/main` waveconfig `secretscontent.tsx:137-150` same.

## A11Y-3
VERDICT: HOLDS (narrowed)

- `secretscontent.tsx:218-223`: inline arrow `ref` that calls `ref.focus()`.
- React premise: react.dev, "Common components > ref callback function": React calls the ref callback again whenever a different callback is passed; an inline arrow is a new function every render, so on each commit the previous callback is called with `null` (or its cleanup, React 19) and the new one with the node. React 19 changelog ("Cleanup functions for refs") keeps this behaviour. No React Compiler in the build (`package.json`: react ^19.2.0, no babel-plugin-react-compiler), so nothing memoises it.
- Re-render triggers: `selectedSecretAtom`, `secretValueAtom`, `secretShownAtom`, `isLoadingAtom` (`197-200`). Typing: textarea already has focus, `.focus()` is a no-op, so "during typing" is overstated. Reveal/Save/Delete: `isLoading` toggles true→false and re-renders, pulling focus from the activated button back to the textarea. That part holds (2.4.3 / unexpected focus move).
- Introduced by branch: no; `origin/main` waveconfig `secretscontent.tsx:225-229` identical pattern.

## A11Y-4
VERDICT: HOLDS (mis-cited file)

- `remotetermconfig.tsx:280-298` (errorMessage, validationError banners), `connectionscontent.tsx:110`, `backgroundscontent.tsx:113`, `secretscontent.tsx:12-22` (`ErrorDisplay`): none has `role="alert"`, `role="status"` or `aria-live`, and none takes focus. 4.1.3 applies.
- `generalcontent.tsx` contains no error UI at all (no `error`/`alert` matches); its save errors surface through the `remotetermconfig.tsx` banner. Drop it from the claim.
- Introduced by branch: partly. Banners in `remotetermconfig.tsx` and `ErrorDisplay` exist unchanged in semantics at `origin/main` waveconfig (`waveconfig.tsx:264,275`; `secretscontent.tsx:335,387`). Quick-add errors in connections/backgrounds are new.

## A11Y-5
VERDICT: HOLDS (severity down)

- `connectionscontent.tsx:231-238`: row `bg-panel ... opacity-70`; subtitle `text-muted` (`235`), fingerprint `text-muted` (`238`).
- Tokens (`frontend/tailwindsetup.css`): `--color-muted` rgb(140,145,140), `--color-panel` rgba(31,33,31,0.5), `--color-background` rgb(34,34,34).
- Compositing premise: CSS `opacity` renders the element and its descendants as one group, then blends the group with the backdrop, so text and row background are both mixed 70/30 with the page background. Computed: muted on panel without opacity 5.00:1; with group opacity 0.7 over #222: text (108,112,108) on (33,34,33) = 3.17:1. Fails 4.5:1. `text-secondary` in the same row: 5.36:1, passes. Assumes the view sits on `--color-background`; a tab background image under a transparent block would change it.
- Exemption: 1.4.3 "incidental" covers text in an inactive user interface component. These rows are non-interactive `div`s, not disabled controls, and the text conveys content, so neither "inactive component" nor "pure decoration" applies. The disabled "Generate / import key" button below them is exempt.
- Intent: `KeychainBanner` (`206-216`) labels the whole view "Concept only ... not wired to anything real"; data is hard-coded mock. Dimming is deliberate.
- Introduced by branch: yes.
- Severity: Low (mock content in a concept preview).

## A11Y-6
VERDICT: REFUTED

- NumberControl (`generalcontent.tsx:1076-1093`): visual 16x11 px; `::before` extends the hit area by 6px each side and 8px/2px vertically, so each target is about 28x21 px. The two are stacked 12px apart, so the spacing exception fails for them in isolation.
- WidgetOrderRow (`widgetscontent.tsx:~145-162`): 20x13 px, stacked with no gap.
- 2.5.8 "Equivalent" exception: "The function can be achieved through a different control on the same page that meets this criterion." NumberControl's value is settable through the adjacent number input (56px wide; undersized in height but its 24px spacing circle clears other targets). WidgetOrderRow reorder is achievable by dragging the whole row (`useDrag`, full-width row). Both plausibly satisfy the exception; the claim itself concedes "possibly exempt" and does not show otherwise. Burden not met.
- Introduced by branch: yes.
- Severity: none as a conformance failure; at most a usability note on small pointer targets.
