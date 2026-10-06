// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/tsunami/build"
)

var previewTokenPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// The fake app prints what a Tsunami app prints and echoes the token it was given.
func useFakeTokenApp(t *testing.T, port string) {
	t.Helper()
	orig := compileApp
	compileApp = func(ctx context.Context, appNS string, appPath string, oc *build.OutputCapture) (string, error) {
		binPath, err := GetBuilderAppExecutablePath(appPath)
		if err != nil {
			return "", err
		}
		script := "#!/bin/sh\necho \"[tsunami] listening at http://localhost:" + port + "\"\necho \"TOKEN=$TSUNAMI_AUTHTOKEN\"\nexec sleep 30\n"
		return binPath, os.WriteFile(binPath, []byte(script), 0755)
	}
	t.Cleanup(func() { compileApp = orig })
}

func runFakeApp(t *testing.T, name string, env map[string]string) (*BuilderController, string) {
	t.Helper()
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, name)
	if err := os.WriteFile(filepath.Join(appDir, "manifest.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	bc := GetOrCreateController("test-token-" + name)
	t.Cleanup(func() { DeleteController("test-token-" + name) })
	bc.RequestUserRebuild("draft/"+name, env)
	waitUntil(t, 5*time.Second, func() bool { return bc.GetStatus().Status == BuilderStatus_Running }, "the fake app to run")
	return bc, "draft/" + name
}

func outputTokenOf(t *testing.T, bc *BuilderController) string {
	t.Helper()
	var token string
	waitUntil(t, 3*time.Second, func() bool {
		for _, line := range bc.outputBuffer.GetLines() {
			if rest, ok := strings.CutPrefix(line, "TOKEN="); ok {
				token = rest
				return token != ""
			}
		}
		return false
	}, "the app to print its token")
	return token
}

func TestRunGivesTheAppAFreshPreviewToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the app")
	}
	useFakeTokenApp(t, "4321")
	bc, _ := runFakeApp(t, "tok-fresh", nil)

	port, token := bc.GetPreviewAuth()
	if port != 4321 {
		t.Fatalf("preview port %d, want 4321", port)
	}
	if !previewTokenPattern.MatchString(token) {
		t.Fatalf("preview token %q is not 32 random bytes in hex", token)
	}
	if got := outputTokenOf(t, bc); got != token {
		t.Fatalf("the app received token %q, the controller holds %q", got, token)
	}

	bc.RequestUserRebuild("draft/tok-fresh", nil)
	waitUntil(t, 5*time.Second, func() bool {
		_, next := bc.GetPreviewAuth()
		return next != "" && next != token && bc.GetStatus().Status == BuilderStatus_Running
	}, "the second run to get its own token")
}

func TestUserEnvCannotChooseThePreviewToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the app")
	}
	useFakeTokenApp(t, "4322")
	env := map[string]string{"TSUNAMI_AUTHTOKEN": "chosen-by-the-user"}
	bc, _ := runFakeApp(t, "tok-env", env)
	_, token := bc.GetPreviewAuth()
	if token == "chosen-by-the-user" || !previewTokenPattern.MatchString(token) {
		t.Fatalf("preview token %q was taken from the user's environment", token)
	}
	if got := outputTokenOf(t, bc); got != token {
		t.Fatalf("the app received %q, want the generated token", got)
	}
	if env["TSUNAMI_AUTHTOKEN"] != "chosen-by-the-user" {
		t.Fatal("the stored builder environment was rewritten")
	}
}

func TestNoPreviewAuthWhileNothingRuns(t *testing.T) {
	bc := makeBuilderController("test-token-idle")
	if port, token := bc.GetPreviewAuth(); port != 0 || token != "" {
		t.Fatalf("an idle controller reports preview auth %d / %q", port, token)
	}
}
