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


## Phase C2b-A trust-source qualification gate source checkpoint (2026-10-08)

Source checkpoint on `feature/m8-permission-approval`, based on
`00114b93` (C2a `8589ae10`). This is the negative qualification boundary only;
**C2b is not complete**. Production `internal/app/runtime.go` still passes **nil**.
Edge stays **CompatibilityUnqualified**. No production attestor, positive qualified
provider, CLI/SSO login claim, admin bypass or environment toggle is implemented.

`RoutePlanner.Resolve` requires a package-private opaque qualification before
forwarding provider assertions to the pure trusted-metadata `ResolveRoute` helper.
The seal binds a detached observation to connector/profile/exact canonical endpoint,
health/verification/authentication, engine/version/transport and capabilities. It
also binds distinct nonnil opaque qualification-source and runtime-incarnation
identities. Raw booleans, fresh matching configuration IDs, HTTP/CDP/WebSocket
reachability and serialized status cannot create this authority. Proof observation
and raw timestamps must agree; both independently pass UTC five-second freshness/TTL checks
prevent renewing expired proof by editing raw timestamps. Unsupported proof format,
missing/cross-identity proof and assertion upgrades fail closed with a fixed sanitized
error and zero decision, with no managed Chrome fallback.

The qualification pointer is a single-use nonce, atomically consumed before
forwarding; copies share consumption across planners. This prevents replay of the
same in-memory authority, not a cryptographic attestation of a browser. Its fields
and detached capability slice are immutable after minting except the atomic used
flag. The trusted package is the authority boundary, not a sandbox against malicious
code inside package browser. No production code constructs these private identities
or qualifications. `qualifyRuntimeStatusForTest` exists solely in `_test.go`, clearly
marked **TEST ONLY**, and fabricates observations for offline source contracts.
Such fixtures cannot establish authenticated Edge profile or process ownership.

Evidence remains private in Core; it is absent from Desktop/BrowserSnapshot and
route decisions. No auth URL, cookie or PID field is added. Snapshot/UI, pure
`ResolveRoute` contract fixtures, external worker lifecycle and managed Chrome's
provider-free path remain unchanged. AGY handoff is untouched; a separate session
owns AGY-ACP, with Next integration deferred until that ACP contract is stable.

### Remaining C2b-B and acceptance gates

A separately reviewed real attestor must independently verify actual process and
runtime incarnation, user profile/authentication and background/no-focus/per-lease-
target/safe-release capabilities, then mint immutable bounded evidence from that
source. Config/status/reachability or canned production fixtures must never mint it.
Source revocation, incarnation turnover, cancellation-safe observation/recovery and
**attach-time revalidation** must be designed and tested before enabling any provider;
a single-use planning nonce does not close the plan-to-attach TOCTOU gap.
Consent-based native Next UAT must prove authenticated Edge, no focus or window
mutation, target ownership and safe release while preserving user-owned resources.
Renderer health projection requires separate allowlist review. No live browser,
network/process probe, GUI UAT, broad build or deployment occurred during C2b-A source validation.
P8/M9 remain open and live Nexus stays deferred; orchestrator owns final acceptance.


Offline validation passed on `go1.27.1 darwin/arm64`, with
`GOCACHE=/private/tmp/agentdock-c2b-a-gocache` and `TMPDIR=/private/tmp`:

- `go test ./internal/tool/browser ./internal/browserpolicy -run 'Test(Planner|ResolveRoute|Catalog|BrokerErrorContract|CompatibilityBaseline)' -count=1 -timeout=30s`
- `go test -race ./internal/tool/browser -run 'TestPlanner' -count=1 -timeout=30s`
- `GODEBUG=panicnil=1 go test ./internal/tool/browser -run '^TestPlannerProviderPanicRecovery$' -count=1 -timeout=30s`
- `go vet ./internal/tool/browser ./internal/browserpolicy`
- `git diff --check`

Tests cover raw perfect Edge/explicit Chrome rejection, qualified synthetic offline
routes, missing/unsupported/cross-identity/source/incarnation proof, assertion upgrades,
expired/future/non-UTC proof, stale/future raw observations, copied-nonce replay across
planners and loss of authority after serialization. Existing cancellation, provider
panic, non-cooperative slot isolation and managed-provider-free regressions pass.
These results establish source contracts only. Codebase-memory refresh was blocked
by another active index operation; no rebuild or overwrite was attempted.

## Phase C2b-B1 offline attach/admission checkpoint (2026-10-08)

Bounded Next-only source change on `feature/m8-permission-approval`, base
`7b90f16a`. This extends C2b-A's planning boundary into lease admission; it
**does not complete C2b or qualify a real Edge peer**. Production runtime still
passes nil to NewRoutePlanner and the production ExternalLeaseManager constructor
still has no peer verifier. Edge remains CompatibilityUnqualified.

Only successful qualified external RoutePlanner.Resolve mints a package-private
externalRouteGrant. It binds the exact canonical RequestScope (workspace/root, all
owner IDs and provenance), route kind, every ResolvedStart field (including
profile/connector/endpoint, browser/engine/version, profile path/class, ownership
and lifecycle/focus flags), and the original distinct qualification source and
runtime incarnation. Expiry inherits the qualified observation's remaining TTL,
at most five seconds; admission never renews it. An atomic single-use flag is
shared by all decision copies and managers, and is consumed before StartExternal,
even if startup or verification subsequently fails. Grant fields are immutable
after minting except that flag. Missing, changed, expired and replayed grants fail
closed before connector start. Pure ResolveRoute remains a pure contract helper
and cannot mint admission. The historical C2b-A statement that evidence is absent
from route decisions is superseded only by this private Core handoff field.

The independent, source-based externalPeerVerifier contract receives the exact
opaque expected source/incarnation and a value copy of the immutable started
WorkerInfo. A future reviewed source must independently observe that worker's
actual peer and return its observed source/incarnation, including revocation and
turnover. Echoing expected identities, trusting planner booleans, connector
readiness, or checking WS/CDP reachability is not a production implementation.
WorkerRegistry does not implement attestation. No production verifier or enabling
wiring is added, including for a future status provider alone.

Acquire verifies immediately before list_pages, then freshly verifies again
after baseline and immediately before new_page. Unavailable verifier, error,
panic, identity mismatch, timeout, caller cancellation or expired grant rejects
admission with fixed safe errors, without returning verifier errors/panic values.
Each verification waits at most two seconds with a cancellable context. One
manager slot remains held until a noncooperative verifier actually exits, so
timeout cannot spawn unbounded verifier goroutines. Go cannot forcibly terminate
such a verifier; a real source still needs cancellation-safe lifecycle/recovery.
After a verification rejection no page close occurs: only the owned connector
Stop is attempted using existing cancellation-independent cleanup, with orphan
recovery retained on Stop failure. The production registry's Stop has its
existing 35-second bound; arbitrary backend implementations must honor cleanup
contracts. Existing owned-page proof/release behavior remains unchanged after
success, and no user-owned page is substituted or closed.

