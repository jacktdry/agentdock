# Pre-M9 Wave 0 Product / Security / Architecture Contract

> 狀態：**Frozen for Wave 1 implementation**（2026-10-05）
>
> 適用範圍：P1 AgentDock Next Connection/Auth Readiness、P2 Public Access / Tunnel foundation。
>
> 前置：M8 已 closeout；`mac-dev-next` 尚未由使用者建立。此文件不開啟 M9，也不允許任何 stable AgentDock runtime 驗證。

## 1. Freeze 目的

Wave 0 的工作不是決定視覺樣式，而是先固定：

- stable native 能力哪些要 Port / Redesign / Native-only / Deprecated / Not needed；
- Shared Desktop 的資訊架構與 interaction contract；
- credential / destructive action 的安全邊界；
- P1/P2 backend schema、mutation semantics 與 cross-platform ownership；
- 三個 Wave 1 backend worker 可以平行寫哪些區域、哪些共同 contract 不得各自發明。

任何 Wave 1 implementation 若需要改變本文件的 frozen contract，必須先回到主 session 做 integration review；不能由單一 worker自行擴張語意。

## 2. 不可破壞邊界

- stable AgentDock / `mac-dev` 是 production control plane。
- 禁止修改、重啟、停止、替換或拿 stable AgentDock、stable Core、`~/.agentdock`、stable registry/live services 作 Next validation target。
- Next 只讀寫 Next-owned runtime/state/log/work/service namespace。
- Next 缺少 identity / ownership evidence 時 fail closed，不 fallback 到 stable。
- Desktop-control credential、OAuth token signing secret、provider credentials、Nexus device token 不進 Shared frontend。
- OAuth password 是建立 ChatGPT connector 所需的唯一可在可信任 local Shared UI 顯示的 secret；仍必須 explicit reveal/copy。
- Tunnel token 是 write-only / replace-only credential；Shared UI 不讀回既有 token。
- Wave 1 不建立或修改實際 Cloudflare account / DNS / route；部署前仍需要使用者確認 account/hostname target。

## 3. Wave 0 audit corrections

初始 parity matrix 有幾個必須修正的地方：

1. **Port ownership preflight 尚未實作。**
   - `internal/desktopapi/basic_settings.go` 目前只做數值設定流程。
   - `internal/desktopruntime/basic_settings.go` 只驗證 `1..65535` 與 log level。
   - `internal/updateplatform/darwin.go::validateNextCoreListener` 有固定 `8767` 的 update-specific ownership check，可作證據/參考，但不是可直接宣稱已完成的通用 P1 classifier。

2. **Shared Local MCP URL 尚未呈現。**
   - current Shared Connection 主要投影 tunnel status。
   - `internal/desktopapi/connection.go::publicOrigin` 刻意只輸出 sanitized HTTPS origin；不輸出完整 `/mcp` URL。

3. **Shared OAuth password 尚未提供。**
   - stable macOS/Windows native UI 有 Show/Copy。
   - Shared backend/status 不得因此直接把 password 塞進一般 snapshot。

4. **stable native 的 Bearer/Auth Token 不搬進 Shared UI。**
   - stable native UI 有顯示此值，但 M8 後 Desktop control 已有 distinct privileged credential。
   - 這個 token 對 Shared operator UX 沒有建立 ChatGPT connector 的必要性。
   - 分類：**Not needed in Shared UI / backend-only secret**。

5. **Named Tunnel 的「connected」不等於 origin 正確。**
   - current Unix Named status 只要 tunnel process running 即可標 Ready。
   - current Named runner 使用 remotely-managed `TUNNEL_TOKEN`；不像 Quick mode 使用本機 `AGENTDOCK_TUNNEL_TARGET`。
   - 因此 local port change 不能只靠既有 token runner 自動同步 Cloudflare published route 的 Service URL。

6. **Quick Tunnel 的 mutation completion 與 URL readiness 必須分離。**
   - Unix `applyQuickTunnelURLUnix` 會自己取得 `.desktop-mutation.lock`。
   - 外層若持 lock 等待 Quick URL publication，會形成 lock wait cycle。
   - Wave 1 必須把「受鎖的 local mutation」與「解鎖後 readiness convergence」分開。

## 4. Frozen parity inventory

