# AgentDock NEXT RC21 — E 合流後 Hosted CI 殘留與三條 Windows 平行工作線（2026-11-12）

> **範圍：本文件為分工規劃與後續交接，尚未執行 F/G/H。** 使用 Mac-Dev MCP，在 ChatGPT 聊天模式；不使用 Codex ACP。不改現役 NEXT／Stable／Original、不推送未驗收分支、不停用 CI、不清理 branch/worktree。

## 0. 五條線已完成的範圍及 CI 真相

- A/B/C/D/E 已整合在 `integration/next-rc21-hosted-ci-abcd-20261112`，E 合入後 Commit：`5af6f00ea25f15d6011378a1debb08e8a6ef69f9`，源碼狀態 clean 且保留完整歷史。
- 既有 GitHub [PR #1](https://github.com/jacktdry/agentdock/pull/1) 候選已安全 Fast-forward 至上述 SHA，遠端 `custom/main` 尚未 Merge。
- [Hosted full CI run 43167819420](https://github.com/jacktdry/agentdock/actions/runs/43167819420)：**Ubuntu Go Check PASS（含全部 Test/Race/Vet/Build/Script）、Browser Integration PASS、Container PASS**；**macOS Go Test/Build PASS，但 Test macOS scripts FAIL**；**Windows Go Test FAIL**。Source-only NEXT Core darwin/arm64 的 PR checks PASS。
- E 的 Codex CLI prerequisite 已在真實 macOS Hosted Go Test 證實不再阻擋。
- macOS script 失敗的第一個根因在 `scripts/test/test-macos-identity.py`：Next arm64 CI 的實際 `AgentDock-Next-macos-arm64.zip/dmg` 被測試誤判為應是 `-universal`。已在**本機整合線**做最小 test-only 修正（預期單架構 arm64、明確雙架構 universal）；**此變更未包含在上述 Hosted run 的 SHA**。本機 `python3 scripts/test/test-macos-identity.py` 與 Python syntax check **PASS**；完整 `make test-scripts-macos` 已通過 Identity、Installer、i18n、Permission 等前段，但在後續 Swift `.app` 編譯時執行逾時遭停止，因此**整組完整腳本仍為 NOT_VERIFIED，須由 Hosted Runner 重跑**。本機最小修正 Commit 為 `a2b47000bbbb3fa349d5f2f739496522bf585d9b`。
- Windows 前一輪約 333 次測試失敗、約 197 次 Permission Store `fsync Access is denied`；本輪約 **55 次 FAIL（含 subtests），33 個頂層測試、7 個 package**。新日誌中舊的 permission directory fsync、`BROWSER_SCOPE_REQUIRED`、panic 越界均未再出現；**不可因此宣稱 Windows full PASS**。
- **三線共同固定起點**：`a2b47000bbbb3fa349d5f2f739496522bf585d9b`（含 macOS identity test-only 修正、來源已在本機提交並核對；三線不得自行取最新 `custom/main` 或其他 Session branch）；原始候選 `5af6f00e` 仍可回溯。

### 本次 Windows 失敗的最小分類

| Owner | 來源套件 | 本次失敗概況（含 subtests） | 實際疑點 |
|---|---|---:|---|
| **F** | `internal/tool/browser` | 30 | Edge process preflight/profile/signature、npx absolute fixture、lease TTL／pin；Windows 路徑與 OS evidence 限制 |
| **G** | `internal/app`、`internal/config`、`internal/desktopapi`、`internal/httpx`、`internal/tool/plugin` | 15 | Browser policy/isolated root、ACP updater trusted fork fixture、Plugin candidate source 和 Token Bridge |
| **H** | `internal/desktopruntime` | 10 | Native Port observation、Next ownership identity、Named Tunnel token reason、ACP update package provenance |

**注意**：失敗次數是 test/subtest 行數，非獨立根因。F/G/H 各自應再把錯誤分成 fixture、平台能力不支援、可能真實產品缺陷與重試時間／測試時序四類；不得先假定全部都是測試噪音。

## 1. 平行工作線 F — Windows Browser／Edge Preflight

**檔案所有權**：`internal/tool/browser/**`（優先只改 `*_test.go`；如遇真實 Runtime 問題，先證明 OS 原生行為與安全影響）。不得修改 G/H 文件與代碼或既有 `internal/permission/**`、CI YAML。

- 對照本輪 Windows job 的 `TestEdgeEvidencePinnedReject`、`TestEdgeProfileMetadataBoundedReadOnly`、`TestEdgeProcessPreflight...`、`TestEdgeCommandMatchesConservativeArguments`、`TestConcurrencyIdleTTLAndSweeps`、`TestExternalWorkerConfig`、`TestManagedWorkerPinAndVersionProbe` 等實際失敗。
- 將 Unix-only 資料、Windows 路徑、模擬 Edge 命令列及 npx executable fixture 與 OS API 觀測資格分開；不可冒充 Microsoft 簽章、Edge kernel process identity 或讓 unsigned/wrong profile 通過。所有外部瀏覽器存取仍保持公司 Profile／Scope／Transport Fence fail-closed。
- 本機可用 Go1.26.5 定點 Mac Tests、Windows test binary cross-compile；Windows native PASS 只能由後續整合者在 GitHub Hosted Runner 真跑。
- 交接報告 `docs/custom/next-rc21-ci-f-windows-browser-handoff-2026-11-12.md`，記錄每組失敗、修復點、測試證據、未驗證 Windows 行為。

## 2. 平行工作線 G — Windows Desktop/API／設定與 Plugin

**檔案所有權**：`internal/app/**`、`internal/config/**`、`internal/desktopapi/**`、`internal/httpx/**`、`internal/tool/plugin/**`；優先只處理 Windows 顯現之 fixtures/必要來源，不更動 E 的 macOS Codex test-only fixture。**不寫 `internal/desktopruntime/**` 或 F 的 Browser 模組**。

- 對照：`TestRuntimeACPBrowserNextSharedWorkspaceOptIn`、`TestRuntimeBrowserDesktopConfiguredAndRetainedRoutes`、`TestBrowserWorkspacePoliciesConfig`、`TestBrowserCatalogConfig`；以及 Desktop ACP probe/update/candidate/connection token 相關 7+ 組失敗、Plugin `TestDecodeDesktopCandidateRequestStrict`。
- Windows absolute/canonical paths、可信 fork/release、profile 私密參數、Plugin candidate source trust 和 loopback-only 必須維持原本的安全拒絕。若問題根源在 DesktopRuntime，文件中提供最小重現與交接 H，不越界修改。
- 本機 Go1.26.5 focused tests、Go vet、Windows cross-compile；不以 cross-build 取代原生 Windows Hosted Test。
- 交接報告 `docs/custom/next-rc21-ci-g-windows-app-handoff-2026-11-12.md`，包含 Windows fixture 差異、ACP/Plugin 信任機制與跨 H 依賴。

## 3. 平行工作線 H — Windows DesktopRuntime／Port／Tunnel

**檔案所有權**：`internal/desktopruntime/**`（優先 Windows testcase/fixture）。不得改 F 的 Browser、G 的 App/Config/DesktopAPI/Plugin，或 Permission Store 和 CI YAML。

- 對照：`TestPortAvailableTempScope`、`TestPortMutationReobservesWithoutAcquiringLock`、`TestPortOccupiedFixtures`、`TestPortProbePreservesCallerDeadline`、`TestPortInvalidIdentityAndScope`、`TestWaitNamedTunnelReadyRejectsInvalidTokenLog`、`TestCheckACPProfileUpdate...`。
- 區分 Windows 無可信 process-birth identity 時的正確 `next_identity_unavailable` 拒絕、Port/Tunnel fixture 與真正產品錯誤。不得為了測試通過偽造 Owner/Port/Epoch 身分或忽略 token 失效。
- 本機 Go1.26.5 focused tests、Go vet、Windows cross-compile，然後由整合者做 GitHub Hosted Windows 真實測試。
- 交接報告 `docs/custom/next-rc21-ci-h-windows-desktop-handoff-2026-11-12.md`，列出不可跨平台模擬的原生情境。

## 4. 共用 Worktree／Git／測試規則

所有 Session 必須先讀本文件與 `AGENTS.md`，由 Mac-Dev 檢查 Git status/branch/Worktree、Source SHA、現役服務不干擾契約，從**固定 SHA**新建各自 Worktree。若指定名稱已存在，先 inspect 不要無條件覆蓋。可各自本機 commit 專屬 HANDOFF，**不 Push／Merge／改 CI／部署／分支清理**，不改共享 Roadmap/Backlog/本文件。不要直接在 `custom/main` 或已整合候選分支開發。

| 線 | Local branch | 專屬 Worktree |
|---|---|---|
| F | `fix/next-rc21-ci-f-win-browser-20261112` | `/Users/wei/sideProject/agentdock-next-rc21-ci-f-win-browser-20261112` |
| G | `fix/next-rc21-ci-g-win-app-20261112` | `/Users/wei/sideProject/agentdock-next-rc21-ci-g-win-app-20261112` |
| H | `fix/next-rc21-ci-h-win-desktop-20261112` | `/Users/wei/sideProject/agentdock-next-rc21-ci-h-win-desktop-20261112` |

整合者在三線回報後，對每筆提交檢查 ownership/commit SHA、保留完整 commit history，在第五個乾淨 Worktree 集中整合，Go1.26.5 與 macOS scripts 回歸，然後更新**既有** PR #1 並啟動 Full CI（Ubuntu/Windows/macOS/Browser/Container）。只有所有原生 Hosted checks 真 PASS，才有資格 Merge 遠端 `custom/main` 與後續分支清理。這些 workstreams **不是發版、Windows GUI/Installer 實機或真實 ACP Browser UAT**。

## 5. 可直接貼入獨立 ChatGPT Session 的提示詞

### F

> 請透過 Mac-Dev MCP、維持 ChatGPT 聊天模式執行 AgentDock NEXT **工作線 F：Windows Browser／Edge Preflight**。先讀 `/Users/wei/sideProject/agentdock-next-rc21-hosted-ci-abcd-20261112/docs/custom/next-rc21-hosted-ci-post-e-windows-parallel-2026-11-12.md`。依固定 SHA 建 F 專用 Git Worktree，對照 GitHub Full CI Run 43167819420 的 Windows Browser/Edge Preflight、npx、TTL/Lease 失敗，在 `internal/tool/browser/**` 內最小修正；保留 Microsoft 簽章／process owner、公司 Edge Profile、Scope、Transport Fence 的安全拒絕。完成 Go1.26.5 可執行測試、Windows Native 待驗清單、本機 Git Commit 與 F HANDOFF。**不 Push、Merge、改 CI、動其他工作線、部署或清分支；不以 cross-build 冒充 Windows Native PASS。**

### G

> 請透過 Mac-Dev MCP、維持 ChatGPT 聊天模式執行 AgentDock NEXT **工作線 G：Windows App／DesktopAPI／Config／Plugin**。先讀 `/Users/wei/sideProject/agentdock-next-rc21-hosted-ci-abcd-20261112/docs/custom/next-rc21-hosted-ci-post-e-windows-parallel-2026-11-12.md`。依固定 SHA 建 G 專用 Git Worktree，處理 Run 43167819420 在 `internal/app`、`internal/config`、`internal/desktopapi`、`internal/httpx`、`internal/tool/plugin` 的 Windows 路徑、Browser Policy、ACP trusted fork/update、Plugin candidate/Token Bridge 測試；勿放寬安全檢查。`internal/desktopruntime` 歸 H，不越界修改；完成 Go1.26.5 定點驗證、本機 Commit、G HANDOFF 與 Windows Native 待驗清單。**不 Push、Merge、改 CI、部署或清分支。**

### H

> 請透過 Mac-Dev MCP、維持 ChatGPT 聊天模式執行 AgentDock NEXT **工作線 H：Windows DesktopRuntime／Port／Tunnel**。先讀 `/Users/wei/sideProject/agentdock-next-rc21-hosted-ci-abcd-20261112/docs/custom/next-rc21-hosted-ci-post-e-windows-parallel-2026-11-12.md`。依固定 SHA 建 H 專用 Git Worktree，專注 Run 43167819420 的 `internal/desktopruntime` Port observer、Next identity、Named Tunnel token、ACP update 測試失敗。Windows 缺乏 OS birth proof 時仍必須 fail-closed，不得偽造成功；F/G 所有模組禁止改動。完成 Go1.26.5 定點測試、Windows Native 待驗清單、本機 Git Commit 與 H HANDOFF。**不 Push、Merge、改 CI、部署或清分支。**
