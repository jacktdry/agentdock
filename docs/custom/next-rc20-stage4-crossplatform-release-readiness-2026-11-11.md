# AgentDock NEXT RC20 — 階段四：跨平台、回歸與 Release Readiness（2026-11-11，台灣）

> **2026-11-11 補驗：**本文件保留當時 RC20 原始七項 A1 `-race` **FAIL**，不能因後續獨立臨時 Go overlay PASS 而改寫歷史。較新[H04 Root Cause](next-rc20-h04-h01-h02-root-cause-2026-11-11.md)確認 1MiB Race decode/pipe 與 1 秒 watchdog 耦合，另一個 deadline Test 亦偶發先超時；兩個 fixture 在臨時 overlay 中使用 128 KiB（queued 加實際阻塞斷言）後，focused 七項 `-race -count=10` PASS，**尚未合入 signed source**。

## 目的、使用者價值與操作界線

使用者要求在**不用 Codex ACP、不打斷既有 macOS Original/NEXT**的條件下，完成剩餘跨平台建置、套件和發行門檻驗證，與接續的階段五工作包。這是一份**本機 release preflight 證據**，不是公開發佈准許，也不假設有 Windows 受測裝置或雲端 CI 已跑過本機尚未推送的 commit。

所有指令皆由 Mac-Dev 在獨立 clean compat 工作樹或 immutable signed RC20 candidate 工作樹執行。只允許 source-only `go build`、`go test`、Python package fixture、Vue/Vitest/typecheck、`codesign`/ZIP/DMG 驗證、Git 遠端唯讀查詢；產物置於獨立 `dist/stage4-20261111-source-only`（gitignore），**不執行跨平台二進位檔、不安裝新 App、不調整 Core/GUI/Tunnel、M8、Browser/Computer、Profile 或憑證、不 Merge/Push/Tag/Release**。

## 1. 基線與簽章來源

- 來源 RC20 候選：`feature/next-a1-rc20-20261105` HEAD `704bb40d08f6144513791a9e1854cd2c9a799b73`，clean，原候選 `dist/next-rc20-candidate.json` 經 `go run ./tools/release next-artifacts` 的 verify 嚴格比對。
- 現役 `~/Applications/AgentDock Next.app`：Bundle `21.0.0`，Core `0.9.2-rc.20` commit `704bb40d08f6`，macOS arm64。Original Core v1.1.1 走 port `8765`、NEXT 走 `8767`，本輪測試前後兩者 `/healthz` HTTP 200。
- 候選 Manifest `schema=agentdock-next-candidate-v1`, `mode=source-only-dry-run`、`channel=next-rc`、`candidate_tag=next-v0.9.2-rc.20`、`required_core_commit=704bb40d...`、`cfbundle_version=21.0.0`、`windows_version=0.9.2.20`、**`release_ready=false`、`update_feed=disabled/unpublished`**。

## 2. 跨平台編譯的**本機真實證據**（沒有在目標 OS 執行）

利用 `CGO_ENABLED=0 GOOS=<target> GOARCH=<target> go build -mod=readonly -trimpath -buildvcs=false -o dist/stage4-20261111-source-only/<name> ./cmd/agentdock|./cmd/agentdock-arbiter`。每個成功輸出以 `file`、`go version -m` 確認平台與格式。**編譯檢查 Core/Arbiter，不等於 Wails Windows/macOS GUI、Windows Setup 或跨平台 Browser/Computer 正式 Host。**

| Target | Core `cmd/agentdock` | Arbiter `cmd/agentdock-arbiter` | 可聲稱之證據 |
| --- | --- | --- | --- |
| macOS / arm64 | **PASS** | **PASS** | 本機 Mach-O arm64；已安裝 CGO/Wails GUI 另有原 signed artifact，不以 CGO0 重建替代 |
| Linux / amd64 | **PASS** | **PASS** | ELF x86_64 source-only cross-build；沒有 Linux GUI/Host |
| Windows / amd64 | **PASS** | **PASS** | PE32+ x86_64 source-only cross-build；沒有 Windows-native install/Host |
| Windows / arm64 | **未完成／timeout** | **未開始** | Core 在本輪批次執行上限內沒有產出檔案；批次超時後程序確認沒有殘留；不把它標 PASS。依 2026-10-14 使用者決策，Windows 為 source-only feasibility，**不阻擋 macOS-focused P10** |

