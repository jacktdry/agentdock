# AgentDock NEXT RC20 — H04／H01／H02 根因診斷交接（2026-11-11）

> **目的：**在不使用 Codex ACP、不中斷使用者正在使用的桌面、Original v1.1.1 與已安裝 NEXT RC20 的前提下，對 Stage5 的三個觀測缺口提出**真正的根因、重現方式與最小修正／Host 驗收界線**，不再因錯誤觀察去重構正常的 Wails GUI 或無限擴大回歸沙盒。本次任務是**完成診斷**，不是聲稱產品修正已部署。所有來源偵查、Go instrumented overlays、macOS 截圖／AX 均透過 Mac-Dev 在本機完成；沒有 ACP 委派、GUI 點擊、MAC TCC 授權修改、任一產品分支程式修改、Push／Merge／Tag／部署。

## 共同安全基線

- signed RC20 source worktree：`/Users/wei/sideProject/agentdock-next-a1-rc20-20261105`，HEAD `704bb40d08f6144513791a9e1854cd2c9a799b73`，**clean**。 installed App `~/Applications/AgentDock Next.app` Core `0.9.2-rc.20`／`704bb40d08f6`，macOS GUI PID `14144`（只作本次觀測）；Original v1.1.1 `8765`、NEXT `8767`，均 HTTP 200。
- 所有 A1 診斷程式以 **`go test -overlay`** 僅將 RC20 `internal/acp/write_timeout_test.go` *虛擬覆蓋*成隔離目錄 `dist/h04-diagnostic-20261111/*.go`。檔案位於 **另一個工作樹的 gitignored dist**，不是任何 Git source／signed package／安裝程式。包含更多量測、診斷性 timeout 及大小替換的 overlay **不是產品修正，也不能冒充原版 `-race` PASS**。
- Original／NEXT 的服務/使用者憑證/Browser Profile 不調整，沒有任何任務自有 Computer 或 ACP session 要回收。觀測期間使用者前景程序有 Chrome、Original、Codex；沒有偷切到 NEXT。

## H04 — ACP Race queued-write 確切性質（診斷完成，來源修復 OPEN）

**原始問題／真實驗收：**A1 的 `internal/acp/write_timeout_test.go::TestQueuedWriteTimeoutPreservesActiveWrite` 使用 1 MiB `large` 通知與 OS pipe，先等「Write 開始」，再發一筆 50ms 會取消的 queued Request，最後啟動 peer JSON decoder 排空 pipe，要求只收到 `large` 與 `healthy`，且 Connection 沒被錯誤關閉。測試 helper `awaitACPWriteResult()` 使用**硬性 1 秒**完成上限；RC20 source `-race -count=3` **3/3 FAIL** 在 `write_timeout_test.go:145`，其他 6 項原測試 race `-count=2` PASS；非 Race `-count=3` PASS。現象不是已證明的 production deadlock。

### Source-preserving 時序實測

| 診斷 Overlay | 實際結果 | 對根因的意義 |
| --- | --- | --- |
| 1 MiB、原本 1 秒 watchdog、Race，增加臨時 Log | first Write 開始 `t≈0.229s`，queued cancel `t≈0.350s`、peer decoder 啟動 `t≈0.354s`，到 `t≈1.35s` watchdog FAIL；測試 cleanup 後 decoder 才看到 pipe close | **Write 已開始但讀取/排空耗時超過 1 秒**，不能用超時直接宣稱僵死 |
| 1 MiB、**診斷用** 15 秒 watchdog、Race `-count=2` | **2/2 PASS**，first active frame 寫完 `0.217s／4.009s`，decoder 完整收到 `large` 與 `healthy` `0.363s／4.189s` | 真實 frame 邊界與 Connection reuse 有成功證據，長訊息在 Race instrumentation 下完成時間有大幅波動 |
| 128 KiB、Race、診斷延長窗口 `-count=2` | **2/2 PASS**，frame 寫完約 `0.178～0.212s`，兩筆訊息可解碼 `0.229～0.265s` | 能保留背壓測試用途，避免把 race instrumentation 的 1 MiB 巨量 read/decode 當作 1 秒 correctness requirement |
| 16 KiB、Race `-count=2` | **2/2 FAIL**：原先的 `large` 能在 peer drain 之前完成，取消中的 `queued` request 實際送出，decode 收到 `queued` 而非 `healthy` | 資料太小會**破壞「第一筆被 OS Pipe 阻塞」的測試前提**，不能只縮到任意小尺寸來刷綠燈 |
| 128 KiB、**原 1 秒 watchdog**，額外確認 first active 通知在 queuing 前及 cancel 後仍阻塞、Race `-count=10` | **10/10 PASS**，耗時約 `0.21～0.75s`／次，原 frame 完整性與後續 `healthy` 測試仍在 | 有**不改產品**、保留語意的測試同步/負載修復候選。overlay `h04-128k-watched-original-1s.json` |
| 全 7 個 focused A1 測試、上述 queued fixture-only 改動、Race `-count=10` | 一次 **FAIL**：另一個 `TestBlockedWriteHonorsRequestContext/deadline` 的 1 秒 deadline 偶發先於真正 Write 發生，回 `pipe write did not start` | 原 suite **第二處測試時序弱點**，不能僅修 queued test 就把全部 A1 Race 結案 |
| 完整 7 個 focused A1、兩處 fixture payload 都以 128 KiB、queued 加明確阻塞斷言，保留原本 deadline/watchdog、Race `-count=10` | **PASS（七個 top-level Test × 十次）**，Go suite `17.387s`，RC20 source 完全不變。overlay `h04-a1-two-scheduling-fixtures.json` | 更強的 source-only 診斷：兩個測試的巨型 1 MiB payload 造成 Race 下高排程/解碼壓力，並與短 deadline 耦合；可以只針對測試 fixture 做最小修正，無需憑此重構 production ACP Pipe |

