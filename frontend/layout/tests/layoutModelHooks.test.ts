// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ getWaveObjectAtom: vi.fn() }));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    const { globalStore } = await import("@/app/store/jotaiStore");
    return {
        atoms: { staticTabId: atom(null) },
        globalStore,
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}`, getWaveObjectAtom: h.getWaveObjectAtom },
    };
});
vi.mock("@/app/hook/useDimensions", () => ({ useOnResize: vi.fn() }));
vi.mock("../lib/layoutModel", () => ({ LayoutModel: vi.fn() }));

import { getLayoutModelForStaticTab } from "../lib/layoutModelHooks";

describe("getLayoutModelForStaticTab", () => {
    it("returns null before a static tab is set, without creating a tab:null object", () => {
        expect(getLayoutModelForStaticTab()).toBeNull();
        expect(h.getWaveObjectAtom).not.toHaveBeenCalled();
    });
});
