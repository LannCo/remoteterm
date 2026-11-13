// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellexec

import (
	"errors"
	"os/exec"
	"runtime"
	"sync"
	"testing"
)

func TestBuildShellCommandLine(t *testing.T) {
	tests := []struct {
		name      string
		shellPath string
		shellOpts []string
		cmdStr    string
		want      string
	}{
		{
			name:      "simple command passes through unquoted",
			shellPath: "/bin/bash",
			shellOpts: nil,
			cmdStr:    "date",
			want:      "/bin/bash -c date",
		},
		{
			name:      "multi-word command is quoted as one word",
			shellPath: "/bin/bash",
			shellOpts: nil,
			cmdStr:    "sleep 0.5",
			want:      `/bin/bash -c "sleep 0.5"`,
		},
		{
			name:      "double quotes inside command are escaped",
			shellPath: "/bin/bash",
			shellOpts: nil,
			cmdStr:    `echo "hi"`,
			want:      `/bin/bash -c "echo \"hi\""`,
		},
		{
			name:      "dollar expansion is deferred to the session shell",
			shellPath: "/bin/bash",
			shellOpts: nil,
			cmdStr:    "echo $HOME",
			want:      `/bin/bash -c "echo \$HOME"`,
		},
		{
			name:      "empty cmdStr omits -c entirely",
			shellPath: "/bin/bash",
			shellOpts: []string{"--rcfile", "/tmp/rcfile"},
			cmdStr:    "",
			want:      "/bin/bash --rcfile /tmp/rcfile",
		},
		{
			name:      "existing shell opts are preserved",
			shellPath: "/bin/bash",
			shellOpts: []string{"-l"},
			cmdStr:    "date",
			want:      "/bin/bash -l -c date",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildShellCommandLine(tt.shellPath, tt.shellOpts, tt.cmdStr)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestSessionWrapExitCode(t *testing.T) {
	t.Run("nil wait error means exit 0", func(t *testing.T) {
		sw := SessionWrap{WaitOnce: &sync.Once{}}
		if code := sw.ExitCode(); code != 0 {
			t.Fatalf("expected 0 for nil WaitErr, got %d", code)
		}
	})
	t.Run("unknown wait error returns -1", func(t *testing.T) {
		sw := SessionWrap{WaitOnce: &sync.Once{}, WaitErr: errors.New("connection reset")}
		if code := sw.ExitCode(); code != -1 {
			t.Fatalf("expected -1 for unknown error, got %d", code)
		}
	})
	t.Run("exec exit error returns real exit code", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("posix-only test process")
		}
		cmd := exec.Command("sh", "-c", "exit 7")
		err := cmd.Run()
		if err == nil {
			t.Fatalf("expected non-zero exit from test command")
		}
		sw := SessionWrap{WaitOnce: &sync.Once{}, WaitErr: err}
		if code := sw.ExitCode(); code != 7 {
			t.Fatalf("expected 7, got %d", code)
		}
	})
	// The *ssh.ExitError branch (ExitStatus for non-zero remote exits) cannot be
	// unit-tested here: ssh.Waitmsg has unexported fields and no public constructor.
	// It mirrors the existing ExitSignal type assertion and is covered by live
	// remote runs (wsh run --wait --json over SSH).
}