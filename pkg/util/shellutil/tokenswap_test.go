// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"strconv"
	"strings"
	"testing"
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
