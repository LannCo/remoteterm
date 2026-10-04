// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as fs from "fs";
import { createRequire } from "module";
import * as path from "path";
import { describe, expect, it } from "vitest";

const RepoRoot = process.cwd();

function taskBlock(taskfile: string, taskName: string): string {
    const start = taskfile.indexOf(`\n    ${taskName}:\n`);
    if (start < 0) {
        return "";
    }
    const rest = taskfile.slice(start + 1);
    const next = rest.slice(1).search(/\n    [a-z][^\n]*:\n/);
    return next < 0 ? rest : rest.slice(0, next + 1);
}

describe("tsunami SDK packaging", () => {
    it("ships dist/tsunamisdk as an extra resource and keeps it out of the asar", () => {
        const require = createRequire(import.meta.url);
        const config = require(path.join(RepoRoot, "electron-builder.config.cjs"));
        expect(config.extraResources).toContainEqual({ from: "dist/tsunamisdk", to: "tsunamisdk" });
        const distEntry = config.files.find((entry: any) => entry?.from === "./dist");
        expect(distEntry.filter).toContain("!tsunamisdk/**/*");
    });

    it("builds the SDK bundle wherever the scaffold is built", () => {
        const taskfile = fs.readFileSync(path.join(RepoRoot, "Taskfile.yml"), "utf8");
        expect(taskBlock(taskfile, "build:tsunamisdk")).toContain("sdkbundle");
        for (const name of ["electron:dev", "package"]) {
            const block = taskBlock(taskfile, name);
            expect(block).toContain("- build:tsunamiscaffold");
            expect(block).toContain("- build:tsunamisdk");
        }
    });
});
