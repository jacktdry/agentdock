# NEXT Native Codex MCP：Core-owned backend catalog（2026-10-25）

## 工作流程與修正範圍

原本 Core-private B6 `prepareNextCodexMCPRegistry`／`attachNextCodexInstrumentedMCP` 可以接收調用者傳入的 backend executable 與 argv；雖由 M8 檢查 `ConfigDigest`，**呼叫者能自行計算 digest 不代表執行來源或組態已獲 Core 認可**。NEXT Shared signed launcher 從前一里程碑已可做唯讀身分驗證，但仍缺少由 NEXT Core 自己選定的 MCP backend 路徑。

這輪新增獨立唯讀候選解析 `resolveNextNativeMCPServer(serverID)`：Core 唯一接受 server ID，依 `os.Executable()` 找到自己的已封印 NEXT Shared App `Contents/Resources/native-mcp-servers.json`。JSON v1 要求精確欄位、上限 64 KiB、最多 32 server；禁止重複鍵／ID、未知欄位、錯誤 UTF-8/Unicode、動態環境變數、shell 或 interpreter ID，以及非固定 `Helpers/native-mcp/<id>` 的 backend 路徑。argv 僅來自封印的 catalog，長度受限，不做 shell 展開。從同 Bundle 的固定 signed wrapper 及 backend 的 arm64 Mach-O、權限／hardlink／symlink、Next-specific code-signing identifier、外層封印核對後，Core 自行計算 `nextCodexMCPConfigDigest`。所有失敗只回 `errNextNativeMCPUnavailable`，不暴露本機命令/路徑。

`packaging/macos/build-app.sh` 對 NEXT Shared **固定產生空清單** `{"schema_version":1,"servers":[]}` 並納入 App seal，無環境變數、自訂輸入或既有使用者 MCP 自動加入；`packaging/macos/next_shared_bundle.py` 在驗證新 Shared 套件時檢查 schema、catalog 安全屬性及其指定的 backend 架構與簽章；installer 重用此驗證。舊 Next non-Shared 和 Stable 打包流程不變。

**重要：目前正式 Shared NEXT 封裝的 server 清單為空，沒有可用的 Native MCP backend，沒有 production UI route、沒有自動啟動、沒有把 Native B6 私有 Prepare/Attach 接入解析器。** 這個 source-only resolver 是未來可信 Desktop operator 的必要素材；無法單憑此功能執行工具，也不能將 ad-hoc code-sign identifier 視為 Developer ID publisher 或人類許可。

## 可重現的驗證與限制

- Go `TestNextNativeMCPCatalogStrictSchema` / `TestNextNativeMCPServerSignedBundle`：解析器的非法／重複／任意 command、空 catalog、unknown ID，臨時獨立 signed NEXT App Bundle 的真實 arm64 executable 選取、惡意腳本、symlink/hardlink、外層資源竄改、錯誤 backend 身分、修改參數後重新選取 digest 等正反例；都是 source-only fixture，不啟動 server。
- Python `scripts/test/test-next-shared-package.py` 納入 catalog schema／封印拒絕，19 項測試 PASS；包含預期的 invalid outer seal stderr，不代表驗證器失敗。
- Orchestrator 獨立 `go test -count=1 ./internal/app ./internal/desktopruntime ./internal/desktopapi ./internal/mcp/launchregistry`、定點 catalog、`go vet`、`git diff --check`、`zsh -n` 與 macOS package fixture PASS；若後續 full signed package 建置/驗證成功，可另補具體路徑與證據。沒有進行 NEXT 安裝／Service 重新註冊／Keychain/TCC/Edge/舊 Stable 操作。

## 剩餘正式 Gate

1. 定義真正受支持的 NEXT MCP backend 產品與封裝來源，以受控建置產生固定的 signed `Helpers/native-mcp/<id>` 以及對應不可修改的 catalog。這版**沒有 backend**；動態自訂 npm/uvx/MCP 安裝、任意使用者 script 都未開放。
2. Core-trusted Desktop operator identity、明確批准與人類確認、在 M8 admission 前重新由 Core resolve 並 immutable pin wrapper+backend/argv/digest，之後再接 Core-private Prepare/Attach；避免要求過程被替換。未結 journal/orphan 仍 fail-closed。
3. 真實 NEXT Shared product full-package 簽章與安裝 Host→登入 ACP Session→model Native tool→Session Close、Connection GUI 失敗跨頁與 Tray 身分／選單 Host 仍 OPEN。無公告發佈、Merge/Push/Tag；保持 RC19/Stable 不受干擾。

承接 [Core Signed Launcher Preflight](next-native-mcp-core-signed-launcher-preflight-2026-10-25.md) 及 [Signed Launcher Package](next-native-mcp-signed-launcher-package-2026-10-25.md)。
