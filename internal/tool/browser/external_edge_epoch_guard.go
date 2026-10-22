package browser

import (
	"context"
	"reflect"
	"time"
)

// This is source-only *process epoch* revalidation, not authenticated Edge
// Profile/CDP session attestation. It never mints a qualified route, probes
// pages, logs browser data, or changes the user's Edge process. Production
// still needs an independent socket-bound peer attestor and trusted provider.
// In particular, a correct process signature and an OS-owned localhost port
// alone do NOT prove the current WebSocket is that user's authorized Profile.
const edgeEpochSnapshotMaxAge = 2 * time.Second

// newEdgeProcessEpochRevalidation adapts the OS evidence reader to the private
// per-action externalFenceSeries hook. Each invocation independently re-reads
// signer/listener/profile/process-start evidence, and matches the immutable
// preflight snapshot; a reused PID with changed start epoch cannot pass.
// The opaque peer and WorkerInfo are matched to the original admission input
// but are not manufactured or certified by this function.
func newEdgeProcessEpochRevalidation(source edgeProcessPreflight, worker WorkerInfo, peer externalPeerIdentity, probe edgeProcessProbe) (externalPeerRevalidation, error) {
	age := time.Since(source.observedAt)
	if worker.WorkerID == "" || worker.State != WorkerReady || worker.Session.PID <= 0 ||
		peer.source == nil || peer.incarnation == nil || peer.source == peer.incarnation ||
		probe == nil || (reflect.ValueOf(probe).Kind() == reflect.Pointer && reflect.ValueOf(probe).IsNil()) ||
		source.pid <= 0 || source.startToken == "" || source.endpoint == "" || source.userDataDir == "" || !source.profile.valid() ||
		source.executable != "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge" ||
		source.observedAt.IsZero() || age < 0 || age > edgeEpochSnapshotMaxAge {
		return nil, externalTransportFenceError()
	}
	return func(ctx context.Context, expected externalPeerIdentity, currentWorker WorkerInfo) error {
		if ctx == nil || ctx.Err() != nil || expected != peer || currentWorker != worker {
			return externalTransportFenceError()
		}
		// readEdgeProcessPreflight has its own 2-second deadline and compares OS
		// evidence twice. Do NOT synthesize a new grant from its result.
		current, err := readEdgeProcessPreflightCommand(ctx, source.endpoint, source.userDataDir, probe, source.defaultLaunch)
		if err != nil || ctx.Err() != nil || current.pid != source.pid ||
			current.startToken != source.startToken || current.endpoint != source.endpoint ||
			current.userDataDir != source.userDataDir || current.profile != source.profile || current.executable != source.executable {
			return externalTransportFenceError()
		}
		return nil
	}, nil
}
