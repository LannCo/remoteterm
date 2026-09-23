# a11y-auditor report
**Target:** WCAG 2.2 AA source review of `frontend/app/view/remotetermconfig/*.tsx` (config modal), commits `8f21f2e8` and `1267d50a`, branch `qa/fleet-2026-09-22`
**Started:** 2026-09-22T00:00:00Z
**Status:** IN PROGRESS

## What works

- Widget drag-reorder (`widgetscontent.tsx`) ships a full keyboard alternative (Move up/down buttons, correctly `disabled` at list boundaries) — satisfies 2.5.7 Dragging Movements.
- `1267d50a` correctly swapped `aria-selected` for `aria-pressed` on the Visual/Raw JSON toggle buttons (`remotetermconfig.tsx:255,268`) — role/attribute pairing now valid.
- `8f21f2e8`'s accent-button recontrast is accurate: recomputed from the actual CSS custom-property values, `bg-accent/80 text-background` = 4.92:1, solid `:hover` (`bg-accent`) = 6.91:1. Matches the 4.91/6.91 claimed in `.kilocode/rules/rules.md`.
- `ToggleControl`/`VisibilityToggle` switches use `role="switch"` + `aria-checked` + `aria-label`, and the `p-1.5 -m-1.5` padding trick genuinely expands the clickable box to ~42×29px — clears 2.5.8's 24×24 minimum.
- `ResetButton` (`generalcontent.tsx:849-859`) is exactly 24×24 — clears the minimum.
- `prefers-reduced-motion` is handled globally (`frontend/app/store/global-atoms.ts:71-80`, composited with `window:reducedmotion`), not per-view, but it covers this view.
- `BackgroundDetailPanel`'s Opacity/Blend mode controls (`backgroundscontent.tsx:147,161`) use real wrapping `<label>` elements — correct native association, unlike the pattern in `secretscontent.tsx` (see A11Y-1).
- Quick-add, background-add, and search text inputs across all five pages use `aria-label` correctly.
- Disabled buttons styled with `opacity-70`/`opacity-50` (e.g. `connectionscontent.tsx:274-280`, `widgetscontent.tsx:296-304`) all carry the native `disabled` attribute, so 1.4.3's inactive-UI contrast exemption genuinely applies — not flagged.
- Deprecated-file badge and error/validation banner contrast all computed passing: black text on `bg-error` 5.43:1; `text-background` on blended `secondary/80` 6.51:1, `secondary/70` 5.32:1.

## Findings

### [Critical] A11Y-1: Secret name/value fields have no programmatic label
- **WCAG criterion:** 1.3.1 Info and Relationships (A), 4.1.2 Name, Role, Value (A)
- **Location:** `frontend/app/view/remotetermconfig/secretscontent.tsx:134-135` (Name input, add form), `:152-153` (Value textarea, add form), `:216-217` (Value textarea, detail view)
- **Issue:** `<label>` is a sibling of the input/textarea, not wrapping it, and neither element has `htmlFor`/`id`. Confirmed by grep — zero `id=` or `htmlFor` in this file. No `aria-label`/`aria-labelledby` either.
- **Impact:** Screen reader users get no accessible name when focusing these fields (announced as bare "edit text"). This is a secrets manager — a user typing a credential with no confirmation of which field has focus is a materially worse failure than the same bug elsewhere in the app.
- **Fix:** Wrap the input in the label (as `backgroundscontent.tsx:147-160` already does correctly), or add matching `id`/`htmlFor` pairs.

### [High] A11Y-2: Invalid secret name is colour-only, no error text or aria-invalid
- **WCAG criterion:** 3.3.1 Error Identification (A), 1.4.1 Use of Color (A)
- **Location:** `frontend/app/view/remotetermconfig/secretscontent.tsx:128,138-141`
- **Issue:** `isNameInvalid` only switches the input's border to `border-error`. No `aria-invalid`, no visible error text, and no `aria-describedby` linking to the format hint at line 148 (which is always-shown guidance, not an active error message).
- **Impact:** Colour-blind and low-vision users get no indication the name is invalid beyond a Save button that silently stays disabled; screen reader users get nothing at all.
- **Fix:** Add `aria-invalid={isNameInvalid}`, render an explicit error string when invalid, and `aria-describedby` it to the input.

### [High] A11Y-3: Secret-value textarea steals focus on every re-render
- **WCAG criterion:** 2.4.3 Focus Order (A)
- **Location:** `frontend/app/view/remotetermconfig/secretscontent.tsx:218-223`
- **Issue:** `ref={(ref) => { model.secretValueRef = ref; if (ref) ref.focus(); }}` is an inline arrow function. Its identity changes every render, so React detaches and reattaches the ref (calling the callback with `null` then the node) on every re-render of `SecretDetailView` — not just on mount. `secretValueAtom` is written on every keystroke and `isLoading` flips during Save/Delete, both of which re-render this component and re-invoke `.focus()`.
- **Impact:** Confirmed from source (documented React ref-identity behaviour). A user who clicks Save or Delete while the textarea isn't focused can have focus yanked back into it mid-action. The effect on caret position while already focused (e.g. mid-keystroke) is `UNVERIFIED:` — needs a rendered check — but the unconditional repeated `.focus()` call itself is a confirmed bug regardless.
- **Fix:** `useCallback` the ref, or move the initial-focus `.focus()` into a `useEffect` with an empty dependency array so it only fires on mount.