The grant and evidence identities have no exported/serialized fields. JSON
round-trip loses the grant and cannot admit a route. No renderer DTO, Snapshot,
UI, auth token, URL or PID projection is added. Existing internal resolved starts
and WorkerInfo remain Core data. Managed admission is unchanged; company Edge
failures never fall back to managed Chrome. All adapter sources, runtime.go,
AGY-ACP and AGY handoff documents are untouched.

TEST ONLY helpers in _test.go synthesize private grants and fake verifiers to
preserve offline external page/lease coverage. The planner-to-manager success
fixture checks the fake source's independently held identities and the same
WorkerInfo at both checks. These tests establish handoff and rejection behavior,
not process/profile/authentication evidence. The opt-in browser_integration
fixture uses explicitly synthetic authority too; it is not live attestation and
is not run in this task.

Remaining gates: a reviewed live process/profile/auth/capability attestor, source
revocation/turnover lifecycle, and consent-based Next native UAT for authenticated
Edge, no focus/window mutation, exact target ownership and safe release. Two fresh
checks reduce plan-to-attach/mid-acquire exposure but cannot atomically fence peer
turnover between the final check and the connector operation; that requires a real
source/transport fencing contract. Post-acquisition call/release re-attestation is
not introduced here. Package browser remains the trusted boundary; private
in-memory grants are not cryptographic protection against code inside it. No
live browser launch, Edge/Chrome/CDP network, installed Next GUI, Nexus, broad
build, push, deployment or commit is performed. Orchestrator owns review,
integration and acceptance; C2b/P8/M9 remain open.

Offline validation completed 2026-10-09 on go1.27.1 darwin/arm64 with
GOCACHE=/private/tmp/agentdock-c2bb1-gocache and TMPDIR=/private/tmp:

- go test ./internal/tool/browser ./internal/browserpolicy: PASS.
- go test -race ./internal/tool/browser ./internal/browserpolicy: PASS.
- go vet ./internal/tool/browser ./internal/browserpolicy: PASS.
- go test -tags browser_integration -run '^$' ./internal/tool/browser:
  compile-only PASS; no integration test ran or browser launched.
- git diff --check: PASS.

Initial sandbox test/race attempts failed because httptest could not bind a
loopback listener (operation not permitted). The same focused packages passed
after the execution environment allowed local fixture listeners; no real browser
or CDP peer was used. Coverage includes pure/serialized/forged admission, full
scope/start mismatch, expired grants, concurrent copied-nonce replay, missing or
failed/mismatched/panicking verifier, bounded timeout/noncooperative slot
isolation, cancellation before start and after baseline, revocation/expiry after
baseline, failed Stop recovery, synthetic qualified planner handoff with both
checks, existing external lease lifecycle, managed regressions and company Edge
no-fallback through ACPBridge. No full suite, broad build, native UAT or live
attestation is claimed. Codebase-memory refresh was blocked by another active
index operation; no rebuild/overwrite was attempted.

## Phase C2b-B2a offline lease peer lifecycle checkpoint (2026-10-09)

Next-only source change on `feature/m8-permission-approval`, base `a1f4e1fc`.
Successful external acquisition now saves the consumed grant's opaque expected
source/incarnation in private Core lease fields. It does not export or serialize
these identities. B1 admission still consumes a single-use grant with at most
five seconds of validity and checks freshness before/after each admission peer
verification. Active leases use the saved identity without requiring or renewing
that admission grant, so verified leases can live for minutes.

After existing owner/scope and operation validation, each active Call holds the
lease mutex and independently verifies the peer before verifyTarget's list_pages
and the action. Release (including SweepExpired) verifies before list_pages or
close_page. The shared verifier retains the two-second cancellation bound and
single manager slot until even a noncooperative verifier exits. Missing verifier,
changed source/incarnation, revocation, provider error/panic, timeout or canceled
verification fails closed with sanitized errors and permanently latches peerLost.
Matching observations later cannot resume the lease. A latched lease skips both
the verifier and all page operations during subsequent calls/cleanup.

Release under suspect identity stops only the AgentDock-owned connector. Successful
Stop marks CleanupComplete with explicit `owned page close unconfirmed`; no browser
page is closed. Failed Stop retains CleanupFailed and existing bounded recovery,
which retries only Stop and preserves the unconfirmed-page reason on recovery.
Successful verification and target proof still close only the owned page. Managed
leases, browser policies, runtime wiring, installed GUI, stable AgentDock,
antigravity-acp and AGY handoff are unchanged.

Offline fixtures cover repeated valid checks through Call/Release, admission
expiry after acquisition, changed source/incarnation, persistent rejection after
matching observations return, revocation/unavailability/missing verifier,
cancellation before/during verification, panic/timeout, ownership rejection before
verification, concurrent callers sharing the latch, safe Release/SweepExpired and
connector-only Stop recovery. Synthetic authority exists only in `_test.go`.

Production status provider and peer verifier remain **nil**; Edge is
**UNQUALIFIED**. A reviewed real process/profile/auth/capability attestor with safe
revocation/cancellation, atomic transport fencing between verification and each
operation, and consent-based Next native UAT remain required. This checkpoint
adds lifecycle revalidation, not atomic protection against turnover after the
check. No actual Edge/CDP/network/browser operation or installed GUI UAT was run;
C2b/P8/M9 remain open. Orchestrator owns integration and release acceptance.

Host-level offline validation on macOS/go1.27.1: `go test ./internal/tool/browser ./internal/browserpolicy -count=1 -timeout=100s`, targeted `go test -race ./internal/tool/browser -run 'Test(ExternalLeasePeer|ExternalLeaseReleaseAndSweep|ExternalAdmission|ExternalLeaseOperations|ExternalAcquire|Planner)' -count=1 -timeout=100s`, `go vet ./internal/tool/browser ./internal/browserpolicy`, and `git diff --check` all **PASS**. These tests use synthetic peer fixtures; no real external browser was accessed.


## Phase C2b-B2b post-operation peer consistency checkpoint (2026-10-09)

Next-only source change on `feature/m8-permission-approval`, based on
`943e546c`. C2b-B2a revalidates before each lease action or release.
B2b now independently revalidates **after** a browser action has completed,
**before** handing any response or operation error to the caller or renewing
the lease's idle TTL. If verification fails (changed source/incarnation,
revoked/unavailable verifier, panic, timeout, or cancellation), it returns
**nil response** plus a sanitized denial, permanently latches `peerLost`,
and does not refresh activity/expiry. Even a previously successful page
tool result containing sensitive fields is quarantined; a subsequent matching
peer cannot restore the compromised lease. Operations themselves may have
taken effect already, so this is **response quarantine, not rollback**.

External `Acquire` now performs a third independent peer check **after**
`new_page` and before publishing the lease. If the peer changed or the
verification context is canceled, it discards the provisional page identity,
**does not call `close_page`**, and stops only its own connector (with the
existing connector-only orphan recovery path). The same conservative rule
applies when the page had appeared to be uniquely agent-created: a changed
remote browser incarnation invalidates authority to close that page.
A healthy peer and a proven page retain prior success and safe cleanup
behavior for actual tool errors. Existing synthetic test fixtures have been
updated for the added check and canceled post-create cleanup.

