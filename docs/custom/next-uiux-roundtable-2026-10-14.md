# AgentDock Next — UI/UX 全頁檢視與 AI 圓桌（2026-10-14）

> 狀態：**工作線開啟／規劃文件先行；source inventory 與 AI 圓桌進行中**。分支 `feature/next-uiux-audit`，不等同 installed Next 原生 UAT；不改 stable Mac-Dev／P10 worktree／正式發佈。

## Round 0 / 問題與邊界

- **痛點**：狀態欄展示「載入中」突然插入或消失造成高度跳動；可用功能的 loading/disabled/unknown 與真正 backend 狀態可能不一致；長篇內部術語讓日常操作難辨；繁體中文殘缺；Menu Bar/Tray 資訊過少；ChatGPT 重複 MCP Apps 卡片。
- **直接改善**：使用者能迅速回答「AgentDock 可用嗎？哪些服務可用？有哪些進行中的工作？下一步如何恢復？」且不被假健康/假完成誤導。最小解：先縮減噪音、建立 State/Action 表與固定佔位，再按證據調整。
- **約束**：stable Mac-Dev 不可更動；Next 與 stable runtime/credential 隔離；不引入無需求的常駐 daemon、全域 polling、VM；不為 UI 放寬 M8/P8 權限；NexusDock 先維持「開發中」；P10 source-only、不發布。
- **方法**：產品/資訊架構、互動/無障礙/中文、前端/系統一致性、真實狀態/安全紅隊四角色 Round 1 獨立觀點→交互挑戰→決策；逐頁按 source、狀態觀察、UI 互動、真實 OS/ChatGPT UAT 不同證據層級建表。
- **最小驗收基準**：狀態佔位固定、無跳動（啟動/重新整理/錯誤/恢復）；按鈕能呼叫真實 authority 或明示 disabled/理由；狀態顯示來源、觀察時間與 stale/unknown；zh-Hant 在各主要流程無簡中/英文硬編碼；不搶焦點/無額外彈窗/不自動開啟 user browser。
- **不做**：正式發佈、擅自重啟安裝版、重新設計 Browser Broker 安全基礎、提前 P12 native deprecation；只做審查/文件/必要實機讀取，不擅改真實使用者資源。

## Screenshot 基線與責任差異

2026-10-14 使用者截圖：**stable AgentDock 1.0.1** macOS 選單列面板，頂部 AgentDock「正在运行」與綠色「运行正常」，內有「核心服务／正在运行」、「NexusDock／未配置」，下方「打开 AgentDock」「重启 AgentDock」「设置」。畫面混用繁/簡（例「正在运行、运行正常、核心服务、打开、重启、设置」），實際健康及按鈕是否可執行尚**未驗證**，不可從文字猜測。截圖中的服務列會否因 loading 被移除而跳高也**待重現**。應以 stable UI 做操作經驗參考，不擅修改 stable。
Next Wails source `desktop/shared-poc/main.go` 目前原生 tray 只有 `Show <product>` / `Quit`；Next 狀態面板仍是**擬新增的產品能力**，不是已通過的 parity。

## 必查頁面與固定檢核軸

詳見後續「逐頁 inventory」。每頁逐項查：
1. **Hierarchy**：主要問題/核心 CTA 是否最先可見，低頻 debug 進 advanced/detail；圖示、密度、行距與跨頁元件一致。
2. **Layout stability**：loading/error/empty/ready 對內容區高度、按鈕/提示的影響；避免 loading text 消失觸發跳動；最好以固定狀態槽、骨架或 reserved min-height，避免畫面閃爍。
3. **State truth**：`running` 是程序還活著、HTTP health、已驗證 connector、還是真正 MCP/ACP 工具可用？不得混用；最後觀察時間與 stale/unknown 明示。
4. **Action fidelity**：UI 可見按鈕必有真正 handler、支援的 OS 能力、失敗/復原結果、禁用原因、無效動作 guard；禁止「看起來可按實際無作用」。
5. **Locale**：zh-Hant 原生詞、英文/簡體字不混入；多語言長度測試、VoiceOver、鍵盤焦點、窄版與 Dark Mode。
6. **Non-interruption**：不搶焦點、不啟用使用者 Edge/Chrome、不彈未授權 consent/Keychain、不重啟 stable。

## P13 / P14 連動

- P13 依 [單一 Task ID 即時進度卡](upstream-ports/task-progress-live-card-pr229.md)，主源為 persisted Task，checkpoint/complete 不新增卡；需真實 ChatGPT host + cache + scope UAT。
- P14 依 [MCP Apps 卡片降噪](mcp-apps-card-noise-reduction.md)，僅避免不必要大型卡、不吞掉審批/錯誤；Host 不提供跨訊息替換時採 compact/no-op metadata 方案。

## Round 1／Round 2／逐頁 Inventory

> 初始框架已記錄；後續審查意見須標明「源碼推論／外部 reviewer／installed GUI 實測」與 evidence path，不得把預期視為已驗證。
## 逐頁 Source Inventory（2026-10-14，尚非 Installed GUI UAT）

