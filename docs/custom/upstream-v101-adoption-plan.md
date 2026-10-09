# AgentDock upstream v0.9.1 → v1.0.1：Next 選擇性採用計畫

> Status: PROPOSED / review only；建立於 2026-10-09。此文件不是 M9 closeout、不是 upstream merge 指令、也不是已完成原生驗收的證明。
>
> Source: `uvwt/agentdock` **遠端正式 tag** `v0.9.1..v1.0.1`（GitHub compare: 51 commits、263 files）；`v1.0.1` tag object `447d738b...` 解參照為 commit `255000078e399be2fdff986ae65d4c24290425b9`。本機歷史同名 tag 有不同物件，**不可**在未確認遠端正式 tag 前使用本機 `git diff v0.9.1 v1.0.1` 作結論。
>
> Next 對照基線：`feature/m8-permission-approval` 的 `5b436f3d`（2026-10-09），不代表部署中 Next App 已包含該 commit；與 P8 工作線並行時須重新檢查狀態。

## 1. 判斷與採用範圍

| 上游能力 | Next 實情 | 決策與時機 |
| --- | --- | --- |
| Windows generation / active state 回復、安裝更新一致性 | Next 已有 updater/arbiter/trial/rollback，缺 release-native fault injection | **ADOPT TEST CONTRACT**：M9/P11 對現有實作補驗，不盲目 cherry-pick WinUI/Windows installer |
| Git tag 不可變 release-source、跨平台 build gates、先驗後發 | Next 的 `.github/workflows/release.yml` 仍要求 `origin/main`；`internal/buildinfo/buildinfo.go` 仍 `const Version = "0.9.1"` | **ADOPT/REIMPLEMENT**：M9/P10 重新建立 Next 專屬版本、來源及更新通道 |
| cloudflared Component Store / Catalog / 原子 active pointer / legacy import | Next cloudflared 仍是 App Helper 且使用 Next signer/identity 驗證 | **REFERENCE → SELECTIVE PORT**：Release 後獨立 milestone，先協調安全／packaging 契約，不直接複製 |
| macOS service lifecycle、Windows WinUI UI | Next 已採 Shared Vue/Wails + native platform adapters | **REFERENCE**：只選生命週期與互動可用性測試，不重建雙份原生 Business UI |
| 觀測／tool stage timing／OpenTelemetry | Next 的 `internal/observability`、`internal/app/runtime_observability.go` 已有相關後端 | **ALREADY PRESENT**：僅評估 Execution Center 的欄位／隱私 UX，不重複移植 |
| Nexus pairing | 使用者要求延後真實配對 | **DEFER**：本次不操作配對、不擴大範圍 |
| **PR #229（不在 v1.0.1 tag 內，尚未 merge）** Task Progress live-card | Next 仍只有 `task_manage`，MCP Apps 會重複掛 Task UI/新狀態不更新舊卡 | **SELECTIVE ADOPT（P2 排程 / 高價值 UX）**：獨立於 P8 導入，需 snapshot scope/權限、UI 快取與真實 ChatGPT UAT；[`詳見`](upstream-ports/task-progress-live-card-pr229.md) |

**採用執行索引**：正式 Roadmap 採用矩陣位於 [`roadmap.md`](roadmap.md)；此處保留來源、實作與安全 Gate。PR #229 是尚未合併的獨立 PR，不應計入 v1.0.1 tag 的變更。

## 2. 不可破壞的不變式

1. 舊版 `mac-dev` 是唯一允許 Next repo/build/install/service mutations 的控制連線；`macbook-air-m3`（Next）不得自我更新／重裝。stable App/Core、`~/.agentdock`、port 8765、Memory 8766 都不碰；Next Core 8767、Tunnel/公網域名/OAuth/connector、state/log/registry/launchd labels 不可串線。
2. Next macOS 現有 `internal/desktopruntime/next_identity_darwin.go` 要求 `AgentDock Next.app/Contents/Helpers/{agentdock,cloudflared}`、相同 Helpers 目錄、Next 專屬 identifier，並檢查 plist/manifest。`internal/selfupdate/desktop_update_darwin.go` 同樣將內建 cloudflared 視為完整 App 必要內容。**不能只搬動 binary 路徑**。
3. Release 執行前沒有有效 Developer ID/certificate-bound updater、簽章／公證與相容 UAT，就不可宣稱 M9 完成或發布正式更新。
4. 不默默改寫用戶 Tunnel token、Cloudflare Named Tunnel、OAuth 密碼，不從任意 `$PATH` 自動匯入可執行檔；不將 secret 寫入診斷、CI artifact 或更新事件。
5. Release tag/更新通道不得共用官方 `uvwt/agentdock`；不可修改 `main` 使其不再能與 upstream 對齊，客製工作走 `custom/main` 專屬流程。

## 3. M9 / P10：發行契約與實作順序

**M9-R1：版本與來源契約（進入 release 工程前凍結）**

- 決定 Next tag namespace、pre-release/stable channel、semver/prerelease 正規化、Windows 4-part version、App Info.plist/Installer/Core/Web UI/update metadata 映射。不得默認沿用官方 `v*` tag／URL／bundle id。
- 改掉單純編入 `const Version = "0.9.1"` 的限制：讓 release pipeline 能可靠寫入 **tag + 完整 source SHA + build timestamp + channel**，並能以相同資料讀回驗證；正式標籤內容與 commit 必須不可變。
- CI `release source` 僅信任 Next 的 `custom/main` 祖先及該 tag 指定 SHA。分支未合併、重用已公開 tag、tag/source/buildinfo 不一致、出現 upstream release namespace 時 fail closed。
- 明確區別 source version、已安裝 bundle version、執行中的 Core version、預備更新版本；installed ≠ running 時 UI/health/report 應標出 restart/recovery-pending，不假稱已升級。

