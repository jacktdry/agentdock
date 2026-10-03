package browser

import (
	"fmt"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	toolcontract "github.com/uvwt/agentdock/internal/tool/contract"
)

type ManagedCompatibility struct {
	Engine                EngineKind
	Version               string
	PageIDRouting         bool
	BackgroundPages       bool
	NamedIsolatedContexts bool
}

// ValidateManagedTools checks the observed 1.7.0 invariants, not descriptions
// or schema byte equality. It does not qualify authentication or full no-focus.
func ValidateManagedTools(tools map[string]mcpclient.Tool) (ManagedCompatibility, error) {
	fail := func(name, reason string) (ManagedCompatibility, error) {
		return ManagedCompatibility{}, browserError(ErrEngineSchemaIncompatible, "managed browser tool schema incompatible", "validation", &ErrorDetails{Method: name, Reason: reason}, nil)
	}
	required := map[string]map[string]string{
		"list_pages": {}, "new_page": {"url": "string"}, "close_page": {"pageId": "number"},
		"navigate_page": {"pageId": "number"}, "take_snapshot": {"pageId": "number"}, "take_screenshot": {"pageId": "number"},
		"evaluate_script": {"pageId": "number", "function": "string"}, "click": {"pageId": "number", "uid": "string"},
		"fill": {"pageId": "number", "uid": "string", "value": "string"}, "press_key": {"pageId": "number", "key": "string"},
	}
	optional := map[string]map[string]string{"new_page": {"background": "boolean", "isolatedContext": "string"}, "navigate_page": {"url": "string", "type": "string"}, "take_screenshot": {"fullPage": "boolean", "format": "string"}}
	for name, fields := range required {
		t, ok := tools[name]
		if !ok {
			return fail(name, "required tool missing")
		}
		schema := t.InputSchema
		if schema["type"] != "object" {
			return fail(name, "object input required")
		}
		validator, err := toolcontract.CompileInputSchema(schema)
		if err != nil {
			return fail(name, err.Error())
		}
		if name == "list_pages" && validator.Validate(map[string]any{}) != nil {
			return fail(name, "empty arguments must be accepted")
		}
		props, _ := schema["properties"].(map[string]any)
		for field, kind := range fields {
			prop, _ := props[field].(map[string]any)
			if prop["type"] != kind || !schemaIncludes(schema["required"], field) {
				return fail(name, fmt.Sprintf("%s must be required %s", field, kind))
			}
		}
		for field, kind := range optional[name] {
			prop, _ := props[field].(map[string]any)
			if prop["type"] != kind || schemaIncludes(schema["required"], field) {
				return fail(name, fmt.Sprintf("%s must be optional %s", field, kind))
			}
		}
		if name == "navigate_page" {
			prop, _ := props["type"].(map[string]any)
			for _, value := range []string{"url", "back", "forward", "reload"} {
				if !schemaIncludes(prop["enum"], value) {
					return fail(name, "navigation enum incomplete")
				}
			}
		}
		if name == "take_screenshot" {
			prop, _ := props["format"].(map[string]any)
			if !schemaIncludes(prop["enum"], "png") {
				return fail(name, "PNG format required")
			}
		}
	}
	return ManagedCompatibility{Engine: EngineChromeDevToolsMCP, Version: ManagedEngineVersion, PageIDRouting: true, BackgroundPages: true, NamedIsolatedContexts: true}, nil
}

func schemaIncludes(raw any, want string) bool {
	switch list := raw.(type) {
	case []string:
		for _, s := range list {
			if s == want {
				return true
			}
		}
	case []any:
		for _, s := range list {
			if s == want {
				return true
			}
		}
	}
	return false
}
