package runtimeapi

import (
	"encoding/json"
	"github.com/uvwt/agentdock/internal/execution"
	"strings"
	"testing"
)

func TestInsertionRequestValidation(t *testing.T) {
	for _, body := range []string{"", "null", "[]", "{}", `{"action":"enqueue","call_id":"c","text":"x","secret":1}`, `{"action":"enqueue","call_id":"c","text":1}`, `{"action":"enqueue","call_id":"c","text":null}`, `{"action":"cancel"}`, `{"action":"enqueue","call_id":"c","text":"x"} {}`, `{"action":"enqueue","call_id":"c","text":"` + string([]byte{0xff}) + `"}`, strings.Repeat("x", execution.MaxInsertionTextBytes+4097)} {
		if _, err := decodeRuntimeInsertionRequest([]byte(body)); err == nil {
			t.Fatalf("accepted invalid body %q", body)
		}
	}
	for _, text := range []string{
		"hello",
		strings.Repeat("x", execution.MaxInsertionTextBytes),
		strings.Repeat("<", execution.MaxInsertionTextBytes),
		strings.Repeat("\"", execution.MaxInsertionTextBytes),
		strings.Repeat("\\", execution.MaxInsertionTextBytes),
		strings.Repeat("x\n", (execution.MaxInsertionTextBytes-2)/2) + "xx",
	} {
		body, _ := json.Marshal(map[string]string{"action": "enqueue", "call_id": "c", "text": text})
		if got, err := decodeRuntimeInsertionRequest(body); err != nil || got["text"] != text {
			t.Fatalf("valid body: %v", err)
		}
	}
}
