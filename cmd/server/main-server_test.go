// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"

	"github.com/LannCo/remoteterm/pkg/authkey"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func TestGrabAndRemoveEnvVarsUnsetsNewAndLegacyNames(t *testing.T) {
	savedConfig, savedData := remotetermbase.ConfigHome_VarCache, remotetermbase.DataHome_VarCache
	t.Cleanup(func() {
		remotetermbase.ConfigHome_VarCache, remotetermbase.DataHome_VarCache = savedConfig, savedData
	})
	t.Setenv(authkey.WaveAuthKeyEnv, "test-auth-key")
	t.Setenv(remotetermbase.WaveConfigHomeEnvVar, t.TempDir())
	t.Setenv(remotetermbase.WaveDataHomeEnvVar, t.TempDir())
	sessionVars := []string{
		remotetermbase.WaveClientIdVarName,
		remotetermbase.LegacyWaveClientIdVarName,
		remotetermbase.WaveWorkspaceIdVarName,
		remotetermbase.LegacyWaveWorkspaceIdVarName,
		remotetermbase.WaveTabIdVarName,
		remotetermbase.LegacyWaveTabIdVarName,
		remotetermbase.WaveBlockIdVarName,
		remotetermbase.LegacyWaveBlockIdVarName,
		remotetermbase.WaveConnVarName,
		remotetermbase.LegacyWaveConnVarName,
		remotetermbase.WaveJwtTokenVarName,
		remotetermbase.LegacyWaveJwtTokenVarName,
		remotetermbase.WaveVersionVarName,
	}
	for _, name := range sessionVars {
		t.Setenv(name, "leaked-"+name)
	}
	if err := grabAndRemoveEnvVars(); err != nil {
		t.Fatalf("grabAndRemoveEnvVars: %v", err)
	}
	for _, name := range append(sessionVars, authkey.WaveAuthKeyEnv, remotetermbase.WaveConfigHomeEnvVar, remotetermbase.WaveDataHomeEnvVar) {
		if val, ok := os.LookupEnv(name); ok {
			t.Errorf("%s still set after grabAndRemoveEnvVars: %q", name, val)
		}
	}
	if authkey.GetAuthKey() != "test-auth-key" {
		t.Errorf("auth key not captured before unset")
	}
}

func TestGrabAndRemoveEnvVarsRequiresAuthKey(t *testing.T) {
	t.Setenv(authkey.WaveAuthKeyEnv, "")
	os.Unsetenv(authkey.WaveAuthKeyEnv)
	if err := grabAndRemoveEnvVars(); err == nil {
		t.Fatalf("expected an error when %s is missing", authkey.WaveAuthKeyEnv)
	}
}
