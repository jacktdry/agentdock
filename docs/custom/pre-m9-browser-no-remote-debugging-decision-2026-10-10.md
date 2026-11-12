> **SUPERSEDED — 2026-10-10 約 01:10（Asia/Taipei）使用者更正。** 本文件全面禁止 Remote Debugging 的結論屬需求誤解，已失效。
> 現行權威決策為 [Non-interrupting Browser Routing](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)：允許已授權且不干擾的 CDP；以下歷史文字原樣保留，不代表有效規格。

# AgentDock Next P8 — Browser 需求／安全架構評估與禁止 Remote Debugging 決策

日期：2026-10-10（Asia/Taipei） · 性質：**需求與架構決策／尚未實作及部署** · 基礎：`70d23cca`。

> **本文件優先於此前 P8 中「公司 Edge = 透過 CDP/Remote Debugging attach」的設計。** 舊 B2i～B2n-B 保留為歷史研究與測試記錄，不再代表 Next 使用者 Profile 的預定正式路徑。**目前安裝與原始碼仍有 CDP 路徑，尚未符合新規格。** 禁止將本決策誤稱為已經完成禁止偵錯的程式控制或正式 UAT。

## 1. 產品需求與不可協商限制

| 使用情境 | 必要路由 | 找不到目標時 |
| --- | --- | --- |
| 公司 CMS、公司專案（未覆寫） | 使用者明確配對、已有登入狀態的 **Microsoft Edge Profile** | 顯示指定 Edge/Profile 不可用；不切到別的 Profile 或 Chrome |
| 使用者明確要求「我的 Chrome Profile」 | 使用者指定並已配對的**既有 Chrome Profile 與瀏覽器** | 多個 Profile 要求選擇；沒有配對或無權限即拒絕，不換成 headless／其他 Profile |
| 一般專案、未指定個人 Profile | AgentDock 擁有的獨立隔離 Chrome（偏好 headless） | 非偵錯通道尚未可用即拒絕；不得暗中改以 CDP 啟動 |
| 多代理同時工作 | 經 Broker 授權的分頁／操作租約 | 分頁鎖或 Profile 不明確時排隊或拒絕，不能越界 |

使用者會把 **Edge 和 Chrome 的 Remote Debugging 關掉**；Next 必須在此狀態下運作。**Next 不得透過任何形式的 Remote Debugging 操作 Edge 或 Chrome，包含自己管理的 Chrome**：

- 不啟用／要求啟用瀏覽器內的 Remote Debugging，也不改使用者的瀏覽器設定、重啟用戶瀏覽器、切換 Profile 或複製登入資料。
- 不使用 `--remote-debugging-port`、`--remote-debugging-pipe`、`DevToolsActivePort`、`/json/version`／`/json/list` 的 DevTools discovery、CDP WebSocket／browser URL、`--wsEndpoint`、`--browser-url`、`chrome-devtools-mcp --autoConnect`。不以 Relay、另一個埠、Chrome for Testing、Playwright/CDP 或 WebDriver 的間接 CDP 包裝來繞過禁令；底層是否使用偵錯協定必須可驗證。
- 不使用 Chrome／Edge 擴充功能的 `chrome.debugger` 權限或 API：它仍是 DevTools Remote Debugging Protocol 的替代傳輸。禁止「雖然沒有旗標，但仍 attach debugger」的替代解法。
- 不得靜默 fallback 到另一個 browser、profile、OS Accessibility 或 remote debugger；必須回報具體失敗原因，並保留使用者既有瀏覽器與分頁。
- 預設不搶焦點；只在 AgentDock 自己建立的背景分頁工作，或在使用者針對既有分頁**明確授權**後工作；任務完成關閉自己的分頁，不得誤關使用者原有分頁、其他 Agent 的分頁。使用者曾反映 CMS 測試遺留 Edge 分頁，此為 P0 操作衛生需求。

## 2. 實際風險模型：按需求決定防護深度

| 等級 | 必須防護 | 建議控制 | 不過度擴張 |
| --- | --- | --- | --- |
| S1 操作／生命週期 | 操作錯誤分頁、跨 Agent 干擾、搶焦點、誤關或累積分頁 | task/agent tab ownership、分頁鎖、回收、cancel/timeout、background-first | 不為了每個理論 race 建立額外全機系統 |
| S2 Profile／站點／權限 | 錯誤 Chrome/Edge Profile、未配對站點、過期操作許可、意外讀取登入資料 | 每個 Profile 單獨配對、擴充功能 origin/site 權限、使用者批准、租約與撤銷、敏感操作 ask | 不提取／複製 Cookies、Token、密碼驗證已登入 |
| S3 本機惡意對手 | 同使用者 OS 帳號遭惡意程式控制、程序注入、Socket 劫持／CDP Frame 偽造 | **V1 不主張抵禦整台本機被控制**；將其留作獨立企業高安全需求 | 不再為了 S3 無限擴充 B2n 的 Socket／CDP 原子證明 |

