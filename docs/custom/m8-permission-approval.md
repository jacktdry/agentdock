# M8 Permission / Approval

> Status: M8 implementation and available-host validation complete; Windows native UAT is an external follow-up
>
> Date: 2026-10-05
>
> Branch: `feature/m8-permission-approval`
>
> Base: AgentDock Next isolation `b4ef0945`
>
> Prerequisite repaired on this branch: M5 Activity Center / execution model integrated as `cbdabeef` from original `a505898a`


## M8 closeout — 2026-10-05

Implementation commit: `3df9a784`. Prior API / document checkpoints: `652331ed` / `f48f6374`. The closeout document commit is titled `docs(permission): record M8 closeout and validation`.

Core contract, versioned Desktop-control API and Shared Permission UI are complete. Approval Reviewer remains `defer`. No automatic reviewer, original-operation dispatch, provider permission authority or OS sandbox claim is added.

### Shared UI and Core truth

- Permission navigation displays the Core profile, global mode and reviewer; Approval Policy supports on-request / never / granular editing while preserving scopes, rules and profile. Confirmation displays exact mutation, approval ID/version, policy revision, operation/scope/reason and challenge expiry. Native Go retains the credential; JS receives only safe challenge/state metadata.
- Core approval records contain bounded creation-time decision/effective-permission traces and capability metadata. Returned nested data is detached from durable state. Existing records without a trace truthfully show unavailable. Approval and dispatch outcome are separate: pending, approved_once, consumed, approved_workspace, rejected, expired and invalidated remain Core-owned states.
- History reads include an atomic snapshot revision/epoch. UI compares status/history revisions and epochs, disables mutation on incoherence/unavailability, and re-reads after every mutation result, including conflicts and transport uncertainty. It refreshes on mount/focus/online/visibility and every five seconds while visible; pending challenges are cancelled on reconnect and do not survive navigation. Cancel/unmount invalidates in-flight confirmation presentation. Event listeners and timer are removed on unmount.
- Policy draft values survive refreshes at the same policy revision; a changed revision resets them to Core truth. Exact payloads are copied when confirmation starts. One confirmation can be submitted once. The UI never calls Runtime.Call or replays the original operation.

### Workspace eligibility final decision

All current built-in classifiers deliberately leave `WorkspaceRuleEligible=false`. File writes can escape a workspace depending on action/path and command, Browser/Computer, management and provider effects are not durable filesystem-confined rule classes. Merely targeting an in-workspace file is not proof that an exact tool/action durable allow rule is confined: that rule does not encode an enforceable path restriction. No built-in operation was invented or reclassified to enable this button.

Core reports `can_approve_workspace=false` and a bounded unavailability reason. UI disables Approve Workspace. Store eligibility projection and actual mutation share the same checks: effects must be known/non-opaque, an enforceable rule class and trusted workspace binding are required, and the resulting rule cannot override a stricter evaluated constraint. Synthetic store tests exercise eligible classes only as contract coverage. Runtime in-workspace file-edit regression and native UAT prove the unavailable state is truthful. Approve Once still requires the stable authenticated principal and exact retry.

### Final security review

Independent read-only review inspected Core admission, Runtime mutation entrypoints, ACP continuation/option selection/missing-hook failure, distinct Desktop authority, strict JSON/fingerprints, one-shot/principal/revision/epoch behavior, protected paths and UI execution truthfulness. It found two Windows issues, both repaired and re-reviewed:

1. Windows native bootstrap named pipe now uses `PIPE_REJECT_REMOTE_CLIENTS` in addition to its current-user DACL. A remote authenticated same-user pipe client must not retrieve the Desktop credential.
2. Windows-backed WSL paths now use conservative case-folding for `/mnt/<drive>` only. Linux-native paths retain case sensitivity. Host injects protected roots; helper canonicalizes symlink ancestors, guards read/write/move/transaction paths, and skips protected scan descendants before inspecting them. Helper/manifest/installer require protocol v2, failing closed against older helpers that would ignore protected roots.

The follow-up review found no concrete unresolved implementation blocker in those fixes. Portable regression and cross-compilation are evidence; actual Windows named-pipe/DrvFS behavior remains an external native-UAT check. This is an inspected review, not a claim of exhaustive proof against unrestricted same-account processes or OS sandboxing.

### Validation actually executed

