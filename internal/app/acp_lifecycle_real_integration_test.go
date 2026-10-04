//go:build acp_integration

package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
	processcontrol "github.com/uvwt/agentdock/internal/process"
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

func createRealIdleManagedSession(t *testing.T, rt *Runtime, profileID, workspace string) acpruntime.SessionRecord {
	t.Helper()
	idleMS := int(acpruntime.MinIdleCloseAfter / time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{
		Action: "new", ProfileID: profileID, CWD: workspace,
		LifecyclePolicy: string(acpruntime.LifecycleIdleManaged), IdleCloseAfterMS: &idleMS,
	})
	if err != nil {
		t.Fatalf("%s idle-managed session/new: %v", profileID, err)
	}
	session, ok := result["session"].(acpruntime.SessionRecord)
	if !ok || session.ID == "" {
		t.Fatalf("%s idle-managed result=%#v", profileID, result)
	}
	return session
}

func runRealAdapterIdleManagedSmoke(t *testing.T, profileID string) {
	t.Helper()
	rt, workspace := newRealACPRuntime(t, profileID)
	session := createRealIdleManagedSession(t, rt, profileID, workspace)
	waitRealPromptTerminal(t, rt, profileID, startRealPrompt(t, rt, profileID, session.ID))
	ready := sessionDiagnostics(t, rt, profileID, session.ID)
	if ready.Status != acpruntime.SessionReady || !ready.IdleManagedIdle || ready.IdleManagedEligible {
		t.Fatalf("%s idle-managed post-prompt diagnostics=%+v", profileID, ready)
	}

	deadline := time.Now().Add(acpruntime.MinIdleCloseAfter + 75*time.Second)
	var closed acpruntime.SessionDiagnostics
	for time.Now().Before(deadline) {
		closed = sessionDiagnostics(t, rt, profileID, session.ID)
		if closed.Status == acpruntime.SessionClosed {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if closed.Status != acpruntime.SessionClosed || closed.ClosedReason != "idle_timeout" || closed.AutoCloseAttemptedAt == nil || closed.AutoCloseError != "" {
		t.Fatalf("%s idle-managed did not close after TTL: %+v", profileID, closed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "open", ProfileID: profileID, SessionID: session.ID})
	cancel()
	if err != nil {
		t.Fatalf("%s session/open after idle close: %v", profileID, err)
	}
	resumed, ok := result["session"].(acpruntime.SessionRecord)
	if !ok || resumed.ID != session.ID || resumed.Status != acpruntime.SessionReady || resumed.ClosedAt != nil || resumed.LifecyclePolicy != acpruntime.LifecycleIdleManaged {
		t.Fatalf("%s resumed session=%#v", profileID, result["session"])
	}
	closeRealSession(t, rt, profileID, session.ID)
}

func TestRealCodexACPIdleManagedCloseAndResume(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_IDLE") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_IDLE=1 to run real idle-managed integration")
	}
	runRealAdapterIdleManagedSmoke(t, "codex")
}

func TestRealAntigravityACPIdleManagedCloseAndResume(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_IDLE") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_IDLE=1 to run real idle-managed integration")
	}
	runRealAdapterIdleManagedSmoke(t, "antigravity")
}

func TestRealAntigravityACPModelAndEffortUpdate(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_CONFIG") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_CONFIG=1 to validate Antigravity model/effort config")
	}
	rt, workspace := newRealACPRuntime(t, "antigravity")
	session := createRealSession(t, rt, "antigravity", workspace, acpruntime.LifecyclePersistent)
	for _, update := range []toolacp.SessionRequest{
		{Action: "update", ProfileID: "antigravity", SessionID: session.ID, ConfigID: "model", ConfigValue: "gemini-3.1-pro"},
		{Action: "update", ProfileID: "antigravity", SessionID: session.ID, ConfigID: "reasoning_effort", ConfigValue: "high"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		result, err := rt.acp.Session(ctx, update)
		cancel()
		if err != nil {
			t.Fatalf("Antigravity config %s update: %v", update.ConfigID, err)
		}
		change, _ := result["change"].(map[string]any)
		want := fmt.Sprint(update.ConfigValue)
		if got := fmt.Sprint(change["after"]); got != want {
			t.Fatalf("Antigravity config %s after=%q want=%q result=%#v", update.ConfigID, got, want, result)
		}
	}
	waitRealPromptTerminal(t, rt, "antigravity", startRealPrompt(t, rt, "antigravity", session.ID))
	if got := sessionDiagnostics(t, rt, "antigravity", session.ID); got.Status != acpruntime.SessionReady {
		t.Fatalf("Antigravity configured session=%+v", got)
	}
	closeRealSession(t, rt, "antigravity", session.ID)
}

