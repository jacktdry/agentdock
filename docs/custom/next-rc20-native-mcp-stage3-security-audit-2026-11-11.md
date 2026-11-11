# AgentDock NEXT RC20 — Native MCP 階段三唯讀安全盤點（2026-11-11）

## 任務與使用者價值

使用者已接受 Stage1 Git/Milestone 盤點與 Stage2 GUI/Tray 不中斷診斷，現要求優先進行**不使用 Codex ACP 的 Stage3 Native MCP 審查**。要解決的問題是：NEXT 看起來有 Native Codex MCP 的 Launcher、M8 與 GUI 狀態，但哪些已安裝／可使用，哪些只有測試與安全草圖，若未區分容易重複開發、錯誤宣稱可啟用，甚至用未授權的本機執行檔進行 Native MCP 啟動。

**最小交付：**比對安裝 RC20 簽章與實際元件、還原 Desktop → Core → M8 → Launcher/Backend 的真實來源路徑、重跑拒絕／狀態的定點離線測試，並列出按產品工作流程排的安全整合順序。**不做** Codex ACP、Native Backend 啟動、信任／授權設定修改、GUI 點擊、Original 安裝/重啟、Push／Merge／Tag／正式 Release。此報告中 `source PASS`／`installed artifact PASS`／`installed Native Host OPEN` 分層不可混用。

## 1. 已安裝 RC20 的唯讀實證

| 核對項 | 本輪觀察 | 不代表 |
| --- | --- | --- |
| 正式 NEXT 安裝身分 | Bundle `dev.dropabit.agentdock.next`、Build `21.0.0`、Core `0.9.2-rc.20`／`704bb40d08f6` | 不等於公開發行／Developer ID |
| Shared App 完整性 | `codesign --verify --deep --strict` PASS（Ad-hoc）；Bundle GUI 與 Launcher 皆有預期 identifier | Ad-hoc 不證明官方可信發布者或人類授權 |
| Native MCP Launcher | `Contents/Helpers/agentdock-next-mcp-launch`，arm64、Identifier `dev.dropabit.agentdock.next.mcp-launch`；與原已簽章 RC20 candidate SHA-256 相同（`4628e12a...`） | 不代表已啟動 Server |
| Core-owned Catalog | `Contents/Resources/native-mcp-servers.json`，`schema_version=1`、`servers=[]`、34 bytes；與 RC20 candidate SHA-256 相同（`58038eb8...`） | **0 個 Backend，無可選 server ID** |
| Backend 資產 | App `Contents/Helpers/native-mcp/` 不存在，亦未發現封裝的 Native Backend | 將來是否要封裝何種產品有用 Backend 尚未決議 |
| 本機健康 | 原版 `127.0.0.1:8765` / NEXT `127.0.0.1:8767` 均 HTTP 200，測試前後不變 | MCP Native Session 的 Host UAT 仍 OPEN |

本次並未查詢／修改任何使用者的 Token、原版 credentials、Native policy journal 內容或未授權身份。密碼、執行參數、Approval principal 皆不寫入文檔。

## 2. 現有呼叫鏈：可重用與尚無正式接線之處

```text
NEXT Shared Wails ACP Panel (`ACPPanel.vue`, `stores/acp.ts`)
  └─ 唯讀 ACPService.NativeCodexReadiness（無啟動/核准按鈕）
      └─ desktopruntime.ReadVerifiedNextNativeCodexReadiness
          └─ local desktopcontrol.CallVerifiedPID → NEXT Core `control.sock`
              ├─ `next_codex.readiness`（唯讀、fail-closed、無 adapter 啟動）
              └─ `next_codex.operator_identity`（唯讀：kernel peer + signed Bundle 出處）

***未接線的 production mutation/Native start：***
  [可信 NEXT GUI 真正明確操作／本人確認／授權意圖：尚無正式 route]
    → [Kernel peer 不等於真人同意；須由 Core 區分與驗證]
    → Core private prepareNextNativeMCPRegistry(serverID)
      → Core-only signed Catalog → immutable selection/digest
      → M8 AuthPrincipal + Ask/Allow/Deny + revision fence
      → private B6 signed Launcher, Core listener, scope/journal/lease
      → native Codex app-server + Backend attach + model tool call
      → precise session cleanup + unresolved generation recovery
    → [完整 Installed Native Host 接線未完成]
```