| Layer | Evidence / result |
| --- | --- |
| Full root suite | `TMPDIR=/private/tmp go test ./... -count=1`, then final-tree `-json`: 2,302 test cases and 62 packages pass. Sole failure: unchanged `TestScriptGovernanceInventoryCoversTrackedScripts` for `packaging/macos/app-identity.sh` missing from inventory. This exact failure was reproduced at clean `f48f6374` before modifications. It is not fixed or reclassified as M8 failure. |
| Root skip | Only `internal/acp.TestRealAdapterInitialize` was skipped without `AGENTDOCK_TEST_ACP_COMMAND`. It was then run explicitly against `/opt/homebrew/bin/codex-acp` using a Next-marked wrapper with isolated HOME/CODEX_HOME and workspace; pass in 0.85 seconds. This is real initialize evidence, not a full provider prompt/OS-permission UAT. |
| Race | `TMPDIR=/private/tmp go test -race` with `-count=1`: permission, app, acp, runtimeapi, httpx, desktopapi, desktopcontrol, execution, nexusbridge, tool/browser/..., tool/computer/..., tool/file, tool/media, mcp/..., plugin/..., desktopruntime, tool/task and wslfilehelper all pass. This includes evaluator/policy/durable/expiry/restart/exact-principal/concurrent consume, handler-not-called, provider continuation and Broker hard-constraint regressions. |
| Vet / formatting | `TMPDIR=/private/tmp go vet ./...`, `git diff --check` and `go run ./tools/i18n check`: pass. Wails beta.27 bindings regenerated. |
| Shared Go | Nested `desktop/shared-poc`: `TMPDIR=/private/tmp go test ./... -count=1`: pass. |
| Frontend | 60 tests in 10 files pass, `npm run typecheck` and `npm run build` pass. Includes Core revision/epoch coherence, reconnect restoration, unavailable workspace grants, explicit double confirmation, at-most-once submission, conflicts, challenge expiry and sanitized errors. |
| macOS compile | Core and Shared Desktop production builds pass on arm64. Inherited deployment-target linker warnings remain; they did not prevent native launch. |
| Windows compile | `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` Core and Shared Desktop production builds pass. Windows file/control and Shared Desktop test executables cross-compile. This is not execution on Windows. |
| WSL protection | Portable DrvFS casing/symlink/ancestor guard tests run on macOS and pass (also race). Linux-only helper test suite and helper binary cross-compile for linux/amd64. Linux dispatch/scan test binaries were not executed: this host has no running Docker daemon or Windows/WSL host. |

An initial added test used a duplicate local variable and an initial race command named a nonexistent settings package. These were diagnosed and corrected; the final root/race runs above have no such failure. Settings regression is covered by the actual `desktopapi` / `desktopruntime` packages.

### Next-only native/runtime UAT

Isolated ad-hoc signed `AgentDock Next.app`, bundle ID `dev.dropabit.agentdock.next.m8-uat`, from this branch; roots under `/private/tmp/agentdock-m8-next-uat-m3z3gzlj/{runtime,state,workspace}`, direct loopback `127.0.0.1:18767`, explicit `AGENTDOCK_DESKTOP_VARIANT=next`. Native Shared Desktop always received `--runtime-root`; its preferences followed that fixture root. Installed stable/Next bundles and live launchd services were not test targets.

Verified through the native Wails UI and Core/MCP/IPC fixtures:

- Native profile/policy/reviewer `defer`, effective decision trace and workspace-disabled reason render truthfully.
- Ordinary bearer mutation returns 401; normal read routes and health succeed. Trusted native IPC bootstrap → Desktop Go → exact challenge → Core mutation works.
- Ask does not create the file; native Approve Once shows `approved_once / not_dispatched`; the file remains absent until the explicit caller retry. The exact same-principal retry creates it and history shows `consumed / succeeded`.
- Native Reject leaves the operation unexecuted; native `never` policy editing/confirmation persists the Core change and the subsequent Ask-class request is Deny without handler execution.
- Native app stop/relaunch against the same Core restores pending/history. A policy change while a confirmation dialog is open makes confirmation fail, re-fetches Core state and shows `invalidated / not_dispatched`; no operation is dispatched.
- Stopping only the fixture Core makes UI show unavailable/stale with policy mutations disabled. Restarting that Core restores policy/history, invalidates the old-epoch pending request and leaves its target absent.
- Fixture Core/App PIDs and descendants are gone; port 18767 is closed and `control.sock` is removed. No temporary runtime was left active. Ad-hoc UAT is not release signing or updater/service-registration validation.

Final committed-tree native rebuild repeated the Ask → Approve Once → explicit retry flow successfully after the frontend lifecycle cleanup correction; all fixture resources were again cleaned up.

Raw transient test logs remain in `/private/tmp/agentdock-m8-{root-acceptance.jsonl,race-final3.log,vet-final.log,desktop-final.log}`. Sanitized native UAT outcomes are in the fixture's `evidence.json`; credentials/payloads are not copied into repo docs or Memory.

### Canonical tasks, handoff and external checks

Per user reconciliation, repo `m8-permission-approval.md`, `roadmap.md`, `README.md` plus Next task store are the source of truth. New canonical Next task: `tsk_9b1ae7b19ea15a40`; full goal, 10 steps and acceptance conditions reconstruct M8 closeout. Existing `tsk_9c4e5c21c4c24c53` remains the linked API continuation/checkpoint. `tsk_b9e4bd0e985de598` is only a **legacy stable-store task reference**; it was never read, exported, modified or migrated.

Engineering Memory contains an additive final implementation/validation note and Codex Memory has an additive update note. Codebase Memory project `Users-wei-sideProject-agentdock-m8-permission-approval` is refreshed (17,251 nodes / 80,539 edges) with a closeout ADR while preserving its earlier API ADR. No destructive store rebuild/cleanup is performed.

