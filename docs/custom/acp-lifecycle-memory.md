# ACP Session Lifecycle and Shared Memory

> 狀態：M7 implementation plan / M6 Broker handoff integrated
>
> 日期：2026-10-03
>
> 目標 Milestone：M7 — ACP Manager

本文件定義 M7 的 session lifecycle 與 shared-memory contract。Browser / Computer resource ownership 已由 M6 落地，M7 應沿用既有 Broker contract，不重新建立 adapter-owned browser/computer backend。

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

### Antigravity ACP

現行來源：自維護 `antigravity-acp` fork
目前分支：`fix/agentdock-hardening`
已安裝版本：`1.2.0-agentdock.5`
M6 capability commit：`2bd8426 feat(agentdock): add computer control broker proxy`

- AgentDock-launched ACP session 使用 private AGY HOME，不繼承 global browser / Computer Use MCP/plugins；
- Browser 與 Computer 都由 AgentDock host 注入 loopback MCP capability，共用 per-session token 但工具面分離；
- `session/close` 後 AgentDock host capability 被 revoke；
- 20 次真實 session new/close/runtime-close stress 已確認沒有殘留 `antigravity-acp`、`chrome-devtools-mcp`、`cua-repl`、`node_repl` 或 `SkyComputerUseClient` descendants。

2026-10-03 的 `refined-antigravity-acp` / `localharness` recycle 實驗保留為歷史背景，不再是現行 AgentDock Browser/Computer ownership 路徑。AgentDock Core 仍不加入 Antigravity-specific kill hack。

2026-10-03 曾觀察到多個 `localharness_external` 持有 `chrome-devtools-mcp --isolated --headless` child；M6 已以 host-owned Browser Broker 取代這種 browser ownership 路徑。現行 diagnostics 能從 ACP owner 追到 lease / worker / page/context / connector ownership，並以 bounded TTL/recovery 清理 AgentDock-owned resource。歷史現場證據與完整設計見 [browser-cdp-lifecycle.md](browser-cdp-lifecycle.md)。

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

## Acceptance Criteria

1. 20 個 ephemeral Codex worker 完成後 loaded sessions 回到基線。
2. 20 個 **prompt-driven ephemeral** Antigravity worker 完成後自動 close，loaded adapter state 回到基線；M6 已另完成 20 次手動 session new/close capability/process stress。
3. persistent session 不因 end_turn 被自動 close。
4. idle-managed session TTL 到期才 close，重新 open 可 resume。
5. close 與 prompt completion race 不造成 stale notification / double settlement。
6. close failure 不把 session 誤標 closed。
7. runtime restart 不遺失 persisted mapping。
8. Codex / AGY / AgentDock 使用同一份 Memory DB。
9. 新 ACP session 不再 spawn mcp-memory-service stdio child。
10. shared memory daemon 不再建立 CoreML partition temp bundle。
11. diagnostics 能區分 managed / loaded / active。
12. M6 Browser regression invariant：4~8 個並行 ACP browser task 不互相改變 context / page target。
13. M6 Browser regression invariant：concurrency 超額會 queue，不無界新增 `chrome-devtools-mcp` / Chrome process。
14. M7 session close / idle-managed policy 必須觸發既有 host capability release，Browser/Computer owner 回到基線。
15. M6 resource-lock invariant 持續成立：同一 resource 的並行 write 可被 serialize。
16. M6 routing invariant 持續成立：ACP 不 silent fallback 到 foreground Computer Use。
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

- shared Memory daemon：完成並運行，mcp-memory-service 11.14.0，repo 固定於 stable/v11.14.0；
- Codex CLI → shared Memory HTTP：完成；
- AGY CLI → shared Memory HTTP：完成；
- AgentDock dynamic MCP → shared Memory HTTP：待 M7 / 安全切換；
- codex-acp fork：既有 lifecycle recycle 工作可沿用，M7 仍需驗證 prompt-driven ephemeral / idle-managed policy；
- Antigravity ACP：現行自維護 `antigravity-acp 1.2.0-agentdock.5`，commit `2bd8426`，Browser / Computer capability 皆由 AgentDock host 注入；
- M6 Browser Broker：`feature/browser-broker` 已完成 lease isolation、4-active bounded admission、5 分鐘 TTL、30 秒 sweep、bounded recovery、external user-page protection、diagnostics 與 cleanup controls；
- M6 stress：8-owner Broker stress、4 個真 managed MCP/Chrome concurrent lease、20 次真實 Antigravity session new/close/runtime-close、managed host hard-crash cleanup、Orca no-focus、Edge external safety、`chrome-devtools-mcp@1.7.0` pin/schema regression均通過；
- M7 **尚未完成**：per-session `persistent | ephemeral | idle-managed` policy、prompt-driven ephemeral auto-close、idle-managed adapter close、shared Memory migration 與對應 Desktop UI；
- legacy stdio memory baseline 與舊 adapter process 應在 M7 drain/restart 時重新量測，不沿用 2026-10-03 的 process count 當現況。

M7 的核心原則是「消費 M6 Broker capability，不重做 M6 ownership」。Browser / Computer physical resource cleanup 由 Broker 負責；M7 決定的是 ACP session 在何時應 release 這些 capability、何時保留 adapter runtime，以及如何呈現 lifecycle diagnostics。
