// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultToolchainBaseURL = "https://go.dev/dl/"
	// The release the packaged app ships. The checksums below belong to this version;
	// a new version needs its own entries (copy them from https://go.dev/dl/?mode=json).
	PinnedToolchainVersion = "1.26.2"

	toolchainFetchTimeout = 15 * time.Minute
)

// Checksums are pinned here rather than read from go.dev next to the archive, so a
// tampered download fails even when the site that serves it also serves the sum.
var pinnedToolchainSHA256 = map[string]string{
	"go1.26.2.darwin-arm64.tar.gz": "32af1522bf3e3ff3975864780a429cc0b41d190ec7bf90faa661d6d64566e7af",
	"go1.26.2.darwin-amd64.tar.gz": "bc3f1500d9968c36d705442d90ba91addf9271665033748b82532682e90a7966",
	"go1.26.2.linux-arm64.tar.gz":  "c958a1fe1b361391db163a485e21f5f228142d6f8b584f6bef89b26f66dc5b23",
	"go1.26.2.linux-amd64.tar.gz":  "990e6b4bbba816dc3ee129eaeaf4b42f17c2800b88a2166c265ac1a200262282",
}

// Of the tools under pkg/tool/<os_arch>, `go build` runs these. vet, cover, fix, nm,
// pprof and the rest are for go test and debugging.
var toolchainTools = map[string]bool{"asm": true, "cgo": true, "compile": true, "link": true, "preprofile": true}

var toolchainVersionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?(rc[0-9]+|beta[0-9]+)?$`)

func PinnedToolchainSHA256(archiveName string) (string, bool) {
	sum, ok := pinnedToolchainSHA256[archiveName]
	return sum, ok
}

func ToolchainArchiveName(version string, goos string, goarch string) (string, error) {
	if !toolchainVersionRe.MatchString(version) {
		return "", fmt.Errorf("%q is not a Go release version such as %s", version, PinnedToolchainVersion)
	}
	if goos != "darwin" && goos != "linux" {
		return "", fmt.Errorf("no tarball for GOOS=%s (Windows releases are zip files)", goos)
	}
	if goarch != "arm64" && goarch != "amd64" {
		return "", fmt.Errorf("unsupported GOARCH %q", goarch)
	}
	return fmt.Sprintf("go%s.%s-%s.tar.gz", version, goos, goarch), nil
}

// ToolchainKeep decides, for one file at a slash path below the Go root, whether a
// bundled toolchain needs it. The goal is "compile a user's app and its dependencies":
// the whole standard library source stays (minus tests and testdata), the compiler,
// assembler, linker and cgo stay, and docs, tests, api, misc and the cmd/ source go.
func ToolchainKeep(rel string) bool {
	rel = path.Clean(rel)
	top, rest, _ := strings.Cut(rel, "/")
	switch top {
	case "VERSION", "go.env", "LICENSE", "PATENTS":
		return rest == ""
	case "bin":
		name := strings.TrimSuffix(rest, ".exe")
		return name == "go" || name == "gofmt"
	case "pkg":
		sub, tail, _ := strings.Cut(rest, "/")
		switch sub {
		case "include":
			return tail != ""
		case "tool":
			_, tool, ok := strings.Cut(tail, "/")
			return ok && !strings.Contains(tool, "/") && toolchainTools[strings.TrimSuffix(tool, ".exe")]
		}
		return false
	case "lib":
		return strings.HasPrefix(rest, "time/")
	case "src":
		return keepSourceFile(rest)
	}
	return false
}

func keepSourceFile(rest string) bool {
	if rest == "" || rest == "cmd" || strings.HasPrefix(rest, "cmd/") {
		return false
	}
	if strings.HasSuffix(rest, "_test.go") {
		return false
	}
	for _, segment := range strings.Split(rest, "/") {
		if segment == "testdata" {
			return false
		}
	}
	return true
}

func toolchainFileMode(mode fs.FileMode) fs.FileMode {
	perm := mode.Perm() | 0644
	if mode.Perm()&0111 != 0 {
		perm |= 0755
	}
	return perm
}

func validateToolchainRel(rel string) (string, error) {
	cleaned := path.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", fmt.Errorf("toolchain entry %q escapes the toolchain directory", rel)
	}
	return cleaned, nil
}

func writeToolchainFile(dstRoot string, rel string, mode fs.FileMode, content io.Reader) error {
	target := filepath.Join(dstRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, toolchainFileMode(mode))
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, content); err != nil {
		out.Close()
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	return out.Close()
}

// TrimToolchainDir copies the kept files of a Go root into dstRoot, replacing whatever
// was there. It is the same filter as ExtractToolchainTarGz, for a toolchain already
// unpacked (the Linux dev analogue, or a source build).
func TrimToolchainDir(srcRoot string, dstRoot string) error {
	if !isExecutableFile(filepath.Join(srcRoot, "bin", goExeName())) {
		return fmt.Errorf("%s is not a Go root: no bin/%s", srcRoot, goExeName())
	}
	if err := os.RemoveAll(dstRoot); err != nil {
		return err
	}
	err := filepath.WalkDir(srcRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !ToolchainKeep(rel) {
			return nil
		}
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", p)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		return writeToolchainFile(dstRoot, rel, info.Mode(), in)
	})
	if err != nil {
		os.RemoveAll(dstRoot)
		return err
	}
	return nil
}

// ExtractToolchainTarGz reads a go.dev release tarball (every entry under "go/") and
// writes only the kept files into dstRoot, so the full tree never touches the disk.
func ExtractToolchainTarGz(r io.Reader, dstRoot string) error {
	if err := os.RemoveAll(dstRoot); err != nil {
		return err
	}
	if err := extractToolchain(r, dstRoot); err != nil {
		os.RemoveAll(dstRoot)
		return err
	}
	return nil
}

func extractToolchain(r io.Reader, dstRoot string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	kept := 0
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		name := strings.TrimPrefix(header.Name, "./")
		inner, found := strings.CutPrefix(name, "go/")
		if !found {
			continue
		}
		rel, err := validateToolchainRel(inner)
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if !ToolchainKeep(rel) {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("toolchain entry %s is a link or special file; refusing to unpack it", header.Name)
		}
		if err := writeToolchainFile(dstRoot, rel, fs.FileMode(header.Mode), tr); err != nil {
			return err
		}
		kept++
	}
	if kept == 0 {
		return errors.New("the archive held no Go toolchain files")
	}
	if _, err := os.Stat(filepath.Join(dstRoot, "bin", goExeName())); err != nil {
		return fmt.Errorf("the archive has no bin/%s", goExeName())
	}
	return nil
}

type FetchToolchainOpts struct {
	Version string
	GOOS    string
	GOARCH  string
	DstRoot string
	// BaseURL ends in a slash; empty means go.dev/dl.
	BaseURL string
	// SHA256 overrides the pinned checksum, for a version this file does not pin yet.
	SHA256 string
	Client *http.Client
}

// FetchToolchain downloads a release, verifies it against the pinned checksum, and
// only then unpacks the trimmed toolchain into DstRoot (replacing it).
func FetchToolchain(ctx context.Context, opts FetchToolchainOpts) error {
	name, err := ToolchainArchiveName(opts.Version, opts.GOOS, opts.GOARCH)
	if err != nil {
		return err
	}
	want := strings.ToLower(opts.SHA256)
	if want == "" {
		pinned, ok := PinnedToolchainSHA256(name)
		if !ok {
			return fmt.Errorf("no pinned checksum for %s: add it to pinnedToolchainSHA256 or pass an explicit checksum", name)
		}
		want = pinned
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultToolchainBaseURL
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: toolchainFetchTimeout}
	}

	tmp, err := os.CreateTemp("", "gotoolchain-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+name, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: HTTP %d", name, resp.StatusCode)
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hash), resp.Body); err != nil {
		return fmt.Errorf("downloading %s: %w", name, err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, got, want)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return ExtractToolchainTarGz(tmp, opts.DstRoot)
}
