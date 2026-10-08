package desktopapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
	"github.com/uvwt/agentdock/internal/nexusbridge"
)

type NexusCapabilities struct {
	CanPair                 bool   `json:"canPair"`
	CanReconcile            bool   `json:"canReconcile"`
	PairDisabledReason      string `json:"pairDisabledReason"`
	ReconcileDisabledReason string `json:"reconcileDisabledReason"`
}

type NexusSnapshotResult struct {
	PairingState      string            `json:"pairingState"`
	ConnectionState   string            `json:"connectionState"`
	Generation        string            `json:"generation"`
	SafeOrigin        string            `json:"safeOrigin,omitempty"`
	NodeID            string            `json:"nodeId,omitempty"`
	DeviceTokenStored bool              `json:"deviceTokenStored"`
	RestartRequired   bool              `json:"restartRequired"`
	ObservedAt        time.Time         `json:"observedAt"`
	Capabilities      NexusCapabilities `json:"capabilities"`
	Error             *APIError         `json:"error,omitempty"`
}

type NexusPairRequest struct {
	Endpoint           string `json:"endpoint"`
	Code               string `json:"code"`
	Name               string `json:"name,omitempty"`
	ExpectedGeneration string `json:"expectedGeneration"`
	ConfirmReplace     bool   `json:"confirmReplace"`
}

type NexusMutationResult struct {
	OperationID        string    `json:"operationId"`
	Completed          bool      `json:"completed"`
	IdentitySaved      bool      `json:"identitySaved"`
	RestartRequired    bool      `json:"restartRequired"`
	ObservedGeneration string    `json:"observedGeneration,omitempty"`
	Error              *APIError `json:"error,omitempty"`
}

// Dependencies are private backend seams; no arbitrary state path is exposed.
type nexusDependencies struct {
	readIdentity func(context.Context, string) ([]byte, error)
	observe      func(context.Context, string) (desktopruntime.ServiceStatus, error)
	resolveHome  func(context.Context, string) (string, error)
	pair         func(context.Context, string, nexusbridge.PairOptions, string, bool) (nexusbridge.Identity, error)
	restart      func(context.Context, string, string) error
}

type NexusService struct {
	runtimeRoot string
	deps        nexusDependencies
}

func NewNexusService(runtimeRoot string) *NexusService {
	return &NexusService{runtimeRoot: runtimeRoot, deps: nexusDependencies{
		readIdentity: desktopruntime.ReadNextNexusIdentity,
		observe:      desktopruntime.ReadVerifiedNextNexusRuntimeStatus,
		resolveHome:  desktopruntime.ResolveNextAgentDockHome,
		pair:         nexusbridge.PairChecked,
		restart:      desktopruntime.RunServiceActionLocked,
	}}
}

func nexusReadError(code string) *APIError {
	return NewError(code, "Nexus status unavailable", ErrorCategoryUnavailable, true, nil)
}

func nexusIdentitySnapshot(data []byte, missing bool) (nexusbridge.Identity, string, error) {
	if missing {
		return nexusbridge.Identity{}, nexusbridge.AbsentGeneration(), nil
	}
	var identity nexusbridge.Identity
	if json.Unmarshal(data, &identity) != nil ||
		identity.Version != 1 ||
		identity.DeviceID == "" ||
		strings.TrimSpace(identity.DeviceToken) == "" ||
		!safeNexusNodeID(identity.NodeID) ||
		safeNexusOrigin(identity.Endpoint) == "" {
		return nexusbridge.Identity{}, "", errors.New("nexus identity invalid")
	}
	return identity, nexusbridge.Generation(identity), nil
}

