# P10 Windows Next identity preflight

> **較新 Windows 驗收範圍（2026-10-14）：** Windows 現階段 source-only 理論可行，無 Windows 設備、不做原生／Installer 驗收，亦**不阻塞 macOS-focused P10**。本文件其餘內容是當時的實作及未交付事實紀錄；Next Windows 仍 fail-closed 且不可發布。見 [Windows source-only acceptance](p10-windows-source-only-acceptance.md)。

2026-10-14; baseline `8caf0995d5ca773bbe1b0f167cdad273ecc2f95e`,
`feature/p10-release-migration`. P10/M9 remains open; no native admission or
installer activation is claimed. Read with [runtime isolation](p10-windows-runtime-isolation.md)
and [installer source](p10-windows-installer-source.md).

## Workflow checkpoint

1. Pain: Next-labelled Windows management can discover/probe/control Stable.
2. Improvement: installer/runtime maintainers have a reusable explicit identity
   preflight, instead of relying on a rename or Stable manifest defaults.
3. Minimum: an independent C# validator; existing WPF entry gates stay intact.
4. Acceptance/non-interruption: executable source checks preserve every existing
   gate and Stable source byte-for-byte; no apps, services, tasks, registry,
   profiles, network probes or installer execution are started.
5. Exclusions: UI/XAML, release/packaging integration, updates, signing/P11,
   installation, launch, push/tag/Release and commit.

## Strict identity envelope

`Models/NextWindowsManifestIdentity.cs` validates a closed JSON identity envelope,
**not a complete runtime.json payload or an installed runtime**. Future installer
and WPF consumers must share this source/contract before removing entry gates.
No current caller invokes it; passing it never authorizes a runtime operation.
LocalAppData and UserProfile parameters must come from the actual installing
user's Windows special folders, not manifest/environment/CLI overrides.

| Required field | Exact value |
| --- | --- |
| product_variant | `next` |
| product_id | `dev.dropabit.agentdock.next` |
| install_root | Windows LocalAppData + `\AgentDock Next` |
| state_directory, agentdock_home | Windows UserProfile + `\.agentdock-next` |
| agentdock_default_dir | Windows UserProfile + `\AgentDock Next` |
| listen_port | JSON integer `8767` |
| agentdock_task_name, startup_value_name | `AgentDockNext` |
| tray_startup_value_name | `AgentDockNextTray` |
| cloudflared_startup_value_name | `AgentDockNextCloudflared` |

All fields are required; no default/rebase/fallback. Unknown fields (including
legacy `port`), duplicates including case aliases, noncanonical key/value case,
wrong types, nulls, incomplete objects, malformed JSON, BOM, comments and trailing
commas fail. The existing Stable RuntimeManifest is never used. The closed schema
deliberately rejects full legacy manifests; a future complete manifest schema
must explicitly define every additional runtime field and collision rule.

Paths must use exact local-drive spelling. Slash aliases, UNC/device paths,
short-name spellings containing `~`, ADS, dot segments, repeated/trailing
separators, trailing spaces/dots and invalid/control characters fail. Manifest
paths must match the computed paths exactly, including case; no normalization
repairs conflicting inputs. `RequireExistingNonAliasedPath` separately rejects
missing/inaccessible paths, wrong file/directory type and reparse points at the
leaf or any ancestor. It reads attributes only and creates nothing.

These checks do not establish account/ACL ownership, hard-link provenance,
payload integrity or race-free activation. Declarative validation alone cannot
detect physical aliases; future callers must perform the filesystem check and
the remaining ownership/provenance checks. The helper is not an install API.

## Why WPF remains closed and how it can open

Existing GetSnapshotAsync reads Stable-default settings and Nexus, probes health,
runs Core commands and queries startup registration. The unchanged UI exposes
management/update actions. A safe read-only launch would require a separate
status consumer plus action boundaries; opening the existing tray after validating
identity would still admit those unsafe paths. Installer lifecycle and complete
payload ownership are also absent. The current early rejection therefore remains
necessary, rather than treating this envelope as evidence of installed state.

Next closest workflow: installer owner implements a complete same-identity payload
and lifecycle, then WPF gains an isolated status reader with explicit capabilities.
Only after Windows evidence proves zero Stable probes may read-only admission
replace the startup block. Elevated helpers, tasks, mutation and updates each
remain closed until their ownership contracts are implemented and reviewed.

## Reproducible verification and native go/no-go

Source-only macOS checks:

```sh
rtk proxy python3 -B scripts/test/test-next-wpf-runtime.py -v
rtk git diff --check
```

Result: **4/4 source checks PASS**, no skips; Python syntax parses. These assert
the closed-schema boundary, no runtime/mutation dependencies, native fixture
coverage and byte-identical existing WPF gates/Stable source. They do not execute
C# path/JSON semantics. `dotnet` and `pwsh` are unavailable on this host.

The same Python runner's `--native` mode requires Windows and .NET 8. It compiles
the actual validator in a temporary console project with no package sources and
executes valid, missing/null/Stable identities, duplicates/case aliases, malformed
JSON, wrong ports and path spelling attacks. It does not start AgentDock. This
mode was **not run**; it errors on unsupported hosts instead of reporting a skip
as success. Temporary project files are cleaned on exit.

Native go/no-go still requires: that C# matrix; full WPF compilation; real
filesystem junction/symlink/ancestor, ACL/account and hard-link cases; missing or
copied manifest rejection; all helper flags/overrides blocked before Stable reads;
captured rejection exit code/no prompts; unchanged Stable behavior. Future
read-only admission additionally requires zero requests/process probes to Stable
8765, no Stable preference/Nexus/task/credential reads and closed management UI.
Installer/UAT needs separate authorization; no such execution occurred here.

Only the new validator, Python runner and this checkpoint belong to this worker.
Concurrent release-source edits are preserved. No runtime resources were started.
This document is the scoped repository decision record; Codebase Memory tooling
is unavailable here, so no index refresh is claimed. No long-term memory write was
requested. The local 4,000-token task budget was exceeded and surfaced.