func runRealAdapterCapabilityReleaseSmoke(t *testing.T, profileID string) {
	t.Helper()
	profile := realACPProfile(t, profileID)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	workspace := t.TempDir()
	cfg := config.Config{
		AgentDockHome: t.TempDir(), AgentDockDefaultDir: workspace,
		Host: "127.0.0.1", Port: port, BrowserEnabled: true, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{profile}, ACPDefaultProfile: profile.ID,
		ACPMaxPrompts: 1, ACPInteractionMS: 30_000,
	}
	if err := cfg.Normalize(); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	if h := rt.ACPBrowserMCPHandler(); h != nil {
		mux.Handle("/internal/acp-browser/mcp", h)
	}
	if h := rt.ACPComputerMCPHandler(); h != nil {
		mux.Handle("/internal/acp-computer/mcp", h)
	}
	server := &http.Server{Handler: mux}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = server.Shutdown(ctx)
		cancel()
		<-done
		_ = rt.Close()
	})

	session := createRealSession(t, rt, profileID, workspace, acpruntime.LifecycleEphemeral)
	owners := rt.acpBrowser.Diagnostics().Owners
	if len(owners) != 1 || owners[0].ACPSessionID != session.ID || owners[0].ProfileID != profileID {
		t.Fatalf("%s browser capability owners after new=%+v", profileID, owners)
	}
	if active := rt.computer.Broker().Diagnostics().ActiveSessions; len(active) != 0 {
		t.Fatalf("%s unexpected active computer sessions before prompt=%+v", profileID, active)
	}

	waitRealPromptTerminal(t, rt, profileID, startRealPrompt(t, rt, profileID, session.ID))
	closed := waitRealSessionStatus(t, rt, profileID, session.ID, acpruntime.SessionClosed)
	if closed.ClosedReason != "ephemeral_prompt_terminal" {
		t.Fatalf("%s closed=%+v", profileID, closed)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(rt.acpBrowser.Diagnostics().Owners) != 0 {
		time.Sleep(25 * time.Millisecond)
	}
	if owners := rt.acpBrowser.Diagnostics().Owners; len(owners) != 0 {
		t.Fatalf("%s browser capability owners leaked after auto-close=%+v", profileID, owners)
	}
	if active := rt.computer.Broker().Diagnostics().ActiveSessions; len(active) != 0 {
		t.Fatalf("%s computer sessions leaked after auto-close=%+v", profileID, active)
	}
}

func TestRealCodexACPEphemeralReleasesHostCapabilities(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_CAPABILITY") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_CAPABILITY=1 to validate host capability release")
	}
	runRealAdapterCapabilityReleaseSmoke(t, "codex")
}

func TestRealAntigravityACPEphemeralReleasesHostCapabilities(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_CAPABILITY") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_CAPABILITY=1 to validate host capability release")
	}
	runRealAdapterCapabilityReleaseSmoke(t, "antigravity")
}

func realACPStatus(t *testing.T, rt *Runtime, profileID string) (acpruntime.DiagnosticsSnapshot, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "status", ProfileID: profileID})
	if err != nil {
		t.Fatalf("%s status: %v", profileID, err)
	}
	diagnostics, ok := result["diagnostics"].(acpruntime.DiagnosticsSnapshot)
	if !ok {
		t.Fatalf("%s status diagnostics type=%T result=%#v", profileID, result["diagnostics"], result)
	}
	pid, _ := result["adapter_pid"].(int)
	return diagnostics, pid
}

