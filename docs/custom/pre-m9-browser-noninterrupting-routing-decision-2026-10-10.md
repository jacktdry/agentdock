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

## 10. 2026-10-10 P8-B real ACPBridge harness checkpoint（SOURCE ONLY；opt-in real test PASS）

- 基於乾淨 `0391264361b7ccc2d9acc2c9fcc1c075ad86778b`、`feature/m8-permission-approval`，新增 `internal/tool/browser/acp_bridge_shared_real_test.go`。Darwin／Linux 的 `TestACPBridgeSharedRealHeadless` 只有 `AGENTDOCK_TEST_REAL_SHARED_ACP_BRIDGE=1` 才執行；普通 test 明確 SKIP。沒有修改 production constructor、runtime wiring、global policies 或既有測試。
- 使用真正 `newACPBridgeBudgetedRotatingSharedForWorkspace`、`NewWorkerRegistry(WorkerDependencies)` 與 real `mcpclient.OpenStdioSession`。三次 `RegisterSession` 模擬 ChatGPT-equivalent、Codex ACP、AGY ACP，固定同一 canonical temporary CWD、同一 owned registry、注入 maxSlots=1 budget；planner 為 `NewRoutePlanner(nil, emptyCatalog, nil)`。Probe 是 test-only exact `ManagedEngineVersion` 回傳，不啟動 probe 或存取網路；real MCP handshake 仍須獨立符合版本／schema。Open 以受控 ManagedWorkerConfig 呼叫 `OpenStdioSession`，並在啟動前增加 `npx --offline`；由測試程序及版本／schema 檢查佐證 Headless／isolated 設定；僅 cached `chrome-devtools-mcp@1.7.0`、`--isolated --headless`、`about:blank` 與唯一 test-only TMPDIR，不設定 HOME 或連接既有 Profile。
- Real session decorator 只驗證／觀測，所有成功操作與結果均來自真實 MCP，不替代 ACPBridge 或偽造 page results。三個 token 須有不同 BrowserLeaseID／borrow WorkerID／PageID／IsolationContextName、相同 connector PID；18 次交錯 `evaluate_script` 讀寫各自 window 狀態，拒絕 caller pageId、select_page 與跨 token lease。既有 private `sharedRegistryBackend` 負責 keeper selection，decorator 額外禁止 foreground new_page、非 keeper／bringToFront selection 及 keeper close。
- 逐個 `ReleaseSession` 檢查 CleanupComplete／shared cleanup reason、sibling 仍可 evaluate、最後 release 未停止 worker且仍佔一個 slot。直接向 owned registry `list_pages` 驗證沒有 borrower isolated pages、只剩一個 pool-owned isolated keeper。**MCP 可能另留 default bootstrap about:blank，並非保證整個 browser 總 page 數只有一個；不猜測或關閉未納入 pool borrow ownership 的 bootstrap page。** 所有列出的 pages 必須仍是 about:blank。
- 手動兩次 `bridge.sweepLifecycle(time.Now().Add(time.Hour))` 驗證 underlying WorkerStopped／owned session Close 恰好一次／slot=0；bounded process observation 等待舊 MCP／Chrome 全部消失後才允許新世代。同一 registry、同 CWD 重新註冊先前 session，取得不同 token 與 underlying WorkerID，重新 reserve slot；old token／lease／borrow IDs 不可操作新世代，新 page 的 window 狀態為空。成功 Close 後所有 session 各停一次，owned MCP live=0／peak=1、budget=0。每個存活世代觀測 Chrome main=1，argv 必須有 --headless 與 test TMPDIR 下的 --user-data-dir；這些是有限時間點觀測與 headless／no-focus call guards，**不是使用者桌面的零提示／零搶焦點實測**。
- 啟動前登記 `t.Cleanup`：只呼叫 bridge.Close／owned registry.Shutdown，再用既有 `realSharedProcesses` 驗證所有記錄 PID／process identities／TMPDIR marker 無程序殘留；test-owned files 由 `t.TempDir` 回收。診斷使用 `realSharedDiagnosticText`，不輸出 raw argv／Profile paths。Close／Shutdown error、process observation failure、retained slot／owner claim 均回報 BLOCKER；不手動 release budget、不重建 budget、不 broad kill。操作 context 150 秒，cleanup observation context 45 秒，bridge／registry 內部仍使用其既有 bounded shutdown contexts；Go timeout／外部強制終止不保證執行 Cleanup，程序快照也不等於 crash/orphan reconciliation。
- Source-only 驗證：gofmt／`git diff --check` PASS；未設定兩個 real opt-in 的 `GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestACPBridgeSharedRealHeadless$' -count=1 -v -timeout=120s` **PASS（套件 0.471s），該 case SKIP（0.01s）**；focused `-run '^(TestACPBridgeShared|TestManagedShared)' -count=1 -v -timeout=120s` **PASS（1.088s），所有合成案例通過、兩個 real cases SKIP**。使用 writable `/private/tmp/agentdock-p8-gocache`、`GOPROXY=off`／`GOSUMDB=off`；首次編譯的 unused loop variable 已修正後重跑。本 worker 未執行 real npx／Chrome、full／race／vet／cross-build／native UAT，未 commit／push／install／deploy／restart；未觸碰 AGY／stable／Edge／使用者 Chrome Profile。
- **Supervisor 已實際執行新 ACPBridge real proof 並取得 PASS；這僅是有界整合測試，非正式 runtime／release UAT。** cached npm 或版本／schema、page cleanup、runner Close 的任何失敗均須 fail closed，不安裝、升版、切 Profile 或 attach workaround。三 tokens 仍是模擬 sessions，沒有真實 ACP service／ChatGPT runtime injection；跨 workspace、crash orphan、既有公司 Edge／explicit Chrome Profile 與 native no-focus UAT 仍 pending。

已驗證的 opt-in 指令（由主 Session 執行）：

```sh
rtk proxy env AGENTDOCK_TEST_REAL_SHARED_ACP_BRIDGE=1 NPM_CONFIG_OFFLINE=true NPM_CONFIG_UPDATE_NOTIFIER=false GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run '^TestACPBridgeSharedRealHeadless$' -count=1 -v -timeout=300s
```

### 主 Session 實際驗收（2026-10-10；SOURCE-ONLY）

- **`TestACPBridgeSharedRealHeadless` PASS（46.76s）**：使用 Node/npm 離線快取的 MCP 1.7.0 與真實 ACPBridge 私有 budgeted rotating constructor。三個模擬 ChatGPT／Codex／AGY Session token 共用一個 MCP PID／Chrome main，分頁與 Context 分離、交錯 `evaluate_script` 不串頁；三次 ReleaseSession 均完成 owned page cleanup，保留 pool keeper；閒置 reap 結束第一世代、budget=0 且 owned processes 全部消失後才啟動第二世代，peak owned browser=1；舊 Session／borrow／lease 操作均被拒絕。最終 Close、registry Shutdown、test TMPDIR 程序觀察與 budget claim 清理均 PASS。
- 實測第一輪曾在啟動時得到 opaque unavailable，後續可重現 run 顯示 `list_pages` 工具要求非 nil 的 JSON arguments object；測試用 `map[string]any{}` 修正後，第三次 opt-in 成功。未改變 production policy、忽略實際錯誤或繞過原有權限。
- 主 Session 獨立執行 Browser／BrowserPolicy 完整 Go tests、`TestACPBridgeShared|TestManagedShared` 專項 Race Detector、Go vet、gofmt／diff 與 Linux／Windows amd64、macOS arm64 no-CGO 交叉編譯，**全部 PASS**；一般測試未設定 opt-in 時 real case SKIP。
- **尚未發行**：現有 `NewACPBridge` 仍為 exclusive default，沒有 production flag、Next 安裝版切換或三個真實 Agent 併發 UAT。日常 Edge／Chrome Profile、零彈窗／零搶焦點 native UAT、跨 workspace 真實多程序行為、crash/orphan reconciliation 與長期用量觀測仍未完成。


## 11. 2026-10-10 P8-B shared budget observability checkpoint（SOURCE ONLY）

- 在私有 budgeted rotating ACPBridge 新增 mutex-consistent `managedSharedBrowserBudget.snapshot()`；`SharedManagedBudgetDiagnostic` 只輸出 `capacity_slots`、`occupied_slots`、`available_slots`、`claimed_owners`、`uncertain_slots` 五個整數，不含 Workspace／PID／Profile／token／頁面／敏感路徑。
- Budget 下的 reservation map 與 `used`、`owners`、`uncertain` 均在同一 mutex 保護下快照；成功 release 原子扣除且冪等，啟動結果不確定持續占 slot。保留原本 pending／failed Stop fail-closed；只有全部 bridge cleanup 成功才退 owner claim。**這不是 OS 權威 orphan reconciliation，也不會按計時器猜測 cleanup 成功。**
- `BrokerDiagnostics.shared_budget` 只在私有 budgeted rotating shared 實例出現；預設 `NewACPBridge` 仍 exclusive 且 JSON 欄位不增加。沒有新增 runtime API、設定、授權或 production switch，亦沒有跨 Core 共享資源上限。
- 合成測試驗證容量耗盡、unknown launch 保留、blocked Stop 不退位、Close owner 清理、idempotence、concurrent snapshot invariants、JSON 欄位僅數字及 exclusive 回歸。驗證 PASS：定點 `Test(SharedBudgetSnapshot|ACPBridgeSharedBudget)`、完整 Browser/BrowserPolicy tests、定點 `-race`、`go vet`、`gofmt`、`git diff --check`；未以本次合成案例冒稱重新跑 §10 的 real Chrome proof。
- 仍待 Next-only runtime opt-in、可信 crash／orphan reconciliation、三個真實 Agent 連續操作與零提示／零搶焦點 native UAT、公司 Edge／指定 Chrome Profile S1/S2 驗證。原版 stable、AGY、已安裝 Next、真實 Chrome／Edge 與使用者 Profile 均未修改、重啟或部署；未 push。


## 12. 2026-10-10 P8-B Next Runtime single-workspace opt-in（SOURCE ONLY）

- 在現有 `internal/app/runtime.go` 的 ACP Browser 初始化處加入 `newRuntimeACPBrowserBridge`。**沒有明確環境變數時沿用原 `NewACPBridge` exclusive per-lease 行為。** 將現有 budgeted rotating shared backend 透過 `NewNextSingleWorkspaceSharedACPBridge` 暴露為一個保守的 source-only Next Runtime integration boundary，建立一個 private budget、全 Core 同時最多一個 shared Chrome generation；不修改一般 managed/external connector 原有 API。
- 選擇性試驗只接受設定 `AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE=/absolute/canonical/existing/workspace`。額外要求 macOS、`AGENTDOCK_DESKTOP_VARIANT=next`、`AgentDockHome == <目前使用者 Home>/.agentdock-next`，以及 ACP/Browser 已啟用且非 stdio；Path 必須是原始字串即 canonical 的現存目錄（拒絕空值、相對路徑、`..`、symbolic-link alias）。未設定變數代表原行為；**明確設定但不合格則拒絕初始化，不能悄悄 fallback 到多個獨立 Chrome**。環境／目錄條件僅避免誤選 stable，**並非已簽章的 Next Core trusted identity，也不代表使用者授權／已登入 Profile attestation**。
- 單一選中 Workspace 的兩個以上 ACP sessions 可以透過既有 Bridge owner token 共享一個 pool-owned headless managed worker；每個 session 仍透過原有 admission、page lease 和 background isolatedContext。其他 Workspace 的 ACP coding session 可註冊 Browser MCP token，但 `Acquire` 在任何 worker startup 前 fail-closed；選中 Workspace 若路由至 External/RequiredExternal 也拒絕，不會偷換 Engine／Profile。沒有 cross-workspace multiplexer、不同 Workspace 的全域程序共享或 crash orphan reconciliation。
- `internal/app/acp_browser_runtime_mode_test.go` 僅建構 source-only bridge：驗證 default exclusive、Next opt-in metrics capacity=1、三個不同 Session scopes、foreign Workspace 拒絕且未啟動 Worker，以及 stable variant／錯誤 HOME／功能關閉／stdio／不正規 Workspace／symlink path 的拒絕矩陣。沿用先前 §10 真實 Headless Chrome 的離線證據，**本次未啟動 Chrome、Edge、npx 或 ACP backend**；實際三個獨立 Agent concurrent UAT 仍未完成。
- 實際 source-only 驗證：`GOPROXY=off GOSUMDB=off GOMAXPROCS=2 go test -p 2 ./internal/app ./internal/tool/browser ./internal/browserpolicy -count=1 -timeout=180s` PASS；Next selector／shared budget 專項 `-race` PASS；`go vet -p 2` 三 package PASS；Linux／Windows amd64 無 CGO 的 app/browser test cross-build PASS；`gofmt`／`git diff --check` PASS。
- **並未部署或切換已安裝 Next**，也未新增 GUI toggle、公開的 Runtime 設定 API、持久化自動啟用或 Windows/Linux 啟用。Production release gate 仍須可信 process/consent authority、Native GUI 無焦點／零彈窗 UAT、權威 Crash/Orphan recovery、跨 Workspace dynamic broker、選定 User Profile 安全路由與回退控制；不能把單 workspace opt-in 視為 P8/M9 完成。


## 13. 2026-10-10 P8-B sticky Core restart fence（SOURCE ONLY；非 Orphan 自動回收）

- 問題：上一輪 private shared budget 只在同一 Core 記憶體生命週期限制 Worker 數量。Core 異常終止後重啟可能將計數歸零，無法證明舊 MCP/Chrome 已退出；既有 process group/Job cleanup 及 real headless proof 無法證明所有 detached/未觀測程序已退出。
- Next-only opt-in 在 *建立* shared Bridge 前以 O_EXCL/O_NOFOLLOW 建立 `<Next AgentDockHome>/.shared-browser-restart-pending.v1`，檢查 private directory owner/mode、regular 0600 owned marker，並 fsync marker 與 directory。存活的另一個 Core 或異常留下的 marker 一律拒絕 shared startup；不依賴 PID、計時器或新的 memory budget 判斷。**未設定 opt-in 的 exclusive 路徑不建立此檔案。**
- Runtime 持有 pinned directory descriptor 與 marker device/inode；**只有 ACP runtime Close 與 ACP Browser Bridge Close 都成功**才會對比原 inode 再 unlink + fsync directory。Close 失敗、marker replacement、symlink、目錄權限錯誤及持久化失敗均 fail-closed；同一次 release 結果冪等，避免重複刪除。沒有 OS PID kill／猜測 orphan 的操作。
- deterministic fixtures 在 test-only private HOME 覆蓋併發競爭、inode replacement、安全目錄、正常 close 與受控子程序 hard kill 後 marker 存留。**此 marker 僅防止意外重啟後再啟動另一個 shared Chrome；並非 OS orphan observer、ownership ledger 或 detached-process 自動回收。** 即使沒有殘留 Chrome 也會保守拒絕，還需後續有權威身分證據的 reconciliation 與使用者可安全解除流程，不能以手動刪檔或 PS PID 猜測取代。
- 驗證：完整 `go test -p 2 ./internal/app ./internal/tool/browser ./internal/browserpolicy ./internal/mcp/client ./internal/process -count=1 -timeout=180s` PASS；Restart Guard／Next opt-in 專項 `-race` PASS；`go vet` 五個模組 PASS；Linux/Windows amd64 無 CGO app/browser test cross-build PASS；`gofmt`、`git diff --check` PASS。一般測試沒有 real headless browser opt-in。
- macOS Next source-only opt-in，未安裝／部署至 Next 或 stable，未操作 Chrome/Edge/npx/AGY；真實多 Agent 原生 UAT、Profile 權限及零搶焦點仍是 P8/M9 gate。


## 14. 2026-10-10 P8-B orphan evidence + read-only crash triage（SOURCE ONLY；不授權清理）

- 承接 §13 persistent restart marker。Next-only shared ACP WorkerRegistry 加入 optional `ReadyWitness` callback（僅此實驗 opt-in registry 設定；原 `NewWorkerRegistry` 的 stable/exclusive caller 不變）；在 managed engine 版本、工具 schema/handshake 驗證通過後、**WorkerInfo 返回 lease 層之前**，將 wrapper `SessionInfo.PID` 的觀測值以 `worker_ready_pid=<int>` append 到原先私有 marker，並 fsync 檔案及 directory。`ReadyWitness` 失敗會讓 Worker 進入 Failed 並關閉 session；budget 將啟動視為 uncertain、sticky restart guard 持續阻擋。每筆 witness 只證明 Core 當時觀測的 wrapper PID，**不是 OS process ownership handle 或 OS 進程起始時間證明，且無法辨識 detached Chrome**。
- Append 使用原 Runtime pinned dirfd + O_NOFOLLOW，同 inode/device/owner/mode 驗證，舊 journal 語法必須完整解析；最多 **64 筆 ready PID**、總 16 KiB。超額、部分寫入或損毀，一律拒絕新 Worker，不自動刪除、重置 budget。正常 clean Bridge Close 解除 marker 前也重新確認 journal 語法；不合格保留 marker。
- 新增 macOS Next-only **read-only** `inspectSharedBrowserOrphanEvidence`：從私有 root `openat`／O_NOFOLLOW 讀取有限 journal，再使用 `process.Observe` 查詢非原子 PID snapshot。分類為 `no_pending_marker`、`startup_unknown`、`pid_present_identity_unverified`、`worker_pid_absent_descendants_unverified`、`observation_unavailable`、`untrusted_marker`；startup gate 因 marker 拒絕啟動時，只將安全的分類字串帶入錯誤，不顯示 PID／argv／路徑／Profile／token。**無論何種狀態都絕不直接 unlink、signal、kill 或授權重啟**；PID 可重用、wrapper 消失不能證明 Chrome 已離開、PS 快照非原子。
- 主 Session source-only 驗證 PASS：完整 `go test -p 2 ./internal/app ./internal/tool/browser ./internal/browserpolicy ./internal/mcp/client ./internal/process -count=1 -timeout=180s`；含 ReadyWitness／Orphan Evidence／Restart Guard 的 app/browser 定點 `-race`；上述五模組 `go vet`；Linux、Windows amd64 無 CGO 的 app/browser 測試交叉編譯；gofmt 與 git diff 檢查。未啟動 real Chrome，亦未通過 Native 3-Agent UAT。
- Tests：假 WorkerSession 的 witness 成功／持久化失敗禁止發佈、版本驗證失敗不呼叫 callback；私有測試 root 中多 generation PID alive/absent/zombie/unknown 組合、並行 append、corrupt/partial/symlink、unknown startup，均保持 fail-closed。本輪沒有啟動真實 Chrome/Edge/npx；真實 AGY+Codex+ChatGPT no-focus UAT、可信 process tree identity、OS handle-based attestation、detached descendants orphan recovery 與 GUI 安全解鎖流程仍是 P8/M9 release blocker。檢查與提交結果記錄於工作線進度。


