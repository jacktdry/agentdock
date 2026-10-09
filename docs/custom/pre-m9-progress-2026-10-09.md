# AgentDock Next — Pre-M9 最新進度（2026-10-09）

> 核對時間：2026-10-09 03:12 Asia/Taipei。Repo：`/Users/wei/sideProject/agentdock-m8-permission-approval`；分支 `feature/m8-permission-approval`，核對時 HEAD `057ae1a2`。本 checkpoint 為狀態與交接，不代表 P8/M9 closeout。

## 各工作線狀態

| 工作線 | 已驗證 | 尚未完成 |
| --- | --- | --- |
| M7/M8、P1–P3 | Next identity/state/port/Tunnel 與 stable 並存隔離；M8 Permission/Approval；Next 公網連線 | Stable cutover、正式發行 |
| P4 ACP Manager、P5 MCP Management | 分別完成 closeout，均已 Next-only 部署；P5 完成獨立複審與 live UAT | 不等同 AGY-ACP .14 功能已整合 |
| P6 Plugin Management | Core/Desktop/Shared UI、獨立安全複審、Next-only Desktop/Core live UAT 通過 | Native GUI／鍵盤／焦點／選檔驗收、正式 closeout |
| P7 Nexus / Platform Essentials | 核心安全契約、Shared UI、Diagnostics 來源及離線驗證 | Native GUI UAT；**實際 Nexus 配對／網路測試依使用者要求延後** |
| P8 Browser Broker | Phase A/B/C1/C2a、C2b-A/B1/B2a/B2b；C2b-B2c macOS 唯讀 Edge 程序預檢 `a1d364e5`；B2d Microsoft on-disk 簽章前置檢查（原始碼驗證） | **真實 Edge 簽章／Profile／認證／no-focus／safe-release attestor、CDP 傳輸原子身分綁定、原生 GUI UAT**；正式 `ConnectorStatusProvider`/peer verifier 仍 `nil`，Edge 維持 `UNQUALIFIED` |
| Next 部署 | arm64 ad-hoc App、Core/Tunnel 服務註冊與 Next-only 安裝／正常更新實機驗收完成（`32abf101`、`5e074802`） | 真實故障注入的自動回復、Developer ID 簽章／notarization、跨平台原生 UAT |
| M9 | 尚未啟動 | 等待必要 parity、安全整合審查及 release-native gates |

**程式碼／已安裝版本不可混淆：** B2c `a1d364e5` 已提交但**尚未部署到 Next App**。上次 Next-only 實機安裝使用的驗證成品建置基線為 `539c68ac`（包含 B2b；不含 B2c）。目前使用 stable `$mac-dev` 作為 Next 開發、編譯、重裝、復原唯一控制連線；Next connector `macbook-air-m3` 不自我修改或安裝。

## AGY-ACP .14（獨立 Session 最新交接）

- 使用者確認 ACP 已到 **`1.2.0-agentdock.14` 候選版**，目前**等待 Next 開發／介面就緒後再安排整合測試**。
- 獨立 ACP 工作樹 `/Users/wei/sideProject/antigravity-acp-next-integration`，`feature/next-integration-optin`；交接記錄 HEAD `87cff1e`，功能修正 `9381b04`。ACP 本身測試 **456 pass／0 fail（43 files）**、TypeScript、Lint、macOS arm64 build PASS（根據 ACP 工作線交接報告）。
- .14 加強已 ACK 的 stream / SQLite 跨事件一致性；DB 刪除／回退／重複 Step ID／文字改寫 → fail-closed，避免重播已送出文字。但**尚未推送／部署**，交接文件記錄 stable 及 Next **已安裝 ACP 仍是 .8**。`AGY_ACP_STREAM_JSON=direct` 未接正式 `Adapter.runPrompt`；native M8 Hook/IPC、State/HOME opt-in、Keychain/OAuth、Session/工具／圖片／取消／Replay、stream parity 與跨專案權限驗證**尚未完成**。
- Next Session 負責 Core-owned Profile/State/M8 權限與 IPC；ACP Session 負責 AGY ACP 自身實作；**不得交叉修改 repo 或誤將 .14 候選版視為 Next 整合完成**。見 [ACP Next 整合交接](agy-acp-next-integration-handoff.md)。

## Loopback 事故與最新狀況

