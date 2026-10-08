# AGY ACP / AgentDock Next 整合交接（2026-10-08）

> 給目前正在開發 AgentDock Next 的 Session：本工作線只提供跨專案需求與驗收合約，不修改 Next Core/UI、不編譯或部署 Next；原版 AgentDock 完全保持原狀。
> ACP repo: /Users/wei/sideProject/antigravity-acp；既有基線 fix/agentdock-hardening @ 4fd494b (1.2.0-agentdock.8)。
> Next repo: /Users/wei/sideProject/agentdock-m8-permission-approval；工作分支 feature/m8-permission-approval。
> ACP 將在獨立開發分支實作；以下規格只代表合約提案，是否實作以後續 ACP commit、測試結果為準。

## 不可改變的相容條件

1. **原版 AgentDock 不動**：不修改 Core、UI、設定、HOME、ACP Profile、服務及部署。維護版 ACP 預設行為須保留 Legacy；不因新增 Next 功能而改變現行權限旗標或 Session 行為。
2. **Next opt-in**：只有 Next Core 明確傳入新環境設定才啟用新行為，不以進程名稱、執行檔路徑猜測身份。
3. **狀態分隔**：Next 自己的 ACP sessions/model cache 與 stable 不共享；Core 指定並驗證 Next-owned 絕對路徑（建議 ~/.agentdock-next/acp/antigravity，具體路徑待凍結）。共用 AGY 原有登入 OAuth/對話僅限目前白名單，不靜默遷移。
4. **工具治理**：保留既有 Browser/Computer Broker，AGY 子程序不得繼承全域 MCP/Plugin/Hook；Broker 關閉或缺失時仍需私有隔離環境及 fail-closed policy。
5. **部署 Gate**：只在獨立測試分支驗證。不得自動替換 stable ~/.local/bin/antigravity-acp 或 Next ~/.agentdock-next/bin/antigravity-acp，不得修改原版 AgentDock 進行新功能測試。

## 工作線 A — 原生工具權限與 Approval（最高優先）

目前 ACP src/agy/process.ts 每個 prompt 固定注入 --dangerously-skip-permissions，--sandbox 選用且預設 false。AGY 原生檔案／Shell 工具不一定由 M8 Core Permission Admission 管理；M8 的 Browser/Computer Admission 只能保護 Broker 管線，不能當作所有 AGY 工具都已受控。

- **Next Session 所有權**：Core-owned allow/ask/deny、ACP principal、workspace 可信根、Native tool 的授權/拒絕/無法攔截策略、需要的 UI 說明。
- **ACP Session 所有權**：盤點 AGY 1.3.1 headless 非互動權限行為；設計 opt-in/fail-closed 啟動策略及啟動參數防繞過。特別防止 AGY_EXTRA_ARGS 在尾端加入危險覆寫。
- 不傳 --dangerously-skip-permissions 只能視為 **關閉自動核准**，不能稱為 M8 全面管理。若 AGY 無外部逐項授權 hook，需要 OS 等級隔離或明確降低可執行能力；先不得對一般工作宣稱可完全防止寫入。
- 驗收：唯讀、Shell 寫入、工作區外路徑、網路、Broker Allow/Deny、headless 拒絕/逾時、Legacy 行為保留；沒有逐項攔截能力時不得上線「Core 完整托管權限」UI。

## 工作線 B — HOME 與 State Directory 解耦

目前 Legacy ACP 以 HOME=~/.agentdock/agy-home 啟動，再於 ACP 建立第二層子程序隔離 HOME；ACP STATE_DIR 與 AGY_CONVERSATIONS_DIR 目前依 os.homedir() 推導。曾導致 macOS loginKC:queryCreate；已在 .8 補救 Legacy 路徑，但目標是不再需要 Next 父 ACP 整個 HOME 覆寫。

- **ACP Session 所有權**：opt-in 自訂 STATE_DIR 與必定使用私有 AGY 子程序 HOME，仍保留正常 HOME 的既有非 Next ACP 行為與 Keychain 白名單 symlink。
- **Next Session 所有權**：決定並檢查 State owner、目錄權限與 symlink，僅 Next Profile opt-in；規劃舊 Next sessions 轉移或明確的保留/回滾，不得默默丟失或共讀 stable state。
- 驗收：login Keychain/OAuth、models、/usage、Session new/load/resume/delete、Broker required/disabled、Core 重啟、Legacy 對照，以及 macOS/Windows/Linux 支援界定。

## 工作線 C — stream-json (效能 PoC)

- 先由 ACP Session 做獨立 parser / --output-format stream-json PoC，不修改對外 ACP wire、不取代 SQLite Poller；確認事件型別、tool call update、quota、replay 是否等價。
- 再評估 --input-format stream-json 長駐 AGY：照片/圖片、取消、多輪模型/推理/權限切換、usage、resume、Broker token 清理、空閒關閉/崩潰復原。
- Core 暫不做 AGY-specific 特殊處理；若 ACP feature gate 可以穩定維持外部 ACP 協定一致，再由 Next Session 明確啟用。
- 驗收要有 cold/warm prompt 延遲、CPU、記憶體、DB 輪詢、程序殘留基準測試；不能提前宣稱性能收益。

## 並行及交付約定

| 工作 | 負責 | 交付/整合 |
| --- | --- | --- |
| ACP feature flag / state dir / isolation / permission guard / stream-json PoC | 本 Session，只在 ACP 獨立分支 | 提供 commit、精確 env 名稱、測試與安全限制；**不部署** |
| Next Core Profile env、狀態路徑、M8 Permission、UI、Next-only UAT | 正在處理 Next 的 Session | 依穩定 ACP 合約修改，勿與本 Session 競爭 ACP source |
| 原版 AgentDock | 既有生產流程 | **不做任何改動** |

下一個 Next Session 工作前請先讀此文件與 ACP repo FORK_NOTES.md；在 ACP commit 及跨系統整合 UAT 前，上述功能都不能列為已完成或併入 Pre-M9 closeout。
