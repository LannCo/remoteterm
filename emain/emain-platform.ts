// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { fireAndForget } from "@/util/util";
import { app, dialog, ipcMain, shell } from "electron";
import envPaths from "env-paths";
import { existsSync, mkdirSync, readdirSync, renameSync, rmdirSync, writeFileSync } from "fs";
import os from "os";
import path from "path";
import { WaveDevVarName, WaveDevViteVarName } from "../frontend/util/isdev";
import * as keyutil from "../frontend/util/keyutil";

// This is a little trick to ensure that Electron puts all its runtime data into a subdirectory to avoid conflicts with our own data.
// On macOS, it will store to ~/Library/Application \Support/remoteterm/electron
// On Linux, it will store to ~/.config/remoteterm/electron
// On Windows, it will store to %LOCALAPPDATA%/remoteterm/electron
app.setName("remoteterm/electron");

const isDev = !app.isPackaged;
const isDevVite = isDev && process.env.ELECTRON_RENDERER_URL;
console.log(`Running in ${isDev ? "development" : "production"} mode`);
if (isDev) {
    process.env[WaveDevVarName] = "1";
}
if (isDevVite) {
    process.env[WaveDevViteVarName] = "1";
}

const waveDirNamePrefix = "remoteterm";
const waveDirNameSuffix = isDev ? "dev" : "";
const waveDirName = `${waveDirNamePrefix}${waveDirNameSuffix ? `-${waveDirNameSuffix}` : ""}`;

// Frozen forever: this is the real pre-v0.8 legacy directory name/prefix on disk, independent of
// whatever the product is branded as today. Never derive this from waveDirNamePrefix.
const LegacyWaveHomeDirName = ".waveterm";
const legacyWaveDirNamePrefix = "waveterm";
const legacyWaveDirName = `${legacyWaveDirNamePrefix}${waveDirNameSuffix ? `-${waveDirNameSuffix}` : ""}`;

const paths = envPaths("remoteterm", { suffix: waveDirNameSuffix });
const legacyPaths = envPaths("waveterm", { suffix: waveDirNameSuffix });

app.setName(isDev ? "RemoteTerm (Dev)" : "RemoteTerm");
const unamePlatform = process.platform;
const unameArch: string = process.arch;
keyutil.setKeyUtilPlatform(unamePlatform);

const WaveConfigHomeVarName = "REMOTETERM_CONFIG_HOME";
const LegacyWaveConfigHomeVarName = "WAVETERM_CONFIG_HOME";
const WaveDataHomeVarName = "REMOTETERM_DATA_HOME";
const LegacyWaveDataHomeVarName = "WAVETERM_DATA_HOME";
const WaveHomeVarName = "REMOTETERM_HOME";
const LegacyWaveHomeVarName = "WAVETERM_HOME";

const alreadyWarnedLegacyVars = new Set<string>();

/**
 * Reads a user-settable override var, preferring the new name but falling back to the
 * deprecated old name (with a one-time warning) so existing shell-profile overrides don't
 * silently stop working when this var is renamed.
 */
function readOverrideEnvVar(newName: string, legacyName: string): string {
    const newVal = process.env[newName];
    if (newVal) {
        return newVal;
    }
    const legacyVal = process.env[legacyName];
    if (legacyVal) {
        if (!alreadyWarnedLegacyVars.has(legacyName)) {
            alreadyWarnedLegacyVars.add(legacyName);
            console.log(`${legacyName} is deprecated, please use ${newName} instead`);
        }
        return legacyVal;
    }
    return null;
}

/**
 * One-time, synchronous local data-dir migration from the old "waveterm"-prefixed paths to the
 * new "remoteterm"-prefixed paths. Must run as a top-level statement in this module (not
 * exported/called from elsewhere) so it completes before any importer of this module's getters
 * (getWaveConfigDir/getWaveDataDir) can call them and side-effect-create the new directories
 * first. See RENAME_PLAN.md Phase 2 step 7 for why this exact placement is required.
 */
