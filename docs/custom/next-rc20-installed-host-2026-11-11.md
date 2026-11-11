# NEXT RC20 本機安裝與 Host 驗收記錄（2026-11-11，Asia/Taipei）

## 使用者工作與結論

使用者授權釋放三個既有 NEXT Computer Broker 直接控制 Session，並接續 RC20 A1 正式安裝驗收。三個指定 Session 均回報 `computer_ok=true`、`cleanup_state=complete`；隨後 Broker `active_sessions=null`。這是用戶授權的清理，不代表可自動清除以後的其他 Session。

**RC20 現已安裝且 Core／Tunnel 健康，安裝包內容與原簽章候選完全一致；正式 installed Codex ACP completed → cancelled → recovered 三階段驗收尚未通過，因 Codex provider 額度限制。GUI 無焦點驗收亦未通過。**

## RC19 → RC20 安裝相容性修正

- 清理完 Session 後，第一次 `packaging/macos/deploy-next-shared.py` 在換版前回報 `Missing native MCP catalog`，安裝仍為 RC19，Original `8765`／NEXT `8767` 皆健康。
- 以獨立 worktree `/Users/wei/sideProject/agentdock-next-rc20-deploy-compat-20261111` 修正：舊 RC19 簽章 Shared App 尚無 `Resources/native-mcp-servers.json` 和 `Helpers/agentdock-next-mcp-launch`；僅對**明確指定且 `CFBundleVersion=20.0.0`、ShortVersion `0.9.2` 的舊版安裝／備份**允許這兩個既有缺項。來源 RC20 仍必須通過完整 Catalog／launcher、Mach-O、權限、身份及嚴格簽章驗證；存在但無效的 catalog 絕不可跳過。
- 相容修正提交：`55fcd06adc09ddd6142f67714de9d599ca53b82f`；回滾後允許驗證合法 RC19 的補充提交：`30a1bb87`。二者僅是**本機 Git 提交**，沒有推送、合併、建立 Tag 或正式發行。
- `python3 scripts/test/test-next-shared-package.py` **20/20 PASS**；真實已簽章 RC19 僅在明確 legacy opt-in 下通過 `validate_bundle(shared=True)`，RC20 來源維持 default strict PASS，唯讀 `manual_version_gate` PASS。負向測試的 sealed-resource-invalid stderr 為預期。

## 現役安裝身分、檔案與回滾證據

- 2026-11-11 15:53 檢查時，現役 `~/Applications/AgentDock Next.app` 已是 **Core `0.9.2-rc.20`、source commit `704bb40d08f6`、Bundle `21.0.0`**。另一次由本輪啟動的部署重試因「目標不再是 RC19」而在前置版本檢查停止，**沒有重複安裝**；沒有將該次重試描述為成功執行 deploy。
- 原始簽章候選位於 `/Users/wei/sideProject/agentdock-next-a1-rc20-20261105/dist/next-shared-arm64.5w7KBj/package/AgentDock Next.app`；已裝 App `codesign --verify --deep --strict` PASS；**兩邊 26 個 regular files 的相對路徑與 SHA-256 全部一致，沒有缺失或額外檔案**。原 A1 RC20 candidate branch HEAD `704bb40d` 仍 clean。
- 新產生回滾位置：`/Users/wei/Applications/.agentdock-next-replacement-iwp9x6vw/AgentDock Next.app`，簽章 PASS，Core `0.9.2-rc.19`／Bundle `20.0.0`，source commit `138c6cb0f123`。本輪**未執行實際回滾**。
- 安裝後 NEXT Core 監聽 `127.0.0.1:8767` HTTP 200，PID 曾觀察為 `11613`；NEXT Tunnel launchd 有 PID `12375`；Original stable custom v1.1.1 監聽 `127.0.0.1:8765` HTTP 200，PID `38464` 未被本輪操作。新版 GUI `~/Applications/AgentDock Next.app/Contents/MacOS/AgentDock` 曾觀察 PID `14144`；以上 PID 僅為當次快照，**不是未來操作的固定識別碼**。

## 真實 installed ACP／GUI 證據界線

- NEXT installed Core 提供的 `acp_session new` 實際建立 Codex Session `acps_037abfeea86a1c5dae6e5e78`，成功更新 read-only／gpt-6-luna／low。第一個 Prompt start → running，但 events 回 `ACP_REMOTE_ERROR` 和 Codex Usage Limit，提示約 2026-11-11 **19:58 台灣時間**重置。此為 provider quota，不算 A1 取消後恢復 PASS；不重試。這個由本輪建立的 Codex Session 已 `close` 與 `delete`。
- 另一個以 Gemini 3.8 Flash low 啟動的 NEXT Antigravity Session `acps_8cc6742864dd5059d2a0d80e` 可以建立和啟動 Run，但未遵守「不使用工具」測試指示，而執行 `ls -la` 及讀取本機 Skill。為避免超出測試界線立即 `acp_prompt cancel`，最後 events `cancelled/cancelled`；Session 已 `close`、`delete`。因此只能聲稱 **installed NEXT ACP 真實 cancellation 路徑 PASS**，不能聲稱 Gemini 正常完成、恢復或無工具行為已驗證。
- 曾取得一個只觀察的 Computer Session（foreground forbidden），`computer_observe get_app_state` 回 `COMPUTER_FOCUS_VIOLATION`。該本輪 Session 已 release cleanup complete；**不可宣稱 GUI zero-focus UAT PASS**，不再以相同方式重試。
- 2026-11-05 曾以簽章 RC20 **隔離測試 Core** 完成真實 Codex ACP Session normal completion／cancel／same-session recovery（見 `next-a1-rc20-candidate-2026-11-05.md` 於原工作樹），但那是 signed package **isolated Core Host**，**不是本次 installed RC20 Core UAT**。
- 15:59 唯讀 Broker 快照出現另一個未由本輪建立的 Antigravity managed Session `acps_3016b924f77b553b344873e1` 和 Browser Broker owner；未對該 Session 做 close／delete／cleanup。任何後續版本操作必須重新核對現役 owner，不可把舊閒置快照當成永久許可。

## 未完成與下一步

1. Codex 配額重置後，重新讀取真實 installed NEXT Core 身分與 ACP 狀態，再以**新建、任務自有**的唯讀 Codex Session 驗證正常 Prompt `completed/end_turn` → 第二個 Prompt `cancelled/cancelled` → **同一 Session 恢復 `completed/end_turn`**，再 close／delete；別用 Gemini 的已取消結果冒充 Codex PASS。
2. GUI 可及性與無焦點背景驗收需另外處理 `COMPUTER_FOCUS_VIOLATION`；不要繞過 macOS consent/TCC 或搶焦點。其他 Native Codex MCP trusted operator→M8→backend、Connection GUI rejected mutation 跨頁、Tray AX/menu、Windows runtime、hosted CI/notarization／正式 release 等門檻仍 OPEN。
3. 此 RC20 為 **本機 ad-hoc 簽章候選**，canonical manifest `release_ready=false`、update feed unpublished。後續若修改 signed App 必須重新建置與正式驗證，不能直接改動已簽章 Contents。
