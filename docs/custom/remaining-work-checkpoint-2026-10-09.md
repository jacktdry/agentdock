# AgentDock Next — 2026-10-09 剩餘工作盤點（B2k 後唯讀 checkpoint）

> 狀態：**獨立唯讀盤點，不是 closeout**。`feature/m8-permission-approval` 檢查到 HEAD `99a88b7a`（2026-10-09）；另有 session 正持續負責 Browser Broker，本文件不改其程式碼、部署或任務分工。預設依較新的 commit/owner session 狀態更新，不能以此凍結其後續進度。

## 已有成果，不重做

- M7.5 AgentDock Next 隔離與 Next connector `macbook-air-m3` 建立，stable/Next 並存；M8 Permission/Approval、P1–P3 Connection、P4 ACP Manager、P5 MCP Management 已 closeout。
- P6 Plugin Management 後端、Shared UI、獨立安全審查、Desktop/Core live UAT 已完成，但 **native GUI/keyboard/picker** 尚未通過。
- P7 Nexus/Platform Essentials 來源與離線驗證已進行到 Shared UI/Diagnostics，**native GUI UAT** 未完成；使用者要求延後真實 Nexus pairing/re-pair/network UAT，不阻塞 non-Nexus 工作。
- P8 B2i `6ff0f789`（`CallExternalFenced` required admission）、B2j `bf77b6b6`（單 upstream WebSocket 的受控 relay primitive）、B2k `99a88b7a`（macOS 已建立 TCP Socket OS owner evidence）已提交且有測試紀錄，**source-only**，不等於可操作公司的 Edge。仍無 production provider/grant、可信 full transport attestor 或 Native UAT。

## 仍待處理（由前到後）

| Priority | 工作 | 確切完成標準 | Ownership/備註 |
| --- | --- | --- | --- |
| **P0** | **P8 Browser Broker trusted transport integration** | 將 B2i/B2j/B2k 接成實際 transport fence：binding immutable worker/lease 與 OS socket/process/profile identity，逐命令有安全證明；Relay 不換 socket；on-loss fail closed，無 legacy fallback；正式 provider/verifier 由 nil 改為可信且經審查的實作 | **已有另一 Session 負責，勿同時修改** |
| **P0** | **P8 原生 Edge 及 Browser UAT** | consent-based、登入 Profile/no-focus/安全釋放、租約回收與 cleanup、授權/取消/跨專案/程序變更測試，含對照 managed Chrome；未驗證前 Edge=UNQUALIFIED | 使用者 Edge 個人資料/視窗不可無授權碰觸 |
| **P0** | **AGY-ACP `.14` Next-only 聯合整合** | 先凍結 Next-owned State/HOME、opt-in、M8 Native PreToolUse/可信 Core IPC，後做 Keychain/OAuth、ACP Session/Tool/Image/Cancel/Replay/Streaming 對照實測 | ACP repo `87cff1e` `.14` candidate tests 456 PASS；未證實部署或聯合 UAT，ACP 改動由獨立 Session 負責 |
| **P0** | **P6/P7/P8 native Shared GUI UAT / integration review** | P6 Picker/focus/keyboard/320px 視覺；P7 Connection/Nexus tab/Diagnostics GUI；P8 Browser status/lease/route GUI 與權限/安全審查 | Nexus 真實 pairing 持續 DEFERRED，不能以 source PASS 宣告整體結案 |
| **P1** | **P9 Pre-M9 hardening** | `packaging/macos/app-identity.sh` script-governance debt 清除；M7 Codebase Memory lifecycle debt triage，僅 release blocker 前置修 | 不擴大成 CBM 全面重構 |
| **P1** | **P10 / M9 Release Migration** | `custom/main` 專屬 release source/tag/version/buildinfo/update-channel；macOS/Windows/必要 Linux helper artifacts；可信簽章、notarization、更新通道隔離 | 詳 `upstream-v101-adoption-plan.md` |
| **P1** | **P11 Release-native UAT** | macOS signed update/trial/commit、真實中斷／回滾；Windows、WSL、Linux-only helper native 測試；stable/Next 隔離與失敗復原 | Cross-build 不算 native，外部必要證據不可偷標 PASS |
| **P2** | **PR #229 Task Progress live-card UX** | 新建後單張 Task card 持續反映 checkpoint/final review/complete；通過授權／負載／快取／真實 ChatGPT UI UAT | 可獨立做；不阻擋 P8 或 M9，詳 `upstream-ports/task-progress-live-card-pr229.md` |
| **P2** | **P12 native business UI deprecation** | M9 與 Shared UI parity 穩定後才逐步停用重複 AppKit/WPF 頁面，保留 OS 權限／安裝器／啟動 native adapters | M9 後 |
| **P2** | **cloudflared Component Manager** | 獨立可信版本、Catalog、原子更新、signed publisher／目錄 owner、舊 Helper 遷移與 Tunnel 不中斷 | M9 後獨立 milestone，詳元件移植文件 |
| **Deferred** | **真實 Nexus pairing / stable retirement** | 需使用者另行同意/安排；只在 Next 完整驗證與獨立連線後提出 stable retirement 計畫 | 目前不做、不得當成已完成 |

**新增 UX follow-up（2026-10-10）**：ChatGPT iOS 使用者實際遇到重複 Workspace / ACP Session / Model(no-change) / Reasoning(no-change) 卡片，**不等於** PR #229 Task 卡片問題。新增 P14 [MCP Apps 卡片降噪](mcp-apps-card-noise-reduction.md)，與 P13 PR #229 並列非阻塞 UX 工作；原 P8／AGY／M9 順序不動。B2 的最新狀態需以開發 session 的 commit 為準，不沿用本文件 B2k 截止標籤判定進度。

## 風險與接手規則

1. B2i/j/k 的 focused/race/vet/crossbuild PASS 是 **source evidence**。`NewRoutePlanner(..., nil)`、Edge `UNQUALIFIED`、沒有 live production grant 的 Gate 仍為真；不要拿 fixture/relay socket 建立成功宣稱可用公司 Edge。
2. P6/P7/P8 需要真實 GUI 視覺/鍵盤 UAT，且 live Edge 必須經同意；不可因使用者延期 Nexus pairing 就偷把它標成已通過。
3. Next/Core/Service/Installer mutation 只能走 stable `$mac-dev`；`macbook-air-m3` 只做 read/runtime 驗證，不用 Next 自改自裝。stable 8765、Next 8767、Memory 8766 與各自 Tunnel/registry/secret 必須嚴格分離。
4. AGY `.14` candidate 與 Next 的接線結果不可混淆；前者 456 tests pass 不等於整合完成。專案工作由原 Owner session 負責，本文件不啟動第二組寫入者。
5. 此分支的文件來源是當下 repository/交接的 read-only snapshot，其他 Session 新增 commit 後需重新核對 status，避免過期交接。