Offline tests `external_peer_completion_test.go` cover source/incarnation
turnover, revocation, verifier failure, cancellation and combined tool error
during actions, denying stale successful responses and preserving the lease
latch. Post-`new_page` turnover prevents publishing leases or closing tabs;
connector Stop failure recovers using Stop only. These use **test-only fake
backends/peer observations**, not a real Edge session or CDP.

**Remaining security gate:** rechecking after operations does not establish
an atomic, transport-bound guarantee that the *actual side effect* occurred
only in the intended peer. Implementing a credible process/profile/auth and
capability attestor, source generation revocation, a fenced transport/operation
contract, and explicit-consent native Next UAT remain mandatory. No production
status provider/peer verifier has been enabled: `runtime.go` still supplies
`nil`, and Edge remains `CompatibilityUnqualified`. Managed isolated Chrome
and policy fallback rules are unchanged. Stable AgentDock and the separate
AGY-ACP repository were not touched; Nexus live UAT remains deferred.
No Edge/browser/CDP operation, installed App build, deployment or restart was
performed. C2b/P8/M9 remain open.

Mac-Dev host validation (Go on macOS): `go test ./internal/tool/browser ./internal/browserpolicy -count=1 -timeout=120s`, focused `go test -race ./internal/tool/browser -run 'Test(ExternalAdmission|ExternalCallQuarantines|ExternalLeasePeer|ExternalLeaseOperations|ExternalAcquire|Planner)' -count=1 -timeout=120s`, `go vet ./internal/tool/browser ./internal/browserpolicy`, and `git diff --check` all **PASS**. `go test -tags browser_integration -run '^$' ./internal/tool/browser` **compiled**, with zero real browser integration tests executed. These are offline mock/contract tests, not a real authenticated Edge, foreground-focus, process identity or transport-fencing UAT.

## Next-only installed build and registrar recovery evidence (2026-10-09)

Source HEAD `539c68ac` (includes C2b-A/B1/B2a/B2b) was built as ad-hoc
signed `AgentDock Next.app`, verified by package checks (8/8), installed
at `~/Applications/AgentDock Next.app` using the repository's Next-only
atomic Contents installer, and activated using the **bundle-local Next
`AgentDockServiceRegistrar reregister core`**. The core's installed SHA-256
matched the built source and its dedicated `127.0.0.1:8767/healthz` returned
HTTP 200 from new PID `44161`. The Next-only Tunnel was re-registered
(label `dev.dropabit.agentdock.next.tunnel`, PID `44594`), and the GUI
was quit/relaunched in the background (PID `44873`).

Important rollback lesson: a bare `launchctl kickstart -k` after an
ad-hoc-signed Contents swap failed with macOS Launch Constraint Violation
(`78: EX_CONFIG`); restoring old Contents alone did not refresh the
SMAppService signing identity. Re-registering **Next** Core via the
shipped Registrar restored it; after that, the verified new Contents and
proper re-registration succeeded. The previous Next Contents rollback
artifact remains in a private Next-specific backup. The safe repeatable
helper `packaging/macos/deploy-next-shared.py` and corresponding offline
tests were added; the helper itself has not yet been used for live native
UAT. See `docs/custom/agentdock-next-isolation.md`.

**This does not complete P8 GUI or real Edge acceptance.** Source route
provider remains nil, Edge remains unqualified, no real CDP/Edge/browser
operation or visual WebView inspection was performed, and native keyboard
UAT remains pending. Original AgentDock and AGY-ACP were unaffected;
Nexus live UAT remains deferred and M9 release signed/update gates remain
open.

## P8 Next deployment wrapper native checkpoint (2026-10-09)

The Next-only deployment wrapper `packaging/macos/deploy-next-shared.py`
now independently binds the live Core's **launchd PID + exact Next helper
binary + 8767 listening socket + /healthz**, before and after its bundled
Core Registrar re-registration. Fake PID/port failures reject before
install; no stable services are queried for mutation. Deployment
negative/identity tests **12/12** and existing signed Next bundle tests
**8/8** pass. The **wrapper success path** was executed in native Next:
new Next Core PID `75320`, Tunnel PID `75385`, GUI relaunched in
background as PID `75792`, Core HTTP 200 and installed source SHA-256
equal to verified arm64 package. Old Next-only Contents backup retained at
`~/Applications/.agentdock-next-replacement-5gla__hu/AgentDock Next.app/Contents`.
Stable Mac-Dev continued throughout.

This proves the Next **application process identity and service
deployment** only. It does **not** provide trusted Edge runtime/profile
incarnation, authenticated session, background/no-focus/safe-release
capability qualification or atomic fenced browser operations. Production
`RoutePlanner(..., nil)` and missing external peer verifier remain
unchanged; Edge is still **UNQUALIFIED**. No user-owned Edge tab was
touched, no Browser/Computer or Nexus live UAT was performed.
The wrapper's failure-injection rollback is still offline-fixture only.
See `docs/custom/agentdock-next-isolation.md` for full steps.

## Phase C2b-B2c macOS read-only Edge process evidence (2026-10-09)

A Next-only, package-private OS preflight now exists in `internal/tool/browser/external_process_preflight*.go`. This is **process evidence, NOT an authenticated ConnectorStatusProvider, peer attestor, browser transport, or route grant**. Production `NewRoutePlanner(..., nil)` and Edge `CompatibilityUnqualified` are unchanged.

The two-second read-only macOS `lsof`/`ps` source accepts only an exact canonical numeric-loopback browser websocket endpoint, explicitly configured absolute `--user-data-dir`, one listener PID, matching Edge executable command line and port, same-user process start fingerprint, and the **first OS-mapped executable image**. Listener ownership, process incarnation, mapped image and args must all match again at completion. Ambiguous, wildcard/nonloopback, multiple-PID, missing, stale, recycled or canceled evidence fails closed. Other platforms explicitly reject; no raw command arguments, profile data or PID enter renderer/snapshots. Unit tests use fixtures only: no live Edge, tab, CDP or login access occurred.

Deliberate limitations: currently recognizes only system `/Applications/Microsoft Edge.app` launched with explicit matching `--user-data-dir`; a default profile without that option remains unqualified. OS command lines and mapped-image paths do **not** prove genuine code signing, authenticated profile, cookie state, background/no-focus, safe release, or atomic CDP transport. No qualification or `externalRouteGrant` is minted. Genuine attestation, browser permission and consent-based native UAT are still required.

Validation: focused fixtures, Browser/Policy Go tests with five unrelated network-test cases excluded, targeted Race Detector, `go vet`, and Linux/Windows cross-build all **PASS**. **Unfiltered Browser suite is not green**: five existing `cdp_discovery_test.go` local HTTP/WebSocket `httptest` cases consistently timed out at their 0.5–1 second limits on this macOS host, even isolated with `-parallel=1`. Investigate that separate loopback test environment; no security or timeout checks were relaxed to disguise it. P8/C2b/M9 remain open, stable AgentDock and AGY-ACP implementation untouched.

