// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

vi.mock("@/app/store/wshclientapi", () => ({
    RpcApi: { FileListCommand: vi.fn() },
}));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/util/remotetermutil", () => ({ formatRemoteUri: (path: string, _c: string) => path }));
vi.mock("@floating-ui/react", () => ({
    useFloating: () => ({
        refs: { setFloating: vi.fn(), floating: { current: null } },
        floatingStyles: {},
    }),
    autoUpdate: () => () => {},
    shift: (o: unknown) => o,
}));

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";

const fileListMock = (RpcApi as any).FileListCommand as ReturnType<typeof vi.fn>;

const ENTRIES = [
    { name: "..", path: "/home", isdir: true },
    { name: "sub", path: "/home/sub", isdir: true },
];

afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    document.body.innerHTML = "";
});

async function setup() {
    const { DirectoryDropdown } = await import("./directorydropdown");
    fileListMock.mockResolvedValue(ENTRIES);
    const anchor = document.createElement("span");
    document.body.appendChild(anchor);
    const onClose = vi.fn();
    const onSelect = vi.fn();
    render(
        <DirectoryDropdown
            currentPath="/home"
            connection="conn1"
            onSelect={onSelect}
            onClose={onClose}
            anchorRef={{ current: anchor }}
        />
    );
    await screen.findByText("sub");
    const user = userEvent.setup();
    return { user, onClose, onSelect };
}

it("ArrowDown highlights the row and Enter drills into it", async () => {
    const { user } = await setup();
    // ArrowDown ×2: no-selection → parent entry (0) → sub entry (1)
    await user.keyboard("{ArrowDown}");
    await user.keyboard("{ArrowDown}");
    const subRow = screen.getByText("sub").closest(".directory-dropdown-item")!;
    expect(subRow.className).toContain("keyboard-active");
    await user.keyboard("{Enter}");
    // breadcrumb shows the drilled path and the directory reloads
    await screen.findByText("/home/sub");
    expect(fileListMock).toHaveBeenLastCalledWith(TabRpcClient, { path: "/home/sub" }, undefined);
});

it("Enter on the focused OK button keeps its native action (applies, does not drill)", async () => {
    const { user, onClose, onSelect } = await setup();
    const okButton = screen.getByRole("button", { name: "OK" });
    okButton.focus();
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith("/home");
    expect(onClose).toHaveBeenCalledTimes(1);
});

it("ArrowDown while the OK button is focused does not leak to the document", async () => {
    const { user } = await setup();
    const docListener = vi.fn();
    document.addEventListener("keydown", docListener);
    try {
        screen.getByRole("button", { name: "OK" }).focus();
        await user.keyboard("{ArrowDown}");
        expect(docListener).not.toHaveBeenCalled();
    } finally {
        document.removeEventListener("keydown", docListener);
    }
});

it("swallows plain printable keys so they cannot reach the terminal behind the portal", async () => {
    const { user } = await setup();
    const docListener = vi.fn();
    document.addEventListener("keydown", docListener);
    try {
        await user.keyboard("a");
        expect(docListener).not.toHaveBeenCalled();
    } finally {
        document.removeEventListener("keydown", docListener);
    }
});

it("Escape closes the dropdown via the document-level handler", async () => {
    const { user, onClose } = await setup();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
});