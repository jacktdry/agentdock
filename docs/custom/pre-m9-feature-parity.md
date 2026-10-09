# Pre-M9 Feature Parity and Connection Readiness

> **最新 checkpoint（2026-10-09 約 03:12）**：P8 B2c 原始碼 `a1d364e5` 已提交但尚未部署或放行 Edge；P6/P7 native GUI 待補；ACP 獨立候選版 `.14`（456 tests pass）等待 Next 整合，stable/Next 已安裝版仍為 `.8`（依 ACP 交接）。Next 8767 HTTP 200、完整 Browser/Policy Go 測試重跑 PASS；loopback 曾異常但未找出根因。見 [最新總覽](pre-m9-progress-2026-10-09.md)。

> 狀態：**P1/P2 deployed + live readiness validated；P3 stable/Next side-by-side validation 已完成（2026-10-06）；P4 ACP Manager full parity 已完成、review 並部署至 Next；P5 MCP Management 已於 2026-10-07 完成 final independent review、Next-only deployment / live UAT 並 closeout。P6 Plugin Management Phase E Desktop/Core live UAT 已通過、native GUI 驗收待補；M9 仍須等待其餘必要 parity + hardening integration review**
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

**狀態：✅ closeout（2026-10-07）**。final Gemini 3.1 Pro / High independent review：**PASS — 0 BLOCKER / 0 HIGH**；Next-only package/deploy 與 Desktop bridge live UAT 已完成。詳見 [P5 MCP Management roundtable](pre-m9-mcp-management-roundtable.md#21-p5-closeout--2026-10-07)。

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

Core/Desktop authority與 Shared UI 已完成，包含 atomic update、revision/generation、authoritative safe reads、OAuth incarnation fence、redirect isolation、truthful partial-outcome、private request binding與 operation reconciliation。第一輪 independent review 的 6 HIGH 已逐項修正，final independent PASS、Next-only package/deploy與 live UAT皆已完成。

### P6 — Plugin Management Shared UI

**狀態：Phase A/B/C + Phase D independent review（0 BLOCKER / 0 HIGH）已完成；Phase E Next-only Desktop/Core live UAT PASS，native GUI UAT 待補（2026-10-08）。**

Checkpoint：

- `f70a4715` — Desktop management authority；
- `55e8be0b` — opaque immutable Desktop candidates；
- `30fa143b` — native-control credential / generic-dispatch isolation / absolute-path redaction hardening。

Phase A/B 已通過 repo-wide Go tests、race、Windows/Linux backend compile、Shared macOS tests / Windows compile / Linux server-tag compile；native Linux Wails build需 Linux CGO/GTK/WebKit環境，留到 release-native UAT。

補 `DomainPlugin`，以管理已連線 / 已安裝的 plugins 為主，不發展 marketplace：

- status / health；
- enable / disable；
- permissions；
- configuration；
- disconnect / uninstall。

### P7 — Nexus / Startup / Platform Essentials

**Phase C（2026-10-08）：Next-only Diagnostics 開啟記錄／設定資料夾的 macOS backend、Shared UI 與三語系來源已建立；Wails beta.27 bindings 生成（74 Methods／114 Models、無 warnings）、Mac Go tests/vet、Windows amd64 cross-build、Vue 121 項測試/typecheck/Vite build 及獨立 source review（0 verified BLOCKER/HIGH）均已通過；native GUI UAT 仍待驗收。Windows/WSL/Linux 維持 requires_native。詳見 [Phase C §13](pre-m9-platform-essentials-roundtable.md#13-phase-c-offline-source-checkpoint-2026-10-08)。使用者明確 DEFERRED 所有 live Nexus pairing/re-pair/connection/network UAT，不阻擋 non-Nexus 工作；未部署、未重新啟動服務，未結案 P7/M9。**

2026-10-08 已完成 P7 能力盤點、UX / Security / Cross-platform 契約與 A1 配對傳輸安全；A2.1 Next-only 唯讀 Snapshot 已提交。A2.2 的 Core peer PID/UID、active identity generation、CLI/Desktop 共用交易鎖、generation fencing、原子寫入與 truthful restart outcome 已於 `209c776e` 修復，兩項獨立審查 HIGH 定點複審均已 CLOSED，後端離線驗證 PASS。**Phase B Shared UI 已進入原始碼離線整合**：Wails typed NexusService、Connection → ChatGPT access / NexusDock 子畫面、配對／重新配對確認、safe status、三語系與離線測試已建立；獨立 Phase B 唯讀安全複審 PASS（0 verified BLOCKER/HIGH），但 Next 原生 GUI 與實機 pairing UAT 尚未結案。詳見 [P7 Phase B source checkpoint](pre-m9-platform-essentials-roundtable.md#12-phase-b-offline-shared-ui-source-checkpoint-2026-10-08)。A1 企業 Proxy:nil、P6 native GUI、P7 Phase C 與 M9 仍受 gate 約束。

補齊日常 parity：

- Nexus endpoint / pairing / device status；
- startup / autostart；
- Open Logs / Open Config；
- tray/menu status；
- platform permission / elevation entry points。

真正 privileged/platform mutation 保持 native adapter，不把 privilege logic 搬進 Vue。

### P8 — Browser Broker Shared UI

> 2026-10-08 P8 Phase A source checkpoint: passive Broker diagnostics projection,
> Core local authenticated GET and **verified macOS Next Unix Desktop Control**
> `browser.snapshot` (PID/UID + signed process instance, no Desktop bearer over
> TCP), typed Wails `BrowserService.Snapshot` and tests are implemented. This
> is not the Shared UI or live browser acceptance. Browser UI, connector health,
> mutation authority, P6/P7 native GUI and M9 release gates remain pending.
> See [P8 Browser Broker Phase A contract](pre-m9-browser-broker-roundtable.md).


> P8 Phase B source checkpoint (2026-10-08): read-only Browser navigation,
> contract-gated manual snapshot UI, safe status/count labels and three locales
> are implemented. Focused tests 15/15 and typecheck pass; global i18n check
> remains blocked by existing unrelated hard-coded text. Native GUI / installed
> Next UAT, live browser health and P6/P7 native acceptance remain pending;
> live Nexus remains deferred. Overrides and cleanup are future inventory only.
> See [Phase B source checkpoint](pre-m9-browser-broker-roundtable.md#phase-b-offline-shared-ui-source-checkpoint-2026-10-08).


> P8 Phase C1 source checkpoint (2026-10-08, from `f45be4a8`): passive
> configured connector / authenticated external Edge profile / all-class required
> workspace policy aggregates and retained lease route counts are implemented in
> the existing Snapshot and typed Browser child. Health is fixed `not_observed`
> (**NOT CHECKED**); config intent remains visible with a verified Core snapshot
> and unavailable broker, while retained counts are hidden. Production planner
> status provider remains nil: company-required external Edge fails closed with
> no managed Chrome fallback. C2 secure live provider contract, Native GUI /
> installed Next and live browser UAT remain open; Nexus remains deferred.
> Offline focused Go tests/vet, JSON privacy validation, Vitest **22/22**, final
> typecheck and production build passed; Wails bindings/i18n were regenerated.
> Global i18n check retains pre-existing unrelated hardcoded-text failures.
> See [Phase C1 source checkpoint](pre-m9-browser-broker-roundtable.md#phase-c1-offline-connectorworkspace-route-source-checkpoint-2026-10-08).

> P8 Phase C2a source checkpoint (2026-10-08, from `3d741d82`): runtime
> status now requires provider-observed connector/profile/exact canonical endpoint
> identity and UTC timestamps with a five-second freshness/TTL limit. Cancellation
> and a two-second derived deadline bound caller response with sanitized errors;
> one provider slot per runtime planner remains held until actual provider return.
> A stuck provider retains at most one provider goroutine per planner and blocks
> later external verification without spawning another call. Underlying network
> cancellation is not guaranteed; C2b requires a cancellation-safe authentic source.
> Provider panics fail closed without exposing values/stacks and release the slot.
> Capability snapshots own their copied slice; providers must avoid concurrent
> mutation during return/copy, which this boundary cannot verify or prevent.
> company-required Edge has no managed Chrome fallback. Offline matching Edge and
> explicit Chrome mocks are source evidence only. Production planner provider
> remains **nil**; BrowserDesktop DTO/UI and direct ResolveRoute fixtures are unchanged.
> Consistency/freshness, websocket reachability and configuration are not genuine
> authentication/profile/no-focus proof. Edge remains **UNQUALIFIED**; no Edge CDP
> probe or installed Next GUI UAT occurred. C2b authentic observation source,
> runtime identity/login/profile/capability proof and any required external lifecycle
> remain pending, as do native acceptance/P8/M9; Nexus remains deferred.
> Focused planner/route/catalog/contract/compatibility Go tests and browser/
> browserpolicy vet and focused planner race tests passed; `git diff --check`
> passed. Review/integration pending.
> See [Phase C2a source checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2a-offline-runtime-evidence-envelope-source-checkpoint-2026-10-08).

> P8 Phase C2b-A source checkpoint (2026-10-08, base `00114b93`):
> planner now defaults to rejecting raw provider booleans even with matching fresh
> IDs/endpoints. A private qualification binds an immutable observation, distinct
> source/runtime incarnation and single-use nonce; mismatches, stale evidence,
> unsupported proof and replay fail closed without fallback. Only TEST ONLY offline
> fixtures mint synthetic authority; no production minting path exists and runtime
> provider remains **nil**. Desktop/Snapshot/UI and pure ResolveRoute stay unchanged.
> Edge is **UNQUALIFIED**. C2b-B real process/profile/auth/capability attestor,
> revocation/cancellation and attach-time revalidation plus consent-based native UAT
> remain open; C2b/P8/M9 are not closed and live Nexus remains deferred. AGY-ACP and
> handoff remain separate until the ACP contract is stable.
> See [Phase C2b-A checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-a-trust-source-qualification-gate-source-checkpoint-2026-10-08).

> P8 C2b-B1 offline attach/admission checkpoint (2026-10-08, base `7b90f16a`):
> qualified planner success now hands off a private, non-serializable, short-lived,
> one-shot grant binding the exact canonical scope, route, full resolved start and
> source/incarnation. Pure resolver/forged/changed/expired/replayed decisions cannot
> start a connector via ExternalLeaseManager. An independent source verifier must
> match the immutable started worker's live peer before baseline and again before
> new_page; missing/error/timeout/revocation fail closed, with connector cleanup.
> Only TEST ONLY synthetic fixtures exercise success. Production provider and
> manager verifier remain **nil**, Edge remains **UNQUALIFIED**. Real attestation,
> atomic peer fencing and consent-based native UAT remain pending; C2b/P8/M9 stay
> open and Nexus remains deferred. No runtime, adapter or AGY handoff change.
> See [C2b-B1 checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b1-offline-attachadmission-checkpoint-2026-10-08).

> P8 C2b-B2a offline lease lifecycle checkpoint (2026-10-09, base `a1f4e1fc`):
> external leases privately retain expected peer identity and revalidate it under
> the lease mutex before each active Call and Release/SweepExpired page operation.
> Admission grant expiry is separate from active lease validity. Peer loss latches
> permanently; missing/error/revoked/mismatched/canceled verification cannot resume
> after matching observations return. Suspect cleanup stops only the owned
> connector and records page close unconfirmed; failed Stop recovery retries only
> Stop. Managed behavior is unchanged. Offline fake fixtures only; production
> provider/verifier remain **nil**, Edge remains **UNQUALIFIED**. Real attestor,
> atomic transport fencing and consent-based Next native UAT remain pending;
> C2b/P8/M9 remain open. No live Edge/CDP, GUI, policy, runtime or AGY handoff change.
> See [C2b-B2a checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2a-offline-lease-peer-lifecycle-checkpoint-2026-10-09).

> P8 C2b-B2b offline post-operation peer checkpoint (2026-10-09, base
> `943e546c`): `ExternalLeaseManager.Call` now verifies peer identity after
> the actual browser action before returning any result or refreshing lease TTL.
> Identity loss returns nil result with sanitized denial and permanently latches
> the lease. After external `new_page`, a third identity check before publishing
> the lease prevents cleanup from closing a page whose browser incarnation
> could have changed; only AgentDock's connector is stopped and retried.
> Offline synthetic fixtures cover turnover, revocation, cancellation and
> action-error combinations. **Not atomic transport fencing or real Edge auth**:
> provider and verifier remain nil, Edge UNQUALIFIED, consent-based native UAT
> remains open. Stable AgentDock, runtime, and AGY-ACP are untouched.
> See [C2b-B2b checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2b-post-operation-peer-consistency-checkpoint-2026-10-09).

> **2026-10-09 Next-only installed source smoke:** arm64 ad-hoc Shared Desktop,
> Core, ZIP/DMG and package validation passed; deployed with isolated Next
> Contents swap and bundled Next `AgentDockServiceRegistrar` re-registration.
> New Next Core 8767 HTTP 200, Tunnel launchd job running, GUI process
> restarted in background; previous Next Contents backup retained. An
> initial bare launchctl kickstart failed due stale macOS Launch Constraint,
> recovered by Next-only Registrar (see
> [isolation UAT](agentdock-next-isolation.md#2026-10-09-next-only-shared-desktop-installed-build--rollback-uat)).
> Browser/Policy/Desktop API/Desktop Runtime Go tests and vet passed. **No
> native visual GUI or authenticated Edge UAT**: P6/P7/P8 and M9 gates stay
> open. Live Nexus remains deferred.

> 2026-10-09 Next-only deployment wrapper native acceptance:
> `packaging/macos/deploy-next-shared.py` successfully swapped Next
> Contents and refreshed only Next Core/Tunnel registrations. New
> Next Core PID `75320` is independently bound to its bundled helper
> executable and port 8767, HTTP 200; Tunnel PID `75385`, GUI PID
> `75792`, rollback Contents retained. Offline wrapper tests 12/12,
> bundle validation 8/8. Real failure-injection rollback, native GUI
> accessibility/visual tests and real Edge Connector qualification remain
> open. See [Next isolation deployment checkpoint](agentdock-next-isolation.md).

> **P8 C2b-B2c macOS preflight (2026-10-09):** Next-only package-private OS evidence checks a canonical loopback Edge endpoint against listener PID, same-user start fingerprint, Edge executable text mapping, remote-debugging port and explicit user-data-dir; repeated to reject turnover. This is NOT auth/login/no-focus proof, no production provider is enabled and Edge stays UNQUALIFIED. Focused/race/vet/cross-build passed; full Browser suite blocked by five reproducible existing local httptest timeouts. See [B2c checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2c-macos-read-only-edge-process-evidence-2026-10-09).

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

上游 v1.0.1 新版發行驗證與 source/identity 的非阻塞選擇性採用方案見 [`upstream-v101-adoption-plan.md`](upstream-v101-adoption-plan.md)；此文件為 proposed，不能替代本節正式 release-native gates。

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

### P13 — MCP Task Progress 單一卡片（上游 PR #229，獨立非阻塞 UX）

**規劃採用，尚未實作或部署**。此編號只作 Next 非阻塞採用項目索引，不新增 M9 hard gate，也不改動現有 P12 native UI deprecation 定義。詳細技術規格及 T01–T08 驗收見 [PR #229 Task live card](upstream-ports/task-progress-live-card-pr229.md)。

- `task_create` 負責建立一個持久 Task，ChatGPT 聊天模式僅掛載**一張** Task Progress MCP App；使用者後續要求「繼續」且確實屬於同一工作時，沿用 `task_id`，不得為了顯示進度重複建立新 Task。
- `task_manage checkpoint/block/resume/final_review/complete` 更新權威 Task State，不掛載第二張 Task 卡；既有 `task_manage action=create` 應保留向後相容且也只掛一張卡。不同 Task ID 各保留自己的卡片，**不得**合併無關任務。
- 已存在的卡片以 app-only `task_snapshot(task_id)` 取得最小化唯讀摘要；active/blocked 分別約 2/10 秒更新，hidden/teardown/completed 停止輪詢，失聯時 truthful stale/retry，具多卡 bounded QPS、並發限制及 Core scope/authorization。
- 需驗證 ChatGPT MCP App bridge、full/compact/off metadata、連線及舊 plugin registration cache 的更新路徑；只跑 Go tests 不代表聊天畫面的原卡片確實原地更新。
- 隔離於 Browser Broker B2i/B2j/B2k、AGY .14、自動更新／M9 release 工作；不修改 stable connector 或已部署的 Next，於獨立 worktree 實作並由 `$mac-dev` 控制驗證。

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
| Logs / config shortcuts | 有 | 有 | Phase C macOS source；生成／離線驗證與 native UAT 待補 | Next-only；其他平台 requires_native |
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
P5 MCP Management ✅
  ↓
P6 Plugin Management ← **Phase A/B/C/D ✅；Phase E Desktop live UAT ✅、GUI UAT 待補**
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

P3 side-by-side validation 已通過；P4 ACP Manager 已於 2026-10-06 完成並部署；P5 MCP Management 已於 2026-10-07 完成 final independent review、Next-only live deployment / UAT 並 closeout。後續仍可用獨立 worktree / branch 推進，現在的順序為：

- ~~ACP Manager~~ — **Complete**；
- ~~MCP Management~~ — **Complete**；
- **Plugin Management — contract frozen / implementation in progress**；
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
4. Wave 3 已完成 ACP Manager 與 P5 MCP Management；P6 Plugin Management UX / IA + authority contract 已於 2026-10-08 凍結，**Phase A/B/C 與 Phase D independent review（0 BLOCKER / 0 HIGH）已完成，Phase E Desktop/Core live UAT 已通過、native GUI UAT 待補**；[P7 Nexus / Platform Essentials UX/security contract](pre-m9-platform-essentials-roundtable.md) 已收斂，下一步 Phase A 安全 backend → Shared UI；再處理 Browser Broker UI；
5. 每個主要 UI domain 實作前先跑 AI UX/IA roundtable，收斂 page goal、資訊層級、actions、help/warning、error/retry/accessibility；
6. 任何 Next mutation 仍由 stable `mac-dev` 執行；`macbook-air-m3` 只作 Next runtime/connector 驗證。M9 必須等必要 parity + hardening integration review。

除非 repo 文件已被後續 commit 明確 supersede，這個順序是 Pre-M9 的 source of truth。
