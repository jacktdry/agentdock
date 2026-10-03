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

## M6 — Browser Routing

實作 workspace-aware policy：

```text
company project
→ persistent profile Edge

default task
→ isolated Chrome

explicit override
→ chosen profile/browser
```

並處理：

- CDP discovery
- profile/process ownership
- stale cleanup
- tmp lifecycle
- health/diagnostics

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
- Adapter-specific cleanup 留在 codex-acp / refined-antigravity-acp fork，不在 AgentDock Core 寫 agent-specific kill hack。

完整設計與接手條件見 [acp-lifecycle-memory.md](acp-lifecycle-memory.md)。

### M7 CBM lifecycle 技術債（2026-10-03）

實測發現 Codebase Memory executable 更新後，舊 daemon / supervisor 仍存活時，新 index worker 可能因 build mismatch 被拒絕並形成連續 worker failure。這屬於 dynamic MCP service lifecycle / process ownership 問題，不應以全域 `pkill` 解決。

M7 / diagnostics 規劃時需一併評估 installed-vs-running build 偵測、restart-pending、active work drain、pending re-index queue、bounded retry/backoff，以及 versioned executable / graceful restart 的責任邊界。完整現場證據與接手確認清單見 [cbm-lifecycle-conflict.md](cbm-lifecycle-conflict.md)。

## M8 — Permission / Approval

評估並最小化引入：

- Permission Profile
- Approval Policy
- effective permission source
- approval history

Approval Reviewer 預設 defer。

此 Milestone 同時完成剩餘 Browser / ACP / MCP / Plugin / Settings 的 shared UI parity 評估，決定哪些舊 native business views 可以 deprecated。

## M9 — Release Migration

建立從 `custom/main` 的正式 cross-platform release pipeline。

前置工作：

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
M7
↓
M8
↓
M9
```

ACP Manager、Browser Routing 與 Activity Center 都不應在 Desktop foundation 尚未確定前新增大量 AppKit / WPF UI，避免之後重做。

其中 Activity / insertion / truthful state 刻意排在 ACP Manager 前面，因為它們更能提早暴露 Desktop API、streaming、backpressure 與 accessibility 架構問題；ACP CRUD 相對屬於可延後的低風險管理介面。
