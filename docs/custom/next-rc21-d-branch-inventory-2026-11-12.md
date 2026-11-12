# AgentDock NEXT D — 本機 56 分支全量分類表（2026-11-12）

> 此表是 D 開始盤點時的 **56 條既有本機 branch**；D 新建 `docs/next-rc21-github-governance-d-20261112` 是第 57 條，屬獨立文件 review worktree，**不列為待 Push 的來源分支**。原始工作樹禁止清除、reset、rebase 或搬移。遠端同名狀態以實際 `git ls-remote --heads origin` 核對；ahead/behind 用相同 SHA 的 local tracking 計算。

## 統計／閱讀方法

- **23/56** 是 A／B／C 整合 HEAD 的 Git 祖先（含 `custom/main`），**33/56** commit SHA 不是整合祖先；不等於功能一定不存在，需原 owner 判斷。
- **10/56** 有同名 origin（其中 8 equal、`main` 54 local ahead、`custom/main` 312 local ahead）；其他 **46** 不應因此盲目 Push。
- 盤點時 **24** 原始 worktrees，其中 4 dirty：M6 (17)、NEXT Edge (12)、P5 OAuth fence (5)、Original maintenance (1)；P10 worktree 另外 `locked initializing`。
- `A` 代表 Git Commit 歷史已由整合分支包含，**不是允許刪除工作樹**；`U` 代表仍需 owner 保留／覆核；`D` 是 deferred，`O` 是 Original；`P` 是優先的唯一整合來源候選；`T` 是 `custom/main`，必須另審；`W` 是禁止清理的 dirty owner 工作；`R` 是獨立保留的既有遠端；`M` 是官方參考。
- `是` / `否` 的祖先欄位僅由 `git merge-base --is-ancestor` 判斷，不推定更名或 cherry-pick 語義。**本輪沒有 Push、Delete 或 Merge。**

