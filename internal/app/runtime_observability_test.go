package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/observability"
	"go.opentelemetry.io/otel/trace"
)

func TestRuntimeAnalyticsRecordsSafeToolMetadata(t *testing.T) {
	runtime := newRuntimeValidationTestRuntime(t)
	ctx := observability.WithSource(context.Background(), observability.SourceMCP)
	secret := "/Users/example/private/token-file"

	if _, err := runtime.Call(ctx, "agentdock_context", map[string]any{"future_field": secret}); err == nil {
		t.Fatal("expected validation error")
	}
	if _, err := runtime.Call(ctx, "secret-tool-"+secret, nil); err == nil {
		t.Fatal("expected unknown tool error")
	}

	snapshot := runtime.observer.Snapshot()
	if snapshot.TotalCalls != 2 || snapshot.TotalErrors != 2 || snapshot.WindowCalls != 2 {
		t.Fatalf("analytics totals = calls %d errors %d window %d", snapshot.TotalCalls, snapshot.TotalErrors, snapshot.WindowCalls)
	}
	if snapshot.RecentCalls[0].Tool != "unknown" {
		t.Fatalf("unknown tool name was retained: %#v", snapshot.RecentCalls[0])
	}
	known := snapshot.RecentCalls[1]
	if known.TraceID != "" || known.SpanID != "" {
		t.Fatalf("default no-op call unexpectedly created trace identifiers = %q / %q", known.TraceID, known.SpanID)
	}
	if known.Tool != "agentdock_context" || known.Source != observability.SourceMCP {
		t.Fatalf("known call metadata = %#v", known)
	}
	if known.ErrorCode != "INVALID_ARGUMENT" || known.ErrorCategory != "validation" {
		t.Fatalf("known call error metadata = %#v", known)
	}

	encoded, err := json.Marshal(runtime.RuntimeAnalytics())
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	for _, forbidden := range []string{secret, "future_field", "secret-tool-"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("runtime analytics leaked %q: %s", forbidden, body)
		}
	}
}

func TestRuntimeAnalyticsIncludesCommandStages(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	rt := newRuntimeValidationTestRuntime(t)
	result, err := rt.Call(context.Background(), "exec_command", map[string]any{
		"cmd":            "sleep 0.02",
		"execution_mode": "sync",
		"timeout_ms":     2000,
	})
	if err != nil {
		t.Fatalf("exec_command: %v", err)
	}
	if result["status"] != "exited" {
		t.Fatalf("exec_command result = %#v", result)
	}

	record := rt.observer.Snapshot().RecentCalls[0]
	if record.Tool != "exec_command" || len(record.Stages) != 2 {
		t.Fatalf("command analytics = %#v", record)
	}
	if record.Stages[0].Name != observability.StageCommandStart ||
		record.Stages[1].Name != observability.StageCommandForegroundWait {
		t.Fatalf("command stages = %#v", record.Stages)
	}
}

func TestRuntimeToolLogOmitsTraceIdentifiersWithoutSDK(t *testing.T) {
	rt := newRuntimeValidationTestRuntime(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx := observability.WithSource(context.Background(), observability.SourceMCP)
	if _, err := rt.Call(ctx, "agentdock_context", map[string]any{}); err != nil {
		t.Fatalf("agentdock_context: %v", err)
	}
	record := rt.observer.Snapshot().RecentCalls[0]
	if record.TraceID != "" || record.SpanID != "" {
		t.Fatalf("default no-op call unexpectedly created trace identifiers: %#v", record)
	}
	body := logs.String()
	for _, forbidden := range []string{"\"trace_id\"", "\"span_id\""} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("tool log unexpectedly contains %s: %s", forbidden, body)
		}
	}
}

