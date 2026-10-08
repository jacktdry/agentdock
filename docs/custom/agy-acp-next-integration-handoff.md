# AGY ACP / AgentDock Next 整合交接（2026-10-08）

> 給目前正在開發 AgentDock Next 的 Session：本工作線只提供跨專案需求與驗收合約，不修改 Next Core/UI、不編譯或部署 Next；原版 AgentDock 完全保持原狀。
> ACP repo: /Users/wei/sideProject/antigravity-acp；既有基線 fix/agentdock-hardening @ 4fd494b (1.2.0-agentdock.8)。
> Next repo: /Users/wei/sideProject/agentdock-m8-permission-approval；工作分支 feature/m8-permission-approval。
> ACP 將在獨立開發分支實作；以下規格只代表合約提案，是否實作以後續 ACP commit、測試結果為準。

## ACP .14 跨事件驗證與 Fail-Closed 修正（2026-10-09）

- **ACP 最新原始碼候選版 `1.2.0-agentdock.14`**；`/Users/wei/sideProject/antigravity-acp-next-integration`，分支 `feature/next-integration-optin`，HEAD **`87cff1e`**，功能修正 **`9381b04`**；尚未推送、部署。原版／Next 已安裝 ACP 仍為 `.8`。
- 修正先前 .13 的安全缺口：文字已向 ACP Client 確認送出後，若 SQLite 對應資料列遭刪除、status 由完成回退、同一 Step ID 重複，或文字改寫，先在 `StreamPoller.poll()` **和下次 reserveExternalText 前**檢查全量已確認列，一旦不一致立即 fail-closed，不能自動重播。驗證採 O(DB rows + acknowledged step count) 索引，避免多列時反覆巢狀掃描。
- `StreamDirectLedger` 同步改為已 ACK 的資料列消失或回退非完成狀態 → `POST_COMMIT_DIVERGENCE`，`safeToRetry=false`。跨事件模擬涵蓋多段 UTF-8、部分送達／遠端 ACK 不明、取消競態、工具事件不直接輸出、上游 ERROR、完成前 DB 尚未落地、Session 恢復時僅發送新回合、歷史 Replay 與雙 Ledger 狀態。
- ACP 整合測試 **456 pass／0 fail（43 files）**，TypeScript、Lint（既有非阻擋提示）及 macOS arm64 build PASS。研究詳見 `docs/research/stream-direct-admission.md` 與 `tests/agy/stream-cross-event-recovery.test.ts`。
- **仍沒有 `AGY_ACP_STREAM_JSON=direct`，未接到正式 `Adapter.runPrompt`。** 本次只用合成／SQLite 測試，不進行真實 AGY `-p`／OAuth。直接輸出仍需要 Next Core 與 ACP 一起證明真實事件 parity、訊息無法撤回時的恢復策略、工具/圖片/Session/Keychain 結合驗收；Hook/M8 IPC 仍待 Next 整合。不可將這份離線測試當正式部署授權。

## ACP .13 共用文字輸出游標 checkpoint（2026-10-09）

