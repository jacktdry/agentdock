# NEXT RC11：2026-10-23 本機實機驗收續作

更新：2026-10-23 23:58 Asia/Taipei。**四個核准 origin／sibling release 真實 Host PASS；RC11 整體仍未達 Definition of Done，沒有合併、push 或公開發布。**
本文件是 RC10／RC11 續作證據，保留原始 [RC10／RC11 安裝記錄](next-rc10-edge-host-uat-2026-10-23.md) 與 [RC 驗收清單](next-local-rc-acceptance.md)。

**2026-10-24 跨日後續（覆蓋下方歷史 GitLab 待授權狀態）：** [NEXT-only GitLab OAuth／MCP 唯讀 Host 驗收](next-rc11-gitlab-host-uat-2026-10-24.md) **PASS**，connector ready／35 tools，已讀取 Project／Commit／Pipeline／Job／MR／Branch；沒有借用 Stable 憑證，也沒有 GitLab 寫入。獨立 REST `read_api` gate 不包含在這次 MCP 驗收。

**最新 RC11 installed Host checkpoint（23:50 起）：** 修正排序後，**只做一次**正式 NEXT Broker connect，4.495 秒 PASS；在**同一條已授權 Edge socket** 上，兩個獨立租約成功完成 `oprueba.com`、`homiplay.io`、`www.oprueba.com`、`www.homiplay.io` 的真實 navigate／AX RootWebArea（正確站名）／PNG capture。Oprueba A release complete、HomiPlay B **在 A release 後 snapshot 成功**，B release complete、均無 cleanup_error。跨 root 讀他人 lease 正確拒絕。網站流程 206 筆 foreground 取樣均為 Chrome，Edge／NEXT 視窗 ID 集合不變。**四站與 sibling/no-focus 已 Host PASS**；但正向租約全部釋放後，測試要另 acquire 一個負向-only lease 時，又遇 `external peer verification unavailable or mismatched`，負向 `direct_evaluate`／不核准 URL **未執行**，整份腳本 `ok=false`，不能稱全部 gate PASS。原 socket 在確定 0 active 後由正式 `disconnect_external` 成功關閉，沒有重連或改動原版。Source 回歸另新增「兩 lease release 後同 socket 再 acquire」測試，離線 fixture PASS；**不等於 macOS Peer Host 修復**。詳見下方「23:50 最終正向 UAT」。

**最新 Host checkpoint：** 使用者稍後明確確認已按 Edge 原生允許偵錯，下一次**唯一明確發起**的正式 NEXT Broker connect 約 5.523 秒 PASS。沿同一條 Core↔Edge socket 取得兩個不同公司 worktree lease；`https://oprueba.com` 與 `https://homiplay.io` 的實際 navigate、AX snapshot 與 PNG screenshot 都 PASS。跨 worktree 拒絕 PASS；未核准 origin 導覽的負向測試使該 lease fence fail-closed，原腳本後續操作無法延續。`www` 兩站與 sibling release **未通過驗收**；同一 socket 後續 fresh acquire 又回報 peer verification mismatched，**沒有重新 connect**。全部 logical leases complete；最後以正式 `disconnect_external` 關閉唯一 Core-owned socket，原版 8765 與 Edge Profile／程序保留。詳見下方「晚間原生同意後」。

## 此次流程與非中斷契約

- 痛點：已安裝的 NEXT 需以一條經使用者原生同意的 Edge socket 完成四個公司 origin 的唯讀 Browser Broker 流程；不得重複短 probe、搶焦點、使用 Stable 憑證或把 source fixture 當實機。
- 最小工作：重查已安裝版本、兩個 Core／工作樹、NEXT Browser idle、既有單次驗收腳本。只發起**一次**正式 NEXT `browser_broker connect_external`，失敗即停止 Edge 重試；繼續獨立檢查 GitLab OAuth、背景 GUI、生命週期回歸與本機封裝回滾證據。
- 可觀察退出條件：四站各自導覽／AX snapshot／PNG、exact-origin／cross-workspace 拒絕、兩租約 sibling surviving release、無程式焦點變化且 owned cleanup；後續 GitLab、GUI／Tray、跨 Session 異常回收與 live rollback 有各自的 Host gates。
- 不做：無新 Edge profile、無短探測／自動重連、無同意代按、無公司資料寫入、無 Stable 8765 異動／憑證複製、無部署升版或 live fault injection、無 push。