type MigrationRootSpec = {
    name: string;
    source: string;
    dest: string;
    overridden: boolean;
    validateSource?: () => boolean;
};

const MigrationMarkerFileName = ".migrated-from-waveterm";

function migrateDataRoot(spec: MigrationRootSpec) {
    const markerFile = path.join(spec.dest, MigrationMarkerFileName);
    if (spec.overridden) {
        if (existsSync(markerFile)) {
            return;
        }
        try {
            mkdirSync(spec.dest, { recursive: true });
            writeFileSync(markerFile, `no-migration-needed:override\n${new Date().toISOString()}\n`);
        } catch (e) {
            console.log(`[migration] failed to write override marker for ${spec.name} root:`, e);
        }
        return;
    }
    if (existsSync(markerFile)) {
        return;
    }
    if (!existsSync(spec.source) || (spec.validateSource && !spec.validateSource())) {
        return;
    }
    if (existsSync(spec.dest)) {
        let destEntries: string[];
        try {
            destEntries = readdirSync(spec.dest);
        } catch (e) {
            console.log(`[migration] could not inspect existing destination ${spec.dest} for ${spec.name} root:`, e);
            return;
        }
        if (destEntries.length === 0) {
            try {
                rmdirSync(spec.dest);
            } catch (e) {
                console.log(`[migration] could not remove empty destination ${spec.dest} for ${spec.name} root:`, e);
                return;
            }
        } else {
            console.error(
                `[migration] ${spec.name} root migration aborted: ${spec.dest} already exists and is not empty. Please merge ${spec.source} into ${spec.dest} manually.`
            );
            return;
        }
    }
    try {
        mkdirSync(path.dirname(spec.dest), { recursive: true });
        renameSync(spec.source, spec.dest);
        writeFileSync(path.join(spec.dest, MigrationMarkerFileName), `moved-from:${spec.source}\n${new Date().toISOString()}\n`);
        console.log(`[migration] migrated ${spec.name} root from ${spec.source} to ${spec.dest}`);
    } catch (e) {
        if (e && e.code === "ENOENT") {
            if (existsSync(markerFile)) {
                // another process already completed this root's migration
                return;
            }
            console.log(
                `[migration] ${spec.name} root move failed with ENOENT and no completion marker was found (source: ${spec.source}):`,
                e
            );
            return;
        }
        console.log(`[migration] error migrating ${spec.name} root from ${spec.source} to ${spec.dest}:`, e);
    }
}

