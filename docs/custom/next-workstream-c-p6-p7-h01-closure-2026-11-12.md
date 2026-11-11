# NEXT 工作線 C — P6／P7／H01 使用者功能缺口盤點與定點修正

> 2026-11-12（台灣時間）。來源：RC21 Source `0a0a0ba98412c6f22f8793a6abd184d6db62a27e`；本文件與修正僅存在 C 專屬 `fix/next-rc21-p6-p7-h01-user-gaps-20261112` worktree，不代表已部署到 NEXT／Original。依 `next-four-workstreams-handoff-2026-11-12.md` 第 0／3／5 節。

## 使用者情境、最小修正與不做事項

- **P6 Plugin 操作結果失蹤**：使用者啟用／更新 Plugin 後遇到 `outcomeUnknown`／逾時或需要恢復，切去其他頁面再回 Plugin，畫面在 `onMounted` 被動 `refresh()` 時會透過 `resetMutationState()` 清空 `lastFeedback`／`mutationError`，即使尚未確認操作是否完成。使用者失去最後錯誤和安全恢復提示，可能誤以為可重複操作。**最小修正**：只有新 mutation 與既有明確 `clearTransient()` 才清除舊操作提示；被動 snapshot refresh 保留上次操作結果。當前快照失敗依舊 stale、禁止 mutation，不改 Core/Plugin ACL 與既有 `operationStatus`。
- **P7 Settings 儲存結果失真**：`Save()` 收到 completed ACK 後即先設定 `completed=true`，若 `Read()` 回錯仍顯示 Pinia completed 成功；若 `Read()` 直接 throw，外層 catch 誤將其歸為 `settings_save_call_failed`，且保留可能過時的可編輯設定。**最小修正**：把已讀回有效設定作為 completed 條件，針對保存後的讀取例外使用 `settings_read_call_failed`，清除舊設定與可修改標記，保留原有原生權限／持久化流程。明確區分「後端收到儲存 ACK」與「已驗證現在設定」。
- **刻意不做**：不修改 Browser Runtime、Desktop API、i18n、Plugin 標準操作流程、NexusDock、P13/P14、M9 發行與 Native UAT 長鏈；不啟動／重啟／部署任何 Next／Original Core／App，不更動使用者 Browser Profile、Plugin 資料或 OS 授權。不 Push/Merge/Tag/Release。

## 逐項盤點與處置

| 範圍 | 現有證據／程式路徑 | 判斷與處置 |
| --- | --- | --- |
| P6 安裝／更新／啟用／停用／清除 | `pre-m9-plugin-management-roundtable.md` §17.5：2026-10-12 已有真實已安裝 NEXT GUI、picker、confirm、ZIP v2 更新與安全刪除 PASS；`stores/plugin.ts` 和 `components/plugin/PluginPanel.vue` | 不重跑、不重寫已 PASS 生命週期；僅修復被動刷新抹除不確定操作結果的使用者阻斷 |
| P6 失敗／恢復與快照 | `stores/plugin.ts`: `refresh`, `runMutation`, `lastFeedback`, `mutationError`, `stale`, `reconcileOperation`；Plugin GUI 顯示 `outcomeUnknown`／recovery | 本工作線修補 `refresh`；繼續保留 stale fail-closed 與新動作覆蓋舊回饋的語意；未聲稱真實逾時現場復現 |
| P7 Connection | `stores/connection.ts`: `actionError` 與 `error` 分離，`refresh` 不清除前一個拒絕；`connection.test.ts` 既有拒絕→被動刷新→錯誤保留、Snapshot fail-closed 測試 | **未發現新的可重現功能缺口，不修改**；不為了 H01 再重播 Stop/Restart 等真實操作 |
| P7 Settings | `stores/basicSettings.ts`: `Save` ACK 與後續讀取的狀態處理 | 本工作線修正狀態不一致與錯誤歸因；不碰設定實際檔案／服務 |
| P7 Diagnostics | `stores/diagnostics.ts`／`components/system/DiagnosticsCard.vue`：open 失敗後禁用 capability，使用者需 Refresh 重新判斷，訊息脫敏；`diagnostics.test.ts` 已測 | 這是明確 fail-closed／手動恢復設計，未有足夠證據表明真實使用者操作受阻，**不修改** |
| H01 拒絕後跳頁返回 | `stores/connection.test.ts` 的 rejected action passive refresh、Overview error test；2026-11-11 H01 root-cause 記錄黑畫面屬擷取觀測問題，真正 mutation Host UAT 尚未完成 | 既有 source logic 已保留拒絕提示。**本次不修改**；Host 操作未獲不干擾現役服務的安全情境，維持「來源層通過，原生互動未宣稱 PASS」 |

## 可重現證據與局部驗證

1. 在未修復的 Source store 上先增加三個回歸案例：`plugin.test.ts` 驗證逾時／outcome unknown 後 passive refresh 不清掉錯誤；`basicSettings.test.ts` 驗證成功 ACK 但後續讀取回錯或 throw 時，不能宣稱已驗證完成且必須正確分類錯誤。這三個案例在原實作**3 FAIL**，形成可驗證的 Source 缺口。
2. 最小修改 `plugin.ts`、`basicSettings.ts`；前述三個案例重新執行 **PASS**。另補正常讀回完成、新 mutation 取代前一筆回饋、儲存被拒不額外讀取等正負向斷言。
3. `pnpm exec vitest run src/stores/plugin.test.ts src/stores/basicSettings.test.ts`：修復後 2 files／7 tests PASS；另補正常讀回完成、新 mutation 取代前一筆回饋、儲存被拒不額外讀取等正負向斷言後，`pnpm test` **26 files／189 tests PASS**（含 Connection／Diagnostics 原回歸）。
4. `pnpm typecheck` (`vue-tsc --noEmit`) **PASS**；`pnpm exec vite build --mode production` **PASS**（189 modules transformed）。僅有既有大型 JS chunk 非阻擋警告；`git diff --check` **PASS**。以上屬 Source/Store 層級，無實機 GUI mutation。
5. 前端依 C worktree 既有 lockfile 使用 `pnpm install --offline --frozen-lockfile --ignore-scripts` 本機套件快取；未操作網路認證或現役 Next。無 Go/API 原始碼變更，沒有額外跑無關後端全庫／Race Gate。

## 接收與交接 A

- 專用 branch：`fix/next-rc21-p6-p7-h01-user-gaps-20261112`
- 專用 worktree：`/Users/wei/sideProject/agentdock-next-p6-p7-h01-20261112`
- 自有內容：`desktop/shared-poc/frontend/src/stores/{plugin.ts,plugin.test.ts,basicSettings.ts,basicSettings.test.ts}` 與本交接文件。
- A 建立的統一 baseline 不應在 C 工作期間被修改；由後續**指定整合者**核對 C commit 與 A HEAD 後順序 cherry-pick。若 B 也更新 shared Desktop API／i18n，應由整合者處理，**本次 C 未改這些共享檔**。
- **驗證層級**：本輪是 Vue Store 的精準 Source 問題復現與離線修復，並非已安裝版原生 GUI／Host PASS。未觸碰現役服務；無 Push。正式部署／發行需另外授權。
