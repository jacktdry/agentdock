# AgentDock NEXT — GitHub CI 驗收後的分支／Worktree 收斂與磁碟清理

> 建立：2026-11-12。**此文件是下一階段執行門檻與操作清單；尚未同步 GitHub 原始碼、未進行本清理，更不是任何分支的刪除許可。** 此文件的執行點緊接 GitHub Push＋PR＋Hosted CI 與 origin `custom/main` 同步完成之後、NEXT 原生 Browser/GUI 實機驗收之前。

## 1. 要解決的使用者問題與停止條件

- 使用者是 GitHub Repository 唯一維護者，不必要求另一位審查者；主要痛點是本機 Git 分支、Worktree 與遠端歷史分支長期累積，難以辨識正式來源並占用磁碟。
- 目標是**可用分支少、磁碟 Worktree 少、歷史可追溯、現役 Stable/Next 不受影響**。不要為了 branch 數目好看刪掉仍在開發中的變更。
- 使用已驗證的 `custom/main` 作主要整合來源；過時分支能否刪除必須由實際 reachable-history、Worktree 狀態、所有者與保留需求判斷，不只憑命名/日期。
- 最簡單實作：**唯一一次新鮮盤點 → 列保留/安全清理候選/隔離待確認清單 → 先清理乾淨、已合併、無人占用項目 → 驗證 → 再按授權處理遠端與待確認項目**。不新建 daemon、清理服務、代理工作線或獨立審查制度。

## 2. 排程與前置 Gate

| 次序 | 任務 | 結案證據 |
|---|---|---|
| 1 | 同步 NEXT GitHub 候選分支；PR 與 Hosted CI 驗收；經授權更新 `origin/custom/main` | Exact SHA/PR/CI run 與遠端 HEAD；不能將 local tests 視為 GitHub CI |
| **1.5** | **清理本機＋遠端不需要的 Git branches / worktrees / tracking refs** | 可清／保留／待確認明細、刪除前後快照、磁碟空間變化、無損驗證；不能超越使用者核准範圍 |
| 2 | P8、P6/P7、H01 的真實 NEXT/Browser/GUI 使用者路徑驗收 | 真實操作 PASS／明確未測風險 |
| 3 | NEXT 專用隔離測試包／安裝／回滾 | 可驗證輸出、Stable 與現役 NEXT 未遭破壞 |
| 4 | 若真有公開分發需求，再討論正式 Release、簽章與 Update Feed | 另案授權 |

**清理開始門檻**：`origin/custom/main` 達核准目標 SHA、GitHub CI 實際通過（包含所選 base 的 PR/diff）、在刪除任何指標前保存來源清單／恢復方案；若 CI 不通過或尚未同步，就先做**只讀盤點，不刪除**。

## 3. 盤點範圍（每次清理前重新查詢，不依賴 11/12 舊數字）

- Git：本機 `refs/heads/*`、`refs/remotes/origin/*`、`git ls-remote --heads origin` 之即時實際遠端 branch、tag、`git branch -vv` upstream 與 ahead/behind、`git merge-base --is-ancestor` 對**經核准的遠端主線 HEAD**。
- Worktree：`git worktree list --porcelain`（含 locked / prunable）、每個實際路徑的 `git status --porcelain --untracked-files=all`、是否有未完成 Session／進程／owner、未追蹤與 ignored 但仍有價值的本機產物；實際刪除前再讀一次。
- 占用：對 Worktree 執行唯讀 `du -sh`，分開統計共享 Git objects、工作樹檔案、`node_modules`/build/cache/產物；注意同一 Git repository 的多個 worktree 共用 object DB，刪 branch ref **不必然釋放空間**。
- 保留資訊：`git log`、`git show-ref`、`git diff`、branch tip 完整 SHA、merge-base、對應 PR/CI/Release（如有）、是否由其他專案/Session 引用。
- 範圍包含 GitHub `origin` 的歷史 branch 與本地 branch；**不刪 upstream、workbench 第三方分支或 Original 服務自身資源**。

前次 D 稽核時原本 56 條本機分支、24 個 Worktree、4 個 dirty，另有 locked P10；A/B/C/D 後新增兩個獨立 worktree，2026-11-12 本輪唯讀估計 **58 branch／26 worktree**。舊統計只是參考；正式清理務必重新量測。[前次 56 branch 詳細分類](next-rc21-d-branch-inventory-2026-11-12.md)。

## 4. 分類與自動操作界線

