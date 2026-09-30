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
const connStatusOverlayTsx = fs.readFileSync(path.join(__dirname, "block", "connstatusoverlay.tsx"), "utf8");
const blockOverlayTsx = fs.readFileSync(path.join(__dirname, "block", "blockoverlay.tsx"), "utf8");
const blockScss = fs.readFileSync(path.join(__dirname, "block", "block.scss"), "utf8");

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

function extractSlice(src: string, startMarker: string, endMarker: string): string {
    const start = src.indexOf(startMarker);
    if (start === -1) {
        throw new Error(`marker not found: ${JSON.stringify(startMarker)}`);
    }
    const contentStart = start + startMarker.length;
    const end = src.indexOf(endMarker, contentStart);
    return end === -1 ? src.slice(contentStart) : src.slice(contentStart, end);
}

// Reads the element-level opacity actually applied by a consumer, straight from its source, so a
// reintroduced `opacity-90` (Tailwind) or `opacity: 0.9` (CSS) is picked up automatically instead
// of this test silently continuing to assume full opacity. This is what round 2's version of this
// test got wrong: it validated the raw --conn-status-overlay-bg-color token value in isolation,
// while every real consumer additionally applied opacity-90/opacity:0.9 on the SAME element,
// re-blending the backdrop behind it back in and reintroducing the contrast failure the opaque
// token was chosen to eliminate. See connstatusoverlay.tsx, blockoverlay.tsx, block.scss history.
function extractElementOpacity(snippet: string): number {
    const twMatch = snippet.match(/\bopacity-(\d{1,3})\b/);
    if (twMatch) {
        return Number(twMatch[1]) / 100;
    }
    const cssMatch = snippet.match(/opacity:\s*([\d.]+)/);
    if (cssMatch) {
        return Number(cssMatch[1]);
    }
    return 1;
}

// The real consumers of --conn-status-overlay-bg-color, and the element-level opacity each one
// currently renders the shell with (extracted from source rather than assumed).
const tsxConsumerSites: { name: string; opacity: number }[] = [
    {
        // Shared by StalledOverlay, DisconnectedOverlay, RetryingOverlay, CountdownOverlay,
        // JobSessionOverlay, DrainCatchUpOverlay, and the inline auth-queue-waiting overlay.
        name: "connstatusoverlay.tsx overlayShellClass",
        opacity: extractElementOpacity(extractSlice(connStatusOverlayTsx, "const overlayShellClass =", ";")),
    },
    {
        // FlappingOverlay duplicates the shell class as its own literal instead of reusing the
        // constant, so it must be checked independently.
        name: "connstatusoverlay.tsx FlappingOverlay",
        opacity: extractElementOpacity(
            extractSlice(connStatusOverlayTsx, "const FlappingOverlay = React.memo(", "FlappingOverlay.displayName")
        ),
    },
    {
        name: "blockoverlay.tsx BlockOverlay",
        opacity: extractElementOpacity(blockOverlayTsx),
    },
];

// The older, pre-Tailwind overlay styling path: block.scss's `.connstatus-overlay` (still used by
// the tail end of ConnStatusOverlay's render for the default/legacy status view). Its text colour
// is a flat `--secondary-text-color`, not one of the Tailwind text-*/opacity cases below, so it
// gets its own explicit case instead of folding into themeCases().
const legacyConnstatusOverlayOpacity = extractElementOpacity(
    extractSlice(blockScss, ".connstatus-overlay {", ".connstatus-content {")
);
const secondaryTextColor = extractVarValues(themeScss, "secondary-text-color");

// Each Tailwind consumer's text colour/opacity pair. All three tsxConsumerSites share
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

// Renders a text pixel the way the browser actually composites it: the text is drawn at its own
// opacity against the shell's own (opaque) background colour first, and only THEN does the whole
// element get alpha-blended against whatever is actually behind it (elementOpacity). A non-1
// elementOpacity re-exposes that real backdrop, which is exactly the bug this test exists to catch.
function renderedTextPixel(
    textColor: string,
    textOpacity: number,
    boxBg: string,
    elementOpacity: number,
    realBackdrop: string
): ReturnType<typeof colord> {
    const preOpacityPixel = compositeRgb(textColor, textOpacity, boxBg);
    return compositeRgb(preOpacityPixel.toHex(), elementOpacity, realBackdrop);
}

function renderedBgPixel(boxBg: string, elementOpacity: number, realBackdrop: string): ReturnType<typeof colord> {
    return compositeRgb(boxBg, elementOpacity, realBackdrop);
}

