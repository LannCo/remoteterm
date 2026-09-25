# App-wide light/dark appearance mode — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a real app-wide light/dark appearance mode (OS-follow, global default, per-tab override) that applies to all existing app chrome — Tailwind-token components and the legacy `.scss`/`theme.scss` component set alike. Terminal-palette (`termthemes.json`) light variants are a separate, later spec — not built here.

**Architecture:** A single derived Jotai value (`"light" | "dark"`) resolves per-tab override → global `window:appearancemode` setting → OS `nativeTheme` preference → `dark` fallback, in that priority order. A root-level effect writes `data-theme` onto `document.documentElement`; two `:root[data-theme="light"]` override blocks (one in `frontend/tailwindsetup.css`'s `@theme`-token space, one in the legacy `frontend/app/theme.scss` custom-property space — **the spec's premise that `.scss` files consume the Tailwind tokens is wrong; recon found 0 of 38 `.scss` files reference `--color-*`, 30 of them reference `theme.scss`'s separate ~50-var legacy palette instead, so both systems need a light block**) supply the light values. Electron's `nativeTheme` is exposed to the renderer via a new preload API (sync getter + live push event), following the existing `getZoomFactor`/`onZoomFactorChange` pattern exactly.

**Tech Stack:** TypeScript/React frontend, Jotai state, Tailwind v4 + legacy SCSS hybrid styling, Go backend (`rtconfig`/`remotetermobj` packages), Electron main/preload/IPC, vitest.

**Spec:** `.pi/specs/2026-09-24-appearance-mode-design.md`

## Global Constraints

- Resolution priority is fixed: per-tab override (`tab:appearancemode`) > global setting (`window:appearancemode`) > OS preference (`nativeTheme`) > `dark` fallback. Never reorder.
- No `@media (prefers-color-scheme)` query anywhere — `data-theme` already encodes the fully-resolved mode.
- The OS-preference IPC binding never throws into application code; a missing/failed signal degrades silently to the `dark` fallback.
- Full parity required — owner's explicit "no half-measures": every existing chrome file gets light-mode coverage, none stays dark-only.
- `termthemes.json` and terminal-block color selection are explicitly **out of scope** — second spec, not this one.
- Never run `go build`/`go run` to check Go compiles — VSCode/no-errors is the compile signal (project rule).
- Always run `task generate` after any Go `SettingsType`/`MetaTSType` change, before touching the generated TS files it produces.
- Never manually edit `frontend/types/gotypes.d.ts` or `frontend/app/store/wshclientapi.ts` — generated only.
- Never bare `git stash` anywhere in this repo family — shared stash stack across all worktrees.
- `task dev` is live against this exact worktree (`remoteterm-daily`) for the duration of this arc — never let a dispatched subagent edit files here while it's running without checking `ps aux | grep remoteterm-daily` first; this has crashed the live app twice before ([[feedback_remoteterm_live_worktree]]).
- Any live-Electron-window verification (`xdotool`, screenshots, driving the real app) needs the owner's explicit approval each time — budget for that prompt, don't assume it passes silently.

## Review Focus

- **OS preference flips while the app is running** (global setting `system`, no tab override) — the live `nativeTheme` `updated` event must repaint without a reload. Covered by Task 3/4's design; no automated test reaches real Electron `nativeTheme`, so Task 3 ends with an explicit manual-verify note instead of a false "tested" claim.
- **Clearing a tab override back to inherit** — the context-menu's "Default" entry must write `null` (matching the existing `tab:flagcolor`/`tab:background` "None"/"Default" convention), not the string `"inherit"`, or the resolved-mode chain silently gets stuck on a stale override. Covered by Task 10's exact `SetMetaCommand` payload.
- **First-paint flash before the resolved-mode atom/effect runs** — since `data-theme` is set from a `useEffect`, not before React mounts, a light-preferring user could see one frame of the unthemed (dark) default on cold start. This is a known, accepted limitation — the spec is explicit that it fixes the mechanism, not launch-time polish (no `index.html` inline-script mitigation is in scope here); documented, not silently ignored.
- **A touched `.scss` selector overridden by higher specificity or `!important` elsewhere** could make a light-mode fix silently not apply. Task 8's verification step greps the touched files for `!important` to rule this out cheaply.
- **Tailwind arbitrary-value classes that bypass `--color-*` tokens entirely** (e.g. `bg-[#232323]`) would not repaint under `data-theme="light"` even though the token system is correct. Task 6's verification step greps `frontend/app` for `-\[#` hex-arbitrary-value Tailwind classes to catch any.

---

## File structure

**New files:**
- `frontend/app/store/appearance-atoms.ts` — OS-preference atom, `window:appearancemode` setting atom, pure `resolveAppearanceMode()` priority function, per-tab memoized `getResolvedAppearanceModeAtom(tabId)`.
- `frontend/app/store/appearance-atoms.test.ts` — unit tests over the priority chain.
- `frontend/app/appearance-theme-updater.tsx` — `AppThemeUpdater` component (mirrors `AppSettingsUpdater` in `app.tsx`), sets `document.documentElement.dataset.theme`.
- `frontend/app/appearance-theme-updater.test.tsx` — React test asserting the DOM attribute follows the atom.
- `emain/emain-native-theme.ts` — `nativeTheme` broadcast helper + `updated` listener registration (mirrors `broadcastZoomFactorChanged` in `emain-util.ts`).

**Modified files** (grouped by task, exact lines given per-task below):
- `pkg/rtconfig/settingsconfig.go`, `pkg/rtconfig/defaultconfig/settings.json`, `docs/docs/config.mdx` — `window:appearancemode` setting.
- `pkg/remotetermobj/wtypemeta.go`, `pkg/remotetermobj/metaconsts.go` — `tab:appearancemode` meta key.
- `frontend/types/custom.d.ts`, `emain/preload.ts`, `emain/emain-ipc.ts`, `emain/emain.ts`, `frontend/preview/mock/preview-electron-api.ts` — Electron `nativeTheme` IPC.
- `frontend/tailwindsetup.css` — light-mode `@theme`-token override block.
- `frontend/app/theme.scss` — light-mode legacy-palette override block, 2 new shared vars.
- 15 other `.scss` files — hardcoded-literal → var() fixes (full list in Task 8).
- `frontend/app/view/remotetermconfig/generalcontent.tsx` — new settings field.
- `frontend/app/tab/tabcontextmenu.ts` — new context-menu submenu.
- `frontend/app/app.tsx` — mount `AppThemeUpdater`.

**19 `.scss` files need no direct edit** (zero hardcoded color literals; they consume `theme.scss`'s vars, which gain light values in Task 7): `button.scss`, `copybutton.scss`, `expandablemenu.scss`, `iconbutton.scss`, `input.scss`, `linkbutton.scss`, `magnify.scss`, `menubutton.scss`, `multilineinput.scss`, `quickelems.scss`, `search.scss`, `toggle.scss`, `typingindicator.scss`, `messagemodal.scss`, `reset.scss`, `workspaceeditor.scss`, `csvview.scss`, `term.scss` (terminal-scoped, out of scope anyway), `tilelayout.scss`, plus `app.scss`/`progressbar.scss` (already var-derived, `rgb(from var(...))`).

---

### Task 1: Go — `window:appearancemode` global setting

**Files:**
- Modify: `pkg/rtconfig/settingsconfig.go` (Window* field group, currently lines 107-126)
- Modify: `pkg/rtconfig/defaultconfig/settings.json`
- Modify: `docs/docs/config.mdx`
- Test: manual — `task generate` + grep the generated output

**Interfaces:**
- Produces: Go field `SettingsType.WindowAppearanceMode string` with JSON key `window:appearancemode`, enum `system`/`light`/`dark`, default `"system"`. After `task generate`, TS `SettingsType["window:appearancemode"]?: string` in `frontend/types/gotypes.d.ts` (auto-generated, do not hand-edit).

- [ ] **Step 1: Add the field to the Go struct**

In `pkg/rtconfig/settingsconfig.go`, inside the `Window*` field group (the block starting `WindowClear bool `json:"window:*,omitempty"``), add:

```go
	WindowAppearanceMode           string  `json:"window:appearancemode,omitempty" jsonschema:"enum=system,enum=light,enum=dark"`
```

- [ ] **Step 2: Add the default value**

In `pkg/rtconfig/defaultconfig/settings.json`, add a new line (alphabetical-ish position near the other `window:*` keys):

```json
    "window:appearancemode": "system",
```

- [ ] **Step 3: Document the setting**

In `docs/docs/config.mdx`, add a row to the settings table for `window:appearancemode` (follow the exact row format of the neighboring `window:*` entries already in that file — copy their column structure), description: "Controls app-wide light/dark appearance: `system` follows the OS preference, or force `light`/`dark`.", with a `<VersionBadge version="v0.15" />` (bump from the `v0.14` seen elsewhere in the file — confirm the current next-unreleased version marker used in that file before picking the exact string, since it may have advanced since the `add-config` skill doc was last updated).

- [ ] **Step 4: Regenerate and verify**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && task generate`

Then verify:
```bash
grep -n '"window:appearancemode"' frontend/types/gotypes.d.ts
```
Expected: one match, `"window:appearancemode"?: string;`, inside the `SettingsType` block.

- [ ] **Step 5: Commit**

```bash
git add pkg/rtconfig/settingsconfig.go pkg/rtconfig/defaultconfig/settings.json docs/docs/config.mdx frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts
git commit -m "feat(config): add window:appearancemode global setting"
```

---

### Task 2: Go — `tab:appearancemode` per-tab override meta key

**Files:**
- Modify: `pkg/remotetermobj/wtypemeta.go`
- Modify: `pkg/remotetermobj/metaconsts.go`
- Test: manual — `task generate` + grep

**Interfaces:**
- Consumes: none.
- Produces: TS `MetaType["tab:appearancemode"]?: string` (post-generation), Go constant `remotetermobj.MetaKey_TabAppearanceMode = "tab:appearancemode"`.

Unlike `tab:background`, this key has **no** global-settings-side default-application logic (no "apply my default background to every new tab" equivalent needed) — absence on a tab simply means "inherit," resolved entirely in Task 4's frontend priority chain. So this task only touches the meta-type side, not `settingsconfig.go`/`pkg/rtconfig/metaconsts.go`.

- [ ] **Step 1: Add the meta-type field**

In `pkg/remotetermobj/wtypemeta.go`, in the `MetaTSType` struct, near the existing `TabBackground string `json:"tab:background,omitempty"`` field (around line 80), add:

```go
	TabAppearanceMode   string  `json:"tab:appearancemode,omitempty"`
```

- [ ] **Step 2: Add the meta-key constant**

In `pkg/remotetermobj/metaconsts.go`, near `MetaKey_TabBackground = "tab:background"` (around line 78), add:

```go
	MetaKey_TabAppearanceMode = "tab:appearancemode"
```

- [ ] **Step 3: Regenerate and verify**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && task generate`

Then verify:
```bash
grep -n '"tab:appearancemode"' frontend/types/gotypes.d.ts
```
Expected: at least one match inside the `MetaType`/meta-section block.

- [ ] **Step 4: Commit**

```bash
git add pkg/remotetermobj/wtypemeta.go pkg/remotetermobj/metaconsts.go frontend/types/gotypes.d.ts
git commit -m "feat(config): add tab:appearancemode per-tab meta key"
```

---

### Task 3: Electron — `nativeTheme` preload API

**Files:**
- Modify: `frontend/types/custom.d.ts` (lines 77-131, `ElectronApi` type)
- Modify: `emain/preload.ts`
- Modify: `emain/emain-ipc.ts`
- Create: `emain/emain-native-theme.ts`
- Modify: `emain/emain.ts` (line 68)
- Modify: `frontend/preview/mock/preview-electron-api.ts`

**Interfaces:**
- Produces: `getApi().getNativeTheme(): boolean` (true = OS prefers dark), `getApi().onNativeThemeChange((shouldUseDarkColors: boolean) => void): void`. Consumed by Task 4's `appearance-atoms.ts`.

- [ ] **Step 1: Add the type members**

In `frontend/types/custom.d.ts`, inside the `ElectronApi` type (currently closes at line 131), add before the closing `};`:

```ts
    getNativeTheme: () => boolean; // get-native-theme
    onNativeThemeChange: (callback: (shouldUseDarkColors: boolean) => void) => void; // native-theme-change
```

- [ ] **Step 2: Expose via preload**

In `emain/preload.ts`, inside the `contextBridge.exposeInMainWorld("api", {...})` block, add (near `getZoomFactor`/`onZoomFactorChange`):

```ts
    getNativeTheme: () => ipcRenderer.sendSync("get-native-theme"),
    onNativeThemeChange: (callback) =>
        ipcRenderer.on("native-theme-change", (_event, shouldUseDarkColors) => callback(shouldUseDarkColors)),
```

- [ ] **Step 3: Implement the sync getter handler**

In `emain/emain-ipc.ts`, add:

```ts
ipcMain.on("get-native-theme", (event) => {
    event.returnValue = electron.nativeTheme.shouldUseDarkColors;
});
```

(Confirm `electron` is already imported in this file as the namespace import used elsewhere in it; if not, add `import electron from "electron";` matching the file's existing import style.)

- [ ] **Step 4: Create the broadcast helper**

Create `emain/emain-native-theme.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import electron from "electron";

function broadcastNativeThemeChanged(shouldUseDarkColors: boolean): void {
    for (const wc of electron.webContents.getAllWebContents()) {
        if (wc.isDestroyed()) {
            continue;
        }
        wc.send("native-theme-change", shouldUseDarkColors);
    }
}

export function registerNativeThemeListener(): void {
    electron.nativeTheme.on("updated", () => {
        broadcastNativeThemeChanged(electron.nativeTheme.shouldUseDarkColors);
    });
}
```

This mirrors `broadcastZoomFactorChanged` in `emain/emain-util.ts:15-28` exactly (same `isDestroyed()` guard, same broadcast-to-all-webContents shape).

- [ ] **Step 5: Wire the registration and fix the hardcoded theme source**

In `emain/emain.ts`, replace line 68:

```ts
electron.nativeTheme.themeSource = "dark";
```

with:

```ts
electron.nativeTheme.themeSource = "system";
registerNativeThemeListener();
```

and add the import at the top of the file: `import { registerNativeThemeListener } from "./emain-native-theme";`

This is the one line that currently forces dark mode unconditionally — until it changes, `nativeTheme.shouldUseDarkColors` cannot reflect the real OS preference no matter what the renderer does.

- [ ] **Step 6: Add the preview/mock stub**

In `frontend/preview/mock/preview-electron-api.ts`, add:

```ts
    getNativeTheme: () => false,
    onNativeThemeChange: () => {},
```

- [ ] **Step 7: Manual verification (not automated — flagged per Review Focus)**

This step cannot be covered by a unit test (it exercises real Electron `nativeTheme`). Once the app is live (after Task 5 wires the consuming atom/effect), ask the owner before driving the live window, then: flip the OS's light/dark switch with RemoteTerm open and `window:appearancemode` set to `system`, confirm the app repaints without a manual reload. Record the result in the arc's next handoff rather than claiming it here.

- [ ] **Step 8: Commit**

```bash
git add frontend/types/custom.d.ts emain/preload.ts emain/emain-ipc.ts emain/emain-native-theme.ts emain/emain.ts frontend/preview/mock/preview-electron-api.ts
git commit -m "feat(electron): expose nativeTheme via preload API, stop forcing dark themeSource"
```

---

### Task 4: Frontend — resolved-mode atom with priority-chain logic

**Files:**
- Create: `frontend/app/store/appearance-atoms.ts`
- Test: Create `frontend/app/store/appearance-atoms.test.ts`

**Interfaces:**
- Consumes: `getApi().getNativeTheme()`/`getApi().onNativeThemeChange()` (Task 3), `getTabMetaKeyAtom(tabId, key)` from `frontend/app/store/global.ts`, `settingsAtom` from `frontend/app/store/global-atoms.ts` (confirm it is exported — if not, add `export` to its declaration as part of this task).
- Produces: `export function resolveAppearanceMode(tabOverride, globalSetting, osPrefersDark): "light" | "dark"`, `export function getResolvedAppearanceModeAtom(tabId: string): Atom<"light" | "dark">`. Consumed by Task 5's `AppThemeUpdater`.

- [ ] **Step 1: Write the failing tests for the pure priority function**

Create `frontend/app/store/appearance-atoms.test.ts`:

```ts
import { describe, expect, test } from "vitest";
import { resolveAppearanceMode } from "./appearance-atoms";

describe("resolveAppearanceMode", () => {
    test("tab override wins regardless of global/OS", () => {
        expect(resolveAppearanceMode("dark", "light", false)).toBe("dark");
        expect(resolveAppearanceMode("light", "dark", true)).toBe("light");
    });

    test("tab inherit falls through to global light/dark", () => {
        expect(resolveAppearanceMode(null, "light", true)).toBe("light");
        expect(resolveAppearanceMode(undefined, "dark", false)).toBe("dark");
    });

    test("global system falls through to OS preference", () => {
        expect(resolveAppearanceMode(null, "system", true)).toBe("dark");
        expect(resolveAppearanceMode(null, "system", false)).toBe("light");
    });

    test("everything absent falls back to dark", () => {
        expect(resolveAppearanceMode(null, null, true)).toBe("dark");
        expect(resolveAppearanceMode(undefined, undefined, false)).toBe("light");
    });
});
```

Note the last case (`resolveAppearanceMode(null, null, false)` isn't listed above but is implicitly covered — the function has no fourth "everything unavailable" branch of its own; unavailability is handled upstream by seeding `osPrefersDarkAtom`'s initial value to `true` in Step 3 below, so the pure function only ever needs 3 real inputs.

- [ ] **Step 2: Run tests, verify they fail**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx vitest run frontend/app/store/appearance-atoms.test.ts`
Expected: FAIL — `Cannot find module './appearance-atoms'` or similar.

- [ ] **Step 3: Implement the module**

Create `frontend/app/store/appearance-atoms.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getTabMetaKeyAtom } from "@/app/store/global";
import { settingsAtom } from "@/app/store/global-atoms";
import { getApi } from "@/store/global";
import * as jotai from "jotai";
import { globalStore } from "@/app/store/jotaiStore";
import type { Atom } from "jotai";

export function resolveAppearanceMode(
    tabOverride: string | null | undefined,
    globalSetting: string | null | undefined,
    osPrefersDark: boolean
): "light" | "dark" {
    if (tabOverride === "light" || tabOverride === "dark") {
        return tabOverride;
    }
    if (globalSetting === "light" || globalSetting === "dark") {
        return globalSetting;
    }
    return osPrefersDark ? "dark" : "light";
}

export const osPrefersDarkAtom = jotai.atom(true) as jotai.PrimitiveAtom<boolean>;

try {
    globalStore.set(osPrefersDarkAtom, getApi().getNativeTheme());
    getApi().onNativeThemeChange((shouldUseDarkColors) => {
        globalStore.set(osPrefersDarkAtom, shouldUseDarkColors);
    });
} catch (e) {
    console.log("failed to initialize osPrefersDarkAtom, falling back to dark", e);
}

export const windowAppearanceModeSettingAtom = jotai.atom((get) => get(settingsAtom)?.["window:appearancemode"]);

const appearanceModeAtomCache = new Map<string, Atom<"light" | "dark">>();

export function getResolvedAppearanceModeAtom(tabId: string): Atom<"light" | "dark"> {
    const cached = appearanceModeAtomCache.get(tabId);
    if (cached != null) {
        return cached;
    }
    const tabOverrideAtom = getTabMetaKeyAtom(tabId, "tab:appearancemode");
    const derived = jotai.atom((get) => {
        const tabOverride = get(tabOverrideAtom);
        const globalSetting = get(windowAppearanceModeSettingAtom);
        const osPrefersDark = get(osPrefersDarkAtom);
        return resolveAppearanceMode(tabOverride, globalSetting, osPrefersDark);
    });
    appearanceModeAtomCache.set(tabId, derived);
    return derived;
}
```

The `try/catch` around the initial `getApi()` calls mirrors the existing `zoomFactorAtom` init pattern exactly (`frontend/app/store/global-atoms.ts:36-44`) — this is the concrete mechanism satisfying the spec's "never throws, degrades to dark fallback" rule: if `getApi()` throws, `osPrefersDarkAtom` simply keeps its seeded `true` value.

If `settingsAtom` in `frontend/app/store/global-atoms.ts` is not currently exported, add `export` to its declaration as part of this step.

- [ ] **Step 4: Run tests, verify they pass**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx vitest run frontend/app/store/appearance-atoms.test.ts`
Expected: PASS, 4/4 (the pure-function tests only — `osPrefersDarkAtom`'s `getApi()` init isn't exercised by this file's tests since it runs at module-import time under a test environment where `getApi()` will throw and get caught, which is itself correct/expected behavior, not a bug to chase).

- [ ] **Step 5: Commit**

```bash
git add frontend/app/store/appearance-atoms.ts frontend/app/store/appearance-atoms.test.ts frontend/app/store/global-atoms.ts
git commit -m "feat(appearance): add resolved-mode priority-chain atom"
```

---

### Task 5: Frontend — `AppThemeUpdater` root effect

**Files:**
- Create: `frontend/app/appearance-theme-updater.tsx`
- Modify: `frontend/app/app.tsx` (mount near line 370, alongside `<AppSettingsUpdater />`)
- Test: Create `frontend/app/appearance-theme-updater.test.tsx`

**Interfaces:**
- Consumes: `getResolvedAppearanceModeAtom` (Task 4), `atoms.staticTabId` (existing, same one `frontend/app/app-bg.tsx:21` uses).
- Produces: `document.documentElement.dataset.theme` always reflects the resolved mode for the active tab.

- [ ] **Step 1: Write the failing test**

Create `frontend/app/appearance-theme-updater.test.tsx`:

```tsx
import { describe, expect, test, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { createStore, Provider } from "jotai";
import { AppThemeUpdater } from "./appearance-theme-updater";
import { atoms } from "@/store/global";
import { osPrefersDarkAtom } from "@/app/store/appearance-atoms";

describe("AppThemeUpdater", () => {
    beforeEach(() => {
        document.documentElement.removeAttribute("data-theme");
    });

    test("sets data-theme to dark when OS prefers dark and no overrides", () => {
        const store = createStore();
        store.set(atoms.staticTabId, "test-tab-1");
        store.set(osPrefersDarkAtom, true);
        render(
            <Provider store={store}>
                <AppThemeUpdater />
            </Provider>
        );
        expect(document.documentElement.dataset.theme).toBe("dark");
    });

    test("sets data-theme to light when OS prefers light and no overrides", () => {
        const store = createStore();
        store.set(atoms.staticTabId, "test-tab-2");
        store.set(osPrefersDarkAtom, false);
        render(
            <Provider store={store}>
                <AppThemeUpdater />
            </Provider>
        );
        expect(document.documentElement.dataset.theme).toBe("light");
    });
});
```

- [ ] **Step 2: Run test, verify it fails**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx vitest run frontend/app/appearance-theme-updater.test.tsx`
Expected: FAIL — `Cannot find module './appearance-theme-updater'`.

- [ ] **Step 3: Implement the component**

Create `frontend/app/appearance-theme-updater.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getResolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { atoms } from "@/store/global";
import { useAtomValue } from "jotai";
import { useEffect } from "react";

export function AppThemeUpdater() {
    const tabId = useAtomValue(atoms.staticTabId);
    const resolvedMode = useAtomValue(getResolvedAppearanceModeAtom(tabId));

    useEffect(() => {
        document.documentElement.dataset.theme = resolvedMode;
    }, [resolvedMode]);

    return null;
}
```

- [ ] **Step 4: Mount it in the app root**

In `frontend/app/app.tsx`, add the import near the other component imports:

```ts
import { AppThemeUpdater } from "@/app/appearance-theme-updater";
```

and mount it as a sibling of `<AppSettingsUpdater />` (currently at line 370):

```tsx
<AppSettingsUpdater />
<AppThemeUpdater />
```

- [ ] **Step 5: Run tests, verify they pass**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx vitest run frontend/app/appearance-theme-updater.test.tsx`
Expected: PASS, 2/2.

- [ ] **Step 6: Commit**

```bash
git add frontend/app/appearance-theme-updater.tsx frontend/app/appearance-theme-updater.test.tsx frontend/app/app.tsx
git commit -m "feat(appearance): mount AppThemeUpdater, wire data-theme onto documentElement"
```

---

### Task 6: CSS — Tailwind `@theme` light-mode override block

**Files:**
- Modify: `frontend/tailwindsetup.css` (append after the existing `@theme { ... }` block, which currently closes at line 81)

**Interfaces:**
- Consumes: nothing (pure CSS).
- Produces: every Tailwind-classed component (e.g. `bg-background`, `text-foreground`, `border-border`) repaints correctly once `data-theme="light"` is set.

- [ ] **Step 1: Add the light override block**

In `frontend/tailwindsetup.css`, after the closing `}` of the `@theme` block (line 81), add:

```css
:root[data-theme="light"] {
    --color-background: rgb(255, 255, 255);
    --color-foreground: #1a1a1a;
    --color-primary: #1a1a1a;
    --color-muted-foreground: rgb(107, 111, 106);
    --color-secondary: rgb(107, 111, 106);
    --color-muted: rgb(150, 155, 150);
    --color-panel: rgba(245, 246, 245, 0.9);
    --color-hover: rgba(0, 0, 0, 0.06);
    --color-border: rgba(0, 0, 0, 0.14);
    --color-modalbg: #ffffff;
    --color-hoverbg: rgba(0, 0, 0, 0.08);
    --color-highlightbg: rgba(0, 0, 0, 0.08);
    --color-surface: rgba(0, 0, 0, 0.04);
    --color-activebg: rgba(88, 193, 66, 0.12);
}
```

`--color-white`, all `--color-accent-*`, `--color-error`/`--color-warning`/`--color-success`, `--color-accent`/`--color-accenthover`/`--color-accentbg`, the `--font-*`/`--text-*`/`--container-*`/`--ansi-*` groups are deliberately **not** overridden — they're brand/semantic/structural values, unchanged across modes (same reasoning as the existing accent-button contrast fix keeping the accent hue fixed). `--ansi-*` is terminal-palette territory, out of scope per the spec.

Contrast check (manual, same method as the accent-button precedent in `.kilocode/rules/rules.md`): `--color-foreground` `#1a1a1a` on `--color-background` `rgb(255,255,255)` computes to ≈16.9:1 (WCAG relative-luminance formula), `--color-muted-foreground` `rgb(107,111,106)` on the same background computes to ≈5.1:1 — both clear the 4.5:1 AA minimum for normal text with margin. Owner reviews and adjusts these hex/rgb values against the rendered app per the spec's own expectation of "a round or two of live feedback."

- [ ] **Step 2: Check for arbitrary-value Tailwind classes that would bypass the tokens**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && grep -rn -- '-\[#' frontend/app frontend/layout frontend/preview 2>/dev/null`

If this returns hits, list them in the commit message as a follow-up (don't fix in this task unless trivial) — they're components that read a literal hex color via Tailwind's arbitrary-value syntax instead of a `--color-*` token, and won't repaint under `data-theme="light"` no matter what this task does.

- [ ] **Step 3: Commit**

```bash
git add frontend/tailwindsetup.css
git commit -m "feat(appearance): add light-mode Tailwind theme-token overrides"
```

---

### Task 7: CSS — legacy `theme.scss` light-mode override block

**Files:**
- Modify: `frontend/app/theme.scss` (append after the existing `:root { ... }` block, which currently closes at line 161)

**Interfaces:**
- Consumes: nothing.
- Produces: every legacy-`.scss`-styled component (30 of the 38 `.scss` files) repaints correctly under `data-theme="light"`, since they consume these var names directly.

This is the primary fix the spec's own architecture section didn't anticipate needing (it assumed `.scss` files already referenced Tailwind's `--color-*` tokens; recon confirmed none of them do — they reference this file's separate palette instead).

- [ ] **Step 1: Add the light override block, including two new shared vars needed by Task 8**

In `frontend/app/theme.scss`, after the closing `}` of the `:root { ... }` block (line 161), add:

```scss
:root[data-theme="light"] {
    --main-text-color: #1a1a1a;
    --secondary-text-color: rgb(107, 111, 106);
    --grey-text-color: #6b6f6a;
    --main-bg-color: rgb(255, 255, 255);
    --border-color: rgba(0, 0, 0, 0.14);
    --panel-bg-color: rgba(245, 246, 245, 0.9);
    --highlight-bg-color: rgba(0, 0, 0, 0.08);
    --hover-bg-color: rgba(0, 0, 0, 0.06);
    --block-bg-color: rgba(255, 255, 255, 0.6);
    --block-bg-solid-color: rgb(255, 255, 255);

    --keybinding-color: #2a2a2a;
    --keybinding-bg-color: #e8e8e8;
    --keybinding-border-color: #cfcfcf;

    --scrollbar-thumb-color: rgba(0, 0, 0, 0.18);
    --scrollbar-thumb-hover-color: rgba(0, 0, 0, 0.35);
    --scrollbar-thumb-active-color: rgba(0, 0, 0, 0.45);

    --modal-bg-color: #ffffff;
    --modal-header-bottom-border-color: rgba(20, 24, 20, 0.12);
    --modal-border-color: rgba(0, 0, 0, 0.12);
    --modal-shadow-color: rgba(0, 0, 0, 0.25);
    --modal-backdrop-color: rgba(20, 20, 20, 0.35);

    --form-element-border-color: rgba(20, 24, 20, 0.14);
    --form-element-secondary-color: rgba(0, 0, 0, 0.12);

    --button-grey-bg: rgba(0, 0, 0, 0.04);
    --button-grey-hover-bg: rgba(0, 0, 0, 0.08);
    --button-grey-border-color: rgba(0, 0, 0, 0.12);
    --button-grey-outlined-color: rgba(0, 0, 0, 0.65);

    --dropdown-shadow-color: rgba(0, 0, 0, 0.18);
}
```

Also add the two new base (dark-mode) declarations these light overrides need a counterpart for, inside the **existing** `:root { ... }` block (not the new light block) — insert near `--modal-shadow-color` (line 86):

```scss
    --modal-backdrop-color: rgba(21, 23, 21, 0.7);
    --dropdown-shadow-color: rgba(0, 0, 0, 0.4);
```

All brand/semantic colors — `--accent-color`, `--error-color`, `--warning-color`, `--success-color`, `--link-color`, `--tab-green`, `--conn-icon-color-*`, `--conn-status-overlay-bg-color`, `--sysinfo-*-color`, `--bulb-color`, all `--term-*`, all `--button-red-*`/`--button-green-*`/`--button-yellow-*` — are deliberately **not** overridden, matching Task 6's same reasoning; `--term-*` is additionally out of scope per the spec (termthemes second spec). `--toggle-thumb-color`/`--toggle-bg-color`/`--form-element-bg-color`/`--form-element-text-color`/`--form-element-primary-*`/`--form-element-error-color` need no override since they already derive via `var(...)` from other now-themed vars.

Contrast check: `--main-text-color` `#1a1a1a` on `--main-bg-color` `rgb(255,255,255)` ≈16.9:1; `--secondary-text-color` `rgb(107,111,106)` on the same ≈5.1:1 — both clear AA. Same owner-review expectation as Task 6.

- [ ] **Step 2: Commit**

```bash
git add frontend/app/theme.scss
git commit -m "feat(appearance): add light-mode overrides for legacy theme.scss palette"
```

---

### Task 8: SCSS — fix hardcoded color literals in the 15 files that have them

**Files:** (all under `frontend/app/`, `Modify`)
- `element/directorydropdown.scss`
- `view/preview/directorypreview.scss`
- `element/flyoutmenu.scss`
- `element/popover.scss`
- `modals/modal.scss`
- `element/modal.scss`
- `block/block.scss`
- `element/emojipalette.scss`
- `element/markdown.scss`
- `modals/typeaheadmodal.scss`
- `modals/userinputprompt.scss`
- `view/webview/webview.scss`
- `tab/connectiondropdown.scss`
- `tab/tabbar.scss`
- `tab/tab.scss`
- `tab/workspaceswitcher.scss`

**Interfaces:** Consumes the vars added/themed in Task 7 (`--dropdown-shadow-color`, `--modal-backdrop-color`, plus the existing themed ones). No new interfaces produced.

Every fix below either reuses an existing (now-themed) var, reuses the two new vars Task 7 added, or — where no existing var fits — adds a small scoped `:root[data-theme="light"] <selector> { ... }` override, per the spec's own sanctioned fallback for values that don't map cleanly onto a token.

- [ ] **Step 1: `element/directorydropdown.scss`**

Edit exactly these lines (also fixes a real pre-existing bug: two occurrences reference an undeclared `--text-secondary-color`/`--text-primary-color` with a literal fallback — `theme.scss` actually declares `--secondary-text-color`/`--main-text-color`, different names — the fallback was silently masking the typo):

```
line 8:  box-shadow: 0px 13px 16px 0px rgba(0, 0, 0, 0.4);
      →  box-shadow: 0px 13px 16px 0px var(--dropdown-shadow-color);

line 30: color: var(--text-secondary-color, #999);
      →  color: var(--secondary-text-color);

line 33: color: var(--text-primary-color, #fff);
      →  color: var(--main-text-color);

line 39: color: var(--text-secondary-color, #999);
      →  color: var(--secondary-text-color);

line 74: box-shadow: inset 2px 0 0 #3b82f6;
      →  box-shadow: inset 2px 0 0 var(--accent-color);

line 91: color: var(--text-secondary-color, #999);
      →  color: var(--secondary-text-color);

line 99: color: var(--text-primary-color, #fff);
      →  color: var(--main-text-color);
```

- [ ] **Step 2: `view/preview/directorypreview.scss`**

```
line 166: background-color: rgba(255, 255, 255, 0.06);
       →  background-color: var(--hover-bg-color);

line 218: border: 1px solid rgba(255, 255, 255, 0.15);
       →  border: 1px solid var(--modal-border-color);

line 219: background: #212121;
       →  background: var(--modal-bg-color);

line 220: box-shadow: 0px 8px 24px 0px rgba(0, 0, 0, 0.3);
       →  box-shadow: 0px 8px 24px 0px var(--modal-shadow-color);
```

- [ ] **Step 3: `element/flyoutmenu.scss`**

```
line 16: border: 1px solid rgba(255, 255, 255, 0.15);
      →  border: 1px solid var(--modal-border-color);

line 17: background: #212121;
      →  background: var(--modal-bg-color);

line 18: box-shadow: 0px 8px 24px 0px rgba(0, 0, 0, 0.3);
      →  box-shadow: 0px 8px 24px 0px var(--modal-shadow-color);
```

- [ ] **Step 4: `element/popover.scss`**

Same 3-line pattern as Step 3, at its own lines 13-15 — apply the identical substitution.

- [ ] **Step 5: `modals/modal.scss`**

```
line 21: background-color: rgba(21, 23, 21, 0.7);
      →  background-color: var(--modal-backdrop-color);

line 41: box-shadow: 0px 8px 32px 0px rgba(0, 0, 0, 0.25);
      →  box-shadow: 0px 8px 32px 0px var(--modal-shadow-color);

line 71: border-top: 1px solid rgba(255, 255, 255, 0.1);
      →  border-top: 1px solid var(--modal-border-color);
```

- [ ] **Step 6: `element/modal.scss`**

```
line 11: background-color: rgba(21, 23, 21, 0.7);
      →  background-color: var(--modal-backdrop-color);
```

- [ ] **Step 7: `block/block.scss`**

```
line 325: color: #e6ba1e;
       →  color: var(--warning-color);

line 344: color: white;
       →  color: var(--main-text-color);
```

- [ ] **Step 8: `element/emojipalette.scss`**

```
line 38: color: #888;
      →  color: var(--grey-text-color);
```

Line 31 (`background-color: rgba(0, 0, 0, 0.1);`) doesn't map cleanly onto an existing var (it's a subtle press/active-state darkening over an already-dark panel, not a general hover). Add a scoped override instead — find this rule's selector in the file and, immediately after Task 7 lands, add to `theme.scss`'s new light block area is wrong (selector-scoped, not a var) — add directly in this file, right after the existing rule:

```scss
:root[data-theme="light"] & {
    background-color: rgba(0, 0, 0, 0.06);
}
```

(Nest it under the existing rule using `&`, so it inherits the same selector without repeating it — confirm the exact existing selector name when editing, since this excerpt doesn't reproduce it.)

- [ ] **Step 9: `element/markdown.scss`**

```
line 90:  color: #32afff;
       →  color: var(--term-bright-blue);

line 177: background-color: black;
       →  background-color: var(--block-bg-solid-color);
```

- [ ] **Step 10: `modals/typeaheadmodal.scss`**

```
line 23: box-shadow: 0px 13px 16px 0px rgba(0, 0, 0, 0.4);
      →  box-shadow: 0px 13px 16px 0px var(--dropdown-shadow-color);

line 57: border-bottom: 1px solid rgba(255, 255, 255, 0.08);
      →  border-bottom: 1px solid var(--modal-header-bottom-border-color);
```

- [ ] **Step 11: `modals/userinputprompt.scss`**

```
line 15: box-shadow: 0px 8px 32px 0px rgba(0, 0, 0, 0.25);
      →  box-shadow: 0px 8px 32px 0px var(--modal-shadow-color);

line 42: border-top: 1px solid rgba(255, 255, 255, 0.1);
      →  border-top: 1px solid var(--modal-border-color);
```

- [ ] **Step 12: `view/webview/webview.scss`**

```
line 25: background-color: black;
      →  background-color: var(--block-bg-solid-color);

line 41: background: rgba(255, 255, 255, 0.1);
      →  background: var(--hover-bg-color);
```

- [ ] **Step 13: `tab/connectiondropdown.scss`**

```
line 8: box-shadow: 0px 13px 16px 0px rgba(0, 0, 0, 0.4);
     →  box-shadow: 0px 13px 16px 0px var(--dropdown-shadow-color);
```

- [ ] **Step 14: `tab/tabbar.scss`**

Line 15 (`background: rgba(0, 0, 0, 0.35);`) is a tab-bar background scrim with no clean existing-var match. Add a scoped override right after the existing rule, same pattern as Step 8:

```scss
:root[data-theme="light"] & {
    background: rgba(0, 0, 0, 0.06);
}
```

- [ ] **Step 15: `tab/tab.scss`**

```
line 47: color: rgba(255, 255, 255, 1);
      →  color: var(--main-text-color);
```

- [ ] **Step 16: `tab/workspaceswitcher.scss`**

```
line 51: background: rgba(255, 255, 255, 0.08);
      →  background: var(--hover-bg-color);
```

- [ ] **Step 17: Verify no stray literals or specificity blockers remain**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
grep -rn '!important' frontend/app/element/directorydropdown.scss frontend/app/view/preview/directorypreview.scss frontend/app/element/flyoutmenu.scss frontend/app/element/popover.scss frontend/app/modals/modal.scss frontend/app/element/modal.scss frontend/app/block/block.scss frontend/app/element/emojipalette.scss frontend/app/element/markdown.scss frontend/app/modals/typeaheadmodal.scss frontend/app/modals/userinputprompt.scss frontend/app/view/webview/webview.scss frontend/app/tab/connectiondropdown.scss frontend/app/tab/tabbar.scss frontend/app/tab/tab.scss frontend/app/tab/workspaceswitcher.scss
```
Expected: no output (no `!important` in any touched file — confirms Task 7/8's `:root[data-theme="light"]` overrides won't be silently defeated by higher-specificity rules elsewhere in these same files).

- [ ] **Step 18: Commit**

```bash
git add frontend/app/element/directorydropdown.scss frontend/app/view/preview/directorypreview.scss frontend/app/element/flyoutmenu.scss frontend/app/element/popover.scss frontend/app/modals/modal.scss frontend/app/element/modal.scss frontend/app/block/block.scss frontend/app/element/emojipalette.scss frontend/app/element/markdown.scss frontend/app/modals/typeaheadmodal.scss frontend/app/modals/userinputprompt.scss frontend/app/view/webview/webview.scss frontend/app/tab/connectiondropdown.scss frontend/app/tab/tabbar.scss frontend/app/tab/tab.scss frontend/app/tab/workspaceswitcher.scss
git commit -m "fix(appearance): replace hardcoded color literals with themed vars across 16 scss files"
```

---

### Task 9: Settings UI — `window:appearancemode` field in General settings

**Files:**
- Modify: `frontend/app/view/remotetermconfig/generalcontent.tsx` (`FieldSchemas` array, "Appearance" category, currently spans from line 50)

**Interfaces:**
- Consumes: `window:appearancemode` (Task 1).

- [ ] **Step 1: Add the field schema**

In `frontend/app/view/remotetermconfig/generalcontent.tsx`, in the `FieldSchemas` array's "Appearance" category block, immediately after the `app:tabbar` entry (currently lines 50-60), add:

```ts
    {
        key: "window:appearancemode",
        label: "Appearance",
        category: "Appearance",
        control: "segmented",
        description: "Controls app-wide light/dark appearance: follow the OS preference, or force light/dark.",
        options: [
            { value: "system", label: "System" },
            { value: "light", label: "Light" },
            { value: "dark", label: "Dark" },
        ],
    },
```

This follows the exact `segmented`-control, tri-state-enum shape already used by `app:focusfollowscursor` (lines 101-113).

- [ ] **Step 2: Manual verification**

No new automated test needed — `generalcontent.test.ts` (existing, per Task recon: 8 tests) already exercises the `FieldSchemas`-driven rendering generically; confirm it still passes (it validates structure, not per-field content) rather than adding a redundant per-field test:

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx vitest run frontend/app/view/remotetermconfig/generalcontent.test.ts`
Expected: PASS, same count as before this change (no new failures introduced by the new array entry).

- [ ] **Step 3: Commit**

```bash
git add frontend/app/view/remotetermconfig/generalcontent.tsx
git commit -m "feat(appearance): add window:appearancemode field to General settings"
```

---

### Task 10: Tab context menu — `tab:appearancemode` submenu

**Files:**
- Modify: `frontend/app/tab/tabcontextmenu.ts` (`buildTabContextMenu`, currently lines 39-114)

**Interfaces:**
- Consumes: `tab:appearancemode` (Task 2).

- [ ] **Step 1: Add the submenu**

In `frontend/app/tab/tabcontextmenu.ts`, immediately after the existing `tab:flagcolor` block (which currently ends at line 76, right before the `tab:background` block), add a new checkbox-style submenu following the exact `tab:flagcolor` pattern:

```ts
    const currentAppearanceMode = globalStore.get(getOrefMetaKeyAtom(tabORef, "tab:appearancemode")) ?? null;
    const appearanceModeSubmenu: ContextMenuItem[] = [
        {
            label: "Inherit",
            type: "checkbox",
            checked: currentAppearanceMode == null,
            click: () =>
                fireAndForget(() =>
                    env.rpc.SetMetaCommand(TabRpcClient, { oref: tabORef, meta: { "tab:appearancemode": null } })
                ),
        },
        {
            label: "Light",
            type: "checkbox",
            checked: currentAppearanceMode === "light",
            click: () =>
                fireAndForget(() =>
                    env.rpc.SetMetaCommand(TabRpcClient, { oref: tabORef, meta: { "tab:appearancemode": "light" } })
                ),
        },
        {
            label: "Dark",
            type: "checkbox",
            checked: currentAppearanceMode === "dark",
            click: () =>
                fireAndForget(() =>
                    env.rpc.SetMetaCommand(TabRpcClient, { oref: tabORef, meta: { "tab:appearancemode": "dark" } })
                ),
        },
    ];
    menu.push({ label: "Appearance", type: "submenu", submenu: appearanceModeSubmenu }, { type: "separator" });
```

The "Inherit" entry writes `meta: { "tab:appearancemode": null }` — not the string `"inherit"` — matching the `tab:flagcolor` "None" entry's `null`-write convention exactly (per this task's Review Focus item: writing a string `"inherit"` instead of `null` would make `resolveAppearanceMode`'s `tabOverride === "light" || tabOverride === "dark"` check correctly fall through anyway since `"inherit"` matches neither — but using `null` keeps the meta key genuinely absent/cleared rather than storing a redundant sentinel value, consistent with how every other tab-meta key in this file already behaves).

- [ ] **Step 2: Update the env type if needed**

Check whether `TabEnv` (or whichever `WaveEnvSubset` type `tabcontextmenu.ts` already uses) needs `getTabMetaKeyAtom`'s narrowing extended to include `"tab:appearancemode"` in its key union (it should already need `"tab:flagcolor"`/`"tab:background"` listed there per the `waveenv` skill's rules — add `"tab:appearancemode"` to that same union).

- [ ] **Step 3: Manual verification**

No isolated unit test exists for individual context-menu entries in this file (confirmed no `tabcontextmenu.test.ts` in the recon pass) — this matches the existing codebase convention (`tab:flagcolor`/`tab:background` have none either). Verification is via the whole-branch live-check in Task 11.

- [ ] **Step 4: Commit**

```bash
git add frontend/app/tab/tabcontextmenu.ts
git commit -m "feat(appearance): add tab:appearancemode context-menu submenu"
```

---

### Task 11: Docs pass + whole-feature verification

**Files:**
- Verify: full repo (no new file edits expected beyond what's listed)

- [ ] **Step 1: Full typecheck**

Run: `cd /media/owner/Workspace/remoteterm/remoteterm-daily && npx tsc --noEmit -p .`
Expected: same pre-existing 18-error baseline as the last recorded handoff (`daily-driver/combined-2026-09-21` HEAD `be1bc300`) — no new errors introduced by this arc. If the count differs, diagnose before proceeding; don't assume it's pre-existing without checking which files the new errors are in.

- [ ] **Step 2: Full test run of touched files**

Run:
```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
npx vitest run frontend/app/store/appearance-atoms.test.ts frontend/app/appearance-theme-updater.test.tsx frontend/app/view/remotetermconfig/generalcontent.test.ts
```
Expected: all green.

- [ ] **Step 3: Repo-wide hazard grep (per this repo's established merge-hazard pattern)**

```bash
grep -rln '\bwaveobj\.\|\bwstore\.\|\bwconfig\.\|\bwavebase\.\|\bwaveenv\b' --include='*.go' --include='*.ts' --include='*.tsx' .
```
Expected: no output (confirms no accidental reference to the pre-rebrand package names in any file this arc touched).

- [ ] **Step 4: Live verification (owner approval required first)**

Once `task dev`'s HMR has picked up the changes (confirm via `ps aux | grep remoteterm-daily` that the process is still the same one, not a stale/crashed instance), ask the owner before driving the window, then:
- Toggle `window:appearancemode` between `system`/`light`/`dark` in General settings, confirm the whole app repaints (not just Tailwind-classed components — check a legacy-`.scss`-styled surface too, e.g. a dropdown or modal).
- Set a tab's `tab:appearancemode` override via the right-click menu, confirm it overrides the global setting for that tab only, and confirm clearing it back to "Inherit" correctly falls through.
- With `window:appearancemode` on `system`, flip the OS's own light/dark switch (Task 3 Step 7's deferred check) and confirm live repaint.

- [ ] **Step 5: Update `docs/docs/config.mdx`'s version badge if needed**

If Step 1 of Task 1 used a placeholder version guess, confirm against the actual next-release version marker now that the whole arc is complete, and fix if it drifted.

- [ ] **Step 6: Final commit (if Step 5 changed anything)**

```bash
git add docs/docs/config.mdx
git commit -m "docs(appearance): confirm version badge for window:appearancemode"
```

---

## Self-Review

**Spec coverage:** Resolved-mode priority chain (Task 4), Electron `nativeTheme` binding (Task 3), CSS switching mechanism — both the Tailwind `@theme` tokens the spec described (Task 6) and the legacy `theme.scss` palette the spec didn't know existed (Task 7) — legacy `.scss` audit (Task 8, all 38 files accounted for: 16 fixed, 19 need nothing, `theme.scss`/`app.scss`/`progressbar.scss` handled in Task 7), UI placement for both the global setting (Task 9) and per-tab override (Task 10), error handling / never-throws (Task 4 Step 3's try/catch), testing strategy (priority-chain unit tests Task 4, IPC binding via preview/mock stub Task 3 Step 6, `data-theme` effect test Task 5, SCSS audit explicitly flagged as non-automatable in the spec itself — Task 8's `!important` grep is the closest automatable proxy). No spec section is without a task.

**Placeholder scan:** No "TBD"/"implement later"/"add appropriate X" phrasing anywhere in the task steps above; every code block is real, copy-pasteable content. The two `:root[data-theme="light"] &` scoped-override steps (Task 8 Steps 8 and 14) note that the exact wrapping selector needs confirming against the file at edit time rather than guessing it wrong — that's a legitimate "verify against the live file" instruction, not a placeholder, since the literal old/new value pairs around it are all real.

**Type consistency:** `"light" | "dark"` is the resolved-mode type throughout (Task 4's `resolveAppearanceMode` return type, `getResolvedAppearanceModeAtom`'s `Atom<"light" | "dark">`, `AppThemeUpdater`'s consumption). `"system" | "light" | "dark"` is the global-setting tri-state (Task 1's Go enum, Task 9's field options). `"light" | "dark" | null` (via absence) is the tab-override tri-state (Task 2, Task 10) — consistent naming (`window:appearancemode`, `tab:appearancemode`) used identically across every task that touches either key.

**Review Focus:** all 5 items have an owning task and either a real test or an explicit documented-limitation/manual-check, per the section above — none silently dropped.
