//go:build darwin || linux

package desktopapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/execution"
)

func TestExecutionClientUsesGoOnlyBearerForSnapshotReplayAndStream(t *testing.T) {
	const token = "core-only-secret"
	epoch := "epoch_test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/internal/runtime/execution":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"snapshot": execution.Snapshot{
					SchemaVersion: 1, Epoch: epoch, LatestSequence: "2", PrunedThrough: "0",
					Calls: []execution.Call{{ID: "call_demo", Tool: "agentdock_context", Status: execution.StatusCompleted, StartedSequence: "1", EndedSequence: "2"}},
				},
			})
		case "/internal/runtime/activity":
			if r.URL.Query().Get("after") != "2" || r.URL.Query().Get("limit") != "25" {
				t.Errorf("replay query = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"page": execution.Page{SchemaVersion: 1, Epoch: epoch, After: "2", LatestSequence: "3", Events: []execution.Event{{
					SchemaVersion: 1, Epoch: epoch, Sequence: "3", Kind: execution.EventCallStarted, CallID: "call_next",
				}}},
			})
		case "/internal/runtime/activity/stream":
			if r.URL.Query().Get("after") != "3" || r.URL.Query().Get("epoch") != epoch {
				t.Errorf("stream query = %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			page := execution.Page{SchemaVersion: 1, Epoch: epoch, After: "3", LatestSequence: "4", Events: []execution.Event{{
				SchemaVersion: 1, Epoch: epoch, Sequence: "4", Kind: execution.EventCallCompleted, CallID: "call_next",
			}}}
			data, _ := json.Marshal(page)
			_, _ = fmt.Fprintf(w, "id: 4\nevent: activity\ndata: %s\n\n", data)
		case "/internal/runtime/insertions":
			if r.Method != http.MethodPost {
				t.Errorf("insertion method = %s", r.Method)
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode insertion body: %v", err)
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			action, _ := body["action"].(string)
			switch action {
			case "enqueue":
				if body["call_id"] != "call_next" || body["text"] != "steer now" {
					t.Errorf("enqueue body = %#v", body)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok": true, "ack": true,
					"insertion": execution.Insertion{ID: "ins_demo", TargetCallID: "call_next", Status: execution.InsertionAccepted},
				})
			case "cancel":
				if body["insertion_id"] != "ins_demo" {
					t.Errorf("cancel body = %#v", body)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":        true,
					"insertion": execution.Insertion{ID: "ins_demo", TargetCallID: "call_next", Status: execution.InsertionCancelled},
				})
			default:
				http.Error(w, "bad action", http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := writeExecutionClientRuntime(t, server.URL, token)
	client := NewExecutionClient(root)

	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Epoch != epoch || len(snapshot.Calls) != 1 || snapshot.Calls[0].Tool != "agentdock_context" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if encoded, _ := json.Marshal(snapshot); strings.Contains(string(encoded), token) {
		t.Fatalf("snapshot leaked token: %s", encoded)
	}

	page, err := client.Replay(context.Background(), 2, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].Sequence != "3" {
		t.Fatalf("replay = %#v", page)
	}

	var streamed []ExecutionStreamMessage
	if err := client.Stream(context.Background(), epoch, 3, func(message ExecutionStreamMessage) error {
		streamed = append(streamed, message)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(streamed) != 2 || streamed[0].Kind != "connected" || streamed[1].ID != "4" || streamed[1].Kind != "activity" ||
		len(streamed[1].Page.Events) != 1 || streamed[1].Page.Events[0].Sequence != "4" {
		t.Fatalf("streamed = %#v", streamed)
	}

	accepted, err := client.EnqueueInsertion(context.Background(), "call_next", "steer now")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ID != "ins_demo" || accepted.Status != execution.InsertionAccepted || accepted.TargetCallID != "call_next" {
		t.Fatalf("accepted insertion = %#v", accepted)
	}

	cancelled, err := client.CancelInsertion(context.Background(), accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.ID != accepted.ID || cancelled.Status != execution.InsertionCancelled {
		t.Fatalf("cancelled insertion = %#v", cancelled)
	}
}

func writeExecutionClientRuntime(t *testing.T, serverURL, token string) string {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	envPath := filepath.Join(root, "agentdock.env")
	env := fmt.Sprintf("AGENTDOCK_HOST=127.0.0.1\nAGENTDOCK_PORT=%d\nAGENTDOCK_AUTH_TOKEN='%s'\n", port, token)
	if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"schema_version":   1,
		"agentdock_binary": filepath.Join(root, "agentdock"),
		"environment_file": envPath,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExecutionClientRejectsInvalidAcknowledgementAndText(t *testing.T) {
	for _, mode := range []string{"missing_ack", "wrong_target", "delivered"} {
		t.Run(mode, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				item := execution.Insertion{ID: "ins", TargetCallID: "call", Status: execution.InsertionAccepted}
				ack := true
				switch mode {
				case "missing_ack":
					ack = false
				case "wrong_target":
					item.TargetCallID = "other"
				case "delivered":
					item.Status = execution.InsertionDelivered
				}
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "ack": ack, "insertion": item})
			})
			client := insertionTestClient(t, handler)
			if _, err := client.EnqueueInsertion(context.Background(), "call", "text"); err == nil {
				t.Fatal("accepted invalid ACK")
			}
		})
	}
	client := NewExecutionClient(t.TempDir())
	if _, err := client.EnqueueInsertion(context.Background(), "call", string([]byte{0xff})); err != execution.ErrInsertionInvalid {
		t.Fatalf("invalid UTF-8 error=%v", err)
	}
}

func TestExecutionClientAcceptsWorstCaseEscapedInsertionEnvelope(t *testing.T) {
	text := strings.Repeat("<", execution.MaxInsertionTextBytes)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["text"] != text {
			t.Fatalf("decoded text length/content mismatch: got %d bytes", len(body["text"].(string)))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":  true,
			"ack": true,
			"insertion": execution.Insertion{
				ID:           "ins_escaped",
				TargetCallID: "call",
				Status:       execution.InsertionAccepted,
			},
		})
	})
	client := insertionTestClient(t, handler)
	item, err := client.EnqueueInsertion(context.Background(), "call", text)
	if err != nil {
		t.Fatalf("valid escaped insertion rejected: %v", err)
	}
	if item.ID != "ins_escaped" {
		t.Fatalf("insertion = %#v", item)
	}
}

func TestExecutionClientBoundsMultilineSSEPayload(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: activity\n")
		part := strings.Repeat("x", maxExecutionResponseBytes/2)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: %s\n", part)
		}
		fmt.Fprint(w, "\n")
	})
	client := insertionTestClient(t, handler)
	connected := false
	activityCalled := false
	err := client.Stream(context.Background(), "", 0, func(message ExecutionStreamMessage) error {
		if message.Kind == "connected" {
			connected = true
		} else {
			activityCalled = true
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "too large") || !connected || activityCalled {
		t.Fatalf("stream bound error=%v connected=%v activityCalled=%v", err, connected, activityCalled)
	}
}

// Exercise HTTP decoding without requiring a sandbox listening socket.
type insertionTestTransport struct{ handler http.Handler }

func (transport insertionTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	transport.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}
func insertionTestClient(t *testing.T, handler http.Handler) *ExecutionClient {
	t.Helper()
	client := NewExecutionClient(writeExecutionClientRuntime(t, "http://127.0.0.1:12345", ""))
	transport := insertionTestTransport{handler: handler}
	client.httpClient = &http.Client{Transport: transport}
	client.streamHTTP = &http.Client{Transport: transport}
	return client
}
