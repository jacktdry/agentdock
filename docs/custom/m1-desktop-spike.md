# M1 — Cross-platform Desktop Architecture Spike

> Branch: `architecture/shared-desktop-ui`
>
> Status: Spike implementation complete; production gates remain
>
> Date: 2026-10-02

## Objective

Validate whether AgentDock can move growing business UI out of the duplicated AppKit/WPF implementations into a shared Vue 3 + TypeScript UI while keeping Go as the authoritative runtime and preserving thin OS-specific adapters.

M1 is an architecture spike. It does not replace the production macOS or Windows desktop applications.

## Existing baseline

### Source maintenance surface

| Surface | Approximate size |
| --- | ---: |
| macOS AppKit Swift | 32 files / 7,117 lines |
| Windows control panel C# | 10 files / 5,325 lines |
| Windows XAML | 515 lines |
| macOS English localization keys | 361 |
| Windows UI resource keys | 240 |

These numbers are not a quality judgment. They quantify the duplicated platform surface that grows whenever a new business screen is implemented twice.

### Running macOS baseline

Measured against the currently installed production AgentDock app:

- UI executable: about 1.2 MB, arm64.
- Idle AgentDock UI process RSS at sample time: about 19.6 MB.
- Full `AgentDock.app` bundle: about 105 MB, but this bundle also includes the Core, cloudflared, Skills and other assets. It is therefore not a valid direct comparison with a standalone WebView shell binary.
- Existing app is ad-hoc signed locally.

Resource comparisons in this spike separate UI process cost from the full production bundle.

## Wails POC

The isolated POC lives at:

```text
desktop/shared-poc/
```

It is a nested Go module so the root AgentDock module is not given Wails dependencies.

### Pinned architecture

- Wails v3.0.0-beta.27 (pre-release)
- Vue 3
- TypeScript
- Vite
- Pinia
- system WebView
- Go services as the frontend boundary
- no Electron/Chromium bundle

### Go boundary

The POC intentionally reuses `internal/desktopruntime` for runtime lifecycle behavior rather than reimplementing process, Scheduled Task, Run-key or macOS startup semantics in the UI layer.

Frontend-visible domains in the spike:

- runtime status
- runtime start / stop / restart commands
- POC-only persisted preference
- synthetic high-frequency event source
- bounded batching / dropped-event telemetry

### Backpressure design

The synthetic source is deliberately capable of producing events faster than the frontend receives them.

The Go side uses:

- a bounded producer channel;
- non-blocking producer writes;
- explicit queue-drop accounting;
- bounded batches;
- configurable delivery interval;
- Wails `StreamConn.TrySend` for the Go → WebView data plane;
- explicit transport-drop accounting when the Wails stream buffer is full or disconnected.

The Vue side opens a `JSONStream` and additionally keeps only a bounded recent-event history.

A critical M1 finding is that the normal Wails event bus is **not suitable for high-frequency Activity data**: its internal frontend-event mailbox is an unbounded queue. The POC therefore does not use `Event.Emit` for this path. Low-frequency control/state can still use normal bindings/events, while high-frequency Activity/Call data must use the bounded Stream transport.

This proves the intended architecture rule:

> The Desktop API owns backpressure end-to-end. A UI must never become an unbounded log/event queue, including inside the desktop framework transport.

### Accessibility design

The spike requires:

- semantic native buttons and inputs
- keyboard-native navigation
- visible `:focus-visible`
- one polite summary live region
- raw event logs explicitly excluded from live announcements

The goal is to avoid VoiceOver/Narrator being flooded by high-frequency activity events.

## Wails binding-generation observation

Wails v3 binding generation invokes `go list ... -compiled=true -deps`.

Observed on this machine:

- warm default generation with no relevant Go boundary changes: about 7 s;
- Windows production-target generation with a warm cache: about 8–13 s;
- invalidated macOS generation after relevant Go/native-boundary changes: roughly 1m54s to 3m05s in observed runs.

The macOS path recompiles the Wails CGO / Objective-C bridge when its compiled dependency graph is invalidated. This is a material developer-experience cost rather than a functional blocker, and M2 should avoid unnecessary changes to the binding surface.

Wails beta.27 also selects a `go-json-experiment` snapshot whose jsonv2 alias does not compile with the installed Go 1.27.1 toolchain when that dependency enters the Windows build graph. The POC therefore uses a nested-module-only replacement to the older compatible snapshot already used by AgentDock/chromedp. This workaround stays isolated to the POC and must be removed or formally justified before production adoption.

## Security and packaging outcome

The POC restricts the frontend boundary to 3 services / 7 methods and adds CSP, `no-referrer`, `nosniff` and frame-deny headers. Runtime status was verified against the real installed AgentDock runtime through the same Go service path.

Artifacts produced from the shared source:

- macOS arm64 production binary: about 9.9 MB;
- ad-hoc signed macOS `.app`: about 11 MB;
- verified macOS DMG: about 5.5 MB;
- Windows ARM64 GUI EXE: about 12 MB;
- Windows x64 GUI EXE: about 13 MB.

A Windows installer remains a native Windows gate. The shared source cross-builds both Windows architectures successfully, but installation, uninstall, WebView2 bootstrap behavior and Authenticode still require a Windows runner/device. The macOS-hosted NSIS path was not accepted as production evidence.

### Runtime resource observation

The current native AppKit UI sampled at about 19.6 MB RSS.

The Wails POC is materially heavier: its main process physical footprint sampled around 37–39 MB, while the associated WebContent helper adds tens of MB more plus smaller WebKit GPU/Networking helpers. Main-process RSS varied materially during startup and settling, while idle CPU returned near 0%.

A short observation window showed no sustained growth, but this is not a leak test. Sleep/wake/background and a longer soak remain production gates.

## Tauri 2 comparison

Tauri remains a valid fallback but was not installed during this spike because this machine initially had no Rust toolchain and the AgentDock runtime is already Go.

For AgentDock, a realistic Tauri architecture would add a bridge:

```text
Vue
  ↓ Tauri IPC
Rust shell
  ↓ sidecar protocol / FFI
Go AgentDock runtime
```

whereas Wails is:

```text
Vue
  ↓ generated binding (control) + bounded Stream (high-rate data)
Go AgentDock runtime
```

### Evidence from official Tauri 2 documentation

- External binaries are shipped as sidecars and use target-triple-specific binaries.
  - https://v2.tauri.app/develop/sidecar/
- Windows ARM64 distribution adds Rust target and Visual Studio ARM64 build-tool requirements.
  - https://v2.tauri.app/distribute/windows-installer/
- Tauri capabilities provide a strong explicit permission model per window/webview.
  - https://v2.tauri.app/security/capabilities/
- Tauri updater requires explicit plugin permissions, signed update artifacts and updater key management.
  - https://v2.tauri.app/plugin/updater/

### Why M1 does not install Rust yet

Installing Rust and scaffolding Tauri would only add decision value if it proves something that source/toolchain analysis cannot answer.

A future Tauri executable spike is warranted if Wails fails one of these gates:

1. lifecycle reliability;
2. production signing/notarization/update integration;
3. permission/security requirements that cannot be expressed safely around the Wails boundary;
4. acceptable long-running WebView/CGO resource behavior;
5. Windows packaging or ARM64 parity.

If needed, the Tauri spike must specifically prove Go sidecar ownership, crash/orphan cleanup, bidirectional event latency, installer behavior and update signing. A generic “hello world” Tauri window is not sufficient evidence.

## Validation matrix

| Gate | Result |
| --- | --- |
| Vue + TypeScript typecheck | Pass |
| Frontend unit tests | Pass, 2/2 |
| Go unit tests | Pass |
| Read-only real Runtime status path | Pass |
| Runtime start / stop / restart binding | Implemented; destructive invocation intentionally not exercised |
| Synthetic event batching/backpressure | Pass: bounded producer queue + `StreamConn.TrySend` transport-drop tests |
| Wails framework transport bound | Pass by design/source audit: high-rate path uses bounded Stream, not unbounded Event mailbox |
| Frontend history bound | Pass in Vitest |
| Typed generated control bindings | Pass, 3 services / 7 methods / 9 generated models / 0 custom events |
| High-rate data contract | Explicit JSON Stream wire contract; 1 named bounded stream |
| AssetServer security headers | Pass in unit test |
| Tray + app menu construction | Build/launch pass; interactive automation blocked by Accessibility permission |
| Window-size persistence | Implemented; interactive close/restore automation blocked by Accessibility permission |
| macOS arm64 `.app` | Pass |
| macOS DMG | Pass; checksum verified |
| Windows ARM64 EXE | Pass |
| Windows x64 EXE | Pass |
| Windows NSIS/MSIX installer | Pending native Windows runner; macOS tooling blocked |
| macOS resource baseline | Recorded; WebView materially heavier than AppKit |
| VoiceOver / Narrator | Pending native/manual accessibility validation |
| sleep/wake + long-run leak soak | Pending production gate |
| Developer ID / notarization / update | Pending certificate gate |
| Windows Authenticode / uninstall | Pending Windows runner |

## M1 conclusion

M1 provides enough evidence to start M2 with Wails as the provisional shell. Do not remove AppKit/WPF yet. Production remains gated on Windows installation/signing, accessibility, notarization/update, macOS deployment-target cleanup, the temporary Go 1.27 dependency workaround, and long-running resource behavior.

## Non-goals

M1 does not:

- replace the production AppKit/WPF UIs;
- migrate every current screen;
- publish a production release;
- add Android, iOS or Linux GUI scope;
- exercise destructive runtime lifecycle commands during the POC validation;
- claim a framework is production-ready based only on a successful macOS build.