function performDataDirMigration() {
    try {
        const homeDir = app.getPath("home");
        const xdgConfigHome = process.env.XDG_CONFIG_HOME;
        const xdgDataHome = process.env.XDG_DATA_HOME;

        const configOverride = readOverrideEnvVar(WaveConfigHomeVarName, LegacyWaveConfigHomeVarName);
        const configSource = xdgConfigHome
            ? path.join(xdgConfigHome, legacyWaveDirName)
            : path.join(homeDir, ".config", legacyWaveDirName);
        const configDest = xdgConfigHome
            ? path.join(xdgConfigHome, waveDirName)
            : path.join(homeDir, ".config", waveDirName);
        migrateDataRoot({
            name: "config",
            source: configOverride ?? configSource,
            dest: configOverride ?? configDest,
            overridden: configOverride != null,
            validateSource: () => existsSync(path.join(configSource, "settings.json")),
        });

        const dataOverride = readOverrideEnvVar(WaveDataHomeVarName, LegacyWaveDataHomeVarName);
        const dataSource = xdgDataHome ? path.join(xdgDataHome, legacyWaveDirName) : legacyPaths.data;
        const dataDest = xdgDataHome ? path.join(xdgDataHome, waveDirName) : paths.data;
        migrateDataRoot({
            name: "data",
            source: dataOverride ?? dataSource,
            dest: dataOverride ?? dataDest,
            overridden: dataOverride != null,
            validateSource: () => existsSync(path.join(dataSource, "wave.lock")),
        });

        const homeOverride = readOverrideEnvVar(WaveHomeVarName, LegacyWaveHomeVarName);
        const legacyHomeSource = path.join(homeDir, LegacyWaveHomeDirName);
        const legacyHomeDest = path.join(homeDir, `.${waveDirName}`);
        migrateDataRoot({
            name: "legacy-home",
            source: homeOverride ?? legacyHomeSource,
            dest: homeOverride ?? legacyHomeDest,
            overridden: homeOverride != null,
            validateSource: () => existsSync(path.join(legacyHomeSource, "wave.lock")),
        });

        // Best-effort fourth root: on Windows, Electron's own userData subtree (cookies, cache,
        // window state) resolves under %APPDATA% as a *sibling* of the config root, not a child
        // of any of the three roots above, so it needs its own explicit move. Unlike the roots
        // above, there is no per-root override var and no natural marker file inside Electron's
        // userData dir to validate against, so we validate on directory existence alone.
        if (process.platform === "win32" && process.env.APPDATA) {
            const winSource = path.join(process.env.APPDATA, "waveterm", "electron");
            const winDest = path.join(process.env.APPDATA, "remoteterm", "electron");
            migrateDataRoot({
                name: "windows-userdata",
                source: winSource,
                dest: winDest,
                overridden: false,
                validateSource: () => existsSync(winSource),
            });
        }
    } catch (e) {
        console.log("[migration] unexpected error during data-dir migration, continuing startup:", e);
    }
}

performDataDirMigration();

export function checkIfRunningUnderARM64Translation(fullConfig: FullConfigType) {
    if (!fullConfig.settings["app:dismissarchitecturewarning"] && app.runningUnderARM64Translation) {
        console.log("Running under ARM64 translation, alerting user");
        const dialogOpts: Electron.MessageBoxOptions = {
            type: "warning",
            buttons: ["Dismiss", "Learn More"],
            title: "RemoteTerm has detected a performance issue",
            message: `RemoteTerm is running in ARM64 translation mode which may impact performance.\n\nRecommendation: Download the native ARM64 version from our website for optimal performance.`,
        };

        const choice = dialog.showMessageBoxSync(null, dialogOpts);
        if (choice === 1) {
            // Open the documentation URL
            console.log("User chose to learn more");
            fireAndForget(() =>
                shell.openExternal(
                    "https://docs.waveterm.dev/faq#why-does-wave-warn-me-about-arm64-translation-when-it-launches"
                )
            );
            throw new Error("User redirected to docsite to learn more about ARM64 translation, exiting");
        } else {
            console.log("User dismissed the dialog");
        }
    }
}

/**
 * Gets the path to the combined Wave home directory (defaults to `~/.remoteterm`, falling back
 * to the frozen pre-v0.8 legacy path `~/.waveterm` if that's what has valid data).
 * @returns The path to the directory if it exists and contains valid data for the current app, otherwise null.
 */
function getWaveHomeDir(): string {
    let home = readOverrideEnvVar(WaveHomeVarName, LegacyWaveHomeVarName);
    if (!home) {
        const homeDir = app.getPath("home");
        if (homeDir) {
            // Check the current (post-migration) default combined-home location first, then
            // fall back to the frozen pre-v0.8 legacy location. The migration shim above moves
            // a valid legacy home dir from the latter to the former, but this function may be
            // called before that migration has a chance to run for a given process, or the
            // migration may have been skipped/failed, so both locations must be checked.
            const migratedHome = path.join(homeDir, `.${waveDirName}`);
            if (existsSync(migratedHome) && existsSync(path.join(migratedHome, "wave.lock"))) {
                return migratedHome;
            }
            home = path.join(homeDir, LegacyWaveHomeDirName);
        }
    }
    // If home exists and it has `wave.lock` in it, we know it has valid data from Wave >=v0.8. Otherwise, it could be for WaveLegacy (<v0.8)
    if (home && existsSync(home) && existsSync(path.join(home, "wave.lock"))) {
        return home;
    }
    return null;
}