信任邊界：Next Core、受控 Native Host、經使用者安裝與配對的 Extension 是可信元件；網頁 DOM/Content Script 輸入不可信，資料須驗證；外部 Agent 的命令需要 Broker 授權。擴充功能安裝與網域權限仍可能需要使用者的一次性同意，這不同於每次以 DevTools attach 所跳出的 Remote Debugging 同意視窗。

## 3. 方案比較與技術判定

| 方案 | 真實 Chrome/Edge Profile | 不使用 Remote Debugging | Background／多代理 | 主要限制 | 判定 |
| --- | --- | --- | --- | --- | --- |
| **A. Chromium Manifest V3 Extension + Native Messaging** | 各 Profile 安裝配對後可望支援；Chrome/Edge 分別註冊 | 是，僅使用 `tabs`／`scripting`／content scripts，**禁止 debugger** | 原則上可將 task 對應到獨立 tab；要 PoC | Host permissions、跨網域 iframe、限制頁、檔案／系統對話框、非 trusted DOM 事件；需先驗證 | **優先進行有門檻的 PoC** |
| B. macOS Accessibility／AppleScript | 可操作使用者既有瀏覽器 | 是，若不呼叫 DevTools | 容易改變前景、鍵鼠焦點；DOM 與背景可靠性有限 | 需 OS 授權、UI 脆弱、焦點干擾 | **僅另行明確批准的 fallback**，不自動啟用 |
| C. Managed isolated headless Chrome + Extension Native Messaging | 非使用者 Profile；用於一般專案 | 理論上可做到，但完整 Native Messaging／headless 支援須實測 | 潛力可行 | Headless extension 啟動、Native Host 綁定、截圖／互動有待驗證 | **納入同一 PoC**，不預設已可用 |
| D. `chrome-devtools-mcp`、CDP Relay、`--autoConnect`／`--wsEndpoint`、`chrome.debugger` | 舊方案可 attach，但依賴偵錯 | **否** | 可做到若干動作，但違反硬性要求 | 每次偵錯允許、登入 Profile 暴露、用戶明確禁止 | **正式路徑禁止** |
| E. 全自研 CDP Dispatcher／更強 CDP Fence | 仍需偵錯協定 | **否** | 增加維護成本 | 無法解決本次「禁用 Remote Debugging」需求 | **不開展 B2n-C 作為正式方案** |

官方資料：Chrome DevTools MCP existing session 的 `--autoConnect` 需要先啟用 Remote Debugging 並允許連線，`--browser-url`／`--wsEndpoint` 也依賴可偵錯的 Chrome；Chrome 136+ 對預設 profile 的 debug switches 另有限制。Chrome/Edge 支援 Extension Native Messaging（擴充功能與 Native Host stdio）、`tabs`、有 host 權限的 `scripting`；`chrome.debugger` 官方定義為遠端偵錯協定替代傳輸。詳見 §8 來源。

## 4. 建議架構（仍待 PoC）

```text
Agent task / Workspace route / Explicit profile preference
    ↓  Broker validates requested Browser + paired Profile + origin + task permit
Next Core (Next-owned state/IPC; no DevTools/CDP routing)
    ↕  private local IPC; no public bearer endpoint or unrestricted commands
Native Messaging Host (allowed extension IDs; stdio)
    ↕  browser runtime.connectNative (not chrome.debugger)
MV3 Extension in EXACT selected Chrome/Edge Profile
    ├─ tabs.create({ active: false }) / query / update / remove (owned tabs only)
    └─ chrome.scripting / site-scoped content script DOM actions
           ↓ authorized origin + tab lease; per-Agent isolation
        User's existing logged-in browser profile, never cloned or re-launched
```