**實際原始碼來源與避免重做：**

- `cmd/agentdock/server.go` 現有 Core `control.sock` dispatcher 只對 `next_codex.readiness`／`next_codex.operator_identity` 提供相關**唯讀** route，`operator_identity` 拒絕攜帶任意參數；沒有 `prepare/start/attach` production IPC/HTTP/CLI entry。
- `internal/desktopapi/acp.go`／`internal/desktopruntime/native_codex_readiness_darwin.go` 已做已驗證 NEXT Core PID/instance/binary 身分、前後核對與錯誤狀態 redaction；Frontend `ACPPanel.vue` 是文字狀態，不是操作入口。
- `internal/app/runtime_next_codex_mcp_registration.go` Core-private B6 有 `prepareNextCodexMCPRegistryChecked` 和 `attachNextCodexInstrumentedMCPChecked`、M8 admission、epoch/revision、Core scope/lock/journal，且 **source-only mode** gate 仍要求 `AGENTDOCK_NEXT_CODEX_MCP_REGISTRATION=source-only`、Next Desktop variant、私有 Home 與 owner。不能未查證就自行設定該環境變數開啟功能。
- `internal/app/runtime_next_native_mcp_registration_darwin.go` 已有只收 `serverID` 的 Core-private Prepare/Attach、簽章 resolver double-check、digest pin 與 M8 fence。`internal/app/runtime_next_native_mcp_catalog_darwin.go`/`runtime_next_native_mcp_catalog.go` 限制只有 bundle 指定 `Helpers/native-mcp/<id>`、不接受任意 command/shell/script/env/參數來源；嚴格檢查 JSON schema、arm64、symlink/hardlink、inner/outer codesign，M8 Allow 後重核對 Catalog。**上述私有 API 不等於已公開啟用路徑。**
- `internal/app/runtime_next_codex_mcp_registration.go::NextCodexOperatorIdentity` 只回傳 `desktop_peer_verified` 或其他受控狀態。Kernel PID/UID/birth、執行檔簽章/Bundle provenance 只能說明「是哪個程式」，**不是操作者同意**。`internal/desktopcontrol` 的來源驗證也不應改寫成「localhost/同 UID 就可信」。
- `internal/app/runtime_next_codex_startup_lock_darwin.go` 對 owner-private generation 與 crash journal 保守處理，來源文件要求 unresolved journal/orphans 不可藉 PID 年齡或程序名稱猜測安全後清理。

## 3. 本輪定點測試、證據等級

在本地 clean compat branch（RC20 source 704bb40d 已是 ancestor，程式碼無 Native MCP 異動）透過 **Mac-Dev 本機 CLI**，不載入 Codex ACP 或 Native Backend，執行：

1. `go test -count=1 -timeout=110s ./internal/app -run '^(TestNextNativeMCPCatalogStrictSchema|TestNextNativeMCPCatalogRejectsCommandOverrides|TestNextNativeMCPIDOnlyRejectsBeforeAnyM8OrNativeStartup|TestNextNativeMCPIDOnlyStillRequiresM8Authorization|TestNextNativeMCPIDOnlyRechecksCatalogAfterM8Approval|TestNextCodexReadinessNeverStartsNative|TestNextCodexOperatorIdentityOnlyReportsTransportProvenance)$' -v` → **7 項頂層 Test PASS**，包含多個拒絕子測試。
2. `go test -count=1 -timeout=90s ./internal/desktopapi -run '^TestACPNativeCodexReadinessRedactedAndFailClosed$' -v` → **1 項 Test PASS**，失敗時 `unavailable`，不洩露 private 訊息。
3. `go vet ./internal/app ./internal/desktopapi ./internal/desktopcontrol` → **PASS**。
4. `git status` clean，Original／NEXT port health 200；未執行完整 Root Suite、Native Backend model tool、實際 Desktop M8 人類確認或 Host native launcher 啟動。

**結果：**ID-only/錯誤 Catalog fail-closed、無法跳過 M8 Ask/Deny、M8 核准後變更 Catalog 拒絕、不可偽造 socket peer 身分、readiness 不啟動程式等**source test PASS**。這些不能證明生產環境已有經受信任 Operator 做出的真人授權。

