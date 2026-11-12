// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// A warm rebuild runs Tailwind, compiles one package and links; seconds on any
	// hardware this ships to.
	WarmBuildTimeout = 60 * time.Second
	// The first build compiles the standard library and the SDK from an empty GOCACHE
	// (about 12 s of CPU on a fast desktop, several times that on an older dual-core
	// Mac) and may fetch modules; Rebuild resumes from the cache, so the deadline only
	// has to be long enough that one click usually finishes.
	ColdBuildTimeout = 5 * time.Minute
)

// BuildTimeout is the deadline to give one build: the cold one until the Go build cache
// holds something, the warm one after.
func BuildTimeout() time.Duration {
	if goBuildCacheIsCold() {
		return ColdBuildTimeout
	}
	return WarmBuildTimeout
}

func goBuildCacheDir() string {
	if dir := os.Getenv("GOCACHE"); dir != "" {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "go-build")
}

// Go creates all 256 two-hex-digit directories when it first opens the cache, so an
// empty cache still has them; only a file inside one means something was compiled.
func goBuildCacheIsCold() bool {
	dir := goBuildCacheDir()
	if dir == "" || dir == "off" {
		return true
	}
	subdirs, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, sub := range subdirs {
		if !sub.IsDir() || len(sub.Name()) != 2 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(dir, sub.Name()))
		if err == nil && len(entries) > 0 {
			return false
		}
	}
	return true
}

// timeoutError is nil unless the build's deadline (not a cancellation) is what stopped
// the command, so a generic "failed" is never reported for a timeout.
func (opts BuildOpts) timeoutError(what string) error {
	if !errors.Is(opts.context().Err(), context.DeadlineExceeded) {
		return nil
	}
	if opts.started.IsZero() {
		return fmt.Errorf("%s timed out: the build deadline passed; Rebuild continues from the cache", what)
	}
	elapsed := time.Since(opts.started).Round(time.Second)
	return fmt.Errorf("%s timed out after %ds: the build deadline passed; Rebuild continues from the cache", what, int(elapsed.Seconds()))
}
