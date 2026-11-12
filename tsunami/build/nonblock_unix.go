// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package build

import "syscall"

// Opening a FIFO without this flag blocks until a writer appears, and no context reaches
// an open in progress.
const nonBlockFlag = syscall.O_NONBLOCK
