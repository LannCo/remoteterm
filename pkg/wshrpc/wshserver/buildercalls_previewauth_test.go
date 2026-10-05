// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"errors"
	"testing"

	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

func TestPreviewAuthIsNotAvailableToPanes(t *testing.T) {
	builderId := uuid.NewString()
	data := wshrpc.CommandGetBuilderPreviewAuthData{BuilderId: builderId}
	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(uuid.NewString()),
		wshutil.MakeTabRouteId(uuid.NewString()),
		"",
	} {
		if _, err := WshServerImpl.GetBuilderPreviewAuthCommand(sourceCtx(source), data); !errors.Is(err, ErrBuilderCallerRefused) {
			t.Errorf("GetBuilderPreviewAuth from %q = %v, want ErrBuilderCallerRefused", source, err)
		}
	}
}

func TestPreviewAuthIsEmptyWhileNothingRuns(t *testing.T) {
	builderId := uuid.NewString()
	data := wshrpc.CommandGetBuilderPreviewAuthData{BuilderId: builderId}
	for _, source := range []string{wshutil.ElectronRoute, wshutil.MakeBuilderRouteId(builderId)} {
		rtn, err := WshServerImpl.GetBuilderPreviewAuthCommand(sourceCtx(source), data)
		if err != nil {
			t.Fatalf("from %q: %v", source, err)
		}
		if rtn == nil || rtn.Port != 0 || rtn.Token != "" {
			t.Fatalf("from %q: %+v, want empty", source, rtn)
		}
	}
}

func TestPreviewAuthNeedsABuilderId(t *testing.T) {
	if _, err := WshServerImpl.GetBuilderPreviewAuthCommand(electronCtx(), wshrpc.CommandGetBuilderPreviewAuthData{}); err == nil {
		t.Fatal("an empty builder id was accepted")
	}
}
