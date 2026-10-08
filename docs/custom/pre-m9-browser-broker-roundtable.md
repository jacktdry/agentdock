# Pre-M9 P8 Browser Broker — frozen Phase A contract

Checkpoint: 2026-10-08, source development from `32822bcd` on
`feature/m8-permission-approval`. This contract narrows the future P8 inventory
in [feature parity §P8](pre-m9-feature-parity.md#p8--browser-broker-shared-ui).
Phase A establishes passive Core-to-Desktop observation only. Phase B adds the
read-only Shared UI described below; connector health, route overrides and
recovery controls are not implemented.

## UX and security decisions

- Default workspace routing remains managed isolated Chrome. Company workspace
  routing requires the configured authenticated external Edge. Existing trusted
  canonical-root policy remains the authority; display data cannot select a
  workspace, connector, profile or route, and cannot bypass company Edge.
- The configured company-policy count is **route intent**, never evidence of
  connector reachability, authentication, browser availability or successful
  attachment. Phase A does not expose a health label or probe the connector.
- Broker availability, retained leases and stale diagnostic state are separate
  facts. A ready worker is a retained broker state, not a fresh process probe.
  No browser page, URL, context, owner/session identifier, path, PID, endpoint,
  credential, capability token, browsing payload or raw error is shown.
- External processes and profiles remain user-owned. No launch, close, kill,
  focus, resize, cleanup, recovery, policy editing or override is added. Any
  future mutation needs its own ownership and permission contract.

## Architecture and authority

`BrowserService.Snapshot(context)` → signed Next launchd Core selection →
process binary / instance verification → verified private Unix socket
`desktopcontrol.CallVerifiedPID` (`LOCAL_PEERPID` + `LOCAL_PEERCRED`) → Core
`browser.snapshot` control method → optional `RuntimeBrowserDesktop()` → existing
`ACPBridge.Diagnostics()` → allowlisted `browserdesktop.Snapshot`.

**Desktop never reads or sends a Core bearer token over TCP for Browser
observations.** The actual connected Unix socket peer must match the selected
Next Core PID and user, and its executable/process instance is reverified after
the request to fence PID reuse. The Core callback performs only a passive
snapshot; no external process is launched, focused, cleaned or killed. Missing
Core, mismatched peer, or untrusted/aliased runtime root fail closed. Windows
and Linux do not yet have equivalent verified Next peer selection and fail closed.

An independent authenticated local `GET /internal/runtime/browser/desktop` Core
endpoint also exists for direct local API use. It requires direct loopback and
Core auth even if other Core endpoints allow unauthenticated access; GET only.
**This TCP API is not the Desktop transport** and is not used by the Browser
service. A caller must authenticate its Core endpoint independently. The
optional Runtime interface preserves older implementations; unsupported Core
returns `BROWSER_DESKTOP_UNSUPPORTED`. No Core startup, restart, Nexus pairing,
installation or live browser operation occurs. Desktop returns fixed sanitized
errors and does not cache successful snapshots.

`DomainBrowser` v1 advertises only read access to `snapshot`. This is service
capability metadata, not proof that the selected Core or broker is available.
Wails registers the typed service; generated bindings are the only frontend
changes in Phase A. Phase A made no Shared UI screen or navigation change; Phase B adds these below.

## Snapshot meanings

`observedAt` is the UTC timestamp of a passive Core diagnostic read, not a last
successful connector probe. Diagnostics take separate locks and are not an atomic
transaction across all broker components. The Desktop service does not cache a
successful snapshot; unreachable Core returns an unavailable result with an empty
timestamp and zero counts. Consumers must use the error/availability and must not
present those zero counts as a confirmed idle broker.

| Field | Meaning |
| --- | --- |
| `availability` | `core_unavailable`, `browser_disabled`, `acp_disabled`, `broker_unavailable`, or `available` (bridge exists). Browser-disabled takes precedence when both features are disabled; both flags are also returned. |
| `state` | `unavailable`, `idle` (no retained leases), `leases_present`, or `stale`. None implies live browser health. |
| `browserEnabled`, `acpEnabled` | Loaded Core configuration flags, independent of observed broker state. |
| `companyRequiredEdgePolicies` | Count of configured company required-external policies; configuration intent only. |
| `owners`, `leases`, `workers` | Retained diagnostic record counts. Owners may have no leases. |
| `activeLeases` | Pending-cleanup, owned, unexpired diagnostic leases; no page or connector liveness claim. |
| `expiredLeases`, `releasingLeases`, `failedLeases`, `unownedLeases` | Aggregate state counts; categories may overlap. Expiration uses `expiresAt <= observedAt`. No sweep occurs. |
| `readyWorkers`, `failedWorkers` | Retained worker state counts, not active process observations. |
| `activeOperations`, `queuedOperations`, `maxConcurrency`, `queueCapacity` | Existing broker admission queue counts and limits; active operations differ from active leases. |
| `managedOrphans`, `externalOrphans`, `lifecycleError` | Existing orphan counts and presence of lifecycle error; no raw reason/error or cleanup action. |
| `stale` | Expired/unowned/failed leases, failed workers, orphans or recorded lifecycle error exist. It does not measure connector health or snapshot age. |

Desktop accepts only fixed availability/state values, parseable timestamps and
nonnegative counts. Typed decoding discards unknown keys before renderer output.
The source projection is separately tested against an exact JSON key allowlist.

## Scoped acceptance and remaining work

Source tests cover empty/disabled Core, genuine in-memory broker observations with
fixture workers, unchanged diagnostics/no worker calls on snapshot, expired/error
projection, company route intent, JSON privacy, local authentication and GET-only HTTP routing, optional-capability
compatibility, untrusted Next identity, Unix peer PID/UID verification (reusing
the existing control transport), stopped/missing Core and sanitized peer errors. Fixtures do
not launch Edge/Chrome, call Nexus or perform external network requests.

Validation checkpoint (2026-10-08): macOS focused Browser Core/Desktop/API
Go tests, Wails `TestBrowserServiceRegisteredReadOnly`, Go vet for the affected
Core/Desktop packages, Vue `npm run typecheck`, and Windows
`go build ./internal/desktopapi ./internal/desktopruntime` all passed. Wails
beta.27 bindings were regenerated. One non-blocking macOS linker deployment-
target warning occurred during the Shared test. Initial independent review
identified credential-to-unverified-TCP risks; the Desktop transport was changed
to existing verified Unix control and no longer handles Core bearer credentials.
Whole-repository test suites were not completed; native GUI or installed Core
UAT and live connector tests are not asserted. At the Phase A checkpoint, P6 GUI, P7 GUI, P8 UI and M9 remained
**pending**. The P8 UI must retain
the distinctions above, show observation time, avoid stale-as-live labels and add
neither company bypass nor external-process cleanup. Live Nexus UAT remains
deferred under the existing user direction and does not block this source phase.

## Phase B offline Shared UI source checkpoint (2026-10-08)

From clean `eafc7b5f`, the Browser primary navigation section, read-only
`BrowserPanel` / `BrowserSnapshotDetails`, and Pinia browser store are implemented.
Snapshot is called only on mount or manual refresh after negotiated
`DomainBrowser.snapshot` capability. Failed negotiation, transport errors and
Core unavailability clear previous observations and counters; no zero-count idle
claim is made. Errors render fixed i18n labels only. No polling or mutations exist.

The semantic headings and definition lists distinguish bridge availability,
disabled features, idle/retained/stale state, UTC observation time, retained record
counts, active leases, queue admission and overlapping anomalies. Company Edge
counts describe configured authenticated route intent only. Responsive styling
supports narrow screens; native keyboard/GUI verification has not been performed.
English, Traditional Chinese and Simplified Chinese YAML sources were added and
catalogs generated with `go run ./tools/i18n generate` (11 artifacts).

Independent Gemini UI/security review: **PASS**, with no BLOCKER/HIGH after
hiding counter grids for disabled/unavailable broker states. A stable live-region
announces loading/status; optional future work includes canceling an in-flight
read when leaving the page and extracting shared status-grid markup.

Offline validation: focused Vitest browser store and i18n suites **15/15 passed**
(2 files); `npm run typecheck` **passed**. `go run ./tools/i18n check` **failed**
on existing hard-coded text in Permission/ACP/Plugin/Connection source/tests;
no Browser additions remain in its findings. Generated artifact comparison passed
before the hard-coded-text scan. No broad builds or whole-repository Go tests ran.

This is built UI source, not native acceptance: native GUI / installed Next UAT
and live browser connector checks are **not done**. P6/P7 native GUI remains
pending, live Nexus remains deferred, and M9 gates remain unchanged. Stable
AgentDock, Core 8765 and stable state were untouched. No Next restart,
installation, deployment, network or live Nexus/browser/CDP call occurred.

## Phase C1 offline Connector/Workspace route source checkpoint (2026-10-08)

Bounded source development from `f45be4a8` on `feature/m8-permission-approval`
extends the existing passive Snapshot and Browser panel, with no new endpoint,
service, capability, mutation, polling or direct connector call.

| Added field | Meaning |
| --- | --- |
| `configuredConnectors` | Number of loaded connector definitions, including unused definitions. |
| `configuredAuthenticatedEdgeProfiles` | Number of configured Edge profiles with authenticated-external class, including profiles without a connector; never a verified login count. |
| `configuredRequiredExternalPolicies` | Required-external policies across **all** workspace classes, including default and company. Existing `companyRequiredEdgePolicies` remains the company subset. |
| `connectorHealth` | Fixed `not_observed`; the UI says **NOT CHECKED**. No live health probe occurs. |
| `managedLeases`, `requiredExternalLeases`, `explicitExternalLeases` | Retained lease records by diagnostic route, including expired/releasing/failed records. Their sum equals `leases` when available; no reachability or successful attachment claim. |

Counts use only trusted loaded Core configuration and existing Broker.Diagnostics.
Configuration roots/endpoints are not re-read or canonicalized on snapshot. A
verified Core snapshot can show configuration intent when Browser/ACP is disabled
or the broker is unavailable. Retained counts are zero and hidden in those states;
missing/unverified Core clears observations entirely. Unknown/empty lease routes
or negative diagnostic queue counts hide the entire broker projection as
`broker_unavailable`, preserving configuration intent without misclassifying an
external route as managed. Desktop rejects negative counts, unknown health values,
route-count mismatches (including overflow), inconsistent retained state/counts,
and any unavailable snapshot carrying retained counters. Anomaly categories may
still overlap. Older Core snapshots missing the required health/route aggregates
fail closed as an invalid response; source validation does not update installed Core. Errors remain fixed and sanitized.

`BrowserRouteObservations.vue` receives the typed Snapshot as a read-only prop.
The existing Pinia store remains the only fetch authority (mount/manual refresh
and negotiated snapshot capability). Flat en/zh-Hant/zh-Hans YAML labels separate
configuration intent, retained records and absent live health. Exact JSON-key
allowlist and canary tests exclude workspace paths, connector/profile/owner IDs,
PIDs, URLs/endpoints, credentials, raw errors and browsing/personal information.

Production `internal/app/runtime.go` still passes **nil** to NewRoutePlanner's
status provider. `RoutePlanner.Resolve` therefore rejects required external Edge
when runtime verification is absent; there is **no company fallback to managed
Chrome**. Configuration, retained leases and ready workers cannot substitute for
verified reachability/authentication.

### Remaining C2b secure live provider and native acceptance gates

C2b must establish a reviewed secure implementation of the existing
ConnectorStatusProvider contract before exposing live health: bind observations
to the trusted catalog connector/profile and verified runtime identity, define
bounded freshness/timeouts and sanitized errors, and verify authentication,
engine/transport compatibility and required background/no-focus/lease-target/safe-
release capabilities. Missing, stale, mismatched or unverifiable observations
must fail closed with no required-route downgrade. A future renderer projection
requires a separate reviewed allowlist; no identities/endpoints/credentials or
raw connector errors may cross it. C1 adds no provider or health probe.

Native keyboard/accessibility/responsive GUI checks and installed signed Next
Core peer verification UAT remain open. Live managed browser and user-owned
external Edge behavior, authentication and ownership-safe release require later
explicitly scoped acceptance. No native GUI or live browser acceptance is claimed;
Nexus work remains deferred. Stable App/ports/services/installers/deployment,
routing policy and user external Edge remain untouched. This offline source
checkpoint does not close P8 or M9; the orchestrator owns review/integration.

Offline validation: focused `go test` for app/desktopapi/desktopruntime/httpx
Browser snapshots and browserpolicy/tool/browser route/catalog/policy/diagnostics
passed; browserdesktop has no standalone tests (its projection is exercised by
app/Desktop tests). Affected app/desktopapi/desktopruntime/browserdesktop `go vet`
passed. Exact JSON-key privacy/canary, configured and retained-route, unknown-route,
negative/overflow/inconsistent count and health-enum tests passed. Focused Vitest
store/component/i18n tests passed **22/22** (3 files), including three-locale SSR
presentation and unavailable-broker counter hiding; these are not native GUI tests.
Wails beta.27 regenerated typed bindings (**75 methods / 116 models**, no warnings)
and `go run ./tools/i18n generate` generated 11 deterministic artifacts. Final
`npm run typecheck` passed; an earlier attempt used bindings before generation
completed and failed on missing fields, then passed after regeneration.
`npm run build` passed (190 modules); Vite reported a non-blocking >500 kB
minified chunk warning. npm also warned about the existing minimum-release-age
configuration, and Node warned that localStorage was unavailable in the test runtime.

Global `go run ./tools/i18n check` still fails on pre-existing hardcoded
Permission/ACP/Plugin/Connection text; no Browser source/test appears in findings.
Generated freshness validation passed before that scan. No unrelated text was
changed, no whole-repository suite or native/live UAT was run, and no commit,
installation, deployment, push or runtime service/browser operation occurred.

## Phase C2a offline runtime evidence envelope source checkpoint (2026-10-08)

Bounded source changes from `3d741d82` on `feature/m8-permission-approval`
extend ConnectorRuntimeStatus with provider-observed connector ID, profile ID,
exact canonical endpoint, UTC observation and UTC expiration timestamps.
Resolve requires exact agreement with the selected catalog connector/profile/
endpoint (including port), a nonzero observation no later than now and at most
five seconds old, and expiration strictly after now and observation with a TTL
at most five seconds. Missing, stale, future, expired or inconsistent evidence
returns sanitized routeUnavailable errors and no route decision. Existing health,
verification, authentication, engine/version/transport and capability gates remain.

Resolve uses a two-second derived context (or the earlier caller deadline) to
bound slot acquisition and result waiting independently of provider cooperation.
Each planner has one in-flight provider slot. A provider goroutine holds it until
the provider actually returns, and sends into a one-item buffered result channel
so an abandoned consumer cannot block cleanup. A timed-out caller fails closed;
later callers wait only until their deadlines and cannot spawn another provider
while that slot remains occupied. Caller/derived cancellation is rechecked after
acquiring the slot and after receiving a result, before evidence validation.

Provider goroutines recover panics, including nil-valued panics, publish a fixed
unavailable failure without panic values/stacks/endpoints, and release the slot.
Capabilities are copied immediately after provider return before publication;
producer and consumer do not share that mutable slice. The provider must not
mutate its backing array concurrently with return/copy: such a race cannot be
prevented or verified by this boundary and remains a C2b source requirement.

This hard-bounded caller response does not cancel an underlying network operation:
a stuck provider can retain one goroutine and prevent further external verification
on that planner indefinitely. The trusted internal planner is created once per
runtime; the bound is per planner, not global across arbitrarily created planners.
C2b needs a cancellation-safe authentic source and reviewed recovery lifecycle.
Plan adds no provider/browser/network IO;
the direct ResolveRoute offline fixture contract and managed routing are unchanged.
Company required Edge cannot downgrade to managed Chrome. BrowserDesktop DTO,
renderer allowlists, UI and compatibility records are unchanged.

Production `internal/app/runtime.go` still passes **nil** as status provider.
There is no live implementation, genuine authentication/profile/no-focus proof,
Edge CDP probe or installed Next GUI UAT. Edge remains **UNQUALIFIED** for the
authenticated default-profile/no-focus/per-lease-target workflow. Passing offline
mocks prove source checks only: matching identity, freshness, websocket reachability
and trusted configuration do not provide cryptographic or profile authentication
attestation. No browser launch, Nexus call, restart, install or deployment occurred.

C2b remains pending: review an authentic observation source and its trust boundary,
bind evidence to the actual connector/profile/runtime incarnation, establish genuine
login/profile and capability proof, and define revocation, replay resistance,
ownership-safe lifecycle and cancellation behavior. Any secure proof requiring an
external browser/profile lifecycle needs separately scoped implementation and UAT;
it must not be replaced by configured booleans or an always-healthy provider.
Renderer health exposure and native acceptance remain separately reviewed gates.

Offline validation passed using `go1.27.1 darwin/arm64` (module declares Go
1.26.5), with `GOCACHE=/private/tmp/agentdock-c2a-gocache` and `TMPDIR=/private/tmp`:

- `go test ./internal/tool/browser ./internal/browserpolicy -run 'Test(Planner|ResolveRoute|Catalog|BrokerErrorContract|CompatibilityBaseline)' -count=1 -timeout=30s`
- `go test -race ./internal/tool/browser -run 'TestPlanner' -count=1 -timeout=30s`
- `GODEBUG=panicnil=1 go test ./internal/tool/browser -run '^TestPlannerProviderPanicRecovery$' -count=1 -timeout=30s`
- `go vet ./internal/tool/browser ./internal/browserpolicy`
- `git diff --check`

Tests cover matching fresh offline Edge/explicit Chrome, identity/endpoint/port
mismatches, noncanonical endpoint, missing/non-UTC/stale/future/expired/ill-ordered
timestamps, excessive TTL, health/authentication/engine/version/transport/capability
rejection, sanitized provider errors, cancellation before/after provider call,
and healthy results returned after a short caller deadline. A non-cooperative
blocked provider test verifies prompt failure, no second provider call while
occupied, and slot recovery after actual return, with test blockers released.
Panic/error-canary and nil-panic tests verify fail-closed recovery and subsequent
fresh success; deterministic snapshot-copy tests verify capability slice isolation.
No whole-repository suite or GUI/browser/network acceptance ran. Codebase-memory refresh was
blocked by another active index operation; no store rebuild/overwrite occurred.
Orchestrator review/integration remains pending; this checkpoint does not close
P8/M9 or claim runtime qualification.
