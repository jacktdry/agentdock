//go:build darwin

package browser

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// NextEdgeProbeResult is diagnostic evidence only. No endpoint, full Profile
// path, browserContextId, route grant or authenticated status leaves Core.
type NextEdgeProbeResult struct {
	Stage                   string              `json:"stage"`
	FailureReason           string              `json:"failure_reason,omitempty"`
	ProtocolErrorCode       int                 `json:"protocol_error_code,omitempty"`
	BrowserVersion          string              `json:"browser_version"`
	ContextCount            int                 `json:"context_count"`
	DefaultContextAvailable bool                `json:"default_context_available"`
	ProfileDirectory        string              `json:"profile_directory,omitempty"`
	WorkspaceRoot           string              `json:"workspace_root,omitempty"`
	OwnedTargetClosed       bool                `json:"owned_target_closed"`
	TransportClosed         bool                `json:"transport_closed"`
	Sites                   []NextEdgeSiteProbe `json:"sites,omitempty"`
}

// ProbeNextEdge checks the existing Edge without discovery/relaunch/config
// mutation or use of Stable MCP. inspectProfile explicitly creates just one
// hidden, background, owned version-page target in the current default
// context. This is a source/Host diagnostic, NOT installed NEXT Broker UAT.
func ProbeNextEdge(ctx context.Context, inspectProfile bool) (NextEdgeProbeResult, error) {
	return probeNextEdge(ctx, inspectProfile, false, "")
}

// ProbeNextEdgeSites is a bounded source smoke for the four owner-approved
// public HTTPS origins. Checks use separate owned hidden targets on one socket;
// no website input, click, arbitrary script, GitLab action or mutation exists.
// This deliberately grants no ConnectorStatusProvider/Browser Broker authority.
func ProbeNextEdgeSites(ctx context.Context, workspace string) (NextEdgeProbeResult, error) {
	return probeNextEdge(ctx, true, true, workspace)
}

func probeNextEdge(ctx context.Context, inspectProfile, sites bool, workspace string) (result NextEdgeProbeResult, err error) {
	fail := func() (NextEdgeProbeResult, error) {
		var protocol *directCDPProtocolError
		if errors.As(err, &protocol) {
			result.FailureReason = protocol.reason
			result.ProtocolErrorCode = protocol.code
		}
		return result, errEdgeProcessUnqualified
	}
	if ctx == nil || ctx.Err() != nil {
		return fail()
	}
	origins := nextEdgeProbeOrigins[:]
	if sites {
		result.Stage = "workspace_scope"
		origins, err = nextEdgeProbeScopeForWorkspace(workspace)
		if err != nil {
			return fail()
		}
		result.WorkspaceRoot = workspace
	}
	limit := 20 * time.Second
	if sites {
		limit = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	home, err := os.UserHomeDir()
	if err != nil {
		return fail()
	}
	root := filepath.Join(home, "Library", "Application Support", "Microsoft Edge")
	const endpoint = "ws://127.0.0.1:9222/devtools/browser"
	result.Stage = "process_preflight"
	source, err := readEdgeProcessPreflightCommand(ctx, endpoint, root, macEdgeProcessProbe{}, true)
	if err != nil {
		return fail()
	}
	peer := externalPeerIdentity{source: &connectorEvidenceIdentity{}, incarnation: &connectorEvidenceIdentity{}}
	result.Stage = "socket_and_websocket"
	var socketWitness atomic.Int32
	// Both the pre-handshake OS witness and the relay use this same concrete
	// connected TCP socket. The relay never redials or reconnects. This remains
	// diagnostic evidence; it does not attest Profile login or owner consent.
	relay, err := newDirectCDPRelay(ctx, endpoint, peer, func(attestCtx context.Context, conn net.Conn) (externalPeerIdentity, error) {
		socketWitness.Store(1)
		if _, err := readEdgeEstablishedSocketEvidence(attestCtx, conn, source, macEdgeSocketProbe{}); err != nil {
			return externalPeerIdentity{}, errEdgeProcessUnqualified
		}
		socketWitness.Store(2)
		return peer, nil
	})
	if err != nil {
		if socketWitness.Load() == 1 {
			result.Stage = "socket_identity"
		}
		if socketWitness.Load() == 2 {
			result.Stage = "websocket_handshake"
		}
		return fail()
	}
	defer func() {
		relay.Close()
		select {
		case <-relay.Done():
			result.TransportClosed = true
		case <-time.After(3 * time.Second):
			err = errEdgeProcessUnqualified
		}
	}()
	// A fixed exact allowlist, never wildcard origins or caller-supplied URLs.
	scope, _ := browserpolicy.NewOriginScope(origins)
	d, err := newDirectCDPDispatcher(ctx, relay, scope)
	if err != nil {
		return fail()
	}
	defer d.Close()
	result.Stage = "browser_metadata"
	opCtx, unlock, err := d.lock(ctx)
	if err != nil {
		return fail()
	}
	var version struct {
		Product string `json:"product"`
	}
	err = d.callLocked(opCtx, "Browser.getVersion", "", struct{}{}, &version)
	var contexts struct {
		IDs     []string `json:"browserContextIds"`
		Default string   `json:"defaultBrowserContextId"`
	}
	if err == nil {
		err = d.callLocked(opCtx, "Target.getBrowserContexts", "", struct{}{}, &contexts)
	}
	unlock()
	if err != nil || !strings.HasPrefix(version.Product, "Edg/") || len(version.Product) > 64 {
		return fail()
	}
	result.BrowserVersion, result.ContextCount = version.Product, len(contexts.IDs)
	result.DefaultContextAvailable = contexts.Default != ""
	if !inspectProfile {
		result.Stage = "metadata_checked"
		return result, nil
	}
	result.Stage = "profile_context"
	if !result.DefaultContextAvailable {
		return fail()
	}
	if err = d.bindBrowserContext(ctx, contexts.Default); err != nil {
		return fail()
	}
	result.Stage = "owned_target_create"
	profileProbe, forkErr := d.forkTarget(scope)
	if forkErr != nil {
		return fail()
	}
	if err = profileProbe.OpenOwnedTarget(ctx, ""); err != nil {
		result.OwnedTargetClosed = profileProbe.closeConfirmed
		return fail()
	}
	result.Stage = "owned_profile_metadata"
	// Close exactly our created target even after a probe failure. Hidden
	// targets are also session-bound, so socket loss never authorizes another
	// connection or a guessed close against a replacement Edge.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		closed, closeErr := profileProbe.CloseOwnedTarget(cleanupCtx)
		result.OwnedTargetClosed = closed
		for _, site := range result.Sites {
			result.OwnedTargetClosed = result.OwnedTargetClosed && site.TargetClosed
		}
		if closeErr != nil || !closed {
			err = errEdgeProcessUnqualified
		}
	}()
	profile, err := profileProbe.inspectOwnedEdgeProfile(ctx, root)
	if err != nil {
		return fail()
	}
	result.ProfileDirectory = profile
	result.Stage = "profile_metadata_checked"
	if sites {
		if closed, closeErr := profileProbe.CloseOwnedTarget(ctx); closeErr != nil || !closed {
			return fail()
		}
		result.Stage = "read_only_sites"
		result.Sites, err = d.probeApprovedSites(ctx)
		if err != nil {
			return fail()
		}
		result.Stage = "read_only_sites_checked"
	}
	return result, nil
}
