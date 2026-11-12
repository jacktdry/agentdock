# AgentDock NEXT — D 工作線：GitHub 分支保存與發行治理結案

> 日期：2026-11-12（Asia/Taipei）。**D 僅審查與本機文件提交；未 Push、Merge、Tag、Release、改 GitHub 設定、部署或重啟現役 NEXT/Original**。D 專屬分支與 worktree 由 A＋B＋C 統一基準 `d76f7f218875963cc067c3213ec8b2a784cec956` 分出，僅寫此分支的文件。

## 0. 為何要做與最小停點

- **使用者問題：**公開 `origin` 分支少於本機；`custom/main` 長期落後本機產品來源。盲目逐條 Push 或 Release 可能曝露歷史機密／未審核來源，並混淆 Stable/Next 發行身分。
- **直接改善：**一份可執行的 56 條分支分類、GitHub 真實 head／權限證據、完整整合來源可達歷史敏感內容掃描、分層 Push／PR／CI／Release 治理建議。
- **最小方法：**Mac-Dev 只用唯讀 `git`、GitHub API、歷史 Blob 離線掃描，最後新增 D 專屬文件 Commit；不清理原工作樹／不修改其他 Session／不啟動服務。
- **可觀測驗收：**各 HEAD／祖先、工作樹 dirty、敏感掃描範圍與結果、CI workflow guard、GitHub Branch Protection/Actions/PR/Release 均有記錄；D 文件連結有效、`git diff --check` 與 D worktree status clean。
- **不做：**Codex ACP、修改程式／重設或 rebase 任何來源、清除 dirty、修改 Original、push / tag / release / update feed、設定 repo 保護規則、要求管理者或 Apple 簽章權限。

## 1. 本機與真實遠端快照

| 檢查項目 | 真實結果／含義 |
|---|---|
| origin | `https://github.com/jacktdry/agentdock.git`，GitHub API：**public**、預設分支 `main`；目前登入身分具 repo `admin`/`push` 權限，但**這不是本輪寫入授權** |
| origin `main` | Live `2ffed4ee8f0392f77bb6dd4a5de268de31ffa9d3` |
| origin `custom/main` | Live `4a6c24afb24bfe4893935eca50cdc046a1b2add7` |
| upstream `main` | Live `43b80c72fbd6b3952e7b46b311aa4eeaededdbde`，**與本機 `upstream/main` tracking 的 `2ffed4ee` 不同**；本輪不 fetch／更新 tracking |
| workbench `main` | Live `82518b9133189823e6adcb51c5d751de17146fe3` |
| Local `custom/main` | `7e87605a91f3a1e48a5879f7e406c3803a06e00c`，相對 live/tracking origin 同分支 **0 behind / 312 ahead** |
| A／B／C 統一基準 | `integration/next-rc21-consolidated-20261112` = `d76f7f218875963cc067c3213ec8b2a784cec956`，相對 `origin/custom/main` **0 behind / 386 ahead**，不是已同步 |
| Local `main` | `dd79d37e5c4540dd6ff206031eb7bf6e6ef62855`；相對 `origin/main` **54 ahead**，不代表已提交上游 |
| 本機分支（D 建立前） | **56**；同名 GitHub origin **10**（8 equal、`main` 與 `custom/main` 分別領先 54／312）；**46 沒有同名遠端分支** |
| Worktrees（D 建立前） | **24**；其中 **4 個 dirty**、1 個 P10 worktree 設有 `locked initializing`；D 建立後總量為 25、分支 57（D 專屬 +1） |
| Git lineage | 56 條中 **23 條 commit 已是統一基準祖先**；其餘 33 條不可無證據視為已被合併／可刪除 |

完整逐分支分類與工作樹所有權請看 [D 分支分類表](next-rc21-d-branch-inventory-2026-11-12.md)。**遠端缺少同名分支不等於需要 Push。**

### 必須保護的 dirty worktrees