func (s *NexusService) Snapshot(ctx context.Context) NexusSnapshotResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r := NexusSnapshotResult{
		PairingState:    "unknown",
		ConnectionState: "unavailable",
		ObservedAt:      time.Now().UTC(),
		Capabilities: NexusCapabilities{
			PairDisabledReason:      "next_identity_unavailable",
			ReconcileDisabledReason: "nexus_not_paired",
		},
	}
	data, err := s.deps.readIdentity(ctx, s.runtimeRoot)
	if errors.Is(err, desktopruntime.ErrNextIdentityUnavailable) || ctx.Err() != nil {
		r.Error = nexusReadError("next_identity_unavailable")
		return r
	}
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		r.PairingState = "invalid"
		r.Error = nexusReadError("nexus_identity_invalid")
		r.Capabilities.PairDisabledReason = "nexus_identity_invalid"
		return r
	}
	identity, generation, err := nexusIdentitySnapshot(data, missing)
	if err != nil {
		r.PairingState = "invalid"
		r.ConnectionState = "unknown"
		r.Error = nexusReadError("nexus_identity_invalid")
		r.Capabilities.PairDisabledReason = "nexus_identity_invalid"
		return r
	}
	r.Generation = generation
	r.ConnectionState = "unknown"
	r.Capabilities.CanPair = true
	r.Capabilities.PairDisabledReason = ""
	if missing {
		r.PairingState = "not_paired"
	} else {
		r.PairingState = "paired"
		r.SafeOrigin = safeNexusOrigin(identity.Endpoint)
		r.NodeID = identity.NodeID
		r.DeviceTokenStored = true
		r.Capabilities.CanReconcile = true
		r.Capabilities.ReconcileDisabledReason = ""
	}

	status, observationErr := s.deps.observe(ctx, s.runtimeRoot)
	switch {
	case observationErr != nil:
		r.Error = nexusReadError("nexus_observation_unavailable")
	case !status.Running:
		r.ConnectionState = "disconnected"
	case missing:
		r.ConnectionState = "disconnected"
	case status.NexusIdentityGeneration == "" || status.NexusIdentityGeneration != r.Generation:
		r.ConnectionState = "unknown"
		r.RestartRequired = true
	case !status.Healthy:
		r.ConnectionState = "unknown"
	case status.NexusConnected:
		r.ConnectionState = "connected"
	default:
		r.ConnectionState = "disconnected"
	}

	// Revalidate the persisted semantic identity after observing Core. Formatting-
	// only file rewrites do not force a restart, but any identity-field change does.
	current, currentErr := s.deps.readIdentity(ctx, s.runtimeRoot)
	currentMissing := errors.Is(currentErr, os.ErrNotExist)
	if ctx.Err() != nil || (currentErr != nil && !currentMissing) {
		r.PairingState, r.ConnectionState, r.Generation, r.SafeOrigin, r.NodeID, r.DeviceTokenStored = "unknown", "unknown", "", "", "", false
		r.RestartRequired = false
		r.Capabilities = NexusCapabilities{PairDisabledReason: "next_identity_unavailable", ReconcileDisabledReason: "next_identity_unavailable"}
		r.Error = nexusReadError("nexus_identity_changed")
		return r
	}
	_, currentGeneration, generationErr := nexusIdentitySnapshot(current, currentMissing)
	if generationErr != nil || currentGeneration != r.Generation {
		r.PairingState, r.ConnectionState, r.Generation, r.SafeOrigin, r.NodeID, r.DeviceTokenStored = "unknown", "unknown", "", "", "", false
		r.RestartRequired = false
		r.Capabilities = NexusCapabilities{PairDisabledReason: "nexus_identity_changed", ReconcileDisabledReason: "nexus_identity_changed"}
		r.Error = nexusReadError("nexus_identity_changed")
	}
	r.ObservedAt = time.Now().UTC()
	return r
}

