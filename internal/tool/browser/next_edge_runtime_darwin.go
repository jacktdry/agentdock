//go:build darwin

package browser

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func nativeEdgePlatformDependencies() (nativeEdgeDependencies, error) {
	return nativeEdgeDependencies{dial: dialNativeEdgeConnection, verify: verifyNativeEdgeConnection}, nil
}

func dialNativeEdgeConnection(ctx context.Context, connector browserpolicy.ConnectorDefinition, profile browserpolicy.ProfileDefinition, scope browserpolicy.OriginScope) (connection *nativeEdgeConnection, err error) {
	stage := "configuration"
	defer func() {
		if err != nil {
			err = &nativeEdgeConnectFailure{stage: stage}
		}
	}()
	fail := func() (*nativeEdgeConnection, error) { return nil, externalPeerVerificationError() }
	home, err := os.UserHomeDir()
	selectionValid := (edgeProfileDirectoryValid(profile.ProfileDirectory) && !profile.BindCurrentProfile) || (profile.ProfileDirectory == "" && profile.BindCurrentProfile)
	if err != nil || profile.UserDataDir != filepath.Join(home, "Library", "Application Support", "Microsoft Edge") || !selectionValid || !directCDPEndpointValid(connector.Endpoint) {
		return fail()
	}
	stage = "process_preflight"
	source, err := readEdgeProcessPreflightCommand(ctx, connector.Endpoint, profile.UserDataDir, macEdgeProcessProbe{}, true)
	if err != nil {
		return fail()
	}
	lifetime, cancel := context.WithCancel(context.Background())
	stopCancel := context.AfterFunc(ctx, cancel)
	keep := false
	var ownedRelay *directCDPRelay
	defer func() {
		if !stopCancel() || ctx.Err() != nil {
			keep = false
			err = externalPeerVerificationError()
		}
		if !keep {
			cancel()
			if ownedRelay != nil {
				ownedRelay.Close()
				select {
				case <-ownedRelay.Done():
				case <-time.After(3 * time.Second):
					err = externalPeerVerificationError()
				}
			}
			connection = nil
		}
	}()
	peer := externalPeerIdentity{source: &connectorEvidenceIdentity{}, incarnation: &connectorEvidenceIdentity{}}
	stage = "socket_and_websocket"
	relay, err := newDirectCDPRelayWithLimit(lifetime, connector.Endpoint, peer, func(check context.Context, socket net.Conn) (externalPeerIdentity, error) {
		if _, err := readEdgeEstablishedSocketEvidence(check, socket, source, macEdgeSocketProbe{}); err != nil {
			return externalPeerIdentity{}, err
		}
		return peer, nil
	}, 0)
	if err != nil {
		return fail()
	}
	ownedRelay = relay
	connection = &nativeEdgeConnection{peer: peer, source: source, profile: profile, connector: connector, cancel: cancel}
	stage = "dispatcher_attach"
	root, err := newDirectCDPDispatcher(ctx, relay, scope)
	if err != nil {
		relay.Close()
		return fail()
	}
	connection.root = root
	stage = "browser_metadata"
	opCtx, unlock, err := root.lock(ctx)
	if err != nil {
		return fail()
	}
	var version struct {
		Product string `json:"product"`
	}
	err = root.callLocked(opCtx, "Browser.getVersion", "", struct{}{}, &version)
	var contexts struct {
		Default string `json:"defaultBrowserContextId"`
	}
	if err == nil {
		err = root.callLocked(opCtx, "Target.getBrowserContexts", "", struct{}{}, &contexts)
	}
	unlock()
	if err != nil || !strings.HasPrefix(version.Product, "Edg/") || contexts.Default == "" {
		return fail()
	}
	stage = "profile_context"
	if err := root.bindBrowserContext(ctx, contexts.Default); err != nil {
		return fail()
	}
	probe, err := root.forkTarget(scope)
	if err != nil {
		return fail()
	}
	stage = "owned_target_create"
	if err := probe.OpenOwnedTarget(ctx, ""); err != nil {
		return fail()
	}
	stage = "owned_profile_metadata"
	directory, probeErr := probe.inspectOwnedEdgeProfile(ctx, profile.UserDataDir)
	cleanup, cancelCleanup := context.WithTimeout(context.Background(), directCDPCallTimeout)
	closed, closeErr := probe.CloseOwnedTarget(cleanup)
	cancelCleanup()
	if probeErr != nil || (profile.ProfileDirectory != "" && directory != profile.ProfileDirectory) {
		return fail()
	}
	stage = "owned_profile_cleanup"
	if closeErr != nil || !closed {
		return fail()
	}
	connection.profile.ProfileDirectory = directory
	connection.profile.BindCurrentProfile = false
	stage = "profile_epoch"
	metadata, err := (macEdgeProcessProbe{}).profileMetadata(ctx, filepath.Join(profile.UserDataDir, directory))
	if err != nil {
		return fail()
	}
	connection.profileEpoch = metadata
	stage = "peer_revalidation"
	if verifyNativeEdgeConnection(ctx, connection) != nil {
		return fail()
	}
	keep = true
	return connection, nil
}

// Re-observe OS process + Profile epochs and the reverse tuple of THIS held
// TCP socket. A successor listening on the same port cannot replace this
// connection. No expected-pointer echo, discovery, reconnect or PID kill exists.
func verifyNativeEdgeConnection(ctx context.Context, connection *nativeEdgeConnection) error {
	if ctx == nil || ctx.Err() != nil || connection == nil || connection.root == nil || !connection.root.live() ||
		!connection.root.relay.MatchesPeer(connection.peer) {
		return &nativeEdgePeerStageFailure{stage: "transport"}
	}
	check, cancel := context.WithTimeout(ctx, nativeEdgePeerVerificationBudget)
	defer cancel()
	fresh, err := revalidateEdgeProcessEpoch(check, connection.source, macEdgeProcessProbe{})
	if err != nil {
		return &nativeEdgePeerStageFailure{stage: "process_epoch"}
	}
	metadata, err := (macEdgeProcessProbe{}).profileMetadata(check, filepath.Join(connection.profile.UserDataDir, connection.profile.ProfileDirectory))
	if err != nil || metadata != connection.profileEpoch {
		return &nativeEdgePeerStageFailure{stage: "profile_epoch"}
	}
	if _, err := readEdgeEstablishedSocketEvidence(check, connection.root.relay.up.UnderlyingConn(), fresh, macEdgeSocketProbe{}); err != nil {
		return &nativeEdgePeerStageFailure{stage: "held_socket"}
	}
	if check.Err() != nil || !connection.root.relay.MatchesPeer(connection.peer) {
		return &nativeEdgePeerStageFailure{stage: "postcondition"}
	}
	return nil
}
