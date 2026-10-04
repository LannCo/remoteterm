# Builder terminal panel: isolated E2E run (Task 18, attempt 3)

Date: 2026-10-04. Branch `feat/builder-terminal` at HEAD `5b755161` (plan v9). The build includes the F1 fix `266449fc` and the final-review wave `05870ea4..05ea640b`. Not committed; the reviewer decides. Earlier attempts are in `E2E-REPORT-attempt{1,2}.md` and `e2e-evidence-attempt{1,2}/`.

## Summary

Every check passed as written: S1-S10, both preflights, the isolation proof and cleanup. No probe needed a correction, and no product code was changed.

| Check | Result | Key evidence |
|---|---|---|
| S1 first open | **PASS** | one builder tab and one block; it is focused and builder focus is `terminal`; `pwd` is the app folder |
| S2 Open terminal | **PASS** | db=2, dom=2, one new id; it is focused and builder focus is `terminal`; `pwd` is the app folder; main windows untouched |
| S3 keys, header split | **PASS** | Alt+D 3/3, Shift+Alt+D 4/4, Ctrl+Shift+S then ArrowDown 5/5, Alt+N 6/6, header split 7/7. Each adds one new focused pane in the app folder |
| S3 two rapid splits | **PASS** | db=9, dom=9. The layout-save race **did not reproduce** |
| S4 close panes | **PASS** | see the S4 section below |
| S5 width persists | **PASS** | the drag clear of the Preview (y=75) gives 54.3, saved as 54.2857; a reload keeps the size, the blocks and the shell |
| S5 drag across preview | **PASS** | the drag at y=363, across the webview, moves 54.3 to 61.4; `layout.terminal` 61.4286; handle state `hover`; webview `pointer-events` back to `auto` afterwards |
| S6 a close | **PASS** | tab and layout gone, blocks gone, 2 shells dead; builder page gone |
| S6 b Alt+W from app side | **PASS** | same checks, 1 shell |
| S6 c app switch replay | **PASS** | tab and layout gone, shell dead; the window stays open on app selection |
| S7 startup sweep | **PASS** | order PASS (spawned line 671, sweep 711, web listening 714); crash tab, layout and blocks swept; `builder_tabs` returns exactly the spoofed tab |
| S8 not in workspaces | **PASS** | tabids PASS; the tab bar (visible window) shows the workspace tab and not the builder tab; workspace count unchanged |
| S9 one rebuild | **PASS** | 43 to 45 lines; one `stopping previous app`; transitions `bg-warning` then `bg-success` |
| S10 spoofed tab survives | **PASS** | the spoof was confirmed applied before relaunch; after the sweep, owner `e2e-spoof` is kept and the tab is still in `tabids` (and keeps its 4 blocks) |

S4 in detail:
- Alt+W closes the focused pane and its shell dies.
- Ctrl+W deletes a word in the shell (writes `hello`).
- 8 distinct shell PIDs; every shell is gone after the one-by-one Alt+W.
- The empty tab is kept, builder focus is `app`, "No terminals" is shown, and the window stays open, also after a held Alt+W.
- The empty-state "Open terminal" adds one focused pane in the app folder.

The menu splits (context menu) were not exercised; S3 used the keys, the chord and the header button.

Points the coordinator asked about:
- **S5 drag across preview (F1 fix):** PASS in a real `<webview>`. The panel grew by 7.1 points, the new size was saved, and the handle left its `drag` state. The webview's computed `pointer-events` was `auto` again after the drag.
- **S1/S4 focus after the last-pane close:** `S4 focus to app PASS` on the brief's single check. That check runs after the per-shell `wait_dead` loops, which take up to 5 s per shell, so it effectively follows a short wait, and `app` was in place by then. S1's builder focus was `terminal` as expected.
- **S10 with the corrected spoof:** the read-back printed `spoof applied to fe951475...` before launch 2. After the sweep, the tab still carries `builder:owner=e2e-spoof`, still sits in its workspace's `tabids`, and is the only row `builder_tabs` returns.

## Run record

- **Scratch root:** `SCR=/tmp/rtbt.aj8D`, `PORT=51463`, `START=2026-10-04 15:38:31`.
- **Node:** `NODEBIN=/home/owner/.nvm/versions/node/v22.22.2/bin`.
- **Pane shell:** `GNU bash, version 5.2.21(1)-release`; the server has `SHELL=/bin/bash`.
- **Build:** `build.rc=0`. All four outputs are newer than `start-marker`. Go, zig and vite caches were scratch.
- **Launch command:** `launch.sh` verbatim from the brief. It runs `env -i ... setsid xvfb-run -a -s "-screen 0 1600x1000x24" dbus-run-session -- node_modules/.bin/electron-vite preview --skipBuild --config $SCR/e2e.vite.config.ts -- --remote-debugging-port=51463 --user-data-dir=$SCR/ud`, with scratch `HOME`, XDG, `TMPDIR`, `REMOTETERM_CONFIG_HOME`/`DATA_HOME`, `REMOTETERM_ISOLATED_PROFILE=1` and `ulimit -c 0`.
- **Launch 1:** session 3117125, equal to the launch PID. Electron 3117156, server 3117198, Xvfb `:99`.
- **Launch 2:** session 3150296, equal to the launch PID. Electron 3150355, server 3150448, Xvfb `:99`.
- **`REMOTETERM_ISOLATED_PROFILE`:** nothing in this worktree reads it (`grep -rn ISOLATED_PROFILE emain frontend pkg cmd` finds nothing). Isolation rests on the config and data homes, `--user-data-dir`, and the scratch `HOME`, XDG and `TMPDIR` dirs.
- **First-run modals:** dismissed through `mcdp`: "Welcome to RemoteTerm" (Continue), then the "Durable SSH Sessions" tour (Skip Feature Tour).

