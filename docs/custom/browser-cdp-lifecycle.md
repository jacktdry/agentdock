# Browser Broker / CDP Lifecycle and Cleanup

> 狀態：Planned / handoff-ready
> 建立日期：2026-10-03
> 主要 Milestone：M6 — Browser Broker、M7 — ACP Manager
> 本輪限制：只更新規劃與交接文件，不修改 AgentDock runtime code。

## 背景

AgentDock Custom 的 Browser Control 已從單純 Browser Routing 收斂為 **Browser Broker**：

```text
ACP / task / upstream ChatGPT
→ Browser Broker
→ route by workspace / auth / explicit user override / isolation / foreground policy

company workspace
→ user's existing Microsoft Edge
→ user's authenticated Edge profile
→ AgentDock-owned background page/lease

non-company/default
→ chrome-devtools-mcp worker
→ AgentDock-managed isolated/headless Chrome

explicit user request
→ requested profile/browser
```

Company routing 是硬規則，不是 preference。公司 GitLab、Cloudflare 與相關後台登入只存在於使用者目前的 Edge profile，因此上游 ChatGPT 自己做 UAT 與 ACP 受派做 UAT 都必須使用同一個 authenticated Edge profile。若該 Edge connector 不可用，Broker 必須 fail closed 並回報 connector/auth requirement，不得靜默 fallback 到 isolated Chrome。

Browser Broker 負責：

- routing；
- browser / connector / profile ownership；
- browser lease；
- BrowserContext / Page isolation；
- concurrency / queue；
- no-focus policy；
- lifecycle / cleanup；
- diagnostics。

`chrome-devtools-mcp` 優先作為 browser automation engine；AgentDock 不優先重寫完整 DevTools automation stack。

目前功能已能工作，但 2026-10-03 現場盤點確認 browser / CDP process ownership 與 ACP session lifecycle 還沒有一致 contract，導致長時間執行後容易累積：

- 專案測試留下的 headless Chrome；
- `/tmp` browser profile；
- 多組 `chrome-devtools-mcp --isolated --headless`；
- 舊 Antigravity refined / Google ACP 路徑的 `localharness_external` tree；
- 同一 Edge CDP endpoint 的重複 connector；
- 「session persisted」與「runtime still loaded」無法從現有 diagnostics 明確區分。

這不是單純的手動 `pkill` 清理問題，而是 lifecycle ownership / observability 缺口。

## 2026-10-03 現場證據

本輪實測：

- Apple History 留有一組獨立 headless Chrome：
  - CDP：`127.0.0.1:9340`
  - profile：`/tmp/apple-history-m9-mobile`
  - process 已運行約 22 小時；
- 該 Chrome 已安全停止，`9340` 已關閉，temporary profile 已刪除；
- 使用者既有 Microsoft Edge：
  - CDP listen：`127.0.0.1:9222`
  - 需保留作 company project authenticated UAT；
- 舊 `edge-devtools-profile` 使用 `--browserUrl=http://127.0.0.1:9222`：
  - `/json/version` 回 `404`；
  - `list_pages` 實測失敗；
- `edge-devtools-profile-ws` 使用 `--wsEndpoint=ws://127.0.0.1:9222/devtools/browser`：
  - `list_pages` 實測成功；
  - 可讀取目前 CMS / GitLab 頁籤；
- 因此舊 HTTP connector 已移除，只保留 WebSocket connector；
- Antigravity：
  - 30 個 `localharness_external`；
  - 23 個 `chrome-devtools-mcp@1.7.0 --isolated --headless` child；
  - 全部 parent 到仍存活的 `agy_acp_server`；
  - Adapter 列出 48 個 native session、45 個 managed session；
  - 這些 session 目前都被標成 `ready` / 可恢復。

重要結論：**不能用 process age、0% CPU 或 `ready` 狀態直接判定 Antigravity browser tree 已過期。**

因此本輪沒有對 Antigravity tree 做批量 kill，避免破壞仍可 resume 的 session。

### 2026-10-04 Antigravity baseline 更新

