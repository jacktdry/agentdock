# AgentDock Next Core Skills 與 Codebase Memory — 2026-10-16

> 狀態：Next-only live enablement + repository implementation verified；尚未將本分支打包部署成新版 App，也尚未完成多 ACP Session 的長時程負載驗收。
>
> 分支：`feature/next-skill-cbm-integration`，工作樹：`~/sideProject/agentdock-next-skills-cbm`。
>
> 決策：stable 和 Next **分別維護** Managed Skills、MCP Registry、ACP/permissions、執行狀態。只使用 Codebase Memory 產品自己已存在的跨客戶端 Daemon / 索引資料共用契約，不用兩套 AgentDock 共用 `AGENTDOCK_HOME`。

## 使用者工作流程、收益與非目標

- 痛點：Next 打包內有 4 個 Core Skills，`agentdock_context` 卻只列出 2 個；Next 沒有 Codebase Memory MCP；直接複製 stable registry / Skill 路徑會破壞 Next 的獨立權限、MCP generation 與生命週期。
- 直接改善：Next 現在能發現並讀取 6 個 Managed Skills，且能透過自身的 standalone MCP 管理查詢現有的 87 個 Codebase Memory 專案。
- 最小改動：在 **Next Shared Desktop** 實際部署路徑補上 bundle bootstrap，維持現有 Core 的交易式 Skill 安裝；新增多實例導引文字；僅在 Next MCP Registry 註冊現有 CBM 0.11.0 stdio thin frontend。
- 驗收：靜態 Source / fixture 測試、Next 自有 Core-only bootstrap、獨立 Next `agentdock_context`、MCP initialize / refresh / `list_projects`，確認沒有新增另一套 CBM daemon 或碰 stable 的服務與設定。
- 不做：stable AgentDock 的 App/Core/registry/state 改寫或重啟；不改 CBM binary、cache、global config、watcher、index options；不 kill 任何 MCP/ACP process；不建立 MCP sidecar 常駐服務、跨 instance 共用 stdio session、全域自動重建索引；不發佈正式版本或合併 P10。

## Core Skills 根因與修正

- `~/Applications/AgentDock Next.app/Contents/Resources/core-skills/manifest.json` 實際包含 `agentdock-user-guide`、`skill-authoring`、`skill-installation`、`plugin-import`，其 ZIP 中 `SKILL.md` 與專案來源一致。
- Next 使用的 Wails Shared Desktop `packaging/macos/deploy-next-shared.py` 在換包、驗證 Core PID / listener / health、重新註冊 Next Core / Tunnel 後原先**沒有呼叫** `skill bootstrap`。舊 Swift `InstallerRunner.swift` 有 bootstrap，但未涵蓋這條 Wails 部署路徑。不能把「打包存在」解釋成「已安裝」。
- 修正：Next-only deploy 在 App 版本、服務與健康檢查成功後，執行 **已安裝 Next 包內** 的 `Contents/Helpers/agentdock skill bootstrap --bundle Contents/Resources/core-skills`，明確固定 `AGENTDOCK_DESKTOP_VARIANT=next`、`AGENTDOCK_HOME=~/.agentdock-next`、`AGENTDOCK_DEFAULT_DIR=~/AgentDock Next`。**2026-10-22 更正**：Skill bootstrap 已移出 App rollback 範圍；Go 的 Skill 交易不能保證在 Python timeout 強制終止後完成回復，不能以還原 App Contents 宣稱 Skills 已復原。失敗處理見下方 checkpoint。Installer-only 腳本仍保持只替換 App，不偷偷變更服務/狀態。
- Core Skill 說明修正：所有原版路徑 `~/.agentdock` / `AgentDock.app` / 8765 明確標註為原版示例；macOS Next 使用自己的 Home、App、8767，優先依當前 `agentdock_context` 和實際設定，不回退原版。
- 現場一次性修復：使用 **Next 已安裝 Core binary**，搭配本分支建置的 Core Bundle，在 Next 獨立目錄執行 bootstrap，**未重啟 Core / GUI**。live context 已回報 6 個 Managed Skills：`acp-adaptive-routing`、`multi-agent-discussion`、`agentdock-user-guide`、`plugin-import`、`skill-authoring`、`skill-installation`。
- 未將本分支 Source 重新建出 Next App，因此「未來新部署自動執行 bootstrap」由離線測試驗證，尚待下一次 **Next-only** deployment 的真實 UAT。

