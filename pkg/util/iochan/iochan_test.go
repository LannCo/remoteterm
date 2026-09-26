// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package iochan_test

import (
	"context"
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
