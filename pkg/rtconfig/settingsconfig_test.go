// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtconfig

import (
	"encoding/json"
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

func TestPortForwardRuleJSON(t *testing.T) {
	t.Parallel()

	t.Run("unmarshal bare string", func(t *testing.T) {
		t.Parallel()
		var r PortForwardRule
		if err := json.Unmarshal([]byte(`"8080 localhost:80"`), &r); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if r.Rule != "8080 localhost:80" || r.Note != "" || r.Enabled != nil {
			t.Errorf("unexpected result: %+v", r)
		}
	})

	t.Run("unmarshal object", func(t *testing.T) {
		t.Parallel()
		var r PortForwardRule
		if err := json.Unmarshal([]byte(`{"rule":"8080 localhost:80","note":"web","enabled":false}`), &r); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if r.Rule != "8080 localhost:80" || r.Note != "web" {
			t.Errorf("unexpected rule/note: %+v", r)
		}
		if r.Enabled == nil || *r.Enabled {
			t.Errorf("expected enabled=false, got %v", r.Enabled)
		}
	})

	t.Run("marshal bare string when no note or enabled", func(t *testing.T) {
		t.Parallel()
		r := PortForwardRule{Rule: "8080 localhost:80"}
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		if string(b) != `"8080 localhost:80"` {
			t.Errorf("expected bare string, got %s", string(b))
		}
	})

	t.Run("marshal object when note set and roundtrip", func(t *testing.T) {
		t.Parallel()
		r := PortForwardRule{Rule: "8080 localhost:80", Note: "web"}
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var back PortForwardRule
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("roundtrip unmarshal error: %v", err)
		}
		if back.Rule != "8080 localhost:80" || back.Note != "web" {
			t.Errorf("roundtrip mismatch: %+v", back)
		}
	})

	t.Run("marshal excludes source", func(t *testing.T) {
		t.Parallel()
		r := PortForwardRule{Rule: "8080 localhost:80", Note: "web", Source: PortForwardSourceSshConfig}
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		if strings.Contains(string(b), "source") || strings.Contains(string(b), "sshconfig") {
			t.Errorf("source should not be serialized: %s", string(b))
		}
	})
}

// TestPortForwardRuleReadPath exercises the full config read path: raw JSON →
// readConfigHelper (untyped MetaMapType) → utilfn.ReUnmarshal into the typed
// Connections map. This is the exact path ReadFullConfig uses, so it verifies
// the string-or-object PortForwardRule form survives the reflection roundtrip.
func TestPortForwardRuleReadPath(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"myhost": {"ssh:localforward": ["8080 localhost:80", {"rule":"9090 localhost:90","note":"web","enabled":false}], "ssh:remoteforward": ["3000 localhost:3000"]}}`)
	meta, cerrs := readConfigHelper("connections.json", raw, nil)
	if len(cerrs) > 0 {
		t.Fatalf("config errors: %v", cerrs)
	}
	var conns map[string]ConnKeywords
	if err := utilfn.ReUnmarshal(&conns, meta); err != nil {
		t.Fatalf("reunmarshal error: %v", err)
	}
	myhost, ok := conns["myhost"]
	if !ok {
		t.Fatal("missing myhost")
	}

	if len(myhost.SshLocalForward) != 2 {
		t.Fatalf("expected 2 local forwards, got %d", len(myhost.SshLocalForward))
	}
	if myhost.SshLocalForward[0].Rule != "8080 localhost:80" {
		t.Errorf("unexpected rule 0: %+v", myhost.SshLocalForward[0])
	}
	if myhost.SshLocalForward[1].Rule != "9090 localhost:90" || myhost.SshLocalForward[1].Note != "web" {
		t.Errorf("unexpected rule 1: %+v", myhost.SshLocalForward[1])
	}
	if myhost.SshLocalForward[1].Enabled == nil || *myhost.SshLocalForward[1].Enabled {
		t.Errorf("expected enabled=false, got %v", myhost.SshLocalForward[1].Enabled)
	}
	if myhost.SshLocalForward[1].Source != "" {
		t.Errorf("expected empty source for connections.json rule, got %q", myhost.SshLocalForward[1].Source)
	}
	if len(myhost.SshRemoteForward) != 1 || myhost.SshRemoteForward[0].Rule != "3000 localhost:3000" {
		t.Errorf("unexpected remote forwards: %+v", myhost.SshRemoteForward)
	}
}
