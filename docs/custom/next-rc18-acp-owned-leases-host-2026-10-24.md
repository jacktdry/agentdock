# NEXT RC18 — 真實 ACP 持有租約時 Adapter 異常退出／精準回收 Host UAT（2026-10-24）

## 使用者情境與驗收判定

使用者將工作分派給多個 ACP Agent 時，Browser/Computer 租約須由 NEXT Core 保管；某個 Adapter 異常退出，不能剝奪其他 Session 的資源，也不能讓 Session 關閉後留有 Chrome／Computer 租約。本輪在**正式安裝的 NEXT RC18** 以實際 Antigravity／Gemini 3.8 Flash Standard Mode 調用 Next Core 發出的 ACP 專屬 `agentdock-computer` 與 `agentdock-browser` MCP 權限，不是測試腳本替 Agent 直接取得租約。

**Host PASS（分別的受控測試）：**
- 真實 ACP **雙 Computer observe-only 租約**：單次 NEXT 專屬 Adapter 非預期退出與恢復後，A/B 擁有者與租約持續隔離，按 A 再 B 關閉順序精準回收，最後 zero active。
- 真實 ACP **單一 managed-isolated Browser 租約**：單次 NEXT 專屬 Adapter 非預期退出與恢復後，租約、工作程序仍由原 ACP Session 擁有；ACP Session 關閉後**非同步清理完成**，最後 idle、0 owners/leases/workers/orphans 且測試 Browser worker PID 不存在。

**未測／不可推論：** 這不是「兩個 ACP Session 分享同一個 Chrome 程序」的驗收；目前 NEXT 未啟用單工作區 shared-worker opt-in。Browser 是 1 Session、1 背景 managed-isolated worker；Computer 是 2 Session、2 observe-only leases。沒有請求前景控制、沒有呼叫 Edge、沒有導航使用者網站。也未驗證正在進行 MCP Browser/Computer 操作當下 Adapter 崩潰或跨 Core 程序失效的情境。

## 正式 ACP → Computer MCP：2 Sessions / 2 leases / 逐一回收

- 專用空白工作目錄 `/private/tmp/agentdock-next-rc18-lease-uat-20261024`。真實 Antigravity ACP Adapter 上各自建立 A/B Session，選擇 `gemini-3.8-flash`、`reasoning_effort=low`、Standard Mode。兩筆各透過**模型調用** `agentdock-computer.computer_acquire`，參數 `capability=observe`, `foreground=forbidden`；ACP Prompt Run 完成後故意**保留**租約，由 NEXT Broker 實際查到 2 active、兩個不同 `owner_acp_session_id` 各 1。
- 下一步確認 NEXT Core launchd PID **27482**、Adapter 原 PID **55599** 的父 PID/UID/絕對程式路徑精確符合 NEXT；Antigravity 有且只有此 2 個 managed Session，沒有正在執行的工作。僅對已核實的 NEXT Adapter 執行**一次** SIGKILL，不碰獨立 Stable Adapter、Edge、Chrome 或任意 Core。
- Adapter 異常退出時 A/B 租約仍各自存在。NEXT 經正常 ACP `info` 恢復後 Adapter 新 PID **60467**，父 PID 仍是 27482，兩個租約所有權不變；`acp_session(close,A)` 後 Broker 只剩 **B 的 1 個租約**；`close(B)` 後**0 活躍租約**；兩筆測試 Session 均 `delete=true`，Antigravity managed/ready/running/loaded 全 0。
- 本機私有驗收與執行證據：`/private/tmp/agentdock-next-rc18-lease-uat-20261024/crash-lease-proof.json`（`lease_recovery_verdict.pass=true`），`crash_with_computer_leases.py`。

## 正式 ACP → Browser MCP：1 Session / 1 managed Chrome worker / 無殘留