## 4. 真正阻擋產品可用性的缺口（依使用者價值排序）

| 次序 | 缺口與影響 | 後續必要最小工作／完成判斷 |
| --- | --- | --- |
| P0 — 功能用途與 Backend 選擇 | **Catalog 是空的、沒有任何 Backend**；使用者無法透過 Native Codex MCP 呼叫工具 | 先挑 **一個明確真實用途**，選定安全可維護的 Native Backend、manifest/allowlist 合約，形成可封裝的 signed backend 身分；在未決定用途前不要擴增平台或盲目引入任意自訂命令 |
| P0 — 使用者操作與 Core 授權 | Desktop 僅有 Readiness 文字，沒有可驗證的操作意圖、批准/拒絕入口或 production Core route | 在 NEXT Wails ACP UI 加**一個清楚的使用者動作**，Core 驗證 Desktop peer 後採已存在 M8 request/confirmation，**不能把 peer_verified 當 M8 Allow**；只傳 Server ID，不讓 UI 選任意程式／argv |
| P0 — 受控生命週期實際接線 | Core-private B6 Prepare/Attach、policy fence、launcher 已有但沒有正式 admission-to-backend 主線 | 將可信操作與既有 B6 private path 最小接線，驗證 exact profile/Adapter generation、single-use consent、config digest/revision、Session 內 native MCP tool、Close 及僅清理 owned 程序 |
| P1 — 安全與復原 | Ad-hoc Bundle 可驗證完整性但非 Developer ID；path-based verify-to-exec 有 same-UID 替換視窗；未結 native generation recovery 未經安裝版證明 | 發版前定義實際信任根、發佈策略、必要的 image/owner pin 或合適替代方案；驗證 crash/timeout/orphan fail-closed 與人工核准復原；避免創造不必要的獨立 VM／權限 daemon |
| P1 — Host 驗收 | Native backend 真實 Session→model tool→close 未執行；Codex 端另有配額限制 | 在 Codex 可用且 Backend 已合法封裝後，以 NEXT-only 實機一步一步測、確認 Original/Core/Browser 不受干擾、沒有亂開全域 native MCP 或改 Codex HOME；對本機測試通過與 Hosted Release 分別結案 |

**不需重做：**既有 Signed Launcher／Catalog parser、Core-owned server ID Resolver、M8 permission policy、Desktop Core PID read-only status、kernel peer provenance source、B6 crash fence/journal、安全配置與 source regression。也不能自動把先前 P5 一般 MCP 管理當成 Native Codex MCP 的完整產品功能；兩者生命週期不同。

## 5. 後續執行策略、邊界與交接

- 這次完成的是 Stage3 **調查與 source proof**。Native Codex MCP 的整條「可信操作→Backend→工具回傳」正式產品 Host Gate **OPEN**；既有 ad-hoc app 不等同 Developer ID/trusted publisher。
- 建議 Stage4 先完成不需要 Codex ACP 的**發行工程現況／跨平台原始碼測試與必要缺口清單**（不 Push／Tag／部署）。若 Stage2 Connection/Tray 的真實 GUI 黑畫面仍未釐清，MCP 新操作 UI 的 Host acceptance 仍需先解決那個阻擋，不能跳過。
- Native MCP 真正開發須先明確定義第一個 Backend 的使用者工作流程與效益，再以獨立 clean worktree 分期提交，搭配 Codex 額度恢復後的 real model Host 驗收。現有使用者前景 app、Original 服務與 signed RC20 當成不允許干擾的固定條件。
- 沒有 Git merge/rebase/push/tag、沒有重新簽章或安裝 App、沒有更動 user credentials、Core M8 Policy、permission journal、Native MCP startup lock、Browser/Computer leases。

**相關最新工作證據：**[Stage1 Git/Milestone](next-stage1-branch-milestone-inventory-2026-11-11.md)、[Stage2 GUI/Tray](next-rc20-connection-tray-nofocus-host-2026-11-11.md)、[RC20 Installed ACP](next-rc20-installed-host-2026-11-11.md)、[Core ID dispatch](next-native-mcp-core-id-dispatch-2026-10-25.md)、[Desktop peer provenance](next-native-mcp-desktop-peer-provenance-2026-10-26.md)、[P13/P14 UX 決策](p13-p14-ux-scope-decision-2026-10-22.md)。

