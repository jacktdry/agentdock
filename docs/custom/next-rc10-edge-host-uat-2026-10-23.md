# NEXT RC10：正式 Edge 連線與部分網站 UAT

> **更新入口（2026-10-23 夜間）：** [RC11 唯一一次正式 Edge connect 失敗及獨立 gates](next-rc11-host-uat-2026-10-23.md)。RC10 的成功導覽／部分截圖故障為歷史證據；RC11 新連線於 socket/WebSocket fail closed，**四站仍未通過，沒有自動重新授權**。

狀態：**安裝及正式 connect PASS；雙租約取得、Oprueba 導覽／AX snapshot PASS；
安裝版 screenshot FAIL CLOSED，四站完整 UAT 未完成。**
後續 source Host 診斷已定位 0×0 viewport 並成功截圖，不能替代安裝版 UAT。

## 來源、安裝與設定

- Source `c389f2361e8a15abb84b0752aa576d48cca3825a`，乾淨的
  `feature/next-edge-rc-20261023`；主線仍為 `7e87605a`，未合併／push。
- 候選 `dist/next-shared-arm64.Llrl8Z/package/`，Core `0.9.2-rc.10`／
  `c389f2361e8a`，App `0.9.2`／`11.0.0`，ad-hoc signature 驗證 PASS。
- ZIP SHA-256 `99fbea1c5c6c9bb350350bb72fd47088ac592dcca0ee442de2a4a38cea71361f`。
- DMG SHA-256 `6e950a20bb2385aa824f4ab4a3ecd939eab15b44bc5b51908603b024605f5594`。
- 真實 package suite 9 PASS；ZIP 與唯讀 DMG 的 24 個 App 檔案逐一與候選 SHA
  相同，本工作建立的 mount 已卸載；DMG checksum valid。
- 部署沿使用者既有「確認 NEXT 閒置後安裝與驗收」授權；再次核對 Browser／Computer
  無 active resource、ACP loaded/ready/running=0、無 pending interaction/operation、
  execution active_calls=0，才使用 stable Mac-Dev 執行 NEXT-only helper。
- helper 成功，RC9 rollback bundle 保留於
  `/Users/wei/Applications/.agentdock-next-replacement-wa8n0dz3/AgentDock Next.app`。
  App 根 inode 保留；已安裝 24 檔與候選相符、26 個既有 Skill 檔 SHA 未改。
  原版 8765 仍為 `1.1.0`，沒有修改／重啟。
- 真實 same-socket version metadata 觀察到 `Default` 後，NEXT-private route file
  已保存明確 `profile_directory: Default`，保留原四個 exact workspace／origin policies，
  owner-only 0600／非 symlink。沒有更動 Edge 自己的 Profile／設定／憑證。

## 真正 installed Broker 結果

`browser_broker connect_external` **3.900 秒成功**，Core 保留同一條已授權連線。
另一個 NEXT MCP client 隨後成功取得 Oprueba 與 HomiPlay 的兩個不同 lease，
證明成功的 connect 不依附短命 client session。

Oprueba lease 成功導覽 `https://oprueba.com` 並取得真實 AX snapshot，RootWebArea 為
`OrpheLink — 安全、穩定的遠端連線與裝置管理方案 | O'Prueba`，包含產品、導航及隱私提示。
接著 screenshot 失敗，transport fail closed。未開始其餘三個 origin；負向 scope、
釋放 A 後 B 存活等後續步驟也未執行，不能宣稱完整流程或四站通過。

兩個已取得租約均回覆 cleanup_state=complete，同時保留 peer-loss cleanup_error；
最後 snapshot 為 idle、owners/leases/workers/active/queued/orphans 全 0。
這是 logical lease retirement，不把異常情況說成收到每個 target 的 close ACK。
隱藏 target 依原連線生命週期處置，沒有換 socket 發出舊 target ID 的 close。

被動觀察取得 **84 個完整樣本**，foreground 一直為 iTerm2，Edge native window ID set
未變。停止取樣時最後一筆 JSON 未完整寫出，已排除；這只涵蓋本次部分流程，
不當成四站或完整原生 GUI no-focus PASS。