External checks: native Windows named-pipe local/remote denial, true DrvFS alternate-case/symlink protection, Linux helper dispatch/scan and Windows UI/runtime UAT require the appropriate host. These are explicit platform-evidence follow-ups under the available-host acceptance scope, not claims of passing native UAT. Existing script-governance inventory debt is separate. No unresolved implementation security blocker remains from review. M8 local closeout does not authorize push, merge, release, stable activation or production deployment.

## Implementation checkpoint — 2026-10-05

M8 is now past the contract-only stage. The current branch has the following reviewable checkpoints:

- `cbdabeef` — repaired the M5 execution prerequisite so M8 uses the existing execution call graph / epoch rather than inventing a second identity model.
- `0a6e93c8` — defined the blocker-cleared M8 authority, binding, retry, approval-history and validation contract.
- `9ed469f1` — implemented the Core Permission Profile / Approval Policy / rule evaluator and decision trace.
- `4abc9376` — implemented bounded durable policy + approval/history state, atomic persistence, expiry/invalidation and one-shot grant consumption.
- `9bc8065c` — added the shared `AdmissionGate` to `Runtime.Call` after schema validation and immutable argument snapshotting, before handler dispatch. Ask/Deny do not call the handler and M5 records the permission error code / category truthfully.
- `4c1c9274` — bound normal MCP authorization to stable opaque principals: configured static bearer credentials use a domain-separated fingerprint and OAuth uses the authenticated client/grant identity, remaining stable across access-token refresh without persisting raw credentials.
- `c8c10053` — propagated Core-owned opaque principals through Nexus and the ACP Browser / Computer capability transports. Capability-token rotation does not become authorization identity.
- `ef9c29ce` — gated ACP Browser / Computer host capabilities through the same Core admission boundary while preserving their independent ownership, foreground, routing and provider constraints.
- `832018c3` — wired Runtime insertion/task/MCP/evolution mutations through the same `AdmissionGate`. Input validation/normalization happens before admission, Ask/Deny never reaches the underlying mutation handler, allow/error dispatch settles the M5 child call truthfully, and authenticated Runtime API callers now preserve the same stable opaque principal used by normal Core admission. Runtime MCP list/inspect remain explicit read-only exceptions.
- `8e469fe0` — closed the ACP provider-continuation / permissive `session/request_permission` bypass. Any provider `option_id` selection now passes an injected Core continuation gate inside `acp.Manager` immediately before the response can resume the provider; a missing hook fails closed. Core cancellation bypasses the gate, while provider-supplied labels/kinds such as `reject_once` are not trusted as cancellation. One-shot matching is bound to principal + ACP session/profile + exact interaction ID + option/tool-call fingerprint, and the outer `acp_interaction respond` wrapper no longer consumes a separate approval before the nested continuation gate.
- `dbf86e17` — added the distinct process-local Desktop control authority. Every Core start generates a fresh credential; normal MCP credentials cannot authenticate it. Permission mutations require an exact bounded one-time confirmation challenge bound to mutation kind, approval id/version, expected policy revision and normalized payload fingerprint. Mismatch/expiry/replay fail closed, concurrent consumption is at-most-once, and Core supplies `decided_by=desktop-control` instead of accepting an actor from request data.
- `f2a6a70f` — protected the durable permission/control-plane subtree from normal file-oriented surfaces. Host `read_file`, `list_dir`, `search_text`, every `file_edit` mode (including structured/unified patches), `file_publish`, and local-path `view_image` now deny direct protected paths; ancestor listing/search skips the protected subtree without leaking contents, while ancestor delete/move/publish is denied. Symlink resolution is canonicalized, unified diff target paths are inspected before `git apply`, and Windows/WSL file runtime applies the same protection through the existing drive-to-`/mnt` mapping. The current Desktop control credential remains process-local and therefore has no serialized credential path to expose.

Current targeted regression is green for `internal/permission`, `internal/execution`, `internal/app`, `internal/httpx/requestmeta`, `internal/httpx`, `internal/nexusbridge`, `internal/tool/browser`, and `internal/tool/computer`. `go test ... -count=1`, `go vet ...` and `git diff --check` pass for that set.
ACP continuation-specific `-race` coverage also passes. Full `go test ./... -count=1` passes every package except the pre-existing script-governance inventory failure for `packaging/macos/app-identity.sh`, unchanged from the prior M8 baseline.

The prior Runtime-management preparation in `internal/app/runtime_permission.go` and `internal/permission/host_operation.go` is now part of committed checkpoint `832018c3`; there is no longer a classifier-only uncommitted slice. Handler-before/after tests prove the covered mutations are gated before dispatch and that success/failure/permission outcomes are reflected in the M5 execution journal.

## Runtime / Shared Desktop API checkpoint — 2026-10-05

Code checkpoint: `652331ed` (`feat(permission): complete versioned Desktop control API`).
The three files preserved from handoff `d330ecd4` are now included in this completed checkpoint; no earlier Core checkpoint was redone.

