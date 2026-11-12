package app

import (
	"strings"

	"github.com/uvwt/agentdock/internal/execution"
)

func (r *Runtime) recordExecutionResultFacts(callID, tool string, args map[string]any, result Result) {
	if r == nil || r.execution == nil || result == nil {
		return
	}
	switch tool {
	case "exec_command":
		status, _ := result["status"].(string)
		sessionID, _ := result["session_id"].(string)
		timedOut, _ := result["timed_out"].(bool)
		fact := execution.OutputFact{
			ContinuationID:   sessionID,
			Status:           status,
			TimedOut:         timedOut,
			StdoutTotalBytes: resultInt(result, "stdout_total_bytes"),
			StderrTotalBytes: resultInt(result, "stderr_total_bytes"),
			StdoutTruncated:  resultBool(result, "stdout_truncated"),
			StderrTruncated:  resultBool(result, "stderr_truncated"),
		}
		if value, ok := result["exit_code"]; ok {
			exitCode := numericInt(value)
			fact.ExitCode = &exitCode
		}
		if value, ok := result["command_ok"].(bool); ok {
			commandOK := value
			fact.CommandOK = &commandOK
		}
		r.execution.RecordOutput(callID, fact)

	case "file_edit":
		if resultBool(result, "dry_run") {
			return
		}
		filesChanged := resultInt(result, "files_changed")
		changed, changedKnown := result["changed"].(bool)
		if (!changedKnown || !changed) && filesChanged < 1 {
			return
		}
		action, _ := result["action"].(string)
		if strings.TrimSpace(action) == "" {
			action, _ = args["action"].(string)
		}
		paths := make([]string, 0, 4)
		for _, key := range []string{"path", "new_path"} {
			if value, _ := result[key].(string); strings.TrimSpace(value) != "" {
				paths = append(paths, value)
			}
		}
		switch affected := result["affected_files"].(type) {
		case []map[string]any:
			for _, item := range affected {
				paths = appendAffectedPaths(paths, item)
			}
		case []any:
			for _, raw := range affected {
				if item, ok := raw.(map[string]any); ok {
					paths = appendAffectedPaths(paths, item)
				}
			}
		}
		_, insertionsKnown := result["insertions"]
		_, deletionsKnown := result["deletions"]
		r.execution.RecordFileChange(callID, execution.FileChangeFact{
			Action:       action,
			Paths:        paths,
			FilesChanged: filesChanged,
			Insertions:   resultInt(result, "insertions"),
			Deletions:    resultInt(result, "deletions"),
			StatsKnown:   insertionsKnown && deletionsKnown,
		})
	}
}

func appendAffectedPaths(paths []string, item map[string]any) []string {
	for _, key := range []string{"path", "move_to"} {
		if value, _ := item[key].(string); strings.TrimSpace(value) != "" {
			paths = append(paths, value)
		}
	}
	return paths
}

func resultInt(result Result, key string) int {
	return numericInt(result[key])
}

func numericInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func resultBool(result Result, key string) bool {
	value, _ := result[key].(bool)
	return value
}
