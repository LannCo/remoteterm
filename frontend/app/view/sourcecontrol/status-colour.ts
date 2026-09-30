// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// The badge draws its text in the status colour over a 0x20-alpha tint of the same colour.
export const StatusBadgeTintAlpha = 0x20 / 255;

// The colours git.go sends (getStatusIcon) are pale enough for a dark surface and 1.8-3.6:1 on a
// light one. The wire format stays as is; light mode swaps in darker equivalents here.
const LightStatusColours: Record<string, string> = {
    "#f0a30a": "#7a5200",
    "#73c991": "#176a34",
    "#f14c4c": "#ad2222",
    "#ffffff": "#454a44",
};

export function mapStatusColour(color: string, mode: "light" | "dark"): string {
    if (mode === "dark") {
        return color;
    }
    return LightStatusColours[color?.toLowerCase()] ?? color;
}
