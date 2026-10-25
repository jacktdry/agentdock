package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNextNativeMCPCatalogStrictSchema(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":1,"servers":[]}`,
		`{"schema_version":1,"servers":[{"id":"fixture","executable":"Helpers/native-mcp/fixture"}]}`,
		`{"schema_version":1,"servers":[{"id":"fixture","executable":"Helpers/native-mcp/fixture","args":["--stdio","literal $HOME; echo x"]}]}`,
	} {
		if _, err := parseNextNativeMCPCatalog([]byte(raw)); err != nil {
			t.Fatalf("valid catalog rejected: %v", err)
		}
	}
	entry := `{"id":"fixture","executable":"Helpers/native-mcp/fixture"}`
	for name, raw := range map[string]string{
		"unknown-root":        `{"schema_version":1,"servers":[],"env":{}}`,
		"duplicate-key":       `{"schema_version":1,"servers":[],"servers":[]}`,
		"duplicate-entry-key": `{"schema_version":1,"servers":[{"id":"fixture","id":"other","executable":"Helpers/native-mcp/other"}]}`,
		"duplicate-id":        `{"schema_version":1,"servers":[` + entry + `,` + entry + `]}`,
		"wrong-version":       `{"schema_version":2,"servers":[]}`,
		"float-version":       `{"schema_version":1.0,"servers":[]}`,
		"bool-version":        `{"schema_version":true,"servers":[]}`,
		"case-fold":           `{"Schema_version":1,"servers":[]}`,
		"null-servers":        `{"schema_version":1,"servers":null}`,
		"trailing":            `{"schema_version":1,"servers":[]} {}`,
		"oversized":           strings.Repeat(" ", nextNativeMCPCatalogLimit+1),
		"invalid-utf8":        "{\"schema_version\":1,\"servers\":[],\"\xff\":0}",
		"surrogate":           `{"schema_version":1,"servers":[{"id":"fixture","executable":"Helpers/native-mcp/fixture","args":["\ud800"]}]}`,
		"deep":                strings.Repeat("[", 20) + "0" + strings.Repeat("]", 20),
		"too-many-servers":    `{"schema_version":1,"servers":[` + strings.Repeat(entry+",", 32) + entry + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if servers, err := parseNextNativeMCPCatalog([]byte(raw)); err != errNextNativeMCPUnavailable || servers != nil {
				t.Fatalf("unsafe catalog accepted: %v", err)
			}
		})
	}
}

func TestNextNativeMCPCatalogRejectsCommandOverrides(t *testing.T) {
	for name, overrides := range map[string]map[string]any{
		"absolute":          {"executable": "/bin/echo"},
		"traversal":         {"executable": "Helpers/native-mcp/../fixture"},
		"encoded-traversal": {"executable": "Helpers/native-mcp/%2e%2e/fixture"},
		"backslash":         {"executable": `Helpers\native-mcp\fixture`},
		"shell":             {"id": "sh", "executable": "Helpers/native-mcp/sh"},
		"script":            {"executable": "Helpers/native-mcp/fixture.sh"},
		"id-path":           {"id": "../fixture"},
		"env":               {"env": map[string]string{"PATH": "/tmp"}},
		"digest":            {"config_digest": strings.Repeat("0", 64)},
		"wrapper":           {"wrapper": "/bin/echo"},
		"null-args":         {"args": nil},
		"string-args":       {"args": "--stdio"},
		"nul-arg":           {"args": []string{"a\x00b"}},
		"newline-arg":       {"args": []string{"a\nb"}},
		"long-arg":          {"args": []string{strings.Repeat("x", 1025)}},
		"too-many-args":     {"args": make([]string, 65)},
	} {
		t.Run(name, func(t *testing.T) {
			entry := map[string]any{"id": "fixture", "executable": "Helpers/native-mcp/fixture"}
			for key, value := range overrides {
				entry[key] = value
			}
			raw, err := json.Marshal(map[string]any{"schema_version": 1, "servers": []any{entry}})
			if err != nil {
				t.Fatal(err)
			}
			if servers, err := parseNextNativeMCPCatalog(raw); err != errNextNativeMCPUnavailable || servers != nil {
				t.Fatalf("caller command override accepted: %v", err)
			}
		})
	}
}