## Codebase Memory：為何與 Memory MCP 問題不完全相同

- 本機執行檔：`~/.local/share/mcp-servers/codebase-memory-mcp/bin/codebase-memory-mcp`，`--help` 回報版本 **0.11.0**、stdio MCP server 和 one-shot CLI；沒有承諾可供 AgentDock 動態 MCP 使用的 Streamable HTTP endpoint。
- 官方本機 README `Session Coordination Daemon`：不同客戶端的 **stdio thin clients** 共享每帳號的 daemon、索引工作與背景 watchers。MCP Client 數量不等於 index workers 數量。這與先前 Memory 由多個完整 stdio Memory server 改成共用 HTTP 服務的問題不同，**不能照搬 Memory 的 HTTP Registry URL**。
- Next 註冊 standalone MCP `codebase-memory`，transport=`stdio`、command=既有本機 CBM binary、無自訂 cache root、無全域重建；Next Registry Version 2 的 generation、runtime owner 與 credential scope 繼續獨立。沒有改動 stable Version 1 Registry。
- Next 執行 `mcp_manage refresh` 後狀態 `ready`、17 tools。Next 工具 `codebase-memory:list_projects` 成功回傳既有 **87** 個 project，重複三次唯讀查詢仍成功。
- 一次性程序快照：嚴格核對 OS 執行命令 basename，22 個 Codebase Memory family processes，其中 **直接隸屬 Next Core 的 stdio Client 1 個、全帳號 daemon 1 個、index worker 0 個**。這是短時間抽樣，不證明外部 Codex/AGY ACP 長時間 session cleanup，也不代表沒有外部 Client；不得因程序數就全域清理。
- 已知舊問題分別是：① CBM binary 在舊 supervisor 仍活躍時被更新，造成 supervisor/worker build mismatch；② Git watcher 自動索引和手動 `index_repository` 使用不同 options 競爭。前者的產品本身 0.11.0 README 已描述 account-wide activation barriers；後者需要 project/commit/options 一致性、single-writer 和 bounded retry。**沒有進行 live 更新或索引壓測，不聲稱完全解決。**
- 危險邊界：即使 Next 有單一 Core-managed stdio Client，也無法管理外部 Codex CLI/Desktop / ACP native threads 啟動的所有 CBM Client；未證實 owner/lease 前不自動終止、drain 或 restart。新 agent / CLI 不應為每個 turn 新建一個不受管理的 MCP frontend；必須在真實 Adapter 證據上治理。
- Memory MCP 保留 Next 自有 streamable_http 設定。未直接搬 `memory-lifecycle` Skill：其「任務結束必重新索引」與跨 session single-writer/read-only 工程規則需收斂後才能引入 Next，避免自動索引碰撞。亦不直接移植 `skill-vetter-runtime`（仍依賴舊路徑）。

## 驗證與接手

- `python3 scripts/test/test-next-shared-deploy.py`：14 tests pass，包含 Next Scoped Bootstrap env、成功路徑與失敗 rollback。
- `go test ./internal/skill/bundle ./internal/skill/state ./internal/skill ./internal/mcp/client`：通過。
- `git diff --check` 與 Python 語法檢查：通過。
- live Next context：6 Managed Skills、CBM `ready` / 17 tools；CBM `list_projects` 顯示 87 projects。stable 未重啟或改動。
- 尚缺：下一次 Next-only App 部署的實際 `bootstrap_next_core_skills` UAT；Codebase Memory across many ACP sessions 的 bounded count / cleanup stress，以及 index options / running build 協調。不得用短時間初始化與查詢取代此驗收。
- Codex ACP 在本輪回報 quota exhausted，未能參與 repo-writing；實作由 Mac-Dev 執行精確、可回溯的小範圍變更，沒有跨模型審查。下一輪可對 diff 再做独立 code review，**不要因 quota 問題升級模型或重複啟動 session**。

## 2026-10-16 後續：Next-only Memory Lifecycle

> 此節更新上方「當時尚未直接搬 `memory-lifecycle`」的階段狀態；`skill-vetter-runtime` 仍維持不移植。

### 問題與目標

使用者已授權只將符合現行 Rule 6／Rule 13 的 Memory Lifecycle 移植到 Next，且不改 stable AgentDock。原版 Skill 存在「重大程式碼變動結束時即重新索引」的固定步驟；這與共用規則要求的必要性判斷、CBM watcher/manual options 競爭、Single-Writer 和 5% 額度交接優先順序不一致。