## 15. 2026-10-10 P8-B macOS process birth fingerprint（SOURCE ONLY；不等於有權終止）

- §14 僅記錄 PID，無法辨識 PID 重新使用。新增 `internal/process/identity.go`／`identity_darwin.go`，以 macOS `unix.SysctlKinfoProc("kern.proc.pid", pid)` **唯讀**取得 kernel 觀測的建立時間（秒＋微秒）、有效 UID 與 process group ID；跨平台 `!darwin` 介面明確 unavailable。不執行 ps command 來解析無小數精度的開始時間；不 attach／hold／signal／kill 任何 PID。PID start identity 本身亦屬不具原子性的觀測值，**不等於核心行程控制權或可安全 kill 的 handle**。
- Next-only shared Worker ready witness 在 schema、handshake 成功後、租約發佈前實際讀取此 kernel 指紋（缺值、程序已結束、時間戳不合格或 OS 不授權都拒絕此次 worker ready），journal 新增完整記錄 `worker_ready_pid=<pid>,start_sec=<s>,start_usec=<us>,uid=<uid>,pgid=<pgid>`；與上一版 `worker_ready_pid=<pid>` 相容，但舊版資料**不能補寫或推論先前的 process birth 身分**。沿用同 inode、private dir、O_NOFOLLOW、bounded append 和 fsync；記錄異常一律保留 sticky crash fence。
- 當 Next shared startup 因 restart marker 而拒絕時，唯讀檢查會對照本次 kernel birth 指紋：相符 `worker_birth_matches_descendants_unverified`，不同 `worker_pid_reused_or_changed_descendants_unverified`，歷史只有 PID `pid_present_identity_unverified`；PID 已不存在仍 `worker_pid_absent_descendants_unverified`，存取受阻／觀測不一致 `observation_unavailable`。**所有狀態都不允許自動移除 marker、不 SIGKILL、也不代表已驗證 detached Chrome 子程序。** 給使用者／日志的 startup error 只包含固定 status 字串，不印 PID、指紋、Profile 或 Workspace。
- 測試覆蓋：真實測試程序自身 PID（非瀏覽器）出生身分讀取／寫入／比對、虛擬 PID 重用（start_usec changed）、UID/PGID/欄位不完整或損毀、核查失敗、PID absent、舊版 PID-only 相容與不升權，以及 Darwin/非 Darwin build boundary。Race/完整 tests 與跨平台建置結果記錄於進度。
- Source-only 確認 PASS：完整 app/browser/browserpolicy/mcp-client/process Go tests；定點 app/browser/process race；上述五模組 go vet；Linux/Windows amd64 無 CGO 的 process/app/browser test crossbuild；gofmt 與 git diff check。
- **尚未完成**：跨 Core 可信 process handle、OS managed launch 與 detached Chrome process tree 的原子性 parentage／birth 取證、故障清理的成功證明、可授權解鎖流程、三個真實 Agent 零搶焦點 native UAT、用戶 Chrome／Edge exact Profile route。維持 source-only opt-in，不安裝／不部署、不干擾 stable AgentDock／AGY／日常瀏覽器。


## 16. 2026-10-10 P8-B kernel group lineage observation（SOURCE ONLY；不授權回收）

- 承接 §15 macOS Next-only wrapper birth fingerprint。現有 `internal/process/process_unix.go` 的 `Controller` 只在 Core 存活期間持有由 `SysProcAttr.Setpgid` 創建的程序群組 ID；Core 被強制終止後，群組 ID 不能視為仍具所有權的核心程序把手。即使群組內看不到成員，Chrome 仍可藉由 setsid/setpgid 或其他分離途徑留下 detached descendants。
- 新增 `process.ObserveGroupBirths(ctx, pgid)`（macOS `unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)`），唯讀取得群組現有 PID、kernel start sec/usec、UID、PGID 觀測；最多 256 個成員、PID 不重複、每項身分/群組必須與 kernel 回傳條件一致。群組快照是非原子的，數字 PGID 可能被重用，也不綁定任何被控制的行程身分；Windows/Linux 本輪保守 unavailable。沒有 shell/ps 列舉任意使用者進程名稱，也不會發出 OS signal 或觸碰 Profile。
- **僅在前一階段證明所有 recorded wrapper PID 都不再可見時**，Next-only crash inspection 才會讀取帶 birth fingerprint 的舊 process group：仍有成員回 `worker_group_members_descendants_unverified`；無成員回 `worker_group_empty_detached_descendants_unverified`；不符合 UID、子程序 starttime 早於 parent、group ID 不符、PID 重用或快照衝突回 `worker_group_identity_conflict`；無法讀取則回 `observation_unavailable`。既有純 PID-only journal 與非 group leader wrapper **不觸發**群組歸屬推論。
- 分類不包含 PID/UID/PGID/argv/Workspace/使用者 Profile；不管是群組存活、群組清空或觀測不確定都**禁止自動解除 §13 sticky marker 或自動終止任何程序**。這是 crash triage／生存性線索，**不是可證明 detached browser child 全部結束的證書**。
- Source-only 驗證 PASS：完整 `go test -p 2 ./internal/app ./internal/tool/browser ./internal/browserpolicy ./internal/mcp/client ./internal/process -count=1 -timeout=180s`；app/process/browser group lineage 專項 `-race`；上述五模組 `go vet`；Linux/Windows amd64 no-CGO 的 process/app/browser tests cross-build；gofmt／diff。
- 測試只唯讀列舉 **自身 Go 測試程序既有群組**，另用 synthetic birth/group snapshots 驗證 PID 重用、UID/PGID 不符、時間線逆序、group member 存活、empty group、legacy、nonleader、observer failure 及 marker 完整保留。禁止 production 切換與瀏覽器 GUI 啟動；仍待 kernel 可驗證的 process handle / descendant 生命週期、Crash/Orphan safe reconciliation、ChatGPT+Codex+AGY 原生零搶焦點併發 UAT。


## 17. 2026-10-10 P8-B Next worker direct-parent launch proof（SOURCE ONLY；非跨重啟 handle）

- 在 §15/§16 原 birth identity 基礎上，`process.BirthIdentity` 加入 macOS kernel `Eproc.Ppid` 觀測，且 `kern.proc.pgrp` 的 group snapshot 同步保留該 parent PID。`process.ObserveDirectChildBirth(ctx, parentPID, childPID)` 各查詢 parent 與 child **兩次**：四次 birth/parent/UID/group metadata 均一致、child kernel PPID 等於當前 Core PID、UID 相同、child 建立時間不得早於 parent、child 必須是獨立 process group leader（PGID=child PID）；任何 kernel query 失敗、PID 重用跡象、父子重設、parent 退出、UID/group 不符都拒絕。其他平台 birth observation 仍 unavailable，僅 Next-only opt-in 會使用本准入。
- macOS Next-only shared ACP `WorkerRegistry.ReadyWitness` 在 managed engine handshake/schema 成功、lease 發佈之前，使用 `ObserveDirectChildBirth(ctx, os.Getpid(), info.Session.PID)` 核對真正由**此 Core 直接啟動**的 stdio MCP wrapper。驗證失敗一律 Worker Failed、保留 sticky restart marker、不釋放 uncertain budget；其他 existing stable/exclusive WorkerRegistry 不套此 witness。舊版紀錄仍可 read-only 解析，但沒有自動回填所有權證據。
- Tests：人工 PID/PPID、父子 PID 重用、child born before Core、核對過程 metadata 變動、UID 不同、非獨立 group、失去 kernel 查詢等 fail-closed；macOS 僅使用由自己啟動的**Go 測試子程序**（設定 Setpgid，無 Chrome/Edge/npx），驗證實際 direct-child kernel identity 和 Next Shared ReadyWitness durable journal；同時確定存在但非自身子程序的 PID（例如測試 Core 自己）不會被認定為 Next Worker。完整 Go、targeted race、vet、Linux/Windows crossbuild 結果記在 progress。
- Source-only 測試全數 PASS：完整 app/browser/browserpolicy/mcp-client/process Go tests、定點 app/process/browser `-race`、五模組 `go vet`、Linux/Windows amd64 no-CGO process/app/browser tests crossbuild、gofmt/git diff 檢查。
- **安全界線：**兩次非原子讀取降低 PID 重用/重設誤判，但不是不可偽造或跨 Core 存活的 OS process handle，不能證明 children 經 PID reparenting 後仍屬於 Next，更不能證明脫離 group 的 Chrome 全部退出；**仍無 Signal/Kill、沒有自動 reconciliation/unlink、沒有上線切換**。持久化 journal 記的是 ready 時的觀測事實，並非 kill authority。真正完整的 orphan cleanup 需持續擁有可驗證的程序控制/生命週期與 detached descendant audit。


## 18. 2026-10-10 P8-B macOS kqueue NOTE_EXIT（SOURCE ONLY；Core 存活期間的唯讀事件）

- macOS `process.WatchVerifiedExit(ctx, expected BirthIdentity)` 新增 `kqueue / EVFILT_PROC / NOTE_EXIT` 單次核心事件監看：註冊前與註冊後**各確認**完整 PID + kernel birth timestamp + UID + PGID + PPID 一致，以拒絕註冊期間的 PID 重用或 parent 變動。事件只接受與已核對 PID 對應的 `NOTE_EXIT`，EV_ERROR、其它 filter、OS query 失敗或事件不符一律失效；kqueue FD 使用 close-on-exec，不持有可跨 Core 重啟的權限把手。`Wait(ctx)` 有可取消的有界輪詢（100ms kernel wait），`Close()` 關閉 FD；非 Darwin 平台 fail-closed unavailable。
- **僅 Next-only shared** `sharedBrowserRestartGuard.recordManagedWorker` 先經 §17 的 direct-parent birth proof，再註冊退出事件，之後才持久化 ready journal 和向 lease 層發佈 Worker。完成後以守護 goroutine 唯讀等待 exit；正常收到 NOTE_EXIT 記錄 Core 記憶體中的 `exitObserved`，異常則標 `exitUncertain`。正常 `releaseAfterVerifiedClose` 前先取消、join 全部 watch goroutine，再處理原本的 inode/content/fsync guard；任何非取消類型的 watcher 異常禁止清除 restart marker。失敗不會產生 kill、unlink 或不受監督的 fallback。
- Native 測試只啟動由 Go 測試程式建立的受控 fixture subprocess，透過 EOF **自然退出**觀察 NOTE_EXIT；驗證 starttime 不符拒絕、等待取消、Close 冪等、Next ready witness 整合後自然退出可觀測且 guard 正常 close。無使用者 Browser / ACP process 被查詢或終止。其它原版／exclusive 路徑不會註冊 watcher。
- Source-only Go 驗證 PASS：Browser queue timing-sensitive case serialized recheck 3x、Browser full、其餘 app/browserpolicy/mcp-client/process full tests；app/process/browser targeted -race；go vet；Linux/Windows amd64 no-CGO app/process/browser crossbuild；gofmt/diff。初次同時交叉編譯時 Browser queue 3 秒等待測試曾逾時一次，交叉編譯結束後 serial recheck 3x 與 full browser 都通過，未改動 queue 排程。
- **限制**：`NOTE_EXIT` 僅能證明該**被監看 wrapper** 收到退出事件；即使 Next Core 仍存在，也無法以事件證明脫離群組的 Chrome 子程序全部離開。Core/電腦崩潰會使 kqueue FD 失效，記憶體計數不做跨 Core 恢復依據；絕不以事件自動回收、釋放 crash fence 或授權 PID 控制。需要另外設計完整可監督的 descendant lifecycle/identity，以及經過明確審查的 orphan reconciliation。


## 19. 2026-10-10 P8-B pre-handshake spawn journal（SOURCE ONLY；不等於 crash cleanup）

- §14–§18 的 shared ACP ReadyWitness 只在 MCP initialize/handshake/schema 成功後記錄 PID，Core 如果在 `exec.Cmd.Start()` 後、握手前 crash，只留下全域 sticky marker。新增 **ephemeral-only** `mcpclient.OpenStdioSessionWithStartedWitness(ctx,cfg,fn)`：stdio child 真正 Start、並成功 attach 已有的 process controller 後，**在**任何 MCP handshake 或工具讀取前呼叫 witness。函式 callback 若失敗，會透過原 transport/owned-controller 進入單一 Wait owner 的失敗清理，不建立可用 StdioSession；一般 `OpenStdioSession`、動態 MCP Manager 和 stable/非共享 registries 均維持原路徑（nil callback）。
- `WorkerDependencies.StartedWitness` 僅由明確 Next shared opt-in 使用，直接透過既有 WorkerRegistry 預設 stdio Open 呼叫上述 ephemeral API。Next guard 先有 durable private crash marker；spawn callback 取得 `process.ObserveDirectChildBirth` 核心身分（parent PID + kernel birth sec/usec + UID + PGID），在 private dirfd + same inode + O_NOFOLLOW 的 append journal 寫 `worker_started_pid=<pid>,start_sec=…,start_usec=…,uid=…,pgid=…`，經 FSYNC 才放行 MCP 初始化；失敗不 fallback。原 `worker_ready_pid=…` 保留，且明確 Next shared 設定要求 ready 必須找到 **完全相同 birth identity** 的先前 started 記錄，避免將 reused PID 或中途改變的程序誤判為 validated ready。
- Parser 仍嚴格接受舊版 `worker_ready_pid=<pid>` 或完整指紋；新的 started 記錄**只**能包含完整指紋。可同時包含多世代 started／ready，最多 64 筆、16KiB；部分寫入/錯誤格式一律保留 marker。重啟時如果 journal 全部只有 started，顯示 `worker_started_handshake_unverified`，不把它冒充 ready、也不啟動新的 worker／不 unlink／不 kill。**Core 若在 Start 與 witness append 之間 crash，仍可能只有 startup_unknown marker**，此設計保守封鎖，不做無證據補推論。
- Native regression tests 使用自建 Go 測試二進位檔模擬 MCP helper（**絕不啟動 Chrome、Edge 或 npx**）。涵蓋正常 MCP session 的 spawn callback 可先於 handshake 成功、callback 拒絕時 session 不會發佈且原 process cleanup 生效、WorkerRegistry default Open 的回呼確實傳遞且 ready hook 沒被執行、實際已啟動但故意無 MCP handshake 的 helper 導致 started-only durable journal、stage/格式嚴格解析、強制 required-start 且 PID birth 不符時 ready fail closed，以及 legacy/exclusive 無 callback 行為。
- Source-only 回歸 PASS：完整 app/browser/browserpolicy/mcp-client/process Go tests；app/browser/mcpclient/process 針對啟動／握手／orphan witness 的 race；go vet；Linux/Windows amd64 no-CGO 的 app/browser/mcp-client/process tests cross-build；gofmt/diff。首次 cross-build 發現非 Darwin stub 缺少新回呼，已以始終回傳 unavailable 的保守 stub 補齊，重新編譯 PASS，未開放非 macOS Next shared。
- 此證據仍不提供跨 Core 的 OS-owned process handle，無法解決 detached Chrome children 或授權 crash recovery。**所有狀態維持 opt-in、source-only、不做自動程序終止/解鎖/部署**。下一階段需研究完整 descendant lifecycle 和可信 durable ownership，才可討論自動 reconciliation。


## 20. 2026-10-10 P8-B detached-group descendant observation（SOURCE ONLY；非自動回收權限）