判讀：以下「已知」僅來自 Next repo 原始碼。對實際畫面、卡片高度、操作結果、OS 健康狀態均必須另用真實 Next/App/ChatGPT host 驗證。P0=正式 RC 前必要，P1=高價值改善，P2=可後補。

| 頁面 | Source evidence / 目前功能 | 要逐頁驗證／設計的重點 | 優先 |
| --- | --- | --- | --- |
| Overview | components/overview/OverviewPanel.vue：Runtime、Connection、Update、Diagnostics 四張卡各自 refresh 並以 OperationStatus 顯示進度 | 四區塊不同時載入的高度穩定、狀態新鮮度；優先顯示可用性/需處理，debug 平台資訊下放 | P0 |
| Runtime | components/runtime/RuntimeStatusPanel.vue：Process/Health/Autostart/Nexus + Start/Restart/Stop/確認 | process running 不等於 Tool/HTTP healthy；核對 backend truth；理由貼近 disabled，runtime path 進 Advanced；Nexus 開發中而非已連線暗示 | P0 |
| Execution | components/execution/ExecutionCenterPanel.vue：SSE state + attention/reconnect/drop/cursor、Calls/Insertions/Facts | 先呈現正在執行、需處理與連線失聯，cursor/epoch/drop 收進 Advanced；失聯不得解釋為沒有任務，enqueue/cancel 真實驗收 | P0 |
| Connection / ChatGPT | components/connection/ConnectionPanel.vue：Port、OAuth、Local/Public URL、Tunnel、Copy、Autostart、確認 | 常用 Public URL 和可用性置頂；Port/Tunnel advanced；waiting_readiness/loading 固定槽；有 URL 不等於外部已驗證；密碼只經明示 Show/Copy | P0 |
| Connection / Nexus | components/connection/NexusDockPanel.vue：有 in_development badge，仍呈現 Pair Form、Reconcile、Refresh、Advanced | 「開發中」同時露出可操作 Pair/Reconcile 有誤導風險；依使用者決定先改 deferred/read-only 說明、操作除非明示重新啟用不得誘導 | P0 |
| ACP | components/acp/ACPPanel.vue：profile/session/default、enable、CRUD、lifecycle、adapter update | 先顯示可用 Agent 和 Session 健康、重要 CTA；profile 設定/版本/協定下放細節，loading/status 排版穩定；readiness 與需 restart 區分 | P1 |
| Browser | components/browser/BrowserPanel.vue：唯讀 Broker Snapshot、readiness、route；busy 插入 v-if Loading p；observedAt 用 ISO8601 UTC | 首次載入與 snapshot 換欄需固定高度；觀察時間以本地語言顯示；managed Chrome 可用不可推論公司 Edge已獲允許（目前 UNQUALIFIED） | P0 |
| MCP | components/mcp/MCPPanel.vue：首次 loading&&未有 snapshot 顯示 empty-state，後整塊變 server-list/detail；OAuth/recovery | 保持 skeleton/狀態占位；區分 configured/connected/tools verified；unknown tool count 不等於 0；權限/密鑰/錯誤揭示清楚 | P0 |
| Plugin | components/plugin/PluginPanel.vue：同樣 loading-to-snapshot 區塊切換，install/update/enable/remove/keep/purge | P6 handler 已通過 macOS UAT，但列表/詳情變高度仍待 UAT；使用狀態與權限摘要優先，技術版號下放；保留 irreversible confirmation | P1 |
| Permission | components/permission/PermissionPanel.vue：5s visible polling + focus/online refresh，stale/error/completed/revision 各自 v-if | 顯示有效需確認權限、再歷史紀錄/政策；state slots 固定以免錯誤/完成文字插拔跳動；Approval/Reject/M8 永不可被降噪隱藏 | P0 |
| Settings | components/settings/BasicSettingsPanel.vue：Port/Log Level/Autostart、Reset/Save、ConfirmDialog | Port 是否與 Connection 重複設定？Reset 是表單還原非 factory reset 要明確；disabled 說明可就近讀；Save 後需重啟的範圍 | P1 |
| System / Update | components/system/UpdateCard.vue：僅檢查更新，另註 native_only | 版本最新與需要下載/套用完全不同；P10 沒發行、P11 沒簽章 UAT，不可宣稱可套用；顯示通道/last checked 與實際下一步 | P0 |
| System / Diagnostics | components/system/DiagnosticsCard.vue：Logs/Config 的原生開啟及 disabled reason，兩個 OperationStatus | user macOS 手動已確認 Logs/Config PASS；仍須檢查兩組載入中/完成文字消失是否移動卡高，非重要平台/路徑折疊 | P1 |
| Developer | components/contract/ContractStatusPanel.vue、ActivityStreamPanel.vue：contract 與 experimental stream | 明確 Experimental，避免佔普通使用者首頁；診斷數據與秘密不得混淆、UI 可用與 Core 狀態需分離 | P2 |
| macOS Menu Bar / Windows Tray | desktop/shared-poc/main.go：Next tray 僅 Show <product>/Quit；tooltip 只有 app 名稱且文案是英文；截圖來自 stable 1.0.1 的另套原生介面 | 以小型可信狀態面板為目標：核心服務「真正健康 + 最後觀察」，ChatGPT 連線、進行中/待處理任務（如來源可靠）；Open Next 主要操作，Settings/Logs 次要、Restart 確認；Nexus 開發中；避免新 daemon/高頻 polling/搶焦點 | P0 |
| ChatGPT MCP Apps | internal/app/mcp_apps.go、internal/mcp/apps.go：task_manage 綁 Task UI | P13 單 Task ID 一張可更新卡、checkpoint 不新增；P14 同值 model/effort、重複 Workspace 狀態低噪；不同工具訊息不能假設 Host 自動合併，權限及失敗不能隱藏 | P0 |

