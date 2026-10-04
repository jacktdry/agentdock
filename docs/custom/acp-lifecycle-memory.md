# ACP Session Lifecycle and Shared Memory

> 狀態：M7 code + stress completed / M7.5 Next isolation pending / stable cutover intentionally deferred
>
> 日期：2026-10-03
>
> 更新：2026-10-05 — M7 已於 d8acb9be 整合完成；Memory cutover 先在 AgentDock Next 驗證，stable control plane 保持原樣
>
> 目標 Milestone：M7 — ACP Manager

本文件定義 M7 的 session lifecycle 與 shared-memory contract。Browser / Computer resource ownership 已由 M6 落地，M7 應沿用既有 Broker contract，不重新建立 adapter-owned browser/computer backend。

## Development isolation boundary

目前連線中的 AgentDock 是 production control plane。先前 live activation 導致 ChatGPT Mac-Dev channel 斷線，故 Next 開發禁止檢查、修改、重啟、停止、替換 stable App/Core，或操作 `~/.agentdock`、stable stdio Memory registry 與 live launchd services。Stable 不是 development target；舊 stdio Memory children 不可為 Next 驗收而 drain / kill / cleanup。

下一步是 [M7.5 AgentDock Next isolation](agentdock-next-isolation.md)，不是 direct stable activation。以下 2026-10-03/04 daemon / adapter 資訊是既有驗證紀錄，不是本次重新檢查結果或操作 live service 的授權。

## 背景

目前 ACP 同時存在長期互動 session 與一次性 delegated worker。AgentDock 在 prompt 回傳 end_turn 後只把 session 從 running 改回 ready，不代表 Adapter runtime 已釋放，因此大量一次性 worker 會累積 loaded session 與 child process。

2026-10-03 實測確認：

- Codex ACP dedicated app-server 會因 loaded thread 累積 stdio MCP / REPL child process。
- Antigravity 的 session/close 實際可用但 capability 未宣告；logical close 後 localharness_external 仍可能留存。
- stdio mcp-memory-service 會讓每個 Agent/thread 各啟 Python + ONNX runtime。

2026-10-04 現行 Antigravity baseline：

- AgentDock 不再使用 `refined-antigravity-acp` / Google `agy_acp_server` 作為 AGY provider；單一路徑為 hardened `jacktdry/antigravity-acp` → system AGY CLI。
- `session/cancel` 採 bounded SIGINT → SIGKILL fallback；`session/close` / `session/delete` 會先 drain active prompt。
- Model 與 Reasoning effort 已分離，persisted session 保存 AGY 真實 concrete model ID。
- `AGY_BIN` 是 explicit override；目前 system AGY 為 `1.2.16`。
- AgentDock 以 isolated HOME 啟動 AGY，只共享 Antigravity CLI auth / conversation state，不載入互動 AGY 的 MCP/plugin 設定。
- M6 再把 Browser / Computer 能力收斂成 AgentDock host-owned loopback MCP capability，不允許 ACP child 恢復自己的 browser / Computer Use backend。

責任分層：

1. AgentDock：session orchestration，決定何時保留、何時 close。
2. ACP adapter：負責自身底層 process cleanup。
3. Memory：獨立共享服務，不是每個 ACP session 的 child process。

## Shared Memory Baseline

~~~text
macOS LaunchAgent
└── mcp-memory-service 11.14.0+
    └── Streamable HTTP
        http://127.0.0.1:8766/mcp
            ├── AgentDock Next (proposed; Next registry only)
            ├── Codex CLI
            ├── Codex ACP
            ├── AGY CLI
            └── Antigravity ACP

storage:
~/.local/share/mcp-memory/sqlite_vec.db
~~~

目前已完成：

- service venv：~/.local/share/mcp-servers/mcp-memory-service/.venv-service
- LaunchAgent：~/Library/LaunchAgents/dev.dropabit.mcp-memory.plist
- endpoint：http://127.0.0.1:8766/mcp
- embedding provider：CPUExecutionProvider
- quality device：cpu
- Codex CLI 與 AGY CLI 已切 shared HTTP memory

