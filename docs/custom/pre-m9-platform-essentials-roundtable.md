# Pre-M9 P7 — Nexus / Platform Essentials UX, Security and Authority Contract

> 2026-10-08. Product/UX, architecture and cross-platform synthesis, with independent reviews. **P7 Phase A1/A2 backend now implemented, patched at `209c776e`, independent targeted A2 security re-review CLOSED its 2 HIGH findings. Phase B typed Wails NexusDock Shared UI is implemented and independently source-reviewed (0 verified BLOCKER/HIGH), but remains an offline source checkpoint, NOT installed or native-UAT validated.** Phase C native opener/platform essentials, P6 native GUI acceptance, A1 enterprise proxy decision, and M9 remain separate open gates. No real Nexus pairing was attempted.
>
> Source inventory: [P7 read-only inventory](pre-m9-platform-essentials-inventory.md). This document specifies *Next-only* behavior. Nothing here authorizes restarting, modifying or migrating stable AgentDock, stable Memory, or their services.

## 1. Decision and divergent viewpoints

| Perspective | Position | Principal concern | Disposition |
| --- | --- | --- | --- |
| Product/operator | Status must explain whether the device is actually connected, not merely paired. Pairing should be a single task with clear recovery. | A successful save but failed Core restart must not be reported as connected. | Adopt explicit `paired` / `connected` / `restart-required` semantics. |
| UX/IA | Reuse Connection navigation; use **NexusDock** as a separate view within Connection, alongside existing ChatGPT MCP/Tunnel controls. Provide Runtime/Overview status link into it. | Adding an eleventh top-level navigation destination worsens narrow layouts; mixing Nexus credentials into existing OAuth/Tunnel editor confuses authority. | Adopt a focused Connection subview, not a new top-level menu. |
| Architecture | A typed, Next-owned Desktop service owns all pairing/status/shortcut mutations; UI is presentation only. | The CLI takes raw code and endpoint; renderer should never be able to execute commands or select filesystem paths. | Desktop-owned status/pair authority. Reuse existing runtime service and platform adapters. |
| Security/operator | Threat model includes a malicious/incorrect endpoint, credential exfiltration via redirects, re-pair races and directory symlinks. | Existing `nexusbridge.Pair` uses `http.Client` with default redirect behavior; URL scheme parsing is not a complete peer/IP SSRF defense. | A hardened network client and generation-fenced mutation are **Phase A prerequisites**, not optional UI polish. |
| Cross-platform | Capability is determined by the real host adapter and ownership verification, never only by `GOOS`. | macOS and Windows legacy UI have operations that are not necessarily wired into Wails/Next, while Linux/WSL have different affordances. | Return typed capabilities and disabled reasons; do not show pretend buttons. |

### Independent Red Team challenge and evidence correction

The read-only Gemini 3.1 Pro / High reviewer identified two real new-UI blockers: redirect/DNS-rebinding SSRF in the shared pairing transport, and concurrent pairing without a generation/commit lock. The reviewer was challenged with source evidence and **explicitly withdrew** a third claim: `cmd/agentdock/server.go` already calls `os.Unsetenv("AGENTDOCK_NEXUS_ENDPOINT")` and `os.Unsetenv("AGENTDOCK_NEXUS_TOKEN")` **before** `config.FromEnv()`; the claimed legacy-env fallback is **not** an existing issue. Do not create a spurious fix for it.

Go `net/http` rewrites POST to GET (dropping the body) for **301/302/303**, so those redirects create SSRF risk but not automatic pairing-code body forwarding. **307/308** can preserve POST body and expose code/device ID; reject **all** redirects in pairing requests. The Windows `RuntimeService.cs` symlink concern applies to a **legacy opener**, not an already shipped Next opener; it is an implementation warning for the *new* P7 Next directory action. Existing Wails Next `desktop/shared-poc/main.go` already provides minimal app/tray **Show/Quit** menus; P7 audits status/health parity and ownership rather than implementing a tray from scratch.

### Debate resolution

- **Option A: copy legacy Native Advanced Settings into Shared UI.** Small apparent UI effort, but creates a second unsafely wired network/secret path and duplicates settings.
- **Option B: expose the existing CLI through a generic command wrapper.** Rejected: raw command args, output and endpoint authority can leak secrets or cross the stable/Next boundary.
- **Option C: a minimal Desktop/Core contract and Connection subview.** **Selected.** Slightly more backend work, but permits bounded authority, honest status and cross-platform tests.
- **No second persistent Nexus token source**: continue to use the existing identity file as Core's source of truth. UI receives only derived, redacted read models.

