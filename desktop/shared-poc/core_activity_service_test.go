package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/desktopapi"
	"github.com/uvwt/agentdock/internal/execution"
)

type fakeCoreExecutionClient struct {
	mu            sync.Mutex
	snapshot      execution.Snapshot
	replay        execution.Page
	streamMessage *desktopapi.ExecutionStreamMessage
	snapshotCalls int
	replayCalls   int
	streamCalls   int
	snapshotFn    func(context.Context) (execution.Snapshot, error)
	replayFn      func(context.Context, uint64, int) (execution.Page, error)
	streamFn      func(context.Context, string, uint64, func(desktopapi.ExecutionStreamMessage) error) error
}

func (f *fakeCoreExecutionClient) Snapshot(ctx context.Context) (execution.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshotCalls++
	if f.snapshotFn != nil {
		return f.snapshotFn(ctx)
	}
	return f.snapshot, nil
}

func (f *fakeCoreExecutionClient) Replay(ctx context.Context, after uint64, limit int) (execution.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replayCalls++
	if f.replayFn != nil {
		return f.replayFn(ctx, after, limit)
	}
	return f.replay, nil
}

func (f *fakeCoreExecutionClient) Stream(ctx context.Context, epoch string, after uint64, callback func(desktopapi.ExecutionStreamMessage) error) error {
	f.mu.Lock()
	f.streamCalls++
	message := f.streamMessage
	f.mu.Unlock()
	if err := callback(desktopapi.ExecutionStreamMessage{Kind: "connected"}); err != nil {
		return err
	}
	if f.streamFn != nil {
		return f.streamFn(ctx, epoch, after, callback)
	}
	if message != nil {
		if err := callback(*message); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeCoreExecutionClient) EnqueueInsertion(_ context.Context, callID, text string) (execution.Insertion, error) {
	return execution.Insertion{ID: "ins_test", TargetCallID: callID, Status: execution.InsertionAccepted, TextPreview: text}, nil
}

func (f *fakeCoreExecutionClient) CancelInsertion(_ context.Context, insertionID string) (execution.Insertion, error) {
	return execution.Insertion{ID: insertionID, Status: execution.InsertionCancelled}, nil
}

type captureStreamSender struct {
	mu             sync.Mutex
	messages       [][]byte
	attempts       int
	failAt         int
	cancel         context.CancelFunc
	cancelAfterOK  int
	successfulSend int
}

func (s *captureStreamSender) TrySend(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	if s.failAt > 0 && s.attempts == s.failAt {
		return errors.New("synthetic backpressure")
	}
	s.messages = append(s.messages, append([]byte(nil), data...))
	s.successfulSend++
	if s.cancel != nil && s.cancelAfterOK > 0 && s.successfulSend >= s.cancelAfterOK {
		s.cancel()
	}
	return nil
}

func TestCoreActivityServiceSendsSnapshotThenActivity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &fakeCoreExecutionClient{
		snapshot: execution.Snapshot{
			SchemaVersion:  execution.SchemaVersion,
			Epoch:          "epoch_demo",
			LatestSequence: "2",
			Calls:          []execution.Call{{ID: "call_root", Tool: "agentdock_context", Status: execution.StatusCompleted}},
		},
		streamMessage: &desktopapi.ExecutionStreamMessage{
			Kind: "activity",
			ID:   "3",
			Page: execution.Page{
				SchemaVersion:  execution.SchemaVersion,
				Epoch:          "epoch_demo",
				After:          "2",
				LatestSequence: "3",
				Events: []execution.Event{{
					SchemaVersion: execution.SchemaVersion,
					Epoch:         "epoch_demo",
					Sequence:      "3",
					Kind:          execution.EventCallStarted,
					CallID:        "call_next",
				}},
			},
		},
	}
	sender := &captureStreamSender{cancel: cancel, cancelAfterOK: 3}
	service := &CoreActivityService{client: client}
	service.run(ctx, sender)

	if len(sender.messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(sender.messages))
	}
	var first, second, third map[string]any
	if err := json.Unmarshal(sender.messages[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sender.messages[1], &second); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sender.messages[2], &third); err != nil {
		t.Fatal(err)
	}
	if first["kind"] != "snapshot" || second["kind"] != "status" || second["code"] != "execution_synchronized" || third["kind"] != "activity" {
		t.Fatalf("messages = %#v / %#v / %#v", first, second, third)
	}
	if service.transportDropped.Load() != 0 {
		t.Fatalf("transport dropped = %d", service.transportDropped.Load())
	}
}

