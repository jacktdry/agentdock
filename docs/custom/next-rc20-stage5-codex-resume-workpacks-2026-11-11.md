# AgentDock NEXT — Stage5：Codex 恢復後工作包與不依賴 Codex 的接續路徑（2026-11-11）

> **2026-11-11 H04／H01／H02 更新：**三項根因已查明，見[診斷與不可混淆的 source/Host Gate](next-rc20-h04-h01-h02-root-cause-2026-11-11.md)。H04 的 1MiB frame＋`-race` 使 1s watchdog/Context deadline 先於實際 Write 完成；兩個 Test 改用有明確阻塞證據的臨時 128 KiB fixture 時 focused 七項×十次 PASS，**正式 source 尚未修改**。H01 並非 confirmed Wails 空白 bug：macOS 原生擷取有內容且 AX 遞迴讀到 WebArea；真正拒絕跨頁仍 OPEN。H02 是 Wails beta.27 macOS `setTooltip` no-op 且缺少 `NSStatusBarButton` AXLabel；正確修正位置已定位。這三項根因調查不必再重新派 Codex；**後續程式碼修改及未過 Host Gate 仍須獨立處理**。

> **使用者工作流程與產品目標：**讓 ChatGPT＋真實代理在使用者日常操作電腦時，安全、低干擾地使用 NEXT 的 Desktop、ACP、Browser／Computer Broker 和必要 MCP 工具。前四階段主要驗證了現在已具備什麼，**本文件把真實 OPEN Gate 收斂為可直接派工、可重現、可驗收的最小工作包**，避免 Codex 額度恢復後重新盤點或重複修補。此處的「Codex 工作包」不是立即啟動 ACP 的授權；**本輪沒有啟動 Codex/Antigravity ACP**。

## 0. 不可破壞的現況／分層

- 已安裝 NEXT Core `0.9.2-rc.20`、簽章原來源 commit `704bb40d08f6`、Bundle build `21.0.0`、NEXT 端口 `8767`；Original Custom v1.1.1 在 `8765` 不得更動。NEXT RC19 signed 回滾 App 保留；目前 signed package 是 **ad-hoc、本地驗收**，非 Developer ID 官方 release。
- RC20 installed Gemini Antigravity ACP **completed→cancelled→same-session recovery PASS**；**Codex installed 同等驗收因提供者額度阻擋**，不是程式錯誤的佐證。
- Connection/Overview source test PASS，**真正 installed GUI 經背景 Window screenshot 空白／AX 僅基本 5 元素**，尚無真實拒絕操作跨頁驗收。Tray 真實 installed Wails `AXExtrasMenuBar` 1 個 enabled item，**NEXT 的 AXTitle/Description 缺失**，Original 有可辨識 label；不得盲點擊。
- Native MCP Launcher + Core-owned strict Catalog + M8 private framework 已具備；**installed Catalog `servers=[]`、Backend 0、Desktop 無 production mutation route**，Native Model tool→Backend Host OPEN。OS Signed Peer 只證明程序來源，**不等於真人同意**。
- Stage4 的 macOS 簽章 App／ZIP／DMG、本機跨平台 source build、Release CI 狀態應以 [Stage4 實測報告](next-rc20-stage4-crossplatform-release-readiness-2026-11-11.md) 為準。Windows installer 預設明確 disabled；`release_ready=false`、Update feed unpublished，任何工作包都不得自行解鎖。
- Git 本機：`custom/main=7e87605a` 是 RC20 源祖先；安裝用 signed source `704bb40d`、compat+Docs branch 後續獨立提交；相對遠端 `origin/custom/main` 有數百個本地提交未發布。原 `agentdock-next-edge-rc-20261023`、M6、P5 OAuth、Original maintenance dirty worktrees 不 reset／stash／覆蓋。P13/P14 已依使用者決策 deferred，NexusDock 亦 deferred。

## 1. 最短使用者可感知路徑（優先）

