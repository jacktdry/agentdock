# ACP Session Lifecycle and Shared Memory

> 狀態：Implementation plan / handoff
>
> 日期：2026-10-03
>
> 更新：2026-10-04 — Antigravity 已切換到 hardened `antigravity-acp` 單一路徑
>
> 目標 Milestone：M7 — ACP Manager

本文件只定義 AgentDock Custom 後續應採用的 session lifecycle 與 shared-memory contract；本輪不修改 AgentDock runtime code。

## 背景

目前 ACP 同時存在長期互動 session 與一次性 delegated worker。AgentDock 在 prompt 回傳 end_turn 後只把 session 從 running 改回 ready，不代表 Adapter runtime 已釋放，因此大量一次性 worker 會累積 loaded session 與 child process。

2026-10-03 舊路徑實測確認：

- Codex ACP dedicated app-server 會因 loaded thread 累積 stdio MCP / REPL child process。
- 舊 Antigravity refined / Google ACP 路徑的 session/close 實際可用但 capability 未宣告；logical close 後 localharness_external 仍可能留存。
- stdio mcp-memory-service 會讓每個 Agent/thread 各啟 Python + ONNX runtime。

2026-10-04 新 baseline：

- AgentDock 不再使用 `refined-antigravity-acp` / Google `agy_acp_server` 作為 AGY provider。
- 單一路徑改為 hardened `jacktdry/antigravity-acp` → AGY CLI。
- `session/cancel` 採 bounded SIGINT → SIGKILL fallback；`session/close` / `session/delete` 會先 drain active prompt。
- Model 與 Reasoning effort 已分離，但 persisted session 仍保存 AGY 真實 concrete model ID。
- Adapter 明確尊重 `AGY_BIN` override，AgentDock 可固定使用已登入、已更新的 system AGY。
- AgentDock 以 isolated HOME 啟動 AGY；只共享 `~/.gemini/antigravity-cli` 狀態，不載入互動 AGY 的 `~/.gemini/config` MCP/plugin 設定。
- 真實 AgentDock E2E 已驗證 prompt / close / resume / delete；底層 live smoke 另驗證兩個 session 並行、cancel < 1 秒，以及 task 結束後無殘留 AGY child。

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
            ├── AgentDock
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

AgentDock dynamic MCP 的 memory 目前故意維持 stdio，以免干擾另一條開發 session。M7 接手時應改成 Streamable HTTP endpoint。

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

### antigravity-acp

Fork：jacktdry/antigravity-acp
Branch：fix/agentdock-hardening
Current deployment：1.2.0-agentdock.2

- `main` 保持跟 `shubzkothekar/antigravity-acp` upstream 對齊；AgentDock custom changes 不直接進 `main`。
- cancel idempotent，non-Windows 先 SIGINT，bounded grace period 後仍存活才 SIGKILL。
- session/close 與 session/delete 先停止 / drain active child，再 evict / delete session state。
- `/usage` 也使用同一套 tracked child lifecycle。
- 同一 session 同時第二個 prompt 會被拒絕，不覆蓋第一個 child 的 ownership。
- ACP 對外顯示 base Model + Reasoning effort，但只組出 `agy models` 實際 advertised 的 concrete ID。
- `AGY_BIN` 為真正 explicit override，優先於 adapter downloaded binary。
- AgentDock profile 使用 isolated HOME，避免每個 delegated worker 自動啟動互動 AGY 的 browser / codebase / GitHub / GitLab MCP。

AgentDock 不加入 Antigravity-specific kill hack；adapter 自己負責 AGY child lifecycle。

2026-10-03 的 `localharness_external` / `chrome-devtools-mcp` process count 屬於舊 refined / Google ACP 路徑的歷史證據。2026-10-04 單一路徑切換後，isolated HOME 下 `agy mcp list` 為空，AgentDock live prompt 不再為每個 AGY worker 啟動這批互動 MCP。Browser ownership、CDP connector 與 temporary profile lifecycle 的完整設計仍見 [browser-cdp-lifecycle.md](browser-cdp-lifecycle.md)，因為 ACP browser request 未來仍必須經 Browser Broker。

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

多 ACP 並行時，lease 必須以 `owner_acp_session_id` / `owner_task_id` 隔離，不得依賴 shared global selected page。Browser worker 可以共用，但 logical browser session 不可共用 target state。

初始 concurrency policy 先採 bounded pool：

```text
managed headless: 4
persistent authenticated profile: 2
external user-owned browser: 1~2
```

實際數值由 M6 4~8 ACP 並行壓測後調整；超出上限 queue，不以無界 spawn MCP / browser 擴充。

