# AgentDock Shared Desktop POC

M1/M2 architecture POC for a shared AgentDock desktop UI and framework-neutral Desktop API.

This is **not** a production desktop replacement. It validates a Vue 3 + TypeScript UI hosted by Wails v3 while `internal/desktopapi` remains the framework-neutral contract authority and Go remains authoritative.

## Scope

Validated in this POC:

- generated Vue ↔ Go bindings;
- protocol-v1 manifest / capability negotiation through `internal/desktopapi`;
- read-only integration with the existing AgentDock runtime;
- runtime start / stop / restart command surface;
- versioned Activity envelopes with epoch + decimal-string cursor;
- bounded synthetic Activity batching plus Wails Stream/`TrySend` transport backpressure;
- 256 KiB Activity payload ceiling and explicit source/transport drop accounting;
- one persisted POC preference and window size;
- native application menu and system tray construction;
- restrictive AssetServer security headers;
- macOS app/DMG packaging;
- Windows ARM64/x64 cross-builds.

See `../../docs/custom/m1-desktop-spike.md`,
`../../docs/custom/m2-shared-desktop-api.md` and
`../../docs/custom/adr-shared-desktop-framework.md` for results and remaining
production gates.

## Toolchain

- Go: follows the AgentDock repository toolchain
- Wails: `v3.0.0-beta.27`
- Node + pnpm
- Vue 3 + TypeScript + Vite + Pinia

Install the matching Wails CLI outside the repository:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27
```

## Validate

```sh
go test ./...
go vet ./...

cd frontend
pnpm install
pnpm typecheck
pnpm test
pnpm build
```

## Build

macOS:

```sh
wails3 build
wails3 task package
wails3 task darwin:create:dmg
```

Windows cross-build from macOS/Linux:

```sh
wails3 task windows:build ARCH=arm64 CGO_ENABLED=0
wails3 task windows:build ARCH=amd64 CGO_ENABLED=0
```

Windows installer/signing remains a native Windows runner/device gate.

## Isolation

This directory is a nested Go module. Wails dependencies and the temporary
`go-json-experiment` compatibility replacement must not leak into the AgentDock
root module.
