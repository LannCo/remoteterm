// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A 64-bit Mach-O header with no load commands: enough for debug/macho to read the CPU.
func machoHeader(cpu uint32) []byte {
	var buf bytes.Buffer
	for _, v := range []uint32{0xfeedfacf, cpu, 0, 2, 0, 0, 0, 0} {
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	return buf.Bytes()
}

const (
	machoCpuArm64 = 0x0100000c
	machoCpuAmd64 = 0x01000007
)

func darwinToolchainFixture(t *testing.T, goarch string, cpu uint32) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "go"), machoHeader(cpu), 0755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(root, "pkg", "tool", "darwin_"+goarch, "compile"), "x")
	return root
}

func TestVerifyToolchainArchAcceptsTheMatchingMachO(t *testing.T) {
	if err := VerifyToolchainArch(darwinToolchainFixture(t, "arm64", machoCpuArm64), "darwin", "arm64"); err != nil {
		t.Errorf("arm64: %v", err)
	}
	if err := VerifyToolchainArch(darwinToolchainFixture(t, "amd64", machoCpuAmd64), "darwin", "amd64"); err != nil {
		t.Errorf("amd64: %v", err)
	}
}

func TestVerifyToolchainArchRejectsTheOtherArchitecture(t *testing.T) {
	err := VerifyToolchainArch(darwinToolchainFixture(t, "arm64", machoCpuAmd64), "darwin", "arm64")
	if err == nil || !strings.Contains(err.Error(), "not arm64") {
		t.Errorf("an x86_64 go in the arm64 bundle must be rejected, got %v", err)
	}
	err = VerifyToolchainArch(darwinToolchainFixture(t, "amd64", machoCpuArm64), "darwin", "amd64")
	if err == nil || !strings.Contains(err.Error(), "not amd64") {
		t.Errorf("an arm64 go in the x64 bundle must be rejected, got %v", err)
	}
}

func TestVerifyToolchainArchRejectsAScriptAndAMissingCompiler(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, filepath.Join(root, "bin", "go"), "#!/bin/sh\n")
	if err := VerifyToolchainArch(root, "darwin", "arm64"); err == nil {
		t.Error("a shell script was accepted as the darwin go binary")
	}
	noCompiler := darwinToolchainFixture(t, "arm64", machoCpuArm64)
	if err := os.Remove(filepath.Join(noCompiler, "pkg", "tool", "darwin_arm64", "compile")); err != nil {
		t.Fatal(err)
	}
	if err := VerifyToolchainArch(noCompiler, "darwin", "arm64"); err == nil {
		t.Error("a toolchain without pkg/tool/<os_arch>/compile was accepted")
	}
}

func TestVerifyToolchainArchOnTheRunningLinuxToolchain(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads the running Go as an ELF")
	}
	goroot := runtime.GOROOT()
	if !isExecutableFile(filepath.Join(goroot, "bin", "go")) {
		t.Skip("no go binary at runtime.GOROOT()")
	}
	trimmed := filepath.Join(t.TempDir(), "gotoolchain")
	if err := TrimToolchainDir(goroot, trimmed); err != nil {
		t.Fatal(err)
	}
	if err := VerifyToolchainArch(trimmed, "linux", runtime.GOARCH); err != nil {
		t.Errorf("host arch %s: %v", runtime.GOARCH, err)
	}
	other := "arm64"
	if runtime.GOARCH == "arm64" {
		other = "amd64"
	}
	if err := VerifyToolchainArch(trimmed, "linux", other); err == nil {
		t.Errorf("the %s toolchain was accepted as %s", runtime.GOARCH, other)
	}
}