實際已完成的 6 個 Core/Arbiter 平台輸出都由本機 Go Build 建置且未執行。若未來要支援 Windows arm64，請在可接受的編譯資源與隔離機器上獨立再做，不應因此對 macOS P10 添加無限測試支線。

## 3. Stage4 測試矩陣（每項區分可見範圍）

| 驗收 | 本輪結果 | 證據及限制 |
| --- | --- | --- |
| Release contract/identity Go tests | **PASS** | `go test -mod=readonly -count=1 -timeout=100s ./tools/release ./internal/buildinfo ./internal/updateidentity`；Release/UpdateIdentity 通過，buildinfo 是 no test files，不冒稱有測試 |
| Windows NEXT source/packaging negative fixtures | **PASS（18 tests, 1 skipped）** | `python3 scripts/test/test-next-windows-packaging.py -v`；選用 crossbuild fixture 略過，實際 Windows amd64 Core/Arbiter 已另交叉編譯；不等於 Inno Setup 可發布 |
| macOS NEXT signed package/legacy compat fixtures | **PASS（20 tests）** | `python3 scripts/test/test-next-shared-package.py <signed-RC20-app>`，同時 validate 真實已簽章 RC20 App；負向 fixture 的 `sealed resource missing/invalid` 是預期拒絕 |
| Vue 共享 UI 完整回歸 | **PASS（25 test files，184 tests）** | 在 signed RC20 source `pnpm --dir desktop/shared-poc/frontend test`；包含 ACP、MCP、Browser、Connection、Overview、Permission、Update 等；這是 source-only，不是 GUI Host |
| Vue TypeScript 靜態檢查 | **PASS** | 在 signed source `pnpm --dir desktop/shared-poc/frontend typecheck`（`vue-tsc --noEmit`），candidate Git clean |
| ACP A1 bounded pipe cancellation source race tests | **局部 FAIL（可重現）** | 在 signed RC20 source 執行 `-race` 7 項 Test，**只有** `TestQueuedWriteTimeoutPreservesActiveWrite` 因 `write_timeout_test.go:145` 1 秒 watchdog `ACP operation remained blocked for more than one second` 失敗；另六項先前執行均未失敗。下方分離重測提供較準確的分類，不得當作整體 -race PASS |

### A1 Race 重現與狀態界線（不能因重試 PASS 而洗掉 FAIL）

對完全相同的 signed RC20 source `704bb40d`、`internal/acp/write_timeout_test.go`：

- 先跑 `go test -mod=readonly -race -count=1 -timeout=140s -run '^Test(BlockedWriteHonorsRequestContext|QueuedWriteTimeoutPreservesActiveWrite|AlreadyCancelledWriteDoesNotTouchHealthyConnection|WriteFailureClosesFramingAndKeepsClassification|CloseReleasesBlockedAndQueuedWrites|CancelledRequestDoesNotWaitForCancelNotification|BlockedNotifyHonorsWriteDeadline)$' ./internal/acp`（7 項）：`TestQueuedWriteTimeoutPreservesActiveWrite` 在 `write_timeout_test.go:145` FAIL，1 秒 watchdog 超時；整體命令 exit 1。當時剛完成多個 Go 交叉編譯，負載可能影響排程，**但尚不能歸因於負載**。
- CPU 負載降低後，單獨 `go test -mod=readonly -race -count=3 -timeout=120s -run '^TestQueuedWriteTimeoutPreservesActiveWrite$' ./internal/acp`：**3/3 FAIL**，各在相同約 1.24–1.40 秒的 watchdog 檢查失敗（exit 1），證明不是單次偶然排程。
- 同一測試**不加 `-race`**、`-count=3`：**3/3 PASS**（Go suite 0.780s）。其餘六項 A1 bounded write test 使用 `-race -count=2`：**PASS**（Go suite 5.918s）。所以失敗範圍目前聚焦**Race Detector 下 1 MiB active frame、pipe drain/decode 完成時間與 1 秒 test helper deadline**；尚無診斷可確認是測試 watchdog 太短、Race-instrumented JSON decoder 負載，還是真實 backpressure/stall defect。
- **未修程式、未延長測試 timeout 來刷綠燈。** 下一個可交付的 P1 必要回歸為 `H04`：先增加**非侵入式精確時間與 frame 完整性證據**，區分活躍寫入真的卡死 vs 1MiB race decoder 的遲延；如果只是 fixture scheduling 應修復 fixture **保留相同語意**、不能弱化 frame/cancel/reuse fail-closed；如果是真正 pipeline defect 要做專門 Core 修復。後續重跑 `-race -count=10` 與 installed Host，並保護 Original。見 [Stage5 Work Packets](next-rc20-stage5-codex-resume-workpacks-2026-11-11.md)。

