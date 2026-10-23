# NEXT RC13 source checkpoint — Native Edge verification budget (2026-10-24)

**Status updated after installation:** RC13 source fix/full Go regression and macOS build/install PASS. The [installed RC13 Host attempt](next-rc13-host-uat-2026-10-24.md) made exactly one preannounced `connect_external`, which **timed out at the client before a lease could start**; no peer admission result was obtained. Core's pending 9222 socket later closed without retry. Thus this fix remains **Host-unconfirmed**; RC12's first-lease FAIL CLOSED is separate earlier evidence.

## User workflow and observed problem

The user needs ChatGPT / ACP to reuse the **one user-authorized Edge Broker socket**, acquiring and releasing multiple distinct company browser leases without re-prompting or focus stealing. RC11 demonstrated four approved site origins and sibling cleanup, but failed on a subsequent fresh lease. RC12 had one successful formal `connect_external` (5.649s), immediately followed by a failed **first** `direct_acquire`, with generic peer verification rejection. That formal socket was explicitly disconnected with zero owners/leases/workers/orphans and **not retried**. Source RC12's lsof scope and native signer-slot ordering did not establish Host success.

## Concrete source correction (no privilege relaxation)

- The native verifier performs **two sequential, independent macOS evidence stages**: process/signer/Profile epoch and exact held-socket proof. Each child has an existing 2s bound. Previously **each of its outer callers also imposed 2s**, making the combined proof vulnerable to deadline failure even if both individual checks remained valid.
- Native Edge's outer budget is now **5s** in `verifyNativeEdgeConnection`, `RoutePlanner.Resolve` for `EngineNativeCDP`, and `ExternalLeaseManager.verifyPeer` for the actual native runtime. Non-native connectors retain **2s**. Each OS process/socket stage keeps its independent 2s bound; no skipped checks, new grants, Profile substitution, automatic retry or increased company origin scopes.
- Planner status and lease peer proof now release their single-flight slot **before** publishing the result to the next serial caller, while retaining bounded cancellation and noncooperative-provider fail-closed semantics.
- `verifyNativeEdgeConnection` reports only a fixed set of safe failure phases (`transport`, `process_epoch`, `profile_epoch`, `held_socket`, `postcondition`) to the native lease rejection path. No OS error string, PID, Profile path, command line or auth secret escapes; unknown stages remain generic. This makes the **next single formal Host attempt actionable if it still fails**.

## Evidence

- A synthetic native verifier taking 2.25s (validly longer than one child budget) is accepted by both the planner and external lease verifier. Previous 2s enclosing budgets would have rejected it. The test also verifies neither completed path leaves a busy single-flight slot.
- Real macOS **read-only** evidence measured for the existing signed Edge listener: `lsof`, `ps`, `codesign` succeed (no new CDP socket, no native debug prompt, no navigation or user's Profile contents). This is timing context, not a transport-bound attestation.
- Finite failure phases and arbitrary canary errors are tested through the actual native lease verifier boundary: the safe phase is displayed and the transport is closed on loss, while unknown private provider content never appears.
- Targeted `go test -race` covering native budget/stages, previous fresh-lease source path, external admission and planner cancellation: **52 PASS, 0 FAIL**. Full four-package Go test **1,692 PASS, 7 opt-in SKIP, 0 FAIL**; four-package `go vet` and `git diff --check` **PASS**.

## Acceptance and explicit non-work

**Real-user gain remains provisional until installed Host acceptance:** one approved Edge connection must admit A/B leases, allow sibling activity after A release, permit fresh acquire after both release, and separately reject out-of-scope JS/origin. The first RC13 Host attempt must remain single-connection with no re-prompt loop. Confirm website no-focus and no cross-lease cleanup; continue keeping Stable 8765 and user Edge Profile intact.

**Do not** repeatedly create short Edge probes, invent attestation success from OS observations, copy stable credentials, merge `custom/main`, push, release or mark DoD complete without Host evidence. RC12 rollback bundle (RC11) and installed RC12 remain available independently of source work.
