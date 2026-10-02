# ADR — Shared Desktop Framework

> Status: Provisional — proceed to M2 with production gates
>
> Date: 2026-10-02

## Context

AgentDock already has a Go Core while desktop business UI is split between Swift/AppKit and C#/WPF. The custom roadmap adds Activity/Call observability, Browser Routing, ACP Manager, Permissions and shared i18n. Continuing to implement each business workflow twice would make upstream integration and product maintenance progressively more expensive.

## Decision under evaluation

Use a shared Vue 3 + TypeScript business UI with Go remaining authoritative.

The current candidate is Wails v3. Tauri 2 remains a fallback/reference option.

This ADR is not yet a final production adoption decision because Wails v3 is still pre-release.

## Why Wails is the first executable candidate

- AgentDock already uses Go for the Core and desktop runtime semantics.
- Wails exposes Go services directly to a system-WebView frontend.
- Wails v3 includes native application menu and system-tray APIs in Go.
- A shared Vue UI matches the existing frontend skill set without introducing a second systems language.
- The runtime bridge can reuse existing `internal/desktopruntime` behavior.

## Why Tauri remains relevant

Tauri 2 has a mature capabilities model, updater ecosystem and packaging story. Those are legitimate advantages.

For this codebase, however, adopting it would also introduce Rust and require an explicit Rust-to-Go boundary (typically a sidecar protocol or FFI). That extra boundary needs a concrete benefit to justify its lifecycle, testing, CI and failure-mode cost.

## Provisional decision

Proceed with Wails v3.0.0-beta.27 as the **shared-desktop migration candidate for M2/M3/M4**, not yet as the production framework.

M1 demonstrated direct generated Vue↔Go control bindings, reuse of the existing runtime lifecycle layer, bounded high-rate delivery through Wails Stream/`TrySend`, POC settings persistence, native menu/tray construction, an ad-hoc signed macOS app plus verified DMG, and Windows ARM64/x64 GUI cross-builds from the same source. The frontend control boundary is intentionally limited to 3 services / 7 methods, high-rate Activity data avoids the Wails Event bus because its internal mailbox is unbounded, and the AssetServer is protected with restrictive browser security headers.

The tradeoffs are also concrete: the WebView shell uses materially more memory than the current AppKit UI, invalidated macOS binding generation can take roughly two to three minutes after relevant Go/native-boundary changes, and beta.27 currently needs an isolated `go-json-experiment` compatibility replacement for Go 1.27 on the Windows dependency path.

Tauri remains the fallback if Wails fails a production acceptance gate where its capabilities or distribution ecosystem provide a concrete benefit that justifies the Rust↔Go boundary.

## Required production gates

Before this ADR can become “Accepted”:

1. Windows NSIS/MSIX installation, uninstall and WebView2 behavior must pass on a Windows runner/device;
2. Windows Authenticode signing must be proven;
3. macOS Developer ID signing, notarization and update flow must be proven;
4. tray/menu/window-state semantics must receive native interactive validation;
5. VoiceOver and Narrator must pass the core shared flows;
6. long-running sleep/wake/background and memory behavior must pass a soak test;
7. the macOS deployment-target linker warning must be resolved;
8. the temporary `go-json-experiment` workaround must be removed or formally justified;
9. Wails pre-release/version policy must be explicitly accepted or moved to a sufficiently stable release;
10. the shared WebView must remain a trusted local UI surface: no remote application content, no executable user-provided HTML, restrictive CSP, and a minimal reviewed privileged Go binding surface;
11. upstream merge surface must remain isolated from the official AgentDock mirror.

## Rejected shortcuts

- Do not port production business logic into Vue while keeping duplicate AppKit/WPF state machines.
- Do not choose Tauri based only on general ecosystem maturity without proving the Rust↔Go boundary.
- Do not choose Wails merely because the first macOS build works.
- Do not replace native credential, privilege, startup, installer or signing semantics with browser-side implementations.
