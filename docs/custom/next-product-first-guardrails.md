# AgentDock Next — Product-first prioritization and anti-drift decision

> Accepted 2026-10-13. This updates work selection and interpretation of P8 test evidence; it does not weaken stable/Next ownership, M8 permission/approval, user browser consent or release safety gates.

## Why this exists

The goal of Next is a practical AI work-control hub: ChatGPT can hand off to Codex/Antigravity, reuse MCP and Browser/Computer broker resources, stay observable through shared Vue UI, respect permissions, finish tasks and release owned resources without interrupting the desktop. Next is independently installed because breaking the original AgentDock during development previously disconnected the only ChatGPT control channel. Separate state/port/tunnel/updater are a means to safe iteration, **not a requirement for ever more isolated artificial test environments**.

## Task-admission contract

Every new work item states: concrete everyday workflow or pain; change the user can observe; smallest change to an existing path; measured acceptance including 'focus steal=0 / unwanted prompt=0 / no user-profile disruption'; explicit non-goals. Fix an actual customer blocker and retain proportionate S1/S2 protections. Prefer live behavior and short repeatable smoke over a new layer of scaffolding. Tests and diagnostic schemas count as supporting evidence, not shipped behavior. A product task with an unrelated test-environment blocker may proceed along another independent workstream.

Do not introduce a new milestone, VM, independent user identity, protocol emulator or additional security control simply because a synthetic/offline test cannot mimic macOS. Investigate such issues only when needed by a legitimate product execution path. No broad user HOME or Keychain access is implied; normal app APIs and already granted user consent are different from pretending everything must work without OS services.

## P8 current truth and next useful acceptance

- Already proven in source/tests: Broker route/lease lifecycle; genuine isolated headless Chrome pool with separate pages/one main browser under three synthetic owners; Codex ACP adapter can pass Browser+Computer MCP endpoint configuration to a fake native backend; independent test MCP SDK can handshake against those scoped endpoints.
- Not yet proven: one *real* ChatGPT caller and two *real* ACP actors operating across the Broker with a single managed headless Chrome, no focus/modals/extra managed process, correct lease cleanup; user-authenticated exact Edge/Chrome profile route acceptance; crash/restart resource lifecycle end to end. Source tests do not count as deployment or live UAT.
- Known optional fixture blocker: isolated native Codex 0.160.0 app-server initializes but `thread/start` fails to sync macOS managed preferences under restrictive Seatbelt. This is NOT evidence that normal ACP usage or Broker tokens are broken. Freeze this research unless it blocks normal-use integration; keep test result unqualified, never force a PASS.

Near-term sequence: (A) user-facing Browser readiness diagnostics anchored in existing Core snapshot; (B) bounded real headless background workflow acceptance through the existing Broker with current Next control path (no unauthorized profile access); (C) exact user-profile routes only with valid consent and explicit evidence; (D) finish remaining Pre-M9 and native/release gates. Do not require 100% of all speculative adversarial cases before delivering independently useful improvements.

## How to tell a future session it drifted

Reject task proposals that mention mostly 'stronger fake proof', 'another supervisor layer', 'more isolated app-server fixture' without identifying what the user can newly accomplish or what observed bug prevents that. A healthy checkpoint says: **user workflow → accepted evidence → no-interruption check → explicit remaining gap → next user-visible slice**. Refer to repository `AGENTS.md` for rules automatically read by AI developers.
