// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { bringWindowToFront, findBuilderWindowForApp, openPathDetached, pickTerminalWindow } from "./emain-builder-select";

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

describe("openPathDetached", () => {
    it("returns at once while the opener is still running, then logs a late failure", async () => {
        vi.useFakeTimers();
        try {
            let finish: (err: string) => void;
            const openPath = vi.fn(() => new Promise<string>((resolve) => (finish = resolve)));
            const log = vi.fn();
            const result = openPathDetached(openPath, "/apps/x", log, 250);
            await vi.advanceTimersByTimeAsync(250);
            expect(await result).toBe("");
            expect(log).not.toHaveBeenCalled();
            finish("no handler");
            await vi.advanceTimersByTimeAsync(0);
            expect(log).toHaveBeenCalledWith("Failed to open /apps/x: no handler");
        } finally {
            vi.useRealTimers();
        }
    });

    it("logs nothing when the opener later succeeds", async () => {
        vi.useFakeTimers();
        try {
            let finish: (err: string) => void;
            const log = vi.fn();
            const result = openPathDetached(() => new Promise<string>((resolve) => (finish = resolve)), "/a", log, 100);
            await vi.advanceTimersByTimeAsync(100);
            expect(await result).toBe("");
            finish("");
            await vi.advanceTimersByTimeAsync(0);
            expect(log).not.toHaveBeenCalled();
        } finally {
            vi.useRealTimers();
        }
    });

    it("surfaces an error string that arrives within the grace period", async () => {
        const log = vi.fn();
        expect(await openPathDetached(async () => "Failed to open path", "/a", log)).toBe("Failed to open path");
        expect(log).not.toHaveBeenCalled();
    });

    it("surfaces a fast rejection and a synchronous throw", async () => {
        const log = vi.fn();
        expect(await openPathDetached(() => Promise.reject(new Error("boom")), "/a", log)).toBe(
            "Could not open the folder: boom"
        );
        expect(
            await openPathDetached(
                () => {
                    throw new Error("bad");
                },
                "/a",
                log
            )
        ).toBe("Could not open the folder: bad");
    });

    it("returns an empty string for a fast success", async () => {
        expect(await openPathDetached(async () => "", "/a", vi.fn())).toBe("");
    });
});
