# NEXT Native MCP：Desktop Unix Socket 來源驗證（2026-10-26）

## 為什麼要做

NEXT Desktop 原本透過 `desktopcontrol.CallVerifiedPID` 驗證接收端確實是目前的 NEXT Core，但 **Core 收到 `control.sock` 請求時，沒有反向確認發送者是 NEXT GUI**。同 UID 程序理論上可連接受 `0600` 權限保護的 Socket。這在未來對 Native MCP 提供 M8 人類操作入口之前，是一個需要明確區分「本機 IPC 可連線」與「具有 Desktop 來源證據」的真實授權邊界。未授權來源不可因送出 JSON 字段就成為操作人員。

## 本次最小改動

- `desktopcontrol.Serve` 透傳 **連線專屬 context** 到既有 Handler；Darwin+cgo 在接受 Unix 連線後、處理任何使用者 bytes 前，以 `LOCAL_PEERPID` + `LOCAL_PEERCRED` 擷取 kernel PID/UID，讀 `libproc` 中 executable path 與完整 process birth timestamp。使用 package-private context key，沒有 exported injector，JSON／環境變數／自行建立的 context 不可偽造證據；非 Darwin 或無 cgo 保守返回未驗證。
- `desktopcontrol.VerifiedNextSender(ctx)` 讀取 Core 自己的 `os.Executable()`，必須是 `AgentDock Next.app/Contents/Helpers/agentdock`；連線程序必須是**同一個 Bundle** `Contents/MacOS/AgentDock`，PID/UID/full birth 與 accept 時快照相同。檢查 bundle Info.plist `CFBundleIdentifier`、`AgentDockVariant`、`CFBundleExecutable`、NEXT Core + GUI 的精確 ad-hoc signature identifier 與整個 App 的 `codesign --verify --deep --strict`，只接受安全 regular file／dir、無 symlink/hardlink、owner/private mode，簽章前後重新觀察本機程序及檔案。
- 新增 Native Core IPC **唯讀**方法 `next_codex.operator_identity`；僅回傳固定的 `disabled`、`core_closing`、`owner_unavailable`、`desktop_peer_unverified` 或 `desktop_peer_verified`。必須使用 source-only Next permission owner；處理簽章時不持有 lifecycle mutex，結束後再次確認 Core 沒有 Closing。**回覆不含 PID、path、birth、UID、token 或 exec args**。
- 原本 `permission.bootstrap`、M8 變更 API、Browser snapshot、Readiness、Stable、Windows/Linux IPC 語意不變；**沒有新增任何 Native MCP 啟動／M8 mutation route**，也不會將已驗證 Socket peer 身分直接視為已核准的 operator。

## 實際驗收

- 使用本機 Mac-Dev Host 執行 `go test -count=1 ./internal/desktopcontrol ./internal/app ./cmd/agentdock` **PASS**；其中包含既有真正 Unix roundtrip 與 verified PID 測試，不只 mock。新的測試包含接受時的 kernel peer PID/UID/birth 與 handler context，變更 PID／UID／full birth／執行路徑拒絕、偽造 JSON/context 拒絕；用短命的測試專用 `/bin/sleep` 複製到臨時簽章 NEXT Bundle 驗證正向、錯誤 GUI 簽章、破壞 outer seal、symlink、parent writable。該測試子程序由單元測試明確建立與回收，不是已安裝 NEXT 或 MCP backend。
- `go vet` Core／Desktop 相關套件與 `go test -race` IPC 重點測試 PASS；Darwin 無 cgo、Linux amd64、Windows amd64 CGO0 核心套件交叉建置 PASS。Source-only 另測當 Core 不能驗證真正的 kernel Next GUI 時，不會回 `desktop_peer_verified`；Core Close 只顯示 `core_closing`，沒有敏感回覆。
- Codex ACP 隔離環境禁止測試 Unix bind，連既有 IPC 測試也回 operation not permitted；改由有正常本機 Socket 能力的 Mac-Dev Host 獨立複驗。曾遇 `x/sys/unix.XUCRED_VERSION` 不存在的編譯問題，已移除不必要的常數比較並重新跑測試。
- 此輪**沒有**安裝或啟動新 NEXT、沒有升級服務、Keychain、TCC/Edge、focus、Tray、正式 MCP backend、Credential 或 PID 清理；已安裝 NEXT RC19／Stable 仍為各自獨立運作版本。沒有發布、Push、Merge 或 Tag。

## Release Gate 仍未通過

1. **`desktop_peer_verified` 只代表目前有 kernel peer + 這份 ad-hoc Bundle 的程序來源證據**，不等於已同意的真人、Developer ID／公證、受信任軟體供應者。相同 UID 可控制自己擁有的 Bundle、ad-hoc identity，可複製／替換程式與簽章；未建立不可替換的 image pin 或 sandbox-hardening trust root。正式 mutation 仍需要額外可被 Core 驗證的操作者意圖／確認／授權，不能直接重用唯讀結果當 M8 principal。
2. NEXT Shared signed catalog 仍**預設空**；不直接提供動態任意 binary/argv，也沒有 M8 Native start route、已安裝授權 Session→model tool→close 的 Host PASS。
3. Connection GUI 真實錯誤跨頁與 Tray 無障礙項目身分／實際安全選單操作仍 **OPEN**。首次來源驗證在原始碼與測試上通過，不宣稱 RC19 GUI 已支援本次新增的 IPC 方法。

承接 [Core-owned ID dispatch](next-native-mcp-core-id-dispatch-2026-10-25.md) 及 [Desktop readiness](next-native-mcp-desktop-readiness-2026-10-25.md)。
