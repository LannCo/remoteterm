# QA loop 2026-09-22: remaining items

Branch `qa/fleet-2026-09-22`. The loop terminated STALLED at the 5-round iteration cap. Code, security and coverage passed round 5; a11y and conformance did not. Evidence for every item is in the `round5-*.md` report named.

## Fixed this pass

| ID | Sev | Fix commit |
|---|---|---|
| R5-A11Y-1 | High | `e1c82356` — focus anchor moved off the panel wrapper onto a small header element (branch-name group / file-title) with a `focus-visible:` ring and `aria-label`. |
| R5-CONF-3 | High | `b1c34bd4` — "Wave" → "RemoteTerm" in the wsh install-prompt title/body; same bug also found and fixed in `pkg/remote/conncontroller/conncontroller.go` (not in the original finding). |

## Open

| ID | Sev | Where | What | Suggested fix |
|---|---|---|---|---|
| R5-CONF-1 | Medium | `frontend/app/onboarding/onboarding-upgrade-v0144.tsx:67,70` | "WaveConfig" left in place. | Rename to the current view name. |
| R5-CONF-2/4/5/6 | Low | `onboarding-layout-term.tsx:80`, `workspace/widgets.tsx:620,664`, `blockcontroller/shellcontroller.go:830,835`, `remote/sshclient.go:1156` | `wavesrv` / "Wave" in rare strings. | Rebrand. |
| R5-CA-1 | Low | `frontend/app/view/remotetermconfig/generalcontent.tsx:1100` | An external write of a value this control wrote earlier is taken as its own echo; the field shows a stale value. | Match echoes by write sequence, not value. |
| R5-CA-2 | Info | `emain/emain-platform.ts:136,344` | `migrationIncomplete` isn't reset on "Migrate anyway"; a transient failure costs one extra quit. | Reset the flag before the re-run. |
| R3-SEC-1 | Low | `emain/emain-platform.ts` merge | The merge follows symlinks inside the destination. Precondition is home-dir write access (out of scope per SEC-1). | Optional: `lstat` and refuse symlinks. |

## Deferred by owner
- CONF-1: remote-host `~/.waveterm` rename (RENAME_PLAN decision 5). Own task.

## Pre-existing, not introduced by this branch
- `go test -race ./pkg/util/iochan` fails (race in test code).
- tsc: 18 errors in preview/fixture files (also on `main`).
- `GetMapValNewOrLegacy` and `pkg/shellexec` have no tests.

## Untested on real hardware
- macOS migration paths (`ps -o comm=`, the `electron/` allowance): mocked only.
- The runtime window icon loading from inside `app.asar`: build output verified, not the packaged app.
- The migration dialogs haven't been seen in the running app.
