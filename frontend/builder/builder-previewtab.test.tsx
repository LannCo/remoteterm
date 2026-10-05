// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, render } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn(() => () => {}) }));
vi.mock("@/layout/index", () => ({ deleteLayoutModelForTab: vi.fn() }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app"), staticTabId: atom(null) },
        getApi: vi.fn(),
        getSettingsKeyAtom: vi.fn(() => atom(false)),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}` },
    };
});

import { globalStore } from "@/app/store/jotaiStore";
import { getApi } from "@/store/global";
import { BuilderAppPanelModel } from "@/builder/store/builder-apppanel-model";
import { BuilderPreviewTab } from "./tabs/builder-previewtab";

describe("BuilderPreviewTab webview", () => {
    afterEach(() => {
        cleanup();
        const model = BuilderAppPanelModel.getInstance();
        globalStore.set(model.resizeDraggingAtom, false);
        globalStore.set(model.builderStatusAtom, null);
        globalStore.set(model.previewAuthAtom, null);
    });

    it("ignores pointer events while a builder divider is being dragged, and restores them after", async () => {
        const model = BuilderAppPanelModel.getInstance();
        globalStore.set(model.builderStatusAtom, { status: "running", port: 5555 } as BuilderStatusData);
        globalStore.set(model.previewAuthAtom, { port: 5555, token: "tok" });
        const { container } = render(
            <Provider store={globalStore}>
                <BuilderPreviewTab />
            </Provider>
        );
        const webview = container.querySelector("webview") as HTMLElement;
        expect(webview.style.pointerEvents).toBe("auto");

        await act(async () => {
            globalStore.set(model.resizeDraggingAtom, true);
        });
        expect(webview.style.pointerEvents).toBe("none");

        await act(async () => {
            globalStore.set(model.resizeDraggingAtom, false);
        });
        expect(webview.style.pointerEvents).toBe("auto");
    });

    it("reports the preview webview focus to the main process so the builder keys are forwarded", async () => {
        const setWebviewFocus = vi.fn();
        vi.mocked(getApi).mockReturnValue({ setWebviewFocus } as unknown as ReturnType<typeof getApi>);
        const model = BuilderAppPanelModel.getInstance();
        globalStore.set(model.builderStatusAtom, { status: "running", port: 5555 } as BuilderStatusData);
        globalStore.set(model.previewAuthAtom, { port: 5555, token: "tok" });
        const { container } = render(
            <Provider store={globalStore}>
                <BuilderPreviewTab />
            </Provider>
        );
        const webview = container.querySelector("webview") as HTMLElement & { getWebContentsId: () => number };
        webview.getWebContentsId = () => 42;

        webview.dispatchEvent(new Event("focus"));
        expect(setWebviewFocus).not.toHaveBeenCalled();

        webview.dispatchEvent(new Event("dom-ready"));
        webview.dispatchEvent(new Event("focus"));
        expect(setWebviewFocus).toHaveBeenLastCalledWith(42);

        webview.dispatchEvent(new Event("blur"));
        expect(setWebviewFocus).toHaveBeenLastCalledWith(null);
    });
});
