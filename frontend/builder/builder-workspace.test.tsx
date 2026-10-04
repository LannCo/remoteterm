// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/store/wshclientapi", () => ({
    RpcApi: { GetRTInfoCommand: vi.fn(async () => ({})), SetRTInfoCommand: vi.fn(async () => {}) },
}));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return { atoms: { builderId: atom("builder-1") } };
});
const handleProps: { className?: string; onDragging?: (isDragging: boolean) => void }[] = [];
vi.mock("react-resizable-panels", async () => {
    const actual = await vi.importActual<typeof import("react-resizable-panels")>("react-resizable-panels");
    return {
        ...actual,
        PanelResizeHandle: (props: (typeof handleProps)[number]) => {
            handleProps.push(props);
            return null;
        },
    };
});
vi.mock("@/builder/builder-apppanel", async () => {
    const { createElement } = await import("react");
    return { BuilderAppPanel: () => createElement("div", null, "app panel") };
});
vi.mock("@/builder/builder-buildpanel", async () => {
    const { createElement } = await import("react");
    return { BuilderBuildPanel: () => createElement("div", null, "build output") };
});
vi.mock("@/builder/builder-termpanel", async () => {
    const { createElement } = await import("react");
    return { BuilderTermPanel: () => createElement("div", null, "terminals") };
});

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderAppPanelModel } from "@/builder/store/builder-apppanel-model";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { BuilderWorkspace } from "./builder-workspace";

describe("BuilderWorkspace", () => {
    afterEach(() => {
        cleanup();
    });

    it("moves builder focus to the app side when the build panel is clicked", async () => {
        BuilderFocusManager.getInstance().setTerminalFocused();
        render(
            <Provider store={globalStore}>
                <BuilderWorkspace />
            </Provider>
        );
        fireEvent.mouseDown(await screen.findByText("build output"));
        expect(BuilderFocusManager.getInstance().getFocusType()).toBe("app");
        expect(document.querySelector("[data-builder-focus]").getAttribute("data-builder-focus")).toBe("app");
    });

    it("flags a divider drag on the shared model for both builder dividers", async () => {
        handleProps.length = 0;
        const model = BuilderAppPanelModel.getInstance();
        render(
            <Provider store={globalStore}>
                <BuilderWorkspace />
            </Provider>
        );
        await screen.findByText("build output");
        const horizontal = handleProps.find((p) => p.className.includes("w-0.5"));
        const vertical = handleProps.find((p) => p.className.includes("h-0.5"));

        for (const handle of [horizontal, vertical]) {
            expect(globalStore.get(model.resizeDraggingAtom)).toBe(false);
            act(() => handle.onDragging(true));
            expect(globalStore.get(model.resizeDraggingAtom)).toBe(true);
            act(() => handle.onDragging(false));
            expect(globalStore.get(model.resizeDraggingAtom)).toBe(false);
        }
    });
});
