//go:build darwin

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextCodexReadinessNeverStartsNative(t *testing.T) {
	rt, _, _, _ := nextCodexAuthorizationFixture(t)
	if got := rt.NextCodexMCPReadiness()["state"]; got != "operator_setup_required" {
		t.Fatalf("initial private readiness = %v", got)
	}
	if rt.nextCodexStartupLock != nil || rt.nextCodexRegistry != nil {
		t.Fatal("readiness started native services")
	}
	marker := filepath.Join(rt.cfg.AgentDockHome, nextCodexStartupLockName)
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("readiness created generation journal: %v", err)
	}
	if err := os.WriteFile(marker, []byte("unresolved fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := rt.NextCodexMCPReadiness()["state"]; got != "recovery_required" {
		t.Fatalf("uncleared prior generation classified incorrectly: %v", got)
	}
	after, err := os.ReadFile(marker)
	if err != nil || string(after) != "unresolved fixture" {
		t.Fatalf("readiness mutated journal: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(rt.cfg.AgentDockHome, "outside"), marker); err != nil {
		t.Fatal(err)
	}
	if got := rt.NextCodexMCPReadiness()["state"]; got != "state_untrusted" {
		t.Fatalf("symlink not rejected: %v", got)
	}
	t.Setenv(nextCodexMCPRegistrationEnv, "")
	if got := rt.NextCodexMCPReadiness()["state"]; got != "disabled" {
		t.Fatalf("off mode unexpectedly observed source-only state: %v", got)
	}
	if rt.acp.NextCodexAdapterProcessID() != 0 {
		t.Fatal("status spawned Codex adapter")
	}
}