- 配對：在**每個實際 Profile** 安裝擴充功能並明確 Pair；以該 Profile 的 `chrome.storage.local` 保存配對識別（不可改用跨 Profile 的 `chrome.storage.sync`），Core 綁定 BrowserKind／Profile label／extension channel／所允許站點；需 PoC 證明無 Profile 交叉或代入。`allowed_origins` 限制 Native Host 只能接受受信任擴充功能 ID，Native Host/Core 還需校驗來源與操作許可。不要以相同 extension ID 推論是同一 Profile。
- 僅對已授權的 site/origin 執行讀取或動作；擴充功能 `optional_host_permissions` 或同級明確授權比一開始取得 `<all_urls>` 更合適。CMS 優先只授權實際使用的公司站點。
- Agent 透過 tab owner/agent/workspace ID 與 expiry lease 隔離；不同代理不得在同一 tab 並行導覽、輸入或關閉。停止／異常時只刪除該任務**新建立且仍確定歸屬**的分頁；預先存在、使用者手動開啟、其他 Agent 的分頁不可刪除。
- 使用者既有 Profile 不是 headless：`active:false` 是背景分頁，不等於隱形瀏覽器；Chrome 視窗狀態、網頁／作業系統限制仍可能造成焦點影響。若功能必須改變焦點，明確告知並先取得同意，無法達成 no-focus 時 fail closed。
- 必須做能力盤點：DOM click 可能不是可信實體輸入；跨來源 iframe、`chrome://`、擴充功能商店等限制頁、原生 file picker、WebAuthn/Passkey、背景 tab screenshot、不透明 canvas/video 都不能預設完整支援。必要時提供明確的人工接手或明確 opt-in Native Accessibility，不暗中升權。
- Managed isolated 模式若改走 Extension 需先確認 `--headless=new`、extension bootstrap/Native Messaging 與流程回收的真實相容性；不能讓底層工具偷偷建立 debugging-port/pipe。保留隔離與自動銷毀資料目錄，但不能複製使用者私有 Profile。

## 5. 現況與必要遷移（目前尚未達標）

**現在的 source 與已安裝 Next 不能宣稱符合本決策。** 目前 `internal/tool/browser/managed_worker_config.go` 使用 `chrome-devtools-mcp --isolated --headless`，`external_worker.go` 使用 `--wsEndpoint`；`cdp_discovery.go`、`service.go`、`contract.go` 等保留原生 CDP／reuse，`internal/browserpolicy/catalog.go` 目前唯一 external driver 為 `chrome-devtools-mcp-ws`，且 B2j～B2n-B pinned relay 仍是 CDP 傳輸研究。這些需要**版本化汰換、阻擋或徹底隔離**，不可因已有安全 fence 就視為符合 no-debug 規則。

實際遷移必備：

1. **Hard gate**：Next Core 路由、工具參數、Worker 配置、Extension Manifest、設定匯入、Desktop API 以及所有實際瀏覽器 launch／attach 出口，明確拒絕 CDP/Remote Debugging 路徑，禁用 silent fallback；包含一般 managed browser。legacy CDP compatibility 不得在 Next runtime 以任何隱藏開關重新啟用。
2. **靜態／動態驗證**：靜態掃描 Next 正式建置中的 debugging flags、Chrome DevTools MCP、CDP endpoint/`chrome.debugger`；Native UAT 在 Edge／Chrome 均完全關閉偵錯時，觀察程序 argv、browser/port/socket、Extension 權限及實際工具呼叫，證明無 DevTools attach／自動重啟偵錯；失敗即阻擋 Next Browser 發行。純 unit PASS 不代表 runtime 已合規。
3. **受控分階段遷移**：先 PoC，不動使用者目前的 Edge／Chrome、原版 AgentDock、Next 已部署 Core 或既有 CMS 流程；通過再建 Extension/Host、路由與 UI，最後 Next-only UAT。舊 CMS 經 Remote Debugging 的工作流，在使用者關閉偵錯後**可能無法使用**，直到新 route 實際安裝、配對及驗證；不能保證立即無縫切換。
4. **B2i～B2n-B 重新分類**：保留歷史 code/doc 與 regression 記錄，但不得繼續視為用戶 Profile Browser 的上線路線；單操作 Connector 更不滿足產品多步驟需求。先停 B2n 子階段延伸，除非下一次需求審查明確批准新的問題定義。

## 6. 有界 PoC 與 Go/No-Go 驗收

PoC 僅以隔離測試站／經使用者批准的測試頁進行；CMS 真實發布、現存登入 Session 與系統 UI 不作首輪試驗。以下每項均需要驗證紀錄：