AgentDock 已停止使用上述 refined / Google ACP runtime 作為 AGY provider，改成 hardened `antigravity-acp` → AGY CLI 單一路徑。新的 AgentDock profile 使用 isolated HOME，只共享 Antigravity CLI authentication / conversation state，不載入互動 AGY 的 MCP config。

實測：

- isolated HOME 的 `agy mcp list` 為空；
- 同一個 Gemini low prompt 在隔離前曾因互動 MCP 啟動超過 25 秒仍未完成，隔離後約 5–10 秒完成；
- AgentDock E2E prompt 正常 `end_turn`；
- session close/delete 後沒有對應 AGY task child；
- 不再為每個 Antigravity delegated turn 預設 spawn `chrome-devtools-mcp` / codebase-memory MCP。

因此 2026-10-03 的 30 個 `localharness_external` / 23 個 `chrome-devtools-mcp` 數量保留作**舊架構歷史證據**，不再代表目前 Antigravity runtime baseline。Browser Broker 的 ownership / lease 設計仍有效：未來 Antigravity 若需要 browser，必須像其他 ACP 一樣向 AgentDock Browser Broker 取得 lease，而不是恢復 per-session browser MCP。

## 2026-10-03 Browser Backend Bake-off

本輪另以相同 Mac / browser environment 做 browser backend 實測：

### chrome-devtools-mcp 1.7.0

- AgentDock-owned `--isolated --headless`：成功；
- attach Chrome Remote Debugging：成功；
- attach Edge Remote Debugging：成功；
- AgentDock 移除 dynamic MCP 後，owned MCP / headless Chrome process 可一起回收；
- external Chrome / Edge 不會因 connector removal 被關閉。

### Existing Chrome / Edge

目前本機：

```text
Chrome → 127.0.0.1:52338
Edge   → 127.0.0.1:9222
```

兩者新版 Remote Debugging 都存在：

```text
/json/version → 404
```

AgentDock native browser 目前無法直接 attach 這兩個 endpoint。

### Newer chrome-devtools-mcp compatibility

較新的 `chrome-devtools-mcp` 在本機 default-profile Remote Debugging 測試出現：

- Chrome `--autoConnect` 找不到 `DevToolsActivePort`；
- Chrome direct WebSocket timeout；
- Edge direct WebSocket timeout。

因此 Browser Broker 不得把：

```text
existing default-profile Chrome / Edge Remote Debugging
```

當成唯一主路徑。

正式設計採：

```text
company workspace
→ existing authenticated Edge profile (required route)

non-company/default
→ managed isolated/headless Chrome via chrome-devtools-mcp

existing Chrome attach / other user-owned browser attach
→ explicit compatibility backend only when requested
```

因此「existing browser attach 不是唯一主路徑」不代表 company workspace 可以改走 managed Chrome；company workspace 的 authenticated Edge 是獨立的 required routing rule。

版本升級必須跑 regression matrix，不採 `@latest` 無驗證升級。

## Browser Broker Target Architecture

```text
ACP A ─┐
ACP B ─┼─→ AgentDock Browser Broker
ACP C ─┘          │
                  ├─ managed-headless Chrome worker (default non-company)
                  ├─ company Edge profile connector (required company route)
                  ├─ optional persistent-auth worker
                  └─ explicit external Chrome/other connector
```

AgentDock 統一持有少量用途明確的 browser workers，而不是每個 ACP session 自己 spawn：

```text
localharness
→ chrome-devtools-mcp
→ browser
```

Browser engine 可以共用，但 logical browser session 必須隔離。

## Multi-ACP Concurrency Model

多個 ACP 並行時，每個 browser consumer 都取得自己的 lease：

```text
ACP A → Lease A → Context A → Page A
ACP B → Lease B → Context B → Page B
ACP C → Lease C → Context C → Page C
```

每個 lease 至少包含：

```text
browser_lease_id
owner_task_id
owner_acp_session_id
worker_id
browser_context_id
page_id
profile_class
foreground_policy
created_at
last_active_at
expires_at
cleanup_state
```

### 禁止 global selected-page semantics

ACP-facing API 不直接要求模型維護：

```text
select_page(...)
then click(...)
```

因為 shared MCP server 下容易形成：