- 承接 §19 pre-handshake spawn 與 §17 ready 的直接父子證據。新增 macOS `process.ObserveDescendantBirths(ctx, expectedRoot)`：只對已由 Core 直接核對 birth/PPID 的 managed MCP wrapper 根 PID 使用 `kern.proc.uid` 的唯讀 kernel metadata snapshot（不讀取 argv、程序名稱、Profile、使用者分頁或路徑）。在 snapshot 前後重查 root 完整 birth 身分，對與 root 仍存在**完整 PPID 祖先鏈**的同 UID 子孫進行建立時間先後核對、再次讀取 kernel PID/PPID/birth/UID/PGID 以拒絕取樣期間的 PID 重用或 reparenting。最多掃描 8192 個 UID process rows，回傳最多 24 個子孫，超過／無法驗證一律 fail-closed；非 macOS 拒絕使用此唯讀觀測。
- 跟既有 `kern.proc.pgrp` 不同，祖先鏈比對**不要求子孫留在原程序群組**：即使孫程序 `setsid` 或 `setpgid`，只要取樣當下仍能循 PPID 連回 managed wrapper，就可記錄當時的出生身分；不能由相同 UID、相同 PGID、程序名稱或 chrome-like argv 推定所有權。
- **僅 Next shared ready** 在 direct-child + kqueue exit watch 已核對後、lease publication 之前做 bounded descendant snapshot，接著以原 O_NOFOLLOW/private-dirfd/inode+UID check，在同一次 bounded append/fsync 中寫出子孫 birth forensic entries 與 ready record。新欄位：`worker_descendant_pid=…,start_sec=…,start_usec=…,uid=…,pgid=…,parent_pid=…,root_pid=…,root_start_sec=…,root_start_usec=…`。Strict parser 驗證每個子孫必須可追溯到先前 `worker_started` 的**同一 PID+root starttime 世代**，且每一條 recorded parent chain 都必須完整、同 UID、時間順序有效；錯誤／超過 64 records 或 16KiB 限額／部分寫入都保留 sticky marker、拒絕 ready，不 auto-kill/unlink。既有 ready-only 舊 journal 仍可讀取。
- Crash 後唯讀診斷**先分開** managed root/descendant witnesses：原 wrapper 已不在時，對歷史已記錄的子孫 PID 進行 kernel birth 交叉比對（故意允許 PPID/PGID 已變，因為可能被 reparent 或 setsid）；仍在且出生身分相符回 `recorded_descendant_birth_matches_cleanup_unverified`；PID 已重用／birth 不符回 `recorded_descendant_identity_changed_cleanup_unverified`；已退出的子孫不視為所有子孫均已退出，繼續之前的群組空/未驗證判斷。所有結果不印 PID／路徑／其他應用 Profile，不具有 cleanup、restart、signal 或 auto-unlock 授權。
- 實測 fixture 只使用自行啟動的 Go 測試 parent 和以 `Setsid` 脫離原群組的 Go 孫程序；證實 kernel UID snapshot 能夠追出不同 group 的 PPID 鏈、Next ready journal 保存其出生指紋、正常 EOF close 收到 wrapper NOTE_EXIT。合成測試覆蓋 root birth changed、欠缺祖先、reparented before sample、UID 不符、PID 重用／生存性分類、部分或錯誤 journal、存活子孫與 wrapper gone、64/24 條件。未啟動 Chrome/Edge/npx/AGY，也未掃描外部程序的命令列／內容或向其他程序發 signal。
- **嚴格限制：**這是非原子 snapshot，無法列出 snapshot 前已 reparent/消失而逃逸的後代，不能證明「所有 Chrome 子程序」的完整性；事後 PID 重用與 snapshots 也不構成可重用的 OS process-control handle。即使某個 descendant birth 在 Core crash 後仍匹配，**絕不直接終止或解除 restart marker**。完整的跨 Core owned handle/監督模型與三 Agent 零搶焦點 native UAT 仍為 blocking gate。

- **驗證 checkpoint（2026-10-10）**：序列化回歸完整 app/browser/browserpolicy/mcp-client/process Go tests PASS（含上次受 CPU 資源競爭影響而未抵達 fork boundary 的既有 process stress 測試）；app/process/browser 指定 -race PASS；五模組 go vet PASS；Linux/Windows amd64 no-CGO app/process/browser tests crossbuild PASS；gofmt、git diff --check PASS。先前平行跑測試的 process 壓力測試曾逾時一次，串行重跑正常，不修改其現有控制邏輯；仍未執行真實 Chrome 三代理 UAT。\n

## 21. 2026-10-10 P8-B live descendant refresh（SOURCE ONLY；增量證據，不授權 cleanup）

- §20 在 Worker ReadyWitness 只取一次 PPID 祖先鏈，漏掉**握手/ready 完成後才啟動**的 managed Chrome descendants。本輪針對**僅 macOS Next explicit shared** 的出生身分 attested wrapper，於既有 `kqueue NOTE_EXIT` watcher 同生命週期附加獨立**每 15 秒**的唯讀低頻 kernel UID-scope descendant observation；用 §20 `ObserveDescendantBirths` 確認 root birth、每個現存子孫當下 ancestry、UID、birth 後，才向私有 sticky marker journal 追加「**第一次出現的**」子孫 PID+birth+PPID+PGID/root generation 事實。同一 PID/birth 重複取樣不執行任何 journal write / fsync；PID 已重用、祖先斷裂、檔案更換 inode、UID、parse、entry 64 筆與 16KiB 上限等任何異常都保守拒絕。**已 reparent 的子孫如果在首次取樣前失去父子鏈，依然無法被追溯**；不能將取樣當時沒找到誤解為不存在。
- 新增 `sharedBrowserRestartGuard.appendNewReadyDescendants` 使用現有 private 0700 root／O_NOFOLLOW／owner／inode 核對及 read-before-append/fsync file+dir 的同步鎖定序列：必須存在完全相符的 wrapper `worker_ready` 出生世代；對本次 kernel snapshot 內每一個子孫取得完整受核對 ancestry，但僅序列化尚未出現的 birth 指紋；寫入之前會將舊 journal＋新增 lines 經原 strict parser **整份再驗證**（仍要求 ancestor chain 包含之前記錄的父子連結與 prior started root）。錯誤、容量耗盡或任何成功性不明的樣本將 `exitUncertain` 設為 true、停止本次 sampler，正常 shutdown **不能自動移除 restart fence**；沒有自動截斷、輪轉、刪除歷史證據，也絕不以 PID/PGID 發 signal。
- Worker 退出的 kernel `NOTE_EXIT` 同時取消該 worker 的定期 sampler。Core verified close 對所有 exit watch 與 sampler 都 cancel+join 後才檢查持久化 marker，避免背景 goroutine 存取已關閉的 dirfd。既有 stable AgentDock、exclusive ACP browser、AGY ACP 均不啟用。
- 驗證：合成 append test 涵蓋 new PID、no-op dedup、PID reuse、missing parent／changed root birth 的失敗保留 marker；macOS Native fixture 僅由**自行建立的 Go worker**握手就緒**之後**才收到指令產生另一 PGID 的 Go grandchild，Core 定期取得新子孫並持久化後，正常 EOF、NOTE_EXIT、sampler 退出與乾淨的 verified close。無 Chrome/Edge/npx/AGY 啟動，無 GUI 焦點干擾，無用戶程序控制。
- **阻擋條件不變**：本功能是取樣到的「曾存在於受控祖先鏈」出生證據；仍沒有可跨 Core crash 的 OS-owned process handle、無法追到在觀測前 double-fork/reparent 的子程序，也不能證明所有 Chrome 派生/遺留程序均已安全退出。現階段不切換 production、不 auto-restart、不授權 PID-based cleanup；未來需要明確的 supervisor/OS 可驗證 handle 與完整 lifecycle UAT。

- 本輪 source-only 實際驗證 PASS：五模組完整 Go tests、macOS app/process/browser 指定 Race Detector、五模組 go vet、Linux／Windows amd64 no-CGO process/app/browser 測試交叉編譯、格式與 diff 檢查；沒有部署或執行 Chrome/Edge/AGY。


## 22. 2026-10-10 P8-B crash-surviving independent exit observer（SOURCE-ONLY PROTOTYPE，尚未整合進 Next）

- §18 的 `kqueue NOTE_EXIT` FD 由 Next Core 持有；Core 被 kill/crash 時 FD 隨之消失，無法觀測之後才結束的 worker。§20／§21 的定期 kernel PPID 子孫取樣也會在 Core 結束時中斷。**不能靠增加 PID 輪詢頻率解決跨 Core 存活的觀測責任。**
- 新增 **未註冊／未打包／未在正式執行路徑呼叫**的 `cmd/agentdock-next-exit-observer`（Darwin-only source prototype）與 `process.WatchIndependentWorkerExit(ctx,IndependentExitRequest,onArmed)`。受控 Core 與 stdio wrapper 必須在**兩者都仍活著**時以 kernel birth timestamp/UID/PGID/PPID 核對為 Core 的**直接子程序**；獨立 watcher 再以雙重 kernel birth snapshot 驗證並註冊 `EVFILT_PROC NOTE_EXIT`。observer 需要由未來的 launcher 放在**獨立 process session/lifecycle**，才能在 Core 自身故障後繼續持有 kqueue FD。
- Source prototype 從限制 4KiB JSON stdin 讀取 `{core,worker,state_dir,receipt_name}`；不接受不受控額外欄位。它只在 owner=目前 UID 且權限不開給 group/other 的私有目錄，以 `O_DIRECTORY/O_NOFOLLOW` 和 `O_EXCL/O_NOFOLLOW/O_CLOEXEC` 建立固定格式 `observer-*.v1` 0600 receipt，寫入 exact Core/worker birth witness 和 `state=armed` 後執行 file+directory fsync，才回覆 stdout 的 `OBSERVER_ARMED`。同一個 PID 的 NOTE_EXIT 真正到達後**唯讀**追加 `state=wrapper_exit_observed` 並再次 fsync。錯誤、PID 重用、親子驗證失敗、receipt 路徑不合法或事件未到達則**不能聲稱正常退出**；留下 armed 或無證據狀態由人判讀，不會 kill 或 unlink。
- Native integration test 使用**三個自行啟動的 Go TEST 子程序**：Core-like 程序啟動獨立 group 的 Go wrapper；獨立 observer 出具 durable armed receipt 後，測試將 Core-like 程序 **強制終止**，仍存活的 wrapper 由自己讀取私有 release sentinel 後**自然退出**；independent observer 最後收到 kernel exit event 並持久化記錄。另檢查 directory/receipt 權限、嚴格 filename、假的 PID/birth 拒絕且不出具 receipt。這證明 Core crash 後監看 FD 可以由**另一個程序**持有，但只對該次預先註冊的 wrapper 有效；不使用 Chrome/Edge/npx/AGY 或外部真實程序。
- 原型安全補強：在將 `state=wrapper_exit_observed` 寫回前，以 `Fstatat(...,AT_SYMLINK_NOFOLLOW)` 和被監看 receipt FD 的 `Fstat` 核對**原始 dev/inode、owner、regular file 與 mode**，替換或 symlink 偽裝不取得完成證據。命令 stdin 僅接一個 JSON 物件，未知欄位或第二個物件／尾隨垃圾拒絕。Linux／Windows 的函式 stub 全數回傳 unavailable，cmd 只拒絕執行；不會註冊 observer。
- **關鍵限制：**本原型**沒有**將 MCP stdio transport ownership 或真實 Chrome 子程序移交 independent supervisor，也沒實作 launchd 持續監督、崩潰後重連、IPC 驗證、可靠的解除保護條件或完整的 detached descendant 追蹤。甚至 wrapper NOTE_EXIT 證明成功，也**不能證明** double-fork/reparent 後的 Chrome 全部消失；原 Next sticky restart fence 的保守拒絕必須維持。本輪沒有 production runtime registration、auto-recovery、OS signal、部署／安裝／推送。下一步應先定義 supervisor-to-Core authenticated handoff + 狀態機，並確認 **worker/Chrome subtree 的真正 process ownership 應在同一個可跨 Core 續存的 supervisor 生命週期內**，否則只有 wrapper exit receipt 不夠安全。

- Source-only 驗證 PASS：完整 process/app/browser/browserpolicy/mcp-client + command Go tests；process/app/browser 的 targeted -race；上述模組 go vet；Linux/Windows amd64 no-CGO process/app/browser tests crossbuild 和新 observer command build；gofmt、diff check。沒有修改已安裝 App、launchd 設定、CDP 或使用者程序。


## 23. 2026-10-10 P8-B authenticated supervisor handoff contract（SOURCE ONLY；未整合 Next Runtime）

- §22 的獨立 kqueue watcher 已證明 Core 被強制終止後仍可保存 Worker NOTE_EXIT，但 Core 與 Supervisor 缺少可信交接與重放防護。本輪新增 `internal/process/supervisor_handoff.go` 唯讀 handoff contract 與 macOS `supervisor_handoff_peer_darwin.go`；沒有 production server、launchd、Next socket registration、MCP transport 或 Chrome subtree Supervisor ownership。**Core 目前仍是 Worker 直接父程序**，不宣稱實際 OS ownership 已移交。
- 每次 handoff 僅接受指定 Core、Supervisor、Worker 的完整 kernel BirthIdentity（PID／PPID／start sec-usec／UID／PGID，同 UID，Worker PPID 指向 Core、出生不早於 Core）。以 128-bit random Session ID 與 128-bit challenge nonce 建立 `attach-readonly` sequence=1，使用 HMAC-SHA256 綁定協定版本、session、nonce、三者完整 birth 身分。拒絕錯誤 secret、錯誤 worker 世代、越序/重播、未知操作、Core 消失後的再次 attach。
- Secret 僅存在每次 session 記憶體中，不放在 argv/env/URL/logs/TCP；未來必須從 Supervisor 經**受保護的 inherited FD 或同等可信通道**交付。隔離原生測試暫以自有 stdin pipe 傳遞 secret，尚不構成正式 secret provisioning/rotation 機制。
- Unix Socket admission 沿用 Next Desktop Control 現有 macOS OS peer 驗證：對收到完整 bounded frame 的**同一條 UnixConn**讀取 `LOCAL_PEERPID` + `LOCAL_PEERCRED`，並前後兩次 `ObserveBirthIdentity`，要求與指定 Core PID、UID、kernel birth 精確相符。只有 exported `AcceptVerifiedSupervisorHandoff(ctx,conn,handoff,frame)` 能進入等待中→已驗證交接；沒有 OS peer 核對的狀態變更 helper 不供其它 package 使用。非 Darwin stub 明確拒絕。
- Advisory 狀態為 `awaiting_core → core_attached → core_exited → wrapper_exit_observed`，或永久性 `uncertain`。Core 退出、Worker 退出等事件，只能由**真實獨立 Supervisor 已註冊的 kernel NOTE_EXIT**在可信環境下更新，絕不接受未驗證的 IPC 自行宣告。重放、Core crash 後重接、重複退出、併發雙交接皆 fail closed。**所有狀態均無 signal/kill、清 marker、自動重啟或 Chrome 子孫清理權限。**
- Native Go-only integration：test Supervisor 啟動自建 Core 子程序；Core 啟動自己的 Worker 子程序、取得私有 stdin pipe 的測試用 secret/challenge，透過 0700 私有 root 的 Unix Socket 傳送 MAC frame。Supervisor 核對 OS peer birth、檢查訊息並再次核對 peer 後才 ACK，Core 確認 ACK 才關閉 Socket。測試 PID/UID 假冒、MAC 變動、nonce/sequence/operation 錯誤、worker 世代不同、重播與併發 attach。Core 在測試中被強制終止後，test Supervisor 仍持有 Core 與 Worker 的 kernel NOTE_EXIT 監看；Worker 依 sentinel 自然退出，狀態依次變更。沒有啟動 Chrome、Edge、AGY、npx，沒有操作使用者程序。
- **尚缺**：Supervisor 自己啟動/持有 MCP stdio 子程序與 Chrome subtree 的 OS lifetime、可靠 secret IPC、受驗證 Socket endpoint、狀態和 Kernel Event 的原子持久化、Core restart reconnect、double-fork 後代完整追蹤、故障回收授權與三 Agent native no-focus UAT。此為 Source-only contract proof，sticky restart fence 不解除，production 不啟用。

- 原型收尾補強：已將「僅憑 Core/Worker 身分參數就更新 OS 退出狀態」的兩個方法收斂成 process package 內部使用（不作為外部 IPC handler）；唯讀 Socket 對端驗證仍是唯一公開 attach 入口。Native Core socket helper 在 Supervisor ACK 後才關閉連線，已獨立重測 5 次避免 peer identity 被提早關閉影響。完整 process/app/browser/browserpolicy/mcp-client Go tests、process/app/browser focused Race Detector、五模組 go vet、Linux／Windows no-CGO process/app/browser crossbuild、observer cmd build、gofmt/diff 全數 PASS。未使用真實 Chrome/AGY、未裝設或部署。


## 24. 2026-10-10 P8-B supervisor-owned stdio worker proof（SOURCE ONLY；不等於 Chrome subtree 安全回收）