| 類別 | 條件 | 預設處置 |
|---|---|---|
| **KEEP** | `custom/main`、GitHub 默認 `main`、正式 Release/rollback 唯一有效來源、目前使用中的 NEXT 與 Original、其他團隊仍依賴的工作線 | 不刪除；遠端/本機重要 branch 保護 |
| **CANDIDATE** | branch HEAD 已是核准 `origin/custom/main` 祖先；不是主線／tag 唯一來源；不被活躍工作線使用；相關 worktree 完全 clean；相依成果已合入/可追溯 | 提供具體 branch+SHA+原 Worktree+空間預估，經使用者核准後可先清 Worktree 再用 `git branch -d` 刪本機 branch |
| **REVIEW** | 尚未合併、比 `origin/custom/main` 領先、有 cherry-pick/非祖先情形、需求暫緩或來源 owner 不明 | 不刪除，先列差異與用途並請所有者決策；必要時建立可還原 snapshot/bundle 後才能結案 |
| **BLOCKED** | worktree dirty（包括 untracked／ignored 重要資料）、locked、Session 正在使用、Git ref 被 release/CI/回滾流程引用、Original/Stable 管理用途 | 絕不自動刪、不得強制用 `git branch -D`／`git worktree remove --force`／`git worktree prune` 掃掉 |

**具體已知保護**：`agentdock-m6-acp-tests`、`agentdock-next-edge-rc-20261023`、`agentdock-p5-mcp-oauth-fence`、`agentdock-original-maintenance-20261021` 曾有未提交內容；P10 worktree 曾有 `locked`。即使 HEAD 為主線祖先，也要**重新判定是否乾淨且 owner 同意**才能列候選。原始 D/A/B/C 分支因 commit 已完整保留在本機 `custom/main`，比較有機會列入 **CANDIDATE**，但依各 worktree 和 PR/CI 後的最新狀態覆核。

## 5. 清理實作順序與驗收

1. 產生 CSV/Markdown 收斂清單：每一列有 Branch（local/remote）、Tip SHA、Merged?、Worktree path、Dirty/locked、Maintainer、Reason、Decision、估算可省磁碟、Recovery reference；先出 **KEEP / CANDIDATE / REVIEW / BLOCKED** 的總數，不在盤點階段刪除。
2. 在核准的 CANDIDATE 中，先備份清單／必要的獨立歷史（如 bundle 或明確仍在的可信 ref），確保遠端有 verified HEAD；不為每個 obsolete branch 建永久 tag 而製造新垃圾。
3. 對**完全乾淨且非活躍**的分支依序 `git worktree remove <path>`，再使用 `git branch -d <branch>`；每筆重新驗證它是否依然為 main 祖先。只對明確不需要的 build/cache 另行核准刪除，避免磁碟節省只停留在 refs。
4. 對 GitHub 遠端歷史分支：先記錄可達關係與本機 SHA/PR/Release 引用，逐條核准後再 `git push origin --delete <branch>`；絕不刪 `main`、`custom/main` 或被保護/活躍的來源。**遠端刪除需要使用者明確同意**，不能從「檢查是否可清」直接推定可刪。
5. 遠端更新後，在已核准情況下 `git fetch --prune origin` 清理 stale remote-tracking refs；先計畫再執行 `git gc`／`git worktree prune`，不對 dirty/locked/其他仍存在的 Worktree 做無差別強制清理。
6. 結案時比對清理前後 branch/worktree 數、Git disk usage、各關鍵 SHA 與 remote HEAD；`git fsck`（適當範圍）、原始服務/Stable 連線狀況、`git status` 等無損檢查須通過；列出實際保留的例外和可恢復點。
7. 只對已核准、明確可清理的一組執行操作，遇異常即停；不能把全部 58 本機 branch、26 Worktree 一次當作垃圾刪除。

## 6. 單人維護與權限

- 使用者已明確確認 GitHub 僅一人維護。既有遠端 `custom/main` 設有禁止 Force Push／禁止刪除主線，沒有 required second reviewer 或尚未驗證的 required CI status。
- GitHub PR/CI PASS 是清理起跑點，不等於授權刪除**所有**遠端 branch；先提交清單供確認，再做實際有破壞性的刪除。
- Worktree 有共享 Git metadata，若大量刪除/後續垃圾回收，先確保磁碟操作不會中斷 Mac-Dev 或現役 NEXT/Original；禁止操作 Git `upstream`/第三方遠端分支和使用者無關資料。
