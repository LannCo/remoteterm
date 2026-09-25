# Round-2 Conformance Audit: RemoteTerm rename fix wave

Branch: `qa/fleet-2026-09-22`, HEAD `75b60911`.
Round-1 report: `docs/qa-review/conformance-auditor.md`.
Source of truth: `RENAME_PLAN.md`, `RENAME_ALLOWLIST.md`.

**Findings:** CRITICAL: 0 | HIGH: 1 | MEDIUM: 1 | LOW: 0 | OK: 4

## Verified

### CONF-1 (remote-host `.waveterm`) — correctly deferred, not half-implemented
Owner decision: defer to its own task. `pkg/remotetermbase/wavebase.go` gained 40 lines in the
fix wave (`AcquireWaveLock`/legacy-lock handling for `282c9246`), but the five remote-host
literals round-1 cited are unchanged in value, only shifted by line number:
`RemoteWaveHomeDirName = ".waveterm"` (wavebase.go:144), `RemoteFullWshBinPath =
"~/.waveterm/bin/wsh"` (:146), `RemoteFullDomainSocketPath = "~/.waveterm/wave-remote.sock"`
(:147), `"~/.waveterm/client/%s/waveterm.sock"` (:304), `filepath.Join(homeDir, ".waveterm",
"jobs")` (:609); `pkg/wshrpc/wshremote/wshremote.go:134`'s inline `"~/.waveterm/bin/wsh"` is
untouched, same line number as round 1. Confirmed with `git diff 35cd6146..75b60911 -- <files>`.

### CONF-2 — cleanup only, as expected
`git diff 35cd6146..75b60911 --stat -- emain/emain-log.ts` is empty; the file the round-1
PARTIAL finding cited was not touched.

### CONF-3 packaged icons — fixed correctly
`e7a30ef8` replaced `build/icon.icns` (65173 bytes, still all 10 Apple icon types
`ic04/05/07/08/09/10/11/12/13/14` per `icns` header parse — full macOS size set),
`build/icon.ico` (9 Windows icon sizes per `file`), and all 8 `build/icons/*.png`
(16..512px, RGBA, per `file`). `electron-builder.config.cjs` has no explicit `icon:` key, so
electron-builder resolves these via its documented convention (`build/icon.icns` mac,
`build/icon.ico` win, `build/icons/*.png` linux) — format/size set matches what the build
needs. No round-1 CONF-3 file still shows old-size Wave art.

### CONF-4 — fixed, and residual sweep of the fix wave clean
`2c50d488` reworded all 8 strings round-1 quoted in
`frontend/app/view/remotetermconfig/generalcontent.tsx` (confirmed lines 66, 75, 120, 196, 560,
758/767/775/783 now read "RemoteTerm"/"RTApps"/"RTApp Builder"). Swept every non-test file
changed in `35cd6146..75b60911` (32 files) for added lines matching `wave(term)?\b|wavesrv`:
zero hits. A broader whole-file (not diff-only) sweep of those 32 files found 4 files with
"wave" hits, all pre-existing and non-user-visible: `emain-platform.ts` (constant/comment names
for the frozen legacy detection, correctly retained per plan), `wavebase.go`/
`wavebase-posix_test.go` (same), `wshremote/git.go:437` (`os.CreateTemp("", "wave-git-askpass-*.sh")`,
an internal temp filename). One incidental, pre-existing (not introduced by the fix wave, first
landed in upstream commit `755d9783`) item: `emain-platform.ts:293`'s ARM64-warning dialog opens
`https://docs.rterm.dev/faq#why-does-wave-warn-me-about-arm64-translation-when-it-launches` — the
domain is correctly rebranded but the anchor slug still says "wave"; the anchor still matches
`docs/docs/config.mdx:38`'s own (also still-"Wave") heading, so the link isn't broken, just
low-visibility residue inside a URL a user never reads directly. Not filing as a gap: it's
outside the fix-wave's diff and falls under the already-flagged `docs/docs/*.mdx` editorial-content
carve-out in `RENAME_ALLOWLIST.md:104-108`.

## Gaps

### [HIGH] Runtime Linux window icon still ships old Wave Terminal art, unaddressed by the icon fix
- **ID:** R2-CONF-1
- **Category:** partial (fix scoped narrower than the visible surface)
- **Source says:** RENAME_PLAN.md:457-465 (Prerequisites decision 8): "ship new RemoteTerm
  artwork everywhere... extended to every load-bearing path named above." `RENAME_ALLOWLIST.md:115-120`
  separately flags `public/logos/wave-*.png` as blocked pending real design assets under this
  same decision.