func (s *NexusService) Pair(ctx context.Context, request NexusPairRequest) NexusMutationResult {
	result := NexusMutationResult{OperationID: newNexusOperationID()}
	if strings.TrimSpace(request.ExpectedGeneration) == "" {
		result.Error = nexusMutationError("nexus_generation_conflict", "Nexus state changed; refresh before pairing", ErrorCategoryConflict, false)
		return result
	}
	home, err := s.deps.resolveHome(ctx, s.runtimeRoot)
	if err != nil {
		result.Error = nexusMutationError("next_identity_unavailable", "Next Nexus identity is unavailable", ErrorCategoryUnavailable, true)
		return result
	}
	operationCtx, finish, err := beginRuntimeMutation(ctx, s.runtimeRoot)
	if err != nil {
		result.Error = nexusMutationError("nexus_mutation_busy", "Another Desktop operation is in progress", ErrorCategoryConflict, true)
		return result
	}
	defer finish()
	lockedHome, err := s.deps.resolveHome(operationCtx, s.runtimeRoot)
	if err != nil || lockedHome != home {
		result.Error = nexusMutationError("next_identity_unavailable", "Next Nexus identity is unavailable", ErrorCategoryUnavailable, false)
		return result
	}
	data, readErr := s.deps.readIdentity(operationCtx, s.runtimeRoot)
	missing := errors.Is(readErr, os.ErrNotExist)
	if readErr != nil && !missing {
		result.Error = nexusMutationError("nexus_identity_invalid", "Existing Nexus identity is invalid", ErrorCategoryConflict, false)
		return result
	}
	_, currentGeneration, parseErr := nexusIdentitySnapshot(data, missing)
	if parseErr != nil {
		result.Error = nexusMutationError("nexus_identity_invalid", "Existing Nexus identity is invalid", ErrorCategoryConflict, false)
		return result
	}
	if currentGeneration != request.ExpectedGeneration {
		result.Error = nexusMutationError("nexus_generation_conflict", "Nexus state changed; refresh before pairing", ErrorCategoryConflict, false)
		return result
	}
	if !missing && !request.ConfirmReplace {
		result.Error = nexusMutationError("nexus_replace_confirmation_required", "Replacing the existing Nexus identity requires confirmation", ErrorCategoryConflict, false)
		return result
	}
	identity, err := s.deps.pair(operationCtx, lockedHome, nexusbridge.PairOptions{
		Endpoint: request.Endpoint,
		Code:     request.Code,
		Name:     request.Name,
	}, request.ExpectedGeneration, request.ConfirmReplace)
	if err != nil {
		switch {
		case errors.Is(err, nexusbridge.ErrGenerationConflict):
			result.Error = nexusMutationError("nexus_generation_conflict", "Nexus state changed; obtain a new pairing code and refresh", ErrorCategoryConflict, false)
		case errors.Is(err, nexusbridge.ErrReplaceConfirmationNeeded):
			result.Error = nexusMutationError("nexus_replace_confirmation_required", "Replacing the existing Nexus identity requires confirmation", ErrorCategoryConflict, false)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), operationCtx.Err() != nil:
			result.Error = nexusMutationError("nexus_pair_timeout", "Nexus pairing timed out or was canceled", ErrorCategoryTimeout, true)
		default:
			result.Error = nexusMutationError("nexus_pair_denied", "Nexus pairing failed", ErrorCategoryOperation, false)
		}
		return result
	}
	result.IdentitySaved = true
	result.ObservedGeneration = nexusbridge.Generation(identity)
	if err := s.deps.restart(operationCtx, s.runtimeRoot, "restart"); err != nil {
		result.RestartRequired = true
		result.Error = nexusMutationError("nexus_restart_required", "Nexus identity was saved but Next Core must be restarted", ErrorCategoryUnavailable, true)
		return result
	}
	status, err := s.deps.observe(operationCtx, s.runtimeRoot)
	if err != nil || !status.Running || status.NexusIdentityGeneration != result.ObservedGeneration {
		result.RestartRequired = true
		result.Error = nexusMutationError("nexus_restart_required", "Nexus identity was saved but the active Next Core identity could not be verified", ErrorCategoryUnavailable, true)
		return result
	}
	result.Completed = true
	return result
}

