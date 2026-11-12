# AgentDock NEXT RC21 — F／G／H Windows 修復本機整合與 Hosted CI 待驗（2026-11-12）

> **本機整合 PASS；Windows Native Hosted CI = NOT_VERIFIED；遠端 PR #1 未更新、不可 Merge custom/main。** 這是 source／test 證據，不是 Windows GUI、Installer、Edge、ACP update 或發行驗收。全程使用 Mac-Dev MCP，維持聊天模式，未使用 Codex ACP。

## 1. 痛點、最小處理與界線

ABCDE 已在本機合流，但前一輪 [Full CI 43167819420](https://github.com/jacktdry/agentdock/actions/runs/43167819420) 仍有 Windows 33 個頂層測試失敗、macOS packaging Identity script FAIL，阻擋 PR。F/G/H 各自修正既有 Windows fixture 與必要安全缺口；本次由唯一整合者核對固定來源與 HANDOFF，建立隔離整合 Worktree，依序保留歷史合併並實測。沒有 Push、remote Merge、部署、服務重啟、修改 CI、Release 或清理 branch/worktree。

## 2. Git lineage

| 身分 | 完整 SHA | 說明 |
|---|---|---|
| 舊整合來源 | `c3abd69265d38f76bdd138bfa6a4241e5bf48d6f` | ABCDE ＋ macOS Identity 修正 ＋ F/G/H 分工文件 |
| F/G/H 固定起點 | `a2b47000bbbb3fa349d5f2f739496522bf585d9b` | 三線的已核對祖先 |
| F | `48b6e5430466297fa232d13b7417829c515b9159` | Browser／Edge test-only＋F HANDOFF |
| G | `954269f8144c3a7b4e863ee2f45636d09ee15720` | App/Config/DesktopAPI/HTTPX/Plugin＋G HANDOFF |
| H | `569425ed196820c6d4188213597b39968b4da15c` | DesktopRuntime＋H HANDOFF（含 `ff94f0c2`） |
| F 整合 Merge | `03a91ecd` | no-ff，零 conflict |
| G 整合 Merge | `33ca886a` | no-ff，零 conflict |
| H 整合 Merge | `30de149dd2c295ba55dd9a55adb59376059dfcd6` | no-ff，零 conflict；本機驗證來源 |

- Branch：`integration/next-rc21-hosted-ci-fgh-20261112`。
- Worktree：`/Users/wei/sideProject/agentdock-next-rc21-hosted-ci-fgh-20261112`。
- 三來源 Worktree 和舊整合 Worktree 都保留未改。所有來源 Commit 均為新整合 HEAD 祖先，三線檔案所有權交集空白；`git diff --check`、變更 Go 檔 `gofmt -l` 均 PASS，整合完 repo clean。

## 3. 安全／修改範圍審查

- **F** 僅改 Browser 測試與交接文件。Windows 沒有可信 Edge 簽章、Owner、Process Epoch、Peer attestation 時繼續 fail-closed；未修改正式 Edge Profile／Scope／External Transport Fence。
- **G** Windows Token Bridge 寫 token 前以既有 `securepath.EnsurePrivate` 限制 root 與 token 的 protected DACL；Unix 原有 0600 邏輯保持，失敗清理暫存 token。Windows ACP managed updater 缺乏受信目標時仍拒絕；其餘為 OS-native path/fixture 修正。
- **H** Windows Winsock 10048 只認定 bind address-in-use，不直接視為 Next Owner；10013 保持 unknown。取消不得被身份缺失遮蔽，無 PID/Birth Identity 或 Token 仿造。無正式 Token validation 或 Trusted Fork 條件放寬。
- 三線互不跨越檔案所有權；`internal/permission/**`、`.github/workflows/**`、Stable、Original、現役 NEXT、Browser Profiles、Keychain、ACP Session 均未更動。完整 Windows 原生權限安全仍需 Hosted 驗證。

## 4. 整合後本機實測：macOS arm64／Go 1.26.5

- `GOTOOLCHAIN=go1.26.5 /opt/homebrew/bin/go version` → `go1.26.5 darwin/arm64`。
- `go test -p 2 -count=1 -timeout=5m`：**16/16 完整套件 PASS**，含 `internal/acp`、`internal/app`、`internal/process`、`internal/controlplane`、`internal/desktopruntime`、`internal/mcp/launchregistry`、`internal/permission`、`internal/fs/securepath`、`internal/tool/browser`、`internal/browserpolicy`、`internal/config`、`internal/desktopapi`、`internal/httpx`、`internal/tool/plugin`、`tools/release`、`scripts/test`。
- 相同 16 個套件 `go vet -p 2`：**16/16 PASS**。
- `python3 -m py_compile scripts/test/test-macos-identity.py` 與 `python3 scripts/test/test-macos-identity.py`：**PASS**。
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.26.5 go test -c`：**7/7 Windows cross-build PASS**（Browser、App、Config、DesktopAPI、HTTPX、Plugin、DesktopRuntime），`file` 均證實 PE32+ x86-64；臨時 binary 已自行清理。
- Windows target `GOOS=windows GOARCH=amd64 ... go vet -p 2`：**7/7 PASS**。
- F/G/H 各自交接記錄其他 focused tests；H 單線 focused Race PASS，但本整合沒有執行完整 Race。交叉編譯與 static vet **絕不冒充 Windows Native 執行**。

## 5. Hosted CI 待驗／控制 Gate

- GitHub [PR #1](https://github.com/jacktdry/agentdock/pull/1) 仍 OPEN：head `5af6f00ea25f15d6011378a1debb08e8a6ef69f9`，base `custom/main` 遠端 `4a6c24afb24bfe4893935eca50cdc046a1b2add7`。Run 43167819420（舊 SHA）Ubuntu／Browser／Container PASS，macOS Scripts／Windows Go FAIL；**不含本次 F/G/H 或 macOS 本機修正**。
- **必跑 Windows Native**：F Windows Edge fail-closed、npx、Lease；G Token protected DACL、USERPROFILE、ACP update trust；H Winsock 10048/10013、Port owner fail-closed、Named Tunnel invalid token、Trusted Fork；重跑全 Windows Go Test/Race/Vet/Build 有設定的 Jobs，與舊 33 個頂層失敗逐項對照。
- **必跑全部 Hosted Jobs**：macOS Go／Packaging Scripts（本機 Swift 後段完整 scripts 尚未驗證）、Ubuntu Go、Windows Go、Native Browser Integration、Container。僅在使用者另行授權 Push／PR 更新後，由一位整合者更新**既有** PR #1 並觸發 Full CI，不新增 PR 或關閉失敗 Job。
- Windows GUI／Installer、真實公司 Edge Windows process authority、Windows managed ACP updater、Power-loss、正式 Release／Notarization 與真正三 Actor Host UAT **本次未驗證**；`release_ready=false`。
- 完整 Hosted PASS 以前不得 Merge 遠端 `custom/main`；合併及目標 SHA 核對後，再依 [清理計畫](next-post-ci-branch-worktree-cleanup-plan-2026-11-12.md) 唯讀分類 KEEP／CANDIDATE／REVIEW／BLOCKED。遠端分支刪除需另行明確授權。

**結論：F/G/H 本機合流、Go 1.26.5 回歸與 Windows AMD64 交叉編譯皆 PASS；下一步是 PR 更新授權＋真正完整 GitHub Hosted CI，而不是部署或清理。**
