# security-auditor report

**Target:** `git diff origin/main...HEAD` on `qa/fleet-2026-09-22` (RemoteTerm daily-driver branch: Electron/React/TS + Go backend, fork of Wave Terminal)
**Started:** 2026-09-22T19:57:36Z
**Status:** IN PROGRESS

## Findings
<!-- appended one at a time, as found -->

### SEC-1 [Medium] [Confidence: Medium] Data-dir migration shim follows symlinks at the source root, letting a pre-existing planted symlink get relocated into the trusted destination path
- **Location:** `emain/emain-platform.ts:121-185` (`migrateDataRoot`), called from `performDataDirMigration` (:187-249)
- **Description:** `migrateDataRoot` checks `existsSync(spec.source)` and `spec.validateSource()` (both symlink-following) then calls `renameSync(spec.source, spec.dest)`. `rename(2)` does not dereference a symlink at `oldpath` — it moves the link itself. If the legacy root (e.g. `~/.config/waveterm` or, for the `data`/`legacy-home` roots, the XDG data dir / `~/.waveterm`) is a symlink rather than a real directory at the moment this one-time, unconfirmed migration runs, the *link* — not its target — gets renamed into the new trusted location (`~/.config/remoteterm`, `paths.data`, `~/.remoteterm`). Every subsequent config/data read-write the app performs through `getRemoteTermConfigDir()`/`getRemoteTermDataDir()` (both of which do plain `fs` calls that follow symlinks) then transparently operates on whatever path the attacker's symlink points at, including files that will later hold the swap-token/JWT-bearing `wave.lock` state and any secrets the OS-keychain-backed Secrets tab writes through this same data root's book-keeping.
- **Impact:** A local actor able to write into the target user's `$XDG_CONFIG_HOME`/`$XDG_DATA_HOME`/`$HOME` *before* the user's first post-rename launch (shared/multi-tenant machine with a shared or world-writable XDG dir, a CI/container image built by an untrusted prior stage, or any other actor with a narrow pre-first-launch write window) can redirect where RemoteTerm's config/data root resolves to after migration — e.g. onto a location they can read (exfiltrating whatever gets written there next) or onto a location that's meaningful elsewhere on the box. `validateSource` (`existsSync(configSource/settings.json)`, `existsSync(dataSource/wave.lock)`) raises the bar — the attacker's symlink target must already contain a plausible `settings.json`/`wave.lock` — but does not require ownership of that target, and is itself evaluated by following the same symlink, so it does not defend against this class of attack.
- **Proof of concept (not run, live-system rule):** Before first launch of a rebranded build: `ln -s /tmp/attacker-owned-dir ~/.config/waveterm` where `/tmp/attacker-owned-dir/settings.json` exists (attacker-created) and, for the data root, `ln -s /tmp/attacker-owned-data ~/.local/share/waveterm` with a planted `wave.lock`. On launch, `migrateDataRoot` moves the symlinks into `~/.config/remoteterm` / the new data path; the app then reads/writes its live config and data (later including anything the Secrets tab or connection state persists to that root) through the attacker-controlled target.
- **Recommendation:** Use `lstatSync(spec.source)` before migrating and refuse (record as a migration failure, do not silently proceed) if `isSymbolicLink()` is true, for every root. This closes the gap without weakening the existing marker/validate logic:
  ```ts
  if (existsSync(spec.source)) {
      const st = lstatSync(spec.source);
      if (st.isSymbolicLink()) {
          recordMigrationFailure(`refusing to migrate ${spec.name} root: ${spec.source} is a symlink`);
          return;
      }
  }
  ```

### SEC-2 [Low] [Confidence: Medium] `SetConnectionsConfigCommand` writes the connection name into `connections.json` with no server-side format validation
- **Location:** `pkg/wshrpc/wshserver/wshserver.go:551-553` (`SetConnectionsConfigCommand`) → `pkg/rtconfig/settingsconfig.go:749-766` (`SetConnectionsConfigValue`)
- **Description:** The frontend's Connections quick-add (`remotetermconfig-model.ts:667-683`, `submitConnectionQuickAdd`) validates the entered host against `ConnectionQuickAddRegex` before calling `SetConnectionsConfigCommand`, but that is a client-side check only. `SetConnectionsConfigValue` takes `connName` and uses it directly as a `MetaMapType` key (`m[connName] = connData`, :764) with no format check of its own, unlike `pkg/remote/connutil.go:29-39` (`ParseOpts`), which independently re-validates the `user@host[:port]` shape at actual connect time via `userHostRe`.
- **Impact:** Bounded — any wshrpc call still requires the local auth key (`pkg/authkey`), so this is not reachable by an unauthenticated network attacker. But it means the format guarantee for `connections.json` keys rests entirely on a UI-layer regex duplicated in TypeScript; any other RPC caller inside the trust boundary (another local process holding the authkey, a future UI surface that adds a connection by a different path) can write an arbitrary string as a connection name/key into the config file. `ParseOpts`'s independent re-validation at connect time means this does not translate into an SSH argument-injection primitive today (connections use `golang.org/x/crypto/ssh`, not an exec'd `ssh` binary), so impact is limited to a malformed/oversized `connections.json` entry rather than command execution.
- **Recommendation:** Validate `connName` in `SetConnectionsConfigValue` (or `SetConnectionsConfigCommand`) against the same pattern `ParseOpts` uses, and reject before writing, so the two validators can't drift and the config file itself is a valid representation of what the app can actually connect to.

