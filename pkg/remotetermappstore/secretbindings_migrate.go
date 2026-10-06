// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// A process that can write the app folder could plant a legacy file to bind the user's stored
// secrets into its own code, so only a plain, parseable file is taken, once, and only while
// nothing exists at the new location. Anything else is left alone and counts as no bindings;
// failures are logged rather than returned so a bad legacy file never blocks the app.
func migrateLegacySecretBindings(appId string) {
	legacyBindingsMigrationLock.Lock()
	defer legacyBindingsMigrationLock.Unlock()
	if err := moveLegacySecretBindings(appId); err != nil {
		log.Printf("secret bindings migration for %s skipped: %v\n", appId, err)
	}
}

func moveLegacySecretBindings(appId string) error {
	newPath, err := GetSecretBindingsPath(appId)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(newPath); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	appDir, err := GetAppDir(appId)
	if err != nil {
		return err
	}
	root, err := openAppRoot(appDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat(legacySecretBindingsFileName); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	data, _, err := readRegularFileInRoot(root, legacySecretBindingsFileName, maxSecretBindingsSize)
	if err != nil {
		return err
	}
	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("legacy file does not parse: %w", err)
	}
	if err := createSecretBindingsBytes(newPath, data); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	if err := root.Remove(legacySecretBindingsFileName); err != nil {
		return fmt.Errorf("migrated, but could not remove the legacy file: %w", err)
	}
	return nil
}

// O_EXCL: a binding written between the existence check and here is never overwritten.
func createSecretBindingsBytes(bindingsPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(bindingsPath), 0700); err != nil {
		return fmt.Errorf("failed to create secret bindings directory: %w", err)
	}
	f, err := os.OpenFile(bindingsPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return errors.Join(fmt.Errorf("failed to write secret bindings: %w", err), os.Remove(bindingsPath))
	}
	if err := f.Close(); err != nil {
		return errors.Join(fmt.Errorf("failed to write secret bindings: %w", err), os.Remove(bindingsPath))
	}
	return nil
}
