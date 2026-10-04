# AgentDock Custom Roadmap

> 狀態：Accepted initial roadmap

## M0 — Repository Governance

### 目標

建立不污染 upstream 的長期開發基線。

### 工作

- `main` 對齊 `uvwt/agentdock:main`
- 建立 `custom/main`
- 保留 `upstream` / `workbench` remotes
- 建立 `docs/custom/`
- 定義 branch / release / upstream tracking 規則
- 記錄 Workbench selective-adoption 原則

### Exit criteria

- `main == upstream/main`
- `custom/main` 存在，並被指定為未來 custom Release 的 source branch
- custom 開發不需要修改 `main`
- 文件可讓新 session 重建開發策略

注意：目前官方 `.github/workflows/release.yml` 仍要求 release source commit 屬於 `origin/main`，且 `verify-version` 維持官方版本契約。M0 不宣稱 custom-only Release 已可發布；真正的 custom release enablement 在 M9 完成。

## M1 — Cross-platform Desktop Architecture Spike

### 目標

驗證 shared Desktop UI 可否取代持續擴張的 AppKit / WPF 雙寫模式。

### 工作

- Wails POC
- Tauri 對照 spike（只做到足以比較 bridge / packaging / platform integration）
- Vue 3 + TypeScript shell
- Go binding / Desktop API prototype
- event streaming
- high-frequency event batching / backpressure
- tray / menu / window persistence
- Core lifecycle integration
- macOS / Windows packaging 與 security gate
- VoiceOver / Narrator / keyboard accessibility spike
- native baseline 對照的 idle / active RAM 與 CPU profiling

### Exit criteria

同一 UI codebase 在 macOS / Windows 能：

- 讀取 Core state
- Start / Stop / Restart
- 接收 event
- 修改並保存一項設定
- 使用 tray / menu
- 建出可安裝 artifact
- 高頻 event 下 UI 仍可互動且不出現無界 queue
- accessibility 與資源使用有可重複量測 baseline

之後才正式決定 Wails / fallback。

### M1 實測結論（2026-10-02）

- Wails v3.0.0-beta.27 POC 已完成，列為 provisional shared Desktop shell。
- Vue/TS、Go tests、typed control binding、bounded Stream/TrySend backpressure、Runtime read-only integration 已驗證。
- macOS arm64 `.app` / DMG 已建置驗證。
- Windows ARM64 / x64 EXE 已 cross-build 驗證。
- Windows installer、Authenticode、Narrator 必須在 Windows runner/device 補驗。
- VoiceOver、tray/window interactive automation、long-running sleep/wake soak 仍是 production gate。
- Wails macOS production binding generation 的 CGO 成本偏高，需持續追蹤。
- M2 可以開始，但 native AppKit/WPF 不移除。

## M2 — Shared Desktop API

### 目標

建立 Shared UI 與 Core 的穩定邊界。

### Domain

- runtime
- connection
- acp
- browser
- activity
- permission
- mcp
- plugin
- update
- diagnostics

### Exit criteria

Shared UI 不需要直接引用散落的 `internal/*` 實作。

### M2 實測結論（2026-10-03）

- 建立 framework-neutral `internal/desktopapi`，Wails 僅作 binding / Stream transport adapter。
- Desktop API protocol v1、capability negotiation、structured error、access level / confirmation metadata 已建立。
- Runtime domain 已正式化，Shared UI 不再暴露 `desktopruntime.ServiceStatus`。
- Activity v1 contract 已建立：epoch + decimal-string sequence、bounded source queue、256 KiB payload ceiling、source / transport drop accounting。
- Activity 高頻 data plane 僅使用 bounded Stream / `TrySend`，禁止使用 Wails unbounded Event mailbox。
- connection / acp / browser / permission / mcp / plugin / update / diagnostics 目前明確標示 unavailable，不建立假實作。
- Go / race / vet、Vue typecheck、Vitest contract fixture、macOS DMG、Windows ARM64/x64 cross-build 均通過。
- M3 可開始；real Activity / execution source 仍留在 M5。

## M3 — i18n Foundation

### 工作

- locale manifest
- strict YAML authoring catalogs
- ICU MessageFormat contract
- schema / validator
- deterministic generator
- Shared UI catalog
- native resource generation
- coverage report
- hard-coded user-facing string check
- glossary
- contributor guide
- existing `.strings` / `.resx` migration inventory

### 初期 locale

