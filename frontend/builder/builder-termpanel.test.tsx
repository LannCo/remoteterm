// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { atom, Provider } from "jotai";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
    layoutModel: null as any,
    tab: null as any,
    open: vi.fn(() => Promise.resolve("")),
}));

vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return {
        atoms: { builderAppId: atom("draft/app"), staticTabId: atom(null), settingsAtom: atom({}) },
        getApi: () => ({ getCursorPoint: vi.fn() }),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}`, getWaveObjectAtom: () => atom(h.tab) },
    };
});
vi.mock("@/layout/index", () => ({
    deleteLayoutModelForTab: vi.fn(),
    getLayoutModelForStaticTab: () => h.layoutModel,
    TileLayout: () => null,
}));
vi.mock("@/app/store/tab-model", async () => {
    const { createContext } = await import("react");
    return { getTabModelByTabId: vi.fn(), TabModelContext: createContext(undefined) };
});
vi.mock("@/builder/builder-termcontents", () => ({ makeBuilderTileContents: vi.fn(() => ({})) }));
vi.mock("@/app/store/builder-terminal", () => ({ openBuilderTerminal: h.open }));

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import {
    BuilderTermMismatchMessage,
    BuilderTermModel,
    type BuilderTermState,
} from "@/builder/store/builder-term-model";
import { BuilderTermPanel } from "./builder-termpanel";

function renderPanel(state: BuilderTermState, extra?: () => void) {
    const model = BuilderTermModel.getInstance();
    vi.spyOn(model, "bootstrap").mockResolvedValue(undefined);
    globalStore.set(model.stateAtom, state);
    extra?.();
    render(
        <Provider store={globalStore}>
            <BuilderTermPanel />
        </Provider>
    );
    return model;
}

describe("BuilderTermPanel", () => {
    beforeEach(() => {
        BuilderTermModel.resetInstance();
        h.layoutModel = null;
        h.tab = null;
        h.open.mockClear();
        BuilderFocusManager.getInstance().setAppFocused();
    });

    afterEach(() => {
        cleanup();
        vi.restoreAllMocks();
    });

    it("shows a neutral state while the app is being switched", () => {
        renderPanel("switching");
        expect(screen.getByText("Switching app…")).toBeTruthy();
        expect(screen.queryByText("Retry")).toBeNull();
    });

    it("shows the mismatch message without mounting or retrying", () => {
        renderPanel("mismatch");
        expect(screen.getByText(BuilderTermMismatchMessage)).toBeTruthy();
        expect(screen.queryByText("Retry")).toBeNull();
    });

    it("shows an Ensure error with a Retry button", () => {
        const model = renderPanel("error", () =>
            globalStore.set(BuilderTermModel.getInstance().errorAtom, "Could not start the terminals: boom")
        );
        const retry = vi.spyOn(model, "retry").mockImplementation(() => {});
        expect(screen.getByText("Could not start the terminals: boom")).toBeTruthy();
        fireEvent.click(screen.getByText("Retry"));
        expect(retry).toHaveBeenCalledTimes(1);
    });

    it("shows the empty state over the layout when the tab has no panes, and opens a terminal from it", () => {
        h.layoutModel = { numLeafs: atom(0) };
        renderPanel("ready", () => globalStore.set(BuilderTermModel.getInstance().tabIdAtom, "tab-1"));
        expect(screen.getByText("No terminals")).toBeTruthy();
        fireEvent.mouseDown(screen.getByText("Open terminal"));
        fireEvent.click(screen.getByText("Open terminal"));
        expect(h.open).toHaveBeenCalledWith("", null);
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("app");
    });

    it("shows no empty state and reports no pane count while the tab has blocks the layout has not mounted yet", () => {
        h.layoutModel = { numLeafs: atom(0) };
        h.tab = { otype: "tab", oid: "tab-1", blockids: ["b1"] };
        const model = BuilderTermModel.getInstance();
        const handlePaneCount = vi.spyOn(model, "handlePaneCount");
        renderPanel("ready", () => globalStore.set(model.tabIdAtom, "tab-1"));
        expect(screen.queryByText("No terminals")).toBeNull();
        expect(handlePaneCount).not.toHaveBeenCalled();
    });

    it("shows the empty state and reports zero panes when the tab has no blocks and no leafs", () => {
        h.layoutModel = { numLeafs: atom(0) };
        h.tab = { otype: "tab", oid: "tab-1", blockids: [] };
        const model = BuilderTermModel.getInstance();
        const handlePaneCount = vi.spyOn(model, "handlePaneCount");
        renderPanel("ready", () => globalStore.set(model.tabIdAtom, "tab-1"));
        expect(screen.getByText("No terminals")).toBeTruthy();
        expect(handlePaneCount).toHaveBeenCalledWith(0);
    });

    it("hides the empty state and takes terminal focus once a pane exists", async () => {
        const leafs = atom(0);
        h.layoutModel = { numLeafs: leafs };
        renderPanel("ready", () => globalStore.set(BuilderTermModel.getInstance().tabIdAtom, "tab-1"));
        await act(async () => {
            globalStore.set(leafs, 1);
        });
        expect(screen.queryByText("No terminals")).toBeNull();
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("terminal");
    });

    it("returns builder focus to the app when the layout unmounts with panes still counted", async () => {
        h.layoutModel = { numLeafs: atom(2) };
        const model = renderPanel("ready", () => globalStore.set(BuilderTermModel.getInstance().tabIdAtom, "tab-1"));
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("terminal");
        expect(model.hasPanes()).toBe(true);
        await act(async () => {
            model.setState("vanished");
        });
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("app");
        expect(model.hasPanes()).toBe(false);
    });
});
