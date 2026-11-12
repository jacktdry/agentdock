package execution

import (
	"strings"
	"time"
)

const (
	MaxFileChangePaths = 16
	MaxFactPathBytes   = 512
)

type OutputFact struct {
	ContinuationID   string `json:"continuation_id,omitempty"`
	Status           string `json:"status,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	CommandOK        *bool  `json:"command_ok,omitempty"`
	TimedOut         bool   `json:"timed_out,omitempty"`
	StdoutTotalBytes int    `json:"stdout_total_bytes,omitempty"`
	StderrTotalBytes int    `json:"stderr_total_bytes,omitempty"`
	StdoutTruncated  bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated  bool   `json:"stderr_truncated,omitempty"`
}

type FileChangeFact struct {
	Action       string   `json:"action,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	FilesChanged int      `json:"files_changed,omitempty"`
	Insertions   int      `json:"insertions,omitempty"`
	Deletions    int      `json:"deletions,omitempty"`
	StatsKnown   bool     `json:"stats_known,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
}

func (s *Store) RecordOutput(callID string, fact OutputFact) (Event, bool) {
	if s == nil || strings.TrimSpace(callID) == "" {
		return Event{}, false
	}
	fact.ContinuationID = normalized(fact.ContinuationID, "")
	fact.Status = normalized(fact.Status, "")
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.calls[strings.TrimSpace(callID)]; !ok {
		return Event{}, false
	}
	event := s.appendEventLocked(Event{
		Kind:   EventOutputSummary,
		CallID: strings.TrimSpace(callID),
		Output: cloneOutputFact(&fact),
	}, now)
	return event, true
}

func (s *Store) RecordFileChange(callID string, fact FileChangeFact) (Event, bool) {
	if s == nil || strings.TrimSpace(callID) == "" {
		return Event{}, false
	}
	fact.Action = normalized(fact.Action, "")
	paths := make([]string, 0, min(len(fact.Paths), MaxFileChangePaths))
	for _, value := range fact.Paths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > MaxFactPathBytes {
			value = value[:MaxFactPathBytes]
			fact.Truncated = true
		}
		if len(paths) >= MaxFileChangePaths {
			fact.Truncated = true
			break
		}
		paths = append(paths, value)
	}
	fact.Paths = paths
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.calls[strings.TrimSpace(callID)]; !ok {
		return Event{}, false
	}
	event := s.appendEventLocked(Event{
		Kind:       EventFileChanged,
		CallID:     strings.TrimSpace(callID),
		FileChange: cloneFileChangeFact(&fact),
	}, now)
	return event, true
}

func cloneOutputFact(value *OutputFact) *OutputFact {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.ExitCode != nil {
		v := *value.ExitCode
		cloned.ExitCode = &v
	}
	if value.CommandOK != nil {
		v := *value.CommandOK
		cloned.CommandOK = &v
	}
	return &cloned
}

func cloneFileChangeFact(value *FileChangeFact) *FileChangeFact {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Paths = append([]string(nil), value.Paths...)
	return &cloned
}
