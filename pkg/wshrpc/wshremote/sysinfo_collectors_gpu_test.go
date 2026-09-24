// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"errors"
	"testing"
)

func withFakeExec(t *testing.T, fn func(name string, args ...string) ([]byte, error)) {
	t.Helper()
	orig := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = orig })
}

func TestNvidiaCollectorParsesMultiGpuCsv(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte("0, 45, 2048, 8192, 62\n1, 12, 512, 8192, 51\n"), nil
	})
	c := MakeNvidiaGpuCollector()
	if !c.Probe() {
		t.Fatal("expected probe true with fake exec returning valid output")
	}
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]float64{
		"gpu:0:util": 45, "gpu:0:vram": 2048, "gpu:0:temp": 62,
		"gpu:1:util": 12, "gpu:1:vram": 512, "gpu:1:temp": 51,
	}
	for k, v := range want {
		if values[k] != v {
			t.Errorf("values[%q] = %v, want %v", k, values[k], v)
		}
	}
}

func TestNvidiaCollectorProbeFalseWhenCommandMissing(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("exec: \"nvidia-smi\": executable file not found in $PATH")
	})
	c := MakeNvidiaGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false when nvidia-smi is not installed")
	}
}

func TestNvidiaCollectorRejectsMalformedOutput(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte("not,csv,at,all,garbage\n"), nil
	})
	c := MakeNvidiaGpuCollector()
	c.Probe()
	if _, err := c.Collect(); err == nil {
		t.Fatal("expected an error on malformed CSV, got nil")
	}
}

func TestNvidiaCollectorRejectsEmptyOutput(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(""), nil
	})
	c := MakeNvidiaGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false on empty output (no GPUs reported)")
	}
}
