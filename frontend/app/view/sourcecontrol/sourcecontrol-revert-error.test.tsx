// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { globalStore } from "@/app/store/jotaiStore";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { atom, Provider } from "jotai";
import { afterEach, expect, it, vi } from "vitest";

vi.mock("@/store/global", () => ({ getFocusedTerminalCwd: () => "/repo" }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/monaco/monaco-react", () => ({ MonacoDiffViewer: () => null }));
vi.mock("monaco-editor", () => ({}));
vi.mock("@/app/element/directorydropdown", () => ({ DirectoryDropdown: () => null }));

const PartialRevertError = "hunk unstaged, but the working tree has since changed and was left as is: git apply failed";

let model: any;

afterEach(() => {
    cleanup();
    model?.dispose?.();
    model?.stopPolling();
});

it("a failed staged-hunk revert shows a dismissible alert in the source control view", async () => {
    const { SourceControlViewModel } = await import("./sourcecontrol-model");
    const { SourceControlView } = await import("./sourcecontrol");
    const status = {
        branch: "main",
        staged: [{ path: "a.ts", status: "M", oldPath: "", icon: "", color: "" }],
        unstaged: [],
        untracked: [],
    };
    model = new SourceControlViewModel({
        blockId: "b1",
        waveEnv: {
            getBlockMetaKeyAtom: () => atom(null),
            getConnStatusAtom: () => atom({ connected: true }),
            rpc: {
                GitStatusCommand: vi.fn(async () => status),
                GitDiffCommand: vi.fn(async () => null),
                GitRevertHunkCommand: vi.fn(async () => {
                    throw new Error(PartialRevertError);
                }),
            },
        } as any,
    } as any);
    vi.spyOn(console, "error").mockImplementation(() => {});
    render(
        <Provider store={globalStore}>
            <SourceControlView blockId="b1" model={model} {...({} as any)} />
        </Provider>
    );
    await waitFor(() => expect(screen.getByText("main")).toBeTruthy());
    expect(screen.queryByRole("alert")).toBeNull();

    await act(() => model.revertHunk("a.ts", 0, true));
    await act(() => model.fetchStatus());

    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain(`Failed to revert hunk: ${PartialRevertError}`);

    await userEvent.setup().click(screen.getByRole("button", { name: "Dismiss error" }));
    expect(screen.queryByRole("alert")).toBeNull();
});
