// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
    focusedRemoteTermWindow: { close: vi.fn() },
    livePopups: new Set<unknown>(),
    builderWebContentsIds: new Set<number>(),
}));

vi.mock("electron", () => ({
    app: {},
    BrowserWindow: class {},
    ipcMain: { on: vi.fn(), handle: vi.fn() },
    Menu: { buildFromTemplate: vi.fn() },
    shell: {},
}));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn() }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("../frontend/util/util", () => ({ fireAndForget: vi.fn() }));
vi.mock("./emain-builder", () => ({
    focusedBuilderWindow: null,
    getBuilderWindowById: vi.fn(),
    getBuilderWindowByWebContentsId: (id: number) =>
        mocks.builderWebContentsIds.has(id) ? { builderId: "b" } : undefined,
}));
vi.mock("./emain-ipc", () => ({ openBuilderWindow: vi.fn() }));
vi.mock("./emain-platform", () => ({ isDev: false, unamePlatform: "darwin" }));
vi.mock("./emain-popup", () => ({ isLivePopup: (win: unknown) => mocks.livePopups.has(win) }));
vi.mock("./emain-tabview", () => ({ clearTabCache: vi.fn() }));
vi.mock("./emain-util", () => ({
    decreaseZoomLevel: vi.fn(),
    increaseZoomLevel: vi.fn(),
    resetZoomLevel: vi.fn(),
}));
vi.mock("./emain-window", () => ({
    createNewRemoteTermWindow: vi.fn(),
    createWorkspace: vi.fn(),
    get focusedRemoteTermWindow() {
        return mocks.focusedRemoteTermWindow;
    },
    getAllRemoteTermWindows: () => [],
    getRemoteTermWindowByWorkspaceId: vi.fn(),
    relaunchBrowserWindows: vi.fn(),
    RemoteTermBrowserWindow: class {},
}));
vi.mock("./emain-wsh", () => ({ ElectronWshClient: {} }));

import { makeFileMenu } from "./emain-menu";

const callbacks = { createNewRemoteTermWindow: vi.fn(), relaunchBrowserWindows: vi.fn() };

function hiddenAccelerators(menu: Electron.MenuItemConstructorOptions[]): string[] {
    return menu
        .filter((item) => item.visible === false && item.acceleratorWorksWhenHidden)
        .map((item) => item.accelerator);
}

beforeEach(() => {
    vi.clearAllMocks();
    mocks.livePopups.clear();
    mocks.builderWebContentsIds.clear();
});

describe("File menu hidden New Window accelerators", () => {
    it("registers Cmd+N and Cmd+T with no main window and no builder focused", () => {
        expect(hiddenAccelerators(makeFileMenu(0, callbacks, null, false))).toEqual(["Command+N", "Command+T"]);
    });

    it("leaves Cmd+N to the builder when a builder window is focused and no main window is open", () => {
        expect(hiddenAccelerators(makeFileMenu(0, callbacks, null, true))).toEqual([]);
    });

    it("registers nothing while a main window exists", () => {
        expect(hiddenAccelerators(makeFileMenu(1, callbacks, null, false))).toEqual([]);
    });
});

describe("File > Close", () => {
    function closeItem() {
        return makeFileMenu(0, callbacks, null, true).find((item) => item.role === "close");
    }

    function click(window: unknown) {
        (closeItem().click as (item: unknown, window: unknown) => void)(undefined, window);
    }

    it("closes the focused builder window, not the last-focused main window behind it", () => {
        const builder = { webContents: { id: 7 }, close: vi.fn() };
        mocks.builderWebContentsIds.add(7);
        click(builder);
        expect(builder.close).toHaveBeenCalledTimes(1);
        expect(mocks.focusedRemoteTermWindow.close).not.toHaveBeenCalled();
    });

    it("still closes a live popup that was clicked", () => {
        const popup = { webContents: { id: 9 }, close: vi.fn() };
        mocks.livePopups.add(popup);
        click(popup);
        expect(popup.close).toHaveBeenCalledTimes(1);
        expect(mocks.focusedRemoteTermWindow.close).not.toHaveBeenCalled();
    });

    it("closes the last-focused main window for a main window", () => {
        const mainWindow = { close: vi.fn() };
        click(mainWindow);
        expect(mainWindow.close).not.toHaveBeenCalled();
        expect(mocks.focusedRemoteTermWindow.close).toHaveBeenCalledTimes(1);
    });

    it("does not mistake an unrelated window with a web contents id for a builder", () => {
        const stray = { webContents: { id: 11 }, close: vi.fn() };
        click(stray);
        expect(stray.close).not.toHaveBeenCalled();
        expect(mocks.focusedRemoteTermWindow.close).toHaveBeenCalledTimes(1);
    });
});
