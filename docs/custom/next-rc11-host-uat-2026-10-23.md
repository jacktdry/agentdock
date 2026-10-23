# NEXT RC11：2026-10-23 本機實機驗收續作

更新：2026-10-23 23:01 Asia/Taipei。**RC11 尚未達 Definition of Done；沒有合併、push 或公開發布。**
本文件是 RC10／RC11 續作證據，保留原始 [RC10／RC11 安裝記錄](next-rc10-edge-host-uat-2026-10-23.md) 與 [RC 驗收清單](next-local-rc-acceptance.md)。

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

## 唯一一次正式 RC11 Edge Broker 連線：FAIL CLOSED

- 進場時 NEXT Broker `idle`、authenticated transport owners／leases／active operations 均為 0，Core 對 9222 無 existing socket。已向使用者**預告一次**原生「允許偵錯」；不自動替使用者點擊。
- 使用 NEXT 本機已授權 MCP client 呼叫 `browser_broker(action=connect_external, workspace_root=<Oprueba exact root>)`，timeout 120 秒，native consent window 90 秒。
- **90.892 秒後失敗**：`BROWSER_REQUIRED_ROUTE_UNAVAILABLE`，階段 `socket_and_websocket`。期間曾觀察到 Core PID 94893 至 127.0.0.1:9222 的 ESTABLISHED TCP，但沒有 Broker 成功 connect／四站操作；不能把 TCP 連通等同 WebSocket admission 或同意完成。
- 被動 foreground observer 有 285 筆完整樣本，均為 `com.google.Chrome`。Edge window ID 集合從 11 變 12，新增一個 ID；**這不是可歸因的對話框內容或授權結果證明**。未執行 navigate、snapshot、PNG、雙租約與 sibling release；四站 Host UAT **仍 open**。
- **沒有進行第二次 connect 或 source probe。** 事後 Broker 回到 idle、owners/leases/workers/active/queued/orphans 全 0、authenticatedTransportOwners=0；Core 已沒有對 9222 的 TCP 連線。Stable 與 NEXT health 各為 1.1.0／0.9.2-rc.11。
- 失敗 JSON、foreground JSONL 留在私有測試目錄 `/private/tmp/agentdock-next-edge-verification-y6b6836m/` 的 `edge-rc11-connect-result.json`、`edge-rc11-connect-foreground.jsonl`。未存瀏覽器 cookie、密碼或 Profile 複製品。

## 獨立 gates：真實／source 不混淆

| Gate | 本輪證據 | 尚未通過 |
| --- | --- | --- |
| GitLab NEXT-only MCP OAuth | `gitlab-company` registry 存在；真實 `refresh` 回 `MCP_AUTH_REQUIRED`；`authorize` 發現 `agentdock`／`local` callback，依本機 8767 選定 local 並產生授權頁，等待使用者同意。沒有複製 Stable grant。 | 尚無完成 grant，不能執行 repo／commit／pipeline／job／MR API UAT；MCP `mcp` scope 也不是 GitLab REST `read_api` token |
| native GUI／Tray | NEXT Computer Broker 真實 `capabilities` PASS，`foreground=forbidden`、`focus_changed=false`；`list_windows(app=AgentDock Next)` PASS 但回傳空視窗；`get_app_state` 為 `window_not_found`。一次 `open -g -a` 不改 Edge foreground，也沒有使可見視窗出現；全部 Computer sessions 已 release。 | RC11 可見 GUI／native Menu Bar 實際動作未驗收；Orca provider `supports.surfaces.menubar=false`，不能冒稱 tray PASS |
| lifecycle／取消／異常 | 新一輪 `go test -race ./internal/tool/browser` 定點測試 **113 PASS／0 FAIL**，包含 native Edge generation／peer loss／lease cleanup。正式失敗後 Browser 實機資源數回零。 | Edge 本次未有已授權租約，故生產四站、雙租約取消／peer loss、跨 Session／crash／恢復仍 open |
| App／ZIP／DMG／rollback | 安裝來源與 binary hash／signature PASS；`test-next-shared-deploy.py` **29 PASS**、`test-next-shared-package.py <RC11 app>` **9 PASS**，candidate bundle validation PASS；RC10 rollback bundle 確實存在且版本吻合。 | 本輪沒有對 **live RC11** 作實際 post-swap failure／rollback；先前 P11 RC2／RC3 的真實注入回滾不能冒稱為 RC11 live rollback |
| Stable separation | Stable 8765 及 1.1.0 health 原樣；未改 Stable、未 touch Edge Profile 或複製 credential | 最終發佈 gates 仍受上述 Host 結果阻擋 |

## 下一步（只在真正前置條件成立時）

1. 讓使用者完成 **一次正常的 Edge 原生授權**後，再做一個明確獲准的正式 Broker 驗收連線；使用既有 `rc11-site-uat.py` 在成功連線後沿同一 socket 完成四站及負向／sibling／focus 證據。**不要以失敗為由背景輪詢 connect 或短 probe**。
2. 等使用者完成 NEXT-only GitLab local OAuth callback，再用該獨立 grant 驗證唯讀 MCP API；GitLab REST 如需 `read_api` 需獨立授權，不借原版憑證。
3. GUI 實機需要可見視窗的明確非搶焦點驗收路徑；目前無法在 Orca 無 menubar 能力下宣稱 Tray。
4. 在真正 NEXT 閒置、授權互動已結束後，安排 NEXT-only live failure rollback 與 recovery。完成必需 Host gates 才進行整合／合併，**不 push**。

本次主要改善是驗收事實與 owned cleanup 的可追溯性；沒有將失敗的 Edge socket 轉述為產品功能已可用。
