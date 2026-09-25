# Refutation pass: frontend claims, reachability and trigger lens

Commit `93aaefc3`. Read-only static trace; nothing was run.

## CA-1

VERDICT: HOLDS (severity: medium; timing-dependent)

Trigger (widgets):
1. Settings > Widgets. Click the hidden switch on widget X: `widgetscontent.tsx:239-244` sets `pendingHidden` and calls `toggleWidgetHidden`, which snapshots `widgetsMapAtom` (`remotetermconfig-model.ts:835`) and enqueues `{X: {...X, hidden: true}}`.
2. Before the config watcher re-broadcasts (`pkg/rtconfig/filewatcher.go` has no debounce, but the window spans FileInfo + FileRead + FileWrite RPCs, fsnotify, reparse and WPS push), click X's "Move up" button (`widgetscontent.tsx:248-263`) or finish a drag (`:226-235`).
3. `reorderWidget` reads the same stale `widgetsMapAtom` (`:811`, derived from `fullConfigAtom` at `:228-231`, only updated by the push) and enqueues `{X: {...X(hidden:false), order: n}}` (`:831`).
4. The queue (`:775-779`) serialises the writes, but `writeWidgetPatch` merges whole keys (`:790`), so write 2 restores `display:hidden: false`. The toggle is reverted on disk; `pendingHidden` clears once the push confirms `false` (`:198-214`), so the switch flips back.

A simpler trigger: double-click the switch. Both clicks read `hidden=false` (`:241`, `:836`), both write `true`; the user's second toggle is lost.

Backgrounds: release the opacity slider (`backgroundscontent.tsx:155` commits via `onMouseUp`), then change Blend mode (`:165`) before the push. `updateBackgroundBlendMode` spreads the stale entry (`remotetermconfig-model.ts:931-934`), so the opacity reverts. Same mechanism.

The queue comment (`:765-770`) claims the race is prevented. It only prevents it for *different* keys (the test at `remotetermconfig-model.widgets.test.ts:242-250` uses two different widgets).

## CA-2

VERDICT: HOLDS (severity: low-medium)

Trigger: General > "Max tab cache size" (`generalcontent.tsx:306-315`, schema `min: 1`). Select the text and press Backspace. `e.target.value === ""`, so `Number("") === 0` and `Number.isFinite(0)` is true (`:1069-1071`). `onChange={write}` (`:1269`) calls `setGeneralSetting({"window:maxtabcachesize": 0})` (`:1156`), which goes to `SetConfigCommand` and then `rtconfig.SetBaseConfigValue` (`wshserver.go:547-548`) with no range check (`schema/settings.json:232` is only `"type": "integer"`). The watcher push reaches `emain.ts:315-316` (the `!= null` check passes for 0), then `setMaxTabCacheSize(0)` (`emain-tabview.ts:238-241`). After that, `checkAndEvictCache` (`:276-290`) evicts every non-active tab view that has been idle for more than 1 s, so every tab switch rebuilds its webContents.

The `min`/`max` attributes (`:1064-1065`) only constrain the spin buttons (`bump`, `:1052-1057`), not typed input. Typing `99` saves 99 (above max 50), and typing `-` gives `value === ""`, which saves 0. Every keystroke saves: typing "25" writes 2, then 25. The same path applies to every `control: "number"` field (for example, font sizes with `min: 6` can be set to 0).

Severity: performance/UX degradation, no data loss. It can be recovered by retyping the value.

## CA-11

VERDICT: HOLDS (severity: high; silent partial revert of user changes)

Trigger: open Source Control > Review mode, then hover a modified tracked file header and click "Revert" (`file-diff-section.tsx:216-223`), or focus the header and press `r` (`:188-191`). There is no confirmation. The call chain is `review-mode.tsx:59-61`, then `revertFileFromReview` (`sourcecontrol-model.ts:627-640`), which reads `hunkCount` once from the cached diff and loops `revertHunk(path, i)` for `i = 0..N-1`.