## 4. macOS signed artifact 與回滾安全

使用真正 signed RC20 檔案，`shasum -a 256 -c` sidecars、`unzip -tqq` ZIP CRC、`hdiutil verify` DMG UDIF 都 **PASS**；`go run -mod=readonly ./tools/release next-artifacts --manifest <RC20.json> --root <signed-package-dir> --platform darwin/arm64 --verify <inventory.json> <ZIP> <DMG>` **PASS**。完整 inventory 已保存在本地忽略目錄 `dist/stage4-20261111-source-only/rc20-macos-available-artifact-inventory.json`。

| 已驗證成品 | byte size | SHA-256 |
| --- | ---: | --- |
| `AgentDock-Next-macos-arm64.zip` | 31,999,539 | `fd9ff3866dc7716ff5e8ef0c6b5073f78249474bfb744fdb384bbb917a08b824` |
| `AgentDock-Next-macos-arm64.dmg` | 36,750,589 | `4d7768d54eeaace032f6ee856754d1064cdb55d2660792e565dbc93bfecbef2e` |

- 使用 Python 從 `signed RC20 source App/Contents` 與 `~/Applications/AgentDock Next.app/Contents` 的 **26 個 regular files** 完整比較 path + SHA-256，**26/26 exact match PASS**。目前 App 與原 RC20 來源都 `codesign --verify --deep --strict` PASS；**簽章類型為 `Signature=adhoc`, `TeamIdentifier=not set`**，不是 Apple Developer ID／Notarization。
- RC19 回滾 `~/Applications/.agentdock-next-replacement-iwp9x6vw/AgentDock Next.app` 可見、`0.9.2-rc.19`／Bundle `20.0.0`，**codesign deep strict PASS**；在 11/11 的 RC20 安裝時已保留此副本，**本輪未中斷 NEXT 做實際 rollback→redeploy**。P11 install PASS，**對當前 RC20 實際 rollback/recovery Host OPEN**。
- 候選 JSON 中仍含歷史字串 `macos-final-arm64-artifacts-not-verified`，此條對這**份本機已存在的 ad-hoc RC20 App／ZIP／DMG**已有後補的本機驗證證據；**不得直接竄改既有 canonical signed candidate manifest**。Hosted Mac CI／正式 Release approval 仍 OPEN，`release_ready=false` 仍然正確。

## 5. GitHub 工作流與正式發行門檻（只查本地程式及遠端 refs）

| 檢查點 | 實際來源/狀態 | 結論 |
| --- | --- | --- |
| Next 候選 CI | `.github/workflows/next-release-preflight.yml` 只在指定檔案 PR 異動觸發，`candidate-core` matrix 為 `macos-15 / darwin arm64`；其中 source-only candidate smoke 使用**固定示例版 `0.9.1-rc.1`、build 1** | 工程測試契約存在，但**未核對曾有 RC20 commit 的 hosted macOS Runner 執行**，不代表本地 RC20 正式 CI PASS |
| 普通 CI/CodeQL | `.github/workflows/ci.yml` PR 目標 main／手動；`codeql.yml` main push／PR／週期 | 不能把其他 branch/repo 的普通 CI 認成此次 RC20 hosted CI |
| Stable Release | `.github/workflows/release.yml` 對 `v*` Tag／手動發布，不是 `next-v*` Candidate | **NEXT 專屬出版、簽章、公證與授權工作流尚未建立或通過**；絕不可試著用 Stable Release workflow 發 NEXT |
| Windows NEXT Installer | `packaging/windows/NextAgentDock.iss` 檔案首段明確 `#error Next Windows installer disabled` | Source metadata/negative fixtures PASS，但 Windows NEXT 不可出 Setup；依 10/14 決策不是 macOS P10 blocker |
| Live remote refs | 唯讀 `git ls-remote origin`：`custom/main` `4a6c24af`；未查到本機 RC20 candidate／compat 分支或 `next-v0.9.2-rc.20` 標籤 | 本輪沒有 Push/Tag/Release；本地 compat HEAD 在完成本輪文件提交前已比遠端多約 **369 個提交**。遠端同步是**另案審查與授權**，不能當直接 Push 幾個檔案 |
| 正式 macOS 外部分發 | 本次 ad-hoc codesign，沒有 Developer ID Team ID、Notary Ticket 或實際 Update Feed 啟用證據 | 發行者信任、Hosted CI、正式發行核准與追蹤/回滾機制 OPEN |

