package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/uvwt/agentdock/internal/desktopapi"
	"github.com/uvwt/agentdock/internal/execution"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const executionStreamName = "desktop:execution"

const maxExecutionReplayPages = 16

var errExecutionResync = errors.New("execution stream requires resync")

type coreExecutionClient interface {
	Snapshot(context.Context) (execution.Snapshot, error)
	Replay(context.Context, uint64, int) (execution.Page, error)
	Stream(context.Context, string, uint64, func(desktopapi.ExecutionStreamMessage) error) error
	EnqueueInsertion(context.Context, string, string) (execution.Insertion, error)
	CancelInsertion(context.Context, string) (execution.Insertion, error)
}

type CoreActivityService struct {
	client coreExecutionClient

	reconnects       atomic.Uint64
	transportDropped atomic.Uint64
}

func NewCoreActivityService(root string) *CoreActivityService {
	return &CoreActivityService{client: desktopapi.NewExecutionClient(root)}
}

func (s *CoreActivityService) EnqueueInsertion(callID, text string) InsertionControlResult {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	item, err := s.client.EnqueueInsertion(ctx, callID, text)
	if err != nil {
		return InsertionControlResult{
			Error: desktopapi.ErrorFrom("execution_insertion_enqueue_failed", desktopapi.ErrorCategoryOperation, true, err),
		}
	}
	return InsertionControlResult{Insertion: item, ACK: true}
}

func (s *CoreActivityService) CancelInsertion(insertionID string) InsertionControlResult {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	item, err := s.client.CancelInsertion(ctx, insertionID)
	if err != nil {
		return InsertionControlResult{
			Error: desktopapi.ErrorFrom("execution_insertion_cancel_failed", desktopapi.ErrorCategoryOperation, true, err),
		}
	}
	return InsertionControlResult{Insertion: item}
}

func (s *CoreActivityService) serveStream(conn *application.StreamConn) {
	s.run(conn.Context(), conn)
}