## 已定位的實機問題及 source 修正

1. Edge hidden WebContents 回報 type=`other`；只對本 dispatcher 成功建立的 hidden
   target 相容，exact ID／session／context／origin checks 保留（RC9 已部署）。
2. 版本頁提交前短暫 URL 為空；只等待自有版本頁，不 evaluate 未提交或其他 document
   （RC10 已部署）。明確 Default pin、native process/socket/Profile epoch/peer 全流程 PASS。
3. Page.navigate 回覆可能早於文件提交。後續 source 在同一 gate 內排空 scoped Fetch events，
   等待 owned target 的 approved document；有 loaderId 時核對該 loader 的 frame tree，
   取消或不明 URL/context 一律 retire transport，沒有額外 reader／重連。
4. 實機 layout metrics 為 **0×0**。僅對自有 target session 設定虛擬 1280×800、scale=1、
   mobile=false 後，source Host 診斷 screenshot **121 ms** 成功，PNG **301,409 bytes**，
   已目視確認真實 Oprueba 首頁；post-peer 驗證與 owned target cleanup、relay join 成功。
   source 修正把固定 virtual viewport 放在 native page 導覽前，不操作原生視窗尺寸或焦點。

使用者詢問為何反覆按偵錯允許後，**停止建立新的 Edge 連線**；不再以每次換版後
一個新 probe 的方式要求同意。完成 source/offline 修正與候選後，下一次應以單一正式
Broker 連線完成剩餘實機流程並保留 socket；正常頁面釋放不得造成另一次授權。
Core／Edge 重啟或真正的身分／transport 失效仍須重新明確連線，不能承諾永久免同意。

## 其他已驗收與 Host gates

- NEXT Computer Broker 的 observe／foreground=forbidden 實機 acquire、capabilities、
  Next-only list_windows、release PASS；前後 iTerm2、focus_changed=false、cleanup complete。
  NEXT GUI PID 33739 回覆無可見視窗，未強制 restore/focus，沒有聲稱新 GUI 已驗收。
- `gitlab-company` 在 NEXT 獨立 registry 已註冊，實際 refresh=auth_required；
  OAuth grant／API repository/commit/pipeline/job/MR UAT 仍待使用者獨立授權。
- 四站、跨工作樹／Session、取消／故障／恢復、原生 GUI／Tray、完整安裝回滾仍 open。
  保留既有三真實 Agent Managed Chrome PASS，不重做或改寫為 Edge PASS。

私有證據在 `/private/tmp/agentdock-next-edge-verification-y6b6836m/`，包含 RC10 manifest、
archive/install verification、正式 connect/site 結果、foreground samples、source Host diagnosis
及 PNG；沒有存放 NEXT／原版憑證。臨時 Host tests 只編譯到私有目錄，source 已移除，
所有診斷／observer 命令均已退出；沒有建立新 Edge Profile、公司瀏覽器或常駐診斷服務。

## RC11 接續 checkpoint

已沿既有授權，在再次確認 NEXT 閒置後安裝 **0.9.2-rc.11**，沒有執行 Edge connect。
同 commit App／ZIP／DMG 的 source 為 **`c331afe47d2e144d930266d99486ca1b4f40c335`**，
候選在 `dist/next-shared-arm64.oSRRh5/package/`，App `0.9.2`／`12.0.0`。
此後的文件 checkpoint commit 不等於 binary source commit，不能混報。

- ZIP SHA-256 `38d09fc6cb6c04c38e12237c3b98b25ae959677533e88b8bca55f220e16539c9`。
- DMG SHA-256 `b60e8148e6ecc4f8c844ea99a910f5497ede2328f4aa0de5144a783ffd24568c`。
- package suite **9 PASS**；兩個 archive 的 24 個 App 檔案均與候選逐一 SHA 相同，
  唯讀 DMG mount 已卸載、checksum valid；ad-hoc signature PASS。
