# NEXT Native Codex MCP：正式套件內的已簽章 launcher（2026-10-25）

## 實際問題與產物

前次 NEXT ACP UI 可唯讀顯示 Native MCP 啟用前狀態，source-only B0–B6 也已有獨立 wrapper，但 Shared NEXT 正式 Builder 過去**沒有隨 App Bundle 帶入** `cmd/agentdock-next-mcp-launch`。即使新增操作者的 M8 授權，安裝版也找不到從自己的簽章 Bundle 選取的 launcher，無法可靠建置 Core-owned 執行檔允許清單。

本次在 `packaging/macos/build-next-shared.sh` 編譯確切 arm64、CGO-disabled 的 `./cmd/agentdock-next-mcp-launch`（不使用使用者已安裝的 Next/Stable binary），再讓 `packaging/macos/build-app.sh` 僅在 NEXT Shared arm64 路徑複製到 `Contents/Helpers/agentdock-next-mcp-launch`，以 `dev.dropabit.agentdock.next.mcp-launch` 的獨立 Helper 識別簽章，最後簽封 App Bundle。這是 launch 前的**封裝必要條件**，不是啟動或授權功能。

`packaging/macos/next_shared_bundle.py` 必須對 Shared NEXT 找到正確位置、可執行 regular file、arm64-only Mach-O、正確簽章識別、有效簽章與 Bundle 封印；以 Go build metadata 驗證命令確實來自 `github.com/uvwt/agentdock/cmd/agentdock-next-mcp-launch` 且為 Darwin arm64。Installer 共用驗證器，不會接受與舊 Stable 混淆的 helper。非 Shared 的舊版 NEXT fixture 仍保留原本驗證契約；Stable Builder 不帶入新 launcher。沒有新增 launchd 工作、App UI、Core listener、M8 mutation route 或任何 auto-enablement。

## 可重現的驗證與驗收範圍

- `python3 scripts/test/test-next-shared-package.py`：原本的套件／安裝安全檢查加上 launcher 缺檔、軟連結、不可執行、錯誤 identity/architecture/build source、修改後不正確簽章及外層 seal；**15 項通過**，其中 macOS 採實際 ad-hoc `codesign` 與 arm64/x86_64 stub 執行檔，Go metadata 部分以 fixture 驗證字串，**不是完整真實產品封包**。
- Orchestrator 獨立複驗 Shell 語法、Python fixture 15 PASS、`go test ./cmd/agentdock-next-mcp-launch`、`go vet ./cmd/agentdock-next-mcp-launch`、`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build` 的確為 arm64 Mach-O 且 Go module path 指向 launcher；`git diff --check` PASS。
- **尚未執行完整 NEXT arm64 正式打包／未安裝／未驗收真實已簽章 package 或原生 Auth Session**。不讀取／複製登入憑證、不更動已安裝的 RC19/Stable、Core 8765/8767 或 Keychain；本次僅本地原始碼 checkpoint，不 Push、Merge、Tag 或公開發布。

## 下一個離正式啟用最近的工作

由 NEXT Core 從自己已簽章 Bundle 中**固定選取上述 launcher**，而不是接受任意前端路徑，並建立 Core-owned backend executable/argv allowlist 和組態雜湊。接著才實作獨立可信任的 Desktop operator 操作／顯式確認及 M8 Allow/Ask/Deny；之後安裝已驗證候選，在使用者明確許可的 Next profile 做 model → stdio MCP tool → Session close。same-boot 未結 journal／native orphan 未達復原條件時仍須 fail-closed，不能自動刪除、重啟或 PID kill。

既有 Connection 真實 GUI 失敗跨頁和 Tray 身分／安全操作 Host 門檻仍 OPEN。參見 [Desktop readiness](next-native-mcp-desktop-readiness-2026-10-25.md) 及 [B0–B6](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。