| Capability | Stable native | Next/backend 現況 | Shared 現況 | Freeze classification / phase |
| --- | --- | --- | --- | --- |
| Runtime status/lifecycle | 有 | 有 | 有 | Complete |
| Activity / Execution | 較少 | 有 | 有 | Complete / Next-enhanced |
| Permission / Approval | native/OS 為主 | M8 有 | M8 有 | Complete / Next-enhanced |
| Core port value | 有 | 有 | 有 | P1 Port |
| Port occupancy / ownership | update path 有部分證據 | **未通用化** | 無 health state | P1 Redesign |
| Local MCP URL | 有 | backend 可導出 | 未呈現 | P1 Port |
| Public MCP URL `/mcp` | 有 | origin 可得 | 只顯示 origin | P1 Port |
| Public endpoint test | 有 | Shared 無正式 contract | 無 | P1 Port/Redesign |
| OAuth password | 有 | Next-owned env 可讀 | 無 | P1 Port with secret-safe UX |
| Stable Bearer/Auth Token reveal | 有 | backend secret | 無 | **Not needed in Shared UI** |
| Local / Quick / Named mode | 有 | Go/native backend 有 | status/actions 部分 | P2 Port |
| Tunnel token replace | 有 | backend store 有 | 無 | P2 Port, write-only |
| Tunnel autostart | 有 | backend/platform 有 | 無 | P2 Native-backed Port |
| Quick regenerate | 有 | 有 | 有 action | Keep；Quick-only |
| Named remote route management | dashboard/account 外部能力 | **未實作** | 無 | Not in Wave 1; unsupported transitions fail closed |
| Nexus endpoint/pairing/device | 有 | 有 | 無 | P8 Port |
| ACP lifecycle/health | 部分 | 有 | 有 | Complete / Next-enhanced |
| ACP profile/package CRUD | 有 | 有/部分 | 未完整 | P4 Port |
| MCP management | Core/native | backend 有 | unavailable | P5 Port |
| Plugin management | Core/native | backend 有 | unavailable | P6 Port |
| Legacy CDP selector | 有 | Browser Broker 已取代 | 無 | **Deprecated UI model** |
| Browser Broker operator UI | 無 | M6 有 | 無 | P7 Redesign |
| Core/menu startup | 有 | platform-specific | 部分 | P8 Native-backed Port |
| Chat Cards / MCP Apps mode | 有 | config capability | Shared Basic Settings 未呈現 | P8/P5 Port，非 P1/P2 blocker |
| Runtime analytics shortcut | 有 | Core 有 | 未完整 | P8 Port |
| Open Logs / Config | 有 | backend/native capability | 未完整 | P8 Native-backed Port |
| Version / update check | 有 | 有 | 有 | Complete |
| Update apply/recovery | 有 | Next backend/native | Shared native-only | M9 / Native-only backend |
| OS permission / elevation | 有 | platform-specific | 未完整 | Native-only backend |
| Tray/menu commands/status | 有 | native | Shared shell 有 tray，但 parity 未完整 | P8 Native-backed |
| Legacy runtime migration | 有 | Next intentionally isolated | 不需要 | Deprecated |
| arbitrary env injection UI | 非正式 operator flow | env files 存在 | 無 | Not added by parity alone；needs separate product decision |

## 5. Shared Desktop IA freeze

### 5.1 Navigation

P1/P2 **不大改 primary navigation taxonomy**。避免為一頁 Connection foundation 同時重構整個 App shell。

目前 primary sections 可保留：

- Overview
- Runtime
- Execution / Activity
- Connection
- ACP
- Permission
- Settings
- System
- Developer（secondary）

後續 P4–P8 可再做一次 domain-level navigation review；尤其 MCP / Plugin / Browser / Platform 進入 Shared 後，才決定是否需要 group/sidebar hierarchy。

### 5.2 Connection page goal

Connection 頁的 primary job 是：

> 讓使用者知道 AgentDock Next 是否已經具備「本機可用、可公開存取、可建立 ChatGPT connector」的必要條件，並能安全完成 Public Access 設定。

使用者不需要先理解 cloudflared、OAuth issuer、launchd/task scheduler 等底層術語。

### 5.3 Connection page hierarchy

#### A. Connection readiness summary

預設第一屏顯示：