**Additional macOS loopback diagnostic (read-only, same checkpoint):** subsequent host checks showed TCP connect to Next `127.0.0.1:8767` timing out even though the dedicated Next PID (`75320`) still owned the LISTEN socket; the original stable `127.0.0.1:8765` remained reachable. An independent temporary Python loopback HTTP listener also timed out on local connection. `lo0` was UP and macOS Application Firewall reported disabled; prior Next logs contained successful `/healthz` HTTP 200 entries up through 02:03 local. Therefore this checkpoint does **not** claim current Next HTTP readiness, and the five Go `httptest` failures cannot yet be attributed to either preflight code or the Next Core. Investigate the host's network extension/PF/loopback policy separately using the stable `mac-dev` control plane; do not disable system security or restart stable services as a workaround.

**2026-10-09 約 03:12 後續確認（不刪原始異常紀錄）：** stable `mac-dev` 重測 Next `127.0.0.1:8767/healthz` 已回 HTTP 200（`0.9.1`），完整 `go test ./internal/tool/browser ./internal/browserpolicy -count=1 -timeout=120s` **PASS**，之前五項 `httptest` 逾時不再重現。根因仍未釐清；不可當作已永久修復。P8 的真實 Edge attestor、atomic CDP fencing、native GUI UAT 仍待完成。

## Phase C2b-B2d macOS Edge on-disk signer prerequisite (2026-10-09)

Read-only `readEdgeProcessPreflight` now adds a **mandatory but insufficient**
Microsoft signer gate between its first and second OS process/listener identity
observations. On macOS the bounded `/usr/bin/codesign --verify --strict`
checks the fixed `/Applications/Microsoft Edge.app` bundle and separately its
executable with a certificate-bound requirement: Apple generic anchor, Microsoft
Team ID `UBF8T346G9`, bundle identifier `com.microsoft.edgemac`. The same
2-second parent context covers all OS observations and signer checks. We do not
use `--deep` in this hot path: nested-framework traversal took ~4 seconds on
the installed Edge version, while strict app + executable checks each took
~0.2 seconds; this does not certify nested helpers/frameworks. Failures,
timeouts, cancellation or intervening listener/PID/epoch/argv/mapped-image changes
return the same opaque unqualified result; no signer output or raw profile/process
evidence is exposed. The user-owned Edge profile, login state, tabs, CDP socket,
and stable AgentDock are not accessed or modified.

**Not live browser authentication or authorization:** on-disk code signatures
cannot authenticate the executable pages already running in memory, the exact
CDP peer, the selected user profile/login/cookies, no-focus lifecycle, or safe
release, and cannot defeat a post-check port swap. Production status provider
and lease verifier remain `nil`; `UNQUALIFIED` stays in force and no connector
qualification or route grant is minted. Actual authenticated runtime identity,
profile consent, atomic CDP transport binding and native GUI/live UAT remain
P8 release gates. The signed Edge build on this macOS host passed a separate
read-only signature check, but that is not live connector qualification.

## Phase C2b-B2e fail-closed Edge command-line grammar (2026-10-09)

`edgeCommandMatches` now parses the macOS `ps -o command=` text using a bounded,
conservative token grammar instead of matching a port regex anywhere within a
command and extracting the first `--user-data-dir` substring. It accepts only the
fixed system Edge executable plus a **single explicit** numeric CDP port and a
**single canonical absolute** user-data directory (both `--flag=value` and
`--flag value`); unambiguous whole-value quoting and escaped spaces are supported.
Duplicate or ambiguous flags, embedded/suffixed lookalikes, malformed quotes,
noncanonical directory text, control/non-ASCII whitespace, non-UTF-8 data, and
oversized process strings fail closed. Regression fixtures cover normal argument
shapes and spoofed/duplicated/hidden flags, and prove rejection occurs before
mapped-image and on-disk signing work. This parser applies only to the private
Edge OS preflight, not generic discovery of managed/other-platform browsers.

**Scope boundary:** `ps` text is not a kernel-provided, immutable argv vector;
even successfully parsed command text is not identity, authentication, signed
running-code proof, user profile consent, no-focus guarantee, safe release, or
atomic CDP transport. Production `ConnectorStatusProvider` and
`externalPeerVerifier` are still `nil`; Edge remains `UNQUALIFIED`, no route
grant is minted, and no live browser/CDP or installed GUI UAT was performed.


## Phase C2b-B2f macOS running-code signer prerequisite (2026-10-09)

Based on clean source HEAD `5b436f3d`, private process preflight now independently
requires Security.framework dynamic code validity for the first observed PID,
after on-disk signing and before all listener/PID/start/argv/mapped-image rechecks.
The `darwin && cgo` implementation uses `SecCodeCopyGuestWithAttributes(NULL,
{kSecGuestAttributePid: CFNumber(pid)}, ...)`, `SecRequirementCreateWithString`,
and `SecCodeCheckValidity` with the same Apple generic anchor, Microsoft Team ID
`UBF8T346G9` and identifier `com.microsoft.edgemac` requirement. It links Security
and CoreFoundation; `darwin && !cgo` rejects. Linux/Windows retain their rejecting
native preflight. Invalid PID, unavailable verifier, API failure, cancellation
and deadline expiry yield only opaque unqualified error and zero evidence.

All checks share the existing two-second context. Since native C calls cannot
be canceled, a process-wide one-slot verifier bounds outstanding native work:
the caller returns on its deadline, the slot stays occupied until C returns,
and further calls reject immediately. Each completed call releases its CF/C
allocations; a permanently stuck native call retains at most one worker and
its in-flight resources. Dynamic validity is a prerequisite, not an atomic
future process fence, CDP peer binding, profile/login proof, no-focus or safe
release guarantee. Runtime provider/peer verifier remain nil, Edge remains
**UNQUALIFIED**, and no grants are minted. No Edge discovery, CDP, tabs, profile,
service, install or deployment action occurred. Native negative fixtures query
only this Go test process, never the user's Edge. P8/C2b/M9 remain open.

## Phase C2b-B2g bounded profile-directory metadata prerequisite (2026-10-09)

Source-only, based on `23fa0dd1`: private `edgeProcessProbe.profileMetadata`
now brackets all process/signing evidence. macOS uses `os.Lstat` only on the
canonical absolute ancestor chain (max 32 components / 4096 path bytes), with
forward/reverse observations per check. Every component must be a real directory,
owned by root or the current uid, with nonzero device/inode and without group/other write permission; the leaf
must belong to the current uid. Symlinks, unsafe modes (including sticky writable
ancestors), missing/non-directory paths, observed owner/mode/device/inode changes,
errors and cancellation yield opaque unqualified error and zero evidence.
No directory enumeration or profile contents are opened/read/written.

This conservative policy rejects paths through sticky `/tmp` or symlink aliases.
Success and race tests use injected metadata; native negatives use only `t.TempDir`.
Observed replacements within a sweep or across signer/process evidence reject;
Lstat is not an atomic fence and cannot exclude unobserved ABA swaps. Filesystem
syscalls cannot be interrupted mid-call; context is checked before/after each.
Physical identity is only a prerequisite, not profile authentication or consent.
Provider/peer verifier remain nil, Edge UNQUALIFIED; no grants, actual Edge/CDP,
login/tab inspection, install/deployment, stable or AGY ACP changes occurred.

