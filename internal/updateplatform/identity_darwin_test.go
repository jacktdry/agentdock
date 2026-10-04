//go:build darwin

package updateplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/updateengine"
	"github.com/uvwt/agentdock/internal/updateidentity"
)

func TestNextPortCollisionRejectsForeignOrAmbiguousListener(t *testing.T) {
	app := filepath.Join(t.TempDir(), "AgentDock Next.app")
	for _, command := range []string{"/Applications/AgentDock.app/Contents/Helpers/agentdock service launch-core", "/tmp/other-core", app + "/Contents/Helpers/agentdock-other"} {
		if err := validateNextListenerCommand(app, command); err == nil {
			t.Fatal("foreign listener accepted")
		}
	}
	for _, output := range []string{"", "123\n456", "-1", "invalid"} {
		if _, err := nextListenerPID(output); err == nil {
			t.Fatal("ambiguous listener accepted")
		}
	}
	if err := validateNextListenerCommand(app, app+"/Contents/Helpers/agentdock service launch-core"); err != nil {
		t.Fatal(err)
	}
}

// The OS boundary is simulated; disk journals, metadata reads and atomic swaps
// are real. Signature validation has separate signed-bundle fixture coverage.
func nextDriverFixture(t *testing.T) (*DarwinDriver, updateengine.Transaction, *int) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	id, _ := updateidentity.Resolve("next")
	root := id.Root(home)
	driver, err := NewDarwinDriver(root)
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := updateengine.NewTransaction("darwin", "1.0.0", "1.1.0")
	target := filepath.Join(home, "Applications", id.AppName())
	tx.MacOS = &updateengine.MacOSPlan{Variant: "next", SigningRequirement: `identifier "dev.dropabit.agentdock.next" and certificate leaf = H"fixture"`, TargetAppPath: target, TrialAppPath: filepath.Join(filepath.Dir(target), "."+id.AppName()+".trial."+tx.TransactionID), SourceArbiterPath: filepath.Join(root, "update", "arbiters", tx.TransactionID, "agentdock-arbiter"), HandoffPath: filepath.Join(root, "update-handoff.json"), ResultPath: filepath.Join(root, "update-result.json"), ServiceStatePath: filepath.Join(root, "update-services.json"), AppWasRunning: true}
	writeSlot := func(path, version string) {
		if err := os.MkdirAll(filepath.Join(path, "Contents"), 0755); err != nil {
			t.Fatal(err)
		}
		data := fmt.Sprintf(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>%s</string><key>AgentDockVariant</key><string>next</string><key>CFBundleShortVersionString</key><string>%s</string></dict></plist>`, id.BundleID, version)
		if err := os.WriteFile(filepath.Join(path, "Contents", "Info.plist"), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeSlot(target, tx.SourceVersion)
	writeSlot(tx.MacOS.TrialAppPath, tx.TargetVersion)
	oldSigner := verifyNextSigner
	verifyNextSigner = func(context.Context, *updateengine.MacOSPlan, ...string) error { return nil }
	oldValidate, oldSigning, oldStop, oldOpen := validateApp, validateSigningContinuity, stopApp, openApp
	mutations := 0
	validateApp = func(ctx context.Context, path, version string) error {
		bundle, err := plistValue(ctx, filepath.Join(path, "Contents", "Info.plist"), "CFBundleIdentifier")
		if err != nil || bundle != id.BundleID || macOSAppVersion(ctx, path) != updateengine.NormalizeVersion(version) {
			return fmt.Errorf("untrusted fixture slot")
		}
		return nil
	}
	validateSigningContinuity = func(context.Context, string, string) error { return nil }
	stopApp = func(context.Context, string, time.Duration) error { mutations++; return nil }
	openApp = func(ctx context.Context, path string) error {
		mutations++
		data, _ := json.Marshal(macOSHandoff{SchemaVersion: 1, Variant: "next", TransactionID: tx.TransactionID, TargetVersion: macOSAppVersion(ctx, path)})
		return os.WriteFile(tx.MacOS.HandoffPath, data, 0600)
	}
	t.Cleanup(func() {
		validateApp, validateSigningContinuity, stopApp, openApp = oldValidate, oldSigning, oldStop, oldOpen
		verifyNextSigner = oldSigner
	})
	return driver, tx, &mutations
}

func TestNextArbiterCommitAndInterruptedRecovery(t *testing.T) {
	for _, mode := range []string{"commit", "interrupted", "missing active", "wrong handoff"} {
		t.Run(mode, func(t *testing.T) {
			driver, tx, _ := nextDriverFixture(t)
			if mode == "interrupted" || mode == "missing active" {
				if err := swapPathsAtomic(tx.MacOS.TargetAppPath, tx.MacOS.TrialAppPath); err != nil {
					t.Fatal(err)
				}
				tx.State = updateengine.StateTrial
				if mode == "missing active" {
					if err := os.RemoveAll(tx.MacOS.TargetAppPath); err != nil {
						t.Fatal(err)
					}
				}
			}
			if mode == "wrong handoff" {
				original := openApp
				openApp = func(ctx context.Context, path string) error {
					if err := original(ctx, path); err != nil {
						return err
					}
					if macOSAppVersion(ctx, path) == tx.TargetVersion {
						data, _ := json.Marshal(macOSHandoff{SchemaVersion: 1, Variant: "stable", TransactionID: tx.TransactionID, TargetVersion: tx.TargetVersion})
						return os.WriteFile(tx.MacOS.HandoffPath, data, 0600)
					}
					return nil
				}
			}
			if err := driver.store.WriteTransaction(tx); err != nil {
				t.Fatal(err)
			}
			result, err := (updateengine.Arbiter{Store: driver.store, Driver: driver}).Run(context.Background(), tx.TransactionID)
			wantState, wantVersion := updateengine.StateCommitted, tx.TargetVersion
			if mode != "commit" {
				wantState, wantVersion = updateengine.StateRolledBack, tx.SourceVersion
				if err == nil {
					t.Fatal("expected original trial failure")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if result.State != wantState || macOSAppVersion(context.Background(), tx.MacOS.TargetAppPath) != wantVersion {
				t.Fatalf("result=%+v", result)
			}
			if _, err := os.Stat(tx.MacOS.TrialAppPath); mode != "missing active" && err != nil {
				t.Fatal("rollback slot removed before terminal cleanup", err)
			}
		})
	}
}

func TestNextRollbackRejectsUnprovenOwnershipBeforeStop(t *testing.T) {
	for _, mode := range []string{"stable destination", "stable slot", "linked slot", "missing identity"} {
		t.Run(mode, func(t *testing.T) {
			driver, tx, mutations := nextDriverFixture(t)
			switch mode {
			case "stable destination":
				tx.MacOS.TargetAppPath = "/Applications/AgentDock.app"
			case "missing identity":
				tx.MacOS.Variant = ""
			case "stable slot":
				if err := os.WriteFile(filepath.Join(tx.MacOS.TrialAppPath, "Contents", "Info.plist"), []byte("stable sentinel"), 0644); err != nil {
					t.Fatal(err)
				}
			case "linked slot":
				if err := os.RemoveAll(tx.MacOS.TrialAppPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), tx.MacOS.TrialAppPath); err != nil {
					t.Fatal(err)
				}
			}
			if err := driver.Rollback(context.Background(), tx); err == nil {
				t.Fatal("unsafe rollback accepted")
			}
			if *mutations != 0 {
				t.Fatalf("process mutation before ownership validation: %d", *mutations)
			}
		})
	}
}