- **Code does:** `emain/emain-builder.ts:63` and `emain/emain-window.ts:193` both set
  `winOpts.icon`/`icon` (Linux only) to `path.join(getElectronAppBasePath(),
  "public/logos/wave-logo-dark.png")` — the literal `BrowserWindow` icon shown in the taskbar,
  alt-tab switcher, and window manager decorations while the app is running. Confirmed by reading
  `public/logos/wave-logo-dark.png`: it is still the old green Wave Terminal ribbon mark, not
  RemoteTerm's `frontend/app/asset/logo.svg`. `e7a30ef8` (the icon fix this round verifies) only
  touched `build/icon.*` (9 files, installer/desktop-file icons), not this path.
- **Impact:** on Linux, the icon a user actually sees while the app is running (taskbar, dock,
  alt-tab) is a separate code path from the installer/AppImage icon the plan's decision 8 and the
  fix wave addressed. After `e7a30ef8`, the app now installs with a RemoteTerm icon but runs with
  a Wave Terminal one — a more visible split-brand state than before the fix, since a user now
  sees "RemoteTerm" in the installer/app list and "Wave" in the live window they're using every
  day. This is not a silent miss: `RENAME_ALLOWLIST.md` already flags `public/logos/wave-*.png`
  generically as blocked on new design assets, but that entry does not record that these specific
  two files are wired directly into the runtime window icon (the plan's stated highest-priority
  surface) rather than being decorative-only image residue, and the same `logo.svg`-render
  pipeline `e7a30ef8` used for `build/icon.*` was available to fix this too.
- **Recommendation:** render `public/logos/wave-logo-dark.png` (and any light-mode counterpart
  actually reachable at runtime) from `frontend/app/asset/logo.svg` using the same approach as
  `e7a30ef8`, or update `RENAME_ALLOWLIST.md:115-120` to explicitly note these two files are the
  live window icon, not just packaging-adjacent residue, so the next pass doesn't treat it as
  low-priority.

### [MEDIUM] RENAME_PLAN.md's Phase 2 text still mandates the dev-build fallback CA-5's fix removed
- **ID:** R2-CONF-2
- **Category:** divergent
- **Source says:** RENAME_PLAN.md:839-842 (Phase 2 step 7): "Do **not** delete the old
  directory's *parent*... This reuses the existing `getWaveHomeDir()` backwards-compat pattern
  already in the file (it already has a legacy `~/.waveterm` fallback for pre-v0.8 installs) —
  the new migration should sit alongside it as a second, newer compat layer, **not replace it**."
- **Code does:** `6563e25c` (fixing code-auditor finding CA-5, `docs/qa-review/code-auditor.md:39-44`)
  removed exactly that fallback: `getRemoteTermHomeDir()` (`emain/emain-platform.ts`) no longer
  falls back to the bare, pre-suffix `~/.waveterm` in dev builds — it now only checks the
  suffix-aware `LegacyRemoteTermHomeDirNameSuffixed` (`.waveterm-dev` in dev, `.waveterm` in prod).
  The commit message states this fallback let a dev build silently read production data, since
  "upstream `getWaveHomeDir` only ever checked `~/.waveterm-dev` in dev" and never had this
  fallback at all.
- **Impact:** the plan text a future maintainer reads still instructs them to keep the bare
  dev-to-prod fallback as a required "existing pattern," directly contradicting the shipped code.
  If someone re-derives this function from the plan (e.g. during a later refactor or a rollback),
  they would reintroduce the exact cross-environment data leak CA-5 found and this fix removed.
  I can't confirm from the repository alone whether the plan's characterization was simply wrong
  when written (most likely, since `6563e25c`'s own diff comment says this fallback was never
  real upstream behaviour and was introduced later by `d6666d49`) or whether the owner still wants
  some bare pre-suffix fallback for a case neither the plan nor the fix's commit message names
  explicitly — that's a genuine ambiguity, not something I can resolve by reading the diff alone.
- **Recommendation:** update RENAME_PLAN.md:839-842 to describe the corrected, suffix-aware-only
  fallback behaviour (or explicitly record that the bare pre-suffix fallback was deliberately
  removed as a bug fix), so the plan and the code agree again.

## Ambiguities

- Whether `RENAME_PLAN.md:839-842`'s "keep the existing `~/.waveterm` fallback" instruction was
  simply incorrect when written (plausible, since `6563e25c` argues it was never real upstream
  behaviour) or whether some narrower version of that fallback is still wanted for a scenario the
  fix's commit message doesn't name — not resolvable without an owner decision (see R2-CONF-2).
- `7e781c9e`'s config-root legacy-detection broadening (any top-level `*.json` or `presets/`,
  not just `settings.json`) is a strict superset of the plan's literal "validate... with
  `settings.json`" instruction (RENAME_PLAN.md:756-757) but matches the plan's own stated goal
  ("a file it actually contains") more completely than the plan's specific example did — read
  this as an improvement consistent with intent, not a divergence; flagging only as worth pinning
  explicitly in the plan text given it already needed one correction (CA-6).
