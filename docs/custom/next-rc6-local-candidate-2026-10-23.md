# NEXT RC6 本機建置、授權安裝與首次 Edge 驗收

狀態：**本機候選建置及 NEXT-only 安裝 PASS；公司 Edge 連線驗收 FAIL CLOSED，四站尚未執行。**
不是完整 RC Definition of Done，也沒有公開 Release、Tag、push 或啟用更新通道。

## 候選與來源

- Source：`cce96a3d0ee9d88404d7223dbda1a14d4e713e88`，建置前工作樹乾淨，
  `feature/next-edge-rc-20261023`。`custom/main` 仍保持 `7e87605a`，沒有提前整合未完成 UAT 的功能。
- Core `0.9.2-rc.6`／`cce96a3d0ee9`／`2026-10-22T23:46:52Z`；arm64；
  App `0.9.2`／bundle ordinal `7.0.0`；ID `dev.dropabit.agentdock.next`。
- 候選資料夾：`dist/next-shared-arm64.uWWgWy/package/`。
  App／ZIP／DMG 來自同一 manifest，ad-hoc 簽章及 real bundle suite **9/9 PASS**。
- ZIP SHA-256：`a2b5e18e10c2385b2a9190adeaf3362cff3ba9b3366a6fb1af32d9def36a85bd`。
- DMG SHA-256：`955a3d93910d18d42e1329718f9697171755d31a01afb86b956756928b752b76`。
- ZIP 解開及 DMG 唯讀、no-browse 掛載後，App 的 **24 個檔案**逐一比對候選 SHA 相同；
  已卸載本工作建立的掛載點。DMG checksum valid。
- `tools/release` **70 PASS**；部署 orchestration 離線 **29 PASS**；實際候選的版本門檻
  接受 RC5→RC6、拒絕 RC6→RC5。不是故意破壞 live Core 的 rollback 驗證。
- 候選 Core 在臨時 private Home 的四個 bundled Core Skills bootstrap 兩次成功，
  **15 檔案相同、idempotent**，臨時 Home 已移除。

## 授權與真正安裝

使用者明確回覆：「允許，確認 NEXT 閒置後安裝與驗收」。從 Stable Mac-Dev 控制通道執行
既有 `packaging/macos/deploy-next-shared.py`，沒有修改／重啟原版。

安裝前即時查核：NEXT `0.9.2-rc.5`、正確 private home、Broker owners/leases/workers/
active/queue/orphans 全 0、execution active_calls=0；Codex 與 Antigravity 的
ready/running/loaded 全 0，無 pending interaction/session operation。候選來源仍為乾淨的 cce96a3d。

helper 完成 App Contents atomic replacement、Next Core/tunnel 註冊、PID/binary/listener/
health 驗證及 NEXT bundled Skills bootstrap，回報 `NEXT_ONLY_DEPLOYED`。
已安裝 binary hash 與候選相同，App 根 inode 保留；**26 個原有 Skill 檔案 SHA 完全相同**。
原版 8765 仍回覆 `1.1.0`。保留的新 RC5 rollback bundle：
`/Users/wei/Applications/.agentdock-next-replacement-ltk7a_b_/AgentDock Next.app`。
沒有清理更早的備份或歷史工作樹。

## 公司設定與連線結果

安裝完成後才以 owner-only 0600／exclusive publication 寫入
`~/.agentdock-next/browser-routes.json`，內容等於 [核准設定範例](next-company-browser-routes.example.json)，
SHA `497abd178c0520f4656b1e737a8747176318b442af819f3ece397678dc3341d5`。
因此 App 更新失敗時不會讓舊版 rollback 必須解析新 schema。再次只註冊 Next Core 載入設定，
PID `18019→20273`；snapshot 正確回報 1 connector、1 authenticated Edge profile、4 company
policies、idle、0 leases；connector health 仍為 not_observed。此步不建立 Edge 連線。

首次透過已安裝 NEXT 的 `browser_broker connect_external` 執行正式 native Runtime，
約 **23.473 秒**後回覆 `BROWSER_REQUIRED_ROUTE_UNAVAILABLE`／external peer verification
unavailable or mismatched。RC6 的 error 丟失了具體驗證階段，**不能推論為使用者未按允許**，
也不能推論是特定 Profile、TCP 或 CDP 步驟故障。尚未開始四站導覽。

已停止重試；下一個 source 修正只增加 finite、無 raw error/憑證的 connection failure stage，
讓下一次正式驗收能定位，並保留所有 fail-closed 與 cleanup。不得靠重複短 probe 替代正式路徑。
GUI 仍待安全背景重開後的 native UAT；本記錄不宣稱已完成 GUI 版本驗收。

## 私有證據與續作

`/private/tmp/agentdock-next-edge-verification-y6b6836m/` 保存 canonical candidate manifest、
artifact inventory、archive hashes、isolated bootstrap、部署前/後狀態、安裝 log、路由載入及
實際連線失敗摘要。不含 Token；NEXT-only client 每次從 NEXT 自己的 service env 讀取認證，
沒有複製原版 OAuth／Token。原始工具失敗由 Stable command session
`session-16bb4061e64430d236fc635f` 記錄，已退出。

下一步：完成 bounded failure-stage 修正、測試並建立可追溯候選，沿原有 NEXT-only
修正／驗收範圍持續定位；不修改 Edge 或公司資料。GitLab 的獨立 OAuth metadata 可讀，
但尚未取得 NEXT grant 或驗證 repository/commit/pipeline/job/MR 的真實能力。