## Verified OK
<!-- appended as checked -->

## Verified OK
<!-- appended as checked -->

- `pkg/authkey/authkey.go:22` (bb79c27d): `ValidateIncomingRequest` now uses `subtle.ConstantTimeCompare` instead of `!=`. Correctly implemented.
- `pkg/shellexec/shellexec.go` `StartWslShellProc` (:184-309) and `StartRemoteShellProc` (:355-493): both build `cmdCombined` in two phases — the unconditional `conn.Infof` logging the assembled shell command (:259, :432) happens *before* the swap token / JWT are string-interpolated into `cmdCombined` (packedToken injected :272/:472, JWT :277-282/:477-482). The only log site that fires *after* injection is `conn.Debugf` on the redacted copy (:294, per equivalent). No unconditional log path reaches the post-injection secret-bearing string. `SessionWrap.StartCmd` (pkg/shellexec/conninterface.go:146,201) stores the secret-bearing command only to hand to `session.Start()`, never logged.
- `pkg/blockcontroller/shellcontroller.go` and `durableshellcontroller.go`: every `Token`/`jwtStr` reference is either an `fmt.Errorf` wrapping an unrelated error, a `RedactSecret()`-wrapped `Debugf` (:426), or a `SetDualEnv` write into the in-memory swap-token env map (never logged). No raw-secret log site found.
- `cmd/wsh/cmd/wshcmd-root.go`, `pkg/remotetermapp/waveapp.go`, `cmd/wsh/cmd/wshcmd-connserver.go`: references to `WaveJwtTokenVarName`/`wshutil.WaveJwtTokenVarName` in log/error strings are the *env var name* (e.g. `REMOTETERM_JWT`), not the token value.
- `pkg/remotetermbase/wavebase.go:93-116` (`SetDualEnv`/`GetEnvNewOrLegacy`/`GetMapValNewOrLegacy`) and all 11 call sites: dual-write/dual-read for the JWT is symmetric — every write site writes both `REMOTETERM_JWT` and `WAVETERM_JWT`, every read site checks new-then-legacy. `cmd/server/main-server.go:157-158` (`grabAndRemoveEnvVars`) unsets both names on startup so a leaked pre-rename env var doesn't persist into the server process's own environment.
- `pkg/wshutil/wshrouter.go:25-29,619-621`: the `wavesrv`→`remotetermsrv` route-name dual-accept is a single lookup-time alias inside `getLinkForRoute`, applied only when resolving the destination of an already-authenticated message. Grepped all other `DefaultRoute` references (`wshrouter.go:571`, `cmd/server/main-server.go:123`, `cmd/test-conn/testutil.go:99`) — none of them do a raw string-equality trust check that the alias could route around; there is exactly one server route regardless of which name addresses it, so accepting both names doesn't add a second trust surface.
- `emain/emain-platform.ts` `migrateDataRoot`/`performDataDirMigration` (:121-249): `renameSync` preserves the moved directory's own permission bits (no permission-widening). ENOENT-during-move is correctly treated as "another process already completed it" only when the destination's marker file is also present (:172-176), not on ENOENT alone — so a genuine failure still gets surfaced via `recordMigrationFailure`/`getMigrationFailures()`, not silently swallowed as the pre-review code apparently did (per d6666d49's commit message).
- `pkg/rtconfig/settingsconfig.go:749-766` (`SetConnectionsConfigValue`): writes go through `WriteWaveHomeConfigFile` → `fileutil.AtomicWriteFile`, JSON-only, no shell/exec invocation. Connections are established through `golang.org/x/crypto/ssh` (native client, `pkg/remote/connutil.go:26`), not by exec'ing a system `ssh` binary with the host string — no argument-injection path from a malicious host string even if one reached this far.
- Frontend `remotetermconfig-model.ts` secret flows (`viewSecret`, `showSecret`, `saveSecret`, `deleteSecret`, `addNewSecret`, :522-653): all error paths interpolate only `error.message`, never the secret value itself, into `errorMessageAtom`. No `console.*` call anywhere in the file references `secretValueAtom`/`secretValueRef`.

- `pkg/wshrpc/wshserver/wshserver.go:1386-1431` (`GetSecretsCommand`/`SetSecretsCommand`/`GetSecretsNamesCommand`): error strings interpolate only the secret *name* (`%q`) and wrapped `err`, never the value. `pkg/secretstore/secretstore.go` diff in this branch is a mechanical `wavebase`→`remotetermbase` import rename only — the actual encryption/keychain logic, `writeSecretsToFile`'s `0600` file mode, and `getLinuxStorageBackend` gating are unchanged from `origin/main`.
- `pkg/rtconfig/settingsconfig.go:439-450` (`WriteWaveHomeConfigFile`): writes `settings.json`/`connections.json` at `0644` — appropriate, since these files hold connection metadata/timing config, not secrets (secrets go through the separate `0600` keychain-backed store above).

## Completion
**Status:** COMPLETE
**Findings:** 2 (0 Critical, 0 High, 1 Medium, 1 Low)
**Not checked / out of scope:** No live-app or `task dev` runtime verification of the migration shim (live-system rule); SEC-1 is reasoned from `rename(2)`/`fs` symlink semantics against the committed code, not exercised in a sandbox. Windows-specific `windows-userdata` migration root (emain-platform.ts:235-245) not separately re-audited beyond the shared `migrateDataRoot` path it calls into.
