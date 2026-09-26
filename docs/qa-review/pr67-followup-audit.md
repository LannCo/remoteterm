# PR #67 follow-up audit: SEC-2 / SEC-3 fix review

Reviewed: `git diff origin/main...HEAD` at `d6903220` (1 commit ahead of `origin/main`), against `docs/qa-review/pr67-followup-fixes.md`. Scope: `emain/emain-popup.ts`, `emain/emain-popup.test.ts`, `emain/emain-websecurity.ts` (comment-only), plus the evidence harness.

## Summary

- Critical: 0
- High: 0
- Medium: 0
- Low: 2

The SEC-2 fix is correct, its crux claim is genuine (confirmed against Electron's own docs, source, and an independent live re-run), and it chains correctly through nested popups. SEC-3's "no code change" closure is a defensible call given the severity, but the race is real and cheaply fixable; I'd schedule the fix rather than leave it purely empirical.

## Findings

### [LOW] SEC-2 backstop has a residual TOCTOU window for a weakened opener

- **Location:** `emain/emain-popup.ts:335-358` (`hardenCreatedPopup`), vs `emain/emain-popup.ts:236-258` (`handleGuestWindowOpen`)
- **Description:** `hasHardenedPreferences(opener)` runs in `hardenCreatedPopup`, wired to the `did-create-window` event (fires from Electron's `-add-new-contents`/`AddNewContents`, i.e. *after* the child WebContents exists and has been shown). For an `about:blank` popup, the child shares the opener's renderer process and inherits its process-level `sandbox`/`nodeIntegration` state at the moment `window.open()` returns to the calling script — synchronously, and before any main-process listener runs. So if the opener is ever weakened (the precondition this check exists to catch), a hostile script in the opener already has a live handle to a same-process, unsandboxed child before `hardenCreatedPopup` gets a chance to destroy it. Destroying it afterwards is a good backstop but doesn't prevent whatever the script did in that window.
- **Impact:** Only reachable if an opener has already escaped `applyGuestWebPreferences` (a separate bug elsewhere) — not exploitable via the current, correct call paths (`hardenWebviewAttach` for webview guests, `buildPopupWindowOptions` for popup openers, both of which unconditionally force hardened prefs). Given that precondition, the gap is that "destroy after creation" is weaker than "deny before creation."
- **Recommendation:** `opener` is already available, with live prefs, inside `handleGuestWindowOpen` — which runs earlier, during the renderer's synchronous `window.open()` call (`-will-add-new-contents`), before any child WebContents is created. Add the same `hasHardenedPreferences(opener)` check there and return `{ action: "deny" }` on failure, in addition to (not instead of) the existing post-creation check in `hardenCreatedPopup`. That closes the window entirely for `about:blank` rather than reacting to it. Small, low-risk change; not blocking, since it hardens an already-defense-in-depth path against a precondition that shouldn't occur today.

### [LOW] SEC-3 popup-cap race is real by inspection; "evidence only" is a reasonable but not final closure

- **Location:** `emain/emain-popup.ts:253-256` (check) / `emain/emain-popup.ts:164-177` (`trackPopup`, the corresponding increment)
- **Description:** The allow/deny decision in `handleGuestWindowOpen` reads `livePopupCount()` synchronously, but the corresponding increment (`trackPopup`) only runs later, in `did-create-window`, once each popup's own WebContents creation completes on its own renderer's timeline. Because these are two IPC events from *different* renderer processes, both handled sequentially but independently on Electron's single main-process event loop, a second renderer's allow-decision can land between renderer A's allow-decision and renderer A's `did-create-window` — seeing the pre-increment count. This is a genuine TOCTOU in the cap logic, not a false alarm; the follow-up doc's own write-up reaches the same conclusion. 30 (this review: +13 more) synchronized cross-process trials found zero overruns, which is decent stress evidence but is not the same as a proof, since it depends on OS/Electron scheduling that a test harness can bias towards synchrony but not fully control.
- **Impact:** Rated Low by the original auditor and I agree: an overrun only ever produces more fully-hardened, capped-feature popups (same session, same origin restrictions, same navigation guard) — it's a resource/UX cap being loosely enforced, not a security boundary being bypassed.
- **Recommendation:** Given the fix is cheap (reserve the slot synchronously at the "allow" decision in `handleGuestWindowOpen`, e.g. an optimistic counter or pending-set decremented if `did-create-window` never follows, rather than only counting in `trackPopup`), I'd schedule a deterministic fix next sprint rather than leave this as empirical-only indefinitely. That said, shipping the current PR without it is acceptable: severity is Low, the auditor's own prior rating already assumed "unenforced," and this follow-up adds real (if non-exhaustive) evidence rather than leaving it purely theoretical.

## Verified OK

**1. `hasHardenedPreferences` reads real, unspoofable state.** `getLastWebPreferences()` is a native-binding method on Electron's `WebContents` (confirmed against `electron/lib/browser/api/web-contents.ts`, which calls `this.getLastWebPreferences()` internally for dialog-restriction checks) — not a JS-visible property the checked page/renderer could override from script. It's absent from `electron.d.ts` (tracked upstream as electron/electron#38451, "bring back `getWebPreferences`", closed not-planned) which is why the diff reaches it through the narrowed `WebContentsWithLastPreferences` type at `emain/emain-popup.ts:306`; a missing accessor or `null` return fails closed (`hasHardenedPreferences` returns `false`), matching the test at `emain-popup.test.ts` ("destroys a popup whose own webPreferences are weakened or unreadable"). Confirmed real and non-spoofable.

**2. The about:blank claim is genuine, not invented.** Electron's own docs (`docs/api/window-open.md`, current `main`): *"When opening `about:blank`, the child window's WebPreferences will be copied from the parent window, and there is no way to override it because Chromium skips browser side navigation in this case."* Electron's `guest-window-manager.ts` (`makeWebPreferences`, v41.1.0) builds `securityWebPreferencesFromParent` from `embedder.getLastWebPreferences()` — the actual opener, not the requested override — confirming why `buildPopupWindowOptions`'s `overrideBrowserWindowOptions` can't reach an `about:blank` child. I independently re-ran the evidence harness (`.pi/evidence/2026-09-26-pr67-followup/harness/probe-main.cjs`) against real Electron 41.1.0 under a fresh `xvfb-run` instance (`DISPLAY`/`WAYLAND_DISPLAY` unset, no live display touched) and reproduced both A1 and A2 on the post-fix code:
   - A1 (hardened opener): `childPid === openerPid` (same-process, confirming the about:blank-shares-process behaviour), `childSurvived: true`.
   - A2 (opener weakened to `sandbox: false`): popup destroyed (`destroyLogged: true`, `windowsCreated: 0` by the time the probe checked), log line `[popup] destroying child window: opener webPreferences are not hardened`.
   This is the crux of the fix and it holds.

**3. Nested popup chaining.** `installGuestWindowOpenHandler(guest, ctx)` closes over `guest` and wires `guest.on("did-create-window", (child, details) => hardenCreatedPopup(child, guest, details, ctx))`; a popup that survives `hardenCreatedPopup` calls `installGuestWindowOpenHandler(wc, ctx)` on its own WebContents (`emain-popup.ts:372`), so at each level "opener" is correctly the immediate parent, not the root. Verified in code and in the existing test "installs the router on the popup so nested opens route through the root" (`emain-popup.test.ts:410-423`): a grandchild opened from an already-hardened popup survives (`livePopupCount(root.id)` reaches 2), which only happens if `hasHardenedPreferences` was evaluated against the popup (the grandchild's real opener), not the root. Chaining is correct.

**4. Tests, types, lint, full suite** (all re-run fresh in this worktree, not taken on the prior report's word):
   - `npx vitest run emain/emain-popup.test.ts emain/emain-websecurity.test.ts` → 51/51 passed.
   - `npx tsc --noEmit` → clean, no output.
   - `npx eslint emain/emain-popup.ts emain/emain-popup.test.ts emain/emain-websecurity.ts` → clean, no output.
   - `npx vitest run` (full suite) → 427/427 passed, 41/41 files, including `frontend/app/view/term/kitty-graphics-chunking.test.ts` (previously-flagged 7 failures gone, consistent with the `npm ci` fix noted in the follow-up doc).

**5. `emain-websecurity.ts` diff is comment-only.** No behavioural change; `AllowedPopupUrlProtocols` and its guard logic are unchanged, only the rationale comment was added.

## Positive observations

- The fix targets the actual root cause identified in live testing (opener process inheritance for `about:blank`), not a cosmetic check on the popup's own (misleading) reported prefs — the code comment at `emain-popup.ts:351-352` correctly documents *why* the child-only check would be a false backstop.
- Fails closed throughout: missing `getLastWebPreferences` accessor, `null` return, and any individual weakened flag all destroy the popup; there's no fallback to "assume hardened."
- The evidence harness is methodologically sound: real Electron (not mocked), fault injection isolated to a marked `weakenedHosts` set so it can't leak into the control case, cross-process race test explicitly confirms ≥2 distinct renderer PIDs per trial rather than assuming it.
- Good test hygiene: the new unit tests encode the exact live-verified surprising behaviour (child reports hardened prefs while actually running unsandboxed) as a regression guard, so the fix can't silently regress even though the underlying Electron behaviour it depends on isn't something a fast unit test can otherwise observe.

## Recommendations

- Add the earlier `hasHardenedPreferences(opener)` deny in `handleGuestWindowOpen` per the first finding above — cheap, closes the residual window, doesn't require removing the existing post-creation check.
- Track SEC-3 as a scheduled fix (reserve-on-allow counter) rather than a closed item, even though this PR's "no code change, evidence only" is fine to ship as-is.

AGENT_VERDICT: PASS
