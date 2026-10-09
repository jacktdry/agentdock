# AgentDock Next — Pre-M9 最新進度（2026-10-09）

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
