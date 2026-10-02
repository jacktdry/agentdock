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

## M4 — Shared Desktop Baseline

先搬足以驗證 shared shell 的既有能力：

1. Overview
2. Runtime
3. Connection
4. Basic Settings
5. Update / Diagnostics 的最小可用路徑

此階段不追求一次搬完 Browser、ACP、MCP / Plugin。舊 native UI 繼續保留。

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
