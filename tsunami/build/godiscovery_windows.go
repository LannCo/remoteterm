// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package build

import "os/exec"

func setProbeProcessGroup(cmd *exec.Cmd) {}
