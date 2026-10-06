// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package build

import (
	"syscall"
	"testing"
	"time"
)

// Polls briefly because a killed process is only gone once its parent or init reaps it.
func requireProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return
		}
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("background process %d is still alive after the probe returned", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