### 舊 Manifest 的四個 Release blockers 與新事實分層

- `macos-final-arm64-artifacts-not-verified` → **本機 ad-hoc 候選 ZIP/DMG/App 已後補驗證 PASS**；僅代表 Local Artifact Gate。下一次正式候選必須重新建立其 own Manifest／驗證，不可沿用舊來源 Hash。
- `hosted-macos-ci-not-verified` → **OPEN**；沒有 RC20 signed source 對應 Hosted Run 可引用。
- `formal-release-not-authorized` → **OPEN**；使用者要求持續完成 Stage4/5，不構成 Push/Tag/Release 授權。
- `p11-adhoc-next-only-install-recovery-uat` → **PARTIAL**：NEXT-only RC20 installed + signed RC19 backup PASS，但本次 RC20 **真實 rollback/recovery 未執行**。

此外，本機產品 gate 繼續 OPEN：Connection/Overview 真實拒絕後跨頁、Wails Tray AX name/menu、Native MCP trusted operator→M8→backend model tool、installed Codex ACP 三段（額度阻擋）。installed Gemini ACP Cancel/recover 已 PASS，不可代替 Codex。

## 6. 發行決策與下一階段

**Stage4 的可完成範圍：**本機 Source/Package/Release metadata 驗證、跨平台 source build（Windows arm64 另記 timeout）、原本及 NEXT 服務健康維持、明確的 Hosted CI 和正式發行範圍。**NOT release-ready。** Stage4 不要求為了 CI 綠燈去改 Stable 發行流程、關閉 NEXT/Original、勉強產生 Windows Installer，或新設無真實需求的持續伺服器。

接續 [Stage5 工作包](next-rc20-stage5-codex-resume-workpacks-2026-11-11.md)：先解決 GUI/Tray 的真實可用性，Codex 額度恢復才補 installed Prompt→Cancel→Recovery；Native MCP 先選一個真正有價值的 Backend 再接可信操作者與 M8；對 Host/安全/發行 Gate 分離計畫，最後才評估 Core 改版、Hosted CI、正式簽章與發布授權。

## R01 — NEXT 專屬最小 CI／Release 契約（2026-11-11；Plan / Offline，未發行）

**目的：**可安全、重現地發布 NEXT macOS arm64，不把 Original `v*` CI/簽章/Update Feed 與 NEXT `next-v*` 混用。產品利益是避免測試中的 NEXT 破壞 Original 的可靠控制入口。**不做** Windows Installer、任意多平台矩陣、重複 Portal／MCP test Job、自動啟用 Feed 或未授權發版。

**唯讀現況：**`.github/workflows/next-release-preflight.yml` 的 `pull_request.paths` 是選擇性觸發，兩處 `RC_VERSION='0.9.1-rc.1'`／`BUNDLE_BUILD='1'` 是固定舊示例；`candidate-core` 為 `macos-15/darwin/arm64` 原生 smoke，未涵蓋正式 Shared Wails package、Developer ID 和發布。`.github/workflows/release.yml` 是 Stable `v*`，具 Stable artifact/name/channel，絕不可將 `next-v*` 套用其身分。`tools/release/next_candidate.go` 已拒絕非 canonical manifest bytes；`tools/release/next_artifacts.go` 可做候選檔 inventory，**不能替代 notarization 或實際 install**。Windows NEXT Installer `#error` 保持停用。RC20 ad-hoc 本機 App、ZIP、DMG 已在上節驗證，正式 Hosted 與外部簽名仍 OPEN。

### 最小 Workflow 設計（待正式程式工作單寫入者實作）

