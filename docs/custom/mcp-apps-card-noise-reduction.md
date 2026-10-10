# MCP Apps 卡片降噪 — ChatGPT 聊天模式 UX 採用規劃

> **Status: PLANNED / no implementation**（2026-10-10）。獨立於 [PR #229 Task Progress live card](upstream-ports/task-progress-live-card-pr229.md)；兩者目標相關，但 PR #229 **不會**自動解決其他工具的卡片膨脹。此規劃不得當成 ChatGPT Host 已支援原地替換歷史卡片的證據。
>
> **使用者真實痛點**：ChatGPT iOS 聊天中，AgentDock 反覆出現「Workspace · 1010-fireworks」、「Codex ready」、「Model: gpt-6.1-sol → gpt-6.1-sol」、「Reasoning effort: high → high」等整張卡片。這些分別來自 Workspace Context、ACP Status/Update 等工具結果，**不是** Task Progress 卡片。

## 1. 目標與明確非目標

1. 讓 ChatGPT 聊天模式的每次必要操作保有可追溯的結果，但不要讓大量 **重複狀態／實際無變化** 的 MCP App 卡片遮蔽主要對話；手機版尤其要減少視覺高度。
2. **Task Progress** 維持 [PR #229](upstream-ports/task-progress-live-card-pr229.md) 的「同一 Task ID 一張即時更新卡片」；不是本文件的重新實作範圍。
3. **ACP Session** 同一 `session_id` 的唯讀 status／model／reasoning effort，評估合併成較精簡、可按需展開的狀態摘要；`no change` 操作不顯示大型重複卡片。**要注意**不同工具呼叫在 ChatGPT 的訊息紀錄中仍可能各有一個工具結果；除非 Host 確認支援原地更新，**不得宣稱不同訊息會被刪除或替換成同一張歷史卡片**。
4. **Workspace Context** 同一 workspace/rules/skills revision 被重複讀取時，降低卡片佔位；變更工作區、規則、Skill、警告或錯誤仍應清楚顯示。
5. **重要結果不能降噪到看不見**：Approval、權限變更、失敗／recovery、工具呼叫進行中、有效模型切換、Session 切換、跨工作區風險與需要使用者決策的提示，應有明確可見結果及完整可存取記錄。
6. 不擅自修改 ChatGPT Client UI、刪除歷史訊息、模擬 Host card dedup 或向 Host 私下假設非公開能力。這是一個 AgentDock MCP Tool UI metadata/formatting 與 Agent 行為的 UX 規劃。

## 2. 基線與可行的實作層

Next 目前由 `internal/app/mcp_apps.go` 的 `UIBinding` 設定工具 descriptor / result UI 的掛載；`internal/mcp/server.go` 依 `toolMetadata` 與 `toolResultMetadata` 實際傳遞 MCP Apps metadata；`internal/mcp/apps.go` 用共享 `mcpapps.HTML(...)` 供應 Workspace、ACP Status、ACP Prompt、Task 等 UI。**即使結果為 no-op，卡片是否展示仍受 Host 的工具描述及結果 metadata 影響**。要優先在 AgentDock 的輸出契約／metadata 降噪，不能只把 DOM 縮小就算解決。

| 情境 | 預期 UX 與技術策略 | 需保留的資訊 |
| --- | --- | --- |
| Task create/checkpoint/complete | PR #229：每 Task ID 僅 create 綁卡；checkpoint 等透過 snapshot 更新原卡 | steps、blocked、review、最終完成 |
| `acp_session` list/info 同狀態重複查詢 | 精簡 tool result；依 Host 能力調整 UI 挂載而非每次大型卡，必要時可展開 | session ID、provider、model、state、錯誤 |
| `acp_session` model/effort `X → X` | 明確回傳 `no_change`／`changed=false` 或既有等價契約；避免掛大型狀態卡 | 操作已處理、目前值、可讀失敗原因 |
| `acp_session` 真正切換模型／effort 或建立/關閉 session | 提供清楚的差異與簡潔結果；必要時保留 App 卡或可擴展內容 | before/after、目標 session、失敗／授權 |
| `workspace_context` 重複相同 path/revision | 保留工具資料回傳；優先去掉重複高卡，適當顯示短摘要 | workspace 路徑、載入規則／技能計數、warning、revision |
| Browser / permission / ACP Prompt / file edit | **不得以 no-op 節省為由隱藏重要狀態**；逐工具審查而非全域一刀切 | 警告、同意、輸出、問題定位與可追溯性 |

### 三層改善手段（由低風險到高風險）

- **N1：輸出語意與 metadata**：先提供 action-specific `UITrigger` / result-level binding policy，對 read-only 重複快照或 no-op 值避免大型 MCP App。關鍵：不能把 `full/compact/off` 的既有預期默默打破；若 Host 強制展示 descriptor-level UI，先增加受控 Next-only opt-in 或「精簡模式」，收集證據再決定預設值。
- **N2：摘要／可展開卡片**：真正變更時保留簡短結果、展開取得完整受控 detail；Workspace / ACP Status 基於正規化現況與版本，不在前端做跨 workspace / session 全域去重。需要 i18n、VoiceOver、手機窄版、dark mode、keyboard UAT。
- **N3：有證據後才做持久單一卡**：針對 ACP session id 與 workspace identity/revision，調查 Host 是否允許原地 refresh（與 PR #229 類似）；即使允許也要保持每張卡對應單一 authority/scope，絕不將不同 session/workspace 的卡片混在一起。

## 3. 安全、相容性與資源邊界

- **資料和 UI metadata 分離**：縮小 UI 不代表刪除權威結果；工具的結構化輸出、錯誤碼、日誌與 Execution Center evidence 仍可供 Agent 診斷。敏感 token、個人 Profile、debugging URL/Workspace 內部細節不得新增到公開卡片。
- **不影響授權與確認**：M8 Permission/Approval 的 fail-closed 規則必須維持；任何會造成 mutation 或需要確認的操作，都不能因重複結果而略過審核、掩蓋失敗或隱藏操作成功與否。
- **跨 Scope 嚴格隔離**：多個 ACP session、多個 Workspace、stable/Next connector、Task ID 各自擁有不同身分與狀態；切換後不可沿用舊卡片的 stale snapshot。
- **相容與負載**：`MCP Apps=full/compact/off`，以及舊 Plugin 註冊快取與開新聊天後 metadata 可能不更新，都須驗證；輪詢型卡片不得每次返回大型完整結果，隱藏／teardown／terminal 必須釋放 timer。避免為降噪新增跨 Session background polling、占用系統記憶體。
- **可稽核**：所有 no-op 判斷由權威處理層決定，不依賴前端畫面 `X → X` 字串或猜測；失敗與已變更結果不可誤分類為 no-op。

## 4. 實作順序與驗收清單

| Case | 驗證 | PASS 條件 |
| --- | --- | --- |
| N01 | `acp_session` model/effort `X → X` | 工具結果正確；無重複大型卡（須有 ChatGPT 實測證據）；no-op 與 error 不混淆 |
| N02 | `acp_session` 真實 model/effort、session 切換／建立／停止 | 必要差異清楚呈現、不吞掉 mutation/error；跨 session 不混淆 |
| N03 | 同一 Workspace 重複呼叫 `workspace_context` | 結構化資料仍回傳，聊天畫面卡片高度減少；切換 Workspace 或 rules 變動不被隱藏 |
| N04 | PR #229 的 Task 3-step checkpoint、complete | 一個 Task ID 一張 live 卡，後續不再掛出另一張（不與 ACP 卡片行為混用） |
| N05 | 不同 ACP Session／Workspace／Task ID 並存 | 每個 scope 狀態正確、切換即換內容，不混淆或洩漏 |
| N06 | 失敗、M8 approval、取消、timeout／recovery | 提示仍清楚可見且可追溯，沒有錯誤偽裝成功 |
| N07 | ChatGPT iOS/桌面 + full/compact/off + 舊/更新 metadata | 真實卡片呈現符合政策；無支援能力則退回安全的精簡結果，不虛稱 Host 原地合併 |
| N08 | 快速連續呼叫 10 次無變更狀態／多步驟工作流 | 卡片數／高度相較基線明顯下降，訊息內容仍可讀，無輪詢／記憶體異常 |

**順序**：先做 N0 卡片 inventory + ChatGPT iOS baseline/UAT fixture（包含 Workspace、Codex ready、Model `X→X`、Reasoning `high→high`），再 N1/N2 減少無效卡片；PR #229 可作平行開發，但須一起做整合 GUI UAT。N3 僅在 Host 能力得到證據後評估。這是 **Pre-M9 後可平行的非阻塞 UX 子線**，不搶 Browser Broker／AGY ACP／M9 release gates 的優先權。

## 5. 來源與交接

- 使用者 2026-10-10 ChatGPT iOS 截圖（1010-fireworks 對話）：Workspace、Codex session/model/effort 重複卡片；此觀察為產品痛點，非 ChatGPT Host 支援重繪能力的證明。
- PR #229: <https://github.com/uvwt/agentdock/pull/229>；[Next Task 卡採用規格](upstream-ports/task-progress-live-card-pr229.md)
- Next code: `internal/app/mcp_apps.go`、`internal/mcp/server.go`、`internal/mcp/apps.go`、`internal/app/specs_task.go`
- [Roadmap](roadmap.md) · [Pre-M9 Feature Parity](pre-m9-feature-parity.md) · [上游採用規劃](upstream-v101-adoption-plan.md)
