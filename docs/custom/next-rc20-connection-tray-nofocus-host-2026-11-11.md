# NEXT RC20 — Connection／Tray 無搶焦點 Host 盤點（2026-11-11，台灣時間）

> **2026-11-11 後續唯讀診斷更新：**[H01/H02 根因報告](next-rc20-h04-h01-h02-root-cause-2026-11-11.md)推翻「整個 GUI 黑畫面／AX 只有 5 個 element」的推測：原生 Window Server `screencapture -l` 取得有內容的視窗（2,500 像素抽樣、96 色），AX 深入第 5 個 root child 之內有 `AXWebArea`、Banner/Nav/Main；原 ScreenCaptureKit 結果與淺層列印不足以判定 Wails 故障。真實 Connection 拒絕跨頁 Host 仍未測。H02 追到 **Wails beta.27 macOS `systemtray_darwin.go::setTooltip` 空函式**，已安裝 NEXT Tray 依然無可及性名稱，需要 Wails 實際路徑修正／Host AX 證據；下面保留先前觀察發生時的內容，不更改其歷史紀錄。

## 為什麼進行與操作界線

使用者要求階段二：對正式已安裝 NEXT RC20 的 Connection「操作遭拒後跳到 Overview 再回 Connection 仍顯示錯誤」及 Tray「與 Original 可識別、安全開啟選單」補真實 GUI 驗收。工作優先不中斷使用者前景工作／Original 的 Stable 控制通道；**本次只做無搶焦點、唯讀 GUI／OS 觀察與既有來源回歸測試**，不做 Core/Tunnel/Port 變更、不碰 Edge 個人 Profile、不開啟或點擊未辨識的 Tray、不點擊 Wails GUI、更不修改 TCC／登入權限或建立新 RC。

## 安裝版本與控制面前置核對

- macOS installed `~/Applications/AgentDock Next.app` 具 bundle identity `dev.dropabit.agentdock.next`、Bundle `21.0.0`；Core `0.9.2-rc.20`／`704bb40d08f6`，`127.0.0.1:8767/healthz` HTTP 200，當次 PID `11613`。NEXT GUI PID `14144`。
- Original custom Core `1.1.1` 的 `127.0.0.1:8765/healthz` HTTP 200，當次 PID `38464`；Original App 本機 AX 觀察 PID `38041`。以上 PID 只是當時讀值，不得當作未來操作固定 ID。
- 初始 Browser Broker owners/workers/leases 均空、queue active0；Computer Broker active_sessions 空。所有本次 `foreground=forbidden`、`capability=observe` 的 Computer Session 均 `release cleanup=complete`；未借用任何 Codex ACP、Browser Worker 或外部瀏覽器。

## Connection GUI：Host 尚未通過（實際遮蔽與未完成內容）

- NEXT macOS Window List 唯讀取得 1 個真實非縮小主視窗，window ID `117813`，1146×948，app identity 對應 PID14144；未發現誤連 Original。
- 使用 NEXT Computer Broker `get_app_state`、`restore_window=false`，當時前景為 ChatGPT 或 Edge，操作後 `focus_changed=false`。AX Tree 只有 5 個基本元素（window、close、fullscreen、minimize、title），**沒有可操作的 Wails WebView navigation/Connection 按鈕**。
- `ScreenCaptureKit` 以該 Window ID **成功擷取視窗**（2292×1896 PNG，未提交至 repository），內容只見 NEXT 標題列與全黑內容區。這是**背景視窗擷取的實際結果**，並不單獨證明使用者將 App 切到前景時也一定黑畫面，也不能據此認定 JS/頁面載入失敗。GUI 主視窗存在、WebKit 子程序存在，但尚無真正可觀測的 Connection／Overview 畫面。
- 簽章 RC20 原始來源 `feature/next-a1-rc20-20261105`（commit704bb40d）直接重跑 `pnpm --dir desktop/shared-poc/frontend exec vitest run src/stores/connection.test.ts src/components/overview/OverviewPanel.test.ts`：**2 test files／16 tests PASS**。這證明操作錯誤保留與 UI render 的 source-level 邏輯，**不是 Host 真實拒絕 mutation→切頁 UAT**；候選 HEAD 與 Git clean 狀態未改。
- 接續真實 Host Gate：在可接受短暫 UI 前景的明確時段，由使用者先讓 NEXT 視窗可見／可互動或同意安全的 NEXT-only 前景驗收，再透過新鮮的 GUI tree 確認 Window／Navigation；找到**不會更動實際執行中 Core/Tunnel 的可控拒絕情境**才能執行 Mutation（絕不使用不明狀態的 Stop/Restart），檢查拒絕原因跨 Overview/Connection 的保留、下一次有意操作的取代與 Snapshot fail-closed。沒有安全受控拒絕情境時維持 Host OPEN，不改成 fake mock PASS。

