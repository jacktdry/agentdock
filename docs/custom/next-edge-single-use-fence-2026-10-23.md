# NEXT external Edge — one-operation fenced transport interface (2026-10-23)

**Status: SOURCE-ONLY transport foundation. Native NEXT company Edge integration is NOT enabled.** This changes no installed NEXT, original Stable, browser Profile, WebSocket endpoint, Company Site UAT page, GitLab OAuth, credentials or LaunchAgent config.

## What changed

- The private `pinnedExternalConnector` now explicitly implements the existing `externalTransportFence.CallExternalFenced` method and delegates to the already reviewed `callBound` socket/worker gate. A private compile-time interface assertion prevents a future mismatch. It is **not** an `ExternalLeaseBackend` or a `ConnectorStatusProvider`, and no production Runtime caller is added. The interface is package-private; this is not a new MCP model action.
- `TestPinnedExternalFenceInterfaceSinglePermit` exercises one real **synthetic loopback WebSocket** operation passing through the interface, then proves three attempts to reuse its permit are rejected, no additional CDP frames reach the upstream peer, and **only the owned test connector** is stopped. `TestPinnedExternalFenceInterfaceRejectsForeignGenerationBeforeDispatch` checks worker generation mismatch, external peer mismatch, unsupported tool and canceled context: no operation is dispatched or raw data returned.
- The signed Edge process preflight and pinned relay source remain independent; no attestor or credential trust can be inferred from a TCP socket, existing Edge window, past Stable MCP success, or a JSON route catalog.

## Critical multi-operation blocker

`pinnedCDPRelay.operationSpent` is a **one-shot, irreversible reservation** for its lifetime. It intentionally disallows a second `Session.Call` and prevents buffered/late CDP frames from a previous action from being forwarded under another permit. However, the actual `ExternalLeaseManager.Acquire` sequence requires at least: initial `list_pages`, fresh peer verification, `new_page`, page catalog/ownership verification, then scoped page operations and cleanup. Reusing a single one-shot relay would fail after the first operation. **Do not reset `operationSpent` or remove its gate** to make a demo appear to work.

A reviewed real connector lifecycle must provide **a fresh independently attested and owner-bound transport fence per operation** (or an equivalently demonstrated atomic isolation mechanism), scoped to the same immutable Edge process/Profile and retained browser lease. It must prevent cross-session frames, prove page ownership before an action or close, react to process incarnation changes, preserve the external Edge application, and stop only the NEXT-owned connector. The following remain unimplemented and MUST NOT be claimed as complete:

1. A production Edge/Profile authentication and WebSocket peer identity attestor; `Runtime` still calls `NewRoutePlanner(..., nil)` for external connector status, so policy remains fail-closed.
2. A production `ExternalLeaseBackend` satisfying multi-action Browser Broker semantics with a real independently verified peer, safe per-operation transport, target ownership, cancellation and cleanup.
3. Runtime wiring of authorized Oprueba/HomiPlay root/origin policies through the NEXT-only owner-private route config, and native Host UAT using company Edge.
4. NEXT's own GitLab token/OAuth registration; source/MR/pipeline checks should prefer the API rather than UI browsing.

The existing `edge-devtools-profile-ws` in original AgentDock is demonstrably useful and should not be duplicated by a second uncontrolled Edge process. But it cannot be imported into NEXT native Browser Broker as authenticated proof without independent owner/lease validation. This file complements [company Browser UAT scope](next-company-browser-uat-scope-2026-10-22.md).

## Test commands

```shell
GOMAXPROCS=2 GOFLAGS=-p=2 GOPROXY=off GOSUMDB=off go test ./internal/tool/browser -run '^TestPinnedExternalFenceInterface' -count=1
GOMAXPROCS=2 GOFLAGS=-p=2 GOPROXY=off GOSUMDB=off go test ./internal/tool/browser ./internal/browserpolicy ./internal/config ./internal/app -count=1
GOMAXPROCS=2 GOFLAGS=-p=2 GOPROXY=off GOSUMDB=off go test -race ./internal/tool/browser -run '^TestPinnedExternalFenceInterface' -count=1
```

Do not enable real Browser routing as a side effect of running these offline unit tests.
