// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import type { PrimitiveAtom } from "jotai";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";

const { tabModeAtom, settingModeAtom } = vi.hoisted(() => {
    const jotai = require("jotai");
    return {
        tabModeAtom: jotai.atom(null) as PrimitiveAtom<string>,
        settingModeAtom: jotai.atom("system") as PrimitiveAtom<string>,
    };
});

vi.mock("@/app/store/global", () => ({
    getApi: () => (window as any).api,
    getTabMetaKeyAtom: () => tabModeAtom,
    getSettingsKeyAtom: () => settingModeAtom,
}));

async function loadFresh() {
    vi.resetModules();
    const atomsModule = await import("./appearance-atoms");
    const { globalStore } = await import("@/app/store/jotaiStore");
    return { ...atomsModule, globalStore };
}

describe("appearance-atoms native-theme IPC binding", () => {
    let nativeThemeCallback: (shouldUseDarkColors: boolean) => void;

    beforeEach(() => {
        nativeThemeCallback = null;
        (window as any).api = {
            getNativeTheme: () => false,
            onNativeThemeChange: (cb: (shouldUseDarkColors: boolean) => void) => {
                nativeThemeCallback = cb;
            },
        };
        vi.spyOn(console, "log").mockImplementation(() => {});
    });

    afterEach(() => {
        delete (window as any).api;
        vi.restoreAllMocks();
    });

    test("seeds from the preload response and follows native-theme updates", async () => {
        const { osPrefersDarkAtom, globalStore } = await loadFresh();
        expect(globalStore.get(osPrefersDarkAtom)).toBe(false);
        expect(nativeThemeCallback).not.toBeNull();

        nativeThemeCallback(true);
        expect(globalStore.get(osPrefersDarkAtom)).toBe(true);

        nativeThemeCallback(false);
        expect(globalStore.get(osPrefersDarkAtom)).toBe(false);
    });

    test("resolved atom flips with a live OS change when global setting is system", async () => {
        const { getResolvedAppearanceModeAtom, globalStore } = await loadFresh();
        globalStore.set(tabModeAtom, null);
        globalStore.set(settingModeAtom, "system");
        const resolved = getResolvedAppearanceModeAtom("tab-ipc-1");
        expect(globalStore.get(resolved)).toBe("light");

        nativeThemeCallback(true);
        expect(globalStore.get(resolved)).toBe("dark");

        globalStore.set(tabModeAtom, "light");
        expect(globalStore.get(resolved)).toBe("light");
    });

    test("missing preload API never throws and degrades to dark", async () => {
        delete (window as any).api;
        const { osPrefersDarkAtom, getResolvedAppearanceModeAtom, globalStore } = await loadFresh();
        expect(globalStore.get(osPrefersDarkAtom)).toBe(true);
        globalStore.set(tabModeAtom, null);
        globalStore.set(settingModeAtom, "system");
        expect(globalStore.get(getResolvedAppearanceModeAtom("tab-ipc-2"))).toBe("dark");
        expect(console.log).toHaveBeenCalled();
    });
});
