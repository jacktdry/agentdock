# Pre-M9 MCP Runtime 生命週期共用規則與重複程序盤點

> 狀態：2026-10-11 唯讀盤點，HIGH／Pre-M9 跨 P4 ACP、P5 MCP、P8 Browser 與 M7 Shared Memory 改善待辦。尚未實作回收或部署；不重新開啟 P4/P5 已完成的 closeout。
>
> 界線：stable AgentDock、Next、Codex Desktop/ACP、AGY、Chrome/Edge、Memory/CBM daemon 與任何使用者 Session **不得因本盤點而被重啟、停止、清除或 kill**。

## 一、現場證據（2026-10-11 約 08:25 +08）

使用 macOS `ps`／`lsof` 唯讀採樣；計數隨時波動，RSS 是程序加總，**不等同獨佔記憶體**。程序多不直接代表沒有 owner／已洩漏／都在執行：

| 家族 | 現況 | 主要 parent | 待證實的風險 |
|---|---|---|---|
| Codebase Memory | **64 個 Client**，另有 **1 個 shared daemon**；Client RSS 約 76 MiB | 62 個 Client 由同一 Codex ACP `codex app-server` 啟動；Codex Desktop／AgentDock 各 1 | Client per-thread 長駐，但不等於 64 個索引 worker；索引工作與 Client 數量必須分開查。 |
| Chrome DevTools | 約 **123–124 個 `chrome-devtools-mcp` server**，另有約 **124 個 npm exec launcher**；兩層 RSS 合計約 710–760 MiB | 121 個 launcher 從 Codex ACP `codex app-server` 啟動，其餘 Codex Desktop／AgentDock | 大量 stateful tool server 與 launcher 配對，selected-page state 不能任意 pool。 |
| `mcp-search-console` | **130 個 Python 程序**，RSS 約 188 MiB | 128 個從同一 Codex ACP `codex app-server` 啟動；Codex Desktop 2 | 第三個可重現的程序倍增類別；尚未證明是否為仍被 loaded threads 持有。 |
| Memory | 約 2 個 Python 服務（shared HTTP 與 legacy stdio 路徑並存） | launchd + AgentDock | 已有刻意保留 stable legacy 的設計，不能直接視為異常。 |
| GitHub／GitLab | AgentDock 下各約 1 個 | AgentDock | 未見同類大量累積，不等於 close/reconnect 壽命測試通過。 |
| AGY adapter | 約 1 個 | AgentDock | 不等於 MCP Server；仍需確認它啟動的工具 child lifecycle。 |

**已證實：**Codebase Memory daemon 日誌曾連續出現 `watcher.changed … strategy=git`、`active_jobs=1`、`index.supervisor.reap outcome=clean`，以及手動 `index_repository` 不同選項衝突。這支持 Git 自動監看和手動索引可能競爭，**並不能推出另一個 ChatGPT Session 正在索引**。2026-10-11 本輪曾成功 `index_repository` 回傳 `indexed`，隨後 `index_status=ready`；先前的更新錯誤也曾有 binary/daemon build mismatch，另見 [CBM lifecycle 紀錄](cbm-lifecycle-conflict.md)。兩個根因不可混為一談。

**未證實：**哪些 PID 已經沒有有效 ACP thread/session，哪些是仍需保留的 persistent loaded threads，是否存在 fork/adapter thread-unload 未釋放，是否能安全共享 `mcp-search-console`。不得用單純 `etime`／CPU／RSS／名字作 orphan 判斷。

## 二、已有能力與責任分界

