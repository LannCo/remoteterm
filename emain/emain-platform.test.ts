// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import fs from "fs";
import os from "os";
import path from "path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const OverrideVarNames = [
    "REMOTETERM_CONFIG_HOME",
    "WAVETERM_CONFIG_HOME",
    "REMOTETERM_DATA_HOME",
    "WAVETERM_DATA_HOME",
    "REMOTETERM_HOME",
    "WAVETERM_HOME",
];
const MarkerFileName = ".migrated-from-waveterm";

let tmpHome: string;
let xdgConfig: string;
let xdgData: string;
const savedEnv = { ...process.env };

function mockElectron(isPackaged: boolean) {
    vi.doMock("electron", () => ({
        app: {
            isPackaged,
            getPath: (name: string) => {
                if (name !== "home") {
                    throw new Error(`unexpected getPath(${name})`);
                }
                return tmpHome;
            },
            setName: () => {},
            runningUnderARM64Translation: false,
        },
        dialog: { showMessageBoxSync: () => 0 },
        ipcMain: { on: () => {} },
        shell: { openExternal: async () => {} },
    }));
}

async function loadPlatform(isPackaged: boolean) {
    mockElectron(isPackaged);
    return await import("./emain-platform");
}

function makeDir(dir: string, files: Record<string, string> = {}) {
    fs.mkdirSync(dir, { recursive: true });
    for (const [name, content] of Object.entries(files)) {
        fs.mkdirSync(path.dirname(path.join(dir, name)), { recursive: true });
        fs.writeFileSync(path.join(dir, name), content);
    }
    return dir;
}

beforeEach(() => {
    vi.resetModules();
    tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), "rt-emain-platform-"));
    xdgConfig = path.join(tmpHome, ".config");
    xdgData = path.join(tmpHome, ".local", "share");
    process.env.HOME = tmpHome;
    process.env.XDG_CONFIG_HOME = xdgConfig;
    process.env.XDG_DATA_HOME = xdgData;
    for (const name of OverrideVarNames) {
        delete process.env[name];
    }
    vi.spyOn(console, "log").mockImplementation(() => {});
    vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.doUnmock("electron");
    vi.doUnmock("fs");
    process.env = { ...savedEnv };
    fs.rmSync(tmpHome, { recursive: true, force: true });
});

describe("combined-home fallback", () => {
    it("dev build ignores the production ~/.waveterm", async () => {
        const prodHome = makeDir(path.join(tmpHome, ".waveterm"), { "wave.lock": "" });
        const mod = await loadPlatform(false);
        expect(mod.getRemoteTermDataDir()).not.toBe(prodHome);
        expect(mod.getRemoteTermDataDir()).toBe(path.join(xdgData, "remoteterm-dev"));
        expect(fs.existsSync(path.join(prodHome, "wave.lock"))).toBe(true);
    });

    it("dev build migrates and uses ~/.waveterm-dev", async () => {
        makeDir(path.join(tmpHome, ".waveterm-dev"), { "wave.lock": "", "db/x": "1" });
        const mod = await loadPlatform(false);
        const devHome = path.join(tmpHome, ".remoteterm-dev");
        expect(mod.getRemoteTermDataDir()).toBe(devHome);
        expect(fs.existsSync(path.join(devHome, "db/x"))).toBe(true);
    });

    it("production build migrates and uses ~/.waveterm", async () => {
        makeDir(path.join(tmpHome, ".waveterm"), { "wave.lock": "" });
        const mod = await loadPlatform(true);
        expect(mod.getRemoteTermDataDir()).toBe(path.join(tmpHome, ".remoteterm"));
        expect(mod.getRemoteTermConfigDir()).toBe(path.join(tmpHome, ".remoteterm", "config"));
    });
});
