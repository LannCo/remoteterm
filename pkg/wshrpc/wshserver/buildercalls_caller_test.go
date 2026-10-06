// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

type builderCallCase struct {
	name string
	call func(ctx context.Context, builderId string) error
}

func builderCalls() []builderCallCase {
	return []builderCallCase{
		{"StartBuilder", func(ctx context.Context, id string) error {
			return WshServerImpl.StartBuilderCommand(ctx, wshrpc.CommandStartBuilderData{BuilderId: id})
		}},
		{"RequestBuilderRebuild", func(ctx context.Context, id string) error {
			return WshServerImpl.RequestBuilderRebuildCommand(ctx, wshrpc.CommandRequestBuilderRebuildData{BuilderId: id})
		}},
		{"WatchBuilderApp", func(ctx context.Context, id string) error {
			_, err := WshServerImpl.WatchBuilderAppCommand(ctx, wshrpc.CommandWatchBuilderAppData{BuilderId: id})
			return err
		}},
	}
}

// A pane's wsh reaches the server on a proc: link, so these must refuse it. The builder has no
// rtinfo in these tests, so a call that passes the caller check stops at "rtinfo not found":
// the test asserts the rejection error, never a started build.
func TestBuilderBuildCommandsRefusePaneCallers(t *testing.T) {
	builderId := uuid.NewString()
	other := uuid.NewString()
	sources := []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(other),
		wshutil.MakeTabRouteId(uuid.NewString()),
		wshutil.MakeControllerRouteId(uuid.NewString()),
		"",
	}
	for _, tc := range builderCalls() {
		for _, source := range sources {
			err := tc.call(sourceCtx(source), builderId)
			if !errors.Is(err, ErrBuilderCallerRefused) {
				t.Errorf("%s from %q = %v, want ErrBuilderCallerRefused", tc.name, source, err)
			}
		}
	}
}

func TestBuilderBuildCommandsAcceptElectronAndOwnRenderer(t *testing.T) {
	builderId := uuid.NewString()
	for _, tc := range builderCalls() {
		for _, source := range []string{wshutil.ElectronRoute, wshutil.MakeBuilderRouteId(builderId)} {
			err := tc.call(sourceCtx(source), builderId)
			if errors.Is(err, ErrBuilderCallerRefused) {
				t.Errorf("%s from %q refused: %v", tc.name, source, err)
			}
			if err == nil {
				t.Errorf("%s from %q: want the missing-rtinfo error, got nil", tc.name, source)
			}
		}
	}
}

func TestBuilderBuildCommandsRejectInvalidBuilderId(t *testing.T) {
	for _, tc := range builderCalls() {
		if err := tc.call(electronCtx(), "not-a-uuid"); err == nil {
			t.Errorf("%s accepted an invalid builder id", tc.name)
		}
	}
}

func builderORef(builderId string) remotetermobj.ORef {
	return remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
}

func TestSetRTInfoBuilderOrefNeedsBuilderCaller(t *testing.T) {
	builderId := uuid.NewString()
	other := uuid.NewString()
	oref := builderORef(builderId)
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
	data := map[string]any{"builder:appid": "draft/evil"}

	for _, source := range []string{
		wshutil.MakeProcRouteId(uuid.NewString()),
		wshutil.MakeBuilderRouteId(other),
		wshutil.MakeTabRouteId(uuid.NewString()),
		"",
	} {
		err := WshServerImpl.SetRTInfoCommand(sourceCtx(source), wshrpc.CommandSetRTInfoData{ORef: oref, Data: data})
		if !errors.Is(err, ErrBuilderCallerRefused) {
			t.Errorf("SetRTInfo from %q = %v, want ErrBuilderCallerRefused", source, err)
		}
		err = WshServerImpl.SetRTInfoCommand(sourceCtx(source), wshrpc.CommandSetRTInfoData{ORef: oref, Delete: true})
		if !errors.Is(err, ErrBuilderCallerRefused) {
			t.Errorf("SetRTInfo delete from %q = %v, want ErrBuilderCallerRefused", source, err)
		}
	}
	if info := rtstore.GetRTInfo(oref); info != nil && info.BuilderAppId != "" {
		t.Fatalf("a refused write reached the store: %+v", info)
	}

	for _, source := range []string{wshutil.ElectronRoute, wshutil.MakeBuilderRouteId(builderId)} {
		good := map[string]any{"builder:appid": "draft/" + source[:3]}
		if err := WshServerImpl.SetRTInfoCommand(sourceCtx(source), wshrpc.CommandSetRTInfoData{ORef: oref, Data: good}); err != nil {
			t.Errorf("SetRTInfo from %q refused: %v", source, err)
		}
		if info := rtstore.GetRTInfo(oref); info == nil || info.BuilderAppId != good["builder:appid"] {
			t.Errorf("SetRTInfo from %q did not store: %+v", source, info)
		}
	}
}

func TestSetRTInfoOtherOrefsStayOpenToPanes(t *testing.T) {
	oref := remotetermobj.MakeORef(remotetermobj.OType_Block, uuid.NewString())
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
	ctx := sourceCtx(wshutil.MakeProcRouteId(uuid.NewString()))
	if err := WshServerImpl.SetRTInfoCommand(ctx, wshrpc.CommandSetRTInfoData{ORef: oref, Data: map[string]any{"shell:state": "ready"}}); err != nil {
		t.Fatalf("a block rtinfo write from a pane was refused: %v", err)
	}
}

func TestGetRTInfoBuilderOrefNeedsBuilderCaller(t *testing.T) {
	builderId := uuid.NewString()
	oref := builderORef(builderId)
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
	if err := WshServerImpl.SetRTInfoCommand(electronCtx(), wshrpc.CommandSetRTInfoData{ORef: oref, Data: map[string]any{"builder:env": map[string]any{"TOKEN": "s3cret"}}}); err != nil {
		t.Fatal(err)
	}
	_, err := WshServerImpl.GetRTInfoCommand(sourceCtx(wshutil.MakeProcRouteId(uuid.NewString())), wshrpc.CommandGetRTInfoData{ORef: oref})
	if !errors.Is(err, ErrBuilderCallerRefused) {
		t.Fatalf("GetRTInfo from a pane = %v, want ErrBuilderCallerRefused", err)
	}
	info, err := WshServerImpl.GetRTInfoCommand(sourceCtx(wshutil.MakeBuilderRouteId(builderId)), wshrpc.CommandGetRTInfoData{ORef: oref})
	if err != nil || info == nil || info.BuilderEnv["TOKEN"] != "s3cret" {
		t.Fatalf("GetRTInfo from the builder renderer = %+v, %v", info, err)
	}
}
