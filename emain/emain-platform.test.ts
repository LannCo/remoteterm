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

function loggedLines(): string[] {
    const logSpy = vi.mocked(console.log);
    const errSpy = vi.mocked(console.error);
    return [...logSpy.mock.calls, ...errSpy.mock.calls].map((c) => String(c[0]));
}

describe("legacy config root validation", () => {
    it.each([["connections.json"], ["widgets.json"], ["presets/ai.json"]])(
        "migrates a root holding only %s",
        async (fileName) => {
            const legacy = makeDir(path.join(xdgConfig, "waveterm"), { [fileName]: "{}" });
            const mod = await loadPlatform(true);
            const configDir = mod.getRemoteTermConfigDir();
            expect(configDir).toBe(path.join(xdgConfig, "remoteterm"));
            expect(fs.existsSync(path.join(configDir, fileName))).toBe(true);
            expect(fs.existsSync(path.join(configDir, MarkerFileName))).toBe(true);
            expect(fs.existsSync(legacy)).toBe(false);
            expect(mod.getMigrationFailures()).toEqual([]);
        }
    );

    it("moves Electron userData along with a valid config root", async () => {
        makeDir(path.join(xdgConfig, "waveterm"), { "settings.json": "{}", "electron/Preferences": "x" });
        const mod = await loadPlatform(true);
        expect(fs.existsSync(path.join(mod.getRemoteTermConfigDir(), "electron", "Preferences"))).toBe(true);
    });

    it("skips an Electron-only root and logs why", async () => {
        const legacy = makeDir(path.join(xdgConfig, "waveterm"), { "electron/Preferences": "x" });
        const mod = await loadPlatform(true);
        expect(fs.existsSync(path.join(legacy, "electron", "Preferences"))).toBe(true);
        expect(mod.getMigrationFailures()).toEqual([]);
        const skipLines = loggedLines().filter((s) => s.includes("[migration]") && s.includes("skipping config root"));
        expect(skipLines).toHaveLength(1);
        expect(skipLines[0]).toContain(legacy);
        expect(skipLines[0]).toMatch(/no config files/);
    });

    it("logs a skip when the legacy root is absent", async () => {
        await loadPlatform(true);
        const skipLines = loggedLines().filter((s) => s.includes("[migration]") && s.includes("skipping"));
        expect(skipLines.some((s) => s.includes("config root") && s.includes("does not exist"))).toBe(true);
        expect(skipLines.some((s) => s.includes("data root") && s.includes("does not exist"))).toBe(true);
    });
});

describe("data-dir migration shim", () => {
    const legacyData = () => path.join(xdgData, "waveterm");
    const newData = () => path.join(xdgData, "remoteterm");

    it("writes the marker after the move and does not migrate again", async () => {
        makeDir(legacyData(), { "wave.lock": "", "db/remoteterm.db": "first" });
        let mod = await loadPlatform(true);
        expect(fs.readFileSync(path.join(newData(), "db/remoteterm.db"), "utf8")).toBe("first");
        const marker = fs.readFileSync(path.join(newData(), MarkerFileName), "utf8");
        expect(marker).toMatch(new RegExp(`^moved-from:${legacyData()}\n`));
        expect(fs.existsSync(legacyData())).toBe(false);
        expect(mod.getMigrationFailures()).toEqual([]);

        makeDir(legacyData(), { "wave.lock": "", "db/remoteterm.db": "second" });
        vi.resetModules();
        mod = await loadPlatform(true);
        expect(fs.readFileSync(path.join(newData(), "db/remoteterm.db"), "utf8")).toBe("first");
        expect(fs.existsSync(path.join(legacyData(), "db/remoteterm.db"))).toBe(true);
        expect(mod.getMigrationFailures()).toEqual([]);
    });

    it.each([["REMOTETERM_DATA_HOME"], ["WAVETERM_DATA_HOME"]])(
        "%s override short-circuits the data root without moving legacy data",
        async (varName) => {
            makeDir(legacyData(), { "wave.lock": "" });
            const override = path.join(tmpHome, "custom-data");
            process.env[varName] = override;
            let mod = await loadPlatform(true);
            const markerFile = path.join(override, MarkerFileName);
            expect(fs.readFileSync(markerFile, "utf8")).toMatch(/^no-migration-needed:override\n/);
            expect(fs.existsSync(path.join(legacyData(), "wave.lock"))).toBe(true);
            expect(fs.existsSync(newData())).toBe(false);
            expect(mod.getRemoteTermDataDir()).toBe(override);

            fs.writeFileSync(markerFile, "sentinel");
            vi.resetModules();
            mod = await loadPlatform(true);
            expect(fs.readFileSync(markerFile, "utf8")).toBe("sentinel");
            expect(mod.getMigrationFailures()).toEqual([]);
        }
    );

    it("replaces an empty destination", async () => {
        makeDir(legacyData(), { "wave.lock": "" });
        makeDir(newData());
        const mod = await loadPlatform(true);
        expect(fs.existsSync(path.join(newData(), "wave.lock"))).toBe(true);
        expect(mod.getMigrationFailures()).toEqual([]);
    });

    it("aborts and reports when the destination is not empty", async () => {
        makeDir(legacyData(), { "wave.lock": "" });
        makeDir(newData(), { "remoteterm.lock": "" });
        const mod = await loadPlatform(true);
        expect(fs.existsSync(path.join(legacyData(), "wave.lock"))).toBe(true);
        expect(fs.existsSync(path.join(newData(), MarkerFileName))).toBe(false);
        expect(mod.getMigrationFailures()).toHaveLength(1);
        expect(mod.getMigrationFailures()[0]).toMatch(/data root migration aborted/);
    });

    describe("ENOENT from rename", () => {
        async function loadWithRacingRename(otherProcessWritesMarker: boolean) {
            const actualFs = await vi.importActual<typeof import("fs")>("fs");
            vi.doMock("fs", () => ({
                ...actualFs,
                renameSync: (src: string, dest: string) => {
                    if (src === legacyData()) {
                        actualFs.renameSync(src, otherProcessWritesMarker ? dest : path.join(tmpHome, "elsewhere"));
                        if (otherProcessWritesMarker) {
                            actualFs.writeFileSync(path.join(dest, MarkerFileName), "moved-by-other-process\n");
                        }
                        throw Object.assign(new Error(`ENOENT: rename '${src}'`), { code: "ENOENT" });
                    }
                    return actualFs.renameSync(src, dest);
                },
            }));
            return await loadPlatform(true);
        }

        it("treats the move as done when another process left the marker", async () => {
            makeDir(legacyData(), { "wave.lock": "" });
            const mod = await loadWithRacingRename(true);
            expect(mod.getMigrationFailures()).toEqual([]);
            expect(fs.readFileSync(path.join(newData(), MarkerFileName), "utf8")).toBe("moved-by-other-process\n");
        });

        it("reports a failure when no marker was left", async () => {
            makeDir(legacyData(), { "wave.lock": "" });
            const mod = await loadWithRacingRename(false);
            expect(mod.getMigrationFailures()).toHaveLength(1);
            expect(mod.getMigrationFailures()[0]).toMatch(/ENOENT and no completion marker/);
        });
    });
});
