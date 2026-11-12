# NEXT Native Codex MCP：Desktop 政策寫入與啟動 Fence（2026-10-25）

## 產品問題與實際增量

NEXT 的 source-only Native Codex MCP 在 `prepareNextCodexMCPRegistry`／`attachNextCodexInstrumentedMCP` 經 M8 admission 後使用 `Runtime.lifecycleMu` 保護啟動和註冊，但本機 Desktop Permission Control 的兩種**改變政策 Revision**的合法操作（`update_policy`、`approve_workspace`）原先沒有持有這把鎖。若政策更新與已核准的 native bootstrap 交錯，可能出現政策撤回與 native 啟動交錯的不確定時序。

`internal/app/runtime_permission_control.go` 在完成獨立確認的 `ConsumeConfirmation` 之後，僅當本 Runtime 持有 NEXT source-only native permission writer（`nextPermissionOwner != nil`）時，使這兩種 Desktop 政策 mutation 持有同一個 `lifecycleMu`。上鎖前與取得後都檢查 cancellation，已關閉的 Runtime 拒絕 commit。Store 自己的 Revision 與持久化、confirmation 單次使用、M8 admission 均沿用；**不在鎖中等待人類批准、沒有建立新服務、沒有在正式執行環境開始 native MCP**。Stable 和未選擇 native source-only 的 NEXT 路徑仍維持原有行為。

## 驗收證據與清楚界線

- `runtime_permission_control_darwin_test.go` 使用 NEXT owner-private 臨時 Core 的**真實 Desktop 確認機制**；政策更新在 native dispatch fence 被持有時必須等待，鬆開後正確提交；等待時取消無法提交；關閉後拒絕更動。測試讀取實際 state.json 以確認磁碟與記憶體 Revision 一致。
- `approve_workspace` 也是政策更新，需要相同序列化。測試由 Core Permission Store 建立**具備可信 workspace metadata 的隔離 fixture**，經正式 Desktop 確認後執行 mutation；它只測試已具合法批准記錄時的交易界線，不聲稱目前任意 `file_edit` 能夠取得 Workspace Grant。未竄改 production workspace eligibility。
- 測試並確認 Stable 與普通 NEXT 在同一把 lifecycle mutex 被持有時，不因此阻塞其既有本機政策更新。此輪操作限 source tests，未修改 Core listener、Native Codex 下載／登入、App、Port、Edge、Stable 或已安裝 RC19。
- Go 定點／race／完整 app+permission+acp+tool-acp+launchregistry／vet 與 source、Live health 結果以本次驗證命令為準；此文不把 source-only 算作 installed Host PASS。

## Remaining production release blockers

1. **Native Codex MCP 實機啟用仍 OPEN**：雖然 policy mutation 與 native 註冊的關鍵階段已序列化，但**正式可信任 Desktop operator 憑證／UI → M8 request → Native app-server → wrapper → model tool → Session close**整條流程未安裝 UAT；不能把 localhost、任意 Bearer 或 ACP prompt 當作本機操作批准。跨 Core writer ownership 的早期 source-only 機制也不是跨版本／完全 OS 交易保證。未結 same-boot journal 和 native orphan 的安全恢復仍未達生產條件。
2. **Connection GUI Host OPEN**：RC19 GUI 本身 AX46／不搶焦點已驗證，但真正拒絕 mutation 後跨頁顯示仍缺實機；Core-outage fail-closed source 尚未安裝。
3. **Tray／MenuBar Host OPEN**：macOS AX proxy 無法確認真正 NEXT status item 身分及安全 Open/Show；不點擊未標記座標，也不更動 Stable 菜單或系統偏好。

這次僅本地 source checkpoint；**不 merge、push、tag、發佈，也不重新安裝 RC20**。既有 RC16 Browser、RC18 rollback 與 RC19 GUI 證據維持有效，避免重複操作授權。延續 [Native Session 碰撞回收](next-native-mcp-session-collision-2026-10-25.md)、[B0–B6 closeout](codex-acp-mcp-b0-b6-closeout-2026-10-12.md)。
