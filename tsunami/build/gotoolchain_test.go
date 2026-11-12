// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestToolchainKeep(t *testing.T) {
	keep := []string{
		"VERSION", "go.env", "LICENSE", "PATENTS",
		"bin/go", "bin/gofmt", "bin/go.exe", "bin/gofmt.exe",
		"pkg/include/textflag.h", "pkg/include/asm_amd64.h",
		"pkg/tool/darwin_arm64/compile", "pkg/tool/darwin_arm64/link", "pkg/tool/darwin_arm64/asm",
		"pkg/tool/darwin_arm64/cgo", "pkg/tool/darwin_arm64/preprofile", "pkg/tool/windows_amd64/compile.exe",
		"lib/time/zoneinfo.zip",
		"src/runtime/proc.go", "src/net/http/server.go", "src/go.mod", "src/vendor/modules.txt",
		"src/vendor/golang.org/x/net/dns/dnsmessage/message.go", "src/runtime/asm_arm64.s", "src/runtime/cgo/gcc_darwin_arm64.c",
		"src/testing/testing.go", "src/net/http/httptest/server.go", "src/crypto/internal/fips140/sha256/sha256block_arm64.s",
	}
	drop := []string{
		"doc/go_spec.html", "test/fixedbugs/bug001.go", "misc/wasm/wasm_exec.js", "api/go1.20.txt", "codereview.cfg",
		"README.md", "CONTRIBUTING.md", "SECURITY.md", "go.mod",
		"pkg/tool/darwin_arm64/vet", "pkg/tool/darwin_arm64/cover", "pkg/tool/darwin_arm64/fix", "pkg/tool/darwin_arm64/nm",
		"pkg/tool/darwin_arm64/pprof", "pkg/tool/darwin_arm64/trace", "pkg/tool/darwin_arm64/objdump",
		"lib/fips140/v1.0.0.zip", "lib/wasm/wasm_exec.js", "lib/hg/hg.rc",
		"src/cmd/go/main.go", "src/cmd/compile/internal/ssa/rewrite.go", "src/cmd/vendor/golang.org/x/mod/modfile/rule.go",
		"src/net/http/server_test.go", "src/runtime/testdata/testprog/main.go", "src/go/build/testdata/x.go",
		"src/crypto/internal/fips140/check/check_test.go",
	}
	for _, rel := range keep {
		if !ToolchainKeep(rel) {
			t.Errorf("ToolchainKeep(%q) = false, want true", rel)
		}
	}
	for _, rel := range drop {
		if ToolchainKeep(rel) {
			t.Errorf("ToolchainKeep(%q) = true, want false", rel)
		}
	}
}

var toolchainFixture = map[string]string{
	"VERSION":                      "go1.26.2\ntime 2026-04-01T00:00:00Z\n",
	"go.env":                       "GOTOOLCHAIN=auto\n",
	"bin/go":                       "#!go",
	"bin/gofmt":                    "#!gofmt",
	"pkg/include/textflag.h":       "// flags",
	"pkg/tool/linux_amd64/compile": "compile",
	"pkg/tool/linux_amd64/vet":     "vet",
	"src/runtime/proc.go":          "package runtime",
	"src/runtime/proc_test.go":     "package runtime",
	"src/cmd/go/main.go":           "package main",
	"src/net/testdata/big.txt":     "big",
	"doc/go_spec.html":             "spec",
	"test/bug.go":                  "package main",
	"lib/time/zoneinfo.zip":        "zip",
	"lib/wasm/wasm_exec.js":        "js",
}

var toolchainFixtureKept = []string{
	"VERSION", "go.env", "bin/go", "bin/gofmt", "pkg/include/textflag.h", "pkg/tool/linux_amd64/compile",
	"src/runtime/proc.go", "lib/time/zoneinfo.zip",
}

func writeToolchainFixture(t *testing.T, root string) {
	t.Helper()
	for rel, content := range toolchainFixture {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0644)
		if strings.HasPrefix(rel, "bin/") || strings.HasPrefix(rel, "pkg/tool/") {
			mode = 0755
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func checkKept(t *testing.T, root string) {
	t.Helper()
	got := listFiles(t, root)
	want := make(map[string]bool)
	for _, rel := range toolchainFixtureKept {
		want[rel] = true
	}
	for _, rel := range got {
		if !want[rel] {
			t.Errorf("unexpected file kept: %s", rel)
		}
		delete(want, rel)
	}
	for rel := range want {
		t.Errorf("missing file: %s", rel)
	}
}

func TestTrimToolchainDirKeepsOnlyWhatBuildsNeed(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "gotoolchain")
	writeToolchainFixture(t, src)
	if err := TrimToolchainDir(src, dst); err != nil {
		t.Fatal(err)
	}
	checkKept(t, dst)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "bin", "go"))
		if err != nil || info.Mode()&0111 == 0 {
			t.Errorf("bin/go lost its executable bit: %v %v", info, err)
		}
		info, err = os.Stat(filepath.Join(dst, "pkg", "tool", "linux_amd64", "compile"))
		if err != nil || info.Mode()&0111 == 0 {
			t.Errorf("compile lost its executable bit: %v %v", info, err)
		}
	}
}

func TestTrimToolchainDirReplacesPreviousOutput(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "gotoolchain")
	writeToolchainFixture(t, src)
	if err := os.MkdirAll(filepath.Join(dst, "doc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "doc", "stale.html"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := TrimToolchainDir(src, dst); err != nil {
		t.Fatal(err)
	}
	checkKept(t, dst)
}

