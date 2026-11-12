# NEXT Native Codex MCP Session 失敗回收／並行隔離（2026-10-25，Asia/Taipei）

## 實際使用痛點與本輪目標

在 NEXT source-only B0–B6 instrumented Codex ACP path，`InstrumentedSessionProvider.Servers`
會先請既有 Host MCP provider 配置一筆 Session 資源，再確認新的 instrumented stdio MCP
名稱是否與 Host 原有名稱衝突。原先在衝突時直接拒絕但未釋放剛配置的 Host Session
資源；若使用者重試，可能留下未被報告的邏輯租約。本輪只修正**失敗時確實回收、
回收失敗時保留可追蹤的未結狀態，以及其他 Session 不被阻塞**，並不將 Native Codex
MCP 打開給正式使用者。

## Source 增量

- `internal/acp/next_instrumented_session_mcp.go`：衝突時只對首次配置的該
  Session 執行底層 `ReleaseSession`；用與取消的原請求脫鉤但最多 35 秒的
  context 進行**有界**回收。失敗時 `errors.Join` 呈報原始原因、保留
  cleanup-pending 計數，拒絕同一 Session 在未處理的狀態下重新配置。
  已接受的 Session 遇到異常重試，不誤釋其既有 Host 資源。
- 最初修補曾使用覆蓋全部 Session 的 mutex 執行底層 I/O，會造成不相關的
  Session 受到最多 35 秒的阻塞。整合審查已要求改用**按 Session 分離、
  可取消等待、空閒立即移除**的 gate；全域 mutex 只保護計數與 map，
  不包住外部配置或回收 I/O。
- `internal/acp/session_mcp.go`：準備流程失敗後，也重設過期的
  `sessionMCPReleased` bookkeeping，避免先前釋放記錄掩蓋新的未結清理義務。
- `internal/acp/next_instrumented_session_mcp_test.go`：回歸覆蓋衝突回收、
  取消後仍可有界回收、底層回收失敗及再次重試、已接受 Session 保護，
  並以阻塞的 A Session 對照仍可配置和釋放的 B Session，分別驗證配置、
  rollback、正常釋放三種情況。未新增 native PID kill、跨 Session 強制釋放、
  更改官方 Codex CLI 或憑證拷貝。

## 實際驗證

- 原始碼測試原版下可重現衝突未呼叫 release 的缺陷；修正後
  `go test ./internal/acp -run '^(TestInstrumentedMCP|TestB5InstrumentedMCP|TestB5SourceOnlyMCP)' -count=1 -timeout=60s`
  **14 tests/subtests PASS**；相同選擇 `go test -race` **14 PASS**。
- Orchestrator 獨立執行完整 `go test -count=1 -timeout=180s ./internal/acp`、
  `go vet ./internal/acp` PASS；關聯 `./internal/app ./internal/tool/acp
  ./internal/permission ./internal/mcp/launchregistry` 的 `go test -count=1`、
  `go vet` **PASS**；`git diff --check` PASS。
- 在同次檢查中 `127.0.0.1:8767/healthz` 回報 NEXT
  `0.9.2-rc.19`，`127.0.0.1:8765/healthz` 回報 Stable
  `1.1.0`。這輪沒有重新建置或安裝 NEXT，不重啟兩個 Core，
  不建立 Edge 授權連線、沒有登入 Native Codex、沒有新增 Native MCP。
- NEXT 安裝版透過自己的 Computer Broker 唯讀觀察：GUI PID 75194、
  Main Window、AX 46、Chrome 前景未變、session release complete；
  這是 GUI **唯讀健康證據**，不是 Connection 失敗操作或 Tray 點擊 PASS。

## 尚未完成的正式發布門檻（不得偷換）

1. **Connection GUI Host OPEN**：已安裝 RC19 存在操作錯誤保留修正，
   HEAD 更新的 Core 失聯 fail-closed 仍屬 source-only。尚未有在不影響既有
   Tunnel／Port／服務的情況下，真實拒絕一筆 GUI 操作再跨 Overview 與
   Connection 查驗的證據。不可用 mock、外部 API 呼叫或單純 AX46 冒稱通過。
2. **Tray/MenuBar Host OPEN**：已有 source 身分標籤，macOS 系統 AX
   proxy 仍無可信 NEXT 標示；不得只憑座標操作相鄰 Stable 項目。
   可接受的下一步是有明確身份辨識且可接受短暫前景控制時，僅驗證無害
   Open/Show 動作，不先操作 Start/Stop/Restart。
3. **Native Codex MCP production OPEN**：本次只是 B0–B6 已驗證隔離方案的
   source-only 失敗路徑修補。可信任的 Desktop 前端操作者權限／M8 接入、
   跨 Core permission 原子協調、native 同 boot orphan／未結 journal
   安全恢復、完整已安裝 ACP Session→MCP→close Host 證據及 M9 sign-off
   均需獨立驗證。回收失敗會真實留在 pending，不可視為已清乾淨。

## 安全與交接

遵守 `AGENTS.md` 的 Stable／NEXT 分離與不干擾原則；沒有
改 `~/.codex`、`~/.agentdock`、原版 App、Edge Profile 或正式
Next Core。保留已通過的 RC16 Browser、RC18 rollback、RC19 GUI
Host 歷史證據，避免反覆跑使用者授權。**不合併 `custom/main`、
不 push、tag、發布，也不將此 source-only 改善稱為 B6 production PASS。**
詳見 [Connection fail-closed](next-connection-failclosed-2026-10-25.md)、
[RC19 Tray AX 實機對照](next-rc19-tray-ax-contrast-2026-10-24.md)、
[B0–B6 Closeout](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。
