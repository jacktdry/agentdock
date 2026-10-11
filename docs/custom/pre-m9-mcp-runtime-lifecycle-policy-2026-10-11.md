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

#### HIGH-0 第一個安全診斷 checkpoint（source-only）

macOS opt-in CLI：`go run ./cmd/agentdock-next-mcp-owner-audit --root-pid <已確認的 Codex app-server 或 adapter PID> --max-nodes 1024 --max-depth 16 --timeout 10s`。必須明示 root PID；不自動找程序、不接入 live adapter、不關閉 Session、不提供 process-control/action flags。JSON 僅送 stdout，預設不寫檔；Linux／Windows 回報 `platform_unavailable`（exit 1），參數錯誤 exit 2，`--help` 回傳 JSON 說明。CLI 不驗證 root 是 Codex 或屬於 Next，操作者必須先確認其來源。

唯讀固定容量 Darwin kernel metadata table（上限 16,384 rows，單次 syscall）搭配既有 `process.ObserveBirthIdentity`：採樣前確認 root，採樣後重新比對所有輸出 PID 的 kernel birth、PPID、UID、PGID，最後重新確認 root。node/depth/deadline 或任何採樣／identity 錯誤皆 fail closed，無部分 nodes/counts。截止時間不取消已在執行的 kernel call，但不再發布其結果；沒有 shell、argv/env、路徑、RSS、Session/thread ID 或原始 OS 錯誤輸出。

成功只證明**非原子時間窗內觀察到的 PID/PPID 祖先關係**，所有 owner 固定 `owner_unverified`。已知 MCP family 計數排除 root，僅是 kernel executable basename 的 allowlist 名稱提示；basename 可能截斷，Node／Python／npm launcher 無法辨識其 MCP payload，仍為 unknown，因此 0 不是不存在，client 也不是 indexing job。缺失 parent、已 reparent/detach 的程序無法歸因；後續 identity 仍可能改變。不得推論 native thread/session、transport、managed_next/external authority、健康、orphan 或清理資格。Browser／Computer host `ReleaseSession` 不擁有 Codex app-server stdio children。下一 gate 仍是 adapter/native instrumenting 真實的 `Session → native thread → stdio transport → PID/birth` 映射；本 checkpoint 不改變第五節第一批 close probe 或停止擴大的結論。

#### HIGH-0 第二個安全診斷 checkpoint（source-only，未接通）

`internal/mcpowneraudit/correlation.go` 新增套件私有、程序內的 typed evidence contract；**沒有 production caller、可信 Adapter diagnostic 實作或 kernel pipe observer，也沒有部署 native ownership**。CLI schema／行為保持第一個 checkpoint；不新增 CLI／HTTP／ACP 輸入、registry、controller、任何 cleanup 或 close/kill 權限。

- Managed mapping 僅保留 AgentDock record 的 local ID／remote native thread ID 與 adapter incarnation；不收集 title、cwd、argv、env、URL。只有 pinned Adapter 明確保證 remote ID 就是 native thread ID 才可填入，persisted／ready record 或標準 `session/list` **不證明 loaded**。
- 每次新 256-bit challenge、固定 Next/Codex/stdio scope、相同 incarnation 與 adapter/native PID＋kernel birth，要求完整 loaded-thread／transport-child snapshot 與獨立完整 kernel pipe snapshot；回應須在本次 request 起算 5 秒內完成採樣及發布。每類上限 1,024 rows、ID 上限 128 bytes，duplicate／conflict、missing／partial、stale／future／replay 一律無部分關聯輸出。
- 重用既有唯讀 kernel collector 的 birth／PPID／UID／PGID 重查；要求 native 是 pinned adapter 的 exact direct child，transport child 是 native 的 exact direct child，且 adapter 與 kernel 觀察的 stdin/stdout pipe object identities 完全一致。此 slice 刻意不推論 npm launcher 孫程序或 shared/reparented transport；那些拓樸在額外證據契約完成前不合格。
- 正向 fixture 只得到 `verified_diagnostic_correlation`、mapping 數及 correlated PID；owner **仍為 `owner_unverified`**。不代表 Next-owned、健康、orphan、授權或可終止；kernel 採樣仍非原子。所有證據 input 欄位私有，普通 JSON 不輸出 raw local/thread/transport ID 或 arbitrary executable name；失敗僅 `not_qualified`，不回傳來源錯誤。