首次診斷曾把 `t.Logf` 放在測試 goroutine，在故意保留短 watchdog 的失敗清理後觸發一次 `Log in goroutine after Test ... completed` 的 panic；**該 panic 是本輪臨時儀表寫法引入，不存在於 signed RC20 source**，後續 overlay 已去除跨測試結束的 goroutine logging。

### 根因判定及修復邊界

- **Root cause（測試可重現層，High confidence）：**在 `-race` 下用 1 MiB JSON frame 和 OS pipe 驗證 cancellation/queued frame 是否保真，但以測試內**固定 1 秒**的 `awaitACPWrite`／`awaitACPWriteResult` 及 Request Context deadline 當 correctness window，且原測試沒有獨立驗證 active frame **持續阻塞**。高負載/長 frame 可能先在 `json.Marshal`／pipe drain／Race decode 消耗時間，讓測試於安全行為未失敗時先誤判。即使 CPU 空閒也能重現，因此不應全歸因於外部 Go cross-build 負載。
- **Product runtime（未證實錯誤）：**所有 Race 診斷中的成功路徑保持 `large`、`healthy` 完整順序與 Connection reuse；非 Race 原測試 PASS。沒有針對 Windows Native Pipe 實機量測，所以**不能保證 production 大型寫入在所有 OS 都不卡**，但目前沒有理由把下一版大規模 ACP 架構重寫排進 P0。
- **最小待派工修正（H04）：**只改 `internal/acp/write_timeout_test.go` 的兩個測試資料量與可觀測阻塞同步（例如以 128 KiB 啟動 OS Pipe 實際堵塞，明確檢查 active first Notify 在排空前仍 pending，保留 50ms queued cancel、1s watchdog，完整 JSON frame + healthy second Notify 斷言）；視不同 OS 的 pipe buffer 進一步確認 128 KiB 足以造成阻塞，若不夠則明確 gate/握手，而非依大小猜測。**不得只把 deadline 延至 15 秒或刪除負向斷言**。提交後跑所有 7 個 focused Race `-count=10`、`go test -race ./internal/acp` 和 Windows 原生 UAT（若屬未來出貨範圍）；原 RC20 測試仍 FAIL，直到實際 source 修正與驗證。

## H01 — Wails GUI 空白：擷取與觀察方式造成的假象（根因診斷完成，真實 mutation Host OPEN）

