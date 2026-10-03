package execution

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestInsertionAcceptedDeliveredAndStable(t *testing.T) {
	store := NewStore(64, 16)
	_, call := store.Begin(context.Background(), BeginInput{InsertionSupported: true, Tool: "exec_command", Source: "mcp"})

	item, err := store.EnqueueInsertion(call.ID, "please verify the output")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || item.TargetCallID != call.ID || item.Status != InsertionAccepted {
		t.Fatalf("accepted insertion = %#v", item)
	}
	if item.TextPreview != "please verify the output" || item.TextBytes != len("please verify the output") {
		t.Fatalf("insertion metadata = %#v", item)
	}

	deliveries := store.DeliverInsertions(call.ID)
	if len(deliveries) != 1 || deliveries[0].ID != item.ID || deliveries[0].Text != "please verify the output" {
		t.Fatalf("deliveries = %#v", deliveries)
	}
	if again := store.DeliverInsertions(call.ID); len(again) != 0 {
		t.Fatalf("insertion delivered twice: %#v", again)
	}
	items := store.Insertions(call.ID)
	if len(items) != 1 || items[0].ID != item.ID || items[0].Status != InsertionDelivered || items[0].DeliveredAt == nil {
		t.Fatalf("delivered insertion state = %#v", items)
	}
}

func TestInsertionRejectCancelExpireAndBounds(t *testing.T) {
	store := NewStore(64, 16)
	_, call := store.Begin(context.Background(), BeginInput{InsertionSupported: true, Tool: "exec_command", Source: "mcp"})

	cancelled, err := store.EnqueueInsertion(call.ID, "cancel me")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err = store.CancelInsertion(cancelled.ID)
	if err != nil || cancelled.Status != InsertionCancelled {
		t.Fatalf("cancel = %#v err=%v", cancelled, err)
	}

	expiring, err := store.EnqueueInsertion(call.ID, "expire me")
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	record := store.insertions[expiring.ID]
	record.public.ExpiresAt = time.Now().Add(-time.Second)
	store.insertions[expiring.ID] = record
	store.mu.Unlock()
	snapshot := store.Snapshot()
	foundExpired := false
	for _, insertion := range snapshot.Insertions {
		if insertion.ID == expiring.ID {
			foundExpired = insertion.Status == InsertionExpired
		}
	}
	if !foundExpired {
		t.Fatalf("expired insertion not visible: %#v", snapshot.Insertions)
	}

	pending, err := store.EnqueueInsertion(call.ID, "reject me")
	if err != nil {
		t.Fatal(err)
	}
	rejected := store.RejectInsertions(call.ID, "target_failed")
	if len(rejected) != 1 || rejected[0].ID != pending.ID || rejected[0].Status != InsertionRejected {
		t.Fatalf("rejected = %#v", rejected)
	}

	store.Finish(call.ID, FinishInput{Status: StatusCompleted})
	if _, err := store.EnqueueInsertion(call.ID, "too late"); err != ErrInsertionTarget {
		t.Fatalf("terminal enqueue error = %v", err)
	}
	if _, err := store.EnqueueInsertion("", "invalid"); err != ErrInsertionInvalid {
		t.Fatalf("invalid target error = %v", err)
	}
	if _, err := store.EnqueueInsertion(call.ID, strings.Repeat("x", MaxInsertionTextBytes+1)); err != ErrInsertionInvalid {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestInsertionTerminalAndConcurrentTransitions(t *testing.T) {
	for _, status := range []Status{StatusCompleted, StatusFailed, StatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			s := NewStore(500, 16)
			_, call := s.Begin(context.Background(), BeginInput{InsertionSupported: true, Tool: "test"})
			item, _ := s.EnqueueInsertion(call.ID, "pending")
			s.Finish(call.ID, FinishInput{Status: status})
			got, err := s.CancelInsertion(item.ID)
			if err != nil || got.Status != InsertionRejected {
				t.Fatalf("terminal insertion: %#v %v", got, err)
			}
			if len(s.DeliverInsertions(call.ID)) != 0 {
				t.Fatal("delivered after terminal call")
			}
			if _, err := s.EnqueueInsertion(call.ID, "late"); err != ErrInsertionTarget {
				t.Fatal(err)
			}
		})
	}
	s := NewStore(500, 16)
	_, call := s.Begin(context.Background(), BeginInput{InsertionSupported: true, Tool: "test"})
	item, _ := s.EnqueueInsertion(call.ID, "once")
	var wg sync.WaitGroup
	var mu sync.Mutex
	deliveries := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := s.DeliverInsertions(call.ID)
			mu.Lock()
			deliveries += len(out)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if deliveries != 1 {
		t.Fatalf("deliveries=%d", deliveries)
	}
	first, _ := s.CancelInsertion(item.ID)
	second, _ := s.CancelInsertion(item.ID)
	if first.Status != InsertionDelivered || second.Status != first.Status || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatal("cancel changed delivered state")
	}
	_, call2 := s.Begin(context.Background(), BeginInput{InsertionSupported: true, Tool: "test"})
	item2, _ := s.EnqueueInsertion(call2.ID, "cancel")
	first, _ = s.CancelInsertion(item2.ID)
	second, _ = s.CancelInsertion(item2.ID)
	if second.Status != InsertionCancelled || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatal("cancel not idempotent")
	}
	counts := map[EventKind]int{}
	for _, event := range s.Page(0, 500).Events {
		if event.InsertionID == item.ID || event.InsertionID == item2.ID {
			counts[event.Kind]++
		}
	}
	if counts[EventInsertionAccepted] != 2 || counts[EventInsertionDelivered] != 1 || counts[EventInsertionCancelled] != 1 {
		t.Fatal(counts)
	}
}

