// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import fs from "fs";
import path from "path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("fetchutil", () => {
    beforeEach(() => {
        vi.resetModules();
    });

    afterEach(() => {
        vi.unstubAllGlobals();
    });

    it("uses globalThis.fetch until the main process registers Electron's net module", async () => {
        const globalFetch = vi.fn().mockResolvedValue(new Response("global"));
        vi.stubGlobal("fetch", globalFetch);
        const { fetch } = await import("./fetchutil");
        await fetch(new URL("http://localhost/a"));
        expect(globalFetch).toHaveBeenCalledWith(new URL("http://localhost/a"), undefined);
    });

    it("routes through the registered net module as a string URL", async () => {
        const globalFetch = vi.fn();
        vi.stubGlobal("fetch", globalFetch);
        const netFetch = vi.fn().mockResolvedValue(new Response("net"));
        const { fetch, setElectronNet } = await import("./fetchutil");
        setElectronNet({ fetch: netFetch });
        await fetch(new URL("http://localhost/b"), { method: "POST" });
        expect(netFetch).toHaveBeenCalledWith("http://localhost/b", { method: "POST" });
        expect(globalFetch).not.toHaveBeenCalled();
    });

    it("never imports the electron package, which would be bundled into the renderer", () => {
        const source = fs.readFileSync(path.join(__dirname, "fetchutil.ts"), "utf8");
        expect(source).not.toMatch(
            /import\s*\(\s*["']electron["']\s*\)|from\s+["']electron["']|require\(\s*["']electron["']/
        );
    });
});
