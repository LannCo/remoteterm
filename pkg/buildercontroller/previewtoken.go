// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
)

// Matches tsunami/engine.TsunamiAuthTokenEnvVar.
const PreviewTokenEnvVar = "TSUNAMI_AUTHTOKEN"

// Every run gets its own token, so a token read from an earlier run opens nothing.
func makePreviewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot make a preview token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// A copy: the builder's environment is stored in rtinfo and must not collect the token, and an
// entry the user set under the same name must not win.
func withPreviewToken(builderEnv map[string]string, token string) map[string]string {
	env := make(map[string]string, len(builderEnv)+1)
	maps.Copy(env, builderEnv)
	env[PreviewTokenEnvVar] = token
	return env
}

// The port and token of the app that is running now, for the builder's own window. Empty while
// nothing runs.
func (bc *BuilderController) GetPreviewAuth() (int, string) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.process == nil {
		return 0, ""
	}
	return bc.process.Port, bc.process.PreviewToken
}
