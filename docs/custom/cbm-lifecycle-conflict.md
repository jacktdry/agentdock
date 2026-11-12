# Codebase Memory Daemon / Worker Build Conflict

> 狀態：Observed / needs design follow-up
>
> 首次記錄：2026-10-03
>
> 影響範圍：AgentDock dynamic MCP / Codebase Memory indexing lifecycle
>
> 目前處置：不要為了 re-index 強制終止其他 active session；等待舊 session drain 後再重啟 / re-index

## 摘要

2026-10-03 在更新 Codebase Memory MCP 後，新的 repository indexing request 持續失敗，worker log 顯示：

~~~text
CBM index worker could not start: worker executable build conflicts with its supervisor; close all CBM sessions and retry
~~~

這不是特定 repository 的 index 內容錯誤。當時 `osaka-trip-2026` 只是第一個被觀察到無法 refresh 的專案；同一個 CBM supervisor 下的其他 repository indexing request 也可能受到影響。

現場證據高度指向：**CBM executable 已被更新，但既有 daemon / supervisor / MCP sessions 仍執行更新前載入的 build。舊 supervisor 從磁碟啟動新版 worker 時，CBM 的 build compatibility guard 拒絕兩個 build 混用。**

這個 guard 本身是合理的資料一致性保護；AgentDock Custom 要改善的是「更新中的 service lifecycle、drain、restart 與 pending re-index orchestration」，而不是繞過 build check。

## 2026-10-03 現場證據

### Process timeline

觀察到的 shared daemon：

~~~text
PID 1184
start: 2026-10-01 19:45:59
command:
codebase-memory-mcp --cbm-daemon-internal
~~~

同時存在大量從 2026-10-01、10-02、10-03 不同時間啟動的 `codebase-memory-mcp` process。

目前磁碟上的 executable：

~~~text
~/.local/share/mcp-servers/codebase-memory-mcp/bin/codebase-memory-mcp

mtime: 2026-10-03 12:28
sha256:
a67b7ccead5d2ca852051f8619458ab96af41393257b56fb36e523a110265d48
~~~

因此至少 daemon PID 1184 與多個 12:28 以前啟動的 MCP process，都是在目前磁碟 binary 被替換之前就已經載入執行。

### Supervisor failure

`~/.cache/codebase-memory-mcp/logs/cbm-daemon.log` 反覆出現：

~~~text
level=info msg=index.supervisor.reap outcome=exit_nonzero exit_code=1
level=warn msg=index.supervisor.worker_failed outcome=exit_nonzero exit_code=1
~~~

對應 worker log 的明確錯誤：

~~~text
CBM index worker could not start: worker executable build conflicts with its supervisor; close all CBM sessions and retry
~~~

失敗並非單次事件；daemon log 中可看到大量連續的 worker start failure。

## 高可信根因模型

目前最符合觀察的流程：

~~~text
old CBM binary starts daemon / MCP sessions
        ↓
CBM executable is updated in place
        ↓
old daemon / supervisor stays resident in memory
        ↓
new indexing request arrives
        ↓
old supervisor spawns worker from current on-disk executable
        ↓
supervisor build != worker executable build
        ↓
compatibility guard rejects worker
        ↓
index refresh fails
~~~

重要：這個結論目前是由 process start time、binary mtime 與 CBM 自身錯誤訊息交叉支持的高可信根因模型。後續實作前仍應確認 upstream CBM 實際如何產生 / 比較 build identity，不應只依錯誤文字猜測內部欄位。

## 為什麼不應直接 kill 全部 CBM process

AgentDock 同時可能有多個 active ChatGPT / ACP / development session 使用 Codebase Memory。

直接：

~~~text
pkill codebase-memory-mcp
~~~

可能：

- 中斷其他 session 正在進行的 architecture / impact query；
- 中斷另一個 repository 的 index worker；
- 讓上游 Agent 收到無法區分「正常 drain」與「突然故障」的 MCP failure；
- 造成 retry storm；
- 破壞之後要釐清 lifecycle 問題所需的現場狀態。

所以目前安全策略是：

1. 不因單一 repository re-index 需求殺掉其他 active CBM session。
2. 暫時將 re-index 視為 deferred maintenance。
3. 等使用舊 build 的 sessions 自然結束。
4. 確認 daemon / supervisor 已由目前 executable 重新啟動。
5. 再執行 pending repository re-index。
6. 驗證 `index_status` 與 repository branch / commit context。

## AgentDock Custom 應改善的行為

理想 lifecycle：

~~~text
CBM update detected
        ↓