- 獨立私人空白工作目錄 `/private/tmp/agentdock-next-rc18-browser-lease-uat-20261024`，真實 Antigravity／Gemini 3.8 Flash Standard Mode ACP Agent 僅呼叫 `agentdock-browser.browser_acquire({url:"about:blank"})` 一次，保留 lease，不執行網站導覽、點擊、JS、截圖或外部網域，亦無新 Edge grant。
- Prompt Run 真實 `completed/end_turn` 後 NEXT Broker **1 owner、1 lease、1 worker**，`owner_acp_session_id` 精確吻合本次 ACP Session，route `managed-isolated`，無 orphan。此測試並非 Opt-in Shared Chrome 模式。
- 已驗證 Adapter 原 PID **60467**、父 NEXT Core PID 27482、UID、NEXT 私有 binary 路徑且僅有這筆測試 Session、沒有活躍 Run。再次**只對自己的 NEXT Adapter PID** 執行單次 SIGKILL。Adapter 不在時 Browser owner/lease/worker 仍為 1/1/1；恢復成新 Adapter PID **65012**（父仍為 27482）後，owner/lease/worker 仍為 1/1/1，Session 擁有者不變。
- `acp_session(close)` 剛回傳時，有一個**短暫的非同步清理視窗**：owner=0、診斷仍顯示 lease 1/worker 1（不能把這一瞬間稱為完整清理 PASS；初版測試用過早的同步斷言而回傳失敗）。隨後正式 NEXT Browser 即時 `/internal/runtime/browser/desktop` 快照連續核實為 `idle`、**owners=leases=workers=activeOperations=queuedOperations=managedOrphans=externalOrphans=0**；Broker 的 `status` 詳細資料僅保留 `cleanup_state=complete` 的歷史 lease 與 `state=stopped` 的歷史 worker。實際原 worker PID **62309** 已不存在，沒有瀏覽器資源殘留。這是**延後完成的真正清理**，不是不會被回收的資源洩漏。
- 該 ACP 測試 Session `delete=true`，Antigravity managed/ready/running/loaded 最後全 0。私有驗收及原始事件：`/private/tmp/agentdock-next-rc18-browser-lease-uat-20261024/crash_with_browser_lease.py`、`browser-crash-lease-proof.json`、`session-manifest.json`。初次斷言失敗也有原始紀錄，未偽裝為立即完成。
- 對使用者的實際意義：Agent 的模型工具操作能走**NEXT 簽發、以 ACP Session 為範圍的 MCP 權限**；不是由 Adapter 自己直接管理 Chrome/GUI 生命週期。

## 環境與繼續開發門檻

完成後 NEXT `8767/healthz=0.9.2-rc.18`、原版 AgentDock `8765/healthz=1.1.0`，兩個 Core 健康；Browser 最終 idle/0，Computer active 0，ACP 無活躍 Session，NEXT 對 Edge 9222 無持有 socket。RC17 rollback 保留；本次不更動簽章 App、Core、launchd、OAuth 或使用者工作樹。

**已通過並可收斂：** 真實 ACP Session 持有 observe-only Computer 與 managed-isolated Browser lease 時，NEXT 專屬 Adapter 非預期退出後 Session 權限及所有權保持、重新啟動，關閉 Session 後各自正確回收；Browser 清理**是非同步**，以最終 `idle/0` 為驗收條件而非即時同步零。

**仍未完成：** NEXT 專屬 shared Chrome 單工作區 opt-in 下兩個真 ACP Sessions 是否使用同一個 Core-owned worker；Tray/menu 的實際 NEXT 圖示歸屬、可見名稱及互動；崩潰時仍在執行 Browser/Computer MCP call 的中斷清理。可參考 [RC18 ACP Adapter crash Host](next-rc18-acp-adapter-crash-host-2026-10-24.md)、[RC18 true launchd rollback](next-rc18-real-launchd-rollback-host-2026-10-24.md)。不要為已通過的租約異常測試反覆 SIGKILL，也不要為 Tray Cosmetic 持續製作 RC。正式發布仍 `release_ready=false`，不得合併／push／tag／發佈。
