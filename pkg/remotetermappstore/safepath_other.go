// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package remotetermappstore

import "io/fs"

const nonBlockFlag = 0

// The Windows FileInfo carries no link count, so the hard-link refusal is unix-only.
func hasOtherHardLinks(info fs.FileInfo) bool {
	return false
}
