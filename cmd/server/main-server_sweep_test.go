// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func callName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			return x.Name + "." + e.Sel.Name
		}
	}
	return ""
}

// mainCallIndex maps each call made directly in main() (not inside a closure) to the index of the
// top-level statement that first makes it.
func mainCallIndex(t *testing.T) (map[string]int, []ast.Stmt) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "main-server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var mainFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "main" {
			mainFn = fn
		}
	}
	if mainFn == nil {
		t.Fatal("main() not found")
	}
	index := map[string]int{}
	for i, stmt := range mainFn.Body.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := callName(call.Fun)
			if _, seen := index[name]; name != "" && !seen {
				index[name] = i
			}
			return true
		})
	}
	return index, mainFn.Body.List
}

func TestBuilderSweepRunsBetweenControllerInitAndReconnect(t *testing.T) {
	index, stmts := mainCallIndex(t)
	sweep, ok := index["runBuilderSweep"]
	if !ok {
		t.Fatal("runBuilderSweep is not called in main()")
	}
	if _, isExpr := stmts[sweep].(*ast.ExprStmt); !isExpr {
		t.Fatal("runBuilderSweep must run synchronously, not in a goroutine")
	}
	for _, before := range []string{"jobcontroller.InitJobController", "blockcontroller.InitBlockController"} {
		if i, ok := index[before]; !ok || i >= sweep {
			t.Errorf("%s (stmt %d) must run before the sweep (stmt %d)", before, i, sweep)
		}
	}
	for _, after := range []string{"blockcontroller.StartupReconnectDurableShells", "web.MakeTCPListener", "web.MakeUnixListener"} {
		if i, ok := index[after]; !ok || i <= sweep {
			t.Errorf("%s (stmt %d) must run after the sweep (stmt %d)", after, i, sweep)
		}
	}
}
