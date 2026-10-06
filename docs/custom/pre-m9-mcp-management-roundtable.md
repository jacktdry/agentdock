# Pre-M9 Wave 3 — MCP Management UX / IA + Authority Contract

> 狀態：**Roundtable frozen for implementation（2026-10-06）**
>
> 前置：P4 ACP Manager 已 closeout 並部署至 AgentDock Next。此文件定義 Wave 3 第二個 domain「P5 MCP Management」的 Shared Desktop 產品、Desktop API、安全與 runtime contract。它不是 `mcp_manage` CLI 的 UI wrapper，也不提前進入 P6 Plugin Management 或 M9。

## 1. 目標與邊界

MCP Management 的主要工作是讓使用者安全回答並完成：

- AgentDock Next 目前有哪些 MCP servers？
- 哪些是自己新增的 standalone MCP，哪些由 Plugin 提供？
- server 是停用、尚未連線、可用、需要登入、正在登入，還是需要處理錯誤？
- 如何新增或修改 standalone MCP 的連線設定？
- 如何安全設定 credential / environment binding，而不把 secret 暴露在 UI、logs 或 registry？
- 何時需要重新連線與重新探索 tools？
- Remote MCP 何時需要 OAuth，以及授權是否真的完成？
- 哪些設定屬於 Plugin owner，應留到 P6 管理？

不可破壞的邊界：

- stable `mac-dev` 仍是 Next repository / build / install / service / tunnel / live-runtime mutation 的唯一開發 control plane。
- `macbook-air-m3` 只作 Next connector / runtime validation；不得自我修改、重裝或修復。
- Shared frontend 不直接讀寫 `~/.agentdock-next` registry/env files，不直接執行 command，不直接建立 OAuth client。
- Core / Desktop API 是 MCP management authority；frontend 只送 typed intents。
- secrets、OAuth tokens、raw provider errors/stderr、secret-bearing URLs/args 不得進普通 frontend store、Activity、Execution、Diagnostics、logs 或 Memory。
- P5 **只管理 standalone MCP**。Plugin-owned MCP 可列出與顯示 owner/status，但 configuration、enable/disable、environment、authorization、remove 都是 read-only；真正 lifecycle/configuration 留給 P6 Plugin Management。
- P5 不提供 MCP marketplace、Plugin install/update、bulk operations、tool execution console、per-tool permission editor、raw schemas/logs、process manager、Connection/Tunnel settings。
- `memory` 不 hard-code 特例；若 source 是 standalone，就依同一 contract 呈現。

## 2. 現況盤點

Core 已有 `internal/tool/mcp` / `internal/mcp/client`：

- list / inspect；
- add；
- remove；
- enable / disable；
- `env_set` / `env_unset` / `env_list`；
- refresh；
- OAuth authorize / auth_clear；
- streamable HTTP / stdio；
- optional protocol compatibility pin；
- tool catalog / search / inspect / call。

但這些能力 **不能直接等同 Shared Desktop contract**：

1. `Add` 是 create-only，沒有 atomic edit / replace。
2. registry file lock 防 concurrent file corruption，但沒有 stale-editor compare-and-save revision。
3. `List` 在 registry sync error 時可回 cached inventory；Shared UI 不能把 cached data 當 authoritative mutation basis。
4. `Refresh` 會 teardown/reconnect；stdio 可啟動 local process，HTTP 可產生 network effects，不是 passive refresh。
5. refresh 現況在建立新 client 前沒有把 old-client close failure 當 blocker。
6. standalone HTTP transport 目前允許非-loopback HTTP，而 Plugin MCP 已採非-loopback HTTPS-only。
7. configured HTTP headers 由 transport 每次 RoundTrip 重加；預設 HTTP redirect 若跨 origin，存在 credential forwarding 風險。
8. OAuth flow 沒有 Desktop-facing opaque flow identity / generation fencing；late exchange 必須不能在 clear/remove/recreate 後把 grant 寫回。
9. current Runtime API `runtimeMCPRequest` 缺 `protocol_version` 欄位，不能完整表達 Core public tool contract。
10. existing Core DTO/error strings不應直接送 renderer；其中 URL、args、provider error 可能含 credential。
11. environment mutation沒有獨立 revision/application result。
12. remove standalone server不會自動刪除 scoped environment values；相同 name 重建時，殘留 values 可能被新 runtime讀取。

