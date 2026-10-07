package runtimeapi

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeDesktopMCPPreservesIdentityFieldsAndProtocol(t *testing.T) {
	for _, action := range []string{
		"desktop_snapshot", "desktop_inspect", "desktop_create", "desktop_update",
		"desktop_remove", "desktop_set_enabled", "desktop_env_snapshot", "desktop_env_set",
		"desktop_env_unset", "desktop_env_purge", "desktop_reconnect", "desktop_auth_status",
		"desktop_authorize", "desktop_auth_clear",
	} {
		t.Run(action, func(t *testing.T) {
			want := map[string]any{
				"action": action, "name": "demo",
				"expected_registry_revision": "registry",
				"expected_generation":        "generation",
				"expected_env_revision":      "environment",
				"protocol_version":           "2025-11-25",
				"enabled":                    false, "value": "write-only",
			}
			body, _ := json.Marshal(want)
			runtime := &runtimeStub{}
			_, err := Dispatch(context.Background(), runtime, Request{
				Method: "POST", Path: "/internal/runtime/mcp/desktop", Body: body,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(runtime.mcpArgs, want) {
				t.Fatalf("args = %#v, want %#v", runtime.mcpArgs, want)
			}
		})
	}
}

func TestRuntimeDesktopMCPPreservesExplicitArgsSemantics(t *testing.T) {
	for name, body := range map[string][]byte{
		"empty":      []byte(`{"action":"desktop_update","name":"demo","args":[],"expected_registry_revision":"registry","expected_generation":"generation"}`),
		"mixed-case": []byte(`{"action":"desktop_update","name":"demo","Args":[],"expected_registry_revision":"registry","expected_generation":"generation"}`),
	} {
		t.Run(name, func(t *testing.T) {
			runtime := &runtimeStub{}
			if _, err := Dispatch(context.Background(), runtime, Request{
				Method: "POST", Path: "/internal/runtime/mcp/desktop", Body: body,
			}); err != nil {
				t.Fatal(err)
			}
			args, ok := runtime.mcpArgs["args"].([]string)
			if !ok || args == nil || len(args) != 0 {
				t.Fatalf("explicit empty args lost at transport boundary: %#v", runtime.mcpArgs["args"])
			}
		})
	}

	runtime := &runtimeStub{}
	if _, err := Dispatch(context.Background(), runtime, Request{
		Method: "POST", Path: "/internal/runtime/mcp/desktop",
		Body: []byte(`{"action":"desktop_update","name":"demo","args":null,"expected_registry_revision":"registry","expected_generation":"generation"}`),
	}); err != nil {
		t.Fatal(err)
	}
	value, present := runtime.mcpArgs["args"]
	if !present || value != nil {
		t.Fatalf("explicit null args lost at transport boundary: present=%v value=%#v", present, value)
	}
}

func TestRuntimeDesktopMCPRejectsLegacyActionAndUnknownFields(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"action":"add"}`),
		[]byte(`{"action":"desktop_snapshot","unknown":true}`),
	} {
		runtime := &runtimeStub{}
		if _, err := Dispatch(context.Background(), runtime, Request{
			Method: "POST", Path: "/internal/runtime/mcp/desktop", Body: body,
		}); err == nil {
			t.Fatalf("Desktop route accepted invalid request %s", body)
		}
		if runtime.mcpArgs != nil {
			t.Fatalf("invalid request reached runtime: %#v", runtime.mcpArgs)
		}
	}
}
