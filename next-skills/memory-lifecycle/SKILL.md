---
name: memory-lifecycle
description: AgentDock Next 的非例行儲存庫工程、架構決策、Memory/Codebase Memory 維護或中斷 Session 接手時使用；提供任務前檢索、可信知識分類、去重寫入、Single-Writer 索引判斷、5% 額度交接與 MCP/CLI 降級流程。一般微小修改不觸發，也不會在每次任務完成時強制重新索引。
---

# Memory Lifecycle — AgentDock Next

此 Skill 是 **Next 自行維護**的工作流程，不是長期記憶資料庫、索引器、同步排程或新的 Runtime。**先遵守使用者的共用 AI 規則（尤其 Rule 6、Rule 13）與目前儲存庫的 `AGENTS.md`；本文件只描述 Next 的具體操作，不另訂衝突的記憶政策。** 不透過 AgentDock 的純 CLI/其他 Agent，也應可依同樣原則執行。

## 適用條件

- 開始／結束較重要的儲存庫工程（架構、跨模組、部署、工作流程、重要決策）。
- 使用者要求查詢、補齊或同步 Memory 與 Codebase Memory。
- 前一 Session 中斷、額度不足，或必須判斷哪些持久知識尚未寫回。

**不適用**：一般 CSS、文字、機械重命名、臨時除錯、單次操作記錄；不因 Skill 被讀取就啟動 MCP Server、索引、寫入或重建。

## 來源與責任

| 事實類別 | 優先放置位置 |
|---|---|
| 實作現況、工作結果、阻擋、下個動作 | Git / 專案 `docs/STATUS.md`（或原有權威進度文件） |
| 跨 Session 恢復所需的操作資訊 | 必要時 `docs/HANDOFF.md`（或原有交接文件） |
| 架構決策、原因與取捨 | 儲存庫既有 ADR；必要時補 Codebase Memory ADR |
| 程式符號、呼叫與依賴關係 | 原始碼與 Codebase Memory 索引 |
| 跨儲存庫長期適用的偏好、慣例、技術教訓 | 長期 Memory MCP |
| 例行修改、暫態錯誤、Git 已明確記載之內容 | 不額外寫入記憶 |

**Git、現行原始碼、專案權威文件永遠優先於索引或過去記憶。** 寫入前先搜尋現有項目並驗證；有衝突時標示可驗證的新舊差異，避免新增一筆互相矛盾的平行記憶。不可儲存密鑰、認證資料、使用者隱私或大量原始日誌。

## 任務開始：先讀，不先寫

1. 確認實際 repository、worktree、branch、當前工作狀態與權威文件；不要混淆主目錄及不同 worktree 的索引身分。
2. 問題確實需要長期資訊時，以狹窄主題搜尋 Memory，取回已驗證的決策、環境限制或既有偏好。
3. 需要程式碼關係時，使用 Codebase Memory 的專案識別、`index_status`／`check_index_coverage` 等 **唯讀查詢**評估現有索引是否可用；同時檢查目前 Git/ref。不要因開始任務或未發現特定專案就自動觸發全量索引。
4. 對缺失、陳舊、結果不完整的索引，改以原始碼搜尋繼續工作；只在**確有正當索引需求**且通過下方單一寫入者門檻後才安排更新。記憶不可代替實際程式碼驗證。

## 寫入與重新索引決策

1. 先判斷是否產生**經驗證、未重複、持續有價值**的知識；沒有就不寫。
2. 長期 Memory：先搜尋相似／矛盾項，再針對跨專案的穩定知識作最小寫入；進度與交接以專案文件為主。不要把程式碼結構與每次提交完整複製進長期 Memory。
3. ADR：先閱讀既有決策，必要時更新專案權威 ADR；Codebase Memory 的 `manage_adr` 只作補充，不能無條件覆寫或新增同義 ADR。
4. **Codebase Memory 不設「任務結束必重新索引」規則。** 先檢查索引狀態、範圍、當前 commit/ref、是否已有 Git watcher 或其他 Session 的索引工作；只在缺失、實質過期或重要變動確有必要且沒有既有工作覆蓋時，才考慮增量／有界索引。支援的 options 依當前工具 schema 驗證，不猜測參數。
5. **Single-Writer**：共用索引同時最多一位寫入者。先確認寫入權限、活躍任務與可觀察的 owner/lock；若正在索引、options 不一致、Session/owner 不明或服務回報衝突，**不搶鎖、不強制重試、不殺程序、不重建**。延後索引，記錄準確的範圍與待辦，讓下一 Session 再檢查。
6. 不要把「同一個 Codebase Memory Daemon 被多個 Client 使用」誤當成「多個索引 worker」。不要因為索引需要更新就重啟共用 Daemon；build mismatch、Git watcher 與手動 options 衝突是不同問題，應如實診斷並採有界補救。
7. 驗證完成後，只報告**真正寫入**的 Memory／ADR／索引成果；未寫入或被延後的項目不能宣稱已同步。

## 5% 額度／中斷 Session：先交接

只在 Provider／工具可靠回報，或使用者明示剩餘額度 ≤5% 時觸發；**不可由上下文 token 估算額度**。

- 停止啟動新任務、昂貴索引或高風險修改，安全完成／暫停當前最小原子工作。
- 先驗證既有成果；更新儲存庫權威 `STATUS`（有需要才更新 `HANDOFF`），記錄 branch/worktree、未提交變更、commit、測試／CI、阻擋、進行中工作與下一步。
- 只有在不延誤交接且確有必要時，才以有界方式寫入少量耐久的 Memory。**不得把長期 Memory 寫回、完整 CBM 索引或 ADR 重建當成交接門檻。**
- 工具失效或已無足夠額度時，提供明確文字交接與延後項目；下個 Session 以目前檔案與 Git 再確認後補做，避免重複與衝突。

## Next 與純 CLI 的工具適配

- Next 的 **Memory** 可透過已註冊的 HTTP MCP 使用，**Codebase Memory** 透過獨立 stdio MCP Client 連到產品自己的帳號級共用 Daemon。Next 只管理**自己的** MCP Registry 與連線，不能停止、改設定或認領其他 Agent/CLI 的 Client 與 Daemon。
- 先透過當前 AgentDock 實例的 `agentdock_context` 與動態 MCP 搜尋／工具 schema 發現能力；例如 Memory 的搜尋／儲存、CBM 的 `list_projects`、`index_status`、`check_index_coverage`、`get_architecture`、`manage_adr`。不要依名稱猜測參數、憑證或啟用狀態。
- 如果只使用 CLI，依當前可用的 MCP/CLI 工具與專案慣例執行相同的「先查證、分層保存、單一寫入者、失敗降級」政策，不必啟動 AgentDock 或複製其私有環境。
- Memory、Codebase Memory 任一方不可用時，用 Git／原始碼搜尋／專案文件持續工作；註明未完成的特定同步項目，不要假裝寫入成功。不要因單個 MCP 不可用而重複啟動 daemon 或升級執行檔。

## 完成判定

任務驗收以使用者需求、實際程式碼／測試及正確的進度交接為準。持久記憶只需明確標記為：**無需新增／已驗證寫回／因明確理由延後**；不把每次索引、寫兩套記憶或建立新服務當成完成條件。