| 工作包 | 優先 / 前置 | 可直接定位的程式位置 | 最小工作與可觀測驗收 | Codex ACP 是否必要 |
| --- | --- | --- | --- | --- |
| **H01：NEXT GUI 真實可用性／拒絕跨頁** | P0；先有受控、可接受的視窗前景觀察，不能搶使用者焦點 | `desktop/shared-poc/main.go`, `desktop/shared-poc/frontend/src/App.vue`, `src/stores/connection.ts`, `src/components/overview/OverviewPanel.vue`, `src/components/connection/ConnectionWorkspace.vue` | 先確認**可見前景真的空白嗎**，或只是 Window-only ScreenCaptureKit/AX 遮蔽。只在有證據顯示真正 GUI 缺陷時做最小修正。以 installed NEXT 受控失敗 mutation（不得停止現役 Core/Tunnel）→ Overview → Connection、錯誤仍可見、下一次有意操作可替代、被動 refresh 不誤報成功。視窗焦點採樣不得干擾現役工作 | **診斷可用 Mac-Dev，不需 Codex**；若需跨程式修復才交 Codex 寫碼 |
| **H02：Wails Tray 真正可識別與安全操作** | P0；與 H01 相互獨立 | `desktop/shared-poc/main.go` 的 `SystemTray.New()`/`SetTooltip`/`SetMenu`，對應實際 Wails v3/macOS NSStatusItem；`desktop/macos/AgentDockApp/Sources/AppDelegate.swift` **非目前安裝 Shared Wails 路徑** | 用 OS AX 依 bundle ID 綁定 NEXT／Original，修正 NEXT Item 真的回 `AXDescription`/`AXHelp` 或等價 accessibilityName=`AgentDock Next`，Original 仍是 `AgentDock`；在可信 Item 上安全開啟狀態選單並只觸發非破壞狀態刷新、驗證健康，未辨識則 fail-closed；不誤按 Quit/Restart | 需要寫程式時由 Codex；Mac-Dev 可單獨做 Host AX 前置觀測 |
| **H03：installed Codex ACP A1 回歸（非寫碼）** | P0 驗收；**僅在 Codex provider 額度實際恢復、且當次可驗證時** | `internal/acp/connection.go`, `pipe_write_unix.go`, `pipe_write_windows.go`, `write_timeout_test.go`；安裝 RC20 Core | 在**已安裝 NEXT** 建立任務自有 Codex Session，正常 Prompt `completed` → 長 Prompt start/cancel `cancelled` → **同 Session** 再 Prompt `completed`；close/delete 釋放。只對自己擁有的 Session 操作；未恢復額度則記 `Blocked: provider quota`，不得升級模型、用 Gemini 結果冒名或重啟現役 Core | **需要真實 Codex ACP 驗收**，但不應自動啟動 code-writing worker |
| **H04：ACP A1 Race 寫入佇列的真相查證** | **P1 必要回歸**；獨立於 GUI/Native MCP，來源 signed RC20 `704bb40d` | `internal/acp/write_timeout_test.go`（`TestQueuedWriteTimeoutPreservesActiveWrite`、`awaitACPWriteResult`）、`internal/acp/connection.go::writeMessageContext`、`pipe_write_unix.go`、`pipe_write_windows.go` | Stage4 再驗：這一項非 race `-count=3` **PASS**、race `-count=3` **3/3 FAIL** (`write_timeout_test.go:145` 的 1 秒 watchdog)，其他六項 race `-count=2` PASS。先測實際 write bytes/progress、Race Detector 下 1MiB JSON decode 及 pipe drain 的時序，**區分真的 stuck 與不切實際的 fixture timeout**；保留 queued cancel 不破壞 active write、完整 frame、後續 healthy Notify 可用的相同語意，負向情境不得降級。修正後 `go test -race -count=10` 及相關 Host non-interruption 完成，不能僅拉長等待閾值就宣稱關閉 | 調查與定點測試不需 Codex；若需程式／測試修復交 Codex 單一 clean 分支 |

> **決策點：**H01 的前景觀測不等於本輪使用者已授權立即切換焦點；對沒有安全拒絕方式的真正 mutation，不可為了測試而自行停止 Core 或更改使用者政策。先看現有 UI 是否可用，再決定是否應修復程式。

## 2. Native MCP 的一條真實產品工作流程，不做抽象擴張

