# NEXT RC14 source checkpoint — Preserve native Edge failure stage through Browser Broker (2026-10-24)

**Status: source-only, NOT INSTALLED.** Installed NEXT is still `0.9.2-rc.13` on 8767; stable AgentDock remains `1.1.0` on 8765. No new Edge consent/connect/probe, no Browser/Computer/ACP session, no rollout, no merge/push/release. **Native post-release fresh acquire remains a real Host blocker.**

## Actual user workflow, failure and minimal useful improvement

- The user explicitly re-opened company Edge and authorized exactly one official NEXT Broker connection on **2026-10-24 09:49–09:50**. Installed RC13 connected in 5.167s, acquired independent Oprueba and HomiPlay leases, navigated HomiPlay, released Oprueba while the HomiPlay snapshot still worked, and cleanly released HomiPlay. **One new fresh acquire after both releases failed closed**: `BROWSER_REQUIRED_ROUTE_UNAVAILABLE: external peer verification unavailable or mismatched`. No independent JS/out-of-origin negative tests ran. The formal disconnect left **0 owners/leases/workers/operations/orphans and no Core-owned 9222 TCP socket**; original stable and Edge process untouched. See [installed RC13 real Host evidence](next-rc13-host-uat-2026-10-24.md).
- Product goal is to allow the same explicitly consented Broker socket to admit new scoped tasks **after all prior leases release**, without repeatedly asking the user for native Edge debug consent or unexpectedly changing the foreground app.
- Before this fix, native macOS `verifyNativeEdgeConnection` returned one of five **reviewed, non-sensitive stages**, but `nativeEdgeRuntime.ConnectorStatus` converted every failure into a generic peer mismatch, and `RoutePlanner.Resolve` converted that again into generic `browser route rejected`. Thus genuine fresh-acquire failures could not distinguish lost shared transport, process epoch, Profile epoch or held socket.
- The narrow source correction preserves the approved native stage **only when produced by the exact built-in `*nativeEdgeRuntime`**, carrying `transport`, `process_epoch`, `profile_epoch`, `held_socket` or `postcondition` through `ConnectorStatus → RoutePlanner → browser_broker` **in the caller-visible `Error.Message`**. Unknown provider errors, canceled verification, or forged stages remain generic; raw PID, OS error, Profile path, token, command line, URL and CDP internals never escape.
- **Crucially, this does not change connector authority or how the peer is verified.** Native refusal still closes/quarantines the existing transport and fails closed. No automatic new connection, route fallback, relaxed cryptographic signature, origin permissions, data copying, background poll or user prompt.

## Acceptance and checks

- Targeted synthetic regression: two independent leases both `CleanupComplete`, zero workers, then inject a native verifier failure at the next `ConnectorStatus`. Seven cases cover all five finite phases plus unknown failure and unknown phase, asserting exact actionable error, zero new dials, no worker admission, and transport quarantine. A forged provider attempting to supply a native stage cannot impersonate the native runtime.
- Existing source `TestNativeEdgeFreshLeaseAfterAllOwnersRelease` still confirms the normal fixture path admits a third lease over one socket. **This is NOT evidence that macOS live Edge has been fixed.**
- `go test -race` selected relevant native/plan/admission paths **56 PASS / 0 FAIL**.
- Four-package Go regression **1,701 PASS, 7 opt-in SKIP, 0 FAIL**; `go vet` four packages and `git diff --check` **PASS**. Private JSON logs are in `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc14-safe-peer-stage-*.jsonl`.

## Explicit release gate

Next genuine Host attempt, **only after a substantive root-cause fix beyond diagnostics and normal user consent**, must show one formal connection, A/B leases, sibling survival, both clean releases, fresh acquire, and independent JS/out-of-origin negative checks on isolated leases; capture the stage if refused, then disconnect once. Check foreground samples and no residual socket. Without that Host proof, leave RC13 installed and RC14 a **source-only diagnostic checkpoint**. Full GUI/Tray, crash/recovery and live rollback remain separate unresolved gates. Do not publish, merge `custom/main`, or ask the user to restart Edge solely for this source instrumentation.