| Local branch | 統一 HEAD 祖先 | `custom/main` 祖先 | origin 同名與先後 | 原始 Worktree | D 建議分類 |
|---|---|---|---|---|---|
| `architecture/agentdock-next-isolation` | 是 | 是 | 同名；remote 0 ahead / local 0 ahead | — | A:已納統一祖先（停止獨立同步） |
| `architecture/i18n` | 是 | 是 | 同名；remote 0 ahead / local 0 ahead | — | A:已納統一祖先（停止獨立同步） |
| `architecture/shared-desktop-api` | 是 | 是 | 同名；remote 0 ahead / local 0 ahead | — | A:已納統一祖先（停止獨立同步） |
| `architecture/shared-desktop-baseline` | 是 | 是 | 同名；remote 0 ahead / local 0 ahead | — | A:已納統一祖先（停止獨立同步） |
| `architecture/shared-desktop-ui` | 是 | 是 | 同名；remote 0 ahead / local 0 ahead | — | A:已納統一祖先（停止獨立同步） |
| `custom/main` | 是 | 是 | 同名；remote 0 ahead / local 312 ahead | agentdock-custom | T:正式客製主線（另審） |
| `docs/upstream-v101-adoption` | 否 | 否 | 同名；remote 0 ahead / local 0 ahead | agentdock-upstream-101-adoption | R:既有遠端獨立分支（保留） |
| `feature/acp-lifecycle-manager` | 是 | 是 | 無同名（非 Push 待辦） | — | A:已納統一祖先（停止獨立同步） |
| `feature/acp-session-retention-7d-20261020` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/activity-center` | 否 | 否 | 同名；remote 0 ahead / local 0 ahead | agentdock-activity-center | R:既有遠端獨立分支（保留） |
| `feature/m6-acp-tests` | 是 | 是 | 無同名（非 Push 待辦） | agentdock-m6-acp-tests（dirty 17） | W:owner dirty（保留） |
| `feature/m7-acp-diagnostics-model` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/m7-desktop-acp-ui` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/m7-desktop-backend-review` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/m7-process-diagnostics` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/m8-permission-approval` | 是 | 是 | 無同名（非 Push 待辦） | — | A:已納統一祖先（停止獨立同步） |
| `feature/next-a1-rc20-20261105` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-a1-rc20-20261105 | A:已納統一祖先（停止獨立同步） |
| `feature/next-acp-retention-integrated-20261021` | 是 | 是 | 無同名（非 Push 待辦） | — | A:已納統一祖先（停止獨立同步） |
| `feature/next-edge-rc-20261023` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-edge-rc-20261023（dirty 12） | W:owner dirty（保留） |
| `feature/next-mcp-card-ux` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-next-mcp-card-ux | D:已延後（不 Push） |
| `feature/next-p13-single-owner-integration-20261022` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-next-p13-integration-20261022 | D:已延後（不 Push） |
| `feature/next-p8-browser-mode-switch-20261112` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-p8-mode-switch-20261112 | A:已納統一祖先（停止獨立同步） |
| `feature/next-skill-cbm-integration` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-next-skills-cbm | U:未納統一祖先（owner 審核） |
| `feature/next-uiux-audit` | 是 | 是 | 無同名（非 Push 待辦） | — | A:已納統一祖先（停止獨立同步） |
| `feature/p10-release-migration` | 是 | 是 | 無同名（非 Push 待辦） | agentdock-p10-release-migration（locked） | A:已納統一祖先（停止獨立同步） |
| `feature/p11-adhoc-native-uat` | 是 | 是 | 無同名（非 Push 待辦） | agentdock-p11-adhoc-native-uat | A:已納統一祖先（停止獨立同步） |
| `feature/p5-mcp-desktop-api` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-p5-mcp-desktop-api | U:未納統一祖先（owner 審核） |
| `feature/p5-mcp-env` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-mcp-inspect-tools` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-mcp-oauth` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-mcp-oauth-fence` | 是 | 是 | 無同名（非 Push 待辦） | agentdock-p5-mcp-oauth-fence（dirty 5） | W:owner dirty（保留） |
| `feature/p5-mcp-oauth-highfix` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-mcp-registry` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-mcp-transport` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-mcp-ui` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-p5-mcp-ui | U:未納統一祖先（owner 審核） |
| `feature/p5-review-manager` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-review-oauth-redirect` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/p5-review-runtime` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/pre-m9-connection-auth` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/pre-m9-connection-ui` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/pre-m9-next-codex-path` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/pre-m9-next-shared-package` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/pre-m9-port-ownership` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/pre-m9-tunnel-backend` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `feature/pre-m9-wave1-integration` | 否 | 否 | 無同名（非 Push 待辦） | — | U:未納統一祖先（owner 審核） |
| `fix/macos-service-startup-reconcile` | 否 | 否 | 同名；remote 0 ahead / local 0 ahead | — | R:既有遠端獨立分支（保留） |
| `fix/next-orca-cli-discovery-20261017` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-next-orca-discovery-20261017 | U:未納統一祖先（owner 審核） |
| `fix/next-orca-live-deploy-20261018` | 是 | 是 | 無同名（非 Push 待辦） | — | A:已納統一祖先（停止獨立同步） |
| `fix/next-rc19-to-rc20-deploy-compat-20261111` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-rc20-deploy-compat-20261111 | A:已納統一祖先（停止獨立同步） |
| `fix/next-rc20-h04-h02-r01-20261111` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-h04-h02-r01-20261111 | A:已納統一祖先（停止獨立同步） |
| `fix/next-rc21-p6-p7-h01-user-gaps-20261112` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-p6-p7-h01-20261112 | A:已納統一祖先（停止獨立同步） |
| `integration/next-rc21-consolidated-20261112` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-rc21-consolidated-20261112 | P:唯一後續來源候選 |
| `integration/next-rc21-product-baseline-20261112` | 是 | 否 | 無同名（非 Push 待辦） | agentdock-next-rc21-product-baseline-20261112 | A:已納統一祖先（停止獨立同步） |
| `main` | 否 | 否 | 同名；remote 0 ahead / local 54 ahead | agentdock | M:上游參考（保留） |
| `maintenance/original-v1.1.0-20261021` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-original-maintenance-20261021（dirty 1） | O:Original 獨立維護 |
| `maintenance/original-v1.1.1-isolated-20261103` | 否 | 否 | 無同名（非 Push 待辦） | agentdock-original-v111-20261103 | O:Original 獨立維護 |

## 後續收斂原則

已納入統一 HEAD 且原 owner 工作樹乾淨的歷史分支，若未來有清理需求，可先讓 owner 確認可停止維護；**不需要逐一 Push**。有 dirty、locked、Original 或 deferred 標記者即使 ancestor 也不得自動清理。未成為祖先的 branch 要先做功能／補丁差異審查，不得一律宣稱已 merge 或安全刪除。D 本輪不調整任何分支或其 worktree。

完整遠端、可達 Git 歷史掃描與正式發行結論： [D 治理結案文件](next-rc21-d-github-governance-closeout-2026-11-12.md)。
