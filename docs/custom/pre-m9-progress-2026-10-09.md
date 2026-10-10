# AgentDock Next — Pre-M9 最新進度（2026-10-09）

## 2026-10-10 P8-B ACPBridge 三 Session 真實 Headless 整合（SOURCE-ONLY）

- 最新 `TestACPBridgeSharedRealHeadless` **真實 PASS（46.76s）**：獨立 Headless Chrome＋cached MCP 1.7.0 由 budgeted rotating ACPBridge 連線，三個 Session Token 共用單一 MCP/Chrome，交錯操作隔離，最後一租約釋放後先完全回收，第二世代才重新建立。全程 peak Chrome main=1、兩個 generation、舊租約拒絕、test-owned 程序無殘留、budget slot/owner claims=0；未接管使用者 Chrome/Edge。先前 `list_pages` 傳 nil 造成 MCP schema 錯誤，已改為空 JSON object，未放寬驗證。
- 主 Session Browser／BrowserPolicy 完整 Go tests、共享池 Race Detector、go vet、gofmt/diff 與 Linux／Windows amd64、macOS arm64 no-CGO 跨平台編譯皆 PASS。正式 `NewACPBridge` 仍使用獨占模式；此為 opt-in 測試，不是 Next runtime 切換、實際三個 Agent 的端對端 UAT 或零打擾保證。
- 決策與限制見 [Browser noninterrupting routing §10](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。

## 2026-10-10 P8-B shared Headless Chrome 真實隔離 PoC（SOURCE-ONLY）

- 對 `ffd67c08` 的 private opt-in shared managed backend 增加 MCP pool-owned `about:blank` keeper；關閉租約 page 前以內部 `select_page({pageId: keeper, bringToFront: false})` 避免 MCP 1.7.0 回傳 `The selected page has been closed`，但不改用戶視窗焦點。Keeper 不分配給 Agent；原有 Managed runtime 尚未切換共享。
- `TestManagedSharedRealHeadless` 預設 SKIP、明確 opt-in 才啟動測試專屬離線 cached `chrome-devtools-mcp@1.7.0 --isolated --headless`。真實 opt-in **PASS（35.37s）**：同一個 Worker／Chrome 主程序供三個不同 owner scope、三組 Context 和 PageID 操作，18+ 次交錯操作隔離成功；所有 owned pages、MCP／Chrome 測試程序最終清理完成。此前前兩輪暴露 last-page 及 selected-page cleanup 問題，均已按實際 MCP 行為修復；未吞掉錯誤或停止其他 Session 程序。
- 主 Session Browser／BrowserPolicy 完整 Go suite、focused `-race`、Go vet、Linux／Windows amd64 及 macOS arm64 no-CGO cross-build、格式與 diff 均 PASS。未使用真實 Edge／個人 Chrome Profile、未碰使用者分頁、未啟動 GUI UAT／未部署 Next／未 push。下一步為跨 workspace 設計、正式 Worker/lease DTO 與自動回收、ACPBridge opt-in 接線及零提示／零搶焦點 native UAT。
- 詳見 [P8-B real proof](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。

## 2026-10-10 P8-A/B shared managed worker initial spike（SOURCE-ONLY）

- 基於乾淨 `376cf23c`、`feature/m8-permission-approval`：新增 private opt-in backend／fake tests，未改 ACPBridge、Manager、Coordinator、Registry、launch config 或 legacy tests；正式 managed 路由仍 exclusive。架構與限制見 [shared worker checkpoint](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。
- 一個 pinned underlying worker connection，concurrent startup singleflight、所有 Call 序列化；既有 Manager 的三個 agent scopes 各取得 distinct background page／isolatedContext。以唯一 borrow token 作 adapter WorkerID，避免相同 workerID refcount 無法識別 Stop 重複／recovery；底層 workerID 仍只有一個。固定 workspace，cross-workspace 拒絕。
- 釋放只清理該 borrow 已確認 page，保留 worker 30 秒至手動 idle reap／Shutdown；重複釋放不扣其他 holder。unknown new_page／清理失敗會禁止新借用且保留 cleanup failure，不猜 page、不殺 sibling。terminal Stop 最多一次，失敗保留；stopped generation 不重用。tombstones 上限 1024。Manager cleanup reason 尚是 exclusive 語意，禁止直接 runtime wiring。
- 指定且唯一 focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestManagedShared' -count=1 -timeout=100s` PASS（0.548s），無 skip；gofmt／git diff --check PASS。TEST ONLY 使用既有 fakeLeaseBackend decorator，實測 startup=1、distinct pages／contexts、serialized peak=1、unowned page closes=0、wrong-worker calls=0；涵蓋 spoof／cross-owner／workspace rejection、單一 failure 隔離、idle reuse／reap、failed start／new-page／close／Stop、cancel after page，以及 concurrent Acquire／Release／Shutdown、stuck startup／Call 的 deadline 與 late cleanup。無背景 runtime 資源。
- Shutdown deadline 不是強制中止 callback 或已完成 browser cleanup 的證據；不合作 callback 返回後才 final Stop。這是 architecture contract／opt-in offline proving，**未部署，未證明 actual chrome-devtools-mcp multi-client safe、零 popup／focus、真實 storage isolation、temp-dir／crash orphan cleanup**。完整／race／vet／cross-build 由 orchestrator 執行。無 real npx／Chrome／Edge／remote debug／GUI／restart／install／deploy／commit／push，未觸碰 AGY／stable；P8／M9 未結案。

> 核對時間：2026-10-09 03:12 Asia/Taipei。Repo：`/Users/wei/sideProject/agentdock-m8-permission-approval`；分支 `feature/m8-permission-approval`，核對時 HEAD `057ae1a2`。本 checkpoint 為狀態與交接，不代表 P8/M9 closeout。

## 各工作線狀態

| 工作線 | 已驗證 | 尚未完成 |
| --- | --- | --- |
| M7/M8、P1–P3 | Next identity/state/port/Tunnel 與 stable 並存隔離；M8 Permission/Approval；Next 公網連線 | Stable cutover、正式發行 |
| P4 ACP Manager、P5 MCP Management | 分別完成 closeout，均已 Next-only 部署；P5 完成獨立複審與 live UAT | 不等同 AGY-ACP .14 功能已整合 |
| P6 Plugin Management | Core/Desktop/Shared UI、獨立安全複審、Next-only Desktop/Core live UAT 通過 | Native GUI／鍵盤／焦點／選檔驗收、正式 closeout |
| P7 Nexus / Platform Essentials | 核心安全契約、Shared UI、Diagnostics 來源及離線驗證 | Native GUI UAT；**實際 Nexus 配對／網路測試依使用者要求延後** |
| P8 Browser Broker | Phase A/B/C1/C2a、C2b-A/B1/B2a/B2b；C2b-B2c macOS 唯讀 Edge 程序預檢 `a1d364e5`；B2d Microsoft on-disk 簽章前置檢查（原始碼驗證） | **真實 Edge 簽章／Profile／認證／no-focus／safe-release attestor、CDP 傳輸原子身分綁定、原生 GUI UAT**；正式 `ConnectorStatusProvider`/peer verifier 仍 `nil`，Edge 維持 `UNQUALIFIED` |
| Next 部署 | arm64 ad-hoc App、Core/Tunnel 服務註冊與 Next-only 安裝／正常更新實機驗收完成（`32abf101`、`5e074802`） | 真實故障注入的自動回復、Developer ID 簽章／notarization、跨平台原生 UAT |
| M9 | 尚未啟動 | 等待必要 parity、安全整合審查及 release-native gates |

**程式碼／已安裝版本不可混淆：** B2c `a1d364e5` 已提交但**尚未部署到 Next App**。上次 Next-only 實機安裝使用的驗證成品建置基線為 `539c68ac`（包含 B2b；不含 B2c）。目前使用 stable `$mac-dev` 作為 Next 開發、編譯、重裝、復原唯一控制連線；Next connector `macbook-air-m3` 不自我修改或安裝。

## AGY-ACP .14（獨立 Session 最新交接）

- 使用者確認 ACP 已到 **`1.2.0-agentdock.14` 候選版**，目前**等待 Next 開發／介面就緒後再安排整合測試**。
- 獨立 ACP 工作樹 `/Users/wei/sideProject/antigravity-acp-next-integration`，`feature/next-integration-optin`；交接記錄 HEAD `87cff1e`，功能修正 `9381b04`。ACP 本身測試 **456 pass／0 fail（43 files）**、TypeScript、Lint、macOS arm64 build PASS（根據 ACP 工作線交接報告）。
- .14 加強已 ACK 的 stream / SQLite 跨事件一致性；DB 刪除／回退／重複 Step ID／文字改寫 → fail-closed，避免重播已送出文字。但**尚未推送／部署**，交接文件記錄 stable 及 Next **已安裝 ACP 仍是 .8**。`AGY_ACP_STREAM_JSON=direct` 未接正式 `Adapter.runPrompt`；native M8 Hook/IPC、State/HOME opt-in、Keychain/OAuth、Session/工具／圖片／取消／Replay、stream parity 與跨專案權限驗證**尚未完成**。
- Next Session 負責 Core-owned Profile/State/M8 權限與 IPC；ACP Session 負責 AGY ACP 自身實作；**不得交叉修改 repo 或誤將 .14 候選版視為 Next 整合完成**。見 [ACP Next 整合交接](agy-acp-next-integration-handoff.md)。

## Loopback 事故與最新狀況

- 10/09 凌晨五項既有 `cdp_discovery_test.go` 本機 HTTP/WebSocket `httptest` 逾時；Next 8767 一度 TCP/HTTP 逾時，儘管 Next PID `75320` 持有 LISTEN；臨時 Python localhost 也逾時，stable 8765 可連、`lo0` UP，macOS Application Firewall 顯示停用。**根因尚未確認**，不能歸咎 B2c 或 Next Core。
- **10/09 約 03:12 經 `$mac-dev` 重新確認：Next `http://127.0.0.1:8767/healthz` HTTP 200（版本 `0.9.1`），Core/Tunnel/GUI 服務仍在，stable Core/Tunnel 不變。完整 `go test ./internal/tool/browser ./internal/browserpolicy -count=1 -timeout=120s` PASS，先前五項逾時均不再重現。** 原本的「持續阻塞」狀態改為「已恢復但根因未明／觀察中」，不得宣稱永久修復。
- P8 B2c 的 focused tests、targeted Race Detector、`go vet`、Linux/Windows cross-build 已通過。沒有因事故放寬安全 Gate 或本機網路設定。

## 下一步（優先順序）

1. **P8 可信來源與傳輸邊界**：Edge 簽章／Profile／登入與 no-focus、safe-release 驗證；實作原子化 CDP 執行個體／操作身分綁定；不得以「連得到」或 OS 程序預檢直接放行公司 Edge。
2. **Next 與 AGY .14 整合前置**：凍結 Next-owned State/HOME、ACP Profile opt-in、M8 Native PreToolUse Hook／可信 Core IPC 的授權與拒絕合約；等 Next 準備好再做 Next-only .14 聯合 UAT，勿改動原版及 AGY Session 的原始碼。
3. **P6/P7/P8 原生 GUI UAT**：視覺／鍵盤／焦點／生命週期驗收；真實 Nexus 配對持續延後。
4. **Pre-M9 整合與發行 gate**：安全複審、故障回復、簽章更新、Windows/WSL/Linux 原生驗證。

交接參照：[roadmap](roadmap.md) · [feature parity](pre-m9-feature-parity.md) · [P8 Browser checkpoint](pre-m9-browser-broker-roundtable.md) · [ACP .14 handoff](agy-acp-next-integration-handoff.md)。Next 的 build/install/service 變更一律走 stable `$mac-dev`，不碰原版 AgentDock。

## 2026-10-09 B2d 原始碼補強（部署前，非 P8 結案）

- 在 B2c 的唯讀 `lsof`／`ps`／映像路徑與二次 PID/epoch/argv 複驗間加入 Microsoft Edge 本機磁碟簽章前置檢查，限制 Apple generic chain、Team ID `UBF8T346G9` 與 `com.microsoft.edgemac`，同一 2 秒 deadline；未觸碰 Edge tabs/登入/CDP。`--deep` 在現有安裝 Edge 約 4 秒，故改為嚴格驗證 App 簽章及執行檔（各約 0.2 秒），**不視作巢狀 framework 信任或實際執行中二進位身分證明**。
- 既有 macOS localhost `httptest` 有間歇性逾時：初次完整 Browser suite 5 例逾時，重新執行整套 PASS；新 Edge focused + race、go vet、Linux/Windows crossbuild PASS。此異常需後續追蹤，不視為已根治。
- **尚未安裝／部署**，正式 runtime provider/peer verifier 維持 nil，Edge `UNQUALIFIED`，仍需 PID/簽章/已登入 Profile /no-focus/安全釋放及原子 CDP transport 綁定，且尚未執行 native GUI UAT。

## 2026-10-09 B2e Edge 參數解析防混淆（Next source-only）

- 接續 `eecb41d8`：將 Edge macOS `ps` 命令列驗證改為嚴格邊界解析，限定固定 Edge executable、唯一 `--remote-debugging-port` 和 `--user-data-dir`，接受明確的空白／引號格式，拒絕重複、混淆、控制字元、過長或非 canonical 路徑。只改私人 Edge OS preflight，不變更通用 CDP discovery。
- 新增正常與惡意 fixture，特別驗證拒絕發生在 image/signature 查驗之前。未修改 stable、AGY-ACP、Next runtime，未部署、未呼叫 Edge/CDP；`NewRoutePlanner(..., nil)` 與 `UNQUALIFIED` gate 持續有效。
- 本機偶有高負載與短時 localhost 測試逾時，對照實際測試紀錄評估，不將負載猜測當成已證實原因；避免多個 Go test/vet 並行編譯。


## 2026-10-09 B2f 執行中程序動態簽章前置檢查（Next source-only）

- 基於 `5b436f3d`，在磁碟簽章後、第二次 listener/PID/start/argv/mapped-image 複驗前，新增獨立 Security.framework PID dynamic code validity 檢查；同樣限定 Apple generic anchor、Microsoft Team ID 與 Edge identifier。`darwin && !cgo` 拒絕，Linux/Windows 原生預檢維持拒絕。
- 共用 2 秒 context；C 呼叫不能取消，因此全程序只允許一個 outstanding native verification。caller 逾時即拒絕，slot 保留至 C 返回，後續呼叫直接拒絕，避免重複呼叫累積 goroutine。任何失敗或取消只回 opaque unqualified error 與零 evidence。
- 未查詢使用者 Edge、tabs/CDP/profile，未修改服務、安裝或部署。native negative fixture 只檢查本次 Go test process。正式 provider/peer verifier 仍 nil，Edge `UNQUALIFIED`；Profile/登入/no-focus/safe-release、原子 CDP 綁定與 native GUI UAT 尚未完成。

## 2026-10-09 B2g Profile 目錄 metadata 前置檢查（Next source-only）

- 基於 `23fa0dd1`，加入私有可注入 metadata probe；程序／簽章證據前後比對 bounded ancestor chain 的 uid、mode、非零 device/inode。macOS 僅 `Lstat`，拒絕 symlink、非目錄、錯誤 owner、不安全 group/other writable mode、觀察到的替換及取消；錯誤維持 opaque、零 evidence。
- 最多 32 個 path components／4096 bytes；保守拒絕 sticky writable ancestors（含 `/tmp`）。所有成功／race fixtures 為注入資料，native negatives 僅 `t.TempDir`；不列目錄、不讀取 profile 內容、不接觸實際 Edge/CDP。
- focused Go tests（`GOMAXPROCS=2 go test -p 2`）及依序 Linux/Windows/macOS 無 cgo test cross-build PASS。獨立複驗完整 Browser/BrowserPolicy suite、targeted race 與 go vet 均 PASS；未做 native browser UAT；未提交／部署。
- 只建立 physical directory prerequisite，不證明認證／同意，也不保證原子 fence 或排除未觀察到的 ABA race；provider/peer verifier 仍 nil、Edge UNQUALIFIED，P8/M9 gate 維持開放。

## 2026-10-09 B2h safe-release peer 一致性（Next source-only）

- 基於乾淨 `d1b0656b`：release 在 target `list_pages` 後、close 前立即複驗 peer，close 返回後再獨立複驗；前者失敗不 close，後者失敗隔離看似成功的 close 或原始 close error，優先保留 opaque peer error 並永久 latch peerLost。
- connector Stop 成功仍為 CleanupComplete；無法獨立確認 close 身分時明確標記 `owned page close unconfirmed`。Stop 失敗回復只重試 AgentDock-owned connector Stop、不重試頁面操作；SweepExpired 共用此路徑。
- TEST ONLY fake hooks 精確注入 list/close 階段 peer 替換、撤銷、verifier failure、close 後取消與錯誤優先順序，檢查 happy-path counts、connector-only recovery 和不重複 close；focused validation 包含 managed lease 回歸。`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'Test(External|ManagedLease)' -count=1 -timeout=120s` PASS（4.740s）；首次 sandbox 指令於 setup 因既有 Go cache 寫入權限失敗，獲准後用同一 cache 執行，未建立新 GOCACHE。已 gofmt；由主 Session 獨立重跑完整 Browser/BrowserPolicy suite、release/admission 專項 race 與 go vet，全部 PASS。這輪未跑 cross-build。
- 這是 TOCTOU 緩解與 response quarantine，**不是 atomic transport/CDP fence**；close 可能已生效，仍不能排除觀察間 swap。正式 provider/verifier 維持 nil、Edge UNQUALIFIED，未呼叫 live Edge/CDP、未做 native browser UAT、安裝／重新啟動／部署／push，P8/M9 gate 仍開放。


## 2026-10-09 B2i transport-fence admission contract（Next source-only）

- 基於乾淨 `44c30c28`，加入 private required `externalTransportFence` 能力契約；要求 owning connector transport 在每次 dispatch 原子 pin／比對 actual socket/session peer，禁止用 echo 或 legacy CallExternal 前後複驗冒充正式 fence。
- Acquire 消耗一次性 grant 後、Start 前拒絕缺少能力的 backend，回 sanitized ErrRequiredRouteUnavailable，零啟動／零頁面操作、不 fallback；Acquire／Call／Release（含取得失敗的已證明頁面清理）全部帶 immutable expected peer 走 CallExternalFenced。既有驗證、worker identity、清理規則保留。
- WorkerRegistry 不實作 fence，直接 StartExternal API 保持不變；build-tag live integration 的真實 registry lease attach 刻意維持阻擋。TEST ONLY fake 保存並比對 private peer，回歸涵蓋 missing fence、grant replay、zero／aliased／forged peer 與 legacy dispatch 零使用。
- **B2i 僅為 fail-closed 前置條件，未實作真實 bound transport、未提供 atomic guarantee、未建立 production external route grant。** 正式 planner provider／peer verifier 仍 nil、Edge UNQUALIFIED，P8/M9 未結案。未觸碰 live Edge/CDP/tab/profile、managed Chrome、stable／AGY、App runtime 或部署；未 push。
- 已 gofmt；唯一 focused command `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'Test(External|Planner)' -count=1 -timeout=120s` PASS（4.974s）；依序 `GOMAXPROCS=2 go test -p 2 -tags browser_integration -c ./internal/tool/browser` PASS（僅編譯）。首次 compile 因 sandbox 既有 Go cache 存取限制於 setup 失敗，獲准後使用同一 cache 通過。`git diff --check` PASS；主 Session 獨立跑完整 Browser/BrowserPolicy 測試、External/Planner 專項 Race Detector、go vet，以及 Linux/Windows amd64 交叉編譯，皆 PASS。未執行 native/live UAT。詳細契約見 [B2i checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2i-transport-fence-admission-contract-2026-10-09)。

## 2026-10-09 B2j single-socket pinned CDP relay（Next source-only）

- 基於乾淨 `6ff0f789`，新增私有 relay 與測試：只接受 canonical `ws` literal `127.0.0.1`／`::1` browser endpoint；必要 callback／expected identity 缺失或 alias 時不 dial。透過 gorilla `NetDialContext` 把已建立的真實 TCP socket 交給注入 attestor，獨立 observed peer 完全相符後才送 WS handshake；無 proxy、不跟 redirect，失敗零 relay、關閉 socket、opaque error。
- upstream WS 成功後才建立 Core-owned `127.0.0.1:0` listener／256-bit 隨機 bearer URL，限定一次 downstream WS；拒絕錯誤 token／URI／Host／Origin、HTTP probes 及第二個 client。雙向 text／binary／ping／pong／close，8MiB 訊息上限、兩秒 handshake／HTTP／write deadline、五分鐘 absolute session/read deadline，最多四個同時 downstream TCP connections。任一端失效或取消永久 latch lost，關閉所有 relay sockets／listener／HTTP connections，永不 reconnect。
- `withPinnedPeer` 僅提供 immutable local transport 的 peer admission check；remote loss 仍需 I/O 觀察，Close 可與 callback 並行，**未提供原子 CDP command 保證**。尚缺獨立審查的 OS established-socket owner attestor、初始 listener 歸屬證明、chrome-devtools-mcp adapter／immutable worker 綁定與新 fencing backend；不合作測試 attestor 最多保留一個全程序 worker，取消不能強制終止任意 callback。
- 正式 provider／externalPeerVerifier 仍 nil；WorkerRegistry 未實作 fence、未啟用正式 Acquire、未 mint status/grant，Edge 維持 UNQUALIFIED，P8/M9 gate 仍開放。未接觸真實 Edge/CDP/tabs/login/profile、stable／AGY .14，未 push／install／deploy；只新增 relay/test 並追加兩份指定 checkpoint。
- gofmt 與最終 `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'TestPinnedCDP' -count=1 -timeout=90s` PASS（0.534s），僅本機合成 fixtures、使用既有 Go cache。首次 sandbox loopback bind 被拒；升權後首次功能測試的 leak 檢查因 fixture HTTP servers 未及時清理而失敗，修正子測試清理後重跑 PASS。涵蓋零 dial／零 handshake 資料、實際 socket address、echo/control、拒絕／一次 attach、雙端不可逆斷線、取消／不合作 attestor bound、concurrent Close、訊息上限與 worker leak bounds。測試資源均已關閉；主 Session 另行完成 Browser/BrowserPolicy 完整測試、relay 專項 Race Detector、go vet、Linux/Windows amd64 交叉編譯，均 PASS；首次 race 發現測試 fixture WaitGroup Add/Wait 競態，修正後重跑 PASS。未做 native UAT。詳細限制見 [B2j checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2j-single-socket-pinned-cdp-relay-primitive-2026-10-09)。

- 後續獨立複核：CDP 上游可能在 downstream attach 之前發送事件，現改為 downstream 接入後才啟動雙向 WS pump，讓上游既有 socket/TCP 緩衝的早到事件正常轉送；新增 `TestPinnedCDPEventBeforeDownstreamAttach`，仍不允許重新撥號或替換上游 socket。

## 2026-10-09 B2k established TCP ownership evidence（Next source-only）

- 基於乾淨 `bf77b6b6805af4f53e252746258bc7f252bf0fc4`：新增 private read-only socket evidence，限定已建立的 concrete TCP socket、兩秒內有效 preflight、loopback endpoint 與 client/server 4-tuple；固定兩秒 context，兩次 lsof observation 與前後 epoch／dynamic Security.framework 簽章檢查。其他平台拒絕，Darwin no-cgo 無法通過簽章。
- 合成 host 實驗僅查自己的 Python PID，確認 `-FpnT` 欄位格式；parser 嚴格要求 PID／f record／canonical numeric loopback tuple／ESTABLISHED state，限制 8192 bytes／64 records／512-byte line，拒絕 wildcard、重複、反向、缺漏與未知格式。所有失敗固定 opaque error、zero evidence，不洩漏 raw OS error。
- TEST ONLY 本機 TCP sockets 與 fake lsof／epoch／signature probes，IPv4／IPv6 parser 為 deterministic fixtures；涵蓋 epoch restart、簽章、取消、malformed／huge output、錯誤 endpoint 和失敗零輸出。首次 sandbox experiment／focused test 被 loopback bind 權限阻擋，獲准後使用原 Go cache，沒有 cache redirect。
- **僅非原子 OS process observation，不是登入認證、socket proof、完整 attestor 或 trusted route grant。** 未 mint externalPeerIdentity、未實作 externalTransportFence、未改 relay wiring／正式 provider，未連 Edge／讀 profile；Edge 維持 UNQUALIFIED，P8/M9 未結案。主 Session 獨立執行完整 Browser/BrowserPolicy suite、Socket 專項 race、go vet、Linux/Windows amd64 及 Darwin arm64 無 CGO cross-build 均 PASS；未執行 native Edge UAT、push／install／deploy。合成 runtime 資源由測試 cleanup 關閉；supervisor 獨立驗證。詳細限制見 [B2k checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2k-established-tcp-ownership-evidence-2026-10-09)。
- 已 gofmt；最終 `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run TestEdgeEstablishedSocket -count=1 -timeout=90s` PASS（2.487s），focused selection 無 skip；`git diff --check`／gofmt verification PASS。先前獲准 focused run 亦 PASS（2.440s），之後補齊 root／NUL／過多 path components 的 malformed preflight 拒絕並重跑。

## 2026-10-09 B2l evidence／independent attestor safe composition（Next source-only）

- 基於乾淨 `99a88b7a52a7dbf5c7a2a7bf9c7f57377a88ada2`，新增 private constructor／adapter：限定 endpoint 與 source.endpoint 完全相同，在 B2j `NetDialContext` 捕捉的同一條 concrete TCP socket 上先通過 B2k tuple／epoch／dynamic signature，再呼叫必要且獨立注入的第二 attestor。B2j 獨立驗證 expected peer 並比對 observed identity；不從 B2k evidence mint identity，也不將 expected 傳給第二 attestor。B2j base constructor／externalTransportFence 均未修改。
- TEST ONLY 本機 synthetic WS／TCP fixtures 驗證單次 upstream dial、`ctscts` 後才 attestation、相同 client conn pointer、echo 與 cleanup。B2k probe 僅接收 PID，測試 decorator 記錄 adapter 收到的 client socket，讓 fake lsof probe 由該 socket 產生 reversed tuple；constructor-only 測試由 accepted server socket 產生同一 tuple。probe 使用自己的 synthetic epoch，不查真實 Edge。
- 拒絕矩陣涵蓋缺少／typed-nil probe、缺少 attestor、無效 peer、endpoint host/path 差異、malformed／different profile、stale／future preflight、PID／tuple／epoch／signature（含第二次觀察）錯誤、取消／deadline／B2j timeout／panic；第一階段失敗不呼叫第二 attestor。全部拒絕在 WS handshake 前，實測零上游 read bytes、socket 關閉、nil relay；成功等待 relay／fixture workers 結束並驗證 listener 已關閉。
- **B2k 仍是非原子唯讀程序證據，第二 attestor 尚無 production 實作；callback 獨立性是 caller 契約，adapter 不能自行推斷。組合不代表 OS-authenticated actual Edge、同意／登入／protected profile 或逐 CDP command 原子保證。** 正式 provider／peer verifier 仍 nil、無 runtime wiring／public API，Edge UNQUALIFIED、P8/M9 未結案；不合作 callback 仍受 B2j 單一 slot 限制，不能強制中止。
- 已 gofmt；唯一 targeted selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'TestEdgeEvidencePinned' -count=1 -timeout=90s` PASS（2.383s），無 skip。首次 sandbox 因 loopback bind 拒絕而未執行功能測試，獲准後沿用既有 Go cache 重跑通過；無 fresh GOCACHE／並行測試。`git diff --check` PASS；主 Session 獨立完成 Browser/BrowserPolicy 完整 suite、B2l 專項 Race Detector、go vet 與 Linux/Windows amd64、macOS arm64 無 CGO 交叉編譯，均 PASS；native/live Edge UAT 未執行。未觸碰真實 Edge/browser/Chrome/AGY/stable/Next installation，未 push／deploy。詳見 [B2l checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2l-safe-evidenceattestor-composition-2026-10-09)。

## 2026-10-09 B2m Connector／pinned relay immutable worker binding（Next source-only）

- 基於乾淨 `184b172e4bf8d1433f8b7ab4ed6a5b0fce606a38`，新增私有 `pinnedExternalConnector`／測試。只接受既有 admitted relay、完全相符 private peer、經 `validateExternalStart` 驗證的原始 external route、有效 context 與未關閉 registry。驗證 capability endpoint／path 與原始 upstream endpoint；僅替換 connector `--wsEndpoint` 為 Core-owned one-shot loopback URL，不修改或消耗 RouteDecision／grant，不輸出 browser status。
- 必要最小調整：relay 保存原始 endpoint 與不可重用 binding reservation；WorkerRegistry 私有 start helper 支援 pre-Open observation／cleanup hook。公開 `StartExternal` 行為不變。reservation 跨越 probe／Open／schema validation；probe 後取消／relay lost 不啟動 Open，start 後驗證 ready WorkerInfo generation、live SessionInfo 與 relay downstream attachment。失敗固定 opaque error，best-effort 停止 AgentDock-owned connector 並關閉 relay；不重試／reattach、不關閉 user browser／profile／page。
- 私有 `checkAlive` 不派送 browser operation；觀察到 worker／session／peer 變更或 relay lost 後永久失效。binding concurrent Stop idempotent；registry Stop／Shutdown 在 connector cleanup 前先關閉 relay，connector Stop error 仍會關閉 transport。TEST ONLY fake Probe／Open 搭配既有 managed schema 與本機 httptest synthetic WS，使用真實 relay 驗證單一 pinned endpoint argument、單次 upstream dial、handshake／echo、immutable state、拒絕矩陣、concurrent reservation／Stop、replay 與 cleanup 後 upstream fixture listener 仍存活。
- **仍未提供逐 command 原子 fence；`withPinnedPeer` 僅 pre／post local observation，Close 可競態、remote loss 可能需 I/O 才可觀察。** 無 generic CallExternal／CallExternalFenced 實作，正式 provider／peer verifier 未啟用、無 production caller／獨立 OS attestor；WorkerRegistry 不滿足 `externalTransportFence`。Edge 維持 UNQUALIFIED，P8/M9 gate 未結案。未做 live Edge／Chrome／discovery／native UAT，未接觸 stable／AGY repo，未 install／deploy／restart Next、real npx、push。
- gofmt 已套用；唯一 focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestPinnedConnector' -count=1 -timeout=100s` PASS（0.591s），無 skip。首次 sandbox 在既有 Go cache 寫入時被拒，尚未執行測試；獲准重跑沿用原 cache，沒有 fresh GOCACHE／並行測試。`git diff --check` PASS；主 Session 獨立執行完整 Browser/BrowserPolicy suite、B2m 專項 Race Detector、go vet 及 Linux/Windows amd64、macOS arm64 無 CGO 交叉編譯，均 PASS。本機測試資源均由 cleanup 關閉。詳見 [B2m checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2m-connector-to-pinned-relay-immutable-worker-binding-2026-10-09)。

## 2026-10-09 B2n-A CDP outbound frame permit gate（Next SOURCE-ONLY）

- 基於乾淨 `6b96e952`，只修改 private relay／binding、新增 targeted 合成測試及兩份 checkpoint。B2j standalone 預設保持 legacy；B2m reservation 進入 restricted bootstrap，完成 ready／validated session／downstream attachment 後 arm CLOSED。binding 私有 `callBound` 檢查 immutable WorkerInfo／peer 與既有 strict allowlist，在 managed worker mutex 下直接呼叫 captured Session.Call，無 general CallExternal fallback；最多一個 permit，overlap 拒絕，caller context 與兩秒 deadline 限制 window。
- 每次 downstream→upstream data／control write 在同一 relay mutex 內檢查 peer／loss／gate／permit context 與 immutable worker identity；write deadline 限制持鎖時間。Session.Call 返回即關閉 permit，再檢查 generation／SessionInfo／reconnect；取消、Stop、loss、變更、error／panic 均隔離結果並只停止 AgentDock-owned connector。socket／listener cleanup 在 relay mutex 外，pump 不取得 worker mutex，鎖順序為 worker→relay。idle unsolicited／late frame 丟棄且永久 latch lost；relay lifetime watcher 清理 owned connector；反向 upstream async CDP events 在 CLOSED 時仍可轉送。
- **LIMITATIONS：僅 controlled frame dispatch，非完整 B2n atomic trust／authorization。** window 內背景 outbound CDP 無法歸因；tool 返回後／idle 時 pump 處理的 frame 會拒絕，但較早排隊且碰巧在後續 permit 處理的 frame 仍無 provenance。bootstrap 僅容許初始化；idle event 引發 outbound response／control reply 可能 quarantine。取消不能撤回已交給 socket 的 bytes，Close 可能等待 bounded write 解鎖；remote loss 需 I/O 觀察。SessionInfo 是 pre／post observation；忽略 context 的 Session.Call 無法強制中止，transport 會到期關閉，但 direct call／connector cleanup 可能仍等待。bearer argv 不抵抗同使用者程序檢視。
- WorkerRegistry／production backend 仍未實作 `externalTransportFence`，planner provider／verifier 仍 nil、無 runtime caller／public API／status／grant；不代表 OS trust／auth／login／consent。Edge UNQUALIFIED，P8/M9 未結案。未使用 real npx／Edge／browser／CDP／user screens，未做 native/live UAT，未 push／install／deploy／restart／修改 stable 或 AGY。managed Chrome 與 standalone B2j source path 維持；其完整回歸由 supervisor 負責。
- 合成測試實測單一 upstream socket 收到的 frame 數，涵蓋 bootstrap、authorized frame／response、CLOSED 反向 async event、unsolicited／late／control frame、worker／peer／live session／tool mismatch 零 operation frames、overlap，以及取消 queued frame／Stop／registry Stop／loss／generation／session／reconnect／error／panic／deadline 的 output quarantine。測試資源由 cleanup 關閉，沒有常駐 runtime。最終 gofmt／`git diff --check` PASS；focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestPinnedCDPGate|TestPinnedConnector)' -count=1 -timeout=120s` PASS（2.645s），selected tests 無 skip。首次 sandbox 因既有 Go cache 寫入拒絕而未執行；獲准沿用原 cache 後 PASS（2.835s），隨後收緊 Session.Call 返回即 close permit 並加入 queued-frame cancellation coverage，為最終程式碼修正重跑相同 selection 一次；沒有其他 test selection。主 Session 獨立執行 Browser/BrowserPolicy 完整測試、B2n/B2m/B2j 專項 Race Detector、go vet 及 Linux/Windows amd64、macOS arm64 無 CGO 交叉編譯，均 PASS；首次 race 發現合成 Server 上游 WebSocket 雙 writer 競態，測試 fixture 加 mutex 後專項與獨立重跑 PASS，未放寬正式 gate。未做 native/live Edge UAT。詳細契約見 [B2n-A checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2n-a-cdp-outbound-frame-permit-gate-2026-10-09)。

## 2026-10-09 B2n-B SINGLE-USE CDP operation epoch（Next source-only）

- 基於乾淨 `2da83b6f`、`feature/m8-permission-approval`：private B2m relay 的 reservation 在 Session.Call 前消耗且不重設。首次操作完成後永久 CLOSED；第二次／replay／concurrent／reentrant 呼叫固定 opaque failure 並 quarantine owned connector/relay，沒有 B window。Stop／取消／panic／failure 不可重新開啟。首次成功後仍保留 idle reverse event，第二次操作才關閉連線；B2m bootstrap 與 standalone B2j legacy gate 維持。
- TEST ONLY httptest fixture：A command／result 成功後，A 的另一個 fragmented command 已送入 relay reader，B 立即嘗試；B 不進入 Session.Call，upstream count 維持 bootstrap + A（零額外 frames）。新增 second-call／replay／reentry／pre-canceled coverage，既有 failure／Stop／panic 測試追加 retry 拒絕。overlap 現在 quarantine 第一個操作。
- 刻意限制每個 Connector **只有 ONE MCP operation**，是 usability regression，不適合實際多步驟流程；並非 full B2n semantic provenance。black-box chrome-devtools-mcp 無 authenticated MCP tool provenance；bootstrap 仍可能 emit CDP，A 期間 background traffic 仍可能通過。未新增 envelope parser。未來必須有可信 CDP command dispatcher／provenance，timer 不足；OS trust／login／consent 仍未解決。production provider／verifier nil、Edge UNQUALIFIED，沒有 externalTransportFence、managed Chrome／browser lifecycle 變更或 live UAT／real npx／restart／install／deploy／push。詳細限制見 [B2n-B checkpoint](pre-m9-browser-broker-roundtable.md#phase-c2b-b2n-b-single-use-cdp-operation-epoch-2026-10-09)。
- gofmt 完成；唯一指定 selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestPinnedCDPGate|TestPinnedConnector)' -count=1 -timeout=120s` PASS（2.553s），selected tests 無 skip。首次 sandbox 因 httptest loopback bind 拒絕失敗，獲准重跑相同 selection 後通過；主 Session 獨立完成 Browser/BrowserPolicy 完整測試、B2n/B2m/B2j 專項 Race Detector、go vet 及 Linux/Windows amd64、macOS arm64 無 CGO 交叉編譯，均 PASS。測試 cleanup 關閉合成資源；未進行 native/live Edge UAT。

## 2026-10-10 P8-B private shared ACPBridge coordinator（Next SOURCE ONLY）

- 基於乾淨 `6d4ca83d`。新增 package-private opt-in workspace constructor，shared managed backend 接既有 coordinator／ACP session owner tokens；runtime `NewACPBridge` 保持 exclusive，沒有產品 flag／production switch。跨 workspace Acquire 啟動前拒絕，無其他 browser fallback。
- Registry general Call 仍拒絕 select_page；pool-owned private adapter 僅可對 immutable ready worker 選既有 owned keeper，pageId 必須有限非負整數、bringToFront=false。shared release／recovery reason 改為 owned PAGE cleanup + BORROW ended，exclusive messages 不變，failed cleanup 仍 fail closed／可 recovery。
- 現有 lifecycle sweep／recovery 後 reap；Close 先 ReleaseSession、shared.Shutdown，再 registry.Shutdown，concurrent Close 等待同一次 cleanup。合成 ACPBridge + fake registry session 驗證多 session 一個 startup、page/context isolation routing、owner／page 拒絕、sibling 保留、最後 release 不提前 stop、idle terminal stop once、private selection、failure recovery 與 default exclusive。
- gofmt 完成；唯一 focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestACPBridgeShared|TestManagedShared)' -count=1 -timeout=120s` **PASS（0.586s）**。real Chrome opt-in unset，real case SKIP，Go 網路下載停用；首次 sandbox cache 拒絕未執行測試，獲准同命令重跑 PASS。主 Session 獨立驗證 Browser／BrowserPolicy 全量 suite、`TestACPBridgeShared|TestManagedShared` -race、go vet、gofmt／diff，以及 Linux／Windows amd64、macOS arm64 no-CGO cross-build 全數 PASS。
- **未部署；僅 private opt-in source path，沒有實際 browser／Edge／Chrome 或 native ACP／UAT。** one workspace per instance、idle reap 後無 generational respawn；runtime generation replacement、automatic global process monitoring、multi-workspace pool 仍 pending。未 build／install／restart Next 或 stable、未碰 AGY／其他 repos、未 push。詳見 [P8-B integration checkpoint](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#9-2026-10-10-p8-b-private-acpbridge-coordinator-integrationsource-only)。

### 2026-10-10 P8-B generation replacement 後續（SOURCE ONLY）

- 基於乾淨 `82a72fab`，新增私有 rotating pool 與不同 opt-in ACPBridge constructor；每世代獨立 pinned registry adapter，owner gate 僅在舊 Stop 完成且成功後允許下一世代啟動。default exclusive 與原 terminal constructor 保留，沒有 runtime wiring。
- epoch borrow fencing、上一世代有界 cleanup tombstones、Stop error 拒絕替換、Shutdown admission revocation 與 idempotent cleanup 已加入。synthetic tests 以 fresh fake registry sessions 驗證 restart 第二次 Open、三個 concurrent acquires singleflight、peak live=1、sibling 保留、舊 lease/page 拒絕、Stop 阻塞／失敗及 Close／Shutdown。
- 指定 focused synthetic tests 最終 **PASS（0.565s）**，gofmt／diff check PASS；real browser opt-in 未啟用而 SKIP。主 Session 已獨立執行完整 Browser/BrowserPolicy Go tests、共享池專項 `-race`、go vet、Linux/Windows amd64 及 macOS arm64 無 CGO 交叉編譯、gofmt/diff，全部 PASS。單 owner 建構前提、跨 owner 仲裁、長期歷史 storage、crash reconciliation 與 native/live UAT 尚未驗證；未啟動 Chrome／Edge／npx，未 build／install／deploy／restart／push。契約與限制見 [§9 後續](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#9-2026-10-10-p8-b-private-acpbridge-coordinator-integrationsource-only)。

### 2026-10-10 P8-B cross-workspace process budget（SOURCE ONLY）

- 基於乾淨 `be3bf251`：新增私有容量注入 budget／reservation wrapper 與 budgeted rotating ACPBridge constructor。各 canonical workspace 獨立 registry／Chrome／pages，shared budget 只仲裁程序數；拒絕重複 registry／canonical owner。原本 budget lifetime claims 已由下方 cleanup checkpoint 改為成功 Close 後釋放。無產品 default／config／flags，`NewACPBridge` 仍 exclusive，沒有 runtime caller。
- 首次及替換世代在 underlying creation 前 reserve；borrow release／idle retention／pending Stop／取消／failed Stop 不退容量。generation.done + stopErr nil 才 exactly-once release；startup error／非 ready／缺 WorkerID 永久 uncertain，nil session 的成功 Stop 不足以證明 launch side effect 消失，保留 slot 並禁止 replacement，需未來權威 reconciliation。
- 新增五組合成測試，涵蓋容量 2、兩 workspace 各兩 owners one worker、global peak≤2、第三 workspace 啟動前拒絕、30 秒 idle reap／B noninterference、rotation 再 reserve、page/context／跨 token／舊 borrow fencing、canonical parent alias、並行 admission、failed／blocking Stop、取消 Shutdown、重複清理與 pre-ready uncertain launch。
- gofmt／diff check PASS；唯一實際 focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestACPBridgeShared|TestManagedShared)' -count=1 -timeout=120s` **PASS（0.816s）**。首次 sandbox Go cache setup failed，獲准沿用原 cache 同命令重跑；real Chrome env unset，real case SKIP，dependency network off。未啟動 browser／Edge／Chrome／npx、無常駐測試資源，未 push／install／restart／deploy、未改 stable／installed Next／AGY。主 Session 獨立重跑 Browser／BrowserPolicy 完整測試、共享池專項 Race Detector、go vet 與 Linux／Windows amd64、macOS arm64 無 CGO 交叉編譯，全部 PASS；runtime wiring、crash orphan 與 real/native UAT 仍 pending。詳見 [§9 budget checkpoint](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#2026-10-10-p8-b-cross-workspace-process-budget-checkpointsource-only)。

### 2026-10-10 P8-B budget claim lifecycle cleanup（SOURCE ONLY）

- 基於乾淨 `8f06e268`：唯一 claim identity 防止 stale／ABA release；僅全部 owner cleanup、rotating Shutdown、registry Shutdown 成功的 ACPBridge.Close 才移除 registry pointer。阻塞／failed Stop／uncertain launch 不轉移 claim，Close 維持 idempotent；constructor canonical validation 先於 claim，無副作用 construction failure 回滾 claim。
- 新增 fake tests：同 CWD 75 次新 registry 替換、Close 後 owners／slots 均 0、舊 token／WorkerID 拒絕、同 registry ABA、double Close／release、失敗與 concurrent blocked Close／claim，以及其他 live registry browser 的 Shutdown fence。
- gofmt／diff check PASS；唯一指定 `^TestACPBridgeSharedBudget` focused test **PASS（1.441s）**，無 skip，real Chrome opt-in unset／dependency network off。沒有實際 Chrome／Edge／npx／ACP、OS orphan detection 或 runtime wiring；完整 suite／race／vet／cross-build／native UAT 待 supervisor。詳見 [claim cleanup checkpoint](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#2026-10-10-p8-b-budget-owner-claim-lifecycle-cleanupsource-only)。


### 2026-10-10 P8-B safe aggregate budget telemetry（SOURCE ONLY）

- 私有 budgeted rotating ACPBridge 新增安全數值 `shared_budget` diagnostics（capacity／used／available／claims／uncertain）；only opt-in shared，預設 exclusive 不變，沒有可識別 Workspace／PID／Profile／token，沒有 runtime switch。
- Reservation／uncertain latch／release 均在同一 budget mutex 同步；unknown launch／failed Stop 不退 slot。Focused／full Browser+BrowserPolicy、focused race、go vet、gofmt／diff 均 PASS；僅 synthetic、沒有真實 Browser UAT／部署／push。詳見 [P8 §11](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#11-2026-10-10-p8-b-shared-budget-observability-checkpointsource-only)。
- 後續需 Next-only runtime wiring、權威 orphan／crash reconciliation 與實際 ChatGPT+2 ACP native UAT；不能因安全數字監測就視 P8/M9 已結案。


### 2026-10-10 P8-B Next Runtime opt-in wiring（SOURCE ONLY）

- ACP Browser Runtime 初始化可在 **明確 Next-only 單 Workspace opt-in** 後使用 private budgeted rotating shared worker（單 Core budget=1）。環境變數 `AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE` 必須是絕對 canonical 現存目錄，並通過 Darwin Next variant、專屬 `~/.agentdock-next`、ACP+Browser enabled、非 stdio checks；未設定完全維持 exclusive，設定錯誤明確 fail-closed。
- 其他 Workspace ACP session 的 coding 功能不因此被拒絕，但該 Browser Acquire 在啟動前拒絕；RequiredExternal/ExplicitExternal 不 fallback 至 managed。NewRuntime 現在呼叫新 selector，預設 production path 未換。詳見 [P8 §12](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#12-2026-10-10-p8-b-next-runtime-single-workspace-opt-insource-only)。
- 主 Session 獨立執行完整 app/browser/browserpolicy Go tests、selector/budget 定點 `-race`、go vet、Linux/Windows amd64 無 CGO 交叉編譯及 diff/gofmt，全部 PASS。
- 本次 runtime wiring／拒絕矩陣為離線測試，沒有 App build/install、Native UAT、使用者 Edge/Profile 操作或 AGY repo 修改。真實多程序 ACP、使用者焦點/彈窗、trusted Core identity 和 crash reconciliation 仍是 release gate。


### 2026-10-10 P8-B durable restart denial（SOURCE ONLY）

- 新增 Next-only shared Browser sticky marker，於共享 ACPBridge 建立前 O_EXCL/O_NOFOLLOW + fsync。僅 ACP 與 Bridge 均 clean Close 且 marker inode/owner 一致才 unlink；Core hard crash、舊 marker、篡改與不安全目錄一律拒絕下一次 shared startup。普通 exclusive 模式不變，沒有 OS PID kill。
- 主 Session 已獨立驗證 app/browser/browserpolicy/mcp-client/process 五模組全量測試、Restart Guard/Next opt-in 專項 race、go vet，以及 Linux/Windows amd64 app/browser 無 CGO 編譯均 PASS；diff/gofmt check PASS。
- 只有 test-only TMP/HOME marker 與自己建立的測試子程序 SIGKILL，沒有啟動 Chrome/Edge/npx。完整 orphan ownership/reconciliation、使用者可安全解除與真實多 Agent UAT 尚未完成。詳見 [P8 §13](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#13-2026-10-10-p8-b-sticky-core-restart-fencesource-only非-orphan-自動回收)。


### 2026-10-10 P8-B orphan diagnostic checkpoint（SOURCE ONLY）

- Next-only 共享 Worker 在 validated ready 時 append+fsync wrapper PID 觀測值至 §13 restart marker；異常或不完整持久化直接 fail-closed。Runtime 因 sticky marker 拒絕重啟時，從私有 marker + OS 唯讀 snapshot 輸出**不含 PID/路徑**的安全分類；PID 存在或已消失都不構成解除 marker／終止程序授權。
- 完整 app/browser/browserpolicy/mcp-client/process Go tests、app/browser targeted race、go vet、Linux/Windows amd64 no-CGO app/browser crossbuild、gofmt/diff check 全部 PASS。
- 新增假的 Worker、並行寫入、corrupt/symlink、失敗注入與 startup unknown 的 deterministic tests。**這不是 orphan cleanup**：可靠 process start identity、detached Chrome 子程序驗證、可證明 ownership 的 terminate/cancel 與 GUI recovery 尚未完成；source-only，沒有改動安裝版 Next、stable、AGY 或真實瀏覽器。詳見 [P8 §14](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。


### 2026-10-10 P8-B process birth identity fingerprint（SOURCE ONLY）

- 在 §14 wrapper PID 唯讀 journal 基礎上，加入 Darwin kern.proc.pid kernel start sec/usec＋UID＋PGID birth fingerprint。Next-only ready witness 缺任何指紋證據就 fail-closed，重啟時僅將匹配、PID 被重用、舊版 PID-only、缺 PID／OS 觀測失敗等**安全分類**回報，不發送 signal、沒有解除 crash marker。
- 完整 app/browser/browserpolicy/mcp-client/process Go tests、定點 app/browser/process race、go vet、Linux/Windows amd64 process/app/browser no-CGO crossbuild 與格式檢查全部 PASS。
- 真實觀測僅自己的測試程序（不啟動 Chrome/Edge），合成 PID/時間/權限失效測試另行隔離；仍**不是**正式 OS orphan cleanup 或 Next native UAT。詳見 [P8 §15](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。