因此 P5 先建立 **Desktop-owned management authority layer**，再做 Shared UI。

## 3. Page goal / information hierarchy

P5 使用單一 **MCP Management** page，階層固定：

1. **Summary**
   - standalone count
   - plugin-owned count
   - available / needs-attention count
   - authorization-required count
   - passive **Reload status**
2. **Servers**
   - all safe inventory rows/cards
   - Primary action：**Add server**
   - filters 可後補，不作 P5 blocker
3. **Server detail**
   - Overview
   - Connection configuration
   - Credentials / environment
   - Authorization
   - Known tools
4. **Advanced**
   - protocol pin
   - timeout
   - command args / cwd for stdio
   - safe diagnostics metadata

Summary 的 Reload 只能重新取得 authoritative Desktop snapshot，不得偷偷 reconnect MCP。

## 4. Server list / safe status language

每個 server row/card顯示：

- display name / stable server name；
- description；
- source badge：Standalone / Plugin；
- Plugin owner name（若有）；
- transport：Remote HTTP / Local process；
- enabled；
- observed runtime state；
- known tool count，且 **unknown 必須和 0 分開**；
- last successful observation time；
- safe error category / code；
- action-required summary。

面向一般使用者的 status 文案：

- **Not connected yet**
- **Available**
- **Disabled**
- **Sign-in required**
- **Signing in**
- **Needs attention**
- **Status unavailable**

「Available」只代表最近一次成功 observed connection/tool catalog，不宣稱持續健康。

Plugin-owned rows保留清楚 owner badge與說明：「由 Plugin 管理；請到 Plugin Management 變更設定。」P5 不顯示假的 disabled management controls。

## 5. Standalone Add / Edit flow

### 5.1 Step 1 — Identity

必填：

- server name
- description
- transport

Name 建立後視為 immutable identity。Rename 不在 P5；若未來需要，必須是有明確 credential/env migration contract 的獨立操作，不能用 remove + add 偽裝。

新 server 預設建議 **disabled**，讓使用者先完成 config/credential review，再 explicit Enable。

### 5.2 Remote HTTP

Default fields：

- MCP endpoint
- Enabled（建立後另行操作較佳）

Advanced：

- protocol compatibility pin
- timeout
- header → environment binding

Shared write policy：

- absolute HTTP(S) URL only；
- userinfo / fragment 拒絕；
- loopback 可用 HTTP；
- **非 loopback 必須 HTTPS**；
- P5 editor 對新/修改 endpoint **不接受 query string**。CLI/runtime 可保留更進階相容性，但 Shared UI不把可能含 secret 的 query帶進普通 renderer state。
- existing legacy URL 若含 query，只回傳 protected/redacted metadata，允許 preserve，不回傳 raw query；要修改 endpoint 時必須輸入新的安全 URL。

### 5.3 Local process / stdio

Default fields：

- executable / command
- Enabled

Advanced：

- args
- cwd
- child env → host env mapping
- protocol pin
- timeout

UX 必須明確告訴使用者：

- stdio MCP 會以**目前本機使用者權限執行程式**；
- AgentDock 不透過 implicit shell，但 interpreter/package runner 本身仍可能執行或下載 code；
- cwd 必須是 absolute path；
- Enable / Reconnect 可能啟動 local process。

現有 args 若 backend判定可能含 credential，snapshot 只標 `protectedArgs=true` 並 preserve；raw args 不送 renderer。P5 新輸入若偵測到明顯 secret shape，拒絕存進 args並引導使用 environment binding。

## 6. Credentials / environment UX

UI 必須區分兩種概念：

1. **Reference host environment variable**
   - registry只存 variable name mapping；
   - value 來自 host environment；
   - UI 不知道也不宣稱 value；
2. **Server-scoped value**
   - value 存於 AgentDock MCP-scoped environment store；
   - ordinary snapshot只回 `key + configured/missing/unknown`；
   - value永遠 write-only。

Rules：

