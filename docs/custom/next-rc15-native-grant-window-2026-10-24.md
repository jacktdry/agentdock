# AgentDock NEXT — RC15 Native Edge fresh-acquire 授權時序修正（2026-10-24）

## Product task and user pain

公司使用者在 ChatGPT／ACP 透過同一條已明確授權的 Edge Browser Broker 連線，開啟 Oprueba 與 HomiPlay 各自的隱藏 target、獨立操作、乾淨釋放，再交給新任務取得新租約。實際 [RC13 Host 驗收](next-rc13-host-uat-2026-10-24.md) 證明前兩筆租約、sibling release 均 PASS；**最後一筆釋放後 fresh acquire** 回報 `BROWSER_REQUIRED_ROUTE_UNAVAILABLE: external peer verification unavailable or mismatched`。沒有重連、沒有更動 Edge Profile 或 Stable。

## Concrete verified source bug and smallest fix

- `RoutePlanner.ConnectorStatus` 產生一個至多 5 秒、單次可消耗的 `externalRouteGrant`。舊版 `ExternalLeaseManager.Acquire` 有效消耗後，還在每一次 `verifyAdmissionPeer` **事前與事後**比對同一張 Grant 是否到期；而真正的 native acquire 要多次做 macOS OS signature／Profile／held-socket 獨立驗證與 CDP target 建立。此時 Grant 時效是在**取得操作途中**到期，並非新任務首次取得時過期。
- 新增 `TestNativeEdgeAdmissionContinuesAfterConsumedGrantExpiry`：在第一次合法驗證後模擬短期 Grant 到期；舊 source 真正 FAIL CLOSED，錯誤與 Host 報告相同；修正後連續 peer 驗證／new target 建立可完成，不需新 dial。
- Source 更改：`consumeExternalRouteGrant` 仍必須於首次開始前驗證未過期、單次 CAS；`nativeEdgeRuntime.StartExternal` **再驗證一次當下 Grant 新鮮度**，防止消耗後因排程延遲才開始。只有`*nativeEdgeRuntime`已經 `used && started` 的**同一次**取得流程，`verifyAdmissionPeer` 不再把舊 Grant expiry 當成中途撤銷，而是繼續對每一步進行獨立、真實的 `verifyPeer` 身分認證。其他 legacy external backend 的 Grant 期限行為維持不變。
- Native `Acquire` 新增 **35 秒總操作期限**（保留任何更短 caller deadline）。超時／取消一律 fail-closed，清理使用既有 owner-scoped stop，無跨 Profile fallback、無自動 Edge 重連。連線、程序身分、簽章、Profile epoch 與 held TCP tuple 仍使用原有驗證；不延長原有單階段原生 OS 檢查上限。
- `TestNativeEdgeExpiredGrantCannotStartOrReplay` 驗證：在消耗前就過期、或消耗後尚未開始 native worker 就過期，都不能取得 worker 或建立其他連線；原有 legacy `expired after baseline`、取消、錯誤身分與 noncooperative verifier 拒絕測試維持 PASS。
- 前輪 [RC14 stage preservation](next-rc14-fresh-acquire-stage-2026-10-24.md) 保留可信原生錯誤階段的診斷，不混同於本次真正在取得途中的誤拒修正。

## Verification status and restrictions

- 四套件 Go（browser/browserpolicy/config/app）**1,703 PASS、7 opt-in SKIP、0 FAIL**，定點 Race Detector **53 PASS、0 FAIL**，四套件 `go vet` 與 `git diff --check` PASS。已先讓新增的 native consumed-grant 測試對舊 source **FAIL**，再確認更改後 **PASS**。Host 安裝證據另記。
- **源碼 PASS 不是 macOS Edge Host PASS**；目前已安裝 RC13，RC15 僅 source 候選。RC15 完成乾淨 commit、manifest、封裝／簽章及 NEXT-only 閒置條件後，才可替換 **只有 NEXT 8767**；保留 RC13 rollback。新 Host 測試必須一條正式使用者同意的 Core-owned Edge socket，檢查雙租約/sibling/fresh + 兩個獨立負向 scope、網站 no-focus、cleanup 0。沒有 real fresh-acquire Host PASS 不能合併/發布。
- **不做**：不改 Stable 8765，不碰未授權 Profile 或 Edge 程序，不在失敗後自動重複原生偵錯同意，不放寬簽章或來源限制，不用 synthetic 結果冒充產品功能。
