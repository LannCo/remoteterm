# round2-a11y-auditor report
**Target:** Round-2 WCAG 2.2 AA verification of `frontend/app/view/remotetermconfig/*.tsx` fixes, `git diff 35cd6146 75b60911`, HEAD `75b60911`
**Started:** 2026-09-22T00:00:00Z
**Status:** COMPLETE

## Fix verification

### A11Y-1 (label association) — FIXED
`secretscontent.tsx:129-131,137,140-141,161,164-165` (add form), `:214,238,242` (detail view): `useId()` + matching `htmlFor`/`id` pairs on all three fields. Verified with `npx vitest run` — `remotetermconfig-a11y.test.tsx` passes for both. `useId()` scopes ids per component-tree position, and `AddSecretForm`/`SecretDetailView`/placeholder are mutually exclusive in `renderRightPane` (`:379-399`), so no duplicate-id risk across renders. Also closes the revealed-value edge case the round-1 refutation narrowed (empty `placeholder` after Reveal): the label now supplies the name regardless of placeholder state.

### A11Y-2 (aria-invalid + linked hint) — FIXED, with a new regression (see R2-A11Y-1)
`secretscontent.tsx:144-145,155-158`: `aria-invalid={isNameInvalid || undefined}` (correctly omits the attribute rather than emitting `"false"`), `aria-describedby={nameHintId}` set unconditionally, and `nameHintId`'s div is unconditionally rendered — so the description never dangles. On invalid input the div's text becomes `"Invalid name. Must start with a letter..."`, read via `aria-describedby` even though the disabled Submit button drops out of tab order, closing the gap `refute-fe-reach` flagged in round 1.

### A11Y-3 (focus once per mount) — FIXED
`secretscontent.tsx:217-223`: `useCallback(..., [model])` gives the ref a stable identity across re-renders of the same mounted instance, so React only invokes it (and calls `.focus()`) on mount/unmount, not on every `isLoading`/`secretShownAtom` change. This directly fixes the reveal-in-flight focus-yank scenario `refute-fe-repro` reproduced (5 `.focus()` calls -> 1). `SecretDetailView` is keyed by `selectedSecret` (`:395`), so switching between secrets unmounts/remounts and correctly refocuses the newly-opened secret's textarea once — verified by reading, matches the existing test's mount-based expectation.

### A11Y-4 (role="alert" on banners) — PARTIALLY FIXED (see R2-A11Y-2)
`backgroundscontent.tsx:111-116`, `connectionscontent.tsx:108-113`, `remotetermconfig.tsx:282,294,333`: all conditionally rendered (`error &&`, `errorMessage &&`, `configErrors?.length > 0`), so the alert element only enters the DOM when there's something to announce — no announce-on-load and no re-announce-per-render, since `role="alert"` only fires on insertion/text mutation of an already-present node. Confirmed by test (`remotetermconfig-a11y.test.tsx`, both quick-add cases pass).

`secretscontent.tsx`'s `ErrorDisplay` (`:14-25`), used for `errorMessage` (`:371,403`, e.g. failed save/delete) and `storageBackendError` (`:355`), was **not** touched — same gap round-1's A11Y-4 and both refutation passes named at this exact location.

### A11Y-5 (keychain row opacity moved to icons) — FIXED
`connectionscontent.tsx`: row `opacity-70` removed from the container; `opacity-70` now applies only to the two `aria-hidden` `<i>` icons. Subtitle/fingerprint text is back to `text-muted` on `panel` with no compositing penalty — round-1 computed this at 4.96:1 (passing). Test confirms zero `opacity-` classes on the two rendered rows.

### A11Y-6 (widget move buttons 24x24) — FIXED
`widgetscontent.tsx:143-162`: both buttons now `w-6 h-6` (24x24, exact minimum), meeting 2.5.8 directly without needing the spacing/equivalent-control exception debated in round 1. Container changed from `flex-col` to `flex` (side-by-side). Distinct accessible names (`Move {label} up` / `Move {label} down`), logical left-to-right tab order matching visual layout, zero gap between the two 24x24 targets is not a problem since each already meets the size minimum on its own (the 24px-circle spacing rule only applies to sub-24px targets).

### CA-2 (NumberControl save on blur/Enter) — no new a11y regression found
`generalcontent.tsx:1053-1064,1066-1136`: `parseNumberInput` centralizes clamping; `commit()` fires on blur or Enter (which blurs). No `aria-live`/status announcement on commit, but this matches the pre-existing, unflagged `SliderControl` pattern (`:1145-1156`) in the same file — not a new deviation introduced by this fix, so not filed as a new finding.

## New findings