Validation: `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run
'Test(Edge|UniqueEdge|MacEdgeProfile)' -count=1 -timeout=120s` PASS;
sequential `CGO_ENABLED=0 go test -p 2 -c ./internal/tool/browser` cross-builds
for linux/amd64, windows/amd64 and darwin/arm64 PASS. Native focused tests include macOS cgo. Additional independent full Browser/BrowserPolicy suite, targeted Race Detector and go vet all PASS (bounded GOMAXPROCS=2); no native browser UAT was run.

## Phase C2b-B2h safe-release peer consistency (2026-10-09)

Next source-only, based on clean `d1b0656b`: release now independently verifies
the saved peer after target `list_pages`, immediately before `close_page`, and
again after close returns, including close errors. A rejected pre-close check
prevents close; a rejected post-close check quarantines apparent success and
raw close errors behind the opaque peer error. Peer loss remains permanently
latched. Successful connector Stop still means `CleanupComplete`, with
`owned page close unconfirmed` whenever close identity cannot be verified.
Stop failure recovery retries only the AgentDock-owned connector, never a page
operation; SweepExpired uses the same release path.

TEST ONLY fake operation hooks cover source/incarnation changes, revocation,
verifier failure, cancellation after close, error precedence, exact check counts,
connector-only Stop/recovery and no repeated close. Managed lease tests are
included in focused validation. This is TOCTOU mitigation and response quarantine,
not an atomic transport/CDP fence: close may already have taken effect and a
swap between observations remains possible. Production provider/verifier remain
nil, Edge remains UNQUALIFIED, and P8/M9 gates remain open. No live Edge/CDP,
native browser UAT, install, service restart, deployment or push occurred.

Validation: `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run
'Test(External|ManagedLease)' -count=1 -timeout=120s` PASS (4.740s); gofmt applied.
The initial sandbox invocation failed during setup because the existing Go cache
was not writable; the authorized rerun used that same cache, with no fresh GOCACHE.
Independent supervisor verification also passed: complete Browser/BrowserPolicy
suite, targeted release/admission `-race`, `go vet` and `git diff --check`
with `GOMAXPROCS=2` and serial execution. Cross-build and live native UAT
were not run for B2h.


## Phase C2b-B2i transport-fence admission contract (2026-10-09)

Next source-only, based on clean `44c30c28`: private required backend capability
`externalTransportFence.CallExternalFenced` now carries the immutable expected
`externalPeerIdentity` alongside the immutable worker at every manager dispatch:
Acquire `list_pages`/`new_page` (including proven-page failure cleanup), Call
`list_pages`/action, and Release `list_pages`/`close_page`. Its contract requires
actual socket/session peer pinning and atomic comparison at EACH dispatch inside
the owning connector transport. Echoing identity or wrapping legacy
`CallExternal` with pre/post checks is forbidden for production implementations.
Existing independent verification, response quarantine, worker generation and
connector-only recovery rules remain in place.

Acquire consumes the one-shot grant, then rejects a backend missing the capability
with sanitized `ErrRequiredRouteUnavailable` before connector Start or any browser
operation. There is no legacy fallback. `WorkerRegistry` deliberately does not
implement the capability; its direct StartExternal API is unchanged, while lease
admission through it stays blocked. TEST ONLY backends compare synthetic distinct
non-nil private identities saved at start and preserve release phase hooks.
Regressions cover missing-fence zero start/call, sanitized errors, grant replay,
zero/aliased/forged peer rejection and no manager legacy dispatch, including
Acquire failure cleanup.

**B2i is ONLY a fail-closed prerequisite. NO real bound transport is implemented,
no production external route grant exists, and no atomic guarantee is provided
by this change.** Production `NewRoutePlanner(..., nil)` and peer verifier stay
nil; Edge remains UNQUALIFIED and P8/M9 gates stay open. The build-tagged live
integration uses real WorkerRegistry and intentionally remains blocked; its
compile check must not be interpreted as live attach acceptance. No live
Edge/CDP/tab/profile, managed Chrome, stable/AGY, app runtime, deployment,
or push work is included.

Validation: gofmt applied; the single focused command
`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'Test(External|Planner)' -count=1 -timeout=120s`
PASS (4.974s). Serial `GOMAXPROCS=2 go test -p 2 -tags browser_integration -c
./internal/tool/browser` PASS; no integration test binary was executed. Its first
sandbox attempt failed at setup on existing Go-cache access; the approved rerun
used the same cache. `git diff --check` PASS.
Independent supervisor verification also PASS: full Browser/BrowserPolicy test suite,
focused External/Planner Race Detector, `go vet`, `git diff --check`, and
Linux/Windows amd64 browser test cross-build, run serially with `GOMAXPROCS=2`.
No native/live Edge UAT or CDP binding proof was performed.

## Phase C2b-B2j single-socket pinned CDP relay primitive (2026-10-09)

Next source-only, based on clean `6ff0f789` on `feature/m8-permission-approval`.
New private `external_pinned_cdp_relay.go` implements a real single-upstream
WebSocket and one-shot Core-owned loopback relay, using existing gorilla/websocket.
It has no production caller and does not implement `externalTransportFence` on
WorkerRegistry or enable ExternalLeaseManager admission.

Canonical upstream input must be `ws://127.0.0.1:<port>/devtools/browser/<opaque>`
or canonical IPv6 loopback (`::1`), passing BrowserPolicy validation and literal
IP checks. Aliases, other hosts/schemes, missing browser IDs, queries, fragments,
escaped/non-browser paths and invalid ports fail before dial. Missing injected
attestor or nil/aliased expected source/incarnation also fail before dial.
`websocket.Dialer.NetDialContext` captures the actual established TCP `net.Conn`;
the injected callback observes that socket before ANY upstream HTTP/WS handshake
bytes. Only its independently observed exact identity match permits handshake.
Proxy is nil; redirects are not followed. Failure closes the TCP socket, returns
nil relay and a fixed opaque error, discarding raw causes and endpoint/token data.

After successful upstream handshake, Core binds `127.0.0.1:0` and creates a
256-bit crypto-random bearer URL under `/devtools/browser/`. This private URL is
the future adapter capability, never a renderer/user browser endpoint. Exact
request URI/Host, absent Origin and WS upgrade are required. Wrong tokens,
malformed routes, plain HTTP probes and second clients are rejected without
redirects or token-bearing responses/logs. Only one authorized upgrade attempt
is permitted; failed upgrade is terminal. At most four simultaneous downstream
TCP connections (including probes/upgrades) are retained, with bounded HTTP
headers and two-second HTTP/handshake/write deadlines. Each WS message is capped
at 8MiB; the session has a five-minute absolute lifetime/read deadline.

Text/binary messages and ping/pong/close controls travel through gorilla WS in
both directions; message payload/type are preserved, not original fragmentation.
No reconnect/redial path exists. Either socket loss, forwarding failure, context
cancellation, timeout or Close permanently latches lost and closes both WS
sockets, listener and pending HTTP connections. Upstream application reads
begin only after the downstream attach: early
unsolicited CDP events remain in the established socket/TCP buffers and are
forwarded after attach (bounded by kernel TCP backpressure, no application queue).
Owned relay workers and HTTP handlers have a completion signal. A noncooperative
injected attestor may retain ONE process-wide worker/slot until it returns;
cancellation still closes its socket and rejects the caller, and further
verification workers are refused. A context deadline cannot forcibly stop an
arbitrary callback.

