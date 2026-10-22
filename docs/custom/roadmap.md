# AgentDock Custom Roadmap

> **2026-10-23 local RC execution:** [Current prioritized acceptance](next-local-rc-acceptance.md) tracks the remaining company Edge, independent GitLab authorization, lifecycle/native UAT and NEXT-only RC gates. No new installed RC or live company-site acceptance is claimed by source diagnostics.

> **Current operational entrypoint (2026-10-21):** [NEXT active backlog](next-active-backlog-2026-10-21.md) and [worktree consolidation](worktree-consolidation-2026-10-21.md). Older sections preserve chronological checkpoints and may state pending gates closed by later evidence.

> 狀態：Accepted initial roadmap

## 2026-10-13 — Product-first work-order reset (accepted)

The reason for Next is **ChatGPT + ACP agents coordinating real local work without disturbing the user**, with one accountable Broker/lease lifecycle, truthful shared UI and maintainable macOS/Windows interfaces. Stable/Next side-by-side exists to protect the production Mac-Dev control channel, not to create an indefinitely stricter test environment. Mandatory gate for every task: actual workflow → smallest useful change → observational acceptance + no-interruption check → explicit remaining blocker. Project-wide developer rules: [`AGENTS.md`](../../AGENTS.md), and [product-first decision](next-product-first-guardrails.md).

**Return to actual P8 integration before adding new test-only security layers.** Short-term order: visible Browser operator readiness from existing Core snapshot → one real ChatGPT + two real ACP background shared-headless workflow / cleanup acceptance → consent-grounded Edge/Chrome route proof → close required P6/P7/M9 parity/release evidence. The native Codex Seatbelt `managed preferences` test-only SKIP remains an unresolved fixture fact, **not a mandatory VM/user-identity feature or a general P8 blocker**. Never claim fake-client protocol success equals live Agent UAT; never bypass genuine permissions or jeopardize stable AgentDock.

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

## Pre-M9 Cross-cutting HIGH — MCP Runtime Lifecycle / Process Ownership（2026-10-11 新增）

**新增後續改進項目，尚未實作共用 controller**；不重開已完成的 M7/P4/P5 closeout。以 [MCP Runtime 生命週期規則與程序盤點](pre-m9-mcp-runtime-lifecycle-policy-2026-10-11.md) 為此工作線的設計與驗收依據。

- **現場證據**：單一 Codex ACP app-server 直接持有 62 個 CBM Clients、121 個 Chrome DevTools npm launcher（其下另有 server）、128 個 Python search-console MCP 程序。存活時間／CPU 閒置 **不能證明是 orphan**；禁止直接終止現有程序。
- **HIGH-0 先唯讀**：Next Diagnostics 關聯 Adapter → loaded ACP thread → stdio MCP child / HTTP client → lease/generation/owner，區分外部 Codex Desktop 與 Next-owned；查證三個大量程序家族何時應 unload/close、Codebase Memory Git watcher/manual indexing options conflict。
  - **2026-10-11 策略更新（取代 Rust fork 必要前置）**：Codex CLI 自己的 MCP 問題交給上游；Next HIGH-0 限定透過 ACP 管理的 Codex app-server。先做 prompt-driven task completion MCP usage／owned-only cleanup report（僅參考）、Next-only stdio launch wrapper 的註冊與實際 kernel birth 驗證（只有已明確配置者適用），並在缺 per-Thread/lease 證據時明確 owner_unverified、不殺程序。Rust CLI fork 保留唯讀研究而非 P0 release blocker。需驗證多 Session 共享同 app-server，不能把每個 Task 當成 child 唯一 owner。詳見 [ACP MCP cooperative design](pre-m9-codex-acp-mcp-cooperative-lifecycle-2026-10-11.md)。
  - **2026-10-11 B1 source-only checkpoint**：完成測試專用 Codex ACP stdio MCP launch wrapper + kernel-verified Unix Registrar 原型，具 preflight nonce、child PID/birth direct-child 診斷與透明 stdio，但尚未接入 Next Core / Codex profile、沒有 Session→native Thread 歸屬或自動清理權限。後續需 Core-only real app-server birth admission、isolated config、operator report、Lease fence，才可進 Next-only live fixture UAT。
  - **2026-10-11 B2 source-only checkpoint**：完成可選用的私有 Core Unix registrar（先證明 Core→Adapter→Codex app-server kernel 身分，再接受 wrapper→MCP IPC 診斷）、ACP 既有 HMAC 的可信 source-only bridge、全新五階段 owned helper fixture。僅觀察不授權終止。仍阻於 Codex ACP per-session config 與全域 native MCP 合併、啟動前無 loaded Session、P5/M8 交叉處理程序 allowlist／generation admission；**沒有正式 Runtime / live MCP 接線**。
  - **2026-10-11 B3 source-only checkpoint**：`codex-acp` opt-in 在原生 app-server spawn 前要求固定 Next-owned isolated `CODEX_HOME`（無全域 config/已授權 auth 才合法）、session 前 `configRead` 拒絕 inherited native/project MCP server，不再悄悄 dedup。Go HMAC native birth scope 支援 Adapter 已啟動但無 loaded Session，舊診斷門檻不變。正式 profile provisioning/auth、初始 Adapter/native generation handshake、Native config 安全證明、P5/M8 lease fence 與 live UAT 仍未完成，不能自動清理。
