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

狀態：**實作完成（2026-10-03）**

這是 shared UI foundation 後第一個新能力，也是架構壓力測試。

- truthful execution state
- root / child call
- high-frequency activity stream
- bounded output / continuation
- file changes
- user insertion
- ACK
- attention semantics

完成邊界：

- AgentDock Core 是唯一 execution truth source；Shared Desktop 以 snapshot / bounded replay / SSE 消費，不依 UI 最近互動推測 active state。
- root / child call 使用 stable ID 與明確 parent，狀態區分 running / waiting_for_user / completed / failed / cancelled。
- Activity Center 前端限制為 120 calls、200 events、128 insertions，active calls 優先，避免無界 queue。
- insertion 使用 stable ID 與 accepted / delivered / rejected / expired / cancelled；ACK 只代表 Core 接受，delivery 另行回報。
- insertion eligibility 由 Core 契約明示；沒有真實 delivery boundary 的 command-session / dynamic-MCP child 不接受 insertion。
- reconnect 期間控制 fail-closed；同 epoch replay 可更新最後已知狀態，只有 Core SSE 真正重新 attach 後才恢復 synchronized。
- expected epoch、cursor-ahead、pruned gap 都會強制 reset/resnapshot，避免 Core restart 後保留舊 running state。
- execution journal 僅保存 bounded structural facts，不保存 raw stdout/stderr、任意檔案內容、tool args/results、env、auth header 或 remote MCP body。
- 獨立 review 的 6 個 P2 已全部修正；P0/P1 為 0。
- full Go test、M5 vet/race、30 個 Vitest、i18n 197/197（三語）、macOS arm64、Windows arm64/x64 shared build 均通過。
- keyboard/semantic accessibility 的程式層 gate 已完成；原生 VoiceOver/Narrator smoke 保留在 release UAT。

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
- stable/global 部署仍為 `1.2.0-agentdock.5`；AgentDock Next 已部署 `1.2.0-agentdock.6`，fork tip `5cb54d2`（Keychain compatibility fix `9f56b52`）；system AGY `1.2.16`。
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

2026-10-05 Next-only live checkpoint：Phase 2 獨立 review 無 blocker；arm64 ad-hoc App/ZIP/DMG、direct Core/GUI smoke、原生 Next-only SMAppService Core register/unregister、Next Memory HTTP registry/health、M6/M7 race regression、真實 Codex Browser Broker injection，以及 Codex/Antigravity prompt-driven ephemeral stress 均通過 lifecycle 驗收，且未產生 Next-owned stdio Memory child。Antigravity 的 macOS Keychain popup 已在 fork `9f56b52` 以「private AGY HOME + real `~/Library/Keychains` symlink」修正，Next 現已部署 `1.2.0-agentdock.6`（`5cb54d2`）；20-prompt stress 的五批 SecurityAgent event 均為 0。Stable `mac-dev` 全程可用。剩餘外部 gate 為 `mac-dev-next` ChatGPT connector，以及本機取得有效 Developer ID / certificate-bound signing identity 後的 GUI updater live transaction；不得以修改系統 trust 繞過。2026-10-05 後續排序已調整：先完成 Shared Desktop Connection/Auth/Tunnel readiness，讓 Next UI 提供完整 Public MCP URL + OAuth password，再由使用者透過 ChatGPT UI 手動建立 `mac-dev-next`；不得改寫既有 `mac-dev` 取代此 gate。

此 gate 是 M8/M9 live migration 的必要前置；後續 feature 開發仍限 Next。只有 Next 完整開發、測試及獨立連線後，才另行提出 migration / retirement 計畫，不把舊 AgentDock cutover 當目前下一步。

## M8 — Permission / Approval

