//go:build acp_integration

package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
)

func requireRealACPIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP=1 to run real ACP lifecycle integration")
	}
}

func realACPProfile(t *testing.T, profileID string) config.ACPProfile {
	t.Helper()
	switch profileID {
	case "codex":
		path, err := exec.LookPath("codex-acp")
		if err != nil {
			t.Skip("codex-acp not installed")
		}
		return config.ACPProfile{ID: "codex", Kind: "codex", Command: path, Enabled: true}
	case "antigravity":
		path, err := exec.LookPath("antigravity-acp")
		if err != nil {
			path = "/Users/wei/.local/bin/antigravity-acp"
			if _, statErr := os.Stat(path); statErr != nil {
				t.Skip("antigravity-acp not installed")
			}
		}
		agy, err := exec.LookPath("agy")
		if err != nil {
			agy = "/Users/wei/.local/bin/agy"
			if _, statErr := os.Stat(agy); statErr != nil {
				t.Skip("agy not installed")
			}
		}
		t.Setenv("AGENTDOCK_REAL_AGY_BIN", agy)
		t.Setenv("AGENTDOCK_REAL_AGY_SKIP_DOWNLOAD", "1")
		return config.ACPProfile{ID: "antigravity", Kind: "custom", Command: path, Enabled: true, EnvFromEnv: map[string]string{"AGY_BIN": "AGENTDOCK_REAL_AGY_BIN", "AGY_SKIP_DOWNLOAD": "AGENTDOCK_REAL_AGY_SKIP_DOWNLOAD"}}
	default:
		t.Fatalf("unsupported real ACP profile %q", profileID)
		return config.ACPProfile{}
	}
}

func newRealACPRuntime(t *testing.T, profileID string) (*Runtime, string) {
	t.Helper()
	profile := realACPProfile(t, profileID)
	workspace := t.TempDir()
	cfg := config.Config{
		AgentDockHome: t.TempDir(), AgentDockDefaultDir: workspace,
		Host: "127.0.0.1", Port: 0, BrowserEnabled: false, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{profile}, ACPDefaultProfile: profile.ID,
		ACPMaxPrompts: 2, ACPInteractionMS: 30_000,
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt, workspace
}

func startRealPrompt(t *testing.T, rt *Runtime, profileID, sessionID string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := rt.acp.Prompt(ctx, toolacp.PromptRequest{Action: "start", ProfileID: profileID, SessionID: sessionID, Prompt: []map[string]any{{"type": "text", "text": "Reply with exactly M7_OK and do not call tools."}}})
	if err != nil {
		t.Fatalf("%s prompt/start: %v", profileID, err)
	}
	runID, _ := result["run_id"].(string)
	if runID == "" {
		t.Fatalf("%s prompt/start result=%#v", profileID, result)
	}
	return runID
}

func waitRealPromptTerminal(t *testing.T, rt *Runtime, profileID, runID string) acpruntime.RunStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	after := 0
	for time.Now().Before(deadline) {
		wait := 10000
		limit := 200
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		result, err := rt.acp.Prompt(ctx, toolacp.PromptRequest{Action: "events", ProfileID: profileID, RunID: runID, AfterSeq: &after, Limit: &limit, WaitMS: &wait})
		cancel()
		if err != nil {
			t.Fatalf("%s prompt/events: %v", profileID, err)
		}
		if next, ok := result["next_seq"].(uint64); ok {
			after = int(next)
		}
		status := acpruntime.RunStatus(fmt.Sprint(result["status"]))
		switch status {
		case acpruntime.RunCompleted:
			return status
		case acpruntime.RunCancelled, acpruntime.RunInterrupted, acpruntime.RunFailed:
			t.Fatalf("%s run %s ended %s: %#v", profileID, runID, status, result)
		}
	}
	t.Fatalf("%s run %s did not settle", profileID, runID)
	return ""
}

func sessionDiagnostics(t *testing.T, rt *Runtime, profileID, sessionID string) acpruntime.SessionDiagnostics {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "status", ProfileID: profileID})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, ok := result["diagnostics"].(acpruntime.DiagnosticsSnapshot)
	if !ok {
		t.Fatalf("%s status diagnostics type=%T result=%#v", profileID, result["diagnostics"], result)
	}
	for _, session := range diagnostics.Sessions {
		if session.SessionID == sessionID {
			return session
		}
	}
	t.Fatalf("%s session %s absent from diagnostics: %+v", profileID, sessionID, diagnostics)
	return acpruntime.SessionDiagnostics{}
}

func waitRealSessionStatus(t *testing.T, rt *Runtime, profileID, sessionID string, want acpruntime.SessionStatus) acpruntime.SessionDiagnostics {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		got := sessionDiagnostics(t, rt, profileID, sessionID)
		if got.Status == want {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	got := sessionDiagnostics(t, rt, profileID, sessionID)
	t.Fatalf("%s session %s status=%s want=%s diagnostics=%+v", profileID, sessionID, got.Status, want, got)
	return got
}

func createRealSession(t *testing.T, rt *Runtime, profileID, workspace string, policy acpruntime.SessionLifecyclePolicy) acpruntime.SessionRecord {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "new", ProfileID: profileID, CWD: workspace, LifecyclePolicy: string(policy)})
	if err != nil {
		t.Fatalf("%s session/new: %v", profileID, err)
	}
	session, ok := result["session"].(acpruntime.SessionRecord)
	if !ok || session.ID == "" {
		t.Fatalf("%s session/new result=%#v", profileID, result)
	}
	return session
}

func closeRealSession(t *testing.T, rt *Runtime, profileID, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "close", ProfileID: profileID, SessionID: sessionID}); err != nil {
		t.Fatalf("%s session/close: %v", profileID, err)
	}
}

func runRealAdapterLifecycleSmoke(t *testing.T, profileID string) {
	t.Helper()
	rt, workspace := newRealACPRuntime(t, profileID)
	ephemeral := createRealSession(t, rt, profileID, workspace, acpruntime.LifecycleEphemeral)
	waitRealPromptTerminal(t, rt, profileID, startRealPrompt(t, rt, profileID, ephemeral.ID))
	closed := waitRealSessionStatus(t, rt, profileID, ephemeral.ID, acpruntime.SessionClosed)
	if closed.ClosedReason != "ephemeral_prompt_terminal" || closed.AutoCloseAttemptedAt == nil || closed.AutoCloseError != "" {
		t.Fatalf("%s ephemeral diagnostics=%+v", profileID, closed)
	}

	persistent := createRealSession(t, rt, profileID, workspace, acpruntime.LifecyclePersistent)
	waitRealPromptTerminal(t, rt, profileID, startRealPrompt(t, rt, profileID, persistent.ID))
	time.Sleep(300 * time.Millisecond)
	ready := sessionDiagnostics(t, rt, profileID, persistent.ID)
	if ready.Status != acpruntime.SessionReady || ready.ClosedAt != nil || ready.AutoCloseAttemptedAt != nil {
		t.Fatalf("%s persistent diagnostics=%+v", profileID, ready)
	}
	closeRealSession(t, rt, profileID, persistent.ID)
}

func TestRealCodexACPLifecyclePolicies(t *testing.T) {
	requireRealACPIntegration(t)
	runRealAdapterLifecycleSmoke(t, "codex")
}
func TestRealAntigravityACPLifecyclePolicies(t *testing.T) {
	requireRealACPIntegration(t)
	runRealAdapterLifecycleSmoke(t, "antigravity")
}
