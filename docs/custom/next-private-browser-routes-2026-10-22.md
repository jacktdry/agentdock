# NEXT — private Browser route catalog admission (2026-10-22)

**State: source-only candidate, not installed or enabled.** User designated the **currently running** Microsoft Edge instance as the Edge profile that can access company resources. Do not require an unrelated profile. Its running PID `86038` owned loopback TCP listener `127.0.0.1:9222` when checked, but the *browser-only* GET `/json/version` returned **HTTP 404**, so no canonical WebSocket endpoint, login, browser profile, page or consent was validated. No `/json/list`, cookies, tabs, site requests or websocket connection were attempted. macOS denied access to Edge/Chrome profile directories, and that permission boundary was respected.

## Real workflow / smallest product change

Existing NEXT Browser Broker operator GUI is a read-only diagnostic surface; it cannot configure the catalog or workspace routing. `internal/config/browser_policy.go` previously accepted `AGENTDOCK_BROWSER_CATALOG_JSON` and `AGENTDOCK_BROWSER_WORKSPACE_POLICIES_JSON` **only in the process environment**. The signed NEXT LaunchAgent requires its exact `AGENTDOCK_DESKTOP_VARIANT=next` environment without arbitrary extra keys, and modifying user-global `launchctl setenv` could affect Stable. The goal of this change is to support an owner-controlled NEXT-only policy file **without** modifying signed app contents, original AgentDock environment, profile data or running Edge.

## Candidate behavior / authorization boundary

- macOS **NEXT non-stdio Core only** may load `${HOME}/.agentdock-next/browser-routes.json`. This is read-only at startup. It is **never created automatically**; absence leaves current empty catalog and standard Managed route behavior untouched. Stable AgentDock and non-macOS variants do not load this file. NEXT with an explicit nondefault `AGENTDOCK_HOME` skips the real user's file to avoid cross-service authority leakage.
- The directory must be a real directory **owned by the current user and permission 0700**. The optional file must be regular, owner-owned **0600**, maximum 128 KiB and **not a symlink**; open the directory using `O_NOFOLLOW|O_DIRECTORY` and the file via `openat(..., O_NOFOLLOW)` to avoid path substitution. Invalid permission, symlink, unknown field, malformed/trailing JSON or unsupported schema fails closed rather than treating the route as healthy.
- Schema is `{"version":1,"catalog":{"profiles":[...],"connectors":[...]},"workspace_policies":[...]}`. A valid configured file must include at least one connector and explicit policy. NEXT rejects combining this file with route configuration supplied by process environment. It reuses existing `browserpolicy.NewCatalog`, `NormalizeWorkspacePolicies` and `ValidateWorkspacePolicies` rules: Edge company route requires authenticated external Edge Profile, a matching connector, canonical WebSocket loopback browser endpoint, existing absolute workspace root, explicit required external routing, and a nonempty list of **exact canonical HTTPS allowed origins**.
- **Configuration is intent, not authorization.** A saved connector is not marked healthy, verified, authenticated or consented by this change. Existing Broker admission rules still block an unverified or wrong-profile company route instead of substituting Managed Chrome.
- No GUI writer, background polling daemon, new auto-generated browser process or global environment edit. No machine-local real company URL/profile root has been inferred or stored. The file is not created until authorized exact workspace roots, allowed origins and a functioning verified CDP browser endpoint have been established.

## Example shape only — synthetic paths and domains are **not** live configuration

```json
{
  "version": 1,
  "catalog": {
    "profiles": [{"id":"company-edge","browser":"edge","class":"authenticated-external","user_data_dir":"/USER-APPROVED/EDGE/PROFILE"}],
    "connectors": [{"id":"edge-cdp","profile_id":"company-edge","driver":"chrome-devtools-mcp-ws","endpoint":"ws://127.0.0.1:9222/devtools/browser/VERIFIED-BROWSER-ID"}]
  },
  "workspace_policies": [{"root":"/USER-APPROVED/COMPANY/WORKTREE","policy":{"class":"company","route":"required-authenticated-external","required_connector_id":"edge-cdp","required_profile_id":"company-edge","allowed_origins":["https://example.com"]}}]
}
```

Example paths/domains are synthetic, **never** insert these in a running NEXT. The exact browser endpoint **must be proven** (current `/json/version` is 404). An owner must select approved worktree roots and company HTTPS origins before actual application. A configured file does not authorize NEXT to read unrelated browser tabs or silently click login/consent prompts.

## Validation and delivery gate

`internal/config/browser_policy_next_darwin_test.go` covers no-file unchanged behavior, Next-only load, Stable/stdio/custom-root isolation, restrictive permissions, symlinked root or file, malformed/unknown/trailing JSON, missing workspace policy, wrong profile, nonloopback endpoint, and environment conflict. Must pass native config/app/browser policy tests plus non-Darwin cross-compilation prior to integrating source. **No Core restart/install** from this source work. After owner confirms allowed roots/origins and real browser endpoint, a separately versioned NEXT-only candidate and controlled rollout can be planned; Stable stays available as fallback. Current installed RC5 still has no routes configured.
