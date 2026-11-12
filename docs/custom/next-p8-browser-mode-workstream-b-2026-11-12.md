# NEXT P8 — 工作線 B：安全的 Browser 共享模式設定

日期：2026-11-12。此文件描述 **Source-only 實作**，不是已安裝的 NEXT 產品功能或正式 Release 驗收。B 從 RC21 Source `0a0a0ba98412c6f22f8793a6abd184d6db62a27e` 建獨立 worktree；A 才有權將其導入整合基準。

## 痛點與最小範圍

- 2026-10-14 ChatGPT/Codex/Antigravity 三個真實 Actor 共享一個 Chrome 的 Broker/Lease/Page 驗收**已 PASS**，這裡不重新實作 Broker、不重新跑三 Actor 合成矩陣。
- 操作者以前要直接編輯 NEXT 專屬 `agentdock.env` 中的 `AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE`；錯誤 workspace 或錯誤狀態可能造成下次啟動失敗，並且修改設定後不容易分辨「已設定」與「已生效」。
- 最小改進：NEXT-only `agentdock browser-mode status|enable|disable`，只允許正式的使用者 NEXT app runtime root；預設仍為 exclusive，enable 只選擇現有的 **NEXT 預設 canonical workspace**。既有 Browser Desktop 頁面繼續顯示實際運行模式與 pool 狀態，不修改共用 Desktop API、Settings 或 i18n。

## 產品操作

以下命令在**包含此 Commit 的 NEXT Helper 日後安裝後**才能操作；目前 Source-only 不得拿現役 Helper 或 Core 來驗收切換：

```sh
NEXT_HELPER="$HOME/Applications/AgentDock Next.app/Contents/Helpers/agentdock"
NEXT_RUNTIME="$HOME/Library/Application Support/AgentDock Next"

AGENTDOCK_DESKTOP_VARIANT=next "$NEXT_HELPER" browser-mode status --runtime-root "$NEXT_RUNTIME"
AGENTDOCK_DESKTOP_VARIANT=next "$NEXT_HELPER" browser-mode enable --runtime-root "$NEXT_RUNTIME"
AGENTDOCK_DESKTOP_VARIANT=next "$NEXT_HELPER" browser-mode disable --runtime-root "$NEXT_RUNTIME"
```

- `status`：只輸出 allowlisted JSON：`configuredMode`、`effectiveMode`、`restartRequired`、`guardPresent`、`changed`、`message`。只有讀到**已驗證的 NEXT Core Unix socket** Browser snapshot，才會報告 live `effectiveMode`；否則是 `not_observed`，不得視為 runtime 已生效。失敗訊息不輸出密碼、Token 或環境檔內容。
- `enable`：預設路徑 `$HOME/AgentDock Next`（或 NEXT env 設定的 `AGENTDOCK_DEFAULT_DIR`），必須存在、絕對且 canonical；明確拒絕其他不合格 workspace 及非 8767 Port。只對 NEXT private `agentdock.env` 的這一個 key 持久設定。
- `disable`：從 NEXT private `agentdock.env` 移除 opt-in key，下次正常啟動恢復原本 exclusive，不會自動將現役共享的 Session 改成 exclusive。
- 所有非此 key 的行、憑據、註解和順序保持原樣；解析不可信、重複/複雜 key、錯誤 runtime/簽章身分、非私有環境檔均拒絕。透過既有跨進程 Desktop mutation lock + atomic file 進行單一寫入。

**明確採用不打斷工作者的「下次正常啟動生效」語義**：這個指令不重啟 Core，不觸碰 Edge/Chrome Profile，不清除 marker，不釋放或停止 Lease，不推送任何部署。使用者須等待其他 Session/Lease 完成，依現有 NEXT 正常服務管理方式重新啟動；不能把 `changed=true` 解讀為立即生效。若涉及 guard，先等待正常已驗證的關閉或依既有程序人工排查，禁止手動刪除 marker。

## Restart guard 防繞過

原本的 guard 只在 shared opt-in startup 時檢查。現在在 **NEXT-only exclusive startup** 也檢查 `~/.agentdock-next/.shared-browser-restart-pending.v1`；若 marker 存在或狀態無法確定，拒絕建立新的 exclusive Bridge。因為當共享模式已被暫存為 disable、但舊 shared Core 意外 crash 留下 marker 時，原本 exclusive 路徑可能繞過共享 Chrome orphan fence。stable AgentDock 與未設 NEXT variant 的 runtime 保持既有行為。

## 證據邊界與合流

- **本機 SOURCE-only 定點 PASS**：`go test -p 1 ./internal/desktopruntime ./internal/app ./cmd/agentdock -run '^(TestNextBrowserMode|TestRuntimeACPBrowserExclusiveRejectsSurvivingSharedGuard|TestRuntimeACPBrowserNextSharedWorkspaceOptIn|TestRuntimeACPBrowserNextSharedOptInDenials|TestRunPrintsVersion)' -count=1 -timeout=65s`，三套件 PASS（日期：2026-11-12）。涵蓋 Next 私有設定往返、no-op、複製/非法 key 拒絕、guard 保留、exclusive bypass 拒絕、既有 opt-in 正負測試；沒有啟動 Chrome/Edge 或 Actor。
- `go vet -p 2 ./internal/desktopruntime ./internal/app ./cmd/agentdock` PASS；`git diff --check` PASS；涉及 Go source 的 `gofmt` PASS。未改 Vue，無需新增 Vue 測試；三 Actor PASS 沿用先前實機證據，**不是此次新的實機驗收**。
- 此線不對現役 NEXT/Original 服務執行 restart、deploy、profile 實測、Push、Merge、Tag 或 Release。
- 仍存在的產品限制：**不是 GUI 一鍵切換**；目前提供 CLI 的安全、可理解的選擇與真實模式查詢，並刻意不提供可能中斷其他 Actor 的 `apply-now`。若將來要求 GUI 操作，須與 C 協調共用 Desktop API/i18n 寫入所有權，並由 A 依序整合。
- A 收件方式：待 B Commit 完成，從 `feature/next-p8-browser-mode-switch-20261112` 非破壞性 cherry-pick 到 A 的統一基準，先確認是否與 C 的 Desktop shared files 衝突；本線沒有修改共用 Desktop API/i18n。
