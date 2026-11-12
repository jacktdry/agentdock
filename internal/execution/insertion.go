package execution

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxInsertionTextBytes    = 8 << 10
	MaxInsertionRequestBytes = MaxInsertionTextBytes*6 + 4096
	MaxInsertions            = 128
	InsertionLifetime        = 5 * time.Minute
	MaxInsertionPreview      = 256
)

type InsertionStatus string

const (
	InsertionAccepted  InsertionStatus = "accepted"
	InsertionDelivered InsertionStatus = "delivered"
	InsertionRejected  InsertionStatus = "rejected"
	InsertionExpired   InsertionStatus = "expired"
	InsertionCancelled InsertionStatus = "cancelled"
)

const (
	EventInsertionAccepted  EventKind = "insertion.accepted"
	EventInsertionDelivered EventKind = "insertion.delivered"
	EventInsertionRejected  EventKind = "insertion.rejected"
	EventInsertionExpired   EventKind = "insertion.expired"
	EventInsertionCancelled EventKind = "insertion.cancelled"
)

var (
	ErrInsertionTarget   = errors.New("insertion target call is unavailable")
	ErrInsertionInvalid  = errors.New("insertion text must be valid UTF-8 between 1 and 8192 bytes")
	ErrInsertionCapacity = errors.New("insertion capacity exceeded")
	ErrInsertionNotFound = errors.New("insertion not found")
)

