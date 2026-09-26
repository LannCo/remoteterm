// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/tsunami/rpctypes"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

func sendUpdate(t *testing.T, h *httpHandlers, resync bool) *rpctypes.VDomBackendUpdate {
	t.Helper()
	update, err := h.processFrontendUpdate(&rpctypes.VDomFrontendUpdate{Resync: resync})
	if err != nil {
		t.Fatalf("processFrontendUpdate: %v", err)
	}
	return update
}

func setAtomAndMarkDirty(t *testing.T, c *ClientImpl, name string, val any) {
	t.Helper()
	if err := c.Root.SetAtomVal(name, val); err != nil {
		t.Fatalf("SetAtomVal: %v", err)
	}
	c.Root.AtomAddRenderWork(name)
}

func useAtom(vc *RenderContextImpl, c *ClientImpl, name string) any {
	vc.UsedAtoms[name] = true
	return c.Root.GetAtomVal(name)
}

func TestNilDepsEffectCleanupRunsExactlyOncePerRun(t *testing.T) {
	cases := []struct {
		name   string
		resync func(i int) bool
	}{
		{"always-resync", func(int) bool { return true }},
		{"never-resync", func(int) bool { return false }},
		{"alternating", func(i int) bool { return i%2 == 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := makeClient()
			h := newHTTPHandlers(c)
			c.Root.RegisterAtom("$shared.tick", MakeAtomImpl(0, nil))
			runs := 0
			cleaned := map[int]int{}
			DefineComponentEx(c, "App", func(_ struct{}) any {
				vc := GetGlobalRenderContext()
				useAtom(vc, c, "$shared.tick")
				UseEffect(vc, func() func() {
					runs++
					id := runs
					return func() { cleaned[id]++ }
				}, nil)
				return vdom.H("div", nil, "x")
			})

			sendUpdate(t, h, true)
			const cycles = 6
			for i := 1; i <= cycles; i++ {
				setAtomAndMarkDirty(t, c, "$shared.tick", i)
				sendUpdate(t, h, tc.resync(i))
			}
			// flush the effect queued by the last render
			sendUpdate(t, h, false)

			if runs != cycles+1 {
				t.Fatalf("expected %d effect runs (one per render), got %d; cleaned=%v", cycles+1, runs, cleaned)
			}
			for id := 1; id < runs; id++ {
				if cleaned[id] != 1 {
					t.Errorf("cleanup for run %d ran %d times, want 1 (cleaned=%v)", id, cleaned[id], cleaned)
				}
			}
			if cleaned[runs] != 0 {
				t.Errorf("cleanup for the live run %d already ran %d times", runs, cleaned[runs])
			}
		})
	}
}

func TestIncrementalUpdateWithoutWorkDoesNotRerender(t *testing.T) {
	c := makeClient()
	h := newHTTPHandlers(c)
	renders := 0
	DefineComponentEx(c, "App", func(_ struct{}) any {
		renders++
		return vdom.H("div", nil, "x")
	})
	sendUpdate(t, h, true)
	update := sendUpdate(t, h, false)
	if renders != 1 {
		t.Fatalf("expected 1 render, got %d", renders)
	}
	if update.FullUpdate {
		t.Fatalf("non-resync update reported FullUpdate")
	}
	if len(update.RenderUpdates) != 1 || update.RenderUpdates[0].VDomWaveId == "" || len(update.TransferElems) == 0 {
		t.Fatalf("incremental update must still carry the root vdom: %+v", update.RenderUpdates)
	}
}

func TestUnmountRecoversFromPanickingCleanup(t *testing.T) {
	c := makeClient()
	h := newHTTPHandlers(c)
	c.Root.RegisterAtom("$shared.show", MakeAtomImpl(true, nil))
	var childId string
	secondCleanupRan := false
	DefineComponentEx(c, "Child", func(_ struct{}) any {
		vc := GetGlobalRenderContext()
		childId = UseId(vc)
		UseEffect(vc, func() func() { return func() { panic("boom") } }, []any{})
		UseEffect(vc, func() func() { return func() { secondCleanupRan = true } }, []any{})
		return vdom.H("span", nil, "child")
	})
	DefineComponentEx(c, "App", func(_ struct{}) any {
		vc := GetGlobalRenderContext()
		if useAtom(vc, c, "$shared.show").(bool) {
			return vdom.H("div", nil, vdom.H("Child", nil))
		}
		return vdom.H("div", nil)
	})

	sendUpdate(t, h, true)
	sendUpdate(t, h, false) // run the child's effects so their cleanups are armed
	if c.Root.CompMap[childId] == nil {
		t.Fatalf("child not mounted")
	}

	setAtomAndMarkDirty(t, c, "$shared.show", false)
	sendUpdate(t, h, false)

	if !secondCleanupRan {
		t.Errorf("a panicking cleanup prevented the next hook's cleanup from running")
	}
	if c.Root.CompMap[childId] != nil {
		t.Errorf("child component left in CompMap after unmount")
	}
	setAtomAndMarkDirty(t, c, "$shared.show", true)
	sendUpdate(t, h, false)
	if c.Root.CompMap[childId] == nil {
		t.Errorf("child did not remount after a panicking cleanup")
	}
}

