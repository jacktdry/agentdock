# AgentDock NEXT — RC21 custom/main 基底整合暨 D 37 筆候選複核結案

> 2026-11-12（Asia/Taipei）。**本機獨立整合完成、未修改原始 `custom/main` 指標及工作樹、未 Push、未改 GitHub 規則、未 Tag/Release、未部署或重啟 NEXT/Original。** 既有 A/B/C/H02/H04 的 Source 與歷史 Host 證據仍分層，不宣稱此次重新跑過真實 Browser/GUI Host。

## 產品目標、最小操作與非干擾

使用者要結束 A/B/C/D 分工後的「基準不統一」問題，並確定 37 筆憑證誤判與 GitHub 單人維護規則不再阻擋**本機整合**。

- 實際做法：先逐筆覆核新增可達 blob 的 37 筆泛用 credential pattern 命中，核對 GitHub 一人維護及 branch protection 現況；另外建立乾淨 worktree 從原 `custom/main` `7e87605a91f3a1e48a5879f7e406c3803a06e00c` 起步，使用 **non-fast-forward merge** 將 D（已是 A+B+C 的直系後代）合入，保留全部 commit 和 merge-parent 歷史。
- 禁止影響現役：不切換／reset／清理現有 A/B/C/D/Original/`custom/main` 的 worktree；不改 Mac/瀏覽器帳號、權限、原服務；無任何遠端 GitHub mutation。
- 驗收：五個來源 ancestor、tree 等同 D、合併零衝突、Scoped Go/Vue tests 與 vet/typecheck/gofmt、文件與乾淨 Git 狀態。

## Git 實際結果

| Item | Git identity | 說明 |
|---|---|---|
| 不變的原本 `custom/main` | `7e87605a91f3a1e48a5879f7e406c3803a06e00c` | 仍在既有 `agentdock-custom` worktree，沒有本輪修改 |
| A | `0de2c5bc8afe6d99b4a3452907789db77ce83174` | RC21 Source＋Docs |
| B | `27888fd9e9f888e5a76dca47056780a82197b582` | NEXT-only P8 CLI |
| C | `03de2bd26e556d35ac52b169d3cc79c605279832` | Plugin/Settings stores |
| ABC | `d76f7f218875963cc067c3213ec8b2a784cec956` | 先前統一 HEAD |
| D | `6ce9113cf1327d0cda8633f48f0bdbd624dc3a28` | D GitHub/Release 治理（ABC 後一筆） |
| `custom/main` 基底＋D merge | `e2def3b51945a3a206e8c2c61f7a6296b045adf7` | 本輪 merge，雙親是原 `custom/main` 和 D，**0 衝突** |

- 目前唯一新的整合分支：`integration/next-rc21-custom-main-reviewed-20261112`
- 獨立 worktree：`/Users/wei/sideProject/agentdock-next-rc21-custom-main-reviewed-20261112`
- 最終文件 commit SHA 由本工作樹 `git rev-parse HEAD` 讀取；此文件的本體 SHA 不先填入。與 D 合流後的 tree diff **0**（寫入本輪文件前）；新增的只有本輪後續審核紀錄與 Roadmap/Backlog 更新。
- 原 `custom/main` 是 ABC 整合 HEAD 的祖先，ABC 比它多 **74 commits**，D 再多 **1 commit**。此處的整合已實際進行，但**並不等於改動原 `custom/main` 分支**。

## D 安全覆核已完成的範圍

- D 原掃描：統一來源完整可達歷史 **1,202 commits／17,623 objects／9,299 blobs（129 MB）**，無 GitHub/GitLab PAT／AWS／OpenAI／PEM 等已知高信心樣式命中，但這不是所有類型憑證的保證。
- 本輪：[37 筆泛用候選一一覆核](next-rc21-d-secret-37-review-2026-11-12.md)（可追溯 Git blob SHA:line、原因）。**37/37 完成、37/37 可解釋、0 筆有發現硬編碼實際正式憑證。** 其中 OAuth runtime 欄位 25、Desktop ReadPassword 6、macOS Installer 2、PowerShell 2、測試 data 2。
- 有界來源審核結論可以解除這 37 筆特定候選的阻斷；它**不是**保證全歷史未含任何類型的秘密。不得在未核對最終推送 SHA、remote 最新 HEAD、授權下推送公有 Repo。
- 來源 D 初始稽核細節及 56 分支分類： [D 治理總結](next-rc21-d-github-governance-closeout-2026-11-12.md)、[分支分類表](next-rc21-d-branch-inventory-2026-11-12.md)。

