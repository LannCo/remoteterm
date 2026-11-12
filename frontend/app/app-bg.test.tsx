// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, render } from "@testing-library/react";
import { createStore, PrimitiveAtom, Provider } from "jotai";
import { beforeEach, describe, expect, test, vi } from "vitest";

const { updateWindowControlsOverlay } = vi.hoisted(() => ({ updateWindowControlsOverlay: vi.fn() }));

vi.mock("@/app/store/appearance-atoms", async () => {
    const { atom } = await import("jotai");
    return { resolvedAppearanceModeAtom: atom("dark") };
});
vi.mock("@/util/platformutil", () => ({ PLATFORM: "linux", PlatformMacOS: "darwin" }));
vi.mock("@/util/remotetermutil", () => ({ computeBgStyleFromMeta: () => ({}) }));
vi.mock("@react-hook/resize-observer", () => ({ default: () => {} }));
vi.mock("throttle-debounce", () => ({ debounce: (_ms: number, fn: () => void) => fn }));
vi.mock("@/app/remotetermenv/remotetermenv", async () => {
    const { atom } = await import("jotai");
    const nullAtom = atom(null);
    return {
        useWaveEnv: () => ({
            getTabMetaKeyAtom: () => nullAtom,
            getConfigBackgroundAtom: () => nullAtom,
        }),
    };
});
vi.mock("./store/global", async () => {
    const { atom } = await import("jotai");
    return {
        atoms: { staticTabId: atom(null) },
        getApi: () => ({ updateWindowControlsOverlay }),
        WOS: { makeORef: (t: string, id: string) => `${t}:${id}` },
    };
});
vi.mock("./store/wos", () => ({ useWaveObjectValue: () => [{ meta: {} }] }));

import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { AppBackground } from "./app-bg";

const modeAtom = resolvedAppearanceModeAtom as PrimitiveAtom<"light" | "dark">;

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
