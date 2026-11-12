# NEXT A1 — Staged Core Host ACP 驗收證據（2026-11-05）

狀態：**Staged Core Host ACP PASS；installed NEXT A1 UAT 尚未通過。** 本紀錄僅整理上游 orchestrator 提供的本輪 Mac-Dev Host 證據；沒有在此工作者回合重跑 Host 操作或程式測試。

A1 選案依據見 [Original v1.1.1 選擇性吸收評估](next-original-v111-selective-adoption-2026-11-04.md)；實作與 pipe 回歸見 [A1 source 交接](next-acp-write-bounded-2026-11-05.md)。

## 使用者流程目的

讓使用者能透過 NEXT Core 對真實 Codex ACP peer 建立 Session、在同一 Session 執行 Prompt 並取消另一個 Run，再安全關閉 Session。這是 A1 有界寫入／取消 source 修正的 staged Core runtime 驗收，推進 ACP 整合證據；它不涵蓋已安裝 App、Native Codex MCP operator→M8→backend、Connection GUI／Tray 或正式發行驗收。

最小驗收路徑是直接編譯本工作樹的 `cmd/agentdock` arm64，使用隔離臨時 state/workspace 和隨機本機 port，啟動實際 Core，透過真實 MCP／ACP RPC 完成建立、唯讀更新、Prompt 終結、同 Session 第二個 Run 取消及關閉。成功標準為上述 RPC/terminal 狀態符合預期、Core 正常結束，且臨時服務清理後不影響 Original 與已安裝 NEXT。

## Host 身分與 staged binary

- NEXT branch：`feature/next-edge-rc-20261023`；HEAD：`4af20cdb`；A1 **尚未提交**。
- 現役安裝版仍為 NEXT `0.9.2-rc.19`、App bundle ordinal `20.0.0`；Original 為 `1.1.1`。
- 在 repo-local `dist/next-a1-host.9EyQgX/` 編譯 `cmd/agentdock` arm64。SHA-256：`0c147c6009c94fe4aaea0668a6c2b21794c6521279fb9e2bd90362e6dc900529`；`BuildDate=2026-11-05-staged-only`、`Commit=worktree-A1-uncommitted`。其 version CLI 顯示 `0.9.1` 與 commit 前 12 字，這是 staged Core binary，**不是完整 App，也不是 installed RC**。
- 正式 NEXT signed RC candidate 必須來自 clean immutable HEAD，並符合 canonical manifest 與 `manual_version_gate`。本輪 dirty source 不可冒充 RC20；沒有執行更新或安裝。

## Staged Core Host 實際流程與結果

使用 0700 權限的私有臨時 state 和 workspace，設定 `AGENTDOCK_DESKTOP_VARIANT=next`、`browser=false`、僅 Codex ACP profile，以獨立 `127.0.0.1` 隨機 port 和 Bearer token 啟動 staged Core。沒有啟動 Edge/Chrome，也沒有操作現役服務。

| 實際步驟 | Host 結果 |
| --- | --- |
| staged Core `/healthz` | HTTP 200 |
| `/context` 與 MCP initialize | ACP enabled；MCP initialize HTTP 200 |
| MCP `tools/list` | 25 tools，包含 `acp_session`、`acp_prompt`、`acp_interaction` |
| 真實 Codex ACP `info` | HTTP 200 |
| Session 建立與唯讀更新 | `new` HTTP 200 → `update` HTTP 200 |
| 第一個 Prompt | start=`running` → events terminal=`completed/end_turn` |
| 同一 Session 第二個 Run | start=`running` → `cancel` HTTP 200、`cancel_requested=true` → events terminal=`cancelled/cancelled` |
| Session 關閉與刪除 | `close` HTTP 200 → `delete` HTTP 200 |
| staged Core 結束 | 正常 exit 0 |

沒有確認模型輸出的文字內容逐字符合任何預期；本 PASS 只依上述真實 live RPC 與 terminal 狀態判定。

## 清理與不中斷證據

- 曾使用四個臨時 port：`61695`、`61714`、`61739`、`61933`。驗收後各 port 均 closed，沒有 staging binary 程序殘留。
- Original `8765` 與 NEXT `8767` 在前後檢查均為 HTTP 200；installed NEXT bundle ordinal 維持 `20.0.0`。
- 現役 NEXT GUI 僅做 observe-only：`computer_session capability=observe foreground=forbidden`。`list_windows` 對 `dev.dropabit.agentdock.next` 回報 App running、PID `94177`，但 `windows:[]`。前景仍是 Chrome PID `31458`，前後不變（`focus_changed=false`）；lease release cleanup complete。
- 沒有點擊 Tray、操作 Connection、清理任何既存 Session，或改動 Original/NEXT 現役服務。曾有一次現役 NEXT `acp_session list` 回覆 `Authentication required`／`ACP_REMOTE_ERROR`，並帶有既有 Codex sessions internal error；這只作單次診斷記錄，不能推論所有使用者都無法使用，也未證明與 A1 有直接關係。

## 限制與仍開啟的驗收

本輪只證明以 staged binary 啟動之 Core 的真實 Codex ACP RPC 流程。它沒有安裝、替換或更新現役 NEXT，也沒有證明安裝版包含 A1。故明確結論是：**Staged Core Host ACP PASS；installed NEXT A1 UAT 尚未通過。**

下列項目仍 OPEN：

- Native Codex MCP operator → M8 → allowlisted signed backend 的安裝版 UAT。
- Connection GUI reject/error 跨 tab 持續顯示，以及 Tray Accessibility name/menu 的實際驗收。
- Windows/Linux runtime parity，以及 M9/release gates。
- 現役 NEXT ACP list 的上述 Authentication required 診斷，尚未定位根因。

Source-only focused race `-count=10`、完整 suite、`go vet` 與 Windows/Linux cross-build 已記錄於 [A1 source 實作與回歸交接](next-acp-write-bounded-2026-11-05.md)；它們不是本輪新跑的測試，也不與本 staged Host RPC PASS 混算。

## 下一個合法的 installed upgrade gate

先由上游審閱並整合 A1，形成乾淨且不可變的候選 HEAD；依正式 NEXT 流程產生 signed RC candidate，通過 canonical manifest 和 `manual_version_gate`。只有該 gate 具備可審核證據後，才可另行進行授權的 NEXT-only 安裝／更新與 installed A1 UAT。此文件回合沒有執行上述 gate、安裝、更新、簽署或發行。
