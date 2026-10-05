// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"sort"
	"strings"
	"testing"
)

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

func TestAllowlistedEnvKeepsWhatToolchainsNeed(t *testing.T) {
	in := []string{
		"PATH=/usr/bin", "HOME=/Users/a", "TMPDIR=/tmp/x", "LANG=en_US.UTF-8", "LC_ALL=C", "LC_CTYPE=UTF-8",
		"GOPATH=/g", "GOFLAGS=-mod=mod", "GOPROXY=https://p", "GOPRIVATE=corp.example", "GOCACHE=/c", "GO111MODULE=on",
		"CGO_ENABLED=0", "CC=clang", "DEVELOPER_DIR=/Library/Developer", "SDKROOT=/sdk",
		"HTTP_PROXY=http://h", "HTTPS_PROXY=http://hs", "NO_PROXY=localhost", "ALL_PROXY=socks5://a",
		"http_proxy=http://h", "https_proxy=http://hs", "no_proxy=localhost",
		"SSL_CERT_FILE=/etc/ca.pem", "SSL_CERT_DIR=/etc/certs",
		"XDG_CACHE_HOME=/xdg",
	}
	got := envMap(AllowlistedEnv(in))
	for _, kv := range in {
		k, v, _ := strings.Cut(kv, "=")
		if got[k] != v {
			t.Errorf("%s was dropped or changed: got %q want %q", k, got[k], v)
		}
	}
}

func TestAllowlistedEnvDropsSecretsAndLookalikes(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=sk-x", "OPENAI_API_KEY=sk-y", "GITHUB_TOKEN=gh", "AWS_SECRET_ACCESS_KEY=aws",
		"SSH_AUTH_SOCK=/tmp/agent.1", "NPM_TOKEN=n", "REMOTETERM_AUTH_KEY=k",
		"GOOGLE_API_KEY=g", "GOPASS_PASSWORD=p", "GOLDEN=1", "CGOCRYPT=1",
		"SSL_CERTIFICATE_PASSWORD=z",
		"ELECTRON_RUN_AS_NODE=1",
	}
	got := envMap(AllowlistedEnv(in))
	if got["PATH"] != "/usr/bin" {
		t.Fatalf("PATH lost: %v", got)
	}
	for _, k := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "SSH_AUTH_SOCK", "NPM_TOKEN",
		"REMOTETERM_AUTH_KEY", "GOOGLE_API_KEY", "GOPASS_PASSWORD", "GOLDEN", "CGOCRYPT", "SSL_CERTIFICATE_PASSWORD",
	} {
		if _, ok := got[k]; ok {
			t.Errorf("%s leaked into the child environment", k)
		}
	}
	if _, ok := got["ELECTRON_RUN_AS_NODE"]; ok {
		t.Error("ELECTRON_RUN_AS_NODE passed through the base list; only the Tailwind run asks for it")
	}
}

func TestAllowlistedEnvExtrasOverrideAndAppend(t *testing.T) {
	in := []string{"PATH=/usr/bin", "GOTOOLCHAIN=auto", "GOROOT=/old", "GOFLAGS=-mod=mod"}
	out := AllowlistedEnv(in, "GOTOOLCHAIN=local", "ELECTRON_RUN_AS_NODE=1", "TSUNAMI_CLOSEONSTDIN=1")
	got := envMap(out)
	if got["GOTOOLCHAIN"] != "local" {
		t.Errorf("extra did not override: %q", got["GOTOOLCHAIN"])
	}
	if got["ELECTRON_RUN_AS_NODE"] != "1" || got["TSUNAMI_CLOSEONSTDIN"] != "1" {
		t.Errorf("extras missing: %v", got)
	}
	if got["GOFLAGS"] != "-mod=mod" || got["GOROOT"] != "/old" {
		t.Errorf("unrelated allowed entries changed: %v", got)
	}
	count := 0
	for _, kv := range out {
		if strings.HasPrefix(kv, "GOTOOLCHAIN=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("GOTOOLCHAIN appears %d times, want exactly once", count)
	}
}

func TestAllowlistedEnvMatchesNamesCaseInsensitively(t *testing.T) {
	got := envMap(AllowlistedEnv([]string{"Path=C:\\Windows", "SystemRoot=C:\\Windows", "ComSpec=cmd.exe", "Api_Key=x"}))
	for _, k := range []string{"Path", "SystemRoot", "ComSpec"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%s dropped", k)
		}
	}
	if _, ok := got["Api_Key"]; ok {
		t.Error("Api_Key leaked")
	}
}

// The built app's explicit environment (the per-run preview token, secrets bound to the
// app, the user's builder:env entries) is appended after the filtered inherited part by
// the caller, and anything handed to AllowlistedEnv as an extra is never filtered.
func TestExplicitEnvironmentIsNotFilteredOnlyTheInheritedPart(t *testing.T) {
	inherited := []string{"PATH=/usr/bin", "TSUNAMI_AUTHTOKEN=inherited-stale", "MY_SERVICE_API_KEY=inherited"}
	explicit := []string{"TSUNAMI_AUTHTOKEN=run-token", "TSUNAMI_CORS=http://localhost:5173", "MY_SERVICE_API_KEY=bound-secret", "ANTHROPIC_API_KEY=user-chose-this"}

	viaExtras := envMap(AllowlistedEnv(inherited, explicit...))
	for _, kv := range explicit {
		k, v, _ := strings.Cut(kv, "=")
		if viaExtras[k] != v {
			t.Errorf("explicit %s did not survive: got %q want %q", k, viaExtras[k], v)
		}
	}

	composed := append(AllowlistedEnv(inherited, "TSUNAMI_CLOSEONSTDIN=1"), explicit...)
	got := envMap(composed)
	if got["TSUNAMI_AUTHTOKEN"] != "run-token" || got["MY_SERVICE_API_KEY"] != "bound-secret" || got["ANTHROPIC_API_KEY"] != "user-chose-this" {
		t.Errorf("explicit entries appended after the filtered part were altered: %v", got)
	}
	if got["TSUNAMI_CLOSEONSTDIN"] != "1" {
		t.Errorf("TSUNAMI_CLOSEONSTDIN missing: %v", got)
	}
}

func TestAllowlistedEnvIgnoresMalformedEntries(t *testing.T) {
	out := AllowlistedEnv([]string{"", "=C:=C:\\x", "NOEQUALS", "PATH=/p", "=PATH"})
	sort.Strings(out)
	if len(out) != 1 || out[0] != "PATH=/p" {
		t.Errorf("got %v", out)
	}
}
