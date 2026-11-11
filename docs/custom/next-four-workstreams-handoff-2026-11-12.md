# AgentDock NEXT — A／B／C／D 並行工作線共同交接

> 日期：2026-11-12，台灣時間。本文件是四個新 ChatGPT Session 的共同任務規格，**開始前先讀文件；只執行被分派的工作線**。文件所載 Git SHA 與 Runtime 狀態為歷史快照，執行前務必重新核對。

## 0. 共通規則與已確定的產品狀態

使用 **Mac-Dev MCP、聊天模式**。本任務規劃不使用 Codex ACP、不切換 Work 模式。四線可並行，但需使用**不同 Git worktree／branch**，禁止同時寫入同一檔案、服務或環境。不任意修改 Original Core 8765、NEXT Core 8767、原有 Edge／Chrome Profile、其他 Session 或 MCP/ACP 資源。沒有明確授權時不 Push、Merge 到 custom/main、Tag、Release、不重新部署、不要求 root 或繞過 macOS 系統授權。

專案：/Users/wei/sideProject/agentdock；自有公開 GitHub origin：jacktdry/agentdock。正式客製整合分支是 custom/main，不等於目前 RC21 Source 或 Docs 分支。

- **RC21 Source**：/Users/wei/sideProject/agentdock-next-h04-h02-r01-20261111；fix/next-rc20-h04-h02-r01-20261111；已知 HEAD 0a0a0ba98412c6f22f8793a6abd184d6db62a27e。
- **RC21 Docs**：/Users/wei/sideProject/agentdock-next-rc20-deploy-compat-20261111；fix/next-rc19-to-rc20-deploy-compat-20261111；本文件建立前 HEAD 97a22c7dcdf5ce31d96a6f50dcf818fd5a96cf64，**本次文件提交將讓 Docs HEAD 再前進**。
- 共同祖先為 297b26b2；前次核對 Source 此後有 1 個專屬 Source Commit，Docs 有 5 個專屬文件 Commit。A 操作時必須把本次交接與索引的新提交也納入差異清單。
- **已結案**：H02 NEXT Tray 辨識、H04 原始 ACP Race fixture 修正、RC21 NEXT-only 本機建置／安裝與簽章回滾備份。H04 大規模 CGO/Swift 並行時的 1 秒 fixture 逾時只是已知風險，不因此重新開啟無限驗收。
- **P8 已有真正實機 PASS**：2026-10-14 ChatGPT／Codex／Antigravity 三個實際 Actor 共用同一 Broker、一個 Chrome main，各自 Lease/Page、跨 Owner 拒絕、依序 Release。共享模式目前只以 NEXT 專用設定顯式 opt-in 啟用，預設 exclusive；不可冒稱預設已共享。
- 公司 Edge 的 RC15／RC16 已授權路由、正負向範圍檢查已有實機 PASS；不代表所有未授權 Profile 都可直接連接。
- **目前不做**：P13 Task 長駐卡片、P14 降噪（使用者已選 deferred）、NexusDock（deferred）、Native MCP 無獨特用途的 Backend（NO-GO）、無授權的正式 Release。管理者彈窗來源未確認，不得歸因為 NEXT 已證實缺陷。

**共同退出標準**：工作線需指出使用者痛點、最小解法、具體改善與必要驗證，不重測已有真實 PASS 的工作、也不再延伸 AX/Focus/Race/rollback 驗收。完成後須交付獨立 Commit SHA、修改檔案、證據、仍存在的 blocker 和是否動到現役服務。若證據表明沒有實際產品缺口，可明確結案為「無需更動」，不可為了有提交而新增抽象框架。

## 1. 工作線 A — RC21 Source + Docs 基準整合

**目的**：解決 Source 與 Docs 分成兩條分支、後續開發無統一真實基準的問題。A 是唯一負責共用整合分支和最新整合 Backlog 的工作線。

**執行範圍：**