```text
ACP A selects page 3
ACP B selects page 5
ACP A next action accidentally targets page 5
```

Broker-facing API 應改成：

```text
browser.acquire(...)
browser.navigate(lease_id, ...)
browser.snapshot(lease_id, ...)
browser.click(lease_id, ...)
browser.release(lease_id)
```

Broker 在每次 operation 前解析 lease → context/page target。

### Concurrency / queue

初始設計值可先採保守上限，再由壓測調整：

```text
managed headless:
  max_concurrency = 4

persistent authenticated profile:
  max_concurrency = 2

external user-owned browser:
  max_concurrency = 1~2
```

超出上限：

```text
queue
→ wait for lease release
→ bounded timeout / cancellation
```

不得以「多 ACP 就多 spawn browser/MCP」無界擴張。

### Application / resource write locks

Browser isolation 只能避免 tab/context 互踩，不能避免同一 SaaS resource 的 write race。

因此需要：

```text
browser lease
site lock
resource lock
```

例如：

```text
CMS article A + GitLab
→ parallel OK

CMS article A + CMS article B
→ policy-dependent

CMS article A + CMS article A
→ serialize
```

## No-Focus / User-Screen Contract

使用者可正常使用 Mac 是 Browser Broker 的硬需求。

### Managed browser

預設：

```text
headless = true
visible_window = false
foreground = forbidden
```

這是一般 Web / UAT 的主路徑。

### Company Edge / existing user-owned browser

公司 workspace 預設 attach 使用者現有 Microsoft Edge 與其 authenticated profile；這條規則同時適用於上游 ChatGPT 自己的 UAT 與 ACP delegated UAT。非公司工作不得因「已有 Edge 可用」就自動使用此 profile；只有 company routing 或使用者明確要求才可使用。

attach 後：

- 不操作 user-owned tab；
- 新 tab 必須 `background=true`；
- page selection 必須 `bringToFront=false`；
- AgentDock 只操作自己 lease 的 page；
- 不 resize / activate browser window；
- release 後只關 AgentDock-owned tab / context；
- 不關 browser process、不刪 user profile。

本輪實測 `new_page(background=true)` 可在 Edge 保持 visible active tab 不變。

### Computer Use escalation

如果 browser task 需要 native browser chrome / extension popup / file dialog：

```text
Browser Broker
→ cannot complete
→ Computer Control Broker
```

但 Computer Use 可能要求 foreground，不能靜默升級。完整規則見 [computer-use-backends.md](computer-use-backends.md)。

## Problem Statement

目前至少混在一起的狀態有：

```text
persisted session
loaded ACP session
running prompt
ready/idle ACP session
attached browser MCP
spawned browser runtime
detached browser runtime
temporary profile
external persistent profile
```

現有 diagnostics 無法可靠回答：

1. 這個 `chrome-devtools-mcp` 屬於哪個 AgentDock session？
2. 這個 browser process 是 AgentDock-owned 還是 external/user-owned？
3. session 已 end_turn 時 browser MCP 是否仍需保留？
4. session/close 後 localharness / MCP / browser child 是否真的回收？
5. idle-managed session TTL 到期時，browser resources 應如何一起 drain？
6. app crash / forced quit 後，下次啟動如何辨識 stale owned processes？
7. temporary profile 是否仍被任何 live process 使用？
8. 同一 external CDP endpoint 是否被重複註冊多個 connector？

## Ownership Model

Browser Routing 實作前先建立明確 ownership：

### external-persistent

主要實例是 company workspace 必須使用的使用者既有 Microsoft Edge profile。該 profile 已登入公司 GitLab、Cloudflare 與相關後台，是 company UAT 的必要依賴，而不是可任意替換的 browser preference。

- AgentDock 不擁有 browser process；
- AgentDock 只擁有 attach session / MCP client；
- close AgentDock session 不得關閉 Edge；
- disconnect 時只回收 connector / target ownership；
- 不刪 profile。

### agentdock-isolated

一般任務的 isolated headless Chrome。

- AgentDock 擁有 browser process；
- AgentDock 擁有 temporary profile；
- AgentDock 擁有 MCP connector；
- session lifecycle 結束時必須一起 close；
- crash recovery 可依 owner metadata 判定並清理。