- Registered direct-loopback permission/approval read routes with normal Runtime auth and bounded status/limit validation.
- Added the separate Desktop-control POST route group. Ordinary MCP bearer, OAuth, Nexus/provider credentials and loopback alone cannot authorize mutations, including when normal Core auth is disabled. Normal Runtime/Nexus dispatch cannot enter these routes.
- Mutation bodies are limited to 64 KiB, require a JSON object and exactly one value, and reject unknown fields recursively, including actor/credential fields and unknown policy fields.
- Confirmation issuance and policy/approve-once/approve-workspace/reject dispatch use the existing Core authority. Challenge mismatch, expiry, replay and policy/approval CAS conflicts fail closed. Core retains `decided_by=desktop-control`; decisions never dispatch the original operation.
- Core native IPC handles `permission.bootstrap` on the existing permissioned Unix socket / current-user Windows named pipe. The Go Desktop backend obtains the per-Core-start secret through that channel and sends it only on direct local HTTP control requests. No credential file, ordinary bearer fallback, frontend credential getter, automatic confirmation or automatic retry is added. Redirects and proxies are disabled in the permission client.
- Registered `PermissionService`, advertised all eight `DomainPermission` v1 operations and generated Wails TypeScript service/state bindings. Policy and approval projections come from Core; UI capability metadata remains separate from authority.
- HTTP mapping: authentication 401, validation 400, missing approval 404, challenge/revision/version conflicts 409, ineligible grant/permission denial 403, capacity 429, sanitized state failure 500. `APPROVAL_REQUIRED` returns 409 with allowlisted non-executed retry binding metadata.

Validation actually run on isolated temporary fixtures:

- `TMPDIR=/private/tmp go test ./internal/permission ./internal/execution ./internal/runtimeapi ./internal/httpx ./internal/desktopapi ./internal/app ./cmd/agentdock ./internal/nexusbridge ./internal/tool/browser ./internal/tool/computer -count=1`: pass.
- `TMPDIR=/private/tmp go test -race ./internal/permission ./internal/runtimeapi ./internal/httpx ./internal/desktopapi ./internal/app ./internal/execution -count=1`: pass.
- `go vet` for the ten targeted packages and `git diff --check`: pass.
- Regression covers handler-before/after, ordinary credentials with normal auth enabled/disabled, proxy rejection, strict nested bodies, missing/replayed/mismatched challenges, policy/approval conflicts, Core controlled-clock expiry plus HTTP expiry mapping, ineligible workspace grants, Core attribution and exact retry dispatch.
- Real fixture Unix IPC bootstrap → Go permission service → HTTP control → Core mutation: pass; response projection omits both credentials and raw remote errors, and missing bootstrap fails closed.
- Shared Desktop nested Go module tests, Wails beta.27 binding generation, frontend 48 tests / typecheck / production build: pass. Native linker emits inherited macOS deployment-target warnings.
- Windows amd64 Core cross-build: pass; this is compile evidence, not native Windows runtime/UAT.

Limits and next gate:

- Current built-in classifiers do not set `WorkspaceRuleEligible`. The `approveWorkspace` API preserves the existing store eligibility checks and returns `APPROVAL_NOT_ELIGIBLE` for current built-in requests, even an in-workspace file edit. Do not broaden this eligibility just to enable a UI button; eligible synthetic store coverage remains in `internal/permission`.
- Permission Profile / Approval Policy / effective permission / approval history UI is the next checkpoint. Reviewer remains `defer`.
- This checkpoint does not claim full `go test ./...`, native permission UI UAT, Windows native IPC execution or live Next rollout. The known `packaging/macos/app-identity.sh` script inventory baseline remains separate.
- An initial path-protection regression used macOS `/var` temporary paths and failed comparison against canonical `/private/var`. Re-running the final test set with canonical `/private/tmp` passed without changing product code or the inherited test.
- Stable App/Core/state/registry and the live stable `mac-dev` control channel were not used. All HTTP/IPC resources created by regression fixtures are closed by test cleanup.

Handoff maintenance: AgentDock Next task `tsk_9c4e5c21c4c24c53` records API completed / UI pending in `~/.agentdock-next/tasks`; the empty Next store required a dedicated task rather than editing stable tasks. Engineering Memory received an additive checkpoint note. Codebase Memory project `Users-wei-sideProject-agentdock-m8-permission-approval` was re-indexed (17,061 nodes / 80,082 edges), with an API ADR; four existing script/packaging files have partial parse coverage and intentional excluded paths remain reported.

Stable AgentDock remains outside the M8 development target. Runtime validation continues on AgentDock Next only.

## Goal

M8 adds a Core-owned permission and approval control plane without pretending to
be an operating-system sandbox. It answers four separate questions truthfully:

1. What is the configured Permission Profile?
2. Which Approval Policy applies?
3. Why did a fixed request resolve to allow / ask / deny?
4. What happened to each approval request?

The Shared Desktop is a client of this state. Native UI, ACP adapters, browser
drivers, MCP providers and third-party tool metadata are not permission
authorities.

## Non-goals

M8 does not:

- elevate or reduce the operating-system account privileges of AgentDock;
- import the Workbench workspace/task/conversation registry;
- trust caller-supplied `workspace_id`, read-only flags, MCP annotations or
  provider claims as authorization facts;
- allow approval to override Browser/Computer ownership, foreground policy,
  external provider denial, operating-system denial or a Permission Profile
  ceiling;
- start an automatic AI Approval Reviewer;
- persist unrestricted tool arguments, command environment values, auth
  headers, provider responses or secrets in approval history;