- ACP 原始碼候選版 **`1.2.0-agentdock.13`**；工作樹 `/Users/wei/sideProject/antigravity-acp-next-integration`，分支 `feature/next-integration-optin`，HEAD **`5c49037`**，核心 Commit **`37f53dc`**。本機未推送、未部署；Stable 與 Next 安裝版均維持 `.8`。
- 新增 **`Translator.reserveExternalText(row, exactText) / ackExternalText(token) / abortExternalText(token)`**，並由 `StreamPoller.reserveExternalText(conversationId, stepIndex, exactText)`（連同 `ack/abortExternalText`）使用**同一個 live Translator**。只有綁定且已開啟的 SQLite DB、有**唯一完成列 status=3**、Session/idx/文字完全一致、較早的步驟已經處理，才能保留外部文字發送。
- 發送保留後阻止其他 Poller 更新插隊；送達確認後相同 SQLite 文字不再發送，但後續工具更新仍走 SQLite。遠端更新結果不確定或提交後 DB 文字改寫時 **fail closed**，不偷偷重播；Replay 仍以 DB 為準、不受此機制改動。
- ACP 整合驗證：**`bun test` 440 pass／0 fail（42 files）**；TypeScript/Lint（僅既有 1 warning、4 info）／macOS arm64 native build **PASS**。合成 + 臨時 SQLite 的新測試見 `tests/conversation/external-text-delivery.test.ts`，研究與剩餘整合門檻見 `docs/research/stream-direct-admission.md`。
- **仍未接入 `Adapter.runPrompt`，也未新增直接串流模式**：`AGY_ACP_STREAM_JSON` 維持預設 disabled、可選 shadow，SQLite 仍是唯一正式 ACP Update／History／Replay 來源。原生 M8 Hook 仍為離線 fixture，沒有可信 Next Core 授權 IPC。ACP 已送出 chunk 無法撤回，未驗證不可宣稱 direct/fallback 可無損切換，亦不可宣稱已改善首 token 延遲。
- **下一階段交接 Gate：** 待 Next Core ready 後，先用完全獨立的 Next 測試環境驗證 Stream/DB event parity、實際 Session 身分、工具/圖片/取消/模型/Keychain/OAuth、嚴格單一輸出序列與送達不確定時的失敗策略，再審核是否可新增 next-only direct opt-in。**本輪未執行真實 AGY `-p`／OAuth 測試，也沒有修改 Next Core/UI。**

## ACP .12 獨立研發 checkpoint（2026-10-09）

> 這是 ACP 端新增的 **直接串流驗證核心**，不是 Next Core 的整合需求變更，也沒有啟用直接串流。

- ACP 工作樹：`/Users/wei/sideProject/antigravity-acp-next-integration`；分支 `feature/next-integration-optin`；候選版 `1.2.0-agentdock.12`，最新 Commit `120ee25`（核心 `aad33b1`，由獨立分支 `b44f5d4` 驗證）。
- `src/agy/stream-direct-ledger.ts` 已加入純資料的 `StreamDirectLedger`：AGY stream 事件和 SQLite **最終完成列**的精確比對、Conversation/Step 身分、1 MiB/128 step 邊界、預約與確認機制、拒絕重複傳送、取消/工具事件/部分送出後不可安全重試的標示。
- 新增 14 項測試（`tests/agy/stream-direct-ledger.test.ts`），與 ACP 整合後 **全套 430 pass/0 fail（41 files）**；TypeScript、Lint、macOS arm64 build 均通過。
- **尚未接進 Adapter.runPrompt**：`AGY_ACP_STREAM_JSON` 仍只有 `disabled`（預設）和 `shadow`，沒有 `direct`。SQLite Poller 維持 ACP Update/Replay 唯一真實來源，不影響原版 AgentDock。
- 核心發現：傳給 ACP Client 的文字不可撤回；若日後 SQLite 改寫已傳送文字，或 `client.update` 可能已成功但回覆拋錯，單靠「fallback SQLite」不能保證零重複／零遺漏。因此 **正式直接串流仍需經獨立 UAT、工具／圖片／Session／Replay 對齊、可持久化的共用去重游標與故障處理策略**，才能由 Next Session 審核 opt-in。見 ACP `docs/research/stream-direct-admission.md`。
- 目前原版 AgentDock 與 Next 安裝版維持 `1.2.0-agentdock.8`，ACP `.12` 只在本機 Git 分支、未推送／部署。本輪沒有呼叫真實 `agy -p`、未觸發 OAuth，未修改任何 Next Core/UI。

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

## ACP .11 P1/P2 source checkpoint（2026-10-09，Next Session 請先讀）

**重要：只完成 ACP-side source-only 實驗功能，Next Core 的真實權限授權與正式串流仍未整合。原版 AgentDock 與 Next 目前均維持已安裝 .8。**

