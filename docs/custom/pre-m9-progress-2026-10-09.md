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
