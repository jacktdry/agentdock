# Codex ACP MCP 生命週期：協作式回報 + 可驗證啟動登記（2026-10-11 決策）

**決議：不為 AgentDock Next 的 P0 要求 Fork 或自行編譯官方 Codex CLI。** Codex CLI 自身的 stdio MCP leak、native Thread unload／shutdown 缺陷由官方修正；AgentDock 的工作只涵蓋透過自己管理的 **Codex ACP Adapter** 所啟動的 app-server 與 MCP。既有 native Rust source-only 查核保留研究紀錄，不再是本路線的阻塞交付。

**需求：**透過 ACP Prompt 提示 Codex 本次任務結束時盤點、釋放「確實由本次任務建立、且可合法關閉」的資源，回報給上層 ChatGPT。當需要早於 prompt 的實際程序身分時，採 **Next-owned stdio MCP launch wrapper + Core 登記**，不要求 Codex 在 `exec` 前猜 PID。已先交付 **Next-only source-only Prompt 規範 opt-in**（需 `AGENTDOCK_NEXT_CODEX_MCP_REPORT=source-only`）；**沒有建立或安裝 wrapper、沒有部署該新功能、沒有任何 live 程序自動清理**。

## A. 避免混淆的三種「所有權」

1. **工具使用記錄（advisory）**：Codex LLM 自述「本輪使用 GitHub／DevTools／CBM MCP」或 `mcpServerStatus/list` 回報 server 名稱和狀態。只證明使用或配置，不證明原生 child PID、stdio transport，不能作為關閉權限。
2. **Adapter / app-server owner（驗證後）**：AgentDock ACP Manager 自己 `exec.Cmd.Start` 的 Adapter PID/birth，該 Adapter 啟動的 dedicated Codex app-server PID/birth。Native MCP wrapper 的一對一 local process 實體可使用 OS 親子 birth 與受信任的本地註冊訊息識別。它仍**不等同每個 Codex Thread／ACP Session 的專屬 owner**：多個 Session 可以共用同一 app-server。
3. **可回收權限（strict）**：只有 AgentDock 自己控制、已完成所有 lease/inflight 且沒有其他 Thread/Session/外部 borrower 的資源才能 drain/release。未知 Native Thread mapping、任務未結束、共用進程或 PID 重用一律 `owner_unverified`/`cleanup_deferred`，禁止提示詞或模型直接執行 `kill`/`pkill`/`killall`。

## B. 安全實作序列（ACP-only）

### B0 — Prompt 規則（先行、僅盤點與安全清理）

AgentDock 的 *Codex ACP 專用* prompt policy 在 `session/prompt` 的 user-content text 附加一致的「任務收尾」契約；不覆蓋原使用者任務、非文字 ContentBlock、舊工具規則，不影響 AGY 或直接使用 Codex CLI。僅允許在明確啟用的 Next profile 生效；不對 `session/prompt` 執行中的 steering 反覆注入，也不把控制訊息寫入共用穩定版。此提示僅為**最佳努力**，不能保證每次都報告或使 MCP 退出。

初版已實作於 `internal/app/runtime_next_codex_mcp_prompt.go`（英文 ACP 協作規範）；以下為中文摘要：

> 完成指派任務後，請簡短回報本輪實際使用的 MCP server 名稱與用途；對於你**明確由本輪建立、具有合法專屬管理權、且沒有其他 session/inflight 借用**的資源，優先依提供者自身的正常關閉方式釋放。對 Codex 原生管理、共享、外部或不能證明歸屬的 MCP 程序，不要自行執行 `kill`、`pkill` 或按 PID／名稱／存在時間強制結束。請回報 `used`、`released`（附可信動作結果）、`still_in_use`、`unknown_or_deferred`；若缺乏可驗證資訊，明確寫出未知，不得聲稱已關閉。不要為產生報告額外啟動／重連或盤查未使用的 MCP。回報僅提供盤點線索，AgentDock 另行核對真正的資源狀態。

AgentDock 最終給上層 ChatGPT 的結構化摘要預計為：

`{run_id, acp_session_id, mcp: [{server_id, evidence_kind, status, cleanup_action, cleanup_result, owner_verified}], unknown_count}`。

輸出只顯示 MCP logical server ID / 分類與核實過的結果；不顯示命令列、env、OAuth token、工具輸入／回傳、Host PID 或內部路徑。**不要解析模型自由文字後直接允許 cleanup**。

### B1 — 啟動前宣告／啟動後核對（可選 instrumented MCP）

配置在 **AgentDock Next 專用的 Codex ACP profile** 下，讓受支援的 stdio MCP command 指向 `agentdock-next-mcp-launch` 小型 wrapper，實際 executable/argv 留在受保護的 per-profile 配置內。不要修改使用者全域 `~/.codex/config.toml`、stable AgentDock、Codex Desktop、AGY 或其他使用者 process。須先盤點 Codex 對 profile-scoped config override 的支援，再做個別 server / user opt-in，不可靜默重寫設定。