`withPinnedPeer` admits only the saved exact peer while this immutable local
transport has not latched loss, and supplies no socket/reconnect/swap capability
to its callback. Loss during callback quarantines completion. **This is NOT
atomic CDP command semantics:** remote loss is observable only when I/O detects
it, Close may race callback execution, and callback side effects cannot be
undone. Connecting this capability URL to chrome-devtools-mcp, immutable worker
binding and a new reviewed fencing backend remain pending. Initial OS listener
attribution is NOT guaranteed: injected fixtures must be replaced by an
independently reviewed established-socket owner attestor. Process/profile
preflight, a successful handshake and an identity fixture cannot qualify Edge.

Production ConnectorStatusProvider and externalPeerVerifier remain nil,
WorkerRegistry remains legacy, no status/grant is minted, Edge remains
UNQUALIFIED and P8/M9 gates remain open. No real Edge/CDP/tab/login/profile,
stable AgentDock, AGY .14, installation, deployment or push was touched.
Only the new relay/test and the two requested append-only checkpoint docs change.

Validation: gofmt and final
`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'TestPinnedCDP' -count=1 -timeout=90s`
PASS (0.534s), using the existing Go cache and LOCAL synthetic httptest/TCP
fixtures only. Tests cover echo/control, one downstream, malformed/wrong token
and HTTP rejection, invalid inputs/nil verifier/aliased peer with no TCP dial,
exact established socket addresses, mismatched/error/panicking verifier with
zero handshake data, cancellation (including before attach), bounded blocked
attestor, both-side irreversible loss/no redial, concurrent Close, both-side
message limits and repeated lifecycle worker bounds. First sandbox attempt
failed on loopback bind permission; approved reruns used the same cache. First
functional run found a leak-test fixture lifetime error (retained HTTP servers),
corrected to clean each child fixture before measurement; subsequent runs PASS.
Independent supervisor validation: `TestPinnedCDP` focused tests including
pre-attach unsolicited events, complete Browser/BrowserPolicy suite, focused
`-race`, `go vet`, Linux/Windows amd64 test cross-build, gofmt and
`git diff --check` all PASS (serial GOMAXPROCS=2). The first supervisor
`-race` run found a fixture WaitGroup Add/Wait cleanup race; guarded the
fixture terminal state under a mutex and reran PASS. No real/native Edge UAT
or authenticated socket/process ownership test was executed. All synthetic
runtime resources are closed by test cleanup; no task server remains running.

## Phase C2b-B2k established TCP ownership evidence (2026-10-09)

Next-only bounded source implementation based on clean `bf77b6b6805af4f53e252746258bc7f252bf0fc4`
on `feature/m8-permission-approval`. New PRIVATE `readEdgeEstablishedSocketEvidence`
accepts an already-established concrete TCP socket and expected process preflight.
It rejects invalid/canceled inputs, preflight older than two seconds or in the
future, malformed PID/epoch/path/endpoint and mismatched/non-loopback TCP tuples.
It performs no socket I/O and does not inspect profile metadata or contents.

The native macOS probe runs only `/usr/sbin/lsof -nP -a -p <PID> -iTCP
-sTCP:ESTABLISHED -FpnT`, reuses the existing UID/startToken observation and
dynamic Security.framework signature check, and shares a fixed two-second
context. Two socket observations surround the first epoch/signature checks;
epoch/signature are checked again after the second socket observation. Each
observation requires exactly one server-to-client tuple reversing the observed
client socket. Output allocation is capped at 8192 bytes, records at 64 and
lines at 512 bytes; stderr is discarded. Parser requires one exact PID header,
distinct numeric file descriptors, one canonical numeric loopback tuple and
one `TST=ESTABLISHED` per record; only optional numeric TQR/TQS fields are
accepted. Wildcards, aliases, mapped/zoned/noncanonical IPv6, duplicate tuples,
missing records, malformed/unknown fields and wrong direction fail closed.
Any failure returns zero evidence and the fixed opaque process-unqualified
error. Other platforms reject; Darwin without cgo cannot pass live signature.

A read-only synthetic host experiment queried ONLY its own Python PID and
confirmed `p`, `f`, `n`, `TST=ESTABLISHED`, `TQR=0`, `TQS=0` output. The original
sandbox experiment/test attempts were denied loopback bind; authorized reruns
used the same existing Go cache. Focused tests use TEST ONLY `net.Listen`/dial,
fake lsof/signature/epoch probes and deterministic IPv4/IPv6 parser fixtures;
all sockets/listeners are registered for cleanup. No user's Edge was queried.

**This is non-atomic OS process evidence, NOT authenticated login, socket proof,
a complete trusted pinned relay attestor, or an external route grant.** PID
epochs have existing timestamp granularity, socket tuples may be reused between
observations, and loss after observation is possible. Concrete probes must
honor context; the helper cannot forcibly terminate arbitrary injected probes.
The existing native signature slot bounds uncancelable Security.framework work.
No externalPeerIdentity is minted, no externalTransportFence implementation is
added, and relay wiring/provider/verifier/WorkerRegistry remain unchanged.
Edge remains UNQUALIFIED; P8/M9 gates remain open. Real/live Edge
ownership and native GUI UAT remain untested.
No stable/AGY, push/install/deploy or persistent background server work occurred.

Validation: gofmt applied; final `GOMAXPROCS=2 go test -p 2
./internal/tool/browser -run TestEdgeEstablishedSocket -count=1 -timeout=90s`
PASS (2.487s). An earlier authorized focused run also passed (2.440s), before
adding explicit root/NUL/overlong-component preflight path rejection.
`git diff --check` and gofmt verification PASS. No test was skipped within
the focused selection; this does not establish native Edge ownership acceptance.

Independent supervisor verification: complete Browser/BrowserPolicy test suite,
`TestEdgeEstablishedSocket` focused Race Detector, go vet, gofmt/diff check,
and Linux/Windows amd64 and Darwin arm64/no-cgo browser test cross-build
all PASS (serial,
GOMAXPROCS=2). No live Edge socket or profile was queried.

## Phase C2b-B2l safe evidence/attestor composition (2026-10-09)

Source-only checkpoint based on clean `99a88b7a52a7dbf5c7a2a7bf9c7f57377a88ada2`
on `feature/m8-permission-approval`. New PRIVATE `newEdgeEvidencePinnedRelay`
requires an injected B2k probe, a distinct independent `pinnedCDPAttestor`, and
an expected private peer validated by B2j. Endpoint must exactly equal the
preflight endpoint, including browser path. Its adapter first requires
`readEdgeEstablishedSocketEvidence` on the exact concrete TCP socket supplied
by B2j's `NetDialContext`; only successful evidence with a still-live context
permits the independent attestor to observe that same socket. Evidence is
discarded, never converted into identity; neither expected identity nor evidence
is passed to the independent attestor. The independent observed peer must match
expected under B2j's existing comparison. Missing inputs, stale/malformed
preflight, socket/epoch/signature failures, second-stage mismatch/error/panic or
cancellation/timeout return nil relay and the fixed opaque B2j error; socket
closure occurs before any WS handshake bytes. B2j and externalTransportFence
are unchanged. No runtime caller, public API, provider, verifier or wiring added.