**M9-R2：矩陣與制品完整性**

- Core + Desktop API + Shared UI + i18n + Windows cross-build + Linux helper 僅是 build gates；再補 macOS arm64/x86_64、Windows amd64/arm64 正式安裝產物。缺少任一要求平台的原生證據，不能把 cross-build 當 UAT。
- Bundle/installer/updater 使用 Next 專屬簽章信任鏈、package identifiers、update endpoints；產物 SHA-256 與 signer/commit/version/channel provenance 寫入 release manifest。
- Release Candidate 先 build → verify → sign/notarize → isolated install/upgrade/recovery → review；僅所有阻塞 gates 過關才 promote 至公開 Release。發布後資產/manifest 不得靜默覆寫。
- Component Catalog/R2 不列為 M9 必要依賴：除非另有需求，先不要因上游使用 R2 而引入獨立儲存服務。

**M9-R3：Windows v1.0.1 特定借鑑**

- 取上游 `internal/selfupdate/generation_windows.go`、`legacy_migration_windows.go` 與測試設計為參考，測「active committed generation vs. stale journal」與「舊 Core／新 UI 或相反」；不要直接導入上游 WinUI Runtime Bootstrap，因 Next 使用 Wails。
- recovery 真值來源必須只有一套：不可因過期 journal 回滾健康 active；不能用 CLI exit code 直接覆蓋已確定的 terminal result。

## 4. M9 / P11：故障注入與正式 UAT 清單

| Case | 操作 | PASS 判準 |
| --- | --- | --- |
| U01 | RC 從不受信任分支/重用 tag/版本不符觸發 | 發行前拒絕；原發布資產不變 |
| U02 | macOS bundle 簽章、Helper identifier、notarization 或 artifact SHA 被竄改 | fail closed；無 service 替換 |
| U03 | updater 在下載／stage／解壓／commit 任一點中斷 | active version 與 rollback slot 可解釋；恢復後仍可啟動 |
| U04 | 試用新 Core `healthz` 逾時或版本回報不符 | 回復舊版本；明確記錄 recoverable cause |
| U05 | Core 與 GUI build/version 不一致 | 顯示不一致與操作建議，不宣稱更新成功 |
| U06 | Windows stale journal + healthy committed active generation | 不因 stale journal 回滾健康版本 |
| U07 | Windows installer 失敗／權限不足／runtime bootstrap 失敗 | 不留下偽健康、卡死工作或多個執行 generation |
| U08 | Next-only app/core/tunnel 更新及失敗回復 | stable `mac-dev` Core PID、port 8765、state 與 MCP 通道完全不變 |
| U09 | 更新時 Next Tunnel 有既有 Named token、OAuth/公網連線 | 回復後 Token/Hostname/connector 沿用；secret 不出現在 log |
| U10 | Windows/WSL/Linux helper 在實際平台執行 | 有真實原生結果，缺席時只報 external gate，不假標 PASS |
| U11 | RC/pre-release 與 stable metadata 混用 | 版本通道不交叉；不誤啟動 stable auto-update |

執行 UAT 前建立 disposable Next test-root/test identity，不使用目前線上 Next Tunnel/connector 進行破壞性故障注入；如需最後 live smoke，需另列明確且安全的操作窗口。

## 5. Component Manager：延後導入但先建立合約

完整源碼與保護條件見 [`upstream-ports/cloudflared-component-manager.md`](upstream-ports/cloudflared-component-manager.md)。

**不能直接在 Pre-M9 啟動實作**：Next Bundle/自簽 Helper 與上游的外部 Cloudflare-signed binary 為不同安全模型。先關閉 P8/AGY 整合、完成 M9 的正式發行／回復驗證，再使用獨立 worktree 走 Component-0～4 的 staged migration。

## 6. 不納入本次採用的工作

- 不快進 `main`、不 merge/cherry-pick upstream 全量 51 commits、不覆寫相同名稱的舊 tag。
- 不在此分支改程式碼、CI workflow、執行中的 App 或 cloudflared。
- 不改原生 WinUI/WPF Business UI；不另造 observability recorder。
- 不擴大到 Codex ACP / AGY ACP 的元件安裝器；.14 AGY-ACP 由其既有 session 整合驗證。

## 7. Source references

- 官方版本差異：<https://github.com/uvwt/agentdock/compare/v0.9.1...v1.0.1>
- 1.0.1 Tag commit：<https://github.com/uvwt/agentdock/commit/255000078e399be2fdff986ae65d4c24290425b9>
- 上游元件與安全：`internal/component/cloudflared.go`、`catalog-v1.json`、`trust_{darwin,windows,linux}.go`、`internal/desktopruntime/cloudflared_component_darwin.go`
- 上游發行：`.github/workflows/release.yml`（`Validate immutable release source`）、`internal/selfupdate/generation_windows.go`
- Next 影響點：`internal/desktopruntime/next_identity_darwin.go`、`internal/selfupdate/desktop_update_darwin.go`、`packaging/macos/next_shared_bundle.py`、`internal/desktopapi/connection_integration.go`
- 現有既定任務：[`roadmap.md`](roadmap.md) M9、[`pre-m9-feature-parity.md`](pre-m9-feature-parity.md) P9–P12、[`agentdock-next-isolation.md`](agentdock-next-isolation.md)