- §23 的交接合約使用 Core 作為 Worker **直接父程序**，Supervisor 只是可信監看者；Core 死亡後沒有保存 stdio 傳輸的事實。新增**來源碼限定且沒有任何 runtime 呼叫**的 `internal/process/supervisor_owned_stdio_darwin.go`：由 Supervisor 自己以 `exec.Cmd.Start` 啟動受控 Worker，設定獨立 PGID、保留 stdin/stdout（真正 OS FD/Go command ownership），在**發布可用 Worker 前**透過兩次 macOS kernel identity 驗證**直接父子關係**，並以 `kqueue NOTE_EXIT` 綁定相同 birth identity。正常 Worker 結束時只能由 Supervisor 在自身生命週期讀取 NOTE_EXIT、呼叫 `cmd.Wait` reap；沒有公開 SIGKILL、killpg、journal-based termination 或 Chrome process control API。僅新啟動、尚未 reap 的**自身子程序**若初始化 kernel attestation 失敗，可執行 scoped rollback，絕不對歷史 PID/其他程序群組送訊號。非 Darwin 明確拒絕。
- 為防止把 Core-owned 假裝 Supervisor-owned，handoff contract 加入 `SupervisorWorkerParentCore`（舊版語意完全保留）和 `SupervisorWorkerParentSupervisor`。新的 `NewSupervisorOwnedReadOnlyHandoff` 僅接受 Worker 的 kernel ParentPID 等於指定 Supervisor、相同 UID、成立的 kernel birth chronology；`HandoffAttachMACForWorkerParent` 將**owner type 與三方完整 birth identity**一起放入 HMAC 的 domain-separated payload。不同 owner 模式之間的請求無法重放；舊版 Core-owned HMAC 函式仍僅接受 Core 子程序，且不能在 Supervisor-owned state 上通過。
- **真實隔離測試**的 Supervisor 是目前的 Go test process，自己的子程序包括（1）同一 Go binary 的受控 Worker，使用受驗證的直接 ParentPID 並由 Supervisor 持有 stdin/stdout；（2）另一個 Core-like Go binary，由後者透過 macOS Unix Socket、kernel peer birth+UID attestation 和一次性 HMAC 交接。測試在 Supervisor ACK 後強制結束**只屬於該測試**的 Core-like child；Supervisor 仍可從自己的 stdin/stdout 向 Worker 發出 `ping-after-core-crash` 並收到回應，後續傳遞 `exit` 讓 Worker 自然退出、收到 NOTE_EXIT 並 reap，狀態更新為 `wrapper_exit_observed`。所有交接憑證只在私人 stdin pipe 使用，沒有啟動 Chrome、Edge、npx、AGY，沒有碰既有 Core/Next 安裝程序。
- 嚴格檢查：不存在的命令／預配置外部 stdin／預設 SysProcAttr 或已啟動命令必須在 ownership prototype admission 前拒絕；Core-owned/Supervisor-owned 交錯或互相重播、錯誤 Worker ParentPID、錯誤 owner 字串皆 fail closed。工作生命週期保持 source-only，無 launchd、無 production IPC、無 real MCP go-sdk transport relay，也沒有讓既有 Next Browser WorkerRegistry 採用這個新 API。
- **剩餘阻擋條件不變**：本次證明 Supervisor 可以直接持有**單個 Worker 的 OS handle 與 stdio pipes**且跨 Core crash 仍可使用，並沒有證明 Chrome helper double-fork 後是否完全屬於同一個可回收的程序域。Production 需要建立 Supervisor-owned managed MCP stdio transport、只能授權特定 Browser Worker 的完整 IPC/Key lifecycle、跨 Core journal reconciliation、Chrome child lifetime completeness 與三代理無焦點 UAT；自動程序回收與解鎖 sticky marker 繼續停用。

- 同輪回歸發現並修正 §21 native test 的兩個 time-of-observation 競態：子程序在 `exec.Start()` 之後未必已完成 `setsid`，所以測試由子程序自身於 `setsid`／exec 後宣告 readiness，並把在群組切換前採樣到的 PGID 視為合法的**歷史觀測**（另外獨立確認目前 PGID 已切換）。另外 Wrapper 自然退出時 kernel UID sampling 可能先看到 PPID 鏈失效，而同一個 NOTE_EXIT watcher 尚在取得事件；新增**每 Worker 獨立**的 verified-exit 信號，對取樣失敗有界等待 350ms，僅在取得該 Worker 的可信 NOTE_EXIT 時避免誤設 `exitUncertain`，其餘錯誤仍保留 sticky marker；絕不根據其他 Worker 退出或 PID 消失解鎖。晚啟動測試連續 10 次 PASS。


## 25. 2026-10-10 P8-B Supervisor-owned MCP SDK transport（SOURCE ONLY；非 production MCP Relay）

- §24 只有 Supervisor-owned 原始 stdio pipes；本輪新增 `internal/mcp/client/supervisor_owned_mcp_darwin.go` 唯讀隔離整合，將 Supervisor **已經實際啟動、透過 kernel birth+direct parent 身分核對、已註冊 NOTE_EXIT**的 Worker stdio 直接交給現有 Go MCP SDK `mcp.IOTransport`：由 **Supervisor 程序內**的 MCP Client 執行 `initialize`、協定版本核對、`tools/list`、原有 catalog/schema validation 與 `tools/call`。與舊 `sdkProtocolClient.startStdioTransport` 完全分開，**不會由 Client 建立第二個 Worker**、沒有更改既有 Next WorkerRegistry、stable/exclusive、AGY ACP 或 managed CDP routing。
- 核心資源邊界：`SupervisorOwnedStdio.ClaimMCPTransport()` 只准在 worker still-owned/alive-ish（未 Wait/reap）且尚未 claim 時取一次 stdin/stdout，claim 後 `Stdin/Stdout()` 不再公開原始 pipe。MCP SDK Session 的 `Close` 只關閉 SDK session/transport（EOF）；**原 Supervisor 自己**保留 OS cmd ownership 與唯一 `NOTE_EXIT → cmd.Wait` 路徑，沒有對 durable PID、整個 PGID 或使用者瀏覽器的 signal 權限。當握手/工具清單失敗，會關閉已 claim 的 pipe、拒絕發布 SDK Session；是否得到正常 zero-exit 要由 Supervisor 等待並解讀 Worker 退出結果，不把 SIGPIPE 或不正常退出偽稱正常。
- macOS native test 使用現有自建 Go MCP helper（不是 Chrome/npx/AGY）：Supervisor（測試程序）直接啟動 Go MCP Worker，透過其 OS-owned pipes 建立 SDK session，正確完成 `initialize → tools/list → tools/call(echo)`；另外建立 Core-like Go 子程序，再**只強制終止自己的 Core-like 測試 child**，Supervisor SDK **同一條 session**繼續成功呼叫 echo，之後正常 `Close → EOF → NOTE_EXIT → Wait` 並確認 worker birth identity 和 single claim。異常測試涵蓋 transport config 不符（不可消耗 claim）、同一 wrapper 重複 claim/SDK attach、malformed MCP handshake fail-closed（可由 SIGPIPE 等非零退出，仍由 Supervisor Wait reap）、close 後拒絕 tool call。未操作使用者任何現有程序。
- 非 macOS 的 `ConnectSupervisorOwnedMCPPrototype` 明確回傳 unavailable；這是獨立 source-only API，**不代表 Core 已可跨 Supervisor IPC 呼叫 MCP tools**，更沒有正式 Supervisor daemon / socket routing / auth frame-to-tool binding、工具授權／使用者 profile 隔離、browser transport proxy、Chrome subprocess OS ownership completeness 或 crash-safe durable reconciliation。§24 的 CORE crash-proof 可操作 MCP Session 是**Supervisor 自己持有 SDK**，不是目前已安裝的 Next Browser path；自動 cleanup/解鎖 sticky fence 仍停用，未 deployment/install/push。下一步應加上由真實 Core kernel-verified Unix peer 進行 scoped MCP call 的驗證 request/response IPC prototype，及 Supervisor 隔離 worker ownership lifetime coverage。

- §25 收尾驗證：macOS 自建 MCP Helper SDK `initialize/list/call`，Core-like 子程序強制退出後同一 session 再次 call PASS；異常 MCP frame 可因 SDK 關閉 transport 而造成非零/SIGPIPE（不稱為 clean exit），但不得發布 session 且 Supervisor 保有 reap 證據。完整五模組 Go tests、process/app/mcpclient -race、五模組 vet、Linux／Windows amd64 no-CGO process/mcpclient/app/browser crossbuild、gofmt/diff 全部 PASS。


## 26. 2026-10-10 P8-B scoped Core-to-Supervisor MCP tool call（SOURCE-ONLY IPC PROOF）

- §25 只證明**Supervisor 自己**在 Core 消失後仍持有 MCP SDK Session，未允許 Next Core 以可信 IPC 呼叫 Supervisor MCP Worker。本輪新添 Darwin-only `internal/process/supervisor_tool_call*.go`、`internal/mcp/client/supervisor_tool_ipc*.go`：每次呼叫都必須另開一條**私有 Unix Socket**、一個**限 8 KiB 單筆 JSON**（固定 version=1 / operation=`call-tool-scoped`）與半關閉寫入端後取得的單筆 bounded response；兩端 socket deadline 5s。工具引數必須是 JSON object，canonical compact 上限 4096 bytes；回應上限 8192 bytes，超額直接 fail-closed（**不能**聲稱工具尚未執行）。接收方拒絕未知頂層欄位、尾隨第二請求、格式錯誤或超額請求。這只是 source-only API，**未註冊 Next Core route、未開啟正式 socket/port/service、無正式 CDP Chrome 實際操作**。
- 每個 HMAC-SHA256 請求都綁定指定 Core／Supervisor／Worker 完整 kernel BirthIdentity、`SupervisorWorkerParentSupervisor` 模式、session ID、連續 sequence（1,2,...）、工具名稱、Worker PID+birth 和 argument bytes SHA256。所有簽章都與 §23 的 attach 使用不同 HMAC domain，避免跨協定重播。狀態只允許**已交接而 Core 仍存活**；Supervisor 會從**目前的同一條 Unix Socket**重新檢查 macOS `LOCAL_PEERPID / LOCAL_PEERCRED` 與 Core 的 kernel birth，並從 macOS kernel 再次確認**Worker 仍是指定 Supervisor 的直接子程序且 birth 相同**。接收方還要比對被持有的 MCP SDK Session **原始 Worker birth 世代**，防止簽章對 A 成功卻透過 B 的 SDK 執行。非 Darwin 永遠拒絕。
- **權限界線**：工具的可用性**只能由 Supervisor 內部 trusted configuration** 提供明確 allowlist（例如 test `echo`）；無 allowlist → deny，不能由 Core 的 frame 指定或擴大 scope。驗證實際 MAC/OS peer/worker 與本地 allowlist 後，**先消耗這次 sequence 再執行** MCP ToolCall，因此重播即使前次工具執行失敗也不會再次執行（Core 重試不可假設 idempotent）。SDK 同時執行原有 MCP tool schema validator。這是**測試中的本地 grant proof**，**尚未整合 Next M8 user principal／approval evaluator、tool-level resource permissions、secret provisioning/rotation、跨 restart durable sequence log**，不能對 production 真實 Browser MCP tools 宣稱已完成授權。
- Native Mac Go-only integration：**自建 Core-like 子程序**透過從測試私有 stdin pipe 取得的一次性 secret 與 §23 attach challenge，先以真正 kernel Unix peer 完成 authenticated handoff。Supervisor（目前 test process）直接擁有受控 Go MCP Worker 的 exec/stdio/Go SDK Session；Core 發出兩次不同序號的 `echo` tool call，Supervisor 真正代理至自己的 SDK，回傳結構化 echo 結果；對同一序號重播、scope 外工具、被改動的 Worker birth 與被竄改引數（即使 JSON 仍合法）均拒絕、保留正確序號。即使另一個 live 同 UID 程序持有有效 attach MAC，也因 kernel peer birth 不符而拒絕冒用。Core-like 自有子程序被測試強制終止後，從另一個 PID 使用舊 Core 的 MAC 也遭拒絕，而 Supervisor 自有 SDK Session 仍能 echo。所有測試程序皆自建的 Go helper，沒有真實 Chrome/Edge/AGY 或使用者現有程序/視窗。
- **未解事項**：實際跨 Next Core restart 的安全重新認證、M8 principal/approval/角色與 Browser capability 綁定、privileged key delivery（測試 secret 只從私有 pipe 傳遞）、真實 Supervisor service/launchd IPC endpoint ownership、response durable commit/重試語意、Chrome 分離子程序的 OS 全量所有權、三代理與真正 Browser 的零搶焦點 UAT。**自動回收、kill/unlink sticky marker 及 crash restart 仍關閉**。這一輪不能從 IPC 可呼叫工具推論可清理 Chrome orphan。


## 27. 2026-10-10 P8-B M8 Permission admission for Supervisor IPC（SOURCE ONLY）

- §26 的本機 Core/Worker kernel birth、HMAC、嚴格序號與 trusted local allowlist 不能當成「使用者已被核准」；OS LOCAL_PEERPID/UID 只能證明**是哪個 Core 程序**。新增 `internal/mcp/client/supervisor_m8_admission.go` 的**可信 Core 專用** `AdmitAndSignSupervisorM8Call`：請求的 principal / runtime epoch / source / audit CallID 必須**已從 Core 的 authenticated context 導出**，絕不能從外部 Socket、ACP request body 或工具引數讀取身分。stable+authenticated principal、既有 `permission.AdmissionGate` / store epoch 缺一不可，fail closed。此 API **尚未與實際 Next Runtime 接線**；開發者仍需用既有 requestmeta / execution 取得真正 principal、CallID，不能由 wire 自行宣稱為 authenticated。
- 在真正產生簽章**之前**先用保守 M8 facts 做 admission：`Tool=supervisor_mcp/<name>`, `Action=call`, `MCP=true`, `Network=true`, `OpaqueProviderExecution=true`, `EffectsKnown=false`，不推論 MCP Provider 的 side effects 為唯讀；現有 M8 policy 的 Full/Rules/Deny/Ask、hard constraints、profile、一次性核准均原樣生效。準備的精確 fingerprint 包含**工具、JSON arguments、Worker PID/birth 與 Supervisor session**，CallID 單獨供原有 Store 辨識重試。若 Ask，M8 Store 真正建立 pending 記錄但**不給工具框架可執行的簽章**；若 Deny，同樣沒有可執行簽章；只有 Allow（包含原有 `ApproveOnce` 所核准且由 Store 原子消耗的完全相符 retry）才簽章。使用一次性核准後，Core 呼叫 `SettleSupervisorM8Dispatch`，在 IPC/回應狀態不確定時保守記為 Unknown、不能重複執行有副作用的工具。
- 新增 HMAC-bound `SupervisorPermissionProof`：`policy_revision`, `runtime_epoch`, prepared request SHA256 fingerprint, **opaque principal hash**, `consumed_approval_id`（若適用）及 Allow 版本標誌；不把 user ID、完整 principal 或 Store JSON 放進 IPC。Proof bytes 也受原本 complete Core/Supervisor/Worker generation + tool/args/sequence MAC 保護，竄改不可能被有效驗證。新增獨立 `ServeSupervisorM8ScopedMCPCallPrototype`，此入口**必須**提供格式正確且已 MAC 綁定的 M8 Allow Proof；舊 allowlist-only 原型 endpoint 留作 source-only 舊測試用途，**禁止接入 production 或代替 M8 入口**。Supervisor 本身仍不能獨立驗證 Core 的完整 policy Store 或 mint principal，只能驗證 attested trusted Core 提供的簽名 proof，再套自己的 OS-peer/worker 和 local allowlist gate。
- Native macOS Go-only UAT：Core-like process 透過 M8 `AdmissionGate` 與 test-only trusted principal，真的產生 allow proof，從**同一個 OS attested Core Unix peer**送出 2 個 `echo` calls 並取得 Supervisor-owned MCP SDK 回覆；同一 Core 的 replay、scope 外工具、被改動 arguments、錯誤 Worker、**缺少 M8 proof 的 legacy frame**、被改動的 proof policy revision 都被拒絕；另一個 PID 冒用與 Core death 後重用也被拒絕。測試排除 socket AcceptUnix 排隊競態：先完成 fake peer 的驗證，才把 secret/challenge 透過 test-private stdin pipe 交給真正的 Core；native negative + positive 路徑連續 5 次 PASS。單元測試用真 M8 Store Rules policy 驗證 Ask 會持久化 pending、桌面可信 actor `ApproveOnce`、相符 retry 被消耗並可 settle Unknown、再重試不能重用、明確 Deny 或 unauthenticated principal 不能簽章。
- **阻擋與邊界**：此為 source-only Core admission → MAC permit → Supervisor gated IPC **原型**。仍未將實際 requestmeta/ACP principal、M8 Runtime.Call 或 Browser WorkerRegistry 路由接上；未實作 production protected handshake key 交付、跨 Core policy revision freshness/re-auth、跨 crash request durability、approval retry dispatch accounting integration，亦無真實 Chrome subtree OS-owned cleanup proof。即使 Core 原型簽章成功也**不能**擴大 Supervisor local allowlist、auto-kill/auto-restart 或解除 sticky browser crash marker。未安裝、部署、推送或修改現有 Chrome/Edge/AGY。

- 本輪驗證結果：Go 完整 6 模組 `process/permission/app/browser/browserpolicy/mcpclient`、定點 process/mcpclient/permission/app `-race`、六模組 `go vet`、Linux/Windows amd64 no-CGO process/permission/mcpclient/app/browser 交叉編譯、gofmt/diff 全數 PASS。M8 pending/approved-once consume 與 native Core→Supervisor MCP tool 的 positive/negative 測試均 PASS（native 新舊兩入口連續 5 次）；沒有觸及已安裝 App 或使用者瀏覽器。


## 28. 2026-10-11 P8-B Runtime principal binding（SOURCE ONLY）