func (s *NexusService) Reconcile(ctx context.Context, expectedGeneration string) NexusMutationResult {
	result := NexusMutationResult{OperationID: newNexusOperationID()}
	if strings.TrimSpace(expectedGeneration) == "" {
		result.Error = nexusMutationError("nexus_generation_conflict", "Nexus state changed; refresh before retrying", ErrorCategoryConflict, false)
		return result
	}
	home, err := s.deps.resolveHome(ctx, s.runtimeRoot)
	if err != nil {
		result.Error = nexusMutationError("next_identity_unavailable", "Next Nexus identity is unavailable", ErrorCategoryUnavailable, true)
		return result
	}
	operationCtx, finish, err := beginRuntimeMutation(ctx, s.runtimeRoot)
	if err != nil {
		result.Error = nexusMutationError("nexus_mutation_busy", "Another Desktop operation is in progress", ErrorCategoryConflict, true)
		return result
	}
	defer finish()
	lockedHome, err := s.deps.resolveHome(operationCtx, s.runtimeRoot)
	if err != nil || lockedHome != home {
		result.Error = nexusMutationError("next_identity_unavailable", "Next Nexus identity is unavailable", ErrorCategoryUnavailable, false)
		return result
	}
	data, readErr := s.deps.readIdentity(operationCtx, s.runtimeRoot)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			result.Error = nexusMutationError("nexus_not_paired", "No Nexus identity is available to reconcile", ErrorCategoryValidation, false)
		} else {
			result.Error = nexusMutationError("nexus_identity_invalid", "Existing Nexus identity is invalid", ErrorCategoryConflict, false)
		}
		return result
	}
	_, currentGeneration, parseErr := nexusIdentitySnapshot(data, false)
	if parseErr != nil {
		result.Error = nexusMutationError("nexus_identity_invalid", "Existing Nexus identity is invalid", ErrorCategoryConflict, false)
		return result
	}
	if currentGeneration != expectedGeneration {
		result.Error = nexusMutationError("nexus_generation_conflict", "Nexus state changed; refresh before retrying", ErrorCategoryConflict, false)
		return result
	}
	result.IdentitySaved = true
	result.ObservedGeneration = currentGeneration
	if err := s.deps.restart(operationCtx, s.runtimeRoot, "restart"); err != nil {
		result.RestartRequired = true
		result.Error = nexusMutationError("nexus_restart_required", "Next Core must be restarted to load the saved Nexus identity", ErrorCategoryUnavailable, true)
		return result
	}
	status, err := s.deps.observe(operationCtx, s.runtimeRoot)
	if err != nil || !status.Running || status.NexusIdentityGeneration != currentGeneration {
		result.RestartRequired = true
		result.Error = nexusMutationError("nexus_restart_required", "The active Next Core identity could not be verified", ErrorCategoryUnavailable, true)
		return result
	}
	result.Completed = true
	return result
}

func nexusMutationError(code, message string, category ErrorCategory, retryable bool) *APIError {
	return NewError(code, message, category, retryable, nil)
}

func newNexusOperationID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ""
	}
	return "nexus_" + base64.RawURLEncoding.EncodeToString(raw[:])
}

func safeNexusNodeID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func safeNexusOrigin(value string) string {
	if len(value) > 4096 {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(value, "#") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || len(host) > 253 {
		return ""
	}
	if port := u.Port(); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return ""
		}
	}
	ip, ipErr := netip.ParseAddr(host)
	if ipErr == nil && ip.Zone() != "" {
		return ""
	}
	if ipErr != nil && host != "localhost" {
		for _, c := range host {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
				return ""
			}
		}
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (host == "localhost" || ipErr == nil && ip.IsLoopback())) {
		return ""
	}
	return u.Scheme + "://" + strings.ToLower(u.Host)
}
