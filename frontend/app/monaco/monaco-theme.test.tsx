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
