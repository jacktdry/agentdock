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

### P0 — Feature Parity Audit + AI UX / IA Design Workshop

先建立 stable native → Next backend → Shared Desktop 的 feature matrix，每項分類為：

- **Port**：功能語意保留，搬到 Shared UI。
- **Redesign**：能力保留，但 UI 需符合新架構。
- **Native-only backend**：Shared UI 可呈現 / 觸發，但 privileged/platform logic 保持 native。
- **Deprecated**：新架構已取代，不再搬舊操作模型。
- **Not needed**：確認沒有產品價值後才排除。

Audit 至少涵蓋 Connection/Tunnel、credentials、Nexus、ACP、MCP、Plugin、Browser、startup/login、Update、Diagnostics/logs/config、OS permissions/elevation、tray/menu。

在任何新的 Shared UI parity implementation 開始前，必須先完成一次 **AI 圓桌 Design Workshop**。這不是視覺 polish，而是產品資訊架構 / 操作模型 gate。

圓桌至少包含以下互補角色：

- Product / operator workflow：使用者最常做什麼、哪些資訊應預設可見；
- UX / information architecture：分類、頁面層級、progressive disclosure、空/錯誤/等待狀態；
- Security / safety UX：credential reveal、permission、approval、destructive actions、風險說明；
- Cross-platform desktop：哪些操作可以 shared、哪些必須 native-backed；
- Technical / architecture critic：確認 UI 不繞過 Core-owned contract、Browser Broker、Permission control、Next isolation。

每個主要分類在進入實作前，圓桌必須產出：

1. page goal / primary user jobs；
2. 必須呈現的 state / metadata；
3. primary / secondary actions；
4. layout hierarchy（summary → detail → advanced）；
5. empty / loading / unavailable / conflict / retry states；
6. 哪些操作需要 confirm、warning、disabled reason；
7. 哪些功能需要 inline help、tooltip、Learn more、first-use explanation；
8. accessibility / keyboard / screen-reader considerations；
9. 窄視窗不可失去可操作性；
10. implementation acceptance criteria。

特別要求：

- Permission / Approval 不只顯示開關；要解釋「這個權限控制什麼」、「Allow / Ask / Deny 的差異」、「批准不代表原操作已執行」、「為什麼某些 Approve Workspace 不可用」。
- Browser Broker 要解釋 company Edge / managed isolated Chrome / explicit override 的差異，不把舊 CDP selector 原樣搬回。
- Connection/Auth 涉及 secrets，reveal/copy 必須 explicit，並以 nearby explanation 說明 Public MCP URL 與 OAuth password 是拿來建立 ChatGPT MCP connector，不是一般 API token。
- 高風險或較少使用的設定採 progressive disclosure，不把所有 advanced fields 平鋪在單一頁。

圓桌先提出 layout / interaction contract，再由 Codex 實作；不可先實作再讓圓桌只做事後美化。

### P1 — AgentDock Next Connection/Auth Readiness

這是第一個實作 checkpoint，也是建立 `mac-dev-next` 前的硬前置。

Shared Desktop 必須直接顯示：

- Local MCP URL；
- **完整 Public MCP URL（含 `/mcp`）**；
- OAuth password（預設遮罩，explicit Show / Hide / Copy）；
- tunnel / public endpoint status；
- public endpoint connection test。

同一頁也要顯示 Next Core local port 與 port health。Next 預設 port 為 `8767`，stable AgentDock 預設 `8765`，Shared Memory endpoint 使用 `8766`。Next **不得與 stable AgentDock 共用 port**。

Port UX / validation 必須做到：

- `8765` 視為 stable-reserved，Next 不允許設定；
- `8766` 視為 shared Memory-reserved，不允許拿來作 Next Core port；
- `8767` 為 Next 預設；
- 自訂 port 儲存 / apply / start 前必須做 occupancy preflight；
- preflight 要區分「目前 Next 自己持有」與「其他 process/service 已占用」，不能只做單純 connect 成功/失敗；
- foreign listener / ownership 不明時 fail closed，UI 明確顯示 port conflict；
- port 變更後 Tunnel target / Local MCP URL / Public endpoint health 必須同步更新，不可留下 stale target。

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

#### Cloudflare Tunnel isolation decision

正式 connector 應為 AgentDock Next 建立 **獨立 Named Cloudflare Tunnel**，而不是讓 stable AgentDock 與 Next 共用同一個 tunnel/token。

理由：

- stable / Next 必須可以同時運作；
- 各自有不同 Core port / service ownership / lifecycle；
- 共用 tunnel token 或 hostname 會讓 rollback、restart、credential rotation 與 ownership 變模糊；
- `mac-dev-next` 需要固定 public MCP URL，Quick Tunnel 的 ephemeral hostname 不適合；
- Next retirement / migration 不應影響 stable public endpoint。

因此 P2 要求：

- dedicated Next Cloudflare Tunnel identity；
- dedicated hostname / route；
- dedicated token / credential storage；
- origin 指向 Next Core port（預設 `127.0.0.1:8767`，若使用者合法改 port 則同步更新）；
- Next-owned tunnel service label / logs / state；
- stable tunnel/token/hostname 不可作 fallback；
- Public MCP URL 顯示為該 hostname + `/mcp`。

實際 Cloudflare Tunnel / DNS 建立屬於部署動作；在執行前先由使用者確認 hostname / account target。若目前只做本機 UI/contract，可先完成 API、validation 與 fixture，不碰既有 stable Cloudflare Tunnel。

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
| Connection status | 有 | 有 | 有 | Complete；需補 port ownership health |
| Public MCP URL | 有 | 有 | **只顯示 origin** | P1 Port |
| OAuth password | 有 | 有 | **未提供** | P1 Port with secret-safe UX |
| Tunnel configuration | 有 | 有 | 部分 | P2 Port；Next 使用獨立 Named Tunnel |
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

