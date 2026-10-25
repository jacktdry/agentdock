# NEXT Native Codex MCP：可信 Desktop 唯讀啟用前檢查（2026-10-25）

## 目的與使用者可見改善

目前 NEXT 的 Native Codex MCP Core B0–B6 已有隔離 source-only 準備／附加授權與程序回收，但 Wails Desktop 的 ACP 頁面沒有可信入口告訴操作者：選取的是否為真正 Next Core、功能是否開啟，或上次 native generation 是否存在未結清 journal。直接開放 `prepare/attach` 會讓未知可執行檔與 launch args 經過一般 UI 傳入，並可能在全域 M8 Full 下放行，**不符合可信任的 Core-owned allowlist 與正式使用者授權要求**。

本次只新增面向真實 ACP 介面的**讀取權限狀態入口**：Next Core 的 `NextCodexMCPReadiness` 經已存在的本機 `desktopcontrol` Unix socket `next_codex.readiness` 唯讀方法回報有限狀態；Wails 的 `ACPService.NativeCodexReadiness` 先以已存在的簽章 Next Core 選取、kernel PID/birth、socket peer PID/UID 驗證結果，不接受普通 TCP、MCP bearer、外部服務冒充 Next。Core 再次驗證 state-only response，避免在 Wails 讀取過程發生程序替換。ACP 頁面在繁中、簡中、英文顯示清楚訊息，明確說明只是唯讀、沒有登入、授權或啟用 Native MCP。

有限狀態：`disabled`（預設不啟用 source-only）、`owner_unavailable`、`core_closing`、`registration_present`（來源註冊存在不代表 production）、`startup_unresolved`、`state_untrusted`、`recovery_required`（上次 generation 的非空 journal，不可自動清除）、`operator_setup_required`（缺可信任 operator action 與 Core-owned executable allowlist）。遇到身份、協定、斷線或平台不支援時，UI 一律 `unavailable`，不保留 stale ready 狀態。所有輸出均沒有 PID、User Path、Token、Secret、launch args 或可執行檔資料。這些狀態皆不等於「可正式啟用」；本次沒有加啟用按鈕或執行命令。

## 邊界與驗證

- 源碼模式下只會 `os.Lstat` 檢查 Next-owned native generation lock 的型別、模式、owner/link 與大小；不建立、不清除、不覆寫 journal，也不取得 kernel process cleanup authority。
- 原始碼 `internal/app/runtime_next_codex_readiness_darwin_test.go` 驗證沒有 Adapter/registrar 啟動，journal 原樣保留；不可信 symlink 僅回報阻擋。Desktop API 測試驗證無法冒用狀態及傳輸錯誤不洩漏秘密。
- `internal/desktopruntime/native_codex_readiness_darwin.go` 重用 existing `ReadVerifiedNextBrowserSnapshot` 的 signed Next selector + `CallVerifiedPID`；Linux/Windows 的 stub 一律 fail-closed，沒有改變 Stable 的 Runtime 與既有註冊方式。
- 這仍是 **source-only** 的 Desktop 可視狀態準備：安裝版 NEXT RC19 沒有新 Wails UI/IPC，無實機 Host 通過宣稱。無外部網站、TCC/Keychain 授權、重新安裝、切換前景或掃描／殺掉本機程序。

## 後續真正操作功能的界線

此里程碑不是正式授權 Route。下一步必須先**從 Next Core 所有的、可信任且不可由前端自由執行任意命令的配置取得完整的 wrapper/backend/server fingerprint allowlist**；接著定義明確 Desktop 人類操作／confirmation 与可信任 operator principal，經 M8 的 exact Allow/Ask/Deny 後才呼叫 Core-private Prepare/Attach，且需要跨 Core writer 排他、可報告的 Native session/stdio MCP lifecycle 和 same-boot orphan recovery policy。不得用只憑 localhost、任意 bearer 或客戶端自填 SHA digest 取代此授權。

仍未驗收：Connection 真實 GUI 拒絕後跨頁錯誤、Tray 真正 NEXT 身分／安全操作、已安裝 Native Auth/ACP Session→MCP→close/Host M9；不得 merge main、push、tag、發布。