M7 已完成 dynamic MCP shared HTTP client、`protocol_version=2025-11-25` compatibility pin 與 integration test，並於 `d8acb9be` 整合。**Stable pre-M7 AgentDock 保留 stdio registry，不做 live cutover。** Next 首先在 `~/.agentdock-next` 的獨立 registry/config 指向 `http://127.0.0.1:8766/mcp`，以固定 protocol 驗證 handshake / health / ACP lifecycle。這不要求重啟或重新設定 shared Memory daemon，更不能修改 stable registry。Next 使用 shared DB 不代表共享 AgentDock state 或 cleanup authority。

HTTP mode 保留一般 store/search/retrieve/tag/delete 等能力。memory_harvest 與 memory_ingest 因可讀 server host filesystem path，被 upstream 刻意設為 local-only。未來若 UI 需要 Import / Harvest，應另建 trusted local maintenance path，不要因此退回 per-session stdio。

## Session Lifecycle Policy

### persistent

適用於使用者明確要延續的互動 session。

- end_turn 後保持 ready
- 不自動 close

### ephemeral

適用於 bounded delegated worker。

- bounded task 完成且 terminal prompt settlement 後，自動 session/close
- close failure 必須可觀察
- close 不等於刪除 remote transcript

### idle-managed

適用於可能延續，但不值得長時間保持 loaded runtime 的 session。

- end_turn 後回 ready
- 超過 TTL 且沒有 active prompt、pending interaction 或 async work 時自動 close
- 後續用 open/resume/load 恢復

建議 SessionRecord 加入：

~~~text
lifecycle_policy: persistent | ephemeral | idle-managed
last_active_at
idle_close_after_ms
closed_reason
auto_close_attempted_at
auto_close_error
~~~

Policy 必須是 per-session，不只做成 profile-level 設定。

## Runtime Requirements

Delegated worker：

~~~text
new(ephemeral)
→ prompt
→ end_turn
→ orchestrator 驗證 bounded task 完成
→ session/close
→ closed
~~~

Idle cleanup 使用單一 bounded sweeper，不為每個 session 建 timer。Close race 應沿用既有 session operation / close fence。

## Diagnostics

M7 至少提供：

~~~text
managed_sessions
loaded_sessions
running_sessions
ready_sessions
idle_ready_sessions
ephemeral_sessions
auto_closed_sessions
close_failures
adapter_process_pid
adapter_child_process_count
adapter_rss
~~~

UI 必須區分 persisted session 數量與目前 loaded runtime session 數量。

## Adapter Fork Responsibility

### Codex ACP

Fork：jacktdry/codex-acp  
Branch：fix/session-lifecycle-recycle

- session/close 仍先 thread/unsubscribe
- 最後一個 ACP session close 後 recycle adapter 專用 codex app-server
- 保留 runtime provider / gateway routing
- 若未來 upstream 提供正式 thread/unload，移除 workaround

AgentDock 不直接 kill Codex app-server。

### Antigravity ACP

Fork：`jacktdry/antigravity-acp`
Branch：`fix/agentdock-hardening`
Current deployment：`1.2.0-agentdock.5`
Current tip：`2bd8426 feat(agentdock): add computer control broker proxy`

演進鏈：`93ad102` → `5620dc4` → `746b52e` → `05c53f3` → `2bd8426`。

