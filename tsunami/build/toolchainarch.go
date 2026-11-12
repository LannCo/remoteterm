// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"debug/elf"
	"debug/macho"
	"fmt"
	"path/filepath"
)

// VerifyToolchainArch opens the staged go binary and checks it is a native executable
// for goos/goarch: a bundle that ships the other architecture's go would pass every
// other check and then fail only on the user's machine.
func VerifyToolchainArch(root string, goos string, goarch string) error {
	goPath := filepath.Join(root, "bin", goExeName())
	switch goos {
	case "darwin":
		file, err := macho.Open(goPath)
		if err != nil {
			return fmt.Errorf("%s is not a Mach-O executable: %w", goPath, err)
		}
		defer file.Close()
		want := map[string]macho.Cpu{"arm64": macho.CpuArm64, "amd64": macho.CpuAmd64}[goarch]
		if want == 0 || file.Cpu != want {
			return fmt.Errorf("%s is built for %v, not %s", goPath, file.Cpu, goarch)
		}
	case "linux":
		file, err := elf.Open(goPath)
		if err != nil {
			return fmt.Errorf("%s is not an ELF executable: %w", goPath, err)
		}
		defer file.Close()
		want := map[string]elf.Machine{"arm64": elf.EM_AARCH64, "amd64": elf.EM_X86_64}[goarch]
		if want == 0 || file.Machine != want {
			return fmt.Errorf("%s is built for %v, not %s", goPath, file.Machine, goarch)
		}
	default:
		return fmt.Errorf("cannot verify binaries for GOOS=%s", goos)
	}
	if !isExecutableFile(filepath.Join(root, "pkg", "tool", goos+"_"+goarch, "compile")) {
		return fmt.Errorf("no compiler at pkg/tool/%s_%s/compile", goos, goarch)
	}
	return nil
}
