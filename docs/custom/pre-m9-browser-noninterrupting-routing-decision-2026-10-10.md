# AgentDock Next P8 — Non-interrupting Browser Routing Decision

日期：2026-10-10（Asia/Taipei）；依使用者約 01:10 的明確更正。
性質：**權威產品／架構決策；已有 private opt-in 離線 spike，尚無 runtime wiring、部署或 live UAT**。文件修正基礎：`e8585929`；spike 基礎：`376cf23c`。

> **SUPERSEDES** [舊 no-remote-debugging 決策](pre-m9-browser-no-remote-debugging-decision-2026-10-10.md)。
> 舊文件把需求誤解為全面禁止 Remote Debugging；正確要求是 UAT 不得干擾使用者日常桌面操作。
> Remote Debugging／CDP 本身允許；安全同意不得繞過或壓制。本次未接觸任何使用者瀏覽器。

## 1. 首要需求與路由

ChatGPT 與 AgentDock child ACP 的 UAT 必須持續推進，不搶焦點、不要求反覆瀏覽器權限對話框，
不為每個 ACP 啟動競爭的瀏覽器，也不等待「Allow debugging」而停滯。
需先取得的設定／Profile／網站授權應在明確授權的準備階段完成；不能把準備授權視為永久瀏覽器同意。
若執行時必要核准不可避免，回報 blocker；只能使用另行明確授權的替代整合。

| 情境 | 必要路由 | 限制與失敗處理 |
| --- | --- | --- |
| 一般 ChatGPT／ACP UAT | 同一 Broker 管理的 Headless Chrome engine；chrome-devtools-mcp 可用 CDP | owned process 應有零使用者提示；不得每 ACP 新開 browser |
| 公司 CMS | 既有、已登入且經授權的指定 Edge Profile | 保留 portal 登入；無有效證據或需反覆同意時 fail closed，不改用 managed Chrome |
| 明確「use my Chrome profile」 | 該 exact Chrome Profile | 不改用另一 Profile、Edge 或 managed Chrome；不得複製 Profile 冒充原 Profile |
| 使用者 Profile 的 CDP | 已授權、驗證符合不干擾條件的既有 attachment | CDP 允許；反覆 modal 的 autoConnect 禁止作 unattended UAT 路徑 |

路由必須驗證 engine、Profile、既有連線身分、網站授權及 no-focus 能力。
設定、目錄身分、endpoint 可達、retained lease 或 worker ready 都不等於已登入／授權／可安全操作。
找不到正確 Profile、登入失效、能力未知或證據過期時，回報 bounded blocker，不做 silent fallback。
不得未經同意重新啟動、替換或關閉使用者日常 Edge／Chrome。

## 2. 共享 Broker／worker leases

目標拓樸：ChatGPT session + ACP A + ACP B → 一個 Browser Broker → bounded worker pool → 選定 engine／Profile。
預設 managed Chrome 共用一個 owned browser instance；外部 Edge／Chrome 按明確路由使用既有實例。
「共享 browser process」不要求所有 session 共用同一 page 或 browserContext。
它要求共享 Broker，並按 engine／Profile 限制 owned browser instances；Chromium 本身可有多個子程序。

- Broker 擁有 admission queue、worker／browser 上限與 engine／Profile route identity。
- 各 session 取得有 owner、期限與 generation 的 lease，再取得明確 owned tab／page。
- managed browser 可按需要隔離 browserContext；不宣稱指定使用者 Profile 的登入可搬到 isolated context。
- 每個 page 使用互斥 lock；同一 page 的選取、操作與回傳必須在同一受控範圍完成。
- worker 若只有全域「selected page」狀態，必須序列化該 worker；page lock 本身不足以防串頁。
- page/context routing 與版本相容性待 PoC；不能假設 MCP 自動提供多 client 安全共享。
- lease 釋放只清理該 owner 的 tabs／contexts；不關閉使用者 tabs 或外部 browser process。
- worker crash／lease 過期以 ownership + generation fencing 回收；unknown ownership 不清除。
- managed 程序、暫存目錄與資源配額由 Broker 統一追蹤；不以任意 endpoint 或 process age 猜測所有權。

歷史背景（使用者提供的既有已驗證紀錄，非本次重新量測）：2026-10-01
`~/.agentdock/browser/tmp` 曾有 29 個目錄、約 3.72 GiB。
2026-10-03 策略是由 Broker 共享 chrome-devtools-mcp worker pool 給 child ACP；不是 MCP 自動共享。
此決策延續避免 memory／disk bloat 的目的，不宣稱目前 runtime 已達成上述拓樸。

## 3. 連線策略與能力限制

### Managed Headless Chrome