- Local Core：Running / Stopped / Unavailable
- Core port：configured value + ownership/availability state
- Public access：Local only / Quick / Named
- Tunnel：registered / running / connected（不可合成一個模糊 Ready）
- Connector readiness：
  - Local ready
  - Public endpoint available
  - OAuth credential available
  - Public reachability last check

不得把 `cloudflared process running` 直接等同「ChatGPT Connector Ready」。

#### B. ChatGPT connector

這是 P1 的主要 operator surface：

- **Local MCP URL**
  - read-only
  - Copy
- **Public MCP URL**
  - 完整 URL，必須包含 `/mcp`
  - read-only
  - Copy
  - Test public endpoint
- **OAuth password**
  - 預設遮罩
  - Show / Hide
  - Copy
  - nearby help：「Public MCP URL + OAuth password 是在 ChatGPT 建立個人 MCP connector 時使用；不是一般 API token。」

Bearer/Auth Token 不出現在此區，也不放在 Advanced。

#### C. Public access

三個清楚的 mode：

- **Local only**
  - 不建立 public endpoint。
- **Quick Tunnel**
  - temporary `trycloudflare.com` address
  - 適合 smoke / 臨時測試
  - Regenerate 只在此 mode 出現
  - 明確提示 URL 會改變，不適合作正式 `mac-dev-next`
- **Named Tunnel**
  - fixed HTTPS public origin / hostname
  - Tunnel Token input（write-only）
  - stored credential state
  - remote route prerequisite / verification state
  - 正式 `mac-dev-next` 的推薦模式

#### D. Advanced

預設收合：

- Core port
- Tunnel autostart
- detailed observed state / last check
- platform capability / unavailable reason
- recovery-required state（若有）

高風險/低頻率設定不與 connector credentials 平鋪。

## 6. Interaction states

所有主要區塊至少支援：

- loading
- available / healthy
- stopped
- unavailable（附 safe reason）
- conflict
- unknown ownership
- applying
- waiting for readiness
- degraded
- recovery required
- retryable failure

### 6.1 Narrow window

- forms/card 改單欄堆疊；
- value + action 可換行，不以水平表格作唯一操作方式；
- secret Show/Copy、Test、Apply 在 760px minimum window 仍可鍵盤操作；
- 不依 hover 才看得到 critical action/reason。

### 6.2 Accessibility

- status 不只靠顏色；
- async result 使用適當 `aria-live`；
- Show/Hide 使用 `aria-pressed` 或等價狀態；
- disabled control 必須有 nearby reason，不只 `disabled`；
- confirm dialog focus trap / Escape / initial focus 必須一致；
- mode selection、copy、test、reveal 全部可鍵盤操作。

## 7. Permission / Approval explanatory contract

M8 UI 的 operator wording 在後續 UX polish 必須保留以下語意：

- **Allow**：該類操作在 policy 允許範圍可通過 admission。
- **Ask**：需要 explicit approval。
- **Deny**：不允許執行。
- **Approve Once**：只批准目前這筆 request 的 admission。
- **Approve Workspace**：只有 binding / policy 允許時才可用；disabled 時顯示原因。
- **Approval does not execute the original operation.**
  批准只更新 admission state；原 caller 必須正常 retry。

這是後續 Permission 頁 wording/education 的 contract，不重開 M8 authority。

## 8. Secret classification freeze

| Secret / credential | Shared status 可見 | Shared reveal | Shared replace | Logs/Activity/Diagnostics | Contract |
| --- | --- | --- | --- | --- | --- |
| OAuth password | availability state | **Yes, explicit** | No in P1 | Never | connector credential |
| Tunnel Token | stored / missing / unreadable | **Never** | **Yes** | Never | write-only |
| Desktop-control credential | Never | Never | Never | Never | backend/native only |
| Core Bearer/Auth Token | Never | Never | Never | Never | backend only |
| OAuth token signing secret | Never | Never | Never | Never | backend only |
| Nexus pairing code | later, transient input only | No persisted reveal | Yes during pairing | Never | P8 |
| Nexus device token | status only | Never | native/backend | Never | backend/native only |
| provider/plugin secrets | safe summary only | default Never | domain-specific later | Never | P5/P6 review required |

### 8.1 OAuth password reveal lifecycle

