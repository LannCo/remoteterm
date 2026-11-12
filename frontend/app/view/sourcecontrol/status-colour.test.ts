// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { colord, extend } from "colord";
import a11yPlugin from "colord/plugins/a11y";
import fs from "fs";
import path from "path";
import { describe, expect, test } from "vitest";
import { StatusBadgeTintAlpha, mapStatusColour } from "./status-colour";

extend([a11yPlugin]);

const gitGo = fs.readFileSync(
    path.join(__dirname, "..", "..", "..", "..", "pkg", "wshrpc", "wshremote", "git.go"),
    "utf8"
);
const getStatusIcon = gitGo.slice(gitGo.indexOf("func getStatusIcon"));
const wireColours = [...new Set([...getStatusIcon.matchAll(/"(#[0-9a-fA-F]{6})"/g)].map((m) => m[1]))];

// Row backgrounds a badge sits on: page, hover row, selected row, surface.
const rowBackgrounds = ["#ffffff", "#ebebeb", "#ebf8e8", "#f5f5f5"];

function tintOver(colour: string, alpha: number, backdrop: string): string {
    const f = colord(colour).toRgb();
    const b = colord(backdrop).toRgb();
    return colord({
        r: Math.round(f.r * alpha + b.r * (1 - alpha)),
        g: Math.round(f.g * alpha + b.g * (1 - alpha)),
        b: Math.round(f.b * alpha + b.b * (1 - alpha)),
    }).toHex();
}

describe("source control status colours", () => {
    test("git.go still sends the wire colours this mapping covers", () => {
        expect(wireColours.sort()).toEqual(["#73c991", "#f0a30a", "#f14c4c", "#ffffff"]);
    });

    test.each(wireColours)("dark mode passes %s through unchanged", (wire) => {
        expect(mapStatusColour(wire, "dark")).toBe(wire);
    });

    test("an unknown wire colour passes through in light mode", () => {
        expect(mapStatusColour("#123456", "light")).toBe("#123456");
    });

    describe.each(wireColours)("light mode %s", (wire) => {
        const light = mapStatusColour(wire, "light");

        test.each(rowBackgrounds)("badge text clears 4.5:1 on its tint over %s", (row) => {
            const tint = tintOver(light, StatusBadgeTintAlpha, row);
            const ratio = colord(light).contrast(tint);
            expect(
                ratio,
                `${wire} -> ${light} on tint ${tint} over ${row} is ${ratio.toFixed(2)}:1`
            ).toBeGreaterThanOrEqual(4.5);
        });
    });
});