type Insertion struct {
	ID           string          `json:"insertion_id"`
	TargetCallID string          `json:"target_call_id"`
	Status       InsertionStatus `json:"status"`
	TextPreview  string          `json:"text_preview,omitempty"`
	TextBytes    int             `json:"text_bytes"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	ExpiresAt    time.Time       `json:"expires_at"`
	DeliveredAt  *time.Time      `json:"delivered_at,omitempty"`
	Reason       string          `json:"reason,omitempty"`
}

type InsertionDelivery struct {
	ID   string `json:"insertion_id"`
	Text string `json:"text"`
}

type insertionRecord struct {
	public Insertion
	text   string
}

func (s *Store) EnqueueInsertion(callID, text string) (Insertion, error) {
	if s == nil {
		return Insertion{}, ErrInsertionTarget
	}
	callID = strings.TrimSpace(callID)
	text = strings.TrimSpace(text)
	if callID == "" || text == "" || !utf8.ValidString(text) || len(text) > MaxInsertionTextBytes {
		return Insertion{}, ErrInsertionInvalid
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireInsertionsLocked(now)

	call, ok := s.calls[callID]
	if !ok || isTerminal(call.Status) || !call.InsertionSupported {
		return Insertion{}, ErrInsertionTarget
	}
	if len(s.insertions) >= MaxInsertions {
		s.pruneInsertionsLocked()
	}
	if len(s.insertions) >= MaxInsertions {
		return Insertion{}, ErrInsertionCapacity
	}
	item := Insertion{
		ID:           newID("ins"),
		TargetCallID: callID,
		Status:       InsertionAccepted,
		TextPreview:  insertionPreview(text),
		TextBytes:    len(text),
		CreatedAt:    now,
		UpdatedAt:    now,
		ExpiresAt:    now.Add(InsertionLifetime),
	}
	s.insertions[item.ID] = insertionRecord{public: item, text: text}
	s.insertionOrder = append(s.insertionOrder, item.ID)
	s.appendEventLocked(Event{
		Kind:        EventInsertionAccepted,
		CallID:      callID,
		InsertionID: item.ID,
		Status:      call.Status,
	}, now)
	return cloneInsertion(item), nil
}

func (s *Store) DeliverInsertions(callID string) []InsertionDelivery {
	if s == nil {
		return nil
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireInsertionsLocked(now)

	call, ok := s.calls[callID]
	if !ok || isTerminal(call.Status) {
		return nil
	}
	out := make([]InsertionDelivery, 0)
	for _, id := range s.insertionOrder {
		record, ok := s.insertions[id]
		if !ok || record.public.TargetCallID != callID || record.public.Status != InsertionAccepted {
			continue
		}
		record.public.Status = InsertionDelivered
		record.public.UpdatedAt = now
		record.public.DeliveredAt = timePointer(now)
		record.public.Reason = "attached_to_tool_result"
		s.insertions[id] = record
		out = append(out, InsertionDelivery{ID: id, Text: record.text})
		s.appendEventLocked(Event{
			Kind:        EventInsertionDelivered,
			CallID:      callID,
			InsertionID: id,
			Status:      StatusRunning,
		}, now)
	}
	return out
}

func (s *Store) RejectInsertions(callID, reason string) []Insertion {
	if s == nil {
		return nil
	}
	callID = strings.TrimSpace(callID)
	reason = normalized(reason, "target_call_ended")
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireInsertionsLocked(now)
	return s.rejectInsertionsLocked(callID, reason, now)
}

func (s *Store) rejectInsertionsLocked(callID, reason string, now time.Time) []Insertion {
	out := make([]Insertion, 0)
	for _, id := range s.insertionOrder {
		record, ok := s.insertions[id]
		if !ok || record.public.TargetCallID != callID || record.public.Status != InsertionAccepted {
			continue
		}
		record.public.Status = InsertionRejected
		record.public.UpdatedAt = now
		record.public.Reason = reason
		s.insertions[id] = record
		out = append(out, cloneInsertion(record.public))
		s.appendEventLocked(Event{
			Kind:        EventInsertionRejected,
			CallID:      callID,
			InsertionID: id,
		}, now)
	}
	return out
}

func (s *Store) CancelInsertion(id string) (Insertion, error) {
	if s == nil {
		return Insertion{}, ErrInsertionNotFound
	}
	id = strings.TrimSpace(id)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireInsertionsLocked(now)
	record, ok := s.insertions[id]
	if !ok {
		return Insertion{}, ErrInsertionNotFound
	}
	if record.public.Status == InsertionAccepted {
		record.public.Status = InsertionCancelled
		record.public.UpdatedAt = now
		record.public.Reason = "cancelled_by_user"
		s.insertions[id] = record
		s.appendEventLocked(Event{
			Kind:        EventInsertionCancelled,
			CallID:      record.public.TargetCallID,
			InsertionID: id,
		}, now)
	}
	return cloneInsertion(record.public), nil
}

func (s *Store) Insertions(callID string) []Insertion {
	if s == nil {
		return []Insertion{}
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireInsertionsLocked(now)
	callID = strings.TrimSpace(callID)
	out := make([]Insertion, 0)
	for _, id := range s.insertionOrder {
		record, ok := s.insertions[id]
		if !ok || (callID != "" && record.public.TargetCallID != callID) {
			continue
		}
		out = append(out, cloneInsertion(record.public))
	}
	return out
}

func (s *Store) expireInsertionsLocked(now time.Time) {
	for _, id := range s.insertionOrder {
		record, ok := s.insertions[id]
		if !ok || record.public.Status != InsertionAccepted || now.Before(record.public.ExpiresAt) {
			continue
		}
		record.public.Status = InsertionExpired
		record.public.UpdatedAt = now
		record.public.Reason = "delivery_deadline_elapsed"
		s.insertions[id] = record
		s.appendEventLocked(Event{
			Kind:        EventInsertionExpired,
			CallID:      record.public.TargetCallID,
			InsertionID: id,
		}, now)
	}
}

func (s *Store) pruneInsertionsLocked() {
	if len(s.insertions) < MaxInsertions {
		return
	}
	kept := s.insertionOrder[:0]
	for _, id := range s.insertionOrder {
		record, ok := s.insertions[id]
		if !ok {
			continue
		}
		if len(s.insertions) >= MaxInsertions && record.public.Status != InsertionAccepted {
			delete(s.insertions, id)
			continue
		}
		kept = append(kept, id)
	}
	s.insertionOrder = kept
}

func insertionPreview(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= MaxInsertionPreview {
		return text
	}
	end := MaxInsertionPreview
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}

func cloneInsertion(item Insertion) Insertion {
	if item.DeliveredAt != nil {
		value := *item.DeliveredAt
		item.DeliveredAt = &value
	}
	return item
}
