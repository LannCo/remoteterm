// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { globalStore } from "@/app/store/jotaiStore";
import { atom, type PrimitiveAtom } from "jotai";

export type BuilderFocusType = "app" | "terminal";

export class BuilderFocusManager {
    private static instance: BuilderFocusManager | null = null;

    focusType: PrimitiveAtom<BuilderFocusType> = atom<BuilderFocusType>("app");

    private constructor() {}

    static getInstance(): BuilderFocusManager {
        if (!BuilderFocusManager.instance) {
            BuilderFocusManager.instance = new BuilderFocusManager();
        }
        return BuilderFocusManager.instance;
    }

    setAppFocused() {
        globalStore.set(this.focusType, "app");
    }

    setTerminalFocused() {
        globalStore.set(this.focusType, "terminal");
    }

    getFocusType(): BuilderFocusType {
        return globalStore.get(this.focusType);
    }
}
