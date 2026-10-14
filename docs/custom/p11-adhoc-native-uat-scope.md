# P11 — macOS Next ad-hoc native install and recovery validation

> **Scope decision (2026-10-14, latest):** The owner has **no paid Apple Developer Program account**, so **Developer ID Application signing and Apple Notarization are explicitly not in P11**. Do not ask for/purchase an account, provision certificates or issue notarization requests. These omissions are accepted scope decisions and **must not remain P11 completion blockers**.

## User-facing goal

Make the **existing Next-only macOS arm64 ad-hoc bundle** safely usable as a *local/test installation*, confirm the installer, service lifecycle, manual local upgrade and failure recovery without disrupting the Stable AgentDock connection or the user's daily work. Next: `dev.dropabit.agentdock.next`, `~/Applications/AgentDock Next.app`, `~/.agentdock-next`, port `8767`. Stable port `8765`, installed Stable app, home, services, browser and running user sessions are **off limits**.

## What P11 will test / implement (as authorized separately for live changes)

1. **Next-only install/launch/reinstall:** ad-hoc bundle `codesign --verify --deep --strict`, bundle/helper identities, arm64, app version/Build Info, downloaded/local payload SHA-256; Next-scoped installer/deployer and service registration, registration errors and idempotency. Record whether Gatekeeper/manual authorization is required; never suppress OS permission prompts or alter system trust to bypass warnings.
2. **Next-only controlled local version replacement:** verified old/new packages from immutable candidate commits, Next-only atomic swap or staged deployment, safe Core `8767` listener ownership, and GUI/runtime recovery. Do not replace the existing Stable app or touch 8765. These exercises use **manual/local approved deployment**, **not** the GUI's certificate-bound automatic updater.
3. **Failure injection, rollback and restart:** missing/corrupt payload, failure before/after swap, failed launch/health, interruption/retry, backup owner and cleanup, only files/services demonstrably owned by Next. No simulated success or forced shutdown of unrelated processes. Prefer offline fixture validation first; real Next-only installation/restart is a distinct gated acceptance.
4. **Native macOS UI/functional smoke:** launch/quit, menu bar/Tray, Core health/status timestamps, Connection, ACP/MCP and browser paths as permitted, localization and expected "NexusDock developing" read-only behavior. Validate truthful state; run unobtrusively without stealing focus or creating repetitive consent dialogs.
5. **Source/release provenance:** version and full Git commit, checksum and next-only archive identity; real commands/logs/exit codes. CI remains read-only before publication and Hosted PR CI must not be described as PASS if not executed.

## Intentionally not done

- No **Developer ID Application** certificate, paid Apple Developer account, Notarization, stapling or public distribution assurance.
- No disabling/hollowing out AppKit Gatekeeper, code signing checks, macOS trust settings, Next runtime ownership checks, updater's source-certificate continuity, or Stable/Next isolation merely to make ad-hoc updates work.
- No public Tag/GitHub Release, no official update-feed activation, no automatic public upgrade, no Stable retirement.
- No Windows native, WSL/VM acceptance or Windows Setup; under the owner's previous decision Windows remains *source/cross-build feasibility only*.

## Critical existing product constraint (real implementation, not an arbitrary test gate)

`internal/updateplatform/darwin.go` `NextSourceSigningRequirement` rejects a Next signer designated requirement without a certificate identity; `validateMacOSSigningContinuity` rejects the legacy ad-hoc cdhash exception for `AGENTDOCK_DESKTOP_VARIANT=next`; `internal/updateengine/identity_darwin.go` likewise requires certificate-bound signer metadata. **Therefore the existing production Next GUI/automatic updater cannot complete a live update with these ad-hoc artifacts**. Do not waive this security behavior, claim an ad-hoc automatic update test passed, or silently reclassify manual deploy as updater acceptance.

P11 within this no-paid-account scope can complete **Next-only local install, manual update/rollback and source/fixture recovery**. If the owner later requires end-user automatic updates *without Developer ID*, that needs a **separately reviewed trust/distribution design** and explicit authorization; it is not solved by the P11 scope exception. Certificate-bound auto-updater execution is intentionally **deferred/disabled**, not passed.

## Practical P11 acceptance gates

- Pass actual Next-only macOS native install/launch/status check and local upgrade + injected rollback/recovery when separately authorized. Stable PIDs/port/app files/state unchanged.
- No surprise permission/Keychain/focus prompts; cancel/unknown states are surfaced truthfully, with safe failure on identity ambiguity.
- Record the exact native/App/package and CLI test evidence. Platform prompts cannot be bypassed. Source-only fixtures never count as native use.
- Conclude **P11 local ad-hoc acceptance** separately from **public distribution and automatic updater acceptance**, both of which remain explicitly outside the current milestone.

This decision supersedes earlier P11 language requiring `macOS Developer ID signed package` or `certificate-bound GUI updater` to close the local-testing scope; it does **not** retroactively alter existing P10 build evidence or lower cryptographic safeguards.