compare installed build vs running daemon build
        ↓
same build ───────────────→ normal operation
        ↓ different
mark: restart_pending
        ↓
block/defer new indexing jobs
        ↓
allow read-only queries when protocol-compatible
        ↓
wait for active jobs / sessions to drain
        ↓
graceful daemon restart
        ↓
verify new daemon build
        ↓
replay pending re-index queue
        ↓
healthy
~~~

### Minimum viable improvement

AgentDock 不一定要立刻接管所有 CBM process，但至少應具備：

- 顯示 installed CBM version / build；
- 顯示 running daemon version / build 或至少 daemon start time；
- 偵測「binary 比 daemon 新」或明確 build mismatch；
- 將 CBM health 標示為 `restart_pending` / `version_mismatch`；
- 發現 mismatch 時停止無意義地反覆 spawn index worker；
- 對需要 re-index 的 repository 建立 bounded pending queue；
- active session drain 後執行 graceful restart；
- restart 後自動重試 pending re-index；
- 失敗有 bounded backoff，禁止 retry storm。

### Process ownership / cleanup

目前同時存在很多 `codebase-memory-mcp` process。後續要先區分：

- shared daemon / supervisor；
- per-client stdio MCP process；
- active index worker；
- stale client process；
- orphan process。

不能只用 process name + age 判斷 stale。

建議 diagnostics 至少包含：

~~~text
installed_version
installed_build
installed_binary_mtime

daemon_pid
daemon_started_at
daemon_version
daemon_build

client_process_count
active_index_worker_count
pending_index_count
last_index_failure

restart_pending
restart_reason
version_mismatch

active_session_count (如果 CBM protocol 可提供)
~~~

## 後續 session 的確認清單

接手 session 不要直接開始寫 restart code，先確認：

1. CBM upstream / local implementation 中 build conflict check 的實際程式位置與 build identity 來源。
2. daemon 是由誰啟動、誰持有、是否已有 single-instance / socket ownership 機制。
3. MCP client process 與 shared daemon 的關係；哪些 process 可以安全退出，哪些代表 active session。
4. AgentDock 更新 CBM package / binary 的實際流程，是否能在 replace 前發出 drain intent。
5. CBM 是否已有 shutdown / restart / health / version RPC 可以直接使用。
6. index supervisor 是否有 queue、backoff 或 job cancellation 機制。
7. binary replacement 能否改成 versioned path + atomic active-version switch，避免 running supervisor spawn 到不同 build。
8. macOS、Windows、Linux 的 lifecycle 差異，不能只做 macOS `pkill` 類 workaround。

## 候選設計方向

### A. Drain-and-restart orchestration

AgentDock 偵測 CBM 更新後：

- 新 indexing job 進 pending queue；
- 既有 query / index job drain；
- 對 daemon 發 graceful shutdown；
- 以新版 executable 啟動；
- replay pending jobs。

優點：使用者體感最佳。
風險：AgentDock 必須知道 CBM active work 狀態與 shutdown contract。

### B. Versioned executable path

不要原地覆蓋：

~~~text
.../codebase-memory-mcp/<build-id>/codebase-memory-mcp
~~~

running daemon 永遠從自己的 build path spawn worker；新 client / daemon 才使用新 path。

優點：從根本避免 supervisor spawn 到不同 build。
風險：需要版本 GC、active-build ownership 與 disk cleanup。

### C. Hybrid

以 versioned executable 保證 process tree build 一致，再由 AgentDock 做 drain/restart 與舊版本 GC。

目前最值得優先評估的是 C；但在讀過 CBM implementation 前不要定案。

## Acceptance criteria 草案

後續修正至少應驗證：

1. CBM binary 更新時，既有 active read/query session 不被粗暴中斷。
2. 更新前啟動的 supervisor 不會 spawn 更新後不同 build 的 worker。
3. mismatch 被明確診斷，不會形成連續 worker failure / retry storm。
4. 新 indexing request 可以 deferred，而不是直接遺失。
5. drain 完成後 CBM 能自動切換至新 build。
6. pending re-index 能自動恢復並驗證成功。
7. 多 repository 同時使用 CBM 時，不因其中一個 refresh 需求終止其他 active job。
8. stale / orphan process cleanup 不會誤殺 active client。
9. macOS / Windows / Linux 都有一致的 lifecycle contract；平台差異只存在 adapter。
10. service restart / AgentDock restart 後 pending state 能收斂，不形成永久 `restart_pending`。

## 現階段結論

這個事件應視為 **CBM update lifecycle / process ownership 問題**，不是 repository indexing correctness 問題。

