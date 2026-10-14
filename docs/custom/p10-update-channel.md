# P10 Next update channel：disabled/unpublished

本文件記錄 source contract，對應 [P10 release preflight](p10-release-preflight.md)。僅完成 channel guard 準備，P10/M9 未 closeout，沒有 updater 成功發布或原生更新／回滾證據。

## 工作與驗收範圍

- 使用者痛點：Next 的一般 update check/apply 可能查詢 upstream Stable Release，造成版本、destination 或 archive 類型混用。
- 直接改善：Next 在 HTTP、下載、staging 或 apply 前取得明確 disabled/unpublished 原因；不會把 Stable Release 當 Next 的更新。
- 最小實作：沿用 `internal/selfupdate` 的 runtime options 與共用 inspection，增加 `channel.go` 拒絕政策；保留 Stable feed、資產選擇、版本比較與更新執行器。
- 可觀察驗收／非干擾：in-memory HTTP transport 計數為零、apply/verify/extract 計數為零、既有 isolated release tests、diff check。不操作已安裝 App、服務、瀏覽器、state、profile 或 OS consent。
- 不做：feed 啟用、URL/token override、publish、Tag、Release、commit/push/deploy、UI/CI 修改、P11 原生 update/rollback UAT。

## 精確 source contract

`validateUpdateChannel(options)` 先透過 `updateidentity.Resolve` 讀取 `AGENTDOCK_DESKTOP_VARIANT`：只有空值、`stable`、`next` 有效，未知值（含大小寫或空白錯配）直接失敗。空值／`stable` 僅在沒有下列 Next 訊號時保留歷史 Stable semantics：

1. marker 是 `next`。
2. Core 或 desktop version 在 trim/lowercase 後以 `next-` 開頭或含 `-rc`。這是防 fallback 的保守辨識，不是 SemVer／候選有效性驗證；不完整 RC 也不能進 Stable feed。正式候選格式仍由 preflight 驗證。
3. executable 或 desktop target/runtime root 的路徑 component（忽略大小寫，接受 `/`、`\` separator）為 `AgentDock Next`、`AgentDock Next.app`、`AgentDock-Next`、`AgentDockNext` 或 `.agentdock-next`。

任何 Next 訊號優先於空值或 `stable` marker；混合 Stable destination 的 RC 也拒絕，不猜測 channel。未知 marker 不會因測試 option 被放行。這些訊號只用來拒絕，不能證明 bundle 簽章、runtime ownership 或 release entitlement。

Next policy 固定為 **disabled/unpublished**，沒有 feed URL。錯誤明示沒有核准的 Next feed，指引使用 P10 release preflight 並等待授權的 Next Release，說明 Stable releases 不能更新 Next。不回報「已是最新版」或成功。

入口與執行順序：

- `runtimeOptions` 在 caller/bundle 驗證及 desktop discovery 前檢查 executable（symlink resolve 前後）與 build version，供 `Check`、`Run`、`RunWithProgress`、Windows `RunLocalArchive` 使用。
- `CheckForRuntime` 在 managed Core discovery/fallback 前檢查 marker、shell build version 與 runtime root；`runtimeOptionsForTarget` 在 caller 檢查前檢查原始路徑，並在讀取目標 Core version 後再次檢查。
- `newOptions` 對拒絕的 channel 將 `ReleaseAPI` 清空；`run` 與 `inspectUpdate` 仍重新驗證，拒絕發生在 `fetchLatestRelease` 前。手動提供 custom `ReleaseAPI` 不會解除 Next 禁用。
- Windows local archive additionally 檢查 target version，拒絕 RC/Next 目標後才會讀取 archive/checksum。未新增 installer/updateengine/rollback 行為。

P10 macOS artifact metadata 已與 canonical shell 的單架構名稱對齊為 `AgentDock-Next-macos-arm64.zip`；Stable 仍保留 `AgentDock-macos-universal.zip`。這只更正 source identity 字串，channel guard 仍在 archive 選擇與 HTTP 前拒絕 Next，不啟用 feed 或放寬 identity validation。

因此禁用期間所有 Next 平台均無法落入 Stable Core archive 選擇／替換路徑，亦無法把 Next RC 本地 payload 寫入 Stable destination。macOS 既有 `DesktopOnly`、Next installation namespace、`AgentDock-Next-macos-arm64.zip`、bundle/signature 與 ownership 檢查保留；不增加 Windows/Linux Next archive 映射，也不把 Setup EXE 當 Stable ZIP/TAR 更新。其他 lower-level engine/repair/internal recovery 路徑未改造，本政策不是 P11 的全面原生更新授權層。

Stable 在空值／`stable` marker、無 Next 訊號時仍使用原 `defaultReleaseAPI`（`https://api.github.com/repos/uvwt/agentdock/releases/latest`）；custom isolated Stable ReleaseAPI 測試不變。

