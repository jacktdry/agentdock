# Wave 1 integration evidence

Backend integration originated on `feature/pre-m9-wave1-integration` and is now
integrated into `feature/m8-permission-approval`. The frozen contract remains
[Wave 0](pre-m9-wave0-contract.md); runtime boundaries remain
[Next isolation](agentdock-next-isolation.md). Shared Connection UI is now implemented
under the same contract. **P1/P2 repository verification plus Next-only live deployment/readiness are complete; the user-owned
Next connector has been created in ChatGPT as `macbook-air-m3`, and P3 stable/Next side-by-side validation completed on 2026-10-06. Wave 3 is now unblocked; M9 remains blocked on required parity plus hardening integration review.**

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

The original Wave 1 fixture run did not validate live services, connector/account
setup, or release-native UAT. A bounded **Next-only live readiness** pass was then
performed on 2026-10-06 after packaging deployment; it does not convert the local
ad-hoc artifact into release-native signed UAT. Windows and Linux Next service
mutations remain unavailable without an authoritative supported identity adapter.

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

## Next-only live readiness — 2026-10-06

- Shared Desktop was packaged and installed as `~/Applications/AgentDock Next.app` with bundle id `dev.dropabit.agentdock.next`; stable `/Applications/AgentDock.app` was not replaced or restarted.
- Next Core runs under `dev.dropabit.agentdock.next.core` on `127.0.0.1:8767`. The macOS package now builds Core with cgo because the Darwin ownership probe requires `libproc`; a non-cgo Core fails closed instead of authorizing a mutation.
- Public access now provisions missing Next OAuth password/signing credentials transactionally before enabling OAuth; existing credentials are preserved.
- In-place SMAppService updates verify actual launchd running state and perform one bounded retry if macOS still enforces the previous helper launch constraint.
- Dedicated Cloudflare Named Tunnel `mac-dev-next` uses `mac-dev-next.dropabit.dev` and remote ingress `http://localhost:8767`. The tunnel is healthy with active connections; its token is stored only in the Next-owned private runtime state.
- Public HTTPS checks pass for `/healthz`, protected-resource metadata, authorization-server metadata, and the expected unauthenticated `/mcp` 401 challenge. Public MCP URL is `https://mac-dev-next.dropabit.dev/mcp`.
- Stable `mac-dev.dropabit.dev`, its Cloudflare tunnel/DNS, stable Core `8765`, App binaries, and live service remained unchanged throughout this pass.
- The user manually created the Next ChatGPT connector; its current ChatGPT display name is `macbook-air-m3` while its logical role remains `mac-dev-next`.
- Connector copy actions were corrected in `a7e92f2c` to use Wails native `Clipboard.SetText` instead of browser `navigator.clipboard`; Local MCP URL, Public MCP URL, and OAuth password copy now work in the desktop WebView.
- Stable `mac-dev` is the only mutation control plane for Next repository changes, build/package, App replacement, service/tunnel mutation, and repair. `macbook-air-m3` is a validation target only and must not self-modify or self-reinstall.
- P3 side-by-side validation is complete. The next implementation stage is Wave 3 domain parity; all Next mutations still route through stable `mac-dev`, and M9 remains blocked until required parity plus hardening integration review completes.

## P3 side-by-side closeout — 2026-10-06

- Both ChatGPT connectors were callable in the same session. Stable reported `AGENTDOCK_HOME=/Users/wei/.agentdock` and default dir `/Users/wei/AgentDock`; Next reported `/Users/wei/.agentdock-next` and `/Users/wei/AgentDock Next`.
- Task state is split between `~/.agentdock/tasks` and `~/.agentdock-next/tasks`. Stable MCP registry listed five stdio servers; Next listed only its `streamable_http` Memory entry.
- Stable listened on `127.0.0.1:8765` from `/Applications/AgentDock.app/Contents/Helpers/agentdock`; Next listened on `127.0.0.1:8767` from `~/Applications/AgentDock Next.app/Contents/Helpers/agentdock`. Next Core/tunnel labels are `dev.dropabit.agentdock.next.core` / `dev.dropabit.agentdock.next.tunnel`; stable uses its existing `com.uvwt.agentdock.*` services.
- Public protected-resource metadata self-identifies separate resources and authorization servers for `mac-dev.dropabit.dev/mcp` and `mac-dev-next.dropabit.dev/mcp`. The `macbook-air-m3` connector resolves to the Next runtime identity, providing no stable endpoint/state fallback evidence.
- From stable `mac-dev`, a bounded `launchctl kickstart -k` was applied only to `dev.dropabit.agentdock.next.core`. Next Core restarted and returned healthy; stable Core retained the same PID and stayed healthy. Both connectors remained callable afterward.
- No stable App/Core/state/registry/service mutation was performed. This closes P3 only; it is not release-native updater/UAT evidence.