// Backdrop-blur means whatever's visually behind the overlay can shift the effective composited
// colour toward either extreme; checking both extremes (not just white) is what caught the
// original bug (a value that only failed against a dark backdrop).
describe.each([
    ["dark", connStatusOverlayBg.dark],
    ["light", connStatusOverlayBg.light],
] as const)("--conn-status-overlay-bg-color (%s)", (mode, bgCssColor) => {
    const tokenAlpha = parseAlpha(bgCssColor);

    describe.each(tsxConsumerSites)("$name", ({ opacity: elementOpacity }) => {
        test.each(["#000000", "#ffffff"] as const)(
            "every text case clears 4.5:1 composited over %s",
            (extremeBackdrop) => {
                const boxBg = compositeRgb(bgCssColor, tokenAlpha, extremeBackdrop).toHex();
                const effectiveBg = renderedBgPixel(boxBg, elementOpacity, extremeBackdrop);
                for (const { name, color, opacity } of themeCases(mode)) {
                    const effectiveText = renderedTextPixel(color, opacity, boxBg, elementOpacity, extremeBackdrop);
                    const ratio = colord(effectiveText).contrast(effectiveBg.toHex());
                    expect(
                        ratio,
                        `${mode}/${name}: ${color}@${opacity} on element-opacity ${elementOpacity} over ${extremeBackdrop} ` +
                            `is ${ratio.toFixed(2)}:1 (effective bg ${effectiveBg.toHex()})`
                    ).toBeGreaterThanOrEqual(MinContrast);
                }
            }
        );
    });

    test.each(["#000000", "#ffffff"] as const)(
        "legacy block.scss .connstatus-overlay secondary text clears 4.5:1 composited over %s",
        (extremeBackdrop) => {
            const boxBg = compositeRgb(bgCssColor, tokenAlpha, extremeBackdrop).toHex();
            const effectiveBg = renderedBgPixel(boxBg, legacyConnstatusOverlayOpacity, extremeBackdrop);
            const secondaryColor = mode === "dark" ? secondaryTextColor.dark : secondaryTextColor.light;
            const effectiveText = renderedTextPixel(
                secondaryColor,
                1,
                boxBg,
                legacyConnstatusOverlayOpacity,
                extremeBackdrop
            );
            const ratio = colord(effectiveText).contrast(effectiveBg.toHex());
            expect(
                ratio,
                `${mode}/secondary-text-color: ${secondaryColor} on element-opacity ${legacyConnstatusOverlayOpacity} ` +
                    `over ${extremeBackdrop} is ${ratio.toFixed(2)}:1 (effective bg ${effectiveBg.toHex()})`
            ).toBeGreaterThanOrEqual(MinContrast);
        }
    );
});

// Light-mode running text: the owner reads #6b6f6a/#707570 as washed-out grey, so the bar for the
// secondary tokens is 7:1 (AAA) on the page background and 4.5:1 on every tinted fill they sit on.
const SecondaryMinOnPage = 7;
const SecondaryMinDistinctFromPrimary = 1.5;

function lightCompositeOnWhite(cssColor: string): string {
    const c = colord(cssColor);
    return compositeRgb(c.toHex(), c.alpha(), "#ffffff").toHex();
}

const lightTextTokens: { name: string; color: string }[] = [
    { name: "--color-secondary", color: colorSecondary.light },
    { name: "--color-muted-foreground", color: extractVarValues(tailwindsetupCss, "color-muted-foreground").light },
    { name: "--color-muted", color: extractVarValues(tailwindsetupCss, "color-muted").light },
    { name: "--secondary-text-color", color: secondaryTextColor.light },
    { name: "--grey-text-color", color: extractVarValues(themeScss, "grey-text-color").light },
];

const lightTintedFills: { name: string; color: string }[] = [
    { name: "page", color: extractVarValues(tailwindsetupCss, "color-background").light },
    { name: "inputbg", color: lightCompositeOnWhite(extractVarValues(tailwindsetupCss, "color-inputbg").light) },
    { name: "activebg", color: lightCompositeOnWhite(extractVarValues(tailwindsetupCss, "color-activebg").light) },
    { name: "surface", color: lightCompositeOnWhite(extractVarValues(tailwindsetupCss, "color-surface").light) },
    { name: "hover", color: lightCompositeOnWhite(extractVarValues(tailwindsetupCss, "color-hover").light) },
    { name: "panel", color: lightCompositeOnWhite(extractVarValues(tailwindsetupCss, "color-panel").light) },
    // selected process-viewer row (bg-accentbg) is the darkest fill running text lands on
    {
        name: "accentbg (selected row)",
        color: lightCompositeOnWhite(extractVarValues(tailwindsetupCss, "color-accentbg").light),
    },
];

