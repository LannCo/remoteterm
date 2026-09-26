// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package genconn

import (
	"context"
	"io"
	"sync"
	"time"
)

// ProgressTracker records the wall-clock time of the most recent write to a
// stall-watched transfer. The clock is injectable so stall detection can be
// tested without real sleeps.
type ProgressTracker struct {
	lock sync.Mutex
	now  func() time.Time
	last time.Time
}

// MakeProgressTracker creates a tracker whose idle clock starts at zero.
// Pass nil for now to use time.Now.
func MakeProgressTracker(now func() time.Time) *ProgressTracker {
	if now == nil {
		now = time.Now
	}
	return &ProgressTracker{now: now, last: now()}
}

// Touch resets the idle clock to zero (a write just happened).
func (p *ProgressTracker) Touch() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.last = p.now()
}

// IdleSince returns how long it has been since the last Touch.
func (p *ProgressTracker) IdleSince() time.Duration {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.now().Sub(p.last)
}

// ProgressWriter wraps Dst so every successful write touches Tracker,
// resetting the stall clock. Used to instrument a byte stream (e.g. an SSH
// exec's stdin) so a watchdog can tell "still moving" from "stuck".
type ProgressWriter struct {
	Dst     io.Writer
	Tracker *ProgressTracker
}

func (w *ProgressWriter) Write(p []byte) (int, error) {
	n, err := w.Dst.Write(p)
	if n > 0 {
		w.Tracker.Touch()
	}
	return n, err
}

// WatchForStall calls cancel once tracker has been idle for at least
// stallTimeout, or once ctx is done (a genuine external cancellation, not a
// fixed wall-clock budget). ticks drives the check interval: production
// callers pass a time.Ticker's channel; tests feed it manually alongside an
// injected clock so stall detection doesn't depend on real elapsed time.
// WatchForStall returns as soon as cancel has fired or stop is closed (the
// caller signals that the watched transfer already finished on its own).
// It must be run in its own goroutine.
func WatchForStall(ctx context.Context, tracker *ProgressTracker, stallTimeout time.Duration, ticks <-chan time.Time, stop <-chan struct{}, cancel context.CancelFunc) {
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			cancel()
			return
		case <-ticks:
			if tracker.IdleSince() >= stallTimeout {
				cancel()
				return
			}
		}
	}
}
