# PR #65 follow-up audit

Scope: `git diff origin/main...HEAD` on `fix/kitty-lockfile-and-postinstall` (`04739796`, `1237fab1`): `postinstall.cjs`, `package-lock.json`, `docs/qa-review/pr65-followup-fixes.md`.

## Summary

The fix is correct and does what it says. A fresh `npm ci` has no chicken-and-egg problem. `CF_PAGES` is a real Cloudflare variable and the claim about where it came from checks out. Tests and typecheck pass. One interaction outside the diff needs attention: Dependabot will now open PRs that fail at install.

## Findings

### [Low] Dependabot will open grouped PRs that fail at install

- **Location:** `.github/dependabot.yml` (no `ignore` entry for `@xterm/addon-image`)
- **Issue:** `package.json` pins `@xterm/addon-image` to `0.10.0-beta.287`. The npm `beta` dist-tag is already at `0.10.0-beta.301`. Dependabot bumps prereleases for packages already on a prerelease. When it does, `patches/@xterm+addon-image+0.10.0-beta.287.patch` will fail with `--error-on-warn` (version mismatch), or with `--error-on-fail` if the context lines have drifted. Before this branch, CI swallowed that failure and the PR went green with an unpatched addon. Now the PR goes red, which is the right signal. But the prod-dependency groups in `dependabot.yml` would also block every unrelated bump grouped with it.
- **Impact:** Weekly grouped Dependabot PRs stay red until someone regenerates the patch by hand. Unrelated security bumps in the same group get held up.
- **Fix:** Add this to the root npm entry in `.github/dependabot.yml`:
  ```yaml
  ignore:
      - dependency-name: "@xterm/addon-image"
  ```
  Bumps to this package then become manual, and the patch is regenerated as part of each bump. That matches the one-exact-pin-per-patch convention the fix enforces.

### [Info] Error message assumes the patch was the cause

- **Location:** `postinstall.cjs:17-22`
- **Issue:** The `catch` fires on any non-zero exit from `npx patch-package`, including patch-package crashing or `npx` failing to resolve it. The message says it "reported a failed patch or a version mismatch". Otherwise the message is accurate: `patches/` exists and contains the single patch, and the exact pin is in `package.json`.
- **Impact:** Minor. patch-package's own output is printed above via `stdio: "inherit"`, so the real cause is still visible.
- **Fix:** Optional wording change: "postinstall: patch-package failed (failed patch, version mismatch, or patch-package error); stopping...".

## Focus-area results

1. **Chicken-and-egg on a fresh install: none.** npm runs the root `postinstall` only after every dependency is extracted and linked. `npm ci` always deletes `node_modules` first, so the prior agent's case E already came from an empty tree. To check this independently, I copied only `package.json`, `package-lock.json`, `postinstall.cjs` and `patches/` into a scratch directory with no `node_modules` and ran `REMOTETERM_SKIP_APP_DEPS=1 npm ci`. Result: exit 0, `@xterm/addon-image@0.10.0-beta.287 ✔`. patch-package 8.0.1 needs only its own declared dependencies (chalk, ci-info, minimist, etc.), and npm installs those as part of the tree. `node_modules/.bin` is on PATH during lifecycle scripts, so `npx` resolves the local binary and does not fetch from the network.
2. **Other install sites:**
   - Every workflow that installs uses `npm ci --no-audit --no-fund`: `build-helper`, `build-linux-ci`, `build-macos-ci`, `build-macos`, `bump-version`, `codeql`, `copilot-setup-steps`. On CI, `ci-info` already made patch-package exit 1, and only the `catch` hid it. So the new behaviour on CI is: a failed patch now fails the job, and a version mismatch now fails it too (`--error-on-warn`). The patch applies cleanly today, so none of these break.
   - No workflow uses `--ignore-scripts`, `--omit=dev` or `NODE_ENV=production`. So the dev-only skip path in patch-package does not come into play.
   - In `Taskfile.yml`, `npm:install` and `init` run the root `npm install`, which is now fatal locally when a patch fails. That is intended. The `docs/` and `tsunami/frontend/` `package.json` files have no `postinstall`, so the `cd docs && npm install` and `scaffold` installs are unaffected.
   - There is no Dockerfile. `BUILD.md:74` says only "run `npm install`", and `AGENTS.md:50` describes the same `npm ci → postinstall → patch-package` chain. Neither needs changing.
   - Dependabot is the one consumer that currently tolerates a failed patch. See the Low finding.
3. **`CF_PAGES`: real, and its origin is confirmed.** The [Cloudflare Pages build configuration docs](https://developers.cloudflare.com/pages/configuration/build-configuration/) list `CF_PAGES=1` and `CI=true` as system variables injected by default. Upstream `9abd5901` ("UI only preview server (+ deployments) (#2919)", Mike Sawka, 2026-02-23) is reachable in this fork's history. It adds 11 lines to `postinstall.cjs`, and `git log -S CF_PAGES -- postinstall.cjs` returns only that commit. The `=== "true"` branch is harmless leniency. Because Pages also sets `CI=true`, patch-package already exits 1 there on a failed patch, and the `isCfPages` branch catches it and continues as intended.
4. **Error message:** accurate apart from the Info note above. `--error-on-warn` is safe to add. In 8.0.1, the only thing `applyPatches.js` pushes to `warnings` is the version-mismatch warning (line 180), so the flag cannot trip on anything unrelated.
5. **Verification I ran myself:** `npx tsc --noEmit` exit 0. `npx vitest run`: 41 files and 425 tests passed. `prettier --check postinstall.cjs`: clean. The results match the prior agent's figures.

## Verified OK

- The lockfile diff is one line: the caret is removed from the `packages[""]` mirror. The resolved entry and integrity hash are unchanged.
- The `CF_PAGES` path warns, then exits 0 through `skip` before `install-app-deps`. The `REMOTETERM_SKIP_APP_DEPS` path is fatal when a patch fails and skips only `install-app-deps`.
- The single synchronous `execSync("electron-builder install-app-deps")` replaces the `import()` promise that had no rejection handler. A failure there now exits non-zero cleanly.
- There is no shell interpolation of external input in either `execSync` string.

AGENT_VERDICT: PASS