在正式改善前，維持以下 operational rule：

> 若遇到 `worker executable build conflicts with its supervisor`，先確認是否存在更新前啟動的 CBM daemon / sessions。不要為了單一 re-index 直接終止所有 CBM process；讓 active work drain，之後再安全 restart 並補做 re-index。


## 2026-10-11 補充：index options 競爭與跨 MCP client 數量

唯讀 CBM daemon log 顯示 `watcher.changed … strategy=git`、`active_jobs=1`、`index.supervisor.reap outcome=clean`，且期間 `index_repository` 因不同 options 而衝突。此現象**不同於**本文件 2026-10-03 的 binary/worker build mismatch；不能把另一項正在索引自動解讀成另一個 ChatGPT Session 正在主動使用。

macOS 観察 64 個 CBM Clients（其中 62 個是 Codex ACP app-server 子程序）及 1 個 shared daemon：64 Clients 不代表 64 個活躍 indexing workers。長期需 project+commit+options coalescing 和 bounded retry/queue，以及 daemon/client per-session lifecycle；不為解決此問題直接 kill daemon 或其他 Session。見 [MCP Runtime 共用生命週期](pre-m9-mcp-runtime-lifecycle-policy-2026-10-11.md)。

## 2026-10-14 P9 bounded release-impact triage（唯讀、未重啟）

這次只回答 **M9 安裝／更新是否會破壞 CBM 或使用者既有工作**，沒有為外部 CBM 建立新的管理服務、改動索引設定、停止程序或重建索引。

- **歷史 build mismatch（2026-10-03）**：在外部 `~/.local/share/mcp-servers/codebase-memory-mcp/bin/codebase-memory-mcp` executable 原地更新後，舊 supervisor 可能仍存活且啟動磁碟上的新 worker，觸發既有 build 相容性拒絕；這是 **CBM 自身 binary 更新與跨進程版本生命週期**，不得繞過一致性檢查，也不能只因 Client 長駐就 `pkill`。
- **本次取樣（2026-10-14 +08）**：上述 executable 磁碟 mtime 為 **2026-10-03 12:28:35**。macOS process name/arguments 僅用於本機唯讀篩選、未記錄完整 argv/env/credentials；共 66 個 CBM 名稱提示，其中 **2 個 daemon**（當次 PID 58769／12280，啟動於 10/07／10/14）與約 **64 個 Client**，多數 Client 的 PPID 為 Codex app-server（本次 parent PID 55205）。未觀察到啟動早於目前 CBM executable mtime 的 CBM 程序，**但這不是可靠的 running build/version API**；不可據此推定不存在更新中的程序。最近 CBM daemon log 尾端 4,000 行含 **794 次 clean supervisor reap**、770 筆 Git watcher change、`active_jobs=1` 訊息及 **0 次 worker_failed**，沒有重現舊式 build mismatch。Client 數量也不是索引 worker／活躍 job 數。
- **獨立的索引選項衝突（2026-10-11）**：Git watcher 自動索引與人工 `index_repository` 用不同 options 可能競爭；此時索引拒絕／等待不等於 supervisor binary mismatch，也無法推斷是哪個 ChatGPT／Codex Session 正在索引。必要時做 project+branch/commit+options 的 bounded coalescing 和 retry，不能直接中止已接受的其他 repo job。
- **與 AgentDock Next install/update 的關係**：實際 Next-only `packaging/macos/install-next-shared.py` 和 `deploy-next-shared.py` 使用固定 allowlist，交易式替換 **`~/Applications/AgentDock Next.app/Contents`** 並重新註冊 Next 自有服務，未包含 CBM 的外部 executable、cache、daemon 或其 PID。M7/P5 MCP Client 與 CBM shared daemon 並無由此 installer 直接啟動的 CBM binary update 路徑。現有證據**不支持** Next 安裝／更新會製造本事件的 installed-vs-running CBM build mismatch；也不支持為 M9 提前修改這個多方共享 daemon。
- **P9 結論**：將此事件判為 **post-M9 CBM service/ACP-MCP runtime lifecycle 維護債**，不列作 *目前已證實* 的 M9 installer/release 阻斷。保留後續清晰退出條件：上游 build/version 介面、owner/lease/generation 真實證據、active index-job drain、index options coalescing、bounded pending retry、restart 與驗證；跨所有 owner 前不得自動 stop。**若日後觀察到 Next 自己的安裝／更新會改變仍在服務中的 managed MCP binary，或影響有效的用戶工作，必須重新開啟 M9 阻斷判定**。
