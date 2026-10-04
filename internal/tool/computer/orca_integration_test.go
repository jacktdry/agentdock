//go:build computer_integration

package computer

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestOrcaBrokerBackgroundObservationSmoke(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("focus smoke currently asserts the macOS frontmost-app adapter")
	}
	command := resolveOrcaCommand()
	if _, err := exec.LookPath(command); err != nil {
		t.Skipf("Orca unavailable: %v", err)
	}
	provider, err := NewOrcaProvider(OrcaProviderConfig{Command: command, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := NewBroker(provider)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = broker.Close() }()
	meta, err := broker.Acquire(AcquireRequest{
		Owner:            OwnerScope{Kind: OwnerDirect, OwnerTaskID: "computer-integration"},
		Capability:       CapabilityObserve,
		ForegroundPolicy: ForegroundForbidden,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, action := range []string{"capabilities", "list_apps"} {
		result, observeErr := broker.ObserveDirect(ctx, meta.SessionID, ObservationRequest{Action: action, NoScreenshot: true, Timeout: 20 * time.Second})
		if observeErr != nil {
			t.Fatalf("%s: %v", action, observeErr)
		}
		if result.Provider != ProviderOrca || result.ForegroundRequired || result.FocusChanged {
			t.Fatalf("%s result=%+v", action, result)
		}
	}
	front, err := provider.FrontmostApp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if front == nil || front.BundleID == "" {
		t.Skip("frontmost app identity unavailable")
	}
	state, err := broker.ObserveDirect(ctx, meta.SessionID, ObservationRequest{Action: "get_app_state", App: front.BundleID, NoScreenshot: true, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if state.FocusChanged || state.ForegroundRequired {
		t.Fatalf("background get_app_state changed focus: %+v", state)
	}
	if _, ok := state.ProviderResult["snapshot"]; !ok {
		t.Fatalf("get_app_state result missing snapshot: %#v", state.ProviderResult)
	}
}