同一 application resource 的 write race 另以 site / resource lock 處理，不能只靠 browser context isolation。

## M7 Work Items

Backend：

- session lifecycle policy model
- ephemeral auto-close
- idle-managed sweeper
- close failure / retry state
- resource diagnostics
- Browser Broker lease acquire / release integration
- session close / TTL / crash 時釋放 browser lease
- browser lease / worker / context / page ownership diagnostics
- 禁止 ACP adapter 預設 per-session 常駐 `chrome-devtools-mcp`
- Computer Control request 經 AgentDock provider abstraction，不由 ACP 自行選 Orca / OpenAI Computer Use
- AgentDock Memory dynamic MCP 改用 shared HTTP
- ACP Manager API 暴露 lifecycle policy 與 diagnostics

Shared Desktop UI：

- session policy
- loaded / idle / running 狀態
- adapter process health
- 手動 Close / close failure
- package installed/latest
- fork/custom source
- shared Memory endpoint / version / storage / provider / health

## Acceptance Criteria

1. 20 個 ephemeral Codex worker 完成後 loaded sessions 回到基線。
2. 20 個 ephemeral Antigravity worker 完成後不留下對應 AGY task child / browser MCP tree。
3. persistent session 不因 end_turn 被自動 close。
4. idle-managed session TTL 到期才 close，重新 open 可 resume。
5. close 與 prompt completion race 不造成 stale notification / double settlement。
6. close failure 不把 session 誤標 closed。
7. runtime restart 不遺失 persisted mapping。
8. Codex / AGY / AgentDock 使用同一份 Memory DB。
9. 新 ACP session 不再 spawn mcp-memory-service stdio child。
10. shared memory daemon 不再建立 CoreML partition temp bundle。
11. diagnostics 能區分 managed / loaded / active。
12. 4~8 個並行 ACP browser task 不互相改變 context / page target。
13. ACP browser concurrency 超額會 queue，不無界新增 `chrome-devtools-mcp` / Chrome process。
14. ACP session close / crash / idle TTL 後，其 browser lease 可回收到基線。
15. 同一 resource 的並行 write 可被 serialize。
16. ACP 不 silent fallback 到 foreground Computer Use。
17. relevant unit / integration / race / Desktop tests 通過。

## Rollout / Cleanup

~~~text
1. shared memory daemon healthy
2. clients migrate to HTTP
3. verify new ACP sessions no longer spawn stdio memory
4. integrate ACP fork lifecycle fixes
5. implement AgentDock lifecycle policy
6. restart / drain old ACP adapters
7. verify old memory/CoreML handles are zero
8. clean stale ONNX/CoreML temp artifacts
~~~

不要在舊 ACP process 還持有檔案 handles 時先刪除 temporary model directories。

## Handoff Status

截至 2026-10-04：

- shared Memory daemon：完成並運行，mcp-memory-service 11.14.0，repo 固定於 stable/v11.14.0
- Codex CLI → shared Memory HTTP：完成
- AGY CLI → shared Memory HTTP：完成
- AgentDock dynamic MCP → shared Memory HTTP：待 M7 / 安全切換
- codex-acp fork：完成，branch fix/session-lifecycle-recycle，commit 630d7c6；已全域安裝
- antigravity-acp fork：`jacktdry/antigravity-acp`，branch `fix/agentdock-hardening`；hardening commits `93ad102`、`5620dc4`
- active Antigravity adapter：`1.2.0-agentdock.2`，已部署至 AgentDock profile
- refined-antigravity-acp：已從 AgentDock profile 移除並解除全域 npm 安裝，不再是可執行 AGY 路徑
- AgentDock Antigravity runtime：使用 system AGY 1.2.16，profile 注入 isolated HOME + `AGY_BIN`
- real lifecycle smoke：Gemini prompt、parallel sessions、cancel、close、resume、delete 均通過；task 結束後 AGY child 回到基線
- model discovery：Gemini 3.8/3.7/3.6 Flash、Gemini 3.1 Pro、Claude Opus 5.5、Claude Sonnet 5.5、GPT-OSS 120B；Model / Reasoning effort 已分離
- Claude routing：Sonnet/Opus 5.5 視為獨立稀缺額度，只派 bounded + context-compacted review / architecture specialist，concurrency = 1
- legacy stdio memory baseline：最終驗證仍有 53 個既有 .venv memory server process；本輪未終止，待 active session drain/restart 後再清理
- AgentDock runtime lifecycle code：本輪刻意未修改，由後續 AgentDock session 接手
