// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshutil

import (
	"context"
	"testing"
)

func TestMakeRpcSourceContextForTest(t *testing.T) {
	if got := GetRpcSourceFromContext(context.Background()); got != "" {
		t.Fatalf("source of a bare context = %q", got)
	}
	ctx := MakeRpcSourceContextForTest(context.Background(), ElectronRoute)
	if got := GetRpcSourceFromContext(ctx); got != ElectronRoute {
		t.Fatalf("source = %q, want %q", got, ElectronRoute)
	}
	if GetIsCanceledFromContext(ctx) {
		t.Fatal("a test context reports cancelled")
	}
}
