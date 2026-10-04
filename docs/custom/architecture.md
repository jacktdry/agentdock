# AgentDock Custom Architecture

> 狀態：Accepted baseline
>
> Primary upstream：`uvwt/agentdock`
>
> Feature reference：`A-m-o-r-F-a-t-i/AgentDock-Workbench`

## 1. 定位

AgentDock Custom 是官方 AgentDock 的 workflow-oriented 客製版本，不是 AgentDock Workbench 的重新包裝版本。客製開發產品名為 **AgentDock Next**；沿用 upstream Core 架構，但以獨立 app identity 與 runtime/state namespace 執行。

我們保留官方 Core、Tool、MCP、Plugin、ACP、Browser 與平台 runtime 的演進能力，並在 `custom/main` 上加入只對實際工作流程有明確收益的功能。

優先改善的問題：

- ACP adapter / package 的偵測、版本、更新、編輯與移除。
- Browser Broker 根據 workspace、認證需求與並行情境分配 browser lease；company workspace 強制使用使用者既有 Microsoft Edge authenticated profile，非公司工作則預設由 `chrome-devtools-mcp` 使用 AgentDock-managed isolated/headless Chrome。無論上游 ChatGPT 或 ACP 都不得各自繞過 Broker 持有另一套 browser backend。
- Computer Use 只作 native GUI fallback；無論上游 ChatGPT 自己需要 GUI 操作或 ACP delegated Computer Use，都預設經 AgentDock Computer Control Broker 使用 Orca，並禁止 foreground control 靜默搶走使用者焦點。
- 長時間 Agent / ACP 任務的執行狀態、call tree、output 與 file changes 可視性。
- 任務執行中追加要求、ACK、有限重送與不中斷原任務的 attention semantics。
- macOS / Windows Desktop 不再重複實作同一套業務 UI。
- 多國語系成為 repo-level 可貢獻資產，不依附特定 Desktop framework。

### Production control plane / Next development plane（2026-10-05）

目前連線中的 AgentDock 是 ChatGPT Mac-Dev 的 production control plane。先前 live activation 嘗試造成 control channel 斷線，因此禁止在 Next 開發中檢查、修改、重啟、停止、替換 stable `AgentDock.app` / Core，或操作 `~/.agentdock`、stable Memory registry 與 live launchd services；stable 不得作為 development target。

```text
ChatGPT Mac-Dev → stable AgentDock → stable Core :8765 / stable state
ChatGPT mac-dev-next (future) → AgentDock Next → Next Core :8767 / Next state
                                                 │
                                                 └─ shared Memory HTTP :8766/mcp
                                                    protocol 2025-11-25
```

兩個 plane 不共用 lifecycle authority、registry/config、runtime state、tunnel identity 或 update target。Next Memory registry 位於 `~/.agentdock-next`；共用 Memory HTTP endpoint 不代表可以修改 stable stdio registry 或接管 shared daemon lifecycle。Shared Desktop Runtime / Connection / Settings / Update / Diagnostics 也必須只指向 Next，不能自動發現並控制 stable。

隔離 namespace、installer fail-closed 契約與驗收以 [agentdock-next-isolation.md](agentdock-next-isolation.md) 為準。M7.5 Phase 1 repository identity/runtime isolation 已於 `architecture/agentdock-next-isolation` 的 `b189ee5` 完成：Next bundle/service labels、runtime/state/log/work roots、8767、Go Darwin runtime variant、legacy migration boundary 與 packaging metadata 已具備 fail-closed contract。Phase 2 self-update / updateplatform / arbiter repository target isolation 亦已完成：closed updater identity、Next-bound transaction paths、persisted source signer、pre-mutation rollback/recovery 驗證；Next 只用 arbiter swap，禁止 legacy standalone/new/backup fallback。GUI updater gate 保留，Next-only signed package/live/Memory/connector 驗證仍待完成。M7 code/stress 在 `d8acb9be` 完成，stable cutover 延後至 Next 完整測試並獨立連線 ChatGPT 後的未來 migration；更名屬 packaging / identity migration，不得因此提早共用 state。

