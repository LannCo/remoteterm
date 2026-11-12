# code-auditor report: PR #65 (Kitty graphics chunked-transmission fixes)

**Target:** `pr/issue-56-kitty-graphics` vs `origin/main` (e80880d0). Scope: `package.json`, `patches/@xterm+addon-image+0.10.0-beta.287.patch`, `frontend/app/view/term/kitty-graphics-chunking.test.ts`.

(Kept separate from `code-auditor.md`, which holds the committed round-1 fleet report for the unrelated `qa/fleet-2026-09-22` branch — commit `35cd6146`. Do not overwrite that file.)

## What checks out
- **Patch vs pinned version:** downloaded `0.10.0-beta.287` tarball checksum matches `package-lock.json:10879`. Patch applies cleanly; pristine tarball + patch is byte-identical to installed `node_modules/@xterm/addon-image`.
- **Tests use the real code:** test imports the real `lib/addon-image.mjs`, no mocks, all 8 pass. Verified via decoder swap + `cmp`: pre-PR build fails 5/8 tests (3 pass as controls); first-commit build (550cd971) fails the 3 edge-case tests only.
- **State machine:** `KittyGraphicsHandler.ts:199-421` handles all 4 stated bug classes correctly. No new leak or double-release found. Minified runtime `.mjs` matches `src` changes.

## Findings
1. **[Medium, code-read, not measured]** Every padded chunk creates a fresh decoder allocating ~4MB WASM memory (`:366-373`, `:228-231`); the old decoder is left for GC. Chafa's per-512-byte padding means thousands of allocations per image — risk of stalls/OOM on large images. Fix: call `decoder.init()` after `data8.slice()` and reuse the same decoder. Add a test streaming ~2000 padded chunks.
2. **[Low]** CommonJS build `lib/addon-image.js` (package's `main` entry) has none of the Kitty fixes and fails to parse (`Unexpected token ','`) — pre-existing on `origin/main`. Not user-facing: renderer loads the ESM build.
3. **[Low]** `package-lock.json:26` still says `^0.10.0-beta.287` while `package.json` now pins the exact version — unclear if `npm ci` rejects this. Fix: `npm install --package-lock-only` and commit the lockfile.
4. **[Info]** CI workflows don't appear to run vitest (grep found nothing); `postinstall.cjs:4-8` treats a patch-package failure as non-fatal. If the patch stops applying, CI wouldn't notice.
5. **[Info, predates this PR]** When a sequence is aborted, `end(!success)` releases the pending decoder but leaves it referenced (`:256-261` with `:225-227`). Next chunk silently restarts decoding and loses earlier data.

AGENT_VERDICT: NEEDS_WORK
