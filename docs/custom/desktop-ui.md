# Cross-platform Desktop UI

> 狀態：Architecture proposal；framework 尚需 POC 驗證

## 1. 問題

目前官方 AgentDock Desktop 分成：

```text
macOS   → Swift / AppKit
Windows → C# / WPF
```

這使每個新的業務功能都可能需要兩套：

- UI
- state binding
- validation
- localization
- tests
- bug fixes

隨著 ACP、Browser、Activity、Permission、Plugin/MCP 管理功能增加，這種模式的維護成本會快速上升。

## 2. 目標

業務 UI 共用：

```text
Vue 3
TypeScript
Vite
Pinia
shared i18n
```

Go Core 與 UI 之間建立明確的 Desktop API，OS-specific 能力保留 thin adapter。

## 3. Framework 候選

### Wails

目前首選 POC。

優點：

- AgentDock 本身是 Go，binding path 最直接。
- Web UI 可使用 Vue 3 / Vite。
- 使用系統 WebView，不需要附帶完整 Chromium。
- 較容易讓 Go Core 與 Desktop shell 維持單一語言邊界。

風險：

- Wails major-version maturity / API stability 需要實測。
- tray、menu、multi-window、streaming、update、signing、installer 必須逐項驗證。
- 不可因 framework 有 API 就假設符合現有 AgentDock 的 service/update security semantics。

### Tauri

作為 fallback / comparison。

優點：

- desktop ecosystem 與 Web UI integration 成熟。
- 系統 WebView、權限模型與 packaging 經驗多。

成本：

- 引入 Rust layer。
- 形成 Vue → Rust → Go Core 的額外 bridge。
- 對目前 Go codebase 會增加維護語言與 build complexity。

### 保持雙 native UI

只作為 migration 期間的 fallback，不建議作為長期方向。

它保留最佳 platform-native integration，但無法解決業務 UI 雙寫問題。

### Electron / Flutter

目前不列為優先 POC。

Electron 增加 Chromium / Node distribution 與 security surface；Flutter 則引入 Dart 與另一套完整 UI ecosystem，對現有 Go + Vue 能力沒有明顯優勢。

## 4. POC 驗收

任何 framework 在正式 migration 前都必須證明：

### Core bridge

- Go method binding
- typed model
- structured error
- async request
- cancellation
- event / stream delivery

### Desktop shell

- main window
- window persistence
- tray
- native menu
- clipboard
- file dialog
- notification
- deep link / callback requirement

### Accessibility

- VoiceOver（macOS）可讀取主要控制項與狀態。
- Narrator（Windows）可讀取主要控制項與狀態。
- 完整 keyboard-only navigation。
- 不允許 focus trap。
- Activity / log 等動態區域需驗證 live-region 行為，不可因高頻事件持續打斷 screen reader。

### Performance

- 先量測目前 native Desktop 的 idle memory / CPU 作為 baseline，再為 WebView shell 設 regression budget。
- 高頻 activity event 不逐筆無限制推入 UI；Desktop API 必須支援 batching / backpressure。
- 壓力測試時 UI 不應因 activity stream 持續堆積而失去互動能力。
- Browser routing / CDP attach 延遲要與目前 native / Core 路徑比較，不能只驗證「功能可用」。
- POC 要留下可重複量測方法；在沒有 baseline 前不先寫死任意 RAM 或 latency 數字。

### Runtime integration

- Core status
- Start / Stop / Restart
- background service state
- safe concurrent-operation handling

### macOS

- background service / login behavior
- TCC / file permissions
- Keychain
- app replacement / update recovery
- signing / notarization
- Universal packaging

### Windows

- 現有 Scheduled Task / Run key / process startup semantics
- privilege / UAC flow
- Credential Manager
- update / rollback
- x64 packaging
- ARM64 只有在 upstream 仍正式支援且成本合理時納入

## 5. Migration 原則

不做 big-bang rewrite。

順序：

1. Desktop API
2. shared shell / navigation
3. Overview
4. Runtime / Connection / Basic Settings / minimal Update & Diagnostics
5. Activity / Execution vertical slice
6. Browser Routing + browser page migration
7. ACP Manager + ACP page migration
8. MCP / Plugin / remaining Settings
9. Permission parity 與 native business view deprecation 評估

舊 AppKit / WPF UI 在 shared UI 達到足夠 parity 前保留。

## 6. 平台 adapter 原則

UI 共用不等於 platform behavior 強行共用。

任何涉及：

- credentials
- service lifecycle
- privilege
- filesystem permission
- code signing
- installer/update transaction

的能力，都由 Go contract + OS adapter 保留平台語意，Shared UI 只呈現結果與操作。

## 7. Framework 決策 Gate

POC 完成後才建立正式 ADR，至少比較：

- implementation complexity
- binary / runtime footprint
- platform API coverage
- update/signing integration
- event/stream reliability
- high-frequency IPC / backpressure behavior
- idle / active memory and CPU relative to current native baseline
- accessibility
- dev experience
- upstream merge impact
- testability

POC 前不把 Wails 寫成不可逆的產品依賴。

## 8. Migration 保護

Shared UI migration 不得遺失目前兩套 native UI 已有的行為與翻譯資產。

正式搬移前應先建立：

- AppKit / WPF feature inventory。
- macOS `.strings` 與 Windows `.resx` 的 key / value migration mapping。
- 既有 platform-specific permission / update / service lifecycle regression tests。

這些資產是 migration input，不應因改 UI framework 而重新手工建立。
