# Cohesive theme system: terminal, editor and window chrome follow appearance mode - design spec

Status: approved by owner 2026-09-25 (scope, palette strategy, verification
approach, wiring approach and all three design sections), ready for
implementation planning.

Predecessor: `.pi/specs/2026-09-24-appearance-mode-design.md` (merged as
`b6a1cae4`). That spec named terminal palettes as "the second spec, built
on top of this spec's resolved-mode primitive". This is that spec, widened
to the other surfaces the first arc left dark-only.

## Intent

With `window:appearancemode: light`, the app's own chrome renders light but
three surfaces stay dark-tuned: terminal blocks paint a light-grey
foreground over the now-white background (illegible, and the bug the owner
reported), Monaco editors stay on `wave-theme-dark`, and the native
window-control symbols keep the colour chosen for a dark titlebar. Each
subsystem has its own theming mechanism and none of them read the resolved
appearance mode that the first arc built.

Goal: every themed surface derives from the one existing source of truth,
`getResolvedAppearanceModeAtom(tabId)` (per-tab override -> global setting
-> OS preference -> `dark`), so that a global setting, a tab override, or a
live OS flip repaints all of them together and without a reload.

Owner decisions captured during brainstorming:

- Scope: all three real gaps (terminal, Monaco, native chrome) in one spec.
- Terminal palettes: one new `default-light` palette. When no `term:theme`
  is set, the terminal follows the appearance mode. A named palette chosen
  anywhere in the config chain is honoured as-is in both modes.
- Wiring: point subscriptions. Each subsystem reads the resolved-mode atom
  directly, the same way `AppThemeUpdater` already does. No central
  dispatcher, no DOM-attribute observers.
- Verification: Xvfb screenshot evidence plus an owner-run checklist.

## Findings that changed the brief

Verified during exploration, recorded so the plan does not re-derive them:

- The tabbar `OverlayScrollbars` thumb is not broken. `app.scss` is
  imported after `overlayscrollbars.css` at equal specificity, so its
  `.os-scrollbar { --os-handle-bg: var(--scrollbar-thumb-color) }` wins,
  and `--scrollbar-thumb-color` already has a light override at
  `frontend/app/theme.scss:183`. The hardcoded `os-theme-dark` string in
  `tabbar.tsx` only selects the library's size/radius preset. Out of scope.
- Native chrome already has a runtime update path. `frontend/app/app-bg.tsx`
  samples the rendered titlebar strip (`capturePage` on the main side,
  `emain/emain-ipc.ts:364`) and calls `setTitleBarOverlay` with
  `symbolColor` black or white by luminance. It never re-runs on an
  appearance-mode change because `getAvgColor`'s dependency array is
  `[bgRef, style]`. The fix is a missing dependency, not a new IPC channel.
- `emain/emain.ts:69`'s `nativeTheme.themeSource = "system"` is not the
  root cause and must stay. `nativeTheme.shouldUseDarkColors` is the input
  to `osPrefersDarkAtom`; pinning `themeSource` to the resolved mode would
  feed the resolution chain's output back into its own input.
- `pkg/rtconfig/defaultconfig/termthemes.json` has seven palettes, all
  dark-tuned, not one.
- Each tab is its own `WebContentsView` (`emain/emain-tabview.ts:119`), so
  page-global calls such as `monaco.editor.setTheme` are already per-tab.

## Explicitly out of scope

- Light siblings for the six named palettes (`onedarkpro`, `dracula`,
  `monokai`, `campbell`, `warmyellow`, `rosepine`) and any
  `display:lightvariant` pairing mechanism. Declined by owner.
- Tabbar scrollbar (verified correct, above).
- Cold-start flash before first paint (`winOpts.backgroundColor =
  "#222222"` at window creation, `data-theme` set from a `useEffect`).
  Carried unchanged from the predecessor spec's accepted limitation.
- Any change to `nativeTheme.themeSource`.
- Relabelling the Themes submenu's "Default" entry.

## Architecture

### Shared primitive

`frontend/app/store/appearance-atoms.ts` gains one export:

```ts
export const resolvedAppearanceModeAtom: Atom<"light" | "dark">
```

derived as `getResolvedAppearanceModeAtom(get(atoms.staticTabId))`.
`atoms.staticTabId` (`frontend/app/store/global-atoms.ts:66`) is fixed per
WebContentsView, so this is "the resolved mode for this window's tab".
`AppThemeUpdater` is unchanged. New consumers read the convenience atom so
none of them threads a tabId.

### Terminal palette

**Palette.** `pkg/rtconfig/defaultconfig/termthemes.json` gains
`default-light` (`display:name` "Default Light", `display:order` 2; the
other six renumber 3 to 8 so the Themes submenu sort stays stable).
Proposed values, to be reviewed by the owner against the rendered app:

- `background` `#ffffff` and `foreground` `#1a1a1a`, matching
  `theme.scss`'s light `--main-bg-color` and `--main-text-color`.
- `black`, `brightBlack`, `gray`: mid greys legible on white.
- The six ANSI hues and their bright variants keep `default-dark`'s hue
  family with lightness reduced until each reaches 4.5:1 on white.
- `cmdtext` `#1a1a1a`; `cursor` and `selectionBackground` empty, as in
  `default-dark`.

`frontend/preview/mock/defaultconfig.ts` imports the JSON directly and
needs no edit. User-defined `termthemes.json` palettes are untouched.

**Selection.** In `frontend/app/view/term/termutil.ts`:

- `DefaultTermThemeLight = "default-light"` beside the existing
  `DefaultTermTheme`.