## 2. 上游關係

```text
uvwt/agentdock
      │
      │ primary upstream
      ▼
jacktdry/agentdock:main
      │
      │ upstream sync
      ▼
jacktdry/agentdock:custom/main
      │
      ├─ architecture/*
      ├─ feature/*
      ├─ custom-fix/*
      └─ port/workbench-*

A-m-o-r-F-a-t-i/AgentDock-Workbench
      │
      └─ reference / selective port only
```

Workbench 的新能力先進入評估流程，不直接進入我們的 codebase。

## 3. 目標 Desktop 架構

目前官方 Desktop 將 macOS 與 Windows 分別實作為 Swift/AppKit 與 C#/WPF。這對少量設定頁尚可，但 ACP Manager、Browser Routing、Activity Center、Permission Center 等功能持續增加後，雙平台重複實作會成為主要維護成本。

長期目標：

```text
                    AgentDock Go Core
                           │
                    Shared Desktop API
                           │
             ┌─────────────┴─────────────┐
             │                           │
      Platform Adapters              Shared UI
             │                           │
     ┌───────┴────────┐           Vue 3 + TypeScript
     │                │            Vite + Pinia
   macOS            Windows          shared i18n
```

Shared UI 負責：

- Runtime overview
- Connection settings
- ACP management
- Browser Broker / CDP registry / lease settings
- Computer Control provider / foreground policy
- MCP / Plugin management
- Activity / Call Center
- Task / Conversation presentation
- Permission configuration
- Language settings
- Update / diagnostics presentation

Platform adapter 只保留真正 OS-specific 的能力，例如：

- macOS：Keychain、TCC、SMAppService / login item、notarization/update integration。
- Windows：Credential Manager、Scheduled Task / Run key / process lifecycle、UAC、installer/update integration。
- 兩平台各自的 native notification、file dialog 與必要 shell integration。

目標不是消除所有 platform code，而是消除同一個業務功能的重複 UI 與重複狀態機。

## 4. Desktop API 邊界

Shared UI 不應直接依賴大量 `internal/*` 實作細節。預計建立穩定的 Desktop-oriented contract：

```text
desktopapi/
├─ runtime
├─ connection
├─ acp
├─ browser
├─ activity
├─ permission
├─ mcp
├─ plugin
├─ update
└─ diagnostics
```

這層應維持：

- typed request / response
- stable error envelopes
- event / stream contract
- cancellable long-running operations
- Core 為 authoritative state，UI 不建立平行 truth

## 5. 來源功能的採用原則

### 官方 AgentDock

預設採用。若官方提供等價能力，優先使用官方實作而不是維護我們自己的版本。

### Workbench

先回答：

1. 我們是否真的有這個 workflow pain point？
2. 官方 AgentDock 是否已有相同或更好的能力？
3. 是否需要完整功能，還是只需要其中的資料模型 / UX semantics？
4. 是否深度依賴 Workbench 自己的管理 API 或產品架構？
5. selective port 後的維護成本是否合理？

只有通過這些條件才進 `port/workbench-*`。

## 6. Non-goals

目前不追求：

- Android Workbench。
- Termux / PRoot node management。
- Linux Desktop GUI。
- Workbench 品牌與 status page。
- 完整複製 Workbench 的 lifecycle / archive / mobile infrastructure。
- 為了 cross-platform 而繞過各 OS 的安全與權限模型。

## 7. 架構成功條件

完成後應達成：

- 官方 `main` 更新仍能低成本整合。
- Desktop 業務 UI 新功能只寫一次。
- OS-specific code 限制在清楚的 adapter 邊界。
- Workbench 更新可以逐項評估而不是整包同步。
- i18n 可由非程式貢獻者獨立維護。
- 我們的 ACP / Browser / Activity 能力不要求 fork 官方 Core 的大量內部邏輯。
