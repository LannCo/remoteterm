# a11y-auditor report (round 4)
**Target:** `git diff 816fb3d8 cd464aff -- frontend/ emain/` at `cd464aff`, `qa/fleet-2026-09-22`
**Started:** 2026-09-22T15:40:00Z
**Status:** COMPLETE

## Findings

### [Medium] Source-control revert-error dismiss button fails 2.5.8 Target Size (Minimum)
- **WCAG criterion:** 2.5.8 Target Size (Minimum) (Level AA)
- **Location:** `frontend/app/view/sourcecontrol/action-error.tsx:23-30`
- **Issue:** `<button aria-label="Dismiss error" className="shrink-0 cursor-pointer text-red-400 hover:text-white">` wraps only a `fa-solid fa-times` icon. Confirmed via `getComputedStyle` in a scratch happy-dom render: no padding, width, or height on the button — its class list carries no sizing utility at all, so the clickable box is exactly the ~10-12px glyph rendered at the banner's `text-xs`. No spacing/inline/equivalent exception applies (it's a standalone icon button, not inline text, and no larger equivalent control exists). Sibling dismiss buttons elsewhere in this codebase (`remotetermconfig.tsx:283-289`, `:295-301`) use `p-1` for a larger hit area — this is a smaller regression relative to the established in-repo pattern, not just a generic gap.
- **Impact:** Motor-impairment and low-precision-pointer (tremor, touch) users can miss the target when trying to dismiss the banner.
- **Fix:** Add hit-area padding without changing the visual icon size, matching the sibling pattern, e.g. `className="shrink-0 cursor-pointer text-red-400 hover:text-white p-1.5 -m-1.5 flex items-center justify-center"` (24x24 effective box around the ~11px glyph).

### [Medium] Focus lost to `<body>` when the revert-error banner is dismissed
- **WCAG criterion:** 2.4.3 Focus Order (Level A, required for AA conformance) — focus management on removal of the focused element
- **Location:** `frontend/app/view/sourcecontrol/action-error.tsx:14-16` (`if (!error) return null`), triggered from `sourcecontrol-model.ts` `dismissActionError()`
- **Issue:** Confirmed with an interaction test (happy-dom + user-event, scratch worktree): focusing the "Dismiss error" button then clicking it removes the whole `ActionErrorBanner` from the DOM (the component returns `null` once `error` is cleared), and `document.activeElement` lands on `<body>`. Keyboard/screen-reader users lose their position in the document; the next Tab restarts from the top of the page instead of continuing from where the banner was.
- **Impact:** Keyboard-only and screen-reader users are disoriented after dismissing the error — no announcement of where focus went, and no logical continuation point.
- **Fix:** In `dismissActionError()` (or the `onDismiss` handler in `sourcecontrol.tsx`), move focus to a sensible anchor before/when the banner unmounts — e.g. the source-control panel's existing focus target, or nearest still-mounted heading/toolbar control — the same way this repo already re-focuses on legitimate remounts elsewhere (per `[[remoteterm_a11y_fix_patterns]]`, "Focus-once-per-mount" pattern).

## Verified OK

- **NumberControl spin-button `onMouseDown` preventDefault** (`generalcontent.tsx:1159,1168`): only suppresses the *mouse* focus-steal; keyboard activation (Tab to button, Enter/Space) is unaffected since `mousedown` never fires for keyboard input. Confirmed via `generalcontent-numbercontrol.test.tsx:97-109` ("keyboard: typed draft, Tab to the increase button, Enter sends one write"), which passes with `npx vitest run generalcontent-numbercontrol.test.tsx` (8/8 pass). Spin buttons remain reachable and operable by keyboard exactly as before; no new keyboard trap.
- **Failed write reverts and is announced:** `NumberControl.write()` (`generalcontent.tsx:1092-1104`) reverts the optimistic `local`/`pending` state on a rejected promise, and the same rejection already flows through `setGeneralSetting()` (`remotetermconfig-model.ts:700-702`) into the shared `errorMessageAtom`, rendered as `<span role="alert">` at `remotetermconfig.tsx:115,282` (the safe insert-only-when-truthy pattern verified in rounds 1-3, per `[[remoteterm_a11y_fix_patterns]]`). `generalcontent-numbercontrol.test.tsx:173-180` confirms the value-revert behaviour; the announcement itself reuses an already-verified live region, not new plumbing. Minor pre-existing gap, unchanged by this diff and not a regression: the alert text ("Failed to save setting: …") doesn't name which field failed when several are visible at once — not blocking.
- **Pending/in-flight write has no `aria-busy`/status indication** (`generalcontent.tsx` `NumberControl`): confirmed no live-region or `aria-busy` marks the optimistic-write window. Not a violation — WCAG 4.1.3 Status Messages is satisfied by the failure being announced when it matters, and the optimistic value update itself is not "important" status content — but worth a low-cost improvement: `aria-busy={pending != null}` on the input would let AT users know a write is outstanding if they tab back mid-flight. Info-level recommendation, not a finding.
- **Source-control revert-error banner content and announce-once pattern** (`action-error.tsx:12-33`): `role="alert"` only exists in the DOM once `error` is truthy (conditional `return null` otherwise) — same insert-on-mount pattern verified safe in rounds 1-3. `sourcecontrol-revert-error.test.tsx` confirms it fires with the expected text and is dismissible. Contrast computed: `text-red-400` (`#f87171`) on `bg-red-500/20` (`#ef4444` at 20%) alpha-composited over this view's effective panel background (`rgb(32.5,33.5,32.5)`, per `[[remoteterm_color_tokens]]`'s blend method) → blended `rgb(74,40,40)` ≈ `#4a2828`, computed ratio **4.66:1** — passes the 4.5:1 normal-text threshold (text is `text-xs`, well under the large-text cutoff) with a small margin.
- **Onboarding logo swap** (`onboarding-command.tsx:7,111-112`): `alt={name}` renders `alt="remoteterm-logo.png"` (was `alt="wave-logo.png"` before) via `onboarding-layout.tsx:61`. This is filename-as-alt-text, not a description — but it is the pre-existing pattern (unchanged by this diff, not newly introduced or newly broken), and this fake onboarding block is a mock "file view" demonstrating the filename itself, so the alt arguably matches its didactic purpose. Not re-flagging as this round's finding since no regression; noting because the brief asked about it explicitly.

## Contrast table

| Text        | Background                          | Blended  | Ratio  | Threshold | Result |
|-------------|--------------------------------------|----------|--------|-----------|--------|
| red-400     | red-500/20 over panel bg (~#202120)  | #4a2828  | 4.66:1 | 4.5:1     | PASS   |

## Completion
**Status:** COMPLETE
**Findings by severity:** Medium 2, Low 0, Info 0 (2 non-blocking Info/Low notes recorded under Verified OK, not counted as findings)
**Not checked:** live/rendered behaviour in the actual app (no app launch permitted); native Electron dialog wording in `emain-platform.ts` (OS chrome, outside WCAG's web-content scope, unchanged pattern from round 3 — still "Quit" as both default and cancel button).
