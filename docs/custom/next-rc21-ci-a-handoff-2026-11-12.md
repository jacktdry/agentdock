# AgentDock NEXT RC21 — 工作線 A（Linux ACP Diagnostic）HANDOFF

日期：2026-11-12；**A 本機實作與驗證完成，Linux Hosted CI 尚未重跑；不得視為 Hosted Gate PASS。**

## 1. 基準與目的

- 共用原始 Repo：`/Users/wei/sideProject/agentdock-custom`
- 固定起點：`b7a26cd9f5fded276f6459622bb6be4916c5caca`
- A 專用 Branch：`fix/next-rc21-ci-a-linux-acp-20261112`
- A 專用 Worktree：`/Users/wei/sideProject/agentdock-next-rc21-ci-a-linux-20261112`
- **程式碼 Commit：`4c810f3a71232af4cc3edc6691d67bfa4b1f263f`**
- 交接文件另以後續本機 Commit 提交；整合者請以 `git log -2 --format='%H %s'` 核對兩筆完整 SHA。
- 對應里程碑：RC21 Hosted full CI 三 OS Go Tests 的工作線 A 阻擋修復；下一個實際 Gate 是整合後 GitHub Hosted Ubuntu 真正執行 Go Test。

已知 Hosted [Run 43143453728](https://github.com/jacktdry/agentdock/actions/runs/43143453728) 的 Ubuntu Go Test 有三項失敗（依 RC21 分工文件，這個 Session **沒有重新查詢遠端或重跑 Hosted CI**）：

1. `TestMCPDiagnosticScopeOnlyLocalLoadedSnapshot`
2. `TestMCPDiagnosticScopeFencesConcurrentManagerMutation`
3. `TestNextEarlyMCPDiagnosticRequiresLiveAdapterButNotLoadedSession`

三項先前都期待 `withMCPDiagnosticScope` 在 Linux 成功執行 callback。但 `internal/process/identity_other.go` 的 `!darwin` observer 明確回傳 `ErrIdentityUnavailable`，`withMCPDiagnosticScopePolicy` 因此正確地 fail-closed；這不表示 CI 缺少 Linux 環境。**本 Session 未在原生 Linux 上重現原始失敗；失敗名稱和歸因源自既有文件與本地程式碼核對。**

## 2. 修復決策與檔案範圍

最小、安全的修復是**測試平台拆分**，而不是把不可信的 PID、StartTime、UID、PPID 或其他程序出生身分偽造成成功：

- `internal/acp/mcp_owner_diagnostic_scope_test.go` → `internal/acp/mcp_owner_diagnostic_scope_darwin_test.go`（99% rename；新增 `//go:build darwin`）。原有三項正向測試、dirty binding / redaction、取消、transport loss、鎖定管理員 map 等 Darwin 正負向覆蓋**完整保留**，不弱化任何生產檢查。
- 新增 `internal/acp/mcp_owner_diagnostic_scope_other_test.go`（`//go:build !darwin`）。使用既有 `newTestManager` 啟動**測試自身擁有的真實 Adapter 子程序**與 loaded Session；確認 `ObserveBirthIdentity` 對該真實 PID 回傳 `ErrIdentityUnavailable` 和空身分，且普通 Scope、`allowEmpty=true` + loaded Session、`allowEmpty=true` + empty Session **均回傳 `errMCPDiagnosticNotQualified`，callback 從未被呼叫**。
- **未修改** `internal/acp/mcp_owner_diagnostic_scope.go` 或 `internal/process/identity_other.go` 的執行時安全拒絕行為；未實作／假設 Linux 或 Windows 已有可驗證 Process Birth Identity。
- 沒有改任何 B/C/D 專屬檔案、Permission Store、Browser Fixture、CI YAML 或共用 Roadmap。

安全契約：若未來要在 Linux/Windows 開放診斷成功路徑，必須另外設計並證明該 OS 上可信、可重採樣的 PID + kernel birth identity（含必要的 parent/UID/group 語義）及完整 negative tests；本次變更**不授權任何非 Darwin 正向診斷／程序回收**。`allowEmpty` 仍只放寬 loaded Session 前提，絕不放寬 OS Birth Identity 門檻。

## 3. Go 1.26.5 本機驗證矩陣

本機：macOS Darwin arm64；`GOTOOLCHAIN=go1.26.5 go version` → `go version go1.26.5 darwin/arm64`。沒有以系統預設 Go 1.27.3 的輸出代替測試證據。

| 項目 | 實際指令／條件 | 結果 | 證據邊界 |
|---|---|---|---|
| macOS focused | 下方 focused 指令 | **PASS**（含三項原失敗測試與 Darwin 正負向） | macOS 原生測試，非 Linux |
| macOS ACP/Process full packages | `GOTOOLCHAIN=go1.26.5 go test ./internal/acp ./internal/process -count=1` | **PASS**（ACP 14.971s、Process 11.305s） | 僅兩個相關套件，不代表整個 Go repo |
| macOS vet | `GOTOOLCHAIN=go1.26.5 go vet ./internal/acp ./internal/process` | **PASS** | macOS target |
| Linux cross-build | `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.26.5 go test -c -o <temp-file> ./internal/acp`；同樣對 `./internal/process` | **PASS**（兩個 ELF x86-64） | **只有編譯；沒有執行 Linux 測試** |
| Windows cross-build | `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.26.5 go test -c -o <temp-file>.exe ./internal/acp`；同樣對 `./internal/process` | **PASS**（兩個 PE32+ x86-64） | **只有編譯；沒有執行 Windows 測試** |
| 格式與 staged patch | `GOTOOLCHAIN=go1.26.5 gofmt -w`（兩個 A 測試檔）、`git diff --cached --check` | **PASS** | 僅 A 改動 |
| Ubuntu Hosted Go Tests | 整合後 GitHub Actions Hosted `ubuntu-latest` | **未測／待整合者** | **禁止宣稱 PASS** |
| Windows Hosted / macOS Hosted Go Tests | 整合後的 Hosted Runner | **未測／待整合者** | 本機和 cross-build 不能代替 |

macOS focused 實際指令（表格外保留原樣，不需跳脫 regex `|`）：

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/acp ./internal/process -run 'Test(MCPDiagnosticScope|NextEarlyMCPDiagnostic|BirthIdentity)' -count=1 -v
```

交叉編譯輸出位於測試專屬 `/tmp` 暫存資料夾，已由 `trap` 清除；無服務安裝或長駐程序。

## 4. 安全性、不干擾與殘留風險

- 原生正向測試只限 Darwin；非 Darwin 必須回傳固定的 redacted refusal，無 callback 外洩 loaded/adapter Session 證據，也不因 `allowEmpty` 放行。
- 測試使用本身建立、由 `Manager.Close` 收尾的 Adapter，**沒有操作 Stable/Original、現役 NEXT、8765/8767、既有 Browser/Edge Profile 或第三方程序**；沒有開 GUI 或部署服務。
- **仍待驗收**：原生 Ubuntu 執行新增 `TestMCPDiagnosticScopeUnsupportedPlatformFailsClosed`；Windows 上同一負向測試；整合 B/C/D 後三 OS 的 full CI。跨平台編譯不能證明測試會在各 Hosted Runner 通過。
- 如 Hosted Ubuntu 仍失敗，先核對實際 test name、Fixture 的 Adapter 啟動情況與 runner 權限，不要放寬 runtime `ErrIdentityUnavailable`、移除 negative test 或偽造 Process Identity。
- 本次工作只是移除錯誤的跨平台正向測試期待；**不等於 Linux 擁有可信程序出生身分、Linux MCP Diagnostic 功能已可用，或 Native Thread/MCP Child owner 已被證明**。

## 5. 整合者接手

1. 於 A Worktree 驗證 `git status --short --branch` clean 與 `git log -2 --format='%H %s'`；固定 baseline `b7a26cd9...`、程式碼 Commit `4c810f3a...`；A 文件獨立提交。
2. 在第五線的**獨立 Integration Worktree** 納入 A 的程式碼與 HANDOFF commit，與 B/C/D 整合；A 不做 Push/Merge。
3. 使用 Go1.26.5 重跑 focused tests、vet，再在既有 GitHub PR #1 的 Hosted Ubuntu/macOS/Windows full CI 中核對實際結果。只允許 Hosted Runner 的實際 PASS 解鎖 Hosted Gate。
4. B/C/D 的平台失敗歸各自工作線，不要在 A branch 修別人的檔案；分支與 Worktree 清理由 Gate 之後另一階段處理。

**本工作線未 Push、未 Merge、未修改 CI／遠端、未部署、未清理 Worktree/分支。**
