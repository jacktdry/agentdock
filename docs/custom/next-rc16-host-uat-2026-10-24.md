# AgentDock NEXT RC16 — Installed Host UAT (2026-10-24, Asia/Taipei)

## Scope / final verdict

**RC16 installed Host acceptance for company Edge's two independent negative policy checks: PASS.** Only one official user-authorized Core-owned Edge transport was opened, reused for three separate direct leases, and formally disconnected once. The prior RC15 cleanup-error regression is fixed **on real Edge**, not merely in fixtures. Combined with [RC15 fresh-acquire Host PASS](next-rc15-host-uat-2026-10-24.md), the **positive browser lease lifecycle and both negative policy gates are now proven**. This is **not** whole-App DoD or release approval: GUI/Tray, crash/recovery, cancel and live injected rollback remain OPEN.

- Source commit frozen to `d51f1bc4e327a7d198a6e8b91c1aff7e5e849020`, package version `0.9.2-rc.16`, build `17.0.0`, `release_ready=false`; candidate `dist/next-shared-arm64.pQhkiC/package/`. Doc-only later commits are **not** different binary sources.
- RC16 install only replaced `~/Applications/AgentDock Next.app`. NEXT `127.0.0.1:8767/healthz` returns rc.16. Original AgentDock Stable `127.0.0.1:8765/healthz` returns **1.1.0**, never modified. The installation preserved **RC15 rollback** `~/Applications/.agentdock-next-replacement-4n1f8_oy/AgentDock Next.app`, with verified binary version; earlier RC13 rollback also retained.
- Installed candidate 24 regular files SHA-256 equal to the sealed candidate, `codesign --verify --deep --strict` PASS. GitLab MCP grant survived restart: `gitlab-company` refresh `ready`, **35 tools**. Predeploy Browser idle (all counters 0), ACP/Computer no active work.
- Source regression: **1,706 PASS, 7 opt-in SKIP, 0 FAIL**, targeted race **56 PASS**, Go vet and diff check PASS; macOS arm64 App/ZIP/DMG same SHA files, ad-hoc codesign and DMG checksum PASS. [Candidate / source audit](next-rc16-edge-policy-denial-2026-10-24.md).

## One formal installed Edge Host test

Private test evidence (only on Mac; not committed): `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc16-negative-result.json` and `rc16-negative-foreground.jsonl`.

| Exact step | Result |
| --- | --- |
| One formal `connect_external` | **PASS 4.754s**, no implicit retry |
| Lease 1 `direct_acquire` | **PASS 4.321s** |
| Lease 1 forbidden `direct_evaluate(() => 1)` | **Explicit `ACTION_INVALID` PASS**, not a generic `transport fence` failure; no JS executed |
| Same lease 1 navigate `https://oprueba.com`, AX snapshot | **PASS 4.464s / 3.704s** after refusal, `RootWebArea` and O'Prueba |
| Lease 1 `direct_release` | **PASS 3.974s**, `complete`, `cleanup_error=null` |
| Independent lease 2 `direct_acquire` | **PASS 4.589s** |
| Lease 2 attempt `https://unapproved.oprueba.com` | **Explicit `ACTION_INVALID` PASS**, no policy bypass |
| Same lease 2 navigate approved Oprueba and AX snapshot | **PASS 3.850s / 3.599s** after refusal |
| Lease 2 `direct_release` | **PASS 4.108s**, `complete`, `cleanup_error=null` |
| Fresh third lease `direct_acquire` after both releases | **PASS 4.806s**, retained single authorized Edge transport |
| Lease 3 `direct_release` | **PASS 4.076s**, `complete`, `cleanup_error=null` |
| One `disconnect_external` | **PASS 0.002s**, before/after Browser `idle` and 0 owner/lease/worker/operation/orphan/authenticatedTransportOwners |
| Foreground / held socket | **134 samples**, all `com.google.Chrome`; no NEXT/Edge focus change observed. After formal disconnect, **0 Core TCP to Edge 9222** |
| Script aggregate | **`ok=true`**; 2/2 policy denials and 3/3 clean releases |

Use the actual RC15 live [dual A/B sibling/fresh, Oprueba PNG 302,829 bytes](next-rc15-host-uat-2026-10-24.md) as independent complementary acceptance; RC16's shorter run did not repeat HomiPlay or take a screenshot and **must not claim those were individually rerun on RC16**.

## Remaining separate gates before release

1. Native GUI window/tray/MenuBar controls, user-visible status, accessibility and no-focus on real menu interaction — **not a Host PASS on RC16**.
2. Cross-session cancel, abnormal termination, detached/admitted worker recovery and orphan cleanup under fault injection — **not a Host PASS**.
3. NEXT-only failed-update live rollback/injection (not merely presence of a verified rollback bundle) — **not a Host PASS**.
4. Any separate GitLab REST `read_api` grant: GitLab MCP `mcp` and 35 tools PASS, but REST grant **not inferred**.

**Do not merge custom/main, push, create official release or claim complete product DoD** until those gates have explicit tested evidence and an approval decision. Leave the now-idle Edge transport disconnected and the original Stable agent untouched.