### Preflight

- **Environment (both launches):**
  - Electron and server: `DISPLAY=:99`, no `WAYLAND_DISPLAY`, `HOME=$SCR/home`, `REMOTETERM_CONFIG_HOME=$SCR/cfg`, `REMOTETERM_DATA_HOME=$SCR/data`.
  - Electron: `TMPDIR=$SCR/tmp`.
  - Server: `SHELL=/bin/bash`.
- **Launch 1:** `fds-outside PASS (36 descriptors)`, `journal PASS`.
- **Launch 2:** `fds-outside PASS (38 descriptors)`, `journal PASS`.
- **Private dbus session:** gvfsd, gvfsd-fuse (`$SCR/run/gvfs`), the xdg portals and at-spi all ran inside the run's session.

## Isolation proof

- **Final fd scan:** launch 2 with the app up, 25 processes, 589 descriptors, **PASS**. Nothing under the real `$HOME` was open, and the node-install list was empty.
  - Launch 1's descriptors were checked only at preflight 1, which passed.
  - Launch 1's processes ended at the S7 crash step.
- **Scratch profile used:** files written after `start-marker`:
  - `$SCR/data/db/{waveterm,filestore}.db*`
  - `$SCR/cfg/settings.json`
  - `$SCR/cfg/electron/*`
- **App folder:** `$SCR/home/waveapps/draft/e2e1`. `~/waveapps/draft/e2e1` does not exist.
- **User files that changed** (`user-files-diff.txt`):
  - `~/.config/RemoteTerm (Dev)/{DawnGraphiteCache,DawnWebGPUCache,GPUCache}/data_1`
  - `~/.config/RemoteTerm (Dev)/Partitions/webblock/{Cookies,Cookies-journal,TransportSecurity}`
  - `~/.local/share/remoteterm-dev/db/filestore.db*`
  - `~/.local/share/remoteterm-dev/rtapp.log`
  - `~/.local/share/remoteterm-memwatch/mem.log`
- **Who wrote them:** none is in `final-fds-in-home.txt` or `preflight{1,2}-fds-outside.txt` (all empty). Your live `remoteterm-built` dev instance (`live-processes-before.txt`) and your memwatch service wrote them, so they are not leaks.
- **Vite stamps:** unchanged (`.vite-temp` excluded, per v8).
- **Journal:** **PASS**.

## Cleanup: PASS

- **Processes:** `stop_run` printed `(no process of this run)` and `no process of this run left`, and the session list was empty.
- **Xvfb:** exited cleanly. `/tmp/.X99-lock` is gone and nothing needed removing.
- **Removal guard:** the first try at the final removal call stopped with "processes of this run are alive". The only process it flagged was my own tool shell, whose cwd was `$SCR/logs` after the evidence copy, so `own_pid`'s cwd rule matched it. The guard worked as intended.
- **Removal:** re-run from the repo root, it printed `removed /tmp/rtbt.aj8D`. No `/tmp/rtbt.*` remains.
- **Kills:** the only processes killed were this run's server (after the `own_pid` and session check) and this run's session (after `own_sid`). No pkill or killall by name. `DISPLAY=:0` was never used, and every call unset `DISPLAY`/`WAYLAND_DISPLAY`.

## Substitutions

- **No window manager:** the title-bar close is `window.close()`.
- **Switch App:** the native Switch App menu was replaced by a replay of `switchBuilderApp`'s RPCs. `typeof window.BuilderTermModel` printed `"undefined"`, so `markSwitching()` was not exercised end to end; the unit tests cover it (Tasks 14 and 15).
- **`WAVESRV-ESTART`:** `spawned remotetermsrv` and `Server [web] listening on` stand in for it, since emain consumes it without logging.

## Harness notes

- **Tracebacks:** Python tracebacks appear while the builder tab has no blocks. Its `blockids` is JSON `null`, which `builder_block_ids` cannot parse. This is cosmetic; the counts are right.
- **Watchdog timeouts:** the `bcdp key Alt+W` in S6b and the `bcdp eval "window.close()"` in Step 10 hit cdp's 30 s watchdog. In both cases the page closed before replying, which is expected.

## Evidence

`.planning/builder-terminal/e2e-evidence/` holds:
- all run logs except the 21k-line user file listings (the diff is kept)
- screenshots
- the scratch `rtapp.log` (`rtapp-scratch.log`) and the app's `build.log`
- the exact helper scripts

Auth keys are redacted in `preflight*-environ.txt`.