## Tray：真實 RC20 AX 讀值及需修正的產品位置

- 透過 Mac-Dev host Swift／`ApplicationServices` **只查詢** `AXIsProcessTrusted()`，回報 `true`；不呼叫授權提示、不按 `AXPress`。精確以 `NSRunningApplication.runningApplications(withBundleIdentifier:)` 取得 NEXT／Original process，讀取各自 `AXExtrasMenuBar` 的 `AXChildren`。
- Installed NEXT `dev.dropabit.agentdock.next`：**1 個 enabled `AXMenuBarItem`**，`AXTitle` 空字串，`AXDescription`／`AXHelp`／`AXValue`／`AXIdentifier` 無值。
- 並行 Original `com.uvwt.agentdock`：**1 個 enabled `AXMenuBarItem`**，`AXDescription=AgentDock`、`AXHelp=AgentDock`；兩者的輔助資訊差異仍存在於正式 RC20。不能把 NEXT `AXMenuBar` app-menu 的名字誤當為 Tray name PASS，也不能根據未命名項目的舊螢幕座標盲點擊。
- **重要的實作來源差異：**目前已安裝 GUI 可由 `go version -m` 證實為 `github.com/uvwt/agentdock/desktop/shared-poc`／Wails v3；`packaging/macos/build-next-shared.sh` 明確 `go build ... desktop/shared-poc` 後把執行檔交給 macOS package。**實際 Shared Wails Tray** 在 `desktop/shared-poc/main.go` 使用 `app.SystemTray.New()`、`tray.SetTooltip(productName)`、`tray.SetMenu(...)`。過去在 `desktop/macos/AgentDockApp/Sources/AppDelegate.swift` 施加的 `setAccessibilityLabel(appName)` 是另一套 AppKit 原始碼，**不能推論本次 Wails installed Tray 已採用該設定**。目前不可斷言 Wails ToolTip 一定會映射成 AX Description；Host 實測沒有。
- 下一個修復工作應先核對 Wails macOS Systray 對應 NSStatusItem／可及性屬性的支援方式，在新隔離來源分支做**精準安全修正與實機**（不是連續盲改舊 Swift）。實際點擊／Open Next／Quit/Restart 的區分與錯誤按鍵風險只在 Item 已可明確辨識、允許受控前景互動後驗收；不得把目前 1 個 enabled item 當完整 Tray Host PASS。

## 驗收矩陣／下一步

| 門檻 | 結果 | 邊界 |
| --- | --- | --- |
| NEXT/Original 服務與版本識別 | PASS | 兩條獨立 loopback health 200；沒有更動服務 |
| 背景 GUI Window/ScreenCaptureKit 存在、無搶焦點 | PASS（觀察） | 1 window、`focus_changed=false`，內容卻是黑畫面；非前景 GUI 可用性 PASS |
| Connection/Overview source-level 拒絕錯誤保留 | PASS（16/16） | 不能冒稱安裝版操作 Host PASS |
| Connection/Overview 安裝版拒絕操作、跨頁維持錯誤 | **OPEN** | 暫無可見/可操作 WebView AX，未引發真實安全拒絕 |
| Tray app-scoped owner、單個 enabled AX item | PASS（觀察） | 本輪 read-only 比對，沒有 click |
| Tray 真正可辨識 NEXT 名稱／安全選單操作 | **OPEN** | `AXTitle` 空、description/help 缺失，後續針對 Shared Wails 實際程式修正 |
| 原版不中斷、Broker resource cleanup | PASS | Original Core PID 健康，觀察 Session 均釋放 |

