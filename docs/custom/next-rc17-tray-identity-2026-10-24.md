# NEXT RC17 — Tray identity and two-client Computer Host checkpoint (2026-10-24)

> **後續已安裝 RC17 的最新結果：** [實機建置、GUI／Tray 驗收](next-rc17-installed-gui-uat-2026-10-24.md)。`0.9.2-rc.17` 已受控安裝、App/ZIP/DMG SHA 與簽章 PASS，保留 RC16 rollback；GUI 背景重啟 120 個 sample 前景全部 LINE 通話、無 NEXT 搶焦點，單窗／AX46／ScreenCaptureKit capture PASS。**但 NEXT 原生 Tray 的 AX Title／Description／Help 仍空白，新的 setAccessibilityLabel 實機不能稱之為已被系統代理正確暴露**；Tray 展開及按鈕仍 OPEN。下文的 source-only 狀態是本次安裝前的歷史。

## User workflow and observed gap

Stable AgentDock and AgentDock NEXT intentionally run side-by-side. A user working in Chrome must be able to identify **which menu-bar icon controls which Core** without risking a click on the wrong one. On installed NEXT RC16, a native, **read-only** macOS Accessibility survey confirmed `AXExtrasMenuBar` with exactly **one enabled `AXMenuBarItem`**; it advertised `AXPress`, had valid size/position, and NEXT's app menu separately used the correct `AgentDock Next` name. **However the Tray status item had no accessible Title or Description**, while `AppDelegate.setStatusItemVisible` hard-coded the status image's accessibility description to `AgentDock`, the **Stable** product's name. This is a concrete accessibility/identity mismatch when both Apps are installed.

## Minimal correction (source pending a new installed build)

- NEXT and Stable already select a validated bundle identity via `AppIdentity.current`; `name` is `AgentDock Next` or `AgentDock`.
- Native macOS status button now sets its image `accessibilityDescription`, the button's explicit `setAccessibilityLabel` and its hover `toolTip` to `AppIdentity.current.name`. No changes to menu actions, launchd, Core, default Browser route, login items, Keychain, UpdateStatusItemVisibility, status item autosave or permissions. No new icon design or runtime daemon.
- Existing Swift `AppIdentityTests` now assert the user-facing name is consistent for both variants. Standalone AppKit `swiftc -typecheck` of the actual `NSStatusBarButton.setAccessibilityLabel`/tooltip/Image API **PASS**. An installed RC17 GUI build and actual AX label observation are still required before marking the visual/accessibility change as Host PASS.

## Independent RC16 Computer direct-client Host proof

Two independent HTTP clients initialized against installed NEXT rc.16 Core (stateless MCP endpoint, **no `Mcp-Session-Id` header**, so **not** two different authenticated ACP sessions). Each acquired its own direct `computer_session` with observe-only/foreground=forbidden, resulting **2 active** Broker sessions and distinct Computer session IDs. Client B's request to restore/activate NEXT foreground was **denied before provider action**. A released with cleanup complete; B **continued successfully** to read NEXT GUI 46-node Accessibility state, `focus_changed=false`; B released cleanup complete and Broker active sessions became **0**. Host script exit 0 and private evidence at `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc16-computer-two-client-host.json`.

The NEXT Tray AX presence and `AXPress` capability were read but **never invoked**; no menu was opened, no main UI button clicked. Before/after foreground samples remained Chrome. This is real NEXT process observation, not a simulated UI; nonetheless **Tray menu interactions, crash/cancel between actual ACP agents, full GUI restart no-focus and updater failure rollback remain OPEN**.

## Required next steps and non-work

Make a fixed-source, signed native RC17 candidate, then only during an idle NEXT-only deployment verify that its actual `AXExtrasMenuBar` item exposes `AgentDock Next` label/tooltip and remains usable without hijacking foreground. Keep installed RC16's real positive/negative Edge Host acceptance separately; do not mark an updated binary as rerunning those tests. The only justified installer scope is **NEXT 8767**, preserve the current RC16 rollback, leave Stable 8765, user's Edge/Profile and GitLab scopes untouched. Do not merge `custom/main` or publish before full product gates are accepted.
