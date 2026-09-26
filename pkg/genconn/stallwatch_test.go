// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package genconn

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

// fakeClock lets tests advance "elapsed time" deterministically instead of
// sleeping real wall-clock seconds.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestProgressTracker(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	tracker := MakeProgressTracker(clock.Now)

	if idle := tracker.IdleSince(); idle != 0 {
		t.Fatalf("expected zero idle right after creation, got %v", idle)
	}

	clock.Advance(3 * time.Second)
	if idle := tracker.IdleSince(); idle != 3*time.Second {
		t.Fatalf("expected 3s idle, got %v", idle)
	}

	tracker.Touch()
	if idle := tracker.IdleSince(); idle != 0 {
		t.Fatalf("expected idle to reset to zero after Touch, got %v", idle)
	}
}

func TestProgressWriter(t *testing.T) {
	t.Parallel()

	t.Run("touches tracker and forwards bytes on a real write", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		tracker := MakeProgressTracker(clock.Now)
		var buf bytes.Buffer
		w := &ProgressWriter{Dst: &buf, Tracker: tracker}

		clock.Advance(5 * time.Second)
		n, err := w.Write([]byte("hello"))
		if err != nil || n != 5 {
			t.Fatalf("unexpected write result n=%d err=%v", n, err)
		}
		if idle := tracker.IdleSince(); idle != 0 {
			t.Fatalf("expected write to reset idle clock, got %v", idle)
		}
		if buf.String() != "hello" {
			t.Fatalf("expected underlying writer to receive bytes, got %q", buf.String())
		}
	})

	t.Run("does not touch tracker on a zero-byte write", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		tracker := MakeProgressTracker(clock.Now)
		w := &ProgressWriter{Dst: io.Discard, Tracker: tracker}

		clock.Advance(5 * time.Second)
		n, err := w.Write(nil)
		if err != nil || n != 0 {
			t.Fatalf("unexpected result n=%d err=%v", n, err)
		}
		if idle := tracker.IdleSince(); idle != 5*time.Second {
			t.Fatalf("expected idle unchanged on zero-byte write, got %v", idle)
		}
	})
}

func TestWatchForStall(t *testing.T) {
	t.Parallel()

	t.Run("cancels once idle exceeds the stall timeout", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		tracker := MakeProgressTracker(clock.Now)
		ticks := make(chan time.Time)
		stop := make(chan struct{})
		canceled := make(chan struct{})
		finished := make(chan struct{})

		go func() {
			WatchForStall(context.Background(), tracker, 5*time.Second, ticks, stop, func() { close(canceled) })
			close(finished)
		}()

		clock.Advance(2 * time.Second)
		ticks <- clock.Now()

		// A send completing only proves the watchdog has begun receiving this
		// tick, not that it has finished evaluating IdleSince() against it (a
		// second unbuffered send here would race that evaluation against the
		// clock.Advance below, not synchronize it -- confirmed by reproducing a
		// deterministic false stall this way). Give the watchdog a short bounded
		// window to actually run instead of checking with a non-blocking default.
		select {
		case <-canceled:
			t.Fatal("canceled before stall timeout elapsed")
		case <-time.After(50 * time.Millisecond):
		}

		clock.Advance(4 * time.Second) // total idle now 6s, past the 5s stall timeout
		ticks <- clock.Now()

		select {
		case <-canceled:
		case <-time.After(2 * time.Second):
			t.Fatal("expected cancel after stall timeout elapsed")
		}
		<-finished
	})

	t.Run("does not cancel while writes keep resetting the idle clock", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		tracker := MakeProgressTracker(clock.Now)
		ticks := make(chan time.Time)
		stop := make(chan struct{})
		canceled := make(chan struct{})
		finished := make(chan struct{})

		go func() {
			WatchForStall(context.Background(), tracker, 5*time.Second, ticks, stop, func() { close(canceled) })
			close(finished)
		}()

		// Ten rounds of "3s elapsed, then a write": 30s of simulated wall-clock
		// progress, far more than the 5s stall timeout would tolerate as a
		// plain fixed deadline, but each individual gap (3s) never reaches it.
		for i := 0; i < 10; i++ {
			clock.Advance(3 * time.Second)
			ticks <- clock.Now()
			tracker.Touch()
		}

		// Close stop and wait for the watchdog to actually exit before
		// asserting on canceled: closing stop only takes effect once the
		// watchdog is back at its top-level select, which happens after it has
		// fully processed the last tick sent above (including any cancel
		// decision) -- checking canceled beforehand would race that decision.
		close(stop)
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Fatal("expected WatchForStall to return after stop is closed")
		}

		select {
		case <-canceled:
			t.Fatal("expected no cancel: writes kept resetting the idle clock")
		default:
		}
	})

	t.Run("cancels when ctx is done even without a stall", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		tracker := MakeProgressTracker(clock.Now)
		ticks := make(chan time.Time)
		stop := make(chan struct{})
		canceled := make(chan struct{})
		finished := make(chan struct{})
		ctx, ctxCancel := context.WithCancel(context.Background())

		go func() {
			WatchForStall(ctx, tracker, 5*time.Second, ticks, stop, func() { close(canceled) })
			close(finished)
		}()

		ctxCancel()

		select {
		case <-canceled:
		case <-time.After(2 * time.Second):
			t.Fatal("expected cancel when ctx is done")
		}
		<-finished
	})
}
