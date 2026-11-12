# NEXT RC5 Browser Broker native GUI Host UAT (2026-10-22)

**Result: actual RC5 Browser Broker GUI presentation PASS; policy configuration / authenticated external Edge and exact Chrome Profile-CDP connection NOT YET TESTED.** This supersedes the preceding observation that the background NEXT GUI offered no window, but only for GUI presentation. Do **not** claim an external browser connection is healthy or authorized based on page contents.

## Actual native Host and non-interruption evidence

- Installed NEXT's own connector `agentdock_context` reported `0.9.2-rc.5`, and `NEXT Computer Control` observed native AgentDock Next GUI PID `33739`, window `56325` with a real macOS accessibility tree. Current Stable `8765` and NEXT `8767` health returned HTTP 200.
- At first Chrome was foreground (bundle `com.google.Chrome`, PID `45017`). A foreground-forbidden `observe` session read the GUI `總覽` view without changing focus: Core healthy, MCP connection running, auto-update disabled.
- With a separate explicit foreground-allowed `computer_session`, clicked the actual sidebar **瀏覽器 Broker** button, then **重新整理** (read-only). Inspected the updated **native RC5** accessibility tree. No external connectors, CDP endpoints, Edge profiles, permissions, workspace policies, browser leases or browser workers were created, launched or changed by the page navigation.
- Returned the foreground to the original Chrome bundle/PID and independently observed Stable 8765 and NEXT 8767 still returning 200. Any claim of permanently no-focus native UI operation would be false: navigating to the sidebar needed a brief foreground switch, then was restored. The read-only observations themselves did not change focus.

## Exact observed Browser UI evidence

| Native Browser Broker field | Observed value |
| --- | --- |
| Broker | Available; idle / no reserved lease |
| Browser enabled; ACP enabled | Both yes |
| Configured connectors | **0** |
| Configured authenticated external Edge profiles | **0** |
| Configured required external workspace policies (all types) | **0** |
| Configured company-required Edge policies | **0** |
| Connector live health | **Not checked** |
| Managed route | Broker can accept **authorized** managed headless Chrome request, does not prove Chrome currently running or consent granted |
| Company Edge route | Missing required Edge policy; must configure verified external Edge and appropriate workspace allowlist; must **not** fall back to managed Chrome |
| Explicit Chrome Profile / CDP | No evidence of specific profile, live endpoint, consent, or debugging enabled |
| Managed/required/explicit retained leases | 0 / 0 / 0 |
| Authenticated MCP transport owners; ACP owners; workers | 0 / 0 / 0 |
| Active operations; queued operations | 0 / 0 |
| Queue max concurrency / capacity | 4 / 16 |
| Retained stale flag / lifecycle error | false / false |
| Managed pool mode | Exclusive per lease |

- UI text explicitly states Broker availability does **not** certify browser process health, login or consent. Existing backend `internal/app/runtime_browser_desktop.go` derives connector/profile/workspace-policy counts from Core-loaded `r.cfg.BrowserCatalog` and `r.cfg.BrowserWorkspacePolicies`, intentionally never probing the endpoint. It is **not** a fake frontend default. Source configuration keys are `AGENTDOCK_BROWSER_CATALOG_JSON` and `AGENTDOCK_BROWSER_WORKSPACE_POLICIES_JSON`; do not set either by inventing paths, endpoints, policy roots or approval. `internal/browserpolicy/catalog.go` requires canonical loopback websocket endpoint and matching connector/profile; company workspace policy requires authenticated external Edge.
- Post-observation backend `browser_broker(status)` independently returned zero authenticated owners, zero worker/queue, and no retained leases. Read-only source/backend tests: Go `./internal/browserpolicy`, `./internal/config`, `./internal/tool/browser` PASS (browserdesktop has no Go tests); actual Vue route/guidance tests `BrowserRouteObservations.test.ts` and `BrowserReadinessGuidance.test.ts` **31/31 PASS across 2 files**. UI source matches the RC5 binary's `custom/main` source identity `3dcdabe2`.

## Explicit gates remaining

1. **External Edge authorization / configuration:** obtain user-designated authenticated company Edge profile and exact CDP endpoint proof **without** opening debugging or replacing any existing profile. Verify consent/access and correct company-workspace required-edge policy, then test one scoped, read-only company browser navigation with fail-closed alternative. No such configuration/consent is currently observed in RC5, so this is **not** a GUI regression. Do not auto-create a browser connector or policy based on generic repo paths.
2. **Explicit Chrome Profile routing:** only if user specifically requests a concrete Chrome profile and consents, record exact profile/transport evidence and test route without killing Chrome or changing flags. Otherwise keep it deferred.
3. **Crash/failure-path and native background session:** previous real 3-actor Managed Chrome broker overlap accepted; need not replay unless a regression occurs. Unverified external-route lifecycle must not be marked PASS based on empty lease lists.
4. **Stable/NEXT release separation:** user-selected P13/P14 UX deferral remains. Signed RC5 installed is a local, ad-hoc build, not an official/tagged/notarized release. The real signed RC4 Contents backup is retained for recovery until broader Next Host acceptance.

**Conclusion:** Close the original P8 Browser **native operator route UI** visualization gate as **PASS** for installed RC5. Reclassify unverified company Edge / explicit profile connection as configuration-and-consent-dependent Host UAT rather than incorrectly labeling the already deployed UI source-only. No new NEXT code is required to present the current safe route distinctions.
