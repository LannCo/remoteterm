// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { DefaultBuilderLayout, mergeBuilderLayout } from "./builder-layout";

describe("mergeBuilderLayout", () => {
    it("uses the defaults when nothing is saved", () => {
        expect(mergeBuilderLayout(null)).toEqual({ terminal: 40, app: 80, build: 20 });
        expect(DefaultBuilderLayout.terminal).toBe(40);
    });

    it("gives a layout saved before the terminal panel a 40% terminal", () => {
        expect(mergeBuilderLayout({ app: 70, build: 30 })).toEqual({ terminal: 40, app: 70, build: 30 });
    });

    it("keeps a saved terminal width", () => {
        expect(mergeBuilderLayout({ terminal: 55, app: 80, build: 20 }).terminal).toBe(55);
    });

    it("ignores values that are not usable percentages", () => {
        for (const bad of [Number.NaN, 0, 150, -5, "30" as unknown as number]) {
            expect(mergeBuilderLayout({ terminal: bad }).terminal).toBe(40);
        }
    });
});
