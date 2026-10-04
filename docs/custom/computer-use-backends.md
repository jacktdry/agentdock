# Computer Use Backend Strategy

> 狀態：M6 completed / M7 handoff
> 建立日期：2026-10-03
> 實作更新：2026-10-04
> 主要 Milestone：M6 — Browser Broker、M7 — ACP Manager、M8 — Permission / Approval

## 2026-10-04 Implementation Checkpoint

Computer Control Broker 已在 `13c9cd9a feat(computer): add Orca computer control broker` 落地：

- 上游 AgentDock 公開 `computer_session`、`computer_observe`、`computer_act`，另提供 `computer_broker` control-plane diagnostics / ACP-owner cleanup；
- ACP 取得獨立 `/internal/acp-computer/mcp`，與 Browser Broker 共用同一個 per-session bearer capability，但工具面彼此分離；
- primary provider 固定為 Orca，現場驗證版本 `1.4.218`；不實作 ChatGPT / Sky 的 silent fallback；
- `foreground=forbidden` 為預設，所有 native mutation 與 `permissions`、`restore_window` 都在 provider call 前 fail closed；
- background observation 若意外改變 frontmost app，回傳 `COMPUTER_FOCUS_VIOLATION`；
- macOS 使用 `lsappinfo` 做前後 active-app observation，不新增 AppleScript / System Events 權限路徑；
- Computer provider operation 目前全域序列化，避免多 ACP 同時搶 native GUI focus/input；
- Orca provider 保留結構化 provider error、action verification，set-value / type / paste 的文字以 stdin 傳送，不放入 process args；
- macOS 真實 `capabilities`、`list_apps`、`get_app_state --no-screenshot` smoke 已驗證 background observation 不改變前景 app；
- Windows amd64 已通過相同 Broker contract 的 cross-build，平台差異只留在 provider / active-app adapter；
- 自維護 `antigravity-acp` 已升至 `1.2.0-agentdock.5`（`2bd8426`），AGY child 只取得 AgentDock 提供的 Browser / Computer MCP，不恢復 global browser / Computer Use plugin；
- `af49037b` 加入 bounded diagnostics：active/released session、`focus_violation`、`provider_failure`、`foreground_denied`；不保存文字輸入、screenshot 或 provider payload；
- `dc46459e` stress validation 搭配真實 Orca background observation，確認 no-focus guard 與 20 次 Antigravity capability lifecycle 回到 baseline。

M7 不需要重做 Computer provider routing；M7 只需讓 ACP persistent / ephemeral / idle-managed session policy 在 close/idle lifecycle 上正確釋放既有 Computer capability，並把 diagnostics 納入 session view。

## Goal

AgentDock 的 Computer Use 必須滿足兩個優先原則：

1. **能用程式化工具完成，就不使用 GUI automation。**
2. **不得把搶焦點／搶螢幕當成正常 background workflow。**

Computer Use 是最後一層 native GUI fallback，不是一般 Web automation backend。

優先順序：

```text
API / MCP / shell / filesystem / Git
→ Browser Broker / chrome-devtools-mcp
→ Orca Computer Use
→ ChatGPT / OpenAI Computer Use product fallback
```

## 2026-10-03 Backend Bake-off

### Orca Computer Use

目前 Orca 1.4.218 可直接由 AgentDock host 使用，已確認：

- Accessibility permission：granted；
- screenshot permission：granted；
- list apps / list windows；
- accessibility tree + screenshot；
- click by element / coordinate；
- left / right / middle click；
- set-value；
- type / paste；
- key / hotkey；
- scroll；
- drag；
- secondary accessibility action；
- explicit window id / window index targeting；
- permission diagnostics。

實測 background observation：

```text
ChatGPT frontmost
→ background Calculator window exists
→ orca computer get-app-state
→ state read succeeds
→ ChatGPT remains frontmost
```

實測 native GUI action：

```text
ChatGPT frontmost
→ Orca AXPress on background Calculator
→ Calculator value changes successfully
→ Calculator becomes frontmost
```

結論：

**Orca 可背景觀察，但 native GUI action 不保證 no-focus。**

### OpenAI Sky / Computer Use MCP

本機 `SkyComputerUseClient mcp` 可 discovery 到完整工具面：

- `list_apps`
- `get_app_state`（screenshot + accessibility tree）
- `click`
- `type_text`
- `drag`
- `scroll`
- `press_key`
- `set_value`
- `select_text`
- `perform_secondary_action`

但 AgentDock 將它作為 generic standalone MCP 呼叫時，實際 tool call 會在 bootstrap 階段 timeout。

本機 binary / plugin 現場證據顯示該 runtime 依賴：

- ChatGPT / Codex per-turn metadata；
- Mach/XPC bootstrap rendezvous；
- turn lifecycle；
- `turn_ended` hook；
- `@oai/sky/service`；
- ChatGPT Desktop 的 `unified-computer-use → cua_repl` runtime。

因此目前不把 `mcp_servers.computer-use` 視為 AgentDock 可依賴的 generic standalone provider contract。

這不是能力不足，而是 **runtime boundary / bootstrap contract 不穩定且屬於 ChatGPT/Codex product integration**。

## Provider Decision