TEST ONLY local synthetic WebSocket/TCP fixtures count actual upstream read
bytes, not just upgrade requests. Constructor echo and instrumented composition
tests prove a single upstream dial, strict `ctscts` probe order before independent
attestation, exact client pointer reuse and reversed four-tuple derived from
that client. Because B2k's probe interface receives PID rather than conn, a test
decorator records the composition's client pointer; the constructor-only test
instead derives the matching tuple from the accepted server socket. The fake
probe owns its synthetic epoch independently. Negative cases cover missing and
typed-nil inputs, invalid peer, endpoint host/path differences, malformed or
different profile, stale/future preflight, wrong PID/tuple/epoch/signature at both
observations, cancellation, deadline, B2j timeout and probe/attestor panic.
First-stage failures never invoke the second attestor. Rejection observes zero
handshake bytes and server-side socket closure with no returned downstream relay;
success closes both relay sockets/listener and waits for relay/fixture workers.
Non-TCP, nil and typed-nil sockets fail the adapter directly.

**B2k remains non-atomic read-only process evidence. The independent second
attestor still has no production implementation; callback independence is a
required caller contract, not something this adapter can infer. Composition is
NOT OS-authenticated actual Edge, consent, login, protected-profile proof or
per-command CDP atomicity.** B2j's single attestation slot still bounds arbitrary
noncooperative callbacks; they cannot be forcibly terminated. Formal provider
and peer verifier remain nil, WorkerRegistry has no fence, Edge remains
UNQUALIFIED and P8/M9 gates remain open. No real Edge/browser/Chrome/AGY/stable
or Next installation, profile, native UAT, push or deployment touched.

Validation: gofmt applied. The sole targeted selection
`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'TestEdgeEvidencePinned' -count=1 -timeout=90s`
PASS (2.383s), no selected test skipped. Initial sandbox attempt failed at
loopback bind before functional testing; authorized rerun used the same existing
Go cache, with no fresh GOCACHE or concurrent testing. `git diff --check` PASS.
Independent supervisor verification also PASS: complete Browser/BrowserPolicy
suites, B2l focused `-race`, `go vet`, `git diff --check` and
Linux/Windows amd64 plus macOS arm64/no-cgo browser test cross-build, all
run serially with `GOMAXPROCS=2`. No native/live Edge UAT or authenticated
peer attestor exists.

## Phase C2b-B2m Connector to pinned relay immutable worker binding (2026-10-09)

Source-only checkpoint based on clean `184b172e4bf8d1433f8b7ab4ed6a5b0fce606a38`
on `feature/m8-permission-approval`. PRIVATE `pinnedExternalConnector` accepts
only an existing admitted relay, exact private expected peer, original external
RouteDecision validated by `validateExternalStart`, live context and open
WorkerRegistry. It validates the relay's canonical capability endpoint/path and
exact original upstream endpoint; no route or grant is changed or consumed.
`externalWorkerConfig` retains every launch option except `--wsEndpoint`, which
is replaced with the Core-owned one-shot loopback capability. The original
browser endpoint is never a connector argument. Relay reservation is permanent
and serialized before start, independently of downstream WS claim, so concurrent
constructors and replay cannot launch a second connector.

Two necessary existing-source adjustments: relay retains its admitted source
endpoint and binding reservation; WorkerRegistry's private start helper accepts
a pre-Open observation and immutable cleanup hook. Legacy `StartExternal`
behavior is unchanged. After the probe, observed loss/cancellation prevents Open;
after start, worker readiness, immutable WorkerInfo generation, live SessionInfo
and successful downstream attachment are checked against the exact relay peer.
Handshake failure returns a fixed opaque error, closes relay and best-effort
stops only the AgentDock connector. Future private `checkAlive` dispatches no
browser operation; observed relay loss or worker/session change permanently
invalidates the binding. Concurrent binding Stop is idempotent. Registry Stop
and Shutdown close the relay before connector cleanup, even on cleanup error.
No cleanup operation closes a user browser process, profile or page.

TEST ONLY fake WorkerDependencies.Probe/Open use the committed managed tool
schemas and synthetic SessionInfo. Local httptest WS fixtures exercise the real
relay: exact single pinned endpoint argument, unchanged other options, one
upstream dial, synthetic handshake/echo, immutable binding and live upstream
fixture listener after cleanup. Rejection coverage includes nil/canceled inputs,
invalid route/source endpoint/path/peer/registry, closed or claimed relay,
concurrent reservation/replay, probe/Open failures, missing attachment/session,
version/schema/SessionInfo mismatch, loss/cancellation during probe or handshake,
later worker/session/peer changes, concurrent Stop and Stop error cleanup.

The bearer capability appears in the child connector process argv; this is
NOT a secret against local same-user process inspection. The only defense at
this stage is loopback-only one-shot attachment plus the short relay lifetime;
production authorization must not rely on the endpoint being confidential.
The existing WorkerRegistry retains legacy direct CallExternal for callers
outside this private binding; this is not yet a browser dispatch security fence.

**This is immutable source-only worker binding, NOT atomic per-command fencing.**
`withPinnedPeer` still provides pre/post local observations; Close may race
execution and remote loss may require I/O to observe. No generic CallExternal
is exposed by this object, no `CallExternalFenced` implementation is added, and
WorkerRegistry still does not satisfy `externalTransportFence`. No production
caller/provider/peer verifier is enabled, no browser status/grant is minted,
and no independent production OS peer attestor exists. Edge stays UNQUALIFIED;
P8/M9 gates remain open. No live Edge/Chrome/discovery, stable/AGY repository,
Next installation/deploy/restart, real npx, push or native/live UAT.

Validation: gofmt applied. The sole focused selection
`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestPinnedConnector' -count=1 -timeout=100s`
PASS (0.591s), no selected tests skipped. Initial sandbox attempt failed at
existing Go-cache write before tests; authorized rerun used that same cache and
synthetic loopback fixtures. Independent supervisor validation also PASS: complete Browser/BrowserPolicy
Go tests, B2m targeted Race Detector, go vet, gofmt/diff checks, and
Linux/Windows amd64 plus macOS arm64 no-CGO browser test cross-build;
all run serially with `GOMAXPROCS=2`. No native/live Edge UAT was done.
`git diff --check` PASS. Test-local relay/WS/listener resources are cleaned up.

## Phase C2b-B2n-A CDP outbound frame permit gate (2026-10-09)

Next SOURCE-ONLY checkpoint based on clean `6b96e952` on
`feature/m8-permission-approval`. Standalone B2j relays retain their legacy
behavior. Only the private B2m binding reserves restricted bootstrap mode;
initialization frames may pass on the original admitted socket until validated
worker readiness and downstream attachment arm the CLOSED gate. Bootstrap
checks the constructor context and exact saved peer; constructor cancellation
closes the relay. There is no reconnect, socket replacement or second attachment.