- `27 的 `AdmitAndSignSupervisorM8Call` 假設參數 `CoreSupervisorM8Context` 已由可信 Core Runtime 建立。本輪新增 `internal/app/runtime_supervisor_m8_scope.go` 內部 adapter `Runtime.admitAndSignSupervisorM8FromRuntime`，將此假設具體綁定至現有 `requestmeta.AuthPrincipalFromContext` + `execution.Store.Begin`。目前**沒有**將 Supervisor 工具呼叫變成公開工具／HTTP／BrowserRegistry route。
- 每次 `Runtime.Call` 完成 `execution.Begin` 後，僅在工具為 `mcp_tool_call`、Call still running、來源一致且真的有 `Authenticated+Stable` principal 時，額外建立 Core **私有 context key**。其中固定 Runtime instance、original principal、execution CallID/ParentCallID、來源、execution Epoch。這只是保留原本驗證結果的 source-only marker，**不改變正常 Runtime Tool Handler**，不接受 JSON/ACP/Socket 自行聲稱的 Principal/CallID。
- Source-only adapter 會從同一個 context 重新核對：private marker（非公開 scope ID）、相同 Runtime instance、原始 principal/source/CallID、`Runtime.execution.Epoch()` 和 M8 Store epoch 一致，以及 `execution.Store.Call` 中指定 CallID 仍然是 `mcp_tool_call` / `StatusRunning`、ParentCallID 沒被替換。只有全部通過才將 trusted binding + audit call metadata 送給**原有 M8 `permission.AdmissionGate`**，在 Allow/Ask/Deny 之前不會建立可執行的 Supervisor MAC。
- 若要 production 仍需讓 validated M8 admitted call 對應真正 Supervisor MCP request/response lifecycle；金鑰安全傳遞、可取消的 request 狀態/approval settlement、跨 Core restart、Chrome detached subtree ownership 與真正三代理無焦點 UAT 均尚未完成。`26 的 legacy allowlist-only endpoint **不得**做 production fallback，sticky restart fence 和自動 orphan cleanup 仍保持關閉。
- 測試使用真正 `NewRuntime` / `requestmeta.NewStableAuthPrincipal` / `execution.Store` / M8 `AdmissionGate`。正向取得 Allow proof；只持有公開 execution CallID 卻無 private marker、無 principal、principal/source/CallID 被換、跨 Runtime、Call 已結束、或在非 `mcp_tool_call` 執行期間嘗試簽章都拒絕。真 M8 Rules/Ask/ApproveOnce：不同 principal、不同 args 不能消耗原核准，只有同一 principal 的精確 retry 可消耗一次且 settle Unknown，再重試必須 Ask。這是隔離 source-only 證明，**不是實際 Browser/ACP UAT**。


- 同輪完整 Race Detector 發現 `TestSharedBrowserLiveLateSpawnCrossGroupNative` 在 root/setsid grandchild 退出期間偶爾看到不完整的 kernel UID/PPID snapshot，導致保守的 `exitUncertain`。這是**既有 P8 descendant sampling 的有效 fail-closed 結果**：即使同一 root 的 `NOTE_EXIT` 成功，不能憑此證明另個 session 的孫程序皆退出。本輪只把 native regression 改成驗證兩個**均安全**的合法結果：快照完整則 release 後 marker 刪除；觀測不確定則 `releaseAfterVerifiedClose` **必須拒絕解除**、marker **必須持續存在**、新的 guard **必須拒絕重啟**。無論哪條分支都先要求 root kqueue 真正收到 exit；未放寬 production sampler/error guard，也不允許直接 kill/signal Chrome。一般模式 10 次 + Race Detector 5 次 PASS。


- 本輪驗證 PASS：Go 完整六模組 `process/permission/app/tool/browser/browserpolicy/mcpclient`；定點 `process/app/mcpclient/permission` Race Detector；六模組 `go vet`；Linux/Windows amd64 no-CGO `process/permission/mcpclient/app/tool/browser` test cross-build；source-only private scope Go tests；gofmt/diff。`TestSharedBrowserLiveLateSpawnCrossGroupNative` 一般 10 次、Race Detector 5 次皆 PASS。沒有修改 Supervisor production daemon/launchd、實際 Browser/Chrome 啟動路徑或 ACP/AGY 使用者程序。


## 29. 2026-10-11 P8-B Supervisor Core-generation rotation guard（SOURCE ONLY；ambiguous dispatch 一律凍結）

- `23–`28 已有 kernel birth、Unix peer 和 M8 call MAC，但 Core 重啟後不能沿用上一代 PID、Session、nonce、secret、sequence 或待回覆中的工具呼叫。本輪新增 Darwin-only `process.SupervisorCoreRotation`（source-only；無 Runtime 接線）：只有目前實際 Supervisor OS PID/出生身分持有的直接 Worker 子程序仍完整匹配、Core 仍活著時，才先註冊 Core 的 kernel `NOTE_EXIT`，然後發出第一代交接。
- 過渡須經兩道驗證：`WitnessCoreExit` 只以已預註冊的 kernel NOTE_EXIT 確定上一代 Core 退出；`AdmitSuccessor` 重新查證 Supervisor/Worker/新 Core 的 live kernel births，為新 Core 建立獨立 Session/nonce 和新 32-byte secret，不能重用之前任何一代 key、Core birth、MAC 或 Session。所有交接仍必須通過真實 Unix peer PID/UID/birth。最多允許 16 個世代，避免無界持有鍵指紋；Linux/Windows fail closed。
- **關鍵保守原則**：上一代如果哪怕有一筆工具請求已通過 `acceptVerifiedToolCall`（即 sequence 已消耗），由於目前**沒有持久化的 MCP dispatch/outcome 記錄**，就不能推論 Core 崩潰時工具是否已執行或能否重送；此情況 manager 永久進入 uncertain、拒絕新的 Core reauth/IPC，**即使舊 Core NOTE_EXIT 和 Worker OS 身分都成功驗證**。所以只允許「上一代從未接受工具請求」的情境重新認證。OS witness 取消／錯誤、伪造 birth、重複 secret、未知狀態同樣 fail closed；不會刪除 sticky restart marker、kill Chrome 或重播 MCP side-effect。
- kqueue Core exit 可能需長時間等待，`WitnessCoreExit` 在 kernel wait 期間釋放 manager mutex，維持 CurrentHandoff health-check 可用；只允許單一 wait owner，取消／失敗後永久隔離。此 in-memory 16-generation gate **僅 Supervisor 程序存活期間成立**，不是 Supervisor 自身 crash 後的 durable recovery。
- macOS native Go-only test：Test Supervisor 自己直接啟動 Go Worker，分別建立 Core A 和 Core B 兩個 Go child，使用私人 0700 Unix socket / 0600 endpoint 和私人 stdin secret pipe，完成兩代真實 OS peer+HMAC attach；Core A 被**測試自己**強制終止且 NOTE_EXIT 證實、未接過工具時 Core B 用全新 key/session 交接成功；舊 MAC 和舊簽工具請求被拒。另一情境 Core A 先接受一個可能執行過的工具 call 再被終止，即使有 NOTE_EXIT 仍不能重接。測試包含 forged Supervisor/Worker birth、取消的 NOTE_EXIT，以及持續監督不阻塞 live Core 的 mutex。沒有 Chrome/Edge/AGY、正式 Next、系統程序控制或部署。
- 尚缺：保護性 secret provisioning（測試 pipe 不等於 production）、Supervisor launchd runtime、受核准 MCP 工具 dispatch/response durable ledger、跨 Supervisor restart 的持久 crash fence、Chrome double-fork 子孫完整 owned OS handles 與三代理真實無焦點 UAT。這一輪不得視作生產級自動恢復、不能啟用任意 Core crash reattach 或解除原 Next sticky marker。


- 驗證 checkpoint：macOS native 自建 Supervisor+Worker+Core A/B 可信交接與 Core NOTE_EXIT、重放拒絕、ambiguous dispatch permanent freeze、假 birth、取消 witness、長等候不阻塞 live Core，都已多次通過。完整六模組 Go tests、process/app/mcp-client targeted Race Detector、六模組 go vet、Linux/Windows amd64 no-CGO process/permission/mcpclient/app/browser test crosscompile、gofmt/diff check PASS。未安裝、部署、推送，未觸及 Chrome/Edge/AGY。


## 30. 2026-10-11 P8-B Supervisor MCP dispatch receipts（SOURCE ONLY；write-ahead + fail closed）

- `29 的 SupervisorCoreRotation 在 Core 已接納任何工具請求後會 permanent uncertain，因為沒有跨 Core 的實際請求與 SDK outcome durable receipts。本輪先新增 macOS-only `process.SupervisorDispatchLedger`，採獨立來源碼原型與 private existing state dir（由 trusted Supervisor 建立）。只接受絕對目錄路徑、private `0700` + 當前 UID；使用 dirfd `O_DIRECTORY|O_NOFOLLOW`，單筆 `dispatch-<session>-<sequence>.v1` 的 `O_EXCL|O_APPEND|O_NOFOLLOW` 0600 regular inode；限 64 筆、單檔 4KiB，name / UID / mode / nlink / symlink / inode 不符、意外其他檔案或 journal 過量全部 fail-closed。Append 時比對 path 的 inode/dev 與 pin 住的 FD，並同步檔案與目錄；絕不 unlink、不修復、不 kill 歷史 PID。
- Durable execution boundary：在原本 OS Unix peer Core PID/UID/birth、HMAC session/sequence、M8 Allow proof、Worker direct-parent birth 與 Supervisor trusted allowlist 全數通過後，**必須在呼叫 MCP SDK 之前**持久建立 `State=accepted` 收據（只記 frame SHA256、Core+Worker birth、session/sequence，不記工具參數或使用者身分）；**成功 fsync 才消耗 one-shot sequence**。若 create/append/fsync/dir permission 失敗，即使 HMAC 正確也不得執行工具，handoff 永久標 `uncertain`；相同序號不得再次入帳。使用新 `AcceptVerifiedSupervisorToolCallDurablePrototype`，原 allowlist-only 與尚未接 journal 的 M8 source-only 路徑仍保留，但不得接 production。
- SDK 返回後，新的 `ServeSupervisorM8DurableMCPCallPrototype` 必須在回覆 Core 前追加並 fsync `State=settled`，附 outcome（`succeeded` / `failed` / `unknown`）及回覆 SHA256，不儲存原始工具 output 或 token。若 outcome 持久化失敗、回覆過量或無法序列化，立即 `MarkUncertain` 並拒絕成功回覆。`failed` 只代表 Supervisor 看到 SDK 錯誤，不證明工具沒有副作用；`succeeded` 只代表 Supervisor 看見 SDK return，**不證明 Core 已收到回覆**。此為獨立 M8-gated prototype IPC，正式 Next Runtime/ACP BrowserRegistry/Chrome 仍未連上。
- `InspectSupervisorDispatchReceipt(stateDir, originalSignedFrame)` 僅唯讀：以 owner-private dirfd + O_NOFOLLOW 讀原始 receipt，重新檢查 inode/UID/mode、大小及 JSON 行數恰好 1（accepted）或 2（accepted+settled），核對原 signed frame session/sequence/request digest、合法 outcome/reply digest。結尾無換行、symlink、檔案遭替換、資料截斷或多出第三行都拒絕。只有一行意味著**可能已執行但結果未知**；兩行也**不授權工具重播**。這不是跨 Supervisor crash 的不可偽造交易記錄，**不能用來解除 `29 的世代鎖定**。
- macOS native Go-only test 使用自建 MCP Worker、Core Unix peer 與 M8 admission，真實走兩次 echo MCP 呼叫，檢查 Core 收到回覆後 private ledger **剛好兩筆** settled/succeeded；重播、scope 不符、缺 M8 proof、Worker birth/引數/proof 被改動都沒有收據。單元測試涵蓋 accepted-only 模擬當機窗口、ledger reopen 同序號 O_EXCL 拒絕、雙 Finish 拒絕、並行 Begin 同序號只接受一次、目錄權限錯、symlink 替換、torn JSON 和未知 state file fail-closed。故意降低私有目錄權限後，已簽章 frame **不可先消耗序號或執行工具**，handoff 永久 uncertain。所有 fixture 僅是 Go 子程序，不碰使用者 Chrome/Edge/npx/AGY。
- 剩餘 blocking：收據僅記 Supervisor SDK 本地觀測結果，沒有 Core receipt ACK、跨 Supervisor restart 的 journal reconciliation、工具執行結果去重交付、M8 approval Store 與本 journal 的原子 transaction，亦未證明 Chrome detached descendant 的完整 OS ownership。**已接受工具的 Core crash 仍禁止自動 reattach**；不允許自動 Browser cleanup/restart 或 marker unlink。Production 必須先解決完整持久化 request state-machine、安全 key lifecycle 與真正三代理無焦點 UAT。


## 31. 2026-10-11 P8-B Core reply ACK + durable receipt（SOURCE ONLY；不開放 replay/restart）