- 已安裝 NEXT Wails v3.0.0-beta.27 GUI PID `14144`，`CGWindowListCopyWindowInfo` 回報 Window ID `117813`、1146×948、`kCGWindowIsOnscreen=1`、app 非 hidden，User foreground 不是 NEXT。先前 AgentDock Computer Broker ScreenCaptureKit 擷取顯示標題列下全黑、AX 工具只印 5 個元素，讓人懷疑 Wails WebView 空白。
- **獨立 macOS window-only 擷取驗證**：`screencapture -x -l 117813 /tmp/...png` 成功，2384×1988、PNG 約 656 KiB。原生 `NSBitmapImageRep` 針對內容中央樣本 2,500 pixels，得到 **96 種不同 RGB、0 個全暗樣本**，與 Computer Broker ScreenCaptureKit「黑畫面」相反；臨時 screenshot 已刪除。這至少證明**另一條 macOS 擷取方式可以取得非黑的視窗內容**，不能根據單一路徑的黑畫面判定 WebView 載入失敗。
- **AX Tree 觀察深度問題**：`AXWindow` **直接 children=5**，但其中 `AXGroup→AXGroup→AXScrollArea→AXWebArea` 有 `AXDescription=AgentDock Next`，再往下有 `AXLandmarkBanner`、`AXLandmarkNavigation`（`桌面導覽`，12 個 direct children）、`AXLandmarkMain`。真正的 Vue WebArea 和 UI 附屬結構**存在**。早先只看到 5 個 window child 是**淺層遞迴/工具快照限制**，不是 AX tree 總共 5 個 element。切換使用者前景視窗後，某次再次探查的 AXWindows 回傳空，顯示需要即時重新建立目標窗口，而不能復用舊 WindowID/PID。
- **資產路徑非根因**：`desktop/shared-poc/main.go` 透過 `//go:embed all:frontend/dist` 和 `application.AssetFileServerFS(assets)`；Wails beta.27 的 `internal/assetserver/asset_fileserver.go::newAssetFileServerFS` 自動 `findPathToFile(index.html)`、`fs.Sub` 到 `frontend/dist`，因此不該對此做無根據的 fs.Sub 改動。Frontend Vite `index.html`/JS/CSS 均存在；此前 RC20 25/25 Vitest files、184/184 tests 和 Vue typecheck 通過。

**根因（High confidence on observation，not mutation）：**此 Host 問題主要是 ScreenCaptureKit/window-only 擷取行為與淺層 AX 讀取造成的**診斷假象**，目前沒有足夠證據支持重寫 Wails Frontend 或重新打包 RC20。改用標準 macOS `screencapture -l` 和遞迴 AX WebArea 驗證，是可立即套用於後續 Host 自動驗收的**工具層改進**。

**尚未完成的獨立產品 Host：**`Connection rejected mutation → Overview → Connection error still visible` 真正 GUI 互動仍 OPEN；它不是黑畫面的同一問題。測試只有 source-level `connection.test.ts`+Overview 16/16 PASS，尚未以可辨識 UI/合法安全拒絕動作完整走一次。未確認任何不會更動現役 Core/Tunnel 的拒絕 mutation 前，**不得為了結案盲點 AXPress／Stop/Restart 或偷切使用者前景**。在可接受安全的前景時段，先定位 NEXT bundleID 與即時 AXWebArea 子項，才開始操作。

## H02 — Wails Tray Accessibility 失名的確切程式路徑（根因確認，程式修復 Host OPEN）

- 現役 NEXT GUI 是 `desktop/shared-poc` 的 **Wails v3.0.0-beta.27**，不是 `desktop/macos/AgentDockApp/Sources/AppDelegate.swift` 的原生 Swift AppKit GUI。實際 `desktop/shared-poc/main.go:174–203` 對 `app.SystemTray.New()` 設定 Template icon、`tray.SetTooltip(productName)`，並掛上刷新/開啟/離開選單；沒有設定 `tray.SetLabel` 或 Accessibility 專用名稱。
- 精確追到 Wails module `github.com/wailsapp/wails/v3@v3.0.0-beta.27/pkg/application/systemtray.go`：`SetTooltip()` 在 `.impl == nil` 時**只儲存 `s.tooltip`**；`Run()` 建立 `newSystemTrayImpl(s)`。macOS implementation `pkg/application/systemtray_darwin.go::setTooltip(tooltip)` **原始碼明寫 `// Tooltips not supported on macOS`，函式體完全為空**；`macosSystemTray.run()` 初始化 `nsStatusItem`、image、menu，但**沒有消費先前儲存的 `s.tooltip`**。
- Objective-C 底層 `pkg/application/systemtray_darwin.m::systemTrayNew()` 使用 `NSStatusBar.statusItemWithLength`／`statusItem.button`，設定事件監聽與 menu，**沒有呼叫 `setAccessibilityLabel`／toolTip**。因此在 Wails beta.27 macOS，專案呼叫 `tray.SetTooltip(productName)` **不能為實際 NSStatusBarButton 提供 AXTitle／AXDescription**；這是產品缺口，不是觀察工具的錯誤。
- 安裝版實際 `AXExtrasMenuBar` 精確比對：NEXT `dev.dropabit.agentdock.next` PID14144，只有 1 個 enabled `AXMenuBarItem`，`AXTitle` 空、`AXDescription/AXHelp/AXIdentifier` 缺失；Original `com.uvwt.agentdock` PID38041 亦 1 個 enabled Item，但 `AXDescription=AgentDock`、`AXHelp=AgentDock`。沒有做 AXPress、沒有改使用者 Menu Bar 設定；前景 app 仍非 NEXT。