Shared renderer is treated as a trusted local signed desktop renderer for this bounded feature, consistent with the existing Pre-M9 requirement. Explicit Show is an operator UX boundary, not proof against a fully compromised renderer.

Requirements:

- ordinary status/store never contains password;
- `RevealOAuthPassword` is a dedicated call;
- returned value stays component-local, not Pinia/global persistence;
- clear on Hide, navigation away, window hide/blur where available, identity/config revision change, cancellation, and bounded timeout;
- late reveal response after state invalidation is ignored;
- reveal path is **read-only** and must never call read-or-create / rotate helpers;
- no raw backend errors that could contain env/file content.

For Copy, prefer a backend/native clipboard operation that returns success only. If platform/Wails clipboard abstraction is not introduced in P1, an explicit short-lived component-local copy path is acceptable only under the same non-persistence rules. Do not claim clipboard auto-erasure unless it is actually implemented.

### 8.2 Tunnel token lifecycle

- current token is never returned to Vue;
- status exposes only `stored | missing | unreadable | unavailable`;
- blank input means “keep existing” only when backend state says stored;
- explicit token input means replace;
- token input is cleared after success/failure/navigation;
- no token in CLI argv, diagnostics, operation metadata or API errors;
- temporary token file, if used to bridge existing backend command contract, must be Next-owned `0600`, non-symlink, bounded, and removed on every path.

## 9. Port ownership contract

### 9.1 Reserved ports

- `8765` — stable AgentDock reserved → reject.
- `8766` — Shared Memory reserved → reject.
- `8767` — AgentDock Next default.
- other `1..65535` values are candidates subject to platform preflight.

### 9.2 Frozen state model

`PortObservation.state`:

- `available`
- `owned_by_next`
- `reserved`
- `conflict`
- `unknown`

Required fields:

- configured port
- observed port
- state
- safe reason code
- observed-at timestamp
- config revision

Do not expose arbitrary foreign PID command lines to normal UI.

### 9.3 Ownership semantics

A simple connect test is insufficient.

Authoritative backend preflight must:

1. reject reserved ports before probing;
2. probe actual listener occupancy on the configured Core bind scope;
3. if no listener conflicts, classify `available`;
4. only classify `owned_by_next` when the platform adapter can prove the listener belongs to the currently selected **Next** runtime/service/binary instance;
5. foreign listener → `conflict`;
6. listener exists but ownership cannot be proven → `unknown` and fail closed.

The existing `updateplatform/darwin.go` fixed-8767 `lsof + ps command` check is reference code, not the final generic proof. Wave 1 platform adapters must bind process/listener evidence to the Next runtime/service identity, not only compare a command prefix.

Windows must have an equivalent Next root / active generation / process or task identity proof before mutations are enabled. No Windows proof → unavailable/unknown, not optimistic success.

### 9.4 TOCTOU

Two levels are required:

- UI/read preflight for immediate feedback;
- **authoritative preflight inside the serialized mutation immediately before first side effect**.

The desktop mutation lock only coordinates AgentDock writers; it cannot prevent an unrelated process from racing for the port. Therefore the final Core bind/restart result is still authoritative. If bind fails, mutation fails/rolls back; it must not reinterpret that as success based on an earlier preflight.

## 10. Connection / Tunnel state model

Do not use one `ready` boolean for everything.

### 10.1 Core

- configured port
- observed port
- running: true/false/unknown
- health: healthy/unhealthy/unknown
- port ownership state
- Local MCP URL

### 10.2 Tunnel

- mode: none/quick/named
- registered: true/false/unknown
- running: true/false/unknown
- connected: true/false/unknown
- autostart: enabled/disabled/unknown/unavailable
- public origin
- generation/revision
- capability / disabled reason

### 10.3 Public endpoint

- `not_configured`
- `unchecked`
- `checking`
- `reachable`
- `unreachable`
- `unexpected_endpoint`
- `stale`

Public reachability is observational. Network failure does not automatically roll back an otherwise coherent local Core/tunnel configuration.

## 11. Public endpoint test contract

The test target is derived from current backend state; frontend does not submit an arbitrary URL.

Requirements:

- target is current configured HTTPS public origin + known AgentDock discovery path;
- no credentials are sent;
- redirects are disabled;
- bounded timeout and response size;
- safe structured result only;
- result is tagged with config revision and ignored if configuration changes before completion;
- verify AgentDock OAuth/MCP discovery metadata is structurally compatible with the configured public origin;
- `HTTP 200` alone is not connector-readiness proof;
- the test must never follow redirect to localhost/private destinations.

The UI label is **Public endpoint test / reachability**, not “verified Next ownership” unless a future attestation mechanism actually proves instance identity.

## 12. Named Tunnel remote-route limitation

### 12.1 Current technical reality

Current Named mode uses a remotely-managed Cloudflare Tunnel token:

- `cloudflared tunnel run` receives `TUNNEL_TOKEN`;
- Quick mode uses local `AGENTDOCK_TUNNEL_TARGET`;
- Named mode does **not** use that local target to choose the published application's Service URL.

For remotely-managed Tunnel, hostname → local Service URL belongs to Cloudflare-managed route configuration. Therefore storing a token/hostname locally does not prove:

- the route is dedicated to Next;
- the route targets `127.0.0.1:8767`;
- a custom local port change has been propagated.

### 12.2 Wave 1 freeze

Remote Cloudflare route-management API credentials are **not** added to Wave 1.

Therefore:

- official `mac-dev-next` setup defaults to Next Core `8767`;
- user-provisioned dedicated Named route must target `http://127.0.0.1:8767`;
- if user wants a different port while Named is active, UI must not claim automatic remote synchronization;
- safest supported transition is:
  1. leave/disable Named public exposure;
  2. change Next port and verify local Core;
  3. user updates the dedicated Cloudflare route Service URL externally;
  4. re-enable Named;
  5. run public endpoint test;
- a future Cloudflare route-management integration can make this automatic, but is a separate capability/credential review.

This supersedes any unconditional wording that “Named remote origin is automatically synchronized by the current token-only backend”.

### 12.3 Ready semantics

Named states are separated:

- tunnel process running;
- tunnel connected to Cloudflare;
- public endpoint reachable;
- Next ownership/connector side-by-side verified later at P3.

A locally stored dedicated token/hostname is not itself remote ownership proof.

## 13. Mutation contract

### 13.1 Shared identifiers

Before A/B/C workers implement independently, the following concepts are frozen:

- `configRevision` — opaque revision/hash for Connection-relevant configuration.
- `operationId` — unique per mutating request.
- `tunnelGeneration` — identifies the currently valid Quick/Named tunnel launch/config generation.
- every async public test / reveal / readiness result carries enough revision/generation context to reject stale completion.

A persistent `instanceId` may be added only if implementation needs stronger local/public attestation; it is not required to expose a fake “verified Next” state in Wave 1.

### 13.2 Operation phases

Mutation result/status vocabulary:

- `pending`
- `applying`
- `applied`
- `waiting_readiness`
- `completed`
- `rolling_back`
- `rolled_back`
- `recovery_required`
- `failed`

Do not collapse “local state changed” and “internet endpoint reachable” into one Completed boolean.

### 13.3 Lock ordering

- one outer Desktop mutation owner per runtime root;
- internal transaction helpers called while the outer lock is held must not reacquire that same lock;
- do not call public API helpers that acquire the lock from inside a locked transaction;
- CLI/native mutation paths that can touch the same Next files/services must converge on the same lock contract or explicitly reject concurrent operation.

### 13.4 Quick Tunnel two-phase convergence

Quick URL publication currently acquires the Desktop mutation lock.

Therefore the allowed flow is:

1. lock;
2. validate full request;
3. authoritative port preflight;
4. capture rollback/recovery state;
5. invalidate old Quick URL + assign new `tunnelGeneration`;
6. apply local config / target / service changes;
7. release lock;
8. cloudflared publishes URL tagged/bound to the generation;
9. URL callback reacquires lock briefly;
10. callback verifies generation is still current;
11. callback writes public origin/OAuth state + ready file;
12. UI readiness converges.

Callbacks from superseded generations are discarded.

### 13.5 Local rollback / recovery

Before first side effect:

- validate complete request;
- capture file existence + bytes where safe;
- capture service running/registered/autostart states independently;
- preserve stopped state; saving config must not silently start a previously stopped Core/tunnel unless the requested action explicitly does so;
- token rollback material must remain protected and never enter ordinary operation metadata.

