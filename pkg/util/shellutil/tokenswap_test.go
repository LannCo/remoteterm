// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func TestRedactSecret(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	tests := []struct {
		name   string
		secret string
		want   string
	}{
		{"empty", "", ""},
		{"one char", "x", "<redacted>"},
		{"exactly eight", "abcdefgh", "<redacted>"},
		{"nine", "abcdefghi", "abcdefgh...<redacted, len=9>"},
		{"jwt", jwt, "eyJhbGci...<redacted, len=" + strconv.Itoa(len(jwt)) + ">"},
		{"swap token", "c3dhcHRva2VuOnNlY3JldHZhbHVl", "c3dhcHRv...<redacted, len=28>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactSecret(tc.secret)
			if got != tc.want {
				t.Fatalf("RedactSecret(%q) = %q, want %q", tc.secret, got, tc.want)
			}
			if len(tc.secret) > 8 && strings.Contains(got, tc.secret) {
				t.Fatalf("RedactSecret output contains the full secret: %q", got)
			}
		})
	}
}

func TestRedactSecretEnv(t *testing.T) {
	if RedactSecretEnv(nil) != nil {
		t.Fatalf("RedactSecretEnv(nil) should be nil")
	}
	secrets := map[string]string{
		remotetermbase.WaveSwapTokenVarName:      "c3dhcHRva2VuOnNlY3JldHZhbHVl",
		remotetermbase.WaveJwtTokenVarName:       "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJ4In0.c2lnbmF0dXJl",
		remotetermbase.LegacyWaveJwtTokenVarName: "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJ5In0.c2lnbmF0dXJl",
	}
	env := map[string]string{"TERM": "xterm-256color", "HOME": "/home/u"}
	for k, v := range secrets {
		env[k] = v
	}
	got := RedactSecretEnv(env)
	if got["TERM"] != "xterm-256color" || got["HOME"] != "/home/u" {
		t.Fatalf("non-secret vars altered: %v", got)
	}
	formatted := fmt.Sprintf("%v", got)
	for k, v := range secrets {
		if got[k] != RedactSecret(v) {
			t.Fatalf("%s = %q, want %q", k, got[k], RedactSecret(v))
		}
		if strings.Contains(formatted, v) {
			t.Fatalf("formatted env still contains %s value", k)
		}
		if env[k] != v {
			t.Fatalf("input map was mutated for %s", k)
		}
	}
}