**Live integration gate：**未來需 Adapter-owned、已驗證 peer 的 diagnostic extension，在 AgentDock pinned adapter incarnation 下提供完整 loaded-thread→stdio transport→child PID/birth 與 pipe identity；由 Manager read lock 擷取 record mapping，generation fence 必須涵蓋採樣直到發布。另一個 OS 唯讀 observer 必須證明 native 與 child 分別持有同一 stdin/stdout pipe 的相對 endpoint，且 transport exclusive binding 完整可核對；不能以 FD 數字、名稱、parent PID 或 peer 自報 `authenticated=true` 代替。私有 interface 只是可信整合邊界，**不是 authentication 實作**。今日標準 ACP 不暴露 `thread/loaded/list`／`mcpServerStatus/list`，既有 kernel table 也沒有 pipe ownership；不得直接共用 live app-server stdio。任何 gate 缺失保持未接通、不合格，無 automatic cleanup。

Deterministic fixtures 涵蓋 positive、unloaded／unmanaged、generation／birth／reparent／pipe mismatch、重播、容量、duplicate／partial、取消與 redaction，並以共享 immutable fixtures 驗證並行唯讀。此 checkpoint 不變更第五節停止擴大 Session close 的結論。

本 slice 本機驗證：`gofmt`、`go test ./internal/mcpowneraudit ./cmd/agentdock-next-mcp-owner-audit`、`go test -race ./internal/mcpowneraudit`、`go vet ./internal/mcpowneraudit ./cmd/agentdock-next-mcp-owner-audit` 與 `git diff --check` 通過。兩個套件分別在 Linux／Windows `CGO_ENABLED=0 GOARCH=amd64 go test -c` 編譯通過，未執行跨平台 binary；既有 macOS kernel self-test 在 sandbox 受限，外部重跑通過。無 live Adapter integration／native ownership UAT，最終整合、測試與 commit 由 orchestrator 負責。

#### HIGH-0 第三個安全診斷 checkpoint：Darwin kernel stdio Pipe 觀測器（source-only，未接通）

- `internal/mcpowneraudit/pipe_observer_darwin_cgo.go` 新增 **macOS + cgo 限定**、套件私有的唯讀 `darwinKernelPipeObserver`，以系統 `libproc` 的 `PROC_PIDLISTFDS`／`PROC_PIDFDPIPEINFO` 擷取 kernel `pipe_handle` 和 `pipe_peerhandle`；不使用 `ps`、shell、argv、路徑、環境或工具參數，亦不新增 CLI、HTTP、ACP、程序控制及任何現有 AgentDock/Next runtime wiring。其他平台與 `CGO_ENABLED=0` 僅回 unavailable。
- 檢查 Adapter/native/child PID、PPID、UID、kernel birth，且 child **必須是 native direct child**。子程序 FD 0（stdin）及 FD 1（stdout）必須是兩條不同的 pipe，native 已開啟 FD 表中各自恰有一個相反 handle/peerhandle 端點。拒絕同端持有、別名 FD、重複 child、缺失 pipe、子程序與父程序相同端點、變更的 process birth 與前後 FD snapshot；**沒有部分證據輸出**。
- 每次完整觀察固定最多 **1,024 children／每程序 4,096 FDs**，要求同一個本次 challenge、incarnation、五秒內完整採樣，拒絕容量／時間／權限／任一 FD 消失或無法讀取，不會改採不完整的部分集合。Native snapshot 由呼叫端提交完整的 child 列表；**任何超量都不會被當作「全數已查」**。macOS kernel handle 僅作為短時取樣內的 object hint，非長期權限 token。
- 測試專用 Go helper 建立實際 adapter(test) → native(helper) → child(helper) 的 stdin/stdout pipes：正向 native kernel object identity、身分篡改、重複／空／超限子程序、future／expired／cancelled，以及子程序 stdout 由 test parent 而非 native 持有的**實際錯誤 Pipe 拓樸**均受驗證；重複 FD snapshot 假資料也必須 fail closed。只使用測試自行建立的程序，未碰使用者 live app-server、其他 ACP Session 或 MCP daemon。
- 此項僅替上一個 typed contract 提供 **可信 OS 端實作候選**，仍非 live authorization：尚缺獨立經 Adapter 驗證的 `native loaded thread → MCP transport → child` 完整診斷擴充、實際協定 handshake/provenance、Manager generation/read-lock fencing，以及「所有 FD/transport 的獨佔性與缺失拓樸」證明。即便 `verified_diagnostic_correlation`，owner 仍固定 `owner_unverified`，不得 kill、close、quarantine 真實程序或判定 orphan。此觀測器也不會證明其他 PID 沒有複製 pipe descriptor，外部共享 daemon／grandchild／reparent 路徑均須另行驗證；符合不了一律不合格。