func TestDepsEqualHandlesNonComparable(t *testing.T) {
	sliceA := []string{"a"}
	cases := []struct {
		name string
		a, b []any
		want bool
	}{
		{"equal-slices", []any{[]string{"a"}}, []any{[]string{"a"}}, true},
		{"different-slices", []any{[]string{"a"}}, []any{[]string{"b"}}, false},
		{"same-slice", []any{sliceA}, []any{sliceA}, true},
		{"maps", []any{map[string]int{"x": 1}}, []any{map[string]int{"x": 1}}, true},
		{"struct-holding-slice", []any{struct{ X any }{[]int{1}}}, []any{struct{ X any }{[]int{1}}}, true},
		{"nil-vs-value", []any{nil}, []any{1}, false},
		{"nil-vs-nil", []any{nil}, []any{nil}, true},
		{"different-types", []any{1}, []any{int64(1)}, false},
		{"pointers-by-identity", []any{new(int)}, []any{new(int)}, false},
		{"func", []any{func() {}}, []any{func() {}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := depsEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("depsEqual = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSliceDepsAcrossRenders(t *testing.T) {
	c := makeClient()
	h := newHTTPHandlers(c)
	c.Root.RegisterAtom("$shared.items", MakeAtomImpl([]string{"a"}, nil))
	runs := 0
	DefineComponentEx(c, "App", func(_ struct{}) any {
		vc := GetGlobalRenderContext()
		items := useAtom(vc, c, "$shared.items").([]string)
		UseEffect(vc, func() func() { runs++; return nil }, []any{items})
		return vdom.H("div", nil, "x")
	})
	sendUpdate(t, h, true)
	setAtomAndMarkDirty(t, c, "$shared.items", []string{"a"})
	sendUpdate(t, h, false)
	setAtomAndMarkDirty(t, c, "$shared.items", []string{"b"})
	sendUpdate(t, h, false)
	sendUpdate(t, h, false)
	if runs != 2 {
		t.Fatalf("expected effect to run for the initial and changed deps only, got %d runs", runs)
	}
}

func TestDuplicateSiblingKeysGetDistinctComponents(t *testing.T) {
	c := makeClient()
	h := newHTTPHandlers(c)
	c.Root.RegisterAtom("$shared.n", MakeAtomImpl(0, nil))
	DefineComponentEx(c, "App", func(_ struct{}) any {
		vc := GetGlobalRenderContext()
		useAtom(vc, c, "$shared.n")
		return vdom.H("div", nil,
			vdom.H("span", map[string]any{"key": "a"}, "one"),
			vdom.H("span", map[string]any{"key": "a"}, "two"),
		)
	})
	check := func() {
		t.Helper()
		div := c.Root.Root.RenderedComp
		if div == nil || len(div.Children) != 2 {
			t.Fatalf("unexpected tree shape")
		}
		if div.Children[0] == div.Children[1] || div.Children[0].WaveId == div.Children[1].WaveId {
			t.Fatalf("duplicate keys aliased one component")
		}
	}
	sendUpdate(t, h, true)
	check()
	setAtomAndMarkDirty(t, c, "$shared.n", 1)
	sendUpdate(t, h, false)
	check()
	if len(c.Root.CompMap) != countComps(c.Root.Root) {
		t.Fatalf("CompMap has %d entries but tree has %d components", len(c.Root.CompMap), countComps(c.Root.Root))
	}
}

func countComps(comp *ComponentImpl) int {
	if comp == nil {
		return 0
	}
	n := 1 + countComps(comp.RenderedComp)
	for _, child := range comp.Children {
		n += countComps(child)
	}
	return n
}

func TestRefOpsAndTermSizeConcurrentWithRender(t *testing.T) {
	c := makeClient()
	ref := &vdom.VDomRef{RefId: "w:0"}
	ref.HasCurrent.Store(true)
	c.Root.CompMap["w"] = &ComponentImpl{WaveId: "w", Hooks: []*Hook{{Idx: 0, Val: ref}}}

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			c.Root.QueueRefOp(vdom.VDomRefOperation{RefId: ref.RefId, Op: "focus"})
			_ = ref.TermSize.Load()
			_ = ref.Position.Load()
		}
	}()
	collected := 0
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			collected += len(c.Root.GetRefOperations())
			c.Root.UpdateRef(rpctypes.VDomRefUpdate{
				RefId:      ref.RefId,
				HasCurrent: true,
				Position:   &vdom.VDomRefPosition{ScrollTop: i},
				TermSize:   &vdom.VDomTermSize{Rows: i, Cols: i},
			})
		}
	}()
	wg.Wait()
	collected += len(c.Root.GetRefOperations())
	if collected != n {
		t.Fatalf("lost ref operations: queued %d, collected %d", n, collected)
	}
	if ts := ref.TermSize.Load(); ts == nil || ts.Rows != n-1 {
		t.Fatalf("unexpected final term size %+v", ts)
	}
}

func TestShowModalWithoutConnectionCancels(t *testing.T) {
	c := makeClient()
	resultChan := c.ShowModal(rpctypes.ModalConfig{ModalId: "m1", ModalType: "alert"})
	result, ok := <-resultChan
	if !ok || result {
		t.Fatalf("expected immediate cancelled result, got result=%v ok=%v", result, ok)
	}

	c.RegisterSSEChannel("conn1")
	resultChan = c.ShowModal(rpctypes.ModalConfig{ModalId: "m2", ModalType: "alert"})
	select {
	case r := <-resultChan:
		t.Fatalf("modal with a live connection resolved early: %v", r)
	default:
	}
	c.CloseModal("m2", true)
	if r := <-resultChan; !r {
		t.Fatalf("expected confirmed result after CloseModal")
	}
}
