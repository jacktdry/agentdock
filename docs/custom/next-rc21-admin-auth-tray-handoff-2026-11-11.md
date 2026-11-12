# AgentDock NEXT RC21 — 管理者授權與 GUI／Tray 進度交接（2026-11-11）

## 結論、來源及安全邊界

使用者已要求先保存現有進度與文件，再處理反覆出現的 macOS 管理者授權視窗。本次僅作本機文件編輯與 Git Commit；**不啟動 GUI、不重新啟動 Core/Tunnel、不要求密碼或管理者授權、不修改 TCC/SIP、無 ACP、無 Push/MR/Tag/Release/Feed**。

- 已完成程式 Source：/Users/wei/sideProject/agentdock-next-h04-h02-r01-20261111，branch fix/next-rc20-h04-h02-r01-20261111，HEAD 0a0a0ba98412c6f22f8793a6abd184d6db62a27e（本次不改）。
- Docs 工作樹：/Users/wei/sideProject/agentdock-next-rc20-deploy-compat-20261111，branch fix/next-rc19-to-rc20-deploy-compat-20261111，先前 HEAD 8d7f0d9e。
- 前次已驗證 installed NEXT RC21：App Bundle 22.0.0，Core 127.0.0.1:8767/healthz HTTP 200，版本 0.9.2-rc.21，GUI PID 22247；Original Core 8765 HTTP 200，版本 1.1.1，未重啟。
- 26/26 個 installed RC21 檔案 SHA256 與候選包一致；實際 Atomic Stage RC20 回滾 Bundle 和獨立 RC20 Backup 26/26 一致、簽章 PASS。沒有執行真正的 RC21→RC20 回滾。
- Local ad-hoc candidate 不是正式發行：release_ready=false、update_feed=disabled/unpublished；Hosted CI、Developer ID、公證、公開 Release 均 OPEN。
- 本次未重新查詢線上 Core，以上執行狀態是前次已驗證的快照，勿誤寫為本次重新驗證。

## H02 後續 AX 與背景啟動證據

1. Source desktop/shared-poc/main.go 早已支援 -background flag 並將其設為 Wails WebviewWindowOptions.Hidden。前次 installed GUI 以 open -g -a 啟動時**沒有傳入 -background**；發生短暫前景活化，不能據此判斷 Wails flag 本身失效。
2. 在 Bundle ID 獨立的 h02uat App，以 open -g -j -n 加上 --args -background -runtime-root 啟動後，核實實際程序 argv 具有 -background，真實狀態列 AXTitle=NEXT，已清理唯一測試 PID。
3. NSWorkspace.didActivateApplicationNotification 事件監控期間 UAT_FOCUS_COUNT=0；**但監控另有 ACTIVATED=com.apple.SecurityAgent 和 ACTIVATED=com.openai.codex**。只能證明 UAT 本身沒有被觀察到前景活化，不能聲稱完全沒有任何授權提示，也不能將 SecurityAgent 事件歸因給特定命令。
4. Installed RC21 Tray 在**唯讀且未 AXPress** 的深度 AX 樹中顯示 AXMenu 下六個 AXMenuItem：核心服務：尚未檢查（disabled）、檢查核心服務狀態（enabled）、開啟 AgentDock Next（enabled）、結束介面（enabled），及兩個分隔。Tray 名稱 NEXT，Original 描述 AgentDock。**選單名稱／結構 PASS，實際選單點擊、Refresh/Open/Quit、瀏海視覺 OPEN**。狀態初始「尚未檢查」不是 Core 已故障的證據。
5. 先前 installed GUI 使用 open -g 仍曾搶焦點的事實保持 FAIL/OPEN。隔離 UAT 沒搶焦點不能取代 installed 整段生命週期驗收。

## P0 管理者授權彈窗（不得自行猜測根因）

使用者提供 macOS 管理者安全提示，內容為「要允許『agentdock』所啟動的工具指令取得管理者權限嗎？」並要求管理者名稱及密碼。這**確定是管理者授權請求**，不是一般 ChatGPT 工具許可或 Accessibility 結果。

**未確認：**是哪一個 agentdock 工具呼叫、子程序、命令、授權 API 或父子 PID 造成提示；SecurityAgent 活化與截圖所見是否同一事件沒有精確時間或呼叫關聯。不得直接宣稱是 open -g、AX、Swift/codesign 或 NEXT App 的錯。不得要求使用者反覆按允許、輸入密碼，亦不得停用 macOS 保護或繞過合法授權。

下次先用已存在的 source、命令歷史、非敏感唯讀診斷對照找出 exact invocation/PID/時間和提權需求；遇到同類彈窗即停止測試。若 P0 證明正常操作走到不必要的提權路徑，才修復最小 source/config 分支並做不提權的驗證。

## 未完成項目與執行順序

| 優先 | Milestone | 完成條件 | 狀態 |
| --- | --- | --- | --- |
| P0 | 管理者彈窗根因 | 唯讀歸因到實際命令與提權機制，辨別必要或多餘 | OPEN，優先 |
| P1 | 不必要提權修復 | 僅在 P0 證實後最小修改；保留合法 macOS 授權 | BLOCKED by P0 |
| P2 | 安裝版背景啟動 | 安全測試窗口用既有 -background，觀察完整啟動前景及彈窗 | isolated PARTIAL PASS；installed OPEN |
| P3 | H02 Tray 動作 | 已完成 AX 結構；在安全窗口驗 Refresh/Open，不無預警 Quit | 結構 PASS，互動 OPEN |
| P4 | H01 Connection | 合法拒絕操作後 Overview→Connection，確保錯誤保留 | OPEN |
| P5 | H04 高負載 Race | -race count10 空閒 PASS；CGO/Swift 並行時 1 秒 fixture timeout 待收斂 | OPEN |
| P6 | R01 Hosted CI/Release | NEXT-only Hosted CI、簽署、公證與發行審批逐關驗收 | OPEN |

## 既有證據與接手入口

- [NEXT RC21 installed Host、Core/Tunnel、SHA、rollback](next-rc21-installed-host-2026-11-11.md)
- [RC21 local Build、隔離 AX、Race 高負載反例](next-rc21-local-candidate-host-evidence-2026-11-11.md)
- [H01/H02/H04 原因與界線](next-rc20-h04-h01-h02-root-cause-2026-11-11.md)
- [R01 CI 與 Release Gate](next-rc20-stage4-crossplatform-release-readiness-2026-11-11.md)

**停止點：**先把這些文件 commit 到 Docs branch；現役 RC21、Original、Source SHA 不動。P0 尚未執行也尚未修復。