#### HIGH-0 第四個安全診斷 checkpoint：ACP Manager read-lock 世代隔離（source-only）

- `internal/acp/mcp_owner_diagnostic_scope.go` 新增**套件私有且沒有 production caller** 的 `withMCPDiagnosticScope`，以同一個 `Manager.mu.RLock` 覆蓋本機已載入 Session 的取樣、未來可信證據回呼與完成前驗證。期間 `session/close`、resume/rebind/loaded map 更新及 Manager 的 Adapter 替換無法在鎖內更改映射；呼叫端 callback 必須是**內部可信、同步、唯讀、遵循 context**，不得遞迴請求寫鎖或進行新 MCP 工具工作。五秒 deadline 為**合作式取消**，若 callback 不遵循 context，鎖仍可能遭長時間占用，因此 production wiring 之前必須先有獨立的有界 Adapter 診斷實作。
- 讀取的只有 `m.loaded`（**Manager local cache，不能當成 native loaded thread**）與對應 `SessionRecord.ID / RemoteSessionID`。逐筆核對 `remoteToLocal` 反向映射、重複 remote ID、session 狀態、正在 close/delete 的 transition、remote response mismatch，最多 1,024 筆；排除純 persisted/ready 但未在本機載入的 Session。只將 opaque ID 在 callback 的 process-local snapshot 暫時傳遞，不記錄 title、cwd、tool argument、credential 或 transcript；回呼錯誤永遠固定 `not qualified`（不暴露回呼的原始錯誤或部分映射）。
- Adapter 身分必須是 Manager 目前握有的 `exec.Cmd.Process.Pid`，不能只要 PID 存在就宣稱是自己的 Adapter；kernel PID/birth/UID/PPID/PGID 在回呼前後重查，ACP transport 需仍存活，任何取消、程式／session 不一致或連線中斷均 fail closed。**這只證明該時間窗 Manager snapshot 未變且其 Adapter PID 身分仍符合，不能證明 RemoteSessionID 就是 Codex native Thread ID，更不能從 PID／PPID 推導 MCP owner。**
- 用 Repo 原有 test-only ACP helper 完成合法取樣、純 persisted 不誤列、11 種錯誤 mapping/PID/transition/容量拒絕、同步回呼錯誤去識別化、取消/逾時與 adapter transport 關閉、並行 Manager 寫入被 fence 的回歸測試。沒有新增 operator endpoint、實際 Adapter native diagnostics、`mcpowneraudit` 連接、production Session close/kill/recycle 或授權 UI。
- 下一個真正 HIGH-0 blocker **不變**：只有 pinned Codex ACP Adapter 的可信協定擴充、可證明 native loaded Thread 與 stdio child 完整對應、管理員授權與跨 Manager/Adapter 的封裝邊界一致，才可將此 Manager scope 與 Darwin Pipe observer 接上；目前的兩個候選 source-only component 不是完整活體歸因系統。除非能保證 complete + exclusive mapping（包括其他 FD 複本與 grandchild），所有結果固定 `owner_unverified`。

#### 2026-10-11 HIGH-0 原始碼核對與唯讀實測

- AgentDock `internal/acp/sessions.go` 對 `session/close` 收到確認後清除本地 loaded/projection，再呼叫 `releaseSessionMCP`。現行 `internal/app/acp_browser.go` 的 Host MCP provider 只釋放 AgentDock 自身 Browser／Computer HTTP token/lease，**不持有** Codex app-server 自行啟動的 stdio MCP 子程序。
- 本機安裝的 `@agentclientprotocol/codex-acp@2.1.1`（`dist/index.js`）之 `closeSession` 會呼叫 `thread/unsubscribe`、清理 adapter 的 session-local bookkeeping，再呼叫 `recycleAppServerIfIdle()`。原始碼註解明確說明已 unsubscribe 的 Thread 與 stdio MCP 會保留一段 grace period；只有 adapter `sessions.size === 0` 且具有有效 app-server process/state，才安全 recycle **該 Adapter 擁有的 dedicated app-server**。因此關閉五個 Session 後 MCP 數量不變，**與已知設計一致，但不能證明各 PID 目前的 native Thread owner、是否仍活躍或有無洩漏**。不得推論可以 recycle 其他 owner（Codex Desktop/ChatGPT）的 app-server。
- 新 CLI 實際唯讀觀察 PID `55205`（當次 Codex ACP dedicated app-server）所得：`status=observed`、**959 個當次 PID/PPID 觀察節點**（包含 root）、`codebase_memory` 名稱提示 63 個；`chrome_devtools` / `search_console` 名稱提示為 0 **不代表沒有**，因 macOS 的 kernel `p_comm` 截短及 Node/Python/npm 啟動器名稱無法識別原始 MCP family。所有節點固定為 `owner_unverified`，無 cleanup eligibility。數據僅是這次時間窗的樣本，不作穩態 SLA。
- 下一個 HIGH-0 gate 仍是**真實** `managed ACP Session ↔ Codex native Thread ↔ MCP transport ↔ OS PID/birth` 對照；需 Adapter/native 額外提供 loaded/unsubscribe/transport-close 事件或等價可驗證介面。此 CLI 不支持按 age/CPU/RSS 清理或推斷 orphan。