- **M7 ACP Manager** 已有 `persistent | ephemeral | idle-managed`、prompt terminal auto-close、bounded idle sweeper、Adapter fork close/recycle 方向與 Next-only 20-cycle test。[ACP lifecycle](acp-lifecycle-memory.md)
- **P5 MCP Management** 已有 standalone/plugin ownership、revision/generation、credential/OAuth、bounded reconnect 與 close failure blocker；這次是 **runtime process lifecycle 的跨域 follow-up**，不是重做 P5 UI。[P5 contract](pre-m9-mcp-management-roundtable.md)
- **M6/P8 Browser Broker** 已有 browser lease、4 active + 16 queued、target/page state 隔離與 host-owned worker。無條件共享同一個 raw Chrome DevTools MCP session 會破壞這些保護。[P8 當前決策](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)
- **Go MCP client** 的 `internal/mcp/client/protocol.go` 已具協定 Close、stdio EOF、bounded grace 與 owned process controller；真正待補的是多 ACP host/loaded thread/Plugin 的**統一所有權、capacity、lifetime、可觀測性**，不是再寫一個全機 `pkill`。
- Codex ACP 所啟動的 dedicated app-server 與 Codex Desktop 自己的 app-server 不能混淆；AgentDock Next **不能**直接殺另一個主控程序或它的子程序。

## 三、共用 MCP Resource Lifecycle Contract（建議作為 Pre-M9 實作基線）

### 3.1 統一 owner／lease 模型

新增 **內部抽象** `MCPResourceLease`（不另建第二份 MCP registry）：

`namespace` (stable/next), `server_id`, `provider_kind`, `owner_runtime`, `owner_acp_session`, `owner_tool_call`/`browser_lease`, `generation`, `actual_state`, `lifecycle_policy`, `borrower_count`, `inflight_count`, `last_active_at`, `close_reason`, `last_close_error`。

**實際由 Next 啟動的 stdio process** 綁 OS pid + kernel birth + direct-parent/UID/PGID（Windows Job Object）；跨 restart/reconnect 必須重新證明。僅被觀察的外部 Codex Desktop/CLI owner 分類為 `observed_external`，不給 close/kill 權限。機密（argv/env/token/OAuth URL、工具參數和回覆）不得放入 metrics、logs 或 Desktop UI。

### 3.2 統一狀態機

`registered → starting → ready/leased → draining → closed/reaped`；任何缺乏 ownership 或不明結果轉 `uncertain/quarantine`。

1. **Acquire**：核對 namespace/owner/generation、service capability；相同 owner 可在明確可重用情境 deduplicate 請求，不為每個 `tool/search`/chat turn 啟動新 server。每類有 capacity + bounded queue，不允許無限制 process 增長。
2. **Release**：ephemeral 工作確定 terminal 且 inflight=0 後 close；idle-managed TTL 到期且無 active prompt／待核准／背景 task 時 close；persistent **不因 end_turn 自動 close**。共享 HTTP daemon 只釋放其 client lease，不因單個 owner 沒有使用就關閉 daemon。
3. **Drain**：禁止新工作，等待已接受工具結算，再按 protocol close + stdio EOF；只有確實由此 controller 啟動並握有 kernel birth/ownership 的 exact process tree 才可 bounded graceful termination。Detached child ownership 不明或 side effects/ACK 不確定時 fail closed，保留 `cleanup_required`；**禁止 global pkill、掃名稱、按存活時間 kill**。
4. **Close failure**：報 `closing_failed/uncertain`，不能把已停用設定當成真的 process 已退出，也不能 silent fallback 建立第二個實例。
5. **Config/OAuth/Plugin/Session generation** 變更：禁止舊 owner/lease 以原權限繼續 request，禁止跨 stable/next 混用；不共享不同 token/user/scopes。
6. **Metrics**：每種 MCP 的 managed/external 程序數、loaded/running/idle threads、leased/inflight、RSS、last-used、close error、generation、reconnects、worker/job count；預設不收集敏感 command line。

### 3.3 不同 transport 的必要例外