- migrate or retire stable AgentDock.

Runtime validation remains AgentDock Next-only.

## Prerequisite: M5 execution identity

M8 depends on M5's `internal/execution` call ID and journal. M5 had been
completed on `feature/activity-center` but was not an ancestor of
`custom/main`, M6, M7 or the original Next isolation branch. M8 repaired that
ordering first by cherry-picking the exact M5 final feature commit.

Permission state must extend the existing execution identity rather than create
a second call graph.

## Authority model

### Core is authoritative

Only AgentDock Core may:

- classify the effects of a tool request;
- derive authorization and audit bindings;
- evaluate Permission Profile / Approval Policy / rules;
- create, settle, invalidate or expire approval records;
- consume a one-shot approval grant;
- expose effective permission state to Shared Desktop.

The UI may request policy changes or approval decisions through explicit
versioned Core APIs. It never computes an effective permission locally.

### Desktop control authority

Permission mutations are not authorized by the normal MCP bearer token, normal
OAuth access token, Nexus credential, provider credential or loopback origin.

Core maintains a distinct per-Core-start Desktop control credential. The Shared
Desktop Go/native backend obtains and holds that credential; frontend
JavaScript never receives it. Permission mutation requests require:

- direct loopback transport;
- the Desktop control credential;
- an unexpired one-time confirmation challenge issued by Core;
- the exact mutation kind;
- approval ID when applicable;
- approval record version when applicable;
- expected policy revision;
- a server-computed hash of the normalized mutation payload.

The challenge is consumed once. `decided_by` is derived by Core from this
authority; it is not accepted from request JSON.

The concrete cross-platform credential transport may use the existing runtime
root, but AgentDock file/control APIs must treat the credential and permission
state as protected control-plane material. This boundary prevents ordinary
AgentDock MCP/Nexus/provider credentials from self-approving. It is not an
operating-system security boundary against unrestricted same-account processes.

### Shared admission boundary

`Runtime.Call` is not the only execution path. M8 therefore defines a shared
Core `AdmissionGate` that every covered side-effecting entrypoint must call
before dispatch.

Covered entrypoints include:

- normal MCP / Nexus tools through `Runtime.Call`;
- Runtime management mutations that currently call services directly;
- ACP Browser Broker acquire/act/release paths;
- ACP Computer Broker acquire/act/release paths;
- ACP session start/prompt/resume/update operations when they can continue
  opaque provider execution;
- permissive ACP `requestPermission` responses.

Read-only authenticated control-plane endpoints may be explicit exceptions.
No operation is exempt merely because its source is `internal` or because it is
classified as management.

### Hard constraints

Permission evaluation produces an AgentDock admission decision only. A later
subsystem may still reject the request.

Examples:

- Browser Broker route ownership / no-focus / authenticated-profile policy;
- Computer broker ownership and explicit control boundary;
- ACP adapter/provider restrictions;
- dynamic MCP/provider authorization;
- OS filesystem/process/keychain/network enforcement.

These constraints are represented in the decision trace as independent
constraints when known. An approval can never convert one of their denials into
an allow. Broker ownership and foreground checks are re-run at actual dispatch,
after permission admission.

AgentDock's own permission state, Desktop control credential and other
authority-changing control files are protected control-plane paths. Ordinary
AgentDock file APIs may not read, write, rename, delete or patch them in any
permission mode. Under unrestricted host command execution, M8 does not claim
to sandbox a same-account process from the host filesystem.

## Permission binding

M8 separates audit identity from authorization identity.

### AuditBinding

Audit-only fields correlate M5 execution records:

```text
AuditBinding v1
- call_id
- parent_call_id
- original_approval_id
- original_call_id
```

A retry always has a new `call_id`; call IDs are never used to decide whether a
one-shot grant matches.

### AuthorizationPrincipal

The principal is derived only from authenticated Core transport/runtime state:

```text
AuthorizationPrincipal v1
- kind
- id
```

Supported v1 identities:

- normal MCP with OAuth: server-derived OAuth client/credential identity;
- normal MCP with static bearer: a non-reversible server-side fingerprint of
  the configured bearer credential;
- Nexus: authenticated device identity;
- ACP Browser/Computer bridge: Core-owned ACP session/profile owner identity;
- Desktop permission mutations: the distinct Desktop control authority;
- explicitly named internal control-plane callers that Core constructs itself.

The normal AgentDock MCP server currently uses stateless Streamable HTTP, so M8
must not invent or depend on an MCP session ID that does not exist. Clients that
share the same static bearer are intentionally the same authorization
principal. If Core cannot derive a stable authenticated principal, implicit
Approve Once is unavailable.

### PermissionBinding

```text
PermissionBinding v1
- principal
- runtime_epoch
- source                   internal | mcp | nexus | acp_bridge | desktop
- workspace_root           only from trusted Core-owned provenance
- workspace_id             opaque ID derived by Core from trusted root
- acp_session_id           when Core owns the ACP session identity
- provider                 fixed provider/server identity when known
```

`workspace_id` is not accepted from a tool argument or discovered repository
root. Trusted workspace provenance in v1 is limited to:

- the configured runtime/default workspace root; or
- a Core-owned ACP/runtime session binding established through an authorized
  control flow.

Caller-selected target paths and `workspace_context` discovery can be inputs to
resolution, but do not establish authorization scope.

Workspace IDs are versioned opaque hashes of the canonical trusted root. Path
canonicalization must resolve symlinks for existing ancestors and use
platform-correct case/volume semantics. If Core cannot prove a trusted root,
workspace fields are empty and workspace-scoped rules do not apply.

## Permission Profile

A profile is an admission ceiling, not an OS sandbox:

```json
{
  "filesystem": "deny | read | write",
  "network": "deny | allow",
  "sandbox_boundary": "none | workspace"
}
```

Rules:

- a restricted profile requires runtime-owned effect classification;
- unknown effects fail closed;
- `filesystem=read` denies writes;
- `network=deny` denies classified network-capable requests;
- `sandbox_boundary=workspace` requires every relevant target to be proven
  inside the bound canonical workspace;
- approval and allow rules cannot widen the profile.

The default profile is permissive relative to the host process
(`write/allow/none`) so M8 does not silently change existing AgentDock
behavior until policy is configured.

## Approval Policy

Supported modes:

- `on-request`: an Ask decision creates an approval request;
- `never`: Ask becomes Deny; **never does not mean auto-approve**;
- `granular`: per-category booleans decide which Ask categories may create an
  approval.

Categories:

- file writes
- commands / process input
- network
- MCP
- management
- other

## Approval Reviewer

M8 defines `approval_reviewer=defer` as the default and required behavior.

`defer` means:

- Core does not start an AI reviewer or external command;
- the request remains a local-user decision;
- no model or adapter can silently approve it;
- high-risk or unknown-effect requests are never auto-approved.

A future milestone may add an independently configured reviewer, but it must be
a new reviewed extension of this contract. Workbench's `auto_review` process
runner is therefore not ported in M8.

## Policy and scopes

Durable policy schema v1 contains:

- revision;
- global mode: `readonly | rules | full`;
- global Permission Profile / Approval Policy / reviewer settings;
- zero or more workspace scopes keyed by Core-derived trusted workspace ID;
- explicit tool/action rules.

Conversation scope is intentionally not in M8 v1 because current AgentDock Core
does not have the Workbench authenticated conversation registry.

Fresh M8 bootstrap preserves current AgentDock behavior:

- mode = `full`;
- profile = `write / allow / none`;
- approval policy = `on-request`;
- reviewer = `defer`;
- no explicit rules.

This bootstrap state is written as normal policy state. Later transitions into
`full`, or any policy change that widens authority, require the Desktop control
authority and exact mutation confirmation.

### Evaluation precedence

Evaluation is deterministic:

1. validate schema and construct runtime-owned facts;
2. resolve trusted authorization principal / workspace binding;
3. apply non-overridable Core hard constraints known at admission time;
4. intersect global and workspace Permission Profile ceilings;
5. collect all matching explicit rules from global + trusted workspace scope;
6. resolve matching rules with fixed precedence `deny > ask > allow`;
   an Allow never overrides a matching Ask or Deny, and workspace rules cannot
   widen a stricter matching global rule;
7. if no explicit rule matched, apply mode fallback;
8. if the result is Ask, apply Approval Policy;
9. only if the result is still Ask may an exact valid one-shot grant convert it
   to Allow;
10. revalidate policy revision, binding, facts and external broker constraints
    immediately before dispatch.

Mode fallback:

- `readonly`: allow only Core-proven read-only operations; management is not
  automatically safe. Everything else denies.
- `rules`: allow only Core-proven read-only operations by default; known
  side-effects ask; opaque/unknown effects may ask only when the operation is
  eligible for one-shot approval and no restricted profile requires proof.
- `full`: allow by default for compatibility. It does not override a matched
  explicit deny/ask rule or a Permission Profile ceiling.

Unknown effects:

- any restricted Permission Profile -> Deny;
- `readonly` -> Deny;
- `rules` -> Ask only for an authenticated stable principal and an
  approval-eligible opaque operation; Approve Workspace is not offered for
  opaque effects;
- `full` with the unrestricted default profile may Allow for compatibility, and
  the decision trace must say that effects are unclassified/unrestricted.

For granular Approval Policy, every category that applies to a request must
permit asking. `never` converts Ask to Deny; it never auto-approves.

## Effective permission and decision trace

Every evaluation returns an explainable trace, not just an effect:

```text
Decision
- effect                   allow | ask | deny
- rule_id
- reason
- policy_revision
- binding
- effective settings
- sources[]
```

Each source is bounded metadata:

```text
DecisionSource
- kind     default | profile | scope | rule | approval_once |
           runtime_constraint | provider_constraint
- id       stable bounded identifier when available
- effect   allow | ask | deny | constrain
- reason   bounded human-readable summary
```

A source entry reports why AgentDock reached its admission result; it does not
claim that external execution must succeed.

## Runtime-owned effect facts

`PermissionFacts` are computed only after tool arguments pass schema
validation. Facts are runtime-owned; caller or third-party annotations never
prove safety.

Facts include:

- effects_known;
- filesystem: none | read | write;
- network;
- workspace_bound;
- read_only;
- management;
- opaque_provider_execution;
- tool;
- action;
- reason;
- authorization binding.

Third-party MCP annotations do not prove read-only behavior. An opaque
`mcp_tool_call` is potentially side-effecting/network-capable unless AgentDock
has a stronger built-in classifier.

ACP start/prompt/resume and permissive permission-option responses are opaque
provider execution unless Core can prove enforceable downstream limits.
Cancellation/rejection is always allowed. Under a restricted profile, opaque
provider execution fails closed. Under `rules`, permissive continuation asks;
under `full`, compatibility policy may allow it.

Browser/Computer ownership, foreground and route constraints remain separate
hard constraints and are rechecked at dispatch.

### Prepared request

Permission evaluation does not fingerprrint the caller's mutable argument map.
Core creates a bounded immutable `PreparedRequest` from a deep copy after schema
validation. It includes:

- versioned canonical JSON encoding of validated arguments;
- resolved defaults used by classification;
- trusted authorization binding;
- resolved target identities/facts needed by the classifier;
- relevant provider/session/configuration generation identifiers.

Fingerprints are computed from the unredacted PreparedRequest in memory. The
persisted display summary is generated separately from an allowlist and is
never used for matching.

On retry Core prepares the request again. A grant matches only if the
fingerprint and every bound generation/identity still match. Operations whose
restricted workspace safety cannot be enforced race-resistantly are denied
rather than approved.

## Ask flow: retry-based approval

Current AgentDock tools have independent strict output schemas. M8 does not
inject a universal pending-approval payload into successful tool results.

Flow:

1. Core creates the M5 execution call.
2. Core validates schema and creates `PreparedRequest`.
3. Shared `AdmissionGate` derives facts/binding and evaluates policy.
4. Allow continues to dispatch.
5. Deny never calls the handler and returns typed `ToolError` code
   `PERMISSION_DENIED`.
6. Ask never calls the handler. Core creates a bounded approval record and
   returns typed `ToolError` code `APPROVAL_REQUIRED` whose `Details` contain
   only `approval_id`, `executed=false`, policy revision, approval version and
   retry guidance.
7. `Approve Once`  is available only when Core has a stable authenticated
   authorization principal. It creates an in-memory grant bound to:
   - approval ID/version;
   - policy revision;
   - runtime epoch;
   - authorization principal;
   - trusted workspace/provider/ACP identities;
   - tool/action;
   - exact PreparedRequest fingerprint and bound generations.
8. The next newly prepared request from the same authorization principal may
   atomically consume the grant only if every bound value still matches.
9. Changed arguments, identities, target resolution, provider/session
   generation or policy revision do not match and are evaluated normally.
10. `Approve Workspace` is available only for a trusted workspace binding and
    a Core-classified, non-opaque rule class whose workspace scope is truthful.
    It atomically adds the durable workspace rule and settles the approval in one
    state transaction. Opaque command/MCP/provider effects never receive a
    durable workspace grant in M8 v1; UI disclosure cannot widen this Core rule.
11. Reject / expiry / invalidation never dispatches the original request.

No approval token is added to tool arguments. The retry is a new M5 execution
call. The original call ID is audit correlation only.

## Approval record and history

Durable records contain only bounded/redacted display data:

- approval ID and record version;
- original M5 call ID;
- audit-safe authorization binding metadata;
- tool/action;
- allowlisted operation summary;
- bounded scope description;
- rule/reason;
- policy revision and runtime epoch;
- status;
- created/expires/decided timestamps;
- server-derived decided_by;
- grant kind (`once | workspace | none`);
- optional granted rule ID;
- optional consumed retry call ID;
- dispatch outcome (`not_dispatched | pending | succeeded | failed | unknown`).

Persisted summaries omit command text, stdin, environment values, headers,
credentials, raw provider payloads and secret-bearing URL query/fragment/user
info. Truncation happens after allowlist-based redaction.

The exact PreparedRequest and one-shot fingerprint live only in Runtime memory
and are never persisted.

Limits:

- max 128 pending approvals;
- max 128 live in-memory one-shot grants/prepared requests;
- max 512 retained durable history records;
- default expiry 15 minutes;
- operation summary <= 16 KiB;
- scope <= 4 KiB;
- reason <= 2 KiB;
- total serialized permission state <= 4 MiB.

### Restart and crash semantics

Core has a fresh runtime epoch on every start.

On startup:

- prior-epoch `pending` approvals become `invalidated`;
- prior-epoch `approved_once` but unconsumed approvals become `invalidated`;
- no one-shot grant or PreparedRequest is reconstructed from disk;
- corrupt/unreadable existing permission state fails closed and does not revert
  to permissive defaults.

Approve Workspace updates policy and approval settlement in one atomic
permission-state transaction.

### At-most-once approval consumption

Approve Once state transitions are:

```text
pending -> approved_once -> consumed
pending -> approved_workspace
pending -> rejected
pending -> expired
pending -> invalidated
approved_once -> expired | invalidated
```

For a matching retry, Core atomically verifies policy revision, epoch,
principal, binding, PreparedRequest fingerprint and expiry, then persists
`consumed` plus the retry call ID **before** handler dispatch. Handler failure
does not restore the grant.