- secret輸入只存在於 local form draft，提交後/關閉 editor立刻清除；
- 不提供 reveal saved value；
- replace 使用新的 masked input；
- unset 要 confirmation；
- server removal預設 **保留 scoped values**，並在 confirmation 明確說明；
- credential purge 是獨立 explicit confirmed action；
- 只要存在 retained values，新建同名 server不得靜默繼承後直接 Enable。Backend snapshot要標示 retained-credential state，使用者必須 explicit reuse 或 purge/replace後才能 Enable；
- 不把 scoped env 描述成 encrypted vault；它依賴 private filesystem permissions；
- Authorization header mapping代表完整 header value，UI需說明是否要包含 `Bearer ` prefix；
- explicit Authorization mapping優先於 automatic OAuth。

## 7. OAuth UX

OAuth 只適用 streamable HTTP。

### Begin

1. frontend 呼叫 backend `BeginAuthorization`；
2. backend先驗證 server identity / revision / generation / endpoint；
3. backend提供可用 callback options；
4. frontend選 backend-issued callback ID，不能提交 arbitrary redirect URI；
5. backend回 opaque `flowId`、provider origin、expiresAt 與 safe authorization metadata；
6. backend/native layer開啟 browser；authorization URL不進 durable frontend state。

UI state：

- Sign-in required
- Preparing sign-in
- Signing in
- Callback received
- Finishing sign-in
- Authorized, reconnect pending
- Available
- Denied
- Expired
- Failed
- Cancelled

**Callback received ≠ authorized；authorized ≠ MCP ready。**

關閉 page只停止 frontend polling，不自動 clear authorization。

`Clear authorization`：

- explicit confirm；
- 清除 AgentDock local grant並關閉 MCP connection；
- 不宣稱 remote provider token已 revoke；
- generation fence必須使晚到 callback/token exchange無法把 cleared grant寫回。

OAuth callback availability是platform capability；未有 native evidence的平台option不advertise。

## 8. Passive reads vs side-effecting operations

必須嚴格區分：

### Passive

- Snapshot / Reload status
- Inspect safe detail
- Read credential-key configured states
- Read cached known tool summaries
- Read authorization flow status

Passive read **不得**：

- connect remote endpoint
- start stdio process
- refresh OAuth token
- close/recreate client
- list tools over network
- modify registry/env/grants

### Side-effecting

- Create / Update
- Enable / Disable
- Reconnect and rediscover tools
- Remove
- Set / Unset scoped value
- Purge scoped credentials
- Begin / Clear authorization

Core現有 `refresh` 在 UI 名稱固定為 **Reconnect and rediscover tools**，不可標成 Refresh。

Tool inventory page只顯示已有 cached catalog；如果從未 discovery，顯示 Unknown / Not discovered，不為了畫面自動連線。

## 9. Confirmation / warning policy

| Action | Policy |
| --- | --- |
| Add disabled server | Save；若 stdio/remote risk text已在 form中，不需二次 modal |
| Enable | Confirm；說明 tools會變可用，HTTP會連外，stdio可能啟動程式 |
| Disable | 若目前 Available/authorizing，Confirm；說明 connection/process會關閉 |
| Edit config | Save，不需要 generic modal；重大 transport/origin/command變更在 inline warning中明示 |
| Reconnect and rediscover | Confirm；可能中斷 current client並啟動 local/remote connection |
| Remove | Confirm；說明 unregister/close/OAuth local grant removal、scoped env預設保留 |
| Set scoped credential | explicit masked Save；不回顯 value |
| Unset scoped credential | Confirm |
| Purge retained scoped credentials | Confirm |
| Begin OAuth | explicit Sign in；顯示 target origin + callback route |
| Clear authorization | Confirm |

Destructive modal初始 focus 在 Cancel；Enter 不應在 ambiguous state直接確認 destructive action。

## 10. Ownership model

Snapshot 必須以 Core fields判斷 ownership：

- `sourceType=standalone`
- `sourceType=plugin` + `pluginName`

禁止依 name prefix / path推測。

P5 backend 對 plugin-owned server：

- read summary/detail：允許；
- cached tool summaries：允許；
- create/update/remove：拒絕；
- enable/disable：拒絕；
- env set/unset/purge：拒絕；
- authorize/auth clear：拒絕；
- reconnect：拒絕。