## 2. Navigation and interaction

**Connection → ChatGPT access / NexusDock.** These are *distinct* scopes. Existing local/public MCP URLs, OAuth password, Port and Tunnel remain in ChatGPT access; no migration or visual mixing of credential fields. NexusDock has a status-first screen:

1. **Status:** Not paired / Paired but offline / Connected / Restart required / Identity invalid / Service unavailable. A connection observation is only valid when Core is healthy and the returned evidence is current. Include timestamp / Refresh; distinguish observation failure from proven offline.
2. **Identity:** safe server origin (not raw URL path), device name if available, safe node identifier in Advanced, Device Token **Saved/Not saved** only. Copying the Device Token is not supported.
3. **Pairing:** NexusDock server URL, one-time code in password field, optional human-friendly device name, and explicit Pair. No prefilled or persisted one-time code. Clear secret state on success/cancel/unmount and on rejected operation.
4. **Existing identity:** Re-pair is a separate secondary/danger action with explicit before→after target origin, warning that the existing link will be replaced only after a successful exchange, and acknowledgement that Core reconnect/restart may briefly interrupt work. Do not offer silent re-pair when a paired identity exists.
5. **Help:** simple explanation of Nexus versus ChatGPT MCP/Tunnel, how to obtain a one-time code, and why "paired" can differ from "connected"; errors show safe recovery instructions, never upstream response bodies.

Do not introduce a new primary sidebar item. Use summary→details→advanced hierarchy; desktop and 320 CSS px widths must show usable vertical controls with no horizontal page scroll. Dialogs require focus trap, Escape/cancel, visible focus, screen-reader status/live region, and disabled-reason text; preserve an unsaved URL draft on ordinary refresh but **never** retain a pairing code.

## 3. Desktop authority and API contract (proposed names)

New typed `desktopapi.NexusService` (or equivalently named domain service) must be owned by Next Desktop backend and bound through Wails like existing ACP/MCP/Plugin services; no renderer `exec`, shell, `nexus pair` process invocation or filesystem access. Suggested contract:

| Operation | Input | Output | Behavior |
| --- | --- | --- | --- |
| `NexusSnapshot(ctx)` | none | `NexusSnapshotResult` | Pure observation: status, capability, redacted identity and Core connection signal. No network pairing, restart or mutation. |
| `NexusPair(ctx, request)` | URL, one-time code, optional name, `expectedGeneration`, `confirmReplace` | `NexusMutationResult` | Next-only, bounded context, exclusive mutation and stale-generation rejection; safe error codes. No code/token echo. |
| `NexusReconcile(ctx, expectedGeneration)` | generation | `NexusMutationResult` | Explicit Next Core restart/reconnect step, only if backend supports it; reobserve status afterwards. Must not restart stable Core. |
| `OpenNextDirectory(kind)` | enum `logs\|configuration` | completed / capability + safe error | Native adapter only; no renderer-provided path or arbitrary URL; enforce Next-owned directory identity. |

`NexusSnapshotResult` fields: `generation` (opaque, not secret), `pairingState` (`not_paired\|paired\|invalid\|unknown`), `connectionState` (`connected\|disconnected\|checking\|unavailable\|unknown`), `restartRequired`, `safeOrigin`, `nodeId` (Advanced only), `deviceTokenStored` boolean, `observedAt`, `capabilities: {canPair, canReconcile, canOpenLogs, canOpenConfiguration}` and each disabled reason, and safe typed `error`. Never expose identity file, raw endpoint path, token, one-time code, raw HTTP response or stderr. `paired` never implies `connected`.

`NexusMutationResult`: `operationId`, `completed`, `identitySaved`, `restartRequired`, `observedGeneration`, safe `error`. **Success is not a claim of live connection**. If the pairing exchange and atomic save succeed but restart fails, report `identitySaved=true, restartRequired=true` and retain the new identity for explicit recovery. If save fails, preserve the previously valid identity. Retry is only explicit; reusing the consumed code automatically is forbidden.

### Authority boundaries