- en
- zh-Hant
- zh-Hans

### M3 實測結論（2026-10-03）

- 建立 repo-level `i18n/manifest.yaml`、strict YAML catalogs、glossary 與 schema。
- `tools/i18n` 提供 deterministic generate/check/coverage/inventory，並以結構化 parser 驗證 ICU arguments。
- Shared Vue UI 已切換為 semantic key + `intl-messageformat`，語言選單直接由 manifest 生成。
- macOS `UILanguagePreference` 與 Windows `UiText` 已改由 generated locale descriptors 驅動，不再各自維護固定語系清單。
- 既有繁中資源沿用先前人工翻譯並補齊最新 analytics keys；macOS 三語各 363 keys，Windows Control Panel 三語各 241 keys。
- generated freshness、coverage 與 Shared UI hard-coded user-facing string scan 已納入 `make i18n-check` 與 CI。
- legacy AppKit/WPF message key 尚未整批轉為 semantic key；migration inventory 明確保留這個邊界，不做猜測式自動合併。
- M4 可從此 foundation 開始搬 Overview / Runtime / Connection / Basic Settings / Update-Diagnostics。

## M4 — Shared Desktop Baseline

狀態：**完成（2026-10-03）**

第一批 production-facing shared shell 已完成：

1. Overview
2. Runtime
3. Connection
4. Basic Settings
5. System：Update Check + Diagnostics

完成邊界：

- `internal/desktopapi` / `internal/desktopruntime` 持有 framework-neutral service semantics，Vue/Wails 不重做 platform state machine。
- Runtime / Connection / Settings mutations 依 runtime root 串行化，並保留 rollback 與原本 running state。
- Quick Tunnel port target 會隨 Basic Settings port 更新；`regenerate` 僅 quick mode 可用，前後端都 fail-closed。
- Basic Settings 僅暴露 port / log level / core autostart；macOS SMAppService autostart 維持 native-only。
- Update shared path 僅提供 check，且版本來源是被控制的 runtime core，不是 shared shell executable。
- Diagnostics 僅回傳 secret-safe metadata。
- Browser、ACP、MCP、Plugin、Permission 尚未搬入 shared shell；Activity 仍是 Developer / Architecture 下的 experimental surface。
- AppKit / WPF native UI 持續保留作 fallback。
- macOS、Windows x64、Windows ARM64 shared builds 與三語系 i18n gate 已驗證。

## M5 — Activity / Execution Vertical Slice

這是 shared UI foundation 後第一個新能力，也是架構壓力測試。

- truthful execution state
- root / child call
- high-frequency activity stream
- bounded output / continuation
- file changes
- user insertion
- ACK
- attention semantics

Exit criteria：

- stream 中斷 / reconnect 後 state 不失真
- UI 不因大量 log/event 形成無界 queue
- insertion 有 stable ID 與可觀察 delivery state
- recent interaction 不會被誤判為 active execution
- keyboard / VoiceOver / Narrator 可以操作核心 execution controls

## M6 — Browser Broker

實作 workspace-aware Browser Control Broker，而不是讓每個 ACP / session 自己選擇或啟動瀏覽器 backend。

### Completion checkpoint — 2026-10-04

M6 runtime、lifecycle、diagnostics 與 stress validation 先在 `feature/browser-broker` 完成，後續已整合回 `custom/main`。完成狀態：

- ✅ `contract` — `c14b21ca feat(browser): define broker routing contract`
  - canonical workspace root policy；
  - company route 必須使用 authenticated external Edge，無法使用時 fail closed；
  - Browser Broker / Engine / lease ownership contract；
  - typed routing errors 與 compatibility matrix baseline。
- ✅ `engine` — `8edcecef feat(browser): add managed MCP worker engine`
  - 重用既有 Go MCP SDK / process controller 建立 ephemeral stdio MCP session；
  - managed worker 固定使用 `chrome-devtools-mcp@1.7.0`；
  - 啟用 `--isolated --headless --experimentalPageIdRouting`；
  - 已驗證 handshake、background isolated page、owned Chrome descendants cleanup 與 temporary profile cleanup。
- ✅ `profiles` — `c08dc81b feat(browser): plan workspace profile routes`
  - browser/profile/connector catalog；
  - loopback WebSocket endpoint canonicalization；
  - company authenticated Edge route；
  - non-company managed isolated Chrome route；
  - explicit user-requested external route；
  - runtime connector status 與 strict config validation。