狀態：**M8 implementation / available-host validation complete**。Core contract、Desktop control API (`652331ed`)、Shared Permission UI 與 authority hardening (`3df9a784`) 已完成；Reviewer 維持 `defer`。完整 commit、security review、tests 與 Next 原生證據見 [M8 closeout](m8-permission-approval.md#m8-closeout--2026-10-05)。

- Core trace / eligibility metadata、explicit one-time confirmation、bounded history、reconnect/conflict/retry truthfulness 已落地。批准不會執行原操作。
- 內建 operation 的 durable workspace grant 維持 fail closed；UI disabled 並顯示 Core 原因，沒有為按鈕放寬 classifier。
- full root suite 重跑：2,302 tests / 62 packages pass；唯一既有失敗仍是 app-identity script inventory。Real ACP initialize 的環境 skip 已用隔離 Codex adapter 補跑通過。M8 relevant race、root vet、Shared Go、60 frontend tests、typecheck/build、i18n/diff check 通過。
- AgentDock Next fixture-only macOS 原生 UAT 通過政策編輯、Ask/批准/明確 retry、Reject、conflict、app reconnect、Core 中斷/restart；PIDs/port/socket 已清除。stable 完全未操作。
- Windows Core / Shared Desktop / control/file test cross-build 與 portable WSL guard regression 通過。Windows/WSL native、Linux-only helper dispatch/scan execution 是明列外部驗證；cross-build 不等於 native UAT。
- 獨立 security review 的 Windows remote named-pipe / DrvFS 大小寫問題已修正並複核；helper protocol v2 拒絕不支援 protected roots 的舊 payload。
- Canonical Next Task `tsk_9b1ae7b19ea15a40` 關聯 API continuation `tsk_9c4e5c21c4c24c53`。舊 `tsk_b9e4bd0e985de598` 僅為 legacy stable-store reference；repo docs + Next task store 是 source of truth。

Engineering Memory / Codebase index / ADR 與 final handoff 同步。此 closeout 不含 push、merge、release、production 部署或 stable migration。原生 Windows 外部證據與既有 governance debt 持續依 [M8 文件](m8-permission-approval.md) 追蹤。

## Pre-M9 — Feature Parity / Connection Readiness

**最新核對（2026-10-09 約 03:12）：** P4/P5 closeout；P6/P7 native GUI UAT 待補，實際 Nexus pairing 依使用者要求延後。P8 C2b-B2c macOS Edge 唯讀程序預檢已提交 `a1d364e5`，真實 Edge Auth/Profile/no-focus 與 CDP 原子傳輸仍未完成。Next 部署與 Core/Tunnel Registrar 實機驗證通過；Next 8767 loopback 逾時已恢復 HTTP 200，完整 Browser/Policy Go 測試最新 PASS，根因待觀察。**AGY-ACP .14 候選版 456 tests pass，尚未部署，等待 Next 開發／介面就緒後整合測試。** 詳見 [Pre-M9 最新進度與交接](pre-m9-progress-2026-10-09.md)。

狀態：**P1/P2 deployed + live readiness complete；P3 stable/Next side-by-side validation 已於 2026-10-06 完成；P4 ACP Manager full parity 已完成、review 並部署至 Next；P5 MCP Management 已於 2026-10-07 完成 final independent review、Next-only package/deploy 與 live UAT，正式 closeout。P6 Plugin Management contract 已於 2026-10-08 凍結，Phase A Core/Desktop authority（`f70a4715`）、Phase B opaque native candidate staging / hardening（`55e8be0b`、`30fa143b`）、Phase C Shared UI（`999128d2`）與 Phase D independent review / hardening（`07a1a935`；0 BLOCKER / 0 HIGH）已完成，Phase E Desktop/Core live UAT 已通過、native GUI UAT 待補。M9 仍受其餘必要 parity + hardening integration review gate 約束**。Wave 0 contract 已凍結；Wave 1 backend integration（`94367b74`）、Connection UI（`80eeed53`）與 final port rollback lifecycle hardening（`b7e440cc`）已完成。後續實際部署補上 Codex launchd resolution、Shared macOS packaging、SMAppService constraint retry、Darwin cgo port ownership、public-access OAuth credential provisioning、Core health reporting 與 native clipboard copy（`6a3dac19`、`2dc80377`、`63730804`、`821d7e64`、`884f158d`、`6fac35b2`、`82251a3a`、`36683aec`、`a7e92f2c`）。Next Core 已在 `8767` 正常執行；獨立 Named Tunnel `mac-dev-next` / `mac-dev-next.dropabit.dev` 已健康連線並指向 `http://localhost:8767`，public OAuth/MCP discovery 已實測。使用者已手動建立 Next connector，ChatGPT 顯示名稱為 `macbook-air-m3`；stable `mac-dev.dropabit.dev` / stable Core `8765` 保持不變。P3 已完成：stable/Next 可同時呼叫、Core 分別固定在 `8765` / `8767`、state/task/registry/service ownership 分離，且由 stable control plane 執行 Next-only Core restart 時 stable Core PID 未變。現在繼續 Wave 3 Plugin Management，但不得直接進 M9。

執行順序：

1. Feature Parity Audit + **AI UX/IA Design Workshop**：stable native → Next backend → Shared UI，分類 Port / Redesign / Native-only / Deprecated / Not needed；每個主要分類先由 Product / UX / Security / cross-platform / architecture 角色定義 page goal、content hierarchy、actions、help/warning、error/retry/accessibility，再進實作。
2. **Next Connection/Auth Readiness**：Shared UI 顯示 Local MCP URL、完整 Public MCP URL（含 `/mcp`）、OAuth password 的 explicit Show/Copy、endpoint status/test；credentials 只讀 Next-owned state，不進 logs/Diagnostics/Activity/Memory。Next 預設 port `8767`，禁止與 stable `8765` 或 Memory `8766` 共用；自訂 port 必須做 ownership-aware occupancy preflight，foreign/unknown listener fail closed。
3. **Public Access / Tunnel UI**：Local / Quick / Named、Tunnel Token、endpoint、autostart；正式 `mac-dev-next` 使用 **獨立 Named Cloudflare Tunnel + dedicated hostname/token**，不與 stable tunnel 共用。Quick Tunnel 只作 smoke。
4. **Next ChatGPT connector + side-by-side validation**：已完成。使用者手動建立的 connector 顯示名稱為 `macbook-air-m3`；驗證確認 stable/Next connector 同時可用、runtime/task/registry/service ownership 分離，且 Next-only lifecycle mutation 不影響 stable。
5. ✅ **ACP Manager full parity（2026-10-06 closeout）**：profile CRUD、enable/disable、default、detect/version/update、Codex/Antigravity presets、custom adapter；revisioned conflict handling、protected args、fail-closed Safe Update、Next-only live deployment 與 security review 已完成。實作 sequence：`5b365a21` → `85e3e75e` → `39b9c127` → `bb40bfd3` → `f9972a67` → `16f63708` → `fa3deb68` → `c58a6f93`。
6. ✅ **MCP Management Shared UI（2026-10-07 closeout）**：standalone MCP management、plugin-owned read-only inventory、revision/generation、write-only credential/environment、OAuth flow、explicit reconnect/discovery 與安全 Desktop authority已實作。第一輪 6 HIGH 全數修正；final Gemini 3.1 Pro / High read-only review 對 `539fd7d7..6c409b49` 給出 **PASS — 0 BLOCKER / 0 HIGH**。arm64 Next package、8 項 bundle verifier、ad-hoc codesign/ZIP/DMG 驗證通過後，只替換 `~/Applications/AgentDock Next.app/Contents` 並重新註冊 Next core/tunnel；stable Core PID 全程未變。Desktop MCP bridge live UAT完成 authoritative snapshot、disabled create/inspect、write-only env、stale-generation fail-closed、env purge/remove與最終 cleanup；`macbook-air-m3` 驗證仍只看到原本的 `memory` server。詳見 [P5 closeout](pre-m9-mcp-management-roundtable.md#21-p5-closeout--2026-10-07)。
7. **Plugin Management Shared UI — Phase A/B/C/D ✅；Phase E Desktop live UAT ✅，native GUI UAT 待補（2026-10-08）**：本機 folder/ZIP install、review、disabled-by-default activation gate、local-package update diff、enable/disable、remove keep/purge、Plugin-owned MCP env，以及 opaque candidate / revision+generation / semantic approval / truthful recovery均已實作；Shared UI 與 security hardening 已通過 independent review（0 BLOCKER / 0 HIGH）。已確認 Next 安裝 commit `89e34852`，完成 disposable fixture native Desktop bridge install/disabled、enable/disable、ZIP update、write-only MCP env、keep/purge、registry cleanup 與 stable/Next isolation live UAT；尚缺可見 Next 視窗的 keyboard/focus/native picker/320px visual UAT，未正式 closeout。詳見 [P6 contract + implementation checkpoint](pre-m9-plugin-management-roundtable.md#171-implementation-checkpoint--2026-10-08)。
8. **Nexus / startup / logs/config / platform essentials — P7 Phase A2 ✅；Phase B Shared UI 原始碼整合／驗證中（2026-10-08）**：Product/UX、安全及跨平台契約已定案；NexusDock 歸入 Connection 子畫面，不複製 Runtime/Settings/Tunnel。A2.2 後端於 `209c776e` 修正 2 項獨立審查 HIGH（CLI/Desktop 共用配對鎖跨越 restart、rename 後 fsync 正確狀態回報），定點複審 CLOSED 2/2、相關 Go/race/vet PASS。Phase B 已建立 Wails NexusService typed binding、Connection → ChatGPT access / NexusDock 分頁、一次性碼清除、明確 re-pair/reconcile 確認與三語系；Mac 離線 Go／Vue 測試與 production build PASS，獨立 UI 安全審查 PASS（0 verified BLOCKER/HIGH）；Next native GUI/UAT 待補。此 source checkpoint **尚未部署 installed Next，也未執行真實 Nexus pairing 或重啟 Core**。Windows/Linux native authority、A1 `Proxy:nil` 企業代理、P6 GUI、Phase C 及 M9 仍待 gate。詳見 [P7 Phase B checkpoint](pre-m9-platform-essentials-roundtable.md#12-phase-b-offline-shared-ui-source-checkpoint-2026-10-08)。
   - Phase C source（2026-10-08）：Diagnostics Next-only macOS 開啟記錄／設定資料夾已建立，保留 Runtime／Settings／Connection／tray 既有分工。Wails beta.27 bindings 生成 PASS（74 Methods／114 Models）、Go tests/vet、Windows amd64 cross-build、Vue typecheck/121 tests/Vite build 及 independent source review（0 verified BLOCKER/HIGH）PASS；native GUI UAT 待執行；其他平台 fail closed requires_native。[行為與限制](pre-m9-platform-essentials-roundtable.md#13-phase-c-offline-source-checkpoint-2026-10-08)。使用者明確 DEFERRED live Nexus pairing/re-pair/connection/network UAT，此 deferral 不阻擋 non-Nexus tasks；P7/M9 尚未 closeout。
9. Browser Broker Shared UI：依 M6 routing/ownership/lease 架構重新設計，不搬舊 CDP selector。
10. Pre-M9 hardening：清除 `app-identity.sh` governance debt；CBM lifecycle debt 先 triage，只有 release/update blocker 才在 M9 前修。
11. 進入 M9 Release Migration。
12. 用 release candidate artifacts 做 macOS signed updater、Windows/WSL native、Linux helper native validation。
13. M9 closeout 後，Shared UI parity 穩定才逐步 deprecated 重複 AppKit/WPF business UI。

重要：Next connector **不等待全部 parity 完成**，目前已建立為 `macbook-air-m3`。它可用於後續 Next runtime / connector 行為驗證，但 **stable `mac-dev` 是 Next repository 修改、build/package、App reinstall、service/tunnel mutation 與修復的唯一 control plane**；Next connector 不得自我修改或自我重裝。

並行策略：Pre-M9 採 wave-based execution。Wave 0 的 parity inventory / UX-IA / Security UX / architecture mapping 可平行；Wave 1 的 port ownership、Connection/Auth API、Tunnel backend 可平行但需 contract freeze 後才做 UI；P1/P2、connector 建立與 P3 side-by-side validation 均已完成。Wave 3 的 ACP 與 MCP domain 均已 closeout；現在依 **Plugin → Nexus / Platform Essentials → Browser Broker** 的順序接續，各 domain 先做 AI UX/IA roundtable，再以獨立 worktree 推進，active writing workers 以 3–4 個為上限；`app-identity.sh` debt / CBM lifecycle triage 可走 side lane。所有 repo/build/install/live mutation 仍由 stable `mac-dev` orchestrate。M9 只能在必要 parity + hardening integration review 完成後啟動；release candidate 凍結後 macOS/Windows/WSL/Linux helper native UAT 才按平台平行。完整 gate / worktree / integration ownership 見 [Pre-M9 parallel execution plan](pre-m9-feature-parity.md#parallel-execution-plan)。

## 上游 v1.0.1／PR #229 選擇性採用（2026-10-09，已定案為待開發清單）

**不直接 merge 上游**。依 [v0.9.1→v1.0.1 採用矩陣](upstream-v101-adoption-plan.md) 和 [PR #229 規格](upstream-ports/task-progress-live-card-pr229.md) 逐項實作、測試；「採用」表示排入 Next 開發清單，**不表示已移植／已部署**。此前來源版本的正式 tag 必須以 upstream 遠端 SHA 核對，不能把本機同名 tag 當比較基準。

| 對照來源 | Next 採用決策／必要修改 | 安排階段與完成 Gate |
| --- | --- | --- |
| v1.0.0/v1.0.1 immutable Release source／簽章驗證／artifact gate | **採用／按 Next 重做**：`custom/main` release source、Next tag/buildinfo、更新通道、Developer ID/Windows installer 信任與制品 provenance | **M9 P10**：版號、來源 commit、安裝／執行／更新狀態一致，正式 RC artifacts 驗證 |
| v1.0.1 updater／Windows active generation recovery／service lifecycle | **採用故障模式與測試**，不覆寫既有 Arbiter／Shared UI；拒絕 stale journal 錯誤回滾、恢復不應改變 stable | **M9 P11**：下載/安裝/中斷/試用/失敗 rollback、macOS/Windows/WSL/Linux 原生證據 |
| v1.0.0 cloudflared Component Store/Catalog/atomic active pointer | **採用設計、延後程式移植**：可信元件來源、驗證、原子版本切換、legacy import、Next 簽章／runtime ownership 與 Tunnel 兼容 | **M9 後獨立里程碑**，依 [Component Manager 設計與 Gate](upstream-ports/cloudflared-component-manager.md) |
| v1.0.0 observability／stage timing | **已具備部分後端**；不重建 recorder，僅補 Execution Center 可用性／隱私 UX | **非阻塞後續 UX**；先比對既有 trace／p95／execution evidence |
| v1.0.0 原生 macOS/WinUI 重設計 | **只參考流程與易用性**；Next 保留 Wails+Vue Shared UI 與 OS 必要原生 adapters | **P12 後續 UX**；不遷回重複原生業務 UI |
| **尚未合併的 PR #229** Task Progress 單張即時卡片 | **採用／獨立設計**：`task_create` 建立一張卡、`task_snapshot` 唯讀同 Task ID 更新；`task_manage` checkpoint/final_review/complete 不再新增 Task UI；保留 legacy create 相容 | **非阻塞 UX 支線（P2）**：完成權限/scope/快取/輪詢負載、ChatGPT full/compact/off 真實 UI UAT 才 closeout |

**MCP Apps 卡片降噪（新增非阻塞 UX 項目）**：使用者 ChatGPT iOS 截圖中的 Workspace、Codex Session、Model `X → X`、Reasoning effort `high → high` 卡片**不屬 PR #229**。規劃依 action/scope 設計精簡結果、no-change 不掛大型卡與必要事件完整可見；不可假定 ChatGPT 能跨工具訊息原地合併。詳見 [MCP Apps 卡片降噪規格](mcp-apps-card-noise-reduction.md)，與 Task live-card 一起進行 ChatGPT 真實 UAT，但不得阻塞 P8/AGY/M9。

**PR #229 的產品邊界**：目標是「**同一個 Task ID 的多次操作只維持一張會更新的卡片**」，不是把不同任務合併。ChatGPT 使用者連續說「繼續」時，若仍是同一工作，Orchestrator 應沿用原 `task_id`；每次另外 create 新 Task 仍會產生新卡片，PR 無法自行消除。最新採用與等待佇列另見 [剩餘工作 checkpoint](remaining-work-checkpoint-2026-10-09.md)。

## M9 — Release Migration

建立從 `custom/main` 的正式 cross-platform release pipeline。

上游 `v0.9.1 → v1.0.1` 唯讀採用提案與 M9 source/recovery gates：[`upstream-v101-adoption-plan.md`](upstream-v101-adoption-plan.md)。Component Manager 的外部執行元件遷移**不納入此 M9 阻塞範圍**，另依該文件的 staged migration gate 評估。

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
Pre-M9 Feature Parity / Connection Readiness
↓
M9
↓
Release-native UAT
↓
Shared UI parity confirmed → duplicated native business UI deprecation
```

ACP Manager、Browser Routing 與 Activity Center 都不應在 Desktop foundation 尚未確定前新增大量 AppKit / WPF UI，避免之後重做。

其中 Activity / insertion / truthful state 刻意排在 ACP Manager 前面，因為它們更能提早暴露 Desktop API、streaming、backpressure 與 accessibility 架構問題；ACP CRUD 相對屬於可延後的低風險管理介面。
