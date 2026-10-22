# NEXT Edge multi-operation fence sequence — source-only acceptance (2026-10-23)

**State: source-only private foundation, NOT an installed multi-operation company Edge lease or authorization mechanism.** No production `ConnectorStatusProvider` or trusted OS/WebSocket attestor is wired; no live Edge/Profile or GitLab credentials were touched.

## What changed

`internal/tool/browser/external_fence_series.go` adds a private `externalFenceSeries` that implements `externalTransportFence` by obtaining a **fresh single-operation fenced transport** from an injected factory for each action. It permanently binds immutable `WorkerInfo` generation and the same opaque external peer identity. Its mutex serializes setup, operation dispatch, and teardown: a subsequent request never starts before its predecessor's owned connector `Close()` finishes.

- Every permitted call creates a new `externalFencedOperation`, uses that object's `CallExternalFenced` once, and calls its `Close` before returning. It **does not reset `pinnedCDPRelay.operationSpent`** or allow a queue from an earlier CDP action to flow under a new permit.
- Wrong worker generation, peer mismatch, missing/canceled context, failed factory, failed tool, panic, or failure/panic during connector cleanup permanently **latch** series closed. If cancellation occurs while `Close` is running, even an earlier successful result is quarantined. Factory/tool errors and panics are returned as a sanitized generic fence rejection, not raw WebSocket endpoints or credentials.
- The per-operation factory is currently **only injectable by source/tests**. To become production-ready, the factory must independently verify a fresh socket belongs to the same authenticated owner-designated Edge process/Profile on **every** action; its `Close` must join/terminate all owned upstream writes and connector resources before returning. This safety condition is stated but is **not** proven by test fixtures to hold for any real Edge connector.
- `externalFenceSeries` deliberately does not implement `ExternalLeaseBackend`: it has no `StartExternal/Stop` authority over an Edge process, no lease-level page ownership grant, and no production Runtime hookup. It can be a component of a future reviewed backend but cannot, by itself, authorize browser operations.

## Verified offline scenarios

- `TestExternalFenceSeriesFreshTransportPerOperation`: three sequential operations with non-reused open/call/close order; closing series prevents any reentry.
- `TestExternalFenceSeriesSerializedOverlappingOperations`: concurrently requested operations do not overlap their transport lifetimes; no second opener executes until the first owned transport is joined.
- `TestExternalFenceSeriesFailureLatchAndNoRawLeak`: malformed owner/peer, nil/canceled context, failing or panicking factory, call errors/panics, cleanup failures/panics, canceled-after-call, and cancellation during cleanup all permanently reject and sanitize outputs.
- `TestExternalFenceSeriesConstructorRejectsNoIdentity`: no connector may open without a ready immutable worker, independent opaque source/incarnation identity, and an injected operator.
- Existing native Browser/Config/App and pinned CDP single-use gate tests are also used in regression. All interactions are synthetic Go objects or loopback fixtures and never inspect user browser state.

## Still required before NEXT company Edge UAT

1. Independently verified macOS Edge running executable/profile and authenticated CDP socket attestation, not just port 9222/list_pages or a route JSON file.
2. A production owner-bound factory whose **fresh transport per action** satisfies an atomic transport peer fence and whose cleanup is joined, timeout-bounded and auditable. Existing one-shot pinned connector's transport fence is not sufficient to implement full leases by itself.
3. Actual `ExternalLeaseBackend` integration with Browser Broker's page proof, task owner/session scoping, TTL/sweep and safe release. Never touch or close pre-existing user-owned Edge pages.
4. Valid NEXT-private UAT site policy for exact Oprueba/HomiPlay roots/origins and NEXT's independently authorized GitLab API connector (Token-first). Keep Chrome/Managed fallback forbidden for company-required Edge route.

The current installed NEXT remains `0.9.2-rc.5`; source work alone neither updates the App nor enables an external connector. User-selected P13/P14 card experiments remain deferred.