- 10/09 凌晨五項既有 `cdp_discovery_test.go` 本機 HTTP/WebSocket `httptest` 逾時；Next 8767 一度 TCP/HTTP 逾時，儘管 Next PID `75320` 持有 LISTEN；臨時 Python localhost 也逾時，stable 8765 可連、`lo0` UP，macOS Application Firewall 顯示停用。**根因尚未確認**，不能歸咎 B2c 或 Next Core。
- **10/09 約 03:12 經 `$mac-dev` 重新確認：Next `http://127.0.0.1:8767/healthz` HTTP 200（版本 `0.9.1`），Core/Tunnel/GUI 服務仍在，stable Core/Tunnel 不變。完整 `go test ./internal/tool/browser ./internal/browserpolicy -count=1 -timeout=120s` PASS，先前五項逾時均不再重現。** 原本的「持續阻塞」狀態改為「已恢復但根因未明／觀察中」，不得宣稱永久修復。
- P8 B2c 的 focused tests、targeted Race Detector、`go vet`、Linux/Windows cross-build 已通過。沒有因事故放寬安全 Gate 或本機網路設定。

## 下一步（優先順序）

1. **P8 可信來源與傳輸邊界**：Edge 簽章／Profile／登入與 no-focus、safe-release 驗證；實作原子化 CDP 執行個體／操作身分綁定；不得以「連得到」或 OS 程序預檢直接放行公司 Edge。
2. **Next 與 AGY .14 整合前置**：凍結 Next-owned State/HOME、ACP Profile opt-in、M8 Native PreToolUse Hook／可信 Core IPC 的授權與拒絕合約；等 Next 準備好再做 Next-only .14 聯合 UAT，勿改動原版及 AGY Session 的原始碼。
3. **P6/P7/P8 原生 GUI UAT**：視覺／鍵盤／焦點／生命週期驗收；真實 Nexus 配對持續延後。
4. **Pre-M9 整合與發行 gate**：安全複審、故障回復、簽章更新、Windows/WSL/Linux 原生驗證。

交接參照：[roadmap](roadmap.md) · [feature parity](pre-m9-feature-parity.md) · [P8 Browser checkpoint](pre-m9-browser-broker-roundtable.md) · [ACP .14 handoff](agy-acp-next-integration-handoff.md)。Next 的 build/install/service 變更一律走 stable `$mac-dev`，不碰原版 AgentDock。

## 2026-10-09 B2d 原始碼補強（部署前，非 P8 結案）

- 在 B2c 的唯讀 `lsof`／`ps`／映像路徑與二次 PID/epoch/argv 複驗間加入 Microsoft Edge 本機磁碟簽章前置檢查，限制 Apple generic chain、Team ID `UBF8T346G9` 與 `com.microsoft.edgemac`，同一 2 秒 deadline；未觸碰 Edge tabs/登入/CDP。`--deep` 在現有安裝 Edge 約 4 秒，故改為嚴格驗證 App 簽章及執行檔（各約 0.2 秒），**不視作巢狀 framework 信任或實際執行中二進位身分證明**。
- 既有 macOS localhost `httptest` 有間歇性逾時：初次完整 Browser suite 5 例逾時，重新執行整套 PASS；新 Edge focused + race、go vet、Linux/Windows crossbuild PASS。此異常需後續追蹤，不視為已根治。
- **尚未安裝／部署**，正式 runtime provider/peer verifier 維持 nil，Edge `UNQUALIFIED`，仍需 PID/簽章/已登入 Profile /no-focus/安全釋放及原子 CDP transport 綁定，且尚未執行 native GUI UAT。

## 2026-10-09 B2e Edge 參數解析防混淆（Next source-only）

- 接續 `eecb41d8`：將 Edge macOS `ps` 命令列驗證改為嚴格邊界解析，限定固定 Edge executable、唯一 `--remote-debugging-port` 和 `--user-data-dir`，接受明確的空白／引號格式，拒絕重複、混淆、控制字元、過長或非 canonical 路徑。只改私人 Edge OS preflight，不變更通用 CDP discovery。
- 新增正常與惡意 fixture，特別驗證拒絕發生在 image/signature 查驗之前。未修改 stable、AGY-ACP、Next runtime，未部署、未呼叫 Edge/CDP；`NewRoutePlanner(..., nil)` 與 `UNQUALIFIED` gate 持續有效。
- 本機偶有高負載與短時 localhost 測試逾時，對照實際測試紀錄評估，不將負載猜測當成已證實原因；避免多個 Go test/vet 並行編譯。