| MCP 類型 | 可以共用的部分 | 禁止直接共用或推論 |
|---|---|---|
| **stdio MCP** | host-owned registry、同 owner 明示可重用 instance/lease | stdio 是 per connection/transport；未證明 session-isolation/thread-safety 不可跨 ACP owner 共享 stdin/stdout。 |
| **CBM / search 類** | 如 provider 正式支援 shared daemon/HTTP，使用一個 daemon + bounded client leases；同 project+commit+mode/options 自動 coalesce index | 不可將不同 options 的索引合併或覆寫、不繞過 build match、不能 kill 其他 repo worker。 |
| **Chrome DevTools** | Browser Broker 對 context/worker lease 管理、bounded queue | 不可跨 owner 共用 selected-page state、使用者 Edge/Chrome profile，不能搶前景、不能不經授權 CDP。 |
| **remote HTTP/OAuth** | 相同 origin/授權 scope 下可共用 transport pool | 不得混不同 user/token/Plugin owner，也不得 cross-origin forwarding secret。 |
| **Plugin-owned** | Controller 管 process lease，Plugin authority 管設定、憑證與 generation | standalone UI 不可修改／終止 Plugin resource，必須先 drain borrowers。 |
| **Memory shared HTTP** | 本機服務 daemon 可為多 Agent 共用 | 單一 session close 不應關 daemon，也不能動 stable 故意保留的 stdio。 |

## 四、排定實作工作（與 P8 的安全 gate 分開）

### HIGH-0：唯讀 owner inventory + 根因確認（優先）

- 在 Next Diagnostics 彙整 `managed_next`／`observed_external`。由 Adapter 真正回報 loaded thread → stdio server 的 owner mapping，不能從 parent PID 推測是不是 orphan。
- 特別對 Codex ACP app-server 的 **Codebase Memory（62）、DevTools launcher（121）、search-console（128）** 建立 thread/transport/child 對照，觀察 thread close 是否觸發 stdio teardown。
- 確認 `mcp-search-console` 是 stateful stdio 或支援共享 transport，再決定是否能改為 shared service。CBM 的 watcher/manual-index 衝突需比較 project + branch/commit + options 和 worker lifecycle，不把「64 Clients」當「64 indexing jobs」。
- 對各 family 建立 20-cycle baseline、穩定後計數、RSS 趨勢、session close failure；先 **warning only**，不啟用 auto-clean。

### HIGH-1：Next 專用共通 MCP lifecycle controller

- 將 `MCPResourceLease` 對接現有 M7 ACP policy、P5 MCP registry、M8 Permission、P8 Broker；統一 ready/borrowed/inflight/draining/uncertain、inflight-close fence、bounded concurrency/reconnect budget。
- 優先處理 Adapter-owned dedicated Codex app-server：支援正確的 thread unload/close，或最後一個 managed ACP Session close 後安全 recycle adapter-owned app-server；**不得**對 ChatGPT/Codex Desktop 所有的共享 app-server 施作 kill。
- CBM 優先沿用既有 shared daemon，避免多重 per-thread client；DevTools 必須走 Browser Broker stateful leased worker；search-console 先證明協定特性，再決定是否可 pool。

### HIGH-2：安全 UI 與驗收

- 顯示 owner/namespace、process/transport 狀態、lease 數、last active/close error、是否擁有操作權；提供 **Next-owned** graceful close、drain/retry 指示，不提供「清理全部 MCP」按鈕。
- 20 次 sequential ephemeral、20 次 close/resume、4–8 parallel、persistent 長駐、HTTP Memory shared、不同 OAuth/Plugin revocation、DevTools 4 active/16 queued、launcher/child/grandchild ownership、macOS + Windows native tests。
- **DoD**：自有 ephemeral Session close 後，owned descendants 在 bounded grace/30 秒觀察窗回到測試 baseline，或準確標示 `cleanup_required`；20-cycle 不單調累積；persistent 和 external client 不得誤關；重連不 duplicate active server；任何 unknown side effects/OS identity 保持 fail-closed，秘密不外洩。

### 對目前 live 系統的約束

本案是**新增改進計畫，不是授權清理現有 PID**。禁止對原版 AgentDock、已安裝 Next、Codex ACP/ChatGPT Desktop、GitHub/GitLab、Chrome/Edge 或 CBM shared daemon 進行 live restart/kill/disable。P8 `31 ACK 與 WAL 尚是 source-only prototype，也不能因這項規則解除 Browser sticky marker 或啟用 auto restart。
