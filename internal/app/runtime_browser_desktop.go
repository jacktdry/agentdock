package app

import (
	"time"

	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

// RuntimeBrowserDesktop reads existing broker state only. Availability is not
// connector health; observedAt timestamps this non-atomic diagnostic read.
func (r *Runtime) RuntimeBrowserDesktop() Result {
	now := time.Now().UTC()
	s := browserdesktop.Snapshot{ObservedAt: now.Format(time.RFC3339Nano), Availability: "core_unavailable", State: "unavailable", ConnectorHealth: browserdesktop.ConnectorHealthNotObserved}
	if r != nil {
		s.BrowserEnabled, s.ACPEnabled = r.cfg.BrowserEnabled, r.cfg.ACPEnabled
		// Loaded configuration describes intent only; do not re-read roots or probe endpoints.
		s.ConfiguredConnectors = len(r.cfg.BrowserCatalog.Connectors)
		for _, p := range r.cfg.BrowserCatalog.Profiles {
			if p.Browser == browserpolicy.BrowserEdge && p.Class == browserpolicy.ProfileAuthenticatedExternal {
				s.ConfiguredAuthenticatedEdgeProfiles++
			}
		}
		for _, p := range r.cfg.BrowserWorkspacePolicies {
			if p.Policy.Route == browserpolicy.RouteRequiredExternal {
				s.ConfiguredRequiredExternalPolicies++
				if p.Policy.Class == browserpolicy.WorkspaceCompany {
					s.CompanyRequiredEdgePolicies++
				}
			}
		}
		switch {
		case !s.BrowserEnabled:
			s.Availability = "browser_disabled"
		case !s.ACPEnabled:
			s.Availability = "acp_disabled"
		case r.acpBrowser == nil:
			s.Availability = "broker_unavailable"
		default:
			s.Availability = "available"
			s = aggregateBrowserDiagnostics(s, r.acpBrowser.Diagnostics(), now)
		}
	}
	return Result{"ok": true, "snapshot": s}
}

func aggregateBrowserDiagnostics(s browserdesktop.Snapshot, d toolbrowser.BrokerDiagnostics, now time.Time) browserdesktop.Snapshot {
	s.ConnectorHealth = browserdesktop.ConnectorHealthNotObserved
	for _, n := range []int{d.Queue.Active, d.Queue.Queued, d.Queue.MaxConcurrency, d.Queue.QueueCapacity, d.Queue.ManagedOrphans, d.Queue.ExternalOrphans} {
		if n < 0 {
			s.Availability, s.State = "broker_unavailable", "unavailable"
			return s
		}
	}
	// Unknown routes cannot be safely classified. Preserve only configuration intent.
	for _, l := range d.Leases {
		switch l.Route {
		case browserpolicy.RouteManaged, browserpolicy.RouteRequiredExternal, browserpolicy.RouteExternal:
		default:
			s.Availability, s.State = "broker_unavailable", "unavailable"
			return s
		}
	}
	s.Owners, s.Leases, s.Workers = len(d.Owners), len(d.Leases), len(d.Workers)
	for _, l := range d.Leases {
		switch l.Route {
		case browserpolicy.RouteManaged:
			s.ManagedLeases++
		case browserpolicy.RouteRequiredExternal:
			s.RequiredExternalLeases++
		case browserpolicy.RouteExternal:
			s.ExplicitExternalLeases++
		}
		expired := !l.ExpiresAt.IsZero() && !l.ExpiresAt.After(now)
		if expired {
			s.ExpiredLeases++
		}
		if l.ACPSessionID == "" {
			s.UnownedLeases++
		}
		switch l.CleanupState {
		case toolbrowser.CleanupPending:
			if !expired && l.ACPSessionID != "" {
				s.ActiveLeases++
			}
		case toolbrowser.CleanupReleasing:
			s.ReleasingLeases++
		case toolbrowser.CleanupFailed:
			s.FailedLeases++
		}
	}
	for _, w := range d.Workers {
		if w.State == toolbrowser.WorkerReady {
			s.ReadyWorkers++
		}
		if w.State == toolbrowser.WorkerFailed {
			s.FailedWorkers++
		}
	}
	s.ActiveOperations, s.QueuedOperations = d.Queue.Active, d.Queue.Queued
	s.MaxConcurrency, s.QueueCapacity = d.Queue.MaxConcurrency, d.Queue.QueueCapacity
	s.ManagedOrphans, s.ExternalOrphans = d.Queue.ManagedOrphans, d.Queue.ExternalOrphans
	s.LifecycleError = d.LifecycleLastError != ""
	s.Stale = s.ExpiredLeases > 0 || s.FailedLeases > 0 || s.UnownedLeases > 0 || s.FailedWorkers > 0 || s.ManagedOrphans > 0 || s.ExternalOrphans > 0 || s.LifecycleError
	s.State = "idle"
	if s.Leases > 0 {
		s.State = "leases_present"
	}
	if s.Stale {
		s.State = "stale"
	}
	return s
}