### [Medium] R2-A11Y-1: New "Invalid name." error text fails 1.4.3 contrast
- **WCAG criterion:** 1.4.3 Contrast (Minimum) (AA)
- **Location:** `frontend/app/view/remotetermconfig/secretscontent.tsx:155` — `className={cn("text-caption", isNameInvalid ? "text-error" : "text-muted")}`
- **Issue:** `--color-error: rgb(229, 77, 46)` (`frontend/tailwindsetup.css:26`) on `--color-background: rgb(34, 34, 34)` (`:9`) computed via WCAG relative luminance = **4.12:1**. `--text-caption: 11px` (`:48`) is normal-size text, threshold 4.5:1. This is the exact text A11Y-2 added to satisfy 3.3.1 Error Identification — the fix for one AA criterion introduces a fresh failure of another.
- **Impact:** Low-vision users get a validation message that is itself hard to read; the same `text-xs text-error` pattern (12px, same color) is reused for the connections/backgrounds quick-add `role="alert"` error text (`connectionscontent.tsx:111`, `backgroundscontent.tsx:114`), so the fix for A11Y-4 also renders through this failing color.
- **Fix:** Brighten the error token for body text use (e.g. a `text-error` variant closer to `--color-accent-300`-equivalent luminance) or reserve `--color-error` for backgrounds/icons/borders and pair error text with `text-primary` (`#f7f7f7`) the way `ErrorDisplay`'s "error" variant already does at `secretscontent.tsx:22`. Verified: `text-primary` (#f7f7f7) on `rgb(34,34,34)` computes 17.8:1, well clear of 4.5:1.
- **Contrast table:**

| Text | Background | Blended | Ratio | Threshold | Result |
|---|---|---|---|---|---|
| `text-error` (#E54D2E) | `background` rgb(34,34,34) | n/a (opaque) | 4.12:1 | 4.5:1 | **FAIL** |
| `text-primary` (#f7f7f7) | `background` rgb(34,34,34) | n/a (opaque) | 17.80:1 | 4.5:1 | PASS |

### [Medium] R2-A11Y-2: Secrets save/delete errors still not announced to assistive tech
- **WCAG criterion:** 4.1.3 Status Messages (AA)
- **Location:** `frontend/app/view/remotetermconfig/secretscontent.tsx:14-25` (`ErrorDisplay`), rendered at `:355` (storage backend error), `:371` and `:403` (`errorMessage`, e.g. failed save/delete/load)
- **Issue:** No `role="alert"`, `role="status"`, or `aria-live` on the component or its call sites. This is the same location round-1's A11Y-4 and both refutation passes (`refute-fe-reach`, `refute-fe-repro`) named explicitly; the round-2 fix covered `backgroundscontent.tsx`, `connectionscontent.tsx`, and `remotetermconfig.tsx` but left this file's error path untouched.
- **Impact:** A screen reader user who fails to save or delete a secret (the exact failure mode A11Y-4 was written to fix) gets no notification. Materially worse here than in the other three fixed locations because a secrets manager save failure means the user's edit was silently discarded.
- **Fix:** Add `role="alert"` to `ErrorDisplay`'s wrapping `div` (`:20`), matching the pattern now used in `backgroundscontent.tsx:114` and `connectionscontent.tsx:111`.

## Unverifiable

- `UNVERIFIED:` Rendered screen-reader announcement timing/ordering when a `role="alert"` node and an `aria-live="polite"` node (the reveal announcer, `secretscontent.tsx:256`) both change in the same commit — no live-display access permitted by brief.
- `UNVERIFIED:` Whether the `text-caption`/`text-xs` computed sizes render at the CSS px values assumed here under the user's OS/browser zoom — source-only, no rendered check.

## Verified Conformant
- A11Y-1, A11Y-3, A11Y-5, A11Y-6: fully fixed, no regressions, confirmed both by reading and by the new `remotetermconfig-a11y.test.tsx` suite (20/20 passing).
- A11Y-2: the aria-wiring itself is correct and closes the round-1 tab-order gap; contrast of the new text is a separate, newly-introduced failure (R2-A11Y-1).
- A11Y-4: correctly fixed for `backgroundscontent.tsx`, `connectionscontent.tsx`, `remotetermconfig.tsx`; gap remains in `secretscontent.tsx` (R2-A11Y-2).
- Widget move buttons: side-by-side 24x24 layout has no focus-order or naming issues.

## Completion
**Status:** COMPLETE
**Fix verification:** 5 of 6 targeted fixes fully resolved (A11Y-1, A11Y-3, A11Y-5, A11Y-6, plus the aria-wiring half of A11Y-2); 1 partially resolved (A11Y-4).
**New findings:** 2 Medium (R2-A11Y-1 contrast regression, R2-A11Y-2 remaining secrets-error live-region gap)
**Could not check:** rendered-only items listed under Unverifiable (no live-display access permitted by brief)
