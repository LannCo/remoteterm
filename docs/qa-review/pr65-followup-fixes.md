# PR #65 follow-up fixes

Branch `fix/kitty-lockfile-and-postinstall`, cut from `origin/main` after #65 merged. These fixes address finding 3/4 in `pr65-code-auditor.md`.

## Fix 1: lockfile root-mirror drift

`package.json` pins `@xterm/addon-image` to `0.10.0-beta.287` exactly. The lockfile's `packages[""].dependencies` mirror still said `^0.10.0-beta.287`. I ran `npm install --package-lock-only` and it changed one line: the caret was removed. The resolved entry and its integrity hash did not change. `npm ci --dry-run` exits 0.

## Fix 2: patch-package failures are now fatal

### Findings

- **The try/catch never ran locally.** patch-package 8.0.1 exits 0 when a patch fails, unless `--error-on-fail` is passed. It only turns that flag on by itself under CI, detected by `ci-info` (`dist/index.js:91-95`). So a local install that failed to apply the patch printed an error and carried on. On CI, the patch-package process did exit 1, and the `catch` hid it.
- **Why it was made non-fatal.** `6ccbb74c` says "patch-package runs non-fatal so CI doesn't break if it fails". That was added in the same commit that switched to `require()` for patch-package and dropped a platform-specific tailwind dependency that broke macOS CI. The patch was also being rewritten many times during that work. Neither reason applies now: the version is pinned exactly and the patch applies cleanly on a fresh `npm ci`.
- **What `CF_PAGES` builds.** The env check came from upstream `9abd5901` ("UI only preview server"). It covers the Cloudflare Pages deploy of `frontend/preview` (`task build:preview`, `vite build`), a static component preview with no Electron and no backend. A scratch build shows the preview does bundle `@xterm/addon-image` (it comes in through `mock-node-model`), so it is affected by the patch. But it is a UI preview, not the shipped app. No workflow in this repo sets `CF_PAGES` or `REMOTETERM_SKIP_APP_DEPS`.
- **What `REMOTETERM_SKIP_APP_DEPS` does.** It only skips `electron-builder install-app-deps` (the native module rebuild). Someone using it may still build the Electron app, so it still needs the patches.

### Change (`postinstall.cjs`)

- Runs `npx patch-package --error-on-fail --error-on-warn`. `--error-on-warn` turns a version-mismatch warning into a failure. This enforces the one-exact-pin-per-patch convention: after a dependency bump, the patch must be regenerated rather than applied to a version it was not made for.
- If this fails, the install exits 1, with a message pointing at `patches/` compared with the pinned and installed version. The one exception is `CF_PAGES`: there it logs a warning and continues, the same behaviour as before.
- `REMOTETERM_SKIP_APP_DEPS` still skips `install-app-deps` and nothing else. Patch failures are fatal on that path.
- The mixed `require` plus async `import()` is gone. `install-app-deps` now runs through a single synchronous `execSync`, so a failure there is an ordinary exception instead of an unhandled promise rejection.

### Verification

| Case | Command | Result |
|---|---|---|
| A: corrupted patch context line | `npm ci` | exit 1, `**ERROR** Failed to apply patch`, postinstall error message |
| B: corrupted, `CF_PAGES=1` | `node postinstall.cjs` | exit 0, non-fatal warning, install-app-deps skipped |
| C: corrupted, `REMOTETERM_SKIP_APP_DEPS=1` | `node postinstall.cjs` | exit 1 |
| D: patch renamed to `+0.10.0-beta.286` (version mismatch) | `node postinstall.cjs` | exit 1 on the mismatch warning |
| E: patch restored (byte-identical, `cmp`) | `npm ci` | exit 0, `@xterm/addon-image@0.10.0-beta.287 ✔`, native deps installed |
| Re-run on an already-patched tree | `npx patch-package --error-on-fail --error-on-warn` | `✔` (idempotent) |

`npx tsc --noEmit`: exit 0. `npx vitest run`: 41 files, 425 tests passed. `prettier --check postinstall.cjs`: clean.

### Still open

No CI workflow runs vitest (tracked separately). If the patch stops applying, CI installs will now fail. The patch's behaviour is still not tested in CI.
