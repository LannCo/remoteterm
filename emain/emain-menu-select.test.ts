// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { pickCloseTarget, wantsHiddenNewWindowItems } from "./emain-menu-select";

describe("wantsHiddenNewWindowItems", () => {
    it("registers the hidden Cmd+N and Cmd+T items only when no main window exists and no builder is focused", () => {
        expect(wantsHiddenNewWindowItems(0, false)).toBe(true);
        expect(wantsHiddenNewWindowItems(0, true)).toBe(false);
        expect(wantsHiddenNewWindowItems(1, false)).toBe(false);
        expect(wantsHiddenNewWindowItems(3, true)).toBe(false);
    });
});

describe("pickCloseTarget", () => {
    type Win = { name: string };
    const popup: Win = { name: "popup" };
    const builder: Win = { name: "builder" };
    const other: Win = { name: "other" };
    const main: Win = { name: "main" };
    const isPopup = (w: Win) => w === popup;
    const isBuilder = (w: Win) => w === builder;

    it("closes the popup that was clicked", () => {
        expect(pickCloseTarget(popup, main, isPopup, isBuilder)).toBe(popup);
    });

    it("closes the builder window that was clicked, never the last-focused main window", () => {
        expect(pickCloseTarget(builder, main, isPopup, isBuilder)).toBe(builder);
        expect(pickCloseTarget(builder, null, isPopup, isBuilder)).toBe(builder);
    });

    it("closes the last-focused main window for anything else", () => {
        expect(pickCloseTarget(other, main, isPopup, isBuilder)).toBe(main);
        expect(pickCloseTarget(null, main, isPopup, isBuilder)).toBe(main);
        expect(pickCloseTarget(undefined, main, isPopup, isBuilder)).toBe(main);
    });

    it("closes nothing when there is nothing to close", () => {
        expect(pickCloseTarget(null, null, isPopup, isBuilder)).toBeNull();
    });
});