即使底層 `mcp_manage` 今天對部分 plugin-owned action technically可呼叫，P5 Desktop authority也必須 fail closed。P6 再定義 plugin lifecycle/config authority。

## 11. Revision / generation contract

P5 不直接沿用無 revision 的 `mcp_manage` mutations。

至少需要：

- **registryRevision**：standalone registry / plugin ownership inventory變更即改；
- **serverGeneration**：create/recreate、ownership變更、config replace即改；
- **environmentRevision**：server-scoped env keys/configured state變更即改；
- **authorizationGeneration**：begin/clear/remove/recreate時遞增或換 opaque generation；
- optional cached **toolCatalogRevision**。

所有 mutation傳 expected revision/generation並在 effect前再次驗證。

Stale mutation：

- 不 dispatch；
- 回 conflict；
- frontend reload authoritative snapshot；
- editor draft保留，但 draft仍綁 opening revision，不能因 reload自動升級後覆蓋別人的修改。

Server delete + same-name recreate必須靠 generation區分，不得讓 late reconnect/OAuth/env mutation作用到新 server。

## 12. Desktop API frozen proposal

### Read DTOs

`Snapshot()`

- revision
- observedAt
- authoritative / stale flag
- server summaries
- counts
- Core availability
- safe error

`Inspect(serverName)`

- registryRevision / serverGeneration / environmentRevision
- safe config
- protected flags
- ownership
- environment key/configured metadata
- authorization observation
- cached tool summaries + toolCatalogRevision/freshness
- runtime observation
- blockedReasons

Raw secrets/provider stderr/raw remote response不得出現。

### Mutation families

- `CreateStandalone(request)`
- `UpdateStandalone(request)` — atomic replace primitive；不是 remove+add
- `SetEnabled(request)`
- `Reconnect(request)`
- `RemoveStandalone(request)`
- `SetEnvironmentValue(request)`
- `UnsetEnvironmentValue(request)`
- `PurgeEnvironment(request)`
- `BeginAuthorization(request)`
- `AuthorizationStatus(flowId)`
- `ClearAuthorization(request)`

Common mutation result至少表達：

- completed
- persisted
- runtimeApplied
- runtimeImpact
- restart/reconnectRequired（若語意適用）
- recoveryRequired
- outcomeUnknown
- authoritative server/snapshot projection（若可安全提供）
- safe `APIError`

Frontend遇 `outcomeUnknown=true` 必須先 reload，不得 blind retry。

## 13. Core adaptation requirements before UI

P5 backend implementation開始時先完成／驗證：

1. atomic standalone Update / replace；
2. authoritative read API，registry read failure不能 fallback成可 mutation 的 cached state；
3. revision / server generation / env generation；
4. `runtimeMCPRequest` parity，至少補 `protocol_version`；
5. plugin-owned P5 mutation fence；
6. close-old-client failure必須阻止 reconnect replacement；
7. remote redirect credential isolation：configured headers不得跨 origin redirect；HTTPS downgrade拒絕；
8. Shared write policy：non-loopback HTTPS；
9. OAuth flow identity + generation fence，clear/remove/recreate後 late exchange不可 revive grant；
10. safe Desktop DTO / allowlisted errors；
11. retained scoped env detection / explicit reuse-or-purge gate；
12. mutation partial-outcome / lost-response reconciliation。

不能為了先出 UI 而略過這些 authority invariants。

## 14. Frontend implementation slices

Frozen component/store方向：

- `api/mcpContract.ts`
- `stores/mcp.ts`
- `features/mcpEditorLogic.ts`
- `components/mcp/MCPPanel.vue`
- `MCPServerList.vue`
- `MCPServerDetail.vue`
- `MCPServerEditor.vue`
- `MCPEnvironmentBindings.vue`
- `MCPOAuthStatus.vue`
- `MCPToolSummaryList.vue`

Navigation新增 MCP Management primary section，但不把 Plugin Management提前塞進同頁。

Store responsibilities：

- authoritative inventory
- stale/read-error state
- pending mutation de-duplication
- opening revisions
- mutation reconciliation
- bounded OAuth status polling
- cancel polling on unmount/terminal state
- secret values不進 Pinia durable state

## 15. Error / recovery UX

必須區分：

