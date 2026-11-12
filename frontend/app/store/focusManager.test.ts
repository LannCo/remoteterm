// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom } from "jotai";
import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ layoutModel: null as any }));

vi.mock("@/app/store/global", async () => {
    const { atom } = await import("jotai");
    return { atoms: { staticTabId: atom(null) }, getBlockComponentModel: vi.fn() };
});
vi.mock("@/layout/index", () => ({ getLayoutModelForStaticTab: () => h.layoutModel }));

import { atoms } from "@/app/store/global";
import { FocusManager } from "./focusManager";
import { globalStore } from "./jotaiStore";

describe("FocusManager.blockFocusAtom", () => {
    it("recomputes once a builder window sets its static tab", () => {
        const focusAtom = FocusManager.getInstance().blockFocusAtom;
        expect(globalStore.get(focusAtom)).toBeNull();
        h.layoutModel = { focusedNode: atom({ id: "node-b1", data: { blockId: "b1" } }) };
        globalStore.set(atoms.staticTabId, "tab-1");
        expect(globalStore.get(focusAtom)).toBe("b1");
    });
});
