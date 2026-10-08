# Pre-M9 P8 Browser Broker — frozen Phase A contract

Checkpoint: 2026-10-08, source development from `32822bcd` on
`feature/m8-permission-approval`. This contract narrows the future P8 inventory
in [feature parity §P8](pre-m9-feature-parity.md#p8--browser-broker-shared-ui).
Phase A establishes passive Core-to-Desktop observation only. Shared UI,
connector health, route overrides and recovery controls are not implemented.

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
changes in Phase A. There is no Shared UI screen or navigation change.

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
UAT and live connector tests are not asserted. P6 GUI, P7 GUI, P8 UI and M9 remain
**pending**. Future P8 UI must retain
the distinctions above, show observation time, avoid stale-as-live labels and add
neither company bypass nor external-process cleanup. Live Nexus UAT remains
deferred under the existing user direction and does not block this source phase.
