# App-wide light/dark appearance mode — design spec

Status: approved by owner 2026-09-24, ready for implementation planning.

## Intent

Owner wants a real light/dark mode for the whole app (not just the
terminal), with an option to follow the OS preference, a global default,
and a per-tab override for anyone who wants a specific tab to differ from
the global setting. No existing light/dark concept exists anywhere in this
codebase today — confirmed by grep (no `prefers-color-scheme`, no theme-mode
setting) — every color is a single fixed dark value, in both
`frontend/tailwindsetup.css`'s `@theme` token block and 38 legacy `.scss`
files that predate the app's ongoing Tailwind migration (per
`.kilocode/rules/rules.md`'s "hybrid system, migrating to Tailwind" note).

This spec is the first of two. It builds the mode-detection, storage, and
CSS-switching infrastructure and applies it to the existing app chrome
(Wave Config modal, tab bar, sidebars, every existing component's colors —
full parity, no file left dark-only per owner's explicit "no half-measures"
instruction). It deliberately does **not** touch `termthemes.json` or
terminal-block color selection — that is a separate, second spec
(light variants for the 7 existing terminal palettes), built afterward on
top of this spec's resolved-mode primitive. Decomposed this way because the
two pieces are independently reviewable, plannable, and shippable, and the
infra piece alone already delivers a working, useful light mode for
everything except the terminal panes themselves.

## Explicitly out of scope (this spec)

