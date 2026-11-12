// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package build

import (
	"os/exec"
	"syscall"
)

// A new session detaches the probe from any controlling tty, so an interactive shell
// cannot stop itself on SIGTTIN; its group id equals its pid, and rc files can leave
// background jobs holding stdout, so killing the whole group is what ends the probe.
func setProbeProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// ESRCH (nothing left in the group) is the normal case after a clean exit.
func killProbeGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