### [Medium] A11Y-4: Save/validation/quick-add errors are not announced to assistive tech
- **WCAG criterion:** 4.1.3 Status Messages (AA)
- **Location:** `remotetermconfig.tsx:280-303` (save-error and validation-error banners), `secretscontent.tsx:351,383` (`ErrorDisplay`), `connectionscontent.tsx:110` (quick-add error), `backgroundscontent.tsx:113` (add-background error)
- **Issue:** None of these error containers carry `role="alert"`, `role="status"`, or `aria-live`. They render/unrender as plain `<div>`s.
- **Impact:** A screen reader user who submits an invalid quick-add host, background, or secret name — or whose save fails — gets no notification unless they happen to navigate back over that part of the DOM.
- **Fix:** Add `role="alert"` to each container. The pattern already exists correctly once in this same file set — `secretscontent.tsx:236`'s `aria-live="polite"` reveal-announcement — extend it to the error paths.

### [Medium] A11Y-5: KeychainRow subtitle/fingerprint text fails contrast
- **WCAG criterion:** 1.4.3 Contrast (Minimum) (AA)
- **Location:** `connectionscontent.tsx:231` (`opacity-70` container), `:235` (subtitle), `:238` (fingerprint)
- **Issue:** `text-xxs text-muted` (`--color-muted: rgb(140,145,140)`) sits inside a `opacity-70` row. Computed: `text-muted` alone against `--color-panel` (`rgba(31,33,31,.5)` on `--color-background rgb(34,34,34)`) is 4.96:1 (barely passes); composited through the row's `opacity-70` it drops to **3.17:1**. This is static informative text on a non-`disabled` element, so the inactive-UI exemption used correctly elsewhere in this file does not apply here.
- **Impact:** Low-vision users cannot read the key subtitle/fingerprint text in the (concept-only) Keychain view.
- **Fix:** Drop the row-level `opacity-70` — `KeychainBanner` already states "Concept only" in text, so the extra opacity dimming is redundant — or brighten the text colour before applying it.
- **Contrast table:**

| Text | Background | Blended | Ratio | Threshold | Result |
|---|---|---|---|---|---|
| `text-muted` | `panel` (on `background`) | `#3e3e3e`→ not composited | 4.96:1 | 4.5:1 | PASS |
| `text-muted` in `opacity-70` row | `panel` | `#575858`≈`rgb(87,88,87)`(text)/`rgb(33,34,33)`(bg) | 3.17:1 | 4.5:1 | **FAIL** |

### [Medium] A11Y-6: Sub-24px interactive targets (2.5.8), Equivalent-exception ambiguity noted
- **WCAG criterion:** 2.5.8 Target Size Minimum (AA)
- **Location:** `generalcontent.tsx:1077-1093` (NumberControl spin buttons, visual 16×11px, pseudo-element-expanded hit box ≈28×21px from `before:-inset-x-1.5 before:-top-2 before:-bottom-0.5`); `widgetscontent.tsx:144-161` (WidgetOrderRow Move up/down buttons, `w-5 h-[13px]` = 20×13px, no hit-area expansion)
- **Issue:** Neither clears 24×24 on both axes even after any hit-area expansion.
- **Impact:** Motor-impaired and low-precision-pointer users may mis-tap these controls.
- **Note (explicit uncertainty):** Both may qualify for 2.5.8's "Equivalent" exception — NumberControl's adjacent input can be typed into directly; WidgetOrderRow's full row is a much-larger-than-24px drag target performing the same reorder. Flagging rather than asserting a hard fail, since the exception's applicability here is a judgement call the team should confirm.
- **Fix if not relying on the exception:** grow WidgetOrderRow's buttons to `h-6` minimum; extend NumberControl's pseudo-element further on the vertical axis.

## Unverifiable

- `UNVERIFIED:` A11Y-3's actual effect on textarea caret position when `.focus()` re-fires while already focused — needs a rendered check, not run per brief (no live-display access).
- `UNVERIFIED:` Rendered hit-target size of the `ConfigSidebar` "✕" close-menu button (`remotetermconfig.tsx:39-45`) — depends on inherited font-size, not computable from source alone.
- `UNVERIFIED:` Whether `RemoteTermConfigView` is presented with any dialog/focus-trap semantics at the Block/window level outside the audited files. No `role="dialog"` or focus-trap code exists within `frontend/app/view/remotetermconfig/*.tsx` itself; if the brief's "config modal" framing means it's wrapped in a true modal elsewhere, that wrapper needs its own check.

## Verified OK

- Accent-button contrast claim in `.kilocode/rules/rules.md:52` — recomputed, accurate (4.92:1 / 6.91:1).
- 2.5.7 Dragging Movements — widget reorder has a full keyboard alternative.
- `aria-pressed`/`aria-selected` fix in `1267d50a` — correct.
- Toggle/switch controls — correct roles, states, names, and effective hit target size.
- Reduced-motion — handled globally.
- `backgroundscontent.tsx` label association (Opacity/Blend mode) — correct, contrasts with the broken pattern in `secretscontent.tsx`.
- Disabled-button opacity contrast exemptions — all genuinely `disabled`, correctly not flagged.

## Completion
**Status:** COMPLETE
**Findings:** 1 Critical, 2 High, 3 Medium, 0 Low
**Could not check:** rendered-only items listed under Unverifiable above (no live-display access permitted by brief).
