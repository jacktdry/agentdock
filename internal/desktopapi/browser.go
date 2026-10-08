package desktopapi

import (
	"context"
	"errors"
	"time"

	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/desktopruntime"
)

type BrowserSnapshotResult struct {
	Snapshot browserdesktop.Snapshot `json:"snapshot"`
	Error    *APIError               `json:"error,omitempty"`
}

type BrowserService struct {
	runtimeRoot string
	rootError   error
	read        func(context.Context, string) (browserdesktop.Snapshot, error)
}

func NewBrowserService(root string) *BrowserService {
	root, err := resolveRuntimeRoot(root)
	return &BrowserService{
		runtimeRoot: root,
		rootError:   err,
		read:        desktopruntime.ReadVerifiedNextBrowserSnapshot,
	}
}

func browserFailure(code string, category ErrorCategory, retry bool) BrowserSnapshotResult {
	return BrowserSnapshotResult{
		Snapshot: browserdesktop.Snapshot{Availability: "core_unavailable", State: "unavailable", ConnectorHealth: browserdesktop.ConnectorHealthNotObserved},
		Error:    NewError(code, "Browser snapshot unavailable", category, retry, nil),
	}
}

// Snapshot never connects to an arbitrary TCP port or reads a Core bearer.
// The verified macOS Unix socket peer must be the selected signed Next Core
// process; other platforms and untrusted roots fail closed.
func (s *BrowserService) Snapshot(ctx context.Context) BrowserSnapshotResult {
	if s == nil || s.rootError != nil || s.read == nil {
		return browserFailure("BROWSER_CORE_UNAVAILABLE", ErrorCategoryUnavailable, false)
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return browserFailure("BROWSER_TIMEOUT", ErrorCategoryTimeout, true)
		}
		return browserFailure("BROWSER_CORE_UNAVAILABLE", ErrorCategoryUnavailable, true)
	}
	snapshot, err := s.read(ctx, s.runtimeRoot)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return browserFailure("BROWSER_TIMEOUT", ErrorCategoryTimeout, true)
		}
		return browserFailure("BROWSER_CORE_UNAVAILABLE", ErrorCategoryUnavailable, true)
	}
	if !validBrowserSnapshot(snapshot) {
		return browserFailure("BROWSER_RESPONSE_INVALID", ErrorCategoryInternal, false)
	}
	return BrowserSnapshotResult{Snapshot: snapshot}
}

// Only allowlisted typed fields are returned; raw errors, identifiers and
// browsing data are never accepted by this API.
func validBrowserSnapshot(s browserdesktop.Snapshot) bool {
	if s.ConnectorHealth != browserdesktop.ConnectorHealthNotObserved {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, s.ObservedAt); err != nil {
		return false
	}
	switch s.Availability {
	case "available", "browser_disabled", "acp_disabled", "broker_unavailable", "core_unavailable":
	default:
		return false
	}
	switch s.State {
	case "idle", "leases_present", "stale", "unavailable":
	default:
		return false
	}
	for _, n := range []int{
		s.ConfiguredConnectors, s.ConfiguredAuthenticatedEdgeProfiles, s.ConfiguredRequiredExternalPolicies,
		s.ManagedLeases, s.RequiredExternalLeases, s.ExplicitExternalLeases,
		s.CompanyRequiredEdgePolicies, s.Owners, s.Leases, s.ActiveLeases,
		s.ExpiredLeases, s.ReleasingLeases, s.FailedLeases, s.UnownedLeases,
		s.Workers, s.ReadyWorkers, s.FailedWorkers, s.ActiveOperations,
		s.QueuedOperations, s.MaxConcurrency, s.QueueCapacity, s.ManagedOrphans,
		s.ExternalOrphans,
	} {
		if n < 0 {
			return false
		}
	}
	if s.CompanyRequiredEdgePolicies > s.ConfiguredRequiredExternalPolicies {
		return false
	}
	if s.Availability != "available" {
		// Unavailable observations must never carry retained broker counters.
		return s.State == "unavailable" && !s.Stale && !s.LifecycleError &&
			s.Owners == 0 && s.Leases == 0 && s.ManagedLeases == 0 && s.RequiredExternalLeases == 0 && s.ExplicitExternalLeases == 0 &&
			s.ActiveLeases == 0 && s.ExpiredLeases == 0 && s.ReleasingLeases == 0 && s.FailedLeases == 0 && s.UnownedLeases == 0 &&
			s.Workers == 0 && s.ReadyWorkers == 0 && s.FailedWorkers == 0 && s.ActiveOperations == 0 && s.QueuedOperations == 0 &&
			s.MaxConcurrency == 0 && s.QueueCapacity == 0 && s.ManagedOrphans == 0 && s.ExternalOrphans == 0
	}
	// Subtract bounded counts to avoid accepting an overflowing route-count sum.
	remaining := s.Leases
	for _, n := range []int{s.ManagedLeases, s.RequiredExternalLeases, s.ExplicitExternalLeases} {
		if n > remaining {
			return false
		}
		remaining -= n
	}
	if remaining != 0 {
		return false
	}
	for _, n := range []int{s.ActiveLeases, s.ExpiredLeases, s.ReleasingLeases, s.FailedLeases, s.UnownedLeases} {
		if n > s.Leases {
			return false
		}
	}
	if s.ActiveLeases > s.Leases-s.ReleasingLeases || s.FailedLeases > s.Leases-s.ReleasingLeases-s.ActiveLeases {
		return false
	}
	stale := s.ExpiredLeases > 0 || s.FailedLeases > 0 || s.UnownedLeases > 0 || s.FailedWorkers > 0 || s.ManagedOrphans > 0 || s.ExternalOrphans > 0 || s.LifecycleError
	if s.Stale != stale || (s.State == "stale") != stale ||
		(!stale && ((s.State == "idle") != (s.Leases == 0))) {
		return false
	}
	return s.ReadyWorkers <= s.Workers && s.FailedWorkers <= s.Workers &&
		s.ReadyWorkers <= s.Workers-s.FailedWorkers && s.State != "unavailable"
}
