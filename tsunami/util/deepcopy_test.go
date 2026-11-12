// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"math"
	"reflect"
	"testing"
	"time"
)

type dcInner struct {
	Tags []string
}

type dcOuter struct {
	Name    string
	Inner   dcInner
	Ptr     *dcInner
	Map     map[string][]int
	Any     any
	Arr     [2][]int
	When    time.Time
	Fn      func() int
	private []string
	Skip    string `json:"-"`
}

type dcNode struct {
	Val  int
	Next *dcNode
}

func TestDeepCopyIsIndependent(t *testing.T) {
	src := dcOuter{
		Name:    "n",
		Inner:   dcInner{Tags: []string{"a"}},
		Ptr:     &dcInner{Tags: []string{"b"}},
		Map:     map[string][]int{"k": {1}},
		Any:     []int{7},
		Arr:     [2][]int{{1}, {2}},
		When:    time.Date(2026, 1, 2, 3, 4, 5, 6, time.Local),
		Fn:      func() int { return 42 },
		private: []string{"p"},
		Skip:    "kept",
	}
	dst := DeepCopy(src)

	dst.Inner.Tags[0] = "X"
	dst.Ptr.Tags[0] = "X"
	dst.Map["k"][0] = 99
	dst.Any.([]int)[0] = 99
	dst.Arr[0][0] = 99

	if src.Inner.Tags[0] != "a" || src.Ptr.Tags[0] != "b" || src.Map["k"][0] != 1 || src.Any.([]int)[0] != 7 || src.Arr[0][0] != 1 {
		t.Fatalf("mutating the copy changed the source: %+v", src)
	}
	if dst.Ptr == src.Ptr {
		t.Fatalf("pointer not copied")
	}
	if !dst.When.Equal(src.When) || dst.When.Location() != src.When.Location() {
		t.Fatalf("time not preserved: %v vs %v", dst.When, src.When)
	}
	if dst.Fn() != 42 {
		t.Fatalf("func field lost")
	}
	if !reflect.DeepEqual(dst.private, []string{"p"}) || dst.Skip != "kept" {
		t.Fatalf("unexported or json:\"-\" field lost: private=%v skip=%q", dst.private, dst.Skip)
	}
}

func TestDeepCopyPreservesNilsAndNaN(t *testing.T) {
	var nilSlice []int
	if DeepCopy(nilSlice) != nil {
		t.Fatalf("nil slice became non-nil")
	}
	var nilMap map[string]int
	if DeepCopy(nilMap) != nil {
		t.Fatalf("nil map became non-nil")
	}
	var nilAny any
	if DeepCopy(nilAny) != nil {
		t.Fatalf("nil interface became non-nil")
	}
	if got := DeepCopy(math.NaN()); !math.IsNaN(got) {
		t.Fatalf("NaN not preserved: %v", got)
	}
	if got := DeepCopy([]float64{math.Inf(1)}); !math.IsInf(got[0], 1) {
		t.Fatalf("Inf not preserved: %v", got)
	}
}

func TestDeepCopyHandlesCyclesAndSharing(t *testing.T) {
	a := &dcNode{Val: 1}
	b := &dcNode{Val: 2, Next: a}
	a.Next = b
	c := DeepCopy(a)
	if c == a || c.Next == b {
		t.Fatalf("nodes not copied")
	}
	if c.Next.Next != c {
		t.Fatalf("cycle not preserved in copy")
	}

	shared := &dcInner{Tags: []string{"s"}}
	pair := DeepCopy([]*dcInner{shared, shared})
	if pair[0] != pair[1] || pair[0] == shared {
		t.Fatalf("shared pointer not preserved as shared copy")
	}
}
