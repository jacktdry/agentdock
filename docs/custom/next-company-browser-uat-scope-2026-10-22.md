# NEXT — Oprueba / HomiPlay Browser UAT and GitLab API-first policy (2026-10-22)

**User-approved intent:** use the **currently running Microsoft Edge company-access profile** for the `oprueba.com` and `homiplay.io` website families, especially visual UAT and web-only operations. GitLab source/pipeline/MR operations should use authenticated **GitLab API / existing token-backed connector** as the first path, rather than using a browser. This is an operational scope decision and **not a grant to access every subdomain or to mutate a GitLab project without task-level authorization**.

## Observed connected capabilities (important revision)

- Original Stable Mac-Dev advertised a **ready `edge-devtools-profile-ws` dynamic MCP**. Calling its `list_pages` **succeeded**, returning a GitLab homepage at `https://gitlab.oprueba.com/`. This proves that this *existing MCP connector* can enumerate at least one page in the designated Edge; it does **not** prove NEXT Browser Broker's native route catalog is configured, nor does it confer unrestricted page permissions. No page snapshot, cookie extraction, browser automation, or write action was performed.
- A direct browser-level probe `http://127.0.0.1:9222/json/version` returned HTTP **404**, although the running Edge executable owned that listener. Therefore **do not equate the 404 to all Edge DevTools access being unusable**. It could be a different transport/route/proxy. Exact canonical `/devtools/browser/<id>` WebSocket identity required by NEXT Broker remains unverified.
- The installed NEXT RC5 `agentdock_context` showed only `memory` and `codebase-memory` dynamic MCP servers; querying for `edge-devtools-profile-ws` through NEXT returned `MCP_SERVER_NOT_FOUND`. Thus existing **Stable** browser MCP and native **NEXT** Browser Broker are different integration paths. Do not copy Stable tokens, private Profile data or server configuration into NEXT. Reuse a supported, authorized connector integration if possible rather than introducing a duplicate CDP session.
- Original Stable's **existing `gitlab` dynamic MCP**, with no token printed or copied, returned valid **read-only `get_project` responses** for `official_site/oprueba` (project 251), `official_site/homiplay` (project 252), and `oos-fwa/homiplay` (project 247), all hosted at `https://gitlab.oprueba.com/`. This establishes that project information can be retrieved via the connector. It does **not** prove every Token scope is enabled or that NEXT has the GitLab MCP registered. Prefer GitLab API via this connected, scoped tool where available; do not scrape GitLab UI to do ordinary status checks.

## Candidate company workspaces (actual checked-out repositories)

The following local Git repositories were verified from their existing remote URLs and working root. These are the narrow candidate roots for policy mapping, not a blanket allowance for `/Users/wei/opruebaCode` or unrelated sites:

| Local worktree root | GitLab project | Intended website origins |
| --- | --- | --- |
| `/Users/wei/opruebaCode/official_site/oprueba` | `official_site/oprueba` | `https://oprueba.com`, `https://www.oprueba.com` |
| `/Users/wei/opruebaCode/official_site/homiplay` | `official_site/homiplay` | `https://homiplay.io`, `https://www.homiplay.io` |
| `/Users/wei/opruebaCode/homiplay` | `oos-fwa/homiplay` | `https://homiplay.io`, `https://www.homiplay.io` |
| `/Users/wei/opruebaCode/official_site/content-cms` | `official_site/content-cms` | Both main site origin pairs above, as relevant to the CMS UAT task |

**Exact-origin security:** these strings are proposals for a minimal first allowlist, not automatic matching of all `*.oprueba.com` or `*.homiplay.io` subdomains. The local code also references `https://beta.oprueba.com`, `https://orphelink.oprueba.com` and `https://demo.orphelink.oprueba.com`, but they are **not part of the default UAT allowlist** until a task needs and authorizes them. `https://gitlab.oprueba.com` is not a normal website-UAT origin: it is an **exceptional browser-UI origin**, only for a web-only GitLab action that cannot be done via the authorized API tool. Browsing it does not replace Token-first GitLab operations.

## Selection and non-interference rules

1. **Default website UAT:** use the designated company Edge through the existing, authorized browser integration when available; preserve the current Edge Profile, don't relaunch with new debugging flags, don't overwrite login state or open/inspect unrelated tabs. For a UAT that creates pages, use a scoped page and clean up only resources it owns. Take no destructive CMS action without the specific task's authorization.
2. **Default GitLab:** use connected `gitlab` API MCP (Token handled by the connector; never expose it, copy it into code/config, or paste it into browser URL). For pipelines/MRs/branches, query API first. If the API lacks a necessary UI-only operation, perform an explicit scoped browser operation against `https://gitlab.oprueba.com` in designated Edge with approval, not a general browser fallback.
3. **Native NEXT Broker readiness:** current installed NEXT RC5 native Browser catalog remains 0 connected profiles/policies. Its **source** HEAD `8ef0d9d5` has a safe, Next-only read-only policy-file loader; it is **not yet installed** in RC5. Do not write live `${HOME}/.agentdock-next/browser-routes.json` until the actual connector-to-Broker transport/canonical endpoint is resolved and per-root origin policy is validated. No global `launchctl setenv`, no manually patched signed `.app`, no copying original AgentDock/Edge secrets.
4. **Separation and fail-closed:** company domain routing must not silently fall back to Managed Chrome when Edge/authorization is unavailable. Avoid adding broad wildcard origins or granting the content-CMS root unrestricted access to the whole enterprise. Explicit profile/CDP health, consent and per-task scope still govern connection and operations.

## Next actionable gate

- Discover whether the working `edge-devtools-profile-ws` transport can be **safely registered within NEXT** under its own separate authorization/ownership boundary. This may be less work than trying to infer a raw canonical CDP WebSocket endpoint from port 9222. Explicitly confirm how the source connector was created and its ownership; do not auto-copy credentials.
- If native NEXT Broker needs a canonical endpoint, obtain it through its supported connector/provider metadata, not through guessed `/json/version` paths. The existing 404 is inconclusive about the functional MCP route.
- Only after those checks: apply the **exact worktree/HTTPS origin pair(s)** relevant to the requested UAT using the Next-private config admission mechanism in a newly versioned Next-only RC, then test one read-only site scenario. GitLab stays API-first, with browser exception per task.

**No live Edge website actions, no GitLab state mutations, no NEXT reinstallation or Core restart took place while recording this scope.**