## Git 與真實安裝身份

- 工作樹 `/Users/wei/sideProject/agentdock-next-edge-rc-20261023`，分支 `feature/next-edge-rc-20261023`；開始時乾淨，HEAD `00a12250`（文件），**binary source** `c331afe47d2e144d930266d99486ca1b4f40c335`。
- NEXT 安裝檔 `~/Applications/AgentDock Next.app`，installed helper `0.9.2-rc.11 / c331afe47d2e`，Core PID 94893，127.0.0.1:8767；Stable 1.1.0／PID 77138／127.0.0.1:8765。
- RC11 與 `dist/next-shared-arm64.oSRRh5/package/AgentDock Next.app` 的 24 個 regular files 路徑／SHA-256 全部相同，`codesign --verify --deep --strict` PASS；ZIP SHA `38d09fc6cb6c04c38e12237c3b98b25ae959677533e88b8bca55f220e16539c9`、DMG SHA `b60e8148e6ecc4f8c844ea99a910f5497ede2328f4aa0de5144a783ffd24568c`。既有 RC10 rollback bundle `~/Applications/.agentdock-next-replacement-05ac6r44/AgentDock Next.app` 的 Core helper 確認是 `0.9.2-rc.10 / c389f2361e8a`。
- 本次沒有替換 App Contents、Core、Tunnel 或任何 Stable 服務。

## 原生同意之前的第一次 RC11 Edge Broker 連線：FAIL CLOSED（歷史）

- 進場時 NEXT Broker `idle`、authenticated transport owners／leases／active operations 均為 0，Core 對 9222 無 existing socket。已向使用者**預告一次**原生「允許偵錯」；不自動替使用者點擊。
- 使用 NEXT 本機已授權 MCP client 呼叫 `browser_broker(action=connect_external, workspace_root=<Oprueba exact root>)`，timeout 120 秒，native consent window 90 秒。
- **90.892 秒後失敗**：`BROWSER_REQUIRED_ROUTE_UNAVAILABLE`，階段 `socket_and_websocket`。期間曾觀察到 Core PID 94893 至 127.0.0.1:9222 的 ESTABLISHED TCP，但沒有 Broker 成功 connect／四站操作；不能把 TCP 連通等同 WebSocket admission 或同意完成。
- 被動 foreground observer 有 285 筆完整樣本，均為 `com.google.Chrome`。Edge window ID 集合從 11 變 12，新增一個 ID；**這不是可歸因的對話框內容或授權結果證明**。未執行 navigate、snapshot、PNG、雙租約與 sibling release；四站 Host UAT **仍 open**。
- **沒有進行第二次 connect 或 source probe。** 事後 Broker 回到 idle、owners/leases/workers/active/queued/orphans 全 0、authenticatedTransportOwners=0；Core 已沒有對 9222 的 TCP 連線。Stable 與 NEXT health 各為 1.1.0／0.9.2-rc.11。
- 失敗 JSON、foreground JSONL 留在私有測試目錄 `/private/tmp/agentdock-next-edge-verification-y6b6836m/` 的 `edge-rc11-connect-result.json`、`edge-rc11-connect-foreground.jsonl`。未存瀏覽器 cookie、密碼或 Profile 複製品。

## 晚間原生同意後：第二次明確正式 Broker 連線與兩站實機 UAT