## N00 — 第一個 Native MCP Backend 產品決策（2026-11-11）

**決策：NO-GO / 維持停用；不立 N01～N03 正式程式實作工作。** 這不是否定已有 Signed Launcher／Catalog／M8 架構，而是目前缺少能證明「真正值得啟動第一個 Backend」的用戶流程。本決議不修改任何安裝版 Catalog 或使用者權限。

| 產品問題 | 經現況核對的答案 |
| --- | --- |
| 目前真實痛點 | ChatGPT 在維持 Mac 桌面正常使用時，安全地完成專案 Git／CLI、Browser/Computer 操作、ACP Session 及權限/租約清理；痛點在不中斷、正確身分、透明恢復，而不是「缺一個 Native MCP 工具」本身 |
| 現有替代方案 | Original Mac-Dev MCP 已可透過已驗證 Core/CLI 操作、日常 Browser/Computer/ACP；具名外部 App 由現有 Plugin、受控第三方 MCP 或專用 CLI 提供。不需 Native Codex Backend 才能完成本輪 A–F；Native 對一般文件/狀態查詢也尚無明確缺口 |
| Native 獨特價值的候選 | 若未來存在**只能由簽章的 NEXT Bundle 內固定執行檔**完成、必須由 M8 每次驗證意圖、同一 ACP Session 定向所有權與精準回收、且現有 MCP/Plugin/CLI 不能安全/可靠實作的實際工作，Native 的身分 pin + Core lifecycle 才可能形成不可替代價值；目前沒有經用戶現場驗證的個案 |
| 額外成本/風險 | 引入 Backend 預設權限、Installer/Apple 簽章與 notary、Catalog allowlist、可信操作者批准 UI、verify→exec 差距、獨立 ACP lifecycle、失敗殘留處置，並帶來跨平台維護與新的發行攻擊面。此成本目前高於已量測的增益（尚未有增益量測） |
| 現在該做什麼 | 保留 `servers=[]`、Native startup off、既有唯讀 Readiness 與安全 source tests；先完成 H01/H02、ACP/Browser 真實必要缺口、R01 安全發行。不要因架構已存在而承諾 N01/N02/N03，也不要把 Kernel peer provenance 當真人批准 |

### 允許重新開案的可量測前置條件

1. 從**真實且自願的**至少 10 筆近期代表性工作流程中紀錄：輸入/所需許可、現有 Mac-Dev／ACP／Plugin 的成功率、步驟、等待/重試時間、殘留/焦點風險；不蒐集 Token/敏感內容。由使用者確認其中**至少 3 筆重複出現、實際阻礙完成**的工作，而不只是功能願望。
2. 對每個候選 Backend 做同一任務的現有工具 Baseline；若同等授權/安全下可完成，Native 必須提出**額外可觀察優勢**（例如中位數操作/人工步驟減少 ≥30%，或能解決有重現證據的高嚴重度 ownership/失敗問題）；不是純粹少敲一個指令。
3. 寫出單一固定 server ID、唯一 signed binary、必要權限與資訊出口、明確的 Deny/Timeout/Cancel/Fail-closed 鍵和獨立回滾機制。現有 Plugin/CLI 完成度若更高，優先擴充既有工具，不新增 Backend。
4. 小範圍真實 Pilot：至少 5 次互相獨立的實用工作、0 次非本人核准/跨 Session cleanup、0 次 Original/Tunnel 中斷、0 次額外前景切換/提示；任何高風險負例即停用，不得依 fixture 假 PASS。再評估是否值得長期簽章/版本維護。

**無法達標的停止條件：**找不到三個實際痛點、既有工具已可滿足、需要全域高權限/任意 shell、沒有真人意圖與 ownership 可靠確認、或成本收益無法測量 → **NO-GO 持續有效**。若未來證據達標再開 N01（單一 Backend）→ N02（可信操作者）→ N03（真實 Session/model tool Host），每個階段另有安全 Gate；本次沒有任何 Native Backend / model tool Host PASS。
