# AgentDock NEXT RC21 — 工作線 E macOS Codex CLI Test Prerequisite HANDOFF（2026-11-12）

> **結論：E 本機修復已完成，Go 1.26.5 的 `internal/app`、11 個 A/B/C/D 關聯套件（低並行重跑）及 12 個套件 Go Vet PASS。GitHub Hosted macOS/Linux/Windows Full CI 尚未重跑，故 Hosted CI Gate = NOT_TESTED。** 本線沒有 Push、Merge、修改 CI、部署或清理分支／Worktree，也沒有啟動既有 Codex Session 或更動現役服務。

## 1. 目的、固定來源、交付定位

- **使用者實際痛點**：GitHub `macos-latest` 完整 Go Tests 的 `internal/app` 在單純測 Core M8 權限、Registrar、Readiness、Recovery、Browser/Native MCP 安全拒絕前，就被「Runner 沒有 Codex CLI」阻斷，無法驗證真正的測試意圖。
- **可觀察改善**：不需安裝 Codex CLI 即可用明確的測試專用 `CODEX_PATH` 完成 legacy 隔離設定的 MCP removal 前置動作；保留正式環境缺 CLI 必須拒絕的安全性。這不是對真實 Codex、ACP、使用者 Browser/Keychain 的驗收。
- **最簡實作**：只新增一個 Darwin test-only prerequisite helper，接到三個既有測試入口；不修改正式 CLI 解析與正式隔離機制。
- **禁止與未執行**：不動 Original/Stable/現役 NEXT、不碰使用者原始 HOME/Browser Profile/Keychain/已存在 Codex Session、不使用 ACP、不 Push/Merge、不更動 `.github/workflows/**`、不部署、不清理分支或 Worktree。
- **固定共同整合起點**：`eb4652c910829cef02269294af73dcf8c550441d`，A＋B＋C＋D 已整合。
- **E branch**：`fix/next-rc21-ci-e-macos-codex-prereq-20261112`
- **E Worktree**：`/Users/wei/sideProject/agentdock-next-rc21-ci-e-macos-codex-20261112`
- **E 程式修復 Commit**：`4de388c845a2052f90d3f06c0fc71e042bc9031f`
- **本 HANDOFF**：獨立文件 Commit，整合者應以 `git rev-parse fix/next-rc21-ci-e-macos-codex-prereq-20261112` 取得最終 E HEAD 的完整 SHA；該 SHA 亦於本輪交付回報。
- **來源問題**：原 GitHub Full CI Run [43143453728](https://github.com/jacktdry/agentdock/actions/runs/43143453728)；日誌顯示 `prepare ACP profile codex environment: resolve codex CLI for ACP browser isolation: executable file not found in $PATH`。

## 2. 測試分類及最小修改

| 類別 | 原有範例／觸發方式 | E 處理與證據界線 |
|---|---|---|
| Core 權限與安全拒絕／狀態生命週期 | `runtime_next_codex_mcp_authorization_darwin_test.go` 共用 `nextCodexAuthorizationFixture`；其 Readiness、Recovery、Native ID、Operator、Permission 等延伸測試 | 指向 test-owned CLI prerequisite；仍測 Core 真實資料流程與 fail-closed；**不**需要真實 Codex 模型或登入 |
| Core Registrar / source-only guard | `runtime_next_codex_mcp_registration_darwin_test.go` | 僅用新 helper 完成 `NewRuntime` 前置設定；不啟動真實 Adapter |
| Test-owned ACP Adapter prompt | `runtime_next_codex_mcp_prompt_darwin_test.go` | 真正執行的是由 `os.Args[0]` 啟動的 test-only ACP Helper，另以 CLI fixture 做配置清理；**不得**稱為真人 Codex ACP |
| 既有正／負向純邏輯 CLI 檢查 | `acp_browser_test.go`、`acp_browser_next_isolation_darwin_test.go` 等 | 保留既有測試；新增明確缺失 `CODEX_PATH` 的 fail-closed 負向測試 |
| 真實 Codex / 原生 ACP 整合 | `runtime_next_codex_real_b6_darwin_test.go`（`next_b6_real` build tag）、`acp_browser_codex_native_offline_darwin_test.go`（獨立 opt-in）、`acp_browser_codex_adapter_isolated_darwin_test.go`（外部依賴 gated） | **保持原有 gating，不產生假實機證據；本線 NOT_TESTED** |

**實際修改（4 檔，全部位於 `internal/app`，未更動任何 production Go 檔）：**

1. **新增** `internal/app/runtime_next_codex_cli_prerequisite_darwin_test.go`：`nextCodexCLIPrerequisite` 僅接受 Codex profile，產生 `t.TempDir()` 內 `0700` shell fixture；只接受 `mcp remove chrome-devtools|node_repl|computer-use` 及與當次測試綁定的隔離 `CODEX_HOME`。使用 `EnvFromEnv` 傳入**明確絕對路徑 `CODEX_PATH`**；每次測試透過 test-only log 確認三個 removal 均已發生。額外測試指定不存在的 CLI 時，正式解析必須 fail-closed。
2. `internal/app/runtime_next_codex_mcp_authorization_darwin_test.go`：共用 Fixture 在 `NewRuntime` 前綁定上述 CLI；延伸十多個安全測試自然受益。
3. `internal/app/runtime_next_codex_mcp_registration_darwin_test.go`：Registrar 的 `NewRuntime` 入口綁定同樣 Fixture。
4. `internal/app/runtime_next_codex_mcp_prompt_darwin_test.go`：test-owned ACP Prompt Helper 的 `NewRuntime` 入口綁定同樣 Fixture，**不把 fake CLI 當成真實 ACP Adapter**。

正式 `internal/app/acp_browser.go` 中 `resolveCodexCLI`、`prepareCodexACPHome`、MCP 禁用策略與拒絕規則**完全未修改**；未改 `internal/acp`、`internal/permission`、`internal/tool/browser`、A/B/C/D 檔案或 CI。

## 3. macOS 本機真實驗收與重現指令

- 本機系統：`darwin/arm64`；`GOTOOLCHAIN=go1.26.5 /opt/homebrew/bin/go version` → **go1.26.5**。
- E 定點測試：**PASS**（`internal/app`，16.636s），含明確不存在的 `CODEX_PATH` 拒絕、Core 安全拒絕、Registrar、Prompt、Readiness、Native ID、Permission、Recovery 相關案例。
- E `internal/app` **完整套件**：**PASS**（`-count=1 -timeout=5m`，80.882s；測試執行 PATH 不含 Codex）。
- A＋B＋C＋D 的 **11 個完整套件**：低並行 `-p 1` 重跑 **11/11 PASS**（`internal/acp`、`internal/process`、`internal/controlplane`、`internal/desktopruntime`、`internal/mcp/launchregistry`、`internal/permission`、`internal/fs/securepath`、`internal/tool/browser`、`internal/browserpolicy`、`tools/release`、`scripts/test`）。
- **Go Vet 12 個套件：PASS**，即 `internal/app` 加上述 11 個套件。
- **gofmt / git diff --check：PASS**，E 四檔無未格式化檔與差異空白錯誤。

建議整合者在自己的 Worktree 重跑：

```sh
export GOTOOLCHAIN=go1.26.5 GOMAXPROCS=4
GO=/opt/homebrew/bin/go
PATH=/usr/bin:/bin:/usr/sbin:/sbin "$GO" test -p 3 -count=1 -timeout=5m ./internal/app
"$GO" test -p 1 -count=1 -timeout=5m \
  ./internal/acp ./internal/process ./internal/controlplane \
  ./internal/desktopruntime ./internal/mcp/launchregistry \
  ./internal/permission ./internal/fs/securepath \
  ./internal/tool/browser ./internal/browserpolicy \
  ./tools/release ./scripts/test
"$GO" vet -p 2 ./internal/app ./internal/acp ./internal/process \
  ./internal/controlplane ./internal/desktopruntime ./internal/mcp/launchregistry \
  ./internal/permission ./internal/fs/securepath ./internal/tool/browser \
  ./internal/browserpolicy ./tools/release ./scripts/test
git diff --check
```

**高並行殘留風險（不得隱藏）**：最初 `-p 3` 同時測 11 套件時，`internal/process/TestTerminateDoesNotLeaveChildHoldingCommandPipes` 在第 80 次迭代報 `shell did not reach fork boundary`，並輸出 deadlock 訊息；該測試的每次 fork 邊界僅等待約一秒。E 未修改 `internal/process`。同一測試獨立 `-p 1` 重跑 **PASS**（128 次、7.65s）、`internal/process` 全套獨立 **PASS**（9.632s），11 套件 `-p 1` 亦**全部 PASS**。初次失敗可能與同時編譯／高負載時序有關，不能聲稱已根治；整合者需在 Hosted 真跑確認，另有必要再獨立研究 process 壓力測試。

## 4. 無 Codex CLI 模擬及安全／不干擾界線

- 本機開發機**確實安裝了** `/opt/homebrew/bin/codex`；僅從 PATH 移除它不足以真實模擬未安裝環境，因 macOS `commandpath.Path` 仍會追加 `/opt/homebrew/bin`。**整台機器未安裝 CLI 的原生測試 = NOT_TESTED**。
- 這次的隔離證據是：在測試明確透過 `EnvFromEnv` 指定 CLI 的 test-owned 絕對路徑，且用 PATH 不含 Codex 的指令成功跑完整套件；另以明確不存在的 `CODEX_PATH` 驗證不得回落 host Codex。**只能證明這些測試不需要呼叫開發機的 Codex 安裝，不能替代原生無安裝 Hosted Runner。**
- Fixture 的所有可執行檔、log、`HOME`、`CODEX_HOME`、Core state 與 Adapter Helper 均受各測試的私人暫存目錄控制；未寫入使用者原有 `~/.codex`、Keychain、Browser Profile 或真實 Codex Session；沒有安裝、停止、重啟 Original/Stable/現役 NEXT。
- 沒有真正 Native Codex app-server、對外模型呼叫、正式 ACP Browser UAT、macOS GUI 啟動、Windows/Linux 原生執行或 GitHub Hosted macOS CI 的完成證據：一律 **NOT_TESTED**。
- 此修復僅解決單元／測試 Prerequisite 障礙；不能因此宣稱 RC21 正式跨平台 CI、真實使用者 Browser workflow 或 Release ready。

## 5. 後續整合 Session 工作

1. 核對 E branch 及程式 Commit `4de388c845a2052f90d3f06c0fc71e042bc9031f`、最終 E HEAD、HANDOFF、`git status --porcelain` 全乾淨；確認固定起點 `eb4652c910829cef02269294af73dcf8c550441d` 是祖先。
2. 由**獨立 Integration Worktree／整合者**審查 E 的四個 test-only 檔案，安全合入 A＋B＋C＋D 的整合分支（建議使用保留歷史的 `git merge --no-ff`，但**本 E 線沒有 Merge**）。
3. 整合者集中執行 Go1.26.5 `internal/app`、A/B/C/D 關聯測試、vet，並檢查 `process` 壓力測試的偶發情形。
4. 只有在使用者另外授權整合者 Push 與既有 PR 更新後，才由整合者觸發**真正 GitHub** `macos-latest`、`ubuntu-latest`、`windows-latest` 全套 Go Tests 及 Browser/Container Jobs；不修改 CI 來隱藏測試，不把單元 fixture 當作真實 ACP。
5. GitHub full CI Gate 真 PASS、PR 合併等前置門檻全過之後，才依原規劃討論分支／Worktree 清理。**E 本身不清理任何分支、Worktree 或測試以外的使用者資源。**

**E 本機結論：GO for integration review；Hosted CI Gate = NOT_TESTED。**
