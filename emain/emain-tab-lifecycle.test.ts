// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";
import { handleTabDidFailLoad, handleTabRenderProcessGone, handleTabUnresponsive } from "./emain-tab-lifecycle";

function makeTabView(isDestroyed = false) {
    return {
        remoteTermTabId: "tab-123",
        isDestroyed,
        webContents: { reload: vi.fn() },
    };
}

beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => {});
});

describe("handleTabRenderProcessGone", () => {
    it("reloads the tab and logs the reason", () => {
        const tabView = makeTabView();
        handleTabRenderProcessGone(tabView, { reason: "oom" } as any);
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(1);
        const logged = vi.mocked(console.log).mock.calls.map((c) => String(c[0]));
        expect(logged.some((l) => l.includes("render-process-gone") && l.includes("oom") && l.includes("tab-123"))).toBe(
            true
        );
    });

    it("does not reload a tab that is already destroyed", () => {
        const tabView = makeTabView(true);
        handleTabRenderProcessGone(tabView, { reason: "crashed" } as any);
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });
});

describe("handleTabUnresponsive", () => {
    it("logs but never reloads", () => {
        const tabView = makeTabView();
        handleTabUnresponsive(tabView);
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
        const logged = vi.mocked(console.log).mock.calls.map((c) => String(c[0]));
        expect(logged.some((l) => l.includes("unresponsive") && l.includes("tab-123"))).toBe(true);
    });
});

describe("handleTabDidFailLoad", () => {
    it("ignores an aborted main-frame load (errorCode -3)", () => {
        const tabView = makeTabView();
        handleTabDidFailLoad(tabView, -3, "ERR_ABORTED", true);
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });

    it("ignores a sub-frame load failure", () => {
        const tabView = makeTabView();
        handleTabDidFailLoad(tabView, -105, "ERR_NAME_NOT_RESOLVED", false);
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });

    it("reloads on a real main-frame load failure", () => {
        const tabView = makeTabView();
        handleTabDidFailLoad(tabView, -105, "ERR_NAME_NOT_RESOLVED", true);
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(1);
        const logged = vi.mocked(console.log).mock.calls.map((c) => String(c[0]));
        expect(logged.some((l) => l.includes("did-fail-load") && l.includes("-105") && l.includes("tab-123"))).toBe(
            true
        );
    });

    it("does not reload an already-destroyed tab on a real load failure", () => {
        const tabView = makeTabView(true);
        handleTabDidFailLoad(tabView, -105, "ERR_NAME_NOT_RESOLVED", true);
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });
});
