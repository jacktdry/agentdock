package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	toolplugin "github.com/uvwt/agentdock/internal/tool/plugin"
)

const (
	desktopPluginOperationTTL  = 15 * time.Minute
	maxDesktopPluginOperations = 256
)

type desktopPluginOperationError struct {
	Code      string
	Category  string
	Retryable bool
	Details   map[string]any
}

type desktopPluginOperationRecord struct {
	RequestID   string
	Action      string
	Target      string
	Fingerprint string
	Outcome     string
	StartedAt   time.Time
	CompletedAt time.Time
	Result      Result
	Error       *desktopPluginOperationError
}

func validDesktopPluginRequestID(value string) bool {
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

func (r *Runtime) desktopPluginRequestFingerprint(request toolplugin.DesktopManageRequest) (string, error) {
	// request_id is an idempotency/journal key, not mutation semantics.
	// Excluding it lets the caller's exact semantic retry consume a one-time
	// approval even when the retry uses a fresh operation id.
	request.RequestID = ""
	canonical, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, r.mcpDesktopFingerprintKey[:])
	_, _ = mac.Write([]byte("agentdock-desktop-plugin-request-v1\x00"))
	_, _ = mac.Write(canonical)
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil)), nil
}

func (r *Runtime) runDesktopPluginMutation(ctx context.Context, request toolplugin.DesktopManageRequest, descriptor map[string]any, dispatch func() (Result, error)) (Result, error) {
	requestID := strings.TrimSpace(request.RequestID)
	if !validDesktopPluginRequestID(requestID) {
		return nil, toolError("PLUGIN_OPERATION_ID_INVALID", "Invalid Desktop Plugin operation id", "validation")
	}
	fingerprint, err := r.desktopPluginRequestFingerprint(request)
	if err != nil {
		return nil, toolError("PLUGIN_OPERATION_UNAVAILABLE", "Desktop Plugin operation could not start", "operation")
	}

	if replay, replayErr, handled := r.beginDesktopPluginOperation(requestID, request.Action, request.Name, fingerprint); handled {
		return replay, replayErr
	}

	result, runErr := r.runRuntimeManagementMutationBound(ctx, "runtime_plugin", request.Action, descriptor, fingerprint, dispatch)
	if desktopPluginAdmissionRetryRequired(runErr) {
		r.abandonDesktopPluginOperation(requestID)
		return result, runErr
	}
	outcome := desktopPluginOperationOutcome(result, runErr)
	if result != nil && result["outcome_unknown"] == true {
		outcome = "outcome_unknown"
	}
	recordedResult := cloneDesktopPluginJournalResult(result)
	if recordedResult == nil {
		recordedResult = Result{}
	}
	recordedResult["request_id"] = requestID
	recordedResult["outcome"] = outcome
	r.finishDesktopPluginOperation(requestID, outcome, recordedResult, runErr)

	if result != nil {
		result["request_id"] = requestID
		result["outcome"] = outcome
	}
	return result, runErr
}

func desktopPluginAdmissionRetryRequired(err error) bool {
	var typed *ToolError
	return errors.As(err, &typed) && typed.Code == "APPROVAL_REQUIRED"
}

func (r *Runtime) abandonDesktopPluginOperation(requestID string) {
	r.pluginDesktopOpsMu.Lock()
	delete(r.pluginDesktopOps, requestID)
	r.pluginDesktopOpsMu.Unlock()
}

func (r *Runtime) beginDesktopPluginOperation(requestID, action, target, fingerprint string) (Result, error, bool) {
	now := time.Now().UTC()
	r.pluginDesktopOpsMu.Lock()
	defer r.pluginDesktopOpsMu.Unlock()
	r.cleanupDesktopPluginOperationsLocked(now)

	if existing, ok := r.pluginDesktopOps[requestID]; ok {
		if existing.Fingerprint != fingerprint || existing.Action != action || existing.Target != strings.TrimSpace(target) {
			return nil, toolError("PLUGIN_OPERATION_ID_CONFLICT", "Desktop Plugin operation id was already used for a different request", "conflict"), true
		}
		if existing.Outcome == "pending" {
			return nil, retryableDesktopPluginError("PLUGIN_OPERATION_IN_PROGRESS", "Desktop Plugin operation is still running", "conflict"), true
		}
		return cloneDesktopPluginResult(existing.Result), restoreDesktopPluginOperationError(existing.Error), true
	}

	if len(r.pluginDesktopOps) >= maxDesktopPluginOperations {
		return nil, retryableDesktopPluginError("PLUGIN_OPERATION_LIMIT", "Desktop Plugin operation journal is full", "capacity"), true
	}
	if r.pluginDesktopOps == nil {
		r.pluginDesktopOps = make(map[string]desktopPluginOperationRecord)
	}
	r.pluginDesktopOps[requestID] = desktopPluginOperationRecord{
		RequestID: requestID, Action: action, Target: strings.TrimSpace(target), Fingerprint: fingerprint,
		Outcome: "pending", StartedAt: now,
	}
	return nil, nil, false
}

