# NEXT 本機 RC：目前執行與驗收狀態

更新：2026-10-23。這是本輪 RC 工作的現況與排序；歷史里程碑證據仍由
[原 Active Backlog](next-active-backlog-2026-10-21.md) 及其連結保存。
**23:58 本日最新：** [RC11 最終正向 Host 證據](next-rc11-host-uat-2026-10-23.md) **四個 exact HTTPS origin 的 navigate／AX／PNG、雙租約與 A release 後 B snapshot／release、網站期間 no-focus 全部實機 PASS**。後續新 negative-only lease 在原連線 peer revalidation 遭拒；禁止 JS／未授權 URL 的獨立負向 gate、Host fresh acquire、GUI／Tray、GitLab OAuth／API 與 live rollback 仍 open。原 socket 已依程序 disconnect、零租約，沒有自動重連或重啟原版；source fixture 新增雙租約全部釋放後 fresh acquire regression，不能代替 Host。
**23:26 同日最新實機結果：** [RC11 原生同意後兩站真實 UAT／清理及剩餘 gates](next-rc11-host-uat-2026-10-23.md)。此前一次新連線在 `socket_and_websocket` FAIL CLOSED；使用者之後明確確認已按偵錯允許，再次正式 connect 在 **5.523 秒 PASS**。同 socket 的 `oprueba.com`／`homiplay.io` 均已導覽／AX／PNG **PASS**，跨 worktree 拒絕 PASS。失敗 URL 的負向測試使 offending lease fence fail-closed，後續 `www`／sibling release **未完成**；不重連，最後正式 disconnect PASS、leases=0、兩 Core 正常。GitLab local OAuth 待獨立授權。
**尚未達到 Definition of Done。RC10 Oprueba 截圖曾 FAIL CLOSED，RC11 installed screenshot 現四站 PASS；sibling release 已 Host PASS，但 Host fresh acquire／獨立負向、GitLab、GUI／Tray、live rollback 等仍未通過。沒有正式發布。**
最新 [RC11 實機續作](next-rc11-host-uat-2026-10-23.md) 與 [RC10 歷史實機證據](next-rc10-edge-host-uat-2026-10-23.md) 分別保存。

文件／記憶同步複核：**2026-10-23 22:41–22:42 Asia/Taipei**。兩個工作樹乾淨，
feature HEAD `dc57213f`（本次文件更新前），主線仍為 `7e87605a`。
已安裝 24 檔仍與 RC11 候選相符、簽章 PASS；Core PID 94893 top-level running／identity
相符，沒有對 9222 的 established connection；Browser idle／resources=0、execution active_calls=0。
原版 health 仍為 1.1.0，Default pin／route SHA 未改。本次只讀查核，沒有重連、重啟或部署。

## 本輪工作契約

- 使用流程／痛點：ChatGPT 與 ACP 在使用者繼續工作時，共用 NEXT 管理的
  Browser／Computer／MCP 資源；公司網站使用使用者指定、名稱為「公司」的現有 Edge Profile。
- 直接改善：修正真實 Edge 無法通過既有 source 前置條件及 hidden target 建立失敗；
  共用單一已授權 WebSocket，各 target 有獨立 session、UID map、origin scope 與精確釋放。
- 最小實作：沿用 private direct CDP dispatcher、現有 Broker／lease／epoch guard；
  不將需要一次性 MCP transport 的機制強行重用為可反覆授權的公司連線。
- 可觀察驗收：真實 NEXT Broker 完成四個核准 HTTPS origin 的唯讀流程、
  其他頁面／Profile／原版保留、無程式搶焦點；source fixture 及握手均不代替此驗收。
- 不做：原版變更、公司資料寫入、P13/P14、NexusDock pairing、遠端 push／公開發布、
  全域子網域放行、安裝其他公司 Profile、繞過 macOS／Edge 授權提示。

## 重新核對的基線

