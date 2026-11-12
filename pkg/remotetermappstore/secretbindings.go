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
	"sync"
	"syscall"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

const (
	// The bindings used to be <app folder>/secret-bindings.json.
	legacySecretBindingsFileName = "secret-bindings.json"
	maxSecretBindingsSize        = 1024 * 1024
)

// Serialises first-access migrations so two readers cannot both move the same file.
var legacyBindingsMigrationLock sync.Mutex

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
	migrateLegacySecretBindings(appId)
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
	// Moves a legacy file out of the app folder before the write supersedes it.
	migrateLegacySecretBindings(appId)
	if bindings == nil {
		bindings = make(map[string]string)
	}
	data, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bindings: %w", err)
	}
	return writeSecretBindingsBytes(bindingsPath, data)
}

type secretBindingsMove struct {
	from string
	to   string
}

// Callers check this before touching an app folder, so a missing data dir fails the
// whole operation with nothing changed instead of halfway through.
func requireSecretBindingsStorage(appIds ...string) error {
	for _, appId := range appIds {
		if _, err := GetSecretBindingsPath(appId); err != nil {
			return err
		}
	}
	return nil
}

func writeSecretBindingsBytes(bindingsPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(bindingsPath), 0700); err != nil {
		return fmt.Errorf("failed to create secret bindings directory: %w", err)
	}
	if err := os.WriteFile(bindingsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write secret bindings: %w", err)
	}
	return nil
}

// The bindings file used to travel with the app folder on publish, draft and revert;
// copying it here keeps that behaviour now that it lives elsewhere. Bytes are copied
// as they are, so a source that no longer parses still moves with its app instead of
// blocking the operation. A missing source clears the target so an older app with the
// same id cannot leave its bindings behind.
func copySecretBindings(fromAppId string, toAppId string) error {
	fromPath, err := GetSecretBindingsPath(fromAppId)
	if err != nil {
		return err
	}
	toPath, err := GetSecretBindingsPath(toAppId)
	if err != nil {
		return err
	}
	migrateLegacySecretBindings(fromAppId)
	data, err := os.ReadFile(fromPath)
	if errors.Is(err, fs.ErrNotExist) {
		return deleteSecretBindings(toAppId)
	}
	if err != nil {
		return fmt.Errorf("failed to read secret bindings: %w", err)
	}
	return writeSecretBindingsBytes(toPath, data)
}

// A rename keeps the bindings in one place at every moment; copy then delete leaves
// two copies if the delete fails, so it is only the fallback for a cross-device move.
func moveSecretBindings(fromAppId string, toAppId string) error {
	fromPath, err := GetSecretBindingsPath(fromAppId)
	if err != nil {
		return err
	}
	toPath, err := GetSecretBindingsPath(toAppId)
	if err != nil {
		return err
	}
	migrateLegacySecretBindings(fromAppId)
	if _, err := os.Lstat(fromPath); errors.Is(err, fs.ErrNotExist) {
		return deleteSecretBindings(toAppId)
	}
	if err := os.MkdirAll(filepath.Dir(toPath), 0700); err != nil {
		return fmt.Errorf("failed to create secret bindings directory: %w", err)
	}
	err = os.Rename(fromPath, toPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return fmt.Errorf("failed to move secret bindings: %w", err)
	}
	if err := copySecretBindings(fromAppId, toAppId); err != nil {
		return err
	}
	if err := deleteSecretBindings(fromAppId); err != nil {
		return errors.Join(err, deleteSecretBindings(toAppId))
	}
	return nil
}

// Applies the moves in order; if one fails, the ones already applied are reversed so
// the bindings stay with the id whose folder they belong to.
func moveSecretBindingsAll(moves []secretBindingsMove) error {
	for i, m := range moves {
		if err := moveSecretBindings(m.from, m.to); err != nil {
			return errors.Join(err, reverseSecretBindingsMoves(moves[:i]))
		}
	}
	return nil
}

func reverseSecretBindingsMoves(moves []secretBindingsMove) error {
	var errs []error
	for i := len(moves) - 1; i >= 0; i-- {
		if err := moveSecretBindings(moves[i].to, moves[i].from); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
