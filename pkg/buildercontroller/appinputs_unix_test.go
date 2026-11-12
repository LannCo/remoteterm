// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package buildercontroller

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestComputeAppInputHashIgnoresFifoNamedAsGoFile(t *testing.T) {
	dir := makeHashAppDir(t)
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "x.go"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "static-fifo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "static"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "static", "pipe.png"), 0644); err != nil {
		t.Fatal(err)
	}
	type result struct {
		hash string
		err  error
	}
	done := make(chan result, 1)
	begin := time.Now()
	go func() {
		h, err := ComputeAppInputHash(dir)
		done <- result{h, err}
	}()
	select {
	case r := <-done:
		if r.err != nil || r.hash == "" {
			t.Fatalf("hash = %q, err = %v", r.hash, r.err)
		}
		t.Logf("hash with FIFOs returned in %v", time.Since(begin))
	case <-time.After(2 * time.Second):
		t.Fatal("ComputeAppInputHash did not return within 2s (blocked on a FIFO?)")
	}
}