### 共通 UI 問題（已由 source 指認）

- OperationStatus.vue 用固定 p 容器但文字可能空白或因長錯誤換行改高度；不能僅以 p 仍存在聲稱無 CLS。以固定 status row 最低高度 + 可展開長錯誤，避免狀態消失突然收縮；不應無差別全頁 skeleton。
- BrowserPanel.vue 的 v-if busy loading 與 v-if snapshot 切換、MCPPanel.vue 和 PluginPanel.vue 的 loading/empty/snapshot 替換、PermissionPanel.vue 的多個狀態 p，屬優先實測的 layout-shift 候選（目前沒有量測，不能稱已證實 bug）。
- i18n 應包含 Wails tray 的 Show/Quit、native menu/tooltip，以及動態錯誤、時間格式，不只 Vue 字串；stable screenshot 裡「正在运行、运行正常、核心服务、打开、重启、设置」是簡體字表現，仍需驗證 Next zh-Hant 選擇/回退。
- Next Wails 最小原生寬度 760px（main.go normaliseWindowSize）；320 CSS viewport 可作 Web layout 掃描，但不能謊稱能把 native 窗口縮到 320px 原生 UAT。

### 驗收矩陣（全部待真實驗證）

| Gate | 設計/測試輸入 | PASS 需提供的真實證據 |
| --- | --- | --- |
| U01 Layout | Idle/Loading/Ready/Empty/Error/Stale/Retry，同一尺寸反覆 10 次 | 各頁各卡高度前後對比，首屏主按鈕無位移；先收集 CLS/畫面坐標基線再訂容許值，不可用虛構測量 |
| U02 Reality | Core stop/start、API timeout、OAuth 失效、外部 Browser unknown | 顯示程序、health、MCP 可用、Observation time 源分離，無未知假綠燈 |
| U03 Actions | 每個可見 button/link/tab/selector/menu | 對應真實 handler/API 與 OS 能力，成功/失敗/disabled reason、重試與取消均有驗收證據；Nexus 開發中行為正確 |
| U04 Locale | en/zh-Hant/zh-Hans、system zh-TW、長字串、timeout/error | 主要頁與選單列/Tray、確認視窗、失敗/連線/日期均完整繁中，不夾簡中/英文硬編碼 |
| U05 A11y | Keyboard/VoiceOver/125-200% zoom/Dark、760px native 與 320px CSS viewport | 合理 tab/focus/aria-live/contrast/wrapping，不能將 320px CSS 通過冒充原生 320 |
| U06 P13 | 一個 Task 的三次 checkpoint + review/complete；2 Task 並行 | 一個 task_id 一張原卡更新、Terminal stop poll、無重複/跨 scope；失連顯示 stale；工具舊 metadata 邊界 |
| U07 P14 | Workspace 重複 10 次、同 model/effort 重複、Approval/Error | 可見大型卡高度比基線下降，重要變更/錯誤/審批仍清楚可追溯 |
| U08 Tray | menu open/close、Core health 變化、Next/stable 共存 | 準確或 unknown、無 layout jump/明文秘密/無頻繁背景連線、只操控 Next owner、不搶焦點 |

### 首輪待挑戰的衝突

1. 固定 status-slot/局部 skeleton vs 全局 skeleton：哪個能以最小成本避免跳動而不產生假載入感？
2. NexusDock 仍有 Pair/Reconcile UI 但又標「開發中」：應先完整隱藏操作，還是只針對能力 disabled？
3. Tray 資訊密度與效能：如何只以可信 existing Core Snapshot 呈現，有效又不常駐大量輪詢？
4. Task/MCP Apps 降噪：ChatGPT host 可能無跨訊息 replace，何種回退最能改善視覺負擔且不失去審計？

## AI 圓桌 Round 1 — 技術 / 狀態真實性 / Red Team（Codex 6.1 Sol，唯讀完成）

> 外部 Codex ACP 唯讀 reviewer 已完成 repo 審查，沒有修改檔案或操作 GUI。本節為其主要 findings 經 Orchestrator 對程式路徑抽核後的摘要；仍需獨立修復與真實 UAT。設計提案不可冒充已通過。

