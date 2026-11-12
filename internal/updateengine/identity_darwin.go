//go:build darwin

package updateengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock/internal/updateidentity"
)

// ValidateMacOSIdentity binds a persisted plan to the caller's explicit identity
// before journal writes, recovery, process inspection, or filesystem mutation.
func ValidateMacOSIdentity(root string, transaction Transaction) (updateidentity.Identity, error) {
	id, err := updateidentity.Resolve(os.Getenv("AGENTDOCK_DESKTOP_VARIANT"))
	if err != nil {
		return id, err
	}
	plan := transaction.MacOS
	if transaction.Platform != "darwin" || plan == nil {
		return id, fmt.Errorf("missing macOS plan")
	}
	planID, err := updateidentity.Resolve(plan.Variant)
	if err != nil || planID != id {
		return id, fmt.Errorf("transaction/caller identity mismatch")
	}
	if filepath.Base(plan.TargetAppPath) != id.AppName() {
		return id, fmt.Errorf("transaction destination identity mismatch")
	}
	if id.Variant != "next" {
		return id, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return id, err
	}
	if root != id.Root(home) {
		return id, fmt.Errorf("Next transaction root mismatch")
	}
	if err := id.ValidateDestination(plan.TargetAppPath, home); err != nil {
		return id, err
	}
	if err := ValidateVersion(transaction.TransactionID); err != nil {
		return id, err
	}
	if !strings.Contains(plan.SigningRequirement, `identifier "`+id.BundleID+`"`) || !strings.Contains(plan.SigningRequirement, "certificate ") || strings.Contains(plan.SigningRequirement, "cdhash") {
		return id, fmt.Errorf("Next source signing requirement missing or invalid")
	}
	if plan.BootstrapMigration {
		return id, fmt.Errorf("Next cannot use legacy bootstrap migration")
	}
	expectedTrial := filepath.Join(filepath.Dir(plan.TargetAppPath), "."+id.AppName()+".trial."+transaction.TransactionID)
	expectedArbiter := filepath.Join(root, "update", "arbiters", transaction.TransactionID, "agentdock-arbiter")
	for _, pair := range [][2]string{
		{plan.TrialAppPath, expectedTrial},
		{plan.SourceArbiterPath, expectedArbiter},
		{plan.HandoffPath, filepath.Join(root, "update-handoff.json")},
		{plan.ResultPath, filepath.Join(root, "update-result.json")},
		{plan.ServiceStatePath, filepath.Join(root, "update-services.json")},
	} {
		actual, expected := pair[0], pair[1]
		if actual != expected {
			return id, fmt.Errorf("Next transaction path mismatch: %s", actual)
		}
		if err := updateidentity.SafePath(actual); err != nil {
			return id, err
		}
	}

	for _, path := range []string{root, plan.TargetAppPath, filepath.Join(root, "update", "transaction.json"), filepath.Join(root, "update", "transaction.lock"), filepath.Join(root, "update", "results", transaction.TransactionID+".json"), filepath.Join(root, "update", "result.json")} {
		if err := updateidentity.SafePath(path); err != nil {
			return id, err
		}
	}
	if plan.HealthURL != "" && plan.HealthURL != "http://127.0.0.1:8767/healthz" {
		return id, fmt.Errorf("Next health target mismatch")
	}
	if plan.CoreWasEnabled && plan.HealthURL == "" {
		return id, fmt.Errorf("Next health target missing")
	}
	return id, nil
}
