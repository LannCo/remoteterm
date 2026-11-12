// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var atomNameSeq atomic.Int64

// SharedAtom registers on the process-global client, so names must be unique across -count runs.
func uniqueAtomName(base string) string {
	return fmt.Sprintf("%s-%d", base, atomNameSeq.Add(1))
}

type privState struct {
	Visible int
	hidden  []string
}

func TestSetFnConcurrentUpdatesAreNotLost(t *testing.T) {
	counter := SharedAtom(uniqueAtomName("test-setfn-counter"), 0)
	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.SetFn(func(v int) int { return v + 1 })
		}()
	}
	wg.Wait()
	if got := counter.Get(); got != n {
		t.Fatalf("SetFn x%d concurrent -> %d", n, got)
	}
}

func TestSetFnMixedWithSetDoesNotRace(t *testing.T) {
	items := SharedAtom(uniqueAtomName("test-setfn-mixed"), []int{})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			items.SetFn(func(v []int) []int { return append(v, 1) })
		}()
		go func() {
			defer wg.Done()
			_ = items.Get()
		}()
	}
	wg.Wait()
	if got := len(items.Get()); got != 50 {
		t.Fatalf("expected 50 appends, got %d", got)
	}
}

func TestSetFnPreservesUnexportedFields(t *testing.T) {
	a := SharedAtom(uniqueAtomName("test-setfn-priv"), privState{Visible: 1, hidden: []string{"x"}})
	a.SetFn(func(p privState) privState { p.Visible++; return p })
	got := a.Get()
	if got.Visible != 2 || len(got.hidden) != 1 || got.hidden[0] != "x" {
		t.Fatalf("SetFn lost data: %+v", got)
	}
}

func TestSetFnWithNaNDoesNotPanic(t *testing.T) {
	a := SharedAtom(uniqueAtomName("test-setfn-nan"), math.NaN())
	a.SetFn(func(v float64) float64 {
		if !math.IsNaN(v) {
			t.Errorf("fn received %v, want NaN", v)
		}
		return 1
	})
	if got := a.Get(); got != 1 {
		t.Fatalf("got %v, want 1", got)
	}
}

func TestSetFnCopyIsIsolated(t *testing.T) {
	orig := []int{1, 2, 3}
	a := SharedAtom(uniqueAtomName("test-setfn-isolated"), orig)
	a.SetFn(func(v []int) []int { v[0] = 99; return v })
	if orig[0] != 1 {
		t.Fatalf("SetFn mutated the previous value in place: %v", orig)
	}
}

func TestTrySetFnIsCompareAndSet(t *testing.T) {
	isOpen := SharedAtom(uniqueAtomName("test-trysetfn-open"), false)
	var opened atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if isOpen.trySetFn(openIfClosed) {
				opened.Add(1)
			}
		}()
	}
	wg.Wait()
	if opened.Load() != 1 {
		t.Fatalf("expected exactly one trigger to open the modal, got %d", opened.Load())
	}
}

// SetFn's callback reading another atom must not deadlock against engine paths that
// look atoms up under RootElem.atomLock and then write them.
func TestSetFnReadingOtherAtomDoesNotDeadlock(t *testing.T) {
	x := SharedAtom(uniqueAtomName("test-deadlock-x"), 0)
	y := SharedAtom(uniqueAtomName("test-deadlock-y"), 0)
	root := x.client.Root
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < 200; i++ {
			wg.Add(3)
			go func() {
				defer wg.Done()
				x.SetFn(func(v int) int { return v + y.Get() + 1 })
			}()
			go func() {
				defer wg.Done()
				_ = root.SetAtomVal(x.name, 0)
				root.AtomAddRenderWork(x.name)
			}()
			go func() {
				defer wg.Done()
				y.SetFn(func(v int) int { return v + x.Get() })
			}()
		}
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("deadlock between SetFn callbacks and engine atom writes")
	}
}

func TestSameRef(t *testing.T) {
	base := []int{1, 2, 3}
	var nilPtr *privState
	p := &privState{}
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"nil-ptr", sameRef(nilPtr, nilPtr), false},
		{"same-ptr", sameRef(p, p), true},
		{"different-ptr", sameRef(p, &privState{}), false},
		{"same-slice", sameRef(base, base), true},
		{"sub-slice", sameRef(base, base[1:]), true},
		{"super-slice", sameRef(base[1:], base), true},
		{"copied-slice", sameRef(base, append([]int(nil), base...)), false},
		{"empty-slices", sameRef([]int{}, []int{}), false},
		{"nil-slices", sameRef([]int(nil), []int(nil)), false},
		{"same-map", sameRef(map[string]int{"a": 1}, map[string]int{"a": 1}), false},
		{"int", sameRef(1, 1), false},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: sameRef = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}