| Issue | source / 發現 | 重要原因 | 建議 |
| --- | --- | --- | --- |
| T-01 Runtime unknown 被折成 false | stores/runtime.ts 的 emptyStatus() 將各布林設 false；RuntimeStatusPanel.vue 直接顯示停止／不健康 | 初始尚未載入、後端讀取失敗和真實停止可能混同 | 改可區分 not_checked/stale/unknown 的 view model，失敗保留資料但必標過期；首屏及細項一致 |
| T-02 Connection mutation error 被 refresh 蓋掉 | stores/connection.ts 的 runMutation 與 readSnapshot 都對同一 error 欄賦值 | 操作拒絕後又成功取得 snapshot，使用者可能看不到錯誤原因 | 將 action feedback 與 refresh error 分離，成功/未知結果也需明確證據 |
| T-03 ACP status 舊資料仍可觸發控制入口 | stores/acp.ts 的 readStatus/readSettings 失敗保留舊資料，canInvoke(close/updateLifecycle) 倚靠 status 非空與契約能力 | 「設定可用」不等於 Adapter / Session 當下可控 | stale 與 cannot verify 時明示、操作按權威 gate 限制；不可錯誤關閉已存在 session |
| T-04 MCP OAuth 可以點但不執行 | components/mcp/MCPOAuthStatus.vue 的登入/清除僅 :disabled=store.busy；stores/mcp.ts 的 runMutation 因 stale/snapshot/permission 不符回 null | 具體 clickable no-op 風險，符合使用者指出情境 | disabled 綁 store.canInvoke 並就近說明原因；同樣驗 Plugin/MCP recover |
| T-05 MCP/Plugin 查詢 Unknown 結果未必更新主狀態 | stores/mcp.ts / plugin.ts 保存 OperationStatus 到 lastOperation，主要提醒讀 lastFeedback | 回收不確定操作結果後 UI 可能仍顯示 outcome_unknown | 明確 reconciled phase/lastFeedback 同步，不自動重發 mutation |
| T-06 Browser 重新整理會清空已有 Snapshot | stores/browser.ts::refresh() 開始 snapshot=null，BrowserPanel.vue v-if 整段資料被卸載 | Loading/Ready 高度跳動候選，也是有可信 last observed 卻暫時空白 | 保留 readonly stale snapshot 與 fixed busy slot，根據刷新結果更新，源的能力證明不能放寬 |
| T-07 Nexus Token 沒 Snapshot 時顯示未儲存 | components/connection/NexusDockPanel.vue：snapshot?.deviceTokenStored ? saved : not_saved | 未觀察不是確定未儲存，加上「開發中」但保留 Pair/Reconcile 容易誤解 | unknown 狀態、簡化 deferred 頁、低風險唯讀診斷，實機配對仍延後 |
| T-08 Overview Update 新鮮度 | OverviewPanel.vue 只在 update 沒資料時刷新 | 回頁面可能用舊結果宣稱最新；「目前版本」不等於 checked now | checked-at/stale，更新結果按來源呈現，不主動高頻檢查 |
| T-09 Developer 真假健康 | ContractStatusPanel.vue negotiated contract vs 服務健康；ActivityStreamPanel 目前 synthetic probe | 測試/支援能力會被解讀為真實 ACP 調用 | 開發者頁標明 simulation/contract，進 Advanced，不混入 production health |
| T-10 Tray 沒資訊，退出不等於停止 Core | desktop/shared-poc/main.go 只 Show/Quit；quit() 停 Wails 自身 probe 退出 App，非 Core stop | 不可將舊 stable 1.0.1 样貌當 Next 能力；不可誤導 Quit 等於 Restart Core | 先做本地化 Show/Quit + 可信唯讀 health，確認 refresh 來源後再評估 Restart/Settings 等二階動作 |

### Round 1 技術 reviewer 對可保留設計的肯定

- PermissionStore 比對 runtime epoch/revision 和 fresh 狀態、用明確確認執行，應保留 M8 失敗關閉；不能因 UX 精簡隱藏 Approval/版本衝突。
- Connection 有 port/tunnel revision 與 endpoint 對照邏輯；ACP profile 管理、Browser read-only contract 及 Plugin P6 已有 authority guard，應修 UI 層錯誤／快取真實性而非重構權限。
- Wails 最小視窗寬 760，native 320 不可驗收；Swift 舊 AppDelegate 的 15 秒狀態檢查與 Menu 是參考，不應直接照搬到 Next 多進程架構。

## Round 2 — Orchestrator Targeted Challenge / 初步取捨

