# Pre-M9 Feature Parity and Connection Readiness

> 狀態：Accepted execution order（2026-10-05）
>
> 前置：M8 Permission / Approval 已 closeout。此文件定義進入 M9 Release Migration 前的功能補齊順序；不是新的大型 Milestone，也不改變 M9 的 release scope。

## 為什麼需要這一層

目前 AgentDock Next / Shared Desktop 已完成 shared architecture、Activity / Execution、ACP lifecycle、Permission / Approval 等核心 vertical slices，但尚未達到舊 native AgentDock 的完整日常功能 parity。

因此 M9 前先完成一次 bounded parity audit，並優先補齊「讓 AgentDock Next 可以獨立被 ChatGPT 使用」所需的 connection/auth surface。不要等所有 Shared UI parity 完成才建立 `mac-dev-next`；一旦 Next connection prerequisites 完成，就先建立 side-by-side connector，後續功能開發即可直接以 Next 作為獨立 control plane。

## 不可破壞的邊界

- stable AgentDock / `mac-dev` 是 production control plane；不得為 Next 開發修改、重啟、停止、替換或拿來做 runtime 驗證。
- Next 的 runtime/state/log/work roots、services、tunnel、credentials 與 connector 必須獨立。
- Next 缺少設定時 fail closed；不得 fallback 讀取或寫入 stable `~/.agentdock`。
- Desktop-control credential、provider credential、raw auth secrets 不得進普通 frontend state、Activity、Diagnostics、logs 或 Memory。
- 使用者需要建立 ChatGPT MCP 的 OAuth password 可以在可信任 local Shared Desktop UI 中以 explicit Show / Copy 操作呈現，但預設遮罩，且不得被一般 snapshot / telemetry / diagnostic surface 洩漏。

## 執行順序

### P0 — Feature Parity Audit

先建立 stable native → Next backend → Shared Desktop 的 feature matrix，每項分類為：

- **Port**：功能語意保留，搬到 Shared UI。
- **Redesign**：能力保留，但 UI 需符合新架構。
- **Native-only backend**：Shared UI 可呈現 / 觸發，但 privileged/platform logic 保持 native。
- **Deprecated**：新架構已取代，不再搬舊操作模型。
- **Not needed**：確認沒有產品價值後才排除。

Audit 至少涵蓋 Connection/Tunnel、credentials、Nexus、ACP、MCP、Plugin、Browser、startup/login、Update、Diagnostics/logs/config、OS permissions/elevation、tray/menu。

### P1 — AgentDock Next Connection/Auth Readiness

這是第一個實作 checkpoint，也是建立 `mac-dev-next` 前的硬前置。

Shared Desktop 必須直接顯示：

- Local MCP URL；
- **完整 Public MCP URL（含 `/mcp`）**；
- OAuth password（預設遮罩，explicit Show / Hide / Copy）；
- tunnel / public endpoint status；
- public endpoint connection test。

目前 Shared Connection UI 只顯示 sanitized HTTPS origin，OAuth password 仍只存在 native macOS/Windows UI；P1 要補齊這個 parity gap。

安全要求：

- UI 只能讀 Next-owned credential/state；
- credential 不進 logs、Diagnostics、Activity、Memory 或一般 Desktop API snapshot；
- Desktop-control credential 永遠不得暴露到 frontend；
- Copy / reveal 必須是 explicit user action。

### P2 — Complete Public Access / Tunnel UI

補齊 Shared Connection 的設定能力：

- Local only；
- Quick Tunnel；
- Named Tunnel / custom domain；
- Tunnel Token；
- public endpoint / status；
- tunnel autostart；
- regenerate 僅 Quick mode 可用。

正式 `mac-dev-next` 優先採固定 HTTPS endpoint / Named Tunnel。Quick Tunnel 可作 smoke，不作長期 ChatGPT connector address。

Next tunnel 必須使用 Next-owned service/config/state，例如 `dev.dropabit.agentdock.next.tunnel`，不得重用 stable tunnel identity。

### P3 — 使用者建立 `mac-dev-next` ChatGPT Connector

**這一步由使用者手動操作，不由 Agent 自動建立。**

P1/P2 完成後，AgentDock Next UI 應已提供建立 ChatGPT 個人 MCP 所需的：

- Public MCP URL；
- OAuth password。

使用者在 ChatGPT 建立獨立 `mac-dev-next` 後，再由開發 session 驗證：

- `mac-dev` 與 `mac-dev-next` 同時可用；
- Next lifecycle 不影響 stable；
- stable lifecycle 不是 Next 的 ownership authority；
- Next connector 使用 Next credential / endpoint；
- Next task/state/registry 不與 stable 混用。

此 gate 完成後，後續 P4+ 開發優先使用 Next control plane；stable 保留安全 fallback。

### P4 — ACP Manager Full Parity

在既有 M7 lifecycle / diagnostics UI 上補：

- Add / Edit / Delete profile；
- Enable / Disable；
- Default profile；
- detect installed；
- installed/latest version；
- update；
- Codex / Antigravity presets；
- custom adapter。

Shared ACP UI 不只做 Runtime Monitor，要完成 M7 原訂 package/profile management scope。

### P5 — MCP Management Shared UI

補 `DomainMCP`：