- `main` 保持跟 `shubzkothekar/antigravity-acp` upstream 對齊；AgentDock custom changes 不直接進 `main`。
- cancel idempotent；non-Windows 先 SIGINT，bounded grace period 後仍存活才 SIGKILL。
- session/close 與 session/delete 先停止 / drain active child，再 evict / delete session state；`/usage` 也使用 tracked child lifecycle。
- 同一 session 的第二個 concurrent prompt 會被拒絕，不覆蓋第一個 child ownership。
- ACP 對外顯示 base Model + Reasoning effort，只組出 `agy models` 真實 advertised concrete ID。
- `AGY_BIN` 為 explicit override，優先於 adapter downloaded binary；目前 AgentDock 使用 system AGY `1.2.16`。
- AgentDock-launched ACP session 使用 isolated/private AGY HOME，不繼承互動 AGY 的 browser / codebase / GitHub / GitLab MCP/plugin 設定。
- Browser 與 Computer 都由 AgentDock host 注入 loopback MCP capability，共用 per-session token 但工具面分離；`session/close` 後 capability 被 revoke。
- 20 次真實 session new/close/runtime-close stress 已確認沒有殘留 `antigravity-acp`、`chrome-devtools-mcp`、`cua-repl`、`node_repl` 或 `SkyComputerUseClient` descendants。

2026-10-03 的 `refined-antigravity-acp` / Google ACP / `localharness_external` 路徑保留為歷史證據，不再是現行 AgentDock Browser/Computer ownership 路徑。該 repo 仍可存在於本機研究用途，但 AgentDock profile 不以它作為 AGY provider。AgentDock Core 不加入 Antigravity-specific kill hack；adapter 負責 AGY child lifecycle，Browser / Computer physical resource lifecycle 則由 M6 Broker 負責。

後續架構不再讓每個 ACP / localharness 長期自行持有 browser backend。ACP browser 工作改成：

```text
ACP session
→ request browser capability
→ AgentDock Browser Broker
→ acquire browser lease
→ isolated context / page
→ task work
→ release lease
```

多 ACP 並行時，lease 以 `owner_acp_session_id` / `owner_task_id` 隔離，不依賴 shared global selected page。M6 實作採 **exclusive managed worker generation per lease**；external browser 也使用 per-lease AgentDock-owned connector generation，避免 MCP selected-page state 跨 lease。

目前 managed admission policy 已固定並通過 8-owner stress：

```text
managed headless active: 4
managed queue capacity: 16
idle TTL: 5 minutes
lifecycle sweep: 30 seconds
```

8-owner stress 實際形成 4 active + 4 queued；即使每個 worker 都回傳相同 `pageId=1`，也沒有 cross-session target contamination。M7 不應把這個安全模型改回 shared worker selected-page state。

同一 application resource 的 write race 另以 site / resource lock 處理，不能只靠 browser context isolation。

## M7 Work Items

Backend：

- session lifecycle policy model：`persistent | ephemeral | idle-managed`；
- prompt terminal 後的 ephemeral auto-close；
- idle-managed bounded sweeper；
- close failure / retry state 與 persisted session mapping；
- managed / loaded / running / idle adapter diagnostics；
- session close / idle policy 透過既有 SessionMCPProvider release host-owned Browser / Computer capability；
- 將 M6 `browser_broker` / `computer_broker` diagnostics 關聯到 ACP session view；
- 維持「ACP adapter 不預設持有 browser/Computer Use backend」這個 M6 invariant，不重新實作 Browser Broker；
- AgentDock Memory dynamic MCP 改用 shared HTTP；
- ACP Manager API 暴露 lifecycle policy、adapter health 與 diagnostics。

Shared Desktop UI：

- session policy
- loaded / idle / running 狀態
- adapter process health
- 手動 Close / close failure
- package installed/latest
- fork/custom source
- shared Memory endpoint / version / storage / provider / health

### M7 Implementation Checkpoint — 2026-10-04

M7 code / stress 已完成：

