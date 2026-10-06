# Pre-M9 Feature Parity and Connection Readiness

> 狀態：**P1/P2 deployed + live readiness validated；P3 stable/Next side-by-side validation 已完成（2026-10-06）；P4 ACP Manager full parity 已完成、review 並部署至 Next；P5 MCP Management contract 已凍結，現在進 Core/Desktop authority implementation。M9 仍須等待必要 parity + hardening integration review**
>
> 前置：M8 Permission / Approval 已 closeout。此文件定義進入 M9 Release Migration 前的功能補齊順序；不是新的大型 Milestone，也不改變 M9 的 release scope。
>
> Wave 0 的實際 parity 修正、Connection IA、secret classification、port ownership、Tunnel state / mutation semantics 與 Wave 1 worker boundaries 已凍結於 [Pre-M9 Wave 0 Contract](pre-m9-wave0-contract.md)。Wave 1 backend integration 已落在 `94367b74`，Shared Connection UI 已落在 `80eeed53`，final port rollback lifecycle hardening 已落在 `b7e440cc`。2026-10-06 又完成實際 Next deployment/readiness hardening：Codex launchd resolution `6a3dac19`、Shared macOS package `2dc80377`、SMAppService update/constraint handling `63730804` / `821d7e64` / `884f158d`、macOS port ownership cgo gate `6fac35b2`、public-access OAuth provisioning `82251a3a`、live Core health reporting `36683aec`、native Wails clipboard copy `a7e92f2c`。使用者已在 ChatGPT 建立 Next connector；ChatGPT 顯示名稱為 `macbook-air-m3`，邏輯角色仍是 `mac-dev-next`。2026-10-06 已完成 P3 stable/Next side-by-side validation：Wave 3 可以開始，但不得直接進 M9；M9 仍需等必要 parity 與 hardening integration review。
>
> Live readiness（2026-10-06）：`AgentDock Next.app` 使用 Shared Desktop，Next Core 維持 `127.0.0.1:8767`；獨立 Cloudflare Named Tunnel `mac-dev-next` 與 `mac-dev-next.dropabit.dev` 已建立並健康連線，remote ingress 明確指向 `http://localhost:8767`，Public MCP URL 為 `https://mac-dev-next.dropabit.dev/mcp`。OAuth discovery metadata 與 unauthenticated MCP challenge 已由 public HTTPS 實測通過；Next OAuth / Tunnel credentials 僅存於 Next-owned private state，文件不記錄 secret。stable `mac-dev.dropabit.dev`、stable tunnel、stable Core `8765` 未變更。
>
> P3 side-by-side closeout（2026-10-06）：stable `mac-dev` 與 Next `macbook-air-m3` 可同時呼叫；兩者分別回報 `~/.agentdock` / `~/AgentDock` 與 `~/.agentdock-next` / `~/AgentDock Next`。stable listener 為 `127.0.0.1:8765` 且 executable 位於 `/Applications/AgentDock.app/Contents/Helpers/agentdock`；Next listener 為 `127.0.0.1:8767` 且 executable 位於 `~/Applications/AgentDock Next.app/Contents/Helpers/agentdock`。Task store 分別為 `~/.agentdock/tasks` 與 `~/.agentdock-next/tasks`，MCP registry 內容與 transport 亦不同。Public protected-resource metadata 分別宣告 `mac-dev.dropabit.dev/mcp` 與 `mac-dev-next.dropabit.dev/mcp`。最後由 stable `mac-dev` 僅重啟 `dev.dropabit.agentdock.next.core`：Next PID 改變並恢復健康，stable Core PID 保持不變，兩個 connector 在重啟後仍可呼叫。

## 為什麼需要這一層

目前 AgentDock Next / Shared Desktop 已完成 shared architecture、Activity / Execution、ACP lifecycle、Permission / Approval 等核心 vertical slices，但尚未達到舊 native AgentDock 的完整日常功能 parity。

因此 M9 前先完成一次 bounded parity audit，並優先補齊「讓 AgentDock Next 可以獨立被 ChatGPT 使用」所需的 connection/auth surface。Next connector 已先建立，不等待所有 Shared UI parity；但 **stable `mac-dev` 仍是 Next repo/build/install/service/tunnel mutation 的唯一 control plane**。Next connector（ChatGPT 顯示為 `macbook-air-m3`）只作 Next runtime / connector / side-by-side 驗證目標，不做自我修改或自我重裝。

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

