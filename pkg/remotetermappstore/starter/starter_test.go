// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package starter

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/tsunami/build"
)

var sdkIdentRe = regexp.MustCompile(`\b(app|vdom|ui)\.([A-Z][A-Za-z0-9_]*)`)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

func starterFile(t *testing.T, name string) []byte {
	t.Helper()
	files, err := GetStarterFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name == name {
			return f.Data
		}
	}
	t.Fatalf("starter file %s not found", name)
	return nil
}

func exportedIdents(t *testing.T, pkgDir string) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(pkgDir, "*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Go files in %s: %v", pkgDir, err)
	}
	idents := make(map[string]bool)
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					idents[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							idents[s.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name.IsExported() {
								idents[name.Name] = true
							}
						}
					}
				}
			}
		}
	}
	return idents
}

func TestGetStarterFiles(t *testing.T) {
	files, err := GetStarterFiles()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
		if len(f.Data) == 0 {
			t.Errorf("%s is empty", f.Name)
		}
	}
	if strings.Join(names, ",") != "app.go,AGENTS.md,CLAUDE.md,TSUNAMI_GUIDE.md" {
		t.Fatalf("names = %v", names)
	}
	if got := string(starterFile(t, ClaudeFileName)); got != "@AGENTS.md\n" {
		t.Fatalf("CLAUDE.md = %q, want the single line @AGENTS.md", got)
	}
}

func TestAgentsMdConstraints(t *testing.T) {
	text := string(starterFile(t, AgentsFileName))
	if lines := strings.Count(text, "\n"); lines >= 120 {
		t.Fatalf("AGENTS.md has %d lines, must be under 120", lines)
	}
	fence := strings.Repeat("`", 3)
	for _, banned := range []string{"go get", fence + "sh", fence + "bash", fence + "shell", "http://", "https://", "curl ", "wget "} {
		if strings.Contains(text, banned) {
			t.Errorf("AGENTS.md contains %q; it must not carry shell commands or network instructions", banned)
		}
	}
	for _, required := range []string{".tsunami/build.log", "TSUNAMI_GUIDE.md", "AppMeta", "AppInit", "go.mod", "manifest.json", "static/tw.css", "bin/"} {
		if !strings.Contains(text, required) {
			t.Errorf("AGENTS.md does not mention %q", required)
		}
	}
}

func TestStarterAppGoShape(t *testing.T) {
	src := starterFile(t, AppGoFileName)
	file, err := parser.ParseFile(token.NewFileSet(), "app.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name.Name != "main" {
		t.Fatalf("package %s, want main", file.Name.Name)
	}
	vars := make(map[string]bool)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "init" {
			t.Fatal("starter app.go defines init(); the build rejects that")
		}
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			for _, spec := range gen.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					vars[name.Name] = true
				}
			}
		}
	}
	if !vars["AppMeta"] || !vars["App"] {
		t.Fatalf("starter app.go must declare var AppMeta and var App, found %v", vars)
	}
}

func TestStarterDocsReferenceOnlyExistingSdkSymbols(t *testing.T) {
	sdk := filepath.Join(repoRoot(t), "tsunami")
	exported := map[string]map[string]bool{
		"app":  exportedIdents(t, filepath.Join(sdk, "app")),
		"vdom": exportedIdents(t, filepath.Join(sdk, "vdom")),
		"ui":   exportedIdents(t, filepath.Join(sdk, "ui")),
	}
	for _, name := range []string{AgentsFileName, GuideFileName} {
		text := string(starterFile(t, name))
		matches := sdkIdentRe.FindAllStringSubmatch(text, -1)
		if name == GuideFileName && len(matches) < 50 {
			t.Errorf("%s references only %d SDK identifiers; the port looks truncated", name, len(matches))
		}
		for _, m := range matches {
			if !exported[m[1]][m[2]] {
				t.Errorf("%s references %s.%s, which the SDK does not export", name, m[1], m[2])
			}
		}
	}
}

func TestGuideHasNoStaleApiOrChatFraming(t *testing.T) {
	text := string(starterFile(t, GuideFileName))
	for _, stale := range []string{"wavetermdev", "const AppTitle", "const AppShortDesc", "SetShortDescription", "UseData", "Built for AI", "AI agent", "AI model", "global-keyboard-handling.md", "graphing.md"} {
		if strings.Contains(text, stale) {
			t.Errorf("TSUNAMI_GUIDE.md still contains %q", stale)
		}
	}
	if !strings.Contains(text, "github.com/LannCo/remoteterm/tsunami/app") {
		t.Error("TSUNAMI_GUIDE.md does not use the LannCo import path")
	}
}

func TestStarterAppCompilesAgainstBundledSdk(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		// go test runs with the toolchain that built it, even when PATH lacks go.
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
		if _, statErr := os.Stat(goBin); statErr != nil {
			t.Skip("go not found: not on PATH and not at runtime.GOROOT()/bin/go")
		}
	}
	root := repoRoot(t)
	sdkSrc := filepath.Join(root, "tsunami")
	minGo, err := build.ReadSdkGoVersion(sdkSrc)
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	verCmd := exec.Command(goBin, "env", "GOVERSION")
	verCmd.Env = env
	verOut, err := verCmd.Output()
	if err != nil {
		t.Skipf("cannot read the local Go version: %v", err)
	}
	if localGo := strings.TrimSpace(string(verOut)); build.CompareGoVersions(localGo, minGo) < 0 {
		t.Skipf("local %s is older than the SDK's go %s", localGo, minGo)
	}

	work := t.TempDir()
	sdk := filepath.Join(work, "sdk")
	if err := build.CopySdkBundle(sdkSrc, sdk); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(work, "app")
	mainTmpl, err := os.ReadFile(filepath.Join(sdkSrc, "templates", "app-main.go.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module tsunami/draft/starter\n\ngo %s\n\nrequire %s v0.12.4\n\nreplace %s => %q\n", minGo, build.TsunamiSdkModulePath, build.TsunamiSdkModulePath, sdk)
	for rel, content := range map[string][]byte{
		"go.mod":          []byte(goMod),
		"app.go":          starterFile(t, AppGoFileName),
		"app-main.go":     mainTmpl,
		"dist/index.html": []byte("<!doctype html>\n"),
		"static/tw.css":   []byte(""),
	} {
		path := filepath.Join(appDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, error) {
		cmd := exec.Command(goBin, args...)
		cmd.Dir = appDir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("mod", "tidy"); err != nil {
		if strings.Contains(out, "GOPROXY=off") || strings.Contains(out, "module lookup disabled") {
			t.Skipf("module cache is cold, cannot resolve SDK dependencies offline:\n%s", out)
		}
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	if out, err := run("build", "-o", filepath.Join(work, "starter-bin"), "."); err != nil {
		t.Fatalf("the starter app does not compile against the bundled SDK: %v\n%s", err, out)
	}
}
