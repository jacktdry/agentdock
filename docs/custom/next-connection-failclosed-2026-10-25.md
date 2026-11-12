# NEXT RC19 follow-up — Connection Core-outage fail-closed observations (2026-10-25, Asia/Taipei)

## User workflow and source defect

After a user tests the public MCP endpoint or preflights a replacement port, NEXT may lose its authoritative Core Connection snapshot, for example during a momentary Core outage. Existing `connection.refresh()` cleared `snapshot` but **retained the previous `preflight` and `publicEndpoint` observations**. `publicEndpointState` preferred the cached reachable endpoint even when no current snapshot existed and otherwise fell back to **`not_configured`**, a potentially untruthful state when configuration was merely unavailable. A delayed preflight result could then overwrite the latest Core read error or attempt to inspect the now-null snapshot. Separately, if a mutation was rejected and the subsequent Core read failed, `operationError` prioritized the mutation rejection and the UI obscured the independent read error.

This was an observable state-model mismatch in installed RC19 source, not evidence that the live Core had actually failed. **Do not intentionally shut down or misconfigure NEXT or Stable to reproduce it in a production GUI.**

## Direct user-facing source fix

- `stores/connection.ts`: invalidate `snapshot`, `preflight`, and `publicEndpoint` together upon a failed Core snapshot read, whether returned as an API error or thrown. The verified public endpoint state becomes `unknown` when no authoritative snapshot exists, **never a cached reachable state nor a false `not_configured`**.
- Port preflight pins the snapshot revision at dispatch. Its late result is ignored if the current snapshot has disappeared or its revision changed, **before** interpreting a possible error; late thrown failures likewise cannot overwrite the more current Core read error after losing that snapshot. The next successful snapshot does not resurrect pre-outage observations even if it happens to return the same revision.
- Existing `actionError` persistence remains intact until the next deliberate mutation. `ConnectionPanel` and `OverviewPanel` render the independent Core read error **as a second `OperationStatus` only when a separate action rejection also exists** and the read is no longer busy. No untrusted platform diagnostic text is shown. Both use the existing sanitized `errorMessage` and localized UI, without inventing new controls, native apps or backend APIs.

## Validation

- Connection Store regression tests prove invalidation of a previous reachable endpoint and port preflight after a returned Core error, no reuse after recovery with the same revision, rejection of a delayed port preflight after a *thrown* Core read failure, and stale preflight API errors failing to overwrite the latest Core read failure.
- `OverviewPanel` SSR regression verifies simultaneous visible mutation/read errors with no unverified endpoint URL.
- **Focused 2 test files / 16 tests PASS; complete frontend suite 25 files / 184 tests PASS / 0 FAIL.** `pnpm exec vue-tsc --noEmit --pretty false` PASS; `pnpm exec vite build --mode production` PASS (189 modules). Vite emits a preexisting nonblocking large-chunk warning and Vitest Node emits localStorage/browser preview warnings; none represent a product regression.

## Acceptance boundary

**Source-only Connection resilience PASS.** Live installed NEXT remains **0.9.2-rc.19 / 8767**, distinct Stable AgentDock **1.1.0 / 8765**. No app rebuild, new RC20, GUI focus, actual service restart, browser login, Edge attach, user interaction, or license/permission changes were part of this task. Real installed Connection error + page navigation interaction is **still Host OPEN**. Native Tray's AX accessible name/menu action and authenticated Native Codex MCP process ownership B0–B6 remain independent OPEN gates (see [RC19 native Tray contrast](next-rc19-tray-ax-contrast-2026-10-24.md) and [Codex MCP B0–B6](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)). Do not merge, push, tag or publish from this source-only checkpoint.
