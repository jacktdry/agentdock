# AgentDock NEXT — RC21 Hosted CI 工作線 D：Windows Portability HANDOFF

> 2026-11-12（Asia/Taipei）｜**D source-only／test-only 完成，Windows Native Hosted CI 尚未驗收**。不 Push、不 Merge、不更動 CI、不部署、不清理分支；未改 C 獨占的 `internal/permission`。

## 1. Git 基準與交付

| 項目 | 確定值 |
|---|---|
| 固定起點 SHA | `b7a26cd9f5fded276f6459622bb6be4916c5caca` |
| D 獨立 Worktree | `/Users/wei/sideProject/agentdock-next-rc21-ci-d-win-portability-20261112` |
| D 分支 | `fix/next-rc21-ci-d-win-portability-20261112` |
| D 程式／測試 Commit | `b64b66971197c32dcb94683e54fc98b110975417` |
| 本 HANDOFF | `docs/custom/next-rc21-ci-d-handoff-2026-11-12.md` |
| HANDOFF 最終 Commit | 使用 D Worktree 的 `git rev-parse HEAD` 取得；文件不在自身提交前預填自身 Commit SHA |

固定 SHA 僅作 D 的獨立 fork，不含 A/B/C 尚未整合的變更。D 的變更限於 11 個**測試檔**與本文件。沒有修改 Runtime 的 Scope、驗證、Transport、Permission Store 正式邏輯，也沒有跨線同步或合併。

## 2. 問題 → 根因 → 最小修正

| 問題／Windows Hosted CI 證據 | 根因 | D 修正與安全界線 |
|---|---|---|
| `internal/browserpolicy` 大量 `user_data_dir must be a canonical absolute non-root path` | `catalogFixture()` 使用 Unix root separator；Windows 的 `\\configured\\edge` 非含磁碟代號的絕對路徑 | `catalog_test.go` 用 `filepath.Join(os.TempDir(),...)` 的 OS-native canonical absolute path；正式 Catalog 仍拒絕非 canonical／root／traversal |
| Browser Planner / Native NEXT Edge 範例因同一 Profile 路徑失效 | `route_planner_test.go` 和 `next_edge_runtime_test.go` 使用 POSIX-only path | 共用 `testEdgeProfileDir()`；仍驗證 required external、origin scope、ownership、profile selection，未放寬路由 |
| `BROWSER_SCOPE_REQUIRED` 大量連鎖錯誤，外部流程尚未啟動 | 所有 Managed/External Lease 共用 `leaseScope()` 的 `CanonicalWorkspaceRoot: "/workspace"`；Windows 不是絕對路徑 | `managed_leases_test.go` 以 OS-native absolute workspace fixture；**保留 invalid/untrusted Scope 拒絕與外部路由不可降級** |
| `external_leases_test.go:405` `index out of range [-1]` | 先被無效 Scope 拒絕，`b.calls` 為空；測試卻直接存取 `b.calls[len(b.calls)-1]`，產生二次 panic | 修正 Scope fixture 並在最後呼叫斷言加入 `len(b.calls)==0` guard；仍要求 cleanup 最後是 `stop`，不能藉此掩蓋原始錯誤 |
| Direct CDP Profile 讀取錯誤，`dispatcher retired` | `external_direct_cdp_context_test.go` 中 `/owned/edge/Default` 不符合 Windows `filepath.IsAbs` | OS-native `testOwnedEdgeDataDir()`，同時保留 wrong Profile、traversal、wrong context/type、僅可讀 `edge://version/`、禁止重用 company target 等拒絕測試 |
| Edge socket／pinned evidence 正向 fixture 在 Windows 不可用 | 測試 `userDataDir: "/private/edge-fixture"` 是 Unix-only | 兩個 socket/evidence test fixture 改 OS-native canonical path；仍使用**合成的 macOS signer／socket 證據**，不得聲稱 Windows OS attestation 或真實 Edge 認證 |
| Edge Process Epoch 多項 macOS fixture 在 Windows 顯示 `external Edge process identity unqualified` | 來源為 macOS `ps/lsof`、POSIX profile ancestor、macOS executable，不能在 Windows 造出可信 OS 身分證據 | `external_edge_epoch_guard_test.go` 的 Windows 分支**實際執行** fail-closed 斷言：不產出 preflight 身分、不得探查 listener/signature、不得建立 external fence。macOS 原本的正向／多輪 epoch／post-action quarantine 情境完整保留。**Windows 並未執行那些 macOS 正向情境**，不可將綠燈解讀成跨平台正向驗收 |
| `TestNextOverlayBinary` 找不到執行檔 | Windows Go build 產生 `.exe`，測試卻執行無副檔名的 fixture | `tools/release/next_candidate_test.go` 為 fixture 與 arbiter fixture 增加 Windows `.exe` 名稱，保留 overlay provenance／clean HEAD 等驗證 |
| AppUserModelID source search false negative | `App.xaml.cs` 從 `WindowsProductIdentity.Current.AppUserModelId` 讀取，而 stable/next 常數已移到 `Models/WindowsProductIdentity.cs` | `scripts/test/desktop_windows_test.go` 分別檢查 App 使用路徑、stable/next 原始常數及 Installer stable shortcut ID；**未修改產品識別實作** |