#### 2026-10-11 P0 Gate 重新核對（不可將 Source-only 視為完成）

**Native Codex 協定核對：**目前本機 Codex ACP fork `fix/session-lifecycle-recycle`（`630d7c6`）與已安裝 `@agentclientprotocol/codex-acp@2.1.1` 的 native app-server method `thread/loaded/list` 只回已載入的 `threadId` 集合與分頁；`mcpServerStatus/list` 雖回 server 名稱、runtime status、插件資訊與能力，**不包含該 stdio MCP child 的 OS PID／完整傳輸端點或 per-thread 可靠的 child 關聯**。`session/close` 的 `thread/unsubscribe` 及最後 ACP 本機 Session 歸零時的 dedicated app-server recycle 已存在（`recycleAppServerIfIdle`），不能把這個緩解作法當成「所有持久 Session 的子程序都已回收」，也不能跨到 ChatGPT/Codex Desktop 所擁有的另一個 app-server。沒有上述原生證據，僅用 FD、PPID、時間差或 server name 逆推 thread→PID 將誤導 orphan 判斷，**HIGH-0 未過 gate**。

**HIGH-1 Next 自有 stdio Source-only slice：**新增 `internal/mcp/client/next_lifecycle.go` 的 `NextStdioLeaseController`，沒有接入正式 Runtime、P5 Registry、M8 Admission 或 Desktop API，**只能管理經本 Controller 新啟動且 Mac Darwin kernel birth/parent/UID 驗證的直接 child**。使用既有 `OpenStdioSessionWithStartedWitness`／Go MCP SDK、`StdioSession.Close` 和 owned `process.Controller`，不建立第二份永久 MCP registry，也不接入／終止 Codex/AGY/ChatGPT/Chrome/Edge/CBM 的外部 process。

- `namespace=next`、per-session `owner+server`、Registry generation + 整份 config identity、crypto borrower token、最多 1–32 個 starting/ready 總容量。重複同 owner 相同 config 可 borrow 同一 stdio，**不同 owner 從不共用 stdio pipe**；超量 fail fast，不在背景無限重試。
- `ephemeral` 最後 release 在鎖內原子切到 `draining`，等待 `inflight=0` 再 close；`persistent` 保持 ready，`idle-managed` 在明確 TTL 且沒有 borrower/inflight 才從單次 `ReapIdle` 清理。變動的 config/generation、invalid/Plugin/HTTP/external owner、重複 token、drain 期間新 request 都 fail closed。**不提供 global cleanup / kill API**。
- 真實 native fork helper 測試 20 個 sequential ephemeral 建立／發工具／EOF+Wait，以及 20 個 persistent close/open cycles、8 個平行競逐上限 4、borrow、generation mismatch、drain fence、idle TTL、失敗握手 child quarantine。發生已觀測 child 卻未確認成功 cleanup 時保留 `cleanup_required` slot，防止 retry 重啟第二個可能仍在執行的 child。其他 OS `ObserveBirthIdentity` 不提供可驗證 child 身分，因而採 fail closed（Linux/Windows 暫不開啟此 Next-only 原型）。
- `Snapshot` 與 `ResourceSnapshot` 是僅有固定欄位的 Next-only read-only process-local DTO（owner local Session ID、server ID、policy、lease/borrow/inflight/status/counts），不輸出 argv/env/secret/token/tool content/kernel PID；**不是已連接使用者 UI**。它不能宣稱 observed_external owner；未實作任何資料庫、P5 ACL 或 Next UI 寫入入口。

