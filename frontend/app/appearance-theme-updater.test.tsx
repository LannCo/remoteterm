// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, render } from "@testing-library/react";
import { createStore, PrimitiveAtom, Provider } from "jotai";
import { beforeEach, describe, expect, test, vi } from "vitest";

vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return { atoms: { staticTabId: atom("test-tab-1") } };
});

vi.mock("@/app/store/appearance-atoms", async () => {
    const { atom } = await import("jotai");
    const resolvedModeAtom = atom("light");
    return { getResolvedAppearanceModeAtom: () => resolvedModeAtom };
});

import { getResolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { AppThemeUpdater } from "./appearance-theme-updater";

const resolvedModeAtom = getResolvedAppearanceModeAtom("test-tab-1") as PrimitiveAtom<"light" | "dark">;

describe("AppThemeUpdater", () => {
    beforeEach(() => {
        document.documentElement.removeAttribute("data-theme");
    });

    test("sets data-theme to dark when the resolved mode is dark", () => {
        const store = createStore();
        store.set(resolvedModeAtom, "dark");
        render(
            <Provider store={store}>
                <AppThemeUpdater />
            </Provider>
        );
        expect(document.documentElement.dataset.theme).toBe("dark");
    });

    test("updates data-theme live when the resolved mode changes", () => {
        const store = createStore();
        store.set(resolvedModeAtom, "light");
        render(
            <Provider store={store}>
                <AppThemeUpdater />
            </Provider>
        );
        expect(document.documentElement.dataset.theme).toBe("light");

        act(() => {
            store.set(resolvedModeAtom, "dark");
        });
        expect(document.documentElement.dataset.theme).toBe("dark");
    });
});
