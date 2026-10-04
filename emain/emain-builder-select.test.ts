// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { bringWindowToFront, findBuilderWindowForApp, pickTerminalWindow } from "./emain-builder-select";

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

describe("bringWindowToFront", () => {
    function makeRevealable(minimized: boolean, visible: boolean) {
        return {
            isMinimized: () => minimized,
            restore: vi.fn(),
            isVisible: () => visible,
            show: vi.fn(),
            focus: vi.fn(),
        };
    }

    it("restores a minimised window before focusing it", () => {
        const win = makeRevealable(true, true);
        bringWindowToFront(win);
        expect(win.restore).toHaveBeenCalledTimes(1);
        expect(win.focus).toHaveBeenCalledTimes(1);
        expect(win.restore.mock.invocationCallOrder[0]).toBeLessThan(win.focus.mock.invocationCallOrder[0]);
    });

    it("shows a hidden window and leaves a normal one alone", () => {
        const hidden = makeRevealable(false, false);
        bringWindowToFront(hidden);
        expect(hidden.show).toHaveBeenCalledTimes(1);
        expect(hidden.restore).not.toHaveBeenCalled();
        const normal = makeRevealable(false, true);
        bringWindowToFront(normal);
        expect(normal.show).not.toHaveBeenCalled();
        expect(normal.restore).not.toHaveBeenCalled();
        expect(normal.focus).toHaveBeenCalledTimes(1);
    });
});
