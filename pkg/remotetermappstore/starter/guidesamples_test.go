// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package starter

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Code blocks in TSUNAMI_GUIDE.md fenced as ```go are type-checked against the bundled SDK.
// A block fenced as ```go illustrative is skipped: use it only for a snippet that is not
// meant to compile, such as a copy of an SDK type shown for reference. Markdown renderers
// read only the first word of the info string, so the marker does not affect highlighting.
const (
	goFence           = "```go"
	illustrativeFence = "```go illustrative"
	minCheckedBlocks  = 30
)

// Fragments compile only with their free names defined. A stub is left out when the block
// declares that name itself.
var guideSampleStubs = []struct {
	name string
	src  string
}{
	{"Todo", "type Todo struct {\n\tId        int    `json:\"id\"`\n\tText      string `json:\"text\"`\n\tCompleted bool   `json:\"completed\"`\n}"},
	{"UserPreferences", "type UserPreferences struct{}"},
	{"UserStats", "type UserStats struct{}"},
	{"APIResult", "type APIResult struct{}"},
	{"MyProps", "type MyProps struct{}"},
	{"MetricsPoint", "type MetricsPoint struct{}"},
	{"todo", "var todo Todo"},
	{"newTodo", "var newTodo Todo"},
	{"todosAtom", "var todosAtom = app.SharedAtom(\"todos\", []Todo{})"},
	{"count", "var count = app.SharedAtom(\"count\", 0)"},
	{"items", "var items []string"},
	{"defaults", "var defaults map[string]any"},
	{"spacing", "var spacing = 4"},
	{"idx", "var idx = 0"},
	{"isActive", "var isActive bool"},
	{"isDisabled", "var isDisabled bool"},
	{"isVisible", "var isVisible bool"},
	{"isCurrentItem", "var isCurrentItem bool"},
	{"someCondition", "var someCondition bool"},
	{"handleToggle", "func handleToggle() {}"},
	{"handleClick", "func handleClick() {}"},
	{"handleKey", "func handleKey(e vdom.VDomEvent) {}"},
	{"handleSubmit", "func handleSubmit(e vdom.VDomEvent) {}"},
	{"fetchAndProcessData", "func fetchAndProcessData() ([]APIResult, error) { return nil, nil }"},
	{"mergeResults", "func mergeResults(a, b []APIResult) []APIResult { return append(a, b...) }"},
	{"samplePoint", "func samplePoint() MetricsPoint { return MetricsPoint{} }"},
	{"renderLineChart", "func renderLineChart(data []MetricsPoint) any { return nil }"},
}

const guideSamplePrelude = `package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/LannCo/remoteterm/tsunami/app"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

var _ = context.Background
var _ = json.Marshal
var _ = log.Printf
var _ = time.Second
var _ = app.Ptr[int]
var _ = vdom.H

func main() {}

`

type guideBlock struct {
	fenceLine int
	src       string
}

func extractGuideGoBlocks(t *testing.T, text string) (checked []guideBlock, illustrative int) {
	t.Helper()
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		fence := strings.TrimSpace(lines[i])
		if fence != goFence && fence != illustrativeFence {
			continue
		}
		start := i
		var body []string
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
			body = append(body, lines[i])
		}
		if i == len(lines) {
			t.Fatalf("TSUNAMI_GUIDE.md:%d: unterminated code block", start+1)
		}
		if fence == illustrativeFence {
			illustrative++
			continue
		}
		checked = append(checked, guideBlock{fenceLine: start + 1, src: strings.Join(body, "\n") + "\n"})
	}
	return checked, illustrative
}

func topLevelNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				names[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names[n.Name] = true
					}
				}
			}
		}
	}
	return names
}