### adapter-owned

由 ACP adapter / local harness 啟動的 browser MCP 或 browser child。

- AgentDock 不直接寫 adapter-specific `pkill`；
- Adapter 必須提供 session close 後的 child recycle contract；
- AgentDock diagnostics 仍需能觀察 adapter PID / child count；
- 若 adapter 無法提供 per-session ownership，M7 必須視為 capability gap，而不是猜測清理。

## Required Runtime Metadata

每個 AgentDock-owned browser attachment 至少記錄：

```text
browser_session_id
browser_lease_id
owner_type: external-persistent | agentdock-isolated | adapter-owned
owner_session_id
owner_task_id
owner_profile_id
worker_id
browser_context_id
page_id
browser_pid
connector_pid
cdp_endpoint
profile_path
foreground_policy
created_at
last_active_at
expires_at
lifecycle_policy
cleanup_state
cleanup_reason
cleanup_error
```

對 external browser：

- `browser_pid` 可以觀察，但不得成為 kill target；
- `profile_path` 不得被 AgentDock cleanup 刪除。

## Lifecycle Contract

### isolated task

```text
browser start
→ connector attach
→ task active
→ task/session terminal settlement
→ connector close
→ browser graceful close
→ wait bounded timeout
→ force-kill only AgentDock-owned PID if required
→ verify port/process gone
→ delete temporary profile
```

### external Chrome / Edge attach

```text
resolve explicit registered endpoint
→ attach connector
→ create AgentDock-owned background page / context
→ bind lease
→ task/session active
→ release lease
→ close AgentDock-owned page / context
→ connector close when worker idle policy requires
→ keep Chrome / Edge running
→ keep user profile
```

### ACP ephemeral

```text
ACP session new(ephemeral)
→ ACP adapter
→ AGY CLI（預設不載入互動 MCP）
→ optional Browser Broker lease
→ prompt
→ end_turn
→ orchestrator completion verification
→ release browser lease
→ session/close
→ adapter drains AGY child
→ AgentDock diagnostics confirm loaded resources return to baseline
```

### ACP idle-managed

```text
ready
→ idle TTL
→ no prompt / pending interaction / async work
→ release idle browser lease before or during session close
→ session/close
→ adapter resource drain
→ Browser Broker lease cleanup
→ later open/resume/load
```

## Startup / Crash Recovery

AgentDock 啟動時只清理 **可證明為 AgentDock-owned 且 owner 已不存在** 的資源。

禁止：

```text
pkill Chrome
pkill chrome-devtools-mcp
rm -rf /tmp/*profile*
```

建議：

1. 讀取 owner metadata；
2. 驗證 recorded PID executable / start identity；
3. 驗證 owner session 是否仍 loaded / resumable；
4. 對 external-persistent 永遠 skip destructive cleanup；
5. 對 stale agentdock-isolated 執行 bounded graceful cleanup；
6. profile 只有在 browser/connector 都確認退出後才刪除。

## Worker / Connector Deduplication

同一 external CDP endpoint 不應長期存在多個功能重疊 connector。

M6 需要：

- canonical endpoint identity；
- duplicate registration detection；
- health probe；
- tested version / transport selection；
- obsolete connector migration / removal；
- shared worker pool；
- lazy startup / idle shutdown；
- per-worker concurrency metrics；
- UI 顯示目前實際被使用的 connector。

2026-10-03 的 Edge 實測結果：

```text
browserUrl/http discovery
→ invalid for current Edge toggle mode

wsEndpoint
→ valid
```

現有 `edge-devtools-profile-ws` 只視為 transitional compatibility connector；Browser Broker 完成後應納入 registry / worker lifecycle，不再作 ad-hoc standalone path。

## Diagnostics

M6 / M7 diagnostics 至少加入：

```text
browser_processes_total
browser_processes_agentdock_owned
browser_processes_external
browser_connectors_loaded
browser_connectors_idle
browser_workers_total
browser_workers_busy
browser_leases_active
browser_leases_queued
browser_lease_timeouts
browser_temp_profiles
browser_stale_candidates
browser_cleanup_failures
browser_foreground_violations

acp_loaded_sessions
acp_idle_ready_sessions
adapter_process_pid
adapter_child_process_count
adapter_browser_mcp_count
```