**P5 Registry 可信讀取閘門（下一個 source-only checkpoint）：**`internal/mcp/client/next_lifecycle_registry.go` 的 `NextStdioRegistryGate` 不接受使用者提供的 `ServerConfig`／argv/env 作為啟動權限；每次建立均向**真實 P5 `Manager.Registry()`（磁碟 Registry + Plugin ownership overlay）**讀取並核對呼叫者事先持有的完整 `Revision` + server `Generation`，限定 `Enabled=true`、`SourceType=standalone`、stdio、有明確 command；透過原有 `Manager.runtimeConfig` 讀取 MCP 的 scoped environment。Plugin、HTTP、停用、revision 或 generation 過期，以及相同名稱刪除重建一律拒絕。

- 建立 MCP 前／完成 handshake 後重新核對同一份 Registry incarnation 與環境，**若中途撤銷權限**，只能撤回自身剛啟動的 Lease：標成 `cleanup_required`、停止新 borrower/tool、使用原 `StdioSession.Close` 的 owned EOF/Wait 回收；callback 實測在 handshake 成功後立即 Disable 仍未發布有效 lease，也沒有重複啟動。工具執行前後也重新驗證；失效時不發布 reply 並標記 quarantine。**工具已派發後才看到 revocation 無法逆轉可能的副作用**；這不等同跨程序原子權限 fence 或可靠 M8 authorization。
- 使用 test-owned 真實 P5 private registry + 真實 MCP echo 子程序進行 20 次 ephemeral、8 個同 owner 競逐、HTTP/disable/generation/config revocation、啟動完成與發佈之間的精確競態；Legacy P5 Manager/Registry 沒有被改寫或重建第二份 registry。此閘門未在正式 Runtime 或 M8 Gate 注入，**不得將 Source-only Gate 作為允許執行工具的 operator API**。跨程序 Registry/Env 變更需要在下一個整合階段用 P5 state lock + M8 atomic admission / dispatch fence 再解決。

**尚未完成的正式 P0 驗收：**可信 Adapter 原生 `loaded thread → MCP stdio transport → child PID/birth` 擴充，跨 Manager 與 Adapter PID/incarnation 的真實端到端 attestation、全部 native transport exclusivity，對現行 62/121/128 家族的 20-cycle baseline；Registry/Plugin/OAuth/M8 權限與 Next Supervisor/Broker 的授權接線、Windows native owner fence、Next UI 及未來 live Next-only UAT。**P0 仍為 blocked/in progress；不准啟用 auto-clean。**

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

## 五、2026-10-11 09:36 第一批人工 Session Close 檢查點（已執行）

依本輪使用者明確授權，透過現行 AgentDock Codex ACP **正式 `acp_session action=close`**，選取 2026-09-02～03 最後更新、`status=ready`、`last_stop_reason=end_turn` 的 **5 個唯讀審查型 managed Session** 做第一批正常 close。事先用 `acp_session inspect` 確認；5 個回應全數 `status=closed`。**沒有 `session/delete`、kill、restart、sign-out 或修改 adapter app-server/MCP daemon。**

| 標準化程序／Session 計數 | 關閉前 | 關閉後 | 變化 |
|---|---:|---:|---:|
| Codex ACP managed ready | 294 | 289 | -5 |
| Codex ACP managed closed | 146 | 151 | +5 |
| CBM Client + daemon | 65 | 65 | 0 |
| Chrome DevTools npm launcher | 124 | 124 | 0 |
| Chrome DevTools server | 124 | 124 | 0 |
| Python MCP/Memory | 132 | 132 | 0 |

**結果：五個 ACP Session 邏輯關閉成功，但 MCP 子程序沒有下降。** 無法確認其原因是 Session 原本沒擁有目前 PID、或 Adapter/native thread 尚未 unload。此時 Codex ACP app-server 已啟動約 3 天 17 小時，與 9 月初 Session 的最後更新時間不同；不能直接以 Session age 推論目前 MCP age。

特別更正：`acp_session list` 的原生 `remote_count=25` 是**分頁**，有 `next_cursor`；未出現在第一頁不代表沒有被載入。此輪唯讀核對前 5 頁 125 筆，不是完整 native thread owner map。

依已宣布停止門檻（關閉有效而 MCP count 無下降），**停止擴大其餘 52 個 >30 天候選**。這次是獲使用者明確授權的五筆 managed Session close，不表示授權批量 kill Codex/Browser/CBM。

HIGH-0 下一步必須先建立 `Codex app-server → native thread/session → MCP stdio transport → child PID/birth` 的真實映射；確認 adapter 的 `session/close` 是否導致 thread unload/stdio EOF/process Wait；僅在**已證明由 Next 擁有**且沒有使用者工作時研究可安全清理的項目。對未知 owner 保守不動。