- ACP 候選版：`1.2.0-agentdock.11`，worktree `/Users/wei/sideProject/antigravity-acp-next-integration`，branch `feature/next-integration-optin`，HEAD **`3956dec`**；包含 `079384d` Hook subprocess fixture 與 `9f7fd01` stream-json shadow，未 push、未部署。
- 完整驗收：`bun test` **416 pass / 0 fail（40 files）**；`bun run typecheck` PASS；`bun run lint` exit 0（只有既存 warning/infos）；`bun run build:mac-arm64` PASS，編譯檔 `--version` 回報 `1.2.0-agentdock.11`。本次完全未執行會觸發 OAuth 的真實 `agy -p`。
- **P1 Hook fixture：** `src/agy/native-hook-runtime.ts` 提供新的 `--agentdock-native-hook` runner 與 `installNativeHookFixture`，在私人 HOME 產生 Hook config，透過可信獨立子程序 mock provider 驗證 allow/deny/ask-deny、payload/binding/digest、timeout/cancel、provider 失效與符號連結/權限拒絕。測試 `tests/agy/native-hook-runtime.test.ts` 7 pass。這只是 fixture，**未接到 AGY 正式 child HOME，也未接到 Next Core M8 Policy**；`AGY_ACP_AUTO_APPROVE=0` 仍拒絕全部真實 Headless Prompt。Hook 自身不具 OS 防繞過保證；請勿把此當正式授權模式。合約與限制：ACP `docs/research/native-hook-runtime.md`、`docs/research/native-permission-policy.md`。
- **P2 Shadow：** `AGY_ACP_STREAM_JSON` 預設 disabled，僅認得 `disabled` 或明確 `shadow`。設定 shadow 後才在**單次** AGY CLI 輸出加上 `--output-format stream-json`；`src/agy/stream-json-runtime.ts` 負責 bounded stdout/事件捕獲、與 SQLite DB 的觀察比對及固定錯誤碼 fallback。SQLite Poller 始終是 ACP 更新、取消、圖片及 replay 的唯一來源；**尚未支援 stream-json 直接輸出 ACP Update，也沒有長駐 input-format stream-json**。ACP `tests/agy/stream-json-runtime.test.ts` 和 `tests/acp/stream-json-shadow.test.ts` 24 pass。合約：ACP `docs/research/stream-json-runtime.md`。
- **Next Core 分工／Gate：** Next Session 可先在自己專用的 Profile/State/臨時測試環境驗證 `AGY_ACP_STATE_DIR` 與 `AGY_ACP_CHILD_ISOLATION=required`，再在明確測試用的 Next Profile 開啟 `AGY_ACP_STREAM_JSON=shadow`。須先處理原有 Sessions/state 轉移及環境注入，不能誤改 stable。真正 native PreToolUse M8 Bridge 需 Next Core 決策端、可信 IPC/auth/binding/lease/取消 API，且實測 AGY Hook 在失敗/繞過時是否 fail closed；在這些 UAT 前不要 enable auto-approval=0 成一般 Coding Profile。
- **尚待整合 UAT：** AUTH/Keychain 不重新跳登入、AGY tool Hook 是否真的觸發、allow/ask/deny 與斷線、模型/usage/Session、Broker enabled/disabled、stream stdout/DB event identity、取消/圖片/replay、reconnect/rollback。上述不可由這次 mock/純協定 smoke 取代；Another Next Session 持續獨立開發，本 Session 沒有變更 Next Core/UI 或其其他未提交檔案。

## ACP .10 新 checkpoint 與 Next-only 相容性 UAT（2026-10-09）

此節優先於下方舊版 .9 checkpoint；舊段落保留供交接追溯。

