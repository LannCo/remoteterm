// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package remotetermappstore

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func returnsWithin(t *testing.T, what string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not return within 2s (blocked on a FIFO?)", what)
		return nil
	}
}

func TestReadAppFileRefusesFifo(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.go"), 0644); err != nil {
		t.Fatal(err)
	}
	err := returnsWithin(t, "ReadAppFile", func() error {
		_, err := ReadAppFile("draft/demo", "pipe.go")
		return err
	})
	if err == nil {
		t.Fatal("expected an error reading a FIFO")
	}
}

func TestWriteAppFileRefusesFifoWithoutReader(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.go"), 0644); err != nil {
		t.Fatal(err)
	}
	err := returnsWithin(t, "WriteAppFile", func() error {
		return WriteAppFile("draft/demo", "pipe.go", []byte("x"))
	})
	if err == nil {
		t.Fatal("expected an error overwriting a FIFO")
	}
}

func TestWriteAppFileRefusesFifoWithReader(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	fifo := filepath.Join(dir, "pipe.go")
	if err := syscall.Mkfifo(fifo, 0644); err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	err = returnsWithin(t, "WriteAppFile", func() error {
		return WriteAppFile("draft/demo", "pipe.go", []byte("leaked"))
	})
	if err == nil {
		t.Fatal("expected an error overwriting a FIFO")
	}
	buf := make([]byte, 16)
	if n, _ := reader.Read(buf); n != 0 {
		t.Fatalf("data reached the FIFO reader: %q", buf[:n])
	}
}
