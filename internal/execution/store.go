package execution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultEventCapacity = 2048
	DefaultCallCapacity  = 512
	MaxPageEvents        = 500
)

type Store struct {
	mu sync.RWMutex

	epoch string

	events    []Event
	eventNext int
	eventSize int
	nextSeq   uint64
	pruned    uint64

	calls          map[string]Call
	completedOrder []string
	callCapacity   int
	insertions     map[string]insertionRecord
	insertionOrder []string
	changed        chan struct{}
}

func NewStore(eventCapacity, callCapacity int) *Store {
	if eventCapacity < 1 {
		eventCapacity = DefaultEventCapacity
	}
	if callCapacity < 1 {
		callCapacity = DefaultCallCapacity
	}
	return &Store{
		epoch:        newID("epoch"),
		events:       make([]Event, eventCapacity),
		calls:        make(map[string]Call, callCapacity),
		callCapacity: callCapacity,
		insertions:   make(map[string]insertionRecord, MaxInsertions),
		changed:      make(chan struct{}),
	}
}

// Epoch returns the immutable identity of this execution-store lifetime.
// Other Core subsystems use the same epoch to bind state that must not survive
// a Core restart as if it were still live.
func (s *Store) Epoch() string {
	if s == nil {
		return ""
	}
	return s.epoch
}

func (s *Store) Begin(ctx context.Context, input BeginInput) (context.Context, Call) {
	if s == nil {
		return ctx, Call{}
	}
	now := time.Now().UTC()
	parent := ScopeFromContext(ctx).CallID
	call := Call{
		ID:                 newID("call"),
		ParentCallID:       strings.TrimSpace(parent),
		Tool:               normalized(input.Tool, "unknown"),
		Source:             normalized(input.Source, "unknown"),
		Status:             StatusRunning,
		InsertionSupported: input.InsertionSupported,
		ContinuationID:     normalized(input.ContinuationID, ""),
		StartedAt:          now,
		UpdatedAt:          now,
	}

	s.mu.Lock()
	event := s.appendEventLocked(Event{
		Kind:               EventCallStarted,
		CallID:             call.ID,
		ParentCallID:       call.ParentCallID,
		Tool:               call.Tool,
		Source:             call.Source,
		Status:             call.Status,
		InsertionSupported: call.InsertionSupported,
	}, now)
	call.StartedSequence = event.Sequence
	s.calls[call.ID] = call
	s.mu.Unlock()

	return WithScope(ctx, Scope{CallID: call.ID}), call
}

func (s *Store) BeginChild(parentCallID string, input BeginInput) Call {
	ctx := WithScope(context.Background(), Scope{CallID: strings.TrimSpace(parentCallID)})
	_, call := s.Begin(ctx, input)
	return call
}

func (s *Store) Finish(callID string, input FinishInput) (Call, bool) {
	if s == nil {
		return Call{}, false
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return Call{}, false
	}
	status := input.Status
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled:
	default:
		status = StatusFailed
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	call, ok := s.calls[callID]
	if !ok {
		return Call{}, false
	}
	if isTerminal(call.Status) {
		return call, true
	}
	s.expireInsertionsLocked(now)
	reason := "target_call_completed_without_delivery"
	if status == StatusFailed {
		reason = "target_call_failed"
	}
	if status == StatusCancelled {
		reason = "target_call_cancelled"
	}
	s.rejectInsertionsLocked(callID, reason, now)
	call.Status = status
	call.ErrorCode = strings.TrimSpace(input.ErrorCode)
	call.ErrorCategory = strings.TrimSpace(input.ErrorCategory)
	call.UpdatedAt = now
	call.CompletedAt = timePointer(now)
	kind := EventCallCompleted
	switch status {
	case StatusFailed:
		kind = EventCallFailed
	case StatusCancelled:
		kind = EventCallCancelled
	}
	event := s.appendEventLocked(Event{
		Kind:               kind,
		CallID:             call.ID,
		ParentCallID:       call.ParentCallID,
		Tool:               call.Tool,
		Source:             call.Source,
		Status:             call.Status,
		InsertionSupported: call.InsertionSupported,
		ErrorCode:          call.ErrorCode,
		ErrorCategory:      call.ErrorCategory,
	}, now)
	call.EndedSequence = event.Sequence
	s.calls[call.ID] = call
	s.completedOrder = append(s.completedOrder, call.ID)
	s.pruneCompletedCallsLocked()
	return call, true
}

