// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import fs from "fs";
import path from "path";
import { describe, expect, it } from "vitest";

const FrontendRoot = path.join(import.meta.dirname, "..");
// The preview harness (frontend/preview) fakes tab switching on its own mock atoms; it is not a window.
const ExcludedDirs = new Set([path.join(FrontendRoot, "preview")]);
const WritePatterns = [
    /\.set\(\s*[\w.]*atoms\.staticTabId\b/,
    /useSetAtom\(\s*[\w.]*atoms\.staticTabId\b/,
    /useAtom\(\s*[\w.]*atoms\.staticTabId\b/,
];

function sourceFiles(dir: string): string[] {
    const rtn: string[] = [];
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
            if (!ExcludedDirs.has(full)) {
                rtn.push(...sourceFiles(full));
            }
            continue;
        }
        if (/\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)) {
            rtn.push(full);
        }
    }
    return rtn;
}

describe("staticTabIdAtom writers", () => {
    it("are limited to the builder terminal bootstrap, so main windows never change their tab", () => {
        const writers = sourceFiles(FrontendRoot)
            .filter((file) => WritePatterns.some((pattern) => pattern.test(fs.readFileSync(file, "utf8"))))
            .map((file) => path.relative(FrontendRoot, file));
        expect(writers).toEqual([path.join("builder", "store", "builder-term-model.ts")]);
    });
});
