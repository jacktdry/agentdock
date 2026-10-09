# PR #229 — Task Progress 單一卡片即時更新：Next 採用評估

> Status: **SELECTIVE ADOPT / implementation deferred**，評估時間 2026-10-09。上游 PR 尚為 `open`、非已合併 release。這份文件是設計決策，不是移植或本機 UI UAT 證明。
>
> Source: <https://github.com/uvwt/agentdock/pull/229>，`handsomedogx/agentdock:feat/task-live-card-v1.0.1`，head `3352c0a2b5c5165aca37966c8d5043c246e5bfe1`。16 檔，+396/-42；作者聲稱 `go test ./...` 和 `make check` PASS、完成 3-step 手動測試；2026-10-09 查詢時無正式審查，不能把作者驗證當成 Next UAT。

## 目標與現況

`task_manage action=create` 初次回傳 Task Progress MCP App，但後續 checkpoint/final_review/complete 的新 MCP Tool Result 不會更新原卡片，因此狀態容易過期。PR 讓 Card 透過持久化的 `task_id` 使用同一張 UI 輪詢 `task_snapshot`，任務 active 時 2 秒、blocked 10 秒，視窗隱藏時暫停、completed/teardown 時停止。只建立動作綁 UI，其餘動作不再額外掛同一張 Task 卡片。

Next 的 `internal/tool/task` 持久化 Task 已存在；`internal/app/specs_task.go` 現只有 `task_manage`；`internal/app/mcp_apps.go` 仍把 Task UI 綁到 `task_manage`；`internal/mcp/apps.go` 使用 `mcpapps.HTML("task_progress", "Task")`。此 PR 是 **MCP App/Task 體驗補強**，不是新的 Task Store、也不是 P8 Edge Browser 安全邊界的一部分，不應重做 Execution Center、Task State 或 M8 Permission。

## 檔案層級採用建議

| 上游實作 | Next 採用方式 | 理由 |
| --- | --- | --- |
| `internal/app/specs_task.go`, `internal/tool/task/request.go`, `contract.go`, `task_actions.go` | **ADOPT** `task_create` + `task_snapshot`，保留 `task_manage action=create` 舊路徑 | 新增/唯讀工具合約；`task_summary` 直接從權威 Task State 讀取 |
| `internal/app/mcp_apps.go`, `internal/mcp/server.go` | **ADOPT WITH REVIEW** 只由 create 掛 UI、其他 Action 不重複掛 | 避免歷史訊息灌滿過時卡片；確認 full/compact 都一致 |
| `internal/mcp/apps.go` 的 `taskProgressRefreshInjection` | **REIMPLEMENT/REVIEW**：保留相同 Lifecycle，盡量不要依賴脆弱字串 Marker HTML Injection | 對 protocol HTML 字串位置／版本敏感；需檢查 CSP、bridge handshake、resource cache、host 是否允許 App tools/call |
| 上游 `UIVisibility: ["app"]` | **REVIEW SECURITY** | UI metadata 不是服務端 ACL，Task snapshots 仍須經 Core 授權及 scope；不得因標示 app-only 就假定任意連線無法呼叫 |
| 上游 `task_snapshot` 含 `state_dir` | **OMIT FROM CARD**（若不必要） | 本機絕對路徑對前端無業務價值，可能洩漏 Host 目錄位置；僅傳必要的 task summary |

## 安全與操作必要條件

1. **權限與 Session/Workspace 隔離**：不可只依賴猜不到 `task_id`；同一 MCP Core 多連線或 Nexus delegation 下需驗證 Task 讀取範圍；snapshot 只返回顯示需要的截斷欄位，不能讀回敏感 workflow guidance/私密環境或 `state_dir`。
2. **狀態唯一來源**：卡片不得自行推進、寫入或偽造 completion；只讀持久化 Task Store。create/legacy create 雙路徑，Task ID 穩定且卡片只掛一次。斷線時顯示上次觀測時間及恢復嘗試，不能默默顯示假完成。
3. **資源上限**：允許一張任務卡片一個 in-flight poll；對多卡、斷線、host hide/restore、blocked 狀態設 bounded concurrency、backoff/jitter。不可因 2 秒輪詢拖垮 Next Core／MCP 控制面；completed/teardown 應真正釋放 timer。
4. **MCP App 行為**：確認 host 接受 `ui/notifications/tool-result` 與 `tools/call`，full/compact 均實測。不能只憑 Go 原始碼或 fixture 保證 ChatGPT App 卡片真的原地更新。
5. **快取與發行**：PR 作者已指出 Plugin registration metadata 可保持舊版；僅開新對話可能無法更新。Next 必須檢驗資源 version/cache refresh、重新連線與明確無破壞刷新策略，不可為測試此功能移除 stable `mac-dev` connector。
6. **向後相容**：既有 `task_manage` create/list/checkpoint/final_review/complete 不變，MCP Apps=off 工具仍可正常操作；需要保留舊 client（未重新載入工具 metadata）的行為。

## 驗收矩陣

| Case | 測試 | PASS |
| --- | --- | --- |
| T01 | 新 `task_create` 與舊 `task_manage action=create` | 都能建立且只顯示一張卡片 |
| T02 | 3-step checkpoint → final_review → complete | 同一張卡片顯示權威進度與完成狀態、無重複卡 |
| T03 | blocked → resume、負載延遲與後端拒絕 | 不偽報進度；恰當 backoff、恢復後追上 |
| T04 | hidden/visible、resource teardown、Core/MCP 重連 | timer 不洩漏、無過期 callback 覆蓋新任務 |
| T05 | 多卡同時輪詢與取消 | bounded QPS/in-flight；不影響 ACP/Browser MCP 處理延遲 |
| T06 | forged/foreign task_id、跨 session/tenant、UI token 遺失 | fail closed，不洩露任務細節或 state_dir |
| T07 | MCP Apps full/compact/off & 既有舊 metadata | 兼容且不重複掛 UI，off 仍可用 Task tools |
| T08 | 真實 ChatGPT Next connector 的 App Card 與快取更新 | 視覺 UAT 更新成功；stable `mac-dev` 不受影響 |

## 優先級與工作邊界

**P1 可獨立安排的 UX 改進**，不應阻塞 B2i/B2j/B2k 或 P8 security gate；如果整合資源有限，可排在 P8 安全閉環與 AGY `.14` 整合後、M9 發行凍結前決定是否納入；若無原生 UI 證據則移到 release 後，不以其阻擋 M9。實作需使用 Next 專用獨立 worktree，不碰另一個 session 的 Browser Broker 檔案，也不重裝正在運作的 stable AgentDock。
