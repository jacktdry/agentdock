# AgentDock NEXT — Active Backlog / Single-page Handoff (2026-10-21)

> Canonical working branch: `custom/main`. **Installed/runtime evidence and source-only proofs are separate.** This short handoff overrides stale progress assertions in older chronological milestone documents; it does not erase their historical evidence.

> **P13 / P14 user decision 2026-10-22:** The user prefers the original compact collapsible Task card and considers current card noise acceptable. Large live polling cards, fixed monitoring, separate Task Monitor and P14 suppression are now **deferred**, not P0 or release blockers. No P13/P14 experimental source merge. See [current scope decision](p13-p14-ux-scope-decision-2026-10-22.md).

> **P8 correction 2026-10-21:** Full *installed* three-genuine-actor shared-Chrome overlap and owned release **passed on 2026-10-14**; earlier Pending text was stale. Read [current P8 follow-up](p8-followup-checkpoint-2026-10-21.md) for remaining Edge/no-focus/crash gates.

## User workflow and boundary

NEXT is the separately installed development control surface for ChatGPT + real Codex/Antigravity ACP workers sharing browser/computer/MCP resources, with truthful Vue UI and lifecycle ownership. Stable AgentDock is **Maintenance-only**. All NEXT source and deployment mutations use **stable Mac-Dev**, never self-modify via NEXT. Stable port `8765`, app, state, memory and browser profile are off-limits; NEXT is port `8767` with isolated identity/state.

**Observed 2026-10-21:** original and NEXT `/healthz` each returned HTTP 200. The NEXT seven-day stale AgentDock-managed ACP mapping retention and Orca CLI launchd PATH fix were deployed in a local `0.9.2-rc.4` candidate on 2026-10-21; managed sessions left after first sweep: **8** (157 mappings pruned). Native Codex/Antigravity transcripts were not deleted. This proves running health / first sweep, **not** 24-hour sweeper recurrence or complete live browser workflows. See [retention evidence](../acp-session-retention.md).

## Active priorities