- Resolve runtime root and `AgentDockHome` **from verified Next-owned runtime identity/manifest**, not renderer strings, process-global defaults, or guessed `~/.agentdock-next` paths. Reject missing, foreign, ambiguous, mismatched or symlink-swapped ownership. Distinguish App Support runtime root from `AgentDockHome` state root. Never fall back to stable.
- Mutations use the existing desktop mutation admission/lock pattern and revalidate owner + `expectedGeneration` inside the lock, immediately before the remote request **and** immediately before commit. Re-pair must be serialized against other pairing/reconcile operations. Return `nexus_generation_conflict` on stale view.
- Identity save must be atomic, private (0600 on POSIX or equivalent Windows ACL), not copied into Desktop runtime JSON. A revision/generation strategy must distinguish changes even if the same origin/node is reused; prefer opaque state revision or generation hash that excludes secrets from output. Design exact journal/recovery before implementation.
- One-time code must not appear in CLI arguments, URL/query, logs, audit trails, analytics, errors, dumps, rendered exception state or telemetry. The existing macOS/Windows legacy CLI paths are **not** the template for new Shared authority.
- Use safe error codes/categories such as `next_identity_unavailable`, `nexus_endpoint_invalid`, `nexus_endpoint_blocked`, `nexus_pair_denied`, `nexus_pair_timeout`, `nexus_generation_conflict`, `nexus_identity_save_failed`, `nexus_restart_required`, `nexus_native_unavailable`. Only sanitized messages may be returned; no unfiltered provider body even for 4xx/5xx.

### Remote pairing transport gate

Current `normalizeEndpoint` in `internal/nexusbridge/identity.go` checks HTTPS for non-loopback and forbids URL userinfo/query/fragment. This does **not** address DNS resolution, IP ranges, redirects or URL-path trickery. Before enabling the new UI:

- Validate canonical HTTPS origin/allowed base path and explicit loopback-only HTTP policy; reject credentials, query, fragment, unusual/untrusted schemes and ambiguous normalization.
- Reject cross-host and same-host HTTP redirects for credential-bearing pairing requests (fail closed on any 3xx), rather than using the default `http.Client` redirect following; never resend code to another destination.
- Protect against SSRF and DNS rebinding: disallow private/link-local/metadata/multicast/unroutable/nonpublic peer IPs for public HTTPS endpoints, validate **actual dial destination** after DNS, not only the hostname before dialing; handle both IPv4 and IPv6. Loopback HTTP is a documented local-development-only exception bound to verified local peer.
- Enforce TLS hostname verification, bounded timeout and response size; no automatic fallback from HTTPS to HTTP, no proxy path that evades destination policy, and no raw remote response data in a UI-visible error.
- Review compatibility of public hosted paths and existing paired identity files separately; do not silently rewrite an existing identity during a read or probe.

## 4. Other P7 parity without duplicating existing UI

| Capability | UI ownership | Safe action and state |
| --- | --- | --- |
| Core service lifecycle/health | **Runtime** (existing) | Link from Nexus status to Runtime restart if supported. Never add another lifecycle implementation. |
| Core autostart | **Settings** (existing) | Keep `CoreAutostartMutable` + disabled reason; distinguish service start, app login item and Tunnel autostart. |
| Tunnel autostart | **Connection** (existing) | Retain Next port/tunnel identity checks; not a Nexus control. |
| Update check/apply | **System** (existing) | Check-only until M9 signing/update gates; do not enable apply as a P7 shortcut. |
| Open Logs / Configuration | **System → Diagnostics** | Native allowlisted Next paths only, open directories in host file manager; no directory picker, arbitrary path or raw file contents in renderer. Verify symlinks/ownership before create/open. Configuration folder can contain secrets: show a concise warning. |
| Tray/menu | **native shell/platform adapter** | Audit Wails Next shell support and app vs Core health indicator. Do not recreate legacy menus in Vue; document unsupported states. |
| OS permissions / elevation | **System → platform-specific information** | Keep operating-system grants distinct from AgentDock Allow/Ask/Deny; read-only explanation and native-request action only with declared capability. No fake "all permissions granted" state. |

Platform capability matrix is runtime-authoritative: macOS native adapter, Windows native adapter, Windows/WSL service boundary and Linux helper each explicitly declares `supported\|unavailable\|requires_native\|read_only` with a reason. Unsupported actions render disabled, not hidden or falsely enabled. Native file openers must derive fixed Next directories from verified identity, reject escaping symlinks and not follow renderer paths. For a non-existent directory, create it only after parent ownership checks and on user action.

## 5. Verification contract and stop/go

**Security / API tests (Phase A first):**

