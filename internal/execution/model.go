package execution

import (
	"context"
	"time"
)

const SchemaVersion = 1

type Status string

const (
	StatusRunning        Status = "running"
	StatusWaitingForUser Status = "waiting_for_user"
	StatusCompleted      Status = "completed"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
)

type EventKind string

const (
	EventCallStarted   EventKind = "call.started"
	EventCallWaiting   EventKind = "call.waiting"
	EventCallResumed   EventKind = "call.resumed"
	EventCallCompleted EventKind = "call.completed"
	EventCallFailed    EventKind = "call.failed"
	EventCallCancelled EventKind = "call.cancelled"
	EventOutputSummary EventKind = "output.summary"
	EventFileChanged   EventKind = "file.changed"
)

type Call struct {
	ID                 string     `json:"call_id"`
	ParentCallID       string     `json:"parent_call_id,omitempty"`
	Tool               string     `json:"tool"`
	Source             string     `json:"source"`
	Status             Status     `json:"status"`
	InsertionSupported bool       `json:"insertion_supported"`
	StartedSequence    string     `json:"started_sequence"`
	EndedSequence      string     `json:"ended_sequence,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	ErrorCode          string     `json:"error_code,omitempty"`
	ErrorCategory      string     `json:"error_category,omitempty"`
	ContinuationID     string     `json:"continuation_id,omitempty"`
}

type Event struct {
	SchemaVersion      int             `json:"schema_version"`
	Epoch              string          `json:"epoch"`
	Sequence           string          `json:"sequence"`
	OccurredAt         time.Time       `json:"occurred_at"`
	Kind               EventKind       `json:"kind"`
	CallID             string          `json:"call_id"`
	ParentCallID       string          `json:"parent_call_id,omitempty"`
	Tool               string          `json:"tool,omitempty"`
	Source             string          `json:"source,omitempty"`
	Status             Status          `json:"status,omitempty"`
	InsertionSupported bool            `json:"insertion_supported,omitempty"`
	ErrorCode          string          `json:"error_code,omitempty"`
	ErrorCategory      string          `json:"error_category,omitempty"`
	InsertionID        string          `json:"insertion_id,omitempty"`
	Output             *OutputFact     `json:"output,omitempty"`
	FileChange         *FileChangeFact `json:"file_change,omitempty"`
}

type Snapshot struct {
	SchemaVersion  int         `json:"schema_version"`
	Epoch          string      `json:"epoch"`
	LatestSequence string      `json:"latest_sequence"`
	PrunedThrough  string      `json:"pruned_through"`
	ActiveCalls    int         `json:"active_calls"`
	Calls          []Call      `json:"calls"`
	Insertions     []Insertion `json:"insertions"`
}

type Page struct {
	SchemaVersion  int     `json:"schema_version"`
	Epoch          string  `json:"epoch"`
	After          string  `json:"after"`
	LatestSequence string  `json:"latest_sequence"`
	PrunedThrough  string  `json:"pruned_through"`
	Gap            bool    `json:"gap"`
	HasMore        bool    `json:"has_more"`
	Events         []Event `json:"events"`
}

type BeginInput struct {
	Tool               string
	Source             string
	ContinuationID     string
	InsertionSupported bool
}

type FinishInput struct {
	Status        Status
	ErrorCode     string
	ErrorCategory string
}

type scopeKey struct{}

type Scope struct {
	CallID string
}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

func ScopeFromContext(ctx context.Context) Scope {
	if ctx == nil {
		return Scope{}
	}
	scope, _ := ctx.Value(scopeKey{}).(Scope)
	return scope
}
