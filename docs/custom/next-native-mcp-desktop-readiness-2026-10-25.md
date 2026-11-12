# NEXT Native Codex MCP：可信 Desktop 唯讀啟用前檢查（2026-10-25）

## 目的與使用者可見改善

目前 NEXT 的 Native Codex MCP Core B0–B6 已有隔離 source-only 準備／附加授權與程序回收，但 Wails Desktop 的 ACP 頁面沒有可信入口告訴操作者：選取的是否為真正 Next Core、功能是否開啟，或上次 native generation 是否存在未結清 journal。直接開放 `prepare/attach` 會讓未知可執行檔與 launch args 經過一般 UI 傳入，並可能在全域 M8 Full 下放行，**不符合可信任的 Core-owned allowlist 與正式使用者授權要求**。

本次只新增面向真實 ACP 介面的**讀取權限狀態入口**：Next Core 的 `NextCodexMCPReadiness` 經已存在的本機 `desktopcontrol` Unix socket `next_codex.readiness` 唯讀方法回報有限狀態；Wails 的 `ACPService.NativeCodexReadiness` 先以已存在的簽章 Next Core 選取、kernel PID/birth、socket peer PID/UID 驗證結果，不接受普通 TCP、MCP bearer、外部服務冒充 Next。Core 再次驗證 state-only response，避免在 Wails 讀取過程發生程序替換。ACP 頁面在繁中、簡中、英文顯示清楚訊息，明確說明只是唯讀、沒有登入、授權或啟用 Native MCP。

有限狀態：`disabled`（預設不啟用 source-only）、`owner_unavailable`、`core_closing`、`registration_present`（來源註冊存在不代表 production）、`startup_unresolved`、`state_untrusted`、`recovery_required`（上次 generation 的非空 journal，不可自動清除）、`launcher_unavailable`（自己的 bundle launcher 缺失或無法驗證）、`operator_setup_required`（launcher 通過本次唯讀檢查，但缺可信任 operator action 與 Core-owned backend executable allowlist）。遇到身份、協定、斷線或平台不支援時，UI 一律 `unavailable`，不保留 stale ready 狀態。所有輸出均沒有 PID、User Path、Token、Secret、launch args 或可執行檔資料。這些狀態皆不等於「可正式啟用」；本次沒有加啟用按鈕或執行命令。

## 邊界與驗證

- 源碼模式下以 `os.Lstat` 檢查 Next-owned native generation lock 的型別、模式、owner/link 與大小；不建立、不清除、不覆寫 journal，也不取得 kernel process cleanup authority。只有既有 source-only permission owner 已啟用且沒有較優先的 lifecycle/journal blocker 時，才驗證自己的 signed launcher；一般 source runner 沒有 signed bundle 時必須回 `launcher_unavailable`，不能 auto-opt-in。
- 原始碼 `internal/app/runtime_next_codex_readiness_darwin_test.go` 驗證沒有 Adapter/registrar 啟動，journal 原樣保留；不可信 symlink 僅回報阻擋。Desktop API 測試驗證無法冒用狀態及傳輸錯誤不洩漏秘密。
- `internal/desktopruntime/native_codex_readiness_darwin.go` 重用 existing `ReadVerifiedNextBrowserSnapshot` 的 signed Next selector + `CallVerifiedPID`；Linux/Windows 的 stub 一律 fail-closed，沒有改變 Stable 的 Runtime 與既有註冊方式。
- 這仍是 **source-only** 的 Desktop 可視狀態準備：安裝版 NEXT RC19 沒有新 Wails UI/IPC，無實機 Host 通過宣稱。無外部網站、TCC/Keychain 授權、重新安裝、切換前景或掃描／殺掉本機程序。

## 後續真正操作功能的界線

### Core-local signed launcher selector（source-only）

`internal/app/runtime_next_codex_launcher_darwin.go` 的 production 入口只從 `os.Executable()` 取得 `AgentDock Next.app/Contents/Helpers/agentdock`；固定選取同目錄 `agentdock-next-mcp-launch`，沒有 caller base path、digest、環境路徑 override 或公開 selector API。對 Core、launcher、Info.plist 與每層祖先拒絕 symlink；regular file 必須單一 hardlink。Bundle 內被檢查的 entries 必須是目前 UID、不可 group/world write 或 setuid/setgid；祖先也可由 root 擁有，僅 root-owned sticky 暫存目錄允許一般暫存寫入。

Core 和 launcher 必須為 thin arm64 Mach-O executable；Go 標準庫 `debug/buildinfo.ReadFile` 驗證 launcher command path 與 Darwin/arm64 build settings，不依賴已安裝 `go`。固定 `/usr/bin/plutil` 驗證 Next bundle identifier／variant，`codesign --verify --strict -R` 驗證**確切** Core 與 launcher identifier，再以 `--deep --strict` 驗證 outer bundle seal。檢查共用 3 秒 child-tool deadline，使用固定環境，不顯示 raw errors。簽章工具在 `lifecycleMu` 外執行，完成後重新讀取 lifecycle／journal；不快取成功結果，B6 private Prepare/Attach semantics 不變。

這是當次磁碟觀測值，沒有 launch／清理／授權能力。前後 inode／POSIX mode／owner／link 和 regular-file size／mtime 檢查可拒絕觀察到的替換，但不是防止同 UID 改寫的原子執行 pin，也沒有額外驗證 ACL 或正在執行映像的 kernel CDHash。沿用現有 ad-hoc 套件簽章契約，並非 Developer ID／公證／可信 publisher 或 operator provenance。不能把這個結果或客戶端 digest 當作 start authorization；正式啟動仍需重新驗證及綁定可信 Core-owned backend executable／argv allowlist。

簽章有效不代表完整檔案逐 byte 相同：原先只翻轉 signed Mach-O 最後一 byte 的 fixture，曾被 native codesign 與 selector 接受，因此不能拿未確認受簽章覆蓋的尾端／signature padding 當作 code tamper 證據。修正後修改 `__TEXT,__text` 的實際 section offset，先斷言 codesign 回 `invalid signature`，再驗證 selector 拒絕。任意 append 的可接受範圍未另行測試；此 preflight 不執行或使用尾端附加資料，也不提供完整檔案 digest 保證。未來若執行路徑會解讀此類資料，必須另行綁定 Core 信任的完整 artifact identity。

此里程碑不是正式授權 Route。下一步必須先**從 Next Core 所有的、可信任且不可由前端自由執行任意命令的配置取得完整的 wrapper/backend/server fingerprint allowlist**；接著定義明確 Desktop 人類操作／confirmation 与可信任 operator principal，經 M8 的 exact Allow/Ask/Deny 後才呼叫 Core-private Prepare/Attach，且需要跨 Core writer 排他、可報告的 Native session/stdio MCP lifecycle 和 same-boot orphan recovery policy。不得用只憑 localhost、任意 bearer 或客戶端自填 SHA digest 取代此授權。

仍未驗收：Connection 真實 GUI 拒絕後跨頁錯誤、Tray 真正 NEXT 身分／安全操作、已安裝 Native Auth/ACP Session→MCP→close/Host M9；不得 merge main、push、tag、發布。