describe("light-mode secondary text tokens", () => {
    test.each(lightTextTokens)("$name reaches 7:1 on the page background", ({ name, color }) => {
        const ratio = colord(color).contrast("#ffffff");
        expect(ratio, `${name} ${color} is ${ratio.toFixed(2)}:1 on white`).toBeGreaterThanOrEqual(SecondaryMinOnPage);
    });

    describe.each(lightTextTokens)("$name", ({ name, color }) => {
        test.each(lightTintedFills)("clears 4.5:1 on $name", ({ name: fillName, color: fill }) => {
            const ratio = colord(color).contrast(fill);
            expect(ratio, `${name} ${color} on ${fillName} ${fill} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
                MinContrast
            );
        });

        test("stays visibly lighter than primary text", () => {
            const primaryRatio = colord(colorPrimary.light).contrast("#ffffff");
            const ratio = primaryRatio / colord(color).contrast("#ffffff");
            expect(ratio, `${name} vs primary luminance step ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(
                SecondaryMinDistinctFromPrimary
            );
        });
    });
});

// text-accent is the raw brand green (2.30:1 on white); text-accent-text is the theme-aware token for
// running text. The consumers below carry real text (headings, links, prompt glyphs), so they must
// use the token. Icon-only uses keep text-accent.
const AccentTextConsumers = [
    "onboarding/onboarding.tsx",
    "onboarding/onboarding-command.tsx",
    "onboarding/onboarding-layout-term.tsx",
    "element/quicktips.tsx",
    "element/markdown.tsx",
    "element/streamdown.tsx",
    "view/remotetermconfig/connectionscontent.tsx",
];
const RawAccentTextClass = /(?<![\w:-])text-accent(?:-400)?(?![\w-])/;

describe("light-mode accent text", () => {
    const accentText = extractVarValues(tailwindsetupCss, "color-accent-text");

    test.each(lightTintedFills.filter((f) => !f.name.startsWith("accentbg")))(
        "--color-accent-text clears 4.5:1 on $name",
        ({ name, color }) => {
            const ratio = colord(accentText.light).contrast(color);
            expect(ratio, `${accentText.light} on ${name} ${color} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
                MinContrast
            );
        }
    );

    test("dark mode keeps accent-text equal to the accent", () => {
        expect(accentText.dark).toBe(extractVarValues(tailwindsetupCss, "color-accent").dark);
    });

    test.each(AccentTextConsumers)("%s uses text-accent-text, not the raw accent, for text", (file) => {
        const src = fs.readFileSync(path.join(__dirname, file), "utf8");
        const offenders = src
            .split("\n")
            .map((line, i) => ({ line, n: i + 1 }))
            .filter(({ line }) => RawAccentTextClass.test(line) && !/\bfa-(solid|brands|sharp)/.test(line));
        expect(offenders.map(({ n, line }) => `${file}:${n} ${line.trim()}`)).toEqual([]);
    });
});

describe("light-mode placeholder text", () => {
    const resetScss = fs.readFileSync(path.join(__dirname, "..", "app", "reset.scss"), "utf8");
    const lightRule = resetScss.match(/:root\[data-theme="light"\]\s+::placeholder\s*\{([^}]*)\}/);

    test("reset.scss gives light mode its own placeholder colour from a token", () => {
        expect(lightRule, "no :root[data-theme=light] ::placeholder rule in reset.scss").not.toBeNull();
        expect(lightRule[1]).toMatch(/color:\s*var\(--placeholder-color\)/);
    });

    test("the base placeholder rule (dark mode) is untouched", () => {
        expect(resetScss).toMatch(
            /\n {4}::placeholder \{\s*opacity: 1;[^}]*color-mix\(in oklab, currentColor 50%, transparent\)/
        );
    });

    const placeholder = {
        light: themeScss
            .slice(themeScss.indexOf('[data-theme="light"]'))
            .match(/--placeholder-color:\s*([^;]+);/)?.[1]
            .trim(),
    };
    test.each(lightTintedFills.filter((f) => !f.name.startsWith("accentbg")))(
        "--placeholder-color clears 4.5:1 on $name",
        ({ name, color }) => {
            const ratio = colord(placeholder.light).contrast(color);
            expect(ratio, `${placeholder.light} on ${name} ${color} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
                MinContrast
            );
        }
    );
});

describe("light-mode markdown code and links", () => {
    const markdownScss = fs.readFileSync(path.join(__dirname, "element", "markdown.scss"), "utf8");
    const githubScss = fs.readFileSync(
        path.join(__dirname, "..", "..", "node_modules", "highlight.js", "scss", "github.scss"),
        "utf8"
    );
    const lightStart = markdownScss.indexOf(':root[data-theme="light"] {');
    const lightBlock = lightStart === -1 ? "" : markdownScss.slice(lightStart, markdownScss.indexOf("\n}", lightStart));

    function selectorColours(css: string): Map<string, string> {
        const out = new Map<string, string>();
        for (const m of css
            .replace(/\/\*[\s\S]*?\*\/|@import[^;]*;|\/\/[^\n]*/g, "")
            .matchAll(/([^{}]+)\{([^}]*)\}/g)) {
            const colour = m[2].match(/(?<![-\w])color:\s*(#[0-9a-fA-F]{3,8})/)?.[1];
            if (!colour) {
                continue;
            }
            for (const sel of m[1].split(",")) {
                out.set(sel.trim(), colour);
            }
        }
        return out;
    }

    test("markdown.scss loads a light highlight.js theme scoped to light mode and keeps the dark one", () => {
        expect(markdownScss).toMatch(/@import url\("[^"]*highlight\.js\/scss\/github-dark-dimmed\.scss"\);/);
        expect(lightBlock).toMatch(/@import "[^"]*highlight\.js\/scss\/github\.scss";/);
    });

    const panel = lightCompositeOnWhite(extractVarValues(themeScss, "panel-bg-color").light);
    const effective = new Map([
        ...selectorColours(githubScss),
        ...selectorColours(lightBlock.replace(':root[data-theme="light"] {', "")),
    ]);

    test("every highlight.js token colour reaches 4.5:1 on the code box (--panel-bg-color)", () => {
        expect(effective.size).toBeGreaterThan(30);
        const failures = [...effective]
            .map(([sel, colour]) => ({ sel, colour, ratio: colord(colour).contrast(panel) }))
            .filter(({ ratio }) => ratio < MinContrast)
            .map(({ sel, colour, ratio }) => `${sel} ${colour} is ${ratio.toFixed(2)}:1 on ${panel}`);
        expect(failures).toEqual([]);
    });

    const link = extractVarValues(themeScss, "term-bright-blue");
    test("markdown links (--term-bright-blue) reach 4.5:1 on the page and on the code box", () => {
        for (const bg of [extractVarValues(tailwindsetupCss, "color-background").light, panel]) {
            const ratio = colord(link.light).contrast(bg);
            expect(ratio, `${link.light} on ${bg} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(MinContrast);
        }
    });

    test("dark mode keeps the original link colour", () => {
        expect(link.dark).toBe("#32afff");
    });
});

describe("Monaco light theme", () => {
    const monacoEnvTs = fs.readFileSync(path.join(__dirname, "monaco", "monaco-env.ts"), "utf8");
    const themeBlock = (name: string) => {
        const start = monacoEnvTs.indexOf(`defineTheme("${name}"`);
        return monacoEnvTs.slice(start, monacoEnvTs.indexOf("});", start));
    };
    const colourOf = (block: string, key: string) => block.match(new RegExp(`"${key}":\\s*"(#[0-9a-fA-F]{6,8})"`))?.[1];
    const light = themeBlock("wave-theme-light");
    const dark = themeBlock("wave-theme-dark");

    test("defines a minimap.background, as the dark theme does, and it is a light colour", () => {
        expect(colourOf(dark, "minimap.background")).toBeDefined();
        const minimap = colourOf(light, "minimap.background");
        expect(
            minimap,
            "wave-theme-light has no minimap.background: Monaco assumes a dark canvas and paints a grey slab"
        ).toBeDefined();
        expect(colord(minimap).luminance()).toBeGreaterThan(0.8);
    });

    test("dimmed line numbers reach 4.5:1 on the page and on the panel", () => {
        const dimmed = colourOf(light, "editorLineNumber.dimmedForeground");
        expect(dimmed, "Monaco derives the dimmed colour at 0.4 alpha (1.76:1) unless the theme sets it").toBeDefined();
        const panel = lightCompositeOnWhite(extractVarValues(themeScss, "panel-bg-color").light);
        for (const bg of ["#ffffff", panel]) {
            const ratio = colord(dimmed).contrast(bg);
            expect(ratio, `${dimmed} on ${bg} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(MinContrast);
        }
    });

    test("dark theme colours are unchanged", () => {
        expect(dark).toContain('"minimap.background": "#00000077"');
        expect(dark).not.toContain("dimmedForeground");
    });
});
