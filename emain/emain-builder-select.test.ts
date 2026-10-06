// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { findBuilderWindowForApp, runBuilderTeardown } from "./emain-builder-select";

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