UI 必須避免把：

```text
48 persisted sessions
```

誤解為：

```text
48 live browser runtimes
```

同樣也不能把 `ready` 直接解讀成「可安全 kill」。

## Current Implementation Checkpoint — 2026-10-04

M6 Browser Broker / Computer Control Broker 已完成 runtime、lifecycle、diagnostics 與 stress validation：

| Step | Status | Commit / State |
| --- | --- | --- |
| Contract | Completed | `c14b21ca` |
| Managed engine | Completed | `8edcecef` |
| Profiles / route planner | Completed | `c08dc81b` |
| Lease isolation | Completed | `449169aa` |
| Concurrency / queue / TTL baseline | Completed | `bd010aa2` |
| External Edge attach safety | Completed | `8c8db73b` |
| ACP Broker integration | Completed | `2b0f58bd` |
| Computer Control Broker integration | Completed | `13c9cd9a` |
| Lifecycle hardening | Completed | `9b6ece24` |
| Ownership diagnostics / manual cleanup | Completed | `af49037b` |
| Concurrency / adapter lifecycle stress | Completed | `dc46459e` |

目前已固定的 runtime 邊界：

- Browser / Computer capability 都由 AgentDock host 擁有，ACP 只拿 session-scoped loopback MCP capability；
- Browser lease operation 不依賴 global selected page；company Edge 仍 fail closed，不 fallback 到 managed Chrome；
- Codex / Antigravity ACP child 不再自帶 `chrome-devtools-mcp`、`cua-repl`、`node_repl` / Sky Computer Use；
- Computer Control default provider 為 Orca，`foreground=forbidden` 預設 fail closed，Browser task 不會 silent fallback 到 Computer Use；
- `antigravity-acp 1.2.0-agentdock.5` 支援 Browser + Computer 兩個 AgentDock host-owned MCP，standalone `agy` 全域能力不受影響；
- 30 秒 lifecycle runner、managed/external 5 分鐘 idle TTL、bounded cleanup recovery 與 acquire-orphan recovery 已落地；
- `browser_broker` diagnostics 可追 owner → lease → worker → page/context → connector/process ownership，且不輸出 capability token；
- 8-owner Broker stress 驗證 4 active + 4 queued，另有 4 個真 managed MCP/Chrome 同時 lease 的 live isolation stress；20 次真實 Antigravity session lifecycle 驗證 capability/process descendants 回到 baseline；
- Microsoft Edge external live test 驗證 user sentinel page 與 browser/CDP process 保留，只清 AgentDock-owned page/connector；Darwin hard-crash integration 另驗證 host 被強制終止後 managed MCP owned process group 自行回到 baseline，不需 global `pkill`。

## Handoff Work Items

### M6 — Browser Broker（completed）

M6 已交付 browser ownership / lease model、`chrome-devtools-mcp@1.7.0` engine pin、company required external Edge routing、managed isolated Chrome、external user-tab protection、endpoint canonicalization、bounded admission/queue、lease-scoped page resolution、resource locks、TTL/stale recovery、Broker diagnostics 與 manual AgentDock-owned cleanup。

重要實作細節：

- managed route 使用 **exclusive worker generation per lease**；external route 同樣使用 per-lease AgentDock-owned connector generation，不因 endpoint dedupe 而共用 selected-page state；
- endpoint dedupe 發生在 catalog identity 層：不同 connector ID 不得指向同一 canonical endpoint；
- cleanup/recovery 只對 AgentDock-owned worker/connector/page 有 authority；external browser PID 永遠只是 observation；
- terminal metadata 暫留供 idempotent release 與 diagnostics 使用，實體 browser/MCP resource 已回收；metadata retention/pruning 可在後續 observability policy 再定義。

### M7 — ACP Manager