| 工作包 | 依賴／優先 | 程式主責範圍 | 交付驗收／絕對不做 | Codex 角色 |
| --- | --- | --- | --- | --- |
| **N00：選定第一個值得啟用的工具工作** | P0 前置 H01 診斷；無須修改程式 | 需求、已存在 Core/ACP/Browser 功能與 Native MCP 的差距；[Stage3 安全審查](next-rc20-native-mcp-stage3-security-audit-2026-11-11.md) | 說清楚誰會用、什麼 MCP 工具、為何既有 ACP 或 Plugin 不足以解決、可觀測效益、最小 backend 權限與停止條件。若沒有新增核心價值，Native MCP 留在 `disabled`，不純為完成 Milestone 啟用 | **不需要 Codex**，ChatGPT＋Mac-Dev 即可完成需求決策 |
| **N01：一個固定 signed Native Backend＋Catalog** | P1，依 N00 確定需求 | `packaging/macos/build-next-shared.sh`, `packaging/macos/build-app.sh`, `Contents/Resources/native-mcp-servers.json` 產生器與 `internal/app/runtime_next_native_mcp_catalog[_darwin].go` | `Contents/Helpers/native-mcp/<固定ID>` 為真實 arm64、指定 identifier、外層密封的受限執行檔，Catalog 唯一 ID、固定 argv／digest；負向拒絕 shell、任意 path/env/偽簽章；尚未透過真人 M8 之前不得開始 Native Backend | Codex 有界 writing worker；安全審查可交 Gemini read-only |
| **N02：可信 Desktop 操作者與 M8 的正式入口** | P1，依 N01 與 H01 的 GUI 可觀測 | `desktop/shared-poc/frontend/src/components/acp/ACPPanel.vue`, `src/stores/acp.ts`, `internal/desktopapi/acp.go`, `internal/desktopcontrol/*`, `cmd/agentdock/server.go`, `internal/app/runtime_next_native_mcp_registration_darwin.go` | 真實 NEXT GUI 清楚顯示 server ID/用途與啟動權限，Core 綁 kernel pid/uid/birth＋簽章過的 NEXT peer **及單次真人意圖**，提交 M8 Ask/Allow/Deny；UI 僅送 server ID，不接收 exe/args/digest。deny/timeout/cancel 不啟動；偽造同 UID client 不被當人類核准；禁止使用 localhost Bearer 繞過身份 | Codex 寫碼，Mac-Dev host 安全驗證，Gemini 唯讀 review |
| **N03：Core 生命週期接線與真實 Native Host** | P1，依 N01＋N02、Codex 額度 | `internal/app/runtime_next_codex_mcp_registration.go`, `runtime_next_native_mcp_registration_darwin.go`, `runtime_next_codex_startup_lock_darwin.go`, `internal/mcp/launchregistry`, `internal/acp`, signed launcher | 重用已存在 Core-private B6 Prepare→Attach，M8 授權與 revision fence、單一 server ID、pin/digest，隔離 Session 進行真正 `model tool call → server response → Cancel/Close`；檢查只回收 owned native children/lease，未結 journal fail-closed。若本輪信任根／verify→exec TOCTOU 不能合理封閉，維持 `release_ready=false`；不清掉未核准 orphan | Codex 寫碼、Host 需真正 Native Codex 模型額度，不能以 fixture 假裝 installed PASS |

**不可重複開發：**Signed Launcher、Strict Catalog Parser、ID-only Resolver、M8 permission Store、Core 讀取 verified PID readiness、Desktop Kernel Peer Provenance、B6 crash fence/journal 的 source 基礎已 PASS。必須從 H01/N00 的真實使用者需求開始，而非讓 Native MCP 成為無限制堆疊的附加價值。

## 3. 發行工程、平台與歷史交接工作包