- configured / enabled state；
- health；
- safe config summary；
- add / edit / delete / enable / disable；
- protocol/version；
- restart/reconnect-required semantics；
- dynamic MCP status。

Secrets 不在一般 list/status 中明文呈現。

### P6 — Plugin Management Shared UI

補 `DomainPlugin`，以管理已連線 / 已安裝的 plugins 為主，不發展 marketplace：

- status / health；
- enable / disable；
- permissions；
- configuration；
- disconnect / uninstall。

### P7 — Browser Broker Shared UI

**不要 1:1 搬回舊 Browser CDP 設定。**

舊的 managed / reuse existing CDP / specified CDP URL 已由 M6 Browser Broker 架構取代。Shared UI 應重新設計為：

- workspace-aware routing；
- company authenticated Edge required route；
- managed isolated Chrome；
- explicit override；
- connector health；
- leases / owners / pages / contexts；
- worker/process ownership；
- queue/concurrency；
- cleanup / recovery status。

### P8 — Nexus / Startup / Platform Essentials

補齊日常 parity：

- Nexus endpoint / pairing / device status；
- startup / autostart；
- Open Logs / Open Config；
- tray/menu status；
- platform permission / elevation entry points。

真正 privileged/platform mutation 保持 native adapter，不把 privilege logic 搬進 Vue。

### P9 — Pre-M9 Hardening

1. 修正既有 `packaging/macos/app-identity.sh` script-governance inventory debt，讓 root suite 不再需要 baseline exception。
2. 針對 M7 Codebase Memory lifecycle debt 做 bounded triage：
   - 若 AgentDock install/update 會造成 installed-vs-running build mismatch 或破壞使用者正常工作，M9 前修；
   - 若只屬於外部 Codebase Memory 自身 executable 更新的 maintenance 問題，正式標記 post-M9 debt，不擴大 M9 scope。

### P10 — M9 Release Migration

完成 `custom/main` release source、version/tag/buildinfo/installer/update-channel contract、macOS/Windows artifacts 與 GitHub Release pipeline。

### P11 — Release-native UAT

在 M9 release candidate artifacts 上執行：

- macOS Developer ID signed package；
- certificate-bound GUI updater update → trial → commit；
- rollback / interrupted recovery；
- Windows native UAT；
- WSL native UAT；
- Linux-only helper native dispatch / scan execution；
- install / update / rollback / recovery。

Cross-build 不等於 native UAT；無原生環境時必須明確記錄 external gate。

### P12 — Native Business UI Deprecation

只有在 M9 closeout 且 Shared UI parity 經 release 驗證後，才逐步 deprecated 重複的 AppKit/WPF business UI。

保留真正需要 native 的 platform adapters，例如 OS permissions、credential store、startup/elevation、installer/updater/signing integration。

## Initial parity matrix

| Domain / 功能 | Stable native | Next backend | Shared UI | 決策 |
| --- | --- | --- | --- | --- |
| Runtime | 有 | 有 | 有 | Complete |
| Activity / Execution | 舊版較少 | 有 | 有 | Complete / Next-enhanced |
| Permission / Approval | OS/native 為主 | 有 | 有 | Complete / Next-enhanced |
| Connection status | 有 | 有 | 有 | Complete |
| Public MCP URL | 有 | 有 | **只顯示 origin** | P1 Port |
| OAuth password | 有 | 有 | **未提供** | P1 Port with secret-safe UX |
| Tunnel configuration | 有 | 有 | 部分 | P2 Port |
| ACP lifecycle/health | 部分 | 有 | 有 | Complete / Next-enhanced |
| ACP profile/package CRUD | 有 | 有/部分 | 未完整 | P4 Port |
| MCP management | Core/native | 有 | unavailable | P5 Port |
| Plugin management | Core/native | 有 | unavailable | P6 Port |
| Browser config | 舊 CDP 模式 | Browser Broker | 未提供管理頁 | P7 Redesign |
| Nexus | 有 | 有 | 未提供 | P8 Port |
| Startup / autostart | 有 | 有 | 部分 | P8 Native-backed Port |
| Update check | 有 | 有 | 有 | Complete |
| Update apply/recovery | 有 | 有 | native-only | M9 / native-backed |
| Logs / config shortcuts | 有 | 有 | 未提供 | P8 Port |
| OS permissions / elevation | 有 | platform-specific | 未完整 | Native-only backend |

Audit 過程若發現其他 stable-only 功能，必須先加入矩陣並分類，不可因 Shared UI 沒有入口就默認「不需要」。

## Recommended execution chain

```text
M8 closeout
  ↓
P0 Feature Parity Audit
  ↓
P1 Next Connection/Auth Readiness
  ↓
P2 Public Access / Tunnel UI
  ↓
P3 USER: create mac-dev-next in ChatGPT
  ↓
verify stable + Next side-by-side
  ↓
P4 ACP Manager full parity
  ↓
P5 MCP Management
  ↓
P6 Plugin Management
  ↓
P7 Browser Broker UI
  ↓
P8 Nexus / startup / platform essentials
  ↓
P9 Pre-M9 hardening
  ↓
P10 M9 Release Migration
  ↓
P11 release-native UAT
  ↓
M9 closeout
  ↓
P12 duplicated native business UI deprecation
```

