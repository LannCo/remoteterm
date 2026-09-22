# QA loop 2026-09-22: remaining items

Branch `qa/fleet-2026-09-22`. The loop terminated STALLED at the 5-round iteration cap. Code, security and coverage passed round 5; a11y and conformance did not. Evidence for every item is in the `round5-*.md` report named.

## Open

| ID | Sev | Where | What | Suggested fix |
|---|---|---|---|---|
| R5-A11Y-1 | High | `frontend/app/view/sourcecontrol/sourcecontrol.tsx:711`, `frontend/app/view/remotetermconfig/remotetermconfig.tsx:193` | The round-4 focus fix puts `tabIndex={-1}` + `outline-none` on the whole view container: no visible focus, and any blank-area click moves focus there. | Move `tabIndex={-1}` to a small anchor (panel heading) with a `focus-visible:` style; give it an accessible name (R5-A11Y-2). |
| R5-CONF-3 | High | `pkg/wslconn/wslconn.go` | The wsh install prompt says "Install Wave Shell Extensions". | Rebrand the title and body. |
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
