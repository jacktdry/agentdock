# NEXT UX-S1 — Connection operation feedback survives passive refresh (2026-10-24)

## User workflow and observed source gap

A user requests an explicit **Connection restart/stop/configuration mutation**. The Core can reject or fail that action but still successfully provide a fresh connection snapshot. On the existing NEXT frontend, `runMutation` correctly retained the mutation failure in `actionError`, but **switching to Overview or returning to Connection** calls `connection.refresh()`, which unconditionally cleared `actionError` even though the user had not issued a new action. Moreover Overview's `OperationStatus` rendered only `connection.error` (the read error), not the action error. Result: a failed user action could appear to have succeeded or simply disappear after page navigation.

## Narrow source fix

- `stores/connection.ts`: passive `refresh()` no longer resets `actionError`. A **new deliberate mutation** still resets the old action error before issuing the request. An action ACK alone remains insufficient to claim success: `completed` requires a new authoritative snapshot. The read error channel still updates independently.
- `components/overview/OverviewPanel.vue`: Connection's status area renders the already existing `connection.operationError`, like the detailed Connection view, instead of `connection.error` alone. No new component, hidden retry, native foreground action or API endpoint.
- **Fail-closed on reads remains unchanged**: if a refresh fails, `snapshot=null` including sensitive public endpoint/port data; the action failure may still be displayed as history but the UI cannot present stale connection data as verified.

## Validation

- Added Connection Store tests: rejected mutation → passive refresh with successful read retains original rejection; next explicit successful mutation clears it; failed snapshot read after rejection clears sensitive snapshot but preserves both distinct error channels.
- Added SSR Overview tests in zh-Hant: rejected mutation visible alongside the successful snapshot; standalone snapshot error visible while unverified public URL is absent.
- Focused tests: **2 files / 12 tests PASS**; `pnpm exec vue-tsc --noEmit --pretty false` PASS.
- Full frontend regression: **25 files / 180 Vitest tests PASS**, `pnpm exec vite build --mode production` PASS (189 modules), `git diff --check` PASS. Build notes include existing Node test environment localStorage warning and chunk-size notice, not new blocking errors.

## Acceptance boundaries

**Source-level UX-S1 regression PASS.** Running native NEXT remains **0.9.2-rc.18** on port 8767, separate Stable 1.1.0 on 8765; no new candidate, no installation or GUI restart. Real GUI page navigation and failed-mutation display have not been re-performed on an installed build, so native Host UI gate stays OPEN. Historical 2026-10-14 genuine P8 ChatGPT/Codex/AGY shared Chrome PASS remains accepted; opt-in shared mode remains off by design, not a regression requiring new integration tests. Do not merge, push, tag or publish in this source-only checkpoint.
