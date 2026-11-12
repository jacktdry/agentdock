//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
)

func TestNextCodexMCPReportPromptPolicyOptInAndIsolation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPReportEnv, "source-only")
	cfg := config.Config{AgentDockHome: filepath.Join(root, ".agentdock-next"), ACPEnabled: true}
	profile := config.ACPProfile{ID: "codex", Kind: "codex", Enabled: true}
	decorator := nextCodexMCPReportDecorator(cfg, profile)
	if decorator == nil {
		t.Fatal("Next Codex prompt decorator did not qualify")
	}
	src := []acpruntime.ContentBlock{acpruntime.TextBlock("do the task")}
	got, err := decorator("session1", src)
	if err != nil || len(got) != 2 || got[0]["text"] != "do the task" || !strings.Contains(got[1]["text"].(string), "unknown_or_deferred") || len(src) != 1 {
		t.Fatalf("next codex policy = %#v, %v", got, err)
	}
	if !strings.Contains(got[1]["text"].(string), "Never execute OS process termination commands") {
		t.Fatal("report policy omitted safe cleanup constraints")
	}
	if alt, err := decorator("session1", []acpruntime.ContentBlock{{"type": "image", "data": "x", "mimeType": "image/png"}}); err != nil || len(alt) != 1 {
		t.Fatal("multimodal prompts must not be decorated")
	}
	// At ACP's 8 MiB ceiling, optional instructions must not cause a new failure.
	if alt, err := decorator("session1", []acpruntime.ContentBlock{acpruntime.TextBlock(strings.Repeat("x", (8<<20)-150))}); err != nil || len(alt) != 1 {
		t.Fatal("near-limit payload enlarged by optional reporting")
	}
	for _, tc := range []struct {
		name           string
		cfg            config.Config
		profile        config.ACPProfile
		variant, optin string
	}{
		{"stable", cfg, profile, "stable", "source-only"},
		{"no-opt-in", cfg, profile, "next", ""},
		{"wrong-home", config.Config{AgentDockHome: filepath.Join(root, ".agentdock"), ACPEnabled: true}, profile, "next", "source-only"},
		{"agy", cfg, config.ACPProfile{ID: "agy", Kind: "custom", Enabled: true}, "next", "source-only"},
		{"disabled-profile", cfg, config.ACPProfile{ID: "codex", Kind: "codex", Enabled: false}, "next", "source-only"},
		{"stdio", config.Config{AgentDockHome: filepath.Join(root, ".agentdock-next"), Stdio: true, ACPEnabled: true}, profile, "next", "source-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", tc.variant)
			t.Setenv(nextCodexMCPReportEnv, tc.optin)
			if nextCodexMCPReportDecorator(tc.cfg, tc.profile) != nil {
				t.Fatal("unqualified profile or runtime injected MCP policy")
			}
		})
	}
}

func TestNextCodexMCPReportReachesActualACPPromptOnPrivateAdapter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPReportEnv, "source-only")
	t.Setenv("NEXT_M8_ACP_HELPER_HOST", "1")
	trace := filepath.Join(root, "test-prompt.json")
	t.Setenv("NEXT_M8_TEST_PROMPT_TRACE_HOST", trace)
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AgentDockHome: filepath.Join(root, ".agentdock-next"), AgentDockDefaultDir: work, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{{ID: "codex", Kind: "codex", Enabled: true, Command: os.Args[0], Args: []string{"-test.run=^TestNextM8ACPHelperProcess$"}, EnvFromEnv: map[string]string{"NEXT_M8_ACP_HELPER": "NEXT_M8_ACP_HELPER_HOST", "NEXT_M8_TEST_PROMPT_TRACE": "NEXT_M8_TEST_PROMPT_TRACE_HOST"}}}, ACPDefaultProfile: "codex",
	}
	nextCodexCLIPrerequisite(t, &cfg.ACPProfiles[0], cfg.AgentDockHome)
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close() }()
	sessionResult, err := rt.acp.Session(context.Background(), toolacp.SessionRequest{Action: "new", ProfileID: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	session, ok := sessionResult["session"].(acpruntime.SessionRecord)
	if !ok {
		t.Fatalf("missing private managed ACP Session: %#v", sessionResult)
	}
	started, err := rt.acp.Prompt(context.Background(), toolacp.PromptRequest{Action: "start", ProfileID: "codex", SessionID: session.ID, Prompt: []map[string]any{{"type": "text", "text": "run my task"}}})
	if err != nil {
		t.Fatal(err)
	}
	if started["run_id"] == "" {
		t.Fatal("no real private ACP run")
	}
	var data []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ = os.ReadFile(trace)
		if len(data) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(data) == 0 {
		t.Fatal("fake ACP Adapter did not receive session/prompt")
	}
	var delivered struct {
		SessionID string `json:"sessionId"`
		Prompt    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal(data, &delivered); err != nil {
		t.Fatal(err)
	}
	if delivered.SessionID == "" || len(delivered.Prompt) != 2 || delivered.Prompt[0].Text != "run my task" || !strings.Contains(delivered.Prompt[1].Text, "unknown_or_deferred") {
		t.Fatalf("wrong ACP protocol prompt blocks: %+v", delivered)
	}
}
