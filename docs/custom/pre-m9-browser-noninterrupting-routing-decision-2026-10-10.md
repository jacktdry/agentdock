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