func TestRuntimeToolLogCorrelatesRemoteTraceWithoutFakeChildSpan(t *testing.T) {
	rt := newRuntimeValidationTestRuntime(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	parentSpanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: parentSpanID, TraceFlags: trace.FlagsSampled, Remote: true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	ctx = observability.WithSource(ctx, observability.SourceNexus)
	if _, err := rt.Call(ctx, "agentdock_context", map[string]any{}); err != nil {
		t.Fatalf("agentdock_context: %v", err)
	}

	record := rt.observer.Snapshot().RecentCalls[0]
	if record.TraceID != traceID.String() || record.SpanID != "" {
		t.Fatalf("remote correlation record = %#v", record)
	}
	body := logs.String()
	if !strings.Contains(body, "\"trace_id\":\""+traceID.String()+"\"") {
		t.Fatalf("tool log missing remote trace id: %s", body)
	}
	if strings.Contains(body, "\"span_id\"") {
		t.Fatalf("tool log invented a local child span id: %s", body)
	}
}

func TestRuntimeDiagnosticsExposeOnlyRemoteSafeRecentCalls(t *testing.T) {
	runtime := newRuntimeValidationTestRuntime(t)
	ctx := observability.WithSource(context.Background(), observability.SourceNexus)
	secret := "/Users/example/private/secret.txt"
	if _, err := runtime.Call(ctx, "agentdock_context", map[string]any{"future_field": secret}); err == nil {
		t.Fatal("expected validation error")
	}

	encoded, err := json.Marshal(runtime.RuntimeDiagnostics())
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		strings.ToLower(secret), "span_id", "tool_stats", "process", "p50_duration_ms",
		"arguments", "output", "command", "path", "error_message", "stack",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("runtime diagnostics leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"error_code":"invalid_argument"`) {
		t.Fatalf("runtime diagnostics missing stable error code: %s", body)
	}
}

func TestRuntimeExecutionTracksSafeTruthfulCalls(t *testing.T) {
	runtime := newRuntimeValidationTestRuntime(t)
	ctx := observability.WithSource(context.Background(), observability.SourceMCP)
	secret := "/Users/example/private/execution-secret"

	if _, err := runtime.Call(ctx, "agentdock_context", map[string]any{"future_field": secret}); err == nil {
		t.Fatal("expected validation error")
	}
	if _, err := runtime.Call(ctx, "secret-tool-"+secret, nil); err == nil {
		t.Fatal("expected unknown tool error")
	}

	snapshot := runtime.execution.Snapshot()
	if snapshot.ActiveCalls != 0 || len(snapshot.Calls) != 2 {
		t.Fatalf("execution snapshot = %#v", snapshot)
	}
	if snapshot.Calls[0].Tool != "agentdock_context" || snapshot.Calls[0].Status != "failed" ||
		snapshot.Calls[0].ErrorCode != "INVALID_ARGUMENT" {
		t.Fatalf("known execution call = %#v", snapshot.Calls[0])
	}
	if snapshot.Calls[1].Tool != "unknown" || snapshot.Calls[1].Status != "failed" {
		t.Fatalf("unknown execution call = %#v", snapshot.Calls[1])
	}

	encoded, err := json.Marshal(runtime.RuntimeExecution())
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	for _, forbidden := range []string{secret, "future_field", "secret-tool-"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("execution snapshot leaked %q: %s", forbidden, body)
		}
	}

	page := runtime.execution.Page(0, 20)
	if len(page.Events) != 4 || page.Events[0].Kind != "call.started" || page.Events[1].Kind != "call.failed" {
		t.Fatalf("execution events = %#v", page.Events)
	}
}

func TestExecutionStatusUsesToolResultFailure(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	rt := newRuntimeValidationTestRuntime(t)
	result, err := rt.Call(context.Background(), "exec_command", map[string]any{
		"cmd": "exit 7", "execution_mode": "sync", "timeout_ms": 2000,
	})
	if err != nil {
		t.Fatalf("exec_command returned transport error: %v", err)
	}
	if result["command_ok"] != false {
		t.Fatalf("command result = %#v", result)
	}
	snapshot := rt.execution.Snapshot()
	call := snapshot.Calls[len(snapshot.Calls)-1]
	if call.Tool != "exec_command" || call.Status != execution.StatusFailed || call.ErrorCode != "COMMAND_FAILED" {
		t.Fatalf("execution call = %#v", call)
	}
}