| Worktree | 變更數量 | 處理 |
|---|---:|---|
| `agentdock-m6-acp-tests` | 17 | 屬原 Session 未提交程式與新檔；保留，禁止 cleanup／switch／rebase／代為 commit |
| `agentdock-next-edge-rc-20261023` | 12 | 屬原 Session 未提交程式與文件；雖 HEAD 是整合祖先也**不能**清除 |
| `agentdock-p5-mcp-oauth-fence` | 5 | 保留未提交 OAuth 程式修改，不能按祖先關係視為可以刪除 |
| `agentdock-original-maintenance-20261021` | 1 | Original 獨立維護 worktree，不與 NEXT 混合，不清理 |

## 2. 公開歷史風險審核（範圍、命中與限制）

- **審核對象：**完整 `integration/next-rc21-consolidated-20261112` 可達歷史（不只最新 commit/diff），共 **1,202 commits／17,623 Git objects**；其中 **5,543 Git objects** 不屬於 `origin/custom/main` 祖先圖可達物件集合（不等於在 origin 所有分支都未存在）。
- **內容掃描：**離線逐一透過 `git cat-file --batch` 讀取 **9,299 個不超過 2 MB 的 Git blobs（總計約 129,128,593 bytes）**，對可辨識為文字的內容進行模式比對，對含 NUL 的疑似二進位內容跳過文字正規式；沒有任何超過 2 MB 而未讀取的 blob。規則包含 GitHub/GitLab PAT、AWS 金鑰、Google API Key、Slack Token、OpenAI API Key、PEM 私鑰、帶密碼的資料庫 URI。
- **高信心特徵：0 命中**。本次掃描沒有輸出任何憑證值；GitHub Secret Scanning API 的 alerts 為 `0 open / 0 total`（只反映可查到的警報，不能證明歷史安全）。
- **一般啟發式匹配：566 筆**（較 `origin/custom/main` 增加的可達物件中 37 筆），因變數名、程式欄位、測試假資料也會命中。37 筆集中在 7 個程式／測試／Installer 路徑：`InstallerRunner.swift`、`connection_auth.go`、`oauth_flow_test.go`、`manager_test.go`、`oauthclient/manager.go`、`oauthclient/store.go`、`install.ps1`；已做不洩漏內容的型態與熵值檢查，**並未逐筆以人工判讀真實值**。不能宣稱獨立人工 secret review PASS。
- **敏感路徑檢查：**歷史中命中 `.env.example`、`desktop/shared-poc/frontend/.npmrc`、`skill-sources/telegram-official/.env.example` 3 種路徑，皆為 `origin/custom/main` 已可達，不是新增加；未見新的 `.pem`/`.p12` 私鑰或打包大檔路徑。這不替代對公開環境設定與第三方依賴的最終審查。
- **結論：**沒有自動掃描判定的高信心外洩，但仍須在真正 Push 前完成對新增歷史的一次有界人工/獨立 secret-scan review（尤其 37 筆泛用匹配與外部配置）。**本輪不宣稱 Public-safe PASS，也不因本機掃描通過自行 Push**。

## 3. GitHub CI、權限與發行路徑