| 分歧 | UX / Product 主張 | Runtime / 維運反方 | 收斂決策（待外部 UX reviewer challenge） |
| --- | --- | --- | --- |
| Loading 防跳動 | 每卡都加 Skeleton 防止文字跳動 | 全站 Skeleton 增加動態高度與感知等待，過度設計 | **先以固定 status slot + 有資料時沿用 stale snapshot**；僅初始大面積 list/detail 需要少量 skeleton；用實測坐標/CLS 調整 |
| Tray 資訊量 | 放核心健康、MCP、ACP、Plugin、Browser、工作量、最新版本等全面清單 | 菜單列應小而可信，所有健康狀態不同信任層級、頻繁輪詢容易干擾 | **只先給 Core health / Connector evidence / 有待處理數的可信聚合 / observed-at**；全部未知時明示 Unknown，不創建新 service/高頻 poll；詳細資訊交主窗 |
| Nexus 「開發中」 | 保留預先設好的 Pair UI 便於未來啟用 | 現在展示可按 Pair/Reconcile 與延期決定矛盾、可能誤導 | **預設改 read-only/in development**；若之後需求重啟才開操作能力與驗收，保留 backend 原型不刪除 |
| P13/P14 擴展 | 最理想是所有歷史卡片原地消失或自動折疊 | MCP Host 未保證跨工具訊息替換，別冒稱已能操控 ChatGPT UI | **P13 單 Task ID 原卡狀態更新 + P14 action-specific minimal UI/result**；保留失敗/批准/有變化，舊 card cache 實測 |
| 介面重做順序 | 先做全面視覺改版 | 若狀態假健康、clickable no-op，視覺改版仍不可信 | **先修 T01–T07 的 state/action fidelity，再固定 layout，最後簡化 IA/字串**；同頁獨立檔案可平行不共享 unsafe write |

## 交付切片 / 後續驗收責任

- UX-S0：先補 UI 截圖/影片與 Core state 對照，源碼 inventory 不宣稱原生功能正常；Menu Bar macOS、Windows Tray 分別驗收。
- UX-S1：狀態語意／權威 refresh（Runtime/Connection/ACP/Browser/Nexus），有單元測試 + failed/stale/unknown 的 UI 交互；不能更動 stable。
- UX-S2：clickable no-op / reconciliation（MCP OAuth、MCP/Plugin 查詢未知結果）；稽核每一個 CTA，保留操作回覆／M8 safe retry。
- UX-S3：共享 loading 區域的固定高度、文字折疊策略、日期本地化，及首次載入時大列表 placeholder；避免 full re-render 跳動。
- UX-S4：Overview/Execution/Connection/ACP/Broker 等首屏主資訊 vs advanced 技術詳細；整體 zh-Hant、accessibility、min-width 矩陣。
- UX-S5：Next Menu Bar/Tray 最少可信狀態聚合 + 本地化/非干擾；P13/P14 可作獨立實作支線（需 ChatGPT host UAT）。
- 所有切片均先 source test，再 installed Next UAT（原生 GUI/真實 API/無焦點干擾）；P10 source-only release prep 可同步，但正式 RC freeze/P11 仍需必要 UIUX 合入後才可驗收。

## AI 圓桌 Round 1 — Product / UX / i18n / Accessibility（Gemini 3.8 Flash High，唯讀完成）

> 第二位獨立外部 reviewer 使用 Gemini/Antigravity Plan Mode，只讀 Source，不觸碰 installed App 或系統服務。原本規劃的 Gemini 3.1 Pro 模型不被本機 AGY CLI 辨識，因此依實際提供的模型改 Gemini 3.8 Flash High 完成；不能冒稱使用 3.1 Pro。其所有視覺跳動結論仍待錄影/座標量測。

| 面向 | UX reviewer 的差異化發現 | 與技術 reviewer 的交叉結論 |
| --- | --- | --- |
| 檢視主頁 / loading | Overview 四張非同步卡片與長 public URL 可造成換行、區塊高度不一致；OperationStatus.vue 空 p 的文字切換可能讓主 CTA 移動 | 兩者同意先固定各狀態槽，必要時只於大面積空資料使用 skeleton；先量測不虛稱 CLS=0 |
| Runtime 版面 | 行內確認區塊 v-if confirmationAction 插入有位移風險；Nexus 連線欄位會造成與 deferred policy 不符的注意力 | 保留 M8 Runtime 確認機制，改固定區域或有正確焦點的確認視窗；狀態 unknown/actual stop 要先修 |
| Execution | cursor/epoch/transport_dropped 對普通使用者干擾；無 active call 時需要友善空狀態 | 保留可稽核 metrics，在 Advanced 展開；不可將 SSE 中斷當沒有執行中任務 |
| ACP | ACPManagedProfileCard.vue 的多動作按鈕在窄版下可能形成多層換行 | 主操作＋次操作 Menu，必須保留啟停/更新/刪除的獨立確認與 disabled reason |
| Browser | BrowserSnapshotDetails.vue 展示大量內部 Broker 統計，但日常最重要的是「路由能否使用、為什麼不能」；observedAt 顯示 ISO 字串 | 技術 reviewer 補充 Store refresh 直接 snapshot=null，先處理狀態/保留 stale 來源；日期格式化加輸入守衛，不能僅因缺值推論一定有 crash |
| MCP/Plugin | stale/read-error/recovery banner 分散，選擇詳細卡後返回列表時焦點恢復需 UAT；候選 dialog 需焦點圈 | 合併展示互斥或相依的狀態訊息而不吞掉任何未知結果；保留既有 P5/P6 authoritative safety |
| Permissions | 技術 enum/revision 直接印在 ConfirmDialog；重要批准不應被設計降噪 | 翻譯為人可理解的動詞/風險/作用範圍，工程 ID 可展開，但 M8 approval 持續醒目與安全 |
| Connection/Nexus | Nexus 子 Tab 尚未在導覽標「開發中」；詳細禁用理由重複佔高 | Tab 加開發中並以現況延期為準，預設只讀；工程診斷 Advanced，不把可按 Pair 假裝已就緒 |
| System/Settings | Update check 無安裝動作，會是「下一步去哪裡？」；Settings port 與 Connection 重複 | 明確顯示 check-only/P11 Native，設定避免雙入口且 deep link 一致；已可用的 Logs/Config 不重作 |
| Menu Bar / Tray | 應有 Core 健康與啟動狀態、常用捷徑與繁中，不重做全套 Desktop | **採最小可信摘要**：Core/Connector/需注意事項有 source 才顯示，沒有就 Unknown；複製 URL 需已驗證 Next-owned 來源，避免洩漏；不加昂貴輪詢 |
| Developer | 合成 Activity probe 不可與真實 ACP 執行混淆 | 清楚標實驗性，移入進階頁，避免假警報 |

