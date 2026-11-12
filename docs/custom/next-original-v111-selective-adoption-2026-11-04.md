# AgentDock Original v1.1.1 → NEXT 選擇性整合待辦（2026-11-04）

> **僅規劃／比較；未在 NEXT 實作、打包或部署。** 原版 Mac-Dev MCP 目前回報 custom **v1.1.1 Build 72**（merge `01ef1157`，官方 release commit `d3d05391`）。NEXT 正式安裝版仍 **0.9.2-rc.19**／Core source `138c6cb0`（11/02 單獨升級 cloudflared 到 `2026.10.7`）；開發樹 `feature/next-edge-rc-20261023` HEAD `4af20cdb`，未整合 `custom/main`。版本不同，不可以 source-only 證據冒稱安裝版驗收。

## NEXT 進度與現有優先門檻

**11/05 A1 後續證據：** [source 實作與真實 pipe 回歸](next-acp-write-bounded-2026-11-05.md)、[Staged Core Host Session 完成／取消驗收](next-a1-staged-core-host-2026-11-05.md)。下方保留 11/04 選案時的比較；A1 尚未安裝，不代表 installed UAT 或 release gate 完成。

- **服務／控制面：** Original `8765` 與 NEXT `8767` 本機 HTTP 200；本次由 Original Mac-Dev 連線盤點，沒有重啟、更新、重新授權或改動任一服務／Browser。
- **已取得實機證據：** P8 三個真實 Actor 共用背景 Chrome 曾於 10/14 Host PASS；RC16 已證明公司 Edge 的指定路由、拒絕與 fresh-acquire；RC18 各別證明 ACP 取消／Adapter 異常後新工作、實際 launchd failed-registrar rollback；RC19 Shared GUI 建置、安裝、無搶焦點觀察通過。**這些證據跨版本、跨情境，不代表 RC19 所有工作流已完成。**
- **仍 OPEN，優先於一般 upstream 移植：** RC19 的 Tray `AXExtrasMenuBar` 尚無可靠的 NEXT 品牌辨識及真正選單操作 Host UAT；Connection 失敗後跨頁顯示已 source PASS、缺真實 GUI 點擊／失敗注入 Host；Native Codex MCP signed catalog/launcher/Core ID-only/M8/peer provenance 目前為 source-only，可信 Desktop operator→M8 授權→真實 Adapter/Backend 的完整 Host 尚未達成；Windows/WSL/Linux parity、完整 updater/release/M9 gate 未關。NexusDock 繼續延後。
- **非本次變更：** `docs/custom/pre-m9-progress-2026-10-09.md` 既有未提交修改維持不動；原版／NEXT live 不動；不發新 RC、沒有合併／push。

## 原版 1.1.1 值得吸收的具體候選

