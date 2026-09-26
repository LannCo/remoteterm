// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package iochan_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/util/iochan"
)

const (
	buflen = 1024
)

func TestIochan_Basic(t *testing.T) {
	// Write the packet to the source pipe from a goroutine
	srcPipeReader, srcPipeWriter := io.Pipe()
	packet := []byte("hello world")
	go func() {
		srcPipeWriter.Write(packet)
		srcPipeWriter.Close()
	}()

	// Initialize the reader channel
	readerChanDone := make(chan struct{})
	readerChanCallback := func() {
		srcPipeReader.Close()
		close(readerChanDone)
	}
	defer srcPipeReader.Close()
	ioch := iochan.ReaderChan(context.TODO(), srcPipeReader, buflen, readerChanCallback)

	// Initialize the destination pipe and the writer channel
	destPipeReader, destPipeWriter := io.Pipe()
	writerChanDone := make(chan struct{})
	writerChanCallback := func() {
		destPipeReader.Close()
		destPipeWriter.Close()
		close(writerChanDone)
	}
	defer destPipeReader.Close()
	defer destPipeWriter.Close()
	iochan.WriterChan(context.TODO(), destPipeWriter, ioch, writerChanCallback, func(err error) {})

	// Read the packet from the destination pipe and compare it to the original packet
	buf := make([]byte, buflen)
	n, err := destPipeReader.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(packet) {
		t.Fatalf("Read length mismatch: %d != %d", n, len(packet))
	}
	if string(buf[:n]) != string(packet) {
		t.Fatalf("Read data mismatch: %s != %s", buf[:n], packet)
	}

	// Callbacks run on the iochan goroutines, so wait on channels rather than sleeping
	select {
	case <-readerChanDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("ReaderChan callback not called")
	}
	select {
	case <-writerChanDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("WriterChan callback not called")
	}
}

// endlessReader yields data forever and closes full once the reader goroutine has
// read enough chunks that it must be blocked sending into a full channel.
type endlessReader struct {
	reads  int
	fullAt int
	full   chan struct{}
}

func (r *endlessReader) Read(p []byte) (int, error) {
	r.reads++
	if r.reads == r.fullAt {
		close(r.full)
	}
	return copy(p, "x"), nil
}

// gatedFailWriter fails its first write, but only after the reader is known to be blocked.
type gatedFailWriter struct {
	gate <-chan struct{}
}

func (w gatedFailWriter) Write(p []byte) (int, error) {
	<-w.gate
	return 0, errors.New("write failed")
}

func TestIochan_ReaderExitsWhenWriterFailsUndrained(t *testing.T) {
	// 32 buffered + 1 held by the writer + 1 blocked in send
	src := &endlessReader{fullAt: 34, full: make(chan struct{})}
	readerCtx, readerCancel := context.WithCancelCause(context.Background())
	defer readerCancel(nil)

	readerDone := make(chan struct{})
	ioch := iochan.ReaderChan(readerCtx, src, buflen, func() { close(readerDone) })

	// Writer ctx is never cancelled, so WriterChan will not drain ioch on exit.
	writerDone := make(chan struct{})
	iochan.WriterChan(context.Background(), gatedFailWriter{gate: src.full}, ioch, func() { close(writerDone) }, readerCancel)

	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("WriterChan did not exit after write error")
	}
	select {
	case <-readerDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("ReaderChan goroutine leaked: blocked on send after its context was cancelled")
	}
}
