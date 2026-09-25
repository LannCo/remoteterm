// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getApi, getSettingsKeyAtom, getTabMetaKeyAtom } from "@/app/store/global";
import { globalStore } from "@/app/store/jotaiStore";
import * as jotai from "jotai";
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

const appearanceModeAtomCache = new Map<string, Atom<"light" | "dark">>();

export function getResolvedAppearanceModeAtom(tabId: string): Atom<"light" | "dark"> {
    const cached = appearanceModeAtomCache.get(tabId);
    if (cached != null) {
        return cached;
    }
    const tabOverrideAtom = getTabMetaKeyAtom(tabId, "tab:appearancemode");
    const globalSettingAtom = getSettingsKeyAtom("window:appearancemode");
    const derived = jotai.atom((get) => {
        const tabOverride = get(tabOverrideAtom);
        const globalSetting = get(globalSettingAtom);
        const osPrefersDark = get(osPrefersDarkAtom);
        return resolveAppearanceMode(tabOverride, globalSetting, osPrefersDark);
    });
    appearanceModeAtomCache.set(tabId, derived);
    return derived;
}