func TestRuntimeInsertionAckAndDeliveryToActiveToolResult(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	rt := newRuntimeValidationTestRuntime(t)
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := rt.Call(context.Background(), "exec_command", map[string]any{
			"cmd": "sleep 0.25; printf done", "execution_mode": "sync", "timeout_ms": 2000,
		})
		done <- outcome{result: result, err: err}
	}()

	deadline := time.Now().Add(2 * time.Second)
	callID := ""
	for time.Now().Before(deadline) {
		snapshot := rt.execution.Snapshot()
		for _, call := range snapshot.Calls {
			if call.Tool == "exec_command" && call.Status == execution.StatusRunning {
				callID = call.ID
				break
			}
		}
		if callID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if callID == "" {
		t.Fatal("active exec_command call was not observed")
	}

	managed, err := rt.RuntimeInsertionManage(context.Background(), map[string]any{
		"action": "enqueue", "call_id": callID, "text": "please verify before continuing",
	})
	if err != nil {
		t.Fatalf("enqueue insertion: %v", err)
	}
	if managed["ack"] != true {
		t.Fatalf("enqueue response = %#v", managed)
	}
	item, ok := managed["insertion"].(execution.Insertion)
	if !ok || item.ID == "" || item.Status != execution.InsertionAccepted {
		t.Fatalf("accepted insertion = %#v", managed["insertion"])
	}

	completed := <-done
	if completed.err != nil {
		t.Fatalf("exec command: %v", completed.err)
	}
	raw, ok := completed.result["agentdock_user_insertions"].([]execution.InsertionDelivery)
	if !ok || len(raw) != 1 || raw[0].ID != item.ID || raw[0].Text != "please verify before continuing" {
		t.Fatalf("delivered insertion payload = %#v", completed.result["agentdock_user_insertions"])
	}
	items := rt.execution.Insertions(callID)
	if len(items) != 1 || items[0].Status != execution.InsertionDelivered || items[0].DeliveredAt == nil {
		t.Fatalf("insertion state = %#v", items)
	}
}
func TestRuntimeInsertionFailedToolResultRejects(t *testing.T) {
	for _, mode := range []string{"exit", "timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			if goruntime.GOOS == "windows" {
				t.Skip("test command uses POSIX shell syntax")
			}
			rt := newRuntimeValidationTestRuntime(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := "sleep 0.25; exit 1"
			marker := t.TempDir() + "/started"
			if mode == "cancel" {
				cmd = "printf ready > '" + marker + "'; sleep 2"
			}
			timeout := 2000
			if mode == "timeout" {
				cmd = "sleep 2"
				timeout = 250
			}
			type outcome struct {
				result Result
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := rt.Call(ctx, "exec_command", map[string]any{
					"cmd": cmd, "execution_mode": "sync", "timeout_ms": timeout,
				})
				done <- outcome{result: result, err: err}
			}()

			deadline := time.Now().Add(2 * time.Second)
			callID := ""
			for time.Now().Before(deadline) {
				snapshot := rt.execution.Snapshot()
				for _, call := range snapshot.Calls {
					if call.Tool == "exec_command" && call.Status == execution.StatusRunning {
						callID = call.ID
						break
					}
				}
				if callID != "" {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if callID == "" {
				t.Fatal("active exec_command call was not observed")
			}

			managed, err := rt.RuntimeInsertionManage(context.Background(), map[string]any{
				"action": "enqueue", "call_id": callID, "text": "please verify before continuing",
			})
			if err != nil {
				t.Fatalf("enqueue insertion: %v", err)
			}
			if managed["ack"] != true {
				t.Fatalf("enqueue response = %#v", managed)
			}
			item, ok := managed["insertion"].(execution.Insertion)
			if !ok || item.ID == "" || item.Status != execution.InsertionAccepted {
				t.Fatalf("accepted insertion = %#v", managed["insertion"])
			}

			if mode == "cancel" {
				deadline := time.Now().Add(time.Second)
				for {
					if _, err := os.Stat(marker); err == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("command did not reach running boundary")
					}
					time.Sleep(5 * time.Millisecond)
				}
				cancel()
			}
			completed := <-done
			if mode == "cancel" && (completed.err != nil || completed.result["status"] != "running" || completed.result["session_reason"] != "request_cancelled") {
				t.Fatalf("public cancellation contract changed: result=%#v err=%v", completed.result, completed.err)
			}
			if _, exists := completed.result["agentdock_user_insertions"]; exists {
				t.Fatal("failed tool result delivered insertion")
			}
			items := rt.execution.Insertions(callID)
			if len(items) != 1 || items[0].Status != execution.InsertionRejected || items[0].DeliveredAt != nil {
				t.Fatalf("failed insertion = %#v", items)
			}
			call, _ := rt.execution.Call(callID)
			if mode == "cancel" && call.Status != execution.StatusCancelled {
				t.Fatalf("cancelled call status=%s", call.Status)
			}
			if call.Status != execution.StatusFailed && call.Status != execution.StatusCancelled {
				t.Fatalf("call status=%s error=%v", call.Status, completed.err)
			}
		})
	}
}

