# Refute Go claims: reproduction lens

Worktree: `scratchpad/repro-go` @ `93aaefc3`. All filesystem tests use temp dirs (`HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME` pinned to a temp dir; Go tests use `t.TempDir()`/`os.MkdirTemp`). No app, daemon, or real config touched. Saved regression tests: `scratchpad/repro-tests/`.

Abbreviations: `$S` = `/tmp/claude-1000/-media-owner-Workspace-remoteterm/6b9bb981-3b3e-4d9c-9a93-b16272c3469e/scratchpad`.

## SEC-1

VERDICT: HOLDS (mechanism); no security delta found

- Test: `$S/repro-tests/SEC-1_CA-5_CA-6_emain_repro-migration.test.ts` ("SEC-1" block)
- Command: `npx vitest run emain/repro-migration.test.ts`
- Observed: `[SEC-1] dest isSymlink=true realpath=/tmp/rtrepro-1YONWw/attacker-target`; assertion fails as predicted.
- Note: before migration, the old code already read and wrote through the same symlink at `~/.config/waveterm`, so the move changes the link's name, not where the I/O goes. The precondition (write access to the user's home) already allows editing dotfiles directly. Severity should be Low/Info.

## SEC-2

VERDICT: HOLDS

- Test: `$S/repro-tests/CA-3_SEC-2_rtconfig_repro_test.go` (`TestReproSEC2ConnNameUnvalidated`)
- Command: `go test -run TestRepro -count=1 -v ./pkg/rtconfig/`
- Observed: `SEC-2 reproduced: 6/6 malformed conn names persisted` (`""`, `"../../etc"`, `"a\nb"`, `"has space"`, `"x\x00y"`, `"user@host:22; rm -rf ~"`).
- Note: the key is only a JSON map key, and the caller is already a trusted RPC peer. Nothing was shown to turn it into path or command injection, so the impact is data-integrity and robustness. `SetConnectionsConfigValue` is unchanged from upstream (`origin/main:pkg/wconfig`), apart from the package rename.

## CA-3

VERDICT: HOLDS

- Test: same file, `TestReproCA3ConcurrentSetBaseConfigValueLosesKeys` (33 goroutines, each setting a distinct bool key, 20 iterations)
- Command: `go test -run TestRepro -count=1 -v ./pkg/rtconfig/`
- Observed: `CA-3 reproduced: 608 keys lost across 20 iterations of 33 concurrent writers`. `configWriteLock` covers the write only; the read happens before the lock is taken.
- Note: this is pre-existing upstream code (`origin/main` has the same function).

## CA-4

VERDICT: HOLDS (lock non-exclusion reproduced; the two-process end-to-end run was not done)