| 驗收 | PASS 的可觀察結果 |
| --- | --- |
| Edge CMS Profile | 在指定已配對 Edge Profile 能導航、讀 DOM、表單操作；偵錯完全關閉且未出現 Remote Debugging 許可對話框 |
| 指定 Chrome Profile | Chrome A/B 多 Profile 明確配對，指令到指定 A；B 絕不被讀取／操作；無配對拒絕且不換 Headless |
| 一般獨立瀏覽器 | Headless/隔離 Chrome Extension + Native Messaging 無 CDP 可完成必要基本操作；否則明確記載 NO-GO |
| 禁用偵錯稽核 | 無遠端偵錯旗標／pipe／CDP WS／debugger API／DevTools MCP autoConnect／probe，Core/API/Worker 不會隱藏 fallback |
| 站點與權限 | 未獲 origin 授權不執行；撤銷站點權限或 Profile pairing 後停止；不讀取／輸出 Cookies/Token |
| 多 Agent | 兩個 Agent 分配不同 Tab；同 Tab 互斥／拒絕；不存在跨 Agent 關閉、導覽、填寫 |
| Focus／Tabs 清理 | 背景建立 tab 不切換目前使用中的視窗；只清除新建 tab；保留原有 tab 與其他 Agent tab；關閉、逾時或失敗有一致回收結果 |
| 網頁互動能力 | 明確驗證點擊、輸入、跳轉、iframe、Canvas/截圖、原生 picker、下載；不支援項標為 unsupported，不偷換方案 |
| 錯誤／失效 | Extension/Host 斷線、Browser/Profile 不見、Browser 版本更新、MV3 SW 重啟、超時／取消均 fail closed、結果隔離 |
| Next/stable 隔離 | Next Core 8767、stable 8765 不互相改設定／重啟；安裝 Next 後使用者現有瀏覽器與登入資料不變 |

## 7. 一次性的決策 Gate 與後續工作

- **B3-PoC（先做一次）**：確認 Chrome／Edge Profile + Extension Native Messaging + DOM 背景操作，及 managed headless extension 通道的實際可行性。完成需求覆蓋／局限報告，**Go/No-Go**；若不能滿足核心無焦點與網站動作，停止，不延伸 B2n-C/D，另提交有成本的替代提案。
- **B3-Bridge／Broker**（僅 PoC GO 後）：Next 私有 Native Host／per-profile pairing／origin consent／tab ownership/locking／取消回收；停用所有 Next CDP launch/attach 出口；測試無 silent fallback。
- **B3-Shared UI 與 Native UAT**（Bridge 通過後）：Edge、Chrome Profiles 管理、授權／健康／錯誤提示；在 Remote Debugging 完全 OFF 的實機執行 CMS/Chrome/managed 測試；Native GUI 驗收。
- **P6/P7 原生 GUI UAT、AGY Next-only 整合、P9 Hardening** 可以平行，不必等整個 Browser。真實 Nexus pairing 依使用者決定**繼續延後**。M9 須等必要安全驗收與 release gates；原版 AgentDock 及 AGY 獨立工作不動。

本評估不承諾擴充功能可完全複製 CDP 功能；優先滿足指定使用者 Profile、可用性、使用者焦點與分頁所有權，在無法達成時真實地拒絕。

## 8. 官方技術依據（查核 2026-10-10）

- [Chrome DevTools MCP — Configuration](https://developer.chrome.com/docs/devtools/agents/get-started/configuration)：`--autoConnect` 要求在 `chrome://inspect/#remote-debugging` 啟用遠端偵錯，連線時會顯示允許提示；`--browser-url`／`--wsEndpoint` 是外部可偵錯瀏覽器連線。
- [Chrome remote-debugging flags security change](https://developer.chrome.com/blog/remote-debugging-port)：Chrome 136+ 對預設 Profile 的 `--remote-debugging-port`／`--remote-debugging-pipe` 限制；不是使用者 Profile 自動化的替代方案。
- [Chrome Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging) 與 [Edge Chrome extension porting](https://learn.microsoft.com/en-us/microsoft-edge/extensions-chromium/developer-guide/port-chrome-extension)：Extension ↔ Native Host stdio 通訊、Chromium 擴充功能相容注意及 Edge Native Host allowed_origins。
- [Chrome scripting API](https://developer.chrome.com/docs/extensions/reference/api/scripting)、[Tabs API](https://developer.chrome.com/docs/extensions/reference/api/tabs)、[Extensions permissions](https://developer.chrome.com/docs/extensions/develop/concepts/declare-permissions)：站點授權、DOM 腳本與分頁能力及限制。
- [Chrome debugger API](https://developer.chrome.com/docs/extensions/reference/api/debugger)：為 DevTools Remote Debugging Protocol 替代傳輸，**明確禁止作為繞過方案**。
- [Chrome Extension headless E2E testing](https://developer.chrome.com/docs/extensions/how-to/test/end-to-end-testing)：新 Headless 模式可測擴充功能；不構成 AgentDock Managed Native Messaging 已獲得正式驗收的證據。