- `30 只證明 Supervisor 以 fsync 紀錄 MCP SDK 觀測到的結果，**不等於 Core 真的收到回覆**。這輪新增 Mac-only `SupervisorDispatchAckFrame`（version=1 / operation=`ack-core-received`）：由已驗證並持有私人 secret 的 Core 在**原本單次 MCP ToolCall IPC 回覆完整收到並驗證後**簽章，MAC 與工具呼叫 domain 不同，涵蓋完整 Core／Supervisor／Worker kernel BirthIdentity、Session、Sequence、原簽章請求 SHA256 和 Core 已收到回覆的 JSON SHA256。Core 不能單靠另一個 M8 grant、工具名稱或 PID 自稱 ACK；不傳遞 secret、原始回覆或使用者身分至 Supervisor journal。
- `SendSupervisorCoreACKPrototype` 在每筆已收到回覆的工具呼叫**另外建立全新 Unix Socket**送出 request+ACK frame，限制 12KiB 並驗證唯一回應；`ServeSupervisorCoreACKPrototype` 必須在該 socket 讀完整單次 frame、限制長度與 5s deadline，然後重新驗證同一個**仍存活**的 Core Unix LOCAL_PEERPID／LOCAL_PEERCRED 與完整 kernel birth、Supervisor 持有的 Worker live direct-child birth、同代 attached state 以及 domain-separated ACK MAC。另一個 PID 即使拿到已簽章 ACK，也不能冒充 Core。Core exit 後或換代後舊 ACK 不會被接受。
- `SupervisorDispatchLedger.ackVerified` 在 owner-private dirfd 上開同一個 `O_RDWR|O_APPEND|O_NOFOLLOW` 的 0600 pinned receipt，拒絕錯誤 Session/Sequence/Request SHA、錯誤 Core/Worker 出生世代、不相符 reply SHA、unknown outcome、沒有 `settled` 的 accepted-only、duplicate ACK 或檔案非可信 inode/UID/mode/size。完整重讀並嚴格驗證原 accepted+settled 後，追加第 3 行 `State=acknowledged` + reply SHA256；同步檔案與目錄才回覆 success。`InspectSupervisorDispatchReceipt` 支援只含 1／2／3 行，並禁止在 first/second JSON 混入偽造的 `Acknowledged` 旗標。ACK append 失敗、讀取損毀或完整回覆 ACK 不確定都不會擴權，無自動重播、補寫成功、刪 marker 或清理 Chrome。
- **語意界線**：ACK 是「指定 Core 程序對指定回覆做了簽章且 Supervisor 持久收到」的證據，**不證明**使用者收到了該結果、Core 在當機前已將結果持久化、M8 ApproveOnce 與 Supervisor journal 已實作原子交易、工具沒有外部副作用，也不能用 ACK 來解鎖 `29 的 Core rotation gate（該 Gate 仍拒絕任何上一代曾接受工具的 Core crash reattach）。如果 Core 在送 ACK 前 crash，`settled` 仍不能當成已確認；如果 ACK 已寫入但回應 ACK itself 斷線，只能 read-only inspection，不能重送 tool。
- macOS 隔離 native UAT：自建 Go Core（不是實際 Next）透過真實 kernel-verified Unix peer＋M8 AdmissionGate 呼叫 Supervisor-owned Go MCP SDK Worker 的兩次 echo；Core 對每次已收到回覆簽 ACK，另外開 Unix Socket 經 kernel peer+MAC 驗證，由 Supervisor 在 `30 的兩筆 private ledger 補第三行；讀回恰好 2 筆 settled+acknowledged，且把**同一份有效 MAC 的 ACK**改由測試 Supervisor 程序送出會遭拒絕。負面測試 accepted-only、unknown outcome、不同 reply/request/sequence/worker/Core birth、重播、同時 16 個 ACK 僅允許一個、torn receipt 拒絕。負面案例實際找出 `ackVerified` 底層缺少獨立序號驗證，已補強，不只依賴外層 socket handler。
- **仍是 source-only**。正式 Next Runtime 接線、可審計 Core durable inbox/ACK redelivery、Supervisor service launchd/key provisioning、M8 Store 與 dispatcher reconciliation、Chrome 完整 detached process ownership、三代理共用瀏覽器 no-focus UAT 都尚未完成；不安裝／部署／推送、不碰 Edge/Chrome/AGY、不啟用任意 process kill、auto restart 或 sticky marker unlink。


## 32. 2026-10-12 P8 aggregate shared-pool Browser Desktop diagnostics (SOURCE ONLY)

接續 P7（macOS Logs/Config 使用者手動驗收 OK，NexusDock 已標示「開發中」且 live pairing defer）後，先落地低風險、**完全唯讀**的 P8 Browser Broker operator observability，**不啟用／切換 managed shared Chrome、Profile、CDP、MCP Connector 或其他 runtime 控制**。

- `ACPBridge.Diagnostics().SharedBudget` 是既有實驗性 Next-only pool 的 mutex-consistent aggregate bookkeeping，與實際 OS Chrome process、Profile 登入、CDP 授權、零彈窗／不搶焦點、orphan 已清理皆無等價關係；繼續保留 single canonical workspace opt-in，未 opt-in 時 production 默認 exclusive。
- `internal/app/runtime_browser_desktop.go` 僅在 Broker 正常可讀後投影 mode：`exclusive` / `experimental_shared`；Broker 不可用時 `unknown`。shared budget 僅允許 capacity/occupied/available/claimed/uncertain 五個整數且必須滿足非負、occupied ≤ capacity、available = capacity - occupied、uncertain ≤ occupied；**任何不一致全數 fail closed 為 `broker_unavailable` + `unknown`，不投影部分 counts**。已驗證的 `uncertain > 0` 則將 Broker retained state 標記 `stale`（即使 leases 為零），不誤報 `idle`。Owner／Worker／Profile／PID／URL／credential／path／page／token 等未加入 Desktop DTO。
- `internal/browserdesktop.Snapshot` 及 Wails generated bindings 新增 allowlisted mode + optional aggregate budget；`BrowserSnapshotDetails.vue` 三語系唯讀區塊清楚聲明只是 Broker 帳務／容量，不代表真實 browser／登入／同意。Broker 不可用仍可呈現未知模式，但舊的 leases/worker/health 仍因不可用而隱藏。
- 合成 fixture 驗證 empty/exclusive/experimental、合法與負值／不一致 budget、序列化白名單＋隱私 canary、共享 Broker 讀 snapshot 不執行 Probe/Open／mutate worker，並覆蓋 Vue 與三語系。此 UI 並非 Shared Worker 已就緒、已通過原生三 Agent UAT 或 consent/no-focus acceptance 的證據。

- 驗證紀錄（source-only）：定點 Go `TestRuntimeBrowserDesktop*` + Desktop Browser PASS；`go vet -p 2 ./internal/app ./internal/browserdesktop ./internal/desktopapi` PASS；Vue `pnpm typecheck` 與全量 Vitest **142/142 PASS**；`pnpm build` PASS（僅既有 >500 KiB chunk 建議）；canonical `go run ./tools/i18n coverage` en/zh-Hant/zh-Hans 均 **867/867**。`go run ./tools/i18n check` 的 freshness 通過，但既有 Permission/ACP 等未修改檔案的 literal scan 仍 FAIL（不宣告 global i18n check PASS）。
- **Gate**：Next source-only，未安裝／部署／restart／啟動 Chrome／Edge／npx，沒有接觸使用者 Session/Profile；Edge C2b native authority、managed pool process/orphan trust、實際三個真實 Agent 的連續 UAT 與 no-focus/no-modal 仍 open；P8 / P9 / M9 皆未結案。



## 33. 2026-10-12 P8 real headless shared pool diagnostics/foreign workspace acceptance (SOURCE ONLY)

接續 §32 共享池唯讀診斷。嚴格採用既有 test-only、顯式 `AGENTDOCK_TEST_REAL_SHARED_ACP_BRIDGE=1` 的 Darwin/Linux `TestACPBridgeSharedRealHeadless`，使用 cached exact `chrome-devtools-mcp@1.7.0` / npm offline、`--isolated --headless`、測試專屬暫存資料夾及 `about:blank`；**沒有將實驗性 shared runtime 改為預設、沒有使用既有 Edge／Chrome Profile，也未部署 Next**。

- **主管端 real proof baseline PASS（41.98s）**：三個模擬 ChatGPT/Codex/AGY owner tokens 經真實 ACPBridge、MCP 1.7.0 與同世代一個 isolated Headless Chrome，完成交錯 JS 狀態隔離、拒絕跨 owner/page/lease、逐個釋放自己的 page、pool keeper 保留、閒置 reap、第二世代重新取得新 Lease 且舊權杖被拒；最終 owned MCP/Chrome 及 test marker 程序皆歸零。
- **新增 real harness assertion**（僅 `internal/tool/browser/acp_bridge_shared_real_test.go`）：`checkBudget` 每次同時讀 `ACPBridge.Diagnostics().SharedBudget`，核對 capacity=1、occupied/available、owner claims=1（完整 Bridge Close 後 0）、uncertain=0；JSON 僅五個 numeric fields，不含 Workspace、Session 身分；診斷前後 owned-session 的 starts/live/peak 必須不變。這是 budget 帳務與實際測試觀察的一致性驗證，**不等於 snapshot 自動驗證 OS Chrome PID／使用者授權**。
- 另用一個隔離的 foreign canonical Workspace 註冊測試 owner，確認 `Acquire` 回傳既有 opaque `errManagedSharedUnavailable`、且尚未啟動 Worker；其註冊隨即安全 Release，不觸碰另一 Workspace 的瀏覽器。
- **主管端修正後 real rerun PASS（34.91s）**：真實 3 simulated owners / MCP worker / 1 Chrome main per generation；兩個世代 budget capacity=1、owned peak=1；新增診斷及 foreign Workspace deny checks 全數通過；test cleanup 確認 owned MCP/Chrome processes gone、無專用 TMPDIR marker 程序、budget=0。
- Regression：`go test -race -p 2 ./internal/tool/browser -run '^(TestACPBridgeShared|TestManagedShared)' -count=1 -timeout=160s` PASS；`go test -p 2 ./internal/tool/browser ./internal/browserpolicy -count=1 -timeout=150s` PASS；`go vet ./internal/tool/browser ./internal/browserpolicy`、gofmt、`git diff --check` PASS。一般 Go suite 未設定 opt-in，不會意外開 Chrome。
- **依然待處理**：這三個仍是 test-only *模擬 Session identity*，不是三個真實 ChatGPT+ACP client 同時通過 Next installed Broker 的證據；使用者桌面的 focus/no-modal 沒有經 macOS 觀測驗收，Chrome/Edge 已登入 Profile、consent、安全授權、crash orphan OS authority、跨 workspace pool 與 producer→Next Core→Wails native source parity 尚未 release acceptance；不得藉本結果啟用正式 shared mode。stable Core 8765 與 Next Core 8767 均未 restart，未改用戶瀏覽器、未安裝／推送。



## 34. 2026-10-12 P8 Supervisor durable journal structural audit (SOURCE ONLY)

延續 §29–31 的 Supervisor durable `accepted → settled → acknowledged` receipts，新增 macOS-only `process.AuditSupervisorDispatchJournal(stateDir)`。這是一個 **source-only、唯讀、非認證式、非復原授權** 的有限稽核工具；沒有接線到 Next Core／BrowserRegistry／ACP，沒有新增 daemon、browser process 控制、journal 維護／回放／修復權限。

- 輸出僅為五個整數：`Total`、`AcceptedOnly`、`Settled`（包含 acknowledged）、`Acknowledged`、`UnknownOutcome`（settled 子集合）。不輸出檔名、Principal、Session、PID/birth、HMAC、工具名稱／參數、Profile、完整輸出或記錄路徑。即使 **0 筆或全數 acknowledged**，也**不能**推論 restart、replay、approval settlement、unquarantine、marker unlink 或 Chrome descendants 已清理。
- 採現有 owner-private `0700`、當前 UID、絕對目錄與 `O_DIRECTORY|O_NOFOLLOW` dirfd；限最多 64 筆、單檔最多 4096 bytes、每筆 pinned `0600` regular、nlink=1 與 path stat/inode/dev；執行前後重新驗證 directory identity、file inode／metadata 與清單一致。`O_NONBLOCK` 避免遭置換為 FIFO 時讀取卡住。缺／多／損壞／截斷／額外 JSON 行、非預期檔名或 mode、symlink／hardlink／被換 inode／過量收據全部 return generic `ErrSupervisorDispatchLedgerUnavailable`，**不回傳部分計數**。既有 signed-frame 單筆 `InspectSupervisorDispatchReceipt` 與 ledger Begin/Finish/ACK 寫入路徑沒有變動。
- 新收據校驗 accepted、settled、acknowledged 順序與合法欄位／出生識別、session/sequence 檔名對應和 64-hex 摘要；採持久化 `json.Marshal` 完全相同的 canonical bytes 驗證，拒絕重複 JSON key、任意欄位重排／空白造成的結構歧義。**不能**在沒有原始 signed frame、MAC secret 和 trusted live kernel peer 的情況下驗證 digest 或 HMAC 真正對應工具，因此這些數字只能作人工 crash triage，絕不作 Core successor gate。
- macOS 隔離 fixture（不啟動真實 Worker/Chrome）：空檔、accepted-only、settled-success/failed/unknown、acked，多 session 匯總；不完整／多出行、無效 sequence／digest／birth、替換目錄／inode、符號／硬連結、不安全 mode/UID（合成 stat）、超過 64 筆、大小 >4KiB、一份壞記錄使所有已知計數歸零；非 Darwin stub 明確 fail-closed，Linux/Windows amd64 no-CGO compile。
- 適用性界線：仍**不能**取得真實 ChatGPT/Codex/AGY 三路 Client → Next installed Browser Broker 的 end-to-end authority；沒有真實桌面 no-focus/zero prompt、Edge/explicit Chrome Profile consent、Chrome double-fork descendant OS 全量歸屬、Supervisor 重啟後金鑰重新驗證、跨 M8 store/journal 原子結算或安全重啟。P8 及 M9 不宣告 closeout。



## 35. 2026-10-12 P8 authenticated ACP Browser MCP transport evidence (SOURCE ONLY)

上一輪 P8 §34 的 Supervisor crash ledger 仍沒有正式 runtime 接線。為解決接下來真實 ACP 多代理 Browser UAT 的**觀測盲點**，本輪在現有 M6/Next Browser MCP 路徑加上 **已註冊 Session vs. 實際經有效 Bearer token 存取 MCP HTTP endpoint** 兩種不同的資料證據；這是已存在於 Core 的 Browser MCP HTTP Handler 路徑，非新增 Chrome/Worker 操作，也未把 Source-only 共用 Chrome 變成 production default。

- `ACPBridge.RegisterSession` 後只建立 profile/session/scoped-token owner，不再將註冊當作 client 已連線的證據。`MCPHTTPHandler()` 成功核對 active owner Bearer token 時，在相同 `ACPBridge.mu` 下記錄該 owner 的單一 `authenticatedMCPRequestObserved`，不累加請求數，避免多次健康檢查／tools/list 虛增 Session。原有通用 `owner()`／只讀 Diagnostics **不能**觸發觀測；ReleaseSession 從 active map 刪除 owner 後，同時撤銷這段資料，re-register 用新 token 才能重置。
- `BrokerDiagnostics.AuthenticatedMCPRequestOwners` 是持有有效 Session 的已觀測 owner 數，不是 MCP initialize/JSON-RPC 呼叫成功、工具成功、Worker process count 或 Profile 授權的證據：有效 token 即使向 endpoint 送 malformed JSON 也會被算入；錯誤 token 401 不計入。`RuntimeBrowserDesktop` 只投影 aggregate 到 `browserdesktop.Snapshot.AuthenticatedTransportOwners`，負值或超過 registered owners fail closed；原本 owner ID、profile ID、token、path、頁面/PID/URL 等不出 Desktop DTO。
- Wails verified Next Desktop Snapshot binding + Vue Browser Broker read-only UI 以三語系（en/zh-Hant/zh-Hans）顯示「已註冊 ACP owner」與「已驗證 token 端點存取 owner」；提示明文標明這不證明 **MCP 握手、已啟動 Chrome、已登入 Profile、ChatGPT connector 連線、零搶焦點或使用者授權**。GUI 只顯示數字，不建立按鈕、後端 API 或 runtime feature flag。
- Source-only 回歸針對 malformed/unauthorized token、重複訪問、多人平行請求、Session Release 舊 token 401、重新註冊、Read-only DTO JSON privacy canary 與數字矛盾 fail-close；不執行 Acquire / CDP / Chrome / Edge 或 ACP prompt。真實 ChatGPT/Codex/Antigravity 連線驗收、Browser Worker 共享執行、no-focus/no-modal、Chrome lifecycle orphan proof，以及 M8/Supervisor 重新接線仍是 P8/M9 gate，不會因本次 operator count 就解除。
- 本次獨立檢查：Go `internal/tool/browser`、`internal/browserpolicy`、`internal/app`、`internal/desktopapi`、`internal/browserdesktop` 全量測試 PASS；新增 `TestACPBrowserAuthenticatedTransportObservation` Race Detector PASS；四個 Go package `go vet` PASS；Vue `pnpm typecheck`、20 files / **149 tests**、production `pnpm build` PASS（僅既有 500 KiB chunk advisory）；三語系 coverage **869/869（100%）**。Global i18n literal scanner 在既有非本輪 Permission/ACP 字串仍有舊違規，未宣告 global check PASS。安全測試全在 ephemeral fixtures，不曾讓已安裝 Next 重新啟動。


## 36. 2026-10-12 P8 isolated real MCP SDK loopback acceptance (SOURCE ONLY)

基於 §35「registered ACP owner 與曾通過 Bearer token 存取 Browser MCP HTTP endpoint 的 owner」區分，新增 `internal/app/acp_browser_mcp_loopback_test.go`，直接用 test-owned `127.0.0.1:0` 的 TCP listener、真實 `net/http.Server`、`acpHostCapabilityProvider.Servers()` 提供的 Session-scoped Bearer Header，並使用官方 `github.com/modelcontextprotocol/go-sdk/mcp` `StreamableClientTransport` + `Client.Connect` 與 `ListTools` 端到端驗證。它不依賴 `httptest.ResponseRecorder`、也**不啟動 Codex/Antigravity adapter、MCP Chrome Worker、Chrome/Edge 或任何使用者 Profile**。匿名 SDK client 名稱 `codex`／`antigravity` 僅是測試識別，不是已驗證兩個真實代理。所有資料／網路只在測試程序、私有 temp workspace 與本機暫時性埠。

- 兩個獨立 Session 於相同合法 workspace 註冊後，`RuntimeBrowserDesktop` 先顯示 owners=2／authenticatedTransportOwners=0；第一個真實 HTTP MCP SDK `initialize`/`tools/list` 後為 2／1，第二個後為 2／2。實際工具清單嚴格驗證九個 Browser Broker 工具名稱。
- `CallTool(browser_snapshot, fabricated lease_id)` 經真實 MCP protocol 產生 fail-closed `lease owner mismatch`，不啟動任何 Worker。測試 Worker Registry 的 `Probe/Open` 均安置禁止執行的 instrumentation。未呼叫 `browser_acquire`、實際 CDP、JavaScript 或任何網站。
- 未知 Bearer token 返回 401，不產生 endpoint owner 觀測。逐一 `ReleaseSession` 後，舊 token 再訪被 401 拒絕，計數 2→1→0；另一個存活的 client 仍可 `tools/list`。Provider 重複註冊不更換 token，兩 Session token 永不共用。
- 專屬 `http.Client` 關閉 Proxy、阻止 redirect、限制只能存取原始 loopback endpoint，ctx 20 秒上限，關閉 SDK sessions／HTTP server／listener／Bridge。不列印 credential、URL、Session 原始內容；不可將測試端身份推論成 live ChatGPT／ACP 已接線。
- 驗證：真實 loopback MCP SDK `go test ./internal/app -run ^TestACPBrowserRealLoopbackMCPHandshake$ -count=1 -v` PASS（0.01s），同測試 `-count=10` 重複 PASS，`go test -race` PASS；`go test` 全量 `./internal/tool/browser ./internal/browserpolicy ./internal/app ./internal/desktopapi` PASS，Go Vet `./internal/app ./internal/tool/browser` PASS，gofmt/diff check PASS。新測試沒有 source production edit，因此不須重新編譯／部署 Wails GUI。
- **本次驗收的邊界**：證明既有 AgentDock Core Browser MCP Provider→實際 HTTP SDK protocol→Broker tool authorization/denial→Desktop aggregate 的可行性。尚未證明真正 Codex/Antigravity adapter 執行端自行完成 MCP initialize、真實 ChatGPT connector 接入、任何 Chrome Process 共用、零焦點干擾／無 Keychain 授權彈窗、Supervisor crash reauth 或自動恢復；後續需要真實 adapter 的**專屬隔離 Home／不複製使用者 auth** 測試。P8/M9 release gate 不因此解除。



## 37. 2026-10-12 P8 real Codex ACP isolated initialize (FAKE native backend; SOURCE ONLY)

接續 §36 真實 Go MCP SDK loopback（兩個合成名稱 Client），本輪加入 macOS opt-in `internal/app/acp_browser_codex_adapter_isolated_darwin_test.go`，驗證實際安裝的 **`codex-acp` 2.1.1 Adapter 二進位入口** 在嚴格隔離模式下能通過 ACP `initialize`；**不是** Session/new、Browser MCP 交握或真實 Codex native app-server 驗收。只在 `AGENTDOCK_TEST_REAL_CODEX_ACP_OFFLINE=1` 指定時執行，其餘默認 skip。

- 測試在每次執行時使用 `t.TempDir()` 建立 owner-private 0700 `HOME`、`~/.agentdock-next`、`codex-mcp-isolated` 與工作目錄。依照已實作的 `AGENTDOCK_DESKTOP_VARIANT=next`、`AGENTDOCK_NEXT_CODEX_MCP_REGISTRATION=source-only`、`AGENTDOCK_NEXT_CODEX_MCP_ISOLATION=source-only` 規則，透過 `EnvFromEnv` 只向實際 Adapter 傳遞明確的 test-owned `HOME`/`CODEX_HOME`/`CODEX_PATH`。`CODEX_PATH` 指向 test-private shell wrapper → test-private Node fake app-server，**不會執行本機真的 Codex native app-server** 或遠端模型。
- Fake app-server 只回應 `initialize`、`account/read`（回報 no account）、`config/read`（空設定）及 `skills/list`（空清單），未支援的 native command 回覆 method-not-found；不處理 `thread/start`、`turn/start` 或任何模型呼叫。Adapter 真實 ACP initialize 必須成功，fake initialize marker 必須存在、Browser Desktop registered/authenticated/worker/lease 均維持零；不存在私有 `.codex/auth.json`、`.codex/config.toml`、`codex-mcp-isolated/auth.json`、`codex-mcp-isolated/config.toml` 與遺留複製認證的 `acp/codex/codex-home/auth.json`。禁止呼叫真實 Chrome/Edge、Keychain/AGY 或外部網址。
- 安全退出：AgentDock ACP Process Controller 對測試擁有的程序組使用 SIGKILL，因此 fake Node 的 JS `exit` event 不能當驗收。改由 fake 寫入私有 PID marker，`rt.Close` 後只用 `kill(pid, 0)` **查詢該測試自己記錄的 PID 是否已不存在**，絕不向使用者程序發送終止訊號。Fake 也包含自身有界 watchdog。
- 實測：第一次光靠 `process.on('exit')` marker 的測試誤判；修正成 OS-owned PID 存亡檢查後初次 PASS。五次回歸偵測出 adapter 正常會有背景唯讀 `config/read`，依照空合成設定補上；**隔離 source-only 真實 Adapter initialize `-count=5` 全數 PASS**，沒有發現 test fake native 殘留程序。原始失敗是 fixture 設計問題，未歸因於產品。
- **明確後續 gate**：這只驗證 Adapter ACP 層啟動與最小的 native 只讀握手，**不代表** real Codex/Antigravity SessionNew、真實 Browser MCP 注入與工具呼叫、使用者登入、Chrome Worker 共用、Zero focus／零授權提示、跨 Core 實機故障恢復。下一步須以專屬 fake native protocol 完整模擬 Thread start/config MCP 注入，或在經確認完全隔離且允許的真實 adapter 使用情境下驗證；Antigravity 曾有 Keychain 彈窗風險，此輪不啟動 AGY CLI、只保留隔離研究待驗。沒有安裝／部署／Push／更動 Stable 或 Next Core。



## 38. 2026-10-12 P8 actual Codex ACP SessionNew to fake native MCP config (SOURCE ONLY)

承接 §37 僅做到實際 `codex-acp` 2.1.1 `initialize`，沿用原先 explicit `AGENTDOCK_TEST_REAL_CODEX_ACP_OFFLINE=1` 的 macOS test-only 私有 HOME / CODEX_HOME / `CODEX_PATH` 測試假 Codex native app-server。新增 `TestACPBrowserCodexAdapterIsolatedOfflineSessionNew` 與舊 initialize-only 路徑並列；**沒有執行真正 Codex native、帳號登入、模型、AGY、Chrome 或 Edge**。

- 真的 installed Codex ACP 在呼叫 AgentDock `acp.Session(Action=new, ProfileID=codex)` 時，與私有 Node fake native 完成 `account/read`（**合成測試帳號**，與使用者無關）、`config/read`（空）、`skills/extraRoots/set`、`thread/start`（**假 thread 與模型**）、`model/list`。Fake 只有明示 allowlist，未知 native methods 全部拒絕；不允許 prompt／turn／實際瀏覽器操作。
- `thread/start` 的 MCP config 驗證：經 AgentDock `acpHostCapabilityProvider` 送到真實 Adapter 並轉入 fake native 的正好是 `agentdock-browser` + `agentdock-computer` **兩個**名為指定端點的能力；各自為 test-owned `127.0.0.1` URL、具有符合格式的 Bearer header，且兩者權杖相同。第一輪測試誤以為應只有 browser 一個，才拒絕這份正確 config；改為嚴格兩端點白名單後真實 `SessionNew` PASS。
- Fake native **不執行** MCP handshake；它只在私有 0600 測試暫存檔提供剛剛注入的 Browser Bearer 給**另一個合成 Go MCP SDK Client**。該 SDK 經本機 test-owned HTTP Server 完成真實 MCP `initialize`、`tools/list`（九個固定 Browser tools），以 fabricated lease 執行 `browser_snapshot` 遭 `ErrLeaseOwnerMismatch` 拒絕；真實 Adapter→fake native 並非真實 Codex appserver→MCP 自行 handshake，務必分開宣告。
- 本機 BrowserDesktop 中 `Owners/AuthTransport` 依序為 `0/0` → SessionNew `1/0` → 合成 SDK 訪問 `1/1` → ReleaseSession `0/0`；同一臨時 token 被撤銷後 HTTP 401，且 `Workers=0 / Leases=0`，沒有 Browser Probe/Open 或 Chrome 啟動。嚴格 `CODEX_HOME` 檢查、fake PID after Close `ESRCH`、私有 auth/config 不存在；private 0600 bearer capture 測試後覆寫零值並刪除，整個臨時 HOME 由測試清除。
- 驗證：`AGENTDOCK_TEST_REAL_CODEX_ACP_OFFLINE=1 go test -p 2 ./internal/app -run ^TestACPBrowserCodexAdapterIsolatedOffline(Initialize|SessionNew)$ -count=5` **PASS（26.146s）**；同組 `-race -count=1` **PASS（10.346s）**。`go test -p 2 ./internal/app ./internal/tool/browser ./internal/browserpolicy ./internal/desktopapi -count=1` 全部 PASS，`go vet -p 2 ./internal/app ./internal/tool/browser` PASS，`gofmt`／`git diff --check` PASS。非 opt-in 路徑仍明確 SKIP，不會在 CI 或普通 Go suite 意外啟動 adapter。
- **Gate 不變**：`codex-acp` 實際 SessionNew 與設定注入已被驗證，真實 Codex native app-server **自主執行 MCP initialize/工具**、正式 ChatGPT/AGY 三個 Agent 共用同一 Chrome、無焦點／無 Keychain 授權彈窗、Supervisor cross-core crash recovery 仍未證明。此為 test-only source、未改 production Core／路由、未啟用 shared managed worker、未 install/Push/restart。



## 39. 2026-10-13 P8 real Codex native MCP isolation preflight and managed-preferences blocker (SOURCE ONLY)

§38 成功的是 installed `codex-acp` → **fake native** 的 SessionNew/MCP 設定交接，再由合成 Go MCP SDK 做 Browser MCP 握手；不等於 Codex native 自行握手。本輪以 macOS opt-in `AGENTDOCK_TEST_REAL_CODEX_NATIVE_MCP_OFFLINE=1` 新增 `internal/app/acp_browser_codex_native_offline_darwin_test.go`，嘗試使用真正的 **Codex native 0.160.0 app-server**（直接 JSON-RPC、無模型回合、無登入）來確認是否能自行對測試專用 Browser MCP endpoint 執行 initialize/tools/list。**尚未取得 native MCP 握手 PASS；P8 gate 不解除。**

- 嚴格 OS containment：`/private/tmp` 下 owner 0700 test-only HOME、CODEX_HOME／workspace；專屬 127.0.0.1 TCP listener；只執行固定 `sandbox-exec`、固定 native binary 與測試自己的 preflight executable。Seatbelt `(deny default)`，允許系統執行所需讀取但**明確 deny 整個 `/Users/wei` HOME 與系統 Keychains**，root-only write、精確 `localhost:<本輪隨機埠>` outbound，其他 loopback 埠與非本機 IP 一律 deny，所有 Mach 服務（包含 user `cfprefsd`／`securityd`）維持 deny。Child `Env` 明確最小化，沒有宿主 Codex token、全域 auth/config、proxy／DYLD，且 Session/Worker PID 用測試自有 process group 清理，不會碰別人的程序。
- 在啟動 native 前，先以另一份 sandboxed probe 強制驗證：私有 localhost 埠可連；非專屬 localhost 埠拒絕；外部 TEST-NET / IP connect 在 kernel 階段 EPERM；使用者 HOME 目錄開啟 EPERM；sandbox root 外的 sentinel 讀／寫 EPERM。任何失敗 fail closed，**絕不啟動 native**。先前首次只有 `/System`、`/usr` 等 read allowlist 太窄，`sandbox-exec /usr/bin/true` 即以 -6 拒絕，並未啟動 Codex；調整為 system file-read allowed + user HOME/Keychains deny，預檢才通過。
- 真實 native 0.160.0 app-server 在沙箱內的 JSON-RPC `initialize` **PASS**。之後 `thread/start`（只含 cwd、空 modelProvider 及 Broker MCP HTTP URL/Authorization）回傳 `-32600`。經測試環境限定、原始訊息不外洩的診斷，確切阻礙是 **`Failed to synchronize managed preferences`**，發生在設定載入階段，**不是 Browser MCP token 的拒絕／握手錯誤**。嘗試僅 allow 系統 `com.apple.cfprefsd.daemon` 仍未解決，已撤回這項允許。沒有進一步開放使用者 `cfprefsd.agent`、Keychain 或整個 Mach IPC，也沒有試圖使用真實登入憑證。
- 測試保留 opt-in gate；**只有**在完全符合此已知 managed-preferences 阻礙時以 `t.Skip` 標記「native MCP unqualified」，其餘錯誤仍嚴格 FAIL。這個 SKIP **不代表 MCP 通過**：沒有收到 native 發出的 MCP initialize、tools/list，Browser authenticated owner 仍為 0，沒有 Worker／Lease，也沒有實際 Browser 或模型。普通 `go test` 不帶 opt-in 時不啟動 native。與先前 §36 合成 SDK MCP 已成功、§38 real Adapter→fake native config handoff 已成功的證據分別呈現，不得合併冒稱完成整條真實 native 協定路徑。
- 後續必須找到不允許使用者偏好／Keychain 仍能讓 Codex native 0.160.0 完成 managed-preferences 載入的安全手段（或重新評估分離的 VM/容器測試方案），才可再進行 real native MCP handshake、3 個 live Agent Browser Pool、Chrome OS process/consent/no-focus crash UAT。`NexusDock` live pairing 仍依使用者要求 DEFER。此輪程式只是 test-only，無正式 Core、Browser routing 或 frontend runtime 修改，stable 與 Next 都未部署／重啟。



## 40. 2026-10-13 P8 genuine MCP HTTP to real managed headless Chrome (SOURCE-ONLY)

**Product objective:** An agent uses the Browser Broker MCP tools rather than launching its own browser, and additional sessions share one managed Headless Chrome without foreground focus, cross-owner page access or leaks. This closes the missing **HTTP MCP protocol → actual Browser engine** vertical in the already-established source-only `TestACPBridgeSharedRealHeadless` fixture. It is intentionally not another fake/native macOS sandbox or VM.

- Existing real `chrome-devtools-mcp@1.7.0` offline npx + isolated Headless Chrome under the canonical test workspace, with bounded shared pool budget 1 and a keeper page. Added two independent *test-owned Go MCP SDK* HTTP clients to the same real ACPBridge, using scoped Bearer tokens and a test-owned ephemeral `127.0.0.1` listener, no HTTP proxy or redirects. Genuine `initialize`, `tools/list` and **tools/call `browser_acquire`, `browser_evaluate`, `browser_snapshot`, `browser_release`** now traverse the production Broker MCPHTTPHandler into the real Chrome backend. They are not installed ACP agents.
- The clients acquire `about:blank` background pages, write/read independent in-page variables, ensure other owner cannot use their lease (`ErrLeaseOwnerMismatch`), release owned pages without disrupting the sibling, and verify token revocation yields HTTP 401. The same managed connector PID / **one real Chrome main** remains in use; no new worker starts on additional MCP clients; active pool budget 1. No navigation to external sites, user Chrome/Edge/profile, direct desktop focus, model prompts or Keychain operations. `ForegroundForbidden` and keeper `bringToFront:false` enforce source routing but **do not alone constitute UI-observed zero-focus/popup UAT**.
- During initial test, three direct test leases + two HTTP leases exceeded existing **MaxConcurrency=4** and HTTP owner 2 failed admission. Kept that real production limit unchanged; moved direct lease release before both HTTP acquisitions, retaining the same idle managed Chrome generation. This is a test-order fix, not a Broker capacity defect.
- First full HTTP run reached the successful tool-and-cleanup assertions but `realSharedProcesses` used a **2s** `ps -axo` timeout; a crowded machine with ~2,030 processes caused the observer to be SIGKILLed, leaving cleanup *observation* unqualified. Increased only the bounded test observer timeout to 6s; never interpret observer failure as process exit, never kill unknown user processes. Subsequent opted-in **real Headless Chrome test PASS (68.55s)**; observed test-owned MCP/Chrome processes gone, test TMPDIR marker absent, shared budget returned to 0, one Chrome main per generation; source-only marker. Existing stable session-owned MCP processes were not cleaned or touched.
- On the running Mac, `ps` identified many long-lived `npm exec chrome-devtools-mcp@1.7.0 --isolated --headless` wrappers under **PID 55205** (Codex native descendant of stable AgentDock Core PID 44832). This is a concrete example of the user-reported redundant browser/MCP process risk and supports Next shared Broker value; it does **not** prove those children are idle/orphans. They belong to stable/user sessions, so we do **not** terminate, adopt or migrate them. Ownership-safe product UAT remains necessary.
- **Still open:** real ChatGPT control path and *actual installed* Codex/Antigravity ACP actors executing their own MCP calls through running Next, zero user focus/modals observed, precise company Edge/explicit Chrome consent, and live crash/restart owned-resource recovery. Earlier native Codex Seatbelt managed-preferences SKIP is an unrelated fixture finding, not a Next milestone. No installed Next/stable deployment or restart.



## 41. 2026-10-13 P8 Next live Browser-disabled blocker and source startup default (PRODUCT)

**User workflow:** A real ChatGPT/ACP task routed through installed Next cannot use Broker at all if the Core is launched with Browser disabled. A read-only, authenticated `GET http://127.0.0.1:8767/internal/runtime/browser/desktop` on the original Next Core process returned HTTP 200 but `availability=browser_disabled`, with owners/workers/leases all zero. The current Next launch environment declares Next role + ACP enabled + non-stdio and **does not define `AGENTDOCK_BROWSER_ENABLED`**; its source `config.FromEnv` previously treated that omission as `false`. This is a normal startup-configuration blocker, not a missing MCP/native sandbox protocol proof.