func TestInsertionBoundsUTF8AndVisibility(t *testing.T) {
	s := NewStore(500, 16)
	_, call := s.Begin(context.Background(), BeginInput{InsertionSupported: true, Tool: "test"})
	for _, text := range []string{"", "  ", string([]byte{0xff}), strings.Repeat("x", MaxInsertionTextBytes+1)} {
		if _, err := s.EnqueueInsertion(call.ID, text); err != ErrInsertionInvalid {
			t.Fatalf("invalid text error=%v", err)
		}
	}
	item, err := s.EnqueueInsertion(call.ID, strings.Repeat("界", 100))
	if err != nil || !utf8.ValidString(item.TextPreview) || len(item.TextPreview) > MaxInsertionPreview+3 {
		t.Fatalf("preview=%q err=%v", item.TextPreview, err)
	}
	for i := 1; i < MaxInsertions; i++ {
		if _, err := s.EnqueueInsertion(call.ID, "bounded"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.EnqueueInsertion(call.ID, "overflow"); err != ErrInsertionCapacity {
		t.Fatal(err)
	}
	if len(s.Snapshot().Insertions) != MaxInsertions {
		t.Fatal("snapshot missing accepted insertions")
	}
	s.CancelInsertion(item.ID)
	got, err := s.CancelInsertion(item.ID)
	if err != nil || got.ID != item.ID || got.Status != InsertionCancelled {
		t.Fatal("terminal record pruned on read")
	}
	if _, err := s.EnqueueInsertion(call.ID, "replacement"); err != nil {
		t.Fatal(err)
	}
	if len(s.Snapshot().Insertions) != MaxInsertions {
		t.Fatal("capacity changed")
	}
}

func TestInsertionEnqueueFinishRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		s := NewStore(16, 1)
		_, call := s.Begin(context.Background(), BeginInput{InsertionSupported: true, Tool: "test"})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); s.EnqueueInsertion(call.ID, "race") }()
		go func() { defer wg.Done(); s.Finish(call.ID, FinishInput{Status: StatusFailed}) }()
		wg.Wait()
		for _, item := range s.Snapshot().Insertions {
			if item.Status != InsertionRejected {
				t.Fatalf("terminal call left insertion %s", item.Status)
			}
		}
	}
}

func TestInsertionRejectsUnsupportedActiveChild(t *testing.T) {
	s := NewStore(32, 8)
	rootCtx, root := s.Begin(context.Background(), BeginInput{
		Tool:               "mcp_tool_call",
		Source:             "mcp",
		InsertionSupported: true,
	})
	child := s.BeginChild(root.ID, BeginInput{
		Tool:   "demo:echo",
		Source: "dynamic_mcp",
	})
	if _, err := s.EnqueueInsertion(root.ID, "root input"); err != nil {
		t.Fatalf("root insertion rejected: %v", err)
	}
	if _, err := s.EnqueueInsertion(child.ID, "child input"); err != ErrInsertionTarget {
		t.Fatalf("unsupported child insertion error = %v, want %v", err, ErrInsertionTarget)
	}

	_, commandChild := s.Begin(rootCtx, BeginInput{
		Tool:           "command_session",
		Source:         "command",
		ContinuationID: "session_demo",
	})
	if _, err := s.EnqueueInsertion(commandChild.ID, "session input"); err != ErrInsertionTarget {
		t.Fatalf("command-session insertion error = %v, want %v", err, ErrInsertionTarget)
	}
}

func TestInsertionExpiryWakesIdleWaiter(t *testing.T) {
	s := NewStore(32, 8)
	_, call := s.Begin(context.Background(), BeginInput{
		Tool:               "exec_command",
		Source:             "mcp",
		InsertionSupported: true,
	})
	item, err := s.EnqueueInsertion(call.ID, "expire while idle")
	if err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	record := s.insertions[item.ID]
	record.public.ExpiresAt = time.Now().UTC().Add(40 * time.Millisecond)
	s.insertions[item.ID] = record
	after := s.nextSeq
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Wait(ctx, after); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	page := s.Page(after, 16)
	if len(page.Events) != 1 || page.Events[0].Kind != EventInsertionExpired || page.Events[0].InsertionID != item.ID {
		t.Fatalf("expiry page = %#v", page)
	}
	items := s.Insertions(call.ID)
	if len(items) != 1 || items[0].Status != InsertionExpired || items[0].Reason != "delivery_deadline_elapsed" {
		t.Fatalf("expired insertion = %#v", items)
	}
}