以上 panic 與路徑歸因依據：GitHub Actions `jacktdry/agentdock` Windows `Go check` run **43143453728**，2026-11-12 原固定 SHA `b7a26cd9`；本線僅**唯讀**拉取失敗 log，未觸發任何 Actions 或編輯 CI。

## 3. 本機實際驗證（Go 1.26.5）

在 **macOS Mac-Dev** 上、D Worktree 執行：

```bash
GOTOOLCHAIN=go1.26.5 go test -count=1 ./internal/browserpolicy ./internal/tool/browser ./tools/release ./scripts/test
# 4 packages PASS（程式 Commit 後再次完整回歸亦 PASS）

GOTOOLCHAIN=go1.26.5 go test -count=1 -run 'TestEdgeEpoch|TestEdgeEstablishedSocket|TestEdgeEvidence|TestExternalAdmission|TestExternalAcquireFailureCleanup|TestDirectCDP|TestPlanner|TestManagedLease' ./internal/tool/browser
# PASS（Edge epoch Windows fail-closed 分支加入後再次跑 Mac 定點測試）

GOTOOLCHAIN=go1.26.5 go vet ./internal/browserpolicy ./internal/tool/browser ./tools/release ./scripts/test
# PASS
```

Windows `amd64` **僅交叉編譯**、非在 Windows 執行：

```bash
for pkg in internal/browserpolicy internal/tool/browser tools/release scripts/test; do
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.26.5 \
    go test -c -o "/tmp/agentdock-rc21-d-$(echo "$pkg" | tr / -).test.exe" "./$pkg"
done
# 四個 Windows target test .exe 均成功產出；未在 Windows 執行
```

11 個修改測試檔經 Go1.26.5 `gofmt`，`git diff --check` PASS。**尚未實際重跑修正後 Windows `go test`、Windows runtime／GUI／Edge 實機 UAT 或 Windows Hosted Workflow**，不能聲稱 Windows PASS 或 RC21 Hosted CI GO。

## 4. C 整合後，Windows Hosted Runner **必驗清單**