PRIVATE `callBound(ctx, expected WorkerInfo, peer, tool, args)` is for synthetic
tests/future reviewed backend only. It rejects overlap without queueing, checks
the existing strict tool allowlist and exact immutable worker/peer, holds the
managed worker mutex, and directly invokes the captured session's Call. It never
uses general WorkerRegistry.CallExternal. A single permit is bounded by caller
context and a two-second operation deadline. The frame pump checks peer, loss,
permit identity/context and gate state under the same relay mutex as every
downstream-to-upstream data/control write. Writes have bounded deadlines;
socket/listener shutdown and worker cleanup happen outside the relay mutex.
The lock order is worker -> relay; the pump never acquires worker.mu. Stop closes
the relay before waiting for worker cleanup, preventing reverse lock acquisition.

Session.Call return closes the permit before post-call SessionInfo/generation
and reconnect checks; cancellation, loss, identity change, call errors or panic
quarantine output and stop only the AgentDock-owned connector. Idle unsolicited
or late downstream frames are dropped and permanently latch loss. A single
relay-lifetime watcher also stops the owned connector after idle transport loss.
Upstream-to-downstream async CDP events still pass while CLOSED. No cleanup
closes the external browser, its profile or its pages. Errors remain fixed opaque
values and do not include tool arguments, bearer URLs, tokens, PIDs or raw causes.

**LIMITATIONS — controlled frame dispatch only, NOT complete B2n atomic trust
or authorization.** Background outbound CDP during a permit cannot be attributed
semantically to its MCP tool. Frames processed after permit closure are rejected,
including commands emitted after the tool returns; initialization traffic is
allowed only in bootstrap. Already queued traffic may be processed during a
later permit: this is a temporal gate, not a command provenance check. Incoming
events that provoke connector-generated outbound replies while idle can
quarantine the connector (including downstream WS control replies). Cancellation
cannot retract bytes already handed to the socket; Close may wait for a bounded
write to release relay.mu. Remote loss may remain unobserved until I/O. The
worker mutex excludes managed generation mutations, but live SessionInfo remains
a pre/post observation, not an atomic per-frame OS identity check. A session
ignoring context cannot be forcibly interrupted: the transport window expires,
but the direct call and owned connector cleanup may wait for that session.
Local same-user inspection can still observe the connector's bearer argv.

WorkerRegistry/production backend do NOT implement `externalTransportFence`.
Planner provider and peer verifier remain nil; no runtime caller, public browser
API, status/grant mint, OS trust/authentication/login/consent claim or independent
production attestor was added. Edge remains UNQUALIFIED and P8/M9 gates stay open.
No real npx, Edge/browser/CDP/user screens, native/live UAT, stable/AGY changes,
install/deploy/restart or push. Tests use only local synthetic httptest
WS and fake WorkerDependencies. Native managed Chrome and standalone B2j source
paths remain unchanged; their broader regression execution belongs to supervisor.

Validation: gofmt and `git diff --check` PASS. Final focused selection
`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestPinnedCDPGate|TestPinnedConnector)' -count=1 -timeout=120s`
PASS (2.645s), no selected skips. The first sandbox attempt failed on the existing
Go cache before execution; authorized synthetic-loopback execution passed
(2.835s). One corrective rerun of the same selection followed immediate
post-Session.Call permit closure and queued-frame cancellation coverage; no other
test selections ran. Existing cache was reused. Independent supervisor verification
also PASS: complete Browser/BrowserPolicy tests, pinned B2n/B2m/B2j focused
Race Detector, go vet, gofmt/diff check, and Linux/Windows amd64 plus macOS
arm64 no-CGO browser test cross-build. First Race Detector pass exposed a
Gorilla WebSocket concurrent writer DATA RACE in the *synthetic server test
fixture*; a single writer mutex now serializes all synthetic upstream writes;
focused -race and supervisor independent rerun PASS. No native/live Edge UAT.
The production relay write gate was not weakened to work around this test issue.

## Phase C2b-B2n-B SINGLE-USE CDP operation epoch (2026-10-09)

Next SOURCE-ONLY change from clean `2da83b6f`, branch
`feature/m8-permission-approval`. This supersedes B2n-A's reusable temporal
permit: each private B2m relay/immutable Worker/Connector lifetime permits at
most ONE MCP operation. `openPermit` consumes an irreversible reservation
before validation and Session.Call; success/failure closes the gate permanently.
Close never resets the reservation; Stop, cancellation and panic leave it spent.
Subsequent calls fail opaque and close the owned relay before any new window
can open, then stop the owned connector. Overlap/reentry closes transport
synchronously and uses the existing lifetime watcher for connector cleanup,
avoiding a reentrant wait on the first operation's worker mutex. No queue/replay
can obtain a second permit. First-operation success alone does not close the
connection: idle upstream-to-downstream events remain allowed until relay loss
or a second operation quarantines the binding.

TEST ONLY httptest/fake-session coverage sends A's valid CDP command and
delivers A's result, then leaves another A command fragmented in the downstream
reader. B is attempted immediately; the final fragment would complete inside
B's permit under B2n-A. B/replays now never invoke Session.Call, close the
connector/relay and leave the upstream count at bootstrap + A only. Tests also
cover idle reverse-event delivery before the second-call quarantine, overlap,
reentry, pre-canceled/nil context, and retry after failure/cancellation/Stop/panic.
B2m bootstrap remains unchanged, including the synthetic plain-text handshake.
Standalone B2j's legacy gate remains unchanged; its broader regression tests
are outside this requested test selection and belong to the supervisor.

**LIMITATIONS:** ONE MCP operation per Connector is an intentional usability
regression and is unsuitable for real multi-step workflows. This removes a
later B window; it does NOT prove authenticated MCP-tool-to-CDP-method
provenance. Black-box chrome-devtools-mcp cannot annotate frames with that
proof. Initial bootstrap may emit CDP; background traffic during A may still
pass. No envelope parser or semantic authorization claim is introduced.
A future remedy requires a trustworthy CDP command dispatcher/provenance,
not a timer. Existing bounded-write, cancellation, remote-loss and
noncooperative-session limits still apply. OS trust/login/consent remain
unresolved. WorkerRegistry/production backend still do not implement
`externalTransportFence`; production provider/verifier remain nil and Edge
UNQUALIFIED. Default managed Chrome and browser lifecycles are unchanged.
No live browser, real npx, AGY/stable/Next restart, install, deploy or push.

Validation: gofmt completed. The ONE requested selection
`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestPinnedCDPGate|TestPinnedConnector)' -count=1 -timeout=120s`
PASS (2.553s), no selected skips. Initial sandbox execution failed at httptest
loopback bind; authorized rerun of the same selection passed. Independent
supervisor verification then PASS: full Browser/BrowserPolicy test suites,
B2n/B2m/B2j pinned focused Race Detector, Go vet, gofmt/diff check, and
Linux/Windows amd64 plus macOS arm64 no-CGO browser test cross-build, all
run serially with `GOMAXPROCS=2`. No native/live Edge UAT. Synthetic fixture
resources are cleaned up by test cleanup; no temporary runtime remains.