## 單人 GitHub 維護政策

- GitHub `jacktdry/agentdock` 確認為 **public**，目前權限可見的 Collaborator 只有 **1 名維護者**。用戶明確指示**不要求第二人審查**；因此**不得設定 required approvals >= 1**，也不設定沒有實際成功執行紀錄的 Required CI check。
- `main`、`custom/main` 在本輪 GitHub API 都 `protected=false`、repository rulesets 空。**遠端目前尚未實際設定保護規則**，此處完成的是治理方案與核對，不冒稱遠端設定已生效。
- 後續由擁有者授權後可設定 `custom/main`：禁止 force pushes／branch deletion，**不要求其他人批准**；PR 可用於自我檢視差異但允許單人合併；需要強制 Status Checks 時，要先有真實 Actions Runs 並確認精確 check 名稱。既有 CI 與 Unpublished Candidate 不能直接視為 Hosted PASS。
- 本機目前適合先用單一整合分支繼續開發，不將 56 條本地 branch 逐條 Push；真正公開時另行審核 remote changes / CI，正式 NEXT Release 不套用 Stable `v*` 發行工作流。

## Source / Host / Release 的誠實狀態

| 能力 | 本機整合結果 | 尚未提供 |
|---|---|---|
| P8 Browser | B NEXT-only CLI Mode 的原始碼與 focused Go checks 保留；預設 exclusive，opt-in 待下一次正常啟動生效 | GUI 一鍵切換／部署到現役 NEXT |
| Plugin／Settings | C Source 完整合流，Vue Store 回饋與讀回修正 | H01 拒絕後 native 跨頁 Host PASS |
| H02／H04 | A 中已結案證據不被覆蓋 | 本輪沒有重測完整真實 Tray/ACP Host |
| GitHub | 單人權限、現有 workflow 與 Branch Protection 現況已盤點；37 候選有界覆核 | 沒有改 GitHub 設定、沒有 Push/PR 或 Hosted CI PASS |
| Release | `release_ready=false`、NEXT Feed off，Stable 發行路徑獨立 | 未授權正式發行／Developer ID/Notary/Feed |

## 本輪定點驗證

- Go：`go test -p 1 ./internal/desktopruntime ./internal/app ./cmd/agentdock -run '^(TestNextBrowserMode|TestRuntimeACPBrowserExclusiveRejectsSurvivingSharedGuard|TestRuntimeACPBrowserNextSharedWorkspaceOptIn|TestRuntimeACPBrowserNextSharedOptInDenials|TestRunPrintsVersion)' -count=1 -timeout=75s`，**3 packages PASS**。
- Go：`go vet -p 2 ./internal/desktopruntime ./internal/app ./cmd/agentdock` **PASS**；B 涉及的 7 檔 `gofmt -l` **PASS**。
- Vue：`pnpm exec vitest run src/stores/plugin.test.ts src/stores/basicSettings.test.ts` **9/9 PASS**；`pnpm typecheck` **PASS**。
- 新 branch git ancestry、`git diff --check`、交接文件連結與最後 `status --porcelain` 乾淨均需在 Commit 後最後核對。**未執行** Full Browser/Tray/native GUI/整套壓測/Hosted CI。

## 下一個最小交接點

D 的 37 筆候選與單人治理模式已收斂，A/B/C/D 已共存於獨立本機整合分支，**不新增第二人審核或新的驗收矩陣**。原 `custom/main` 還是 `7e87605a`；若日後要把這個新的整合 HEAD 實際移到原 `custom/main`、Push 到公開 GitHub、修改 Branch Protection 或發行，須依使用者範圍在不干擾其他 worktree 的時機單獨執行。
