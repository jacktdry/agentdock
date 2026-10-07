package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	toolmcp "github.com/uvwt/agentdock/internal/tool/mcp"
)

const (
	desktopMCPOperationTTL  = 15 * time.Minute
	maxDesktopMCPOperations = 256
)

type desktopMCPOperationError struct {
	Code      string
	Category  string
	Retryable bool
	Details   map[string]any
}

type desktopMCPOperationRecord struct {
	RequestID   string
	Action      string
	Target      string
	Fingerprint string
	Outcome     string
	StartedAt   time.Time
	CompletedAt time.Time
	Result      Result
	Error       *desktopMCPOperationError
}

func newDesktopMCPRequestID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func validDesktopMCPRequestID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func (r *Runtime) desktopMCPRequestFingerprint(request toolmcp.DesktopManageRequest) (string, error) {
	argsState := "omitted"
	if request.ArgsProvided() {
		if request.Args == nil {
			argsState = "null"
		} else {
			argsState = "value"
		}
	}
	canonical, err := json.Marshal(struct {
		Request   toolmcp.DesktopManageRequest `json:"request"`
		ArgsState string                       `json:"args_state"`
	}{
		Request: request, ArgsState: argsState,
	})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, r.mcpDesktopFingerprintKey[:])
	_, _ = mac.Write([]byte("agentdock-desktop-mcp-request-v1\x00"))
	_, _ = mac.Write(canonical)
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil)), nil
}

func (r *Runtime) runDesktopMCPMutation(ctx context.Context, request toolmcp.DesktopManageRequest, descriptor map[string]any, dispatch func() (Result, error)) (Result, error) {
	requestID := strings.TrimSpace(request.RequestID)
	if requestID == "" {
		var err error
		requestID, err = newDesktopMCPRequestID()
		if err != nil {
			return nil, toolError("MCP_OPERATION_UNAVAILABLE", "Desktop MCP operation could not start", "capacity")
		}
	}
	if !validDesktopMCPRequestID(requestID) {
		return nil, toolError("MCP_OPERATION_ID_INVALID", "Invalid Desktop MCP operation id", "validation")
	}
	fingerprint, err := r.desktopMCPRequestFingerprint(request)
	if err != nil {
		return nil, toolError("MCP_OPERATION_UNAVAILABLE", "Desktop MCP operation could not start", "operation")
	}

	if replay, replayErr, handled := r.beginDesktopMCPOperation(requestID, request.Action, request.Name, fingerprint); handled {
		return replay, replayErr
	}

	result, runErr := r.runRuntimeManagementMutationBound(ctx, "runtime_mcp", request.Action, descriptor, fingerprint, dispatch)
	outcome := desktopMCPOperationOutcome(result, runErr)
	recordedResult := cloneDesktopMCPJournalResult(result)
	if recordedResult == nil {
		recordedResult = Result{}
	}
	recordedResult["request_id"] = requestID
	recordedResult["outcome"] = outcome
	r.finishDesktopMCPOperation(requestID, outcome, recordedResult, runErr)

	if result != nil {
		result["request_id"] = requestID
		result["outcome"] = outcome
	}
	return result, runErr
}

func (r *Runtime) beginDesktopMCPOperation(requestID, action, target, fingerprint string) (Result, error, bool) {
	now := time.Now().UTC()
	r.mcpDesktopOpsMu.Lock()
	defer r.mcpDesktopOpsMu.Unlock()
	r.cleanupDesktopMCPOperationsLocked(now)

	if existing, ok := r.mcpDesktopOps[requestID]; ok {
		if existing.Fingerprint != fingerprint || existing.Action != action || existing.Target != strings.TrimSpace(target) {
			return nil, toolError("MCP_OPERATION_ID_CONFLICT", "Desktop MCP operation id was already used for a different request", "conflict"), true
		}
		if existing.Outcome == "pending" {
			return nil, retryableDesktopMCPError("MCP_OPERATION_IN_PROGRESS", "Desktop MCP operation is still running", "conflict"), true
		}
		return cloneDesktopMCPResult(existing.Result), restoreDesktopMCPOperationError(existing.Error), true
	}

	if len(r.mcpDesktopOps) >= maxDesktopMCPOperations {
		return nil, retryableDesktopMCPError("MCP_OPERATION_LIMIT", "Desktop MCP operation journal is full", "capacity"), true
	}
	if r.mcpDesktopOps == nil {
		r.mcpDesktopOps = make(map[string]desktopMCPOperationRecord)
	}
	r.mcpDesktopOps[requestID] = desktopMCPOperationRecord{
		RequestID: requestID, Action: action, Target: strings.TrimSpace(target), Fingerprint: fingerprint,
		Outcome: "pending", StartedAt: now,
	}
	return nil, nil, false
}

func (r *Runtime) finishDesktopMCPOperation(requestID, outcome string, result Result, operationErr error) {
	r.mcpDesktopOpsMu.Lock()
	defer r.mcpDesktopOpsMu.Unlock()
	record, ok := r.mcpDesktopOps[requestID]
	if !ok {
		return
	}
	record.Outcome = outcome
	record.CompletedAt = time.Now().UTC()
	record.Result = cloneDesktopMCPResult(result)
	record.Error = snapshotDesktopMCPOperationError(operationErr)
	r.mcpDesktopOps[requestID] = record
}