func waitRealAdapterDescendantsAtMost(t *testing.T, pid, maximum int) processcontrol.Snapshot {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var snapshot processcontrol.Snapshot
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		snapshot = processcontrol.Observe(ctx, pid)
		cancel()
		if snapshot.Error == "" && snapshot.DescendantCount != nil && *snapshot.DescendantCount <= maximum {
			return snapshot
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("adapter pid=%d descendants did not return to <=%d: %+v", pid, maximum, snapshot)
	return snapshot
}

func waitRealAdapterStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var snapshot processcontrol.Snapshot
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		snapshot = processcontrol.Observe(ctx, pid)
		cancel()
		if snapshot.State == "not_found" || (snapshot.Alive != nil && !*snapshot.Alive) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("adapter pid=%d still alive after runtime close: %+v", pid, snapshot)
}

func runRealAdapterTwentyPromptStress(t *testing.T, profileID string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("real adapter process baseline stress is currently macOS-only")
	}
	profile := realACPProfile(t, profileID)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	workspace := t.TempDir()
	cfg := config.Config{
		AgentDockHome: t.TempDir(), AgentDockDefaultDir: workspace,
		Host: "127.0.0.1", Port: port, BrowserEnabled: true, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{profile}, ACPDefaultProfile: profile.ID,
		ACPMaxPrompts: 4, ACPInteractionMS: 30_000,
	}
	if err := cfg.Normalize(); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	if h := rt.ACPBrowserMCPHandler(); h != nil {
		mux.Handle("/internal/acp-browser/mcp", h)
	}
	if h := rt.ACPComputerMCPHandler(); h != nil {
		mux.Handle("/internal/acp-computer/mcp", h)
	}
	server := &http.Server{Handler: mux}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	closed := false
	shutdown := func() {
		if closed {
			return
		}
		closed = true
		_ = rt.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = server.Shutdown(ctx)
		cancel()
		<-serverDone
	}
	t.Cleanup(shutdown)

	const batchSize = 4
	const batches = 5
	adapterPID := 0
	baselineDescendants := -1
	for batch := 0; batch < batches; batch++ {
		sessions := make([]acpruntime.SessionRecord, 0, batchSize)
		runs := make([]string, 0, batchSize)
		for i := 0; i < batchSize; i++ {
			session := createRealSession(t, rt, profileID, workspace, acpruntime.LifecycleEphemeral)
			sessions = append(sessions, session)
		}
		owners := rt.acpBrowser.Diagnostics().Owners
		if len(owners) != batchSize {
			t.Fatalf("%s batch %d browser owners before prompts=%d want=%d: %+v", profileID, batch+1, len(owners), batchSize, owners)
		}
		for _, session := range sessions {
			runs = append(runs, startRealPrompt(t, rt, profileID, session.ID))
		}
		for i, runID := range runs {
			waitRealPromptTerminal(t, rt, profileID, runID)
			closedSession := waitRealSessionStatus(t, rt, profileID, sessions[i].ID, acpruntime.SessionClosed)
			if closedSession.ClosedReason != "ephemeral_prompt_terminal" || closedSession.AutoCloseAttemptedAt == nil || closedSession.AutoCloseError != "" {
				t.Fatalf("%s batch %d session %d close diagnostics=%+v", profileID, batch+1, i+1, closedSession)
			}
		}

		diagnostics, pid := realACPStatus(t, rt, profileID)
		expectedClosed := (batch + 1) * batchSize
		if diagnostics.Counts.Managed != expectedClosed || diagnostics.Counts.Closed != expectedClosed || diagnostics.Counts.Loaded != 0 || diagnostics.Counts.Running != 0 || diagnostics.Counts.Ready != 0 || diagnostics.Counts.AutoCloseFailures != 0 {
			t.Fatalf("%s batch %d diagnostics=%+v want managed/closed=%d loaded/running/ready/failures=0", profileID, batch+1, diagnostics.Counts, expectedClosed)
		}
		if owners := rt.acpBrowser.Diagnostics().Owners; len(owners) != 0 {
			t.Fatalf("%s batch %d browser owners leaked: %+v", profileID, batch+1, owners)
		}
		if active := rt.computer.Broker().Diagnostics().ActiveSessions; len(active) != 0 {
			t.Fatalf("%s batch %d computer sessions leaked: %+v", profileID, batch+1, active)
		}
		if pid <= 0 {
			t.Fatalf("%s batch %d missing adapter pid", profileID, batch+1)
		}
		if adapterPID == 0 {
			adapterPID = pid
		} else if pid != adapterPID {
			t.Fatalf("%s adapter pid changed during stress: got=%d want=%d", profileID, pid, adapterPID)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		snapshot := processcontrol.Observe(ctx, adapterPID)
		cancel()
		if snapshot.Error != "" || snapshot.DescendantCount == nil {
			t.Fatalf("%s batch %d adapter observation unavailable: %+v", profileID, batch+1, snapshot)
		}
		if baselineDescendants < 0 {
			baselineDescendants = *snapshot.DescendantCount
		} else {
			waitRealAdapterDescendantsAtMost(t, adapterPID, baselineDescendants)
		}
	}

	shutdown()
	waitRealAdapterStopped(t, adapterPID)
}

