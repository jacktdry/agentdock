# NEXT 本機 RC：目前執行與驗收狀態

更新：2026-10-23。這是本輪 RC 工作的現況與排序；歷史里程碑證據仍由
[原 Active Backlog](next-active-backlog-2026-10-21.md) 及其連結保存。
**尚未達到 Definition of Done。RC8 已於 NEXT-only 驗收流程安裝；正式連線已通過握手，隱藏 target 的 `other` type 被舊檢查拒絕，四站未執行。沒有正式發布。**
最新 [RC6 實際建置／安裝／失敗證據](next-rc6-local-candidate-2026-10-23.md) 優先於下列歷史 source checkpoint。

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
| NEXT | 已更新為 `0.9.2-rc.8`／`cd1193c802ca`；App `0.9.2`／`9.0.0`；helper 部署成功，原版未動 |
| NEXT App | `/Users/wei/Applications/AgentDock Next.app`；bundle ID `dev.dropabit.agentdock.next` |
| 公司路由 | 已授權寫入 NEXT-private file，載入 1 native connector／1 profile／4 exact company policies；首次 connect 被拒絕，沒有網站 PASS |
| NEXT MCP | 已新增獨立 `gitlab-company`，direct HTTP `/api/v4/mcp`；真實 refresh 回覆 auth_required，尚無 NEXT grant；未複製原版憑證 |
| Edge | 唯讀 WebSocket 成功回覆過 `Edg/155.0.4283.60`，之後回覆 `.62`；程序在本輪期間由外部換代，並非本工作重啟 |
| Profile | 使用者回覆只有一個名稱為「公司」的 Profile；尚未完成其 directory/context 的實機綁定 |
| CBM | 既有索引為 2026-10-14；本輪 Edge 新檔未覆蓋、部分檔案 metadata 改變，依 current source 查核；沒有重建索引 |

## 優先工作清單

| 順序 | 工作 | 狀態／剩餘門檻 |
| --- | --- | --- |
| P1 / Phase 1 | 真實 Edge 相容性 | **source 已實作**：一般啟動形狀、kernel executable path、無 suffix 的 WS endpoint、explicit context pin、hidden target 參數修正；尚未完成最終實機流程 |
| P1 / Phase 1 | 公司 Edge 持續連線 | **正式 Runtime 已部署 RC8**：真實 Broker 通過握手／metadata；owned target type 相容修正仍待下一候選實機驗收；尚無網站 PASS |
| P1 / Phase 1 | ConnectorStatusProvider / ExternalLeaseBackend | **source 已實作並離線整合驗證**：NEXT-only constructor、明確 connect/disconnect、同 socket Profile metadata、epoch/peer/fence、one-shot grant、task/session/worker、TTL／停止；實機 Profile／取消／異常回收仍待驗收。詳見 [Runtime 整合](next-edge-runtime-2026-10-23.md) |
| P1 / Phase 1 | 四個網站的真正 NEXT Broker UAT | **待實機驗收**：精確工作樹／HTTPS origin pair、owner consent、單一連線、多頁隔離與 owned release；目前沒有網站 UAT PASS |
| P1 / Phase 2 | GitLab 獨立 API 授權 | **NEXT connector 已註冊，待獨立 OAuth**：discovery 200／DCR，refresh 正確要求授權；尚無 repo/commit/pipeline/job/MR API UAT，mcp grant 不等於 REST read_api token |
| P1 / Phase 3 | Edge/Chrome 與多 Session 生命週期 | **保留既有三真實 Agent 共用 Managed Chrome PASS**；仍需 Edge、macOS no-focus、取消／逾時／crash／恢復及原生程序保護的實機證據 |
| P1 / Phase 4 | GUI／Tray／OAuth／原生動作 | **保留 RC5 Browser GUI PASS**；廣泛 GUI stale/loading/error、Tray／診斷／開資料夾與原生基本動作仍待針對性查核及 Host UAT |
| P1 / Phase 4 | Core Skills／CBM/MCP | **保留已安裝 helper isolated bootstrap PASS**；下一 RC 實際更新／bootstrap、並行生命周期與必要 Native UAT 待完成 |
| Phase 5 | 同 commit App／ZIP／DMG、安裝／回滾 | **RC6 候選建置及授權安裝 PASS**；同 commit archives payload／簽章／Skills PASS，正式 Edge connect fail closed。完整 UAT／live failure recovery 仍未完成；無正式 Tag／Release／feed／push |
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
   Profile current-binding 的首次 directory pin 僅在同 Core 記憶體中保留，跨重啟 pin 尚未自動保存。
10. Broker diagnostics 納入 native logical workers 與 authenticated direct ownership；
    修正直接租約被 GUI 誤算為 unowned/stale，保留未觀察 connector health 的 truthful 狀態。

## 實機授權與失敗記錄

- 使用者按過 Edge 的「允許偵錯」，後續握手與 browser metadata **確實成功**。
  失敗位置曾為 hidden target `-32602`，不能當成使用者沒授權。
- 舊診斷在錯誤後關閉了自己持有的連線。後來的新診斷是新連線；最後一次在
  signed socket evidence 通過後，WebSocket handshake 逾時退出。**已停止短連線重試**。
  不能告訴使用者按過一次就保證另一條新連線不再需要瀏覽器自身的同意。
- `Local State` 的 Profile-name metadata 讀取被 macOS 拒絕（EPERM）；已尊重拒絕，
  沒有讀 cookies／憑證、調權限、換 Profile，或透過其他路徑取得該檔內容。
- 沒有任何四個網站導覽／snapshot／screenshot 的成功實機證據；最後的 full smoke
  在握手階段便退出。foreground 取樣未變動只覆蓋這個未進入頁面操作的失敗探測，
  **不算 website no-focus PASS**。

## 驗證

前一 source checkpoint（`b50fdc29`）已執行過完整
`./internal/tool/browser ./internal/browserpolicy ./internal/config ./internal/app`：
最終本輪 source **1,639 case PASS、7 個既有 opt-in cases SKIP**。七個跳過項目是 shared real-headless、managed real worker，
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

## 下一個安全步驟

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
