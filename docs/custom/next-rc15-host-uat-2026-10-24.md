# NEXT RC15 installed Host — Fresh Acquire 真實通過、負向回報待修（2026-10-24）

## Overall

**RC15 installed source `1489ad33685b` 的真實 Edge Host：CONNECT、雙租約、sibling release、全部釋放後 fresh acquire、Oprueba navigate／AX／PNG、正式 disconnect、no-focus sampling **PASS**。整份腳本 `ok=false`：禁止 JavaScript 的確被擋下，但傳回通用 `external transport fence unavailable or mismatched`，不是可驗收的明確政策拒絕；未核准 URL 之獨立負向未執行。RC15 尚未達完整 DoD，未合併、push、tag 或發布。**

### Version / NEXT-only deployment

- Branch `feature/next-edge-rc-20261023` source commit `1489ad33685bf2a1646dbfe0c8b38b5c44f629a7`，Core `0.9.2-rc.15`，bundle `16.0.0`、`release_ready=false`，canonical manifest `/private/tmp/agentdock-next-edge-verification-y6b6836m/next-rc15-candidate.json`。
- App／ZIP／DMG 在 `dist/next-shared-arm64.n9LV1D/package/`，Vue typecheck/Vite、Core、macOS arm64 App、9 個 package tests、ad-hoc deep strict codesign、DMG checksum PASS。
- 候選 App/ZIP/DMG **24 files SHA match**。ZIP SHA-256 `4b621888902934c8752050522f55620b13b1b4dad5b489e19296085e831fd261`；DMG SHA-256 `8508c1332e6a39ebf7e6ed2f65dea7cce60654ebae58e98fc30686d6c6a61c0d`。安裝 `~/Applications/AgentDock Next.app` 24 file SHA match & codesign PASS。
- 安裝前 Browser idle／0 owner/lease/worker/operation/orphan，Computer 0 active session、ACP ready/running/loaded 0、GitLab ready/35。NEXT-only installer returned `NEXT_ONLY_DEPLOYED`；NEXT 8767 healthy `rc.15`，**Stable 8765 remains 1.1.0 untouched**。
- RC13 rollback bundle at `~/Applications/.agentdock-next-replacement-isy0dclw/AgentDock Next.app` 存在且 Core 版本已驗證 `0.9.2-rc.13`。未做故意 failed-update live rollback 注入；此 gate OPEN。GitLab `gitlab-company` 原 OAuth grant 重啟後普通 refresh **ready／35 tools**，未重授權／借用 Stable。

### Exactly one formal company Edge Host

正式事前預告使用者原生偵錯許可，僅執行**一次** `connect_external`（**4.926s PASS**）；一直沿用此 Core-owned TCP/CDP socket，沒有 short probe、Managed fallback、重連或 Profile 交換。

| Step | Host 結果 |
| --- | --- |
| Oprueba A `direct_acquire` | **PASS**，6.965s |
| HomiPlay B 獨立 `direct_acquire` | **PASS**，4.684s |
| B `direct_navigate homiplay.io` | **PASS**，5.326s |
| A `direct_release` | **PASS**，4.619s，cleanup complete／無 error |
| A release 後 B `direct_snapshot`，確認 HomiPlay | **PASS**，4.044s |
| B `direct_release` | **PASS**，5.065s，cleanup complete／無 error |
| **A+B 全部釋放後 Oprueba 新 `direct_acquire`** | **PASS**，5.630s — **RC11／RC12／RC13 真實 Host 長期 FAIL 的 fresh acquire gate 現已 PASS** |
| 第三個 lease `direct_navigate oprueba.com` | **PASS**，4.399s |
| 第三個 lease `direct_snapshot`，AX `RootWebArea`／O'Prueba | **PASS**，3.894s |
| 第三個 lease `direct_screenshot` | **PASS**，4.058s，確認有效 PNG **302,829 bytes** |
| 第三個 lease `direct_release` | **PASS**，4.127s，cleanup complete／無 error |
| 隔離的新 lease 嘗試 `direct_evaluate(() => 1)` | 腳本拒絕標 PASS：呼叫回傳 **`BROWSER_REQUIRED_ROUTE_UNAVAILABLE: external transport fence unavailable or mismatched`**，不是明確政策錯誤；未執行使用者 JavaScript，但政策拒絕呈現與 lease 健康 gate 仍 OPEN |
| 上述負向 lease `direct_release` | cleanup_state `complete`，但 cleanup_error 為 transport fence mismatch；**不能稱 clean release PASS** |
| 未核准 URL 負向 | **NOT RUN**，腳本於前一項驗收不符時停止 |
| `disconnect_external` | **PASS**，0.003s；前後 Browser `idle`，0 owners/leases/workers/active/queued/orphans/authenticatedTransportOwners；NEXT Core 對 Edge 9222 **無殘留 socket** |
| 網站焦點 | **184 foreground samples all `com.google.Chrome`**，瀏覽頁面腳本無搶焦點，未代按原生授權 |

實機 private machine-only 證據：`/private/tmp/agentdock-next-edge-verification-y6b6836m/rc15-edge-fresh-result.json`、`rc15-edge-fresh-foreground.jsonl`；其內容未納入 Git。

### Product follow-up

現在可由真實 Host 證明 **合法共享 Edge 連線在 A+B clean release 後可以繼續開第三個租約**，也能讀取 approved web 頁面與影像。下一個直接改善是 Company Edge 的**明確負向政策拒絕且不損傷健康租約**：應在 `ACPBridge.DirectCall` 核實 principal／lease owner 後、發起 CDP fenced operation 前拒絕 JavaScript 與未核准 origin，產生 `ACTION_INVALID` 並允許乾淨釋放。這次原安裝 RC15 並未具備該修正，因此不能把離線 source patch 當成 Host PASS。GUI／Tray、跨 session 故障恢復及 NEXT-only failed-update rollback 也仍 OPEN。保持 Stable 8765 untouched 與原版主控制能力；不得在剩餘 gate 未驗收前發布。
