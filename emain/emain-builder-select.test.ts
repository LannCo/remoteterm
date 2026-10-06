// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { findBuilderWindowForApp, openPathDetached, runBuilderTeardown } from "./emain-builder-select";

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

    it("skips a window that is tearing down", () => {
        const closing = [...windows, { builderId: "b4", builderAppId: "draft/one", tearingDown: true }];
        expect(findBuilderWindowForApp(closing, "draft/one", "b2")).toBe(windows[0]);
        expect(findBuilderWindowForApp([closing[3]], "draft/one", "b2")).toBeNull();
    });

    it("never matches an empty app id", () => {
        expect(findBuilderWindowForApp(windows, "", "b1")).toBeNull();
        expect(findBuilderWindowForApp(windows, null, "b1")).toBeNull();
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

function makeTeardownWindow() {
    let destroyed = false;
    return {
        tearingDown: false,
        hide: vi.fn(),
        isDestroyed: () => destroyed,
        destroy: vi.fn(() => {
            destroyed = true;
        }),
    };
}

describe("runBuilderTeardown", () => {
    it("hides the window, deletes the builder, then its rtinfo, then destroys the window", async () => {
        const order: string[] = [];
        const win = makeTeardownWindow();
        win.hide.mockImplementation(() => order.push("hide"));
        await runBuilderTeardown(win, {
            deleteBuilder: async () => {
                order.push("delete");
            },
            deleteRtInfo: async () => {
                order.push("rtinfo");
            },
            destroyWindow: () => {
                order.push("destroy");
                win.destroy();
            },
            logError: vi.fn(),
        });
        expect(order).toEqual(["hide", "delete", "rtinfo", "destroy"]);
    });

    it("waits for the delete before going on", async () => {
        const order: string[] = [];
        let finish: () => void;
        const win = makeTeardownWindow();
        const done = runBuilderTeardown(win, {
            deleteBuilder: () =>
                new Promise<void>((resolve) => {
                    finish = () => {
                        order.push("delete");
                        resolve();
                    };
                }),
            deleteRtInfo: async () => {
                order.push("rtinfo");
            },
            destroyWindow: () => order.push("destroy"),
            logError: vi.fn(),
        });
        await Promise.resolve();
        expect(order).toEqual([]);
        finish();
        await done;
        expect(order).toEqual(["delete", "rtinfo", "destroy"]);
    });

    it("still destroys the window when both RPCs fail", async () => {
        const order: string[] = [];
        const logError = vi.fn();
        await runBuilderTeardown(makeTeardownWindow(), {
            deleteBuilder: async () => {
                order.push("delete");
                throw new Error("server gone");
            },
            deleteRtInfo: async () => {
                order.push("rtinfo");
                throw new Error("server gone");
            },
            destroyWindow: () => order.push("destroy"),
            logError,
        });
        expect(order).toEqual(["delete", "rtinfo", "destroy"]);
        expect(logError).toHaveBeenCalledTimes(2);
    });

    it("runs the teardown and destroys the window once when asked twice at the same time", async () => {
        const win = makeTeardownWindow();
        const deleteBuilder = vi.fn(() => new Promise<void>((resolve) => setTimeout(resolve, 10)));
        const steps = {
            deleteBuilder,
            deleteRtInfo: vi.fn(async () => {}),
            destroyWindow: () => win.destroy(),
            logError: vi.fn(),
        };
        await Promise.all([runBuilderTeardown(win, steps), runBuilderTeardown(win, steps)]);
        expect(deleteBuilder).toHaveBeenCalledTimes(1);
        expect(steps.deleteRtInfo).toHaveBeenCalledTimes(1);
        expect(win.hide).toHaveBeenCalledTimes(1);
        expect(win.destroy).toHaveBeenCalledTimes(1);
    });

    it("leaves a window alone that is already destroyed, or destroyed during the teardown", async () => {
        const gone = makeTeardownWindow();
        gone.destroy();
        const steps = {
            deleteBuilder: vi.fn(async () => {}),
            deleteRtInfo: vi.fn(async () => {}),
            destroyWindow: vi.fn(),
            logError: vi.fn(),
        };
        await runBuilderTeardown(gone, steps);
        expect(steps.deleteBuilder).not.toHaveBeenCalled();
        expect(gone.hide).not.toHaveBeenCalled();

        const closing = makeTeardownWindow();
        await runBuilderTeardown(closing, { ...steps, deleteBuilder: async () => closing.destroy() });
        expect(steps.destroyWindow).not.toHaveBeenCalled();
    });
});
