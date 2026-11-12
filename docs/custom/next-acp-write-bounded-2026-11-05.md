# NEXT A1 — ACP 有界寫入與取消（2026-11-05）

狀態：**source-only 實作與 Host 程式回歸 PASS；未安裝，無 installed ACP live UAT。**

工作樹：`/Users/wei/sideProject/agentdock-next-edge-rc-20261023`；分支：`feature/next-edge-rc-20261023`。本文件記錄 [11/04 Original v1.1.1 選擇性吸收評估](next-original-v111-selective-adoption-2026-11-04.md) 的 **A1** 交付，不將其他候選或 P8／M9 Host gate 視為完成。

後續真實 Codex Session 完成／取消的證據見 [A1 Staged Core Host 驗收](next-a1-staged-core-host-2026-11-05.md)；staged PASS 不代表 installed A1 UAT。

## 使用者流程與最小修正

Codex／Antigravity ACP Session 的 peer 不讀取本端寫出的 JSON-RPC stream 時，既有 `writeMu` + `writer.Write` 可能使 Request 無法取消，另一請求也可能無界等待寫入鎖。本次參考 Original v1.1.1 的取消／有界寫入機制，適配 NEXT 現有 Connection；本 checkout 沒有 `internal/fs/pipeio`，必要平台 helper 限縮在 `internal/acp`，未引入新依賴。

- Request 寫入受 request context／cancel、connection close 與 **production 預設 30 秒**期限約束；一般 notification／response 寫入同樣有界。
- `writeSlot` 取代無法取消的鎖等待。排隊者可依自己的 context 返回，不關閉第一筆仍在寫入的健康連線；預先取消且尚未寫入的請求不碰 writer。
- 部分寫入、short write 或寫入失敗採 fail-closed，避免沿用損壞的 JSON-RPC framing；保留 ACP cancellation／connection error classification。
- 等待回應的 Request 取消後，取消通知在背景以 **1 秒**期限 best-effort 發送，不阻塞已取消的 caller。
- Unix 在取消時關閉 owned writer 以解除 pipe 寫入；Windows 以 `CancelSynchronousIo` 取消該次寫入的特定執行緒，等待 Write 結束才返回，避免遺留寫入破壞後續 framing。

回歸使用真正 `os.Pipe`，涵蓋寫入中取消／到期、排隊者到期後原 frame 完整送達及連線續用、預先取消、partial／short write、connection close 和取消通知阻塞。Steering 測試的 writer 換裝同步方式一併適配。

## 程式修改範圍

| 檔案 | 變更 |
| --- | --- |
| [internal/acp/connection.go](../../internal/acp/connection.go) | context-aware writeSlot、有界寫入、背景取消通知與 terminal connection 處理 |
| [internal/acp/steering_compatibility_test.go](../../internal/acp/steering_compatibility_test.go) | writer 換裝改用同一 writeSlot 同步 |
| [internal/acp/pipe_write_unix.go](../../internal/acp/pipe_write_unix.go)（新增） | Unix 可取消 pipe 寫入與 short-write 檢查 |
| [internal/acp/pipe_write_windows.go](../../internal/acp/pipe_write_windows.go)（新增） | 特定執行緒的同步 pipe I/O 取消與等待結束 |
| [internal/acp/write_timeout_test.go](../../internal/acp/write_timeout_test.go)（新增） | 真實 pipe focused regressions 與預設期限驗證 |

為降低日常測試成本，Connection 私有 `writeTimeout` 預設仍是 **30s**；deadline regression 先斷言此預設值，再僅對測試 Connection 注入 **50ms**。沒有全域可變 timeout 或 production 環境變數設定，也未放鬆取消／fail-closed 語義。

## Mac-Dev Host 驗證證據

以下為上游提供的真實 Mac-Dev Host 結果；本次文件收尾未重跑程式測試。

| 檢查 | 首輪：regression 實等 30 秒 | 後續：regression 注入 50ms |
| --- | --- | --- |
| 定點 Go test | **PASS 0.384s** | — |
| 完整 ACP suite | `go test -count=1 -timeout=180s ./internal/acp`：**PASS 37.182s** | `go test -count=1 -timeout=120s ./internal/acp`：**PASS 6.782s** |
| focused `-race` | **PASS 2.026s** | **PASS 2.178s** |
| `go vet` | **PASS** | **PASS** |
| Windows amd64 `go build` | **PASS** | **PASS，CGO_ENABLED=0** |
| diff check | **PASS** | **PASS** |

