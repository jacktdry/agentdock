# Wave 1 integration evidence

Backend integration originated on `feature/pre-m9-wave1-integration` and is now
integrated into `feature/m8-permission-approval`. The frozen contract remains
[Wave 0](pre-m9-wave0-contract.md); runtime boundaries remain
[Next isolation](agentdock-next-isolation.md). Shared Connection UI is now implemented
under the same contract. **P1/P2 repository verification is complete; execution stops
at the user-owned `mac-dev-next` connector gate.**

- Quick invalidation disables OAuth until the current generation publishes its
  origin and enables OAuth in the same environment write. Real auth validation
  covers the intermediate and published configurations.
- Bundled macOS Next derives helper/service authority from its bundle and ignores
  external helper overrides; Connection selection needs no external manifest.
  External CLI selection requires a private manifest, canonical executable
  helpers, Next helper signing identifiers, and Next bundle metadata.
- Connection and public CLI tunnel mutations revalidate Next identity under the
  Desktop lock and preflight the current configured bind scope before effects.
  Stop requires identity validation but no port preflight. Connection requests
  also validate revision before acquiring the lock and again while holding it.
- Port changes reselect the restarted Core and require `owned_by_next`; stopped
  Core changes preserve stopped state and require the candidate to remain free.
  Failed final verification restores the previous settings **and lifecycle**, then
  proves the old port is again `owned_by_next` (or free for a previously stopped
  Core). Any failed lifecycle restore / ownership proof reports `recovery_required`.
- CLI owns the lock; Shared uses the explicit locked entry point. Basic Settings
  validates Next identity before service queries/effects and leaves Next port
  changes to Connection. Quick config-only saves while stopped return `applied`.
- Snapshots expose availability rather than secrets; OAuth reveal is explicit,
  tunnel tokens are replace-only, and Named custom-port exposure remains blocked
  pending manual route changes. macOS service effects are marked native-backed.

## Verification boundary

Go tests use temporary roots, fake launchctl/service adapters, and temporary
listeners. `AGENTDOCK_LAUNCHCTL_BIN=/usr/bin/false` masks launchctl globally;
fixtures can replace it with their own temporary fake. Go cache is temporary.
The sandbox denies temporary listener binding and some Swift executable fixture
checks, so those suites require an approved unsandboxed run with the same masks.

No live services, production listeners, connector/account setup, signed release
installation, or UAT are validated. Windows and Linux Next service mutations
remain unavailable without an authoritative supported identity adapter.

Verification completed on repository fixtures:

- focused Connection / port / Tunnel regression tests: pass;
- `go test ./internal/desktopapi ./internal/desktopruntime`: pass on integration and again on the final main branch;
- `go test -race ./internal/desktopruntime`: pass;
- focused `desktopapi -race`: bounded at 120 seconds and remained in compilation
  with no tests started / no test failure; recorded as a verification limitation;
- macOS `scripts/test/test-macos-app.sh --fixtures-only`: pass with launchctl masked;
- Windows amd64 and Linux amd64 compile-only checks for both `desktopapi` and
  `desktopruntime`: pass;
- Shared Desktop frontend: `pnpm test` **70/70 pass**;
- Shared Desktop frontend: `pnpm build:dev` passes Vue typecheck and Vite development build.

## Native installer concurrency debt

The Next installer now participates in the Go owner-directory lock protocol;
fixture tests cover contention, owner permissions, bounded rejection of unknown
owners, release, and manifest rollback. It does not reclaim stale locks itself.
This is serialization of cooperating writers, not a globally atomic installer.

Real SMAppService installer/Shared concurrency, other native lifecycle entry
points, crash recovery across multiple native file writes, and final native
listener ownership remain unvalidated debt. No live acceptance or atomicity claim
is made. Quick URL publication still reacquires the lock asynchronously after
installer/local mutation completion.
