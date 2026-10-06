// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, render, waitFor } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn(() => () => {}) }));
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
import { BuilderConfigDataTab } from "./tabs/builder-configdatatab";

function okJson(body: unknown) {
    return { ok: true, statusText: "OK", json: async () => body };
}

describe("BuilderConfigDataTab with the preview token", () => {
    const model = BuilderAppPanelModel.getInstance();
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        fetchMock.mockImplementation(async (url: string) => okJson(url.endsWith("/api/config") ? { a: 1 } : { b: 2 }));
        vi.stubGlobal("fetch", fetchMock);
        globalStore.set(model.activeTab, "configdata");
        globalStore.set(model.builderStatusAtom, { status: "running", port: 5555 } as BuilderStatusData);
    });

    afterEach(() => {
        cleanup();
        vi.unstubAllGlobals();
        globalStore.set(model.activeTab, "preview");
        globalStore.set(model.builderStatusAtom, null);
        globalStore.set(model.previewAuthAtom, null);
    });

    function renderTab() {
        return render(
            <Provider store={globalStore}>
                <BuilderConfigDataTab />
            </Provider>
        );
    }

    it("sends the token as a bearer header on both requests", async () => {
        globalStore.set(model.previewAuthAtom, { port: 5555, token: "tok" });
        renderTab();
        await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
        const calls = fetchMock.mock.calls.map(([url, init]) => [url, init?.headers?.Authorization]);
        expect(calls).toContainEqual(["http://localhost:5555/api/config", "Bearer tok"]);
        expect(calls).toContainEqual(["http://localhost:5555/api/data", "Bearer tok"]);
    });

    it("does not call the app before the token has arrived, and does once it does", async () => {
        const { container } = renderTab();
        await act(async () => {});
        expect(fetchMock).not.toHaveBeenCalled();
        expect(container.textContent).not.toContain("App Not Running");

        await act(async () => {
            globalStore.set(model.previewAuthAtom, { port: 5555, token: "late" });
        });
        await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
        expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe("Bearer late");
    });

    it("ignores a token fetched for another port", async () => {
        globalStore.set(model.previewAuthAtom, { port: 4444, token: "old-run" });
        renderTab();
        await act(async () => {});
        expect(fetchMock).not.toHaveBeenCalled();
    });
});