- ✅ `leases` — `449169aa feat(browser): add managed browser leases`
- ✅ `concurrency` — `bd010aa2 feat(browser): bound managed browser concurrency`
- ✅ `external` — `8c8db73b feat(browser): add external lease isolation`
- ✅ `acp` — `2b0f58bd feat(browser): route ACP through browser broker`
- ✅ `computer` — `13c9cd9a feat(computer): add Orca computer control broker`
  - 上游 direct 與 ACP Computer Use 統一由 AgentDock Computer Control Broker routing；
  - Orca 為 default provider，不 silent fallback 到 ChatGPT / Sky；
  - foreground control 預設 forbidden，background observation 有 focus-change violation guard；
  - `antigravity-acp 1.2.0-agentdock.5` 已支援 host-owned Browser + Computer MCP。
- ✅ `lifecycle` — `9b6ece24 feat(browser): harden broker lifecycle recovery`
  - managed / external 5 分鐘 idle TTL 與 30 秒 lifecycle sweep；
  - bounded cleanup recovery、acquire orphan recovery；
  - canonical-equivalent connector endpoint 在 catalog 層拒絕重複；
  - external recovery 只處理 AgentDock-owned connector，不取得 user browser/profile/process kill authority。
- ✅ `diagnostics` — `af49037b feat(browser): expose broker ownership diagnostics`
  - `browser_broker` / `computer_broker` control-plane status 與 bounded cleanup；
  - owner → lease → worker → page/context → connector/process ownership 可追蹤；
  - capability token 不進 diagnostics；Computer focus/provider failure 使用 bounded event history。
- ✅ `stress` — `dc46459e test(browser): stress ACP broker lifecycle`
  - 8 個不同 ACP owner 實測 4 active + 4 queued，無 cross-token / target contamination；另以 4 個真 `chrome-devtools-mcp` / Chrome worker 同時 lease 驗證真 engine isolation；
  - 20 次真實 `antigravity-acp 1.2.0-agentdock.5` session new/close/runtime close 回到 baseline；
  - 真實 Orca no-focus observation、Microsoft Edge external user-page preservation、managed host hard-crash process-group cleanup、pin/schema regression 全部通過。
- ✅ `handoff` — 本文件、[browser-cdp-lifecycle.md](browser-cdp-lifecycle.md)、[computer-use-backends.md](computer-use-backends.md) 與 [acp-lifecycle-memory.md](acp-lifecycle-memory.md) 已同步 M6 完成基線。

M6 已在獨立 merge-result 上重新通過 repo-wide tests、vet、focused race 與 Windows cross-build，並以 `e8044084 merge: integrate M6 browser broker` 整合回 `custom/main`。`feature/browser-broker` 保留的意義僅為 milestone 歷史，不再是主線執行來源。

版本策略暫時維持 `chrome-devtools-mcp@1.7.0`。即使 upstream 已有較新版本，也必須先通過 managed-headless、persistent-profile、Chrome-live、Edge-live compatibility matrix 才能升級，避免破壞 company Edge / default-profile attach。

M6 實作期間持續遵守以下硬限制：

- 不以 global selected page 作為 ACP/browser 操作狀態；
- 不以 global `pkill` 清理 browser/MCP；
- external Edge/Chrome 的 PID 只可作 observation，不能被視為 kill authority；
- company Edge 只操作 AgentDock-owned background leased target，使用者既有 tabs/profile/browser process 必須保留；
- Browser Broker 不得 silent fallback 到 foreground Computer Use；
- Computer Use 與 Browser automation 分開，Computer Use 統一走 AgentDock Computer Control Broker → Orca default。


預設 policy：

```text
company project
→ user's existing Microsoft Edge
→ user's authenticated Edge profile
→ AgentDock-owned background tab / leased target only

non-company browser task
→ chrome-devtools-mcp
→ AgentDock-managed isolated/headless Chrome

explicit user request
→ chosen user profile/browser
```

Browser engine 優先採 `chrome-devtools-mcp`，AgentDock Core 負責 routing / ownership / lease / lifecycle，而不是重寫完整 CDP automation engine。