P1 已完成：Shared Connection UI 現在顯示 Local MCP URL、完整 Public MCP URL（含 `/mcp`）、explicit OAuth password Show / Hide / Copy、public endpoint test 與 ownership-aware port state；ordinary snapshot 不含 password / tunnel token / Desktop-control credential。

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
- dedicated Named route 的 Service URL 必須指向 Next Core port（正式 connector 預設 `127.0.0.1:8767`）；current token-only remotely-managed backend **無法**自動改寫 Cloudflare route 的 Service URL，因此 custom-port transition 必須依 [Wave 0 contract](pre-m9-wave0-contract.md#12-named-tunnel-remote-route-limitation) fail closed / 先解除 Named exposure，再由使用者更新 dedicated route 後重新驗證；
- Next-owned tunnel service label / logs / state；
- stable tunnel/token/hostname 不可作 fallback；
- Public MCP URL 顯示為該 hostname + `/mcp`。

實際 Cloudflare Tunnel / DNS 建立屬於部署動作；在執行前先由使用者確認 hostname / account target。若目前只做本機 UI/contract，可先完成 API、validation 與 fixture，不碰既有 stable Cloudflare Tunnel。

### P3 — Next ChatGPT Connector + side-by-side validation

**Connector 建立動作由使用者手動完成；目前已完成。ChatGPT 顯示名稱為 `macbook-air-m3`，邏輯角色仍是 `mac-dev-next`。**

P1/P2 完成後，AgentDock Next UI 應已提供建立 ChatGPT 個人 MCP 所需的：

- Public MCP URL；
- OAuth password。

P3 驗證已完成，closeout evidence：

- stable `mac-dev` 與 Next connector `macbook-air-m3` 已確認可同時呼叫；
- stable / Next runtime home、default dir、task store、MCP registry 皆為不同 namespace；
- stable Core 維持 `8765`，Next Core 維持 `8767`，listener executable 分別來自 stable App 與 `AgentDock Next.app`；
- public protected-resource metadata 分別綁定 `mac-dev.dropabit.dev/mcp` 與 `mac-dev-next.dropabit.dev/mcp`，而 Next connector 實際回報 Next runtime identity，沒有 stable endpoint/state fallback 證據；
- 由 stable `mac-dev` 執行一次 Next-only Core restart 後，Next 恢復健康、stable Core PID 不變，兩個 connector 均持續可用。

P3 已 closeout，可進 P4+ / Wave 3。**後續 Next repo 修改、build/package、App reinstall、service/tunnel mutation 仍由 stable `mac-dev` 執行；`macbook-air-m3` 僅用來驗證 Next connector/runtime 行為。M9 仍須等待必要 parity + hardening integration review。**

### P4 — ACP Manager Full Parity

**狀態：Complete / deployed to AgentDock Next（2026-10-06）。** UX / IA contract 先於實作凍結於 [ACP Manager roundtable](pre-m9-acp-manager-roundtable.md)。實作 sequence 為 `5b365a21` → `85e3e75e` → `39b9c127` → `bb40bfd3` → `f9972a67` → `16f63708` → `fa3deb68` → `c58a6f93`。

已完成：

- Add / Edit / Delete profile；
- Enable / Disable 與 default profile；
- installed adapter detection，且 detect result 只 overlay，explicit **Use detected** 才持久化；
- installed/latest version 與 update state；
- Safe Update 只允許 backend 可證明的 Next-owned target + approved release source + SHA-256 digest；目前沒有可信 AgentDock fork release 時 fail closed，不以 upstream artifact 取代 hardened fork；
- Codex / Antigravity presets 與 custom adapter；
- protected args 不回傳 frontend；敏感參數辨識涵蓋 API key/token/Authorization/JSON credential 等常見形狀；
- revisioned settings / stale-write conflict，editor draft 固定使用開啟當下的 base revision，background reload 不會替 stale draft 升級 revision；
- persisted disable 與 currently-running runtime state 分離：restart 前仍可看到與關閉現有 sessions；
- Unix Next-owned update target 驗證 owner / permission / symlink 邊界；release metadata 與 asset redirect 均 fail closed；
- update staging 不依賴 executable `/tmp`，version probe 限制 trusted adapter 形狀、輸出大小與版本格式。

Verification / review：

- Go `internal/config` / `internal/desktopruntime` / `internal/desktopapi` 全綠；
- Windows amd64 / Linux amd64 ACP / contract compile-only checks 全綠；
- Shared frontend typecheck、88 tests、development build 全綠；
- independent Codex security/correctness review 找出的 HIGH findings 已在 `fa3deb68` / `c58a6f93` 修正，final targeted re-review 對 secret detection 與 stale editor revision 均回報 **RESOLVED**，無新 blocker/high；
- Antigravity reviewer 因 AGY OAuth timeout 未產出可用 review，因此不宣稱完成 cross-model review；
- arm64 ad-hoc Shared package 由 `c58a6f93` 建置並通過 bundle identity、helper architecture、strict codesign、ZIP/DMG 與 8 個 package verifier tests；已由 stable `mac-dev` 原子替換 `~/Applications/AgentDock Next.app`，只重新註冊 Next Core/Tunnel；
- live Next Core 仍在 `127.0.0.1:8767`，stable Core 仍在 `127.0.0.1:8765` 且整個 Next replacement / service re-registration 過程 stable Core PID 保持不變；
- `macbook-air-m3` 在部署後可回報 `~/.agentdock-next` / `~/AgentDock Next`，ACP profiles 為 `codex` + `antigravity`，兩者 observation-only status 正常；
- public protected-resource metadata 仍綁定 `https://mac-dev-next.dropabit.dev/mcp`，unauthenticated `/mcp` 仍回 401。

Live GUI 的人工視覺 inspection **未宣稱完成**：Next 的 Orca Computer Control provider 當下 unavailable，stable control plane 又沒有 Accessibility / Screen Recording 權限，因此無法可靠擷取/讀取 WebView 畫面。此 limitation 不取代上述 build/API/runtime evidence，也不算 release-native GUI UAT；若要做人工視覺驗收，需由使用者直接看目前已啟動的 AgentDock Next 或另行授權對應 macOS 權限。

Shared ACP UI 不再只是 Runtime Monitor；P4 closeout 後下一個 Wave 3 domain 是 **P5 MCP Management Shared UI**。

### P5 — MCP Management Shared UI

**狀態：UX / IA + authority contract frozen（2026-10-06）；implementation next。** 詳見 [P5 MCP Management roundtable](pre-m9-mcp-management-roundtable.md)。

P5 不直接把 Core `mcp_manage` 做成表單，而是先建立 Desktop-owned authority。Scope：

- standalone MCP configured / enabled / observed state；
- Plugin-owned MCP read-only inventory + owner attribution；mutation留給 P6；
- safe config summary；
- add / atomic edit / delete / enable / disable；
- protocol compatibility pin / timeout；
- write-only environment/credential management；
- Remote MCP OAuth flow；
- explicit **Reconnect and rediscover tools**；
- cached tool inventory，不自動連線 discovery。

Frozen security rules：

- secrets / raw OAuth token / protected args / secret-bearing URL/query不進 ordinary frontend state；
- Shared write的 non-loopback endpoint必須 HTTPS；
- Shared editor不接受 endpoint query；legacy query只提供 protected preserve semantics；
- registry / server / environment / authorization使用 revision/generation防 stale mutation；
- reconnect teardown失敗就 fail closed；
- HTTP credentials不可跨-origin redirect；
- plugin-owned mutation在 P5 backend一律拒絕；
- no offline writes、no automatic tool discovery、no tool-call console。

目前 Core已有 list/inspect/add/remove/enable/disable/env/refresh/OAuth，但仍需補 atomic update、revision/generation、authoritative safe reads、OAuth generation fence、redirect isolation與 truthful partial-outcome semantics，完成後才開 Shared UI。

### P6 — Plugin Management Shared UI

補 `DomainPlugin`，以管理已連線 / 已安裝的 plugins 為主，不發展 marketplace：

- status / health；
- enable / disable；
- permissions；
- configuration；
- disconnect / uninstall。

### P7 — Nexus / Startup / Platform Essentials

補齊日常 parity：

- Nexus endpoint / pairing / device status；
- startup / autostart；
- Open Logs / Open Config；
- tray/menu status；
- platform permission / elevation entry points。

真正 privileged/platform mutation 保持 native adapter，不把 privilege logic 搬進 Vue。

### P8 — Browser Broker Shared UI

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

## Current parity matrix

| Domain / 功能 | Stable native | Next backend | Shared UI | 決策 |
| --- | --- | --- | --- | --- |
| Runtime | 有 | 有 | 有 | Complete |
| Activity / Execution | 舊版較少 | 有 | 有 | Complete / Next-enhanced |
| Permission / Approval | OS/native 為主 | 有 | 有 | Complete / Next-enhanced |
| Connection status / port ownership | 有 | 有 | 有 | **P1 Complete**；reserved / available / owned / conflict / unknown fail-closed |
| Public MCP URL | 有 | 有 | **完整 `/mcp` URL** | **P1 Complete** |
| OAuth password | 有 | 有 | **explicit Show / Hide / Copy** | **P1 Complete**；ordinary snapshot 不含 secret |
| Tunnel configuration | 有 | 有 | Local / Quick / Named + write-only token + endpoint + advanced autostart | **P2 Complete**；Named remote route 保持 manual prerequisite |
| ACP lifecycle/health | 部分 | 有 | 有 | Complete / Next-enhanced |
| ACP profile/package CRUD | 有 | 有 | 有 | **P4 Complete**；revisioned CRUD / detect/version / fail-closed safe update / protected args |
| MCP management | Core/native | 有 | unavailable | P5 Port |
| Plugin management | Core/native | 有 | unavailable | P6 Port |
| Nexus | 有 | 有 | 未提供 | P7 Port |
| Browser config | 舊 CDP 模式 | Browser Broker | 未提供管理頁 | P8 Redesign |
| Startup / autostart | 有 | 有 | 部分 | P7 Native-backed Port |
| Update check | 有 | 有 | 有 | Complete |
| Update apply/recovery | 有 | 有 | native-only | M9 / native-backed |
| Logs / config shortcuts | 有 | 有 | 未提供 | P7 Port |
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
P3 USER: connector created as macbook-air-m3
  ↓
verify stable + Next side-by-side
  ↓
P4 ACP Manager full parity ✅
  ↓
P5 MCP Management ← **contract frozen / implementation next**
  ↓
P6 Plugin Management
  ↓
P7 Nexus / startup / platform essentials
  ↓
P8 Browser Broker UI
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

## Parallel execution plan

Pre-M9 可以大量並行，但不能跨過 dependency gate。原則是「先凍結 contract，再平行 implementation」，而不是一次啟動所有 domain。

### Wave 0 — Audit / Design（可平行）

第一波以 read-only / planning worker 為主：

- **A — Feature parity inventory**：盤點 stable native → Next backend → Shared UI；
- **B — UX / IA**：導航、分類、頁面層級、progressive disclosure、help / warning；
- **C — Security / operator UX**：Permission、credentials、Tunnel、destructive actions；
- **D — Technical architecture**：Desktop API / native adapter / Core contract / missing backend mapping。

Round 1 可完全平行；之後由主 orchestrator 整合共識、分歧與遺漏，再做 targeted Round 2 challenge。**未完成圓桌收斂前，不開始 Shared UI implementation。**

### Wave 1 — Connection foundation（部分平行）

P1/P2 是同一個 dependency cluster：

```text
Port ownership
  ↓
Local MCP URL
  ↓
Tunnel origin
  ↓
Public URL
  ↓
Public MCP URL
  ↓
ChatGPT connector
```

可拆成三個 backend worker：

- **Port / ownership**：reserved ports、occupancy、listener ownership、conflict、port-change atomicity；
- **Connection/Auth API**：Local/Public MCP URL、OAuth password safe reveal、health/test；
- **Tunnel backend**：Local/Quick/Named、token、autostart、Next-only Named Tunnel isolation、target synchronization。

三者可平行，但 schema / mutation semantics 必須先由主 orchestrator 做 integration review 並 **contract freeze**；Connection UI 在 contract freeze 後再實作，避免多個 worker 各自發明 API。

### Wave 2 — User connector gate（序列）

P1/P2 完成並驗證後停止：

1. AgentDock Next UI 顯示完整 Public MCP URL + OAuth password；
2. Next Named Tunnel / port readiness 完成；
3. **由使用者手動建立 Next ChatGPT connector**：已完成，ChatGPT 顯示名稱為 `macbook-air-m3`；
4. **stable `mac-dev` + `macbook-air-m3` side-by-side validation：已於 2026-10-06 完成。**

Connector 建立仍屬 user-owned gate；本次 side-by-side closeout 由主 orchestrator 驗證，沒有交給 worker 自動跨越。

### Wave 3 — Domain parity（高度可平行）

P3 side-by-side validation 已通過；P4 ACP Manager 已於 2026-10-06 完成並部署。後續仍可用獨立 worktree / branch 推進，現在的順序為：

- ~~ACP Manager~~ — **Complete**；
- **MCP Management — contract frozen / implementation next**；
- Plugin Management；
- Nexus / Platform Essentials；
- Browser Broker UI。

每個 domain 原則上自行完成：

```text
Desktop API
→ store / service
→ Vue components
→ i18n
→ tests
```

建議同時 active writing workers **3–4 個為上限**。再更多會因 `navigation`、Desktop API bindings、i18n、common components、styles/design tokens 等共享檔案產生 merge conflict，抵銷並行收益。

Browser Broker 的風險與架構複雜度高於一般 CRUD domain，應獨立用較強 worker / review；不可和 ACP/MCP/Plugin 視為同等簡單的管理頁。

### Wave 4 — Hardening side lane（可與 Wave 3 並行）

以下不必等所有 UI parity 完成：

- `app-identity.sh` governance debt；
- Codebase Memory lifecycle bounded triage。

兩者可在獨立 worktree 同步進行。CBM 只有被證明會阻塞 AgentDock install/update/release 時才進入 Pre-M9 fix scope。

### Wave 5 — M9（序列 integration gate）

必要 parity + P9 hardening 收斂並通過 integration review 後，才開始 M9 Release Migration。不要讓單一 domain worker自行提前修改 release pipeline。

### Wave 6 — Release-native UAT（按平台平行）

release candidate artifact 凍結後可平行：

- macOS signed/update/rollback/recovery；
- Windows native UAT；
- WSL native UAT；
- Linux helper native execution。

平台驗證可並行，但共同 release blocker / version contract / artifact identity 由主 orchestrator 統一裁決。

## Non-crossable gates

以下三個 gate 不得用「平行化」繞過：

1. **AI UX/IA roundtable → UI implementation**
   先收斂 page / interaction contract，才開始各 domain UI。
2. **P1/P2 → USER creates Next connector → side-by-side validation**
   connector 已由使用者建立為 `macbook-air-m3`，side-by-side validation 已於 2026-10-06 通過；此 gate 已完成，但不得因此跳過後續 parity / hardening gate。
3. **Required parity + hardening → M9 → release candidate → native UAT**
   cross-build / source-tree smoke 不能當作 release-native acceptance。

## Integration ownership

平行 worker 不是獨立產品 owner。主 session / orchestrator 必須負責：

- worktree / branch scope；
- contract freeze；
- shared-file serialization；
- cross-domain navigation / design consistency；
- merge / conflict resolution；
- full Shared Desktop regression；
- security / UX cross-review；
- final acceptance。

Worker 成功不等於 Pre-M9 step 完成。

## New-session entrypoint

新 session 接手時：

1. 先讀本文件、`roadmap.md`、`agentdock-next-isolation.md`；
2. 確認 branch / clean worktree / latest docs commit；
3. P1/P2、connector 建立與 P3 side-by-side validation 均已完成；不要重跑已 closeout 的 Wave 0–2。
4. Wave 3 已完成 ACP Manager；**從 P5 MCP Management 接續**，之後為 Plugin Management → Nexus / Platform Essentials → Browser Broker UI；
5. 每個主要 UI domain 實作前先跑 AI UX/IA roundtable，收斂 page goal、資訊層級、actions、help/warning、error/retry/accessibility；
6. 任何 Next mutation 仍由 stable `mac-dev` 執行；`macbook-air-m3` 只作 Next runtime/connector 驗證。M9 必須等必要 parity + hardening integration review。

除非 repo 文件已被後續 commit 明確 supersede，這個順序是 Pre-M9 的 source of truth。