- ACP 工作樹：/Users/wei/sideProject/antigravity-acp-next-integration；分支 feature/next-integration-optin。
- 最新 Commit：5e720cd（依序整合 376a77d stream PoC、bbcf356 native Hook policy PoC）。**1.2.0-agentdock.10 僅源碼候選版，未推送，未正式部署**。
- 全套 Bun 測試 **385 pass / 0 fail**、TypeScript typecheck PASS、lint exit 0（原有 1 warning/4 infos）、macOS arm64 native build PASS。
- stream parser 位於 src/agy/stream-json-poc.ts，採用 AGY 官方 init / step_update / result NDJSON 事件格式；1MiB 單事件／32 queue 限制，支援多輪累計、UTF-8 chunks、紅線錯誤遮罩；將 1MiB-per-event 配置改為小緩衝區擴容/重用。**完全沒有接入 ACP runtime：現行依然由 SQLite Poller 處理 prompt/replay/圖片/取消**。完整設計見 docs/research/stream-json-poc.md。
- 權限研究找到上游官方 PreToolUse Hooks: https://www.agy.dev/docs/hooks/；src/agy/native-permission-policy.ts 僅實作可信 Session/Turn/operation digest、provider mock、Core allow/deny/timeout/cancel、未知工具拒絕的離線 PoC；docs/research/native-permission-policy.md 列完整 M8 Core 合約與限制。**Hook 尚未注入私有 AGY HOME、沒有 Next Core 授權端點；AGY_ACP_AUTO_APPROVE=0 持續拒絕所有 prompt，不可啟用為一般 Next Coding Profile**。
- **Next-only 實機相容 smoke 已通過**：先確認 Next Antigravity ACP 無執行中任務，臨時備份並原子替換 Next 專用 ACP 檔案為 .10；透過 Next Core ACP info 回報 1.2.0-agentdock.10、protocol 1，成功 new Session (ready、model options 可讀)、close Session (closed)。完成後**將 Next 專用檔案恢復 .8**、重新載入並由 Next ACP info 確認回報 .8；原版 stable 全程維持 .8、沒有動過。
- 此 smoke **不是新環境變數的 End-to-End UAT**：目前 Next Core 尚未設定 AGY_ACP_STATE_DIR、AGY_ACP_CHILD_ISOLATION=required，也未實作 M8 native PreToolUse 授權橋與 stream-json runtime 切換。下一個 Next Session 先完成 Next-only profile/state 路徑 owner/遷移與必要 env 注入、API/權限合約，才能執行這三項的完整跨程序 UAT；不要把 smoke 當成正式功能完成。
- 其他 Session 正在編修 Next Browser Broker 與 Route Planner 檔案，本 ACP Session **未觸碰任何 Next Core/UI source 或該工作樹的其他未提交變更**。

## ACP 交付 checkpoint（2026-10-08）

ACP 工作樹：/Users/wei/sideProject/antigravity-acp-next-integration
Branch：feature/next-integration-optin
Commit：4714f65（1.2.0-agentdock.9，未推送、未部署）
已驗證：293/293 Bun tests、typecheck、lint（僅既有非阻擋警告）、macOS arm64 build。

**實際新增的 ACP opt-in 變數及注意事項：**

| 變數 | 預設（Legacy） | Next opt-in |
| --- | --- | --- |
| AGY_ACP_STATE_DIR | 使用既有 HOME/.agy-acp | 指定 Next-owned **實體絕對路徑**，ACP 用它存 sessions.json 與 models.json；拒絕不安全路徑/權限/連結 |
| AGY_ACP_CHILD_ISOLATION | 維持 .8 舊行為 | 僅接受 required，強制所有 AGY 子程序使用白名單私有 HOME，Broker 有無均一致 |
| AGY_ACP_AUTO_APPROVE | 維持原本 --dangerously-skip-permissions 行為 | 僅接受 0：**所有非互動 AGY Prompt 目前都會 fail closed**，含 /usage；尚無 native tool approval hook，所以不能讓一般 Next 任務立即啟用！ |
| AGY_CONVERSATIONS_DIR | 維持原本對話路徑 | 可另指定，與 AGY_ACP_STATE_DIR 分開；使用前須驗證實體目錄存在、讀寫及 Poller 一致 |

**給 Next Session 的下一步：** 可先在 Next-only 試驗整合前兩個變數與 State 初始化／migration，但請勿直接啟用 AGY_ACP_AUTO_APPROVE=0 作正式 Coding Profile：該模式刻意拒絕全部 Prompt，不提供 M8 native tool approval。應先完成 Core Permission / native tool 安全邊界或 OS sandbox 的技術合約。stream-json 目前只有 ACP docs/research/stream-json-poc.md 設計文件，未改動 Adapter，也不可在 Next 宣稱可用。

原版 AgentDock、Next 目前已安裝的 ACP 執行檔仍是 1.2.0-agentdock.8；不動、不部署。上述 ACP commit 是供 Next Session 程式整合與 code review 的來源，不代表 native GUI/Core UAT 已完成。
