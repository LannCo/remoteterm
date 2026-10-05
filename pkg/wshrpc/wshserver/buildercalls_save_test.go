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

func TestWriteAppGoFileWithBuilderIdNeedsBuilderCaller(t *testing.T) {
	setupBuilderApps(t, "demo")
	builderId := uuid.NewString()
	data := wshrpc.CommandWriteAppGoFileData{AppId: "draft/demo", BuilderId: builderId, Data64: "cGFja2FnZSBtYWluCg=="}
	_, err := WshServerImpl.WriteAppGoFileCommand(sourceCtx(wshutil.MakeProcRouteId(uuid.NewString())), data)
	if !errors.Is(err, ErrBuilderCallerRefused) {
		t.Fatalf("WriteAppGoFile naming a builder, from a pane = %v, want ErrBuilderCallerRefused", err)
	}
	if _, err := WshServerImpl.WriteAppGoFileCommand(sourceCtx(wshutil.MakeBuilderRouteId(builderId)), data); err != nil {
		t.Fatalf("WriteAppGoFile from the builder renderer: %v", err)
	}
}
