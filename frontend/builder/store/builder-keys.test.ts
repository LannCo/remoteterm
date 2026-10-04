// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { BuilderWebviewKeys, makeBuilderKeyTables } from "./builder-keys";

type Focus = "app" | "terminal";

function setup(focus: Focus, focusedBlockId: string = "b1") {
    const state = { focus, moveDisabled: false };
    const deps = {
        getFocusType: () => state.focus,
        closeBuilderWindow: vi.fn(),
        closeFocusedPane: vi.fn(),
        openTerminal: vi.fn(),
        getFocusedBlockId: () => focusedBlockId,
        isFocusMoveDisabled: () => state.moveDisabled,
        switchBlockInDirection: vi.fn(),
        switchBlockByBlockNum: vi.fn(),
        magnifyFocused: vi.fn(),
        activateSearch: vi.fn(() => true),
        handleEscape: vi.fn(() => true),
    };
    return { state, deps, tables: makeBuilderKeyTables(deps) };
}

function press(handler: (e: WaveKeyboardEvent) => boolean, repeat = false): boolean {
    return handler({ type: "keydown", repeat } as WaveKeyboardEvent);
}

describe("builder key tables", () => {
    it("closes the focused pane with Cmd:w on the terminal side and the window on the app side", () => {
        const { state, deps, tables } = setup("terminal");
        expect(press(tables.keyMap.get("Cmd:w"))).toBe(true);
        expect(deps.closeFocusedPane).toHaveBeenCalledTimes(1);
        expect(deps.closeBuilderWindow).not.toHaveBeenCalled();
        state.focus = "app";
        expect(press(tables.keyMap.get("Cmd:w"))).toBe(true);
        expect(deps.closeBuilderWindow).toHaveBeenCalledTimes(1);
    });

    it("ignores auto-repeated Cmd:w in both foci", () => {
        const { state, deps, tables } = setup("terminal");
        expect(press(tables.keyMap.get("Cmd:w"), true)).toBe(true);
        state.focus = "app";
        expect(press(tables.keyMap.get("Cmd:w"), true)).toBe(true);
        expect(deps.closeFocusedPane).not.toHaveBeenCalled();
        expect(deps.closeBuilderWindow).not.toHaveBeenCalled();
    });

    it("opens panes and splits through open-builder-terminal on the focused block", () => {
        const { deps, tables } = setup("terminal");
        press(tables.keyMap.get("Cmd:n"));
        expect(deps.openTerminal).toHaveBeenLastCalledWith("", null);
        press(tables.keyMap.get("Cmd:d"));
        expect(deps.openTerminal).toHaveBeenLastCalledWith("splitright", "b1");
        press(tables.keyMap.get("Shift:Cmd:d"));
        expect(deps.openTerminal).toHaveBeenLastCalledWith("splitdown", "b1");
        const chord = tables.chordMap.get("Ctrl:Shift:s");
        const expected: [string, string][] = [
            ["ArrowUp", "splitup"],
            ["ArrowDown", "splitdown"],
            ["ArrowLeft", "splitleft"],
            ["ArrowRight", "splitright"],
        ];
        for (const [key, action] of expected) {
            expect(press(chord.get(key))).toBe(true);
            expect(deps.openTerminal).toHaveBeenLastCalledWith(action, "b1");
        }
    });

    it("does not split without a focused pane", () => {
        const { deps, tables } = setup("terminal", null);
        expect(press(tables.keyMap.get("Cmd:d"))).toBe(true);
        expect(deps.openTerminal).not.toHaveBeenCalled();
    });

    it("leaves every key except Cmd:w to the app side when it is focused", () => {
        const { deps, tables } = setup("app");
        for (const [key, handler] of tables.keyMap) {
            if (key === "Cmd:w") {
                continue;
            }
            expect(press(handler), key).toBe(false);
        }
        for (const handler of tables.chordMap.get("Ctrl:Shift:s").values()) {
            expect(press(handler)).toBe(false);
        }
        expect(deps.openTerminal).not.toHaveBeenCalled();
        expect(deps.switchBlockInDirection).not.toHaveBeenCalled();
        expect(deps.magnifyFocused).not.toHaveBeenCalled();
        expect(deps.activateSearch).not.toHaveBeenCalled();
        expect(deps.handleEscape).not.toHaveBeenCalled();
    });

    it("runs focus moves, magnify, block numbers, search and Escape as a tab does", () => {
        const { state, deps, tables } = setup("terminal");
        press(tables.keyMap.get("Ctrl:Shift:ArrowLeft"));
        press(tables.keyMap.get("Ctrl:Shift:l"));
        expect(deps.switchBlockInDirection.mock.calls).toEqual([[3], [1]]);
        press(tables.keyMap.get("Cmd:m"));
        expect(deps.magnifyFocused).toHaveBeenCalledTimes(1);
        press(tables.keyMap.get("Ctrl:Shift:c{Digit3}"));
        press(tables.keyMap.get("Ctrl:Shift:c{Numpad9}"));
        expect(deps.switchBlockByBlockNum.mock.calls).toEqual([[3], [9]]);
        expect(press(tables.keyMap.get("Cmd:f"))).toBe(true);
        expect(press(tables.keyMap.get("Escape"))).toBe(true);
        state.moveDisabled = true;
        expect(press(tables.keyMap.get("Ctrl:Shift:ArrowUp"))).toBe(false);
    });

    it("leaves the tab-window keys unbound", () => {
        const { tables } = setup("terminal");
        const unbound = ["Cmd:t", "Cmd:Shift:w", "Cmd:[", "Shift:Cmd:[", "Cmd:]", "Shift:Cmd:]", "F2", "Ctrl:Shift:i", "Ctrl:Shift:x", "Cmd:g", "Cmd:i", "Ctrl:w"];
        for (let idx = 1; idx <= 9; idx++) {
            unbound.push(`Cmd:${idx}`);
        }
        for (const key of unbound) {
            expect(tables.keyMap.has(key), key).toBe(false);
        }
    });

    it("forwards only Cmd:w from the preview webview", () => {
        expect(setup("app").tables.webviewKeys).toEqual(["Cmd:w"]);
        expect(BuilderWebviewKeys).toEqual(["Cmd:w"]);
    });
});