- **使用者確認原生偵錯同意**之後，23:21 核對 NEXT Core 94893／rc.11、Stable Core 77138／1.1.0、Broker idle、沒有任何 9222 socket 或 active lease；只建立**一條新的正式 NEXT Broker 連線**，不是 short probe、source helper 或透過 Stable Edge MCP。5.523 秒 `connect_external` PASS，Core 94893 對 Edge 9222 的本地 TCP 為 `ESTABLISHED`。接下來兩個驗收 helper **都不呼叫 connect**。
- 原有 `rc11-site-uat.py` 真正透過已安裝 Broker 取得 Oprueba／HomiPlay **不同 lease ID**，各自成功 `direct_navigate`、`direct_snapshot`、`direct_screenshot`。Oprueba AX RootWebArea 為「OrpheLink — 安全、穩定的遠端連線與裝置管理方案 | O'Prueba」，PNG **301,650 bytes**。HomiPlay AX RootWebArea 為「一個入口，存取不同地方的個人設備 | HomiPlay」，PNG **341,116 bytes**。各已保存 snapshot JSON／PNG；這是 **installed RC11 Host UAT**，不再只是 source 診斷。
- 第一項跨 worktree `direct_snapshot` 用 HomiPlay root 讀 Oprueba lease，回 `BROWSER_LEASE_OWNER_MISMATCH`，**PASS（正確拒絕）**。下一項 Oprueba lease 的 `direct_navigate(https://unapproved.oprueba.com)` 回 `external transport fence unavailable or mismatched`，**沒有授權越界成功**。Source `external_direct_cdp_dispatcher.go` 的 URL allowlist 在 CDP Page.navigate 之前拒絕；`external_fence_series.go` 對 operation error 將該 worker series 關閉，這與此次後續該 lease fence 不可用一致。此因果仍是依 source 解讀，不將通用 fence error 認定為觀察到任何未核准 Page.navigate frame。
- 驗收 helper 把有意造成 fail-closed 的 origin 負向操作排在其後的同 lease `www` 正向流程**之前**，所以在腳本再操作 Oprueba lease 時收到 fence mismatch 而中止；之後的 `direct_evaluate` 只得到已失效 fence 拒絕，**不能算獨立 JavaScript/M8 禁止 gate**。兩個 lease 的 release 均回 `cleanup_state=complete`，Oprueba 帶 `cleanup_error`（page close 未明確 ACK），HomiPlay release complete；Broker 0 owners／leases／workers／orphans。
- 接著讀取同一 Core PID 的 9222 socket 仍為 `ESTABLISHED`，沒有另建 Edge socket。試圖只從該**原有**連線 fresh acquire 來測剩餘 `https://www.oprueba.com`、`https://www.homiplay.io` 與 sibling surviving release，但第一個 acquire 在 peer verification 即 fail-closed，沒有取得新 lease 或進行 `www` 導覽；所以**四站與 sibling gate 仍 open**。只觀察到 TCP established，不能當成可繼續 dispatch 的權限或 peer attestation。
- Core-owned socket 最後經正式 `browser_broker disconnect_external` **PASS**，已確認沒有 active worker／lease／operation，9222 的 established TCP 消失；未關閉使用者 Edge、未動 Profile、未自行重試同意或安裝。NEXT 8767 health=rc.11、Stable 8765 health=1.1.0 都正常。
- 首個成功網站流程的 observer **137 個完整樣本** foreground 均為 `com.google.Chrome`、Edge window ID set 不變；獨立正式 connect 的 observer 16 筆也維持 Chrome foreground，但新增 Edge window ID，不能斷言是何種原生同意 UI；故只把網站操作期間無 foreground change 列為 PASS，不擴大為整體原生 dialog／Tray UAT。
- 私有原始證據位於 `/private/tmp/agentdock-next-edge-verification-y6b6836m/`：`edge-rc11-confirmed-consent-connect.json`、`edge-runtime-rc11-sites.json`、`rc11-oprueba-snapshot.json`／`rc11-oprueba.png`、`rc11-homiplay-snapshot.json`／`rc11-homiplay.png`、`edge-rc11-www-sibling-result.json`、`edge-rc11-foreground.jsonl`。公司資料沒有寫入網站；不把工作樹／tmp 實物或擷圖放入對話作公開附件。

