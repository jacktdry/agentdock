//go:build darwin

package app

import (
	"context"
	"fmt"
	"github.com/uvwt/agentdock/internal/permission"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestB6ManualRecoveryOnlyAfterVerifiedKernelRebootAndM8Allow(t *testing.T) {
	rt, _, _, ctx := nextCodexAuthorizationFixture(t)
	root := rt.cfg.AgentDockHome
	lock, err := acquireNextCodexStartupLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.markBootstrapAttempt(); err != nil {
		t.Fatal(err)
	}
	if err := lock.close(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, nextCodexStartupLockName)
	current, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	// Approval is mandatory; a caller-provided marker alone never enables clear.
	if err := rt.reconcileNextCodexAfterReboot(context.Background()); err == nil {
		t.Fatal("untrusted caller cleared marker")
	}
	if err := rt.reconcileNextCodexAfterReboot(ctx); err == nil {
		t.Fatal("unapproved caller cleared marker")
	}
	unchanged, err := os.ReadFile(filename)
	if err != nil || string(current) != string(unchanged) {
		t.Fatal("rejected recovery touched crash record")
	}
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
	if err := rt.reconcileNextCodexAfterReboot(ctx); err == nil {
		t.Fatal("same-boot native generation was cleared")
	}
	// v1 legacy or corrupted journal has no boot-proof; cannot be cleared.
	if err := os.WriteFile(filename, []byte("agentdock-next-codex-native-bootstrap-unresolved-v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rt.reconcileNextCodexAfterReboot(ctx); err == nil {
		t.Fatal("legacy unknown boot generation was cleared")
	}
	// Simulate a previous-boot journal with a deterministic OS boot timestamp;
	// tests MUST NOT reboot a computer or touch a real existing native process.
	boot, err := unix.SysctlTimeval("kern.boottime")
	if err != nil || boot == nil {
		t.Fatalf("unavailable kernel boot proof: %v", err)
	}
	record := fmt.Sprintf("agentdock-next-codex-native-bootstrap-unresolved-v1;boot=%d.%06d\n", boot.Sec-1, boot.Usec)
	if err := os.WriteFile(filename, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rt.reconcileNextCodexAfterReboot(ctx); err != nil {
		t.Fatalf("M8 Allow and previous boot proof not sufficient: %v", err)
	}
	after, err := os.ReadFile(filename)
	if err != nil || len(after) != 0 {
		t.Fatalf("reconciled marker not cleared safely: %q %v", after, err)
	}
	next, err := acquireNextCodexStartupLock(root)
	if err != nil {
		t.Fatalf("safe native generation remained blocked: %v", err)
	}
	if err := next.close(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(current)) == "" {
		t.Fatal("initial unresolved marker unexpectedly absent")
	}
}
