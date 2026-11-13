// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
//
// Stage the remotetermsrv binary as a proper macOS helper-app bundle
// ("RemoteTerm Helper (server).app") inside the packaged app.
//
// Why: macOS 15+ gates connections to on-link (local network) hosts behind a
// per-app permission. The gate attributes traffic by code identity; a bare
// Go binary in app.asar.unpacked has no bundle identity (nehelper logs
// "bundle_id: (null)" and blocks silently, with no prompt). Wrapping the
// binary in a bundle with its own CFBundleIdentifier and
// NSLocalNetworkUsageDescription gives the gate something to attribute,
// prompt for, and persist.
//
// usage (called from electron-builder afterPack):
//   stageServerHelper(context, archName)  // archName: "arm64" | "x64"

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const HelperBundleName = "RemoteTerm Helper (server).app";
const HelperIdentifier = "dev.remoteterm.app.server";

function stageServerHelper(context, archName) {
    const appDir = path.join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`);
    const srcBin = path.join(
        appDir,
        "Contents",
        "Resources",
        "app.asar.unpacked",
        "dist",
        "bin",
        `remotetermsrv.${archName}`
    );
    if (!fs.existsSync(srcBin)) {
        throw new Error(`stage-server-helper: ${srcBin} is missing; the backend build did not produce it`);
    }

    const bundleDir = path.join(appDir, "Contents", "Frameworks", HelperBundleName);
    const macOsDir = path.join(bundleDir, "Contents", "MacOS");
    fs.mkdirSync(macOsDir, { recursive: true });

    const dstBin = path.join(macOsDir, "remotetermsrv");
    fs.copyFileSync(srcBin, dstBin);
    fs.chmodSync(dstBin, 0o755);

    const plistTemplate = path.join(__dirname, "server-helper-Info.plist");
    const plist = fs.readFileSync(plistTemplate, "utf8").replaceAll("@VERSION@", context.packager.appInfo.version);
    fs.writeFileSync(path.join(bundleDir, "Contents", "Info.plist"), plist);

    // Ad-hoc signing gives the helper a stable code identity so LaunchServices
    // resolves it to the bundle. With a real signing identity (CSC_NAME), use
    // it — electron-builder's own signing pass does not know about this bundle.
    const identity = process.env.CSC_NAME || "-";
    execFileSync("codesign", ["--force", "--sign", identity, "--timestamp=none", bundleDir], { stdio: "inherit" });
    // The bare fallback binary (dev/unpackaged runs) also gets a stable
    // identifier so even bundle-less runs carry a resolvable code identity.
    execFileSync("codesign", ["--force", "--sign", identity, "--timestamp=none", "--identifier", HelperIdentifier, srcBin], {
        stdio: "inherit",
    });

    console.log(`stage-server-helper: staged ${bundleDir}`);
}

module.exports = { stageServerHelper, HelperBundleName, HelperIdentifier };