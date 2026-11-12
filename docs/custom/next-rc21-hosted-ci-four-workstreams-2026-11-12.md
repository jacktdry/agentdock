# AgentDock NEXT RC21 — Hosted CI 四工作線修復規格與交接（2026-11-12）

> **狀態：規劃／待分派；尚未開始四線修復。** 四個 Session 只獲准在各自 Git Worktree 進行本機實作、測試、提交及自己的 HANDOFF。**禁止 Push、Merge、修改 CI／遠端、安裝／重啟 NEXT、動 Original／Stable、正式發行或刪除 Worktree。** 一律 ChatGPT 聊天模式＋Mac-Dev MCP，不用 Codex ACP。

## 1. 目的、基準、驗收等級

使用者痛點：NEXT RC21 A/B/C/D 已本機整合並建立 GitHub PR，但 **Hosted full CI** 的三個 OS Go Tests 失敗，阻擋整合與其後的 branch/worktree 清理。不是缺本機 Linux/Windows：GitHub Hosted `ubuntu-latest`、`windows-latest` 和 `macos-latest` Runner **都已真實執行**。

| 項目 | 已核對值（啟動各線時重新核對） |
|---|---|
| 共用來源 Repo | `/Users/wei/sideProject/agentdock-custom` |
| **四線固定起點 Commit** | `b7a26cd9f5fded276f6459622bb6be4916c5caca` |
| PR | [#1](https://github.com/jacktdry/agentdock/pull/1)，base `custom/main`，head `integration/next-rc21-github-ci-20261112`，目前 OPEN |
| NEXT source-only CI | [Run 43143421426](https://github.com/jacktdry/agentdock/actions/runs/43143421426)：preflight、macOS ARM64 Core PASS |
| Hosted full CI | [Run 43143453728](https://github.com/jacktdry/agentdock/actions/runs/43143453728)：Browser Integration、Container PASS；Ubuntu、macOS、Windows Go Test FAIL |
| 原遠端 `origin/custom/main` | `4a6c24afb24bfe4893935eca50cdc046a1b2add7`；尚未合併 |
| 必須一致的 Go 版本 | `go.mod` 使用 **Go 1.26.5**，不要用 Mac 本機 Go1.27.3 的格式結果代表 CI |

直接成果：修復真實跨平台相容性問題、讓測試依合理平台支援範圍運作，且**不能用假證據取得綠燈**。最簡實作是修改既有測試或小範圍平台 helper，不新建 VM、長駐 Agent、測試框架，亦不更動安全邊界。

## 2. 工作線及檔案所有權（可同時進行）

| 線 | 主要責任 | 專屬寫入範圍 | 絕對禁止跨入 |
|---|---|---|---|
| **A Linux ACP** | 3 個 ACP MCP Diagnostic Scope tests 與非 Darwin Process Identity 支援契約 | `internal/acp/mcp_owner_diagnostic_scope*`，必要時僅評估 `internal/process/identity_other.go` | Permission Store、Browser Fixture、Darwin 測試、CI YAML |
| **B macOS** | Codex CLI test prerequisites、Unix Socket 臨時路徑、PathSet、Browser Guard | `internal/desktopruntime/*_darwin_test.go`、`internal/mcp/launchregistry/*_darwin_test.go`、`internal/controlplane/pathset*`（只有證據足夠才動共享程式） | ACP Diagnostic、Permission Store、Windows Fixture、CI YAML |
| **C Windows Core** | Windows Permission Store 目錄 Sync／durability、回滾與權限保存 | `internal/permission/**`，新增必要平台安全 helper 也限同目錄 | Browser Fixture、Windows GUI/Release Fixture、A/B、CI YAML |
| **D Windows Portability** | Windows Browser Profile/Scope、測試執行檔 `.exe`、AppUserModelID 來源測試及 panic | `internal/tool/browser/*test.go`、`internal/browserpolicy/*test.go`、`tools/release/*test.go`、`scripts/test/desktop_windows_test.go` 等已辨識的 Windows 測試資源 | **不可寫 `internal/permission/**`（C 獨占）**、A/B、CI YAML |

每線只新增／更新自己的交接檔，不一起編輯 Roadmap、Backlog 或本文。發現必須改別線檔案，先寫 HANDOFF 記錄重現、原因及建議 patch，等整合者協調；**不能直接修改別人所有權**。

## 3. A — Linux ACP Diagnostic

**實際失敗**：`TestMCPDiagnosticScopeOnlyLocalLoadedSnapshot`、`TestMCPDiagnosticScopeFencesConcurrentManagerMutation`、`TestNextEarlyMCPDiagnosticRequiresLiveAdapterButNotLoadedSession`，均無法取得 `withMCPDiagnosticScope` 的資格。`internal/process/identity_other.go` 在 `!darwin` 直接回傳 `ErrIdentityUnavailable`，這不是缺 Linux Runner。

工作：
1. 檢視 `internal/acp/mcp_owner_diagnostic_scope.go` 和 Process Birth Identity 支援界線，判斷 positive tests 應 Darwin-only、以平台拆測試，或存在可驗證的 Linux 需求。**不能**用假 PID、假 StartTime、假認證證據換 PASS。
2. 對不支援平台仍保留負向 fail-closed 測試；對 Darwin 支援平台維持正負向覆蓋。不臆測 Windows/Linux Process Identity 有正式實作。
3. `GOTOOLCHAIN=go1.26.5` 本機 focused Go 和相關 vet；Linux 真實驗收只能由整合者之後在 Hosted Ubuntu 重跑。
4. HANDOFF 說明已測/未測 OS、精確 SHA、原失敗重現、修後指令與結果。

**完成門檻**：Mac focused tests PASS、平台不支援時安全拒絕的回歸成立；**不能**自稱 Linux Hosted CI PASS。

## 4. B — macOS Test Prerequisites

**實際失敗**：
- GitHub macOS Runner 的 Codex CLI 不在 PATH，多個 `runtime_next_codex_..._darwin_test.go` 在前置準備直接失敗。
- `internal/mcp/launchregistry` 的 `t.TempDir()` 過長，Unix socket `listen unix ... bind: invalid argument`。
- 另有 `internal/controlplane/pathset_test.go` 兩項及 `internal/desktopruntime/next_browser_mode_darwin_test.go` 一項，**不能尚未分析就歸咎 Runner**。

工作：
1. 分辨純邏輯測試與必須真實 Codex 的整合測試。純邏輯採隔離、受控的 mock CLI 或嚴謹 prerequisite；**不得**冒充真人 ACP/權限/實機驗收，也不能讓無 CLI 安全拒絕變成成功。
2. 僅測試使用短路徑 Unix Socket，資源由測試清理。不能修改現役服務路徑、HOME、Mac-Dev 原 Stable、Keychain 或原始瀏覽器 Profile。
3. PathSet、Browser Guard 分別定點重現，確定測試假設 vs 真實功能回歸；必要時最小修正並標註平台。
4. Mac 本機 Go 1.26.5 對應 focused tests、無搶焦/無新 Agent 啟動證據；Hosted Mac CI 待整合者驗證。

**完成門檻**：Codex 不存在時單元測試前置合理、socket 不超限，PathSet/Guard 歸因有證據；不以假 CLI 當真實 Host PASS。

## 5. C — Windows Permission Durability（優先度最高）

**實際失敗**：Windows Hosted Job 日誌約 **197 次相同錯誤訊息**，不是 197 種 Bug：`fsync permissions directory ... Access is denied`；`internal/permission/store.go` 對目錄呼叫 `dir.Sync()`，Windows 不支援同等 Unix 操作。

工作：
1. 先確認 Windows 的 durable file flush、atomic replace、目錄同步及失敗後語意；評估最小平台化 helper／build tag。不能把所有 `Sync` 錯誤直接吞掉或降低 Permission 安全持久化承諾。
2. 正／負向涵蓋保存、重載、失敗注入、權限私有狀態、rollback/fail-closed；如果 Windows 平台不能滿足某持久化保證，明確列入風險與阻擋，不得稱已驗證耐斷電。
3. Mac 做 compile/targeted Go 測試與 cross-build 可行證據；**原生 Windows Go tests 必須在 Hosted Runner 真跑才算 PASS**。
4. 不修改 D 的 Browser Profile/Scope Fixture；交接需指出哪些 Windows 連鎖錯誤可能隨本修復消失。

**完成門檻**：本機方案含可驗證負向與相容性證據；整合後 Hosted Windows Permission Store PASS 才解除此 Gate。

## 6. D — Windows Portability（可先修，完整驗收依賴 C）

**實際失敗**：
- 約 20 次 Profile `user_data_dir must be a canonical absolute non-root path`，含 POSIX-only 路徑 Fixture；
- 約 44 次 `BROWSER_SCOPE_REQUIRED`，**需分辨** Profile 失敗的連鎖 vs 真實缺資格，不能直接放寬安全條件；
- `tools/release/next_candidate_test.go` 在 Windows 用不帶 `.exe` 的測試 binary path；
- `scripts/test/desktop_windows_test.go` 期待 `App.xaml.cs` 內含 AppUserModelID 常數，但目前 identity 定義位於 `Models/WindowsProductIdentity.cs`；
- `internal/tool/browser` 尚有 `index out of range [-1]` panic，需獨立定位。

工作：
1. 先做不依賴 C 的靜態合法路徑 Fixture、測試 `.exe`、AppID 真正來源核對；不要在 production code 複製常數只為讓 string-search PASS。
2. Browser Scope 拒絕分群，維持 Workspace Scope、Owner、Profile 和 External Transport Fence **fail-closed**；不能刪或跳過既有安全測試。
3. 解析 panic 是否前置拒絕後的錯誤 test fixture，修到不會越界；若涉及別線來源，改成 HANDOFF 協調。
4. **等 C 修好之後**由整合者重新執行 Hosted Windows full CI 才能判定 D 結案。Mac cross-compile 不代表原生 Windows 測試成功。

**完成門檻**：已能完成的定點測試／靜態檢查有輸出；剩餘 Windows Native 測試列未驗證並說明 C 依賴。

## 7. 四個 Session 的 Worktree、分支與交接檔

每個 Session **開工時**用 Mac-Dev 讀本文件和 AGENTS，確認 `git status`、`git worktree list`、分支與精確固定 SHA。若 branch/path 已存在，先檢查再沿用；**禁止** `-B`、`-f`、`-D`、強制 Worktree remove。只在不存在時用 `git worktree add -b <branch> <path> b7a26cd9f5fded276f6459622bb6be4916c5caca`。

| 線 | Branch | Worktree | HANDOFF |
|---|---|---|---|
| A | `fix/next-rc21-ci-a-linux-acp-20261112` | `/Users/wei/sideProject/agentdock-next-rc21-ci-a-linux-20261112` | `docs/custom/next-rc21-ci-a-handoff-2026-11-12.md` |
| B | `fix/next-rc21-ci-b-macos-20261112` | `/Users/wei/sideProject/agentdock-next-rc21-ci-b-macos-20261112` | `docs/custom/next-rc21-ci-b-handoff-2026-11-12.md` |
| C | `fix/next-rc21-ci-c-win-permission-20261112` | `/Users/wei/sideProject/agentdock-next-rc21-ci-c-win-permission-20261112` | `docs/custom/next-rc21-ci-c-handoff-2026-11-12.md` |
| D | `fix/next-rc21-ci-d-win-portability-20261112` | `/Users/wei/sideProject/agentdock-next-rc21-ci-d-win-portability-20261112` | `docs/custom/next-rc21-ci-d-handoff-2026-11-12.md` |

HANDOFF 最低內容：目標與原失敗、實際改動檔案、branch/commit exact SHA、Go 版本、測試指令及 PASS/FAIL/未測平台、殘留風險、跨線依賴、確認無 Stable/Original/現役 NEXT 變動。各線可各自本機 commit，**不 Push、沒有 PR、沒有 Release**。各線不更新共享 `roadmap.md`、`next-active-backlog-2026-10-21.md` 或 `.github/workflows/**`。

## 8. 第五線：整合與 Hosted CI Gate（等四線本機交接完成）

1. 先核對四份 HANDOFF 與所有 Worktree clean/commit SHA；在第五個獨立 Integration Worktree 完整納入四條提交，保留原分支歷史、修衝突並測試。
2. 集中執行 Go1.26.5 gofmt、focused Go、vet 和關聯測試。Windows/Linux/Hosted Mac 原生測試仍必須交 GitHub Runner 真跑，不能用本機編譯替代。
3. **僅整合者**評估修正 `.github/workflows/ci.yml`：PR base `custom/main` 也能自動執行 full CI（目前只對 `main` 的 PR 自動觸發）；不修改 Release workflow、關閉 Job、縮小安全 Coverage 或要求第二位 Reviewer。
4. 同步候選到**原有** PR #1，核對遠端 branch/PR SHA，讓 GitHub Windows、Linux、macOS、Browser、Container 全 PASS。失敗即回對應工作線處理；不強制 Merge。
5. 完整 Hosted CI 真 PASS 且 PR 已合併、`origin/custom/main` SHA 確認後，**才**開始 [後續本地與遠端 Branch/Worktree 清理](next-post-ci-branch-worktree-cleanup-plan-2026-11-12.md)。清理前重新盤點 KEEP/CANDIDATE/REVIEW/BLOCKED、dirty/locked/活躍作業與可回滾點；遠端分支逐筆刪除仍需明確確認。
6. 真實 GUI、Windows Installer/更新/rollback、正式 notarization/Release 是另一階段，不因 Source CI PASS 便宣稱完成。

## 9. 四份短提示詞（每份貼到不同 ChatGPT 聊天）

### A — Linux ACP

> 請透過 **Mac-Dev MCP**，在目前聊天模式接續 AgentDock NEXT。先讀 `/Users/wei/sideProject/agentdock-custom/docs/custom/next-rc21-hosted-ci-four-workstreams-2026-11-12.md`，認領**工作線 A（Linux ACP）**。從文件固定 SHA 建立獨立 Worktree，修復三項 ACP MCP Diagnostic Scope 跨平台測試與 Process Identity 支援條件；非 macOS 無可信證據必須 fail-closed，不能偽造成功。完成 Go1.26.5 定點測試、本機 commit 與 A 的 HANDOFF，Linux Hosted CI 未測請註明。**不 Push、不 Merge、不改 CI/其他工作線、不部署或清理分支。**

### B — macOS

> 請透過 **Mac-Dev MCP**，在目前聊天模式接續 AgentDock NEXT。先讀 `/Users/wei/sideProject/agentdock-custom/docs/custom/next-rc21-hosted-ci-four-workstreams-2026-11-12.md`，認領**工作線 B（macOS）**。從固定 SHA 建獨立 Worktree，處理 Codex CLI 測試依賴、Unix Socket 路徑過長、PathSet 與 Browser Guard 殘留測試；假 CLI 不能冒充真實 ACP 驗收，不能干擾原始 Stable 或現役 NEXT。完成 Go1.26.5 本機測試、commit 與 B 的 HANDOFF。**不 Push、不 Merge、不改 CI/其他工作線、不部署或清理分支。**

### C — Windows Permission

> 請透過 **Mac-Dev MCP**，在目前聊天模式接續 AgentDock NEXT。先讀 `/Users/wei/sideProject/agentdock-custom/docs/custom/next-rc21-hosted-ci-four-workstreams-2026-11-12.md`，認領**工作線 C（Windows Permission Durability）**。從固定 SHA 建獨立 Worktree，專修 `internal/permission` 的 Windows 目錄 fsync Access Denied，保留持久化、atomic replace、fail-closed 和 rollback 安全；不得吞掉全部同步錯誤或改 D 的 Fixture。完成 Go1.26.5 本機可行測試、commit、Windows 必驗項目及 C 的 HANDOFF。**不 Push、不 Merge、不改 CI/其他工作線、不部署或清理分支。**

### D — Windows Portability

> 請透過 **Mac-Dev MCP**，在目前聊天模式接續 AgentDock NEXT。先讀 `/Users/wei/sideProject/agentdock-custom/docs/custom/next-rc21-hosted-ci-four-workstreams-2026-11-12.md`，認領**工作線 D（Windows Portability）**。從固定 SHA 建獨立 Worktree，修 Windows Browser Profile/Scope Fixture、測試執行檔 .exe、AppUserModelID 來源檢查並調查 panic；C 獨占 `internal/permission`，不得互改。保留 Browser/External Transport Fence 安全拒絕，完整 Windows 驗證等 C 整合後執行。完成本機測試、commit、D 的 HANDOFF。**不 Push、不 Merge、不改 CI/其他工作線、不部署或清理分支。**