| 項目 | 本輪實際觀察 |
| --- | --- |
| 主線 | `custom/main`，起點 `7e87605a91f3a1e48a5879f7e406c3803a06e00c`，開工時乾淨；未 push |
| 本輪工作樹 | `/Users/wei/sideProject/agentdock-next-edge-rc-20261023`；`feature/next-edge-rc-20261023` |
| 原版 | 8765 `/healthz` 回覆 `1.1.0`；沒有修改／重啟／安裝 |
| NEXT | 已更新為 `0.9.2-rc.11`／`c331afe47d2e`；App `0.9.2`／`12.0.0`；Core PID 94893 running，未連 Edge；GUI 仍待背景重開／可見畫面驗收 |
| NEXT App | `/Users/wei/Applications/AgentDock Next.app`；bundle ID `dev.dropabit.agentdock.next` |
| 公司路由 | NEXT-private file 載入 1 native connector／1 profile／4 exact company policies；正式 connect、雙租約及首站快照 PASS；沒有完整四站 PASS |
| NEXT MCP | 已新增獨立 `gitlab-company`，direct HTTP `/api/v4/mcp`；真實 refresh 回覆 auth_required，尚無 NEXT grant；未複製原版憑證 |
| Edge | 唯讀 WebSocket 成功回覆過 `Edg/155.0.4283.60`，之後回覆 `.62`；程序在本輪期間由外部換代，並非本工作重啟 |
| Profile | 真實 same-socket owned version metadata 觀察 `Default`；跨重啟設定 pin 已保存並由 RC10 正式 Runtime 驗證成功 |
| CBM | 既有索引為 2026-10-14；本輪 Edge 新檔未覆蓋、部分檔案 metadata 改變，依 current source 查核；沒有重建索引 |

## 優先工作清單

| 順序 | 工作 | 狀態／剩餘門檻 |
| --- | --- | --- |
| P1 / Phase 1 | 真實 Edge 相容性 | **source 已實作**：一般啟動形狀、kernel executable path、無 suffix 的 WS endpoint、explicit context pin、hidden target 參數修正；尚未完成最終實機流程 |
| P1 / Phase 1 | 公司 Edge 持續連線 | **RC10 正式 Runtime connect PASS**：另一個 MCP client 取得雙租約，首站導覽／快照成功；截圖故障隔離，完整 workflow 仍 open |
| P1 / Phase 1 | ConnectorStatusProvider / ExternalLeaseBackend | **source 已實作並離線整合驗證**：NEXT-only constructor、明確 connect/disconnect、同 socket Profile metadata、epoch/peer/fence、one-shot grant、task/session/worker、TTL／停止；實機 Profile／取消／異常回收仍待驗收。詳見 [Runtime 整合](next-edge-runtime-2026-10-23.md) |
| P1 / Phase 1 | 四個網站的真正 NEXT Broker UAT | **部分通過**：Oprueba 導覽／AX snapshot PASS；截圖及其餘三個 origin、負向 scope／sibling release 尚未通過 |
| P1 / Phase 2 | GitLab 獨立 API 授權 | **NEXT connector 已註冊，待獨立 OAuth**：discovery 200／DCR，refresh 正確要求授權；尚無 repo/commit/pipeline/job/MR API UAT，mcp grant 不等於 REST read_api token |
| P1 / Phase 3 | Edge/Chrome 與多 Session 生命週期 | **保留既有三真實 Agent 共用 Managed Chrome PASS**；仍需 Edge、macOS no-focus、取消／逾時／crash／恢復及原生程序保護的實機證據 |
| P1 / Phase 4 | GUI／Tray／OAuth／原生動作 | **NEXT 背景 Computer observe/release、no-focus PASS；保留 RC5 Browser GUI PASS**；目前 GUI 無可見視窗，Tray／新 GUI 與原生動作仍需 Host UAT |
| P1 / Phase 4 | Core Skills／CBM/MCP | **RC11 真實 helper bootstrap／26 個既有 Skills hash PASS**；CBM 新檔索引與並行生命週期、必要 Native UAT 待完成 |
| Phase 5 | 同 commit App／ZIP／DMG、安裝／回滾 | **RC11 同 commit 封裝／簽章／授權安裝／Skills PASS**；完整 UAT／live failure recovery 仍未完成；無正式 Tag／Release／feed／push |
| 暫緩 | P13/P14、NexusDock pairing、付費簽章／公證、Windows 完整 UAT | **明確暫緩**，不納入本機 RC 必要門檻 |

## 已做的 source 變更與證據界線

1. 原 MCP preflight 仍要求明確 debugging/data-dir flags；另有 private native-default
   前置路徑只接受固定 Edge executable 的普通啟動、`--no-startup-window` 或 `--restart`，
   且只接受 OS 使用者預設 Edge data root。它仍是必要條件，**不能自行 mint 認證／授權**。
2. 真實 Edge 的 `lsof -d txt` 回覆超過既有 8 KiB。改用 macOS `proc_pidpath` 讀取
   核心報告的執行檔路徑，保留獨立 disk/live Microsoft signature、listener、epoch 與
   Profile ancestor metadata 重驗；無 CGO 仍 fail closed。