- per-session `persistent | ephemeral | idle-managed` policy、metadata persistence 與 legacy default；
- prompt terminal 後 ephemeral auto-close、close failure/retry、manual-vs-auto race fence；
- 單一 30 秒 bounded idle sweeper，每次最多 4 個，且不為 unloaded/dead adapter 重新啟動 process；
- `acp_session action=status`：managed / loaded / running / ready / idle / closed、auto-close failure、adapter PID / descendants / RSS，以及 M6 Browser / Computer correlation；
- Shared Desktop ACP lifecycle UI、manual close / policy update confirmation、adapter health、Memory health；
- `modelcontextprotocol/go-sdk` 升至 `v1.8.0`，dynamic MCP 支援 optional `protocol_version`，Memory 使用 `2025-11-25`；
- 真實 `codex-acp 2.1.1` 與 `antigravity-acp 1.2.0-agentdock.5` 均通過 persistent、ephemeral、idle-managed close/resume；
- 兩個 adapter 各完成 20 個 prompt-driven ephemeral session stress，同一 Runtime / adapter PID 下 resources 每批回 baseline；
- Antigravity `gemini-3.1-pro` + `reasoning_effort=high` update / prompt 實測通過；
- M6 regression：8 owner = 4 active + 4 queued、4 live managed Chrome leases、same-resource lock、foreground Computer fail-closed、adapter-owned browser/computer backend 不復活；
- Shared Memory：Codex CLI / AGY CLI 已指向 `http://127.0.0.1:8766/mcp`；新 ACP session 不產生 per-session stdio Memory child；daemon idle CPU 約 0.1%，stress 後未見 CoreML / `.mlmodelc` / partition bundle。

M7 feature code / stress 已完成；stable pre-M7 App 的 stdio `memory` registry 保持不動，舊 live cutover 刻意延後。新增 M7.5 驗證 Next-only HTTP registry 與新 Next ACP sessions 無 stdio Memory child；不要求 stable children 歸零。

## Acceptance Criteria

以下 lifecycle / process / Memory 驗收適用於 isolated Next runtime；既有 M7 stress 為歷史完成證據，不宣稱 Next isolation 已通過。Stable sessions / children 不納入 cleanup 目標。

1. 20 個 ephemeral Codex worker 完成後 loaded sessions 回到基線。
2. 20 個 **prompt-driven ephemeral** Antigravity worker 完成後自動 close，loaded adapter state 回到基線；M6 已另完成 20 次真實 session new/close/runtime-close capability/process stress。
3. persistent session 不因 end_turn 被自動 close。
4. idle-managed session TTL 到期才 close，重新 open 可 resume。
5. close 與 prompt completion race 不造成 stale notification / double settlement。
6. close failure 不把 session 誤標 closed。
7. runtime restart 不遺失 persisted mapping。
8. Next registry 以 protocol pin `2025-11-25` 連到 shared HTTP `http://127.0.0.1:8766/mcp`，與既有 Codex / AGY 使用同一份 Memory DB；stable stdio registry 保留。
9. 新 Next ACP session 不再 spawn mcp-memory-service stdio child。
10. shared memory daemon 不再建立 CoreML partition temp bundle。
11. diagnostics 能區分 managed / loaded / active。
12. M6 Browser regression invariant：4~8 個並行 ACP browser task 不互相改變 context / page target。
13. M6 Browser regression invariant：concurrency 超額會 queue，不無界新增 `chrome-devtools-mcp` / Chrome process。
14. M7 session close / idle-managed policy 必須觸發既有 host capability release，Browser/Computer owner 回到基線。
15. M6 resource-lock invariant 持續成立：同一 resource 的並行 write 可被 serialize。
16. M6 routing invariant 持續成立：ACP 不 silent fallback 到 foreground Computer Use。
17. relevant unit / integration / race / Desktop tests 通過。

## Next Rollout / Cleanup