## 23:50 最終正向 UAT：四站／sibling／網站 no-focus PASS，負向-only acquire OPEN

| Verified installed action | Evidence |
| --- | --- |
| `browser_broker connect_external` | **PASS 4.495s**；已安裝 Core `0.9.2-rc.11`／source `c331afe47d2e`，只有一個正式連線，無其他 connect／short probe |
| `https://oprueba.com` | navigate／AX RootWebArea `O'Prueba`／PNG **302,838 bytes PASS** |
| `https://homiplay.io` | navigate／AX RootWebArea `HomiPlay`／PNG **336,743 bytes PASS** |
| `https://www.oprueba.com` | navigate／AX RootWebArea `O'Prueba`／PNG **303,523 bytes PASS** |
| `https://www.homiplay.io` | navigate／AX RootWebArea `HomiPlay`／PNG **327,591 bytes PASS** |
| Cross-workspace ownership | HomiPlay root 讀取 Oprueba lease 回 `BROWSER_LEASE_OWNER_MISMATCH`，**正確拒絕 PASS** |
| Sibling release | Oprueba A release `complete`／無 cleanup_error，HomiPlay B 繼續 snapshot PASS，再 B release `complete`／無 cleanup_error，**PASS** |
| macOS no-focus | 206 筆完整 foreground samples 均 `com.google.Chrome`；Edge 及 NEXT window IDs 的 set 未改，**網站工作期間 PASS**，不涵蓋 native Tray |
| Fresh acquisition after both released | **FAIL CLOSED**：額外 negative-only lease 的 `direct_acquire` 回 `external peer verification unavailable or mismatched`；先前正向結果不撤銷，但故障階段未定位，不可宣稱最後兩項負向已執行 |
| Owned cleanup | Broker owners／leases／workers／active／queued／orphans 全 0；`disconnect_external` 真實 PASS，Core PID 94893 對 9222 不再有 socket；NEXT 8767／Stable 8765 health 均 PASS |

原始證據：`/private/tmp/agentdock-next-edge-verification-y6b6836m/edge-rc11-final-connect-result.json`、`edge-rc11-final-positive-first-result.json`、`edge-rc11-final-foreground.jsonl`、四組 `final-rc11-*-snapshot.json`／`.png`，原檔 0600、沒有覆蓋前輪 RC11 證據。**不把 PNG 圖片嵌入 commit 或公開分享。**

Source 改動：在 `internal/tool/browser/next_edge_runtime_test.go` 增加 `TestNativeEdgeFreshLeaseAfterAllOwnersRelease`：兩個 owner 的頁面全部釋放後，以原連線建立第三個獨立 lease，確認沒有重新 dial、頁面 cleanup 次數及無外部程序終止。既有 source tests 與該定點測試、相關 `-race` selection **4 PASS**；它使用合成 CDP peer，**不能證明 Host 上 release 後再次 acquire 的原生驗證已修復**。

**Source 完整回歸：** 同輪 `GOMAXPROCS=2 GOFLAGS=-p=2 go test ./internal/tool/browser ./internal/browserpolicy ./internal/config ./internal/app -count=1 -json` 結果 **1,685 PASS、7 個既有 opt-in SKIP、0 FAIL**，定點 native fresh acquire／peer loss／共享連線／explicit reconnect 的 `-race` selection **PASS**，`git diff --check` PASS。證據 `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc11-final-source-tests.jsonl`。新增 regression 是 source 測試變更；**目前已安裝 RC11 仍來自 c331afe47d2e，沒有重建／升版／重啟**。

### 下次 Host UAT 正確排序

在**再次取得明確授權且真實連線成功**前，先從 source／離線測試改正驗收腳本的排序。用同一 Broker/socket 先完成四個已核准 origin 的 navigate／AX／PNG、A release 後 B snapshot／release，再在專用 lease 上做會不可逆 retire series 的失敗操作。跨 owner、未核准 URL 及禁止 JavaScript 要分別記錄；一個失敗 lease 不得被重用來做其他 gate。不得為補救錯誤腳本而自動重連或反覆跳出原生授權。成功連線後才能重新做跨 session、取消／故障與 native no-focus。