公司 workspace 是硬路由例外：無論由上游 ChatGPT 自己做 UAT，或分派 Codex / Antigravity ACP 做 UAT，只要需要公司的已登入 Web 狀態，就必須 attach 使用者現有的 Microsoft Edge 與該使用者 profile。原因是公司 GitLab、Cloudflare 與相關後台登入只存在於這個 Edge profile；不得靜默改用 isolated Chrome 或其他未登入 profile。若 company Edge attach 不可用，應 fail closed / 回報需要恢復 Edge connector，而不是改走未登入 browser。

非公司 workspace 或其他一般 browser 需求，除非使用者明確指定自己的 profile/browser，否則一律由 `chrome-devtools-mcp` 使用 AgentDock-managed isolated/headless Chrome。

並處理：

- browser registry / explicit routing
- managed headless + persistent profile
- existing Chrome / Edge explicit attach
- `chrome-devtools-mcp` worker pool / version policy
- multi-ACP browser lease / BrowserContext / Page isolation
- per-lease page ownership，禁止依賴 global selected page
- bounded concurrency / queue / lease TTL
- site / resource write lock
- no-focus / background page policy
- external browser user-tab protection
- profile/process ownership
- stale cleanup
- tmp lifecycle
- health/diagnostics

2026-10-03 bake-off 補充：

- `chrome-devtools-mcp@1.7.0` 已實測可操作 AgentDock-owned isolated headless Chrome、Chrome Remote Debugging 與 Edge Remote Debugging；
- AgentDock 管理的 isolated MCP 移除後，可連同 owned headless Chrome 一起回收；
- background tab 建立可避免把測試 tab 帶到前景；
- 但目前 Chrome / Edge default-profile 新版 Remote Debugging 與較新 `chrome-devtools-mcp` 存在相容性風險，因此 existing browser attach 只能當 explicit compatibility backend，不可作唯一主路徑；
- 不再採 `AGENTDOCK_BROWSER_REUSE_EXISTING_CDP=true` 這種「找到任意 CDP 就重用」的長期策略，因為目前同時可能存在 Chrome / Edge 多個 endpoint。

M6 已建立 browser process / connector / profile / lease ownership contract，並以 bounded lifecycle recovery 取代 process age / 全域 `pkill` 推測式清理。完整現場證據、並行模型、lifecycle 與驗收條件見 [browser-cdp-lifecycle.md](browser-cdp-lifecycle.md)。

Computer Use 不屬於一般 browser routing；只有程式化工具與 Browser Broker 都做不到時才進入 Computer Control provider。無論是 ACP 請求 Computer Use，或上游 ChatGPT 自己需要直接操作本機 GUI，預設都必須經 AgentDock Computer Control Broker 使用 Orca；不得繞過 AgentDock 直接把 ChatGPT/OpenAI Computer Use 當一般執行路徑。只有使用者明確指定或未來正式 fallback policy 允許時，才考慮其他 provider。Provider / no-focus contract 見 [computer-use-backends.md](computer-use-backends.md)。

## M7 — ACP Manager

整合現有 ACP management branch 中可重用的 backend 經驗，但 UI 必須走 Shared UI。

能力：

- detect
- installed/latest version
- update available
- update
- edit/delete
- enable/disable
- health
- Codex / Claude / Antigravity presets
- custom adapter

UI 以管理目前實際使用的 adapters 為主，不發展成通用 package marketplace。

### M7 lifecycle / shared-memory 補充（2026-10-03）

ACP Manager 同時承接 session resource lifecycle，而不只 package CRUD：

- per-session persistent / ephemeral / idle-managed policy；
- delegated ephemeral worker 完成後標準 session/close；
- idle-managed session 的 bounded TTL sweeper；
- managed / loaded / running / idle session 分流與 adapter process diagnostics；
- AgentDock Memory dynamic MCP 從 per-process stdio 遷移至本機 shared Streamable HTTP daemon；
- Adapter-specific runtime recycle 留在自維護 codex-acp / antigravity-acp，不在 AgentDock Core 寫 agent-specific kill hack。

完整設計與接手條件見 [acp-lifecycle-memory.md](acp-lifecycle-memory.md)。

### M7 Antigravity provider baseline（2026-10-04）