func TestCoreActivityServiceBackpressureForcesResnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &fakeCoreExecutionClient{
		snapshot: execution.Snapshot{
			SchemaVersion:  execution.SchemaVersion,
			Epoch:          "epoch_demo",
			LatestSequence: "2",
			Calls:          []execution.Call{},
		},
		streamMessage: &desktopapi.ExecutionStreamMessage{
			Kind: "activity",
			ID:   "3",
			Page: execution.Page{
				SchemaVersion:  execution.SchemaVersion,
				Epoch:          "epoch_demo",
				After:          "2",
				LatestSequence: "3",
				Events: []execution.Event{{
					SchemaVersion: execution.SchemaVersion,
					Epoch:         "epoch_demo",
					Sequence:      "3",
					Kind:          execution.EventCallStarted,
					CallID:        "call_next",
				}},
			},
		},
	}
	sender := &captureStreamSender{failAt: 3, cancel: cancel, cancelAfterOK: 4}
	service := &CoreActivityService{client: client}
	service.run(ctx, sender)

	if client.snapshotCalls < 2 {
		t.Fatalf("snapshot calls = %d, want resnapshot after transport drop", client.snapshotCalls)
	}
	if service.transportDropped.Load() != 1 {
		t.Fatalf("transport dropped = %d, want 1", service.transportDropped.Load())
	}
	if len(sender.messages) != 4 {
		t.Fatalf("successful messages = %d, want snapshot + synchronized + reconnecting + recovery snapshot", len(sender.messages))
	}
	wantKinds := []string{"snapshot", "status", "status", "snapshot"}
	wantCodes := []string{"", "execution_synchronized", "execution_reconnecting", ""}
	for index, raw := range sender.messages {
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["kind"] != wantKinds[index] || (wantCodes[index] != "" && payload["code"] != wantCodes[index]) {
			t.Fatalf("message %d = %#v", index, payload)
		}
	}
}

func TestCoreActivityServiceInsertionControls(t *testing.T) {
	client := &fakeCoreExecutionClient{}
	service := &CoreActivityService{client: client}

	accepted := service.EnqueueInsertion("call_demo", "change direction")
	if accepted.Error != nil || !accepted.ACK || accepted.Insertion.ID != "ins_test" ||
		accepted.Insertion.TargetCallID != "call_demo" || accepted.Insertion.Status != execution.InsertionAccepted {
		t.Fatalf("enqueue result = %#v", accepted)
	}

	cancelled := service.CancelInsertion(accepted.Insertion.ID)
	if cancelled.Error != nil || cancelled.Insertion.ID != accepted.Insertion.ID ||
		cancelled.Insertion.Status != execution.InsertionCancelled {
		t.Fatalf("cancel result = %#v", cancelled)
	}
}