func (r *Runtime) desktopMCPOperationStatus(requestID string) (Result, error) {
	requestID = strings.TrimSpace(requestID)
	if !validDesktopMCPRequestID(requestID) {
		return nil, toolError("MCP_OPERATION_ID_INVALID", "Invalid Desktop MCP operation id", "validation")
	}
	now := time.Now().UTC()
	r.mcpDesktopOpsMu.Lock()
	defer r.mcpDesktopOpsMu.Unlock()
	r.cleanupDesktopMCPOperationsLocked(now)
	record, ok := r.mcpDesktopOps[requestID]
	if !ok {
		return Result{
			"action": "desktop_operation_status", "request_id": requestID, "found": false,
			"pending": false, "outcome": "outcome_unknown",
		}, nil
	}
	result := cloneDesktopMCPResult(record.Result)
	if result == nil {
		result = Result{}
	}
	result["action"] = "desktop_operation_status"
	result["request_id"] = requestID
	result["operation_action"] = record.Action
	result["found"] = true
	result["pending"] = record.Outcome == "pending"
	result["outcome"] = record.Outcome
	if !record.StartedAt.IsZero() {
		result["started_at"] = record.StartedAt.Format(time.RFC3339Nano)
	}
	if !record.CompletedAt.IsZero() {
		result["completed_at"] = record.CompletedAt.Format(time.RFC3339Nano)
	}
	if record.Error != nil {
		result["safe_error"] = map[string]any{
			"code": record.Error.Code, "category": record.Error.Category,
			"retryable": record.Error.Retryable, "details": cloneDesktopMCPDetails(record.Error.Details),
		}
	}
	return result, nil
}

func (r *Runtime) cleanupDesktopMCPOperationsLocked(now time.Time) {
	if len(r.mcpDesktopOps) == 0 {
		return
	}
	cutoff := now.Add(-desktopMCPOperationTTL)
	for id, record := range r.mcpDesktopOps {
		if record.Outcome != "pending" && !record.CompletedAt.IsZero() && record.CompletedAt.Before(cutoff) {
			delete(r.mcpDesktopOps, id)
		}
	}
	if len(r.mcpDesktopOps) < maxDesktopMCPOperations {
		return
	}
	type completedRecord struct {
		id string
		at time.Time
	}
	completed := make([]completedRecord, 0, len(r.mcpDesktopOps))
	for id, record := range r.mcpDesktopOps {
		if record.Outcome != "pending" {
			completed = append(completed, completedRecord{id: id, at: record.CompletedAt})
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].at.Before(completed[j].at) })
	for _, item := range completed {
		if len(r.mcpDesktopOps) < maxDesktopMCPOperations {
			break
		}
		delete(r.mcpDesktopOps, item.id)
	}
}

func desktopMCPOperationOutcome(result Result, err error) string {
	if err != nil {
		var typed *ToolError
		if errors.As(err, &typed) {
			switch typed.Category {
			case "validation", "conflict", "permission", "authentication":
				return "rejected"
			}
		}
		return "failed"
	}
	if result != nil {
		if recovery, _ := result["recovery_required"].(bool); recovery {
			return "partial"
		}
		persisted, _ := result["persisted"].(bool)
		completed, hasCompleted := result["completed"].(bool)
		if persisted && hasCompleted && !completed {
			return "partial"
		}
	}
	return "completed"
}

func snapshotDesktopMCPOperationError(err error) *desktopMCPOperationError {
	if err == nil {
		return nil
	}
	var typed *ToolError
	if !errors.As(err, &typed) {
		return &desktopMCPOperationError{Code: "MCP_REQUEST_FAILED", Category: "operation"}
	}
	code := strings.TrimSpace(typed.Code)
	if code == "" {
		code = "MCP_REQUEST_FAILED"
	}
	category := strings.TrimSpace(typed.Category)
	switch category {
	case "validation", "conflict", "permission", "authentication", "capacity", "timeout", "unavailable", "operation":
	default:
		category = "operation"
	}
	details := map[string]any{}
	if code == "APPROVAL_REQUIRED" {
		for _, key := range []string{"approval_id", "approval_version", "policy_revision", "executed", "retry"} {
			if value, ok := typed.Details[key]; ok {
				details[key] = value
			}
		}
	}
	return &desktopMCPOperationError{Code: code, Category: category, Retryable: typed.Retryable, Details: details}
}

func restoreDesktopMCPOperationError(snapshot *desktopMCPOperationError) error {
	if snapshot == nil {
		return nil
	}
	err := toolErrorDetails(snapshot.Code, "Desktop MCP operation could not complete", snapshot.Category, cloneDesktopMCPDetails(snapshot.Details))
	err.Retryable = snapshot.Retryable
	return err
}

func retryableDesktopMCPError(code, message, category string) *ToolError {
	err := toolError(code, message, category)
	err.Retryable = true
	return err
}

func cloneDesktopMCPResult(input Result) Result {
	if input == nil {
		return nil
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return Result{}
	}
	var output Result
	if json.Unmarshal(encoded, &output) != nil {
		return Result{}
	}
	return output
}

func cloneDesktopMCPJournalResult(input Result) Result {
	output := cloneDesktopMCPResult(input)
	if output == nil {
		return nil
	}
	// The authorization URL is an ephemeral browser handoff. Operation
	// reconciliation keeps the opaque flow id/status but never retains or replays
	// the provider URL or embedded OAuth state.
	delete(output, "authorization_url")
	return output
}

func cloneDesktopMCPDetails(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
