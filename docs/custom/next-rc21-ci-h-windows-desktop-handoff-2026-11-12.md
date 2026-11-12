# AgentDock NEXT RC21 — H Windows DesktopRuntime / Port / Tunnel HANDOFF (2026-11-12)

## 1. Scope and authority

- Owner: **H**, exclusive code ownership `internal/desktopruntime/**`, plus this dedicated HANDOFF.
- Source worktree: `/Users/wei/sideProject/agentdock-next-rc21-ci-h-win-desktop-20261112`
- Local branch: `fix/next-rc21-ci-h-win-desktop-20261112`
- **Fixed parent SHA**: `a2b47000bbbb3fa349d5f2f739496522bf585d9b` (contains local macOS identity fixture correction).
- CI evidence: [Full run 43167819420](https://github.com/jacktdry/agentdock/actions/runs/43167819420), Windows Go Check / Test, package `github.com/uvwt/agentdock/internal/desktopruntime`, job `131013723150`. That run predates H changes.
- Product workflow: a Windows desktop user inspecting or changing the Next Core port and using Named Tunnel / ACP managed updates must not see misleading ownership, token, or update authority. Fix real Windows socket error classification and make regression tests reflect unsupported native ownership rather than manufacturing a PASS.
- Non-interruption: source and local unit tests only; did not touch Stable / Original / running Next, ports 8765/8767, credentials, Cloudflare, browser profiles, or OS services.
- **Not done**: no ACP delegation, no changes to F/G modules, permission store, CI, or shared planning files; no push, merge, deployment, branch/worktree cleanup or Windows-native execution.

## 2. Exact Hosted CI failures and classification

| Windows failure from run 43167819420 | Root cause / classification | H remedy |
|---|---|---|
| `TestPortAvailableTempScope` | Platform capability: `ObservePort` intentionally returns `unknown/next_identity_unavailable` on Windows when Next process authority is unavailable; old portable test incorrectly expected `available` | Assert Windows fail-closed; confirm `PreflightPortMutation` does not authorize |
| `TestPortMutationReobservesWithoutAcquiringLock` | Fixture assumed initial Windows port observation is `available` | Windows initial state must be `unknown`; mutation remains denied under the held desktop lock |
| `TestPortOccupiedFixtures/owned` and `/foreign` (plus parent failure) | Windows Winsock error `WSAEADDRINUSE=10048` differs from synthetic `syscall.EADDRINUSE`; exclusive binds may instead report `WSAEACCES=10013`. Fixture mixed pure simulated classifier evidence with native bind semantics | Recognize only verified `10048` as occupied, retain other errors as unknown; test `classifyPortOwner` separately from kernel bind |
| `TestPortProbePreservesCallerDeadline` | Same kernel bind dependency in a test of ownership probe deadlines | Mac/Unix and Windows `10048` check deadline and cancellation; Windows `10013` is permitted only as `unknown/probe_failed` without invoking owner |
| `TestPortInvalidIdentityAndScope` | Runtime masked canceled context with `next_identity_unavailable` on Windows | Return `observation_cancelled` without binding when already canceled, still never authorize |
| `TestCheckACPProfileUpdateFailClosesWhenTrustedReleaseMissing` | Unsupported Windows managed updater target: `platformTrustedAntigravityTarget` intentionally returns false. Old test expected Unix trusted-release check | Explicitly require `unsupported/update_target_not_next_owned` on Windows |
| `TestCheckACPProfileUpdateReturnsNewerVerifiedForkRelease` | Same Windows unsupported managed updater; known-digest release cannot create target authority | Explicitly require Windows fail-closed; retain Unix successful trusted fork/digest assertions |
| `TestWaitNamedTunnelReadyRejectsInvalidTokenLog` | Test expected translated text `Token 无效` while production returns stable `named_tunnel_token_invalid` reason | Test the exact machine-readable security reason; no token value is logged or accepted |

The Windows `PortOwnedByNext` state is **not activated**. `windowsPortSelection` still returns false without a trustworthy source of selected Next variant/active generation authority. `GetProcessTimes` can read a Windows process creation time, but that does not independently prove that the owner belongs to the selected Next Core. No fabricated PID, process birth instance, or ownership metadata is introduced. Reserved Stable 8765 / Memory 8766 remain reserved.

## 3. Files changed

1. `internal/desktopruntime/port_observation.go` — correctly identify Winsock address-in-use while keeping access denied as unknown; propagate cancellation before Windows authority rejection.
2. `internal/desktopruntime/port_observation_test.go` — separate pure ownership classifier fixtures from OS bind conditions; assert Windows fail-closed and source error handling.
3. `internal/desktopruntime/acp_update_test.go` — distinguish Unix trusted target success from Windows blocked managed updater without widening trust.
4. `internal/desktopruntime/tunnel_supervisor_windows_test.go` — assert the canonical invalid Named Tunnel token reason.
5. This HANDOFF only.

No runtime changes to Named Tunnel token validation or ACP download/verification logic. A successful simulation of a process owner in `classifyPortOwner` test **is not Windows native owner evidence**.

## 4. Verification on macOS (Go 1.26.5)

All commands run only in the H worktree with `GOTOOLCHAIN=go1.26.5`, resolved as Go 1.26.5 darwin/arm64.

- `go test ./internal/desktopruntime -run 'Test(Port|CheckACPProfileUpdate|FetchACPRelease|DownloadACPAsset)' -count=1 -timeout=90s` — **PASS**.
- `go test ./internal/desktopruntime -count=1 -timeout=120s` — **PASS**.
- `go test -race ./internal/desktopruntime -run 'Test(Port|CheckACPProfileUpdate|FetchACPRelease|DownloadACPAsset)' -count=1 -timeout=120s` — **PASS**.
- `go vet ./internal/desktopruntime` — **PASS**.
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/next-rc21-h-desktopruntime-win-amd64.test.exe ./internal/desktopruntime` — **PASS**, `file` reports PE32+ x86-64 Windows test binary, Go binary metadata reports Go 1.26.5 and `GOOS=windows` / `GOARCH=amd64`. Local test binary SHA256: `f79cf23095827ae3fd6671842e21798583439a08878d8c44c03557152421dd9c`.
- `git diff --check` — **PASS**.
- **Windows Native: NOT VERIFIED**. A Windows cross-compiled test binary is not an executed Windows test; full hosted run has not been triggered here.

## 5. Mandatory Windows Native follow-up checklist (integration owner)

- [ ] Re-run the full GitHub Windows Go Check on combined F/G/H code with actual Go 1.26.5; confirm all H-listed failures, plus entire `internal/desktopruntime` package, pass. Do not infer this from cross-compile.
- [ ] On Windows, inspect `TestPortAddressInUsePlatform` and an actual loopback occupied TCP listener: `WSAEADDRINUSE` should identify an occupied port but never automatically grant ownership; `WSAEACCES` must remain unknown, never be reclassified as the owner.
- [ ] Verify `TestPortAvailableTempScope` and `TestPortMutationReobservesWithoutAcquiringLock` return `unknown/next_identity_unavailable` and reject mutation without trusted Next authority. Check Stable 8765 / Memory 8766 still reserved.
- [ ] Verify selected Next process authority remains unavailable until there is OS-backed selection proof; test PID reuse, process birth drift, different binary, ambiguous/foreign listeners, and canceled context without elevating authority.
- [ ] Verify `TestPortProbePreservesCallerDeadline` on Windows under Winsock `10048` and conservative `10013`; do not claim probe called when a bind failed for access reasons.
- [ ] Verify `TestWaitNamedTunnelReadyRejectsInvalidTokenLog` reads only newly appended output and returns `named_tunnel_token_invalid` immediately; ensure historical logs, redaction, and actual token never authorize readiness.
- [ ] Verify `TestCheckACPProfileUpdate*` on Windows is explicitly blocked for unsupported managed target, even when fake release metadata/digest is valid; Unix verified-fork success and redirect/asset-host rejection must still pass.
- [ ] Re-run macOS and Ubuntu jobs, Browser Integration and Container jobs on the full integrated source; this H commit does not certify combined F/G/H or native Windows GUI/Installer/UAT.

## 6. Integration instructions

Review `git diff a2b47000bbbb3fa349d5f2f739496522bf585d9b..fix/next-rc21-ci-h-win-desktop-20261112 -- internal/desktopruntime docs/custom/next-rc21-ci-h-windows-desktop-handoff-2026-11-12.md` and verify no other paths. Apply H's *local commit* into the designated clean integration worktree only after F/G handoffs; run full platform matrix, then update the existing PR only under separate authorization. **Do not merge `custom/main` merely because cross-compile passed.**

## 7. Final local test / commit record

- H code commit (four DesktopRuntime files): `ff94f0c2d81bce54f444869f4e9d92481d2f4d6d`.
- H HANDOFF commit: separate local documentation commit immediately after this report; use H branch HEAD for exact SHA.
- All focused, full-package, focused Race, Vet, cross-compile and diff checks: **PASS** using Go 1.26.5.
- Production and native Windows runtime behavior: **NOT VERIFIED**; next authorized integration owner must run real Hosted Windows CI.