For synchronous local failure, restore coherent prior local state when possible. If restoration cannot be proven, return `recovery_required` and fail closed.

Quick public hostname is ephemeral: once regenerated, rollback cannot promise restoration of the old hostname. UI/error copy must state local rollback and public-address restoration are different things.

Crash/interruption recovery for any new multi-file transaction must have bounded Next-owned recovery metadata; do not call a sequence of atomic file writes a globally atomic transaction without a recovery contract.

## 14. API contract freeze for Wave 1

Exact Go names may change during implementation, but semantics may not.

### 14.1 Safe snapshot

Connection status returns a safe snapshot containing:

- `configRevision`
- Core status / port observation / Local MCP URL
- Tunnel structured state
- Public MCP URL
- OAuth password availability state only
- Tunnel token stored state only
- platform capabilities / disabled reasons
- last public endpoint test summary if non-secret

No raw secrets.

### 14.2 Read operations

Required read operations:

- `status`
- `preflightPort(port)`
- `testPublicEndpoint()`
- `revealOAuthPassword()`

`revealOAuthPassword()` returns only the password or a safe missing/unavailable error; it never generates/rotates.

A separate `copyOAuthPassword()` backend/native clipboard operation is preferred if implemented.

### 14.3 Mutations

Required semantics:

- tunnel lifecycle start/stop/restart;
- Quick regenerate;
- configure mode none/quick/named;
- optional Named token replacement;
- tunnel autostart where platform capability allows;
- Core port save with authoritative preflight and coherent target/config update.

Every mutation:

- includes/validates current `configRevision` when stale-write protection matters;
- returns `operationId` + structured outcome;
- never returns token/password;
- uses safe stable error codes + category + disabled reason;
- preserves Next isolation.

### 14.4 Capability metadata

Boolean `mutable` is insufficient for all platform cases.

Connection snapshot must be able to express per-operation:

- supported / unsupported / unavailable;
- confirmation required;
- disabled reason;
- native-backed requirement.

Especially for macOS autostart/registration and Windows service/task identity.

## 15. Confirmation / warning freeze

### No confirmation

- Refresh/status
- preflight
- endpoint test
- Hide secret
- Copy non-secret URL
- Start an already-approved unchanged local configuration, if it does not newly expose a public endpoint

### Explicit user action, no destructive confirm

- Show OAuth password
- Copy OAuth password
- save low-risk non-disruptive setting that does not restart/expose service

### Confirmation required

- Stop / Restart when it interrupts active service
- Core port change
- changing public mode
- enabling public exposure for the first time
- Named hostname/token replacement
- Quick Regenerate
- disabling public access while connector may depend on it
- recovery/reset action

Later ACP/MCP/Plugin delete/disconnect/uninstall remain explicit-confirm operations in their own domain review.

## 16. Wave 1 parallel write boundaries

### Worker A — Port ownership / occupancy / conflict

Owns primarily:

- reusable port observation/classifier;
- macOS + Windows platform ownership proof;
- reserved port rules;
- authoritative preflight integration hooks;
- tests/fixtures.

Must not invent Connection UI schema independently.

### Worker B — Connection/Auth API

Owns primarily:

- safe snapshot projection;
- Local/Public MCP URL derivation;
- OAuth password availability + explicit reveal;
- public endpoint test;
- safe errors/revision staleness.

Must not expose desktop-control/Bearer credential.

### Worker C — Tunnel backend

Owns primarily:

- configure none/quick/named;
- token replace state;
- autostart capability;
- tunnel structured states;
- tunnel generation / stale callback guard;
- transaction/rollback hooks.

Must not add Cloudflare account route-management credentials in Wave 1.

### Shared-file serialization

The following areas require main-session integration before merge because all three may need them:

- `internal/desktopapi/contract.go`
- `internal/desktopapi/connection.go`
- common mutation helpers
- `desktop/shared-poc/main.go` service registration/bindings
- frontend API model/bindings
- Connection store
- i18n keys
- Connection page/component hierarchy

After A/B/C backend work, main session reviews schema + mutation semantics and freezes the implementation contract before Connection UI starts.

## 17. Wave 1 test / acceptance contract

### Port