- Fresh Next identity missing/valid/invalid/unreadable; stable manifest selected; symlink race; state root not equal runtime root; cross-platform permissions.
- Pair first time, replace after explicit confirmation, stale generation / simultaneous pair, save failure preserving old identity, Core restart failure returning `restartRequired`, repeated consumed code rejected without automatic retry, reload after Core crash.
- Reject `file://`, URL userinfo, query, fragment, external HTTP, public HTTPS hostname resolving to loopback/private/link-local/metadata, IPv4-mapped IPv6, DNS rebinding, redirects 301/302/303/307/308, HTTPS→HTTP downgrade, proxy bypass, overly long/error bodies and oversized code/name/endpoint.
- Log/telemetry/error snapshots cannot contain code, token, raw upstream response or private filesystem paths. `ReadStatus` and UI refresh cause no outbound request or restart.
- Open Next log/config directories only, including symlink swaps, absent directories and macOS/Windows/Linux native-unavailable behavior; stable directories unchanged.

**UX / accessibility (Phase B after API gate):** unpaired, paired but offline, healthy connected, stale/checking, invalid identity, permission unavailable, wrong code, timeout, re-pair confirmation, restart-required recovery, cancellation, focus return, keyboard-only, 320 CSS px layout and three UI locales (zh-TW/en plus one additional). The password field clears and never rehydrates after refresh.

**Regression / platform (Phase C):** Go package tests and race, desktop bridge tests, Shared frontend typecheck/unit/build, Windows cross-build, macOS Next-only live paired-status/read behavior with **no real Nexus re-pair without explicit test endpoint authorization**, then native Windows/WSL/Linux checks at release-native gate. Verify stable app/core/memory/tunnel untouched after all operations.

### Explicit stop conditions

- Do not begin Phase B pairing UI until transport safety, Next-only identity derivation, stale generation, mutation serialization and safe error contracts have backend tests.
- Do not declare P7 done merely because read-only status UI renders; native directory actions, platform truthfulness and tray audit need verification or explicit scoped deferral with user approval.
- P6 native GUI acceptance remains pending; P7 planning does not close it. No M9 until required P6–P9 parity/hardening gates are complete.

## 6. Execution slices

1. **Phase A — Backend and security:** Next-only identity resolver; Nexus Desktop status read model, hardened `Pair` transport and redaction; generation-fenced save and restart-required outcome; focused Go tests. Start here.
2. **Phase B — Shared Connection Nexus view:** safe binding, state machine, i18n, keyboard and narrow-width tests. Do not invoke legacy CLI from Vue.
3. **Phase C — Native essentials:** Diagnostics fixed Next file-open actions, tray/menu and platform capability audit; no duplicate core/tunnel settings.
4. **Phase D — Independent review / Next native UAT:** security/cross-platform review, Side-by-side stable isolation recheck, documentation and memory checkpoint. Native GUI evidence required for formal P7 closeout.

## 7. Known unresolved implementation details

- Decide whether to support public Nexus hosting under URL path prefixes without exposing secret-bearing paths in the UI; current CLI permits path segments.
- Determine exact Next manifest → `AgentDockHome` resolver and Windows/WSL native file opener ownership; platform support must be verified, not inferred.
- Determine atomic generation/journal format and how a restart outcome is observed without claiming a connection prematurely.
- Audit actual Wails Next tray/menu support. Update/apply remains M9-gated regardless of P7 scope.
- **Proxy compatibility:** Phase A1 deliberately bypasses environment/system HTTP(S) proxies for pairing. Enterprises that require outbound proxy egress cannot use this transport as-is. Re-enable proxy support only with an explicit trusted-egress/destination-verification design; do not silently inherit `HTTPS_PROXY` in order to satisfy SSRF policy.

## 8. Implementation checkpoint — Phase A1 transport (2026-10-08)

Following the design freeze, the first bounded implementation slice changed only `internal/nexusbridge/identity.go` and `identity_test.go`. It introduces an explicit no-redirect pairing HTTP client; dial-time resolved-IP validation with the validated IP passed as a literal destination; strict public HTTPS peer filtering with explicit local HTTP loopback development exception; no environment-proxy bypass; original HTTPS hostname TLS validation; sanitized pairing errors and bounded request/response parameters.