- source 四包 Go **1,684 PASS／7 opt-in SKIP／0 FAIL**，相關 browser Race selection PASS，
  四包 vet PASS，候選建置再次執行 Vue typecheck／Vite build PASS。
  Vue 未修改，保留本輪先前 Vitest **24 files／176 tests PASS**；未宣稱每個候選都重跑 Vitest。
- 已安裝 24 檔 SHA 與候選相同、App 根 inode 保留、26 個既有 Skills SHA 未改，
  NEXT helper bootstrap 成功；RC10 rollback bundle 保留於
  `/Users/wei/Applications/.agentdock-next-replacement-05ac6r44/AgentDock Next.app`。
- 真實 Core **PID 94893**、launchd **top-level running + positive matching PID**，
  helper 再核對 binary／listener identity；runtime version rc.11、private home 正確。
  該 PID 對 9222 **沒有 established connection**，Broker idle、resources/operations/orphans=0。
- 原版 health 仍為 `1.1.0`；沒有操作原版服務。全部暫時 Host test／observer process 已退出。
- live route SHA `83ebae0ebb75d9799f347d5dd8988a42ad9ecc3e3d9f92842f784a49ac5f0053`，
  Default pin／四 root policies 未改。原版憑證沒有複製至 NEXT。

下一個安全動作：在使用者能處理**一條新連線**的原生同意時，使用 NEXT 自己的 client
對 Oprueba exact root 呼叫 `browser_broker connect_external`，client timeout=120 秒，
native consent window=90 秒。成功後**保留 Core／socket**，再執行私有
`/private/tmp/agentdock-next-edge-verification-y6b6836m/rc11-site-uat.py`。
此 helper 已 syntax check、**未執行**，有 rc.11 version guard 與逐步 operation evidence，
不會建立 Edge 連線，也不會自動重連。它會釋放自己取得的租約；最終仍需驗證四站內容、
foreground samples 覆蓋及 owned cleanup，再明確 disconnect 收尾。

RC11 GUI 尚未重開或取得可見畫面；GitLab 獨立 OAuth／API UAT、跨 Session／異常恢復、
完整 rollback 及主線整合未完成。工作樹維持 `feature/next-edge-rc-20261023`，沒有 push、
公開 release／tag／feed，也沒有其他正在執行的驗收命令。原生已安裝 NEXT 服務保留運作。

## 文件／記憶同步（2026-10-23 晚間）

- 工作流程／痛點：避免新對話根據歷史 `provider=nil`／RC5 敘述重做整合或反覆要求偵錯同意。
- 直接改善／最小改動：更新 Backlog／Roadmap 入口與現況文件、標明歷史敘述，保留本文件為
  RC10／RC11 的可追溯實機證據；記憶只新增可重用經驗的更新筆記，不複製 PID／版本待辦。
- 實際複核：22:41–22:42 Asia/Taipei，RC11／source c331afe47d2e、24 檔 SHA／簽章、
  trusted Core PID／top-level running、Default pin／四 policies 與原版 health 均相符；
  NEXT idle、active_calls=0、沒有 established Edge socket。本次沒有重啟／重連／部署。
- CBM 只讀查核：mainline `agentdock-custom` index 仍為 2026-10-14T08:53:43Z；
  `next_edge_runtime.go`／`next_edge_operation.go` coverage missing，docs excluded by design。
  此 index 不能代表未合併 feature worktree；未改寫共享索引，後續以 current source 查核。
- 驗證／不做：本次只改文件及獨立記憶更新筆記；檢查 diff／本次改動的相對連結，
  不重跑產品測試、不重建 RC、不開新 Agent／Edge／GUI、不改原版、不合併或 push。

下一輪先讀 `AGENTS.md`、`next-local-rc-acceptance.md` 與本文件的 RC11 checkpoint，
再核對 worktree／binary source／服務與 scope。以一條正式 NEXT Broker 連線完成四站、
負向 scope、sibling release 及 no-focus，再進行 GitLab 獨立 OAuth/API、原生 GUI／
生命週期／rollback 等剩餘 gates；不得因舊 source 診斷成功就提前宣稱 RC 完成。
