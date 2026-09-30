// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { rgba } from "@xterm/xterm/src/common/Color";
import { colord, extend } from "colord";
import a11yPlugin from "colord/plugins/a11y";
import { describe, expect, test } from "vitest";
import termthemes from "../../../../pkg/rtconfig/defaultconfig/termthemes.json";
import { LightThemeMinimumContrastRatio } from "./termutil";

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

const AnsiKeys = ForegroundKeys.filter((key) => key !== "gray" && key !== "cmdtext" && key !== "foreground");

describe("default-light palette", () => {
    const theme = (termthemes as Record<string, Record<string, unknown>>)["default-light"] as Record<string, string>;

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

describe("default theme cursors", () => {
    // xterm.js falls back to its own hardcoded white cursor whenever a theme's
    // "cursor" is blank. That's invisible-safe for default-dark's black
    // background but was silently invisible against default-light's white
    // background, since nothing here overrides the library default per theme.
    test.each(["default-dark", "default-light"])("%s defines an explicit, non-blank cursor colour", (name) => {
        const theme = (termthemes as Record<string, Record<string, unknown>>)[name] as Record<string, string>;
        expect(theme.cursor, `${name}.cursor must not be blank (falls back to xterm's white default)`).toBeTruthy();
    });

    test.each(["default-dark", "default-light"])("%s cursor reaches WCAG AA contrast on its background", (name) => {
        const theme = (termthemes as Record<string, Record<string, unknown>>)[name] as Record<string, string>;
        const ratio = colord(theme.cursor).contrast(theme.background);
        expect(
            ratio,
            `${name}.cursor=${theme.cursor} is ${ratio.toFixed(2)}:1 on ${theme.background}`
        ).toBeGreaterThanOrEqual(MinContrast);
    });
});

// xterm.js lifts the foreground of any cell that falls below minimumContrastRatio against the cell's
// own background, using the same routine imported here. That is what keeps palette-as-background cells
// (`ls` LS_COLORS `ow` is blue on green, ANSI 40-47 with any foreground) readable on the light theme,
// where a palette tuned for text on white cannot serve both roles.
describe("default-light palette cell pairs after xterm's minimum contrast", () => {
    const theme = (termthemes as Record<string, Record<string, unknown>>)["default-light"] as Record<string, string>;
    const ratioFloor = LightThemeMinimumContrastRatio;
    const toRgba = (hex: string) => {
        const { r, g, b } = colord(hex).toRgb();
        return (((r << 24) | (g << 16) | (b << 8) | 0xff) >>> 0) as number;
    };
    const fromRgba = (v: number) => colord({ r: (v >>> 24) & 0xff, g: (v >>> 16) & 0xff, b: (v >>> 8) & 0xff });
    function adjusted(fg: string, bg: string): string {
        const out = rgba.ensureContrastRatio(toRgba(bg), toRgba(fg), ratioFloor);
        return out == null ? colord(fg).toHex() : fromRgba(out).toHex();
    }
    const palette = AnsiKeys.map((key) => [key, theme[key]] as const);
    const xterm256 = (n: number) => {
        const v = 8 + (n - 232) * 10;
        return colord({ r: v, g: v, b: v }).toHex();
    };

    test("the light theme requests that floor", () => {
        expect(ratioFloor).toBe(4.5);
    });

    test.each(palette)("ANSI colour %s as a background: default foreground is readable", (key, bg) => {
        const fg = adjusted(theme.foreground, bg);
        const ratio = colord(fg).contrast(bg);
        expect(
            ratio,
            `${key} bg ${bg}, foreground ${theme.foreground} -> ${fg} is ${ratio.toFixed(2)}:1`
        ).toBeGreaterThanOrEqual(MinContrast);
    });

    test("every palette foreground on every palette background is readable", () => {
        for (const [bgKey, bg] of palette) {
            for (const [fgKey, fgColour] of palette) {
                const fg = adjusted(fgColour, bg);
                const ratio = colord(fg).contrast(bg);
                expect(
                    ratio,
                    `${fgKey} on ${bgKey}: ${fgColour} -> ${fg} on ${bg} is ${ratio.toFixed(2)}:1`
                ).toBeGreaterThanOrEqual(MinContrast);
            }
        }
    });

    test.each([248, 252])("256-colour grey %i on the default background is readable", (n) => {
        const grey = xterm256(n);
        const fg = adjusted(grey, theme.background);
        const ratio = colord(fg).contrast(theme.background);
        expect(ratio, `grey ${n} ${grey} -> ${fg} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(MinContrast);
    });
});
