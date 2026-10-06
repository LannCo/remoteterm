// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
)

func setBuilderRtInfoForTest(t *testing.T, builderId string, appId string, env map[string]any) {
	t.Helper()
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": appId, "builder:env": env})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
}
