# Refutation: Go/emain claims, reachability lens

Commit `93aaefc3`. Checker lens: reachability and trigger. Each verdict gives the concrete input/state and the entry-point chain, or why none exists. The only thing executed was `go test -race -count=1 ./pkg/remote/conncontroller/` (3 runs, for CA-10). No source edits.

## SEC-1

**VERDICT: REFUTED**

The mechanics are right: `renameSync` at `emain/emain-platform.ts:162` moves a symlink as a link, and `existsSync` in `validateSource` follows it. But no trigger crosses a privilege boundary:
- The precondition (write access to the victim's home before first launch) already gives full control of that account (`~/.bashrc`, `~/.profile`, autostart entries).
- Migration adds nothing. The same attacker can plant the symlink at the *new* path (`~/.config/remoteterm`, `~/.local/share/remoteterm`) directly. `getRemoteTermConfigDir`/`getRemoteTermDataDir` (`emain-platform.ts:325-373`) follow it through `existsSync` with no migration involved.
- Before the rename, the pre-rename app also followed a symlink at the legacy path. Moving the link keeps that behaviour exactly as it was.

## SEC-2

**VERDICT: REFUTED**

It is true that `SetConnectionsConfigCommand` (`pkg/wshrpc/wshserver/wshserver.go:551`) passes `data.Host` to `SetConnectionsConfigValue` (`pkg/rtconfig/settingsconfig.go:749`) unvalidated. That is unchanged from `origin/main:551`. No harmful trigger exists:
- The value becomes a JSON object key through `jsonMarshalConfigInOrder`, so any string is escaped and valid. No path, shell or SQL sink sits on this route.
- Every caller of this RPC is authenticated and already holds stronger primitives on the same server: `FileWriteCommand` (`wshserver.go:371`) can write `connections.json` wholesale, and `ControllerInputCommand` (`:313`) types into terminals.
- `connections.json` is a user-editable file by design (the config editor view). Arbitrary keys already reach it without this RPC.
- "Only the frontend validates" is also inaccurate. `connstatusoverlay.tsx:756` sends `connName` with no validation, and the backend itself writes unvalidated names (`conncontroller.go:1172`). Only the quick-add path (`remotetermconfig-model.ts:672`) validates.

## CA-3

**VERDICT: HOLDS (Low; pre-existing upstream, same code at `origin/main:pkg/wconfig/settingsconfig.go:700`)**

`configWriteLock` is taken only inside `WriteWaveHomeConfigFile` (`settingsconfig.go:440`). The read at `:712` sits outside it. RPC requests each run in their own goroutine (`pkg/wshutil/wshrpc.go:434`), so two `SetConfigCommand`s can interleave read, merge and write.

Trigger: `wsh setconfig a=1 & wsh setconfig b=2` from a terminal (`cmd/wsh/cmd/wshcmd-setconfig.go` -> `wshserver.go:547` -> `SetBaseConfigValue`). Both read the old file and the last writer drops the other key. The window is microseconds, and no UI path issues concurrent `SetConfigCommand`s: all 9 frontend call sites are single click handlers. `SetConnectionsConfigValue` (`:749`) has the same shape.

## CA-4

**VERDICT: HOLDS (High for dev builds; the user's daily driver runs `task dev`)**

Trigger chain for a dev build on Linux:
1. The old instance is running with data at `~/.local/share/waveterm-dev` and holds a `flock` on `wave.lock`.
2. The new build launches, and `emain-platform.ts` evaluates `performDataDirMigration()` at `:245` before `requestSingleInstanceLock()` (`emain.ts:264`). `validateSource` sees `wave.lock` (`:213`), and `renameSync` succeeds on Linux while the old process holds open fds.
3. The single-instance lock does not detect the old app. The new userData is `~/.config/remoteterm/electron` (`app.setName("remoteterm/electron")`, `:17`) and the old one is `~/.config/waveterm/electron`. In dev the config root is `~/.config/waveterm-dev`, so the old Electron dir is not moved with it.
4. The new server's `AcquireWaveLock` (`pkg/remotetermbase/wavebase-posix.go:16-29`) flocks `remoteterm.lock` (`wavebase.go:134`), a different file and inode from the old `wave.lock`, and succeeds. Both servers then use the same `db/` inodes.

Had the lock name stayed `wave.lock`, the new server would have opened the moved inode and got `EWOULDBLOCK`, so the rename of the lock file is what defeats the guard.

Severity correction: on a Linux **prod** build, `~/.config/waveterm/electron` (with Chromium's `SingletonLock`) lies inside the config root that gets moved. The new instance then probably finds the old singleton and quits. Even so, the migration has already run under a live instance. That path is inferred, not run.

## CA-5

**VERDICT: HOLDS (Medium; branch-introduced)**

In `getRemoteTermHomeDir` (`emain-platform.ts:282-306`), a dev build (`!app.isPackaged`) with no `REMOTETERM_HOME`/`WAVETERM_HOME` checks `~/.remoteterm-dev/wave.lock`, then `~/.waveterm-dev/wave.lock`, then falls to bare `~/.waveterm` (`:298`). If that holds `wave.lock` (a prod install that still uses the legacy combined home and has not yet been migrated by a prod launch, or a real Wave Terminal install), `getRemoteTermDataDir()` returns it. It is then passed to the Go server as `REMOTETERM_DATA_HOME` (`emain-remotetermsrv.ts:71`), so the dev build runs on prod data.

The intent comment's premise (`:38-41`: "so a dev build can still recognise a genuinely old, pre-suffix, bare `.waveterm` install") is false. Upstream `origin/main:emain/emain-platform.ts:74-87` built only `.${waveDirName}`, which is `.waveterm-dev` in dev, so dev builds never read bare `~/.waveterm`. Commit `d6666d49` added the chain.

This machine currently has no `~/.waveterm*` (read-only `ls`), so it is not live here.

## CA-6

**VERDICT: HOLDS (Low-Medium)**

`migrateDataRoot` returns silently at `emain-platform.ts:136-138` when `validateSource()` is false, with no `recordMigrationFailure` and no log. Only the config root is affected. The data and legacy-home roots validate on `wave.lock`, which the old server always creates.

Nothing seeds `settings.json` on first run: the only writer is `SetBaseConfigValue`, reached only from explicit user actions or the wsh-install "don't ask again" checkbox (`conncontroller.go:1184`).

Trigger: a user who added SSH hosts (quick-add writes `connections.json` through `SetConnectionsConfigValue`) or edited `widgets.json`, but never changed a setting. After upgrade, `~/.config/waveterm` is skipped, `getRemoteTermConfigDir` creates an empty `~/.config/remoteterm`, and the connections vanish from the UI with no dialog.

## CA-7

**VERDICT: HOLDS (Medium)**

`ReconnectJob` uses `reconnectConnGroup` and `ReconnectJobRoute` uses `reconnectRouteGroup` (`pkg/jobcontroller/jobcontroller.go:1998-2010`). They are separate singleflight groups, so they do not dedupe against each other.

Trigger, on the normal reconnect path. The entry points are `blockcontroller.go:263` (startup), `durableshellcontroller.go:184`, `wshserver.go:1466`, and `jobcontroller.go:762/851/2182`. Each enters through `ReconnectJob`, then `doReconnectJob`:
1. `RemoteReconnectToJobManagerCommand` succeeds, the job manager registers its route, and the router publishes `route:up` (`wshrouter.go:845`).
2. `handleRouteEvent` (`:558`) runs `SetJobConnStatus(Connected)` (`:563`). Stream health is still inactive: `restartStreaming` never seeds `jobStreamHealth` (only `registerNewJobStream` at `:1499` does, and only `StartJob` calls it). So the handler spawns `ReconnectJobRoute` (`:590`).
3. That `doReconnectJob` passes `CheckJobConnected` (the status is now Connected), sees no active stream, and calls `restartStreaming` (`:2031`).
4. Meanwhile the first call returns from `WaitForRegister` (`:2115`) and also calls `restartStreaming` (`:2124`).

There is no per-job lock in `restartStreaming` (`:2209-2420`), and the window is wide: `waitForStreamLoopExit` can wait up to 1s, plus two RPCs with a 5s timeout. Both calls create readers and send `JobPrepareConnect`. The outcome depends on interleaving: at best one stream is superseded; at worst `jobStreamIds` and the job manager's active stream disagree, which is the Connected-but-no-stream wedge.

The comment at `:577-581` ("so it can't race a concurrent doReconnectJob") is false.

## CA-8

**VERDICT: HOLDS (Low-Medium)**

At `jobcontroller.go:2031-2036`, when `restartStreaming` fails in the Connected-but-no-stream branch, `SetJobConnStatus(Disconnected)` runs with no `sendBlockJobStatusEventByJob`. `SetJobConnStatus` (`:1435`) only updates the map. The sibling failure branch at `:2124-2128` does send the event, which confirms the omission.

Trigger: route-up. The handler has already published `"connected"` (`:568`) and spawns `ReconnectJobRoute`. `restartStreaming` then fails, for example with a `JobPrepareConnectCommand` timeout (`:2320`) or a `JobStartStreamCommand` error (`:2406`; in that case `setJobDrainProgress` at `:2331` has already re-published "connected"). The UI stays on "connected" until some unrelated event arrives. No periodic republish exists; the frontend listens to `block:jobstatus` events (`term-model.ts:368`).

## CA-9

**VERDICT: HOLDS (Low)**

After `registerNewJobStream` (`jobcontroller.go:1610`), the error returns at `:1618` (`MakeFile`) and `:1650-1660` (`RemoteStartJobCommand` failure) close neither `reader` nor reset `jobStreamHealth.active`.

Trigger: start a durable remote shell while the connection route drops or times out (30s, `:1641`).

Severity correction:
- The reader leak predates this branch: `origin/main` also set `jobReaders` there without closing. The branch added only `active=true` (`58bad6e2`).
- Stale `active=true` matters only if that job's route later comes up. That happens only when `RemoteStartJobCommand` timed out locally but the manager did start remotely. There it makes route-up log "stream already active" and skip the restart, for a job already marked `Done/StartupError`.
- `jobStreamHealth`, `jobReaders` and `jobStreamIds` are never deleted anywhere in the file, so per-job map growth is general, not specific to this path.

## CA-10

**VERDICT: HOLDS (Low; test-only)**

Reproduced. `go test -race -count=1 ./pkg/remote/conncontroller/` FAILs every run; 3 runs gave 1, 2 and 1 `DATA RACE` warnings, so the count is intermittent, not a fixed 2.
- Race A: the test's unlocked read of `cm.KeepAliveInFlight` (`conncontroller_test.go:448`, inside the cited 444-450) against `clearKeepAliveInFlight` (`connmonitor.go:119`, under `cm.lock`), which runs from the goroutine `SendKeepAlive` spawns (`:137`).
- Race B, outside the cited lines: a test writes the package-global `getConnectionConfigTestHook` in a defer (`conncontroller_test.go:1878`). Meanwhile a goroutine from `closeWithLifecycleLock` (`conncontroller.go:471`), left over from an earlier test's close, reads it at `conncontroller.go:1943`.

In both races the racing write or read is test code or a test-only hook, which matches "both in test code". Production does not read `KeepAliveInFlight` unlocked there.

## CONF-1

**VERDICT: HOLDS**

The premise checks out: `RENAME_PLAN.md:439` (decision 5, under "All Prerequisites decisions — RESOLVED 2026-09-20") says remote-host `.waveterm` state **renames**. The code still uses `.waveterm`:
- `RemoteWaveHomeDirName`, `RemoteFullWshBinPath` and `RemoteFullDomainSocketPath` (`pkg/remotetermbase/wavebase.go:139,141,142`)
- `GetPersistentRemoteSockName` (`:264`)
- `GetRemoteJobLogDir` (`:569`)
- `getWshPath` (`pkg/wshrpc/wshremote/wshremote.go:134`)

`RENAME_ALLOWLIST.md:76-91` does not exempt them; it covers only the local shim's `.waveterm`/`wave.lock` literals. The Phase 3 commit `668a8eda` renamed only marker filenames and claims the remote side self-heals. It is reachable on every SSH/WSL connection. Correction: this is a conformance gap, not a functional fault. Keeping `.waveterm` is the no-reinstall behaviour, so the fix could be either the code or the decision.

## CONF-2

**VERDICT: REFUTED (the harm has no trigger; the letter-of-gate violation stands as a doc nit)**

`emain-log.ts:96-97,106` does call `getRemoteTermDataDir()` at module scope. That breaks the literal Phase 2 verification check (`RENAME_PLAN.md:813-817`).

The claimed consequence, "could resolve the data dir before migration runs", cannot happen. `emain-log.ts:8` imports the getter from `./emain-platform`, and ES module semantics run a dependency's top level to completion before the importer's body. `performDataDirMigration()` is a top-level statement at `emain-platform.ts:245`, and `emain-platform.ts` imports nothing that leads back to `emain-log`: its imports are `@/util/util`, `electron`, `env-paths`, `fs`, `os`, `path`, `frontend/util/isdev` and `keyutil`. The plan itself (`:807-813`) says this placement is "provably first" for every importer, so the gate is stricter than the risk it guards.

## TC-1

**VERDICT: HOLDS (Low)**

`TestHandleSystemResumeSmoke` (`pkg/jobcontroller/jobcontroller_test.go:509-531`) calls `HandleSystemResume(ctx)` and makes no assertion afterwards: no check of scheduler state, reconnect attempts or conn status. Correction: it is not quite "passes whatever": a panic would still fail it.

It also runs `t.Parallel()` while assigning the package-global `hasRunningDurableJobsTestHook`. Any other parallel test that touches that hook races it, the same pattern as CA-10 race B.
