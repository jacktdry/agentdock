# NEXT Native Codex MCP — Core-owned server ID dispatch（2026-10-25）

## 從真實操作流程看需要補什麼

Next Shared arm64 已可封裝 signed launcher，並在 `Contents/Resources/native-mcp-servers.json` 記錄 Core-owned backend allowlist；但 B6 私有 `prepareNextCodexMCPRegistry` 與 `attachNextCodexInstrumentedMCP` 接受一個完整 `nextCodexMCPConfig`。M8 的 config digest 僅保證當次輸入未改，不能證明 backend 是 NEXT 自己允許的。正式 Desktop operator 未來**只能提出 server ID**，不能提交程式路徑、args 或 digest。

本輪 source-only 引入 private `prepareNextNativeMCPRegistry`／`attachNextNativeMCP`。兩者只以 ID 呼叫已存在的 signed Next catalog resolver，先驗證 ID、wrapper/backend/args/digest，然後重用現有真正 M8 admission 與 B6 Core-owned crash fence/Session attach；同時保留 B6 先前的來源專用測試方法但沒有新增 HTTP、Desktop IPC、MCP、CLI 或外部啟動入口。

Preparation 在 M8 放行後、取得 lifecycle mutex 前重新 resolve signed catalog；成功時在 mutex 內 pin Core-owned 完整設定。Attach 在 M8 前比對 Core pin、M8 後再 resolve 比對完整設定，且 Core 私有 B6 raw attach 不可接續已有 ID-only 的準備作業。Runtime.Close 會清除 pin，但不擅自清除原本 native bootstrap crash journal；如果 ACP teardown 失敗，原有保留 reservation 的 fail-closed 規則仍適用。

現有正式 NEXT Shared catalog 為 `{"schema_version":1,"servers":[]}`，因此沒有任何可準備的 native server；本輪預設一定拒絕，不新增假 MCP backend，不借用已存在的 agentdock-browser/computer HTTP MCP 服務重複建立子程序。實際第一方 Browser/Computer MCP 已使用 Core per-session Token/lease 生命週期，不需為了測試再建一個無用途 stdio bridge。

## 風險邊界與待辦

- 這仍是 Core-only **source 路徑**，不表示可信任 Desktop 操作者、M8 人類確認、已安裝 Next 或真正 Native Codex 模型工具呼叫通過。
- 檢驗 Core-signed Bundle 仍是 path-based snapshot：在 verify→lock／native exec 之間可能由同 UID 更動檔案，尚無原子 process/image pin；不把這輪的重複驗證宣稱為能抵擋 same-UID hostile mutation。正式 release 必須在受信任 operator 原則下定義不可被替換的本機執行物身分與 recovery。
- 尚待封裝真正有產品用途的 backend、被 Core 驗證的 Desktop operator 主體及明確確認/授權、M8 Ask→Allow/Denied 後的 Host lifecycle，以及獨立未結 native journal/orphan 協調。
- installed NEXT RC19、Connection UI 拒絕後跨頁顯示與 Tray 的真正 AX 身分/操作 UAT 仍 OPEN。這次 macOS 實測 Chrome 是前景，故不搶前景或盲點沒有可驗證名稱的 NEXT Tray item；Stable 與 NEXT 服務保持原狀。

## 驗收方法

`TestNextNativeMCPIDOnlyRejectsBeforeAnyM8OrNativeStartup` 驗證各種不可信 resolver／未知 ID／catalog 缺席皆在 M8 前拒絕，沒有 Adapter 啟動、journal 或 pin；`TestNextNativeMCPIDOnlyStillRequiresM8Authorization` 驗證即使 Core 正確選到設定，也依既有 ReadOnly／Rules 返回拒絕或待確認，不可因 server ID 繞過。完成真正 M8 一次核准後若 catalog 再改，`TestNextNativeMCPIDOnlyRechecksCatalogAfterM8Approval` 驗證不開始 startup，且 grant 以失敗結果結案。完整測試與 race/vet 結果以實際輸出為準。

**本輪實際驗證：**
- Source-only ID-only focused 三組測試 PASS（空 catalog、不合法 ID、nil/變動 resolver、正確 ID 但 M8 Ask/Deny、一次核准後 catalog 變動導致已消耗 grant DispatchOutcome=Failed，均無 native lock/adapter/journal）。
- Orchestrator 獨立 `go test -count=1 ./internal/app ./internal/acp ./internal/permission ./internal/desktopapi ./internal/desktopruntime ./internal/mcp/launchregistry ./internal/tool/acp` 全數 PASS；`go vet` 同七套件 PASS；`go test -race -count=1 -run '^(TestNextNativeMCPIDOnly|TestNextCodexMCP)'` PASS。
- Linux amd64／Windows amd64 `CGO_ENABLED=0 go build ./internal/app ./internal/desktopapi ./cmd/agentdock` 均 PASS；`git diff --check` PASS。
- 兩個現有 Core 服務仍健康、原生狀態列未被無法證實身分的座標點擊；當時 Chrome 為前景，不觸發 NEXT UI focus，故 Connection/Tray 的 Host gate 仍 OPEN。

**不安裝、不編譯新 RC、不 merge/push/tag、不發佈。** 原始碼變更僅限 NEXT 私有路徑及必要的關聯測試。