## Round 2 — 主持人 Cross-Challenge（兩位獨立觀點已交叉收斂）

1. **先修 CLS，還是先修假狀態？** UX reviewer 強調 jump/密度的可見痛點；技術 reviewer 發現 T01/T02/T03/T04 有真實誤導操作的 source 風險。**裁決：truth/action fidelity 先列 P0；同時用 shared status-slot 作獨立低耦合 P0 切片。** 兩者並行開不同檔案，整合驗收以 Native GUI 完成為準。
2. **Nexus 開發中是否顯示 Pair 表單？** UX reviewer 傾向標記 (開發中) 並保留頁籤；技術 reviewer 指出按鈕仍存在可能讓使用者誤以為正式支援。**裁決：保留 Connection 子頁導覽與開發中標記，但普通使用者不主動呈現可執行配對/恢復操作；必要唯讀 status 放 Advanced，避免使用者誤操作；後續可再以明確開發開關開啟測試。**
3. **Tray 應有多少資訊？** UX reviewer 提議健康、埠號、複製 URL；技術 reviewer 擔心靜態/快取結果被當作即時連線或洩漏秘密。**裁決：第一期只做狀態來源 + 標準啟閉 + 繁中；信任級別獨立（Core process/health、Connector confirmed、work attention）且有時間戳，複製 URL 經 Next ownership/scope 檢查後第二期才給；不公開 OAuth password。**
4. **要不要把 Browser 日期格式化的風險當 S1 崩潰？** UX reviewer 指出 toISOString 對 invalid date 可能拋 RangeError；技術 reviewer 提醒 Source DTO 已有驗證界線。**裁決：列 P1 防禦性 formatting/本地語系改進與 invalid-fixture 測試；未重現時不得宣稱 Browser 真實 crash。**
5. **320px 是否算原生 UAT？** UX reviewer 提 320px 設計驗收，但 Next Wails 最小寬度 760。**裁決：分開 CSS 320px headless/browser layout 與 native 760px/zoom/VoiceOver；不得以 320 CSS 冒充 native 320；Windows 原生額外獨立驗收。**
6. **Permissions 要不要取消 5 秒 polling？** UX reviewer 認為無條件輪詢過多；技術 reviewer指出目前 visible+confirmation gate 並有使用者等待審批需求。**裁決：先測負載、真實等待/焦點與 timer teardown；只在證據表明影響效能或 UI 時再調整頻率，不先拆既有安全監聽。**

## 主持人總結 / 接受的設計原則

**共識**：排版、狀態真實性、可操作性、文案密度與 zh-Hant 都是同一套 UX 質量，不可單獨只修 CSS。先以最低改動排除誤導與不可靠狀態；低頻診斷藏 Advanced、關鍵權限/失敗仍浮到首屏；單卡/降噪需遵守 ChatGPT Host 能力邊界。用真實 Next GUI + user flow 的錄影、API 狀態、鍵盤、Focus/Privacy 驗收完成，而非只宣稱 source PASS。

**仍待驗證**：本輪沒有取得 installed Next 全頁 screenshots/真實 UI click、卡片高度量測、macOS/Windows Tray 真實選單 UAT、ChatGPT MCP App 更新/metadata cache 實測；本輪也沒有直接改動 Core/Vue runtime。這些列為 UX-S0 至 S5 及 P13/P14 的後續 tasks，**不得當作已修完**。

**Implementation sequencing**：
1. UX-S0 獲得 installed Next 視覺基線/各頁 loading/unknown/failed/empty 錄影，避免打擾正在使用的 stable 控制面。
2. UX-S1 修 Runtime/ACP/Connection/Broker 的 unknown/stale/權威 feedback（來源 T01–T03/T06），逐頁測狀態一致。
3. UX-S2 修 MCP OAuth clickable no-op 與 Plugin/MCP unknown reconciliation（T04/T05），進行針對性 UAT。
4. UX-S3 固定 Loading/Copy/Recovery/Confirm status slot、合理處理動態高度，並完成追蹤差異/視覺量測。
5. UX-S4 信息架構/中文與 Accessibility：Overview/Execution/Browser/ACP 簡化、狀態版型、窄版、Focus/Keyboard/VoiceOver、時間格式化。
6. UX-S5 Menu Bar/Windows Tray 本地化 + 可信唯讀服務狀態與可執行入口；P13/P14 可各自獨立修復與實機聊天驗收。