## 2026-10-09 B2f 執行中程序動態簽章前置檢查（Next source-only）

- 基於 `5b436f3d`，在磁碟簽章後、第二次 listener/PID/start/argv/mapped-image 複驗前，新增獨立 Security.framework PID dynamic code validity 檢查；同樣限定 Apple generic anchor、Microsoft Team ID 與 Edge identifier。`darwin && !cgo` 拒絕，Linux/Windows 原生預檢維持拒絕。
- 共用 2 秒 context；C 呼叫不能取消，因此全程序只允許一個 outstanding native verification。caller 逾時即拒絕，slot 保留至 C 返回，後續呼叫直接拒絕，避免重複呼叫累積 goroutine。任何失敗或取消只回 opaque unqualified error 與零 evidence。
- 未查詢使用者 Edge、tabs/CDP/profile，未修改服務、安裝或部署。native negative fixture 只檢查本次 Go test process。正式 provider/peer verifier 仍 nil，Edge `UNQUALIFIED`；Profile/登入/no-focus/safe-release、原子 CDP 綁定與 native GUI UAT 尚未完成。

## 2026-10-09 B2g Profile 目錄 metadata 前置檢查（Next source-only）

- 基於 `23fa0dd1`，加入私有可注入 metadata probe；程序／簽章證據前後比對 bounded ancestor chain 的 uid、mode、非零 device/inode。macOS 僅 `Lstat`，拒絕 symlink、非目錄、錯誤 owner、不安全 group/other writable mode、觀察到的替換及取消；錯誤維持 opaque、零 evidence。
- 最多 32 個 path components／4096 bytes；保守拒絕 sticky writable ancestors（含 `/tmp`）。所有成功／race fixtures 為注入資料，native negatives 僅 `t.TempDir`；不列目錄、不讀取 profile 內容、不接觸實際 Edge/CDP。
- focused Go tests（`GOMAXPROCS=2 go test -p 2`）及依序 Linux/Windows/macOS 無 cgo test cross-build PASS。獨立複驗完整 Browser/BrowserPolicy suite、targeted race 與 go vet 均 PASS；未做 native browser UAT；未提交／部署。
- 只建立 physical directory prerequisite，不證明認證／同意，也不保證原子 fence 或排除未觀察到的 ABA race；provider/peer verifier 仍 nil、Edge UNQUALIFIED，P8/M9 gate 維持開放。

## 2026-10-09 B2h safe-release peer 一致性（Next source-only）

- 基於乾淨 `d1b0656b`：release 在 target `list_pages` 後、close 前立即複驗 peer，close 返回後再獨立複驗；前者失敗不 close，後者失敗隔離看似成功的 close 或原始 close error，優先保留 opaque peer error 並永久 latch peerLost。
- connector Stop 成功仍為 CleanupComplete；無法獨立確認 close 身分時明確標記 `owned page close unconfirmed`。Stop 失敗回復只重試 AgentDock-owned connector Stop、不重試頁面操作；SweepExpired 共用此路徑。
- TEST ONLY fake hooks 精確注入 list/close 階段 peer 替換、撤銷、verifier failure、close 後取消與錯誤優先順序，檢查 happy-path counts、connector-only recovery 和不重複 close；focused validation 包含 managed lease 回歸。`GOMAXPROCS=2 go test -p 2 ./internal/tool/browser -run 'Test(External|ManagedLease)' -count=1 -timeout=120s` PASS（4.740s）；首次 sandbox 指令於 setup 因既有 Go cache 寫入權限失敗，獲准後用同一 cache 執行，未建立新 GOCACHE。已 gofmt；由主 Session 獨立重跑完整 Browser/BrowserPolicy suite、release/admission 專項 race 與 go vet，全部 PASS。這輪未跑 cross-build。
- 這是 TOCTOU 緩解與 response quarantine，**不是 atomic transport/CDP fence**；close 可能已生效，仍不能排除觀察間 swap。正式 provider/verifier 維持 nil、Edge UNQUALIFIED，未呼叫 live Edge/CDP、未做 native browser UAT、安裝／重新啟動／部署／push，P8/M9 gate 仍開放。
