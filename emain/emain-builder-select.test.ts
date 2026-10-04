// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { findBuilderWindowForApp, pickTerminalWindow } from "./emain-builder-select";

function makeWin(id: string, destroyed = false) {
    return { id, isDestroyed: () => destroyed };
}

describe("pickTerminalWindow", () => {
    it("prefers the last focused window", () => {
        const a = makeWin("a");
        const b = makeWin("b");
        expect(pickTerminalWindow(b, [a, b])).toBe(b);
    });

    it("falls back to the first live window when the last focused one is gone", () => {
        const gone = makeWin("gone", true);
        const live = makeWin("live");
        expect(pickTerminalWindow(gone, [gone, live])).toBe(live);
        expect(pickTerminalWindow(null, [live])).toBe(live);
    });

    it("returns null when no window is open", () => {
        expect(pickTerminalWindow(null, [])).toBeNull();
        expect(pickTerminalWindow(null, [makeWin("x", true)])).toBeNull();
    });
});

describe("findBuilderWindowForApp", () => {
    const windows = [
        { builderId: "b1", builderAppId: "draft/one" },
        { builderId: "b2", builderAppId: "draft/two" },
        { builderId: "b3", builderAppId: "" },
    ];

    it("finds another window that already has the app", () => {
        expect(findBuilderWindowForApp(windows, "draft/two", "b1")).toBe(windows[1]);
    });

    it("ignores the asking window itself", () => {
        expect(findBuilderWindowForApp(windows, "draft/one", "b1")).toBeNull();
    });

    it("never matches an empty app id", () => {
        expect(findBuilderWindowForApp(windows, "", "b1")).toBeNull();
        expect(findBuilderWindowForApp(windows, null, "b1")).toBeNull();
    });
});
