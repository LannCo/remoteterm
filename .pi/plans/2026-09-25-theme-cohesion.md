# Cohesive theme system - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the terminal default palette, the Monaco editor theme and the native window-control symbols follow the app's resolved appearance mode, so `window:appearancemode: light` (or a tab override, or an OS flip) repaints every surface together and without a reload.

**Architecture:** Point subscriptions. One new convenience atom, `resolvedAppearanceModeAtom` (the resolved mode for this window's tab, built from the existing `getResolvedAppearanceModeAtom` + `atoms.staticTabId`), is read directly by three subsystems: the terminal's `termThemeNameAtom` picks `default-light`/`default-dark` when no `term:theme` is set; a Monaco hook calls `monaco.editor.setTheme` on change; and `app-bg.tsx`'s existing titlebar sampler gains the mode as a dependency so the already-present `setTitleBarOverlay` path re-runs. No new IPC, no change to `nativeTheme.themeSource`.

**Tech Stack:** TypeScript/React frontend, Jotai, xterm.js, monaco-editor, Electron main/preload, vitest (+ happy-dom, @testing-library/react), colord (+ a11y plugin, already vendored).

**Spec:** `.pi/specs/2026-09-25-theme-cohesion-design.md`

One deviation from the spec's file placement, made so the Monaco code is unit-testable: `monacoThemeForMode` and `useMonacoAppearanceTheme` live in a new `frontend/app/monaco/monaco-theme.ts` rather than in `monaco-env.ts` / `monaco-react.tsx`. Loading `monaco-env.ts` in vitest drags in five `?worker` imports and `monaco-yaml`; the new file imports only `monaco-editor` (one `vi.mock`). Interfaces are exactly as the spec names them.

## Global Constraints

- Work in a fresh worktree, never in `remoteterm-daily` (it has a live `task dev`; editing product files there has crashed the live app before). Task 1 Step 1 creates `/media/owner/Workspace/remoteterm/remoteterm-theme-cohesion` on branch `feat/theme-cohesion` from `daily-driver/combined-2026-09-21`. Every path and command below is relative to that worktree root.
- Resolution priority is unchanged and never re-derived: `getResolvedAppearanceModeAtom` already encodes per-tab override > global `window:appearancemode` > OS preference > `dark`. New code only reads it.
- `nativeTheme.themeSource` stays `"system"` (`emain/emain.ts:69`). It feeds `osPrefersDarkAtom`; pinning it would loop the chain back on itself.
- A `term:theme` set anywhere (block meta, connection, `settings.json`) is honoured as-is in both modes. Only the unset case follows appearance mode.
- Every `default-light` colour must reach 4.5:1 contrast against `#ffffff` (enforced by Task 2's test).
- No `go build`; the palette change is JSON only and needs no `task generate`.
- Never bare `git stash` in this repo family (shared stash stack across worktrees).
- Run tests as `npx vitest run <path>` from the worktree root.
- Commit trailer on every commit: `Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>`.
- Style: 4-space indent, named exports only, `@/` imports across directories, relative within a directory, PascalCase global consts, no descriptive comments, copyright year 2026 on new files.
- Live-window work (Task 8) runs only on a nested Xvfb display with the exact launch sequence given there, never on the owner's `DISPLAY=:0`.

## Review Focus

1. **User config has `term:theme: default-dark` in `settings.json`, appearance is light.** Expected: terminal stays dark (explicit choice honoured). Pinned in Task 3 (`resolveTermThemeName` override test).
2. **A block's `term:theme` names a palette that is not in `fullConfig.termthemes`, appearance is light.** Expected: falls back to `default-light`, not `default-dark`. Pinned in Task 3 (`computeTheme` fallback test).
3. **OS flips light -> dark while a terminal, an editor and the titlebar are on screen, global setting `system`.** Expected: all three repaint live. Terminal already flows through `TermThemeUpdater`; Monaco pinned in Task 5 (hook re-calls `setTheme`); titlebar pinned in Task 6 (`updateWindowControlsOverlay` re-called).
4. **User replaces `termthemes.json` with a file lacking `default-light`.** Expected: no throw, terminal renders with whatever fallback exists or `{}`. Pinned in Task 3 (missing-both-themes test returns `{}` with transparent background).
5. **Monaco is opened before `loadMonaco()` has ever been called in this tab, appearance is light.** Expected: first editor paints light, not dark-then-light. Pinned in Task 5 (`loadMonaco` seeds from the atom; test asserts the initial `setTheme` argument).

---

## File structure

**New files:**
- `frontend/app/monaco/monaco-theme.ts` - `monacoThemeForMode`, `useMonacoAppearanceTheme`.
- `frontend/app/monaco/monaco-theme.test.tsx` - tests for both.
- `frontend/app/view/term/termthemes-contrast.test.ts` - WCAG contrast gate for `default-light`.
- `frontend/app/app-bg.test.tsx` - titlebar resample follows the mode.
- `.pi/evidence/2026-09-25-theme-cohesion/` - Task 8 screenshots.

**Modified files:**
- `frontend/app/store/appearance-atoms.ts` (+ `appearance-atoms.test.ts`) - `resolvedAppearanceModeAtom`.
- `pkg/rtconfig/defaultconfig/termthemes.json` - `default-light`, renumbered `display:order`.
- `frontend/app/view/term/termutil.ts` (+ `termutil.test.ts`) - `DefaultTermThemeLight`, `getDefaultTermThemeName`, `resolveTermThemeName`, `computeTheme` fallback parameter.
- `frontend/app/view/term/term-model.ts:233-252` - `termThemeNameAtom`, `blockBg`.
- `frontend/app/view/term/termtheme.ts:21`, `frontend/app/view/term/term.tsx:273-277` - `computeTheme` call sites.
- `frontend/app/monaco/monaco-env.ts:58-71` - light theme background, seeded `setTheme`.
- `frontend/app/monaco/monaco-react.tsx:25,128` - hook calls.
- `frontend/app/app-bg.tsx:20-50` - dependency.
- `docs/docs/config.mdx:56,88,235-283` - docs.

---

### Task 1: Worktree + `resolvedAppearanceModeAtom`

**Files:**
- Modify: `frontend/app/store/appearance-atoms.ts`
- Test: `frontend/app/store/appearance-atoms.ipc.test.ts`

**Interfaces:**
- Consumes: `getResolvedAppearanceModeAtom(tabId)`, `atoms.staticTabId` (both existing).
- Produces: `export const resolvedAppearanceModeAtom: Atom<"light" | "dark">` from `@/app/store/appearance-atoms`.

- [ ] **Step 1: Create the worktree**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-daily
git worktree add -b feat/theme-cohesion /media/owner/Workspace/remoteterm/remoteterm-theme-cohesion daily-driver/combined-2026-09-21
cd /media/owner/Workspace/remoteterm/remoteterm-theme-cohesion
npm install
```

Expected: `git worktree list` shows the new path on `feat/theme-cohesion`; `node_modules` present.

- [ ] **Step 2: Write the failing test**

Append to `frontend/app/store/appearance-atoms.ipc.test.ts`, inside the existing `describe` block, after the last `test(...)`. The mock for `@/app/store/global` at the top of that file must also supply `atoms`; change it to:

```ts
vi.mock("@/app/store/global", () => ({
    getApi: () => (window as any).api,
    getTabMetaKeyAtom: () => tabModeAtom,
    getSettingsKeyAtom: () => settingModeAtom,
    atoms: { staticTabId: staticTabIdAtom },
}));
```

and add `staticTabIdAtom: jotai.atom("tab-1")` to the `vi.hoisted` return object (destructure it alongside `tabModeAtom, settingModeAtom`). Then the test:

```ts
    test("resolvedAppearanceModeAtom follows the static tab's resolved mode", async () => {
        const { resolvedAppearanceModeAtom, globalStore } = await loadFresh();
        globalStore.set(settingModeAtom, "light");
        expect(globalStore.get(resolvedAppearanceModeAtom)).toBe("light");
        globalStore.set(tabModeAtom, "dark");
        expect(globalStore.get(resolvedAppearanceModeAtom)).toBe("dark");
    });
```

- [ ] **Step 3: Run test to verify it fails**

Run: `npx vitest run frontend/app/store/appearance-atoms.ipc.test.ts`
Expected: FAIL, `resolvedAppearanceModeAtom` is undefined.

- [ ] **Step 4: Implement**

In `frontend/app/store/appearance-atoms.ts`, change the import line to `import { atoms, getApi, getSettingsKeyAtom, getTabMetaKeyAtom } from "@/app/store/global";` and append at the end of the file:

```ts
export const resolvedAppearanceModeAtom: Atom<"light" | "dark"> = jotai.atom((get) => {
    return get(getResolvedAppearanceModeAtom(get(atoms.staticTabId)));
});
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `npx vitest run frontend/app/store/`
Expected: all appearance tests PASS, including the pre-existing ones.

- [ ] **Step 6: Commit**

```bash
git add frontend/app/store/appearance-atoms.ts frontend/app/store/appearance-atoms.ipc.test.ts
git commit -m "feat(appearance): add resolvedAppearanceModeAtom for the current tab

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 2: `default-light` palette + contrast gate

**Files:**
- Modify: `pkg/rtconfig/defaultconfig/termthemes.json`
- Create: `frontend/app/view/term/termthemes-contrast.test.ts`

**Interfaces:**
- Produces: `fullConfig.termthemes["default-light"]` with the same `TermThemeType` shape as `default-dark`.

- [ ] **Step 1: Write the failing test**

Create `frontend/app/view/term/termthemes-contrast.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { colord, extend } from "colord";
import a11yPlugin from "colord/plugins/a11y";
import { describe, expect, test } from "vitest";
import termthemes from "../../../../pkg/rtconfig/defaultconfig/termthemes.json";

extend([a11yPlugin]);

const MinContrast = 4.5;
const ForegroundKeys = [
    "black",
    "red",
    "green",
    "yellow",
    "blue",
    "magenta",
    "cyan",
    "white",
    "brightBlack",
    "brightRed",
    "brightGreen",
    "brightYellow",
    "brightBlue",
    "brightMagenta",
    "brightCyan",
    "brightWhite",
    "gray",
    "cmdtext",
    "foreground",
] as const;

describe("default-light palette", () => {
    const theme = (termthemes as Record<string, Record<string, string>>)["default-light"];

    test("exists with a white background and display metadata", () => {
        expect(theme).toBeDefined();
        expect(theme.background).toBe("#ffffff");
        expect(theme["display:name"]).toBe("Default Light");
    });

    test.each(ForegroundKeys)("%s reaches WCAG AA contrast on the background", (key) => {
        const value = theme[key];
        expect(value, `${key} missing`).toBeTruthy();
        const ratio = colord(value).contrast(theme.background);
        expect(ratio, `${key}=${value} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(MinContrast);
    });

    test("display:order values are unique across built-in palettes", () => {
        const orders = Object.values(termthemes as Record<string, Record<string, unknown>>).map(
            (t) => t["display:order"]
        );
        expect(new Set(orders).size).toBe(orders.length);
    });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run frontend/app/view/term/termthemes-contrast.test.ts`
Expected: FAIL, `theme` is undefined.

- [ ] **Step 3: Add the palette and renumber**

In `pkg/rtconfig/defaultconfig/termthemes.json`, insert after the `default-dark` object (before `"onedarkpro"`):

```json
    "default-light": {
        "display:name": "Default Light",
        "display:order": 2,
        "black": "#3c3c3c",
        "red": "#b3261e",
        "green": "#1e7a1e",
        "yellow": "#8a6d00",
        "blue": "#1f5fa8",
        "magenta": "#9a2c9a",
        "cyan": "#0b6e85",
        "white": "#5c5c5c",
        "brightBlack": "#6b6b6b",
        "brightRed": "#a01c1c",
        "brightGreen": "#2a6b2a",
        "brightYellow": "#7a5c00",
        "brightBlue": "#2a5c9a",
        "brightMagenta": "#7f2f8a",
        "brightCyan": "#1c6a7a",
        "brightWhite": "#1a1a1a",
        "gray": "#6b6f6a",
        "cmdtext": "#1a1a1a",
        "foreground": "#1a1a1a",
        "selectionBackground": "",
        "background": "#ffffff",
        "cursor": ""
    },
```

Then change `display:order` on the remaining six: `onedarkpro` 2 -> 3, `dracula` 3 -> 4, `monokai` 4 -> 5, `campbell` 5 -> 6, `warmyellow` 6 -> 7, `rosepine` 7 -> 8.

These values were checked against white with colord's a11y plugin before this plan was written; the lowest is `yellow` at 4.92:1. The owner reviews them rendered (Task 9) and may adjust; the test keeps any adjustment legible.

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run frontend/app/view/term/termthemes-contrast.test.ts`
Expected: PASS, 21 tests.

- [ ] **Step 5: Confirm the preview mock picks it up without edits**

Run: `grep -n "termthemes.json" frontend/preview/mock/defaultconfig.ts`
Expected: one line importing the JSON directly. No change needed.

- [ ] **Step 6: Commit**

```bash
git add pkg/rtconfig/defaultconfig/termthemes.json frontend/app/view/term/termthemes-contrast.test.ts
git commit -m "feat(term): add default-light palette with WCAG contrast gate

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 3: Theme-name resolution helpers in `termutil.ts`

**Files:**
- Modify: `frontend/app/view/term/termutil.ts:4,33-55`
- Modify: `frontend/app/view/term/termtheme.ts:21`
- Modify: `frontend/app/view/term/term.tsx:273-277`
- Modify: `frontend/app/view/term/term-model.ts:244-252`
- Test: `frontend/app/view/term/termutil.test.ts`

**Interfaces:**
- Consumes: `resolvedAppearanceModeAtom` (Task 1).
- Produces, all exported from `@/app/view/term/termutil`:
  - `DefaultTermThemeLight = "default-light"`
  - `getDefaultTermThemeName(mode: "light" | "dark"): string`
  - `resolveTermThemeName(override: string | null | undefined, mode: "light" | "dark"): string`
  - `computeTheme(fullConfig: FullConfigType, themeName: string, termTransparency: number, fallbackThemeName: string): [TermThemeType, string]` (fourth parameter is new and required).

- [ ] **Step 1: Write the failing tests**

Append to `frontend/app/view/term/termutil.test.ts`. Extend the existing import line `import { createRemoteTempFileFromBlob } from "./termutil";` to:

```ts
import {
    computeTheme,
    createRemoteTempFileFromBlob,
    DefaultTermTheme,
    DefaultTermThemeLight,
    getDefaultTermThemeName,
    resolveTermThemeName,
} from "./termutil";
```

Then add at the end of the file:

```ts
describe("getDefaultTermThemeName", () => {
    it("maps dark to default-dark and light to default-light", () => {
        expect(getDefaultTermThemeName("dark")).toBe(DefaultTermTheme);
        expect(getDefaultTermThemeName("light")).toBe(DefaultTermThemeLight);
    });
});

describe("resolveTermThemeName", () => {
    it("honours an explicit override in both modes", () => {
        expect(resolveTermThemeName("dracula", "light")).toBe("dracula");
        expect(resolveTermThemeName("default-dark", "light")).toBe("default-dark");
        expect(resolveTermThemeName("default-light", "dark")).toBe("default-light");
    });

    it("follows the appearance mode when no override is set", () => {
        expect(resolveTermThemeName(null, "light")).toBe("default-light");
        expect(resolveTermThemeName(undefined, "dark")).toBe("default-dark");
    });
});

describe("computeTheme fallback", () => {
    const fullConfig = {
        termthemes: {
            "default-dark": { background: "#000000", foreground: "#c1c1c1" },
            "default-light": { background: "#ffffff", foreground: "#1a1a1a" },
        },
    } as unknown as FullConfigType;

    it("uses the named theme when present", () => {
        const [theme, bg] = computeTheme(fullConfig, "default-dark", 0, "default-light");
        expect(bg).toBe("#000000");
        expect(theme.foreground).toBe("#c1c1c1");
        expect(theme.background).toBe("#00000000");
    });

    it("falls back to the supplied fallback when the named theme is missing", () => {
        const [theme, bg] = computeTheme(fullConfig, "not-a-theme", 0, "default-light");
        expect(bg).toBe("#ffffff");
        expect(theme.foreground).toBe("#1a1a1a");
    });

    it("returns an empty theme with transparent background when both are missing", () => {
        const [theme, bg] = computeTheme({ termthemes: {} } as unknown as FullConfigType, "x", 0, "y");
        expect(bg).toBeUndefined();
        expect(theme.background).toBe("#00000000");
    });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npx vitest run frontend/app/view/term/termutil.test.ts`
Expected: FAIL, `DefaultTermThemeLight` / `getDefaultTermThemeName` / `resolveTermThemeName` undefined.

- [ ] **Step 3: Implement in `termutil.ts`**

Replace line 4 (`export const DefaultTermTheme = "default-dark";`) with:

```ts
export const DefaultTermTheme = "default-dark";
export const DefaultTermThemeLight = "default-light";

export function getDefaultTermThemeName(mode: "light" | "dark"): string {
    return mode === "light" ? DefaultTermThemeLight : DefaultTermTheme;
}

export function resolveTermThemeName(override: string | null | undefined, mode: "light" | "dark"): string {
    if (override != null) {
        return override;
    }
    return getDefaultTermThemeName(mode);
}
```

Replace the `computeTheme` signature and first fallback (lines 34-42) with:

```ts
export function computeTheme(
    fullConfig: FullConfigType,
    themeName: string,
    termTransparency: number,
    fallbackThemeName: string
): [TermThemeType, string] {
    let theme: TermThemeType = fullConfig?.termthemes?.[themeName];
    if (theme == null) {
        theme = fullConfig?.termthemes?.[fallbackThemeName] || ({} as any);
    }
```

The rest of the function is unchanged.

- [ ] **Step 4: Update the three call sites**

`frontend/app/view/term/termtheme.ts`: add imports and pass the fallback.

```ts
import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import type { TermViewModel } from "@/app/view/term/term-model";
import { computeTheme, getDefaultTermThemeName } from "@/app/view/term/termutil";
```

and inside `TermThemeUpdater`, after `const transparency = useAtomValue(model.termTransparencyAtom);`:

```ts
    const appearanceMode = useAtomValue(resolvedAppearanceModeAtom);
    const [theme, _] = computeTheme(fullConfig, blockTermTheme, transparency, getDefaultTermThemeName(appearanceMode));
```

(replacing the existing `const [theme, _] = computeTheme(...)` line).

`frontend/app/view/term/term.tsx`: add `import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";` next to the other `@/app/store` imports, extend the `./termutil` import to include `getDefaultTermThemeName`, and change line 277 to:

```ts
        const appearanceMode = globalStore.get(resolvedAppearanceModeAtom);
        const [termTheme, _] = computeTheme(
            fullConfig,
            termThemeName,
            termTransparency,
            getDefaultTermThemeName(appearanceMode)
        );
```

`frontend/app/view/term/term-model.ts`: add `import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";`, extend the `./termutil` import (line 38) to include `getDefaultTermThemeName`, and change `blockBg` (currently lines 244-252) to:

```ts
        this.blockBg = jotai.atom((get) => {
            const fullConfig = get(atoms.fullConfigAtom);
            const themeName = get(this.termThemeNameAtom);
            const termTransparency = get(this.termTransparencyAtom);
            const appearanceMode = get(resolvedAppearanceModeAtom);
            const [_, bgcolor] = computeTheme(
                fullConfig,
                themeName,
                termTransparency,
                getDefaultTermThemeName(appearanceMode)
            );
            if (bgcolor != null) {
                return { bg: bgcolor };
            }
            return null;
        });
```

- [ ] **Step 5: Run tests and typecheck**

Run: `npx vitest run frontend/app/view/term/termutil.test.ts && npx tsc --noEmit -p tsconfig.json`
Expected: tests PASS; tsc reports no errors (a missing fourth argument anywhere would fail here, which is why the parameter is required).

- [ ] **Step 6: Commit**

```bash
git add frontend/app/view/term/termutil.ts frontend/app/view/term/termutil.test.ts frontend/app/view/term/termtheme.ts frontend/app/view/term/term.tsx frontend/app/view/term/term-model.ts
git commit -m "feat(term): mode-aware default palette helpers and computeTheme fallback

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 4: `termThemeNameAtom` follows appearance mode

**Files:**
- Modify: `frontend/app/view/term/term-model.ts:233-237`

**Interfaces:**
- Consumes: `resolveTermThemeName`, `resolvedAppearanceModeAtom`.
- Produces: unchanged `termThemeNameAtom: jotai.Atom<string>`; its value now follows the mode when no override is set. All existing consumers (`blockBg`, `TermThemeUpdater`, `term.tsx`, `getSettingsMenuItems`) keep working unchanged.

- [ ] **Step 1: Change the atom**

Replace lines 233-237:

```ts
        this.termThemeNameAtom = useBlockAtom(blockId, "termthemeatom", () => {
            return jotai.atom<string>((get) => {
                const override = get(getOverrideConfigAtom(this.blockId, "term:theme"));
                return resolveTermThemeName(override, get(resolvedAppearanceModeAtom));
            });
        });
```

Extend the `./termutil` import (line 38) to include `resolveTermThemeName`. `DefaultTermTheme` stays imported only if still referenced elsewhere in the file; run `grep -n DefaultTermTheme frontend/app/view/term/term-model.ts` and drop it from the import if the only hit is the import line.

- [ ] **Step 2: Typecheck and run the term test folder**

Run: `npx tsc --noEmit -p tsconfig.json && npx vitest run frontend/app/view/term/`
Expected: no tsc errors; all term tests PASS.

- [ ] **Step 3: Confirm no consumer still hardcodes the dark default**

Run: `grep -rn "DefaultTermTheme\b" frontend/app --include=*.ts --include=*.tsx | grep -v termutil`
Expected: no matches outside `termutil.ts` (and its test).

- [ ] **Step 4: Commit**

```bash
git add frontend/app/view/term/term-model.ts
git commit -m "feat(term): default palette follows resolved appearance mode

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 5: Monaco follows appearance mode

**Files:**
- Create: `frontend/app/monaco/monaco-theme.ts`
- Create: `frontend/app/monaco/monaco-theme.test.tsx`
- Modify: `frontend/app/monaco/monaco-env.ts:58-71`
- Modify: `frontend/app/monaco/monaco-react.tsx:25-32,128-135`

**Interfaces:**
- Consumes: `resolvedAppearanceModeAtom`.
- Produces, from `@/app/monaco/monaco-theme`:
  - `monacoThemeForMode(mode: "light" | "dark"): "wave-theme-light" | "wave-theme-dark"`
  - `useMonacoAppearanceTheme(): void` (React hook)

- [ ] **Step 1: Write the failing tests**

Create `frontend/app/monaco/monaco-theme.test.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, renderHook } from "@testing-library/react";
import { createStore, PrimitiveAtom, Provider } from "jotai";
import { beforeEach, describe, expect, test, vi } from "vitest";

const { modeAtom, setTheme } = vi.hoisted(() => {
    const jotai = require("jotai");
    return {
        modeAtom: jotai.atom("dark") as PrimitiveAtom<"light" | "dark">,
        setTheme: vi.fn(),
    };
});

vi.mock("monaco-editor", () => ({ editor: { setTheme } }));
vi.mock("@/app/store/appearance-atoms", () => ({ resolvedAppearanceModeAtom: modeAtom }));

import { monacoThemeForMode, useMonacoAppearanceTheme } from "./monaco-theme";

describe("monacoThemeForMode", () => {
    test("maps modes to the two defined Monaco themes", () => {
        expect(monacoThemeForMode("dark")).toBe("wave-theme-dark");
        expect(monacoThemeForMode("light")).toBe("wave-theme-light");
    });
});

describe("useMonacoAppearanceTheme", () => {
    beforeEach(() => {
        setTheme.mockClear();
    });

    test("applies the current mode on mount and again when it changes", () => {
        const store = createStore();
        store.set(modeAtom, "light");
        renderHook(() => useMonacoAppearanceTheme(), {
            wrapper: ({ children }) => <Provider store={store}>{children}</Provider>,
        });
        expect(setTheme).toHaveBeenLastCalledWith("wave-theme-light");

        act(() => {
            store.set(modeAtom, "dark");
        });
        expect(setTheme).toHaveBeenLastCalledWith("wave-theme-dark");
        expect(setTheme).toHaveBeenCalledTimes(2);
    });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npx vitest run frontend/app/monaco/monaco-theme.test.tsx`
Expected: FAIL, module `./monaco-theme` not found.

- [ ] **Step 3: Create `monaco-theme.ts`**

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { useAtomValue } from "jotai";
import * as monaco from "monaco-editor";
import { useEffect } from "react";

export function monacoThemeForMode(mode: "light" | "dark"): "wave-theme-light" | "wave-theme-dark" {
    return mode === "light" ? "wave-theme-light" : "wave-theme-dark";
}

export function useMonacoAppearanceTheme(): void {
    const mode = useAtomValue(resolvedAppearanceModeAtom);
    useEffect(() => {
        monaco.editor.setTheme(monacoThemeForMode(mode));
    }, [mode]);
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npx vitest run frontend/app/monaco/monaco-theme.test.tsx`
Expected: PASS, 2 tests.

- [ ] **Step 5: Seed `loadMonaco()` and fix the light background**

In `frontend/app/monaco/monaco-env.ts` add imports:

```ts
import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { globalStore } from "@/app/store/jotaiStore";
import { monacoThemeForMode } from "@/app/monaco/monaco-theme";
```

Change the `wave-theme-light` definition's `"editor.background": "#fefefe"` (line 63) to `"editor.background": "#00000000"`.

Replace line 71 `monaco.editor.setTheme("wave-theme-dark");` with:

```ts
    monaco.editor.setTheme(monacoThemeForMode(globalStore.get(resolvedAppearanceModeAtom)));
```

- [ ] **Step 6: Call the hook from both editor components**

In `frontend/app/monaco/monaco-react.tsx` add `import { useMonacoAppearanceTheme } from "@/app/monaco/monaco-theme";`. In `MonacoCodeEditor` (line 25), add `useMonacoAppearanceTheme();` as the first line of the function body, before the `useRef` calls. In `MonacoDiffViewer` (line 128), add the same line as the first line of its body. Hooks must sit above any conditional return (project rule).

- [ ] **Step 7: Typecheck and run the monaco + sourcecontrol tests**

Run: `npx tsc --noEmit -p tsconfig.json && npx vitest run frontend/app/monaco/ frontend/app/view/sourcecontrol/`
Expected: no tsc errors; PASS (the sourcecontrol test mocks `@/app/monaco/monaco-react` wholesale, so it is unaffected).

- [ ] **Step 8: Commit**

```bash
git add frontend/app/monaco/monaco-theme.ts frontend/app/monaco/monaco-theme.test.tsx frontend/app/monaco/monaco-env.ts frontend/app/monaco/monaco-react.tsx
git commit -m "feat(monaco): editor theme follows resolved appearance mode

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 6: Titlebar overlay re-samples on appearance change

**Files:**
- Modify: `frontend/app/app-bg.tsx:20-50`
- Create: `frontend/app/app-bg.test.tsx`

**Interfaces:**
- Consumes: `resolvedAppearanceModeAtom`; existing `getApi().updateWindowControlsOverlay(rect)`.
- Produces: no new exports. Behaviour: `updateWindowControlsOverlay` is called again whenever the resolved mode changes.

- [ ] **Step 1: Write the failing test**

Create `frontend/app/app-bg.test.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, render } from "@testing-library/react";
import { createStore, PrimitiveAtom, Provider } from "jotai";
import { beforeEach, describe, expect, test, vi } from "vitest";

const { modeAtom, nullAtom, updateWindowControlsOverlay } = vi.hoisted(() => {
    const jotai = require("jotai");
    return {
        modeAtom: jotai.atom("dark") as PrimitiveAtom<"light" | "dark">,
        nullAtom: jotai.atom(null),
        updateWindowControlsOverlay: vi.fn(),
    };
});

vi.mock("@/app/store/appearance-atoms", () => ({ resolvedAppearanceModeAtom: modeAtom }));
vi.mock("@/util/platformutil", () => ({ PLATFORM: "linux", PlatformMacOS: "darwin" }));
vi.mock("@/util/remotetermutil", () => ({ computeBgStyleFromMeta: () => ({}) }));
vi.mock("@react-hook/resize-observer", () => ({ default: () => {} }));
vi.mock("throttle-debounce", () => ({ debounce: (_ms: number, fn: () => void) => fn }));
vi.mock("@/app/remotetermenv/remotetermenv", () => ({
    useWaveEnv: () => ({
        getTabMetaKeyAtom: () => nullAtom,
        getConfigBackgroundAtom: () => nullAtom,
    }),
}));
vi.mock("./store/global", () => ({
    atoms: { staticTabId: nullAtom },
    getApi: () => ({ updateWindowControlsOverlay }),
    WOS: { makeORef: (t: string, id: string) => `${t}:${id}` },
}));
vi.mock("./store/wos", () => ({ useWaveObjectValue: () => [{ meta: {} }] }));

import { AppBackground } from "./app-bg";

describe("AppBackground titlebar sampling", () => {
    beforeEach(() => {
        updateWindowControlsOverlay.mockClear();
        (window.navigator as any).windowControlsOverlay = {
            getTitlebarAreaRect: () => ({ top: 0, left: 0, width: 800, height: 32 }),
        };
    });

    test("re-samples the titlebar when the resolved appearance mode changes", () => {
        const store = createStore();
        store.set(modeAtom, "dark");
        render(
            <Provider store={store}>
                <AppBackground />
            </Provider>
        );
        expect(updateWindowControlsOverlay).toHaveBeenCalledTimes(1);

        act(() => {
            store.set(modeAtom, "light");
        });
        expect(updateWindowControlsOverlay).toHaveBeenCalledTimes(2);
    });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run frontend/app/app-bg.test.tsx`
Expected: FAIL on the second assertion: called 1 time, expected 2.

- [ ] **Step 3: Add the dependency**

In `frontend/app/app-bg.tsx` add `import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";`. Inside `AppBackground`, after `const configBg = useAtomValue(env.getConfigBackgroundAtom(tabBg));` add:

```ts
    const appearanceMode = useAtomValue(resolvedAppearanceModeAtom);
```

and change the `useCallback` dependency array (currently `[bgRef, style]`) to `[bgRef, style, appearanceMode]`.

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run frontend/app/app-bg.test.tsx`
Expected: PASS.

- [ ] **Step 5: Confirm main-side handler is untouched and `themeSource` unchanged**

Run: `git diff --stat emain/ && grep -n 'themeSource' emain/emain.ts`
Expected: no `emain/` changes; `emain.ts:69` still `electron.nativeTheme.themeSource = "system";`.

- [ ] **Step 6: Commit**

```bash
git add frontend/app/app-bg.tsx frontend/app/app-bg.test.tsx
git commit -m "fix(chrome): re-sample titlebar overlay colour on appearance change

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 7: Documentation

**Files:**
- Modify: `docs/docs/config.mdx:56,88,235-283`

- [ ] **Step 1: `term:theme` row (line 56)**

Change the description cell to:

```
preset name of terminal theme to apply by default. When unset, the terminal follows the appearance mode: `default-dark` in dark mode, `default-light` in light mode. A named theme is used as-is in both modes.
```

- [ ] **Step 2: `window:appearancemode` row (line 88)**

Append to the description cell:

```
 Drives the app chrome, the terminal's default palette, the code editor theme, and the window-control symbol colour.
```

- [ ] **Step 3: Terminal themes section (lines 235-283)**

After the sentence ending `This uses the JSON key value as the identifier.` add a new paragraph:

```
Two built-in palettes are tied to the appearance mode: `default-dark` and `default-light`. A terminal with no `term:theme` set uses whichever matches the current mode. Setting `term:theme` (including to `default-dark` or `default-light`) pins that palette regardless of mode.
```

Update the `wsh setmeta this term:theme="default-dark"` example's surrounding text if it claims that is the default (it does not need to change otherwise).

- [ ] **Step 4: Check the file still renders**

Run: `npx prettier --check docs/docs/config.mdx`
Expected: formatted (or run `npx prettier --write` on that file if the repo's prettier config formats mdx).

- [ ] **Step 5: Commit**

```bash
git add docs/docs/config.mdx
git commit -m "docs(config): term:theme and window:appearancemode describe mode-following surfaces

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 8: Live evidence under nested Xvfb

**Files:**
- Create: `.pi/evidence/2026-09-25-theme-cohesion/*.png` and `isolation.txt`

**Interfaces:**
- Consumes: the built app from this worktree. Nothing downstream consumes the screenshots except the owner's review (Task 9).

This task is the only one that runs the app. It never touches the owner's display. The commands below are the launch sequence; run them exactly, in one shell.

- [ ] **Step 1: Build**

```bash
cd /media/owner/Workspace/remoteterm/remoteterm-theme-cohesion
task build:backend
npm run build:dev
```

Expected: `dist/` populated, no errors.

- [ ] **Step 2: Start the nested display and prove the shell is detached from the owner's**

```bash
unset DISPLAY WAYLAND_DISPLAY
Xvfb :99 -screen 0 1600x1000x24 -nolisten tcp &
XVFB_PID=$!
sleep 1
export DISPLAY=:99
xdpyinfo -display :99 | head -3
```

Expected: `xdpyinfo` prints `name of display: :99`. If it prints anything else, stop.

- [ ] **Step 3: Scratch config and data homes, global light**

```bash
export EVID=/media/owner/Workspace/remoteterm/remoteterm-theme-cohesion/.pi/evidence/2026-09-25-theme-cohesion
export SCRATCH=/tmp/claude-1000/theme-cohesion-xvfb
rm -rf "$SCRATCH"; mkdir -p "$SCRATCH/config/remoteterm" "$SCRATCH/config/remoteterm-dev" "$SCRATCH/data" "$EVID"
export XDG_CONFIG_HOME="$SCRATCH/config" XDG_DATA_HOME="$SCRATCH/data"
for d in remoteterm remoteterm-dev; do
cat > "$SCRATCH/config/$d/settings.json" <<'EOF'
{ "window:appearancemode": "light", "window:nativetitlebar": false }
EOF
done
```

`emain/emain-platform.ts:42-44` names the config dir `remoteterm` or `remoteterm-dev` depending on `isDev`; writing both covers either build mode.

- [ ] **Step 4: Launch and prove isolation**

```bash
REMOTETERM_NOCONFIRMQUIT=1 npm run start > "$SCRATCH/app.log" 2>&1 &
APP_PID=$!
sleep 15
ELECTRON_PID=$(pgrep -f "electron.*remoteterm-theme-cohesion" | head -1)
tr '\0' '\n' < /proc/$ELECTRON_PID/environ | grep -E '^(DISPLAY|XDG_CONFIG_HOME)=' | tee "$EVID/isolation.txt"
xdotool search --onlyvisible --name "RemoteTerm" | head -1
```

Expected: `isolation.txt` contains `DISPLAY=:99` and `XDG_CONFIG_HOME=/tmp/claude-1000/theme-cohesion-xvfb/config`. If `DISPLAY` is anything but `:99`, kill the app immediately (`kill $APP_PID`) and stop; do not proceed. `xdotool` prints a window id.

- [ ] **Step 5: Capture global light**

```bash
WID=$(xdotool search --onlyvisible --name "RemoteTerm" | head -1)
xdotool windowactivate --sync $WID
xdotool type --delay 40 'ls --color=always /usr/lib | head -20; printf "\e[31mred \e[32mgreen \e[33myellow \e[34mblue \e[35mmagenta \e[36mcyan\e[0m\n"'
xdotool key Return; sleep 2
import -window root "$EVID/01-global-light-terminal.png"
xdotool type --delay 40 'wsh view /media/owner/Workspace/remoteterm/remoteterm-theme-cohesion/frontend/app/monaco/monaco-theme.ts'
xdotool key Return; sleep 4
import -window root "$EVID/02-global-light-editor.png"
import -window root -crop 1600x40+0+0 "$EVID/03-global-light-titlebar.png"
```

- [ ] **Step 6: Capture tab override dark (global still light)**

```bash
xdotool type --delay 40 'wsh setmeta -b tab tab:appearancemode=dark'
xdotool key Return; sleep 3
import -window root "$EVID/04-taboverride-dark-terminal-editor.png"
import -window root -crop 1600x40+0+0 "$EVID/05-taboverride-dark-titlebar.png"
xdotool type --delay 40 'wsh setmeta -b tab tab:appearancemode=null'
xdotool key Return; sleep 2
```

`tab` is a built-in `SimpleId` keyword (`pkg/wshrpc/wshserver/resolvers.go:22`) and `key=null` clears a meta key (`cmd/wsh/cmd/wshcmd-setmeta.go:63`).

- [ ] **Step 7: Capture global dark via live setting change**

```bash
xdotool type --delay 40 'wsh setconfig window:appearancemode=dark'
xdotool key Return; sleep 3
import -window root "$EVID/06-global-dark-terminal-editor.png"
import -window root -crop 1600x40+0+0 "$EVID/07-global-dark-titlebar.png"
```

- [ ] **Step 8: Tear down**

```bash
kill $APP_PID; sleep 2; pkill -f "electron.*remoteterm-theme-cohesion" || true
kill $XVFB_PID
ls -la "$EVID"
```

Expected: seven PNGs and `isolation.txt`; no `Xvfb :99` or app processes remain (`pgrep -f 'Xvfb :99'` empty).

- [ ] **Step 9: Record what the screenshots show, then commit**

Write `$EVID/README.md` with one line per PNG stating what is visible (light/dark terminal foreground and background, editor theme, symbol colour), as observed, not as expected. Then:

```bash
git add .pi/evidence/2026-09-25-theme-cohesion/
git commit -m "test(theme): nested-Xvfb screenshot evidence for light/dark surfaces

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 9: Full test run and owner checklist

**Files:**
- Create: `.pi/evidence/2026-09-25-theme-cohesion/CHECKLIST.md`

- [ ] **Step 1: Whole-suite run**

Run: `npx vitest run`
Expected: all green, including the pre-existing suites. Record the totals line in the checklist file.

- [ ] **Step 2: Typecheck**

Run: `npx tsc --noEmit -p tsconfig.json`
Expected: no errors.

- [ ] **Step 3: Write the owner checklist**

Create `.pi/evidence/2026-09-25-theme-cohesion/CHECKLIST.md`:

```markdown
# Theme cohesion: owner sign-off

Run against your own `task dev` session. Tick each line when observed.

Vitest totals from Task 9: <paste>

## Global light (`window:appearancemode: light`, no tab override)
- [ ] Terminal with no `term:theme`: dark text on white, ANSI colours legible (`ls --color`, the printf line from Task 8).
- [ ] Code editor block: light Monaco theme (vs base), transparent editor background over the app background.
- [ ] Linux/Windows only: window-control symbols are dark on the light titlebar.
- [ ] Terminal with `term:theme` set to Dracula (right-click > Themes): stays Dracula.

## Tab override dark (right-click tab > Appearance > Dark, global still light)
- [ ] Terminal, editor and symbols in that tab flip to dark without reload; other tabs stay light.
- [ ] Setting the tab back to Default restores light.

## Global dark
- [ ] Everything as before this arc: `default-dark` terminal, dark Monaco, light symbols.

## OS flip (global `system`)
- [ ] Toggle the desktop light/dark setting with the app open: all three surfaces follow.

## Palette review
- [ ] `default-light` colours acceptable as rendered. Adjustments go in `pkg/rtconfig/defaultconfig/termthemes.json`; `termthemes-contrast.test.ts` enforces 4.5:1.
```

- [ ] **Step 4: Commit and report**

```bash
git add .pi/evidence/2026-09-25-theme-cohesion/CHECKLIST.md
git commit -m "docs(theme): owner sign-off checklist

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
git log --oneline daily-driver/combined-2026-09-21..HEAD
```

Report the branch, the commit list, the vitest totals and the evidence directory path. The plan ends here; merging into `daily-driver/combined-2026-09-21` is the owner's call after the checklist.
