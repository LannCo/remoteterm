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

async function renderView(rpc: Record<string, (...args: any[]) => Promise<unknown>>) {
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
                ...rpc,
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
}

const rejects = (message: string) =>
    vi.fn(async () => {
        throw new Error(message);
    });

it("a failed staged-hunk revert shows a dismissible alert in the source control view", async () => {
    await renderView({ GitRevertHunkCommand: rejects(PartialRevertError) });

    await act(() => model.revertHunk("a.ts", 0, true));
    await act(() => model.fetchStatus());

    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain(`Failed to revert hunk: ${PartialRevertError}`);

    await userEvent.setup().click(screen.getByRole("button", { name: "Dismiss error" }));
    expect(screen.queryByRole("alert")).toBeNull();
});

it("dismissing the alert from the keyboard leaves focus in the source control view, not on <body>", async () => {
    await renderView({ GitRevertHunkCommand: rejects(PartialRevertError) });
    await act(() => model.revertHunk("a.ts", 0, true));

    const user = userEvent.setup();
    screen.getByRole("button", { name: "Dismiss error" }).focus();
    await user.keyboard("{Enter}");

    expect(screen.queryByRole("alert")).toBeNull();
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement.contains(screen.getByText("main"))).toBe(true);
});

// happy-dom does no layout, so this pins the sizing utility rather than a measured box:
// WCAG 2.5.8 needs at least 24x24 CSS px, and the bare icon is about 11px.
it("the dismiss button carries a 24x24 minimum target", async () => {
    await renderView({ GitRevertHunkCommand: rejects(PartialRevertError) });
    await act(() => model.revertHunk("a.ts", 0, true));

    const cls = screen.getByRole("button", { name: "Dismiss error" }).className.split(/\s+/);
    expect(cls).toContain("min-w-6");
    expect(cls).toContain("min-h-6");
});

it.each([
    ["stage files", "GitStageCommand", (m: any) => m.stageFiles(["a.ts"])],
    ["unstage files", "GitUnstageCommand", (m: any) => m.unstageFiles(["a.ts"])],
    ["stage hunk", "GitStageHunkCommand", (m: any) => m.stageHunk("a.ts", 0)],
])("a failed %s action shows the alert, and a later status poll leaves it up", async (what, rpcName, run) => {
    await renderView({ [rpcName]: rejects("index.lock exists") });

    await act(() => run(model));
    await act(() => model.fetchStatus());

    expect(screen.getByRole("alert").textContent).toContain(`Failed to ${what}: index.lock exists`);
});