/**
 * Ensure the given path exists, creating it recursively if it doesn't.
 * @param path The path to ensure.
 * @returns The same path, for chaining.
 */
function ensurePathExists(path: string): string {
    if (!existsSync(path)) {
        mkdirSync(path, { recursive: true });
    }
    return path;
}

/**
 * Gets the path to the directory where Wave configurations are stored. Creates the directory if it does not exist.
 * Handles backwards compatibility with the old Wave Home directory model, where configurations and data were stored together.
 * @returns The path where configurations should be stored.
 */
function getWaveConfigDir(): string {
    // If wave home dir exists, use it for backwards compatibility
    const waveHomeDir = getWaveHomeDir();
    if (waveHomeDir) {
        return path.join(waveHomeDir, "config");
    }

    const override = readOverrideEnvVar(WaveConfigHomeVarName, LegacyWaveConfigHomeVarName);
    const xdgConfigHome = process.env.XDG_CONFIG_HOME;
    let retVal: string;
    if (override) {
        retVal = override;
    } else if (xdgConfigHome) {
        retVal = path.join(xdgConfigHome, waveDirName);
    } else {
        retVal = path.join(app.getPath("home"), ".config", waveDirName);
    }
    return ensurePathExists(retVal);
}

/**
 * Gets the path to the directory where Wave data is stored. Creates the directory if it does not exist.
 * Handles backwards compatibility with the old Wave Home directory model, where configurations and data were stored together.
 * @returns The path where data should be stored.
 */
function getWaveDataDir(): string {
    // If wave home dir exists, use it for backwards compatibility
    const waveHomeDir = getWaveHomeDir();
    if (waveHomeDir) {
        return waveHomeDir;
    }

    const override = readOverrideEnvVar(WaveDataHomeVarName, LegacyWaveDataHomeVarName);
    const xdgDataHome = process.env.XDG_DATA_HOME;
    let retVal: string;
    if (override) {
        retVal = override;
    } else if (xdgDataHome) {
        retVal = path.join(xdgDataHome, waveDirName);
    } else {
        retVal = paths.data;
    }
    return ensurePathExists(retVal);
}

function getElectronAppBasePath(): string {
    // import.meta.dirname in dev points to waveterm/dist/main
    return path.dirname(import.meta.dirname);
}

function getElectronAppUnpackedBasePath(): string {
    return getElectronAppBasePath().replace("app.asar", "app.asar.unpacked");
}

function getElectronAppResourcesPath(): string {
    if (isDev) {
        // import.meta.dirname in dev points to waveterm/dist/main
        return path.dirname(import.meta.dirname);
    }
    return process.resourcesPath;
}

const wavesrvBinName = `wavesrv.${unameArch}`;

function getWaveSrvPath(): string {
    if (process.platform === "win32") {
        const winBinName = `${wavesrvBinName}.exe`;
        const appPath = path.join(getElectronAppUnpackedBasePath(), "bin", winBinName);
        return `${appPath}`;
    }
    return path.join(getElectronAppUnpackedBasePath(), "bin", wavesrvBinName);
}

function getWaveSrvCwd(): string {
    return getWaveDataDir();
}

ipcMain.on("get-is-dev", (event) => {
    event.returnValue = isDev;
});
ipcMain.on("get-platform", (event, url) => {
    event.returnValue = unamePlatform;
});
ipcMain.on("get-user-name", (event) => {
    const userInfo = os.userInfo();
    event.returnValue = userInfo.username;
});
ipcMain.on("get-host-name", (event) => {
    event.returnValue = os.hostname();
});
ipcMain.on("get-webview-preload", (event) => {
    event.returnValue = path.join(getElectronAppBasePath(), "preload", "preload-webview.cjs");
});
ipcMain.on("get-data-dir", (event) => {
    event.returnValue = getWaveDataDir();
});
ipcMain.on("get-config-dir", (event) => {
    event.returnValue = getWaveConfigDir();
});
ipcMain.on("get-home-dir", (event) => {
    event.returnValue = app.getPath("home");
});