New offline tests cover 301/302/303/307/308 rejection without forwarding, mixed public/private DNS answers, DNS rebind before dial, public/IPv6/special IP classification, proxy disabled, localhost-only HTTP, TLS hostname verification, redacted HTTP errors and failure preserving existing identity. On macOS arm64 with Go 1.27.1, independently run `go test ./internal/nexusbridge`, `go test -race ./internal/nexusbridge`, `go test ./cmd/agentdock`, `go vet ./internal/nexusbridge`, Windows/Linux amd64 `go test -c` cross-compiles, and `git diff --check` all passed.

Independent Gemini 3.1 Pro / High read-only code review reported **PASS for tested security vectors**, with **one HIGH functional-compatibility finding**: `Proxy: nil` blocks pairing on networks that require an outbound proxy. For this initial security slice the no-proxy behavior is intentional to avoid bypassing validated destinations, but it remains a visible product limitation and a required design/acceptance decision before P7 closeout; **do not label this review 0 HIGH**. No proxy fallback is introduced.

**A1 is not all of Phase A:** neither the Next-only Desktop authority, pairing generation/mutation lock, transactional re-pair/restart outcome, nor Next native folder opener / Shared UI exists yet. No installed Next or stable services were restarted. Phase A2 and release-native verification remain separate acceptance gates.

## 9. Implementation checkpoint — Phase A2.1 Next-only read-only status

First isolated Desktop observation slice (not Wails-wired, not deployed):

- `internal/desktopapi/nexus.go`: a typed safe `NexusService.Snapshot(ctx)` contract; `not_paired / paired / invalid / unknown`, opaque generation, safe HTTPS/loopback origin, safe node id, token-stored boolean, observed timestamp and disabled pair/reconcile capabilities. No raw token, code, file path or stdout/stderr. No mutation or automatic state file creation.
- `internal/desktopruntime/nexus_home_darwin.go`: derive fixed Next state root using **verified signed Next bundle/runtime identity and AppPaths(.next).stateDirectory**; verify the trusted Next environment configuration doesn't redirect `AGENTDOCK_HOME` to stable, ignore ambient process-level `AGENTDOCK_HOME` for authority, enforce no-follow, owner/permission, regular-file/single-link and bounded reads. The actual state root is `~/.agentdock-next`, distinct from App Support runtime root; never use the renderer to select a source.
- Windows/Linux backend is **explicitly unavailable** in this slice pending equivalent Next native authority. Windows/Linux compilation is not evidence of live Next support.
- **Core connection is deliberately UNKNOWN:** existing `service.status.nexus_connected` does not identify which persisted Nexus identity the running Core loaded, and a generic client dialing mutable `control.sock` is vulnerable to socket-swap/peer-identity uncertainty. Default Snapshot performs **no** unverified socket dial. The API cannot truthfully claim `connected` until a Next-authenticated peer and an active Nexus identity generation matching persisted state are available. Test injection of a bare connected flag still returns `unknown`; a reliably observed stopped Core can be `disconnected`.
- Stale revision snapshots fail closed if the identity changes between reads. Domain-separated SHA-256 digest is an opaque generation: hashing the high-entropy device token makes even token-only identity replacement observable, while never returning the raw token or input bytes. Token changes must not share a generation; a public-only hash would fail this property. Full generation-fenced pair/commit remains Phase A2.2.

**Independent review checkpoint:** Gemini 3.1 Pro / High first-round Red Team flagged a potential stable-to-Next directory confusion, the local control Socket TOCTOU, connection-state truthfulness, and hash-derived-generation concerns. Targeted changes removed reliance on ambient `AGENTDOCK_HOME` while preserving the **signed, fixed Next state directory** as authority; removed the unsafe local control Socket dial; and enforced `unknown` rather than `connected` until a matching active Core identity is proven. SHA-256 generation remains opaque and sensitive to token-only changes. **Round 2 re-review: PASS — 0 BLOCKER / 0 HIGH** for the bounded **read-only A2.1** scope. This is **not** approval to enable pairing mutations or to claim live Nexus connection. On macOS arm64 the affected `desktopapi` / `desktopruntime` Go suites, focused race tests and vet passed; Windows/Linux cross-build supports compile-time parity only.

### Remaining A2.2 gate

Add owner-verified Next Core peer identity + runtime identity-generation attestation; serialize local CLI and Desktop re-pair across all paths and verify expected generation before network and commit, preserve prior identity on save failure, return truthful `identitySaved/restartRequired` states and safe errors, then wire Wails. Keep `canPair=false` and `canReconcile=false` until this is tested. Native GUI P6 acceptance, proxy compatibility and M9 remain separate gates.

