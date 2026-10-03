package execution

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestStoreTracksTruthfulRootAndChildLifecycle(t *testing.T) {
	store := NewStore(16, 8)
	rootCtx, root := store.Begin(context.Background(), BeginInput{Tool: "exec_command", Source: "mcp"})
	childCtx, child := store.Begin(rootCtx, BeginInput{Tool: "dynamic-mcp:search", Source: "internal"})
	if ScopeFromContext(childCtx).CallID != child.ID {
		t.Fatal("child scope missing call id")
	}
	if child.ParentCallID != root.ID {
		t.Fatalf("child parent = %q, want %q", child.ParentCallID, root.ID)
	}

	if _, ok := store.SetWaiting(root.ID, true); !ok {
		t.Fatal("root waiting transition failed")
	}
	snapshot := store.Snapshot()
	if snapshot.ActiveCalls != 2 {
		t.Fatalf("active calls = %d, want 2", snapshot.ActiveCalls)
	}
	rootView, _ := store.Call(root.ID)
	if rootView.Status != StatusWaitingForUser {
		t.Fatalf("root status = %s", rootView.Status)
	}

	store.Finish(child.ID, FinishInput{Status: StatusCompleted})
	store.Finish(root.ID, FinishInput{Status: StatusCancelled, ErrorCode: "CANCELED", ErrorCategory: "runtime"})
	snapshot = store.Snapshot()
	if snapshot.ActiveCalls != 0 {
		t.Fatalf("active calls = %d after finish", snapshot.ActiveCalls)
	}
	rootView, _ = store.Call(root.ID)
	if rootView.Status != StatusCancelled || rootView.ErrorCode != "CANCELED" {
		t.Fatalf("root terminal state = %#v", rootView)
	}
}

func TestStoreReplayReportsPrunedGap(t *testing.T) {
	store := NewStore(3, 8)
	for index := 0; index < 3; index++ {
		_, call := store.Begin(context.Background(), BeginInput{Tool: "tool" + strconv.Itoa(index), Source: "test"})
		store.Finish(call.ID, FinishInput{Status: StatusCompleted})
	}
	snapshot := store.Snapshot()
	if snapshot.LatestSequence != "6" {
		t.Fatalf("latest = %s, want 6", snapshot.LatestSequence)
	}
	page := store.Page(0, 10)
	if !page.Gap || page.PrunedThrough != "3" {
		t.Fatalf("gap page = %#v", page)
	}
	if len(page.Events) != 3 || page.Events[0].Sequence != "4" || page.Events[2].Sequence != "6" {
		t.Fatalf("events = %#v", page.Events)
	}
}

func TestRecentInteractionNeverRevivesCompletedCall(t *testing.T) {
	store := NewStore(16, 8)
	_, call := store.Begin(context.Background(), BeginInput{Tool: "file_edit", Source: "mcp"})
	store.Finish(call.ID, FinishInput{Status: StatusCompleted})
	if _, ok := store.SetWaiting(call.ID, true); !ok {
		t.Fatal("completed call lookup failed")
	}
	view, _ := store.Call(call.ID)
	if view.Status != StatusCompleted {
		t.Fatalf("completed call revived to %s", view.Status)
	}
}

func TestStoreWaitWakesOnlyAfterNewSequence(t *testing.T) {
	store := NewStore(8, 8)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- store.Wait(ctx, 0)
	}()

	select {
	case err := <-done:
		t.Fatalf("wait returned before an event: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	store.Begin(context.Background(), BeginInput{Tool: "demo", Source: "test"})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not wake after new sequence")
	}
}
