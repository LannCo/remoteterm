// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, render, screen } from "@testing-library/react";
import { Provider } from "jotai";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn(() => () => {}) }));
vi.mock("@/layout/index", () => ({ deleteLayoutModelForTab: vi.fn() }));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    // One atom for every key: a component given a new atom on each render re-renders forever.
    const settingAtom = atom(false);
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app"), staticTabId: atom(null) },
        getApi: vi.fn(),
        getSettingsKeyAtom: vi.fn(() => settingAtom),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}` },
    };
});

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderTermModel } from "@/builder/store/builder-term-model";
import { BuilderAppHeader } from "./builder-appheader";

describe("BuilderAppHeader Open terminal button", () => {
    afterEach(() => {
        cleanup();
    });

    it("is disabled until the terminal panel is ready, and again when it leaves ready", async () => {
        const model = BuilderTermModel.getInstance();
        model.setState("loading");
        render(
            <Provider store={globalStore}>
                <BuilderAppHeader />
            </Provider>
        );
        const button = screen.getByText("Open terminal").closest("button");
        expect(button.disabled).toBe(true);
        await act(async () => {
            model.setState("ready");
        });
        expect(button.disabled).toBe(false);
        await act(async () => {
            model.markSwitching();
        });
        expect(button.disabled).toBe(true);
    });
});