func TestCoreActivityReconnectReplaysSameEpoch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := &fakeCoreExecutionClient{snapshot: execution.Snapshot{Epoch: "a", LatestSequence: "2"}}
	streams := 0
	client.streamFn = func(ctx context.Context, epoch string, after uint64, cb func(desktopapi.ExecutionStreamMessage) error) error {
		streams++
		if streams == 1 {
			if after != 2 {
				t.Fatalf("initial cursor=%d", after)
			}
			client.snapshot = execution.Snapshot{Epoch: "a", LatestSequence: "4"}
			return errors.New("interrupted")
		}
		if after != 4 {
			t.Fatalf("reconnected cursor=%d", after)
		}
		cancel()
		return ctx.Err()
	}
	client.replayFn = func(_ context.Context, after uint64, limit int) (execution.Page, error) {
		if limit != 200 {
			t.Fatalf("unbounded limit=%d", limit)
		}
		return execution.Page{Epoch: "a", Events: []execution.Event{{Sequence: strconv.FormatUint(after+1, 10)}}, HasMore: after < 3}, nil
	}
	sender := &captureStreamSender{}
	service := &CoreActivityService{client: client}
	service.run(ctx, sender)
	if streams != 2 || client.replayCalls != 2 || len(sender.messages) != 6 || service.reconnects.Load() != 1 {
		t.Fatalf("streams=%d replay=%d messages=%d", streams, client.replayCalls, len(sender.messages))
	}
	var payloads []map[string]any
	for _, raw := range sender.messages {
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, payload)
	}
	if payloads[1]["code"] != "execution_synchronized" || payloads[2]["code"] != "execution_reconnecting" ||
		payloads[5]["code"] != "execution_synchronized" {
		t.Fatalf("reconnect status order = %#v", payloads)
	}
}
func TestCoreActivityReconcileResyncAndBounds(t *testing.T) {
	for _, mode := range []string{"epoch", "gap", "restart_during_replay", "empty_more", "nonadvancing", "continuation", "send_failure", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &fakeCoreExecutionClient{snapshot: execution.Snapshot{Epoch: "a", LatestSequence: "10000"}}
			sender := &captureStreamSender{}
			epoch := "a"
			cursor := uint64(2)
			if mode == "epoch" {
				client.snapshot.Epoch = "b"
			}
			if mode == "cancelled" {
				cancel()
			}
			client.replayFn = func(ctx context.Context, after uint64, limit int) (execution.Page, error) {
				if err := ctx.Err(); err != nil {
					return execution.Page{}, err
				}
				page := execution.Page{Epoch: "a", Events: []execution.Event{{Sequence: strconv.FormatUint(after+1, 10)}}}
				switch mode {
				case "gap":
					page.Gap = true
				case "restart_during_replay":
					page.Epoch = "b"
				case "empty_more":
					page.Events = nil
					page.HasMore = true
				case "nonadvancing":
					page.Events[0].Sequence = strconv.FormatUint(after, 10)
					page.HasMore = true
				case "continuation":
					page.HasMore = true
				}
				return page, nil
			}
			if mode == "send_failure" {
				sender.failAt = 1
			}
			service := &CoreActivityService{client: client}
			ok := service.reconcile(ctx, sender, &epoch, &cursor)
			if mode == "epoch" {
				if !ok || epoch != "b" || cursor != 10000 || client.replayCalls != 0 {
					t.Fatal("epoch did not resnapshot")
				}
				return
			}
			if ok {
				t.Fatal("must request a fresh snapshot after unsafe replay")
			}
			if client.replayCalls > 16 {
				t.Fatal("unbounded continuation")
			}
			if mode == "gap" || mode == "restart_during_replay" {
				if len(sender.messages) != 0 || epoch != "a" || cursor != 2 {
					t.Fatal("sent stale snapshot / advanced cursor")
				}
			}
			if mode == "send_failure" && (cursor != 2 || service.transportDropped.Load() != 1) {
				t.Fatal("drop accounting/cursor")
			}
		})
	}
}
func TestCoreActivityStreamResetAndCancellation(t *testing.T) {
	for _, mode := range []string{"reset", "epoch", "gap", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &fakeCoreExecutionClient{snapshot: execution.Snapshot{Epoch: "a", LatestSequence: "2"}}
			client.streamFn = func(ctx context.Context, epoch string, after uint64, cb func(desktopapi.ExecutionStreamMessage) error) error {
				if mode == "cancel" {
					cancel()
					return ctx.Err()
				}
				message := desktopapi.ExecutionStreamMessage{Kind: "activity", Page: execution.Page{Epoch: "a"}}
				switch mode {
				case "reset":
					message.Kind = "reset"
				case "epoch":
					message.Page.Epoch = "b"
				case "gap":
					message.Page.Gap = true
				}
				client.snapshot = execution.Snapshot{Epoch: "b", LatestSequence: "10"}
				return cb(message)
			}
			cancelAfter := 4
			if mode == "cancel" {
				cancelAfter = 2
			}
			sender := &captureStreamSender{cancel: cancel, cancelAfterOK: cancelAfter}
			service := &CoreActivityService{client: client}
			service.run(ctx, sender)
			want := 4
			if mode == "cancel" {
				want = 2
			}
			if len(sender.messages) != want || client.replayCalls != 0 {
				t.Fatalf("wrong recovery path: messages=%d replay=%d", len(sender.messages), client.replayCalls)
			}
			if mode != "cancel" {
				var reconnect map[string]any
				if err := json.Unmarshal(sender.messages[2], &reconnect); err != nil {
					t.Fatal(err)
				}
				if reconnect["kind"] != "status" || reconnect["code"] != "execution_reconnecting" {
					t.Fatalf("missing reconnecting status: %#v", reconnect)
				}
				var payload map[string]json.RawMessage
				json.Unmarshal(sender.messages[3], &payload)
				var snap execution.Snapshot
				json.Unmarshal(payload["snapshot"], &snap)
				if snap.Epoch != "b" || snap.LatestSequence != "10" {
					t.Fatal("recovery snapshot stale")
				}
			}
		})
	}
}