## 明確隔離的既有 fixture

只有 package-private `options.nextChannelFixture=true` 且 ReleaseAPI 為無 userinfo 的 `http://127.0.0.1:<port>` 或 `http://[::1]:<port>` 可進既有 Next macOS archive-selection fixture。runtime constructor 從不設定此值，沒有 exported API、CLI flag、environment/token bypass，也沒有自動以 custom URL 判定測試。

`TestNextReleaseRequiresNextArtifactWithoutFallback` 明確使用此 option，保留對 Next archive 成功選擇及 Stable archive 拒絕的離線證據。這是可信任測試程式的注入邊界，並非 production feed／redirect security contract；不授予任何 live update entitlement。

## 未來啟用前提

須由 release owner 另行授權並提供可 review 的獨立實作，才能解除禁用：

1. UIUX 凍結及 P10 owner integration，clean immutable SHA、canonical RC version／`next-v` tag、同 commit metadata／manifest 與實際 artifact 證據。
2. 明確 Next-only feed 的來源、channel/tag/schema 與 URL/redirect policy；證明不能 fallback upstream Stable。不能僅增加任意 URL 或 token env override。
3. 各平台 destination/ownership、archive 類型與 payload identity 的 fail-closed binding；獨立 Next 安裝名稱、state/services，不沿用 Stable ZIP/TAR/Setup 類型。
4. 每項 artifact 的 checksum、簽章／certificate continuity、完整 metadata/provenance 與平台建置證據（P10 Next macOS 僅支援 arm64，不要求 Intel / universal）。
5. P11 另行授權的原生 update → trial → commit、rollback/interruption/recovery 與非干擾 UAT。source fixtures、cross-build 或本文件不能取代它們。

最接近的下一個流程是 owner 整合 channel guard 與 Windows packaging source contract，再核對 clean SHA 候選 artifact；目前仍等待授權的 Next feed 與上述 activation gates。

## 2026-10-14 worker 驗證紀錄

執行一次 focused command，sandbox loopback bind 失敗後，以相同 command 取得權限重跑：

```sh
rtk proxy env GOCACHE=/private/tmp/agentdock-p10-update-gocache go test ./internal/selfupdate \
  -run 'Test(NextChannel|UpdateChannel|NextFixture|InspectUpdate|Run|NextRelease|NextArchive|PlatformAssetNames)' -count=1
```

- 首次：`TestNextReleaseRequiresNextArtifactWithoutFallback` 的 `httptest` 因 `listen tcp6 [::1]:0: bind: operation not permitted` panic，中止；不能將該輪視為 PASS。
- 權限重跑：整輪 **FAIL**（4.122s）；唯一回報失敗為既有 `TestRuntimeOptionsForTargetReadsManagedCoreVersion`，`ExecutablePath` 實際 `/private/var/folders/...`，預期 `/var/folders/...`。沒有更改該測試或重跑以掩蓋此結果。
- 同一權限重跑中其餘選定 tests 未報失敗：新增 `TestNextChannelBlocksCheckAndApplyBeforeHTTP`、`TestUpdateChannelRuntimeSelection`、`TestNextFixtureCannotSelectExternalFeed`，既有 `TestNextReleaseRequiresNextArtifactWithoutFallback`、`TestNextArchiveRejectsStablePayload`、`TestInspectUpdate*`、release `TestRun*` 與 `TestPlatformAssetNames`。這些 source/fixture 結果不能改寫整輪 FAIL，也不是 updater 成功 Release。
- `git diff --check` PASS；沒有全套 Go tests、hosted CI、跨 OS 執行、原生 update/rollback、App/service 操作或 feed activation。依 owner 收斂指示不再編譯／重測。