- validation
- conflict / stale revision
- owned by plugin
- credential required
- auth required / denied / expired
- network / TLS
- protocol mismatch
- local executable/cwd unavailable
- process cleanup failed
- registry unavailable
- runtime unavailable
- mutation outcome unknown
- recovery required

遠端 text一律當 untrusted text，長度 bounded，不 render HTML。

Read failure：
- retained prior snapshot可標 **Stale / unavailable**；
- 所有 mutation disabled；
- Retry只重做 passive snapshot。

Mutation failure：
- 若 backend證明無 effect，可直接顯示 error；
- 若 outcome unknown，先 reconcile；
- partial persistence/cleanup必須 truthful，不用 generic “failed” 抹掉已發生的 durable change。

## 16. Accessibility / narrow window

- keyboard 完整可達；
- persistent form labels；
- field error與 input以 aria relationship關聯；
- status不用顏色當唯一訊息；
- operation result用 polite live region；
- dialogs focus trap + close後 focus restore；
- Escape = cancel；
- destructive confirm初始 focus Cancel；
- long URL/path/name可 wrap；
- 320 CSS px等價窄寬採 single-column；
- list → detail時窄視窗提供 **Back to servers**；
- advanced fields預設 collapsed，保持 progressive disclosure。

## 17. Verification gates

### Backend/security fixtures

- concurrent writers / stale editor
- delete + same-name recreate generation
- plugin ownership change during mutation
- passive Snapshot/Inspect不連線、不啟動process、不refresh token
- close failure blocks reconnect
- redirect不能帶 credential到新 origin或降級
- secret canaries不出 DTO/error/log/Activity/Execution
- protected args/URL preserve
- retained env values不能 silent reuse
- env revision conflict
- OAuth clear/remove/update vs late callback/exchange race
- callback mismatch/replay/expiry/issuer/resource
- lost mutation response / outcome reconciliation

### Frontend

- unavailable ≠ empty
- stale snapshot disables mutations
- HTTP vs stdio editor validation
- plugin-owned read-only
- confirmations
- secret draft cleanup
- revision conflict keeps draft pinned
- OAuth polling cleanup
- unknown tool count ≠ zero
- narrow-window / keyboard semantics

### Cross-platform

Source + cross-build不是 native acceptance。至少保留後續 native evidence：

- macOS/Linux process-tree teardown + executable/path permissions
- Windows process-tree / ACL / cwd / executable lookup
- IPv4/IPv6 loopback handling
- native browser opening
- Windows/WSL OAuth callback reachability

Release-native signed GUI UAT仍屬 M9/P11 gate。

## 18. Roundtable decisions / deferred scope

已凍結：

- P5 = standalone MCP management + plugin-owned read-only inventory；
- Shared UI採安全子集，不追求 CLI等價自由度；
- non-loopback Shared remote endpoint write要求 HTTPS；
- Shared endpoint editor不接受 query；legacy query preserve-only/redacted；
- Reconnect是 side-effecting explicit action；
- add預設 disabled；
- atomic edit必須 backend primitive；
- scoped credential default remove-retain，但 same-name reuse必須 explicit；
- OAuth flow採 opaque ID + generation；
- no offline writes；
- no automatic tool discovery；
- no raw tool-call console；
- no special-case `memory`。

Deferred：

- force-disconnect（v1先採 bounded drain/cleanup fail closed）；
- bulk management；
- rename；
- marketplace/discovery；
- Plugin mutation（P6）；
- per-tool permission UI；
- offline mutation；
- release-native cross-platform GUI UAT。

## 19. Implementation order

```text
P5 contract freeze
  ↓
A. Core safety + revision/generation + atomic update
  ↓
B. Desktop DomainMCP authority + safe DTOs
  ↓
C. Shared store/navigation/components/i18n
  ↓
D. integration + security review
  ↓
E. Next-only package/live validation through stable mac-dev
  ↓
P5 closeout
  ↓
P6 Plugin Management roundtable
```

同一階段的 writer不可共享 unsafe write boundary。Core safety/revision先於 Desktop API；Desktop API contract freeze後才開始 Shared UI。

P5 完成不代表可進 M9。仍須完成後續 Plugin、Nexus/Platform Essentials、Browser Broker與 Pre-M9 hardening integration review。