| Gate | 現況與明確判斷 |
|---|---|
| GitHub Branch Protection | API 查 `main`、`custom/main` 都回 **Branch not protected**；Repository rulesets `[]`。**不應把未設保護的 custom/main 直接當正式發行唯一來源** |
| GitHub Actions | API `enabled=true`；API 目前回 `workflow_runs=[]`，不能把 Source-only 工作流程冒稱 Hosted CI PASS |
| Pull Requests | API 查到 `0` 筆 PR；無 NEXT 統一分支 PR 驗證證據 |
| GitHub Releases / Tags | Releases `[]`，本輪 `refs/tags/v*`／`next-v*` 未查到；無正式公開 NEXT 發行 |
| `next-release-preflight.yml` | 本機程式有 `pull_request.paths` 選擇性觸發和 source-only smoke／candidate Core，尚無 Hosted run；不是完整安裝版簽章 Host |
| `next-unpublished-candidate.yml` | **NEXT-only `workflow_dispatch`**，受 SHA／版本輸入制約且 `contents: read`；在 **public repo 不上傳候選 Artifact**，沒有 Release／Tag／Feed／安裝，目的為未發布候選 |
| `release.yml` | **Stable `v*` 與 workflow_dispatch**，具有 `contents: write`；驗證 source commit 必須為 `origin/main` 祖先，且 `verify-version` 依 Stable 合約；不能直接拿來發 NEXT 或 `next-v*` |
| `windows-installer.yml` | 具 `contents: write`，手動 dispatch 有可發布測試套件路徑；公開發布需另做輸入、權限、tag 審核，不在 NEXT RC21 本機 gate |
| 正式 macOS Distribution | RC21 先前本機 **ad-hoc** signed、rollback 證據有效，但不是 Developer ID／notarization／正式 Update Feed 的證據。`release_ready=false`，Feed disabled/unpublished |

## 4. 分支保留與最小 GitHub 同步順序（**建議，沒有操作授權**）

1. **保護現有來源**：D 報告獨立提交；A/B/C 原分支、dirty／locked worktrees、Original 與 `custom/main` 均維持原樣。已是統一基準祖先的歷史分支可停止獨立維護，但不自動刪除。剩餘 33 條不可當作已合併；先依 [分類表](next-rc21-d-branch-inventory-2026-11-12.md) 保留或由原 owner 復核。
2. **公開前最後有界審查**：指定負責人核對 386 筆超前 commit 的公開性與 37 筆泛用 credential 候選、重跑可信 secret scanner（例如預先核准工具）及擬公開 diff／檔案；明確判定「可以公開」。若查到真實憑證應先作 rotate/revoke 與歷史處理，絕不可只刪除 HEAD 檔案後 Push。
3. **先明確取得 Push 授權，再最小同步**：只推一條整合後分支供 Review，不把 46 條無同名遠端的分支逐條公開。可選 `integration/next-rc21-consolidated-20261112`；若治理文件也要納入同一遠端候選，應先由指定整合者非破壞性將 **D 文件 Commit** 接回統一分支（而不是自行改 A/B/C），並重新檢查最終 SHA 和推送範圍。
4. **PR / Hosted CI**：將候選 branch 指向指定 Base 做 PR 前先評估 base 與差異，驗證來源與審核 CI 結果、擬保護規則與 required review；Branch Protection/Ruleset 建議先以 repo owner 設定並另行授權。不得將沒有 Hosted Run 的工作流程判為通過。
5. **`custom/main` 另行決策**：其本機 HEAD 對 origin +312，而統一基準對 origin +386，且 origin 是公開 repo；先確認 312 歷史的公開意圖、分支保護、PR base/merge 策略與使用者批准，再考慮從候選合併到 `custom/main`。這次**沒有** Merge。
6. **正式 Release 更晚且獨立授權**：只有具體外部分發需求時才加入 NEXT 專用 protected source/tag、Hosted CI、Apple Developer ID、Notary、精確簽後 hash、rollback 與 Feed gate；需 NEXT-only 發行流程，**不能觸發 Stable `release.yml`**。本次未請求 Apple 憑證／root、未簽署或發布。

## 5. D 的停止與交接

- D 本地可完成的是**治理調查與文件結案**，不是「允許公開／正式發行」。本文件的 Commit 是 D worktree 的唯一新增內容之一，與來源 A/B/C 沒有交叉寫入。
- 開發工作統一基準仍是 `integration/next-rc21-consolidated-20261112`；D 工作樹 `/Users/wei/sideProject/agentdock-next-rc21-governance-d-20261112`，分支 `docs/next-rc21-github-governance-d-20261112`，僅為保存治理報告，不代表此 D 分支已在 GitHub。
- Source 階段下一步不需再開廣泛 GUI/Browser 驗收；**只需要在真正公開前完成明確的安全放行、Push 授權及遠端 PR/CI 實證**，正式發行另案。
