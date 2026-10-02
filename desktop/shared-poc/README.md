# AgentDock Shared Desktop POC

M1 architecture spike for a shared AgentDock desktop UI.

This is **not** a production desktop replacement. It validates a Vue 3 + TypeScript UI hosted by Wails v3 with Go remaining authoritative.

## Scope

Validated in this POC:

- generated Vue ↔ Go bindings;
- read-only integration with the existing AgentDock runtime;
- runtime start / stop / restart command surface;
- bounded synthetic event batching plus Wails Stream/`TrySend` transport backpressure;
- one persisted POC preference and window size;
- native application menu and system tray construction;
- restrictive AssetServer security headers;
- macOS app/DMG packaging;
- Windows ARM64/x64 cross-builds.

See `../../docs/custom/m1-desktop-spike.md` and
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