1. 先核對兩個 worktree、git status、merge-base、HEAD、Source 對 Docs 的獨立提交。**要包含本交接文件最新 Docs Commit**，不能只照舊記錄的五筆。
2. 從 RC21 Source 已知 HEAD 建立獨立工作樹及 integration/next-rc21-product-baseline-20261112（若名稱已存在，檢查其 owner，不得覆寫）。
3. 非破壞性整合 Docs 專屬提交，保留 RC21 安裝、回滾、P8、最新結案及本文件。若有衝突，只人工處理衝突的最小內容，不用 ours/theirs 抹除其他證據。
4. 在整合分支更新 roadmap、active backlog 的**最新有效狀態**和 B/C/D 分工。舊文件作歷史證據保留，不要把已結案 Gate 再標示成 NEXT 上線阻擋。
5. 僅做必要的 lineage、diff --check、文件引用和因實際程式差異而需要的局部測試。產生乾淨本機 Commit、最終 HEAD、供 B/C 交付後整合的目標。

**明確禁止**：不修改原 Source/Docs worktree HEAD、不操作 B/C/D 工作樹、不重新建置／部署 RC21、不執行 P8 或 Desktop 功能開發、不 Merge custom/main、不 Push/Tag/Release。

**完成條件**：一個無衝突、乾淨且可繼續開發的 Source＋Docs 統一本機基準；A 的整合 Commit 與接收 B/C 方法清楚。B/C 不需要等待 A 完成才能獨立開工。

## 2. 工作線 B — P8 Browser Broker 共用模式產品化

**目的**：已驗證一個 Chrome 能供三 Actor 共享，但一般使用仍要人工改 NEXT 專用 env、重新啟動 Core。B 要建立**明確、安全、易理解的啟用／停用工作流程**，不是重新實作 Shared Broker。

**執行範圍：**

1. 從 RC21 Source 建 B 專屬 branch/worktree；先讀 internal/app/acp_browser_runtime_mode.go、AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE、sticky restart guard、現有 Browser mode diagnostics/Browser UI，確認最小可重用的設定入口。
2. 實作 NEXT-only、預設 canonical workspace 的明確共享模式選擇、狀態及失敗理由。Shared **不得未經 opt-in 變成一般預設**。
3. 啟用／停用前必須尊重活躍 Lease、其他工作者／Session 與現有 restart guard；不得在尚有任務時直接重啟 Core、移除 guard marker、強制終止不屬於本次任務的 Chrome/MCP 或靜默 fallback。
4. 只做變更所需的 Go／Vue 定點回歸與可安全執行的最短使用流程。直接沿用 2026-10-14 真正三 Actor PASS，不建立新的合成多代理驗收矩陣。若受現有保護約束而無法安全完成單一功能，保留現狀並明確指出限縮版本，禁止冒險部署。

**寫入所有權**：B 主責 Browser Runtime 與 Browser 專用 UI。若要動共用 desktopapi、settings 或 i18n 檔案，先通知 C 協調單一寫入者，後續在 A 的整合階段處理。

**明確禁止**：不操作真實使用者 Edge／Chrome Profile、不建立獨立高權限常駐代理、不啟動多套 Chrome、不對現役 NEXT 或 Original 進行無批准的重啟／部署、不擴充 Release Gate。

**完成條件**：可實際使用的最小安全模式選擇功能／可信回退與錯誤呈現，或證據充分的不變更結論；獨立 Commit、局部驗證和向 A 提供的整合說明。

## 3. 工作線 C — P6／P7／H01 Desktop 實用性缺口

**目的**：Plugin、Connection、Settings、Diagnostics 等功能已有大量 Source 與部分實機證據。不要再以全部原生 GUI 測試作為結案主軸，**先找真實會讓使用者操作失敗的問題再修**。

**執行範圍：**

1. 從 RC21 Source 建立 C 專屬 branch/worktree，對 P6 Plugin 安裝／更新／啟用，P7 Connection／Settings／Diagnostics，H01 錯誤拒絕後切頁返回的錯誤保留，逐項比對現在實作與最新產品決策。
2. 只選可重現、實際影響完成操作的最小缺口；優先修錯誤被覆蓋、狀態失真、恢復／重試操作失效、無法完成必要的 Plugin/Connection 流程。每個小修正都要能說明對使用者直接改善。
3. 沿用 Shared Vue、Desktop API、現有權限與持久化模型；採局部 Go／Vue 測試及不干擾工作的必要觀測。沒有證實的 bug 就不要改，不為了報表好看而開更多 Milestone。
4. 產生 C 自有 Commit 與 A 可依序整合的文件。C 專屬 Plugin／Connection／Settings／Diagnostics；可能碰到 B 的共用 Desktop API／i18n 時先協調，不能跨線直接寫別人的 branch。

