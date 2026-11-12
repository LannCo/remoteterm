// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package remotetermappstore

import (
	"io/fs"
	"syscall"
)

// Opening a FIFO without this flag blocks until a peer appears, which would hang the caller.
const nonBlockFlag = syscall.O_NONBLOCK

func hasOtherHardLinks(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}