- **HIGH-1 統一管理**：沿用 M7 ACP session policy、P5 MCP/Plugin authority、P8 Browser Broker 實作 MCP owner-bound lease、bounded capacity、inflight-close fence、drain/quarantine 和 generation fencing。CBM 優先 shared daemon；DevTools 必須 per Browser Broker lease 隔離 selected-page；一般 stdio 不可無條件跨不同 ACP owner 共用 stdin/out；Codex adapter app-server 依其真正 thread unload/close authority 管理。
- **HIGH-2 驗收**：20 次 ephemeral／close/resume、4–8 parallel、persistent 保留、Browser 4 active/16 queue，macOS/Windows process birth/ownership UAT；可證實 Next-owned child 才允許 bounded graceful close，unknown 則 cleanup_required，不開 auto kill/restart。
- **安全限制**：禁止 `pkill`、按 PID 名稱或年齡回收、修改 stable、干擾 Codex Desktop／AGY／已安裝 Next、Edge/Chrome、或以本議題解鎖 P8 sticky marker。

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
12. 用 release candidate artifacts 做 macOS Next-only **ad-hoc 本地安裝／手動版本替換／回滾**及原生 UI 驗收；不做 Developer ID／Apple Notarization（無付費帳號），現有 certificate-bound 自動更新仍 fail-closed。Windows/WSL 只需 source-only 理論可行、無原生必過；Linux helper 有環境再驗證。詳 [P11 範圍](p11-adhoc-native-uat-scope.md)。
13. M9 closeout 後，Shared UI parity 穩定才逐步 deprecated 重複 AppKit/WPF business UI。

重要：Next connector **不等待全部 parity 完成**，目前已建立為 `macbook-air-m3`。它可用於後續 Next runtime / connector 行為驗證，但 **stable `mac-dev` 是 Next repository 修改、build/package、App reinstall、service/tunnel mutation 與修復的唯一 control plane**；Next connector 不得自我修改或自我重裝。

