# AgentDock NEXT RC21 — Hosted CI 四線本機整合結案／未解阻（2026-11-12）

> **結果：A＋B＋C＋D 的 Source Commit 與四份 HANDOFF 已安全在本機整合；本機 Go 1.26.5 關聯測試 PASS，但 Github Hosted Ubuntu/macOS/Windows Full CI 尚未重跑，macOS Codex CLI test prerequisite 仍 OPEN，禁止宣稱 CI Gate PASS。** 無 Push、遠端 Merge、CI YAML 更動、Release、安裝/重啟服務或分支清理。

## 基準與來源

- 固定分工起點：`b7a26cd9f5fded276f6459622bb6be4916c5caca`。
- 現有本機 `custom/main`：`c72e0c84530659055893cb381f7ff1c3624d0b81`（包含四線規劃文件）；**本輪未修改它**。
- 新獨立整合 Worktree：`/Users/wei/sideProject/agentdock-next-rc21-hosted-ci-abcd-20261112`。
- 新整合 Branch：`integration/next-rc21-hosted-ci-abcd-20261112`。
- **四線 Merge 後 SHA**：`cd9162e6551e3ea8ca63763e5862d95454959dd5`；依序 `git merge --no-ff --no-edit` 合入，**4/4 零 conflict**，保留每條線完整 commit history。
- A `fix/next-rc21-ci-a-linux-acp-20261112`：`5414e0150e1bed4c83c6497a91e128f80edf52ab`，implementation `4c810f3a`。
- B `fix/next-rc21-ci-b-macos-20261112`：`c16ea7dccdae906d06f4e3121ad4fb82eb4b6bf1`，implementation `abec6e92`。
- C `fix/next-rc21-ci-c-win-permission-20261112`：`bd892fe7c8d993e6bda08d32857dd3d6fd580b8b`，implementation `51b38a5c`。
- D `fix/next-rc21-ci-d-win-portability-20261112`：`c860592708f6719f8af30bfcae5543f46606c28e`，implementation `b64b6697`。
- 四線各自 `status --porcelain --untracked-files=all` 完全乾淨；檔案所有權交集為空。原始 A/B/C/D branches & Worktrees 保留供回溯，不要現階段清理。

## 本機實測證據（Go 1.26.5，macOS arm64）

- `GOTOOLCHAIN=go1.26.5 go version` → `go1.26.5 darwin/arm64`。
- `git ls-files -z '*.go' | xargs -0 "$(GOTOOLCHAIN=go1.26.5 go env GOROOT)/bin/gofmt" -l` → **零未格式化檔案**。
- `GOTOOLCHAIN=go1.26.5 go test -p 3 -count=1 -timeout=5m`，以下 11 個完整套件 **11/11 PASS**：
  `./internal/acp`、`./internal/process`、`./internal/controlplane`、`./internal/desktopruntime`、`./internal/mcp/launchregistry`、`./internal/permission`、`./internal/fs/securepath`、`./internal/tool/browser`、`./internal/browserpolicy`、`./tools/release`、`./scripts/test`。
- 同樣 11 個套件執行 `GOTOOLCHAIN=go1.26.5 go vet -p 3` → **PASS**。
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.26.5 go test -c`，四個套件的 Windows `.test.exe` **4/4 build PASS**：`./internal/permission`、`./internal/tool/browser`、`./internal/browserpolicy`、`./tools/release`。僅編譯，不代表執行 Windows Native tests；臨時輸出已刪除。
- 整合完成時 `git status --porcelain` 為空。**未執行**完整 `go test ./...`、未在 GitHub Hosted 執行新的全平台 CI、未做真實 macOS GUI／Codex ACP／Browser UAT、Windows Installer/Power-loss 測試。

## 核心未結案缺口與下個所有權

**E — macOS Codex CLI prerequisite（需第五條有界修復工作線，非 A/B/C/D 未提交）。**

- 依 B HANDOFF，GitHub 原 [Full CI Run 43143453728](https://github.com/jacktdry/agentdock/actions/runs/43143453728) 的 `macos-latest` 多個 `internal/app/*_darwin_test.go`，`NewRuntime` 前置驗證要求真實 `codex` CLI，Runner PATH 不存在，報 `resolve codex CLI for ACP browser isolation: executable file not found in $PATH`。這批檔案沒有在 B 授權範圍，B 正確保留未修；目前仍預期造成 Hosted macOS Go Test FAIL。
- 最小下一步：由整合者或新 E 專用工作樹，**先分類** test-only pure logic/negative tests vs 真實 Codex/ACP integration tests。純邏輯 fixture 可使用 test-owned `CODEX_PATH` 與受控 CLI stub，但不得在程式碼正式路徑把缺 CLI 視為成功；整合／原生證據應使用真實 CLI 並明確標記，不以 stub 冒充 Host PASS。
- E 只能修改 `internal/app` 的必要測試 fixture 與獨立 E HANDOFF；不可無條件修改 `internal/acp`、`internal/permission`、`internal/tool/browser`、已有四線提交；不能直接更改 GitHub CI Workflow 逃避失敗。任何跨範圍需求先記錄並由整合者決定。
- 整合者也應審查 B 的 PathSet lexical/canonical safety、C Windows `MoveFileExW`/ACL/Fail-closed 原生行為。即使 Windows `.exe` 編譯成功仍**必須**由 GitHub Windows Runner 實測；非 macOS Process Birth Identity 仍必須 fail-closed。

## GitHub 與清理 Gate（尚未跨越）

- PR [#1](https://github.com/jacktdry/agentdock/pull/1) 仍 OPEN，head `integration/next-rc21-github-ci-20261112` 於本輪仍為 `b7a26cd9f5fded276f6459622bb6be4916c5caca`；遠端 `origin/custom/main` 仍為 `4a6c24afb24bfe4893935eca50cdc046a1b2add7`。
- 等 E 缺口具體修復並整合、審查全部安全回歸後，由**單一**整合者決定是否讓 CI PR base=`custom/main` 自動觸發，再經明確授權推送至原 PR #1，同一次完整 Hosted CI 驗證 Ubuntu／macOS／Windows Go Tests、Browser integration、Container。不能忽略失敗或稱舊 Source-only NEXT CI PASS 是 full CI PASS。
- Hosted full CI 全部 PASS、PR 內容與 exact SHA 已確認、遠端 `origin/custom/main` 成功合併與核對之後，才啟動 [Branch/Worktree 安全清理計畫](next-post-ci-branch-worktree-cleanup-plan-2026-11-12.md)。先重新盤點，不碰 dirty／locked／active，遠端刪除須獨立確認。
