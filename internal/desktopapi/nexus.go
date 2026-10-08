package desktopapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	ObservedAt        time.Time         `json:"observedAt"`
	Capabilities      NexusCapabilities `json:"capabilities"`
	Error             *APIError         `json:"error,omitempty"`
}

// Dependencies are private backend seams; no arbitrary state path is exposed.
type nexusDependencies struct {
	readIdentity func(context.Context, string) ([]byte, error)
	observe      func(context.Context, string) (desktopruntime.ServiceStatus, error)
}

type NexusService struct {
	runtimeRoot string
	deps        nexusDependencies
}

func NewNexusService(runtimeRoot string) *NexusService {
	return &NexusService{runtimeRoot: runtimeRoot, deps: nexusDependencies{
		readIdentity: desktopruntime.ReadNextNexusIdentity,
		observe: func(ctx context.Context, root string) (desktopruntime.ServiceStatus, error) {
			// Do not dial the mutable control.sock path until a Next-Core peer
			// identity and the active Nexus identity generation are verified.
			// A socket type check followed by dial is vulnerable to a symlink swap.
			// This safe read-only phase intentionally cannot claim live connectivity.
			return desktopruntime.ServiceStatus{}, errors.New("nexus_connection_unverified")
		},
	}}
}

func nexusGeneration(data []byte, missing bool) string {
	h := sha256.New()
	h.Write([]byte("agentdock-next-nexus-identity-v1\x00"))
	if missing {
		h.Write([]byte("absent"))
	} else {
		h.Write([]byte("present\x00"))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func nexusReadError(code string) *APIError {
	return NewError(code, "Nexus status unavailable", ErrorCategoryUnavailable, true, nil)
}

func (s *NexusService) Snapshot(ctx context.Context) NexusSnapshotResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r := NexusSnapshotResult{PairingState: "unknown", ConnectionState: "unavailable", ObservedAt: time.Now().UTC(),
		Capabilities: NexusCapabilities{PairDisabledReason: "nexus_pair_unavailable_until_a2_2", ReconcileDisabledReason: "nexus_reconcile_unavailable_until_a2_2"}}
	data, err := s.deps.readIdentity(ctx, s.runtimeRoot)
	if errors.Is(err, desktopruntime.ErrNextIdentityUnavailable) || ctx.Err() != nil {
		r.Error = nexusReadError("next_identity_unavailable")
		return r
	}
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		r.PairingState = "invalid"
		r.Error = nexusReadError("nexus_identity_invalid")
		return r
	}
	r.Generation = nexusGeneration(data, missing)
	r.ConnectionState = "unknown"
	if missing {
		r.PairingState = "not_paired"
	} else {
		var identity nexusbridge.Identity
		if json.Unmarshal(data, &identity) != nil || identity.Version != 1 || identity.DeviceID == "" || strings.TrimSpace(identity.DeviceToken) == "" || !safeNexusNodeID(identity.NodeID) || safeNexusOrigin(identity.Endpoint) == "" {
			r.PairingState = "invalid"
			r.Error = nexusReadError("nexus_identity_invalid")
			return r
		}
		r.PairingState, r.SafeOrigin, r.NodeID, r.DeviceTokenStored = "paired", safeNexusOrigin(identity.Endpoint), identity.NodeID, true
	}
	status, observationErr := s.deps.observe(ctx, s.runtimeRoot)
	if observationErr == nil {
		if !status.Running {
			r.ConnectionState = "disconnected"
		} else if !status.Healthy {
			r.ConnectionState = "unknown"
		} else if !status.NexusConnected {
			r.ConnectionState = "disconnected"
		} else {
			// A Core connection flag cannot prove it is using the identity
			// currently persisted on disk. Phase A2.2 needs a matching active
			// identity generation before reporting connected.
			r.ConnectionState = "unknown"
		}
	} else {
		r.Error = nexusReadError("nexus_observation_unavailable")
	}
	// Revalidate authority and revision after the observation without mutation locks.
	current, currentErr := s.deps.readIdentity(ctx, s.runtimeRoot)
	if ctx.Err() != nil || (currentErr != nil && !errors.Is(currentErr, os.ErrNotExist)) || nexusGeneration(current, errors.Is(currentErr, os.ErrNotExist)) != r.Generation {
		r.PairingState, r.ConnectionState, r.Generation, r.SafeOrigin, r.NodeID, r.DeviceTokenStored = "unknown", "unknown", "", "", "", false
		r.Error = nexusReadError("nexus_identity_changed")
	}
	r.ObservedAt = time.Now().UTC()
	return r
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
