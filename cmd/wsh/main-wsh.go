// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/LannCo/remoteterm/cmd/wsh/cmd"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

// set by main-server.go
var WaveVersion = "0.0.0"
var BuildTime = "0"

func main() {
	remotetermbase.WaveVersion = WaveVersion
	remotetermbase.BuildTime = BuildTime
	cmd.Execute()
}