**可觀察的改善**：Next 可讀取並選用自己的 `memory-lifecycle` Managed Skill，在任務開始時先讀現有 Memory／CBM，結束時分類來源並只做必要寫入；索引繁忙或 owner 未知時延後，不為了交接啟動昂貴索引。Skills 和 MCP registry 都保持 stable/Next 分離。

### 實作邊界

- Source：`next-skills/memory-lifecycle/SKILL.md`；**獨立 Next-managed Skill**，不是新增到跨發行方式的官方 `core-skills/` bundle，也不是 symlink 到 stable 的 `~/.agentdock/skills/`。
- 在 Next 自有 `~/.agentdock-next/skills/memory-lifecycle`，透過 **stable Mac-Dev 控制通道**呼叫 Next 已授權的 `skill_manage install`；只動 Next 的 managed Skill state。
- Source 是純 Markdown：無獨立執行程式、排程、新 Daemon、額外設定檔或祕密；仍使用已註冊 Next Memory HTTP 和 Next Codebase Memory stdio/共用 Daemon。
- Rule 13 是上位來源：Git、程式碼、專案文件權威；Memory 保存跨專案長期知識，Codebase Memory 保存符號索引；只在必要且有單一已確認的寫入者時索引。不自動執行全量重建、清理、合併或殺其他 Session。
- Rule 6：可靠確認 provider 額度 ≤5% 才啟動交接模式，優先驗證、STATUS/HANDOFF、明確列出延後記憶更新；不因 token 估計觸發。
- 純 CLI 情境依相同政策降級，不要求連接 AgentDock。MCP unavailable 時保留 Git/docs 工作流，不虛構已寫回結果。
- **安裝／更新維護**：修改版控來源後，再由 stable Mac-Dev 控制通道對 Next 執行 `skill_manage install`，source 為此工作樹內的 `next-skills/memory-lifecycle`；Next 可用 `agentdock_context`、`read_file` 验證。目前是獨立 Managed Skill，不是新機安裝時自動隨官方 Core bundle 安裝；部署更新應保留 Next 的獨立 managed state。

### 2026-10-16 本輪證據

- 與 `~/.ai-rules/RULES.md` 實際 Rule 6/13 比對；原版來源只讀，未改 stable。
- `python3 core-skills/skill-authoring/run.py` 的 `lint`：`portable=true`、`error_count=0`、`warning_count=0`，只檢查一份純文檔來源。
- Next `skill_manage install`：`changed=true`、`skill=memory-lifecycle`、`content_digest=sha256:ef2d911931407040e8686f5f82e2145ec2f0c15949b6640a1f7e5165425b3b9b`。
- Next live `agentdock_context`：Home `~/.agentdock-next`、共 7 個 Managed Skills，包含新 Skill；Next `read_file` 可讀取它的完整 `SKILL.md`，與 Git Source bytes 完全相符。
- Next `memory:memory_search` 唯讀成功（2 個結果）；Codebase Memory 仍為 `ready`，17 tools，先前 `list_projects` 讀到現有 87 個 projects。沒有 Memory store、索引或 CBM Daemon 的寫入/重建操作。
- 這是工作流程指令型 Skill：上述驗證證明**安裝、載入及兩套讀取工具通道**正常，不代表已完成多 Session 並發索引寫入的壓力測試；此次刻意不製造競爭/索引更新。
- `skill-vetter-runtime` 未複製或安裝，仍保持原版專用；舊版靜態 ClawHub 稽核已經在前階段分類為可選後續改進。

## 2026-10-22：部署失敗邊界（source-only）

- 保留 `manual_version_gate`、PID/listener/health gate；僅 App activation errors（install／registrar／health 等）沿用原 App rollback。bootstrap 在 App 已更新且通過健康檢查後執行，失敗或 timeout 不回滾 App、不印完整成功，保留 Previous Next bundle backup，明確要求 **Next-only manual repair**；Skills 可能部分更新，App backup 不代表 Skill snapshot。
- 指定 Skills 寫入 root 及祖先在 `jobs`／installer 前與 bootstrap 前拒絕 symlink；檢查至寫入間仍有競態窗口。未操作 live state、Registry 或服務。
- 本輪 deploy tests **29/29 PASS**、兩檔 `py_compile` 與 `git diff --check` 通過；變更未 stage／commit。這是 source-only 驗證，installed App／Host UAT 尚未執行。
