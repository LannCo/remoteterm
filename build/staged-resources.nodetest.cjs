// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Run with: node --test build/staged-resources.nodetest.cjs
// (not named *.test.* so vitest does not try to collect it)

const test = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { checkSdk, checkMacArch, failIfAny, machoCpu } = require("./staged-resources.cjs");

function macho(cpu) {
    const header = Buffer.alloc(32);
    header.writeUInt32LE(0xfeedfacf, 0);
    header.writeUInt32LE(cpu, 4);
    header.writeUInt32LE(2, 12);
    return header;
}
const CpuArm64 = 0x0100000c;
const CpuX64 = 0x01000007;

function put(root, rel, content = "x", mode = 0o644) {
    const p = path.join(root, rel);
    fs.mkdirSync(path.dirname(p), { recursive: true });
    fs.writeFileSync(p, content, { mode });
}

function stage(root, arch, { goCpu, natives }) {
    const goarch = arch === "x64" ? "amd64" : "arm64";
    put(root, `dist/gotoolchain-${arch}/bin/go`, macho(goCpu), 0o755);
    put(root, `dist/gotoolchain-${arch}/pkg/tool/darwin_${goarch}/compile`, "x", 0o755);
    put(root, "dist/gomodcache/github.com/google/uuid/@v/v1.6.0.zip");
    for (const n of natives) {
        put(root, `dist/tsunamiscaffold-${arch}/nm/${n}/package.json`);
    }
}

const goodNatives = (arch) => [`@tailwindcss/oxide-darwin-${arch}`, `lightningcss-darwin-${arch}`, `@parcel/watcher-darwin-${arch}`];

test("a complete arm64 and x64 staging passes", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "staged-"));
    stage(root, "arm64", { goCpu: CpuArm64, natives: goodNatives("arm64") });
    stage(root, "x64", { goCpu: CpuX64, natives: goodNatives("x64") });
    assert.deepStrictEqual(checkMacArch(root, "arm64"), []);
    assert.deepStrictEqual(checkMacArch(root, "x64"), []);
});

test("the other architecture's go binary is rejected", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "staged-"));
    stage(root, "x64", { goCpu: CpuArm64, natives: goodNatives("x64") });
    const problems = checkMacArch(root, "x64");
    assert.ok(problems.some((p) => p.includes("not a x64 macOS executable")), problems.join("\n"));
});

test("natives for another architecture or platform are rejected", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "staged-"));
    stage(root, "arm64", { goCpu: CpuArm64, natives: [...goodNatives("arm64"), "@tailwindcss/oxide-linux-x64-gnu"] });
    const problems = checkMacArch(root, "arm64");
    assert.ok(problems.some((p) => p.includes("oxide-linux-x64-gnu")), problems.join("\n"));

    const swapped = fs.mkdtempSync(path.join(os.tmpdir(), "staged-"));
    stage(swapped, "arm64", { goCpu: CpuArm64, natives: goodNatives("x64") });
    assert.ok(checkMacArch(swapped, "arm64").length >= 2);
});

test("missing pieces are each named", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "staged-"));
    const problems = checkMacArch(root, "arm64");
    for (const part of ["tsunamiscaffold-arm64/nm", "gotoolchain-arm64/bin/go", "gomodcache"]) {
        assert.ok(problems.some((p) => p.includes(part)), `${part} not reported:\n${problems.join("\n")}`);
    }
    assert.throws(() => failIfAny(problems), /Cannot package/);
});

test("the SDK check names the task that builds it", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "staged-"));
    assert.match(checkSdk(root)[0], /task build:tsunamisdk/);
    put(root, "dist/tsunamisdk/go.mod");
    assert.deepStrictEqual(checkSdk(root), []);
});

test("machoCpu reads the CPU and refuses scripts", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "staged-"));
    put(root, "a", macho(CpuArm64));
    put(root, "b", "#!/bin/sh\necho hi\n");
    assert.strictEqual(machoCpu(path.join(root, "a")), "arm64");
    assert.strictEqual(machoCpu(path.join(root, "b")), null);
});