**最小修復路徑及取捨：**

1. **先驗證最小零 fork 方案**：於單獨 clean 修正樹中對現有 Wails `SystemTray` 使用 `tray.SetLabel("NEXT")` 或經產品確認的精簡字樣。Wails macOS `systemtray_darwin.m::systemTraySetLabel` 會設 `NSStatusItem.button.title`，比 `SetTooltip` 真正接到 Objective-C；由 Host AX 測 `AXTitle` 是否確實非空，並確認沒有把狀態列擠滿、圖示與既有選單動作保持正確。**這是待驗證方案，不保證 AXTitle 在所有 macOS 版本皆可用。**
2. 若產品必須保留**純圖示**且輔助名稱必須完整為 `AgentDock Next`，檢查 Wails 新版 API／上游可用的修正；如沒有，只對其 Darwin bridge 做**最小 native patch**：在建立 `NSStatusBarButton` 後，從 Core-owned productName 設定 `button.accessibilityLabel`（必要時 `accessibilityHelp`），並確保 `SetTooltip`/初始儲存名稱在 `run()` 之後真正傳到 NSStatusItem。可能須以限定版本的 module patch/replace 管理，不應默默改 go/pkg/mod cache、更不能用未簽章 runtime injection 去修改已安裝 App。
3. 真正 Host Gate：原版保持 `AgentDock`，NEXT 以 bundle-ID scoped AXExtrasMenuBar 唯一可辨識 **`AgentDock Next` 或一致明確的 Next 名稱**；由使用者允許的安全互動窗口進行 MenuBar `Refresh Core Status`（不碰 Quit/Restart/Original），並讀回 NEXT `8767`、Original `8765` health200；0 焦點干擾及回滾可操作。**在此 Gate 之前不可按無名狀態列按鈕**。

## 交付判定與下一步

| 項目 | 本輪診斷是否完成 | 產品/Host Gate 是否完成 | 下一步 |
| --- | --- | --- | --- |
| H04 可重現 Race 機理 | **完成：根因是 fixture size × Race 耗時 × 過短 watchdog/Context deadline**，且臨時 overlay 全 7 個焦點測試 10 次 PASS | **未完成**：signed RC20 source 原測試仍會在 race 下 FAIL | Codex 恢復後或授權單寫入者在新 clean 分支只修兩個 Test fixture，同時驗證真的 blocked、負向失敗仍有效，重跑全 focused `-race -count=10`、完整 `go test -race ./internal/acp`；不擴大 ACP 架構 |
| H01 黑畫面／AX5 懷疑 | **完成：擷取與淺層 AX 觀察假象；有正常 WebArea 的正向觀測** | **未完成**：真正拒絕後跨頁操作 Host | 不先改 UI；找即時 AXWebArea 與不更動現役服務的合法安全拒絕情境，再以受控 UI 完成操作 UAT |
| H02 Tray 失名 | **完成：Wails v3.0.0-beta.27 macOS `setTooltip` no-op + 無 AXLabel** | **未完成**：已安裝 NEXT 尚未套補丁，無安全 MenuBar Host 驗證 | 先做 `SetLabel("NEXT")` 最小方案評估；若需 icon-only，才考慮極窄 Wails Native bridge；以 AXDescription／AXTitle 真實值驗收 |

**總結：**三項**根因調查交付已完成**，其中 H01 原先的「整個 GUI 載入失敗」假設已被新的 Host 證據反駁；H04 已找到不降級測試斷言的可行修復方案；H02 已定位到 Wails macOS tooltip 的空實作。沒有把尚未修改／驗收的產品功能標成 PASS。後續接手見 [Stage5 工作包](next-rc20-stage5-codex-resume-workpacks-2026-11-11.md)；P13/P14 仍依先前使用者決議延後，NEXT RC20 仍 ad-hoc、`release_ready=false`。

