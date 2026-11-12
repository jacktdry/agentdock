package app

import (
	"testing"

	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
	toolcomputer "github.com/uvwt/agentdock/internal/tool/computer"
)

func TestCorrelateACPBrokerResources(t *testing.T) {
	browser := toolbrowser.BrokerDiagnostics{
		Owners: []toolbrowser.BrokerOwnerDiagnostic{
			{ACPSessionID: "acps-b", ProfileID: "antigravity", LeaseIDs: []string{"lease-b"}},
			{ACPSessionID: "acps-a", ProfileID: "codex", LeaseIDs: []string{"lease-a2", "lease-a1"}},
		},
		Leases: []toolbrowser.BrokerLeaseDiagnostic{
			{LeaseID: "lease-a1", ACPSessionID: "acps-a", ProfileID: "codex", CleanupState: toolbrowser.CleanupPending},
			{LeaseID: "lease-a2", ACPSessionID: "acps-a", ProfileID: "codex", CleanupState: toolbrowser.CleanupFailed},
			{LeaseID: "lease-b", ACPSessionID: "acps-b", ProfileID: "antigravity", CleanupState: toolbrowser.CleanupComplete},
		},
	}
	computer := toolcomputer.BrokerDiagnostics{ActiveSessions: []toolcomputer.SessionMetadata{
		{SessionID: "computer-a-observe", Owner: toolcomputer.OwnerScope{Kind: toolcomputer.OwnerACP, OwnerProfileID: "codex", OwnerACPSessionID: "acps-a"}, Capability: toolcomputer.CapabilityObserve},
		{SessionID: "computer-a-act", Owner: toolcomputer.OwnerScope{Kind: toolcomputer.OwnerACP, OwnerProfileID: "codex", OwnerACPSessionID: "acps-a"}, Capability: toolcomputer.CapabilityAct},
		{SessionID: "computer-direct", Owner: toolcomputer.OwnerScope{Kind: toolcomputer.OwnerDirect}, Capability: toolcomputer.CapabilityAct},
	}}

	got := correlateACPBrokerResources(&browser, &computer)
	if !got.BrowserAvailable || !got.ComputerAvailable || len(got.Sessions) != 2 {
		t.Fatalf("correlation=%+v", got)
	}
	if got.Sessions[0].ProfileID != "antigravity" || got.Sessions[0].SessionID != "acps-b" || got.Sessions[0].Browser.ActiveLeases != 0 || got.Sessions[0].Browser.CleanupIssues != 0 {
		t.Fatalf("antigravity=%+v", got.Sessions[0])
	}
	codex := got.Sessions[1]
	if codex.ProfileID != "codex" || codex.SessionID != "acps-a" {
		t.Fatalf("codex identity=%+v", codex)
	}
	if len(codex.Browser.LeaseIDs) != 2 || codex.Browser.LeaseIDs[0] != "lease-a1" || codex.Browser.ActiveLeases != 1 || codex.Browser.CleanupIssues != 1 {
		t.Fatalf("codex browser=%+v", codex.Browser)
	}
	if len(codex.Computer.SessionIDs) != 2 || codex.Computer.Observe != 1 || codex.Computer.Act != 1 {
		t.Fatalf("codex computer=%+v", codex.Computer)
	}
}

func TestCorrelateACPBrokerResourcesAvailabilityWithoutOwners(t *testing.T) {
	browser := toolbrowser.BrokerDiagnostics{}
	got := correlateACPBrokerResources(&browser, nil)
	if !got.BrowserAvailable || got.ComputerAvailable || len(got.Sessions) != 0 {
		t.Fatalf("correlation=%+v", got)
	}
}
