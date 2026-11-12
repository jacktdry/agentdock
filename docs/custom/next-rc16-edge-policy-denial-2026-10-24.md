# NEXT RC16 source checkpoint — 公司 Edge 負向權限拒絕與租約保全（2026-10-24）

> **2026-10-24 13:23 後續正式安裝／Host 結果：** [RC16 Host 驗收完整記錄](next-rc16-host-uat-2026-10-24.md)。RC16 NEXT-only 安裝 PASS；2/2 獨立 JS 與越界 URL `ACTION_INVALID`、拒絕後正常導覽／AX、3/3 clean release、第三 fresh acquire、單次 connect／正式 disconnect、134 Chrome foreground samples、0 orphan／0 held socket 均 **真實 Host PASS，整輪 `ok=true`**；原版 8765 仍是 1.1.0、RC15 rollback 已保留。**下文「尚未安裝／待 Host」為候選階段歷史，請以此最新記錄為準；GUI／Tray、crash/cancel、live rollback 仍 OPEN。**

**狀態（2026-10-24 13:09）：source 與 RC16 本機候選封裝均 PASS；RC16 尚未安裝或執行真實 Edge Host UAT。** 目前安裝仍為 RC15 `0.9.2-rc.15`，Stable 8765 1.1.0 未更動，release_ready=false。

## 產品工作流程與 RC15 Host 剩餘差異

[RC15 真實 Host UAT](next-rc15-host-uat-2026-10-24.md) 已證明正式一次 Edge 連線、Oprueba／HomiPlay A/B 互不干擾、全部 release 後 fresh-acquire、Oprueba 導覽／AX／有效 PNG、網站 no-focus **PASS**。但同一輪使用一筆隔離租約測 `direct_evaluate(() => 1)`，雖然沒有執行 JS，回覆卻是 `BROWSER_REQUIRED_ROUTE_UNAVAILABLE: external transport fence unavailable or mismatched`，且該 lease release cleanup_error 不為空，整輪 `ok=false`，未核准 URL 負向因此沒有執行。**不能把泛化傳輸故障當作可辨識的權限拒絕 PASS。**

## 最小 source 修正

- 原版 `ACPBridge.DirectCall` 在核對使用者身份與 lease ownership 後，會把所有支援語法的 direct tools 送入 `ExternalLeaseManager.Call`；`nativeEdgeWorker.call` 才拒絕 JS／超出 origin。當 `externalFenceSeries` 看到操作錯誤時會封閉 series 並泛化為 transport error，令已核准、健康的租約也受到影響。
- 只對**外部公司 Edge**的 direct 操作，在完成 `directScope`／`directLease` 所有者檢查後、建立 fenced CDP 操作**之前**先拒絕 `evaluate_script`，回 `ACTION_INVALID`。對 `navigate_page` 先由**可信 workspace policy**的 `RoutePlanner.Plan` 檢查原生 route 與 `OriginScope.Allows`，若 URL 不在核准來源內則回 `ACTION_INVALID`。拒絕不會產生 CDP 封包、不關閉共享 socket、不退休合法 lease。
- Managed Chrome 的 `evaluate_script` 保持原樣，仍是合法功能；跨 principal、租約所有權與其他 M8 邊界仍先執行，未越權回傳網站資訊；native 的其餘操作仍照舊經多段驗證後執行。
- 新增 `TestNativeEdgeDirectPolicyDenialPreservesLease`：在真實 local WebSocket 的 fixture 中，先讓兩項負向測試於舊版 source **失敗**（收到 transport mismatch、release 出現 cleanup_error），再用新程式驗證兩者明確 `ACTION_INVALID`、**0 額外 CDP messages**、共用 transport/worker 未損壞、沒有重連、後續正常 lease release `CleanupComplete` 且無錯誤。
- 四套件 Go 回歸 **1,706 PASS／7 opt-in SKIP／0 FAIL**，定點 Race Detector **56 PASS／0 FAIL**，`go vet` 和 `git diff --check` PASS。

## 仍需驗證／不做

### 固定 RC16 本機候選已完成（尚未部署）

- Branch `feature/next-edge-rc-20261023`、source commit `d51f1bc4e327a7d198a6e8b91c1aff7e5e849020`、Core `0.9.2-rc.16`、bundle `17.0.0`，canonical manifest `/private/tmp/agentdock-next-edge-verification-y6b6836m/next-rc16-candidate.json`（`release_ready=false`）。
- App、ZIP、DMG 位於 `dist/next-shared-arm64.pQhkiC/package/`，macOS arm64 App／Core／Vue typecheck／Vite production build／Core Skills／9 個 package tests 全 PASS。App `codesign --verify --deep --strict` PASS，DMG checksum VALID。
- 候選 App／ZIP／DMG **24 個 regular files SHA-256 全部相同**，唯讀 DMG 驗證後已卸載；ZIP SHA-256 `90ba217cf67f1ab2ebf6f9487137ce9568436e7046886f5f6b986a906683595c`，DMG SHA-256 `720ecb8aab80839d16dc74f7740ecf6c36aaab0a4a8431184bc6b66884c5adb8`。
- 有界下一步的 Host 腳本已準備在私人測試目錄 `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc16-negative-host.py`，但**沒有執行**；它只會進行一次正式 connect，在兩筆獨立租約上分別驗證 JS／未核准 URL 明確拒絕、拒絕後正常導覽／AX／無 cleanup_error、最後新租約／正式 disconnect／no-focus。

安裝版 RC15 尚無上述修正，**RC15 的 negative gate 仍 OPEN**。RC16 已完成固定 HEAD 的 manifest、macOS App/ZIP/DMG 簽章，但只在 NEXT-only 閒置且不干擾使用者時才能部署，透過**一條**正式使用者同意的 Edge 連線核對 JavaScript 與未核准 URL 各自明確拒絕、合法租約可繼續操作／清理、沒有 focus steal 或殘留，才能認列 Host PASS。沒有實機 UAT 不合併、push、tag 或發布。

不更動 Stable 8765、現有 Edge/公司 Profile、OAuth grants、Managed Chrome 權限、不新增 daemon、不反覆要求原生偵錯同意；其他 GUI／Tray、跨 session crash/cancel cleanup 與故障 rollback gates 維持 OPEN。
