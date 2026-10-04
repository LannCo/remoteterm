// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

// Bindings live outside the app folder so a process that can write the folder (an
// agent, an editor plugin) cannot bind the user's stored secrets into its own code.
func GetSecretBindingsPath(appId string) (string, error) {
	if err := ValidateAppId(appId); err != nil {
		return "", fmt.Errorf("invalid appId: %w", err)
	}
	dataDir := remotetermbase.GetWaveDataDir()
	if dataDir == "" {
		return "", fmt.Errorf("data directory is not set")
	}
	appNS, appName, _ := ParseAppId(appId)
	return filepath.Join(dataDir, "builder", "secret-bindings", appNS, appName+".json"), nil
}

func ReadAppSecretBindings(appId string) (map[string]string, error) {
	bindingsPath, err := GetSecretBindingsPath(appId)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(bindingsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read secret bindings: %w", err)
	}
	var bindings map[string]string
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("failed to parse secret bindings: %w", err)
	}
	if bindings == nil {
		bindings = make(map[string]string)
	}
	return bindings, nil
}

func WriteAppSecretBindings(appId string, bindings map[string]string) error {
	bindingsPath, err := GetSecretBindingsPath(appId)
	if err != nil {
		return err
	}
	if bindings == nil {
		bindings = make(map[string]string)
	}
	data, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bindings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(bindingsPath), 0700); err != nil {
		return fmt.Errorf("failed to create secret bindings directory: %w", err)
	}
	if err := os.WriteFile(bindingsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write secret bindings: %w", err)
	}
	return nil
}

// The bindings file used to travel with the app folder on publish, draft and revert;
// copying it here keeps that behaviour now that it lives elsewhere.
func copySecretBindings(fromAppId string, toAppId string) error {
	fromPath, err := GetSecretBindingsPath(fromAppId)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(fromPath)
	if errors.Is(err, fs.ErrNotExist) {
		return deleteSecretBindings(toAppId)
	}
	if err != nil {
		return fmt.Errorf("failed to read secret bindings: %w", err)
	}
	var bindings map[string]string
	if err := json.Unmarshal(data, &bindings); err != nil {
		return fmt.Errorf("failed to parse secret bindings: %w", err)
	}
	return WriteAppSecretBindings(toAppId, bindings)
}

func moveSecretBindings(fromAppId string, toAppId string) error {
	if err := copySecretBindings(fromAppId, toAppId); err != nil {
		return err
	}
	return deleteSecretBindings(fromAppId)
}

func deleteSecretBindings(appId string) error {
	bindingsPath, err := GetSecretBindingsPath(appId)
	if err != nil {
		return err
	}
	if err := os.Remove(bindingsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove secret bindings: %w", err)
	}
	return nil
}
