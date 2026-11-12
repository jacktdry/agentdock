# P10 Windows WPF runtime isolation checkpoint

2026-10-14; source base `816376e81e5476cb5dd88095dae0ddbe5cc6f515`,
branch `feature/p10-release-migration`. Uncommitted, bounded source change.
P10/M9 remains open. **Next WPF runtime is deliberately unavailable**, including
with a present, apparently complete manifest. This is not an installable Next tray.

## Workflow and acceptance

- Pain: a Next-labelled WPF executable can otherwise signal Stable's singleton,
  discover its root, read its state, invoke its scheduled task or update its Core.
- Improvement: an explicitly selected Next process stops before those operations.
- Minimum implementation: one immutable process product identity and three early
  rejection gates; preserve the original Stable operation implementations.
- Observable acceptance/non-interruption: static identity and gate-order checks;
  Next startup returns exit code 2 without culture preference reads, AUMID calls,
  helper dispatch, singleton/event creation, runtime discovery or update handoff.
  No consent prompts or runtime processes are requested by the new gate.
- Excluded: installer/scripts/workflows, XAML/resources/layout, live app/service
  changes, Windows processes/UI, installation, publish, tag, Release, push or commit.

This advances P10's source isolation boundary, not native runtime acceptance.

## Contract implemented

`WindowsProductIdentity.Current` lazily selects the environment once per process.
Unset/empty or exact `stable` keeps Stable. Exact
`AGENTDOCK_DESKTOP_VARIANT=next` selects Next. Other values, including case or
whitespace variants, fail closed instead of silently choosing Stable.

| Identity | Stable | Next |
| --- | --- | --- |
| Product | AgentDock | AgentDock Next |
| Mutex | `Local\AgentDock.ControlPanel.Singleton` | `Local\AgentDock.Next.ControlPanel.Singleton` |
| Show event | `Local\AgentDock.ControlPanel.Show` | `Local\AgentDock.Next.ControlPanel.Show` |
| AUMID | `com.uvwt.agentdock.controlpanel` | `dev.dropabit.agentdock.next.controlpanel` |
| State directory name | `.agentdock` | `.agentdock-next` |
| Default port | 8765 | 8767 |
| Default runtime root | `%LocalAppData%\AgentDock` | `%LocalAppData%\AgentDock Next` |

Next identities are reserved contract values, **not activated runtime configuration**.
Startup rejects before registering the AUMID or opening either namespace. The
gate does not inspect `AGENTDOCK_RUNTIME_DIR`, command-line roots, executable
ancestors or `runtime.json`. Missing, malformed, ambiguous, Stable or copied
manifests therefore cannot enable Next or trigger Stable fallback. No root alias,
path relocation, task alias or synthetic health status is used.

`App.OnStartup` reports the rejection to stderr and shuts down with code 2 before
all existing helper flags (`--task-admin`, `--run-core-task`,
`--run-elevated-agentdock`, `--start-core`, `--start-tunnel`), normal tray startup
and update resumption. No dialog is introduced. WinExe stderr visibility depends
on the caller; shell/launcher capture and Windows exit behavior remain unverified.
The gate rejects even an explicit Stable `--runtime-root` before logging there.

`RuntimeService` independently rejects construction before root resolution;
no Next service object can call credential reads, health probes, Core processes,
task/Run-key changes, tunnel operations or updates. `TaskAdminService.Run` also
rejects direct calls before parsing task defaults, elevation checks or scheduler
COM access (existing error handling returns 1). The guard cannot be bypassed by
supplying task/root arguments. No child is launched, so variant propagation to
elevated children is not claimed as implemented.

For supported Stable selection, original RuntimeService and TaskAdminService
source bodies are byte-identical after removing the added guard lines. Existing
manifest/settings defaults, discovery, UI preferences and helper semantics are
unchanged. This is source preservation evidence, not native behavioral UAT.

## Why execution remains blocked

The actual `RuntimeManifest` describes `install_root`, Core/tray/cloudflared
binary and launcher paths, local/public URLs, `port`, `privilege_mode`, task and
Run-value names, and tunnel mode. It has no product variant/state identity fields;
its defaults are port 8765, task/Run value `AgentDock`, tray `AgentDockTray` and
tunnel `AgentDockCloudflared`. `ReadRuntimeManifestAsync` relocates recorded
paths and accepts fallback binaries. `GetSnapshotAsync` accepts a missing
manifest, reads `control-panel-settings.json`, probes local health and can run
Core version/status commands. Nexus reads `%UserProfile%\.agentdock\nexus\device.json`.
UI preferences use `%LocalAppData%\AgentDock\ui-language`.

The inspected Stable installer also consumes `agentdock_home` and
`agentdock_default_dir`, which WPF's model does not represent. It still defaults
to `.agentdock`, the `AgentDock` workspace and Stable startup names. A directory
rename or 8767 override cannot prove those consumers are isolated.

Before removing the rejection gate, the owner must implement and review:

1. A shared installer/Core/WPF Next manifest identity contract, explicit required
   fields, duplicate/ambiguous-field rejection, canonical isolated state/workspace
   and root validation (including filesystem aliases/reparse points).
2. Next-only binary/generation provenance and path ownership without Stable
   rebasing/fallback; isolated settings/URLs/port, credentials, Nexus and UI preference
   handling. No Stable health/port probe during Next status reads.
3. Next-only task/Run-key identifiers, independent owned-process cleanup,
   validated elevated helper roots/arguments and variant propagation. Do not reuse
   Stable tasks or legacy installer/uninstaller behavior.
4. An isolated Next update channel/target contract and same-identity handoff;
   existing update operations remain blocked for Next.
5. Windows compilation and authorized native coexistence/UAT, including startup
   rejection exit/status reporting, no focus/prompts and unchanged Stable operation.

Packaging integration is owned separately. The packaging worker must not treat
these reserved identities as permission to make a runnable installer. The closest
next workflow is agreeing on the actual Next runtime manifest and installer/Core
ownership, then implementing the corresponding WPF consumers before enabling startup.

## Verification and ownership

- macOS host: `dotnet`, `csc`, `mcs` unavailable. No `dotnet build --no-restore`,
  C# compilation, Windows UI/process, runtime UAT or native behavioral tests ran.
- Read-only Python source contract assertions: **27/27 PASS**. Checked exact
  identity values, variant rejection, frozen selection, unconditional Next block,
  startup guard ordering, independent constructor/admin gates and Stable source
  preservation. These text assertions do not prove C# execution semantics.
- Persisted CI-targeted `InstallerSourceTests.test_wpf_next_entrypoints_fail_before_stable_access` now guards identity/installer AUMID agreement and WPF early-gate order; combined fast source suite **6/6 PASS**.
- `rtk git diff --check`: **PASS**. No runtime resources started.
- Changed files: `App.xaml.cs`, `Models/WindowsProductIdentity.cs`,
  `Services/RuntimeService.cs`, `Services/TaskAdminService.cs` under
  `desktop/windows/control-panel/`, and this document only.
- Repository Codebase Memory index refresh is tracked by the owner; an earlier P10
  fast-index attempt timed out. This scoped document records the decision, and
  successful Codebase Memory re-index must not be assumed.

Task exceeded the local 4,000-token budget during source inspection and completion;
no further workstream or agent delegation was introduced.