Each `GitRevertHunkCommand` re-runs `git diff` and indexes into the *current* hunks (`pkg/wshrpc/wshremote/git.go:321-341`). For N=4: i=0 reverts h0, leaving [h1,h2,h3]. i=1 reverts h2, leaving [h1,h3]. i=2 and i=3 are out of range (`:336-338`). The error is swallowed by `revertHunk`'s catch (`sourcecontrol-model.ts:395-397`, console only), and h1 and h3 survive. In general, floor(N/2) hunks survive with no user-visible error. The test (`review-mode.test.ts:505-531`) mocks `revertHunk` and asserts indices 0 and 1, so it encodes the bug.

Adjacent, outside my lens: for `staged=true`, the server applies the inverse of the `--cached` hunk to the working tree via `applyPatch` rather than `applyPatchCached` (`git.go:341`, `:1133-1154`).

## CA-12

VERDICT: HOLDS (severity: low)

Trigger: `backgrounds.json` contains malformed JSON (edited externally), or `FileWriteCommand` fails (read-only config dir). Go to Backgrounds > "New background", fill in the name and CSS, then press Enter or click Add (`backgroundscontent.tsx:96,102`). In `submitBackgroundAdd` (`remotetermconfig-model.ts:979-996`), `addBackground` calls `persistBackgroundPatch`, and `writeBackgroundPatch` either returns early on `rawContent == null` (`:893-896`) or catches the write error (`:910-912`). Neither throws, so `addBackground` returns `key` (`:962`). The form closes and the typed input is discarded (`:992`, `:972-977`). `applyBackgroundToTab(key)` sets `{"bg:*": true, "tab:background": "bg@<slug>"}` (`:915-921`).

Result: `app-bg.tsx:24-26` finds no config entry, falls back to tab meta whose `bg:*` was just cleared, and the user's existing tab background disappears. No tile shows as active. The top banner does show the error (`remotetermconfig.tsx:280-290`), so the failure is not silent.

## A11Y-1

VERDICT: HOLDS for 1.3.1 and 2.5.3; partially REFUTED for "no accessible name"

None of the `<label>`s at `secretscontent.tsx:134`, `:152` and `:216` has `htmlFor` or wraps its control, and no `aria-label` or `aria-labelledby` exists. However, the HTML-AAM accname computation falls back to `placeholder`, which Chromium implements:
- Add form, Name input (`:135-146`): NVDA reads "edit, MY_SECRET_NAME". A name exists, but it is not the visible label "Name", which fails 2.5.3 Label in Name and 1.3.1.
- Add form, Value textarea (`:153-160`): reads "Enter secret value...".
- Detail Value textarea (`:217-235`): reads "Enter new secret value..." until the user clicks Reveal. After that, `placeholder=""` (`:234`) and focus is moved here (see A11Y-3), so NVDA reads "edit, multi line" plus the secret text with **no name**. A 4.1.2 failure is reachable only in the revealed state.

## A11Y-2

VERDICT: HOLDS (severity: low-medium)

Trigger: Secrets > Add, then type `1abc` into Name. `isNameInvalid` (`secretscontent.tsx:128`) changes only the border colour (`:138-141`). The input gets no `aria-invalid` or `aria-describedby`. The hint (`:147-149`) is static text, always visible and not linked to the input. The only other signal is that "Add Secret" becomes `disabled` (`:174`), which drops it from the Tab order, so a screen reader user tabbing Name, Value, Cancel never learns why there is no Add button. This fails 3.3.1 (the error is not identified in text) and 1.4.1 for sighted users (the red border is the only change on the field).

## A11Y-3

VERDICT: HOLDS, but the "during typing" part is REFUTED (severity: low-medium)