// wrapGuideFragment turns a statement fragment into a function body. Variables the
// fragment declares are marked used, because a snippet often stops before using them.
func wrapGuideFragment(body string, fenceLine int) (string, error) {
	const head = "package p\n\nfunc _() any {\n"
	src := head + body + "}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return "", err
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	var uses strings.Builder
	for _, stmt := range fn.Body.List {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if s.Tok != token.DEFINE {
				continue
			}
			for _, lhs := range s.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
					fmt.Fprintf(&uses, "\t_ = %s\n", id.Name)
				}
			}
		case *ast.DeclStmt:
			gen, ok := s.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				for _, n := range spec.(*ast.ValueSpec).Names {
					fmt.Fprintf(&uses, "\t_ = %s\n", n.Name)
				}
			}
		}
	}
	wrapped := fmt.Sprintf("func _() any {\n//line TSUNAMI_GUIDE.md:%d\n", fenceLine+1) + body
	stmts := fn.Body.List
	if len(stmts) > 0 {
		if ret, ok := stmts[len(stmts)-1].(*ast.ReturnStmt); ok {
			off := fset.Position(ret.Pos()).Offset - len(head)
			bodyStart := len(wrapped) - len(body)
			return wrapped[:bodyStart+off] + uses.String() + wrapped[bodyStart+off:] + "}\n", nil
		}
	}
	return wrapped + uses.String() + "\treturn nil\n}\n", nil
}

// guideBlockFiles returns the files for one block's package: complete programs as they
// are, declarations and fragments with the shared imports and the stubs they need.
func guideBlockFiles(t *testing.T, block guideBlock) map[string][]byte {
	t.Helper()
	lineDirective := fmt.Sprintf("//line TSUNAMI_GUIDE.md:%d\n", block.fenceLine+1)
	if strings.HasPrefix(strings.TrimSpace(block.src), "package ") {
		return map[string][]byte{"block.go": []byte(lineDirective + block.src + "\nfunc main() {}\n")}
	}
	var blockSrc string
	declared := map[string]bool{}
	if file, err := parser.ParseFile(token.NewFileSet(), "", "package main\n"+block.src, parser.SkipObjectResolution); err == nil {
		declared = topLevelNames(file)
		blockSrc = lineDirective + block.src
	} else {
		wrapped, wrapErr := wrapGuideFragment(block.src, block.fenceLine)
		if wrapErr != nil {
			t.Fatalf("TSUNAMI_GUIDE.md:%d: block parses neither as declarations nor as statements: %v", block.fenceLine, wrapErr)
		}
		blockSrc = wrapped
	}
	var stubs strings.Builder
	stubs.WriteString("package main\n\nimport (\n\t\"github.com/LannCo/remoteterm/tsunami/app\"\n\t\"github.com/LannCo/remoteterm/tsunami/vdom\"\n)\n\nvar _ = app.Ptr[int]\nvar _ = vdom.H\n\n")
	for _, stub := range guideSampleStubs {
		if !declared[stub.name] {
			stubs.WriteString(stub.src + "\n")
		}
	}
	return map[string][]byte{
		"block.go": []byte(guideSamplePrelude + blockSrc),
		"stubs.go": []byte(stubs.String()),
	}
}

func TestGuideCodeBlocksTypeCheck(t *testing.T) {
	blocks, illustrative := extractGuideGoBlocks(t, string(starterFile(t, GuideFileName)))
	if len(blocks) < minCheckedBlocks {
		t.Fatalf("found %d checkable Go blocks (%d illustrative) in TSUNAMI_GUIDE.md, want at least %d; the extraction looks broken", len(blocks), illustrative, minCheckedBlocks)
	}
	work := t.TempDir()
	e := prepareSdkModuleEnv(t, work)
	modDir := filepath.Join(work, "guidesamples")
	files := map[string][]byte{"go.mod": e.goMod("guidesamples")}
	for _, block := range blocks {
		dir := fmt.Sprintf("line%04d", block.fenceLine)
		for name, content := range guideBlockFiles(t, block) {
			files[filepath.Join(dir, name)] = content
		}
	}
	writeTree(t, modDir, files)
	e.tidyOrSkip(t, modDir)
	out, err := e.run(modDir, "vet", "./...")
	if err != nil {
		t.Fatalf("Go code blocks in TSUNAMI_GUIDE.md do not type-check against the bundled SDK (positions are guide lines; mark a block that is not meant to compile with %q):\n%s", illustrativeFence, out)
	}
	t.Logf("type-checked %d Go blocks, skipped %d illustrative", len(blocks), illustrative)
}