**明確禁止**：不擴張 P13/P14、NexusDock、Native MCP、全頁 native picker／AX／Focus 長鏈 UAT，不修改現役 Original、NEXT 或 Browser Profile。

**完成條件**：可證實的最小實際修復與測試，或明確證實不需要改動的缺口清單；回報 Commit、真實改善與未處理事項。

## 4. 工作線 D — GitHub 分支保存與發行治理

**目的**：本機 Git branch 多且遠端公開，避免盲目 Push 過時實驗、敏感資料或大量未審核歷史。D 是**唯讀 GitHub／Release 決策工作線**，不代表已取得 Push 或發版授權。

**執行範圍：**

1. 讀取真實 origin/upstream/workbench HEAD、全部 Local 分支、worktrees、dirty 狀態。2026-11-11 快照：52 local branches、origin 同名 8 同步／2 本機領先／42 缺少；custom/main 比 origin 多 312 commits。此為歷史快照，須重新核對，不能只看 local remote-tracking。
2. 區分已納入 RC21 Source 祖先的老分支、A 的 Docs 整合、B/C 新分支、未提交的 worktree 與 deferred P13/P14 實驗。**缺少遠端同名分支不代表一定要 Push**。
3. 審核公開 GitHub 的完整可達 Commit 歷史、token／內部設定／binary／機密、GitHub Actions、PR／Release 權限與 custom/main 同步風險；不只審最後一筆 SHA。
4. 分開規劃 Source 公開、整合 custom/main、Hosted CI、發版、Update Feed 和 signer/notary。當前 RC21 本機 ad-hoc Signed 不等於公開 Release-ready；無正式發行目標時不加入付費開發者帳號要求。
5. 產出明確分支分類表：值得保留作開發／包含於其他分支可停止維護／保有 dirty 需原 owner 處理／可等 A/B/C 整合後再 Push。D 若需寫紀錄，要用自己的分支或交接回報，不得改 A 的整合檔案。

**明確禁止**：不 Push、Force Push、Merge、Reset、Rebase、Tag、Release、清理其他 worktree 或修改現役服務。需要正式發布，先由使用者另行授權。

**完成條件**：具體分支分類及公開審核清單、整合／PR／GitHub 同步順序、Release blockers 與非 blockers；不能誤認 Source-only CI 等於 Hosted CI PASS。

## 5. 並行開發協調與合流

- **A 是唯一整合者**；B/C 可在 A 進行時從 RC21 Source HEAD 建獨立工作樹，但不得移動 A／Source 原 branch。D 隨時可唯讀查詢，直到 A/B/C 完工才下最終 GitHub 建議。
- 各線同時操作 Source 是安全的前提為**不同 branch/worktree**；任何共同環境檔、使用者桌面、NEXT Core／Browser 重啟／安裝都不可多 Session 同時執行，預設全部保持 Source-only。
- 共享 Desktop API/i18n、Roadmap/Backlog 只允許各線自己的分支修改，最終由 A 或後續指定整合者順序合併並解決衝突。禁止直接動另一線的工作樹或把別人的未提交修改納入自己 Commit。
- A 建立乾淨基準 → B/C 提交各自可用成果 → 指定整合者依序導入並做必要範圍驗證 → D 以最終 HEAD 給出要 Push 的最小分支建議；**不自動 Push 或啟用正式 Release**。
- 每條線完成時需提供：分支／工作樹／Commit、真實新增的產品價值或停止理由、最小測試證據、是否影響現役服務、後續明確待辦。不要創造新驗收 Gate 以延長既有工作。

## 6. 對照文件

- [RC21 結案與 P8 現況](next-rc21-product-closeout-and-p8-resume-2026-11-11.md)
- [RC21 安裝與簽章回滾](next-rc21-installed-host-2026-11-11.md)
- [先前分支／Milestone 盤點](next-stage1-branch-milestone-inventory-2026-11-11.md)
- [P8 真實三 Actor 及權威路由 §52](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)
- [P13／P14 延後決策](p13-p14-ux-scope-decision-2026-10-22.md)
- [產品優先與防止驗收漂移](next-product-first-guardrails.md)
- [Roadmap（部分歷史 OPEN 已由較新結案覆蓋）](roadmap.md)

**閱讀入口**：新 Session 先讀本文件第 0、5 節，再只執行被指派工作線的專屬章節；完成前不自行跨線擴充。