- 將既有 `browser_broker` / `computer_broker` diagnostics 納入 ACP session diagnostics；
- persistent / ephemeral / idle-managed policy 決定何時保留或釋放 host-owned capability；
- ephemeral session terminal 後自動 session/close，並驗證既有 Broker owner/lease 回到 baseline；
- idle-managed sweeper 觸發 adapter resource drain 與既有 host capability release；
- 明確區分 persisted / loaded / running / idle；
- adapter process recycle 仍由 adapter fork 負責，不在 AgentDock Core 寫 Antigravity / Codex 專屬 process-name kill；
- 不重新建立 ACP-owned browser backend，也不繞過 Computer Control Broker。

## M6 Verification Result

M6 code-level acceptance 已完成 unit / integration / race / cross-build / live-provider 驗證；managed route另有 4-worker live concurrency stress 與 host hard-crash process-group cleanup regression。真實 Microsoft Edge 測試已證明 external browser process 與 user sentinel page 不會被 AgentDock release；但「公司實際已登入 default-profile Edge」能否 attach 仍取決於部署時是否提供健康、已驗證、authenticated 的 configured connector。這是 runtime deployment qualification，不會以 managed Chrome fallback 掩蓋。

此外，舊 public `browser_session` / `browser_act` / `browser_snapshot` native-CDP 工具仍保留作相容層；M6 的 host-owned Broker contract、ACP Browser capability 與 `browser_broker` control plane 已完成，但不宣稱所有 legacy browser callers 已在本 milestone 全數改寫成 Broker API。

## Acceptance Criteria

1. 公司 workspace 的 browser/UAT task，無論由上游 ChatGPT 自己執行或 ACP delegated 執行，都預設使用使用者既有 Microsoft Edge 與 authenticated user profile；Edge connector 不可用時 fail closed，不 silent fallback 到 Chrome。
2. 非公司 browser task 除非使用者明確要求自己的 profile/browser，否則一律由 chrome-devtools-mcp + AgentDock-managed isolated/headless Chrome 執行，不產生 visible window。
3. 4~8 個 ACP 可同時取得獨立 lease / context / page，不互相改變 target。
4. ACP 不直接依賴 global selected page。
5. concurrency 超出上限時 queue，而不是無界 spawn MCP / Chrome。
6. 同一 resource 的 write task 可被 lock / serialize。
7. isolated browser task 完成後，不留下 AgentDock-owned Chrome process 或 temporary profile。
8. crash/restart 後可回收可證明 stale 的 AgentDock-owned browser / lease。
9. company Edge / external browser attach 結束後 browser 本身仍保持執行。
10. company Edge user profile 與 user-owned tabs 永遠不被 cleanup / navigation / close；AgentDock 只操作自己 lease 的 background page。
11. background page workflow 不改變 visible active tab / frontmost app。
12. 同一 CDP endpoint 不會因重複設定長期啟動多套等價 connector。
13. 20 次真實 Antigravity ACP session new/close/runtime close 後，host capability owner 與 adapter/browser/computer-control descendants 回到基線；M7 另驗證 prompt-driven ephemeral auto-close policy。
14. persistent / resumable session 不因 idle age 被誤殺。
15. diagnostics 能從 ACP session → browser lease → worker → context/page → process 追蹤 ownership。
16. cleanup failure 有 bounded retry 與可讀錯誤，不 silently leak。
17. macOS / Windows lifecycle contract 一致，平台差異只放 adapter。
18. browser engine 版本升級必須通過 managed-headless / persistent-profile / Chrome-live / Edge-live compatibility matrix。
19. Browser task 不會 silent fallback 到 foreground Computer Use。

## Non-goals

本項目不做：

- 關閉使用者自己的 Chrome / Edge；
- 以 process name 全域 kill 作正式方案；
- 依 CPU idle 或 process age 猜測 session 已失效；
- 讓每個 ACP session 自己常駐一套 `chrome-devtools-mcp`；
- 把使用者 default-profile Remote Debugging 當唯一 browser backend；
- 讓 Browser Broker 靜默升級成會搶焦點的 Computer Use；
- 在 AgentDock Core 寫 Antigravity-specific / Codex-specific kill hack；
- 為了宣稱架構一致而刪除仍需相容的 legacy native-CDP public tools。
