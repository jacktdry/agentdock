//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/uvwt/agentdock/internal/updateengine"
	"github.com/uvwt/agentdock/internal/updateidentity"
	"github.com/uvwt/agentdock/internal/updateplatform"
)

func newPlatformDriver(root string) (updateengine.Driver, error) {
	return updateplatform.NewDarwinDriver(root)
}

func expectedSourceArbiter(transaction updateengine.Transaction) string {
	if transaction.MacOS == nil {
		return ""
	}
	return transaction.MacOS.SourceArbiterPath
}

func validatePlatformRoot(root string) error {
	id, err := updateidentity.Resolve(os.Getenv("AGENTDOCK_DESKTOP_VARIANT"))
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// Even an unmarked arbiter must never read a Next transaction as stable.
	if id.Variant != "next" && filepath.Base(root) == "AgentDock Next" {
		return fmt.Errorf("Next arbiter identity is missing")
	}
	if id.Variant == "next" {
		if root != id.Root(home) {
			return fmt.Errorf("Next arbiter root mismatch")
		}
		for _, path := range []string{root, filepath.Join(root, "update", "transaction.json"), filepath.Join(root, "update", "transaction.lock")} {
			if err := updateidentity.SafePath(path); err != nil {
				return err
			}
		}
	}
	return nil
}
func validatePlatformTransaction(root string, transaction updateengine.Transaction) error {
	_, err := updateengine.ValidateMacOSIdentity(root, transaction)
	return err
}