func (s *CoreActivityService) run(ctx context.Context, sender streamSender) {
	if s == nil || s.client == nil || sender == nil {
		return
	}
	var epoch string
	var cursor uint64
	needsSnapshot := true
	backoff := 250 * time.Millisecond

	for {
		if ctx.Err() != nil {
			return
		}
		if needsSnapshot {
			snapshot, err := s.client.Snapshot(ctx)
			if err != nil {
				s.trySendError(sender, "execution_core_unavailable")
				if !waitExecutionRetry(ctx, backoff) {
					return
				}
				backoff = nextExecutionBackoff(backoff)
				continue
			}
			if !s.trySendSnapshot(sender, snapshot) {
				if !waitExecutionRetry(ctx, backoff) {
					return
				}
				backoff = nextExecutionBackoff(backoff)
				continue
			}
			epoch = snapshot.Epoch
			cursor = decimalExecutionCursor(snapshot.LatestSequence)
			needsSnapshot = false
			backoff = 250 * time.Millisecond
		}

		if ctx.Err() != nil {
			return
		}
		streamErr := s.client.Stream(ctx, epoch, cursor, func(message desktopapi.ExecutionStreamMessage) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if message.Kind == "connected" {
				if !s.trySendStatus(sender, "execution_synchronized") {
					needsSnapshot = true
					return errExecutionResync
				}
				backoff = 250 * time.Millisecond
				return nil
			}
			if message.Kind == "reset" || message.Page.Gap || message.Page.Epoch != epoch {
				needsSnapshot = true
				return errExecutionResync
			}
			if !s.trySendPage(sender, message.Page, message.ID) {
				needsSnapshot = true
				return errExecutionResync
			}
			cursor = pageExecutionCursor(message.Page, cursor)
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		s.reconnects.Add(1)
		if !s.trySendStatus(sender, "execution_reconnecting") {
			needsSnapshot = true
		}

		if errors.Is(streamErr, errExecutionResync) || needsSnapshot {
			continue
		}
		if !s.reconcile(ctx, sender, &epoch, &cursor) {
			needsSnapshot = true
		}
		if !waitExecutionRetry(ctx, backoff) {
			return
		}
		backoff = nextExecutionBackoff(backoff)
	}
}

func (s *CoreActivityService) reconcile(ctx context.Context, sender streamSender, epoch *string, cursor *uint64) bool {
	snapshot, err := s.client.Snapshot(ctx)
	if err != nil {
		return false
	}
	if snapshot.Epoch != *epoch {
		if !s.trySendSnapshot(sender, snapshot) {
			return false
		}
		*epoch = snapshot.Epoch
		*cursor = decimalExecutionCursor(snapshot.LatestSequence)
		return true
	}

	for attempt := 0; attempt < maxExecutionReplayPages; attempt++ {
		if ctx.Err() != nil {
			return false
		}
		page, err := s.client.Replay(ctx, *cursor, 200)
		if err != nil {
			return false
		}
		if page.Epoch != *epoch || page.Gap {
			// The snapshot was read before replay; Core may have restarted or pruned since then.
			return false
		}
		next := pageExecutionCursor(page, *cursor)
		if (len(page.Events) > 0 && next <= *cursor) || (page.HasMore && len(page.Events) == 0) {
			return false
		}

		if len(page.Events) > 0 {
			if !s.trySendPage(sender, page, page.Events[len(page.Events)-1].Sequence) {
				return false
			}
			*cursor = pageExecutionCursor(page, *cursor)
		}
		if !page.HasMore || *cursor >= decimalExecutionCursor(snapshot.LatestSequence) {
			return true
		}
	}
	return false
}

func (s *CoreActivityService) trySendSnapshot(sender streamSender, snapshot execution.Snapshot) bool {
	payload := map[string]any{
		"kind":                  "snapshot",
		"schemaVersion":         execution.SchemaVersion,
		"snapshot":              snapshot,
		"reconnectTotal":        strconv.FormatUint(s.reconnects.Load(), 10),
		"transportDroppedTotal": strconv.FormatUint(s.transportDropped.Load(), 10),
	}
	return s.trySend(sender, payload, 1)
}

func (s *CoreActivityService) trySendPage(sender streamSender, page execution.Page, id string) bool {
	payload := map[string]any{
		"kind":                  "activity",
		"schemaVersion":         execution.SchemaVersion,
		"id":                    id,
		"page":                  page,
		"reconnectTotal":        strconv.FormatUint(s.reconnects.Load(), 10),
		"transportDroppedTotal": strconv.FormatUint(s.transportDropped.Load(), 10),
	}
	dropped := len(page.Events)
	if dropped < 1 {
		dropped = 1
	}
	return s.trySend(sender, payload, uint64(dropped))
}

func (s *CoreActivityService) trySendStatus(sender streamSender, code string) bool {
	payload := map[string]any{
		"kind":                  "status",
		"schemaVersion":         execution.SchemaVersion,
		"code":                  code,
		"reconnectTotal":        strconv.FormatUint(s.reconnects.Load(), 10),
		"transportDroppedTotal": strconv.FormatUint(s.transportDropped.Load(), 10),
	}
	return s.trySend(sender, payload, 1)
}

func (s *CoreActivityService) trySendError(sender streamSender, code string) bool {
	payload := map[string]any{
		"kind":                  "error",
		"schemaVersion":         execution.SchemaVersion,
		"code":                  code,
		"reconnectTotal":        strconv.FormatUint(s.reconnects.Load(), 10),
		"transportDroppedTotal": strconv.FormatUint(s.transportDropped.Load(), 10),
	}
	return s.trySend(sender, payload, 1)
}

func (s *CoreActivityService) trySend(sender streamSender, payload any, dropped uint64) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		s.transportDropped.Add(dropped)
		return false
	}
	if err := sender.TrySend(data); err != nil {
		s.transportDropped.Add(dropped)
		return false
	}
	return true
}

func pageExecutionCursor(page execution.Page, fallback uint64) uint64 {
	if len(page.Events) == 0 {
		return fallback
	}
	value, err := strconv.ParseUint(page.Events[len(page.Events)-1].Sequence, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func decimalExecutionCursor(value string) uint64 {
	cursor, _ := strconv.ParseUint(value, 10, 64)
	return cursor
}

func nextExecutionBackoff(current time.Duration) time.Duration {
	current *= 2
	if current > 5*time.Second {
		return 5 * time.Second
	}
	return current
}

func waitExecutionRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