func (s *Store) SetWaiting(callID string, waiting bool) (Call, bool) {
	if s == nil {
		return Call{}, false
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return Call{}, false
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	call, ok := s.calls[callID]
	if !ok || isTerminal(call.Status) {
		return call, ok
	}
	next := StatusRunning
	kind := EventCallResumed
	if waiting {
		next = StatusWaitingForUser
		kind = EventCallWaiting
	}
	if call.Status == next {
		return call, true
	}
	call.Status = next
	call.UpdatedAt = now
	s.appendEventLocked(Event{
		Kind:               kind,
		CallID:             call.ID,
		ParentCallID:       call.ParentCallID,
		Tool:               call.Tool,
		Source:             call.Source,
		Status:             next,
		InsertionSupported: call.InsertionSupported,
	}, now)
	s.calls[call.ID] = call
	return call, true
}

func (s *Store) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{SchemaVersion: SchemaVersion, Calls: []Call{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireInsertionsLocked(time.Now().UTC())
	calls := make([]Call, 0, len(s.calls))
	active := 0
	for _, call := range s.calls {
		calls = append(calls, cloneCall(call))
		if call.Status == StatusRunning || call.Status == StatusWaitingForUser {
			active++
		}
	}
	sort.Slice(calls, func(i, j int) bool {
		a, _ := strconv.ParseUint(calls[i].StartedSequence, 10, 64)
		b, _ := strconv.ParseUint(calls[j].StartedSequence, 10, 64)
		if a == b {
			return calls[i].ID < calls[j].ID
		}
		return a < b
	})
	insertions := make([]Insertion, 0, len(s.insertions))
	for _, id := range s.insertionOrder {
		if record, ok := s.insertions[id]; ok {
			insertions = append(insertions, cloneInsertion(record.public))
		}
	}
	return Snapshot{
		SchemaVersion:  SchemaVersion,
		Epoch:          s.epoch,
		LatestSequence: strconv.FormatUint(s.nextSeq, 10),
		PrunedThrough:  strconv.FormatUint(s.pruned, 10),
		ActiveCalls:    active,
		Calls:          calls,
		Insertions:     insertions,
	}
}

func (s *Store) Page(after uint64, limit int) Page {
	if s == nil {
		return Page{SchemaVersion: SchemaVersion, After: strconv.FormatUint(after, 10), Events: []Event{}}
	}
	if limit < 1 || limit > MaxPageEvents {
		limit = MaxPageEvents
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	page := Page{
		SchemaVersion:  SchemaVersion,
		Epoch:          s.epoch,
		After:          strconv.FormatUint(after, 10),
		LatestSequence: strconv.FormatUint(s.nextSeq, 10),
		PrunedThrough:  strconv.FormatUint(s.pruned, 10),
		Gap:            after < s.pruned,
		Events:         []Event{},
	}
	for _, event := range s.eventsLocked() {
		sequence, _ := strconv.ParseUint(event.Sequence, 10, 64)
		if sequence <= after {
			continue
		}
		if len(page.Events) >= limit {
			page.HasMore = true
			break
		}
		page.Events = append(page.Events, event)
	}
	return page
}

func (s *Store) Call(id string) (Call, bool) {
	if s == nil {
		return Call{}, false
	}
	s.mu.RLock()
	call, ok := s.calls[strings.TrimSpace(id)]
	s.mu.RUnlock()
	return cloneCall(call), ok
}

func (s *Store) appendEventLocked(event Event, occurredAt time.Time) Event {
	s.nextSeq++
	event.SchemaVersion = SchemaVersion
	event.Epoch = s.epoch
	event.Sequence = strconv.FormatUint(s.nextSeq, 10)
	event.OccurredAt = occurredAt.UTC()

	if s.eventSize == len(s.events) {
		overwritten := s.events[s.eventNext]
		if sequence, err := strconv.ParseUint(overwritten.Sequence, 10, 64); err == nil && sequence > s.pruned {
			s.pruned = sequence
		}
	} else {
		s.eventSize++
	}
	s.events[s.eventNext] = event
	s.eventNext = (s.eventNext + 1) % len(s.events)
	close(s.changed)
	s.changed = make(chan struct{})
	return event
}

func (s *Store) eventsLocked() []Event {
	if s.eventSize == 0 {
		return []Event{}
	}
	out := make([]Event, 0, s.eventSize)
	start := (s.eventNext - s.eventSize + len(s.events)) % len(s.events)
	for offset := 0; offset < s.eventSize; offset++ {
		index := (start + offset) % len(s.events)
		out = append(out, s.events[index])
	}
	return out
}

func (s *Store) pruneCompletedCallsLocked() {
	for len(s.completedOrder) > s.callCapacity {
		id := s.completedOrder[0]
		s.completedOrder = s.completedOrder[1:]
		if call, ok := s.calls[id]; ok && isTerminal(call.Status) {
			delete(s.calls, id)
		}
	}
}

func isTerminal(status Status) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

func normalized(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 160 {
		return value[:160]
	}
	return value
}

func newID(prefix string) string {
	var data [12]byte
	if _, err := rand.Read(data[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(data[:])
	}
	return prefix + "_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
}

func timePointer(value time.Time) *time.Time {
	v := value
	return &v
}

func cloneCall(call Call) Call {
	if call.CompletedAt != nil {
		value := *call.CompletedAt
		call.CompletedAt = &value
	}
	return call
}

var ErrInvalidTransition = errors.New("invalid execution transition")

func (s *Store) Wait(ctx context.Context, after uint64) error {
	if s == nil {
		return errors.New("execution store is unavailable")
	}
	for {
		s.mu.Lock()
		now := time.Now().UTC()
		s.expireInsertionsLocked(now)
		latest := s.nextSeq
		changed := s.changed
		nextExpiry, hasExpiry := s.nextInsertionExpiryLocked()
		s.mu.Unlock()
		if latest > after {
			return nil
		}

		var timer *time.Timer
		var expiry <-chan time.Time
		if hasExpiry {
			delay := time.Until(nextExpiry)
			if delay < 0 {
				delay = 0
			}
			timer = time.NewTimer(delay)
			expiry = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return ctx.Err()
		case <-changed:
			if timer != nil {
				timer.Stop()
			}
		case <-expiry:
		}
	}
}

func (s *Store) nextInsertionExpiryLocked() (time.Time, bool) {
	var next time.Time
	for _, id := range s.insertionOrder {
		record, ok := s.insertions[id]
		if !ok || record.public.Status != InsertionAccepted {
			continue
		}
		if next.IsZero() || record.public.ExpiresAt.Before(next) {
			next = record.public.ExpiresAt
		}
	}
	return next, !next.IsZero()
}