- AgentDock 的 AGY 路徑已收斂為單一 hardened `antigravity-acp` provider；不再經過 `refined-antigravity-acp`。
- Fork：`jacktdry/antigravity-acp`；`main` 跟 upstream 對齊，AgentDock hardening 位於 `fix/agentdock-hardening`。
- 目前部署版本：`1.2.0-agentdock.5`，tip `2bd8426`；system AGY `1.2.16`。
- Adapter 已補強 cancel/close/delete、active child cleanup、同 session concurrent-turn guard，以及 Model / Reasoning effort 分離。
- AgentDock profile 以 isolated HOME + `AGY_BIN` 啟動；不繼承互動 AGY 的 MCP/plugin 設定。
- M6 再把 Browser / Computer 收斂成 host-owned loopback MCP capability；ACP child 不直接持有 browser/Computer Use backend。
- 實測 AgentDock E2E：Gemini prompt、close、resume、delete、parallel sessions、cancel 均正常；M6 另完成 20-cycle 真實 Antigravity lifecycle stress。
- Claude Sonnet 5.5 / Opus 5.5 視為獨立稀缺額度 specialist，只用於 context 已壓縮的窄範圍 review / architecture，預設 concurrency = 1。

Browser / CDP child ownership、connector cleanup 與舊 Antigravity `localharness` / `chrome-devtools-mcp` 現場盤點另見 [browser-cdp-lifecycle.md](browser-cdp-lifecycle.md)；這些舊 process tree 是歷史證據，不再是現行 Browser ownership 架構。

M7 不重新實作 Browser Broker。M6 已完成 ACP browser request → AgentDock lease、bounded concurrency、TTL / cleanup recovery 與 ownership diagnostics；M7 只需讓 persistent / ephemeral / idle-managed session policy 在正確時機觸發既有 host capability release，並把 Broker diagnostics 納入 ACP session view。

Computer Use 亦採 AgentDock provider abstraction；ACP 不自行選 Orca / OpenAI Computer Use。現階段 AgentDock primary provider 為 Orca，OpenAI Sky / ChatGPT Computer Use 僅保留 product fallback，完整決策見 [computer-use-backends.md](computer-use-backends.md)。

### M7 Completion checkpoint — 2026-10-04

M7 feature branch 已完成 lifecycle、Memory client、diagnostics、Shared Desktop 與 real-adapter stress：

- `persistent / ephemeral / idle-managed` per-session policy 與 restart-persistent metadata；
- ephemeral terminal auto-close、bounded idle-managed sweeper、close/retry/race hardening；
- ACP diagnostics + adapter process observation + M6 Browser/Computer correlation；
- Shared Desktop ACP lifecycle controls / adapter health / shared Memory health；
- MCP go-sdk `v1.8.0` + per-server `protocol_version` compatibility pin，Memory HTTP 使用 `2025-11-25`；
- `codex-acp 2.1.1` 與 `antigravity-acp 1.2.0-agentdock.5` 均通過 persistent / ephemeral / idle-managed 真實 lifecycle；
- 兩個 adapter 各完成 20 prompt-driven ephemeral stress，同一 Runtime / adapter PID 下 Browser / Computer capability 與 child resources 回 baseline；
- M6 4-active bounded Browser queue、live managed Chrome isolation、resource lock、foreground Computer fail-closed 等 regression 全綠；
- root tests / vet、ACP / Desktop / MCP race、Shared Desktop 30 frontend tests + build、macOS nested module test、Windows amd64 cross-build均通過。

M7 feature code / stress 已於 `d8acb9be` 整合完成。舊 direct stable rollout gate 刻意撤回並延後：live activation 嘗試曾造成 ChatGPT Mac-Dev control channel 斷線。不得為完成 M7 重建 / 重啟 / 替換 stable App/Core 或切換 stable stdio Memory registry；下一步改為 M7.5 AgentDock Next isolation。

### M7 CBM lifecycle 技術債（2026-10-03）

實測發現 Codebase Memory executable 更新後，舊 daemon / supervisor 仍存活時，新 index worker 可能因 build mismatch 被拒絕並形成連續 worker failure。這屬於 dynamic MCP service lifecycle / process ownership 問題，不應以全域 `pkill` 解決。

M7 / diagnostics 規劃時需一併評估 installed-vs-running build 偵測、restart-pending、active work drain、pending re-index queue、bounded retry/backoff，以及 versioned executable / graceful restart 的責任邊界。完整現場證據與接手確認清單見 [cbm-lifecycle-conflict.md](cbm-lifecycle-conflict.md)。

## M7.5 — AgentDock Next Isolation（pre-M8 gate）

狀態：**Phase 1 identity/runtime 與 Phase 2 updater/arbiter repository isolation 已完成；GUI updater gate 保留，Next-only package/live 驗證待完成；stable live cutover 刻意延後**。

