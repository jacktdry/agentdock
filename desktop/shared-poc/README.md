# AgentDock Shared Desktop Baseline

This nested module hosts the cross-platform AgentDock desktop shell built with Wails v3, Vue 3 and TypeScript. M1/M2 established the architecture POC; M3 established the i18n foundation; M4 moves the first production-facing baseline surfaces into the shared shell while keeping the native AppKit/WPF interfaces as fallbacks.

`internal/desktopapi` remains the framework-neutral contract authority. Go owns runtime, connection, settings, update and diagnostics semantics; Wails is a binding adapter and Vue renders/orchestrates state.

## M4 surfaces

Primary shared navigation now includes:

- Overview
- Runtime
- Connection
- Settings
- System — Update + Diagnostics

A secondary Developer / Architecture view retains the experimental Activity stream and Desktop API contract diagnostics.

M4 intentionally does **not** move Browser, ACP, MCP, Plugin or Permission management into the shared UI.

### Important native-only boundaries

- named tunnel / Tailscale credentials and connection configuration;
- macOS SMAppService startup registration;
- privileged UAC/TCC and installer operations;
- update install/apply, signing and recovery;
- native AppKit/WPF fallback surfaces.

The shared Update API is check-only. Diagnostics exposes only secret-safe platform/runtime metadata. Basic Settings is limited to port, log level and core autostart, with platform capability flags preventing the shared shell from pretending macOS startup registration is mutable.

## Existing architecture validation

The module also retains the earlier architecture probes:

- generated Vue ↔ Go bindings;
- protocol-v1 manifest / capability negotiation;
- versioned Activity envelopes with epoch + decimal-string cursor;
- bounded synthetic Activity batching and explicit drop accounting;
- 256 KiB Activity payload ceiling;
- local shell preferences and window size persistence;
- native application menu and system tray construction;
- restrictive AssetServer security headers.

See:

- `../../docs/custom/m1-desktop-spike.md`
- `../../docs/custom/m2-shared-desktop-api.md`
- `../../docs/custom/m3-i18n-foundation.md`
- `../../docs/custom/m4-desktop-api-backend.md`
- `../../docs/custom/adr-shared-desktop-framework.md`

## Toolchain

- Go: follows the AgentDock repository toolchain
- Wails: `v3.0.0-beta.27`
- Node + pnpm
- Vue 3 + TypeScript + Vite + Pinia
- M3 semantic-key i18n with `intl-messageformat`

Install the matching Wails CLI when it is not already available:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27
```

## Validate

From the repository root:

```sh
make i18n-check
go test ./internal/desktopapi ./internal/desktopruntime ./internal/envstore ./internal/selfupdate ./internal/fs/atomicfile
go vet ./internal/desktopapi ./internal/desktopruntime ./internal/envstore
```

From `desktop/shared-poc/frontend`:

```sh
pnpm install
pnpm typecheck
pnpm test
pnpm build
```

From `desktop/shared-poc`:

```sh
go test ./...
wails3 build
```

Windows cross-builds:

```sh
wails3 task windows:build ARCH=arm64 CGO_ENABLED=0
wails3 task windows:build ARCH=amd64 CGO_ENABLED=0
```

Windows installer/signing remains a native Windows runner/device gate.

## Internal naming

The directory, nested Go module, bundle identifiers and executable name still use the historical `shared-poc` identifier during M4 to avoid mixing product-surface migration with a repository/package rename. User-visible product names are `AgentDock Desktop`. A future cleanup can rename internal identifiers separately.

## Isolation

This directory is a nested Go module. Wails dependencies and the temporary `go-json-experiment` compatibility replacement must not leak into the AgentDock root module.
