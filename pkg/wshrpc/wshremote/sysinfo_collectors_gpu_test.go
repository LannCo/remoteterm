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
		"gpu:0:util":      45, "gpu:0:vram": 2048, "gpu:0:vramtotal": 8192, "gpu:0:temp": 62,
		"gpu:1:util":      12, "gpu:1:vram": 512, "gpu:1:vramtotal": 8192, "gpu:1:temp": 51,
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

func TestAmdCollectorParsesRocmSmiJson(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(`{
			"card0": {
				"GPU use (%)": "45",
				"VRAM Total Used Memory (B)": "2147483648",
				"VRAM Total Memory (B)": "17179869184",
				"Temperature (Sensor edge) (C)": "62.0"
			}
		}`), nil
	})
	c := MakeAmdGpuCollector()
	if !c.Probe() {
		t.Fatal("expected probe true with fake exec returning valid JSON")
	}
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["gpu:0:util"] != 45 {
		t.Errorf("gpu:0:util = %v, want 45", values["gpu:0:util"])
	}
	wantVramMb := 2147483648.0 / (1024 * 1024)
	if values["gpu:0:vram"] != wantVramMb {
		t.Errorf("gpu:0:vram = %v, want %v", values["gpu:0:vram"], wantVramMb)
	}
	wantVramTotalMb := 17179869184.0 / (1024 * 1024)
	if values["gpu:0:vramtotal"] != wantVramTotalMb {
		t.Errorf("gpu:0:vramtotal = %v, want %v", values["gpu:0:vramtotal"], wantVramTotalMb)
	}
	if values["gpu:0:temp"] != 62.0 {
		t.Errorf("gpu:0:temp = %v, want 62.0", values["gpu:0:temp"])
	}
}

func TestAmdCollectorProbeFalseOnMissingCommand(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("exec: \"rocm-smi\": executable file not found in $PATH")
	})
	c := MakeAmdGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false when rocm-smi is not installed")
	}
}

func TestAmdCollectorRejectsMalformedJson(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte("not json at all"), nil
	})
	c := MakeAmdGpuCollector()
	c.Probe()
	if _, err := c.Collect(); err == nil {
		t.Fatal("expected an error on malformed JSON, got nil")
	}
}

func TestAmdCollectorRejectsEmptyOutput(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) ([]byte, error) {
		return []byte(""), nil
	})
	c := MakeAmdGpuCollector()
	if c.Probe() {
		t.Fatal("expected probe false on empty output")
	}
}