- Test: `$S/repro-tests/CA-4_remotetermbase_repro_lock_test.go`. It flocks `wave.lock` (the old server's lock), renames the dir as the emain shim does, then calls `AcquireWaveLock()`.
- Command: `go test -run TestReproCA4 -count=1 -v ./pkg/remotetermbase/`
- Observed: `CA-4 reproduced: new server acquired remoteterm.lock while old server still holds wave.lock in the same dir`. `rename(2)` succeeds while the old fd is held.
- Not run: a real old-version server plus a new one. The migration runs before `requestSingleInstanceLock`, and the Electron app name changed, so the old and new instances do not exclude each other at the Electron level either. SQLite's own POSIX locks follow the inode, so they still serialise raw DB writes. The failure mode is two servers, each with its own in-memory caches, sharing one DB and one filestore.

## CA-5

VERDICT: HOLDS (introduced by the branch)

- Test: `SEC-1_CA-5_CA-6_emain_repro-migration.test.ts` ("CA-5" block, `app.isPackaged=false`, temp `~/.waveterm/wave.lock`)
- Observed: `[CA-5] isDev=true dataDir=/tmp/rtrepro-m8X2AJ/.waveterm`; assertion fails.
- `origin/main` resolved dev to `~/.waveterm-dev` only. The branch's bare-`.waveterm` fallback in `getRemoteTermHomeDir` (lines 298-307) now points dev builds at a production data dir.

## CA-6

VERDICT: HOLDS

- Test: same file, "CA-6" block (legacy `$XDG_CONFIG_HOME/waveterm/connections.json`, no `settings.json`, prod)
- Observed: `configDir=.../.config/remoteterm connections.json present=false legacyStillThere=true failures=[] migrationLogs=[]`. The directory is not migrated, nothing is logged, and no failure is recorded. The app then runs on a fresh empty config dir.

## CA-7

VERDICT: HOLDS

- Test: `$S/repro-tests/CA-7_CA-8_CA-9_jobcontroller_repro_test.go` (`TestReproCA7ConcurrentRestartStreaming`). It uses a real temp wstore and filestore, plus a fake `job:<id>` route whose `JobPrepareConnectCommand` holds for 500ms and records the peak number of calls in flight.
- Command: `go test -run TestRepro -count=1 -v ./pkg/jobcontroller/`
- Observed: `max concurrent JobPrepareConnect for one job = 2` / `CA-7 reproduced: 2 restartStreaming calls in flight at once`.
- Negative control: with both calls routed through `ReconnectJob` (the same group), `max = 1` and the test PASSES. The comment at jobcontroller.go:577-581 ("can't race a concurrent doReconnectJob") is therefore wrong.

## CA-8

VERDICT: HOLDS

- Test: same file, `TestReproCA8DisconnectedWithoutStatusEvent`. The job is Running, the status is Connected, there is no stream, and the prepare RPC fails (no route).
- Observed: `statuses published: before=[connected] after=[]`, and `CA-8 reproduced: backend status Disconnected, last published status "connected"`. None of `ReconnectJob`'s callers publish either. The sibling path at jobcontroller.go:2131-2132 does publish.

## CA-9

VERDICT: HOLDS

- Test: same file, `TestReproCA9StartJobErrorLeaksStreamHealth` (`StartJob` on `local`; `RemoteStartJobCommand` fails with no route)
- Observed: `job ... status=done health.active=true healthOk=true readerRegistered=true`, then `CA-9 reproduced`.
- Note: the reader leak is inherited from upstream. Leaving `health.active=true` is new with `registerNewJobStream`. Nothing ever deletes these map entries. The impact is small because the job is marked Done.

## CA-10

VERDICT: HOLDS, with corrections

- Command: `go test -race -count=1 ./pkg/remote/conncontroller/`, run 6 times. Log: `$S/repro-tests/CA-10_race_run4.log`
- Observed: exit 1 every run. The race count varies from 1 to 3 per run, so "2 races" is not stable. There are three distinct sites:
  1. `conncontroller_test.go:448`: the test reads `cm.KeepAliveInFlight` without a lock while the `SendKeepAlive` goroutine clears it (connmonitor.go:119).
  2. `conncontroller_test.go:2439`: the test writes a package-level test hook while a goroutine leaked from an earlier test (`disconnectOnStall` -> `closeWithLifecycleLock`, conncontroller.go:470) reads it.
  3. `conncontroller_test.go:1878`: the same pattern via `getConnectionConfig` (conncontroller.go:1943).
- In every site, test code is the unsynchronised side. The production accessors involved are lock-guarded or are test hooks, so "in test code" holds.

## CONF-1

VERDICT: HOLDS

- Test: `$S/repro-tests/CONF-1_remotetermbase_repro_conf1_test.go`
- Command: `go test -run TestReproCONF1 -count=1 -v ./pkg/remotetermbase/`
- Observed: `RemoteWaveHomeDirName=".waveterm"`, `RemoteFullWshBinPath="~/.waveterm/bin/wsh"`, `RemoteFullDomainSocketPath="~/.waveterm/wave-remote.sock"`, `GetPersistentRemoteSockName="~/.waveterm/client/cid/waveterm.sock"`, then `CONF-1 reproduced`. `wshremote.go:134` hardcodes `~/.waveterm/bin/wsh`. `RENAME_ALLOWLIST.md` has no entry for the remote paths. I did not check whether the phase that carries this rename is intentionally still pending; that belongs to the premise lens.

## CONF-2

VERDICT: REFUTED (harm); the literal gate text is violated

- Test: same emain file, "CONF-2" block. It imports `emain-log.ts` with a legacy `$XDG_DATA_HOME/waveterm` (`wave.lock` + sentinel).
- Observed: `[CONF-2] migrated=true marker=true logInNew=true legacyRecreated=false`. The test PASSES.
- `emain-log.ts` imports its getter from `emain-platform.ts`, whose top-level `performDataDirMigration()` therefore runs first. This is the placement argument in RENAME_PLAN.md:807-814. The data dir cannot resolve before migration. The verification-gate grep does hit `emain-log.ts:97,106` at module scope, so a gate cleanup item remains, but it is not a defect.

## TC-1

VERDICT: HOLDS

- Mutation: insert `if true { return }` at the top of `HandleSystemResume` (reverted with `git checkout` afterwards).
- Command: `go test -count=1 -run TestHandleSystemResumeSmoke -v ./pkg/jobcontroller/`
- Observed: `--- PASS: TestHandleSystemResumeSmoke`. The mutant survives, so the test asserts nothing about behaviour.