## P4 ACP Manager closeout — 2026-10-06

Wave 3 第一個 domain 已完成。UX / IA contract 先凍結於
[ACP Manager roundtable](pre-m9-acp-manager-roundtable.md)，實作與 hardening 依序為：

- `5b365a21` — revisioned profile settings backend；
- `85e3e75e` — Shared profile manager UI；
- `39b9c127` — adapter detection / installed version probe；
- `bb40bfd3` — trusted adapter update flow；
- `f9972a67` — protected args / update recovery hardening；
- `16f63708` — update redirect + detected-adapter trust hardening；
- `fa3deb68` — independent review findings；
- `c58a6f93` — editor base-revision pinning + broader secret detection。

Security / correctness closeout:

- settings writes use revisioned compare-and-save semantics；stale editor drafts remain pinned to the revision captured when the editor opened, so a conflict reload cannot silently retry the stale draft against a newer revision；
- configured / detected protected args are not exposed to ordinary frontend snapshots；secret classification covers API-key/token/Authorization/JSON credential shapes used by adapter commands；
- adapter detection may overlay detected metadata, but persistence requires explicit **Use detected**；profiles with protected args cannot use that automatic replacement path；
- Safe Update requires an approved repository/release shape, SHA-256 asset digest, trusted final metadata/asset URL, Next-owned target, and Unix owner/permission/symlink checks；missing trusted AgentDock fork release keeps `canUpdate=false`；
- update verification stages in the trusted Next-owned bin directory rather than executable `/tmp`，and version probing only accepts trusted adapter command shapes, bounded output and recognized version syntax；
- persisted disable does not hide still-running runtime sessions before restart；runtime close/lifecycle controls follow observed running state rather than desired persisted enable state；
- restart-required state remains sticky across later mutations until the actual runtime is reconciled.

Verification:

- Go `internal/config`, `internal/desktopruntime`, `internal/desktopapi`: pass；
- Windows amd64 / Linux amd64 ACP/contract compile-only checks: pass；
- Shared frontend typecheck: pass；
- Shared frontend tests: **88/88 pass**；
- Shared development build: pass；
- independent Codex review found concrete HIGH issues and the fixes were re-reviewed; final targeted review reported the secret-detection and stale-editor-revision HIGH findings **RESOLVED** with no new blocker/high；
- Antigravity independent review could not start because the AGY OAuth session timed out; this is recorded as a review limitation rather than counted as a pass.

Next-only deployment / live evidence:

- a fresh arm64 ad-hoc Shared package was built from `c58a6f93`; bundle verifier ran 8 tests, app/helpers were arm64, strict codesign passed, and ZIP/DMG validation passed；
- stable `mac-dev` atomically replaced only `~/Applications/AgentDock Next.app/Contents` using the Next-only installer, then re-registered only `dev.dropabit.agentdock.next.core` and `dev.dropabit.agentdock.next.tunnel`；
- live Next helper reports git revision `c58a6f93d6909ad5bb66d393f03947c2c2959b6c`; Next listener is `127.0.0.1:8767` while stable remains `127.0.0.1:8765`；
- stable Core PID remained unchanged across the entire Next replacement / service re-registration, proving this mutation did not restart stable；
- post-deploy `macbook-air-m3` returned `AGENTDOCK_HOME=/Users/wei/.agentdock-next`, default dir `/Users/wei/AgentDock Next`, ACP enabled with `codex` and `antigravity`, and observation-only status for both profiles；
- public protected-resource metadata still declares `https://mac-dev-next.dropabit.dev/mcp`; unauthenticated public `/mcp` still returns the expected 401 challenge.

GUI visual UAT limitation: the deployed Next GUI is running, but the Next Computer Control Broker reported Orca unavailable and the stable control plane does not currently have macOS Accessibility / Screen Recording permission. Therefore no claim is made that the live WebView was visually inspected in this pass. Repository component tests, production package verification, live service/connector checks and backend authority evidence are the acceptance evidence for P4; release-native signed GUI UAT remains a later gate.

**P4 is closed. The next Wave 3 domain is P5 MCP Management.**

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