/**
 * Gets the value of the XDG_CURRENT_DESKTOP environment variable. If ORIGINAL_XDG_CURRENT_DESKTOP is set, it will be returned instead.
 * This corrects for a strange behavior in Electron, where it sets its own value for XDG_CURRENT_DESKTOP to improve Chromium compatibility.
 * @see https://www.electronjs.org/docs/latest/api/environment-variables#original_xdg_current_desktop
 * @returns The value of the XDG_CURRENT_DESKTOP environment variable, or ORIGINAL_XDG_CURRENT_DESKTOP if set, or undefined if neither are set.
 */
function getXdgCurrentDesktop(): string {
    if (process.env.ORIGINAL_XDG_CURRENT_DESKTOP) {
        return process.env.ORIGINAL_XDG_CURRENT_DESKTOP;
    } else if (process.env.XDG_CURRENT_DESKTOP) {
        return process.env.XDG_CURRENT_DESKTOP;
    } else {
        return undefined;
    }
}

/**
 * Calls the given callback with the value of the XDG_CURRENT_DESKTOP environment variable set to ORIGINAL_XDG_CURRENT_DESKTOP if it is set.
 * @see https://www.electronjs.org/docs/latest/api/environment-variables#original_xdg_current_desktop
 * @param callback The callback to call.
 */
function callWithOriginalXdgCurrentDesktop(callback: () => void) {
    const currXdgCurrentDesktopDefined = "XDG_CURRENT_DESKTOP" in process.env;
    const currXdgCurrentDesktop = process.env.XDG_CURRENT_DESKTOP;
    const originalXdgCurrentDesktop = getXdgCurrentDesktop();
    if (originalXdgCurrentDesktop) {
        process.env.XDG_CURRENT_DESKTOP = originalXdgCurrentDesktop;
    }
    callback();
    if (originalXdgCurrentDesktop) {
        if (currXdgCurrentDesktopDefined) {
            process.env.XDG_CURRENT_DESKTOP = currXdgCurrentDesktop;
        } else {
            delete process.env.XDG_CURRENT_DESKTOP;
        }
    }
}

/**
 * Calls the given async callback with the value of the XDG_CURRENT_DESKTOP environment variable set to ORIGINAL_XDG_CURRENT_DESKTOP if it is set.
 * @see https://www.electronjs.org/docs/latest/api/environment-variables#original_xdg_current_desktop
 * @param callback The async callback to call.
 */
async function callWithOriginalXdgCurrentDesktopAsync(callback: () => Promise<void>) {
    const currXdgCurrentDesktopDefined = "XDG_CURRENT_DESKTOP" in process.env;
    const currXdgCurrentDesktop = process.env.XDG_CURRENT_DESKTOP;
    const originalXdgCurrentDesktop = getXdgCurrentDesktop();
    if (originalXdgCurrentDesktop) {
        process.env.XDG_CURRENT_DESKTOP = originalXdgCurrentDesktop;
    }
    await callback();
    if (originalXdgCurrentDesktop) {
        if (currXdgCurrentDesktopDefined) {
            process.env.XDG_CURRENT_DESKTOP = currXdgCurrentDesktop;
        } else {
            delete process.env.XDG_CURRENT_DESKTOP;
        }
    }
}

export {
    callWithOriginalXdgCurrentDesktop,
    callWithOriginalXdgCurrentDesktopAsync,
    getElectronAppBasePath,
    getElectronAppResourcesPath,
    getElectronAppUnpackedBasePath,
    getWaveConfigDir,
    getWaveDataDir,
    getWaveSrvCwd,
    getWaveSrvPath,
    getXdgCurrentDesktop,
    isDev,
    isDevVite,
    unameArch,
    unamePlatform,
    WaveConfigHomeVarName,
    WaveDataHomeVarName,
};