| 代碼／排序 | NEXT 與 Original 差距 | 後續行動及可觀察驗收 |
| --- | --- | --- |
| **A1｜高價值可靠性候選** | **ACP 阻塞寫入超時／取消。** Original `internal/acp/connection.go` 使用 `writeSlot`、`writeMessageContext`、`pipeio.Write`，綁定 request/connection cancellation、30 秒上限，partial write 將 connection fail-closed；NEXT 仍是 `sync.Mutex` + `writer.Write`，等待鎖與 pipe 寫入可能無界阻塞。 | 在 NEXT 現有 lifecycle／ownership 上選擇性補此機制。先重現 blocked writer + 第二位等待者／cancel，再驗證 timeout、partial-write 關閉、unrelated Session 不受影響，含 `-race`。參考 Original `internal/acp/write_timeout_test.go`。**是已驗證的 source gap，非已確認的現役故障；不得擠掉上方 Host gate。** |
| **A2｜Windows/M9 平台 gate** | **Named Tunnel 已啟動卻斷線的復原。** Original 新增 `internal/desktopruntime/tunnel_health.go`、`tunnel_monitor_windows.go`、`tunnel_network_windows.go`：child `/ready`／metrics owner、DNS/VPN 恢復、有界 retry、stale supervisor 代數防護，Origin 失敗不應破壞健康 Core；NEXT 沒有這幾個 monitor 檔，已有自身 `tunnel_windows.go` supervisor。 | 只借鑑 health/reconnect 機制，先查 NEXT ownership/lease，勿整包覆蓋。Windows 實測斷網／VPN 改變→復原、健康 Core 不中斷、手動 Stop 中斷 retry、過期代次不覆寫狀態。參考 Original `tunnel_health_test.go` 與 `tunnel_monitor_windows_test.go`。 |
| **A3｜Windows 更新／Unix 回滾候選** | **保留原啟停狀態與 payload stage 失敗回滾。** Original 的 Windows 升級只恢復原本 running 的 Tunnel，並新增 `internal/installer/payload_rollback_test.go`；NEXT 自有 NEXT-only 安裝和 signed/atomic rollback，不可退回原版。 | 逐項比對現有 Installer：原 stopped 不自啟、原 running 恢復、stage failure 交還舊版本；在 Windows Host／CI 實證。RC18 **已實機 PASS 的** NEXT macOS registrar-failure rollback 不重複造測試。 |
| **A4｜MCP 安全測試缺口盤點** | Original 新增 OAuth Clear/exchange/refresh 撤銷、expired session recovery、MCP redirect 不外送 header 等 regression（`internal/mcp/client/recovery_security_test.go`、`internal/mcp/oauthclient/revocation_test.go`）。NEXT **已有** OAuth generation＋disk epoch、`sameHTTPOrigin`＋`CheckRedirect` 和 M8 admission，不能說缺少整套防禦。 | 逐一對照 NEXT 既有測試，只補實際缺的 case；驗證 Clear 後 in-flight 結果不可重建授權、跨來源跳轉不傳 secrets、壞 session 不阻斷下一次 call。**禁止直接覆蓋 NEXT manager／OAuth policy。** |
| **A5｜可延後的維護去重** | **JSON Schema 單一 validator。** Original `internal/tool/contract/validator.go` 已使用 `google/jsonschema-go`；NEXT 還用 `santhosh-tekuri/jsonschema/v6`，`go.mod` 同時有兩套。 | 先盤點真實 Draft 需求和 JSON-path/required 誤差語義；若收益明確再遷移，維持 Draft-07／2020-12、明確拒絕未支援的 Draft-04／06／2019-09，做 MCP contract tests、Go/race/vet、跨平台驗證才移除依賴。 |
| **A6｜有缺口才做的低優先安全回歸** | Original 增補 command initial-stdin、公開 Artifact symlink／filename、Linux cloudflared permission 回歸；NEXT 沒有相同檔名的測試，**不代表必然沒有防護**。 | 先比對 NEXT 已有測試與使用場景，選擇性補失敗重現或平台覆蓋。不得順手增加新的工具、service 或廣泛測試框架。 |

## 明確不自動移植／已存在機制

1. **Task 卡片/P13/P14：** Original v1.1.1 改為 `task_create` 掛 Task 卡，`task_manage` 保留後續生命週期；NEXT 卡片目前仍掛 `task_manage`。只記錄為相容性差異，不自動引入 polling iframe 或 persistent monitor。使用者 [2026-10-22 決議](p13-p14-ux-scope-decision-2026-10-22.md) 明確選原版精簡、可折疊 Task 卡；除非看到 NEXT 真實重複卡片／資訊遺失，否則 P13/P14 不列為主動開發。
2. **NexusDock/Resource Library：** Original 的雲端備份可能包含 `.env`、credentials、keys 等敏感內容；沒有明確使用者同意、資料分類及加密策略前，不在 NEXT 預設上傳或開啟。NexusDock 仍「開發中」。
3. **整包 UI／service 移植：** NEXT 使用 Shared Vue/Wails、獨立 state/Broker/M8，Original AppKit／Windows WPF／Tunnels 不能當作可直接 merge 的替代實作。避免退化現有 Connection fail-closed／epoch／rollback。
4. **Cloudflared 版本：** 上游目錄 `2026.10.4` 不是現役元件升級理由；本機 Original 與 NEXT 已為 `2026.10.7`，不得倒退。

**下一個工程工作順序：** 先真實 Native MCP operator/Host、Connection 失敗跨頁、Tray 身分與操作；A1 適合獨立小修正及回歸；A2/A3 進 Windows／M9 gate；A4 只補證實的 coverage；A5/A6 待確定效益。**本文件只有待辦評估，不表示已修復／已部署。**
