# AgentDock NEXT RC21 — F Windows Browser / Edge Preflight HANDOFF (2026-11-12)

**Owner: F only.** This is a local source/test handoff, **not** Windows-native CI success or a release approval.

## Scope, baseline, and non-interruption

- Pinned start: `a2b47000bbbb3fa349d5f2f739496522bf585d9b` (not a moving integration branch).
- Local branch: `fix/next-rc21-ci-f-win-browser-20261112`.
- Worktree: `/Users/wei/sideProject/agentdock-next-rc21-ci-f-win-browser-20261112`.
- Source failure: [GitHub full run 43167819420](https://github.com/jacktdry/agentdock/actions/runs/43167819420), Windows Go Check job `131013723150`, package `internal/tool/browser`, 30 failed test/subtest lines in original run. Linux Go Check, browser integration and container were successful in that run; it predates this local F change.
- User workflow: Next must reject an unqualified external **company Edge** process rather than silently switching profile, scope or transport, while the Windows build/test pipeline remains useful. This fix makes platform-specific tests accurate; it **does not enable Windows external Edge authentication/routing**.
- No service, installed browser, Edge profile, company data, stable Mac-Dev, current NEXT runtime, CI YAML, GitHub branch, worktree outside F, or other workstream was modified. No Push/Merge/deploy/branch cleanup.

## Failures and causal classification

| CI test group | Classification and observed cause | F correction |
|---|---|---|
| `TestEdgeCommandMatchesConservativeArguments`, `TestEdgeProfileMetadataBoundedReadOnly`, `TestEdgeProcessPreflightProfileFence`, `TestEdgeProcessPreflightMatchesOnlyOSProcessEvidence` | **macOS/POSIX positive fixture on Windows**, not proof of a Windows product capability. Uses fixed `/Applications/Microsoft Edge.app/...`, `/tmp/...`, and POSIX lstat/uid evidence, which Windows canonical-path validation correctly rejects before attestation. | Existing `edgeEpochPositiveFixtureAvailable` gate now checks the Windows zero-evidence / no-listener / no-signer / no-transport-fence rejection, and runs positive fixture logic only on supported POSIX platforms. |
| `TestEdgeProcessPreflightRejectsUnsignedAndChangedDuringSigning`, `TestEdgeProcessPreflightLiveSignatureFence`, `TestEdgeProcessPreflightExposesNoAuthOrRouteGrant`; related ambiguous/unsafe-input tests | **macOS-specific mock signature/process tests** cannot qualify a real Windows native Edge. The original Windows failures were in the mock's positive stage or the expected stage ordering, not observed Windows Microsoft signature proof. | Same explicit unsupported-platform gate, preserving existing macOS/Linux synthetic negative/signature coverage. New Windows-tagged `TestWindowsNativeEdgeProcessIdentityUnavailable` asserts the real native Windows stub returns only `errEdgeProcessUnqualified` and zero evidence. |
| `TestEdgeEvidencePinnedReject/profile-mismatch` | **Portable fixture path bug**: `/private/other-edge` is not an absolute Windows path, so validation failed before the intended independent peer-attestor stage. | Use `filepath.Join(os.TempDir(), "other-edge-socket-fixture")` as a distinct canonical OS-native absolute profile path. Profile mismatch still fails, but at the intended stage. |
| `TestExternalWorkerConfig`, `TestManagedWorkerPinAndVersionProbe` | **Windows executable fixture assertion** demanded basename exactly `npx`; Windows Node may resolve `npx.cmd` (or a Windows executable shim). | Assert `commandpath.LookPath("npx")` is absolute and its Windows basename is `npx` plus a recognized executable suffix, without changing runtime resolution or allowing an arbitrary command. |
| `TestConcurrencyIdleTTLAndSweeps`, `TestManagedLeaseIsolationAndOperations` | **Clock-resolution/test scheduling**: immediate acquire/call can share a `time.Now()` tick on Windows, so `After(initial acquire)` need not be true. | Seed a known earlier fixture `LastActiveAt`; assert a successful call updates it beyond that value, TTL formula and stale sweep hold, failed operations do not update activity. No runtime TTL or permission relaxation. |

## Security gates preserved

- `external_process_preflight_other.go` continues to return `errEdgeProcessUnqualified` on Windows. There is no Windows OS process/birth-identity/signature/owner attestor here; the tests **do not fabricate one**.
- No change to fixed on-disk Edge signing checks, live Microsoft process signature checks, process-epoch revalidation, canonical owner/Profile matching, scope, authenticated browser evidence, relay peer attestation or transport fence. The company Edge profile cannot be qualified based on a mock fixture or a raw CDP socket.
- No production Go source changed. Only `internal/tool/browser/*_test.go` and this F-owned HANDOFF were touched.

## Local verification — Go 1.26.5, macOS arm64

Commands run within F worktree with `GOTOOLCHAIN=go1.26.5` (observed `go version go1.26.5 darwin/arm64`):

1. `go test ./internal/tool/browser -run 'Test(Edge|ConcurrencyIdleTTLAndSweeps|ExternalWorkerConfig|ManagedLeaseIsolationAndOperations|ManagedWorkerPinAndVersionProbe|WindowsNativeEdge)' -count=1 -timeout=120s` — **PASS**.
2. `go test ./internal/tool/browser -count=1 -timeout=180s` — **PASS** (`ok`, approximately 28.3 s).
3. `go vet ./internal/tool/browser` — **PASS** (no diagnostic).
4. `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/agentdock-next-rc21-f-browser-windows-amd64.test.exe ./internal/tool/browser` — **PASS**, verified PE32+ x86-64 test executable (compile only, not executed).
5. `git diff --check` — **PASS**.

These results confirm macOS tests and Windows compile **only**. There is no Windows host, Windows Edge process, Windows kernel identity, Microsoft-signed running code, or Windows GUI available to this F session, and **no Windows Native PASS** is claimed.

## Windows Native Hosted Runner follow-up (integration owner)

After the integrator has inspected ownership/commits and integrated F/G/H in a separate clean worktree, rerun the genuine Windows Go Check on GitHub Hosted Windows; do not bypass or disable any checks:

1. Confirm the full `internal/tool/browser` package passes, including `TestEdgeEvidencePinnedReject/profile-mismatch`, Windows `TestWindowsNativeEdgeProcessIdentityUnavailable`, `TestConcurrencyIdleTTLAndSweeps`, `TestManagedLeaseIsolationAndOperations`, `TestExternalWorkerConfig` and `TestManagedWorkerPinAndVersionProbe`.
2. Verify the POSIX synthetic preflight tests on Windows perform **fail-closed rejection** and do not execute simulated signed Microsoft identity as if it were a native observation; macOS/Linux continue to execute the full signature and changed-process negative suites.
3. Verify `commandpath.LookPath("npx")` on Hosted Windows resolves actual `npx.cmd`/approved Windows executable; version probe uses the configured command and PATH without silently substituting another executable.
4. Re-run Windows full Go Test/Race/Vet/Build where configured and compare the **actual new run** with 43167819420; other packages and Windows native GUI/installer are outside F scope.
5. A fully functional Windows external company Edge route still requires separately implemented and reviewed OS-authenticated process owner/signature/birth identity, selected Profile proof and transport-bound peer attestation. Until then it **must fail closed**.

## F handoff and boundaries

Expected modified paths: `internal/tool/browser/concurrency_test.go`, `external_pinned_cdp_evidence_test.go`, `external_process_preflight_profile_test.go`, `external_process_preflight_test.go`, `external_process_preflight_windows_test.go`, `managed_leases_test.go`, `managed_worker_test.go`, plus this HANDOFF. **No edits to** G (`internal/app`, `internal/config`, `internal/desktopapi`, `internal/httpx`, `internal/tool/plugin`), H (`internal/desktopruntime`), `internal/permission`, CI YAML, shared status/roadmap or other branches.

**Result:** locally repaired and cross-compiled **candidate**, awaiting Windows Hosted native execution by the integrator. No remote changes authorized in F.
