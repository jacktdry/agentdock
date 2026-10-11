package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNextM8DispatchArgumentsDetachedFromCallerMutation(t *testing.T) {
	original := map[string]any{
		"nested": map[string]any{"value": "before"},
		"items":  []any{map[string]any{"index": json.Number("9007199254740993")}},
	}
	canonical, detached, err := nextM8SnapshotArgs(original)
	if err != nil {
		t.Fatal(err)
	}
	original["nested"].(map[string]any)["value"] = "after"
	original["items"].([]any)[0].(map[string]any)["index"] = "forged"
	original["injected"] = "after-fingerprint"
	after, err := json.Marshal(detached)
	if err != nil || string(after) != string(canonical) {
		t.Fatalf("approved bytes changed: %s => %s (%v)", canonical, after, err)
	}
	if strings.Contains(string(after), "forged") || strings.Contains(string(after), "injected") || strings.Contains(string(after), "after") {
		t.Fatal("mutable caller arguments leaked into approved dispatch")
	}
	if detached["items"].([]any)[0].(map[string]any)["index"] != json.Number("9007199254740993") {
		t.Fatal("JSON number precision changed during permission fingerprinting")
	}
	for _, args := range []map[string]any{{"unsupported": make(chan int)}, {"oversize": strings.Repeat("s", 65<<10)}} {
		if _, _, err := nextM8SnapshotArgs(args); !errors.Is(err, ErrNextM8NotAllowed) {
			t.Fatal("invalid arguments accepted")
		}
	}
	var untrusted NextStdioM8Context
	if err := json.Unmarshal([]byte(`{"CoreSupervisorM8Context":{"Binding":{"Principal":{"Authenticated":true}}}}`), &untrusted); !errors.Is(err, ErrNextM8NotAllowed) {
		t.Fatal("untrusted JSON tried to construct Core M8 authority")
	}
}
