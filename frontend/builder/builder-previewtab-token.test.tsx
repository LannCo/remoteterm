// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, render } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ GetBuilderPreviewAuthCommand: vi.fn() }));

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: rpc }));
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
import { BuilderAppPanelModel } from "@/builder/store/builder-apppanel-model";
import { BuilderPreviewTab } from "./tabs/builder-previewtab";

function renderTab() {
    return render(
        <Provider store={globalStore}>
            <BuilderPreviewTab />
        </Provider>
    );
}

describe("BuilderPreviewTab preview token", () => {
    const model = BuilderAppPanelModel.getInstance();

    beforeEach(() => {
        globalStore.set(model.originalContentAtom, "package main");
        globalStore.set(model.builderStatusAtom, { status: "running", port: 5555 } as BuilderStatusData);
    });

    afterEach(() => {
        cleanup();
        globalStore.set(model.originalContentAtom, "");
        globalStore.set(model.builderStatusAtom, null);
        globalStore.set(model.previewAuthAtom, null);
    });

    it("loads the preview with the run's token in the URL, once", () => {
        globalStore.set(model.previewAuthAtom, { port: 5555, token: "abc123" });
        const { container } = renderTab();
        const webview = container.querySelector("webview");
        expect(webview?.getAttribute("src")).toBe("http://localhost:5555/?clientid=wave:builder-1&tsunamitoken=abc123");
    });

    it("does not load the preview before the token has arrived", () => {
        const { container } = renderTab();
        expect(container.querySelector("webview")).toBeNull();
    });

    it("does not use a token that was fetched for another port", () => {
        globalStore.set(model.previewAuthAtom, { port: 4444, token: "old-run" });
        const { container } = renderTab();
        expect(container.querySelector("webview")).toBeNull();
    });

    it("shows the preview once the token arrives", async () => {
        const { container } = renderTab();
        expect(container.querySelector("webview")).toBeNull();
        await act(async () => {
            globalStore.set(model.previewAuthAtom, { port: 5555, token: "late" });
        });
        expect(container.querySelector("webview")?.getAttribute("src")).toContain("tsunamitoken=late");
    });
});

describe("BuilderAppPanelModel preview auth", () => {
    const model = BuilderAppPanelModel.getInstance();

    beforeEach(() => {
        vi.clearAllMocks();
        vi.spyOn(console, "error").mockImplementation(() => {});
        globalStore.set(model.previewAuthAtom, null);
    });

    it("fetches the token for a running app, and only then", async () => {
        rpc.GetBuilderPreviewAuthCommand.mockResolvedValue({ port: 5555, token: "tok" });
        await model.syncPreviewAuth({ status: "building" } as BuilderStatusData);
        await model.syncPreviewAuth({ status: "running" } as BuilderStatusData);
        expect(rpc.GetBuilderPreviewAuthCommand).not.toHaveBeenCalled();

        await model.syncPreviewAuth({ status: "running", port: 5555 } as BuilderStatusData);
        expect(rpc.GetBuilderPreviewAuthCommand).toHaveBeenCalledWith(expect.anything(), { builderid: "builder-1" });
        expect(globalStore.get(model.previewAuthAtom)).toEqual({ port: 5555, token: "tok" });
    });

    it("forgets the token when the app stops or rebuilds", async () => {
        globalStore.set(model.previewAuthAtom, { port: 5555, token: "tok" });
        await model.syncPreviewAuth({ status: "building" } as BuilderStatusData);
        expect(globalStore.get(model.previewAuthAtom)).toBeNull();
    });

    it("drops a reply that lands after a newer status", async () => {
        let releaseOld: (v: unknown) => void;
        rpc.GetBuilderPreviewAuthCommand.mockReturnValueOnce(new Promise((resolve) => (releaseOld = resolve)));
        const older = model.syncPreviewAuth({ status: "running", port: 5555 } as BuilderStatusData);
        await model.syncPreviewAuth({ status: "stopped" } as BuilderStatusData);
        releaseOld({ port: 5555, token: "stale" });
        await older;
        expect(globalStore.get(model.previewAuthAtom)).toBeNull();
    });

    it("leaves no token when the fetch fails or the server has none", async () => {
        rpc.GetBuilderPreviewAuthCommand.mockRejectedValueOnce(new Error("refused"));
        await model.syncPreviewAuth({ status: "running", port: 5555 } as BuilderStatusData);
        expect(globalStore.get(model.previewAuthAtom)).toBeNull();
        rpc.GetBuilderPreviewAuthCommand.mockResolvedValueOnce({ port: 0, token: "" });
        await model.syncPreviewAuth({ status: "running", port: 5555 } as BuilderStatusData);
        expect(globalStore.get(model.previewAuthAtom)).toBeNull();
    });
});
