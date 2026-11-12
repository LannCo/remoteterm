// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ RequestBuilderRebuildCommand: vi.fn() }));

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

describe("BuilderPreviewTab when the auto-run was declined", () => {
    const model = BuilderAppPanelModel.getInstance();

    beforeEach(() => {
        vi.clearAllMocks();
        rpc.RequestBuilderRebuildCommand.mockResolvedValue(undefined);
        globalStore.set(model.originalContentAtom, "package main");
        globalStore.set(model.builderStatusAtom, { status: "init" } as BuilderStatusData);
        globalStore.set(model.autoRunDeclinedAtom, true);
    });

    afterEach(() => {
        cleanup();
        globalStore.set(model.originalContentAtom, "");
        globalStore.set(model.builderStatusAtom, null);
        globalStore.set(model.autoRunDeclinedAtom, false);
    });

    it("offers a Start button instead of the empty state, and starts only when it is pressed", async () => {
        const { getByText, queryByText } = renderTab();
        expect(queryByText("No App to Preview")).toBeNull();
        expect(getByText("Review before starting")).toBeTruthy();
        expect(rpc.RequestBuilderRebuildCommand).not.toHaveBeenCalled();

        await act(async () => {
            fireEvent.click(getByText("Start App"));
        });
        expect(rpc.RequestBuilderRebuildCommand).toHaveBeenCalledTimes(1);
        expect(rpc.RequestBuilderRebuildCommand).toHaveBeenCalledWith(expect.anything(), { builderid: "builder-1" });
        expect(globalStore.get(model.autoRunDeclinedAtom)).toBe(false);
    });

    it("does not show the prompt once the app is building", () => {
        globalStore.set(model.builderStatusAtom, { status: "building" } as BuilderStatusData);
        const { queryByText } = renderTab();
        expect(queryByText("Review before starting")).toBeNull();
    });

    it("does not show the prompt when auto-run was not declined", () => {
        globalStore.set(model.autoRunDeclinedAtom, false);
        const { queryByText } = renderTab();
        expect(queryByText("Review before starting")).toBeNull();
    });
});
