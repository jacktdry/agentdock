package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestBrowserServiceRegisteredReadOnly(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	registrations := 0
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		outer, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || outer.Sel.Name != "NewService" {
			return true
		}
		inner, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		constructor, ok := inner.Fun.(*ast.SelectorExpr)
		if ok && constructor.Sel.Name == "NewBrowserService" {
			registrations++
		}
		return true
	})
	if registrations != 1 {
		t.Fatalf("Browser registrations = %d", registrations)
	}
	data, err := os.ReadFile("frontend/bindings/github.com/uvwt/agentdock/internal/desktopapi/browserservice.ts")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "export function ") != 1 || !strings.Contains(string(data), "export function Snapshot()") {
		t.Fatal("Browser binding must expose only Snapshot")
	}
}
