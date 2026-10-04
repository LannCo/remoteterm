// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestIsRelevantAppPath(t *testing.T) {
	relevant := []string{"app.go", "helpers.go", "static/logo.png", "static/css/site.css", "static/tw.css.map"}
	ignored := []string{
		"go.mod", "go.sum", "manifest.json", "static/tw.css", "bin/app", ".tsunami/build.log",
		".git/HEAD", ".app.go.swp", "app.go~", "app.go.swp", "app.go.swx", "app.go.tmp", "4913",
		"static/.DS_Store", "static/node_modules/x.js", "sub/x.go", "README.md", "AGENTS.md", "static", "",
	}
	for _, rel := range relevant {
		if !IsRelevantAppPath(rel) {
			t.Errorf("%q should be relevant", rel)
		}
	}
	for _, rel := range ignored {
		if IsRelevantAppPath(rel) {
			t.Errorf("%q should be ignored", rel)
		}
	}
}

func TestComputeAppInputHash(t *testing.T) {
	dir := makeHashAppDir(t)
	write := func(rel string, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	hash := func() string {
		h, err := ComputeAppInputHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	write("app.go", "package main\n")
	write("static/site.css", "body{}")
	base := hash()

	for _, rel := range []string{"go.mod", "go.sum", "manifest.json", "static/tw.css", ".tsunami/build.log", "bin/app"} {
		write(rel, "generated")
		if hash() != base {
			t.Fatalf("writing %s changed the input hash", rel)
		}
	}
	write("app.go", "package main // edited\n")
	edited := hash()
	if edited == base {
		t.Fatal("editing app.go did not change the input hash")
	}
	write("static/site.css", "body{color:red}")
	if hash() == edited {
		t.Fatal("editing a static file did not change the input hash")
	}
}

func TestComputeAppInputHashDoesNotReadStaticContents(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs an unreadable file, which root and Windows ignore")
	}
	dir := makeHashAppDir(t)
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(dir, "static", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(media), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media, []byte("frames"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(media, 0644) })
	before, err := ComputeAppInputHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(media, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, err := ComputeAppInputHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("an unreadable static file's mtime change did not change the hash; it must be hashed from metadata")
	}
}

func TestComputeAppInputHashStaticSameSizeNewMtime(t *testing.T) {
	dir := makeHashAppDir(t)
	css := filepath.Join(dir, "static", "site.css")
	if err := os.MkdirAll(filepath.Dir(css), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(css, []byte("aaaa"), 0644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(css, past, past); err != nil {
		t.Fatal(err)
	}
	before, _ := ComputeAppInputHash(dir)
	if err := os.WriteFile(css, []byte("bbbb"), 0644); err != nil {
		t.Fatal(err)
	}
	after, _ := ComputeAppInputHash(dir)
	if before == after {
		t.Fatal("a same-size rewrite of a static file did not change the hash")
	}
}

func TestComputeAppInputHashAppGoSameBytesUnchanged(t *testing.T) {
	dir := makeHashAppDir(t)
	appGo := filepath.Join(dir, "app.go")
	if err := os.WriteFile(appGo, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before, _ := ComputeAppInputHash(dir)
	if err := os.Chtimes(appGo, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appGo, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	after, _ := ComputeAppInputHash(dir)
	if before != after {
		t.Fatal("rewriting app.go with identical bytes changed the hash")
	}
}

// ComputeAppInputHash reads through the app-folder root, which only accepts folders
// under ~/waveapps, so the fixtures live there rather than in a bare temp dir.
func makeHashAppDir(t *testing.T) string {
	t.Helper()
	home, _ := setupBuilderTest(t)
	dir := filepath.Join(home, "waveapps", "draft", "hash")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestComputeAppInputHashRefusesFolderOutsideWaveapps(t *testing.T) {
	setupBuilderTest(t)
	if _, err := ComputeAppInputHash(t.TempDir()); err == nil {
		t.Fatal("hashed a folder outside ~/waveapps")
	}
}

func TestComputeAppInputHashDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures need a Unix filesystem")
	}
	dir := makeHashAppDir(t)
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.go"), []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "big.png"), []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "static"), 0755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"linked.go":         filepath.Join(outside, "secret.go"),
		"static/linked.png": filepath.Join(outside, "big.png"),
		"static/linked-dir": outside,
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := ComputeAppInputHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.go"), []byte("two, longer"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "big.png"), []byte("two, longer"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "new.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	after, err := ComputeAppInputHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("the hash followed a symlink out of the app folder")
	}
}
