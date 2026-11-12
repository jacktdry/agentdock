package runtimeapi

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/execution"
)

func decodeRuntimeInsertionRequest(body []byte) (map[string]any, error) {
	if !utf8.Valid(body) || len(body) == 0 || len(body) > execution.MaxInsertionRequestBytes {
		return nil, runtimeInsertionRequestError("invalid insertion request body size")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return nil, runtimeInsertionRequestError("invalid insertion request body")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, runtimeInsertionRequestError("insertion request body must contain exactly one JSON value")
	}
	allowed := map[string]bool{"action": true, "call_id": true, "insertion_id": true, "text": true}
	for key := range fields {
		if !allowed[key] {
			return nil, runtimeInsertionRequestError("insertion request contains an unknown field")
		}
	}

	readString := func(key string) (string, error) {
		raw, ok := fields[key]
		if !ok {
			return "", nil
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", runtimeInsertionRequestError(key + " must be a string")
		}
		return strings.TrimSpace(value), nil
	}

	action, err := readString("action")
	if err != nil {
		return nil, err
	}
	action = strings.ToLower(action)
	args := map[string]any{"action": action}
	switch action {
	case "enqueue":
		callID, err := readString("call_id")
		if err != nil {
			return nil, err
		}
		text, err := readString("text")
		if err != nil {
			return nil, err
		}
		if callID == "" || text == "" {
			return nil, runtimeInsertionRequestError("enqueue requires call_id and text")
		}
		if len(text) > execution.MaxInsertionTextBytes {
			return nil, runtimeInsertionRequestError("insertion text exceeds 8192 bytes")
		}
		args["call_id"] = callID
		args["text"] = text
	case "cancel":
		id, err := readString("insertion_id")
		if err != nil {
			return nil, err
		}
		if id == "" {
			return nil, runtimeInsertionRequestError("cancel requires insertion_id")
		}
		args["insertion_id"] = id
	default:
		return nil, runtimeInsertionRequestError("insertion action must be enqueue or cancel")
	}
	return args, nil
}

func runtimeInsertionRequestError(message string) error {
	return &app.ToolError{Code: "INVALID_INSERTION_REQUEST", Message: message, Category: "validation"}
}
