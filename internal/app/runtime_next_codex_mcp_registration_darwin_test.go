//go:build darwin

package app

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/config"
)

func TestNextCodexRegistrarSourceOnlyGuardAndNoImplicitACPStart(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPRegistrationEnv, "source-only")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AgentDockHome: filepath.Join(root, ".agentdock-next"), AgentDockDefaultDir: work, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{{ID: "codex", Kind: "codex", Enabled: true, Command: os.Args[0], Args: []string{"-test.run=^TestNextM8ACPHelperProcess$"}}}, ACPDefaultProfile: "codex"}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close() }()
	key := hex.EncodeToString(make([]byte, 32))
	profile := cfg.ACPProfiles[0]
	provider, err := rt.nextCodexMCPRegistrarScopeProvider(profile, key)
	if err != nil || provider == nil {
		t.Fatalf("opt-in private provider unavailable: %v", err)
	}
	if scope, err := provider(context.Background()); err == nil || scope.AppServer.PID != 0 {
		t.Fatalf("unloaded ACP process yielded native ownership: %+v %v", scope, err)
	}
	for _, tc := range []struct {
		name   string
		mutate func()
	}{
		{"stable", func() { t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable") }},
		{"disabled", func() { t.Setenv(nextCodexMCPRegistrationEnv, "") }},
		{"bad-key", func() { key = "not-a-key" }},
		{"agy", func() { profile.Kind = "custom" }},
		{"other-profile", func() { profile.ID = "alternate" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
			t.Setenv(nextCodexMCPRegistrationEnv, "source-only")
			key = hex.EncodeToString(make([]byte, 32))
			profile = cfg.ACPProfiles[0]
			tc.mutate()
			if _, err := rt.nextCodexMCPRegistrarScopeProvider(profile, key); err == nil {
				t.Fatalf("unqualified Core registrar provider enabled")
			}
		})
	}
}