| Priority | Track / owner branch | Next smallest user-visible outcome and acceptance | Status / gate |
| --- | --- | --- | --- |
| **P1** | **P8 Browser Broker follow-up + Computer Use**; main product line | Preserve the verified 3-actor one-Chrome flow; separately validate actual macOS foreground/no-popup behavior, truthful non-default shared-mode status, restart/crash owner recovery, and browser/computer resource lifecycle without affecting stable | **2026-10-14 installed NEXT 8767 three real actors (ChatGPT + Codex ACP + AGY ACP) concurrently shared one Chrome, held isolated leases, and safely released: PASS**. Shared mode was intentionally restored to exclusive afterward. Crash/SIGKILL, consent-grounded Edge, native no-focus remain **open**. [Evidence §52](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#52-2026-10-14-p8-installed-next-three-real-actor-shared-chrome-overlap--owned-release-uat-pass) |
| **P0** | **P8 route/CDP operator controls** | Show actionable Browser route status and authorized Edge/explicit Chrome Profile/CDP configuration; company workflow must require the selected authenticated Edge, general background tasks use managed headless Chrome, exact Profile requests cannot silently fall back | **2026-10-21 source-only:** Browser UI now separates Managed/Company Edge/explicit Chrome Profile/CDP guidance and missing vs unverified definitions, with conservative unknown/stale handling and en/zh-Hant/zh-Hans text. **Not yet installed/deployed**. No verified Edge connector/auth attestor or safe write controls; real route/consent UAT remains open |
| **Deferred / observe** | **P13 Task / P14 MCP card UX**; experiment worktrees retained | Preserve original compact Task card + one logical Task ID. No pinned/large live card or separate monitor and no proactive suppression; reopen only on actual regression or user request | **2026-10-22 explicit UX choice:** isolated P13 candidate proved live updates but worsened scrollback monitoring; original card is preferred; P14 noise currently acceptable. Do not merge or deploy experiment. [Current decision](p13-p14-ux-scope-decision-2026-10-22.md) |
| **P1** | **UX-S0..S5 Shared UI and native Tray** | Complete initial/stale/unknown semantics, precise action-feedback vs snapshot-error, OAuth status scoped by server+generation, stable loading regions, concise zh-Hant text, a11y/keyboard, and real macOS Menu Bar state/actions with timestamp | Some source fixes merged 2026-10-14; broad installed GUI/focus/shift/Tray UAT and narrower remaining items still pending |
| **P1** | **NEXT Core Skills + Codebase Memory**, `feature/next-skill-cbm-integration` | Reconcile 2 unmerged commits: packaged core skills, isolated memory-lifecycle skill and installer tests; test installed NEXT capability without writing stable/global memory | Source branch remains separate. Review overlap with `fix/next-orca-cli-discovery-20261017` before cherry-picking |
| **P1/P2** | **Cross-session MCP native process ownership** | Bound ownership/lease/generation and safe cleanup in a real admitted workflow; never kill native Codex/AGY transcripts or foreign MCP processes | B0–B6 source and authenticated isolated UAT exist; production activation/operator UX remains pending, larger CBM executable lifecycle debt deferred |
| **P2** | **P7 Platform Essentials** | Validate local Diagnostics/open-folder native actions and truthful startup/log states; treat NexusDock as read-only **開發中** | Nexus real pairing/re-pair/deployment intentionally deferred; don't block independent work |
| **P2** | **P11 native ad-hoc UAT** | Finish safe macOS first-install / Menu Bar / essential real action checks, where possible without damaging the already installed Next; keep evidence truthful | Existing NEXT-only local manual update + post-swap rollback and 12-page non-destructive GUI UAT passed 2026-10-14, not full native parity |
| **P3 / gated** | **P10 / M9 Release** | Freeze UX and accepted product workflows, re-run same-commit App/ZIP/DMG validation and then request authorization to publish | P10 local prepublish engineering completed; **no formal Tag, Release, feed, signing/notarization or public auto-update**. Windows remains **source/cross-build feasibility only** per user decision |

## Already closed — do not reopen merely because an old worktree exists

P4 ACP Manager, P5 MCP Management, P6 Plugin Management GUI closeout, M8 Permission/Approval, **P8 three genuine ChatGPT/Codex/AGY actors sharing one real Chrome with isolated leases and safe per-actor release (installed NEXT 2026-10-14, §52)**, core UI source changes from 2026-10-14, NEXT Orca CLI detection deployed 2026-10-18/21, seven-day managed ACP retention rollout 2026-10-21. P8 Edge/consent/crash/no-focus are separate open gates; do not reopen the completed triple-actor test by misreading older checkpoints. Explicitly distinguish historical checkpoint text claiming P6 GUI pending from the later [2026-10-12 P6 GUI closeout](pre-m9-plugin-management-roundtable.md).

## Outstanding Git integration triage (do not auto-merge)

- `feature/next-mcp-card-ux` / `feature/next-p13-single-owner-integration-20261022`: P13/P14 historical UX experiments deferred and unmerged. `feature/next-skill-cbm-integration` has been reconciled into `custom/main` at `4abf6280` + `6540b58f` (source-only; installed rollout pending). Keep historical worktrees for later scoped closeout.
- `feature/m6-acp-tests`: 8 tracked modifications and 9 untracked Browser/ACP files. Dirty, preserved; compare implementation against later M6/P8 before resolving.
- `feature/p5-mcp-oauth-fence`: 5 tracked changes. Dirty, preserved; compare with newer OAuth authority fixes before resolving.
- `feature/activity-center`, `feature/p5-mcp-desktop-api`, `feature/p5-mcp-ui`, `fix/next-orca-cli-discovery-20261017`, `docs/upstream-v101-adoption`: clean but still patch-distinct from `custom/main`; inspect each before integration/archival.
- `feature/p10-release-migration`: clean but Git worktree marked `locked initializing`; do **not** remove or unlock without proving why, and preserve local RC validation artifacts.
- `feature/p11-adhoc-native-uat`: ancestry merged; retain local RC/rollback artifacts and validation evidence for now.
- Stable `maintenance/original-v1.1.0-20261021` is a separate, protected maintenance line; do not mix with NEXT.

## NEXT release policy / work admission

Do not publish official NEXT Tag/Release, enable production feed, loosen certificate-bound updater rules, introduce a paid Developer ID requirement, or build an unvalidated Windows installer without explicit new authorization. P10 source prepublish engineering is complete; P11 manual **ad-hoc** acceptance is distinct from certificate-bound public updater acceptance. Avoid new milestones/daemons/VMs just to satisfy synthetic fixtures. Follow repository `AGENTS.md` product-first acceptance contract.

Read: [roadmap](roadmap.md), [P8 current route decision](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md), [UI/UX roundtable](next-uiux-roundtable-2026-10-14.md), [P11 UAT](p11-native-gui-and-rollback-uat-2026-10-14.md), [prepublish engineering](p10-local-prepublish-closeout-2026-10-14.md), and [worktree closeout](worktree-consolidation-2026-10-21.md).