**續作實作：** 已在原有私有 `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc11-site-uat.py` 直接修正測試排序：四個正向站點優先，接著 cross-owner 拒絕、A release→B snapshot→B release，最後對禁止 JavaScript 與不核准 origin **各使用一個專用 negative-only lease**；不再於失效 lease 上測 `www`。已執行 Python AST syntax check PASS、檔案保持 0600；**尚未執行新排序的 Host UAT**，不將語法驗證當成實機 PASS，也不會自動 connect。

## 獨立 gates：真實／source 不混淆

| Gate | 本輪證據 | 尚未通過 |
| --- | --- | --- |
| GitLab NEXT-only MCP OAuth | **2026-10-24 新驗收 PASS**：`gitlab-company` ready／35 tools，Project／Commit／Pipeline／Job／MR／Branch 唯讀 Host API PASS，未複製 Stable grant。[詳見](next-rc11-gitlab-host-uat-2026-10-24.md) | GitLab REST `read_api` token／OAuth renew/revoke 或更廣泛權限尚未另外驗收 |
| native GUI／Tray | NEXT Computer Broker 真實 `capabilities` PASS，`foreground=forbidden`、`focus_changed=false`；`list_windows(app=AgentDock Next)` PASS 但回傳空視窗；`get_app_state` 為 `window_not_found`。一次 `open -g -a` 不改 Edge foreground，也沒有使可見視窗出現；全部 Computer sessions 已 release。 | RC11 可見 GUI／native Menu Bar 實際動作未驗收；Orca provider `supports.surfaces.menubar=false`，不能冒稱 tray PASS |
| lifecycle／取消／異常 | 新一輪 `go test -race ./internal/tool/browser` 定點測試 **113 PASS／0 FAIL**，包含 native Edge generation／peer loss／lease cleanup。正式失敗後 Browser 實機資源數回零。 | Edge 本次未有已授權租約，故生產四站、雙租約取消／peer loss、跨 Session／crash／恢復仍 open |
| App／ZIP／DMG／rollback | 安裝來源與 binary hash／signature PASS；`test-next-shared-deploy.py` **29 PASS**、`test-next-shared-package.py <RC11 app>` **9 PASS**，candidate bundle validation PASS；RC10 rollback bundle 確實存在且版本吻合。 | 本輪沒有對 **live RC11** 作實際 post-swap failure／rollback；先前 P11 RC2／RC3 的真實注入回滾不能冒稱為 RC11 live rollback |
| Stable separation | Stable 8765 及 1.1.0 health 原樣；未改 Stable、未 touch Edge Profile 或複製 credential | 最終發佈 gates 仍受上述 Host 結果阻擋 |

## 下一步（只在真正前置條件成立時）

1. **四站／sibling／網站 no-focus 已 Host PASS**；不要重做已成功的項目。優先定位**最後一個 lease 正常 release 後，新的負向-only acquire 在真實 macOS peer revalidation 失敗**；source fixture 仍 PASS，不能把該結果當成 Host 修復。日後再做獨立禁止 JS／未授權 URL 負向 gate，不能以失敗為由背景輪詢 connect 或短 probe。
2. 等使用者完成 NEXT-only GitLab local OAuth callback，再用該獨立 grant 驗證唯讀 MCP API；GitLab REST 如需 `read_api` 需獨立授權，不借原版憑證。
3. GUI 實機需要可見視窗的明確非搶焦點驗收路徑；目前無法在 Orca 無 menubar 能力下宣稱 Tray。
4. 在真正 NEXT 閒置、授權互動已結束後，安排 NEXT-only live failure rollback 與 recovery。完成必需 Host gates 才進行整合／合併，**不 push**。

本次主要改善是驗收事實與 owned cleanup 的可追溯性；沒有將失敗的 Edge socket 轉述為產品功能已可用。
