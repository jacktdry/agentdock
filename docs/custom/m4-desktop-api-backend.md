# M4 Shared Desktop Baseline

M4 moves the first production-facing AgentDock desktop surfaces into the shared Wails + Vue shell while preserving the native AppKit/WPF applications as fallbacks. The framework-neutral contract remains in `internal/desktopapi`; Wails only binds services and Vue only renders/orchestrates them.

Protocol version remains v1. The M4 domain additions are additive.

## Shared domains

| Domain | Operations | Boundary |
| --- | --- | --- |
| Runtime | status; start, stop, restart | Existing M2 runtime contract. Mutations require confirmation. |
| Connection | status; start, stop, restart, regenerate | Existing `desktopruntime` tunnel adapter. Regenerate is quick-mode only and is enforced in both UI and backend. Connection configuration and credentials stay native-only. |
| Settings | read; save | Port, log level and core autostart only. Advanced Browser/ACP/MCP/OAuth configuration is preserved but not exposed. |
| Update | check | `selfupdate.Check` only. Apply/install/signing/recovery remain native. |
| Diagnostics | snapshot | Secret-safe platform, architecture, runtime-root and manifest availability only. No file/log/env contents. |

Activity remains experimental. Browser, ACP, MCP, Plugin and Permission remain explicitly unavailable in the shared contract.

## Basic Settings ownership

Basic settings are centralized in `internal/desktopruntime` instead of creating a Vue-owned or Wails-owned configuration state machine.

Darwin/Linux:

- reads the existing managed environment file;
- rejects symlinks/non-regular files and broad permissions;
- on macOS also requires current-user ownership;
- patches only `AGENTDOCK_PORT` and `AGENTDOCK_LOG_LEVEL`;
- preserves all unrelated environment values and credentials;
- writes atomically with private permissions;
- preserves service/tunnel running state and rolls back exact managed file bytes when apply/restart fails;
- keeps macOS autostart immutable in the shared shell because SMAppService remains owned by the native application.

Windows:

- patches only `port` and `log_level` in `control-panel-settings.json`;
- preserves advanced and unknown JSON fields;
- reuses the existing runtime/service/tunnel transaction and autostart adapter;
- preserves the previous core/tunnel running state for the Basic Settings path;
- retains the legacy full native configuration transaction semantics separately.

Quick Tunnel target metadata is updated when the core port changes so an active quick tunnel cannot continue proxying the previous local port.

## WebView safety

Shared service results intentionally exclude credentials, auth tokens, arbitrary environment values, ACP command environment, task/tool payloads and log/file contents.

- raw platform errors and stderr are converted to fixed structured `APIError` values;
- public connection URLs are reduced to HTTPS origins without credentials, query, fragment or non-root path;
- update free-form implementation messages are not forwarded;
- diagnostics reads availability metadata only;
- named tunnel / Tailscale configuration remains native-only.

## Shared UI

Primary navigation:

1. Overview
2. Runtime
3. Connection
4. Settings
5. System — Update & Diagnostics

Contract and synthetic Activity diagnostics are retained under a secondary Developer / Architecture view.

The shared UI uses the M3 semantic-key i18n contract and supports English, Traditional Chinese and Simplified Chinese plus Follow System mode. M4 adds no router dependency; local typed navigation is sufficient for the current shell.

Mutating Runtime, Connection and Settings actions require explicit confirmation. Update is check-only. macOS autostart is displayed but disabled with a native-app explanation.

## Validation

Completed during M4 development:

- affected Go unit tests and `go vet`;
- Desktop API and desktopruntime Windows amd64 cross-compilation;
- shared Wails module Go tests on macOS;
- Vue `vue-tsc`, Vitest and production Vite build;
- strict M3 i18n generate/check with all stable locales at 100% coverage;
- Wails production build on macOS;
- Wails Windows x64 and ARM64 production cross-builds;
- browser DOM/responsive UAT of Overview, Settings, System, Developer navigation and locale switching.

Plain-browser UAT intentionally cannot reach Wails bindings, so it verifies the frontend's fail-closed error rendering rather than backend success. Native executable launch was verified separately. macOS desktop screenshot capture was unavailable to the command process because it does not hold Screen Recording permission.

Windows installer/signing and update-apply acceptance remain native-device gates and are outside the M4 shared-shell scope.

## Internal naming

The historical `desktop/shared-poc` path, module/binary identifier and bundle identifiers are retained during M4 to avoid mixing a repository/package rename with product-surface migration. User-visible product naming is now `AgentDock Desktop`.