func TestRealCodexACPTwentyPromptEphemeralStress(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_STRESS") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_STRESS=1 to run 20-prompt Codex lifecycle stress")
	}
	runRealAdapterTwentyPromptStress(t, "codex")
}

func TestRealAntigravityACPTwentyPromptEphemeralStress(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_STRESS") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_STRESS=1 to run 20-prompt Antigravity lifecycle stress")
	}
	runRealAdapterTwentyPromptStress(t, "antigravity")
}

func realACPDescendantCommands(root int) ([]string, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil, err
	}
	parents := map[int]int{}
	commands := map[int]string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		parents[pid] = ppid
		commands[pid] = strings.Join(fields[2:], " ")
	}
	isDescendant := func(pid int) bool {
		seen := map[int]bool{}
		for pid > 0 && !seen[pid] {
			seen[pid] = true
			parent, ok := parents[pid]
			if !ok {
				return false
			}
			if parent == root {
				return true
			}
			pid = parent
		}
		return false
	}
	result := []string{}
	for pid, command := range commands {
		if isDescendant(pid) {
			result = append(result, command)
		}
	}
	return result, nil
}

func TestRealACPSessionsDoNotSpawnPerSessionMemoryStdio(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_REAL_ACP_MEMORY") != "1" {
		t.Skip("set AGENTDOCK_RUN_REAL_ACP_MEMORY=1 to verify ACP Memory transport")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("process-tree Memory transport check is currently macOS-only")
	}
	for _, profileID := range []string{"codex", "antigravity"} {
		profileID := profileID
		t.Run(profileID, func(t *testing.T) {
			rt, workspace := newRealACPRuntime(t, profileID)
			session := createRealSession(t, rt, profileID, workspace, acpruntime.LifecyclePersistent)
			waitRealPromptTerminal(t, rt, profileID, startRealPrompt(t, rt, profileID, session.ID))
			if got := sessionDiagnostics(t, rt, profileID, session.ID); got.Status != acpruntime.SessionReady || !got.Loaded {
				t.Fatalf("%s loaded persistent session diagnostics=%+v", profileID, got)
			}
			commands, err := realACPDescendantCommands(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range commands {
				lower := strings.ToLower(command)
				if strings.Contains(lower, "mcp-memory-service") || strings.Contains(lower, "/.venv/bin/memory server") || strings.Contains(lower, "/memory server") {
					t.Fatalf("%s spawned per-session stdio Memory child: %s", profileID, command)
				}
			}
			closeRealSession(t, rt, profileID, session.ID)
		})
	}
}
