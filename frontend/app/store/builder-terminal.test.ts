// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { BuilderNoticeAtom, isTermBlockDef, openBuilderTerminal, showConnectionUi, splitActionFor } from "./builder-terminal";
import { globalStore } from "./jotaiStore";
import { setWaveWindowType } from "./windowtype";

describe("splitActionFor", () => {
    it("maps the layout split directions to the server's target actions", () => {
        expect(splitActionFor("horizontal", "after")).toBe("splitright");
        expect(splitActionFor("horizontal", "before")).toBe("splitleft");
        expect(splitActionFor("vertical", "after")).toBe("splitdown");
        expect(splitActionFor("vertical", "before")).toBe("splitup");
    });
});

describe("isTermBlockDef", () => {
    it("matches only term views", () => {
        expect(isTermBlockDef({ meta: { view: "term", controller: "shell" } })).toBe(true);
        expect(isTermBlockDef({ meta: { view: "web" } })).toBe(false);
        expect(isTermBlockDef(null)).toBe(false);
    });
});

describe("openBuilderTerminal", () => {
    const open = vi.fn();

    beforeEach(() => {
        open.mockReset();
        (window as any).api = { openBuilderTerminal: open };
    });

    it("sends the target and clears the notice on success", async () => {
        globalStore.set(BuilderNoticeAtom, "old notice");
        open.mockResolvedValue("");
        expect(await openBuilderTerminal("splitdown", "b1")).toBe("");
        expect(open).toHaveBeenCalledWith({ targetblockid: "b1", targetaction: "splitdown" });
        expect(globalStore.get(BuilderNoticeAtom)).toBe("");
    });

    it("appends with an empty target and shows the error as the notice", async () => {
        open.mockResolvedValue("builder terminal not ready");
        expect(await openBuilderTerminal("", null)).toBe("builder terminal not ready");
        expect(open).toHaveBeenCalledWith({ targetblockid: "", targetaction: "" });
        expect(globalStore.get(BuilderNoticeAtom)).toBe("builder terminal not ready");
    });
});

describe("showConnectionUi", () => {
    it("is off in builder windows only", () => {
        setWaveWindowType("builder");
        expect(showConnectionUi()).toBe(false);
        setWaveWindowType("tab");
        expect(showConnectionUi()).toBe(true);
    });
});
