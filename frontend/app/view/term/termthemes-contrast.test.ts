// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { colord, extend } from "colord";
import a11yPlugin from "colord/plugins/a11y";
import { describe, expect, test } from "vitest";
import termthemes from "../../../../pkg/rtconfig/defaultconfig/termthemes.json";

extend([a11yPlugin]);

const MinContrast = 4.5;
const ForegroundKeys = [
    "black",
    "red",
    "green",
    "yellow",
    "blue",
    "magenta",
    "cyan",
    "white",
    "brightBlack",
    "brightRed",
    "brightGreen",
    "brightYellow",
    "brightBlue",
    "brightMagenta",
    "brightCyan",
    "brightWhite",
    "gray",
    "cmdtext",
    "foreground",
] as const;

describe("default-light palette", () => {
    const theme = (termthemes as Record<string, Record<string, unknown>>)["default-light"] as Record<
        string,
        string
    >;

    test("exists with a white background and display metadata", () => {
        expect(theme).toBeDefined();
        expect(theme.background).toBe("#ffffff");
        expect(theme["display:name"]).toBe("Default Light");
    });

    test.each(ForegroundKeys)("%s reaches WCAG AA contrast on the background", (key) => {
        const value = theme[key];
        expect(value, `${key} missing`).toBeTruthy();
        const ratio = colord(value).contrast(theme.background);
        expect(ratio, `${key}=${value} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(MinContrast);
    });

    test("display:order values are unique across built-in palettes", () => {
        const orders = Object.values(termthemes as Record<string, Record<string, unknown>>).map(
            (t) => t["display:order"]
        );
        expect(new Set(orders).size).toBe(orders.length);
    });
});
