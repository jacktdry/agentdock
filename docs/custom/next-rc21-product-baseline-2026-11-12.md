# AgentDock NEXT — RC21 Source＋Docs 統一本機開發基準（工作線 A）

> 2026-11-12，Asia/Taipei。只整合 **本機 Source／Docs Git 歷史與產品文件**，不是新建置、安裝、Push、`custom/main` 合併或正式發行。後續 B／C 完成前不宣稱其成果已整合。

## 解決的問題與最小做法

- **使用者痛點：**RC21 Source/H04/H02/R01 實作與最新 RC21 安裝、產品結案及 A～D 分工散在兩條分支，開發者容易依過時 Roadmap 重開已完成 Gate，或誤用一條缺少另一條證據的 HEAD。
- **直接改善：**獨立且可追蹤雙親系譜的整合 branch；Roadmap 與 Active Backlog 頁首統一現行決策，B／C 能交付同一目標，D 能判斷哪些 branch 不需逐條公開。
- **最小實作：**以 Source 指定 HEAD 新建 worktree，合併 Docs 的六筆專屬提交，僅人工調整 Backlog 同頁衝突並新增短交接頁；不做任何 Core/Vue 改動或測試 Gate 擴張。
- **驗證與不打擾：**Git parent/merge-base/ancestor、Source/Docs 差異、文件引用、`git diff --check`、worktree status；來源 HEAD 不動、不碰 8765/8767、User Browser/Profile、其他 Session 或其工作樹。
- **不做：**P8 開發、P6/P7/H01 修復、P13/P14、NexusDock、Native MCP Backend、RC21 重新 build/deploy/rollback、Hosted CI、Push/Tag/Release、Merge custom/main。

## 已核對的來源與結果

| 身分 | Branch | 核對的 HEAD | 紀錄 |
| --- | --- | --- | --- |
| Source | `fix/next-rc20-h04-h02-r01-20261111` | `0a0a0ba98412c6f22f8793a6abd184d6db62a27e` | 共同祖先後 1 筆；H04 race fixture、H02 Tray Source、NEXT-only unpublished workflow/封裝 |
| Docs | `fix/next-rc19-to-rc20-deploy-compat-20261111` | `1f0a390b0141f9b74cfac8ce8a2dbcd49859ccf8` | 共同祖先後 6 筆；RC21 本機 installed/rollback、候選、結案及 **最新四線交接** |
| 共同祖先 | — | `297b26b2086062742b141d5dbf4e100756e16bc3` | 兩來源盤點時 status 均乾淨 |
| A 整合目標 | `integration/next-rc21-product-baseline-20261112` | 此文件隨本地 merge commit 一併提交，`git rev-parse HEAD` 為最終準據 | worktree `/Users/wei/sideProject/agentdock-next-rc21-product-baseline-20261112` |

Docs 專屬清單包含 11/12 交接 commit `1f0a390b`，並非舊快照的「5 筆」。整合唯一同頁衝突是 `docs/custom/next-active-backlog-2026-10-21.md` 頁首：保留 Source 具體實作沿革、Docs RC21 installed/結案證據及本文件，將過時 AX/H04 OPEN 宣告移到歷史視角，不用 ours/theirs 抹除。其他舊 Markdown 仍為時間序列證據，不大規模改寫。

## 最新有效產品狀態（覆蓋歷史 OPEN）

