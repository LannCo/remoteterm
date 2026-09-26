# `window.open()` disposition mapping — Electron 41 observed behaviour

Task 1 of `.pi/plans/2026-09-26-webblock-popup-support.md` (remoteterm-daily),
run against a standalone harness rather than `task dev` (see
"Deviation from plan" below).

## Command used

```
cd /media/owner/Workspace/remoteterm/remoteterm-webpopup
env -u DISPLAY -u WAYLAND_DISPLAY xvfb-run -a -s "-screen 0 1600x1000x24" node_modules/.bin/electron --no-sandbox .pi/evidence/2026-09-26-webblock-popup/harness/probe-main.cjs
```

`--no-sandbox` was required: without it, Electron's `BrowserWindow`/renderer
never came up under this container's Xvfb (`app.whenReady()` resolved and the
HTTP server started, but nothing after `new BrowserWindow()` progressed, no
renderer or GPU child processes appeared, and it hung indefinitely). With
`--no-sandbox` renderer/zygote child processes appeared immediately and the
run completed in a few seconds. `--disable-gpu` was tried during triage and
made no difference either way; it is not needed and is not in the final
command above.

## Observed `[popup-probe]` output (raw, in order)

```
[popup-probe] {"url":"http://127.0.0.1:36231/popup-child.html","disposition":"new-window","features":"width=480,height=600","frameName":"fixturePopup"}
[popup-probe] {"url":"http://127.0.0.1:36231/popup-child.html","disposition":"foreground-tab","features":"","frameName":""}
[popup-probe] {"url":"http://127.0.0.1:36231/popup-child.html","disposition":"new-window","features":"noopener,width=480,height=600","frameName":""}
[popup-probe] {"url":"about:blank","disposition":"new-window","features":"width=480,height=600","frameName":"fixtureBlank"}
[popup-probe] {"url":"http://127.0.0.1:36231/popup-child.html","disposition":"foreground-tab","features":"","frameName":""}
[popup-probe] {"url":"http://127.0.0.1:36231/popup-child.html","disposition":"new-window","features":"popup","frameName":""}
[popup-probe] {"url":"http://127.0.0.1:36231/popup-child.html","disposition":"foreground-tab","features":"noopener","frameName":"x"}
```

(Port number is whatever the harness's embedded HTTP server bound on that
run; irrelevant to the result.)

## Table

| Trigger | Call | url | disposition | features | frameName | Matches plan expectation? |
|---|---|---|---|---|---|---|
| `feat` | `window.open(child, "fixturePopup", "width=480,height=600")` | popup-child.html | `new-window` | `width=480,height=600` | `fixturePopup` | Yes — feat expected `new-window` |
| `plain` | `window.open(child)` | popup-child.html | `foreground-tab` | `` | `` | Yes — plain expected `foreground-tab` |
| `noopener` | `window.open(child, "_blank", "noopener,width=480,height=600")` | popup-child.html | `new-window` | `noopener,width=480,height=600` | `` | Yes — plan's Step 3 note says noopener → `new-window` **with `noopener` in features** (this is exactly the case rule 2 of `classifyWindowOpen` exists to catch) |
| `blank` | `window.open("about:blank", "fixtureBlank", "width=480,height=600")` | about:blank | `new-window` | `width=480,height=600` | `fixtureBlank` | Yes — blank expected `new-window` |
| `target=_blank` link | `<a href="popup-child.html" target="_blank">` | popup-child.html | `foreground-tab` | `` | `` | Yes — target=_blank expected `foreground-tab` |
| extra: `window.open(url, "_blank", "popup")` | features = just the token `popup`, no size | popup-child.html | `new-window` | `popup` | `` | Not in plan's 5-row table; recorded because the brief asked for it — see finding below |
| extra: `window.open(url, "x", "noopener")` | features = just the token `noopener`, no size | popup-child.html | `foreground-tab` | `noopener` | `x` | Not in plan's 5-row table; recorded because the brief asked for it — see finding below |

All five of the plan's Step 3 rows match its stated expectation exactly,
including the `noopener` row (`new-window` with `noopener` present in
`features`, not denied or filtered before reaching the handler).

## What surprised me: the two extra probes

The plan's rule 2 (`features contains a truthy noopener or noreferrer -> tab`)
implicitly assumes `noopener` always arrives with a `new-window` disposition
that then needs downgrading. The two extra probes the brief asked for show
that isn't universally true — **disposition depends on which feature keys are
present, not on whether `noopener` is present**:

- `noopener` **combined with** window-geometry features (`width=480,height=600`,
  as in the fixture's `noopener` button) → `new-window`. Rule 1 alone would
  let this through as a popup; rule 2 is what correctly downgrades it to
  `tab`. This confirms rule 2 is load-bearing, not defensive-only.
- `noopener` **alone**, with no geometry/`popup` feature (extra probe 7,
  `window.open(url, "x", "noopener")`) → `foreground-tab` already, same as
  the `plain` case. Rule 1 handles this on its own; rule 2 never gets a
  chance to fire, but doing no harm since a `tab` result is what both rules
  agree on.
- A features string containing only the bare token `popup` (extra probe 6,
  `window.open(url, "_blank", "popup")`, no size at all) still yields
  `new-window`, same as a full geometry string. So `new-window` disposition
  is driven by Chromium recognising *any* popup-shaping key (`popup`,
  `width`, `height`, `left`, `top`, etc.) in the features string, not by the
  string being merely non-empty and not by size specifically.

Net effect on the classifier: rule 1 (`disposition !== "new-window" -> tab`)
and rule 2 (`noopener`/`noreferrer` in features -> tab) as specified in the
plan are sufficient and correctly ordered for every case observed here,
including the case that could have been missed (`noopener` + geometry).
No change to the routing rule is indicated by this run.

`frameName` is empty string (not `"_blank"`) whenever the caller passed
`"_blank"` as the target name — Electron does not surface `_blank` itself as
a `frameName`. Not needed by `classifyWindowOpen` (which never inspects
`frameName`), noted only because it appears in the raw table above.

## Files created

- `.pi/evidence/2026-09-26-webblock-popup/popup-fixture.html`
- `.pi/evidence/2026-09-26-webblock-popup/popup-child.html`
- `.pi/evidence/2026-09-26-webblock-popup/harness/probe-main.cjs`
- `.pi/evidence/2026-09-26-webblock-popup/dispositions.md` (this file)

## Deviation from plan (as instructed)

The plan's own Task 1 (Steps 2-3) says to add a temporary probe line to
`emain/emain-tabview.ts` and drive it via `task dev` against the real app.
This run instead used a standalone Electron harness
(`harness/probe-main.cjs`) that never touches `remoteterm-daily`, `task dev`,
or the app's own webview-attach/hardening code — per explicit instruction, to
avoid writing to or running against `remoteterm-daily`'s live dev process.
The harness reproduces the same shape (a `<webview partition="persist:webblock"
allowpopups>` guest with a `setWindowOpenHandler` logging `HandlerDetails`)
and Electron's disposition reporting is a property of Electron/Chromium, not
of RemoteTerm's own code, so the observed dispositions are unaffected by this
substitution. It is structured (fixture pages reused as-is from the plan,
harness isolated under `harness/`) so a later task can extend it — e.g. swap
the inline `setWindowOpenHandler` for the real `emain/emain-popup.ts` bundled
with `esbuild` — without rewriting the scaffolding.
