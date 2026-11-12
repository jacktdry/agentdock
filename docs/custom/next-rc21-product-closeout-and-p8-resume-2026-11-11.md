# AgentDock NEXT RC21 — Milestone 結案、P8 接續（2026-11-11）

## RC21 結案（本機候選，不是公開正式發行）

| 原任務 | 結論 | 事實與限制 |
| --- | --- | --- |
| H02 NEXT Tray 辨識 | **DONE** | Installed RC21 macOS AXTitle = NEXT，選單 AX 樹/標籤可讀、Original AgentDock 未受影響；不再重複做選單點擊/瀏海/焦點的延伸驗收 |
| H04 ACP Race fixture 修正 | **DONE（原修正）** | 7 項重點 Race count10 及 ACP 整體 Race 在無重編譯負載時 PASS；大規模 CGO/Swift 並行編譯下 1 秒等待逾時作為已知 CI 穩定性風險，若真影響 CI 再修 |
| RC21 本地建置、安裝 | **DONE** | App/ZIP/DMG 校驗、codesign、Core 版本與 commit、26/26 installed SHA 和 RC20 備份 PASS；Next 8767 / Tunnel / GUI 健康、Original 8765 無變更；不為無故障情境反覆 rollback |
| R01 Hosted CI / 正式發行 | **OPEN（獨立發行時處理）** | 本機 ad-hoc、release_ready=false、Update Feed 關閉；Developer ID、Notary、Hosted CI、公開正式版都未完成 |
| H01 Connection 失敗跨頁 | **獨立待辦** | 不反向阻擋 H02/H04/RC21 已完成範圍 |
| 管理者授權提示 | **來源未明／跨 Session 待釐清** | 使用者指出 Session Coordinator K1/K2 也可能在要求提權；沒有證據確定是 NEXT 引起，不列 NEXT 必要阻擋，也不繞過 macOS 權限 |

## P8 既有真實驗收（避免重複開發）

權威的 [P8 決策與歷史驗收 §52](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md) 已於 2026-10-14 通過：ChatGPT first-party、Codex ACP、Antigravity ACP 三個**真實使用者端**，同一 NEXT Core 下共用**一個 Chrome main**，持有獨立 about:blank Leases/Pages，跨 Actor 讀取拒絕；AGY、Codex、ChatGPT 依序釋放自己 Lease 後其餘使用者端持續可用，最後 idle cleanup 後 worker/lease/orphan 歸零。

這已經是原先 P8 核心使用流程 PASS；不需要重新製造合成多個 actor 或做同一驗收。

**真正尚缺**：當時透過 Next-only 私有環境變數 AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE 暫時啟用；測試後回復環境，預設 Runtime 仍使用 per-lease exclusive。現有 source 為 internal/app/acp_browser_runtime_mode.go，只有明確 opt-in 才建 shared bridge。這代表「功能存在並通過」不等於「一般使用時已自動共享」。

## P8 接續產品工作（不增加驗收框架）

1. 查清 Next GUI/Runtime 現有設定與安全啟用入口。只在**真的缺少簡單可靠的使用方式**時做最小 source 調整；保持 stable、browser profile、權限規則不變。
2. 在安全 restart/rollback 規則及 Crash Restart Guard 尚不明確時，**不得直接把實驗性 shared 當作所有人的預設**；需要產品明確 opt-in 與可觀察模式。
3. 已有三真實 Actor PASS 作為沿用證據；新變更只跑最接近的回歸和相符的真實工作流程。不啟動多餘 Chrome，不接管使用者 Edge/Chrome，亦不干擾任何當前 ACP session。
4. CMS 指定 Edge profile、使用者指定 Chrome profile 和正式 CI/發行是各自獨立任務，不納入 H02/H04/RC21 結案。

相關證據：[RC21 installed Host](next-rc21-installed-host-2026-11-11.md)、[本地候選](next-rc21-local-candidate-host-evidence-2026-11-11.md)、[權限提示](next-rc21-admin-auth-tray-handoff-2026-11-11.md)、[P8 §52](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md)。

**禁止範圍：**本次不變更 installed RC21/Original、使用者日常 Browser/Edge、macOS 權限，不使用 Codex ACP，不 Push/Tag/正式發行。

## 本輪 P8 原始碼盤點結論（不做沒有必要的改動）

已核對現有 Core 與 GUI，這不是共用 Broker 功能「尚未寫出」：

- internal/app/specs_browser.go 的第一方 browser_broker 與 ACP 使用相同 ACPBridge，具備 direct_acquire/evaluate/snapshot/release，具名 owner/lease 給既有協調器處理；另有 legacy browser_session，不應誤稱為共享 Broker。
- internal/app/acp_browser_runtime_mode.go 的 shared 單 workspace/capacity=1 實作存在；目前需要 Next-only canonical workspace 明確選擇才啟用，未設時維持 exclusive。
- internal/app/runtime_browser_desktop.go 及 Vue BrowserSnapshotDetails/BrowserReadinessGuidance 已可顯示 exclusive vs experimental_shared 的實際 Broker 模式和剩餘容量、非真實路由能力不得推論登入或使用者許可。
- 2026-10-14 §52 已是真正三 Actor 共用一 Chrome／互不串頁／分別 release 的產品驗收，不需要再跑三 Agent／Synthetic Browser test。

**具體未解決的產品問題：**缺少一般使用者可安全操作、可回復、能在啟用前檢查活躍 Lease 的共享模式設定入口；現行 env 直改加 Core restart 為測試階段的操作流程。不能只把 Next Runtime 直接改成預設 shared：有意設計的 sticky restart guard 會在不確定 crash/orphan ownership 時阻止再次啟用 shared，直接擴大預設可能影響整體服務可用性；也不能清掉 marker 或偷換成 exclusive 來掩蓋尚存的 owned resource。

因此**本輪不改 Core/GUI Source，也不更動現役 NEXT Core 設定**：沒有可證明安全且比現有方式更簡單的局部 patch；請不要誤認為 P8 尚未實作或以更多測試框架代替產品啟用體驗。要正式產品化時應建立一個獨立、定界的 NEXT-only 明確啟用/停用 UI 或受管控設定流程，包含原本 existing leases 已清空、可恢復舊設定與 shared guard fail-closed 的使用者友善錯誤提示。任何實際修改都不得影響 Original、user Edge/Profile 或當前工作。

**產品優先順序調整：**P8 真實共享/隔離/cleanup 基礎已驗收 PASS，啟用 UX 為獨立產品化工作（不追究更多合成 UAT）；H01 GUI 只在實際使用者遇到失敗狀態問題時優先；R01 正式發行需求確定後才啟動 CI/Signing。不再擴大 RC21 關門測試清單。
