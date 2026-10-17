# AgentDock Next — Orca Computer Use 本機部署與即時驗收（2026-10-18）

> 狀態：**Next-only local candidate 0.9.2-rc.3 deployed；Broker / Orca background observation verified**。不是正式發佈、Tag、公開 RC、Developer ID / notarization 或 P11 全面 closeout。
>
> 變更來源：`fix/next-orca-cli-discovery-20261017` 的 Orca 修正 `692e8eed`，**僅移植三個 Orca 原始碼／測試檔案**到目前已安裝 Next Core `fa279cd45f2e` 的基線。部署分支 `fix/next-orca-live-deploy-20261018`、建置來源 `35a69c7407873ba8bfbd8d84325013d9b377cf1e`。
>
> 前次原因分析：`docs/custom/next-orca-cli-discovery-2026-10-17.md` 位於獨立原始修正分支；Next launchd PATH 找不到 `/usr/local/bin/orca`，而原版可用 `exec_command` 直接呼叫。現有 `internal/commandpath.LookPath` 可補常見 macOS CLI 路徑；不變更原版 AgentDock。

## 使用者需求、可觀察改善與邊界

- 工作流程：先前 HomiPlay Home Center / Terminal 問題排查期間，AgentDock Next 的 `computer_session` 回 `COMPUTER_PROVIDER_UNAVAILABLE`，即使 macOS Orca CLI 1.4.224 真正可用。本輪修復後需要透過 **Next 自己的 Computer Control Broker** 呼叫 Orca，而不是繞回 stable `exec_command`。
- 最小實作：`internal/tool/computer/orca.go` 對純 CLI 名稱採用專案已存在的跨平台 `commandpath.LookPath`，保留含目錄的 `ORCA_CLI_COMMAND` 明確命令路徑；增加受限 launchd PATH 與實機無焦點操作的測試。
- 不做：不修改／重啟 stable AgentDock、不全域修改 PATH、不更改 Orca 版本或 macOS Accessibility / Screen Recording 權限、不操作 HomiPlay Terminal、不搶前景、不關閉使用者 App，不建立正式 Release / Tag、公開發佈或啟用自動更新。

## 避免版本回退

- 前置：已安裝 Next Core 版本 `0.9.2-rc.2`、source `fa279cd45f2e`，App version `0.9.2`、bundle build `3.0.0`。
- Orca 初始修正分支 `692e8eed` 並非 `fa279cd` 祖先／後代，不能直接用它完整建置回覆當前 Next，否則可能覆蓋已部署的更新通道與連線 UI 改善。
- 由已安裝 commit `fa279cd` 建立另一獨立工作樹，**只回填 Orca 三個檔案**，source commit `35a69c74`。
- `tools/release next-preflight` 產生 canonical candidate JSON：`0.9.2-rc.3`、`next-v0.9.2-rc.3`（識別字串，**未建立 Git Tag**）、bundle ordinal 4、完整 source SHA、source UTC date `2026-10-17T16:33:38Z`。自動更新 feed `disabled/unpublished`，`release_ready=false`，`mode=source-only-dry-run`。
- 使用 clean HEAD、已安裝的 Next cloudflared payload，執行 `zsh packaging/macos/build-next-shared.sh <existing-Next-cloudflared> --candidate-manifest <canonical-manifest>`，沒有用 Stable 執行檔作為 Next payload。

## 測試與建置

- `go test ./internal/tool/computer ./internal/commandpath ./internal/app -count=1`：PASS。
- `go test -tags=computer_integration ./internal/tool/computer -run '^TestOrcaBrokerBackgroundObservationSmoke$' -count=1 -v`：PASS，受限 PATH + real Orca 觀察不改焦點。
- `python3 -B scripts/test/test-next-shared-deploy.py`：23/23 PASS，包括候選版本降級／相同版本不同 commit 拒絕、rollback 和原版路徑排除。
- 真正 App build: `dist/next-shared-arm64.HozszB/package/AgentDock Next.app`；App / Core arm64、ad-hoc codesign、payload SHA、DMG verification、Next package test 9/9 PASS。
- 候選 Core `version --json` = `0.9.2-rc.3`、`35a69c740787`、`darwin/arm64`；候選 Bundle ID `dev.dropabit.agentdock.next` / CFBundleVersion `4.0.0`。安裝前 Next `0.9.2-rc.2`；P11 版本比較門檻不需繞過。

## Next-only 安裝與連線 UAT

- 從原版 Mac-Dev 控制通道執行 `PYTHONDONTWRITEBYTECODE=1 python3 -B packaging/macos/deploy-next-shared.py <candidate App>`。
- Deployment command **exit 0**，回傳 `NEXT_ONLY_DEPLOYED`；目前 `~/Applications/AgentDock Next.app` Core `0.9.2-rc.3`、`35a69c740787`，新 Next launchd Core PID `55637`、port `8767`，Tunnel PID `55830`。
- 先前原版 AgentDock Core PID `53083`、port `8765`，部署後仍相同。Stable App/registry 無改動。
- 舊 Next Bundle Contents 透過既有 rollback 保留在：
  `~/Applications/.agentdock-next-replacement-144vbbzw/AgentDock Next.app/Contents`。**不要自動清理**。
- Next GUI 未重啟，避免搶焦點；部署回報 `NEXT_GUI_RESTART_PENDING`。本次 Orca 功能由更新後的 Next Core 提供，無需為該能力立即重啟 GUI。
- 透過真實 `macbook-air-m3` Connector 執行：`computer_broker status` PASS、`computer_session acquire(capability=observe,foreground=forbidden)` PASS、`computer_observe(capabilities,no_screenshot=true)` PASS、`computer_observe(list_apps,no_screenshot=true)` PASS（13 apps）、`computer_session release` cleanup=complete。
- **一次焦點變更事件**：第一次 `computer_observe(capabilities)` 回 `COMPUTER_FOCUS_VIOLATION`，Broker 觀察前景從 Chrome 換成 ChatGPT；隨後同一個 Session 重試 `capabilities` 與 `list_apps` 均 `focus_changed=false` 且 `foreground_required=false`，故不能把首個事件直接認定為 Orca 主動切換視窗，也不能宣稱已排除外部並發 focus 競態。已保留 event 供後續觀察，未放寬安全阻擋。
- Next `agentdock_context` 版本 `0.9.2-rc.3`、Home `~/.agentdock-next`、七個 Managed Skills 全部保留。Memory MCP refresh `ready` 26 tools；CBM refresh `ready` 17 tools。
- Session 釋放後 Computer Broker 無 active sessions、released session cleanup complete。無任何實際滑鼠點擊／鍵盤輸入或前景視窗 UAT，因本次不應干擾 HomiPlay／其他使用者工作。

## 後續

1. 若需要完整 **Computer Act** 前景控制驗收，必須有明確前景允許、可安全測試的非使用者工作視窗與清理計畫；本次只驗證不搶焦點的 native observe。
2. 保留第一次焦點切換事件的追蹤資訊；只有在可重複證明 provider 主動切換前景時才擴大修改；不能因安全攔截而關掉 focus guard。
3. Next GUI 仍保留原本已開啟的 GUI Process，日後在合適時機可安全手動退出再打開驗收 GUI，不能假稱本輪有完整 GUI UAT。
4. 本機 source branch 的 ad-hoc package **不是**正式 Release、不代表 P10/P11 完成；應由 owner 在後續 code integration 中保留 `35a69c74` 的 Orca 修正而非直接用較舊工作樹整包覆蓋新功能。
