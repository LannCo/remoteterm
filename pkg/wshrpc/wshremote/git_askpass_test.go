// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/pkg/wshrpc"
)

// gitCredentialFill drives the askpass script through git itself: with no credential
// helpers configured, `git credential fill` prompts via GIT_ASKPASS exactly as push does.
func gitCredentialFill(t *testing.T, username, password string) (string, string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	scriptPath, err := writeGitAskpassScript()
	if err != nil {
		t.Fatalf("writeGitAskpassScript: %v", err)
	}
	defer os.Remove(scriptPath)
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if strings.Contains(string(script), password) || strings.Contains(string(script), username) {
		t.Fatalf("askpass script on disk contains a credential")
	}
	cmd := exec.Command("git", "credential", "fill")
	cmd.Env = append(os.Environ(), gitAskpassEnv(scriptPath, username, password)...)
	cmd.Stdin = strings.NewReader("protocol=https\nhost=example.invalid\n\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git credential fill: %v\n%s", err, out)
	}
	var gotUser, gotPass string
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "username="); ok {
			gotUser = v
		}
		if v, ok := strings.CutPrefix(line, "password="); ok {
			gotPass = v
		}
	}
	return gotUser, gotPass
}

func TestGitAskpassDoesNotShellExpandCredentials(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	cases := []struct {
		name     string
		username string
		password string
	}{
		{"command substitution", "user", "x$(touch " + marker + ")y"},
		{"backticks", "user", "x`touch " + marker + "`y"},
		{"variable and pid", "user", "pa$$word$HOME"},
		{"quotes and backslash", "us\"er'", `a\tb"c'd\`},
		{"injection in username", "$(touch " + marker + ")", "pw"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotUser, gotPass := gitCredentialFill(t, tc.username, tc.password)
			if gotUser != tc.username {
				t.Errorf("username = %q, want %q", gotUser, tc.username)
			}
			if gotPass != tc.password {
				t.Errorf("password = %q, want %q", gotPass, tc.password)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("credential was executed by the shell: %s exists", marker)
			}
		})
	}
}

func TestGitPushRejectsOptionLikeRemoteAndBranch(t *testing.T) {
	impl := &ServerImpl{}
	marker := filepath.Join(t.TempDir(), "pwned")
	for _, data := range []wshrpc.CommandGitPushData{
		{Dir: t.TempDir(), Remote: "--receive-pack=touch " + marker, Branch: "main"},
		{Dir: t.TempDir(), Remote: "origin", Branch: "--receive-pack=touch " + marker},
	} {
		_, err := impl.GitPushCommand(context.Background(), data)
		if err == nil {
			t.Fatalf("GitPushCommand accepted option-like remote/branch %q/%q", data.Remote, data.Branch)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("option-like argument was executed: %s exists", marker)
	}
}