chrome-devtools-mcp 的 `--headless` + `--isolated` 為 owned browser 的候選啟動方式，CDP 允許。
產品要求：其 own process 不應觸發使用者瀏覽器的 remote-debugging 同意，整段 UAT 必須零提示。
Broker 須重用該 instance，而非每個 ACP 各啟一份 isolated browser／目錄。
正常退出的自動暫存清理不等於 crash 清理保證；orphan reconciliation 仍需 ownership 證據。
managed 啟動、操作、重連及清理的零提示能力仍須實測，不以 flags 當驗收證據。

### Existing Edge／personal Chrome

Chrome `--autoConnect` 對每次 remote-debugging session 要求同意；不能假設 Chrome／Edge UI 同意永久有效。
長時間共享 attachment 可減少重連次數，但 crash／restart 後不能保證消除新同意。
若仍需反覆 modal，該路徑不符合 unattended UAT；不得自動點 Allow、隱藏提示或繞過安全同意。
Chrome 官方說明不等於 Edge 的行為證據；Edge 版本／平台／Profile 需獨立 UAT。

候選 A：事先明確授權、持續且穩定的 CDP attachment。
`--browser-url`／`--browserUrl` 透過 HTTP 發現 CDP endpoint；`--wsEndpoint` 指定 CDP WebSocket。
MCP client transport 與 browser CDP transport 是不同層；換 endpoint flag 並未移除 CDP。
手動設定的 persistent debugger **可能**避免 autoConnect prompt，但仍需平台／Profile 的 startup／reconnect UAT。
Chrome 136+ 在預設 user data directory 不接受 remote-debugging port／pipe flags；不能據此承諾 daily Profile 可直接使用。
若所選 Profile 的啟動條件不可滿足，不得偷偷改 user data dir、重啟或替換使用者 browser。

候選 B：Chromium Extension + Native Messaging，作使用者 Profile 零反覆提示路徑的 contingency。
需 bounded PoC 驗證安裝同意、精確 Profile、site permissions、background tabs、操作能力與重連。
這不是 managed Chrome 的強制替代，也不是已可用的自動 workaround；涉及 debugger 權限時仍遵守同意。
兩個候選都未獲 live acceptance；不可把 capability 假設寫成 verified／deployed。

## 4. 比例適當的 threat model

| 層級 | v1 要求 |
| --- | --- |
| S1：tab ownership／no-focus | lease／page lock 防串頁；背景操作不搶焦點；只回收 owned tabs；拒絕未授權目標 |
| S2：Profile／site permissions／consent | 精確綁定 engine／Profile／網站範圍；保護登入與資料；尊重 browser 同意及撤回 |
| S3：惡意 OS actor | 已控制本機 OS 的攻擊者不在 v1 防禦範圍；不以此理由取消 S1／S2 |

本機 debugger endpoint 必須有受限的暴露／存取邊界，不公開到不受信任網路。
日誌／renderer 不輸出 cookies、tokens、Profile paths、原始敏感 browser payload 或錯誤。
不把 Profile 目錄或 signed process 證據當作登入、網站授權或安全同意。

## 5. 可量測 acceptance gates（全部待驗證）

1. 兩個 ACP agents + 一個 ChatGPT session 經同一 Broker，合計完成至少 10 個連續 UAT 步驟；三者皆實際操作。
2. 全程記錄 user prompt = 0、focus steal = 0、等待 Allow debugging = 0；使用者可同時正常使用桌面。
3. 記錄 engine／Profile instance 與程序樹：managed instance 上限為一；新增 ACP 不新增 browser instance。
4. 交錯請求驗證 page lock／context isolation：無串頁、跨 lease 操作或未授權資料讀取。
5. 公司 CMS 命中指定既有 Edge Profile；explicit Chrome 命中 exact Chrome Profile；portal 登入及使用者 tabs 保留。
6. lease release／取消／worker crash 後僅 owned tabs 被回收；使用者 browser 不關閉、不重新啟動。
7. 正常結束與 crash reconciliation 後 task-owned orphan dirs = 0；記錄前後數量／容量與 memory，無逐 ACP 無界成長。
8. startup／restart 負向案例：錯誤 engine／Profile、失效登入、stale lease、未知 ownership、缺權限或 consent modal 均 fail closed。
9. 負向案例不得改路由／Profile 或自動核准；回報 blocker，替代整合須有獨立明確授權。
10. 各版本／平台分別保存 startup、reconnect、cleanup 證據；離線測試與設定不等於 live UAT。

初次決策修正只更正文件；當時未修改 Go／JS、未執行 build／tests／live UAT、未操作 browser、未部署。
既有 B2i～B2n-B checkpoints 保留歷史證據，不能自動轉成新決策的 acceptance。