## 10. Implementation checkpoint — Phase A2.2 Nexus pairing backend (2026-10-08)

**Source-stage only, not Wails-bound, not rebuilt or deployed.** The bounded A2.2 implementation builds on A2.1 without touching the stable installation:

- `internal/nexusbridge/identity.go` and OS storage adapters: semantic identity generation, cross-process `PairChecked` lock shared by CLI and Desktop, stale-generation rejection **before** consuming a one-time code and **before** commit, explicit re-pair acknowledgement, sanitized errors, bounded transport and atomic private identity writes. Fresh Next state-home creation now verifies the parent and final directory **before network**; symlinked state/parent input is rejected and tests assert the simulated stable target is unchanged.
- `internal/desktopcontrol/control_verified_darwin.go`, `internal/desktopruntime/nexus_status_darwin.go`, `cmd/agentdock/server.go`: the Core exposes the generation of the identity it loaded at startup, while macOS Desktop verifies the selected Next launchd Core PID, Unix socket peer PID/UID and process binary/creation instance before and after the control response. A bare boolean `nexus_connected` without matching active identity generation never proves a connection. Windows/Linux Nexus native authority remains explicitly unavailable until implemented.
- `internal/desktopapi/nexus.go`: typed `Pair` / `Reconcile` mutators with Desktop runtime lock, fresh Next identity verification, generation confirmation and safe operation result. Pairing can save a new identity even if the subsequent **Next-only** Core restart fails; caller receives `identitySaved=true, restartRequired=true`, never a false connected claim. No automatic one-time-code replay.
- `internal/desktopruntime/mutation.go`: **stable continues to use the original runtime-local `.desktop-mutation.lock`** to preserve existing clients. Only Next uses the new runtime-root-scoped sibling lock to avoid symlink-mediated writes into stable paths. Regression tests fix both behaviors.
- CLI `nexus pair` requires explicit `--replace` when an existing pairing is present. The new Shared Desktop backend calls a typed Go API, **not** the CLI or shell. Legacy CLI's `--code` flag remains a separate pre-existing exposure; do not use it for the Shared UI.

Verification on macOS arm64: targeted/full package `go test` for `nexusbridge`, `desktopcontrol`, `desktopruntime`, `desktopapi`, `cmd/agentdock`; targeted `go test -race`; `go vet`; `git diff --check`; Windows/Linux amd64 `go test -c` for affected packages. **Passed.** Windows/Linux *compilation* is not native UAT. An earlier ACP review attempt failed on adapter/model configuration; independent external review is being retried in read-only Plan mode and must be reported honestly before claiming security signoff.

**Important remaining gates:** independent A2.2 security review and any fixes; native Next Core/GUI live test with authorized, disposable pairing endpoint (no real account re-pair without approval); Wails binding and Phase B UI; Phase C native directory shortcut / tray and OS parity; P6 native GUI; P9 and M9 release gate. The A1 `Proxy:nil` enterprise limitation remains unresolved. Do **not** label P7 closed or announce live deployment based on these offline tests.

## 11. A2.2 independent review findings and bounded repairs (2026-10-08)

An independent Codex 6.1 Sol / High read-only backend review found **0 BLOCKER / 2 HIGH** in the `46fb5400` checkpoint. Both were real transaction-state correctness risks; therefore the original A2.2 checkpoint must **not** be treated as signed off:

1. **CLI/Desktop pairing race during Core restart:** the original `nexusbridge.PairChecked` released its state-home pairing lock immediately after saving, while Desktop still needed to restart and observe Core; CLI `--replace` could save another identity in between and Desktop might falsely mark the old identity as applied. **Repair:** `AcquirePairLock` is shared by CLI and Desktop. Desktop acquires runtime lock, then identity lock, invokes `PairCheckedLocked` without nested lock acquisition, and holds both until restart + trusted active-generation observation completes. `Reconcile` uses the same two-lock order. `TestNexusDesktopMutationsRetainCLIPairLockThroughRestart` tests that competing CLI lock acquisition cannot proceed during either restart and can proceed after completion.
2. **Post-rename fsync failure reported as unsaved:** Unix storage had replaced `device.json` atomically before `Fsync` could return an error. **Repair:** use `ErrIdentityCommitUncertain` to distinguish a visible committed identity with unverified crash durability from a failed pre-commit save. Both normal and checked pairing preserve the newly written identity in that error outcome; Desktop reports `identitySaved=true`, `restartRequired=true`, `nexus_identity_durability_unverified`, and does not restart or imply a reusable one-time code. `TestWriteIdentityPostRenameFsyncFailureIsTruthful` injects directory `EIO` after rename; `TestNexusPairPostRenameFailureCannotClaimUnsaved` verifies the safe Desktop result and redaction.

