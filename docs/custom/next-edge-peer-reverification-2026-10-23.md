# NEXT external Edge peer revalidation between fenced operations (2026-10-23)

**Status: reviewed source-only safety contract; NOT a production Edge attestor, a native Browser Broker `ExternalLeaseBackend`, or an activated company browser route.** The installed NEXT is still `0.9.2-rc.5`. No Edge, Profile, user-owned page, company website, original Stable AgentDock, credentials, or GitLab service was accessed as part of these tests.

## Implementation

`internal/tool/browser/external_fence_series.go` now requires a non-nil `externalPeerRevalidation` callback when constructing its private multi-operation transport series. A verifier is mandatory even for valid worker/peer/catalog fields; a missing verifier denies series construction. Each operation under the immutable `WorkerInfo` generation and opaque `externalPeerIdentity` performs:

1. **Before** opening each new one-shot WebSocket transport, re-observe the peer from an independently verified source. Missing/changed/unavailable evidence, a canceled context or verifier panic permanently fail-close the series before factory/transport dispatch.
2. Open a fresh one-shot pinned operation, perform the single fenced action, then close/join its owned transport. Never reset the previous `pinnedCDPRelay.operationSpent` gate.
3. **After** the transport has been fully joined, revalidate against the same externally observed peer and only then release the result. Peer change, verifier failure/panic or cancellation quarantines the response, keeps series closed and prevents any new operation. No raw token, endpoint, process identity, panic or verifier error is returned.

The `externalPeerRevalidation` type is source-private. The **only qualified production candidate** is the existing `ExternalLeaseManager.verifyPeer` helper, which already bounds verification to two seconds and retains an in-flight slot until a noncooperative verifier exits. A direct echo of expected peer/pointer would be **unsafe** and remains TEST-ONLY; an actual verifier still needs independent OS/CDP socket/Edge Profile attestation. Merely requiring the callback does not create trust. A future production backend also needs to prove atomically that every freshly opened socket belongs to the same verified peer, and must clean only NEXT-owned resources.

## Verified source-only behavior

- Existing one-shot, concurrency and multi-operation tests still pass; each fresh operation stays serialized through factory / call / cleanup.
- `TestExternalFenceSeriesRequiresFreshPeerRevalidation` exercises pre/post verifier error, panic, peer-incarnation drift and cancellation. Precheck rejection sends **zero** operations; postcheck rejection ensures the owned connector was closed and the possibly sensitive result never escapes; subsequent calls remain rejected.
- `TestExternalFenceSeriesPeerRevalidatedBeforeAndAfterEveryAction` checks the exact `verify → open → call → close → verify` order for three consecutive independent transports.
- `TestExternalFenceSeriesUsesBoundedLeaseVerifier` integrates the real, bounded `ExternalLeaseManager.verifyPeer` control with an independent **synthetic** witness: two operations require four verifications, no verifier prevents socket creation. This is not live authentication.
- `TestExternalFenceSeriesBoundedNoncooperativeVerifierFailClosed` confirms a verifier that ignores cancellation cannot hold the caller beyond the caller's deadline or start a transport; its single in-flight source remains accounted for and the series latches closed. The fixture worker is released at the end of the test.
- Run full Go `./internal/tool/browser ./internal/browserpolicy ./internal/config ./internal/app`, the adversarial repeat group and `go test -race ./internal/tool/browser -run '^TestExternalFenceSeries'` before integration.

## Remaining true production gates

1. Implement a trusted, independently verified **running Edge OS process + exact authenticated Profile + pinned CDP WebSocket session** identity provider (not a process PID or TCP listener alone); preserve macOS privacy/access controls.
2. Build an **owner-bound** factory for each fresh operation whose underlying transport has an atomic peer fence, combined with the bounded verifier and immutable same-lease identity; this source slice alone does not prove safe actual WebSocket transport.
3. Wire a verified `ConnectorStatusProvider` into NEXT Runtime and a real `ExternalLeaseBackend` into Browser Broker page ownership, acquisition, TTL/cleanup, and cancellation. Current production planner still uses **nil** provider and stays fail-closed.
4. After distinct owner authorization and a new NEXT-only deployment, test one scoped Oprueba/HomiPlay **read-only** UAT in the already-running Edge; GitLab API/Token remains first-choice for MR/Pipeline/Repository. No general `*.domain` wildcard, no Managed Chrome fallback for company-required Edge.

This step extends [multi-operation transport isolation](next-edge-multi-operation-fence-2026-10-23.md) and the [company site policy](next-company-browser-uat-scope-2026-10-22.md) without implying real Host UAT has passed.