**與 release 交互**：P10 是 source-only 版本/Tag/Installer/Pipeline prep，繼續不阻擋；必要 UX/權威修正與 ChatGPT Card 工作結束並 UAT 後再凍結 Release Candidate；P11 macOS Developer ID/Updater/Rollback 和 Windows/WSL/Linux 原生驗收不提前混入 UI/UX。

## AI 圓桌第三方 Claude Sonnet 5.5 專家複核（2026-10-14）

> 執行者：AGY ACP `claude-sonnet-5-5`、High effort、Plan Mode，**唯讀**（實際 ACP 執行完畢）。建立在已存在的 Gemini/Codex 兩位 Reviewer Round 1/2 之後，僅進行窄範圍反例審查與主持人整合；**沒有啟動 installed Next GUI、部署或修改程式碼**。原始 Code review 使用指定 store/component/Main tray 以及 P13/P14 規格；下述凡涉及畫面結果均是「待真實 UAT」。

### Claude Round 1 — 對既有結論的挑戰

| 原評估 | Claude 補充／不同意 | Source evidence 與驗證程度 | 收斂決定 |
| --- | --- | --- | --- |
| **T02 Connection operation error**（P0） | 問題不只是錯誤被覆蓋；mutation 成功、snapshot refresh 卻失敗時也可能同時顯示 `completed=true`、`snapshot=null` 與 `error`，回傳值卻為 true | `desktop/shared-poc/frontend/src/stores/connection.ts` 的 `runMutation()` 對同一 `error` 賦值、接著 `readSnapshot()` 也會覆寫。Source verified，GUI 提示尚未重現 | **P0**：分開 `actionFeedback` 與 `readError`，保留操作結果；操作成功但刷新失敗需顯示「已受理，最新狀態尚未確認」，不可強行標 complete healthy |
| **T01 Runtime 初始全 false**（P0） | 舊結論把「讀取失敗」也可能顯示停止說太廣：store 的 `stateKind` 在有 `error` 時會返回 `unavailable`。真正源碼缺口是「首次取得狀態前 false 導致 stopped」，以及 loading 覆蓋最後已知狀態 | `stores/runtime.ts`：`emptyStatus` 初值全 false；`stateKind` 按 `loading → error → !running` 判斷。子欄位還需讀 panel/實機驗收 | **P0 限定補強**：加 `observed=false` 或 null+unknown，修首次載入語意；先不用重寫所有 ViewModel |
| **T03 ACP stale**（原 P0） | `canInvoke` 依舊 `status` 判斷、readStatus 失敗保存 snapshot 確實存在，但尚無「權限外洩」的 source 證據；Core 再檢查權限 | `stores/acp.ts` source verified；具體操作結果、後端授權與失敗仍待 UAT | **P1**：顯示過期狀態、disabled/重新驗證時機；保留 M8 guard，不新增權限架構 |
| **T04 OAuth clickable no-op**（P0） | 新增更高價值風險：`authorization` 是**共用單值**，`MCPOAuthStatus.vue` 首選該值而非目前 server 觀察；切換不同 Server／generation 可能短暫顯示前一個的「已授權」，尤其失敗時舊值不清 | `stores/mcp.ts` `authorization`（單例）與 `readAuthorization(server)`、`MCPOAuthStatus.vue` 的 `status`；`MCPPanel.vue` 雖以 serverName:generation key remount detail，Store 仍未依 server/generation 隔離。**Source risk verified，實際錯顯尚未重現** | **P0**：OAuth status 與 callback options 用 `serverName + generation` 或受控當前選取範圍繫結；請求序號拒絕過期 response；Login/Clear disabled 與 `canInvoke` 一致並說明原因 |
| **T06 Browser loading 跳動**（待測） | Browser refresh 清空 snapshot 是明確 source；然而「某頁必然跳動」沒有坐標證據。MCP 已有 stale 保留模式，可選擇性借用；**不能對 Connection 同樣套用**，因其當前 fail-closed 避免顯示過期 Public URL | `stores/browser.ts` 在 refresh 先設 snapshot=null；`stores/mcp.ts` 已有 stale gate。視覺幅度未測 | **P1 視覺 + P0 保真**：允許 Browser 顯示明標過期的上次 Snapshot，動作仍 guard；Connection 保留 fail-closed 清空授權/連線快照行為，僅先分離錯誤回饋 |

### Claude 發現最容易忽略的產品錯誤：隱藏視窗時的 Tray 假健康

`desktop/shared-poc/main.go` 的 Wails Tray 目前只有靜態 Show/Quit，**沒有具備 provenance 的 Core health reader**。關閉主視窗後前端的 refresh/visibility 可能停下來，讓 Tray 若直接讀 Vue Store 就長期顯示過期綠燈。**這是設計風險，不是已發生的 bug**。