Independent of those two findings, an additional red-to-green regression found that server-provided `node_id` could previously be persisted even when Next Desktop would reject its format, leaving an invalid pairing state. `nexusbridge.ValidNodeID` is now shared by the pairing transport and Desktop snapshot; `TestPairCheckedRejectsNodeIDDesktopCannotRead` covers slash, newline, and overlength values.

The repaired backend passed affected Go package tests, targeted race tests, vet and git diff checks locally. **Independent Codex 6.1 Sol / High targeted read-only re-review: PASS — the 2 previously reported HIGH are CLOSED, 0 new BLOCKER / 0 HIGH in that bounded remediation scope.** The reviewer verified code and test assertions, but did not rerun tests or perform live UAT. Windows amd64 cross-compilation passed; the subsequent Linux cross-build attempt timed out, so latest Linux compilation is **not confirmed** (the earlier A2.2 checkpoint compiled). Compile success does not establish native feature parity. No Wails binding, installed Next update, real Nexus re-pair or Core restart was done. Phase B/P7 closeout and M9 remain gated.

## 12. Phase B offline Shared UI source checkpoint (2026-10-08)

**Source integration only — installed Next remains unchanged.** Building on the reviewed A2.2 `209c776e` backend, Wails `desktop/shared-poc/main.go` registers a typed `NewNexusService` and its generated `nexusservice.ts` and desktopapi models. `frontend/src/api/desktopApi.ts` exposes typed bounded `nexusSnapshot`, `nexusPair`, and `nexusReconcile` calls. Transport cancellation/timeouts return a safe **outcome unknown** code, not an auto-retry or a false unsaved/success claim.

Connection now owns two tabs, **ChatGPT access** (existing ConnectionPanel unchanged) and **NexusDock** (new ConnectionWorkspace/NexusDockPanel/NexusPairForm). The separate status-first view presents paired and connected as distinct states, safe origin, token-stored boolean, timestamp and Advanced node identifier. Pair and re-pair use backend snapshot `generation` plus capability flags; replacing an existing link requires explicit before/after-origin confirmation. One-time code lives only in a local Vue ref and is cleared on send, cancel, blur, timeout or unmount; no Pinia, storage, logs or CLI invocation. Reconcile is an explicit confirmed Next-only operation, not an automatic restart. Existing ConfirmDialog now restores focus when dismissed. No renderer-selected filesystem paths.

Validation on the development Mac: Wails beta.27 typed bindings regenerated using the normal warmed Go cache, preserving existing bindings; nested Wails Go module includes `github.com/gorilla/websocket@v1.5.3` for the newly imported Nexus implementation; `desktop/shared-poc/go test ./...` PASS (macOS linker deployment-target warnings only). Affected root Go tests/vet PASS. Vue `pnpm typecheck`, original 110 tests and production build PASS, plus 8 Nexus-focused offline tests PASS (total 118). en/zh-Hant/zh-Hans generated catalogs have 100% coverage (790/790, including 48 Nexus keys). The full `go run ./tools/i18n check` still flags existing legacy test strings and PermissionPolicy option literals unrelated to this patch; **do not claim the global i18n checker passes**. No Nexus file remains in the warning list.

**Independent Phase B focused security review:** Codex 6.1 Sol / High read-only code review of the bounded Wails/Vue pairing/navigation/secret-handling slice returned **PASS — no verified BLOCKER/HIGH** against the frozen contract §§2–3. This is source review only, not native GUI or live pairing approval. **Outstanding gates:** Next-only installed Wails/Desktop GUI acceptance, native 320 CSS px/keyboard/picker UAT, real Nexus protocol smoke only against an explicitly authorized disposable endpoint, P6 native GUI UAT, P7 Phase C platform essentials, A1 `Proxy:nil` enterprise compatibility decision, and subsequent M9 integration gate. Read-only Next baseline check saw launched Core PID `44682` listening on `127.0.0.1:8767` and an installed GUI process; there was **no service restart, live pairing or stable AgentDock operation** in this Phase B source pass.
