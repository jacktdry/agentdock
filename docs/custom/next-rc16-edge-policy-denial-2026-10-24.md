# NEXT RC16 source checkpoint — 公司 Edge 負向權限拒絕與租約保全（2026-10-24）

**狀態：source 修正 PASS，RC16 尚未 build／install；目前安裝仍為 RC15 `0.9.2-rc.15`，Stable 8765 1.1.0 未更動。**

## 產品工作流程與 RC15 Host 剩餘差異

[RC15 真實 Host UAT](next-rc15-host-uat-2026-10-24.md) 已證明正式一次 Edge 連線、Oprueba／HomiPlay A/B 互不干擾、全部 release 後 fresh-acquire、Oprueba 導覽／AX／有效 PNG、網站 no-focus **PASS**。但同一輪使用一筆隔離租約測 `direct_evaluate(() => 1)`，雖然沒有執行 JS，回覆卻是 `BROWSER_REQUIRED_ROUTE_UNAVAILABLE: external transport fence unavailable or mismatched`，且該 lease release cleanup_error 不為空，整輪 `ok=false`，未核准 URL 負向因此沒有執行。**不能把泛化傳輸故障當作可辨識的權限拒絕 PASS。**

## 最小 source 修正

- 原版 `ACPBridge.DirectCall` 在核對使用者身份與 lease ownership 後，會把所有支援語法的 direct tools 送入 `ExternalLeaseManager.Call`；`nativeEdgeWorker.call` 才拒絕 JS／超出 origin。當 `externalFenceSeries` 看到操作錯誤時會封閉 series 並泛化為 transport error，令已核准、健康的租約也受到影響。
- 只對**外部公司 Edge**的 direct 操作，在完成 `directScope`／`directLease` 所有者檢查後、建立 fenced CDP 操作**之前**先拒絕 `evaluate_script`，回 `ACTION_INVALID`。對 `navigate_page` 先由**可信 workspace policy**的 `RoutePlanner.Plan` 檢查原生 route 與 `OriginScope.Allows`，若 URL 不在核准來源內則回 `ACTION_INVALID`。拒絕不會產生 CDP 封包、不關閉共享 socket、不退休合法 lease。
- Managed Chrome 的 `evaluate_script` 保持原樣，仍是合法功能；跨 principal、租約所有權與其他 M8 邊界仍先執行，未越權回傳網站資訊；native 的其餘操作仍照舊經多段驗證後執行。
- 新增 `TestNativeEdgeDirectPolicyDenialPreservesLease`：在真實 local WebSocket 的 fixture 中，先讓兩項負向測試於舊版 source **失敗**（收到 transport mismatch、release 出現 cleanup_error），再用新程式驗證兩者明確 `ACTION_INVALID`、**0 額外 CDP messages**、共用 transport/worker 未損壞、沒有重連、後續正常 lease release `CleanupComplete` 且無錯誤。
- 四套件 Go 回歸 **1,706 PASS／7 opt-in SKIP／0 FAIL**，定點 Race Detector **56 PASS／0 FAIL**，`go vet` 和 `git diff --check` PASS。

## 仍需驗證／不做

安裝版 RC15 尚無上述修正，**RC15 的 negative gate 仍 OPEN**。下個真正版本需先完成固定 HEAD 的 manifest、macOS App/ZIP/DMG 簽章，再在 NEXT-only 閒置且不干擾使用者時部署，透過**一條**正式使用者同意的 Edge 連線核對 JavaScript 與未核准 URL 各自明確拒絕、合法租約可繼續操作／清理、沒有 focus steal 或殘留，才能認列 Host PASS。沒有實機 UAT 不合併、push、tag 或發布。

不更動 Stable 8765、現有 Edge/公司 Profile、OAuth grants、Managed Chrome 權限、不新增 daemon、不反覆要求原生偵錯同意；其他 GUI／Tray、跨 session crash/cancel cleanup 與故障 rollback gates 維持 OPEN。
