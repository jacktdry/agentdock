# NEXT RC10：正式 Edge 連線與部分網站 UAT

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