| 工作包 | 優先 | 執行內容／驗收 | 依賴與模式 |
| --- | --- | --- | --- |
| **R01：NEXT 專屬 CI / Canonical Manifest 及正式授權政策** | P1 | 依 [Stage4 發行證據](next-rc20-stage4-crossplatform-release-readiness-2026-11-11.md)，釐清 NEXT workflow `next-release-preflight.yml` 的 PR-only scope、hardcoded RC1 Core smoke，Stable `release.yml` 只接受 `v*`，而 RC20 candidate tag 為 `next-v*`；不能假設已具自動發行。正式專用工作流必須固定候選 commit、macOS arm64 source/GUI/package、ZIP/DMG 封印、必要 CI、明確人工核准與 rollback，更新 feed 先保持 disabled；未授權不得 push/tag/release | 文件與 CI 靜態審查可用 Mac-Dev；需改 Workflow 時 Codex 限定 `.github/workflows/*` 與工具契約；外部 Hosted CI 待另行授權 |
| **R02：P11 NEXT-only 安裝回滾／發行者信任** | P1／P2（真正外部分發時） | RC20 installed 與 RC19 signed rollback 已有 source/Host 證據，但未實際將**這次 RC20**回滾；不能把備份存在當成恢復 UAT。選擇獨立安全時段做受控 NEXT-only restore→health→再部署，或保留 OPEN，絕不碰 Original；外部分發應釐清 Developer ID、notarization、簽章/Update Feed 與憑證權限，ad-hoc 並非完整發布簽章 | 不需 Codex ACP 的驗證部分可由 Mac-Dev 執行，但會中斷 NEXT，不能本輪擅自做 |
| **R03：Windows/Linux 平台範圍決策** | P2 | Go Core／Arbiter 交叉編譯通過（按 Stage4實測），不等於真 Windows Host。`packaging/windows/NextAgentDock.iss` 有 `#error` 的 fail-closed 停用，是使用者 10/14 認可 macOS P10 先行策略，不能為追綠燈解除。未來要支援 Windows Installer 才安排 Windows 真機/CI Runner、WSL/UI/安裝／更新／刪除 Stable 隔離全鏈路 | 先政策/需求判斷，不自動排到 NEXT 首發阻擋 |
| **R04：歷史未提交文件小範圍採納** | P2 | 舊 NEXT edge worktree 的 `next-a1-rc20-candidate-2026-11-05.md` 與 `pre-m9-progress` 11/02 Tunnel 維護記錄僅能帶原始時間，選擇性保存。不能覆蓋 11/11 已安裝 RC20 真實狀態；M6 ACP、P5 OAuth、Original maintenance dirty 樹全部保留 | Mac-Dev 只做 docs，需保護原 owner，非 Codex 必備工作 |
| **R05：P13/P14 與 NexusDock** | Deferred | 依 10/22 用戶決策，維持產品範圍外；沒有真實痛點或重新授權就**不新增實作** | 不排進 Codex Queue |

## 4. Codex 恢復後的最小派工順序及資源規則

1. **先補實測而不是立即寫碼：**H03 真實 Codex installed 三段 ACP（若額度恢復）。H01 若 GUI 前景觀測是可用的，就直接完成 Connection Host，不做新 UI 重構。
2. **先修會妨礙安全使用的可見缺口：**H04 Race regression 需明確釐清（不得因 source-only 失敗就宣稱 installed Host 也故障）；若 H01 證明實際 Wails WebView 故障才開 GUI 最小修復；H02 以真正 Wails systray 做 accessibility fix + 安全菜單 Host。兩者每一個都是**不同 clean worktree/單寫入者**，不可同時在安裝 App／原 dirty 樹動手。
3. **Native MCP 要先有用：**N00 決策明確才 N01 一個 Backend → N02 Core-owned 認證/單次 M8 意圖 → N03 真實 model tool/Close。任何沒有真實使用者效益的複雜 backend framework 都不得啟動。
4. **再整合與正式發行：**R01/R02 在發行產品範圍確認、必要 Host Gate 完成與明確授權後執行。`custom/main` 不可因與 RC20 分支有快轉關係就擅自推送：遠端仍落後數百 commits；需要全段 diff/CI/版控檢視。保持 signed source commit `704bb40d` 與之後的文件／部署工具 commit **不同**，不可把新的 HEAD 宣稱 RC20 執行檔來源。
5. **每個工作包最小交接條件：**負責分支/起始 HEAD、允許修改檔案、使用者可做什麼、3–5 個成功/失敗案例、Host/Source 類型、不可觸碰的 APP/Port/Browser profile、`go test`/frontend Vitest/AX 的可重現命令、OWNED session cleanup 與 rollback。Codex 額度或前景權限不足時標註 Blocked 並繼續下一個獨立工作，勿自動切高價模型或假設權限可用。

## 5. 階段五結案定義

- 這份文件是**可直接接續的工作包與依賴決策**，不是任何 P0/P1 功能本身已完成。
- **不需 Codex ACP**：H01 可見性前置觀察、N00 第一個 Backend 產品需求定義、R01/R03/R04 靜態評估與文件整理，及 Stage4 既有測試。
- **需要 Codex ACP/Code Writing 時才派工**：H01 若真實有 bug、H02 Wails Tray 修正、**H04 若需修 ACP Pipe/Test**、N01–N03 生產程式接線、R01 若需改 CI 工作流。
- **真正需要 Codex 額度的 Hosted UAT**：H03 和 N03 的 model/native ACP Host；即使所有 source tests PASS，Host Gate 仍應明確保留 OPEN，不能捏造通過。
- 原版、NEXT、雙端 Broker 清理與 Host 非干擾為所有工作包共同且必要的接受條件。