func TestAsyncCommandContinuationRemainsActiveWithoutConsumingOutput(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	rt := newRuntimeValidationTestRuntime(t)
	const output = "m5-sensitive-output-value"
	started, err := rt.Call(context.Background(), "exec_command", map[string]any{
		"cmd":            "sleep 0.25; printf '" + output + "'",
		"execution_mode": "async",
		"timeout_ms":     2000,
	})
	if err != nil {
		t.Fatalf("exec_command: %v", err)
	}
	sessionID, _ := started["session_id"].(string)
	if sessionID == "" || started["status"] != "running" {
		t.Fatalf("async command result = %#v", started)
	}

	snapshot := rt.execution.Snapshot()
	if snapshot.ActiveCalls != 1 {
		t.Fatalf("active calls = %d, want command continuation only; calls=%#v", snapshot.ActiveCalls, snapshot.Calls)
	}
	var rootCall, continuation execution.Call
	for _, call := range snapshot.Calls {
		switch {
		case call.Tool == "exec_command":
			rootCall = call
		case call.Tool == "command_session" && call.ContinuationID == sessionID:
			continuation = call
		}
	}
	if rootCall.ID == "" || rootCall.Status != execution.StatusCompleted {
		t.Fatalf("root call = %#v", rootCall)
	}
	if continuation.ID == "" || continuation.ParentCallID != rootCall.ID || continuation.Status != execution.StatusRunning {
		t.Fatalf("continuation = %#v root=%#v", continuation, rootCall)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		view, ok := rt.execution.Call(continuation.ID)
		if ok && view.Status != execution.StatusRunning {
			continuation = view
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if continuation.Status != execution.StatusCompleted {
		t.Fatalf("continuation did not complete: %#v", continuation)
	}

	observed, err := rt.Call(context.Background(), "session_observe", map[string]any{
		"action": "status", "session_id": sessionID, "max_output_bytes": 4096,
	})
	if err != nil {
		t.Fatalf("session_observe: %v", err)
	}
	if observed["stdout"] != output {
		t.Fatalf("execution observer consumed stdout: %#v", observed)
	}

	page := rt.execution.Page(0, 200)
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), output) {
		t.Fatalf("execution journal leaked stdout content: %s", encoded)
	}
	foundFinalOutput := false
	for _, event := range page.Events {
		if event.CallID == continuation.ID && event.Kind == execution.EventOutputSummary &&
			event.Output != nil && event.Output.Status == "exited" && event.Output.StdoutTotalBytes == len(output) {
			foundFinalOutput = true
		}
	}
	if !foundFinalOutput {
		t.Fatalf("missing bounded continuation output fact: %#v", page.Events)
	}
}

func TestFileEditExecutionFactOmitsContentsAndDiff(t *testing.T) {
	rt := newRuntimeValidationTestRuntime(t)
	const secretContent = "m5-file-content-must-not-enter-journal"
	result, err := rt.Call(context.Background(), "file_edit", map[string]any{
		"action": "add", "path": "m5-execution-fact.txt", "content": secretContent,
	})
	if err != nil {
		t.Fatalf("file_edit: %v", err)
	}
	if result["changed"] != true {
		t.Fatalf("file_edit result = %#v", result)
	}

	page := rt.execution.Page(0, 200)
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if strings.Contains(body, secretContent) || strings.Contains(body, "diff_preview") {
		t.Fatalf("execution journal leaked file contents/diff: %s", body)
	}
	found := false
	for _, event := range page.Events {
		if event.Kind != execution.EventFileChanged || event.FileChange == nil {
			continue
		}
		if event.FileChange.Action == "add" && len(event.FileChange.Paths) == 1 &&
			strings.Contains(event.FileChange.Paths[0], "m5-execution-fact.txt") {
			found = true
			if event.FileChange.FilesChanged != 1 || !event.FileChange.StatsKnown {
				t.Fatalf("file change stats = %#v", event.FileChange)
			}
		}
	}
	if !found {
		t.Fatalf("file change fact missing: %#v", page.Events)
	}
}
