# ACP Session Lifecycle and Shared Memory

> 狀態：Implementation plan / handoff
>
> 日期：2026-10-03
>
> 目標 Milestone：M7 — ACP Manager

本文件只定義 AgentDock Custom 後續應採用的 session lifecycle 與 shared-memory contract；本輪不修改 AgentDock runtime code。

## 背景

目前 ACP 同時存在長期互動 session 與一次性 delegated worker。AgentDock 在 prompt 回傳 end_turn 後只把 session 從 running 改回 ready，不代表 Adapter runtime 已釋放，因此大量一次性 worker 會累積 loaded session 與 child process。

2026-10-03 實測確認：

- Codex ACP dedicated app-server 會因 loaded thread 累積 stdio MCP / REPL child process。
- Antigravity 的 session/close 實際可用但 capability 未宣告；logical close 後 localharness_external 仍可能留存。
- stdio mcp-memory-service 會讓每個 Agent/thread 各啟 Python + ONNX runtime。

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

### refined-antigravity-acp

Fork：jacktdry/refined-antigravity-acp  
Branch：fix/session-lifecycle-recycle

- 補上實測存在但未宣告的 session/close capability
- 不宣告實測不支援的 session/delete
- lifecycle request 成功後才移除 session cache
- 最後一個 session close 後 recycle Google ACP child，回收舊 localharness tree

AgentDock 不加入 Antigravity-specific kill hack。

## M7 Work Items

Backend：

- session lifecycle policy model
- ephemeral auto-close
- idle-managed sweeper
- close failure / retry state
- resource diagnostics
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
2. 20 個 ephemeral Antigravity worker 完成後不留下對應 harness tree。
3. persistent session 不因 end_turn 被自動 close。
4. idle-managed session TTL 到期才 close，重新 open 可 resume。
5. close 與 prompt completion race 不造成 stale notification / double settlement。
6. close failure 不把 session 誤標 closed。
7. runtime restart 不遺失 persisted mapping。
8. Codex / AGY / AgentDock 使用同一份 Memory DB。
9. 新 ACP session 不再 spawn mcp-memory-service stdio child。
10. shared memory daemon 不再建立 CoreML partition temp bundle。
11. diagnostics 能區分 managed / loaded / active。
12. relevant unit / integration / race / Desktop tests 通過。

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

截至 2026-10-03：

- shared Memory daemon：完成並運行，mcp-memory-service 11.14.0，repo 固定於 stable/v11.14.0
- Codex CLI → shared Memory HTTP：完成
- AGY CLI → shared Memory HTTP：完成
- AgentDock dynamic MCP → shared Memory HTTP：待 M7 / 安全切換
- codex-acp fork：完成，branch fix/session-lifecycle-recycle，commit 630d7c6；已全域安裝
- refined-antigravity-acp fork：完成，branch fix/session-lifecycle-recycle，commit 2b99cb4；wrapper 1.3.2 已全域安裝
- Google Antigravity ACP runtime：已由 1.2.1 更新至 1.3.0
- real lifecycle smoke：Codex 與 Antigravity 都已驗證最後一個 session/close 後底層 child PID 被替換
- AgentDock runtime lifecycle code：本輪刻意未修改，由後續 AgentDock session 接手
