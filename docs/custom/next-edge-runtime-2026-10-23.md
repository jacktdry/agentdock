# NEXT 原生公司 Edge Runtime 整合

狀態：正式 Core 呼叫路徑已接入並安裝 RC6／RC7；公司網站 Host UAT 尚未通過。
90 秒 explicit consent window 修正等待下一個候選驗證。
這份文件接續 [RC 執行清單](next-local-rc-acceptance.md)，不取代歷史實機證據。

## 使用流程與界線

公司工作原本無法由 NEXT Broker 取得可信租約，診斷短連線還會重複要求使用者允許偵錯。
現在沿用既有 planner、ExternalLeaseManager、M8、epoch/peer 驗證及 operation fence，
由 NEXT Core 明確連線一次，讓直接 ChatGPT 與 ACP 租約共用同一 socket。
成功標準仍是四站唯讀導覽／snapshot／screenshot、精確 owned release、無焦點變動，
以及原版、使用者頁面及其他租約保留。離線 WebSocket fixture 不算此驗收。

不建立 Profile、不啟動 Edge、不列舉使用者頁面、不讀 cookies／Local State、不修改
全域下載或視窗設定，不自動重連、不回退 Managed Chrome。公司寫入及任意 JS 未開放。

## 啟用與正常使用

1. 新 source 只接受 macOS `next`、實際 `~/.agentdock-next`、8767、Browser/ACP enabled、
   非 stdio Core。未配置 native driver 的既有 Managed Chrome 行為保留。
2. [可審查的設定範例](next-company-browser-routes.example.json) 僅包含四個已核准工作樹及
   各自的 exact HTTPS origin pair；CMS 可使用兩組。**尚未寫入 live 設定**。
   既有 private loader 仍要求 home 0700、檔案 0600、owner／非 symlink、嚴格 JSON。
3. `browser_broker` 的 `connect_external` 加 `workspace_root` 明確啟動連線。
   status、planner、acquire 均不啟動或重建 socket。Edge 原生同意由使用者操作。
   explicit Core connection 最長等待原生同意 90 秒，仍可取消；status／admission 不使用
   此寬限，也不開啟連線。成功後連線持續到明確 disconnect、Core shutdown、來源身分變更或 I/O 不確定。
4. `direct_acquire`、`direct_navigate`、`direct_snapshot`、`direct_screenshot`、
   `direct_release` 必須維持同一 authenticated principal、exact configured root 及 lease。
   ACP 使用既有 session token 及 task/session scope。每個 logical worker 僅看見自己的
   page 1，實際 target/session ID 不會成為呼叫者可指定的參數。
5. `disconnect_external` 在仍有租約時拒絕；正常 release 只關閉該租約的 owned hidden
   target，其他租約及共用 socket 保留。取消若已送出指令，會 retire 整條 transport
   generation；其餘租約亦需重新取得，不宣稱此情形能無損保留 sibling。

## Profile 與 held-socket 證明

使用者選擇的是現有名稱「公司」的 Profile，並表示應只有一個。設定中的名稱是
**owner selection，不能充當獨立的登入證明**。`bind_current_profile: true` 代表明確綁定
這次已選取的現有 Profile；不得同時指定 `profile_directory`。亦可改用明確的
`Default`／`Profile N` directory pin。

Core 先驗證 Edge 程序、Microsoft signer、啟動 epoch、預設 data root 與實際 TCP reverse
tuple，再使用同一 socket 及 admitted default context 建立 owned hidden `edge://version/`
頁面，只讀固定 Profile-path metadata，確認 directory 及 ancestor identity，隨即關閉。
首次觀察的 directory 會在此 Core 生命週期內 pin；明確重連也不能跟隨其他 current Profile。
Core 重啟後需再次明確 connect；要跨 Core 重啟固定 directory，應將實際觀察值存為
owner-reviewed `profile_directory`，不能把記憶體 pin 說成已持久化。

每次 qualification／operation 前後重新觀察 process/Profile epoch 及**這條 held socket**
的 OS ownership；無法核實即關閉 transport 並隔離結果。五秒 sealed observation 與一次性
grant 不能由 config/status booleans 偽造。停止時若身分已變，不會送出舊 target ID 或 kill PID。
網站的實際公司登入／可讀內容仍需 Host UAT，並非 signer 或 Profile path 能證明。

## Origin 與操作範圍

每租約只可縮小 root 的允許 origins。建立 blank owned target 後，在首次網站導覽前
啟用 target-session 的 Fetch Document request/response interception：拒絕非 allowlist hop、
非 GET/HEAD document、attachment response；HTTP auth challenge 一律取消，不提供憑證或彈窗。
事件依事件所屬 session 的 immutable scope 處理，不借用目前呼叫者 scope；control ACK
到齊前不返回操作結果。釋放後的 session 不再發出 control。

此範圍限制文件導覽與 agent 的唯讀操作，**不是隔離整個網站的 network sandbox**：
正常網站 scripts/subresources 仍使用該 Profile 的瀏覽器行為。隱藏 target 的原生 popup、
下載、no-focus 行為需在 Edge 實機驗證，不以 Chromium 結構或 fixture 代替。

## 離線驗證與剩餘 gate

- 真實 loopback WebSocket fixture 經 production planner/bridge/manager/fence，覆蓋同一
  連線的 ChatGPT/ACP ownership、page 隔離、owned release、active disconnect 拒絕、
  身分改變拒絕舊 ID、明確重連的 Profile pin 與 generation 隔離、pending consent 取消。
- Fetch 測試覆蓋 exact origin、外站 redirect hop、POST、attachment、HTTP auth、sibling
  scope、ACK 先後及取消、retired/unknown session。OS attestation 在這些測試中是 synthetic。
- Broker diagnostics 納入 native logical workers；authenticated direct lease 不再誤報 unowned。
  GUI 仍不宣稱觀察到 connector health；沒有新增 background connect/poll。
- 仍需新 candidate 的 installed Broker 四站 UAT、實機 Profile pin、background/no-dialog、
  多租約取消／逾時／Edge 自行退出／重連、GUI 與 RC 更新／回滾；沒有 PASS 證據的項目保持 open。

GitLab 不在此瀏覽器 allowlist；API／獨立 OAuth/Token gate 獨立處理。不得為讓網站 UAT
通過而加入萬用子網域、複製原版憑證或改用新 Profile。
