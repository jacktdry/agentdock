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
		Snapshot: browserdesktop.Snapshot{Availability: "core_unavailable", State: "unavailable"},
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
	return true
}
