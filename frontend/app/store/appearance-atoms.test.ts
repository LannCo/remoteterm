// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { resolveAppearanceMode } from "./appearance-atoms";

describe("resolveAppearanceMode", () => {
    test("tab override wins regardless of global/OS", () => {
        expect(resolveAppearanceMode("dark", "light", false)).toBe("dark");
        expect(resolveAppearanceMode("light", "dark", true)).toBe("light");
    });

    test("tab inherit falls through to global light/dark", () => {
        expect(resolveAppearanceMode(null, "light", true)).toBe("light");
        expect(resolveAppearanceMode(undefined, "dark", false)).toBe("dark");
    });

    test("global system falls through to OS preference", () => {
        expect(resolveAppearanceMode(null, "system", true)).toBe("dark");
        expect(resolveAppearanceMode(null, "system", false)).toBe("light");
    });

    test("everything absent falls back to dark", () => {
        expect(resolveAppearanceMode(null, null, true)).toBe("dark");
        expect(resolveAppearanceMode(undefined, undefined, false)).toBe("light");
    });
});
