// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// allows for streaming an io.Reader to a channel and an io.Writer from a channel
package iochan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/LannCo/remoteterm/pkg/util/iochan/iochantypes"
	"github.com/LannCo/remoteterm/pkg/util/utilfn"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
)

// ReaderChan reads from an io.Reader and sends the data to a channel
func ReaderChan(ctx context.Context, r io.Reader, chunkSize int64, callback func()) chan wshrpc.RespOrErrorUnion[iochantypes.Packet] {
	ch := make(chan wshrpc.RespOrErrorUnion[iochantypes.Packet], 32)
	go func() {
		defer func() {
			log.Printf("Closing ReaderChan\n")
			close(ch)
			callback()
		}()
		send := func(pkt wshrpc.RespOrErrorUnion[iochantypes.Packet]) bool {
			select {
			case ch <- pkt:
				return true
			case <-ctx.Done():
				return false
			}
		}
		sha256Hash := sha256.New()
		for ctx.Err() == nil {
			buf := make([]byte, chunkSize)
			n, err := r.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					send(wshrpc.RespOrErrorUnion[iochantypes.Packet]{Response: iochantypes.Packet{Checksum: sha256Hash.Sum(nil)}})
					return
				}
				send(wshutil.RespErr[iochantypes.Packet](fmt.Errorf("ReaderChan: read error: %v", err)))
				return
			}
			if n == 0 {
				continue
			}
			if _, err := sha256Hash.Write(buf[:n]); err != nil {
				send(wshutil.RespErr[iochantypes.Packet](fmt.Errorf("ReaderChan: error writing to sha256 hash: %v", err)))
				return
			}
			if !send(wshrpc.RespOrErrorUnion[iochantypes.Packet]{Response: iochantypes.Packet{Data: buf[:n]}}) {
				return
			}
		}
	}()
	return ch
}

// WriterChan reads from a channel and writes the data to an io.Writer
func WriterChan(ctx context.Context, w io.Writer, ch <-chan wshrpc.RespOrErrorUnion[iochantypes.Packet], callback func(), cancel context.CancelCauseFunc) {
	go func() {
		defer func() {
			if ctx.Err() != nil {
				utilfn.DrainChannelSafe(ch, "WriterChan")
			}
			callback()
		}()
		sha256Hash := sha256.New()
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-ch:
				if !ok {
					return
				}
				if resp.Error != nil {
					cancel(resp.Error)
					return
				}
				if _, err := sha256Hash.Write(resp.Response.Data); err != nil {
					cancel(fmt.Errorf("WriterChan: error writing to sha256 hash: %v", err))
					return
				}
				// The checksum is sent as the last packet
				if resp.Response.Checksum != nil {
					localChecksum := sha256Hash.Sum(nil)
					if !bytes.Equal(localChecksum, resp.Response.Checksum) {
						cancel(fmt.Errorf("WriterChan: checksum mismatch"))
					}
					return
				}
				if _, err := w.Write(resp.Response.Data); err != nil {
					cancel(fmt.Errorf("WriterChan: write error: %v", err))
					return
				}
			}
		}
	}()
}