- 8765 rejected
- 8766 rejected
- 8767 available
- foreign listener conflict
- unknown owner fail closed
- current managed Next listener classified owned only with platform proof
- race between preflight and bind fails safely
- Windows and macOS ownership fixtures
- no stable runtime inspection required

### Secrets

- status contains no OAuth password/token
- reveal missing/unreadable/unavailable cases
- reveal never rotates
- tunnel token never returned
- raw adapter errors scrubbed
- component/store tests prove no secret persistence in global state

### Quick

- old generation callback rejected
- new URL publication waits for mutation lock and resumes
- config mutation does not wait for callback while holding lock
- stale quick URL removed before regeneration
- port change rewrites local Quick target
- stopped-state preservation
- public readiness timeout does not fabricate rollback success

### Named

- missing token / invalid token
- hostname validation
- running != public reachable
- connected != route verified
- custom-port transition warns/fails closed according to remote-route prerequisite
- dedicated Next runtime token store only
- no stable tunnel/token fallback

### Public endpoint test

- derived URL only
- HTTPS only
- redirects disabled
- bounded body/time
- expected OAuth/MCP discovery metadata
- stale revision result ignored
- no secret headers/body

### Mutation / recovery

- failure at each local write/service transition
- prior absent file restoration
- rollback failure → recovery_required
- cancellation does not leave success state
- configRevision stale write rejected
- no nested mutation-lock deadlock

## 18. UI implementation acceptance

Connection UI may begin only after backend integration review confirms the frozen snapshot/mutation semantics.

UI acceptance:

1. Local MCP URL visible/copyable.
2. Public MCP URL always shown as full `https://…/mcp`, never only origin.
3. OAuth password masked by default, explicit Show/Hide/Copy, no global persistence.
4. Port state shows available / Next-owned / reserved / conflict / unknown with reason.
5. Public mode settings Local/Quick/Named include operator explanation.
6. Tunnel Token is write-only; UI says stored/not stored but cannot reveal.
7. Quick Regenerate only appears/enables in Quick mode and warns that URL changes.
8. Named page explains remote route prerequisite; does not claim current local port was auto-synced when no route-management integration exists.
9. Tunnel process, connected state, public reachability and connector readiness are not collapsed.
10. all disabled actions have reasons; all async/stale states recover safely.
11. narrow window and keyboard/screen reader requirements pass.
12. no stable AgentDock resource is used for validation.

## 19. Stop gate

After P1/P2 code + tests + Next-only/repository-safe verification pass:

**STOP.**

The user manually creates `mac-dev-next` in ChatGPT using the Next Public MCP URL + OAuth password.

Only after user-confirmed connector validation may Wave 3 start:

- ACP Manager
- MCP Management
- Plugin Management
- Browser Broker
- Nexus / Platform

Wave 3 uses independent worktrees and at most 3–4 active writing workers.

`app-identity.sh` governance debt and Codebase Memory lifecycle triage may run in the hardening side lane, but neither permits entering M9 early.

## 20. References / evidence anchors

Repository evidence:

- `internal/desktopapi/connection.go`
- `internal/desktopapi/basic_settings.go`
- `internal/desktopapi/mutation.go`
- `internal/desktopruntime/basic_settings*.go`
- `internal/desktopruntime/tunnel*.go`
- `internal/desktopruntime/mutation.go`
- `internal/updateplatform/darwin.go`
- `desktop/shared-poc/main.go`
- `desktop/shared-poc/security.go`
- `desktop/shared-poc/frontend/src/components/connection/ConnectionPanel.vue`
- `desktop/shared-poc/frontend/src/components/settings/BasicSettingsPanel.vue`
- `desktop/macos/AgentDockApp/Sources/SetupWindowController.swift`
- `desktop/macos/AgentDockApp/Sources/AdvancedSettingsWindowController.swift`
- `desktop/macos/AgentDockApp/Sources/TunnelTokenStore.swift`
- `desktop/windows/control-panel/MainWindow.xaml`
- `desktop/windows/tray/app_windows.go`

Cloudflare behavior reference used for the Named Tunnel constraint:

- Cloudflare Tunnel tokens: remotely-managed tunnel token is used to run the connector.
- Cloudflare published application route: public hostname maps to a configured local Service URL.
- Remotely-managed route Service URL is configured in Cloudflare, not by AgentDock's current local token-only runner.

