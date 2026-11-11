# AgentDock NEXT — RC21 A／B／C 統一基準與後續交接

> 2026-11-12（Asia/Taipei）。本線僅完成**本機 Source-only Git 整合**與局部驗證。沒有 Push、Merge `custom/main`、部署、重啟 NEXT／Original、Tag 或 Release；正式發行未授權。

## 解決的問題、最小處理與非干擾

RC21 Source＋Docs、P8 Browser 安全啟用與 Desktop Plugin／Settings 修正分別位於 A、B、C 三條分支。為免後續開發依過時狀態重做驗收，本次以 A 的已核對乾淨 HEAD 建立新的**獨立** worktree，順序執行 B、C `git merge --no-ff`，不動 A/B/C/Original 的工作樹或現役服務；不做功能擴充。

### Git lineage

| 身分 | SHA | 說明 |
|---|---|---|
| A 整合基底 | `0de2c5bc8afe6d99b4a3452907789db77ce83174` | RC21 Source＋Docs，含 H02／H04／RC21 installed 與 rollback 既有證據 |
| B 來源 | `27888fd9e9f888e5a76dca47056780a82197b582` | P8 NEXT-only CLI、安全模式設定、guard |
| B Merge | `04205ff8178964dd6ced5501b54a6f1326b29913` | A＋B，保留完整雙親歷史 |
| C 來源 | `03de2bd26e556d35ac52b169d3cc79c605279832` | Plugin／Settings Vue store 與回歸 |
| C Merge | `7aad04f137b34d208aa611fdaad3a532b7de7e17` | A＋B＋C，保留完整雙親歷史 |

- 新分支：`integration/next-rc21-consolidated-20261112`。
- 新 worktree：`/Users/wei/sideProject/agentdock-next-rc21-consolidated-20261112`。
- 最終文件提交 SHA 請以這個 worktree 的 `git rev-parse HEAD` 取得；文件不能在自身提交前宣稱自身 Commit。
- B/C 共同分叉點為 Source `0a0a0ba98412c6f22f8793a6abd184d6db62a27e`；A 另納入 Docs `1f0a390b0141f9b74cfac8ce8a2dbcd49859ccf8`。兩次 Merge 使用 `ort`，**零衝突、零人工內容調和**，不曾 cherry-pick／rebase／reset 原始分支。
- B 修改 `cmd/agentdock`、`internal/app/acp_browser_runtime_mode*`、`internal/desktopruntime/next_browser_mode*`；C 修改 `desktop/shared-poc/frontend/src/stores/{plugin,basicSettings}*`；未交叉覆蓋共用 Desktop API/i18n。

## 整合能力、證據與未完成項目

| 功能 | 此次合入的事實／明確限制 |
|---|---|
| H02／H04／RC21 Source | 沿用 A 已結案的 Tray 身分、ACP race fixture 修復及 RC21 本機安裝／簽章 rollback 歷史證據，**不是這次重新部署／Host UAT** |
| P8 Browser | **NEXT-only CLI** `agentdock browser-mode status/enable/disable`，安全 canonical workspace、private env、狀態／restart guard；shared 需顯式 opt-in，預設仍 exclusive |
| P8 套用限制 | **不是 GUI 一鍵切換**。設定改動只在**下次正常啟動**生效，不自動重啟 Core、移除 marker、關掉其他 Session/Lease。Source-only，**尚未部署到現役 NEXT** |
| P6 Plugin | 被動 refresh 不清除前次不確定／逾時的操作回饋與錯誤，stale fail-closed 不變；Vue source 測試不等於真實逾時 Host 復現 |
| P7 Settings | Save ACK 之後只有讀回有效設定才標記 completed，讀取回錯／例外正確分類及清空舊可修改狀態；未改現役設定檔 |
| P7 Connection／Diagnostics | C 未確認新的操作阻斷，沒有加改功能 |
| H01 native Host | 拒絕後跨頁的原生 Host 驗收**沒有在 C 完成**；現有來源層通過，不得宣稱已實機 PASS |
| P13／P14、NexusDock、Native MCP Backend | deferred／deferred／NO-GO，沒有開案 |

## 本次必要驗證

- 依新 worktree 真實 Git 檢查：A／B／C 原 SHA、Source 和 Docs HEAD 均為最終 Merge HEAD 祖先；B/C 無重疊原始檔，`git diff --check` 與 worktree clean 於提交後確認。
- Go：`go test -p 1 ./internal/desktopruntime ./internal/app ./cmd/agentdock -run '^(TestNextBrowserMode|TestRuntimeACPBrowserExclusiveRejectsSurvivingSharedGuard|TestRuntimeACPBrowserNextSharedWorkspaceOptIn|TestRuntimeACPBrowserNextSharedOptInDenials|TestRunPrintsVersion)' -count=1 -timeout=65s` 三套件 **PASS**。
- Go：`go vet -p 2 ./internal/desktopruntime ./internal/app ./cmd/agentdock` **PASS**；本次 7 個變更 Go 檔逐檔 `gofmt -l` **全部格式乾淨 PASS**（初次 shell 檔名展開方式錯誤，修正檢查命令後成功，無來源格式問題）。
- Vue：`pnpm exec vitest run src/stores/plugin.test.ts src/stores/basicSettings.test.ts` **2 files／9 tests PASS**；`pnpm typecheck` **PASS**。使用 `pnpm install --offline --frozen-lockfile --ignore-scripts` 本線獨立依賴目錄。
- 不重新跑先前三 Actor Browser、Tray、完整 native GUI／Host、壓力或 Hosted CI；相應既有歷史 PASS 不應當成本次產物的實機結果。

## D 的 GitHub／Release 治理與建議順序

- 已讀 [共同四線交接](next-four-workstreams-handoff-2026-11-12.md) 的 D 規格並唯讀查詢本機分支、近期 Docs commits、GitHub `origin` heads；**沒有找到 D 的正式結案文件或獨立治理提交**，不能代其宣稱公開歷史、Secrets、Actions、權限稽核已 PASS。
- 核對時遠端 `origin/custom/main` 為 `4a6c24afb24bfe4893935eca50cdc046a1b2add7`；沒有 A/B/C/新整合分支的同名 remote head。本機 `custom/main`=`7e87605a91f3a1e48a5879f7e406c3803a06e00c`，比本機 `origin/custom/main` tracking 多 **312 commits**。以上是快照，Push 前需重新核對。
- **建議但未授權**：先由 D／指定審查者完成可達 Commit 全歷史、token／內部設定／binary、GitHub Actions、Remote/PR 權限審核，且重新確認分支差異；取得使用者明確 Push 授權後，優先只把本**統一整合分支**同步到 GitHub 供 review，不必逐條 Push A/B/C。D 未結案前不宣稱已審核安全。
- `custom/main` 的本機超前歷史與合併衝突需獨立審查並另行授權；不能直接 fast-forward 或無評估合併。Hosted CI、Developer ID、notarization、Update Feed、正式 Release 依實際發行需求和個別授權後再處理。RC21 本地 ad-hoc 簽章及 Source-only unpublished workflow **不等於 Hosted CI 或正式 Release-ready**；維持 `release_ready=false`。

## 停止點

整合基準可作後續 Source 開發唯一入口；若無真實產品阻斷，不因驗收清單再增新 Gate。下個獨立決策只有**補齊 D 的治理證據／公開同步授權**及未來明確要求的部署／發行，這些均不是此次已完成的事。
