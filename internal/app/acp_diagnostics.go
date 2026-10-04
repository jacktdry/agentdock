package app

import (
	"sort"

	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
	toolcomputer "github.com/uvwt/agentdock/internal/tool/computer"
)

type acpBrowserResourceSummary struct {
	LeaseIDs      []string `json:"lease_ids"`
	ActiveLeases  int      `json:"active_leases"`
	CleanupIssues int      `json:"cleanup_issues"`
}

type acpComputerResourceSummary struct {
	SessionIDs []string `json:"computer_session_ids"`
	Observe    int      `json:"observe_sessions"`
	Act        int      `json:"act_sessions"`
}

type acpBrokerResourceCorrelation struct {
	ProfileID string                     `json:"profile_id"`
	SessionID string                     `json:"session_id"`
	Browser   acpBrowserResourceSummary  `json:"browser"`
	Computer  acpComputerResourceSummary `json:"computer"`
}

type acpBrokerCorrelation struct {
	BrowserAvailable  bool                           `json:"browser_available"`
	ComputerAvailable bool                           `json:"computer_available"`
	Sessions          []acpBrokerResourceCorrelation `json:"sessions"`
}

type acpBrokerKey struct {
	profileID string
	sessionID string
}

func correlateACPBrokerResources(browser *toolbrowser.BrokerDiagnostics, computer *toolcomputer.BrokerDiagnostics) acpBrokerCorrelation {
	result := acpBrokerCorrelation{BrowserAvailable: browser != nil, ComputerAvailable: computer != nil}
	byKey := map[acpBrokerKey]*acpBrokerResourceCorrelation{}
	ensure := func(profileID, sessionID string) *acpBrokerResourceCorrelation {
		if profileID == "" || sessionID == "" {
			return nil
		}
		key := acpBrokerKey{profileID: profileID, sessionID: sessionID}
		if existing := byKey[key]; existing != nil {
			return existing
		}
		entry := &acpBrokerResourceCorrelation{ProfileID: profileID, SessionID: sessionID}
		byKey[key] = entry
		return entry
	}

	if browser != nil {
		ownerByLease := make(map[string]toolbrowser.BrokerOwnerDiagnostic)
		for _, owner := range browser.Owners {
			entry := ensure(owner.ProfileID, owner.ACPSessionID)
			if entry == nil {
				continue
			}
			for _, leaseID := range owner.LeaseIDs {
				ownerByLease[leaseID] = owner
				entry.Browser.LeaseIDs = append(entry.Browser.LeaseIDs, leaseID)
			}
		}
		for _, lease := range browser.Leases {
			profileID, sessionID := lease.ProfileID, lease.ACPSessionID
			if owner, ok := ownerByLease[lease.LeaseID]; ok {
				if profileID == "" {
					profileID = owner.ProfileID
				}
				if sessionID == "" {
					sessionID = owner.ACPSessionID
				}
			}
			entry := ensure(profileID, sessionID)
			if entry == nil {
				continue
			}
			if !acpStringSliceContains(entry.Browser.LeaseIDs, lease.LeaseID) {
				entry.Browser.LeaseIDs = append(entry.Browser.LeaseIDs, lease.LeaseID)
			}
			switch lease.CleanupState {
			case toolbrowser.CleanupPending, toolbrowser.CleanupReleasing:
				entry.Browser.ActiveLeases++
			case toolbrowser.CleanupFailed:
				entry.Browser.CleanupIssues++
			}
		}
	}

	if computer != nil {
		for _, session := range computer.ActiveSessions {
			if session.Owner.Kind != toolcomputer.OwnerACP {
				continue
			}
			entry := ensure(session.Owner.OwnerProfileID, session.Owner.OwnerACPSessionID)
			if entry == nil {
				continue
			}
			entry.Computer.SessionIDs = append(entry.Computer.SessionIDs, session.SessionID)
			switch session.Capability {
			case toolcomputer.CapabilityObserve:
				entry.Computer.Observe++
			case toolcomputer.CapabilityAct:
				entry.Computer.Act++
			}
		}
	}

	keys := make([]acpBrokerKey, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].profileID != keys[j].profileID {
			return keys[i].profileID < keys[j].profileID
		}
		return keys[i].sessionID < keys[j].sessionID
	})
	for _, key := range keys {
		entry := byKey[key]
		sort.Strings(entry.Browser.LeaseIDs)
		sort.Strings(entry.Computer.SessionIDs)
		result.Sessions = append(result.Sessions, *entry)
	}
	return result
}

func acpStringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