- **preflight（Codex 已啟動 wrapper、真正 MCP 尚未 spawn）**：Wrapper 向 Next Core 的本機受控 Unix socket/具授權的私有通道申報 `server_id`、一次性 launch nonce、Next namespace、已觀測 Adapter/app-server generation、wrapper process PID + OS birth 以及不可敏感的 config fingerprint；該資料由 Core 依 `SO_PEERCRED` 等同等級 kernel peer PID/UID 與 parent birth 再查核。不得由 MCP 提供者自行選擇 owner ID、Thread ID、PID 或證據狀態。
- **spawn（真正 MCP 開始）**：由 wrapper 管理 `exec`/child spawn 或明確的 stdio 透明接力；將 `MCP PID + birth + parent/pipe evidence` 記錄到 Next Core。每個實體生命週期有唯一 generation，去重必須與實際 owner/transport 同一世代；wrapper 對 stdout 必須 100% 透明，所有診斷走 fd2 或私有 IPC，不能污染 JSON-RPC。一次性權杖只能在 wrapper 使用、不可向底層 MCP 的 env 洩漏。
- **session match 限制**：這個登記能驗證「由某個 AgentDock 管理的 app-server 啟動的某一 MCP 實體」，不必修改 Rust；**不能單靠它驗證是哪個 native Thread**，因多個 ACP Session 可共用 app-server。除非架構額外保證 per-task dedicated app-server，或取得真正可信的 per-thread lease/transport evidence，不能宣稱 per-task 獨佔或在單一 Prompt 完成後自動關閉該 MCP。
- **fail safe**：新 wrapper 未配置、registrar 不可達、驗證失敗或 parent/pipe 傳輸不符合時，標示 `unregistered` / `owner_unverified`，不可落回「猜 PID」。是否允許實際 spawn 必須由權限／可用性策略決定，不能因遺失記錄就宣告成功。不干擾現行已使用的 MCP。

### B2 — Task terminal 回報 + 正確卸載

- `ACP Run` terminal（completed/cancelled/failed/interrupted）時由 AgentDock 的獨立 read-only registry/audit endpoint 彙整 `run_id`、Adapter/Session、已驗證 MCP 實體與情況；事件與 JSON schema 必須是程式核對，不依賴 Codex 自由文字。**沒有已知 per-thread owner 則不要冒險關閉某個 stdio server**。
- `ephemeral` Session 真正 terminal + `session/close` confirmed 時，使用既有 `recycleAppServerIfIdle()`；只有該 Adapter 已無任何 live Session 且其 dedicated app-server ownership 可驗證，才能關閉整個 app-server。`persistent` / `idle-managed` 仍依既有 policy；不能因工具 call 停止就刪除需要復用的 MCP。
- Chrome DevTools／Browser Broker、Codebase Memory shared daemon、HTTP MCP、Plugin、background runs 及其他需跨 task 使用者，保留其 lease 與自己的服務端生命週期，**不要對其執行全域 `pkill`**。

## C. 下一個施工順序與驗收

1. **B0 source-only 已實作，未部署**：`internal/acp/types.go` 新增 Core-only `PromptDecorator`、`internal/acp/prompts.go` 僅在新 Run 的 `StartPromptBlocks` 驗證原始 ContentBlock 後套用，然後再次 ACP capability/8 MiB 驗證；steered Run 不重複附加。`internal/app/runtime.go` 對有資格的 macOS Next Codex ACP profile 注入 private decorator，Next variant + `~/.agentdock-next` + `AGENTDOCK_NEXT_CODEX_MCP_REPORT=source-only` 才有效。非文字／多媒體與接近 8 MiB 上限的 Prompt 不變。整合測試使用**測試專用** ACP Adapter，直接檢查其 `session/prompt` 收到原任務 + 第二塊 MCP 安全回報規範，沒有操作 live Codex。此 prompt **仍只是可選 LLM 行為提示**，不是可驗證資源盤點。
2. `Adapter / P4` 元資料：在任務事件中以既有 `run_id/session_id` 建立邏輯 MCP usage/inventory；明確標示 self-reported vs kernel-attested，沒收到回報就顯示 `unknown`。不要因 native status 對應 server 名稱就標記 PID 已驗證。
3. `Wrapper / P5 / M8` 獨立 source-only 原型：可信 Unix socket 註冊 + OS birth/pipe/parent identity 與 one-shot nonce；只有測試建立的 MCP helper。缺少 Thread owner 時 `cleanup_allowed=false`。
4. 測試 20 輪正常結束、取消、Adapter app-server recycle、並發兩個 ACP Session、出現相同 server name、Native child crash/PID reuse、CBM/DevTools/shared/HTTP excludes。安裝／部署前需通過跨處理序 admission/close fence，後續再評估 release gate。

## D. 與舊 P0 的關係

本決議以 **ACP-driven cooperative observability** 取代「先 Fork Codex CLI 修改 Rust，再能完成 P0」的工程策略。`docs/custom/p0-native-codex-rust-owner-2026-10-11.md` 和原 `readNextNativeMCPPartialEvidence` 是**歷史 read-only provenance 紀錄**，不得因此推論已驗證真正 native Thread ownership；不再對本 P0 施加自行編譯 Rust 的前置條件。

新的 P0 驗收定義是：**可以安全列出和驗證 AgentDock/Adapter 擁有的 MCP 資源、可信且如實回報 unknown、不做未授權 close、在可證明的 scope 釋放資源**。對原生 Codex 所管理且不可驗證的 per-Thread MCP，保持保護狀態並追蹤官方修正；不得用「全部自動殺掉」作為 P0 的替代驗收。