- Terminal palette (`termthemes.json`) light variants — second spec.
- Automatic per-application-window appearance beyond what Electron's
  `nativeTheme` already exposes (e.g. this does not attempt to also
  restyle native OS chrome like the window titlebar on platforms where
  Electron doesn't already handle that itself).
- Any color value beyond a good-faith proposed light palette — owner
  reviews and adjusts the actual hex/rgb values once rendered, this spec
  fixes the *mechanism*, not the final palette.

## Architecture

### Resolved-mode computation

A single derived value, `"light" | "dark"`, computed in priority order:

1. **Per-tab override** — `tab:appearancemode` tab-meta key, tri-state
   (`inherit` / `light` / `dark`), default `inherit`. Uses the existing
   `getTabMetaKeyAtom` mechanism already used for `tab:background` (see
   `frontend/app/app-bg.tsx`), same storage class, no new plumbing pattern.
2. **Global setting** — `window:appearancemode`, tri-state (`system` /
   `light` / `dark`), new `settings.json` key, default `system`.
3. **OS preference** — read via a new Electron `nativeTheme` binding (see
   below). Used when the global setting is `system` and no per-tab override
   is set.
4. **Fallback** — `dark`, used only if the OS-preference signal is ever
   genuinely unavailable (IPC failure, preload not yet initialized). Never
   throws; a missing signal degrades to the current default, not an error
   state.

Exposed as a Jotai atom (derived, read-only) combining the three input
atoms — no component computes this inline, everything downstream (the
`data-theme` effect below, any conditional styling) reads the one derived
atom.

### Electron OS-preference binding

New preload API method (per `.kilocode/skills/electron-api/SKILL.md`'s
pattern) exposing Electron's `nativeTheme.shouldUseDarkColors` as an
initial value, plus a subscription to `nativeTheme`'s `updated` event
pushed to the renderer over IPC. The renderer-side atom for "OS prefers
dark" subscribes once at app start and updates live if the OS preference
changes while the app is running (e.g. the user flips their system's
light/dark switch with RemoteTerm already open).

### CSS switching mechanism

A single effect at the app root reads the resolved-mode atom and sets
`data-theme="light"` or `data-theme="dark"` on `document.documentElement`.
`tailwindsetup.css`'s existing `@theme` block keeps its current values
unchanged as the dark defaults (no behavior change for anyone who never
touches this feature). A new `:root[data-theme="light"] { ... }` block,
using the identical custom-property names Tailwind's `@theme` emits
(`--color-background`, `--color-foreground`, `--color-accent-*`,
`--color-panel`, `--color-border`, etc. — the ~25 tokens currently in
`tailwindsetup.css`), supplies the light-mode values. No
`@media (prefers-color-scheme)` query anywhere — the attribute already
encodes the fully-resolved mode (OS preference is only one of its three
inputs), so CSS never needs to re-derive it.

### Legacy `.scss` audit

All 38 `.scss` files get inventoried during implementation planning. For
each file's hardcoded colors, the fix is either (a) replace the hardcoded
value with a reference to the corresponding `--color-*` custom property, so
it inherits the toggle for free, or (b) where a file's color doesn't map
cleanly onto an existing token, add an explicit `[data-theme="light"]`
override block scoped to that file's own selectors. Full parity is the
target — the planning pass produces the actual file-by-file list, this
spec commits to the goal and the mechanism, not a pre-audited inventory
(that inventory is planning work, not design work).

### Color values

I propose the actual light-mode values (derived from the existing dark
tokens' hue/saturation relationships, inverted lightness, checked against
WCAG AA text-contrast minimums the same way the earlier accent-button fix
was — see `.kilocode/rules/rules.md`'s accent-button contrast note as the
precedent for how this project handles contrast math) as part of the
implementation. Owner reviews against the rendered app and adjusts — this
is expected to take a round or two of live feedback, not a one-shot correct
guess.

### UI placement

- Global setting (`window:appearancemode`): new field in the General
  settings visual component (`frontend/app/view/remotetermconfig/generalcontent.tsx`),
  alongside the existing ~80 other settings fields it already covers.
- Per-tab override (`tab:appearancemode`): new entry in the tab right-click
  context menu (`frontend/app/tab/tabcontextmenu.ts`), same place
  `tab:background` is already set/cleared — same interaction pattern,
  same file, no new UI surface invented.

## Error handling

The OS-preference IPC binding never throws into application code — a
missing/failed preload call resolves to "no OS signal available," which
the priority chain above treats identically to "OS preference unknown,"
falling through to the `dark` fallback. This mirrors the existing
project-wide convention (seen in `sysinfo.go`'s collectors and elsewhere)
of degrading silently on a lower-priority signal rather than surfacing an
error for something recoverable by falling back to a sane default.

## Testing

- **Resolved-mode atom**: unit tests over the priority chain directly
  (per-tab override present → wins regardless of global/OS; per-tab
  `inherit` + global `light`/`dark` → global wins; global `system` + OS
  atom `true`/`false` → OS wins; all three absent/unavailable → `dark`
  fallback). No real Electron call needed for this layer — the OS-preference
  input is just another atom under test.
- **Electron IPC binding**: exercised via a `WaveEnv` preview/mock
  environment (per `.kilocode/skills/waveenv/SKILL.md`'s documented
  pattern for narrowing environmental dependencies), not a real OS
  `nativeTheme` call — tests inject a fake preload response and a fake
  `updated` event to confirm the renderer-side atom updates live.
- **`data-theme` effect**: a small React test asserting the attribute
  follows the resolved-mode atom's value, including on live changes (not
  just initial render).
- **SCSS/CSS audit**: no automated test realistically covers "every legacy
  file has correct light colors" — this is planning-time inventory +
  manual/visual verification once rendered, same as the color-value review
  above. Flagged explicitly rather than implied covered by the unit tests
  above.

## Open items carried into the implementation plan (not decisions, just don't-forget)

- Exact preload API method name and IPC channel name for the `nativeTheme`
  binding — finalize against existing `custom.d.ts`/`ElectronApi` naming
  conventions during planning.
- The 38-file `.scss` audit's actual per-file list and per-file
  classification (maps onto an existing token vs needs its own override
  block) is planning work, not enumerated here.
- Whether any current component reads a Tailwind class that assumes a
  specific literal color rather than a token (i.e. doesn't go through
  `--color-*` at all) needs to surface during the audit — this spec assumes
  the existing `@theme` tokens already cover the Tailwind-driven majority
  of the app (per the earlier config-reskin arc's stated direction of
  migrating toward Tailwind classes), but that assumption gets verified,
  not taken on faith, during planning.