2026-10-05 checkpoint：文件 gate 已於 `9962c61` 完成；Phase 1 於 `architecture/agentdock-next-isolation` 的 `b189ee5` 完成。Shell syntax、packaging metadata fixture、Swift fixture/typecheck、Go `internal/desktopruntime` / `scripts/test` 均通過；Gemini 3.1 Pro cross-review 無 blocker。整個 Phase 1 未操作 stable App/Core/state/registry/live launchd。

目前連線的 AgentDock 是 production control plane，Next 開發不得檢查、修改、重啟、停止或替換 `/Applications/AgentDock.app`、stable Core、`~/.agentdock`、stable Memory registry 或 live launchd services，也不得以 stable 作 development target。

工作與 Exit criteria：

- ✅ Phase 1：以 **AgentDock Next.app** side-by-side 實作獨立 bundle / LaunchAgent / runtime / log / state / work directory / port namespace；精確 defaults 見 [agentdock-next-isolation.md](agentdock-next-isolation.md)。
- ✅ Phase 2 repository isolation：`selfupdate` / `updateplatform` / arbiter 已綁定 Next artifact、bundle/helper signer、destination、service labels、journal/trial/rollback/recovery；unit/race、vet、Swift/packaging fixtures 與 cross-build 證據見 [Phase 2 checkpoint](agentdock-next-isolation.md#m75-phase-2-checkpoint--2026-10-05)。Next GUI check/apply/recovery gate 保留；signed package 與 live 驗證仍待完成。
- Memory HTTP cutover 先在 Next registry / ACP sessions 驗證，固定 `http://127.0.0.1:8766/mcp` 與 `protocol_version=2025-11-25`；stable stdio registry 與其 children 不列入 Next cleanup。
- 通過隔離、Next lifecycle / update / rollback 與 M6/M7 regression；證據不得來自開發中操作 stable runtime。
- Next 必須能以未來 `mac-dev-next` connector 獨立連上 ChatGPT，Next lifecycle 操作不能影響原 Mac-Dev control channel。

此 gate 是 M8/M9 live migration 的必要前置；後續 feature 開發仍限 Next。只有 Next 完整開發、測試及獨立連線後，才另行提出 migration / retirement 計畫，不把舊 AgentDock cutover 當目前下一步。

## M8 — Permission / Approval

評估並最小化引入：

- Permission Profile
- Approval Policy
- effective permission source
- approval history

Approval Reviewer 預設 defer。

本 Milestone 的 runtime 驗證限 Next；live migration 必須先通過 M7.5。此 Milestone 同時完成剩餘 Browser / ACP / MCP / Plugin / Settings 的 shared UI parity 評估，決定哪些舊 native business views 可以 deprecated。

## M9 — Release Migration

建立從 `custom/main` 的正式 cross-platform release pipeline。

前置工作：

- 通過 M7.5，Next 完整開發 / 測試並能獨立連上 ChatGPT；stable retirement 必須另行規劃與授權。
- Next packaging / installer / self-update identity 隔離與 fail-closed gate；未來更名作獨立 identity migration，不共用 stable runtime state。
- 讓 custom release workflow 驗證 `custom/main`，而不是沿用官方只接受 `origin/main` 的 gate。
- 定義 custom tag / `buildinfo.Version` / installer metadata / update channel 的一致版本契約。
- 確保上述改動只存在 custom layer，不回寫污染 `main`。

目標 pipeline：

```text
custom/main
     ↓
Core tests
Desktop API tests
Shared UI tests
i18n validation
     ↓
macOS package
Windows package
     ↓
GitHub Release
```

Shared UI 達到穩定 parity 後，再逐步 deprecated 重複的 native business UI。

## 優先順序

```text
M0
↓
M1
↓
M2
↓
M3
↓
M4
↓
M5
↓
M6
↓
M7 (code / stress complete)
↓
M7.5 (Next isolation / separate ChatGPT connection)
↓
M8
↓
M9
```

ACP Manager、Browser Routing 與 Activity Center 都不應在 Desktop foundation 尚未確定前新增大量 AppKit / WPF UI，避免之後重做。

其中 Activity / insertion / truthful state 刻意排在 ACP Manager 前面，因為它們更能提早暴露 Desktop API、streaming、backpressure 與 accessibility 架構問題；ACP CRUD 相對屬於可延後的低風險管理介面。
