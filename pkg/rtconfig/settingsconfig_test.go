// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtconfig

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/util/utilfn"
)

const concurrentWriteIterations = 20

func withConfigDir(t *testing.T) {
	t.Helper()
	saved := remotetermbase.ConfigHome_VarCache
	remotetermbase.ConfigHome_VarCache = t.TempDir()
	t.Cleanup(func() { remotetermbase.ConfigHome_VarCache = saved })
}

func boolSettingsKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	boolType := reflect.TypeOf(true)
	settingsType := reflect.TypeOf(SettingsType{})
	for i := 0; i < settingsType.NumField(); i++ {
		field := settingsType.Field(i)
		key := utilfn.GetJsonTag(field)
		if key == "" || strings.HasSuffix(key, ":*") || getConfigKeyType(key) != field.Type {
			continue
		}
		if field.Type == boolType || field.Type == reflect.PointerTo(boolType) {
			keys = append(keys, key)
		}
	}
	if len(keys) < 8 {
		t.Fatalf("too few bool settings keys for a concurrency test: %v", keys)
	}
	return keys
}

func runConcurrently(n int, fn func(i int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
}

func TestSetBaseConfigValueConcurrentWritersKeepAllKeys(t *testing.T) {
	keys := boolSettingsKeys(t)
	for iter := 0; iter < concurrentWriteIterations; iter++ {
		withConfigDir(t)
		runConcurrently(len(keys), func(i int) {
			if err := SetBaseConfigValue(remotetermobj.MetaMapType{keys[i]: true}); err != nil {
				t.Errorf("set %s: %v", keys[i], err)
			}
		})
		m, cerrs := ReadWaveHomeConfigFile(SettingsFile)
		if len(cerrs) > 0 {
			t.Fatalf("read: %v", cerrs)
		}
		for _, key := range keys {
			if _, ok := m[key]; !ok {
				t.Fatalf("iteration %d: key %s lost to a concurrent writer", iter, key)
			}
		}
	}
}

func TestSetConnectionsConfigValueConcurrentWritersKeepAllConns(t *testing.T) {
	const numConns = 32
	for iter := 0; iter < concurrentWriteIterations; iter++ {
		withConfigDir(t)
		runConcurrently(numConns, func(i int) {
			connName := fmt.Sprintf("user@host%d", i)
			if err := SetConnectionsConfigValue(connName, remotetermobj.MetaMapType{"conn:wshenabled": false}); err != nil {
				t.Errorf("set %s: %v", connName, err)
			}
		})
		m, cerrs := ReadWaveHomeConfigFile(ConnectionsFile)
		if len(cerrs) > 0 {
			t.Fatalf("read: %v", cerrs)
		}
		for i := 0; i < numConns; i++ {
			connName := fmt.Sprintf("user@host%d", i)
			if m.GetMap(connName) == nil {
				t.Fatalf("iteration %d: connection %s lost to a concurrent writer", iter, connName)
			}
		}
	}
}
