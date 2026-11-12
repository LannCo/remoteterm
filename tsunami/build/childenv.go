// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import "strings"

// The builder runs a Go toolchain, Tailwind and the built app, none of which should see
// API keys or an SSH agent that the app process happens to hold. A name list rather than
// a "GO" prefix: GOOGLE_API_KEY and GOPASS_* start with GO too.
var childEnvNames = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "LANG": true, "LANGUAGE": true, "TZ": true,
	"TMPDIR": true, "TMP": true, "TEMP": true,

	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,

	"CC": true, "CXX": true, "DEVELOPER_DIR": true, "SDKROOT": true,

	// Names from `go help environment` (Go 1.26).
	"GO111MODULE": true, "GOARCH": true, "GOAUTH": true, "GOBIN": true, "GOCACHE": true, "GOCACHEPROG": true,
	"GODEBUG": true, "GOENV": true, "GOFLAGS": true, "GOINSECURE": true, "GOMODCACHE": true, "GONOPROXY": true,
	"GONOSUMDB": true, "GONOSUMCHECK": true, "GOOS": true, "GOPATH": true, "GOPRIVATE": true, "GOPROXY": true,
	"GOROOT": true, "GOSUMDB": true, "GOTMPDIR": true, "GOTOOLCHAIN": true, "GOVCS": true, "GOWORK": true,
	"GO386": true, "GOAMD64": true, "GOARM": true, "GOARM64": true, "GOMIPS": true, "GOMIPS64": true,
	"GOPPC64": true, "GORISCV64": true, "GOWASM": true, "GOCOVERDIR": true, "GOEXPERIMENT": true,
	"GOFIPS140": true, "GO_EXTLINK_ENABLED": true, "GOTELEMETRY": true, "GOTELEMETRYDIR": true,
	"GOMAXPROCS": true, "GOGC": true, "GOMEMLIMIT": true, "GOTRACEBACK": true,

	// Windows cannot start a process or resolve a profile without these.
	"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true,
	"USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true,
	"PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "PROGRAMW6432": true, "COMMONPROGRAMFILES": true,
}

var childEnvPrefixes = []string{"LC_", "SSL_CERT_", "CGO_", "XDG_"}

func childEnvAllowed(key string) bool {
	upper := strings.ToUpper(key)
	if childEnvNames[upper] {
		return true
	}
	for _, prefix := range childEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// AllowlistedEnv returns the allowed entries of environ followed by extra; an extra
// replaces any earlier entry with the same name, so it can pin a value the caller's
// environment tried to set (GOTOOLCHAIN) or add one that is never inherited
// (ELECTRON_RUN_AS_NODE).
func AllowlistedEnv(environ []string, extra ...string) []string {
	out := make([]string, 0, len(environ)+len(extra))
	index := make(map[string]int, len(environ)+len(extra))
	add := func(entry string, filter bool) {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return
		}
		if filter && !childEnvAllowed(key) {
			return
		}
		if i, seen := index[key]; seen {
			out[i] = entry
			return
		}
		index[key] = len(out)
		out = append(out, entry)
	}
	for _, entry := range environ {
		add(entry, true)
	}
	for _, entry := range extra {
		add(entry, false)
	}
	return out
}