- `getDefaultTermThemeName(mode: "light" | "dark"): string`.
- `computeTheme(fullConfig, themeName, termTransparency, fallbackThemeName)`
  uses the supplied fallback when the named theme is missing, instead of
  always reaching for `default-dark`. Callers pass the mode-appropriate
  default.

In `frontend/app/view/term/term-model.ts` (currently line 233),
`termThemeNameAtom` resolves to the explicit `term:theme` from
`getOverrideConfigAtom` (block meta -> connection -> `settings.json`) when
set, else `getDefaultTermThemeName(get(resolvedAppearanceModeAtom))`. The
mapping is extracted as a pure `resolveTermThemeName(override, mode)` so
it can be unit-tested without a block (`getOverrideConfigAtom` returns
`NullAtom` in the preview window).

Every consumer already reads `termThemeNameAtom`: `blockBg` (line 244),
`TermThemeUpdater` (`termtheme.ts`, which pushes changes into
`terminal.options.theme` live), the xterm constructor in `term.tsx:273`,
and `getSettingsMenuItems`. No consumer changes. The Themes submenu's
existing "Default" entry (writes `null`) now means "follow appearance
mode"; no UI change.

### Monaco editor

`frontend/app/monaco/monaco-env.ts`:

- New pure helper `monacoThemeForMode(mode): "wave-theme-light" |
  "wave-theme-dark"`.
- `loadMonaco()` replaces the literal `setTheme("wave-theme-dark")` (line
  71) with `setTheme(monacoThemeForMode(globalStore.get(resolvedAppearanceModeAtom)))`.
- `wave-theme-light`'s `editor.background` changes from `#fefefe` to
  `#00000000` so it composites over the app background the way
  `wave-theme-dark` does. Owner reviews the rendered result.

`frontend/app/monaco/monaco-react.tsx` gains `useMonacoAppearanceTheme()`:
reads `resolvedAppearanceModeAtom`, effect calls
`monaco.editor.setTheme(monacoThemeForMode(mode))` on change. Called from
both mount points, `MonacoCodeEditor` and the diff editor, so every open
editor in the tab repaints live.

### Native window chrome

`frontend/app/app-bg.tsx`: `AppBackground` reads
`resolvedAppearanceModeAtom` and adds it to `getAvgColor`'s dependency
array. The existing 30ms debounce and `useLayoutEffect` ordering mean the
titlebar sample runs after the light tokens have painted, and the existing
main-side handler sets `symbolColor` to black on a light strip.

Coverage: Linux with `titleBarStyle: "hidden"` (overlay symbols) and
Windows (overlay `color` and `symbolColor`). macOS uses native traffic
lights with no overlay; Linux with `window:nativetitlebar: true` uses the
window manager's decorations. Neither needs anything.

## Error handling

Nothing new throws, matching the predecessor spec's convention of degrading
to the current default rather than surfacing an error for a cosmetic
signal:

- A missing `default-light` entry falls through `computeTheme()`'s existing
  missing-theme path to the supplied fallback, then to `{}` as today.
- `monaco.editor.setTheme` with an unknown name is a no-op in Monaco.
- The overlay resample already sits inside a try/catch on the main side.

## Testing

Unit tests (vitest, beside the existing `appearance-atoms.test.ts`,
`appearance-atoms.ipc.test.ts` and `appearance-theme-updater.test.tsx`):

- `termutil.test.ts`: `getDefaultTermThemeName` for both modes;
  `computeTheme` uses the supplied fallback when the requested theme is
  missing and still strips `background` to transparent.
- `resolveTermThemeName`: explicit override wins in both modes; null
  override follows mode.
- `monaco-env.test.ts`: `monacoThemeForMode` mapping; `useMonacoAppearanceTheme`
  calls `setTheme` again when the atom flips (Monaco mocked, in the style
  of the existing IPC test).
- `app-bg`: `updateWindowControlsOverlay` is called again after the
  resolved-mode atom changes (preload API via the existing preview mock).
- Palette contrast: every `default-light` foreground and ANSI value reaches
  4.5:1 against its background (colord is already a dependency), so a
  future palette edit that breaks legibility fails CI.

Live verification, one plan task: run the built app under a nested display
and capture screenshots of a terminal block, a code-editor block and the
titlebar strip, in global light, global dark, and a tab override that
differs from global. The task's brief carries the exact launch sequence
(`unset DISPLAY`; explicit `Xvfb :99 -screen 0 ...`; app launched with
`DISPLAY=:99` and a scratch `XDG_CONFIG_HOME`) and requires the executor to
prove isolation by reading the app process's `DISPLAY` from
`/proc/<pid>/environ`, not by asserting it. Screenshots go to the plan's
evidence directory.

A final checklist task hands the owner the same three scenarios to sign off
on their own session. The plan ends at "unit tests green, screenshots
captured, checklist handed over".

## Documentation

`docs/docs/config.mdx`:

- `term:theme` row (line 56): "unset follows the appearance mode
  (`default-dark` / `default-light`)" in place of "default is default-dark".
- Terminal-themes section (lines 235-283): list `default-light` among the
  built-ins; state that a named `term:theme` is honoured in both modes.
- `window:appearancemode` row (line 88): one sentence naming what it drives:
  app chrome, terminal default palette, code editor, window-control symbols.

## Open items carried into the implementation plan

- Exact `default-light` hex values. The spec fixes the constraints
  (token-matched background/foreground, 4.5:1 for every colour); the plan
  task computes and records the values, and the contrast test enforces them.
- Whether `computeTheme`'s new fallback parameter is a required positional
  argument or defaults to `DefaultTermTheme`. Required is preferred so no
  caller silently keeps the dark fallback in light mode; the plan confirms
  all three call sites.
- Implementation happens in a fresh worktree, not `remoteterm-daily`, which
  has a live `task dev` (see the predecessor plan's global constraints).
