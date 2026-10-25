# NEXT Native Codex MCP：Core 簽章 Launcher Preflight（2026-10-25）

## 使用者需要的能力與這輪增量

前一版 `1326c6bb` 為 NEXT Shared macOS arm64 套件新增已簽章 `Contents/Helpers/agentdock-next-mcp-launch`，但 Core 仍沒有辦法自行區分：正在執行的 Next App 是否真有該 launcher、檔案是否為 Next 的正確產物。ACP 頁面既有 `operator_setup_required` 容易把尚未安裝的 RC19 或 source runner 表示得過度接近可啟用。

這輪新增只讀 Core-local `selectNextCodexSignedLauncher()`。唯一 production 輸入是 `os.Executable()`（目前 Core 的真實程式路徑），必須**正好是** `AgentDock Next.app/Contents/Helpers/agentdock`；launcher 必須是該 Bundle 的固定 sibling `Contents/Helpers/agentdock-next-mcp-launch`。不接受 Desktop/HTTP/MCP/環境變數中的 launcher 路徑或 caller 自填 digest。檢查 Next Info.plist 識別、原始檔與所有祖先不可 symlink、正確 owner/不可被群組或其他人寫入、沒有 hardlink、launcher arm64 Mach-O 執行檔、Go 內建 buildinfo 正確 command path/GOOS/GOARCH、Core/launcher 的 macOS `codesign --verify --strict` 精確 identifier，以及 App Bundle 的 strict recursive seal。驗證前後重新 Lstat 比較，不將任何 raw path/error/credential 傳往 Desktop。這是一次性的唯讀 snapshot，**未賦予後續 exec 權限**；真正啟動時仍必須重新驗證及取得可信 Core-owned allowlist/M8 授權。

只有在 source-only NEXT 權限 owner 存在、既有 native readiness 為 `operator_setup_required` 時才做 bounded Core preflight（外部 codesign/plutil 在 lifecycle mutex **之外**進行，之後再確認 runtime 仍有效）。不符合條件回覆 `launcher_unavailable`，ACP 頁面以 en、zh-Hans、zh-Hant 顯示明確缺少可信套件。若 preflight 通過，仍顯示 `operator_setup_required`，文案強調操作者的可信動作與 backend argv 允許清單尚未完成；**沒有啟動按鈕，不建立/清除 journal，不啟動 Native Codex 或 MCP，Stable 的行為不變**。

## Source-only 驗證證據

- 原始碼 `internal/app/runtime_next_codex_launcher_darwin_test.go` 組裝純臨時 App Bundle，使用真實 Go 編譯出的 `cmd/agentdock-next-mcp-launch`、`codesign` 與 `plutil`。正向測試確認 arm64、Go 身分與精確簽章成功；負向測試驗證未安裝 Source Runner、路徑／文件缺失、rename、symlink、hardlink、祖先 symlink、可寫／不可執行、錯誤 Core/launcher identity、Bundle ID/variant、失效 App 外層 seal、已取消 context。
- 最初測試僅竄改 Mach-O 檔案尾端，該 byte 未必屬 codesign 覆蓋範圍，故意外通過；修復後精確修改 `__TEXT,__text` 受簽章保護的程式碼，並先要求 macOS 原生 `codesign --verify` 證實 `invalid signature`，再驗證 Core selector 拒絕，沒有降低正式驗證條件。
- Orchestrator 獨立 PASS：focused macOS Core selector test（包含正向／負向）、Go app/desktopruntime/desktopapi/acp/permission/launchregistry 測試、frontend 184 Vitest、TypeScript、i18n-check、production Vite build；其餘 race/vet 結果以實際終端驗證為準。若僅驗證 source fixture，不代表已安裝 host UAT 或 full package release PASS。
- 未讀取實際登入憑證、未執行 launcher、未重啟/安裝 NEXT RC19 或 Stable 1.1.0、未操作任何 Edge profile/system dialog；只在專用 worktree 改 source、測試及文件。

## 後續真正的 Release Gate

1. Core-owned **Backend Executable + argv** allowlist：來源需來自已驗證的 Next 專用可信組態與封裝，而非 UI 任意命令、shell/script 或客戶端填的 SHA-256；與此輪選定的 launcher、M8 request fingerprint 一起 immutable bind。此輪只選 launcher，不允許任何 backend。
2. 獨立可信 Desktop operator evidence、明確使用者確認、M8 Allow/Ask/Deny → Core 私有 Prepare/Attach，Session 擁有／後端清理與 same-boot orphan/journal fail-closed；不能拿 localhost／普通 bearer／尚未生效的簽章 preflight 充當授權。
3. 完整 NEXT Shared arm64 產品套件實際簽章驗證、授權的 installed Host Native 模型工具呼叫/Session close；**Connection GUI 失敗跨頁與 Tray 原生 status item 身分/安全點擊 Host Gate 仍 OPEN**。

承接 [Signed Launcher Package](next-native-mcp-signed-launcher-package-2026-10-25.md)、[Desktop Readiness](next-native-mcp-desktop-readiness-2026-10-25.md) 與 [M8 policy dispatch fence](next-native-mcp-policy-dispatch-fence-2026-10-25.md)。**不 merge、不 push、不 tag、不發佈**。
