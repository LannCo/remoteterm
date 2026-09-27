// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { colord, extend } from "colord";
import a11yPlugin from "colord/plugins/a11y";
import fs from "fs";
import path from "path";
import { describe, expect, test } from "vitest";

extend([a11yPlugin]);

const MinContrast = 4.5;

const themeScss = fs.readFileSync(path.join(__dirname, "theme.scss"), "utf8");
const tailwindsetupCss = fs.readFileSync(path.join(__dirname, "..", "tailwindsetup.css"), "utf8");

// Both files put the dark/default value first and the `:root[data-theme="light"]` override
// second, so splitting on that selector and taking the first match on each side keeps this
// parsed straight from source instead of a copy that can drift.
function extractVarValues(css: string, varName: string): { dark: string; light: string } {
    const lightSplitIndex = css.indexOf('[data-theme="light"]');
    const darkSection = lightSplitIndex === -1 ? css : css.slice(0, lightSplitIndex);
    const lightSection = lightSplitIndex === -1 ? "" : css.slice(lightSplitIndex);
    const re = new RegExp(`--${varName}:\\s*([^;]+);`);
    const darkMatch = darkSection.match(re);
    const lightMatch = lightSection.match(re);
    if (!darkMatch) {
        throw new Error(`--${varName} not found in dark section`);
    }
    return { dark: darkMatch[1].trim(), light: (lightMatch ?? darkMatch)[1].trim() };
}

const connStatusOverlayBg = extractVarValues(themeScss, "conn-status-overlay-bg-color");
const colorPrimary = extractVarValues(tailwindsetupCss, "color-primary");
const colorSecondary = extractVarValues(tailwindsetupCss, "color-secondary");
const colorWarning = extractVarValues(tailwindsetupCss, "color-warning");
const colorError = extractVarValues(tailwindsetupCss, "color-error");

function parseAlpha(cssColor: string): number {
    if (!cssColor.startsWith("rgba")) {
        return 1;
    }
    const rgbaMatch = cssColor.match(/rgba\([^)]+,\s*([\d.]+)\s*\)/);
    return rgbaMatch ? Number(rgbaMatch[1]) : 1;
}

function compositeRgb(fg: string, alpha: number, backdrop: string): ReturnType<typeof colord> {
    const f = colord(fg).toRgb();
    const b = colord(backdrop).toRgb();
    return colord({
        r: Math.round(f.r * alpha + b.r * (1 - alpha)),
        g: Math.round(f.g * alpha + b.g * (1 - alpha)),
        b: Math.round(f.b * alpha + b.b * (1 - alpha)),
    });
}

// Each consumer's text colour/opacity pair. All three components share
// --conn-status-overlay-bg-color: connstatusoverlay.tsx (text-primary, text-primary/70,
// text-warning, text-error), uploadoverlay.tsx and preview-error-overlay.tsx (text-primary,
// swapped from a fixed white — see those files' history), and preview-error-overlay.tsx's
// container (text-secondary). connstatusoverlay.tsx's two text-primary/50 error lines were
// changed to text-primary/70: 50%-opacity near-black text over any near-white surface caps
// around 3.3:1 regardless of the surface colour, so /50 can never pass here in light mode.
type TextCase = { name: string; color: string; opacity: number };

function themeCases(mode: "dark" | "light"): TextCase[] {
    const primary = mode === "dark" ? colorPrimary.dark : colorPrimary.light;
    const secondary = mode === "dark" ? colorSecondary.dark : colorSecondary.light;
    const warning = mode === "dark" ? colorWarning.dark : colorWarning.light;
    const error = mode === "dark" ? colorError.dark : colorError.light;
    return [
        { name: "text-primary", color: primary, opacity: 1 },
        { name: "text-primary/70", color: primary, opacity: 0.7 },
        { name: "text-warning", color: warning, opacity: 1 },
        { name: "text-error", color: error, opacity: 1 },
        { name: "text-secondary", color: secondary, opacity: 1 },
    ];
}

// Backdrop-blur means whatever's visually behind a non-opaque overlay colour can shift the
// effective composited colour toward either extreme; checking both extremes (not just white)
// is what caught the original bug (a value that only failed against a dark backdrop). A fully
// opaque bg collapses both extremes to the same result, which is the point of going near-opaque.
describe.each([
    ["dark", connStatusOverlayBg.dark],
    ["light", connStatusOverlayBg.light],
] as const)("--conn-status-overlay-bg-color (%s)", (mode, bgCssColor) => {
    const alpha = parseAlpha(bgCssColor);

    test.each(["#000000", "#ffffff"] as const)("every consumer clears 4.5:1 composited over %s", (extremeBackdrop) => {
        const effectiveBg = compositeRgb(bgCssColor, alpha, extremeBackdrop);
        for (const { name, color, opacity } of themeCases(mode)) {
            const effectiveText = compositeRgb(color, opacity, effectiveBg.toHex());
            const ratio = colord(effectiveText).contrast(effectiveBg.toHex());
            expect(
                ratio,
                `${mode}/${name}: ${color}@${opacity} on ${effectiveBg.toHex()} (bg ${bgCssColor} over ${extremeBackdrop}) is ${ratio.toFixed(2)}:1`
            ).toBeGreaterThanOrEqual(MinContrast);
        }
    });
});
