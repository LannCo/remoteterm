# Refute Go claims: premises and intent

Commit `93aaefc3`; base `origin/main` (merge-base `6d6128e5`). All line refs are `git show 93aaefc3:<path>` unless marked `origin/main`. Package renames `wavebase`->`remotetermbase`, `wconfig`->`rtconfig` were accounted for when comparing against upstream.

## SEC-1

**VERDICT: REFUTED**

- `emain/emain-platform.ts:121-185` `migrateDataRoot` does `renameSync(spec.source, spec.dest)` (line ~168). If the source is a symlink, `rename(2)` moves the link itself: premise on mechanism holds.
- Failing premise: that the migration newly lets an attacker choose where reads/writes land. At `origin/main`, `getWaveConfigDir`/`getWaveDataDir` (origin/main `emain/emain-platform.ts:110-130`) read and wrote `~/.config/waveterm` / the `envPaths("waveterm")` data dir directly, so a planted symlink at the legacy path was already followed to the same target before this branch. After migration the link points at the same target; nothing changes.
- The stated precondition (write access to the user's home before first launch) already allows planting a symlink at the new `~/.config/remoteterm` path, or editing `settings.json`/shell rc files outright. No privilege boundary is crossed.
- Introduced by branch: migration yes; the exposure no.

## SEC-2

**VERDICT: REFUTED**

- `pkg/wshrpc/wshserver/wshserver.go:551-553` and `pkg/rtconfig/settingsconfig.go:749-765`: confirmed no server-side validation of `connName`. Identical at `origin/main` (`wshserver.go:551-552`, `wconfig/settingsconfig.go:738`); the diff is package renames only.
- Premise "only the frontend validates it" is inaccurate: the only frontend format check is the new quick-add regex (`frontend/app/view/remotetermconfig/remotetermconfig-model.ts:672`, `ConnectionQuickAddRegex`). The other caller, `frontend/app/block/connstatusoverlay.tsx:756`, does not validate. That regex is a UX input check for a new form, not a security control the server relies on.
- No harm is stated. The key is JSON-encoded by `WriteWaveHomeConfigFile` (no injection into the file), and any caller able to issue this RPC can already issue `SetConfigCommand` and command-running RPCs. Values in the same function are also unvalidated upstream.
- Introduced by branch: no.

## CA-3

**VERDICT: HOLDS** (upstream parity; branch adds reliance on it)

- `pkg/rtconfig/settingsconfig.go:711-746` `SetBaseConfigValue`: `ReadWaveHomeConfigFile` at 712, merge, then `WriteWaveHomeConfigFile` at 745. `configWriteLock` (line 30) is taken only inside `WriteWaveHomeConfigFile` (440-441), so it does not span the read. Two interleaved calls can each read the old file and the second write drops the first's key.
- Concurrency premise: RPC requests are dispatched on goroutines (`pkg/wshutil/wshrpc.go:434-438`), so two concurrent `SetConfigCommand`s are possible.
- Identical at `origin/main` (`wconfig/settingsconfig.go:700-735`, same lock scope).
- Intent: the branch's new comment at `remotetermconfig-model.ts:685-687` states "settings.json merges per-top-level-key server-side (SetBaseConfigValue), so ... no read-modify-write queue needed". That comment's premise is false under concurrent writes; the new visual settings tab issues one RPC per key change, which makes the race easier to hit than upstream.
- Severity: Low (single user, requires two writes in the same window).

## CA-4

**VERDICT: HOLDS**

- `pkg/remotetermbase/wavebase.go:134`: `const WaveLockFile = "remoteterm.lock"` (was `wave.lock`, origin/main `wavebase/wavebase.go:59`). `AcquireWaveLock` (`wavebase-posix.go:16-29`) flocks `<datadir>/remoteterm.lock`.
- `emain/emain-platform.ts:207-217`: data-root migration gates only on `wave.lock` existing (line 216), not on it being unlocked. No running-instance check anywhere in `performDataDirMigration`.
- Pre-rename and post-rename Electron apps can coexist: origin/main `app.setName("waveterm/electron")` vs `remoteterm/electron` (line 17) give different userData dirs, so `requestSingleInstanceLock` does not arbitrate between them.
- On POSIX a directory rename succeeds under open fds; the old server's flock on `wave.lock` follows the inode into the new dir, but the new server locks a different file (`remoteterm.lock`), so it acquires the lock and opens the same SQLite files. SQLite's own locking still arbitrates file-level writes; what is lost is the app-level single-server guarantee `wave.lock` existed to enforce (duplicate in-memory state/caches, two servers owning the same blocks and sockets).
- `RENAME_PLAN.md:1078-1083` records the lock-name split as intentional only for legacy detection vs new writes; it does not address a live old instance. `RENAME_PLAN.md` "Concurrency" bullet (~line 824) covers two new processes racing, not old-vs-new.
- Windows: `renameSync` on a dir with open files fails, recorded as a migration failure, so the exposure is POSIX only.
- Introduced by branch: yes (commits `668a8eda` for the lock name, Phase 2 for the shim).

## CA-5

**VERDICT: HOLDS**

- The cited lines are in `getRemoteTermHomeDir`, not the data-dir getter. `emain/emain-platform.ts:298-306`: after checking `~/.remoteterm-dev` and the suffixed legacy `~/.waveterm-dev`, a dev build falls back to bare `~/.waveterm` (`LegacyRemoteTermHomeDirName`, line 35) and uses it if it holds `wave.lock`. `getRemoteTermDataDir` (354-359) and `getRemoteTermConfigDir` (329-334) then return that home, so both data and config resolve to the production legacy combined home.
- At `origin/main`, `getWaveHomeDir` (origin/main line ~79) only checked `.${waveDirName}` = `.waveterm-dev` in dev. The bare fallback in dev is new.
- Intent: the comment at lines 37-41 says the bare constant "exists so a dev build can still recognise a genuinely old, pre-suffix, bare `.waveterm` install". "Still" is false: no dev build at origin/main ever recognised bare `.waveterm`. `wave.lock` alone cannot distinguish a "pre-suffix" install from any production pre-XDG install. `RENAME_PLAN.md:696-705` asked for a frozen `.waveterm` constant only to decouple from the prefix rename, not to widen dev detection.
- Compounds CA-4: a dev server adopting the prod home locks `remoteterm.lock`, not the prod server's `wave.lock`.
- Introduced by branch: yes (`815835c0`). Severity: Medium for developers with a legacy `~/.waveterm` containing `wave.lock`; no effect on packaged builds.

## CA-6

**VERDICT: HOLDS**

- `emain/emain-platform.ts:205`: config root `validateSource` requires `settings.json`. `migrateDataRoot` line ~139-141 returns with no log and no `recordMigrationFailure` when validation fails.
- Intent: `RENAME_PLAN.md` Phase 2 step 7 mandates `settings.json` as the config marker; it does not mandate silence.
- Premise that such dirs exist: `settings.json` is not created by default (only by `SetBaseConfigValue`/user edits; `pkg/rtconfig/settingsconfig.go` has no default write). On Linux the legacy config root always exists because origin/main `app.setName("waveterm/electron")` puts Electron userData at `~/.config/waveterm/electron`. So a user who never changed a setting keeps `widgets.json`/`presets/`/Electron state orphaned with no trace.
- Introduced by branch: yes. Severity: Low.

## CA-7

**VERDICT: HOLDS** (mechanism wording imprecise)

- `pkg/jobcontroller/jobcontroller.go:573-593`: the route-up handler does not call `restartStreaming` directly; it calls `ReconnectJobRoute` (590) -> `doReconnectJob` -> `restartStreaming` (2032). Same effect.
- `ReconnectJob` uses `reconnectConnGroup` (1999); `ReconnectJobRoute` uses `reconnectRouteGroup` (2006). Different singleflight groups keyed by the same `jobId`, so both `doReconnectJob`s run concurrently. `restartStreaming` (2209+) holds no per-job lock across reader swap, seq computation and `jobStreamIds`/`jobReaders` writes.
- Realistic trigger: during an in-flight `ReconnectJob`, `WaitForRegister` (2118) lets the route register, the route event sets Connected (563), health is not yet active, so the handler fires `ReconnectJobRoute` while the first call is about to run its own `restartStreaming` (2128).
- Intent: the code comment at 578-581 says the route entrypoint is used "so it can't race a concurrent doReconnectJob for the same job". That is false for the `ReconnectJob` path.
- Introduced by branch: yes. Two groups existed upstream, but upstream `doReconnectJob` returned early when connected, and the route-up goroutine is new (`91814977`).

## CA-8

**VERDICT: HOLDS**

- `pkg/jobcontroller/jobcontroller.go:2033-2036`: on `restartStreaming` failure in the connected-but-no-stream branch, `SetJobConnStatus(jobId, Disconnected)` with no `sendBlockJobStatusEventByJob`.
- `SetJobConnStatus` (1435-1443) only mutates `jobConnStates`; it publishes nothing. `GetBlockJobStatus` reads `GetJobConnStatus` (339), so the UI keeps the last published "connected" until some later event.
- The sibling failure path at 2125-2129 does send the event, so the omission is inconsistent with the file's own pattern. Callers (`ReconnectJobsForConn` 755-775, route-up goroutine 590-592) only log on error.
- Introduced by branch: yes (`91814977`).

## CA-9

**VERDICT: HOLDS** (severity correction: Low; reader half is upstream parity)

- Line 1610 is the `registerNewJobStream` call inside `StartJob` (starts 1542). `registerNewJobStream` (1499-1508) sets `jobStreamIds`, `jobReaders` and `jobStreamHealth{active:true}`.
- Error returns after it: `MakeFile` failure (1616-1618) and `RemoteStartJobCommand` failure (1650-1661). Neither closes the reader nor clears health. There are no `jobReaders.Delete`/`jobStreamHealth.Delete` calls anywhere in the file.
- Reader leak: identical at origin/main (1529-1531 set `jobStreamIds`/`jobReaders` before the same error paths). Not introduced.
- `active=true` seeding: introduced by `58bad6e2`. Consumers (574, 838, 2020, 2200) are reconnect/restart paths; a start-failed job is marked `Done` (RemoteStartJobCommand path) or never had a job manager (MakeFile path), so the stale flag is largely inert. State pollution, not a user-visible fault on current paths.

## CA-10

**VERDICT: HOLDS** (count and location correction)

- `conncontroller_test.go:440-451` (`TestCheckConnectionRespectsConfiguredKeepaliveInterval`) reads `cm.KeepAliveInFlight` unlocked (442, 448) while the `SendKeepAlive` goroutine clears it under `cm.lock` (`connmonitor.go:137-141`, `119`). Test added by branch (`ace80192`).
- Evidence: five existing race runs at `scratchpad/ca10-race-{1..5}.log` (against `scratchpad/repro-go`, whose `conncontroller_test.go` and `connmonitor.go` I confirmed byte-identical to `93aaefc3`). All five FAIL. Race count per run: 1, 2, 2, 3, 2. Not a stable "2".
- Races observed: (a) test:448 read vs `connmonitor.go:119` write (the cited one); (b) `ReconnectHysteresisDuration` written by test:2439 vs read at `conncontroller.go:470` by a goroutine leaked from an earlier test; (c) `getConnectionConfigTestHook` written by test:1878 vs read at `conncontroller.go:1943`. All are test-harness races (unsynchronised test reads, or package globals mutated while other tests' goroutines still run), so "in test code" holds; the cited range covers only (a).
- I did not run `go test` myself (repo rule); verdict rests on the logs plus static reading.

## CONF-1

**VERDICT: HOLDS**

- `pkg/remotetermbase/wavebase.go:139` `RemoteWaveHomeDirName = ".waveterm"`, `141` `RemoteFullWshBinPath = "~/.waveterm/bin/wsh"`, `142` `RemoteFullDomainSocketPath`, `264` `~/.waveterm/client/%s/waveterm.sock`, `569` `filepath.Join(homeDir, ".waveterm", "jobs")`; `pkg/wshrpc/wshremote/wshremote.go:134` `ExpandHomeDir("~/.waveterm/bin/wsh")`. All confirmed.
- `RENAME_PLAN.md:406` "All Prerequisites decisions - RESOLVED 2026-09-20", decision 5 (line 439): "Remote-host `.waveterm` state: **renames.**" No later revision in the plan. `RENAME_ALLOWLIST.md` does not allowlist remote `.waveterm` (its `.waveterm` entry at ~line 82 covers only the Phase 2 local migration shim).
- The Phase 3 commit `668a8eda` body claims "existing remote-host installs self-heal via reinstall on next connect", which implies the rename was done; the code does not match its own commit message.
- Introduced by branch: the strings are upstream; the non-conformance with decision 5 is the branch's omission.

## CONF-2

**VERDICT: REFUTED**

- Literal part holds: `emain/emain-log.ts:96-97` (inside a top-level `try`) and `106` call `getRemoteTermDataDir()` at module scope, and `RENAME_PLAN.md:813-817` asks to confirm every hit outside `emain-platform.ts` is inside a function body.
- Failing premise: "it could resolve the data dir before migration runs". `emain-log.ts:8` imports from `./emain-platform`, and ESM evaluates a dependency's top-level before the importer's. Migration runs top-level at `emain-platform.ts:251`, before the getters can be called by any importer. `RENAME_PLAN.md:807-813` states this placement is "provably first ... regardless of import order elsewhere". The gate is stricter than the placement rationale needs.
- Identical at origin/main (`emain-log.ts:69,97,106`), so not introduced. Correct severity: Info (plan-gate conformance nit, no ordering bug).

## TC-1

**VERDICT: HOLDS** (minor correction; upstream parity)

- `pkg/jobcontroller/jobcontroller_test.go:509-531`: no `t.Error`/`t.Fatal`/assert calls. The doc comment (507-508) says it "verifies that HandleSystemResume filters correctly", which is not checked.
- Correction: "passes whatever HandleSystemResume does" is too strong. A synchronous panic in `HandleSystemResume` (870+) would still fail the test. It detects panics only.
- Identical at origin/main (`jobcontroller_test.go:508-530`). Not introduced, but the branch added filtering to `HandleSystemResume` (`SuppressAutoReconnect`, 879-883) that this test does not cover. It also mutates the package global `hasRunningDurableJobsTestHook` under `t.Parallel()`.
