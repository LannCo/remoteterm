// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Checks run by electron-builder.config.cjs on the inputs that `task package` stages under
// dist/. Each returns a list of problems, empty when the inputs are fine, so a missing or
// wrong-architecture input stops the packaging with a message instead of shipping an app
// whose builder only fails on the user's machine.

const fs = require("fs");
const path = require("path");

const MachoMagic64 = 0xfeedfacf;
const MachoCpu = { 0x0100000c: "arm64", 0x01000007: "x64" };
const GoArch = { arm64: "arm64", x64: "amd64" };
const OtherPlatformNative = /(android|freebsd|linux|win32|wasm32|musl|gnu)/;

function isFile(p) {
    try {
        return fs.statSync(p).isFile();
    } catch {
        return false;
    }
}

function isDir(p) {
    try {
        return fs.statSync(p).isDirectory();
    } catch {
        return false;
    }
}

function machoCpu(file) {
    const header = Buffer.alloc(8);
    const fd = fs.openSync(file, "r");
    try {
        if (fs.readSync(fd, header, 0, 8, 0) < 8) {
            return null;
        }
    } finally {
        fs.closeSync(fd);
    }
    if (header.readUInt32LE(0) !== MachoMagic64) {
        return null;
    }
    return MachoCpu[header.readUInt32LE(4)] ?? null;
}

function checkSdk(root) {
    if (isFile(path.join(root, "dist", "tsunamisdk", "go.mod"))) {
        return [];
    }
    return [
        "dist/tsunamisdk is missing, so the packaged app could not build Tsunami apps. " +
            'Run "task build:tsunamisdk" (or "task package", which does) before electron-builder.',
    ];
}

function nativeDirs(nm) {
    const found = [];
    for (const [scope, prefix] of [
        ["@tailwindcss", "oxide-"],
        ["", "lightningcss-"],
        ["@parcel", "watcher-"],
    ]) {
        const dir = path.join(nm, scope);
        if (!isDir(dir)) {
            continue;
        }
        for (const name of fs.readdirSync(dir)) {
            if (name.startsWith(prefix)) {
                found.push(path.join(scope, name));
            }
        }
    }
    return found;
}

function checkMacArch(root, arch) {
    if (!GoArch[arch]) {
        return [`unsupported macOS architecture "${arch}" (expected arm64 or x64)`];
    }
    const problems = [];
    const stage = 'Run "task package" (or "task package:macresources") before electron-builder.';

    const nm = path.join(root, "dist", `tsunamiscaffold-${arch}`, "nm");
    if (!isDir(nm)) {
        problems.push(`dist/tsunamiscaffold-${arch}/nm is missing. ${stage}`);
    } else {
        const natives = nativeDirs(nm);
        for (const needed of [`oxide-darwin-${arch}`, `lightningcss-darwin-${arch}`]) {
            if (!natives.some((n) => path.basename(n) === needed)) {
                problems.push(`dist/tsunamiscaffold-${arch}/nm has no ${needed}, so Tailwind would fail in the ${arch} app.`);
            }
        }
        for (const native of natives) {
            const name = path.basename(native);
            if (!name.includes(`darwin-${arch}`) || OtherPlatformNative.test(name)) {
                problems.push(`dist/tsunamiscaffold-${arch}/nm/${native} is a native for another platform.`);
            }
        }
    }

    const goDir = path.join(root, "dist", `gotoolchain-${arch}`);
    const goBin = path.join(goDir, "bin", "go");
    if (!isFile(goBin)) {
        problems.push(`dist/gotoolchain-${arch}/bin/go is missing. ${stage}`);
    } else if (machoCpu(goBin) !== arch) {
        problems.push(`dist/gotoolchain-${arch}/bin/go is not a ${arch} macOS executable (found ${machoCpu(goBin) ?? "something else"}).`);
    }
    if (!isFile(path.join(goDir, "pkg", "tool", `darwin_${GoArch[arch]}`, "compile"))) {
        problems.push(`dist/gotoolchain-${arch}/pkg/tool/darwin_${GoArch[arch]}/compile is missing.`);
    }

    const modCache = path.join(root, "dist", "gomodcache");
    if (!isDir(modCache) || fs.readdirSync(modCache).length === 0) {
        problems.push(`dist/gomodcache is missing or empty. ${stage}`);
    }
    return problems;
}

function failIfAny(problems) {
    if (problems.length > 0) {
        throw new Error(`Cannot package:\n  - ${problems.join("\n  - ")}`);
    }
}

module.exports = { checkSdk, checkMacArch, failIfAny, machoCpu };
