// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"strings"
	"testing"
)

// Every shell template must warn (interactively) when the token swap fails.
// The swap token is one-time and the app's token table is in-memory, so an app
// restart orphans every long-lived remote shell: without the warning, such a
// shell silently loses all wsh RPC. See .pi/specs/token-swap-failure-warning.md.
func TestShellTemplatesWarnOnTokenSwapFailure(t *testing.T) {
	const warnMarker = "[remoteterm] wsh RPC disabled"

	tests := []struct {
		name              string
		template          string
		unsetMarker       string // statement that clears REMOTETERM_SWAPTOKEN
		interactiveMarker string // per-shell interactivity test ("" = warn unconditionally)
		stderrSuppressed  bool   // swap line must keep stderr suppressed
	}{
		{name: "bash", template: BashStartup_Bashrc, unsetMarker: "unset REMOTETERM_SWAPTOKEN", interactiveMarker: `"$-" == *i*`, stderrSuppressed: true},
		{name: "zsh", template: ZshStartup_Zshrc, unsetMarker: "unset REMOTETERM_SWAPTOKEN", interactiveMarker: "-o interactive", stderrSuppressed: true},
		{name: "fish", template: FishStartup_Wavefish, unsetMarker: "set -e REMOTETERM_SWAPTOKEN", interactiveMarker: "status is-interactive", stderrSuppressed: true},
		{name: "pwsh", template: PwshStartup_wavepwsh, unsetMarker: "Remove-Item Env:REMOTETERM_SWAPTOKEN", interactiveMarker: "", stderrSuppressed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.template, warnMarker) {
				t.Fatalf("template does not contain the token-swap warning marker %q", warnMarker)
			}
			if !strings.Contains(tt.template, tt.unsetMarker) {
				t.Fatalf("template no longer clears REMOTETERM_SWAPTOKEN (expected %q)", tt.unsetMarker)
			}
			if tt.interactiveMarker != "" && !strings.Contains(tt.template, tt.interactiveMarker) {
				t.Fatalf("template does not gate the warning on interactivity (expected %q)", tt.interactiveMarker)
			}
			if tt.stderrSuppressed && !strings.Contains(tt.template, "2>/dev/null") {
				t.Fatalf("template no longer suppresses stderr on the swap line; non-interactive shells would emit noise")
			}
			if tt.name == "pwsh" && !strings.Contains(tt.template, "$LASTEXITCODE") {
				t.Fatalf("pwsh template does not check $LASTEXITCODE after the token swap")
			}
		})
	}
}