**決策：**階段二完成了當前不中斷模式下的真實盤點與程式驗證，但**兩個核心 Host UAT 都尚未關閉**。不要為了湊綠燈開啟無權限的 macOS 前景／繞過 WebKit／發新版／動 Original。工作交接見 `next-stage1-branch-milestone-inventory-2026-11-11.md` 與 `next-rc20-installed-host-2026-11-11.md`；P13/P14／Nexus 仍按既有決議延後。

## 2026-11-11 A — 非干擾安裝版補充觀察（18:19+ Asia/Taipei）

**目的／限制：**確認已安裝 NEXT 真正能否安全執行「Connection 拒絕 → Overview → 返回 Connection 保留錯誤」；不切前景、不點擊、不停止 Core／Tunnel、不改 M8、Original 或使用者設定。

- **Host / Runtime PASS（觀察而非 Mutation）：**Mac-Dev 核實 NEXT GUI 即時 Bundle `dev.dropabit.agentdock.next`、PID `14144`；Original GUI `com.uvwt.agentdock`、PID `38041`；現行前景 `com.openai.codex`。Core 正式 `127.0.0.1:8767/healthz` 回 `{"ok":true,"version":"0.9.2-rc.20"}`、Original `8765/healthz` 回 `{"ok":true,"version":"1.1.1"}`（均 HTTP 200）。`/health` 是 404，不是健康端點。
- **AX / GUI 本輪局部可觀察：**AX 權限有效；按即時 Bundle 取得 NEXT `AXExtrasMenuBar` → 一個無名 `AXMenuBarItem`，子選單清楚含「核心服務：資料已過期」、「檢查核心服務狀態」、「開啟 AgentDock Next」、「結束介面」；Original Item `AXDescription=AgentDock`。**本輪 NEXT `AXWindows` 回 0**，無法在不搶前景的情況下重新讀取 Connection／Overview 子樹。之前根因報告已以另一輪原生截圖＋遞迴 AX 讀到 `AXWebArea`、Banner／Navigation／Main，該歷史可觀察證據有效，但不得冒充本輪 WebArea 或本輪完整 UAT PASS。
- **Source PASS（契約核對）：**`desktop/shared-poc/frontend/src/stores/connection.ts` 的被動 `refresh()` 不清 `actionError`；`runMutation()` 僅於下一筆操作清除舊 `actionError`，先處理 mutation 結果再刷新 snapshot；`OverviewPanel.vue` 顯示 `operationError`。`internal/desktopapi/connection_integration.go` 的 `mutate()` 驗證 Core 所屬根、ConfigRevision、M8 外的服務權威能力及必要的 preflight；`Action` 限 `start/stop/restart/regenerate`。UI 上不可操作的無效 Port／Mode，不會自然走到 `runMutation` 的錯誤跨頁路徑。
- **安全拒絕情境盤點：**後端直接送未知 Action 會 `connection_request_invalid`，stale revision 會拒絕且副作用為零，但**不代表合法 GUI 操作**：正常 UI 封鎖不合法選項，刻意繞過 Wails Binding 或改變現役 Revision 都違背本輪驗收目標。可見的 Start／Restart／Port／Tunnel 更改可能觸發真實服務副作用，不能拿來製造 FAIL。
- **Mutation Host OPEN／BLOCKED：**當前無可核實又不搶焦點的 NEXT 主視窗 AX 操作路徑，亦未找到可從 UI 合法觸發且保證不影響現役 Core／Tunnel 的拒絕 Mutation。沒有執行 AXPress、注入 Wails Binding、撰寫假錯誤、操作 Tray 或 GUI。**不能把 Source PASS、先前 AXWebArea PASS 說成真實 Mutation Host PASS。**

**下一個唯一必要 Host 工作：**在另獲可接受的前景操作窗口及已驗證不會生效的合法拒絕情境後，重新以 Bundle ID→即時 AXWindow→AXWebArea→Connection/Overview 取得具名操作控制，執行一次拒絕→Overview→Connection，檢查仍顯示相同 `actionError`，被動 refresh 不清錯誤，再確認雙 `healthz`。若仍無安全拒絕路徑，保持 OPEN，不加入新的欺騙式測試入口，也不為測試重構正常 GUI。