In React 19, an inline ref callback is a new function on every render, so it detaches and reattaches on every commit of `SecretDetailView`, and `ref.focus()` (`:218-223`) runs each time. The component is `memo` with a stable `model` prop, so it re-renders only when `selectedSecretAtom`, `secretValueAtom`, `secretShownAtom` or `isLoadingAtom` changes (`:197-200`).
- Typing: `secretValueAtom` changes, but the textarea is already focused, so `focus()` is a no-op. No yank.
- Save or Delete **failure**: focus is on the button, then `isLoading` goes true, so the button is disabled and focus is lost. When `isLoading` goes false (`remotetermconfig-model.ts:576-577`, `:597-598`), the ref reattaches after the DOM `disabled` is removed and focus jumps to the textarea instead of the error. A success closes the view (`:573`, `:593`), so no yank.
- Real 2.4.3 trigger: click Reveal (`secretscontent.tsx:247`, `showSecret` `:535-557`). While `GetSecretsCommand` is pending (an OS keychain unlock prompt makes this take seconds), the user clicks into a terminal block or another setting. On resolve, `secretValueAtom`, `secretShownAtom` and `isLoadingAtom` commit, and the textarea steals focus from wherever the user is, including another block.

## A11Y-4

VERDICT: HOLDS for 3 of 4 files; REFUTED for `generalcontent.tsx`

No `role="alert"`, `role="status"` or `aria-live` exists on:
- `remotetermconfig.tsx:280-301`: the save/validation banners. Trigger: the CA-12 path or any failed `setGeneralSetting` (`remotetermconfig-model.ts:694`). Focus stays on the control, and NVDA announces nothing.
- `backgroundscontent.tsx:113`: in the Add background form, press Enter on the CSS field with the name empty. "Name cannot be empty" renders silently.
- `connectionscontent.tsx:110`: the quick-add error, same pattern.

`generalcontent.tsx` renders no error banner at all (grep for "error" finds nothing). Its errors go through the `remotetermconfig.tsx` banner, so citing it is a misattribution. The only live region nearby is the reveal announcer at `secretscontent.tsx:236`. `ErrorDisplay` (`secretscontent.tsx:14-25`) also has no live semantics.

## A11Y-5

VERDICT: HOLDS (severity: low)

Trigger: Connections, then click the "Keychain" toggle (`connectionscontent.tsx:36-41`, rendered at `:306`). `KeychainRow` (`:229-243`) applies `opacity-70` to the whole row. `text-muted` is rgb(140,145,140) (`tailwindsetup.css:15`) and `panel` is rgba(31,33,31,.5) (`:29`). Composited over `background` #222 (`:9`), the ratio is **3.17:1** (recomputed), against 5.0:1 without the opacity. This affects the subtitle (`:234`) and fingerprint (`:237`) at `text-xxs`. The rows are non-interactive divs, so the "inactive UI component" exemption does not apply. They are placeholder concept content ("Concept only", `:212`), which lowers the practical impact. The result also assumes an opaque #222 ancestor; a tab background changes it.

## A11Y-6

VERDICT: HOLDS for WidgetOrderRow; NumberControl measured larger than claimed and likely exempt

- WidgetOrderRow move buttons (`widgetscontent.tsx:146-160`): `w-5 h-[13px]` gives 20x13 with no hit-area extension. Up and Down are stacked with no gap, so the 24 px spacing circles intersect. The equivalent-control exception does not apply: the only other way to reorder is dragging, and these buttons *are* the 2.5.7 single-pointer alternative for it. Fails 2.5.8.
- NumberControl spin buttons (`generalcontent.tsx:1077-1092`): the visible size is 16x11, but the `before:` pseudo-element extends the hit area to about 28x21, and Up and Down overlap by about 3 px. This is still under 24 in height. The function (set the value) is also available by typing in the adjacent `<input type="number">` (`:1061-1074`), which is keyboard-operable. The equivalent exception plausibly applies, although the input's own height (about 16-18 px) is marginal.
