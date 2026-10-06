// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/buildercontroller"
	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func TestAutoRunRebuildIsDeclinedForAnAppNeverStartedByHand(t *testing.T) {
	home := setupBuilderApps(t, "autorun-demo")
	if err := os.WriteFile(filepath.Join(appDirFor(home, "autorun-demo"), "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	remotetermappstore.DeleteTrustedBuildHash("draft/autorun-demo")
	builderId := uuid.NewString()
	oref := builderORef(builderId)
	t.Cleanup(func() {
		rtstore.DeleteRTInfo(oref)
		buildercontroller.DeleteController(builderId)
	})
	if err := WshServerImpl.SetRTInfoCommand(electronCtx(), wshrpc.CommandSetRTInfoData{ORef: oref, Data: map[string]any{"builder:appid": "draft/autorun-demo"}}); err != nil {
		t.Fatal(err)
	}

	err := WshServerImpl.RequestBuilderRebuildCommand(sourceCtx(wshutil.MakeBuilderRouteId(builderId)), wshrpc.CommandRequestBuilderRebuildData{BuilderId: builderId, AutoRun: true})
	if !errors.Is(err, buildercontroller.ErrAutoRunDeclined) {
		t.Fatalf("auto-run of an untrusted app = %v, want ErrAutoRunDeclined", err)
	}
	status := buildercontroller.GetOrCreateController(builderId).GetStatus()
	if status.Status != buildercontroller.BuilderStatus_Init {
		t.Fatalf("a declined auto-run left the builder in %q, want init", status.Status)
	}
}