1. **NEXT 專用候選驗證入口：**在既有 `next-release-preflight.yml` 消除硬編碼版本，改由明確的 `workflow_dispatch` 輸入 `next_version`／`bundle_build` 與受保護來源的 **完整 Source SHA**，或由下一個獨立 `next-v*` 的 protected tag 觸發；先驗證 tag prefix、版本 ↔ tag 一致、`git rev-parse HEAD` 與完整 SHA 一致、checkout clean、BuildDate 使用該 commit timestamp；所有未核可 SHA/tags 拒絕。CI 實際跑 `next-preflight` 寫入一次 Canonical JSON，並用 `--verify` 校對 exact bytes；以該檔在同一 commit 之下產生/驗證 `next-candidate`。**目前 RC20 canonical `release_ready=false` 不可修改或冒充正式 candidate**；若產品已變更需在新 source commit 產生其自己的 Manifest。
2. **一個 macOS arm64 預簽驗證＋封裝 Job：**`macos-15` arm64，針對 Go Core/Arbiter、Shared Wails、Vue typecheck/Vitest、必要安全/Package fixtures 執行現有測試指令；使用 `packaging/macos/build-next-shared.sh` 的已批准 build path 生成與同一 manifest 綁定的 NEXT-only App/ZIP/DMG。嚴格核對 `CFBundleIdentifier=dev.dropabit.agentdock.next`、`CFBundleVersion`、Core `version --json` SHA、簽章身份與 sealed native Catalog；`next-artifacts` 檢驗 canonical Inventory、ZIP CRC、DMG verify，針對三種成品計算 SHA256、記錄路徑/size/hash。先將**未發布 Artifact + manifest + inventory + job SHA**上傳為不可任意覆寫的候選產物（具保留期限）。
3. **人工 Release Approval、正式簽章：**僅來源保護規則、Hosted Runner、A/B/C/H Host Gate、資安／回滾審核通過，且 Release Owner 明確批准後，才由 protected GitHub Environment（建議 `next-production`、required reviewer）解鎖 Developer ID Application 身分、Apple Team、ASC API Key/Keychain 私密設定；簽章含主 App、Core、cloudflared、Native Launcher 與封裝內容，`codesign --verify --deep --strict`、`spctl`、`xcrun notarytool submit --wait`、`stapler validate` 驗證，重新計算**簽後** Artifact hash。這是 **OPEN**：ad-hoc 不可冒用 Developer ID；憑證/Hosted CI 尚無可引用 RC20 成功 Run。
4. **顯式 NEXT-only Publication Gate：**人工覆核最終 canonical manifest、新 Source SHA、簽章公證 ticket、已驗證 bytes/hash、Windows 未出貨的宣告、NEXT-only rollback 可用與經授權的發版範圍。正式公開前 `release_ready=false` 和 Update Feed `disabled/unpublished`；唯有 gate 全達成，才由**獨立授權發版來源**產生新 approved release metadata（絕不事後修改歷史 RC20 canonical bytes），以 `next-v*` 專用 Release/tag/存放位置發布。不得觸發 Stable `release.yml` 或 Original 的 `v*` Feed。
5. **更新與回滾安全：**NEXT Feed 先發布到不可被 Stable 讀取的私有/候選 channel，驗證 version monotonic、签章 issuer、artifact SHA，已確認 NEXT-only RC19/前一穩定包可恢復；小範圍人工 canary、服務/GUI 非干擾以及 8767／8765 健康後才公開啟用 NEXT channel。出錯時原子回退**只屬於 NEXT** 的 feed 指標與可驗證前一 Artifact、停止 NEXT 更新通道、保留 installed rollback 證據，不停止、回退或覆寫 Original。Feed 未獲額外發行授權前維持 off。

**驗收拆層：**本次 = Local workflow/source contract **PASS**、最小設計與人工 gate **COMPLETE**；Hosted macOS CI **OPEN**、Developer ID/Notary **OPEN**、RC20 real rollback **OPEN**、發行核准/Feed **BLOCKED（未授權）**。不會因既有本機 ad-hoc hash/RC19 備份或 CI 規劃而把 `release_ready` 設為 true。

**後續允許改檔：**`.github/workflows/next-release-preflight.yml` 及必要時新增**一個**明確命名的 NEXT 發行 Workflow、對應 `tools/release/*` 測試與 `packaging/macos/build-next-shared.sh` 參數契約；不得改 `.github/workflows/release.yml` Stable 身分、`packaging/windows/NextAgentDock.iss`、Source signed RC20 commit 或任何 live Apple/Credentials。先在隔離 PR 作 fail-closed negative tests（錯誤 tag/SHA、非 canonical JSON、dirty checkout、缺簽章/公證、錯用 Original feed/identity、被覆寫 artifact），再要求 Hosted proof。原始 Wails 3 upstream release 最新資訊只作升級評估，不是本輪發版依據。
