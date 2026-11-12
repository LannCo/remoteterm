// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { BuilderTermAction } from "@/app/store/builder-terminal";
import type { BuilderFocusType } from "@/builder/store/builder-focusmanager";
import { NavigateDirection } from "@/layout/lib/types";

export type BuilderKeyHandler = (waveEvent: WaveKeyboardEvent) => boolean;

export type BuilderKeyDeps = {
    getFocusType: () => BuilderFocusType;
    closeBuilderWindow: () => void;
    closeFocusedPane: () => void;
    openTerminal: (action: BuilderTermAction, targetBlockId: string) => void;
    getFocusedBlockId: () => string;
    isFocusMoveDisabled: () => boolean;
    switchBlockInDirection: (direction: NavigateDirection) => void;
    switchBlockByBlockNum: (blockNum: number) => void;
    magnifyFocused: () => void;
    activateSearch: (waveEvent: WaveKeyboardEvent) => boolean;
    handleEscape: () => boolean;
};

export type BuilderKeyTables = {
    keyMap: Map<string, BuilderKeyHandler>;
    chordMap: Map<string, Map<string, BuilderKeyHandler>>;
    webviewKeys: string[];
};

// The preview webview forwards only these to the builder, so every other key stays with the app being built.
export const BuilderWebviewKeys = ["Cmd:w"];

const FocusMoveKeys: [string, NavigateDirection][] = [
    ["Ctrl:Shift:ArrowUp", NavigateDirection.Up],
    ["Ctrl:Shift:ArrowDown", NavigateDirection.Down],
    ["Ctrl:Shift:ArrowLeft", NavigateDirection.Left],
    ["Ctrl:Shift:ArrowRight", NavigateDirection.Right],
    ["Ctrl:Shift:k", NavigateDirection.Up],
    ["Ctrl:Shift:j", NavigateDirection.Down],
    ["Ctrl:Shift:h", NavigateDirection.Left],
    ["Ctrl:Shift:l", NavigateDirection.Right],
];

const SplitChordKeys: [string, BuilderTermAction][] = [
    ["ArrowUp", "splitup"],
    ["ArrowDown", "splitdown"],
    ["ArrowLeft", "splitleft"],
    ["ArrowRight", "splitright"],
];

export function makeBuilderKeyTables(deps: BuilderKeyDeps): BuilderKeyTables {
    const onTerminal = (handler: BuilderKeyHandler): BuilderKeyHandler => {
        return (waveEvent) => {
            if (deps.getFocusType() !== "terminal") {
                return false;
            }
            return handler(waveEvent);
        };
    };
    const split = (action: BuilderTermAction): BuilderKeyHandler =>
        onTerminal(() => {
            const blockId = deps.getFocusedBlockId();
            if (blockId == null) {
                return true;
            }
            deps.openTerminal(action, blockId);
            return true;
        });

    const keyMap = new Map<string, BuilderKeyHandler>();
    // A held key would close every pane, move focus to the app side and then close the window.
    keyMap.set("Cmd:w", (waveEvent) => {
        if (waveEvent.repeat) {
            return true;
        }
        if (deps.getFocusType() === "terminal") {
            deps.closeFocusedPane();
            return true;
        }
        deps.closeBuilderWindow();
        return true;
    });
    keyMap.set(
        "Cmd:n",
        onTerminal(() => {
            deps.openTerminal("", null);
            return true;
        })
    );
    keyMap.set("Cmd:d", split("splitright"));
    keyMap.set("Shift:Cmd:d", split("splitdown"));
    keyMap.set(
        "Cmd:m",
        onTerminal(() => {
            deps.magnifyFocused();
            return true;
        })
    );
    for (const [key, direction] of FocusMoveKeys) {
        keyMap.set(
            key,
            onTerminal(() => {
                if (deps.isFocusMoveDisabled()) {
                    return false;
                }
                deps.switchBlockInDirection(direction);
                return true;
            })
        );
    }
    for (let blockNum = 1; blockNum <= 9; blockNum++) {
        const handler = onTerminal(() => {
            deps.switchBlockByBlockNum(blockNum);
            return true;
        });
        keyMap.set(`Ctrl:Shift:c{Digit${blockNum}}`, handler);
        keyMap.set(`Ctrl:Shift:c{Numpad${blockNum}}`, handler);
    }
    keyMap.set(
        "Cmd:f",
        onTerminal((waveEvent) => deps.activateSearch(waveEvent))
    );
    keyMap.set(
        "Escape",
        onTerminal(() => deps.handleEscape())
    );

    const splitChord = new Map<string, BuilderKeyHandler>();
    for (const [key, action] of SplitChordKeys) {
        splitChord.set(key, split(action));
    }
    return {
        keyMap,
        chordMap: new Map([["Ctrl:Shift:s", splitChord]]),
        webviewKeys: [...BuilderWebviewKeys],
    };
}
