package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestMCPServiceRegisteredExactlyOnce(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	constructorCount := 0
	registrationCount := 0
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "NewMCPServiceWithOpenURL" {
			if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "desktopapi" {
				constructorCount++
			}
		}
		if len(call.Args) != 1 {
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
		if service, ok := call.Args[0].(*ast.Ident); ok && service.Name == "mcpService" {
			registrationCount++
		}
		return true
	})
	if constructorCount != 1 {
		t.Fatalf("MCP native-opener constructors = %d, want 1", constructorCount)
	}
	if registrationCount != 1 {
		t.Fatalf("MCP service registrations = %d, want 1", registrationCount)
	}
}