1. 先完成 M7.5 namespace / target isolation；不得對 stable App/Core、registry 或 live launchd services 動作。
2. 在 Next registry/config 設定 shared HTTP endpoint 與 `2025-11-25` pin；驗證既有 endpoint 可用，失敗則停止 Next rollout，不重新設定 / 重啟 shared daemon。
3. 以 Next-owned ACP 驗證 Memory health、persistent / ephemeral / idle-managed、20 prompt-driven stress；只計算 Next-owned processes / resources。
4. Drain / restart / cleanup 僅限可證明 Next ownership 的 adapters / children / temp artifacts；ownership 不明時 fail closed，不碰 stable stdio Memory children 或 shared daemon/model artifacts。
5. Next lifecycle 與 Memory 驗證完成後，以獨立 `mac-dev-next` 連線 ChatGPT；確認原 Mac-Dev channel 持續可用。
6. Next 完整開發 / 測試 / 獨立連線後，stable migration / retirement 才能作未來另行規劃與授權的工作。

不要在 Next-owned process 還持有 handles 時刪除其 temporary directories；不以全域 process 清理或 stable children 歸零作驗收。

## Handoff Status

2026-10-04 驗證紀錄，2026-10-05 更新 rollout 決策（未重新檢查 live runtime）：

- shared Memory daemon：完成並運行，mcp-memory-service 11.14.0，repo 固定於 `stable/v11.14.0`；live idle CPU 約 0.1%，M7 stress 後沒有 CoreML / `.mlmodelc` / partition bundle；
- Codex CLI → shared Memory HTTP：完成；
- AGY CLI → shared Memory HTTP：完成；
- AgentDock dynamic MCP → shared Memory HTTP：**程式碼 / integration test 已完成；先驗證 Next registry，stable live cutover 刻意延後**；
- codex-acp：`2.1.1` 真實驗證 persistent / ephemeral / idle-managed、20 prompt-driven ephemeral stress、host capability release 均完成；
- Antigravity ACP：`jacktdry/antigravity-acp` `fix/agentdock-hardening`，目前部署 `1.2.0-agentdock.5`，tip `2bd8426`；system AGY `1.2.16`；
- refined-antigravity-acp：不再是 AgentDock 可執行 AGY 路徑；僅保留歷史/研究 repo；
- AgentDock Antigravity runtime：isolated HOME + `AGY_BIN`，不繼承互動 AGY MCP/plugin；Browser / Computer capability 由 AgentDock host 注入；
- real adapter：Codex / Antigravity 均完成 persistent / prompt-driven ephemeral / idle-managed TTL close+resume；兩邊各跑 20 prompt stress，同一 adapter PID 下 resources / descendants 回到 baseline；
- model discovery / routing：Model 與 Reasoning effort 分離；Claude Sonnet 5.5 / Opus 5.5 視為獨立稀缺額度 specialist，只派 bounded + context-compacted review / architecture，concurrency = 1；
- M6 Browser Broker：lease isolation、4-active bounded admission、5 分鐘 TTL、30 秒 sweep、bounded recovery、external user-page protection、diagnostics 與 cleanup controls 已完成；
- M6/M7 regression：8-owner Broker stress（4 active + 4 queued）、4 個真 managed MCP/Chrome concurrent lease、same-resource lock、managed host hard-crash cleanup、Orca no-focus、foreground fail-closed、Edge external safety、`chrome-devtools-mcp@1.7.0` pin/schema regression均通過；
- M7 code / stress **已完成**：lifecycle policy、prompt-driven auto-close、idle-managed close/resume、diagnostics、Shared Desktop UI、Memory HTTP client / protocol pin 與 real-adapter stress；已於 `d8acb9be` 整合；下一 gate 為 M7.5 Next isolation，不再要求 stable live Memory cutover；
- Next process baseline 在 Next-only 驗證時建立；不沿用 2026-10-03 的 process count，不 drain/restart 或檢查 stable legacy stdio memory / adapters。

M7 的核心原則是「消費 M6 Broker capability，不重做 M6 ownership」。Browser / Computer physical resource cleanup 由 Broker 負責；M7 決定的是 ACP session 在何時 release 這些 capability、何時保留 adapter runtime，以及如何呈現 lifecycle diagnostics。
