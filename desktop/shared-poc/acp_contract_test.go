package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestACPServiceRegisteredExactlyOnce(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "NewService" {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "application" {
			return true
		}
		constructor, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		service, ok := constructor.Fun.(*ast.SelectorExpr)
		if !ok || service.Sel.Name != "NewACPService" {
			return true
		}
		pkg, ok = service.X.(*ast.Ident)
		if ok && pkg.Name == "desktopapi" {
			count++
		}
		return true
	})
	if count != 1 {
		t.Fatalf("ACP service registrations = %d, want 1", count)
	}
}