**Focused product change:** For `AGENTDOCK_DESKTOP_VARIANT=next` + `AGENTDOCK_ACP_ENABLED=true` + non-stdio Core, source `FromEnv` now defaults `AGENTDOCK_BROWSER_ENABLED` to `true` only when the setting is absent/empty. Explicit `false` retains user opt-out, invalid values still error; stable/non-Next, ACP-disabled, and stdio configurations remain disabled by default. The Browser Broker remains **lazy**: creation of Core/ACPBridge does not launch Chrome or attach to Edge. M8 host operation admission, page/owner scope, existing 4-concurrent-lease policy and 1-workspace **experimental shared opt-in** are unchanged. Default Browser-capability availability is not evidence that Chrome is running, an external Edge profile is authenticated, or actual Codex/AGY UAT succeeded.

**Execution boundary:** Source-only change. Installed Next Core remains as originally running and reported disabled until a controlled **Next-only** update and reload; it was not restarted, installed, pushed, or silently enabled live. Stable AgentDock and its sessions are out of scope. Next task after safe update: check signed/live Browser Snapshot says `available` with zero workers, then use one permitted real ACP session to do an owned, background lease/acquire/evaluate/release. Do not confuse readiness with activated experimental shared Chrome or three-actual-Agent validation.



## 42. 2026-10-13 P8 installed Next real Codex tool attempt and historical Browser snapshot correction

**Real product evidence (no synthetic native replacement):** The installed Next Core on `127.0.0.1:8767` is a signed Next-only bundle whose `go version -m` vcs revision equals `55916091` (`fix(browser): resolve npx and node for Next launchd workers`). The stable AgentDock listener remains PID `44832`, unmodified; the deployed Next Core was subsequently observed at PID `7455`. Authentication-scoped read-only Broker/ACP status returned Browser enabled/available, no running or loaded Codex/Antigravity ACP sessions before the bounded trial. No user Edge/Chrome profile was attached.

An actual installed Next **Codex** ACP `session/new` and model `turn/start` were attempted in a new ephemeral `/private/tmp` workspace, with explicit instructions to use only the injected `agentdock-browser` on `about:blank`, no external pages or other tools. The model-side event stream showed **`browser_acquire`** attempted and run terminal **completed**, but **no `browser_evaluate`, `browser_snapshot`, or `browser_release` tool-call events**. The ACP session itself was explicitly closed successfully, owner count returned to zero; this **does not pass the end-to-end real-Agent Browser UAT**. Antigravity was not launched (previous Keychain modal risk).

Read-only Broker diagnostics after the turn showed a new Worker in `failed` state (no active PID) with error category `signal: killed` / `BROWSER_ENGINE_UNAVAILABLE`; this is **not the previous `npx not found`** diagnostic. Root cause remains open; do not assume real Chrome launched in the Next Core. Two bounded fixed-package `npx --yes chrome-devtools-mcp@1.7.0 --version` probes under launchd-like minimal PATH (one npm offline, one normal) both reported **1.7.0**. A targeted real `NewWorkerRegistry(WorkerDependencies{}).StartManaged` launched a real managed Headless Chrome Worker under similarly minimal PATH **in 10 seconds**, obtained a PID and stopped its own Worker successfully. This confirms basic executable discovery and owned process path work in a separate CLI, **not** that the Next Core/model HTTP request lifecycle is qualified. Do not increase process/timeouts or add VM merely to force an observed PASS.