func (r *Runtime) finishDesktopPluginOperation(requestID, outcome string, result Result, operationErr error) {
	r.pluginDesktopOpsMu.Lock()
	defer r.pluginDesktopOpsMu.Unlock()
	record, ok := r.pluginDesktopOps[requestID]
	if !ok {
		return
	}
	record.Outcome = outcome
	record.CompletedAt = time.Now().UTC()
	record.Result = cloneDesktopPluginResult(result)
	record.Error = snapshotDesktopPluginOperationError(operationErr)
	r.pluginDesktopOps[requestID] = record
}

func (r *Runtime) desktopPluginOperationStatus(requestID string) (Result, error) {
	requestID = strings.TrimSpace(requestID)
	if !validDesktopPluginRequestID(requestID) {
		return nil, toolError("PLUGIN_OPERATION_ID_INVALID", "Invalid Desktop Plugin operation id", "validation")
	}
	now := time.Now().UTC()
	r.pluginDesktopOpsMu.Lock()
	defer r.pluginDesktopOpsMu.Unlock()
	r.cleanupDesktopPluginOperationsLocked(now)
	record, ok := r.pluginDesktopOps[requestID]
	if !ok {
		return Result{
			"action": "desktop_operation_status", "request_id": requestID, "found": false,
			"pending": false, "outcome": "outcome_unknown",
		}, nil
	}
	result := cloneDesktopPluginResult(record.Result)
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
			"retryable": record.Error.Retryable, "details": cloneDesktopPluginDetails(record.Error.Details),
		}
	}
	return result, nil
}

func (r *Runtime) cleanupDesktopPluginOperationsLocked(now time.Time) {
	if len(r.pluginDesktopOps) == 0 {
		return
	}
	cutoff := now.Add(-desktopPluginOperationTTL)
	for id, record := range r.pluginDesktopOps {
		if record.Outcome != "pending" && !record.CompletedAt.IsZero() && record.CompletedAt.Before(cutoff) {
			delete(r.pluginDesktopOps, id)
		}
	}

	if len(r.pluginDesktopOps) < maxDesktopPluginOperations {
		return
	}
	type completedRecord struct {
		id string
		at time.Time
	}
	completed := make([]completedRecord, 0, len(r.pluginDesktopOps))
	for id, record := range r.pluginDesktopOps {
		if record.Outcome != "pending" {
			completed = append(completed, completedRecord{id: id, at: record.CompletedAt})
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].at.Before(completed[j].at) })
	for _, item := range completed {
		if len(r.pluginDesktopOps) < maxDesktopPluginOperations {
			break
		}
		delete(r.pluginDesktopOps, item.id)
	}
}

func desktopPluginOperationOutcome(result Result, err error) string {
	if err != nil {
		var typed *ToolError
		if errors.As(err, &typed) {
			switch typed.Category {
			case "timeout", "unavailable":
				return "outcome_unknown"
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

func snapshotDesktopPluginOperationError(err error) *desktopPluginOperationError {
	if err == nil {
		return nil
	}
	var typed *ToolError
	if !errors.As(err, &typed) {
		return &desktopPluginOperationError{Code: "PLUGIN_REQUEST_FAILED", Category: "operation"}
	}
	code := strings.TrimSpace(typed.Code)
	if code == "" {
		code = "PLUGIN_REQUEST_FAILED"
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
	return &desktopPluginOperationError{Code: code, Category: category, Retryable: typed.Retryable, Details: details}
}

func restoreDesktopPluginOperationError(snapshot *desktopPluginOperationError) error {
	if snapshot == nil {
		return nil
	}
	err := toolErrorDetails(snapshot.Code, "Desktop Plugin operation could not complete", snapshot.Category, cloneDesktopPluginDetails(snapshot.Details))
	err.Retryable = snapshot.Retryable
	return err
}

func retryableDesktopPluginError(code, message, category string) *ToolError {
	err := toolError(code, message, category)
	err.Retryable = true
	return err
}

func cloneDesktopPluginResult(input Result) Result {
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

func cloneDesktopPluginJournalResult(input Result) Result {
	output := cloneDesktopPluginResult(input)
	if output == nil {
		return nil
	}
	return output
}

func cloneDesktopPluginDetails(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