| 工作／問題 | 現行定義 | 後續 owner |
| --- | --- | --- |
| H02 NEXT Tray 身分 | **DONE**；RC21 installed AXTitle=NEXT。舊 RC20「Tray 未命名」已過期 | 不重開驗收 |
| H04 ACP Race fixture | **DONE**；正常無並行高負載定點/ACP race PASS。大量 CGO/Swift 同時編譯時 1 秒 fixture timeout 是已知 CI 風險，不等於 runtime race 缺陷 | 若真影響 CI 才限縮修復 |
| RC21 本機 Candidate／Install | **DONE**；App/ZIP/DMG、26/26 installed SHA、ad-hoc codesign 與 RC20 rollback 備份、NEXT 8767/Original 8765 隔離已有證據；不是公開 Release | 本線不重建/部署 |
| P8 共享 Chrome 主流程 | **真實 PASS（2026-10-14）**；ChatGPT、Codex、Antigravity 同一 Broker/單一 Chrome，各自 Lease/Page 與 release；但 shared 要明確 NEXT-only opt-in、預設 exclusive | **B**：可安全使用的啟用/停用、真實狀態與保護中回退 |
| P6／P7／H01 Desktop | 現有功能與局部驗證在；只有能重現的 Plugin／Connection／Settings／Diagnostics 操作失敗、錯誤跨頁保留等才是修復目標；H01 真實跨頁 Host 未證實，不反向阻擋 RC21 | **C**：最小實用性修復或證據支持不修改 |
| GitHub 分支與正式 Release | `custom/main` 尚未整合本基準；unpublished candidate workflow 是 Source-only，不等於 Hosted CI PASS；Developer ID、Notary、Update Feed 未完成、`release_ready=false` | **D**：唯讀分類、敏感資訊/歷史與發行順序；有明確發行目標再授權 |
| 管理者授權提示 | 來源未知，不能當成 NEXT 證實缺陷，也不得要求 root 或繞過 OS consent | 若有真實證據再另立修復 |
| P13／P14、NexusDock、Native MCP Backend | deferred／deferred／無獨特用途 NO-GO | 不納入 A/B/C |

依據：[RC21 結案](next-rc21-product-closeout-and-p8-resume-2026-11-11.md)、[已安裝 Host](next-rc21-installed-host-2026-11-11.md)、[Source R01](next-rc20-stage4-crossplatform-release-readiness-2026-11-11.md)、[共享 Actor 權威 §52](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。

## B／C 交付後的最小合流流程

1. **工作樹所有權：**A 僅寫本 worktree 的 Roadmap/Backlog/整合紀錄；B、C 仍從相同 Source `0a0a0ba9` 分別開獨立 branch/worktree，不等待 A、不更改 A/Source/Docs 的 HEAD。D 完全唯讀。
2. **交接資料：**B、C 各自提供 exact branch、Commit SHA、變更檔清單、真實使用改善／不修改理由、局部測試結果與 blocker。共同 Desktop API/i18n 等寫入時各線先協調 ownership；不能以跨線檔案覆蓋結案。
3. **合併方式：**交付完成後由使用者指定整合者在 **A integration worktree** 逐條確認 commit ancestry、狀態乾淨；原則上 B → C 使用 `git merge --no-ff --no-commit <已核實的 B 或 C commit>`，人工解決實際重疊檔案且保留雙方產品行為，再跑僅限差異的驗證並分別 commit。若有新發展，依依賴重新排順序，不預先宣稱成功；不直接改 B/C worktree。也可評估必要時選擇性 cherry-pick，但須記錄 lineage。
4. **D 最後盤點：**以最新已整合 HEAD 做公開歷史/secret/CI/Release 決策；缺少同名遠端不代表需要 Push，並且發行簽章／notary/Feed 是不同授權邊界。
5. **明確停止點：**A 此階段只做到 Source＋Docs 的乾淨本機 merge commit；未收到 B/C 提交前停止，不開新 Gate，不擅自 Push 或合併 `custom/main`。

## 最小驗證與待辦

- 必驗：本整合 HEAD 的兩個來源 commit 都是祖先，merge parent 明確、沒有 unmerged path、git status clean、`git diff --check` PASS、上述文件相對連結存在。
- **此輪沒有新增程式差異**（程式仍是 Source `0a0a0ba9`）；既有 RC21 race／Swift／Vue／打包／Host PASS 僅引用歷史證據，不重跑或冒稱在此整合 worktree 重新驗證。
- 待辦：B 的 P8 shared 啟用 UX；C 的真實操作缺口；D 的唯讀 GitHub/Release 分類；其後由指定整合者進行逐條合流。現役服務不因 Git merge 變更。
