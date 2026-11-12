// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func TestCheckBuilderCaller(t *testing.T) {
	builderId := uuid.NewString()
	other := uuid.NewString()
	cases := []struct {
		source        string
		allowRenderer bool
		ok            bool
	}{
		{wshutil.ElectronRoute, false, true},
		{wshutil.ElectronRoute, true, true},
		{wshutil.MakeBuilderRouteId(builderId), true, true},
		{wshutil.MakeBuilderRouteId(builderId), false, false},
		{wshutil.MakeBuilderRouteId(other), true, false},
		{wshutil.MakeProcRouteId(uuid.NewString()), true, false},
		{wshutil.MakeControllerRouteId(uuid.NewString()), true, false},
		{wshutil.MakeConnectionRouteId("user@host"), true, false},
		{wshutil.MakeTabRouteId(uuid.NewString()), true, false},
		{wshutil.MakeFeBlockRouteId(uuid.NewString()), true, false},
		{"", true, false},
	}
	for _, tc := range cases {
		err := checkBuilderCaller(tc.source, builderId, tc.allowRenderer)
		if (err == nil) != tc.ok {
			t.Errorf("checkBuilderCaller(%q, allowRenderer=%v) = %v, want ok=%v", tc.source, tc.allowRenderer, err, tc.ok)
		}
	}
	for _, bad := range []string{"", "not-a-uuid", strings.ToUpper(builderId), "{" + builderId + "}", "urn:uuid:" + builderId} {
		if err := checkBuilderCaller(wshutil.ElectronRoute, bad, true); err == nil {
			t.Errorf("builder id %q accepted", bad)
		}
	}
}

func TestBuilderLockIsPerBuilder(t *testing.T) {
	a := uuid.NewString()
	if getBuilderLock(a) != getBuilderLock(a) {
		t.Fatal("the same builder got two locks")
	}
	if getBuilderLock(a) == getBuilderLock(uuid.NewString()) {
		t.Fatal("two builders share a lock")
	}
}

func TestWithBuilderLockSerialises(t *testing.T) {
	builderId := uuid.NewString()
	var inside, maxInside atomic.Int32
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			withBuilderLock(builderId, func() error {
				n := inside.Add(1)
				for {
					m := maxInside.Load()
					if n <= m || maxInside.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				inside.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()
	if maxInside.Load() != 1 {
		t.Fatalf("%d callers inside the lock at once", maxInside.Load())
	}
}

func TestMakeBuilderWriteContextIsDetachedAndTracksUpdates(t *testing.T) {
	ctx, cancelFn := makeBuilderWriteContext()
	defer cancelFn()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > BuilderWriteTimeout || time.Until(deadline) < BuilderWriteTimeout-time.Second {
		t.Fatalf("deadline = %v (ok=%v), want about %v from now", deadline, ok, BuilderWriteTimeout)
	}
	if remotetermobj.ContextGetUpdates(ctx) == nil {
		t.Fatal("write context does not collect object updates")
	}
}

func TestIsValidBuilderTargetAction(t *testing.T) {
	for _, ok := range []string{"", "splitright", "splitleft", "splitup", "splitdown"} {
		if !isValidBuilderTargetAction(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"replace", "SPLITRIGHT", "split", "insert"} {
		if isValidBuilderTargetAction(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestMakeBuilderLayoutAction(t *testing.T) {
	insert := makeBuilderLayoutAction("b2", "", "")
	if insert.ActionType != rtcore.LayoutActionDataType_Insert || insert.BlockId != "b2" || !insert.Focused || insert.TargetBlockId != "" {
		t.Errorf("insert = %+v", insert)
	}
	cases := map[string][2]string{
		"splitright": {rtcore.LayoutActionDataType_SplitHorizontal, "after"},
		"splitleft":  {rtcore.LayoutActionDataType_SplitHorizontal, "before"},
		"splitup":    {rtcore.LayoutActionDataType_SplitVertical, "before"},
		"splitdown":  {rtcore.LayoutActionDataType_SplitVertical, "after"},
	}
	for action, want := range cases {
		got := makeBuilderLayoutAction("b2", "b1", action)
		if got.ActionType != want[0] || got.Position != want[1] || got.TargetBlockId != "b1" || got.BlockId != "b2" || !got.Focused {
			t.Errorf("%s = %+v, want type %s position %s", action, got, want[0], want[1])
		}
	}
}