## 後續直接實作規格 B／C（2026-11-11，無 ACP／無正式 Source 修改）

### B / H02：Wails Tray 最小變更決議（Source 契約已確認，修正 Host OPEN）

**優先方案：零 Fork，現行 Wails `v3.0.0-beta.27`，僅修改 `desktop/shared-poc/main.go` 的 NEXT Shared Wails Tray 建立區塊（目前約 174–203 行）。** 保留 `tray.SetTemplateIcon(icons.SystrayMacTemplate)`、`tray.SetTooltip(productName)` 和原 `SetMenu()`，在 App Run 之前新增 `tray.SetLabel("NEXT")`；不修改 `desktop/macos/AgentDockApp/Sources/AppDelegate.swift`、穩定版、module cache、已安裝 App 或 M8。若需要跨平台不顯示文字，限定 `runtime.GOOS=="darwin"`。

- **精確 Wails API 已唯讀查證：**`SystemTray.SetLabel()` 在 `impl == nil` 時儲存 label；macOS `newSystemTrayImpl` 取得初始值、`setLabel` 經 `systemTraySetLabel` 在 main queue 設定 `NSStatusItem.button.title`。`SetTooltip` 在 Darwin `setTooltip` 只有 `// Tooltips not supported on macOS`，不會產生 AX 名稱。預設 Template Icon 維持；新增 4 個字母會占用狀態列額外寬度（右側瀏海／擁擠狀態列需實測），文字與圖示是否同時顯示仍屬 Host Gate。既有 `Refresh/Open/Quit` Menu callback 不更動。
- **上游取捨：**Wails v3.0.0 已於 2026-10-26 公告正式發布，v3.1.0 於 11-01 發布；有較新版本**不等於已證實修復 AX label**。未找到可直接證實新版已提供 `NSStatusBarButton.accessibilityLabel` 的相容正式修正，不能為單一 AX 問題升級整個 Wails/Go 工具鏈。先測 `SetLabel`，失敗才決定是否需上游修補。
- **正向 Host：**重建的 NEXT-only App 本機包具正確 Bundle、簽章；用 Bundle ID 精準查 NEXT `AXExtrasMenuBar` 的唯一 Item，`AXTitle="NEXT"`（或等價穩定可識別名稱）；Original `AXDescription="AgentDock"` 不變；Template icon 清楚、文字沒被 notch 隱藏、Menu 有狀態刷新／Open／Quit 且僅在允許窗口安全驗收刷新，前景不被搶走。**AXTitle 尚未以改版實機驗證，不寫 PASS。**
- **負向：**無 AX name、命中 Original、Icon 消失、選單項失蹤/誤指向、狀態列擠壓、暗色/亮色不辨識、文字多重 Tray、刷新導致 Core／Tunnel 重啟，任一即拒絕通過；**不得根據未命名 X 座標點擊**。
- **備選最小 Bridge：**若產品堅持 icon-only 且 `SetLabel` 無法滿足完整的 `AgentDock Next` 可及性，單獨維護有版本 pin 和 patch provenance 的 Darwin `pkg/application/systemtray_darwin.m` / Go bridge，僅在 `systemTrayNew` 建立 button 時設定 `accessibilityLabel`（可能補 `accessibilityHelp`），確保初始名稱生效；同步測 macOS AppKit／主執行緒、正常文字/Icon/Menu/Destroy/Show Hide。**不能改 Go module cache 或引入動態 runtime injection**。上游可接受之前保持局部 patch，若維護成本大於收益就保留 `H02 OPEN`。

### C / H04：正式 Fixture 變更規格（Source 未修改）

**可直接交付修改者的檔案邊界：**僅 `internal/acp/write_timeout_test.go`，先不要動 `connection.go`、`pipe_write_unix.go`、`pipe_write_windows.go`、`awaitACPWrite` 或 `awaitACPWriteResult`。現有臨時 Overlay `dist/h04-diagnostic-20261111/h04-a1-two-scheduling-fixtures.go` SHA-256 `7bb1ec60f5016568f879d99869d4a695c31ece2794a415f12eb9b335edc2b3f0`；JSON SHA-256 `37d876c57d16029b6a7ffed833926ea3ae021ca5400699c11b65a1287e441b58`，只是診斷材料，不加入正式成品。

