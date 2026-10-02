# Workbench Adoption

> 狀態：Initial accepted decisions
>
> 原則：Workbench 是 feature incubator / reference implementation，不是第二 upstream。

## 決策分類

- **ADOPT**：需求與工作流程高度吻合，值得納入 roadmap。
- **PARTIAL**：採用資料模型、UX semantics 或部分能力，不複製整套。
- **REFERENCE**：只作架構參考。
- **DEFER**：可能有價值，但不是目前優先級。
- **REJECT**：目前明確沒有產品需求。

## ADOPT

### Activity / Call Observability

希望具備：

- root / child call
- command / tool / MCP 類型
- running / completed / failed / cancelled
- duration
- exit code
- bounded output
- file changes
- truthful terminal state

這直接解決長時間 ACP 任務「現在到底在做什麼」的問題。

### Output Continuation

採用 bounded output + continuation，而不是無限制提高 output budget。

需要：

- next segment
- restart from beginning
- copy
- export
- 明確顯示 truncated state

### User Insertion

採用：

- 執行中追加要求
- stable insertion ID
- delivery state
- acknowledgement
- cancel when possible
- bounded retry

### Attention Semantics

收到 insertion 後，Agent 應：

1. 在下一個主要業務動作前確認收到。
2. 簡短回報目前進度。
3. 說明新要求如何影響後續工作。
4. 繼續同一任務。

收到 insertion 本身不代表任務停止，也不要求使用者再輸入「繼續」。

### Truthful Execution State

必須分開：

```text
recent interaction
!=
active execution
```

最近有互動不能被當成仍在執行。

### Implementation Priority

在 Shared Desktop foundation 可用之後，這一組 execution features 應優先於 ACP package CRUD：

1. truthful execution state
2. Activity / call stream
3. bounded output continuation
4. user insertion / ACK
5. attention semantics

原因是高頻 event、stream recovery、UI backpressure 與中途插入是新 Desktop 架構最難的 data-flow 驗證；越早做 vertical slice，越早能判斷 shared UI / Desktop API 是否真的適合 Agent workflow。

## PARTIAL

### Permission Profile

採用「硬上限」概念，特別是：

- filesystem boundary
- network boundary
- workspace boundary

不預設直接複製 Workbench 完整 permission stack。

### Approval Policy

評估：

- allow
- ask
- deny
- category policy

先與官方 AgentDock permission model 對齊，再決定最小擴充。

### Task / Conversation association

對我們有價值的是能把 call / activity 正確歸屬到目前工作，而不是完整複製 Workbench 的 archive / trash / lifecycle UI。

## DEFER

- Approval Reviewer / 自動 reviewer。
- 完整 archive / trash / restore。
- 完整 Workbench task lifecycle。
- Linux online-management UX。
- 完整 Workbench plugin / skill lifecycle UI；等 shared Desktop foundation 後再評估。

## REJECT

目前不採用：

- Android Workbench
- Termux / PRoot
- Android WorkManager / Guardian
- Android SAF
- Android Quick Settings
- Android node deployment
- Workbench branding
- Workbench public status page
- 只為完整產品 parity 而新增的 mobile infrastructure

理由不是功能品質不足，而是與目前實際工作流程無關；加入只會提高 merge、build、release 與 security surface。

## 我們自己的功能優先於 Workbench parity

Workbench 沒有完整解決、但我們明確需要：

### ACP Manager

- UI 以狀態、版本、健康檢查與安全操作為主，不發展成通用 package store。
- adapter discovery
- installed version
- latest version
- update availability
- update
- edit / delete
- enable / disable
- health check
- Codex / Claude / Antigravity presets
- custom adapter 使用同一 model

### Browser Routing

```text
company development
→ persistent profile Edge

normal browser task
→ isolated managed Chrome

explicit override
→ selected profile/browser
```

需要搭配：

- CDP discovery
- connection health
- profile lock detection
- process/session ownership
- stale cleanup
- temporary directory lifecycle
- diagnostics
