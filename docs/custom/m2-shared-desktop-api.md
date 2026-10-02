# M2 — Shared Desktop API

> Branch: `architecture/shared-desktop-api`
>
> Date: 2026-10-03
>
> Status: completed

## Objective

Create a framework-neutral Desktop contract between the shared UI and AgentDock Core.

M2 does **not** move all product domains into the shared shell. It stabilizes the contract rules first so M3/M4/M5 can add UI and execution features without coupling Vue or Wails directly to scattered Core implementation types.

## Boundary

The control plane is:

```text
Vue / TypeScript
  ↓ generated Wails binding
internal/desktopapi
  ↓ thin adapter
existing AgentDock implementation packages
```

The high-rate data plane is:

```text
Core/source
  ↓ bounded ActivityBuffer
ActivityBatch v1
  ↓ Wails bounded Stream / TrySend
JSONStream
  ↓ runtime validation
bounded Vue history
```

Wails is a transport adapter. `internal/desktopapi` is the contract authority.

## Protocol version and negotiation

Desktop API protocol version starts at `1`.

`ContractService` exposes:

- `Manifest()`
- `Negotiate(request)`

The manifest declares:

- protocol version range;
- every known Desktop domain;
- per-domain version;
- availability: `available`, `experimental`, or `unavailable`;
- exposed operations and access level;
- whether a mutating operation requires confirmation;
- stream transport/backpressure constraints.

A client using an unsupported protocol version or unknown domain gets a structured compatibility/validation error rather than silently receiving partial data.

## Domain state after M2

| Domain | Status | M2 contract |
| --- | --- | --- |
| runtime | available | status + start/stop/restart |
| activity | experimental | Activity v1 bounded stream contract |
| connection | unavailable | capability scaffold only |
| acp | unavailable | capability scaffold only |
| browser | unavailable | capability scaffold only |
| permission | unavailable | capability scaffold only |
| mcp | unavailable | capability scaffold only |
| plugin | unavailable | capability scaffold only |
| update | unavailable | capability scaffold only |
| diagnostics | unavailable | capability scaffold only |

An unavailable domain is intentionally truthful. It must not expose a fake empty service that could be mistaken for a successful implementation.

## Structured errors

`APIError` defines:

- `code`
- `message`
- `category`
- `retryable`
- optional string details

Categories:

- validation
- unavailable
- operation
- timeout
- compatibility
- internal

Frontend-side transport/contract failures are normalized into the same shape.

## Runtime domain

`desktopapi.RuntimeService` wraps the existing `internal/desktopruntime` lifecycle path.

The shared contract exposes only:

```text
RuntimeStatus
├─ running
├─ healthy
├─ startupEnabled
└─ nexusConnected
```

The generated frontend bindings no longer expose `desktopruntime.ServiceStatus` or its implementation-specific JSON names.

Operations:

- `status` — read
- `start` — mutating, confirmation required
- `stop` — mutating, confirmation required
- `restart` — mutating, confirmation required

The UI does not reimplement launchd / Scheduled Task / Run-key / process semantics.

## Activity v1 contract

Activity is intentionally only an **experimental wire contract** in M2. Real execution/call integration remains M5.

Each event contains:

- schema version;
- source epoch;
- decimal-string sequence cursor;
- UTC timestamp;
- kind;
- source;
- optional JSON payload.

### Why sequence is a decimal string

Go internally uses an unsigned 64-bit monotonic counter.

The JSON wire contract serializes the cursor as a decimal string so a long-running source cannot lose precision in JavaScript after `Number.MAX_SAFE_INTEGER`.

A shared Go/TypeScript fixture explicitly tests cursors and cumulative counters above that boundary. All long-lived cumulative counters on the Activity wire contract are decimal strings for the same reason.

### Epoch and reconnect semantics

- one running source lifecycle has one epoch;
- transport reconnect does not reset the epoch or sequence;
- a new source/process lifecycle may use a new epoch;
- sequence gaps are valid and observable;
- the frontend tracks the last cursor and surfaces observed gaps;
- drop counters remain explicit rather than pretending lost events were replayed.

### End-to-end bounds

Source boundary:

- bounded `ActivityBuffer`;
- publish is non-blocking once the queue is full;
- source drops increment a cumulative counter;
- invalid/oversized payloads also become visible source drops;
- payload must be valid JSON;
- payload maximum: 256 KiB.

Framework transport:

- high-rate Activity never uses Wails `Event.Emit`;
- data uses Wails Stream / `StreamConn.TrySend`;
- a full or disconnected transport increments transport-drop accounting.

Frontend:

- incoming JSON is runtime-validated before entering state;
- batch epoch/cursor bounds are validated;
- event sequence must be strictly increasing inside a batch;
- timestamps must be RFC3339 UTC;
- recent history is capped at 200 items;
- raw Activity rows remain outside live-region announcements.

Large tool output must use future bounded output/continuation contracts instead of being embedded into Activity payloads.

## Capability/security review rule

Every exported Desktop operation must declare an access level:

- read
- mutating
- privileged

Mutating or privileged methods must be reviewed for:

- confirmation requirement;
- platform-specific privilege semantics;
- whether the operation belongs in the shared contract at all.

The M2 Runtime panel consumes negotiated operation metadata directly. Mutating actions fail closed until capability negotiation succeeds, and start/stop/restart use inline confirmation because the manifest declares `requiresConfirmation=true`.

The Wails generator exports public service methods, so adding an exported method is considered an API/security-surface change.

M2 final audit distinguishes:

- stable root Desktop API: 2 services / 4 methods
  - ContractService: Manifest, Negotiate
  - RuntimeService: Status, Action
- POC-only helpers: 2 services / 5 methods
  - SettingsService: Get, Save
  - ActivityProbeService: Status, Start, Stop

POC helpers are not production domain commitments.

## Type consistency

Control-plane models use generated Wails TypeScript bindings.

Activity is a Stream wire contract and therefore uses an explicit TypeScript runtime parser. Contract drift is guarded by:

- Go struct tests;
- a shared JSON fixture under `internal/desktopapi/testdata`;
- frontend Vitest loading the same fixture;
- decimal-string cursor verification above JS safe integer range.

## Verification

Passed in M2:

- `go test ./internal/desktopapi`
- `go vet ./internal/desktopapi`
- root Desktop API race test
- nested Wails POC Go tests/vet
- Activity probe race tests
- real read-only Runtime status integration
- Wails binding generation
- Vue TypeScript typecheck
- Vitest contract/history tests
- Vite production build
- frontend audit: no `desktopruntime` type exposure
- frontend audit: no high-rate Wails Event bus usage
- macOS arm64 app + ad-hoc codesign
- verified macOS DMG
- Windows ARM64 GUI cross-build
- Windows x64 GUI cross-build

The known macOS deployment-target linker warning remains a production-framework gate from M1.

## What M2 deliberately does not implement

M2 does not:

- create production Activity from real call/session execution;
- implement connection/ACP/browser/permission/MCP/plugin/update/diagnostics adapters;
- remove AppKit or WPF;
- add Windows installer/signing validation;
- add i18n;
- define the bounded output/continuation contract used by M5.

## Next

M3 establishes the shared i18n source/generator before M4 starts migrating user-facing business UI.
