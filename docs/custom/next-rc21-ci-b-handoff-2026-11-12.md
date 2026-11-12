# AgentDock NEXT RC21 — 工作線 B macOS HANDOFF（2026-11-12）

## 1. 結論與驗收等級

**B 授權檔案範圍內的 PathSet、Browser Guard 與 Unix Socket 修復已完成並本機 Commit；Codex CLI prerequisite 尚未解決，不得宣稱 macOS Hosted full Go CI PASS。**

- 分支：`fix/next-rc21-ci-b-macos-20261112`
- Worktree：`/Users/wei/sideProject/agentdock-next-rc21-ci-b-macos-20261112`
- 固定起點：`b7a26cd9f5fded276f6459622bb6be4916c5caca`
- 實作 Commit：`abec6e92f9f9e5eae2d8d470f6a13fa672ea2fe4`
- 此 HANDOFF 另有文件 Commit；從上述 Worktree 執行 `git log -1 --format=%H` 可得到文件提交的最終 SHA。
- Hosted 原始失敗：[run 43143453728 / macOS job 130935696624](https://github.com/jacktdry/agentdock/actions/runs/43143453728)
- 驗證環境：Mac Apple Silicon，`GOTOOLCHAIN=go1.26.5` / `go version go1.26.5 darwin/arm64`。
- **未執行 GitHub Hosted macOS CI 重跑**；未執行 Linux／Windows 原生驗收；未用模擬 ACP 取代實際使用者 Agent／權限驗收。

使用者痛點：RC21 Hosted macOS Go Check 被開發工具前置條件、暫存 socket 限制與路徑別名的測試假設阻擋，無法對 PR 合併做可信判斷。直接改善：修掉跨 macOS Runner 可重現的路徑／安全保護問題，讓本機回歸能驗證真實拒絕。最簡作法是修改既有 PathSet 與測試 fixture，不引入常駐服務、額外 Agent、VM 或權限繞過。驗收需要 Go1.26.5 真實執行與 Hosted 重新執行；本線不 Push、不 Merge、不更改 CI、不部署、不清分支。

## 2. 原始失敗與歸因

| 類別 | GitHub macOS Hosted 原失敗 | 根因判定 | 處置 |
|---|---|---|---|
| PathSet | `TestPathSetContainsIntersectsAndResolvesSymlinks`、`TestPathSetCanonicalizesMissingProtectedLeaf` | 保護根經 `canonicalPath` 解析為 `/private/var/...`，但 Contains/Intersects/Descendants 的輸入只 `filepath.Clean`；Hosted `/var` symlink 造成字面比較假陰性，還可能讓真正 symlink alias 繞過保護 | **修復**現有實作，保留 lexical 路徑與 canonical 路徑兩個候選；新增 raw symlink 正負向測試 |
| Browser Guard | `TestNextBrowserModeStatusPreservesGuardWithoutClearing` | Hosted `t.TempDir` 的 `/var` 路徑並非 canonical，產品的 workspace guard 正確判 `invalid`，原 fixture 卻期待 `shared` | **只修測試**：將有效 workspace canonicalize；當 alias 存在時額外確認非 canonical workspace 仍被拒絕，guard bytes 不變 |
| Unix Socket | Launch Registry 的 `TestB6TwentyStdioMCPRegistrationRounds` 等 `listen unix ... bind: invalid argument` | Hosted `t.TempDir` 帶長 test/subtest 名稱，socket path 超過 macOS `sockaddr_un` 限制 | **只修測試**：獨立 0700 private `/private/tmp/ad-mcp-*` fixture，`t.Cleanup` 關閉 listener 後移除；不改正式 listener 路徑或現役服務 |
| Codex CLI | `internal/app` 中 `runtime_next_codex_mcp_authorization_darwin_test.go`、`runtime_next_codex_mcp_prompt_darwin_test.go`、`runtime_next_native_mcp_registration_darwin_test.go`、`runtime_permission_control_darwin_test.go` 等大量測試在 NewRuntime 時 `resolve codex CLI for ACP browser isolation: executable file not found in $PATH` | `internal/app/acp_browser.go` 的 `resolveCodexCLI` 在未指定 `CODEX_PATH` 時要求 PATH 真實 `codex`，Hosted Runner 未安裝；多項原本不需要真實 Codex 的邏輯測試也在初始化時提前中止 | **未更動**：失敗檔案與 fixture 都在 `internal/app/**`，**不是工作線 B 專屬寫入範圍**；需整合者擴充檔案所有權／指派後修復 |

Codex 的建議 patch 邊界：首先盤點測試究竟驗證純邏輯的拒絕／確認流程，還是原生 Codex/ACP 鏈路。純邏輯測試可在 `internal/app/*_darwin_test.go` 的 test-only fixture 明確注入受控的 `CODEX_PATH` 和 `runCodexMCPRemove` 假身分，**限於測試使用、不觸及使用者 HOME/Keychain/既有 Profile、不產生真實 Agent 成功訊號**；不可把此結果標記為 real native ACP PASS。需要真實 Codex CLI 的整合測試必須由後續整合者在合法 CI 環境提供 genuine CLI 或另列明確 prerequisite；缺 CLI 仍應清楚拒絕，不可在 production `resolveCodexCLI` 虛構成功。不得為了 Hosted 綠燈刪除安全負向測試或改弱條件。所有 `internal/app` 修改都需先協調檔案所有權，本線未越界提交。

## 3. 修改檔案（僅四個）

1. `internal/controlplane/pathset.go`：`Contains`、`Intersects`、`Descendants` 改用 lexical＋canonical candidates；如果無法解析仍保留原 lexical 拒絕語意。
2. `internal/controlplane/pathset_test.go`：新增直接對 raw symlink alias 的 protected child、ancestor、descendant 回歸；保留原既有測試。
3. `internal/desktopruntime/next_browser_mode_darwin_test.go`：canonical fixture；增測非 canonical alias 被拒絕，驗證 guard 未被清除。
4. `internal/mcp/launchregistry/registrar_darwin_test.go`：test-only 短 socket private tmp directory 與完整 cleanup；其餘 20-cycle、8-parallel 與 forged child/nonce negative tests 均繼續運行。

**沒有改** `internal/app/**`、ACP Diagnostics、Permission Store、Windows Fixture、`.github/workflows/**`、Original/Stable/NEXT 的任何現役設定或服務，也沒有修改 `~/.agentdock`、Keychain、瀏覽器 Profile、HOME、正式 Socket 路徑。

## 4. Go 1.26.5 本機驗證

首先在原本短 TMPDIR 對原始測試做 baseline：PathSet 2 項、Browser Mode 4 項、Launch Registry 20-cycle + listener + stdio fixture **均 PASS**。這表示短暫存路徑的 Mac 無法直接重現 Hosted fixture 問題；並非 Hosted 沒有真的執行。

完成修正後特意建立**僅測試所有、執行後清理**的 `/var/tmp/agentdock-rc21-b-*/hosted-path-prefix-...` 暫存根，設定 `TMPDIR` 約 **149 個字元**，保留 macOS `/var -> /private/var` 目錄別名；用以下指令在該環境逐項執行，全部 **PASS**：

```sh
export GOTOOLCHAIN=go1.26.5 GOMAXPROCS=4
go test -p 2 ./internal/controlplane -run '^TestPathSet' -count=1 -v -timeout=120s
go test -p 2 ./internal/desktopruntime -run '^TestNextBrowserMode' -count=1 -v -timeout=120s
go test -p 2 ./internal/mcp/launchregistry -run '^(TestVerifiedStdioMCPLaunchAndNoProtocolPollution|TestRegistrarRejectsWrongParentBeforeSpawn|TestRegistrarRejectsWrongServerOrDigest|TestRegistrarRefusesForgedChildAndNonce|TestB6TwentyStdioMCPRegistrationRounds|TestB6EightParallelMCPChildBirthsDoNotCrossAuthorize|TestB6CancelledMCPRegistrationNeverStartsUnderlyingProgram)$' -count=1 -v -timeout=120s
```

驗證範圍含 20 次實際子程序 stdio 註冊、8 路平行 fixture、偽造 parent／birth／nonce 的 fail-closed 和已取消後不 spawn 的回歸。這些是 **test-owned subprocess fixture**，並非真人 Codex ACP、真實模型工具調用或正式 Core/Browser 的 UAT。

另外原生本機 **PASS**：

```sh
GOTOOLCHAIN=go1.26.5 go vet -p 2 ./internal/controlplane ./internal/desktopruntime ./internal/mcp/launchregistry ./internal/tool/file ./internal/tool/media
GOTOOLCHAIN=go1.26.5 go test -p 2 ./internal/tool/file ./internal/tool/media -count=1 -timeout=180s
git diff --check
gofmt -l internal/controlplane/pathset.go internal/controlplane/pathset_test.go internal/desktopruntime/next_browser_mode_darwin_test.go internal/mcp/launchregistry/registrar_darwin_test.go
```

Go vet 與兩個下游完整 package tests PASS，`git diff --check` 無異常，`gofmt -l` 為空。**尚未**重跑原生 Hosted macOS full CI；原有 run 仍 FAIL，不可聲稱 macOS Gate 已解除。這也不能替代 Linux／Windows Native Test 或 real browser/ACP acceptance。

## 5. 交接與限制

- **阻擋**：`internal/app` 的 Codex CLI 測試 prerequisite 仍會使 macOS Hosted `go test ./...` FAIL；需整合者協調新的擁有者與驗證層級，否則即使本線 PathSet/Socket/Guard 修復成功，整體 Hosted Gate 仍不可過。
- **其他風險**：本地 long TMPDIR 是仿真而非 Hosted Runner 原生證據；`/private/tmp` 是 macOS 標準私有可寫 temp 目錄但仍須由 Hosted macOS 真跑確認權限與 cleanup；PathSet lexical＋canonical 採安全保守比對，保護規則的回歸覆蓋已增補。
- **不干擾**：沒有啟動/停止任何正式 NEXT、Original、Stable、AgentDock Core 或 user-owned Browser/ACP Session；沒有操作瀏覽器、Keychain、使用者設定或部署。所有註冊子程序與 socket 僅在測試內建立且依 test cleanup 結束；長暫存路徑經 trap 移除。
- **禁止操作均遵守**：沒有 Push、Merge、PR/CI 修改、Release、部署、跨線覆寫、分支／Worktree 清理。
- **下一步（由整合者）**：先處理 Codex CLI prerequisite 的 `internal/app` 檔案所有權與真實／fixture test 分級，然後把工作線 B 的實作 commit 與本 HANDOFF commit 納入第五個 Integration Worktree，合併四線後跑 Go1.26.5 与 GitHub Hosted macOS/Linux/Windows full CI，只有 Hosted 真 PASS 才考慮解鎖 Gate。
