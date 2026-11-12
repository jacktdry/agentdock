//go:build darwin

package updateengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/updateidentity"
)

func nextPlanFixture(t *testing.T) (string, Transaction) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	id, _ := updateidentity.Resolve("next")
	root := id.Root(home)
	tx, _ := NewTransaction("darwin", "1.0.0", "1.1.0")
	tx.MacOS = &MacOSPlan{Variant: "next", SigningRequirement: `identifier "dev.dropabit.agentdock.next" and certificate leaf = H"fixture"`, TargetAppPath: filepath.Join(home, "Applications", id.AppName()),
		TrialAppPath:      filepath.Join(home, "Applications", "."+id.AppName()+".trial."+tx.TransactionID),
		SourceArbiterPath: filepath.Join(root, "update", "arbiters", tx.TransactionID, "agentdock-arbiter"),
		HandoffPath:       filepath.Join(root, "update-handoff.json"), ResultPath: filepath.Join(root, "update-result.json"),
		ServiceStatePath: filepath.Join(root, "update-services.json")}
	return root, tx
}

func TestNextPlanRejectsUnownedTargetsBeforeIO(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(*MacOSPlan)
	}{
		{"missing signer", func(p *MacOSPlan) { p.SigningRequirement = "" }},
		{"missing identity", func(p *MacOSPlan) { p.Variant = "" }},
		{"unknown identity", func(p *MacOSPlan) { p.Variant = "future" }},
		{"stable target", func(p *MacOSPlan) { p.TargetAppPath = "/Applications/AgentDock.app" }},
		{"stable trial", func(p *MacOSPlan) {
			p.TrialAppPath = filepath.Join(filepath.Dir(p.TargetAppPath), ".AgentDock.app.trial")
		}},
		{"escaped arbiter", func(p *MacOSPlan) { p.SourceArbiterPath = "/tmp/agentdock-arbiter" }},
		{"missing coordination", func(p *MacOSPlan) { p.HandoffPath = ""; p.ResultPath = ""; p.ServiceStatePath = "" }},
		{"stable health", func(p *MacOSPlan) { p.HealthURL = "http://127.0.0.1:8765/healthz" }},
		{"missing health", func(p *MacOSPlan) { p.CoreWasEnabled = true }},
		{"bootstrap bypass", func(p *MacOSPlan) { p.BootstrapMigration = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, tx := nextPlanFixture(t)
			test.alter(tx.MacOS)
			if _, err := ValidateMacOSIdentity(root, tx); err == nil {
				t.Fatal("accepted unsafe plan")
			}
		})
	}
	root, tx := nextPlanFixture(t)
	if _, err := ValidateMacOSIdentity(root, tx); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "")
	if _, err := ValidateMacOSIdentity(root, tx); err == nil {
		t.Fatal("missing caller identity accepted")
	}
}

func TestNextPlanRejectsSymlinkedJournal(t *testing.T) {
	root, tx := nextPlanFixture(t)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "update")); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateMacOSIdentity(root, tx); err == nil {
		t.Fatal("linked journal accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("outside directory modified")
	}
}
