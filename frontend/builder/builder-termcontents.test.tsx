// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ deleteBlock: vi.fn(() => Promise.resolve()) }));

vi.mock("@/app/block/block", () => ({ Block: () => null }));
vi.mock("@/store/services", () => ({ ObjectService: { DeleteBlock: h.deleteBlock } }));

import { makeBuilderTileContents } from "./builder-termcontents";

describe("makeBuilderTileContents", () => {
    it("deletes the pane's block when its node is deleted", async () => {
        const contents = makeBuilderTileContents("tab-1", 3);
        expect(contents.tabId).toBe("tab-1");
        expect(contents.gapSizePx).toBe(3);
        await contents.onNodeDelete({ blockId: "b1" } as TabLayoutData);
        expect(h.deleteBlock).toHaveBeenCalledWith("b1");
    });
});