1. `TestBlockedWriteHonorsRequestContext` 的 `prompt` 從 `strings.Repeat("x", 1<<20)` 改為 `1<<17`（128 KiB），保留 `cancel`/`deadline` 兩個子測試、`awaitACPWrite` 的真實 Write start、完整取消分類、`Closed()` 與 `pending` 清理斷言。
2. `TestQueuedWriteTimeoutPreservesActiveWrite` 的首筆 `Notify("large", ...)` 同樣用 `1<<17`；在 `awaitACPWrite(started)` 後、送 queued request **之前**，非阻塞檢查 `first` **尚未結束**；queued 50 ms deadline 返回 `ACP_REQUEST_CANCELLED` 後、啟動 peer drain **之前**，再次確認 `first` 仍阻塞。任何提前完成均 `Fatal`，不可讓測試以 payload 太小而假陽性。
3. 保留 `awaitACPWriteResult` 固定 1 秒 watchdog、未被傳送的 queued frame、`requireNoPendingWrites`、`Closed()` 不可觸發、peer JSON decoder 只收到完整 `large` 再 `healthy` 的順序與 Connection reuse。**不得僅延長 Timeout 或刪除負向斷言。**
4. Unix 實際 `os.Pipe` + `writeACPStream` 使用 `context.AfterFunc` 關閉 writer 中止已嘗試寫入；Windows synchronous pipe 用鎖定 OS thread + `CancelSynchronousIo` 並 join，不能將 Unix 的阻塞/關閉語意直接類推 Windows。`128 KiB` 在每個目標系統**不保證**大於實際 pipe buffer；若 native Windows 測試發現沒有有效背壓，需以明確 blocking handshake/fixture 設計收斂，不能把 Windows Host 標 PASS。
5. Source 提交後在**正式修正 source，非 Overlay**執行七項 focused `go test -mod=readonly -race -count=10 -timeout=140s -run '^Test(BlockedWriteHonorsRequestContext|QueuedWriteTimeoutPreservesActiveWrite|AlreadyCancelledWriteDoesNotTouchHealthyConnection|WriteFailureClosesFramingAndKeepsClassification|CloseReleasesBlockedAndQueuedWrites|CancelledRequestDoesNotWaitForCancelNotification|BlockedNotifyHonorsWriteDeadline)$' ./internal/acp`，再完整 `go test -race ./internal/acp`、Go vet、必要的 Windows native Pipe 測試及 Original/NEXT 健康、Session ownership；失敗時不得刷掉 FAIL。

**已確定且不需 Codex 重做的根因：**兩處 1 MiB Fixture × Race 下高 Marshal/Decoder/Pipe 壓力與短 deadline 的測試同步不穩，縮小 Fixture 且驗證實際阻塞的臨時 Overlay 七項×十次 PASS。**原簽章 RC20 Source Race 仍 FAIL、正式修正/跨平台 Host OPEN**，不因此重寫 ACP pipeline。


## 後續 Source 修復：H04 與 H02（2026-11-11 晚間）

此段是先前根因與規格完成後的較新實作結果，不改寫上文原始 signed RC20 704bb40d 的 FAIL 歷史。開發樹：/Users/wei/sideProject/agentdock-next-h04-h02-r01-20261111，分支 fix/next-rc20-h04-h02-r01-20261111，從文件 compat HEAD 297b26b2 分支建立；無 Codex ACP、沒有動現役 Core/GUI 或 Original。

- H04：正式修改 internal/acp/write_timeout_test.go，兩個 1 MiB Fixture 改 128 KiB，新增 active write 在 queued 前和 queued timeout 後、peer drain 前仍被阻塞的負向斷言。原有 1 秒 watchdog、cancel/code、完整 JSON Frame、healthy reuse、pending/Closed checks 全部保留。針對正式修正 source 的七項 Go Race -count=10 PASS（18.658s），不是舊診斷 Overlay 充數。Runtime Pipe、Windows native 和真實 Host UAT 沒有因此被宣告 PASS。
- H02：正式修改 desktop/shared-poc/main.go 只在 Darwin 對 NEXT Tray 加 SetLabel("NEXT")，不改 Template Icon、選單、Original、Wails module cache。Shared Desktop Go 測試 PASS，使用簽章 RC20 相同的 Frontend dist（來源檔已比對相同）離線編譯 arm64 Mach-O PASS；SHA256 fcaea02753eb4fd0ba072565b294f52968c17f56e8ff2bd3f6917c5a6a9e58f2。沒有啟動新 GUI、不偷焦點；安裝版 AXTitle、Tray 外觀、Notch 和操作 Host 依然 OPEN。