3. Native direct relay 支援實機 `/devtools/browser`；舊 single-use MCP relay 的嚴格 grammar
   不變。實際 TCP 在握手前驗證，relay 永不自動 redial/reconnect。
4. Chromium hidden target 會因 `newWindow` optional **存在**而拒絕，即使其值是 false；
   dispatcher 省略此欄位，不在失敗後回退到 visible target。
   [Chromium primary source](https://chromium.googlesource.com/chromium/src/+/111ecbc657c45da1115a6bb97157721e11cbb82c/content/browser/devtools/protocol/target_handler.cc)。
5. explicit context missing／drift 拒絕。default context 的 optional ID 相容路徑仍需在同一
   transport 重驗 admitted default ID，且不能採用未選的其他 context。
6. `forkTarget` 共用同一 transport 與 command sequence，只能縮小 origin scope；各頁
   獨立 session／UID map／target。正常釋放及 dispatch 前的排隊取消不丟棄 sibling 連線。
   **已送出的操作若取消／I/O 模糊，仍會 retire 整個 transport generation；尚未完成
   production 多租約取消／回收驗收，不宣稱單租約所有失敗皆不影響其他租約。**
7. 新增 source-built `cmd/agentdock-next-edge-probe`：預設只查 browser metadata；
   explicit Profile 模式只在 owned hidden `edge://version/` 讀取固定 Profile-path metadata。
   `--read-only-sites --workspace <exact-approved-root>` 必須指定核准的實際公司 Git worktree，
   依該 root 限制四個固定 HTTPS origins 中的適用 pair、獨立 owned targets 與同一 socket，
   僅導覽／固定 readiness read／AX snapshot／PNG capture，無 click/fill/任意腳本或公司寫入。
   此工具**不啟動 Core、不改 live config、不 mint route grant，也不是 installed Broker UAT**。
   此歷史診斷 helper 不等於新的正式 Runtime；後者另在 owned session 啟用 Document
   request/response origin interception。popup/no-focus 實機證據仍 open。
8. App suite 的 protected-path 搜尋測試在乾淨主線同樣失敗：macOS `/var`／`/private/var`
   alias 比較不一致。測試改以 `EvalSymlinks` 後的實際檔案路徑比較；保留 protected state
   不可被搜尋且 ordinary visible file 必須保留的兩個行為斷言，沒有改 production 搜尋行為。
9. 正式 NEXT Core 經 `next-core-edge-cdp` driver 接入 native provider/backend。
   `browser_broker` 明確 connect／disconnect、exact company worktree selection、scoped
   navigate/snapshot/screenshot 已接上 M8；直接與 ACP 租約共用連線，各自 owned cleanup。
   `bind_current_profile` 的首次 pin 僅在同 Core 記憶體中保留，不自動改寫設定；本機已將
   same-socket 觀察的 `Default` 明確保存為 `profile_directory`，因此本機跨重啟 pin 已存在。
10. Broker diagnostics 納入 native logical workers 與 authenticated direct ownership；
    修正直接租約被 GUI 誤算為 unowned/stale，保留未觀察 connector health 的 truthful 狀態。

## RC10 之前的實機授權與失敗記錄（歷史）

- 使用者按過 Edge 的「允許偵錯」，後續握手與 browser metadata **確實成功**。
  失敗位置曾為 hidden target `-32602`，不能當成使用者沒授權。
- 舊診斷在錯誤後關閉了自己持有的連線。後來的新診斷是新連線；最後一次在
  signed socket evidence 通過後，WebSocket handshake 逾時退出。**已停止短連線重試**。
  不能告訴使用者按過一次就保證另一條新連線不再需要瀏覽器自身的同意。
- `Local State` 的 Profile-name metadata 讀取被 macOS 拒絕（EPERM）；已尊重拒絕，
  沒有讀 cookies／憑證、調權限、換 Profile，或透過其他路徑取得該檔內容。
- **當時**沒有四個網站導覽／snapshot／screenshot 的成功實機證據；該次 full smoke
  在握手階段便退出。foreground 取樣未變動只覆蓋這個未進入頁面操作的失敗探測，
  **不算 website no-focus PASS**。後來 RC10 首站部分成功與 RC11 待驗收狀態，以頁首及實機證據為準。

## 驗證

前一 source checkpoint（`b50fdc29`）已執行過完整
`./internal/tool/browser ./internal/browserpolicy ./internal/config ./internal/app`：
該歷史 source checkpoint 為 **1,639 case PASS、7 個既有 opt-in cases SKIP**。七個跳過項目是 shared real-headless、managed real worker，
以及四個 Codex adapter/native/containment opt-in；沒有將它們算成 live acceptance。

定點 direct/context/Profile/default-launch/kernel-path/site-smoke synthetic tests 已通過；
最終相關 Race Detector selection（含 shared connection、queued cancel）及四包 `go vet` PASS。
source diagnostic CLI `go build` PASS；`git diff --check` PASS。
新 Runtime 整合 checkpoint 的四包完整 Go 測試為 **1,666 PASS、0 FAIL、7 opt-in SKIP**，
JSONL 證據 `/private/tmp/agentdock-next-edge-verification-y6b6836m/go-tests.jsonl`。
相關 Race Detector selection **137 PASS**；四包 `go vet` PASS。
Shared GUI `pnpm typecheck` PASS；Vitest **24 files／176 tests PASS**。
Vitest 有 Node localStorage experimental warning，無測試失敗。本輪未修改 Vue。
後續 RC6 App／ZIP／DMG 及安裝檢查已實際完成，見上方最新記錄；原生 GUI、完整 rollback、四站 UAT 仍 open。

RC10 後 navigation readiness／owned viewport source checkpoint：四包完整 Go
**1,684 PASS、7 opt-in SKIP、0 FAIL**，相關 browser Race selection PASS，四包 vet PASS。
證據 `go-rc11-tests.jsonl`；RC11 已安裝，但 installed Broker 的四站流程尚未驗收通過。

## RC11 連線之前的原定安全步驟（歷史，已由最新續作取代）

**目前：RC11 已安裝並核對，沒有新 Edge 連線或臨時診斷程序。** navigation readiness／
自有 target viewport 的 source、離線回歸及可追溯 App／ZIP／DMG 均已完成。
下一次 Host UAT 應只啟動一條正式 Broker 連線，
一次完成四站／負向 scope／sibling release／no-focus，不能以重複短 probe 要求同意。
GitLab 獨立 OAuth、新 GUI 可見畫面與完整回滾仍為 Host gates；未提前合併主線。
詳見 RC10 證據文件尾的 **RC11 接續 checkpoint**，不要直接重跑舊 RC10 helper。

下列保留本輪故障定位時間線；最新實機結果見上方 RC10 證據。

RC7 正式連線約 15.484 秒後於 `socket_and_websocket` 結束。使用者確認有按允許，
並補充剛剛才按；不能歸因於使用者沒有同意。RC8 將 explicit native consent window
延長為 90 秒，status／admission 短時限不變。RC8 安裝後版本／24 個 App 檔案／簽章／
26 個既有 Skills／路由 SHA／原版 1.1.0 health 已實際核對相符。

RC8 真正 Broker connect 約 **2.727 秒**在 `owned_target_create` fail closed，已通過
WebSocket／browser metadata。兩次有界 source Host diagnosis（各一條 90 秒 consent
window，非 website UAT）定位：create／attach 成功，target type 是 **`other`**、
ID 完全吻合、URL about:blank、context 存在且等於同 socket 的 selected default；
兩次均確認只關閉自己建立的 target，relay 已 join。沒有查詢既有 targets 或導覽公司網站。
這是舊程式只接受 `page` 的相容性錯誤，不是 owner consent 或 Profile context 拒絕。

Source 修正只允許本 dispatcher 成功建立的 hidden target 使用 `other`；每次操作仍核對
exact target／session／context／origin，其他 type、未建立 target、context drift 都拒絕。
下一步建置可追溯候選、確認 NEXT 閒置後沿既有授權部署，再用正式 Broker 完成四站流程。
GUI 尚未背景重開／原生驗收，GitLab 獨立授權待後續不中斷的 callback 流程。

RC9 真實 Broker connect 約 **1.650 秒**在 `owned_profile_metadata` fail closed；
hidden target 的 `other` type 相容已實際通過。後續同 socket 有界診斷確認 version 導覽成功、
無 download，Profile directory=`Default` 且 data root 正確。初期 target URL 尚在過渡，
原檢查在文件就緒前退出。修正只容許 owned version target 的空 URL 等待；不在空 URL
evaluate，不放行其他 URL／context。production `dialNativeEdgeConnection` 的真實 source
qualification **4.13 秒 PASS**，包含 Profile epoch／peer 重驗、owned target cleanup、
transport close/join。這是 Host 診斷，仍不等於 installed Broker 或公司網站 PASS。
下一候選部署將沿既有授權把 NEXT 設定保存為已觀察的 `profile_directory: Default`，
避免跨 Core 重啟再次跟隨 current Profile；不改 Edge 自己的設定。
