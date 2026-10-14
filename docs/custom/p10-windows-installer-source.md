# P10 Next Windows installer source contract

2026-10-14, source baseline `816376e81e5476cb5dd88095dae0ddbe5cc6f515`.
This is a blocked declarative source, **not a completed or installable installer**.
The earlier [Windows packaging checkpoint](p10-windows-packaging.md) describes
metadata preparation; its historical statement that no Next `.iss` exists is
superseded only by this source addition. P10/M9 acceptance remains open.

## Workflow and bounded improvement

1. User pain: preparing Next must not overwrite or control operational Stable Windows.
2. Improvement: maintainers can review a separate installer identity and explicit
   payload contract without invoking Stable installation code or producing Setup.
3. Simplest implementation: a new Inno entry point, two Next-only includes, an
   unchanged unconditional wrapper gate, and fast static regression checks.
4. Acceptance/non-interruption: source isolation checks and pre-tool rejection;
   no installation, launch, service/port/profile mutation or process cleanup.
5. Exclusions: UIUX, runtime implementation, install scripts, Stable packaging,
   workflows, publishing, commit/push/tag/Release, signing and P11 updates/rollback.

## Identity and payload

| Field | Next source contract |
| --- | --- |
| Entry point | `packaging/windows/NextAgentDock.iss` |
| AppId | `{725CE7CB-6F4D-4D78-957E-64CF729FF413}` |
| Product/publisher | `AgentDock Next` / `AgentDock` |
| Root | Current user's `LocalAppData\AgentDock Next` |
| Start menu | Current user's `AgentDock Next` directory |
| Tray shortcut AUMID | `dev.dropabit.agentdock.next.controlpanel`, matching runtime worker's canonical identity |
| Uninstaller | Separate AppId registry identity, files under `{app}\installer` |
| Version inputs | Required canonical `NextVersion` and numeric `NextWindowsVersion`; no default version |
| Architecture | Required `amd64` or `arm64`, explicit rejection of other values |
| Planned output | `AgentDock-NextSetup-{architecture}.exe`, fresh explicit output directory |

The amd64 declaration excludes Arm64 to keep the two candidate architectures
separate; Arm64 uses its own payload. See Inno's
[architecture rules](https://jrsoftware.org/ishelp/topic_setup_architecturesallowed.htm).
No emulation or native installation has been tested.

`next-payload.iss` lists only Core, arbiter, tray and canonical `candidate.json`
from an explicit `NextPayloadDir`. This is **incomplete**: it does not yet model
accepted generation/shim layout, tray dependencies, WSL helpers, skills,
cloudflared or install/uninstall lifecycle. File presence and compile defines do
not prove Next provenance. Neither Stable archives nor Stable install scripts
are accepted payload inputs.

There is no Run/UninstallRun, registry startup, task registration, credential
handling, runtime launch, external process execution or Stable uninstall lookup.
Inno's own eventual uninstall registration is scoped by the Next GUID.
Automatic application closure and restart are disabled.
Previous install directory/group reuse is disabled; shortcuts use a fixed Next
menu path. `PrepareToInstall` rejects any `/DIR` or saved-INF target whose
normalized spelling differs from the exact Next root. `InitializeUninstall`
rejects a different root. These are source declarations only: physical-path,
junction/reparse-point and Windows account ownership validation remain gates.

## Build enablement gates

`prepare-next-rc.py --build-installer` always returns exit 2 before reading a
manifest, creating output, locating tools, building binaries or calling ISCC.
Direct ISCC use also encounters an unconditional first `#error`; no define
enables it. Removing either gate requires a separate reviewed implementation:

1. Accept native Next runtime identities: root/state/workspace, port 8767,
   tray AUMID/singleton/events, service/task/startup/tunnel/credential namespaces,
   discovery and updater targets. Reject Stable/no-identity fallback. Refer to
   the runtime worker's `docs/custom/p10-windows-runtime-isolation.md` when integrated.
2. Implement separately owned Next install/uninstall lifecycle with current-user
   ownership, physical-path/reparse-point rejection, exact task/process ownership,
   bounded activation/cleanup, generation recovery and Stable preservation.
   Do not reuse `scripts/install/*` or `includes/code.iss` as a renamed installer.
3. Produce the complete same-SHA Next payload with canonical candidate bytes,
   architecture and checksum binding; validate actual Core/arbiter/tray/shim PE
   metadata, tray assembly/dependencies, helper/skills manifests and any bundled
   third-party integrity. No recursive/unverified Stable payload fallback.
4. Add an explicit fail-closed pre-ISCC verifier for clean provenance, all payload
   checks, fixed identities, safe fresh output and architecture; wire it to this
   entry point only. Compile on Windows and inspect Setup/output checksums.
5. Under separate installation authorization, exercise side-by-side Windows
   install/uninstall and invalid `/DIR`/INF/junction/account cases. Prove Stable
   files, registry, tasks, credentials, processes and services remain intact;
   verify no unsolicited focus/dialogs/prompts and truthful Next health.

Signing, publication, updater/release readiness and P11 update/rollback acceptance
remain separate gates. Static success cannot replace those results.

## Verification

Fast targeted command (does not run `PackagingFixture` or its cold Go clone):

```sh
rtk proxy python3 -B scripts/test/test-next-windows-packaging.py InstallerSourceTests -v
```

Tests cover identity/architecture, fixed roots and directory guards, exact include
and payload lists, absence of Stable/external runtime calls, unconditional source
compile rejection, and wrapper rejection for both architectures with missing
manifest and empty tool PATH. These are static/source and Python execution checks,
not Inno compilation, Pascal execution, native Windows or installer UAT.
The existing expensive canonical fixture and opt-in crossbuild are not rerun for
this change. Python AST syntax and `git diff --check` provide source lint.

Worker results: **5/5 targeted tests PASS** (4.020s) before owner integration;
after adding the cross-component WPF early-gate regression, combined fast suite
**6/6 PASS**. **2/2 Python AST PASS**,
`git diff --check` PASS. The first test invocation exposed a Python string syntax
error; it was corrected before these successful results. Stable installer,
`includes/code.iss`, offline builder and `scripts/install/*` have no diff.
Concurrent control-panel/runtime-document changes belong to the runtime worker
and were preserved. ISCC, native Windows, CI and installation UAT were not run.

No runtime resources were started. The closest next useful workflow is an accepted
complete Next payload and lifecycle, followed by authorized side-by-side Windows UAT.