After dispatch the approval record updates the outcome to succeeded/failed. If
Core crashes after durable consumption but before outcome settlement, recovery
reports `unknown`; it never replays the operation.

### M5 execution journal

The original Ask call terminates as a non-executed failed admission with:

- error category `permission`;
- error code `APPROVAL_REQUIRED`;
- no handler facts implying side effects.

A Deny call similarly terminates with `PERMISSION_DENIED`.

The retry is a distinct call. Approval history correlates original/retry IDs;
M5 journal retention remains independently bounded, so old correlated execution
details may legitimately be unavailable.

## ACP permission interaction

ACP adapter `requestPermission` is provider-side interaction state; offered
labels/options are not Core authorization facts.

Rules:

1. Core admission applies to AgentDock's `acp_session`, `acp_prompt` and
   `acp_interaction` entrypoints according to runtime-owned facts.
2. Starting/resuming an ACP prompt is opaque provider execution unless Core can
   prove enforceable downstream constraints.
3. Core cancellation (`response.action=cancel`) is always safe to admit because
   it does not select a provider option. Provider labels/kinds such as `reject`
   are not trusted as cancellation semantics.
4. Any `option_id` selection, including one labelled or typed as reject by the
   provider, re-enters `AdmissionGate` as opaque provider continuation before
   `RespondInteraction` resumes the adapter. A future adapter may exempt a
   rejection option only if Core enforces an explicit mapping to cancellation.
5. Under a restricted profile opaque continuation denies; under `rules` it asks;
   under `full` the compatibility fallback may allow.
6. Browser/Computer tools exposed to ACP additionally pass their own shared
   admission boundary and broker ownership/foreground checks.
7. Provider refusal after Core Allow remains a provider constraint, not a Core
   permission bug.

## Shared Desktop API

M8 makes `DomainPermission` available with versioned operations:

Read:
- `status` / effective policy
- `history`
- `approval`

Mutating:
- `beginConfirmation`
- `updatePolicy`
- `approveOnce`
- `approveWorkspace`
- `reject`

`RequiresConfirmation` remains UI capability metadata only; it is not an
authorization primitive.

Every mutation is accepted only through the Desktop control authority described
above. The Shared Desktop Go/native backend performs the control request and
injects the control credential; frontend JavaScript receives only bounded
status/challenge metadata, never the credential.

Revision/version mismatch is a conflict and forces refresh. A confirmation
challenge is bound to the exact normalized mutation payload and cannot be
reused for another approval or broader policy update.

Shared UI renders:

- effective source/scope and decision trace;
- Permission Profile;
- Approval Policy;
- reviewer = defer;
- pending approvals;
- approval history;
- whether Approve Once is unavailable because the caller has no stable
  authenticated principal;
- explicit retry-required state after approval;
- opaque-provider and non-filesystem-confined grant warnings.

The UI never implies that a pending or merely approved request executed.

## Runtime API

Core exposes permission **read** endpoints on the normal loopback runtime API,
subject to existing direct-loopback/auth rules:

```text
GET /internal/runtime/permissions
GET /internal/runtime/approvals
```

Permission mutations use a distinct Desktop-control route group. They are not
authorized by ordinary Runtime API bearer/OAuth credentials and remain
mandatory-auth even when normal Core auth is disabled:

```text
POST /internal/desktop-control/permission-confirmations
POST /internal/desktop-control/permissions
POST /internal/desktop-control/approvals
```

Bodies are bounded strict JSON with unknown fields rejected. Mutation routes
require direct loopback, Desktop control credential and an exact unexpired
one-time confirmation challenge.

## Storage

Permission state belongs under the active AgentDock home:

```text
<AGENTDOCK_HOME>/permissions/
  state.json
```

`state.json` schema v1 contains policy plus bounded approval history so policy
changes and approval settlement can commit as one transaction. Writes use a
same-directory temporary file, bounded encoding, fsync and atomic rename.

The per-Core-start Desktop control credential is separate protected runtime
control material and is never serialized into `state.json` or returned through
normal Runtime/MCP APIs.

AgentDock Next therefore uses `~/.agentdock-next/permissions`; stable state is
not read, migrated or modified during M8 development.

Normal AgentDock file APIs hard-deny permission state and Desktop-control
credential paths. This protects the AgentDock control plane from its own file
tools; it does not claim to confine arbitrary host commands in unrestricted
`full` mode.

## Validation gates

Before M8 closeout:

- permission package unit/race/vet;
- policy revision and fail-closed profile tests;
- exact-request one-shot grant tests;
- approval expiry/restart/at-most-once tests;
- Runtime handler-not-called tests for Ask/Deny;
- Browser/Computer hard-constraint regression;
- ACP interaction regression;
- MCP/plugin/settings regression;
- execution journal truthfulness;
- Runtime API strict-body tests;
- Shared Desktop backend/API/frontend tests;
- reconnect-safe UI state;
- i18n generation/coverage;
- macOS + Windows Shared builds;
- AgentDock Next-only runtime smoke.

Known inherited baseline debts must be reported separately rather than silently
attributed to M8.
