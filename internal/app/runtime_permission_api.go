package app

import (
	"errors"
	"sort"
	"strings"

	"github.com/uvwt/agentdock/internal/permission"
)

func (r *Runtime) RuntimePermissions() (Result, error) {
	state, err := r.permissions.Snapshot()
	if err != nil {
		return nil, toolErrorCause("PERMISSION_STATE_ERROR", "AgentDock permission state could not be read", "permission", nil, err)
	}
	return Result{
		"ok":             true,
		"source":         runtimeAPISource,
		"schema_version": permission.SchemaVersion,
		"state_revision": state.Revision,
		"runtime_epoch":  state.RuntimeEpoch,
		"policy":         state.Policy,
	}, nil
}

func (r *Runtime) RuntimeApprovals(approvalID, status string, limit int) (Result, error) {
	approvalID = strings.TrimSpace(approvalID)
	status = strings.ToLower(strings.TrimSpace(status))
	if limit < 0 || limit > permission.MaxHistory {
		return nil, toolError("INVALID_APPROVAL_LIMIT", "approval limit must be between 0 and 512", "validation")
	}
	if status != "" && !runtimeApprovalStatus(status) {
		return nil, toolError("INVALID_APPROVAL_STATUS", "unsupported permission approval status", "validation")
	}
	if approvalID != "" {
		record, err := r.permissions.Approval(approvalID)
		if err != nil {
			if errors.Is(err, permission.ErrNotFound) {
				return nil, toolError("APPROVAL_NOT_FOUND", "permission approval was not found", "not_found")
			}
			return nil, toolErrorCause("PERMISSION_STATE_ERROR", "AgentDock permission approval could not be read", "permission", nil, err)
		}
		return Result{
			"ok":             true,
			"source":         runtimeAPISource,
			"schema_version": permission.SchemaVersion,
			"approval":       record,
		}, nil
	}

	state, err := r.permissions.Snapshot()
	if err != nil {
		return nil, toolErrorCause("PERMISSION_STATE_ERROR", "AgentDock permission history could not be read", "permission", nil, err)
	}
	history := state.History
	sort.SliceStable(history, func(i, j int) bool {
		if history[i].CreatedAt.Equal(history[j].CreatedAt) {
			return history[i].ID > history[j].ID
		}
		return history[i].CreatedAt.After(history[j].CreatedAt)
	})
	filtered := make([]permission.ApprovalRecord, 0, len(history))
	for _, record := range history {
		if status != "" && record.Status != status {
			continue
		}
		filtered = append(filtered, record)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	return Result{
		"ok":             true,
		"source":         runtimeAPISource,
		"schema_version": permission.SchemaVersion,
		"approvals":      filtered,
		"state_revision": state.Revision,
		"runtime_epoch":  state.RuntimeEpoch,
		"count":          len(filtered),
	}, nil
}

func runtimeApprovalStatus(status string) bool {
	switch status {
	case permission.Pending, permission.ApprovedOnce, permission.Consumed, permission.ApprovedWorkspace,
		permission.Rejected, permission.Expired, permission.Invalidated:
		return true
	default:
		return false
	}
}
