// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtcore

import (
	"context"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
)

func TestDeleteBlockPublishesBlockCloseWhenCascadeFails(t *testing.T) {
	ctx := context.Background()
	orphan := insertTestTab(t, nil)
	block := addTestTermBlock(t, orphan.OID)
	if err := DeleteBlock(ctx, block.OID, true); err == nil {
		t.Fatal("expected the cascade to fail for a tab that belongs to no workspace")
	}
	if objExists(t, remotetermobj.OType_Block, block.OID) {
		t.Error("block row survived")
	}
	if !blockCloses.closed(block.OID) {
		t.Fatal("a cascade error skipped BlockClose, so the shell would keep running")
	}
}