**主持人採用的最小 MVP 契約：**
1. Tray 由 Next-owned Go/desktop authority 直接取得**一次性、有期限的唯讀健康資料**（具體是否已有 Service 需進 UX-S5 前實查），不能從 `running` 或 Vue 的 `false` 推論健康。附 `source`、`observedAt`、`stale` 與分層狀態。
2. 優先考慮「使用者打開 Tray／前景恢復」等既有事件觸發有界 refresh；先確認 Wails v3 有無 menu-show hook，沒有就採低頻 TTL/顯式刷新，不照搬舊 Swift 的 15s 輪詢、不新增 daemon。必要資料源未準備好時，**可先只交付繁中 Show/Quit＋未知狀態**，不得先放綠燈。
3. MVP 僅用能證明的 Core health、必要時新鮮的 M8 attention count（不確定則 `—`，非 0）、public endpoint 已驗證狀態（必核對 `configRevision` / `tunnelGeneration`）。不宣稱 ChatGPT 連線正常、MCP/ACP 全可用、Nexus 已配置、升級已最新；敏感憑證不在 Tray 顯示。
4. `Quit` 只退出 UI、**不等於停止 Core**，需清楚說明。暫不在 Tray 新增 Restart Core；須待 Next-owned authority + confirmation 與原生非干擾 UAT 完成。更新標籤不呼叫 `Show`、不偷焦點。

### Loading／卡高設計契約（Claude 建議，主持人採用）

- 沿用 `OperationStatus.vue`，新增 `idle/busy/ok/error/unknown/stale` 可辨識的顯示狀態，**舊版 `busy/error/completed` props 保持相容**，逐步採用。以 `--ad-status-row-min-h` 這類 token 預留 1–2 行，優先相對字級 `em/lh` 以支援縮放；`<p>` 不隨切換卸載，長錯誤可展開、不可裁掉核心內容。
- 有可靠上次 snapshot 時顯示 `stale` 與 timestamp；只有**無任何已驗證資料**時用 3 種最低成本的摘要／列表／詳細區占位。**Connection 的敏感配置與端點不應無條件保留**：需看來源與安全語意分頁決定 fail-closed / hold-last-value。
- 原生 UAT 不假定 WKWebView 可用 `PerformanceObserver.layout-shift`；以控件 `getBoundingClientRect` 或同一縮放倍率截圖測首要 CTA/卡片的位移 px，初始、刷新、成功、錯誤、過期、重試各 10 次，macOS native 最低 760px 與一般 1080px，Windows 另驗。**先量基線再設定門檻**，不要寫「已做到 CLS=0」。

### P13／P14 邊界、額外評估

Claude 確認既有規格已正確防止三種誤區：不同 Tool Message 不假裝能直接替換、`UIVisibility: ["app"]` 不等於服務端 ACL、no-op 必由權威 state 產生。

補充：`visibilitychange` 只保證 Host 頁面隱藏，不保證**卡片捲離視窗但頁面還可見**時停止輪詢；評估 `IntersectionObserver`（視實際 MCP App iframe Host 支援）或 polling TTL/backoff，上限實測，**不預先假設 Host 行為**。P13 Task 單卡和 P14 MCP App 卡片降噪與 Tray 原生介面**分開 UAT / 分開完成門檻**，不綁同一 UX-S5 結案。

### Round 2 兩個問題／主持人回覆

1. **問：隱藏視窗後 Tray 靠什麼維持可信資料？** 決定 Go 端使用 Next-only 可信快照＋觀測時間，若未獲可信來源退回未知／僅提供 Show/Quit；這不阻擋其他 UI 修改。
2. **問：Connection 讀取失敗清空快照是否也要因防跳動而保留？** **不全面保留**。Connection 的 port/OAuth/Public URL 很可能隨 generation/revision 失效；保留易誤導。先維持 fail-closed，只拆 mutation feedback 和 read error；待實測再設限定安全的摘要舊值。

### 修訂後的最高優先序與可驗收證據

| 順序 | 切片 | 通過標準（source test → installed UAT） |
| --- | --- | --- |
| **1 / P0** | UX-S1：Connection actionFeedback/readError 分離、Runtime 初次尚未觀測不可標 stopped | mutation 失敗後 read 成功，原錯誤仍明確可見；mutation accepted/read 失敗不能假稱完成可用；Runtime 首次 `observed=false` 為 unknown，真停機才 stopped |
| **2 / P0** | UX-S2：MCP OAuth status server/generation scoped + stale disabled | Server A→B 切換、A 延遲成功、B 失敗均不顯示 A 已授權；stale/capability 不足 button disabled 且有原因；不觸發無提示額外 OAuth |
| **3 / P1** | UX-S3：Browser client stale snapshot、共用 fixed status slot、Native UI 位移基線 | Browser refresh 時顯示「上次觀測」而不整塊消失；畫面不同狀態切換有測量 px 與 baseline，CTA 不因 loading 進出跳動；未經量測不宣稱視覺 PASS |

**驗證等級：** 本次只有源碼和文件複核，全部視覺 UAT 待實作後執行。與 P10 release-prep 平行，不混入 P11，stable Mac-Dev/Next live runtime 保持不變。
