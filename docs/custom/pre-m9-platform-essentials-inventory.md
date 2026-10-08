# P7 Nexus / Startup / Platform Essentials — Read-only Inventory

> 2026-10-08 discovery only. UX/authority contract not frozen, no P7 implementation started. P6 still needs native GUI acceptance.

## Existing implementation and gaps

| Capability | Evidence | P7 classification |
| --- | --- | --- |
| Nexus pairing | `cmd/agentdock/command_nexus.go` CLI pair/status; `internal/nexusbridge/identity.go` persisted device identity; native macOS/Windows pairing forms | **Native-backed Port**: Shared pairing UI absent. Never expose pairing code/device token. |
| Nexus status | `nexusbridge.ReadStatus` reports paired, endpoint, IDs, token-stored boolean; `cmd/agentdock/server.go` and `desktopapi.RuntimeStatus.NexusConnected` report connection | **Port**: distinguish paired vs connected vs identity/config failure. |
| Core lifecycle | `desktopapi/runtime.go` and Shared Runtime UI already provide status/start/stop/restart | **Existing Shared**: avoid duplicates. |
| Core autostart | `desktopapi/basic_settings.go` & Shared Basic Settings support capability-aware state; macOS SMAppService controls registration | **Native-backed**: show accurate read-only/unsupported state. |
| Tunnel autostart | `desktopapi/connection_integration.go` and Shared Connection UI already implemented | **Existing Shared**: do not duplicate. |
| Update check/apply | Shared Update card check-only, native updater apply/recovery gated | **Native-only**: apply stays closed until M9 signing/release gate. |
| Diagnostics/Open Logs/Open Config | `desktopapi/diagnostics.go` reports limited metadata, Shared Diagnostics card; no safe file opener yet | **Redesign**: native allowlisted Next-owned paths, no raw log/secret content or arbitrary renderer paths. |
| Tray/menu | Legacy macOS AppKit and Windows tray exist; Next Shared Desktop uses Wails | **Native parity audit**: inventory actual Next menu ownership first. |
| OS permission/elevation | Existing M8 Permission policy + separate OS/native adapters | **Native-backed**: OS grants distinct from AgentDock Allow/Ask/Deny. |

## Proposed UX design questions (not approved)

Preserve existing Connection, Settings, Runtime, Diagnostics page ownership. Prefer focused Nexus status-first UI: connection health, safe identity details, explicit pairing, then advanced/help. Device token is write-only; use `device_token_stored`, not raw value. Pairing must explain restart/reconnect truthfully.

Security roundtable must freeze Desktop-owned pairing authority, re-pair confirmation, stale identity fencing, endpoint SSRF/redirect checks, one-time code handling, safe errors, and unavailable/timeout/retry states. For native shortcuts, allow only Next-owned log/config destinations after symlink/ownership checks. Distinguish Core service startup, App login item, and Tunnel autostart; no stable app/service mutations. Build truthful macOS/Windows/WSL/Linux capability flags plus keyboard/focus/320px checks.

## Next gate

Run sequential Product/UX + Security/operator + Cross-platform design review and freeze interaction/authority contracts **before P7 code**. This was read-only investigation, not P7 implementation or P6 closeout. M9 remains gated.