**Concrete operator UX bug:** The live Broker intentionally retains audit diagnostics for lease `CleanupState=complete` and Worker `stopped` after a Session closes. `RuntimeBrowserDesktop.aggregateBrowserDiagnostics` had treated a **completed historical lease with no longer-registered owner** as an active `unownedLeases` and `stale`, and counted stopped Workers as active count. It caused persistent misleading “stale” even if that was the only resource. Source-only fix projects **only non-complete leases and non-stopped Workers** in *passive current-state counts*, preserving all raw audit records, fail-closed unknown-route checks and genuine `CleanupFailed`/`WorkerFailed`/orphan/uncertain alerts. A real failed Worker from this UAT still makes the Snapshot `stale`; this fix does **not** pretend the underlying failure disappeared.

**Remaining acceptance:** Identify why the installed Next Core Browser acquire's actual worker is killed despite standalone version/Worker startup passing. Prefer bounded real-Session error-stage observation, Core request lifetime and native ACP HTTP timeout correlation, rather than further fake protocol fixtures. Re-run a single real Codex owned `about:blank` acquire/evaluate/release only after an evidence-backed fix, then separately complete ChatGPT and AGY Actor adoption without focus/modals. Historical-count source change is not deployed; installed Next state may still show retained completion as stale until a controlled Next-only update.

## 43. 2026-10-13 P8 real default Worker post-start diagnostic and host process pressure (NO CLOSEOUT)

**Normal user workflow:** Installed Next Codex ACP previously reached `browser_acquire` but did not complete `evaluate/snapshot/release` (§42). To distinguish startup-only success from usable Browser service, add a **single opt-in real** test `TestManagedWorkerRegistryRealPostStart` (`AGENTDOCK_TEST_REAL_WORKER_POST_START=1`) with the existing default `WorkerRegistry` stdio opener, cached pinned `chrome-devtools-mcp@1.7.0`, offline npm, isolated `about:blank`, explicit caller cancellation, post-start `list_pages/new_page/evaluate_script`, and owned Stop/observation. No new daemon, VM, synthetic account, login/Profile attachment, or production runtime change.

**Actual native evidence, not a pass:** Codex ACP's workspace-write sandbox could not run `/bin/ps` (`operation not permitted`) and its own run was rejected rather than granting broader permissions. The stable Mac-Dev host ran the real opt-in test. First attempt reached `WorkerReady` then `list_pages` failed MCP argument schema because the *test* sent `nil` instead of `{}`; its owned cleanup reported success. Test corrected to send the protocol-required object. Second host attempt again reached `WorkerReady`, but first post-start `list_pages` timed out; cleanup observer reported `signal: killed`, so **complete owned-process cleanup was not independently proven on that run**. These results do not prove the Next Core cancellation mechanism or SDK cause, and are **not** successful end-to-end ACP UAT. Ordinary non-opt-in focused worker tests and `go vet ./internal/tool/browser` passed.

**Observed host condition:** Read-only macOS process inventory identified Codex native `app-server` PID 55205 with **781 direct children**, roughly **170** of them npm commands, including many long-lived `chrome-devtools-mcp@1.7.0 --isolated --headless`; `uptime` reported load averages about **33.10 / 32.47 / 35.32** and `memory_pressure -Q` showed 21% free. This is concrete host-resource/child-lifecycle pressure and may explain unstable startup/tools, but **PID ancestry/counts do not identify which ACP Session owns which child, do not prove an orphan, and confer no cleanup authority**. No existing child/AgentDock/Chrome/Edge was terminated or adopted. Stable 8765 PID44832 and Next 8767 PID7455 remained listening.

**Next closest product work (corrected scope):** Do **not** turn official Codex native Thread→MCP PID attribution or a Rust fork into a Next prerequisite. The accepted B0–B6 lifecycle contract only allows Next to manage resources that its ACP/host capability paths can actually prove and keeps opaque native Codex resources `unknown_or_deferred`. The immediate product work is therefore to stop **new Next ACP sessions** from being persistent by default, preserve explicit persistent sessions when requested, and then re-run **one** installed Next Codex `about:blank` acquire/evaluate/snapshot/release on a normally loaded host. `signal: killed` remains root-cause **unknown**; P8 real multi-agent/focus acceptance and M9 remain open. This checkpoint adds only an opt-in native regression and honest evidence; no Next source reinstall/push or stable mutation.

## 44. 2026-10-13 Next ACP default idle-managed lifecycle (PRODUCT)

**User workflow / observed pain:** A normal delegated ACP task currently creates a `persistent` Session whenever the caller omits `lifecycle_policy`. Persistent Ready sessions intentionally survive the task, so repeated short-lived Codex work can retain native app-server/MCP state much longer than the user workflow requires. The §43 host inventory (hundreds of Codex app-server children) is not proof that each child is orphaned, but it is concrete evidence that an unbounded default is the wrong product behavior for Next.

**Smallest product change:** The shared ACP runtime semantics remain unchanged; `NormalizeSessionLifecycleOptions({})` still means `persistent`. Instead, `tool/acp.Service` now accepts a runtime-scoped default lifecycle. Stable AgentDock and unset/stable callers continue to construct the service with `persistent`; **AgentDock Next desktop, non-stdio only**, supplies `idle-managed`, whose existing default TTL is 30 minutes. If `acp_session new` explicitly supplies `persistent`, `ephemeral`, `idle-managed`, or a valid idle TTL, that explicit request always wins. Next stdio also retains persistent semantics. The tool contract now tells orchestrators to use `ephemeral` for bounded one-run delegated workers and `idle-managed` when short-term follow-up context is useful.

This deliberately reuses the existing lifecycle engine: idle-managed cleanup only considers Ready/loaded eligible sessions, respects active runs/interactions/session operations, performs normal ACP `session/close`, and releases registered Browser/Computer host capabilities through the existing provider path. It adds **no PID scan, process-name kill, Rust/native Codex ownership inference, daemon, or new cleanup authority**. Existing sessions are not migrated or closed by this change.

**Validation boundary:** Focused new default/override tests PASS; `internal/acp` and `internal/tool/acp` package regressions PASS. A broader `internal/app` ACP-pattern run timed out in the pre-existing output-contract prompt test while the host was under extreme unrelated load; the new runtime lifecycle selector test itself PASSed. During later load spikes the main CPU consumers observed were newly spawned ChatGPT desktop Renderer/Service plus `tar`, so no Browser/ACP process was killed to manufacture a clean environment. Installed Next PID 7455 / 8767 and stable PID 44832 / 8765 were not restarted or mutated. Deployment and real installed Codex Browser UAT remain pending.

## 45. 2026-10-13 installed Next real Codex Browser acceptance PASS (PRODUCT)

**Runtime cutover verified:** The installed `AgentDock Next.app` helper is vcs `9356bac7`; the live Next Core had already rotated from the older §42 PID/revision to PID `85517` while stable Core PID `44832` on 8765 stayed unchanged. No duplicate kickstart was issued after detecting that rotation. A live Next `acp_session new` with no lifecycle override returned `idle-managed` with `idle_close_after_ms=1800000`, proving the §44 default is active in the running product; the test Session was then closed normally.

**Real actor/browser flow:** Through the actual `macbook-air-m3` Next connector, ChatGPT created a real Codex ACP Session and started a model turn instructed to use only the injected `agentdock-browser` MCP on `about:blank`. The native event stream showed the exact sequence `browser_acquire → browser_evaluate → browser_snapshot → browser_release`. Evaluate set and returned `agentdock-p8-ok`; snapshot observed the same text; release reported `cleanup_state=complete`; the Run ended `completed/end_turn`. No shell/web fallback, external Edge, user Chrome profile, filesystem mutation, or other browser path was used. No pending ACP interaction or Keychain prompt was emitted by this flow, and no foreground browser action was requested.

**Owned cleanup evidence:** After explicit ACP Session close, Next diagnostics reported ready=0, running=0, loaded=0, auto_close_failures=0 and Broker correlation `sessions=null`. A read-only descendant walk rooted at Next Core PID 85517 found only the owned Codex adapter/app-server chain and **zero `chrome-devtools-mcp` descendants and zero Chrome/Chromium descendants**. This is real installed-product cleanup evidence for this bounded managed-Headless flow, not authority to clean the unrelated long-lived stable Codex tree.

**Interpretation:** The earlier §42/§43 `signal: killed` failure is no longer reproducible after the running Next Core moved onto current source and the host load subsided enough for the bounded workflow. The exact historical kill cause remains unproven, so no speculative timeout/process-kill change is introduced. **Codex actor acceptance for managed Headless Chrome is now PASS.** P8 as a whole remains open for the remaining real Actor/adoption surfaces (notably Antigravity without Keychain/modal regressions, ChatGPT/direct control-path semantics where applicable, and explicit company Edge / requested Chrome profile authority).

## 46. 2026-10-13 installed Next real Antigravity .14 Browser acceptance PASS with caveat (PRODUCT)

**Deployment/isolation boundary:** AgentDock Next was transactionally updated to Core revision `afab0339` on 8767 (live PID `44263`) while stable Core 8765 PID `44832` remained unchanged. The clean Antigravity ACP candidate `1.2.0-agentdock.14` from worktree HEAD `87cff1e` was built outside the ACP repo and staged only at `~/.agentdock-next/bin/antigravity-acp`; stable/global `/Users/wei/.local/bin/antigravity-acp` remains `.8`. Live `session/new` reported Agent version `.14`, `idle-managed` 30-minute lifecycle, and created the Next-owned `~/.agentdock-next/acp/antigravity` state root at mode 0700. The AGY child used a private `/private/.../agy-acp-browser-*` HOME with injected Broker config. Existing global Antigravity conversation storage remains shared by the adapter's current design and is not claimed as isolated by this checkpoint.

**No-modal evidence:** Before/during/after the bounded UAT, no `loginKC`, `queryCreate`, or `SecurityAgent` prompt event was observed; the only persistent UI helper was the pre-existing `CoreServicesUIAgent`. The real AGY turn authenticated and proceeded without a new OAuth login flow.

**Real actor/browser flow:** The actual Antigravity model used only the injected `agentdock-browser` on `about:blank`. `browser_acquire` returned an `isolated-temporary` lease with `foreground_policy=forbidden`. The first `browser_evaluate` omitted `lease_id` and the Broker correctly rejected it with `ACTION_INVALID: lease_id required`; Antigravity then retried using the acquired lease and evaluation returned `agentdock-agy14-p8-ok`. `browser_snapshot` confirmed that exact text on `about:blank`; `browser_release` returned `cleanup_state=complete` / `exclusive managed worker stopped`; the Run ended `completed/end_turn`. There was no shell, external browser, Edge/user Chrome profile, Computer Use, or alternate browser fallback.

**Owned cleanup:** After explicit Session close, Antigravity diagnostics returned ready/running/loaded all zero, Broker correlation `sessions=null`, and adapter descendant count zero. A read-only descendant walk rooted at the Next Core found **0 chrome-devtools-mcp, 0 Headless Chrome, 0 AGY child and 0 browser proxy**; only the long-lived Antigravity ACP adapter root remained.

**Interpretation:** **Antigravity .14 managed-Headless Browser actor acceptance is PASS with a compatibility caveat.** The omitted first `lease_id` is a real adapter/model tool-argument issue and must not be erased from the record; Broker fail-closed behavior worked and this run self-recovered, so it is not a rollback blocker for the Next-only `.14` candidate. Any proxy-side lease propagation improvement belongs to the Antigravity ACP workstream, not an AgentDock Core special case. P8 still remains open for ChatGPT direct Browser-tool adoption semantics and the explicit company Edge / requested Chrome profile authority/no-focus gate; native PreToolUse M8, `AGY_ACP_AUTO_APPROVE=0`, stream direct and broader AGY feature parity are also not closed by this Browser PASS.

## 47. 2026-10-13 ChatGPT direct Next Browser tool exposure / Broker adoption diagnostic (BLOCKED)

**Product goal:** The current ChatGPT conversation should be able to use the **same Next Core-owned Browser Broker** already proven by Codex and Antigravity without delegating a model, starting a second unrelated Chrome, focusing a user window, or keeping unowned resources.

**Live ChatGPT connector observation:** The available connected `Macbook_Air_M3` tool declarations include ACP/session/command/dynamic MCP tools, but **do not contain** `browser_broker`, `browser_session`, `browser_act`, or `browser_snapshot`. Next's `agentdock_context` reports only the standalone `memory` dynamic MCP; `mcp_tool_search("browser")` returns zero. This is the *current conversation's* declared tool surface; it does not prove what a freshly refreshed ChatGPT connector would see.

**Next runtime/source verification:** Installed Next Core remains healthy at `127.0.0.1:8767`, PID `44263`, helper commit `afab0339`; stable Core remains PID `44832` on 8765. The source tool registry (`internal/app/specs_browser.go`, `specs_registry.go`, `internal/mcp/server.go`) defines these four tools and registers available definitions into SDK `tools/list`. `browser_broker` depends on ACP+Browser enabled/non-stdio; other three depend on Browser enabled. Next `/.well-known/mcp.json` advertises OAuth2; unauthenticated POST `/mcp` `tools/list` correctly returns **401**, so a bare HTTP probe cannot verify the authenticated live tool list. No OAuth material, token, user profile or credentials were read or reused.

**Critical architecture distinction:** Existing `browser_broker` is an ACPBridge status/cleanup control plane; `browser_session`/`browser_act`/`browser_snapshot` invoke the separate legacy `toolbrowser.Service` and **do not acquire or call shared ACPBridge managed leases**. Simply surfacing these tool names in ChatGPT or testing legacy `browser_session` would **not** close the shared ChatGPT/Codex/AGY Broker requirement. `mcp_tool_call` searches/invokes only dynamic MCP servers and is not a legitimate substitute for invoking built-ins; neither a local shell/CURL browser call nor delegated ACP usage is ChatGPT direct MCP acceptance.

**Bounded acceptance path:** (1) Using a supported ChatGPT connection/tool-catalog refresh, verify with an authenticated, current connector that the needed *new or correctly bound* direct Broker tool is callable. (2) Add or enable a first-party ChatGPT-facing Broker lease owner that is bound to an authenticated trusted MCP principal and canonical workspace (never model-provided scope, reusable ACP token, or synthetic ACP session), using the **existing** worker/lease/admission/cleanup coordinator rather than a parallel Browser Service. (3) Run a real ChatGPT-origin `about:blank` acquire/evaluate/snapshot/release, verify no focus/modal and only owned resource cleanup, then close this actor gate. Require design/authorization review before enabling mutation; do not accidentally bypass M8 or issue `Authenticated=true` for Edge.

**Conclusion:** **ChatGPT direct shared-Broker actor UAT = BLOCKED**, not failed browser execution and not PASS. No new Chrome, Edge operation, ACP session, daemon, connector installation/restart, source modification, or stable/Next restart was done in this diagnostic. This blocker is independent of Codex/AGY Browser PASS; advancing unrelated P8/P9 work is permitted.

## 48. 2026-10-13 Next Core first-party direct shared Browser Broker lease（source-only）

- **目標**：讓 ChatGPT 的 Next MCP 工具使用已由 Codex／Antigravity 驗收的同一組 Core-owned ACPBridge managed coordinator / WorkerRegistry / RoutePlanner，而不是舊的獨立 browser.Service；避免替 ChatGPT 偽造 ACP Session/token、重複啟動 Chrome 或觸碰 user Edge。
- **實作入口**：既有 built-in `browser_broker` 補上嚴格參數契約的 `direct_acquire`、`direct_evaluate`、`direct_snapshot`、`direct_release`。這四個操作會經過 `Runtime.Call` 的 M8 admission；evaluate 按不透明 JavaScript 效果處理，snapshot 按唯讀處理。原有 status／cleanup 功能保留。舊 `browser_session`／`browser_act`／`browser_snapshot` 仍保留 legacy service，**不可**誤標記成共享 Browser Broker。
- **可信歸屬**：只從 HTTP MCP 已驗證的 stable `requestmeta.AuthPrincipal` 與 Next Core `AgentDockDefaultDir` canonical workspace 建立 `ScopeUpstream`，使用 opaque stable owner ID；模型無法提供 principal、ACP session、workspace 或 pageId。經相同 RoutePlanner 檢查，**只接受 managed route**，company／external 無法 fallback 到 general Chrome；共享 pool 僅在既有明確啟用且 workspace 相符時重用。owned lease 仍由原 Coordinator 與 Worker lifecycle 完整管理；跨 principal／跨 actor 操作／release 依 scope fail-closed。
- **範圍**：這是 Next/macOS 專用 source checkpoint，沒有新增 Provider／Chrome launcher／daemon／Edge qualification 或變更 AGY；沒有使用的瀏覽器仍由原先 background-only/owned cleanup 設計約束。取消後 release 與 Core Close 的 direct lease 清理沿用 coordinator；release 錯誤不可被回報成成功。
- **驗證**：完整 browserpolicy/browser/app/config package PASS；新 direct 主體與 M8/Schema focused -race ×2 PASS；Go vet PASS；Linux/Windows amd64 Browser/App compile-only PASS，git diff check PASS。僅 synthetic in-process fake worker，不曾開啟真實 Chrome／Edge。
- **未滿足的實機 gate**：目前 ChatGPT 對話的 `Macbook_Air_M3` connector 尚未宣告 `browser_broker`；已安裝 Next Core 仍為 `afab0339`，沒有部署本分支，所以**真實 ChatGPT → Next Browser Broker acquire/evaluate/snapshot/release 驗收仍是 BLOCKED，不可宣告 PASS**。若用 shell／直接 localhost HTTP／Codex／AGY 來測，只能算其他 evidence level。完成路徑是受支援的 ChatGPT connector 工具清單更新與 Next-only 安全部署，再由 ChatGPT 本人完成一次背景 owned-browser workflow、無焦點/對話框與資源清理驗證。
