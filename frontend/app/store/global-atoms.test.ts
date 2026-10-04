// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { getAtoms, initGlobalAtoms } from "./global-atoms";
import { globalStore } from "./jotaiStore";

describe("global-atoms", () => {
    it("throws before initialization", () => {
        expect(() => getAtoms()).toThrow("Global atoms accessed before initialization");
    });
});

describe("uiContext", () => {
    it("reads the static tab id when it is called, so a builder can set it after init", () => {
        vi.spyOn(console, "log").mockImplementation(() => {});
        initGlobalAtoms({ windowId: "win-1", builderId: "builder-1", platform: "linux", environment: "renderer" } as GlobalInitOptions);
        const atoms = getAtoms();
        expect(globalStore.get(atoms.uiContext).activetabid).toBeUndefined();
        globalStore.set(atoms.staticTabId, "tab-9");
        expect(globalStore.get(atoms.uiContext)).toEqual({ windowid: "win-1", activetabid: "tab-9" });
    });

    it("is unchanged for main windows", () => {
        initGlobalAtoms({ windowId: "win-2", tabId: "tab-2", platform: "linux", environment: "renderer" } as GlobalInitOptions);
        expect(globalStore.get(getAtoms().uiContext)).toEqual({ windowid: "win-2", activetabid: "tab-2" });
    });
});