AgentDock 的 Computer Control abstraction 採 provider model：

```text
upstream ChatGPT direct computer-use need ─┐
ACP delegated computer-use need           ├─→ AgentDock Computer Control Broker
                                          │          │
                                          │          ├─ default/primary: Orca
                                          │          └─ explicit product fallback: ChatGPT / OpenAI Computer Use
                                          └──────────┘
```

這條 routing 同時適用於「上游 ChatGPT 自己要操作本機 GUI」與「ACP 被指派需要 Computer Use」兩種情境。上游 ChatGPT 不應因為自己具備產品層 Computer Use 能力，就繞過 AgentDock；預設一律透過 AgentDock 的 Orca provider。

目前：

- **Orca**：AgentDock primary/default provider；上游 ChatGPT 直接需要 Computer Use 時也走這條路。
- **OpenAI Sky / standalone computer-use MCP**：不作 AgentDock primary provider。
- **ChatGPT Computer Use**：保留使用者權限與產品能力，但不作 AgentDock / ACP 的一般執行路徑；只有使用者明確指定或正式 fallback policy 允許時才使用。
- 未來若 OpenAI 提供正式穩定的 standalone provider contract，可新增 provider，不需改 ACP-facing interface。

## No-Focus Contract

Computer Use 不得被包裝成「背景一定不影響使用者」。

AgentDock 必須區分：

```text
background-safe observation
foreground-required action
```

### Background-safe

優先允許：

- list apps / windows；
- read accessibility state；
- screenshot when provider can capture without activation；
- read-only diagnostics；
- non-GUI APIs / MCP / browser CDP。

### Foreground-required

任何可能造成：

- activate app；
- restore window；
- bring-to-front；
- synthetic keyboard focus；
- native click 導致 app activation；
- modal / file picker interaction；

都必須標記成 foreground-required。

預設 policy：

```text
foreground_control = forbidden
```

只有：

- user explicitly requests visible/native control；
- 或 task 無程式化 / Browser Broker / background-safe alternative；

才允許升級。

未來 UI 應可顯示：

```text
This action requires foreground computer control.
```

而不是靜默搶走 active app。

## ACP Contract

ACP 不應直接決定：

```text
use OpenAI computer-use plugin
use Orca
use another desktop automation backend
```

同樣地，上游 ChatGPT 自己需要 Computer Use 時，也不直接挑 product backend，而是先請求 AgentDock Computer Control Broker。

ACP 只應請求：

```text
computer.acquire({
  capability: ...,
  foreground: forbidden | allowed
})
```

上游 ChatGPT direct GUI task 也走相同 broker contract。由 AgentDock 決定 provider，預設選 Orca。

這讓 Codex / Antigravity 不必各自持有多套 Computer Use stack。

## Codex ACP Configuration Direction

目前 global Codex config 仍可保留 ChatGPT Desktop 的：

- computer-use plugin；
- unified-computer-use plugin；
- browser / chrome product integrations。

但 AgentDock 專用 Codex ACP profile 最終應停用 ACP 自己的重複 Computer Use path，改由 AgentDock broker 提供。

目標不是修改使用者日常 ChatGPT Desktop 功能，而是：

```text
ChatGPT Desktop
→ 可保留自身 Computer Use

AgentDock Codex ACP
→ AgentDock Computer Control Broker
→ Orca primary

Upstream ChatGPT direct GUI task
→ AgentDock Computer Control Broker
→ Orca primary
```

## Required Diagnostics

至少提供：

```text
computer_provider
computer_session_id
owner_acp_session_id
foreground_policy
foreground_required
active_app_before
active_app_after
window_id
action_kind
action_verification
cleanup_state
provider_error
```

如果 action 造成 active app 改變，diagnostics 必須可觀察。

## Acceptance Criteria

1. Background observation 不改變 active app。
2. 一般 Web UAT 不會升級成 Computer Use。
3. foreground-required action 在 policy=forbidden 時不執行。
4. 上游 ChatGPT 直接需要 Computer Use 時，預設透過 AgentDock Computer Control Broker → Orca，不直接繞到 ChatGPT/OpenAI product Computer Use。
5. ACP delegated Computer Use 同樣透過 AgentDock broker，由 AgentDock 決定 Orca provider。
6. AgentDock / ACP 不以 ChatGPT private Sky bootstrap 當核心依賴。
7. Orca provider failure 有明確 error，不 silent fallback 到 foreground ChatGPT Computer Use。
8. 未來新增 OpenAI / Windows provider 不需改 ACP-facing API。
9. Computer Use session lifecycle 可追到 owner ACP / task；上游 direct task 則可追到 owner task/request。
10. provider process / session 可 bounded cleanup。
11. GUI action 是否改變 active app 可驗證。
12. macOS / Windows 使用相同 broker contract，平台差異留在 provider adapter。

## Non-goals

本項目不：

- 保證所有 native GUI 操作都能完全 background；
- 讓 ACP 直接持有多個 GUI automation backend；
- 把 ChatGPT Desktop private runtime 當穩定 public API；
- 為了統一而移除使用者日常 ChatGPT Computer Use 權限；
- 讓 Computer Use 取代 Browser Broker。
