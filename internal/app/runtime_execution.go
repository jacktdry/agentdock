package app

import (
	"context"
	"errors"
	"strings"

	"github.com/uvwt/agentdock/internal/execution"
	toolcommand "github.com/uvwt/agentdock/internal/tool/command"
)

func (r *Runtime) finishExecutionCall(ctx context.Context, callID string, result Result, callErr *error) {
	if r == nil || r.execution == nil || callID == "" {
		return
	}
	status := execution.StatusCompleted
	errorCode := ""
	errorCategory := ""
	if ctx != nil && ctx.Err() != nil {
		status = execution.StatusCancelled
		errorCode = "CANCELED"
		errorCategory = "runtime"
	} else if callErr != nil && *callErr != nil {
		if errors.Is(*callErr, context.Canceled) {
			status = execution.StatusCancelled
			errorCode = "CANCELED"
			errorCategory = "runtime"
		} else {
			status = execution.StatusFailed
			errorCode, errorCategory = observableError(*callErr)
		}
	} else if failed, code, category := executionResultFailure(result); failed {
		status = execution.StatusFailed
		errorCode = code
		errorCategory = category
	}

	r.execution.Finish(callID, execution.FinishInput{
		Status:        status,
		ErrorCode:     errorCode,
		ErrorCategory: errorCategory,
	})
}

func (r *Runtime) observeCommandSession(ctx context.Context, lifecycle toolcommand.SessionLifecycle) {
	if r == nil || r.execution == nil || lifecycle.SessionID == "" || lifecycle.Done == nil || lifecycle.Metadata == nil {
		return
	}
	parentCallID := execution.ScopeFromContext(ctx).CallID
	if parentCallID == "" {
		return
	}
	child := r.execution.BeginChild(parentCallID, execution.BeginInput{
		Tool:           "command_session",
		Source:         "command",
		ContinuationID: lifecycle.SessionID,
	})
	r.execution.RecordOutput(child.ID, execution.OutputFact{
		ContinuationID: lifecycle.SessionID,
		Status:         "running",
	})

	go func() {
		<-lifecycle.Done
		snapshot := lifecycle.Metadata()
		outputStatus := "exited"
		if snapshot.TimedOut {
			outputStatus = "timeout"
		}
		exitCode := snapshot.ExitCode
		commandOK := snapshot.CommandOK
		r.execution.RecordOutput(child.ID, execution.OutputFact{
			ContinuationID:   lifecycle.SessionID,
			Status:           outputStatus,
			ExitCode:         &exitCode,
			CommandOK:        &commandOK,
			TimedOut:         snapshot.TimedOut,
			StdoutTotalBytes: snapshot.StdoutTotalBytes,
			StderrTotalBytes: snapshot.StderrTotalBytes,
			StdoutTruncated:  snapshot.StdoutDroppedBytes > 0,
			StderrTruncated:  snapshot.StderrDroppedBytes > 0,
		})
		status := execution.StatusCompleted
		code := ""
		category := ""
		if snapshot.TimedOut {
			status = execution.StatusFailed
			code = "COMMAND_TIMEOUT"
			category = "process"
		} else if !snapshot.CommandOK {
			status = execution.StatusFailed
			code = "COMMAND_FAILED"
			category = "process"
		}
		r.execution.Finish(child.ID, execution.FinishInput{
			Status:        status,
			ErrorCode:     code,
			ErrorCategory: category,
		})
	}()
}

func executionResultFailure(result Result) (bool, string, string) {
	if result == nil {
		return false, "", ""
	}
	if value, ok := result["command_ok"].(bool); ok && !value {
		code := "COMMAND_FAILED"
		if status, _ := result["status"].(string); strings.EqualFold(status, "timeout") {
			code = "COMMAND_TIMEOUT"
		}
		return true, code, "process"
	}
	if value, ok := result["browser_ok"].(bool); ok && !value {
		code, _ := result["code"].(string)
		if strings.TrimSpace(code) == "" {
			code = "BROWSER_OPERATION_FAILED"
		}
		return true, strings.TrimSpace(code), "external"
	}
	for _, key := range []string{"isError", "is_error"} {
		if value, ok := result[key].(bool); ok && value {
			return true, "TOOL_RESULT_ERROR", "external"
		}
	}
	if value, ok := result["ok"].(bool); ok && !value {
		code, _ := result["code"].(string)
		if strings.TrimSpace(code) == "" {
			code = "RESULT_REPORTED_FAILURE"
		}
		return true, strings.TrimSpace(code), "runtime"
	}
	if nested, ok := result["result"].(map[string]any); ok {
		return executionResultFailure(Result(nested))
	}
	return false, "", ""
}
