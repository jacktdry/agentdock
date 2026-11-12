package browser

import (
	"context"
	"net"
)

// PRIVATE source-only composition; no production caller or independent
// attestor exists. B2k is non-atomic read-only process evidence, not proof of
// actual Edge identity, consent, login or a protected profile. Even combined
// with an independent attestor this does not provide per-command CDP atomicity.
func newEdgeEvidencePinnedRelay(ctx context.Context, endpoint string, source edgeProcessPreflight, probe edgeSocketProbe, expected externalPeerIdentity, independentAttestor pinnedCDPAttestor) (*pinnedCDPRelay, error) {
	if probe == nil || independentAttestor == nil || endpoint != source.endpoint {
		return nil, errPinnedCDPUnavailable
	}
	// B2j validates expected separately and compares the independently observed
	// identity. Neither expected nor B2k evidence is given to the second attestor.
	return newPinnedCDPRelay(ctx, endpoint, expected, edgeEvidencePinnedAttestor(source, probe, independentAttestor))
}

func edgeEvidencePinnedAttestor(source edgeProcessPreflight, probe edgeSocketProbe, independentAttestor pinnedCDPAttestor) pinnedCDPAttestor {
	return func(ctx context.Context, conn net.Conn) (externalPeerIdentity, error) {
		if probe == nil || independentAttestor == nil {
			return externalPeerIdentity{}, errPinnedCDPUnavailable
		}
		if _, err := readEdgeEstablishedSocketEvidence(ctx, conn, source, probe); err != nil || ctx.Err() != nil {
			return externalPeerIdentity{}, errPinnedCDPUnavailable
		}
		return independentAttestor(ctx, conn)
	}
}
