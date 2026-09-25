// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, render } from "@testing-library/react";
import { createStore, PrimitiveAtom, Provider } from "jotai";
import { beforeEach, describe, expect, test, vi } from "vitest";

const { staticTabIdAtom, resolvedModeAtom } = vi.hoisted(() => {
    const jotai = require("jotai");
    return {
        staticTabIdAtom: jotai.atom("test-tab-1"),
        resolvedModeAtom: jotai.atom("light") as PrimitiveAtom<"light" | "dark">,
    };
});

vi.mock("@/store/global", () => ({
    atoms: { staticTabId: staticTabIdAtom },
}));

vi.mock("@/app/store/appearance-atoms", () => ({
    getResolvedAppearanceModeAtom: () => resolvedModeAtom,
}));

import { AppThemeUpdater } from "./appearance-theme-updater";

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