## 6. 官方參考

- [Chrome DevTools agents configuration](https://developer.chrome.com/docs/devtools/agents/get-started/configuration)：headless／isolated 與 endpoint flags。
- [Debug your browser session](https://developer.chrome.com/blog/chrome-devtools-mcp-debug-your-browser-session)：autoConnect 每次 debugging session 同意。
- [Remote debugging switches security changes](https://developer.chrome.com/blog/remote-debugging-port)：Chrome 136+ default user data directory 限制。

## 7. P8-A/B initial shared managed worker spike（2026-10-10，SOURCE-ONLY）

基於乾淨 `376cf23ce80b1d8474b10eaf79d5e905adc13da4`、`feature/m8-permission-approval`。
新增 package-private `managed_shared_worker_backend.go` 與合成測試；**只由測試明確 opt-in 建構**。
`ACPBridge`、`ManagedLeaseManager`、exclusive coordinator、WorkerRegistry、managed launch config 及 legacy tests 都未修改。
production 仍是每個 exclusive lease 啟動／停止 worker；本 spike 尚未達成產品 live acceptance。

### Adapter 契約與必要偏離

- 底層只呼叫一次 StartManaged，保存 immutable underlying WorkerInfo／worker generation，透過同一 backend worker connection 序列化所有 Call。既有 Manager 建立各 lease 的 background page、唯一 named isolatedContext，驗證完整 owner scope 並注入 pageId；adapter 再檢查每個 borrow 的已知 pageId。
- **不向 Manager 回傳相同 WorkerID**：單純 refcount 無法辨認 paired Stop、cleanup recovery 或重複 Stop，可能扣掉其他 holder。採不修改 Manager 的 adapter：每次借用回傳唯一 opaque borrow token 作 WorkerID；底層操作映射到同一 pinned workerID。token 不可直接用於 WorkerRegistry，也不是獨立 process／worker generation；不能直接當 production worker DTO。
- 固定一個 canonical workspace root；不同 agent owner 可共享，不同 Cwd 拒絕且不派送底層呼叫。跨 workspace 共享路由／authority 未實作。
- Stop 只釋放指定 borrow，重複 Stop／recovery 不再次扣數。成功意味該 borrow 已知 page 已清理，**不是 shared browser 已停止**。Manager 既有 cleanup reason 仍寫 exclusive worker stopped；此文字不適合 shared runtime，故禁止直接 wiring。
- Manager 的 close_page 失敗時，Stop 以獨立 bounded context 再清理已確認 owned page；仍失敗則回報 cleanup failure。new_page error／malformed／duplicate-page result 無法排除未知副作用，禁止新借用、保留其他 active holders，不猜 page、不關 sibling worker；待 active=0 後手動 reap 或 explicit Shutdown 完整清理 owned worker。
- 預設 idle retention 30 秒，手動 ReapIdle(now)，沒有背景 timer。取消 startup waiter 不取消其他 waiter 共用的 startup；取消後已確認建立的 page 仍由 paired Stop 清理。startup 若無 holder，保留到 idle reap；維持者必須呼叫 reap／Shutdown。
- reap／Shutdown 永久封閉這個 adapter generation，不自動 restart 或重用 stopped worker。底層 Stop 全 generation 最多一次；錯誤保留、無隱式重試。borrow tombstones 上限 1024，達上限拒絕新借用；replacement／輪替不在本 spike。
- Shutdown 先禁止新工作並取消 startup／Call context，再等待 startup 完成與單一 connection gate，最後只 Stop 該 owned backend 的 StartManaged 回傳的 worker。ownership 依賴受信任 backend 契約，PID 不授予所有權。不碰 user Chrome／Edge／profile。Shutdown deadline 到期回報未完成；不合作 callback 無法強制中止，只保留一個受阻 callback 與一個 finalizer，callback 最終返回後完成一次清理，不能宣稱 deadline 即完成 cleanup。

### 本次證據與未驗證事項

唯一 focused selection：`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestManagedShared' -count=1 -timeout=100s` **PASS（0.548s），無 skip**。已 gofmt；git diff --check PASS。
TEST ONLY decorator 使用既有 fakeLeaseBackend，無 socket、真實 browser 或程序啟動。
三個並行 agent scopes（runtime + 兩個 ACP）經完整 Manager Acquire／Call／Release，實測底層 startup=1、不同 page／context／borrow identity、底層並行 Call peak=1、unowned page closes=0、wrong-worker calls=0；跨 workspace／跨 owner／caller pageId 不到達 backend。
涵蓋 repeated Release／borrow Stop、單一操作 failure 不殺 sibling、順序 idle reuse、active holder 不 reap、idle retention／terminal reap／Stop once、failed start／unknown new_page／malformed result／cancel after page／close failure／Stop error，以及 concurrent Acquire／Release／Shutdown、startup waiter 取消、不合作 startup／Call 的 bounded Shutdown 與 late cleanup。

**這是 architecture contract 與 opt-in offline proving；不是 actual chrome-devtools-mcp multi-client-safe 證據。**
主 Session 獨立驗證 PASS：Browser／BrowserPolicy 完整 Go tests、共享原型定點 `-race`、Go vet、Linux／Windows amd64 與 macOS arm64 無 CGO 編譯、gofmt／git diff check。尚未證明實際 Chrome DevTools MCP context／storage isolation、零 debugging popup／focus、owned temp-dir 清理或 crash orphan reconciliation。
沒有 real npx、Chrome／Edge、remote debug、GUI、app restart、install、deploy／push；未觸碰 AGY repo／stable。
P8／M9 gates、公司 Edge authority 與指定 Chrome Profile acceptance 仍未結案。

## 8. P8-B real headless proof harness（SOURCE-ONLY；isolated live proof PASS）

基於 `ffd67c08` 新增 `internal/tool/browser/managed_shared_worker_real_test.go`，不修改 production wiring。
Darwin／Linux 的 `TestManagedSharedRealHeadless` 僅在 `AGENTDOCK_TEST_REAL_MANAGED_CHROME=1` 執行；預設 suite 跳過。
Supervisor 專用指令（worker 未執行 real test）：

```sh
rtk proxy env AGENTDOCK_TEST_REAL_MANAGED_CHROME=1 NPM_CONFIG_OFFLINE=true GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestManagedSharedRealHeadless$' -count=1 -v -timeout=170s
```

測試使用 cached exact `chrome-devtools-mcp@1.7.0`，真實 `NewWorkerRegistry` stdio connection 與 shared adapter，Open 額外加 npm `--offline`。
保留 managed `--isolated --headless` config，不使用 attach／autoConnect／使用者 Profile、外部 URL、GUI 或 focus action。
同一 canonical temporary cwd 的 runtime owner 與兩個 fake ACP owners 經 Manager Acquire／Call／Release：檢查唯一 context／page／borrow token、同一實際 connector PID、底層 starts=1、一個 isolated headless Chrome main process；A window value 對 B／C 不可見，18 次交錯讀寫及 owner release 後 sibling 可用。
`evaluate_script` 嚴格驗證 `isError`、`structuredContent.message` 的 JSON value 與 reconnect flag；不以 text fallback 猜 page identity。

Cleanup 在 startup 前以 `t.Cleanup` 註冊，Fatal 仍執行；先透過 backend owned Stop／registry Shutdown 清理，再 bounded `ps` 檢查 connector process group、已觀測 child identities 與 test-only TMPDIR marker，觀測失敗或殘留均 fail。
主操作 context 100 秒、cleanup context 45 秒，terminal hard cap 170 秒；外部強制中止／Go timeout panic 不保證執行 cleanup，supervisor 應保存測試 PID log 與唯一 temporary profile path，僅以這些 owned identities 查核殘留，不得 broad kill Chrome／Node／npx 或其他 session MCP。
TMPDIR 僅改測試程序環境，HOME 不變；test-owned temporary files 由 `t.TempDir` cleanup 回收。程序快照非原子，不能證明任意逃逸／未觀測 descendants 或 crash reconciliation。

可能 blocker：offline npm cache 不完整、Node／Chrome 不可用、MCP handshake/schema 不合 1.7.0、實際 pages／isolatedContext 或 evaluate structured message 不符契約、page targeting 串頁、close_page failure、owned shutdown failure、ps 權限／程序殘留。不得以安裝、升版、切 profile 或 attach workaround 自動修復。
此 harness 初次撰寫時只有未啟用選擇的編譯／skip 驗證；後續 supervisor 已在 managed isolated headless Chrome 執行並通過，最終證據見本節末尾。使用者真實工作時的零提示、零搶焦點與完整 native UAT 仍 pending。

### P8-B keeper 修正（歷史失敗與修正）

Supervisor 回報首次 real opt-in：MCP PID `79761`、Chrome main=1、三個 owners 的 ownership／isolation 與 18+ 交錯操作通過；僅最後 owner2 Release 因 `close_page` 回報 ACTION_FAILED。失敗後 cleanup 確認 owned MCP／Chrome processes gone。
這是 shared pool cleanup invariant 缺口；本機 cached exact 1.7.0 `McpContext.js:215-217` 確認 `closePage` 在 `#mcpPages.size === 1` 拋出 `CLOSE_PAGE_ERROR`。

Shared adapter 在底層 StartManaged 成功後、關閉 startDone／發布任何 borrow 前，建立一次 pool-owned `about:blank` background page，context 為 `agentdock-pool-keeper-` + random token。
驗證 MCP isError 與 isolatedPageID；keeper startup／解析／取消失敗會 poison 並封閉 generation，由既有 finalizer 等 startup 完成後停止該 owned worker一次，不發布 borrow，也不猜測 page cleanup。
keeper page ID 不可回傳為 lease page；借用仍只能操作自己的已確認 page。Keeper 不逐頁 close，idle 時保留，terminal owned worker Stop 才完整清理。
Fake backend 新增 1.7.0 last-page guard，測試涵蓋 keeper 建立一次、borrower 無法 close keeper、final release 僅剩 keeper、terminal Stop 清理、keeper call／malformed／isError startup failure 與 borrower page ID collision fail closed。

Worker 本次僅執行 `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestManagedShared' -count=1 -timeout=120s`（明確移除 real opt-in env，加 `-v` 保存 skip 證據）：**PASS（0.553s）；合成案例全數通過，real test 明確 SKIP**。gofmt 與 git diff --check 通過。
此歷史 checkpoint 的 keeper-only 第一次 rerun 仍失敗；最終由下一段 selected-page guard 解決，見本節末尾驗證。未啟動 npx／Chrome／Edge，未 commit／push／deploy，也未修改 production wiring。

### P8-B selected-page cleanup guard（real rerun PASS）

Supervisor 在 keeper 修正後再次 real rerun：三 owners／一個 headless Chrome／18+ evaluates 通過；close_page 3、4 成功，最後 pageId=5 實際已關閉，但 response 與 retry 均 isError，訊息為 `The selected page has been closed. Call list_pages to see open pages.`；owned processes cleanup 通過。因此 keeper-only 並未結案，last-page guard 不能解釋這次失敗。
Shared adapter 的 primary close 與 Stop fallback 均在既有 gate 內，先檢查 owned keeper != target，再以 internal `select_page`／`bringToFront:false` 選 keeper，驗證 isError 後才 close owner page；backend Call 期間不持有 adapter mu。
Selection failure 保留已知 owner page、poison generation 並回報錯誤，禁止 close 與新借用；仍由 terminal owned Shutdown 清理。Caller 無法呼叫 select_page，也無法取得／關閉 keeper。
WorkerRegistry 原有 select_page 禁令維持；僅 opt-in real test backend wrapper 在核對 owned worker/PID 後，透過該 owned StdioSession 執行 selection 並驗證實際工具 input schema。未加入 production ACPBridge wiring，不能直接視為 production backend 已支援共享 selection。
Fake tests 驗證 primary／Stop fallback 每次 close 前均選 owned keeper、bringToFront=false、正確 underlying worker，且 selection transport／isError failure 不派送 close、不掩蓋 cleanup error。Real close_page 受限診斷 logging 保留（只記錄測試自建分頁 ID／結果，不輸出使用者資料）；supervisor rerun 最終 PASS。
本次 focused `^TestManagedShared` selection（移除 real opt-in env，`GOMAXPROCS=2`／`-p 2`／`-count=1`／`-timeout=120s`）**PASS（0.554s）**；real test 明確 SKIP。gofmt 與 git diff --check 通過；未啟動 npx／Chrome／Edge，未 commit／deploy。

### 2026-10-10 主 Session 實機隔離驗證結論（P8-B source checkpoint）

- **真實 MCP 1.7.0 + 一個 AgentDock-owned 隔離 Headless Chrome 驗證 PASS**：`AGENTDOCK_TEST_REAL_MANAGED_CHROME=1 NPM_CONFIG_OFFLINE=true NPM_CONFIG_UPDATE_NOTIFIER=false GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestManagedSharedRealHeadless$' -count=1 -v -timeout=180s`（35.37s）。三個不同 Owner 借用同一 MCP Worker／同一瀏覽器主程序；實際 JavaScript 狀態讀寫在各自 Context 隔離，交錯操作不串頁，分別釋放所有 Owner 後 shared owned Worker 最終停止一次。
- 測試以離線 npm cache 使用 exact `chrome-devtools-mcp@1.7.0`；僅 `about:blank`、測試專屬 `TMPDIR`／隔離 Chrome Profile；沒有啟動／接管使用者的 Edge、Chrome、真實登入 Profile，也沒有使用者互動 UI、網站發布或前景動作。
- 首輪發現「最後一個分頁不可關閉」，加 pool-owned keeper 後第二輪仍遇選取目標關閉的 MCP 格式化錯誤；第三輪在**內部 `select_page`／`bringToFront:false` 選 keeper 再 close owner page** 後，3 次實際 `close_page` 均 `isError=false`，`t.Cleanup` 實測 owned MCP／Chrome process 清零。原版／Next Core 與其他 ACP 原有 MCP 程序未干預。
- 主 Session 獨立回歸：Browser／BrowserPolicy Go suite、P8 shared focused `-race`、Go vet、gofmt／git diff check，以及 Linux／Windows amd64、macOS arm64 無 CGO browser test cross-build **全部 PASS**。普通測試不設定顯式 opt-in 會 SKIP real Chrome case。
- **仍未結案：** `managedSharedWorkerBackend` 仍 private opt-in，`ACPBridge` 的 runtime route／lease coordinator 還是 per-lease exclusive；real 試驗是**同一 canonical CWD 的合成 ChatGPT+兩個 ACP Owner scopes**，不是三個真實 ACP/ChatGPT 的 live broker injection。跨 workspace 共享、真正 page/context storage 安全性全貌、Chrome 程序與暫存檔的長期量測、crash orphan reconciliation、真實桌面 no-focus/零 debugging prompt、使用者原有 Edge/explicit Chrome Profile 都需要後續 native UAT。不能以 1 次 isolated PASS 假設完整產品 ready，禁止直接部署切換。

## 9. 2026-10-10 P8-B private ACPBridge coordinator integration（SOURCE ONLY）

- 基於乾淨 `6d4ca83d`、`feature/m8-permission-approval`。新增 package-private `newACPBridgeSharedForWorkspace`；明確 opt-in 才把 shared backend 接入既有 managed coordinator／ACP owner tokens。固定一個 canonical workspace；其他 CWD 可註冊，但 Acquire 在 planner／worker startup 前拒絕。此路徑只接受 managed decision，無 external browser fallback。**runtime `NewACPBridge` 仍 exclusive；未新增產品設定或 production caller，未部署。**
- `WorkerRegistry.Call(select_page)` 禁令不變。僅 opt-in pool-owned `sharedRegistryBackend` 可呼叫 pinned ready WorkerSession 的 keeper selection：驗證 immutable worker／SessionInfo、有限非負整數 pageId、完全相符的 pool-owned keeper、`bringToFront:false`，拒絕其他 page／worker／額外參數／focus。既有 shared gate 序列化 selection + close；borrower／ACP tool 仍不能 select_page 或指定 pageId。未放寬 Manager operation contract。
- 私有 shared cleanup marker 使 release／recovery metadata 表達「owned page 已清理、borrow 已結束」，不再誤稱 underlying worker stopped；exclusive reasons 保留。底層 borrow Stop 成功才 CleanupComplete；page cleanup failure 保持 CleanupFailed，保留 coordinator recovery，terminal owned Stop 成功後才能完成 recovery。
- 沿用 ACPBridge lifecycle ticks，managed SweepExpired／RecoverFailed 後才 ReapIdle；沒有新增 worker timer。Close 同步且 idempotent：禁止新 owner，ReleaseSession 後先 shared.Shutdown，再 registry.Shutdown。只停止該 owned registry worker，沒有 broad process kill 或使用者 browser 操作；不合作 backend／deadline failure 仍回報錯誤，不能把 deadline 當 cleanup 完成。
- 真正 ACPBridge + WorkerDependencies／fake WorkerSession 的合成測試涵蓋兩個 ACP sessions、一個底層 startup、distinct manager lease／borrow／page／isolatedContext、交錯 page routing、caller pageId／跨 token 拒絕、release 一個保留 sibling、最後 page release 不停止 worker、manual idle sweep stop once、concurrent／active Close、private selection 拒絕矩陣、cleanup failure／terminal recovery，以及 default exclusive constructor／reason。沒有 fake bridge passing assertion。
- 已 gofmt。唯一 focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestACPBridgeShared|TestManagedShared)' -count=1 -timeout=120s` **PASS（0.586s）**；命令移除 `AGENTDOCK_TEST_REAL_MANAGED_CHROME`，停用 GOPROXY／GOSUMDB 網路下載，既有 real test 因 opt-in 未啟用而 SKIP。首次 sandbox Go cache 寫入拒絕，尚未執行測試；獲准沿用 cache 重跑同一 selection 後通過。主 Session 獨立驗證全數 PASS：Browser／BrowserPolicy 全量 suite、`TestACPBridgeShared|TestManagedShared` 專項 Race Detector、go vet、gofmt／`git diff --check`，以及 Linux／Windows amd64、macOS arm64 無 CGO 交叉編譯。
- **限制仍在：** 一個 opt-in instance 只有一個 workspace；idle reap 永久封閉 generation，不再接受新 leases；runtime 使用需要 pool generation replacement。automatic global process monitoring、multi-workspace pool、crash orphan reconciliation 與 actual ChatGPT + native ACP UAT 仍 pending。未啟動任何真實 Chrome／Edge／npx、未操作使用者 browser、未 build／install／deploy／restart Next 或 stable、未修改 AGY／其他 repos、未 push。先前 isolated real PoC 不等於本次 ACP integration live acceptance。

### 82a72fab 後續：私有 generation replacement（synthetic source-only）

- 新增不同的 package-private `newACPBridgeRotatingSharedForWorkspace`，使用 `managedSharedRotatingBackend` 接既有 coordinator／owner tokens；原 terminal constructor 與 default `NewACPBridge` exclusive 行為保留，沒有 production caller 或設定切換。
- owner gate 序列化 Acquire、Call、borrow Stop 與 ReapIdle。每個世代各有不可重新指向的 `sharedRegistryBackend.shared`／keeper；舊世代 finalizer 的 done 已關閉且 Stop 成功，下一個 Acquire 才建立新的 backend／registry worker。Stop error 或尚未完成時拒絕 replacement；不以 timeout 推論程序已停止。TTL 沿用 30 秒，sweep／recovery 後才 reap，active borrow 不 reap。
- 對外 borrow ID 加單調 epoch，底層 WorkerRegistry ID 每次新建。舊 Call／page operations 永不轉送新世代；Stop recovery 最多保留上一個成功退休世代的 1024 個 token，僅回報已清理，不操作新 worker。更舊 token 拒絕；目前世代仍保留原 1024 tombstone 上限，滿額拒絕 admission，待 idle reap。這是有界 recovery window，不是無限歷史 lease storage；既有 Manager／Registry 歷史紀錄未改動。
- Shutdown 先撤銷 admission，再等待 gate 與世代 finalizer；重複 Shutdown／ReapIdle／bridge Close 不重複 underlying Stop。rotating path 錯誤與 registry shutdown failure 均使用 opaque unavailable；沒有 process／token 原始錯誤洩漏。
- focused synthetic selection 使用每次 Open 都新建的 fake session，涵蓋兩 session 一次 startup、sibling release、30 秒 retention、idle stop once、reap 後三個 concurrent acquires 恰好第二次 startup／peak live=1、數值 page ID 重複下的舊 borrow／lease quarantine、workspace／owner 拒絕、Stop 阻塞／失敗、Shutdown 與 reap 競態、Close idempotence，以及既有 terminal／exclusive 回歸。
- 本次指定 focused synthetic selection 最終 **PASS（0.565s）**，gofmt 與 diff check PASS；移除 real Chrome opt-in 並停用 dependency network download。既有 real-headless case 未啟用而 SKIP。主 Session 獨立驗證 Browser／BrowserPolicy 全量 Go suite、`TestACPBridgeShared|TestManagedShared` Race Detector、go vet、gofmt／diff check 與 Linux／Windows amd64、macOS arm64 無 CGO cross-compilation，**全數 PASS**。未做 live browser／native UAT、build／install／push／deploy／restart。單 owner per workspace/registry 的建構前提仍在；多 instance 跨 owner 仲裁、crash/orphan reconciliation 與 production wiring 未完成，不能視為 production readiness。

### 2026-10-10 P8-B cross-workspace process budget checkpoint（SOURCE ONLY）

- 基於乾淨 `be3bf251`，新增私有 `managedSharedBrowserBudget(maxSlots)` 與 `newACPBridgeBudgetedRotatingSharedForWorkspace`。容量由 caller 注入，合成測試用 2；沒有產品 default／設定／flag。所有參與 workspace owners 必須共用同一 budget；不同 budget、exclusive／未 budgeted 路徑不在此仲裁範圍。`NewACPBridge` 仍 exclusive，無 production caller／runtime wiring，stable 與 installed Next 未改動。
- 每個 canonical workspace 使用獨立 WorkerRegistry、Chrome／profile／pages；只共用 admission counter，不跨 workspace reuse browser。此 checkpoint 原先在 budget lifetime 保留 registry→canonical root ownership claims；下方 claim lifecycle cleanup checkpoint 已改為僅在權威成功 Close 後移除 claim。仍拒絕存活 owner 的同 registry 或同 canonical workspace 第二 owner，包括 parent alias。
- rotating owner gate 內先 reserve，才可 dispatch 第一個 underlying StartManaged；full budget 在 registry 建 worker／Probe／Open 前拒絕。borrow release、idle 30 秒 retention、pending Stop、取消及 Stop failure 都不能釋放名額。ReapIdle／Shutdown 或下一次 admission 只在 generation.done 已關閉、stopErr nil 後 exactly-once release；下一世代必須重新 reserve，無舊新程序 overlap。既有 keeper／epoch／bounded retired tokens 邏輯保留。
- **啟動 fail-closed lifetime：** registry StartManaged error／非 ready／缺 WorkerID 會將該 reservation 永久標記 uncertain。registry 可能在 Open side effect 後回傳 nil session，Stop 即使回傳 nil 也不能證明所有 launch side effects 已停止；因此保守保留 slot，ReapIdle／Shutdown 回報 opaque unavailable，禁止 replacement。不自行猜測 worker／PID 或 kill；即使部分 failure 實際已清理也不自動退還容量。需要後續權威 crash/orphan reconciliation，不能以重建 budget 作為安全恢復證據。
- Synthetic tests `TestACPBridgeSharedBudgetWorkspaceIsolation`／`ConcurrentAdmission`／`StopFailureAndPending`／`UncertainStartup` 及 `TestManagedSharedBudgetConstructorOwnership`：兩 workspace 各兩 ACP owners 恰好一 worker、global peak≤2、第三 workspace 零 registry worker／Open、borrow release 不退 slot、精確 30 秒 reap 後第三 workspace 入場、B 持續操作、替換世代需新 reservation、跨 token／borrow 拒絕、page/context routing、parent alias、並行 admission、blocking／failed Stop、取消 Shutdown、重複 ReapIdle／Close 與 uncertain pre-ready launch。fixtures 僅 owned fake session Stop，沒有 real browser／npx／網路或使用者程序操作。
- gofmt／`git diff --check` PASS；唯一實際 focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^(TestACPBridgeShared|TestManagedShared)' -count=1 -timeout=120s` **PASS（0.816s）**。命令 unset `AGENTDOCK_TEST_REAL_MANAGED_CHROME`、`GOPROXY=off`／`GOSUMDB=off`，既有 real Chrome case SKIP。首次 sandbox 在既有 Go cache 寫入時 setup failed，未執行測試；獲准使用原 cache 後同命令重跑通過。主 Session 獨立驗證 **全部 PASS**：Browser／BrowserPolicy 完整 Go suite、`TestACPBridgeShared|TestManagedShared` 專項 Race Detector、go vet、gofmt／diff，以及 Linux／Windows amd64 和 macOS arm64 無 CGO 交叉編譯。沒有安裝／重啟／部署／推送 Git、常駐 runtime 或使用者瀏覽器操作；runtime wiring、crash orphan、真實 ChatGPT＋原生 ACP UAT 仍 pending。

### 2026-10-10 P8-B budget owner claim lifecycle cleanup（SOURCE ONLY）

- 基於乾淨 `8f06e268`；私有 claim handle 具有唯一 identity，budget.mu 下的 releaseOwner 僅刪除完全相同 claim，重複／stale release 不影響後續 claimant。成功刪除 registry key，長期保留 budget 不再因歷史成功 Close 而 pin 每個 retired registry。
- ACPBridge 保存 budgetOwnerClaim；closeOnce 序列化 Close，完成全部 owner lease cleanup、rotating Shutdown 與 owned WorkerRegistry Shutdown，合併錯誤為 nil 才 releaseOwner。registry Shutdown 禁止新 worker 並等待所有已知 worker（不限 rotating generation）。阻塞期間不預先釋放；Stop error／uncertain reservation 仍保留 claim 與 occupied slot，重複 Close 不重試或隱式回收。
- canonical validation 先於 claim；claim failure 不建立 bridge。newACPBridge 的失敗點均在 lifecycle runner／worker dispatch 之前，僅此無副作用 construction failure 退還新 claim，錯誤維持 opaque unavailable。原始 NewACPBridge 不變，仍無 production wiring。
- deterministic fake tests：同 CWD、75 個新 registry 連續替換，每次成功 Close 後 owner count=0／slot count=0；舊 ACP token／borrow WorkerID 拒絕，無跨 session fallback；同 registry/root ABA、double release、duplicate Close、invalid root、Stop error、uncertain launch，以及阻塞 rotating／其他 registry browser Shutdown 時 concurrent Close／claim 拒絕。
- gofmt／`git diff --check` PASS；唯一 focused selection `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestACPBridgeSharedBudget' -count=1 -timeout=120s` **PASS（1.441s）**，沒有 skip。unset real Chrome opt-in，`GOPROXY=off`／`GOSUMDB=off`，僅 fake fixtures。主 Session 獨立驗證 Browser／BrowserPolicy 全量 Go tests、共享 Budget／Generation 專項 `-race`、Go vet、gofmt／diff，以及 Linux／Windows amd64、macOS arm64 無 CGO 交叉編譯均 PASS；**Native UAT 尚未進行**。本機 OS orphan detection 與 crash reconciliation 未實作；仍是 source-only private opt-in，不代表產品或 live runtime acceptance。