- [ ] 由整合工作線將 A/B/C/D 以明確順序整合到新候選分支，再觸發 Windows Hosted Go 1.26.5。此 D Worktree 不做 Merge／Push／CI 修改。
- [ ] 確認 **C `internal/permission` 持久化修復已整合**，而非先關閉 Store、忽略目錄 fsync 或略過測試；檢查 Atomic Replace、Fail-closed、rollback、實際重開 Store 的資料持久性及 Access Denied 不再發生。
- [ ] Windows Native 執行 `go test -count=1 ./internal/browserpolicy ./internal/tool/browser ./tools/release ./scripts/test`（不要僅看 cross-compile 成功）。
- [ ] Browser 核對 `TestCatalogUserDataDir`、`TestCatalogWorkspacePolicies`、`TestPlannerCompanyIntentAndCanonicalScope`、`TestNativeEdgeProfileSelectionIsExplicitAndUnambiguous`、`TestDirectCDPOwnedProfileProbeCannotBecomeCompanyTarget`、`TestDirectCDPOtherTypeRequiresOwnedHiddenTargetAndSameContext`、`TestDirectCDPProfileWaitsForOwnedVersionDocument`。
- [ ] Scope／外部 Transport 拒絕回歸：`TestExternalAdmissionMissingTransportFenceConsumesGrantWithoutDispatch`、`TestExternalAdmissionVerifierFailuresPreserveUserPages`、`TestExternalManagerDispatchNeverUsesLegacyCall`、`TestExternalStartValidation`、`TestExternalAcquireFailureCleanup`，確認 invalid scope 無 dispatch、無 fallback、無 user page 變更，且無 `[-1]` panic。
- [ ] Windows Edge Epoch 僅應**拒絕**尚未支援的 macOS 身分證據；不可因合成測試就將 Windows Native CDP attestation 視為已合格。macOS 環境另重跑完整 Epoch 正向與 quarantine cases。
- [ ] `TestNextOverlayBinary`、`TestNextArbiterOverlayBuild`、`TestWindowsControlPanelUsesStableAppUserModelID` 在 Windows Native PASS，檢查 `.exe`、stable/next identity 不混用。
- [ ] A／C 整合後再跑 Windows `go test ./...`，釐清 Go 全倉失敗中哪些是獨立外部問題；保留 Hosted log、實際測試結果、fail-closed 驗證與整合 SHA。

### D 不負責、不可藉由本線掩蓋的其他失敗

| 來源 | 原 Windows CI 問題與交接建議 |
|---|---|
| **C：`internal/permission`** | Directory fsync `Access is denied` 造成大量 `internal/app`、MCP、Store 測試連鎖失敗。C 專屬，**D 未修改、未假設已修復** |
| **A：`internal/acp`** | 三項 MCP Diagnostic Scope / Process Identity 跨平台失敗。由 A 檢查，不可捏造 Windows OS identity |
| **其他 owner：`internal/tool/plugin/desktop_candidate_test.go`** | `TestDecodeDesktopCandidateRequestStrict` valid `/private/plugin` 在 Windows 非 native absolute path；**超出 D 文件允許的寫入範圍**，整合者另分派測試 fixture 修正 |
| **其他 owner：`internal/app`** | `TestRuntimeACPBrowserNextSharedWorkspaceOptIn` 報 `Next Browser startup requires its isolated state root`，需依 A/B/整合基底重新歸因，不因 D 成果宣稱通過 |
| **其他 owner：`internal/config`** | `TestBrowserWorkspacePoliciesConfig`、`TestBrowserCatalogConfig` 原 Windows run 失敗，整合時應核對是否另有 POSIX-only fixture／C 連鎖失敗；D 不越權修改 |

## 5. 安全與停止點

**Browser Scope、Required External Route、External Transport Fence 都維持嚴格拒絕**；此次沒有改動任何 production runtime 校驗、沒有給未合格 Browser/CDP 任何新權限。所有可通過的 positive source tests 都是離線／合成 fixture，不代表登入使用者 Edge、現役 NEXT、真實 ACP 或 OS-level session authority。

D 的實作與 handoff 可供整合者 cherry-pick／merge 作審核候選；由**獨立整合工作線**決定整合與 Hosted CI 順序。此次未 Push、未 Merge、未修改任何 CI、未部署、未清理本地或遠端分支；其餘三工作線及 `custom/main` 均保持原狀。
