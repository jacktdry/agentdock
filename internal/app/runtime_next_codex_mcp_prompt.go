package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
)

// Only the opt-in macOS Next Codex ACP profile gets cooperative task-end
// guidance. No MCP process/CLI configuration, API, or native tool authority
// is changed. A prompt cannot reliably run before the Codex native MCP
// transport starts, and a model report is never verified owner evidence.
const nextCodexMCPReportEnv = "AGENTDOCK_NEXT_CODEX_MCP_REPORT"

const nextCodexMCPReportInstructions = `[AgentDock Next / Codex ACP MCP task completion]
At the end of this task, briefly report the MCP server names actually used and what they were used for. Categorize each as: used; released (only with an observed result from an authorized provider lifecycle operation); still_in_use; or unknown_or_deferred. If no MCP was used, say so explicitly. Report in the user's language. Do not start, reconnect or inspect otherwise-unused MCP servers just to prepare this report. Do not include credentials, tokens, command lines, environment values, tool parameters or PIDs.
Only release a resource you definitely created and exclusively own for this task, using the provider's supported graceful close action. Never execute OS process termination commands (including kill, pkill, killall, signals) as part of this report, and never shut down Codex-managed, shared, external, or unverified MCP processes; never infer ownership from server name or PID. If no safe close is available, report unknown_or_deferred and leave the process alone. A model-produced report is advisory: AgentDock independently verifies process ownership and cleanup status.`

func nextCodexMCPReportDecorator(cfg config.Config, profile config.ACPProfile) func(string, []acpruntime.ContentBlock) ([]acpruntime.ContentBlock, error) {
	if runtime.GOOS != "darwin" || cfg.Stdio || !cfg.ACPEnabled || !profile.Enabled || profile.Kind != "codex" ||
		os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || os.Getenv(nextCodexMCPReportEnv) != "source-only" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" || cfg.AgentDockHome != filepath.Join(home, ".agentdock-next") {
		return nil
	}
	// Pure deterministic function; safe to call while the ACP Manager is
	// admitting an individual Run. Never mutate a caller-owned block.
	return func(_ string, blocks []acpruntime.ContentBlock) ([]acpruntime.ContentBlock, error) {
		if len(blocks) == 0 {
			return blocks, nil
		}
		for _, block := range blocks {
			if block["type"] != "text" {
				return blocks, nil
			}
		}
		copied := make([]acpruntime.ContentBlock, 0, len(blocks)+1)
		copied = append(copied, blocks...)
		copied = append(copied, acpruntime.TextBlock(nextCodexMCPReportInstructions))
		// A near-limit ACP prompt must not begin failing merely because of
		// optional lifecycle guidance. The Manager still validates all input.
		data, err := json.Marshal(copied)
		if err != nil || len(data) > 8<<20 {
			return blocks, nil
		}
		return copied, nil
	}
}