並行策略：Pre-M9 採 wave-based execution。Wave 0 的 parity inventory / UX-IA / Security UX / architecture mapping 可平行；Wave 1 的 port ownership、Connection/Auth API、Tunnel backend 可平行但需 contract freeze 後才做 UI；P1/P2、connector 建立與 P3 side-by-side validation 均已完成。Wave 3 的 ACP 與 MCP domain 均已 closeout；現在依 **Plugin → Nexus / Platform Essentials → Browser Broker** 的順序接續，各 domain 先做 AI UX/IA roundtable，再以獨立 worktree 推進，active writing workers 以 3–4 個為上限；`app-identity.sh` debt / CBM lifecycle triage 可走 side lane。所有 repo/build/install/live mutation 仍由 stable `mac-dev` orchestrate。M9 只能在必要 parity + hardening integration review 完成後啟動；release candidate 凍結後 macOS/Windows/WSL/Linux helper native UAT 才按平台平行。完整 gate / worktree / integration ownership 見 [Pre-M9 parallel execution plan](pre-m9-feature-parity.md#parallel-execution-plan)。

> **P8 2026-10-10 約 01:10 更正 gate：** CDP／Remote Debugging 允許，首要要求是共享 Broker 與不干擾 UAT：零反覆提示、不搶焦點、不為每 ACP 新啟 browser。公司 CMS 使用既有 Edge Profile，明確指定 Chrome 時使用 exact Profile；失敗不 silent fallback。managed Headless Chrome DevTools MCP 為候選主路徑；使用者 Profile 可評估 authorized persistent CDP 或 Extension + Native Messaging contingency，均須獨立 UAT、不得繞過同意。**尚未完成新規格實作／部署／live acceptance；歷史 checkpoints 保留。** 詳見 [現行 Browser 決策](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。


## P13 / P14 — Next ChatGPT 卡片與 UI/UX 品質工作線（2026-10-14）

**規劃已接受／實作未開始**。與 P10 Release Migration 的 source-only prep 並行，P10 不正式發佈；不把 UI/UX 審查或尚未驗證的界面宣稱 release-ready。工作線：`feature/next-uiux-audit`。

- **P13 Task Progress 單一即時卡**：選擇性參考 upstream PR #229（目前 open），一個 Task ID 一張進度卡，checkpoint/final_review/complete 只更新該卡。真實 ChatGPT MCP App、權限/Scope、hide/teardown、multi-card polling、full/compact/off 和舊 Plugin metadata cache 必須驗證，既有 task_manage API 相容；不同 Task 絕不錯誤合併。見 [P13 規格](upstream-ports/task-progress-live-card-pr229.md)。
- **P14 MCP Apps 卡片降噪**：重複 workspace_context、ACP status、model/effort no-change 應縮減卡片密度，重大變更、錯誤、審批需清楚可見；不假設 ChatGPT 可以刪除或原地合併不同工具訊息。見 [P14 規格](mcp-apps-card-noise-reduction.md)。
- **全頁 UI/UX 審查（UX-0 起）**：從 Next Shared UI 各頁、系統原生選單列/Tray、到 ChatGPT MCP Apps 逐頁檢查功能可用性、狀態來源、loading/空白/error/cancel/retry、固定佔位防 layout shift、文案精簡/進階摺疊、繁體中文、a11y/keyboard/深淺主題/窄版、操作成功/失敗/未知的證據。其檢視/決策/分期驗收以 [UX 圓桌](next-uiux-roundtable-2026-10-14.md) 為準。
- **Menu Bar/Tray**：參考使用者提供的 stable AgentDock 1.0.1 screenshot；Next Wails 現有 tray 只有 Show/Quit，故「服務健康/連線/正在執行」皆為待設計/待原生驗證，不應臆測已完成。尤其 loading 消失不可使行高跳動；NexusDock 依使用者決定維持「開發中」而不宣稱可用。
- **2026-10-14 UX-S5 第三批 Source**：Next Wails Tray 新增手動「檢查核心服務狀態」，使用原有 Next RuntimeService Status 權威讀取；初始 Unknown、附最後觀察時間，90 秒後自動標成過期。無背景輪詢、不顯示無證據 MCP/ACP/ChatGPT 健康燈號；原生 macOS/Windows Tray live GUI UAT 未驗證。參見 [UX 第三批](next-uiux-roundtable-2026-10-14.md)。
- **2026-10-14 UX-S4/S5 第二批 Source**：NexusDock「開發中」頁面不再提供 Pair/Reconcile/Restart 變更入口，保留唯讀、unknown 狀態及本地化時間；Next Wails 選單列及原生 Plugin 選檔加入 en/zh-Hant/zh-Hans 系統語言標籤，未新增無證據健康綠燈。Vitest 168/168、vue-tsc、Vite、Go native、i18n check 通過；installed macOS/Windows GUI/Tray UAT 與真實健康資訊仍待後續。詳見 [UX 第二批](next-uiux-roundtable-2026-10-14.md)。
- **2026-10-14 Claude Sonnet 5.5 補充審查**：重點確認 MCP OAuth 多 Server 授權狀態跨 scope 殘留風險、Connection mutation/read 錯誤覆蓋、Runtime 首次載入未知、Tray 需 Go 端帶時間戳的可信來源。ACP stale 由 P0 風險疑點改列 P1 語意/動作一致性改善（沒有權限洩漏證據）；P13/P14 Host UAT 與 Native Tray 分開驗收。詳見 [UX 圓桌 Claude 複核](next-uiux-roundtable-2026-10-14.md#ai-圓桌第三方-claude-sonnet-55-專家複核2026-10-14)。
- **排序與合併**：P10 source-only 版本與發佈管線繼續；UX review/P13/P14 可先獨立實作測試並審查後有序合併。正式 RC freeze 與 P11 原生安裝/更新回滾驗收放在 UI/UX 必要修正後；P12 仍是 M9 closeout 後才淘汰重複 native 業務 UI，不因新增 P13/P14 改變其定義。

> **2026-10-14 P11 無付費開發帳號決策**：P11 不做 Developer ID、Notarization、Stapling 或憑證綁定的 GUI 自動更新實測；保留 Next-only ad-hoc 本地安裝、手動更新與回滾、服務與 UI 原生驗收，**絕不放寬現有更新器的 signer fail-closed**。見 [P11 source of truth](p11-adhoc-native-uat-scope.md)。

## M9 — Release Migration

> **2026-10-14 P10 macOS prepublish engineering closeout**：已整合 `feature/next-uiux-audit`（不含 P13/P14 實驗），從乾淨 `cceadb0c` 實際編譯 Next arm64 App／ZIP／DMG，同 SHA 版本／來源／codesign（ad-hoc）／DMG／ZIP SHA inventory 全部 PASS；Shared Desktop 168/168 Vitest、獨立 Go、root `make check` PASS。Windows 依使用者決定僅 source-only 理論可行、不交付 Setup。**正式 Tag／Release／Push／update feed 及 Hosted CI 遠端驗證尚未進行，P11 簽章／更新回滾／原生 UAT 另案。** 詳 [P10 結案證據](p10-local-prepublish-closeout-2026-10-14.md)。


建立從 `custom/main` 的 Next 來源／候選版驗證契約。依 2026-10-14 使用者決定，**P10 僅 macOS arm64 發佈前工程收尾**；Windows 為不交付的 source/cross-build feasibility。正式對外 Release 工作流程仍須使用者另外確認發佈時機。

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
macOS arm64 RC App / ZIP / DMG + SHA inventory
Windows source/cross-build feasibility (non-shipping)
     ↓
Formal GitHub Release (deferred until authorization / P11 gates)
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

**2026-10-12 Codex ACP MCP B0–B6 固定收斂**：見 [B0–B6 closeout](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。B4 source-only pre-Session signed native app-server bootstrap + nonblocking Core GenerationPin + digest-bound stdio MCP injection、B5 logical multi-Session leases + atomic pre-ACK Core registry commit + read-only reconciliation、B6 20-owned-fixture + 8-concurrency + native auth-free isolated Codex app-server smoke 已做。**B6 production gate 未放行**：因尚缺 Next isolated auth、真正 P5/M8 cross-Core admission/lease、已登入 real Next-only Codex ACP instrumented session + recycle UAT；不得描述成已部署或自動回收現有 MCP，也不得為此 fork 官方 CLI 或任意 `kill`。

**2026-10-12 B6 P5/M8 source-only 補強**：Core-private Prepare/Attach 現在需要 request-context 中已認證穩定 principal 與 M8 Allow；server/wrapper/backend/args 完整 v2 digest 作為審核指紋，避免原始敏感參數進審核歷史。macOS Next 以 owner-private O_NOFOLLOW + flock 跨 Core 鎖限制同一個原生世代的啟動，權限版本變更/關閉拒絕，保持只讀 MCP 庫存與不允許 PID 強制終止。一般回歸、Race、Vet、Linux/Windows 交叉編譯通過。**尚非 B6 正式發布**：缺跨 Core policy store 原子交易、意外當機後殘存程序調和，以及真正已授權的 Next Profile E2E/UAT。詳見 [固定驗收文件](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。

**2026-10-12 B6 當機安全補強**：Next MCP 啟動前持久化 `unresolved native generation` marker，以 PID 來源不明時 fail closed 防止 crash 後其他 Core 因 `flock` 自動釋放而重啟；追加 Permission Store read-only disk drift guard，遇不同 Core epoch/policy/審核歷史、symlink 或 corrupt 拒絕啟動。這不是 native MCP 回收權限，也不是跨 Core M8 mutation transaction；**需 operator-reconciled cleanup、獨立登入 UAT 才可能放行 B6**。參照 [固定 B0-B6 文件](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。

**B6 後續實測與 release gate**：Next-only Core 的權限寫入唯一所有權鎖、開機世代驗證的授權 recovery、原生 Codex 產生的 cache/sqlite 在 Go + TS 端安全重啟已補強。正式 Next Core ACP Adapter/native app-server Prepare + M8 Attach 通過；Session/new 因獨立 Codex HOME 尚未認證而被拒絕，故完整 kernel-registered stdio MCP + authenticated model tool-call/close UAT 尚缺。**No release, no extra B7**，留待獨立使用者授權登入及安全 operator route。詳見 [驗收追蹤](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。

**2026-10-12 B6 source-only E2E PASS（未部署）**：獨立官方 device-auth 後，真實 Next Core + M8 Allow + ACP Adapter/native Codex + wrapper/kernel provenance + 模型 `ping`/Run completed + Session/Core Close 的完整隔離 UAT 已通過 (22.30s)；實測三個受控測試 PID 全部退出。Next Go/ACP TS 已將登入產生的 owner-private `log/` 納入安全原生狀態白名單。B0–B6 固定工程工作線可收斂；**正式版本／operator UX／production activation 尚未部署或放行**，不代表目前已安裝的 NEXT 在執行 MCP 自動生命週期管理，亦不得任意清除未解決世代 journal。參照 [驗收明細](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。

**2026-10-12 P6 closeout**：已在安裝中 Next 真實 macOS GUI 完成 Plugin empty/list/detail、folder/ZIP native picker、1.0→2.0 review、enable/disable、Escape/焦點返回、P5 deep link、remove keep/purge 確認與 registry 0 清理，配合先前 Desktop bridge write-only env / generation / lifecycle UAT，P6 已結案。原生視窗 Wails MinWidth 760；另以隔離 Headless Chrome 320 CSS viewport 驗證 frontend reflow/no horizontal overflow（不冒充 native 320）；尚未做完整焦點 trap 環測。詳細 [P6 GUI UAT 證據](pre-m9-plugin-management-roundtable.md#175-p6-native-gui-uat-closeout--2026-10-12)。下一優先 P7 Nexus/Platform Essentials（實際 pairing 維持 defer）、P8 Browser Broker 和 P9 hardening，未提前進 M9。

**2026-10-12 P7 read-only GUI probe（未結案）**：Next 原生 NexusDock/Diagnostics UI 已確認可見；Nexus 狀態仍為「未知／服務無法使用」，不能冒充未配對驗收；Diagnostics 開啟資料夾原生動作因本次工具安全限制尚未執行。Go 相關 package regression 與 Vue typecheck、139/139 tests PASS，stable/Next Core PID 觀察前後不變；live Nexus 配對維持 deferred。見 [P7 GUI checkpoint](pre-m9-platform-essentials-roundtable.md#2026-10-12-p7-native-read-only-gui-probe-not-closeout)。

**2026-10-12 P7 使用者收斂**：Logs/Config 原生資料夾開啟由使用者手動驗收 PASS；NexusDock 暫停處理，於其頁面顯示三語系「開發中」，不再將 Nexus 身分/配對問題當作 P8 前置。維持既有安全契約與 P7 跨平台原生、P9/M9 gates。詳見 [P7 更新](pre-m9-platform-essentials-roundtable.md#2026-10-12-p7-user-acceptance-adjustment-folders-pass-nexusdock-deferred)。

**2026-10-12 P7 安裝版更新**：NexusDock 三語系「開發中」已透過 Next-only bundle 成功建置部署（Go Core 8767 / stable 8765 分離、兩者健康）；macOS Registrar 的 `launchctl` stdout Pipe 等待問題已修復。使用者手動 Logs/Config PASS；Nexus pairing/unknown 狀態 deferred，原生此頁像素驗證未做；P8/P9/M9 gate 不受此註記放寬。見 [release checkpoint](agentdock-next-isolation.md#2026-10-12-next-nexusdock-label-release--registrar-pipe-fix)。

**2026-10-12 P8 source-only operator observability checkpoint**：新增 Broker managed pool mode `exclusive`／`experimental_shared`／`unknown` 與五個安全的 aggregate shared budget 數字，任何不一致 fail closed；Browser Desktop/Wails/Vue 三語系顯示皆唯讀。仍未切換 shared Chrome production opt-in、部署 Next 或接管 Edge/Chrome；三 Agent no-focus/no-modal 與 Profile authority 仍是 P8 gate。詳見 [P8 §32](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#32-2026-10-12-p8-aggregate-shared-pool-browser-desktop-diagnostics-source-only)。

**2026-10-12 P8 REAL headless source-only validation**：原有真實 3 模擬 Owner／1 MCP+Chrome per generation／2 generation isolated Headless proof 重跑 PASS（41.98s）。擴充 test-only 容量診斷一致性／owner claim 歸零／非同 Workspace 進入拒絕，重新執行真實 Headless PASS（34.91s），Browser/BrowserPolicy full tests、shared `-race`、vet PASS，owned process cleanup confirmed；未開放 runtime shared，未進行真實 3-Agent native focus/consent UAT，P8 仍未 closeout。見 [P8 §33](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#33-2026-10-12-p8-real-headless-shared-pool-diagnosticsforeign-workspace-acceptance-source-only)。

**2026-10-12 P8 Supervisor journal audit（source only）**：§34 新增 macOS 私有 dirfd 的 durable receipt 全目錄唯讀稽核，只輸出五個安全 aggregate 數字（accepted/settled/acked/unknown）；任何檔案／目錄或 JSON 完整性不可信便全數 fail closed。**不提供** restart/replay/approval/Chrome cleanup 解鎖或實際三代理 UAT 佐證；Next/stable runtime/Browser 路由未接線、未部署。見 [P8 §34](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#34-2026-10-12-p8-supervisor-durable-journal-structural-audit-source-only)。

**2026-10-12 P8 ACP Broker authenticated transport evidence（source only）**：§35 在現有 Browser MCP HTTP token 驗證路徑加入已核准 endpoint 存取 owner 彙總，與純註冊 owner 分開；同鎖撤銷、防重複計算，Desktop Wails／Vue 三語系只讀顯示，數字異常 fail closed，不洩漏 Session、Profile、Token。它不是 handshake/browser process/Chrome Profile/no-focus 的證據；尚未啟用 shared Chrome 或完成 3 真 Agent UAT。見 [P8 §35](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#35-2026-10-12-p8-authenticated-acp-browser-mcp-transport-evidence-source-only)。

**2026-10-12 P8 real MCP SDK loopback proof（source only）**：新增兩個合成名稱 Client，經真實 localhost TCP + Go MCP SDK `initialize`/`tools/list`／虛構 lease 的工具拒絕／Session token 撤銷，以 `RuntimeBrowserDesktop` 核對 endpoint count 2/0→2/1→2/2→0/0，Worker Probe/Open 被禁止，無瀏覽器或真實 Codex/AGY adapter。這是 protocol path proof，**不是**真實三個 Agent + Chrome/no-focus native UAT。見 [P8 §36](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#36-2026-10-12-p8-isolated-real-mcp-sdk-loopback-acceptance-source-only)。

**2026-10-12 P8 真實 Codex ACP 初始化（SOURCE-ONLY）**：§37 加入 opt-in 獨立 0700 HOME/CODEX_HOME／`CODEX_PATH` 指向專屬假 Native Backend 的真實 `codex-acp` 初始化測試，未使用真實 Codex Native/登入/Chrome/Keychain。處理 fake fixture SIGKILL 與唯讀 `config/read` 後重跑 5 次 PASS，確認私有 auth/config 無複製且測試擁有的 child 已退出。這**不是** SessionNew/Browser MCP 握手／真實 AGY UAT，P8/M9 不解除。見 [P8 §37](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#37-2026-10-12-p8-real-codex-acp-isolated-initialize-fake-native-backend-source-only)。

**2026-10-12 P8 真實 codex-acp SessionNew → fake native MCP 注入（SOURCE ONLY）**：§38 補足實際 installed Codex ACP SessionNew，假帳號／假 native thread 不執行模型，嚴格證明 Browser + Computer 兩個指定 MCP URL／共用 Session Bearer 從 Provider 送進 fake native `thread/start`。另一個合成 Go MCP SDK 用擷取的 0600 臨時 Browser Bearer 真的 handshake、列九工具、拒絕偽造 lease、撤銷後 HTTP 401；不是 Codex native 自行連線。無 Chrome/Edge/AGY/Keychain，尚未完成真實三 Agent/共用 Chrome/no-focus 驗收。見 [P8 §38](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#38-2026-10-12-p8-actual-codex-acp-sessionnew-to-fake-native-mcp-config-source-only)。

**2026-10-13 P8 Codex native Seatbelt 隔離／macOS managed-preferences 阻礙（SOURCE ONLY）**：§39 新增實際 Codex native 0.160.0 直接 app-server JSON-RPC 的 test-only opt-in harness，preflight 強制 user HOME/Keychain deny、單一 localhost 埠 allow、其餘本機埠／外網 deny、process group owned-only cleanup。Native `initialize` PASS，但 `thread/start` 因 macOS `Failed to synchronize managed preferences` 被嚴格 sandbox 擋下；已嘗試 system cfprefsd.daemon 仍不夠且撤回；只有此已知狀態會 SKIP，**沒有**宣告 native MCP tools/list/三 Agent Chrome UAT 完成。見 [P8 §39](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#39-2026-10-13-p8-real-codex-native-mcp-isolation-preflight-and-managed-preferences-blocker-source-only)。

**2026-10-13 P8 真實 MCP HTTP → Headless Chrome**：source-only `TestACPBridgeSharedRealHeadless` 現已使用兩個獨立 SDK 客戶端，經真實 HTTP MCP `browser_acquire/evaluate/snapshot/release` 操作同一個受管理 Headless Chrome，實測僅一個 Chrome main、隔離頁面/跨權限拒絕、撤銷 401、owned cleanup/budget 0，**real opt-in PASS 68.55s**。仍不等於安裝版 Codex/Antigravity/ChatGPT 三個 Agent 自行呼叫；下一步以真正 Next 連線與不干擾桌面的 UAT 為主，不再增生合成沙箱。詳見 [P8 §40](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#40-2026-10-13-p8-genuine-mcp-http-to-real-managed-headless-chrome-source-only)。

**2026-10-13 P8 Next Browser 啟用預設（PRODUCT）**：現有 Next 8767 真實授權 HTTP Browser Snapshot 回報 `browser_disabled`／0 workers、leases；Next ACP 已啟用、非 stdio，但啟動環境未設定 `AGENTDOCK_BROWSER_ENABLED`，被共用 `FromEnv` 預設 false 關閉。新增原始碼 Next-only 有條件預設啟用 lazy Browser Broker（明確 false 仍關閉；stable / non-ACP / stdio 不變），不啟用試驗共享池、不自動開 Chrome/Edge。**尚未部署／重啟 Next**，所以 live 仍為 disabled；下一步安全更新後做真正的 ACP 後台工作驗收。見 [P8 §41](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#41-2026-10-13-p8-next-live-browser-disabled-blocker-and-source-startup-default-product)。

**2026-10-13 P8 Next real Codex 及歷史 Snapshot 修正**：已驗證已安裝 Next Core vcs=`55916091`（npx/node 修復版），真實 Codex ACP 已成功 new/run/close；模型有呼叫 `browser_acquire`，但 Worker 出現 `signal: killed`，未執行 evaluate/snapshot/release，**不得視為 real Agent UAT 通過**。獨立精簡 PATH 的 `npx@1.7.0 --version` 與實際 Worker Registry 啟動／回收正常，仍須查明 Next Core 特定呼叫生命週期。另修正被 `CleanupComplete` 租約／`WorkerStopped` 歷史紀錄誤判 `stale` 的唯讀 Browser Snapshot（真正 Failed/Orphan 繼續警示），未動底層清理／權限；待測試通過後源碼提交，未部署該新 UI 投影。詳見 [P8 §42](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#42-2026-10-13-p8-installed-next-real-codex-tool-attempt-and-historical-browser-snapshot-correction)。

**2026-10-13 P8 真實 Worker post-start 診斷（未結案）**：預設 `WorkerRegistry` 真實 Headless 測試可到 `WorkerReady`，但第一個 post-start `list_pages` 仍可能 timeout；不把 Ready 冒充可用，也不因 PID/PPID 猜測清理既有 Codex MCP。主機亦觀察到大量長期 Codex MCP children，證明資源生命週期需要產品層收斂，但不等於孤兒歸屬證據。詳見 [P8 §43](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#43-2026-10-13-p8-real-default-worker-post-start-diagnostic-and-host-process-pressure-no-closeout)。

**2026-10-13 Next ACP Session lifecycle 防累積（PRODUCT）**：Next desktop / non-stdio 新 Session 在未指定 policy 時，從共用 persistent 預設改為 runtime-scoped **idle-managed 30 分鐘**；Stable、Next stdio 與明確 persistent 仍完全保留。一次性 delegated worker 應明確使用 ephemeral。沿用既有安全 `session/close`／host capability release，不掃 PID、不改官方 Codex、不遷移既有 Session。原始碼與定點/ACP regression 已驗證，尚未部署；真實 Browser UAT 等主機負載正常後再做。詳見 [P8 §44](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#44-2026-10-13-next-acp-default-idle-managed-lifecycle-product)。