完整 suite 的時間是 Go 單套件報告；後續 shell wall time 略久，不應混作相同計時指標。Windows cross-build 只證明編譯通過，不是 Windows runtime pipe 取消 UAT。

## 追加重複 Race 回歸

原有 Host 單次 PASS 後，追加 focused `-race -count=3` 暴露兩處問題；上方已實際發生的單次測試記錄完整保留，以下追加修正與最終 Host 結果。

1. **測試排程假設：** `TestBlockedWriteHonorsRequestContext/deadline` 的 100ms request deadline 在 Race Detector 下可能先於實際 Write 排程到期，出現 `pipe write did not start`。已先準備 payload，將該 case 的 request deadline 改為 **1s**，仍要求收到真正進入 Write 的訊號，再驗證 deadline 返回與 framing fail-closed。另有獨立的私有 **50ms** 寫入期限測試，先斷言 production 預設仍為 **30s**；兩個測試用途不同。
2. **實際錯誤分類競態：** 主動 Close 已發布 connection terminal 狀態，但非同步取消尚未傳播到寫入 context 時，writer.Close 的 I/O 錯誤可能先返回，誤分類為 `ACP_CONNECTION_FAILED`。`internal/acp/connection.go` 現在優先檢查 connection terminal 狀態，回報 `ACP_CONNECTION_CLOSED`；真正獨立寫入失敗仍為 `ACP_CONNECTION_FAILED`。`internal/acp/write_timeout_test.go` 已新增固定重現此時序的回歸，舊碼 overlay 可重現 FAIL，修正後通過；partial-write fail-closed、cancel deadline 與 Windows `CancelSynchronousIo` 語義保持。

最新 Mac-Dev Host 重複 Race 命令：

```sh
go test -race -count=10 -timeout=120s ./internal/acp -run 'Test(BlockedWriteHonorsRequestContext|QueuedWriteTimeoutPreservesActiveWrite|AlreadyCancelledWriteDoesNotTouchHealthyConnection|WriteFailureClosesFramingAndKeepsClassification|CloseReleasesBlockedAndQueuedWrites|CancelledRequestDoesNotWaitForCancelNotification|BlockedNotifyHonorsWriteDeadline)$'
```

以下為上游提供的最終實際 Host 結果；本次文件補充未重跑程式測試或探查服務。

| 檢查 | 最終 Mac-Dev Host 結果 |
| --- | --- |
| 上述 focused `-race -count=10` | **PASS 16.677s** |
| `go test -count=1 -timeout=120s ./internal/acp` | **PASS 7.190s**（Go 單套件報告） |
| `go vet` | **PASS** |
| `CGO_ENABLED=0` cross `go build` | **Windows amd64 PASS；Linux amd64 PASS** |
| `git diff --check` | **PASS** |
| Original `8765`／NEXT `8767` | **皆 HTTP 200** |

以上仍是 source-only 程式驗證與既有服務健康觀察，不代表修正已安裝、installed ACP live UAT 或 Windows／Linux runtime pipe 取消驗收完成；不改變下方 OPEN gates。

## 證據邊界與下一步

**A1 已達本次 source-only 實作／回歸交付；installed ACP live UAT 尚未執行。** 未 install、merge、push 或 release。Original v1.1.1／NEXT RC19 的 App、服務與環境保持不變；沒有 Browser／Edge、全域 auth／Keychain 操作，也未擴張 Native MCP source-only 啟用開關。

Native Codex MCP **可信 operator → M8 → native backend installed Host 仍 OPEN**；**Connection GUI 失敗跨頁與 Tray 身分／實際操作 Host 仍 OPEN**。本次 pipe regression 與 Host Go 驗證不能替代這些 gate，也不代表安裝版 Session 已取得修正。

下一個安全動作由上游審閱這批未提交差異與 Host 證據，決定後續整合及 installed ACP 驗證；維持上述獨立 Host gate 的 OPEN 狀態。
