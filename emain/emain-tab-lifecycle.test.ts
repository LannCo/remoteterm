// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
    BaseReloadDelayMs,
    handleTabDidFailLoad,
    handleTabLoadSucceeded,
    handleTabRenderProcessGone,
    handleTabUnresponsive,
    MaxConsecutiveReloads,
} from "./emain-tab-lifecycle";

function makeTabView(isDestroyed = false) {
    return {
        remoteTermTabId: "tab-123",
        isDestroyed,
        webContents: { reload: vi.fn() },
    };
}

function loggedLines(): string[] {
    return vi.mocked(console.log).mock.calls.map((c) => String(c[0]));
}

beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(console, "log").mockImplementation(() => {});
});

afterEach(() => {
    vi.useRealTimers();
});

describe("handleTabRenderProcessGone", () => {
    it("reloads the tab after the base delay and logs the reason", () => {
        const tabView = makeTabView();
        handleTabRenderProcessGone(tabView, { reason: "oom" } as any);
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
        vi.advanceTimersByTime(BaseReloadDelayMs);
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(1);
        expect(
            loggedLines().some((l) => l.includes("render-process-gone") && l.includes("oom") && l.includes("tab-123"))
        ).toBe(true);
    });

    it("does not reload a tab that is already destroyed", () => {
        const tabView = makeTabView(true);
        handleTabRenderProcessGone(tabView, { reason: "crashed" } as any);
        vi.runAllTimers();
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });

    it("does not reload a tab destroyed while the reload was pending", () => {
        const tabView = makeTabView();
        handleTabRenderProcessGone(tabView, { reason: "crashed" } as any);
        tabView.isDestroyed = true;
        vi.runAllTimers();
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });
});

describe("handleTabUnresponsive", () => {
    it("logs but never reloads", () => {
        const tabView = makeTabView();
        handleTabUnresponsive(tabView);
        vi.runAllTimers();
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
        expect(loggedLines().some((l) => l.includes("unresponsive") && l.includes("tab-123"))).toBe(true);
    });
});

describe("handleTabDidFailLoad", () => {
    it("ignores an aborted main-frame load (errorCode -3)", () => {
        const tabView = makeTabView();
        handleTabDidFailLoad(tabView, -3, "ERR_ABORTED", true);
        vi.runAllTimers();
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });

    it("ignores a sub-frame load failure", () => {
        const tabView = makeTabView();
        handleTabDidFailLoad(tabView, -105, "ERR_NAME_NOT_RESOLVED", false);
        vi.runAllTimers();
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });

    it("reloads on a real main-frame load failure", () => {
        const tabView = makeTabView();
        handleTabDidFailLoad(tabView, -105, "ERR_NAME_NOT_RESOLVED", true);
        vi.advanceTimersByTime(BaseReloadDelayMs);
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(1);
        expect(
            loggedLines().some((l) => l.includes("did-fail-load") && l.includes("-105") && l.includes("tab-123"))
        ).toBe(true);
    });

    it("does not reload an already-destroyed tab on a real load failure", () => {
        const tabView = makeTabView(true);
        handleTabDidFailLoad(tabView, -105, "ERR_NAME_NOT_RESOLVED", true);
        vi.runAllTimers();
        expect(tabView.webContents.reload).not.toHaveBeenCalled();
    });
});

describe("reload budget", () => {
    function failAndFire(tabView: ReturnType<typeof makeTabView>, attempt: number) {
        handleTabDidFailLoad(tabView, -102, "ERR_CONNECTION_REFUSED", true);
        const delay = BaseReloadDelayMs * 2 ** attempt;
        vi.advanceTimersByTime(delay - 1);
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(attempt);
        vi.advanceTimersByTime(1);
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(attempt + 1);
    }

    it("backs off exponentially and gives up after the maximum consecutive reloads", () => {
        const tabView = makeTabView();
        for (let attempt = 0; attempt < MaxConsecutiveReloads; attempt++) {
            failAndFire(tabView, attempt);
        }
        handleTabDidFailLoad(tabView, -102, "ERR_CONNECTION_REFUSED", true);
        vi.runAllTimers();
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(MaxConsecutiveReloads);
        expect(loggedLines().some((l) => l.includes("reload-giveup") && l.includes("tab-123"))).toBe(true);
    });

    it("resets the budget after a successful load", () => {
        const tabView = makeTabView();
        for (let attempt = 0; attempt < MaxConsecutiveReloads; attempt++) {
            failAndFire(tabView, attempt);
        }
        handleTabLoadSucceeded(tabView);
        handleTabRenderProcessGone(tabView, { reason: "crashed" } as any);
        vi.advanceTimersByTime(BaseReloadDelayMs);
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(MaxConsecutiveReloads + 1);
    });

    it("coalesces a crash and a failed load that arrive while a reload is pending", () => {
        const tabView = makeTabView();
        handleTabRenderProcessGone(tabView, { reason: "crashed" } as any);
        handleTabDidFailLoad(tabView, -102, "ERR_CONNECTION_REFUSED", true);
        vi.runAllTimers();
        expect(tabView.webContents.reload).toHaveBeenCalledTimes(1);
    });

    it("keeps budgets independent per tab", () => {
        const tabA = makeTabView();
        const tabB = makeTabView();
        for (let attempt = 0; attempt < MaxConsecutiveReloads; attempt++) {
            failAndFire(tabA, attempt);
        }
        handleTabDidFailLoad(tabB, -102, "ERR_CONNECTION_REFUSED", true);
        vi.advanceTimersByTime(BaseReloadDelayMs);
        expect(tabB.webContents.reload).toHaveBeenCalledTimes(1);
    });
});