func TestTrimToolchainDirRejectsAnythingThatIsNotAGoRoot(t *testing.T) {
	if err := TrimToolchainDir(t.TempDir(), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("an empty directory was accepted as a Go root")
	}
}

func makeToolchainTarGz(t *testing.T, entries map[string]string, extra func(tw *tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for rel, content := range entries {
		mode := int64(0644)
		if strings.HasPrefix(rel, "bin/") || strings.HasPrefix(rel, "pkg/tool/") {
			mode = 0755
		}
		if err := tw.WriteHeader(&tar.Header{Name: "go/" + rel, Mode: mode, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if extra != nil {
		extra(tw)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractToolchainTarGzTrimsWhileExtracting(t *testing.T) {
	archive := makeToolchainTarGz(t, toolchainFixture, nil)
	dst := filepath.Join(t.TempDir(), "gotoolchain")
	if err := ExtractToolchainTarGz(bytes.NewReader(archive), dst); err != nil {
		t.Fatal(err)
	}
	checkKept(t, dst)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "bin", "go"))
		if err != nil || info.Mode()&0111 == 0 {
			t.Errorf("bin/go lost its executable bit: %v %v", info, err)
		}
	}
}

func TestExtractToolchainTarGzRefusesPathEscape(t *testing.T) {
	archive := makeToolchainTarGz(t, map[string]string{"VERSION": "go1.26.2"}, func(tw *tar.Writer) {
		body := "pwned"
		_ = tw.WriteHeader(&tar.Header{Name: "go/src/../../../escape.go", Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	})
	base := t.TempDir()
	dst := filepath.Join(base, "a", "b", "gotoolchain")
	if err := ExtractToolchainTarGz(bytes.NewReader(archive), dst); err == nil {
		t.Fatal("a path-escaping entry was accepted")
	}
	if _, err := os.Stat(filepath.Join(base, "escape.go")); err == nil {
		t.Fatal("the escaping entry was written")
	}
}

func TestExtractToolchainTarGzRefusesKeptSymlink(t *testing.T) {
	archive := makeToolchainTarGz(t, map[string]string{"VERSION": "go1.26.2"}, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "go/bin/go", Linkname: "/bin/sh", Typeflag: tar.TypeSymlink, Mode: 0755})
	})
	if err := ExtractToolchainTarGz(bytes.NewReader(archive), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("a symlinked binary was accepted")
	}
}

func TestToolchainArchiveName(t *testing.T) {
	name, err := ToolchainArchiveName("1.26.2", "darwin", "arm64")
	if err != nil || name != "go1.26.2.darwin-arm64.tar.gz" {
		t.Fatalf("got %q, %v", name, err)
	}
	if _, err := ToolchainArchiveName("1.26.2", "windows", "amd64"); err == nil {
		t.Error("windows ships a zip; it must be refused rather than fetched as a tarball")
	}
	if _, err := ToolchainArchiveName("1.26.2/../x", "darwin", "arm64"); err == nil {
		t.Error("a version containing a path separator was accepted")
	}
}

func TestPinnedToolchainChecksumsAreWellFormed(t *testing.T) {
	for _, arch := range []string{"arm64", "amd64"} {
		name, _ := ToolchainArchiveName(PinnedToolchainVersion, "darwin", arch)
		sum, ok := PinnedToolchainSHA256(name)
		if !ok {
			t.Fatalf("no pinned checksum for %s", name)
		}
		if len(sum) != 64 {
			t.Errorf("%s: checksum %q is not 64 hex characters", name, sum)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func serveArchive(t *testing.T, name string, body []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestFetchToolchainVerifiesChecksumBeforeExtracting(t *testing.T) {
	archive := makeToolchainTarGz(t, toolchainFixture, nil)
	sum := sha256.Sum256(archive)
	name := "go1.26.2.darwin-arm64.tar.gz"
	server := serveArchive(t, name, archive)

	dst := filepath.Join(t.TempDir(), "gotoolchain")
	err := FetchToolchain(context.Background(), FetchToolchainOpts{
		Version: "1.26.2", GOOS: "darwin", GOARCH: "arm64", DstRoot: dst,
		BaseURL: server.URL + "/dl/", SHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	checkKept(t, dst)

	badDst := filepath.Join(t.TempDir(), "gotoolchain")
	err = FetchToolchain(context.Background(), FetchToolchainOpts{
		Version: "1.26.2", GOOS: "darwin", GOARCH: "arm64", DstRoot: badDst,
		BaseURL: server.URL + "/dl/", SHA256: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a wrong checksum must fail with a checksum error, got %v", err)
	}
	if _, statErr := os.Stat(badDst); statErr == nil {
		t.Error("nothing may be extracted from an archive that failed verification")
	}
}

func TestFetchToolchainRefusesUnpinnedDownload(t *testing.T) {
	archive := makeToolchainTarGz(t, toolchainFixture, nil)
	name := "go1.99.0.darwin-arm64.tar.gz"
	server := serveArchive(t, name, archive)
	err := FetchToolchain(context.Background(), FetchToolchainOpts{
		Version: "1.99.0", GOOS: "darwin", GOARCH: "arm64", DstRoot: filepath.Join(t.TempDir(), "x"),
		BaseURL: server.URL + "/dl/",
	})
	if err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("an unpinned version without an explicit checksum must be refused, got %v", err)
	}
}

func TestFetchToolchainReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	err := FetchToolchain(context.Background(), FetchToolchainOpts{
		Version: "1.26.2", GOOS: "darwin", GOARCH: "arm64", DstRoot: filepath.Join(t.TempDir(), "x"),
		BaseURL: server.URL + "/dl/", SHA256: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("got %v", err)
	}
}
