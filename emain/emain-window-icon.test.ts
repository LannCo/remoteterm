// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import fs from "fs";
import path from "path";
import { expect, test } from "vitest";

const RepoRoot = path.resolve(import.meta.dirname, "..");

function readEmain(name: string): string {
    return fs.readFileSync(path.join(RepoRoot, "emain", name), "utf-8");
}

// The Linux window icon used to be a getElectronAppBasePath()-relative path to
// dist/public/logos, which the build never produces, so windows got no icon. An
// electron-vite `?asset` import is emitted next to the main bundle in dev and packaged
// builds alike, so the path it resolves to always exists.
test("Linux window icon is a ?asset import of the RemoteTerm icon render", () => {
    const src = readEmain("emain-window.ts");
    const match = src.match(/^import \w+ from "([^"]+\.png)\?asset";$/m);
    expect(match, "no ?asset png import in emain-window.ts").not.toBeNull();
    const iconPath = path.resolve(RepoRoot, "emain", match[1]);
    expect(path.relative(RepoRoot, iconPath)).toMatch(/^build\/icons\/\d+x\d+\.png$/);
    expect(fs.existsSync(iconPath)).toBe(true);
});

test("no window points at the unbuilt Wave logo path", () => {
    for (const name of ["emain-window.ts", "emain-builder.ts"]) {
        const src = readEmain(name);
        expect(src, name).not.toMatch(/public\/logos/);
        expect(src, name).not.toMatch(/wave-logo/);
    }
});
