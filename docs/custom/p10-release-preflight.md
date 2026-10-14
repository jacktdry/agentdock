# P10 第一批：Next source release preflight

2026-10-14 checkpoint；基底 `78d893603f9f1cf8f0f252f89b74f81fdad30284`，分支 `feature/p10-release-migration`。**只有 source dry-run，P10/M9 尚未 closeout，也不是 release-ready。**

## 本批工作與驗收

- 使用者痛點：UI/UX 調整完成後，需要從同一 commit 準備可核對的 Next RC，避免誤用 Stable 版本／產物／更新來源。
- 直接改善：現在可在本機或 CI 輸出並比對同一份候選 JSON；錯配與資訊不足會失敗。
- 最小實作：沿用 `tools/release`，新增 `next-preflight`；不更改 Stable `buildinfo.Version`、catalog 或原 `release.yml`。
- 可觀察驗收：focused Go tests、成功 dry-run / exact manifest verify、錯誤 tag / commit / identity / manifest 的負向驗證、`git diff --check`。不啟動 runtime，故沒有 focus、OS dialog、App/service/profile 變動。
- 不做：正式 tag/Release、push、installer 建置／安裝、Signing、Updater 修改、UI/UX、其他 worktree、8765/8767 或既有 state 操作。

## 候選契約 v1

| 欄位 | 合約 |
| --- | --- |
| version | 明確輸入 `M.m.p-rc.N`；不接受 leading zero、build metadata 或 Stable 版本；M/m/p 0..65535，N 1..65535 |
| candidate_tag | 嚴格等於 `next-v<version>`；與原 `v*` Stable trigger 不重疊；只輸出字串，不建立 tag |
| commit / build_date | 完整 lowercase 40 位非零 SHA-1，必須等於目前 HEAD；日期必須等於 HEAD commit timestamp 的 UTC RFC3339，不取當下 wall clock |
| Core BuildInfo | **要求值** Version = 完整 RC version，Commit = 完整 SHA，BuildDate = source timestamp；現有 `Current()` 對外縮短 Commit 至 12 位，不能據此證明完整 artifact provenance |
| macOS identity | 讀取 canonical `app-identity.sh` Next block；必須是 `AgentDock Next` / `dev.dropabit.agentdock.next` / `AgentDock-Next` / `.agentdock-next` / 8767；只讀 source 字串，不連線 port |
| CFBundleShortVersionString | `M.m.p`，不含 rc suffix |
| CFBundleVersion | **另需明確輸入** bundle build ordinal B（1..9999），映射為 `B.0.0`；不將四段 Windows 版本或 SemVer suffix 填入此欄位 |
| Windows File/Product/Assembly version | `M.m.p.N` 四段數字；InformationalVersion / Core 保留完整 RC 字串 |
| channel / feed | `next-rc`；feed = `disabled/unpublished`、URL 為空、`release_ready:false`；不接入任何 updater |
| 預計產物 | `AgentDock-Next-macos-arm64.zip/.dmg`（P10 支援 macOS arm64-only，與 canonical shell 單架構 identity 一致；不要求 Intel），`AgentDock-NextSetup-amd64.exe`、`AgentDock-NextSetup-arm64.exe`；Windows 名稱是預定契約，尚無對應 Next installer；後續每項須具備 `.sha256` 與同 commit metadata 證據 |

CFBundle 使用三段數字的依據：[Apple CFBundleVersion](https://developer.apple.com/documentation/BundleResources/Information-Property-List/CFBundleVersion)。Bundle ordinal 與 SemVer 是不同欄位；此批要求顯式輸入以避免猜測。後續 release gate 仍須證明 B 對已分發 Next build 單調遞增、每個 RC 對應唯一 source commit，不能只看本機 preflight。

JSON 是確定性的 source contract，**不包含 artifact checksum、簽章或建置成功證據**。可在 dirty source checkout 執行，但 HEAD 只綁定要求的 commit，不能宣稱未提交檔案已屬於該 commit；正式候選 artifact gate 必須要求 clean checkout 並從該 SHA 建置。`--verify` 逐 byte 比對原樣輸出的 canonical JSON（含末尾 newline）；unknown/duplicate keys、格式或值變動均拒絕，不提供寬鬆 reformat/import。

## 本地命令

以下版本與 ordinal 僅示範，並非選定正式 RC：

```sh
rtk proxy go test ./tools/release ./internal/buildinfo -count=1
# HEAD 的 source timestamp 必須轉成 UTC；此基底為 2026-10-14T01:49:23Z。
rtk proxy go run ./tools/release next-preflight \
  --version 0.9.1-rc.1 --tag next-v0.9.1-rc.1 \
  --commit 78d893603f9f1cf8f0f252f89b74f81fdad30284 \
  --build-date 2026-10-14T01:49:23Z --bundle-build 1 \
  > /private/tmp/agentdock-p10-candidate.json
# 以相同參數加上 --verify /private/tmp/agentdock-p10-candidate.json 核對。
rtk git diff --check
```

新 CI `.github/workflows/next-release-preflight.yml` 只用 `contents: read`，checkout 不保存 credentials；僅 checkout/setup Go、focused tests、產生與 verify JSON。PR 使用測試版本，manual input 透過 quoted environment variables 傳入，沒有 tag push trigger、secrets、artifact upload、publish、release、install、遠端 mutation 或 updater 呼叫。CI 尚未執行；本地通過不等於 hosted CI 通過。

## 剩餘 gate / 下一步

1. **UIUX unfrozen、custom/main 尚未整合**；P10 pipeline source authorization / parity integration 仍待 upstream owner 收斂。
2. 第二批已接入 opt-in Next overlay 與 plist 分離版本（見下方）；Stable `internal/buildinfo.Version` 仍是常數。真正 clean HEAD 的候選 Core binary smoke 待 orchestrator 整合後執行，fixture compile 不是 RC artifact acceptance。
3. P10 Next macOS 支援目標為 arm64-only，不要求 Intel 或 universal；manifest 與 update identity 已對齊 canonical shell arm64 包名。Windows Next identity / installer、完整 artifact/hash/metadata verify 亦待實作。
4. **Next channel guard 已在 HTTP 前明確拒絕更新，feed 維持 disabled/unpublished**；Stable `defaultReleaseAPI` 不變。本批不呼叫 updater；核准 Next feed 與完整 activation gates 仍未完成。
5. P11：Developer ID signed macOS 13 deployment target、certificate-bound update → trial → commit、rollback/interrupted recovery、Windows/WSL/Linux helper native UAT、install/update/recovery。Cross-build、source tests 或本 preflight 都不能取代这些驗收。

最接近的後續使用者流程：UIUX 凍結與 owner 整合後，在 clean immutable SHA 將本契約接到既有 Next build metadata，核對真正 artifact；其後才進 P11。沒有安裝、發布、服務或 Stable retirement 授權。

## 2026-10-14 第二批：opt-in RC metadata 接線

- 痛點／直接改善：同一 manifest 驅動 Core、bundled arbiter 的 RC Version/Commit/BuildDate 與 App 分離版本，避免 RC Core 與 Stable arbiter 的更新／回滾版本不一致。
- 最小實作：`tools/release next-candidate --manifest MANIFEST [--output DIR]` 沿用 canonical preflight，逐 byte 驗證 JSON，核對 HEAD、UTC source date、Next identity、clean checkout（含 untracked）。只在 repo-local dist 子目錄 exclusive-create overlay，不改 Stable 常數、version/verify-version 或 `release.yml`。
- 明確 opt-in：`build-next-shared.sh CLOUDFLARED --candidate-manifest MANIFEST` 將已驗證 provenance 與 overlay 接到 Core。`build-app.sh` 的 `AGENTDOCK_NEXT_CANDIDATE_MANIFEST` 重新驗證 manifest，限制 candidate output 到 dist；arbiter 使用同一 manifest 的 overlay 和 Commit/BuildDate。ShortVersion = M.m.p、BundleVersion = B.0.0；未 opt-in 的 dev/Stable/metadata-only 行為維持。
- 驗收與非干擾：focused canonical/clean/dirty tests、isolated Git fixture 的實際 metadata-only plist、tiny Go binary `version --json` / `go version -m`、實際 arbiter compile / `go version -m`。不執行 arbiter、App、服務或 updater；fixture 不是 clean HEAD release acceptance。
- 尚未做：UIUX unfrozen、Windows identity/installer、Next updater feeds、macOS full signed arm64 release、候選安裝與真實更新／回滾 UAT；P10/M9 尚未 closeout。`remaining_gates` 不自動授予 closeout；本次 metadata cleanup 將已接線的 Core overlay 與已禁用的 Next channel 舊 gate 換為 `windows-next-installer-not-isolated` 與 `hosted-ci-native-artifacts-not-verified`；本 checkpoint 與 orchestrator artifact 驗收才是完成度依據。

整合後 orchestrator 在 **clean HEAD** 重新產生 manifest，再執行下面的準備與只讀查核；示例不是發布授權：

```sh
rtk proxy mkdir -p dist/p10-core-smoke
rtk proxy go run ./tools/release next-candidate --manifest /private/tmp/agentdock-p10-candidate.json --output "$PWD/dist/p10-core-smoke"
# 上一步 TSV 提供已驗證 version / full commit / UTC date / short / build。
# 以下 linker placeholders 必須換成該 verified full commit/date。
rtk proxy go build -mod=readonly -overlay "$PWD/dist/p10-core-smoke/overlay.json" \
  -ldflags "-X github.com/uvwt/agentdock/internal/buildinfo.Commit=<verified-full-commit> -X github.com/uvwt/agentdock/internal/buildinfo.BuildDate=<verified-UTC-date>" \
  -o dist/p10-core-smoke/agentdock ./cmd/agentdock
rtk proxy dist/p10-core-smoke/agentdock version --json
rtk proxy go version -m dist/p10-core-smoke/agentdock
```

核對完整 RC version、12 位 runtime Commit、UTC BuildDate；完整 40 位 commit 用 `go version -m` linker metadata 核對。overlay output 必須 fresh。修改中的 dirty checkout 拒絕候選建置，不以自行 commit 或放寬 gate 跑 smoke。沒有完整 GUI/DMG build、簽章、安裝、tag/push、發布或 P11 改造。Next CI 仍 `contents: read`、無 publish；hosted CI 未執行。

本批驗證紀錄：orchestrator 回報 focused `go test ./tools/release ./internal/buildinfo -count=1`、`zsh -n`、diff check PASS。Worker 最後新增／修正部分以 `go test ./tools/release -run '^TestNext(ArbiterOverlayBuild|CandidatePlistFixture)$' -count=1` PASS（16.964s），並完成 `git diff --check`。先前 worker fixture 曾因預設 Go cache 的 sandbox 寫入限制及 macOS temp symlink 路徑失敗；後者已以 canonical temp root 修正，上述兩項重驗 PASS。沒有再重跑整組測試或完整建置。

## 2026-10-14 第二批 clean-HEAD Core 驗收補記

- P10 source commit `b4092271` 後工作樹乾淨，使用該 HEAD 的 full SHA 與 UTC commit timestamp，產生 `next-v0.9.1-rc.1` 示例 manifest；`next-candidate --output` 驗證後生成 repo-local Go overlay。
- 以 `GOMAXPROCS=2 GOFLAGS=-p=2 CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 MACOSX_DEPLOYMENT_TARGET=13.0` 及對應 CGo flags 真實建置 `./cmd/agentdock`，只執行無服務副作用的 `version --json`、`go version -m`。
- **PASS**：`version=0.9.1-rc.1`、`commit=b4092271b07a`（runtime 僅顯示 12 位）、`build_date=2026-10-14T02:33:12Z`；`go version -m` 包含完整 Commit 與 BuildDate；後續 `git status --porcelain` 仍為空。
- 產物只保存在本工作樹 `dist/p10-clean-core.qWKpBz/agentdock`，未執行服務、安裝、部署、推送、建立 Tag 或 GitHub Release。這是**實際 Core 編譯與版本驗證**，仍**不是**完整 macOS .app/DMG、Windows installer、Next updater channel 或 P11 簽章與原生更新驗收。UI/UX 仍未凍結。


## 2026-10-14 第三批：macOS arm64 本地候選包實證（未發布）

- 使用者流程／直接收益：在 UIUX 尚未凍結期間，可以從乾淨的 P10 SHA 檢查「同一候選 Manifest → Next Wails／Core／arbiter → 本地 App／ZIP／DMG」實際打包路徑，避免只靠 source fixture 宣稱可發佈。
- 來源：**已提交的** `15b116dd`；`0.9.1-rc.1` 只是示範版本，`release_ready=false`、feed disabled/unpublished。候選 JSON 留在本工作樹 `dist/p10-macos-candidate.json`。Stable 安裝／更新、正式 Release、Tag、Push 均未操作。
- 首次完整 build 因前端 pnpm 網路下載過慢而逾時，後以 repo-local `dist/pnpm-store` 的 `pnpm install --offline --frozen-lockfile` 重用 80 個套件（0 download），前端 `vue-tsc` / Vite 192 modules **PASS**；JS chunk 大於 500 kB 僅效能警告。
- 本地建置成功產生 arm64 Wails Desktop、Next Core (`version=0.9.1-rc.1`、runtime commit `15b116ddf76e`、UTC BuildDate `2026-10-14T03:14:47Z`)、Core Skills 與 cloudflared payload，皆為 source-only 測試，不安裝／啟動服務。
- 直接從這些編譯產物執行 `build-app.sh`：**Next App、ZIP、DMG 實際產出，ad-hoc codesign 檢查通過，DMG `hdiutil verify` checksum valid**。第一次整包腳本最後的 Python 套件 fixture 因繼承 `AGENTDOCK_NEXT_CANDIDATE_MANIFEST`，把獨立的 `--metadata-only` fixture 誤判為 candidate，導致整體 exit 1；不是 App 啟動或簽章失敗。隔離環境後 `test-next-shared-package.py <bundle>` **原有 8 項 PASS**，完整實際 Bundle 驗證 PASS。
- 修正：package fixture 明確移除所繼承的 candidate marker，另加一項 arm64/universal/Stable 名稱驗證，**9 項 PASS**，包含故意設置無效 candidate marker 的負向情境。Next arm64-only 打包原先錯用 `universal` 檔名，現改為使用 `AgentDock-Next-macos-arm64.zip/.dmg`；Stable 及 Next 雙架構命名規則不變。先前已建立的本地 ZIP／DMG 僅在忽略版控的 dist 內重新命名並重新計算 SHA-256，兩項 checksum **PASS**；它們仍是 commit `15b116dd` 的測試包，不能冒充修正後 SHA 的完整 build acceptance。
- 限制：**僅 arm64**（P10 支援目標，不要求 Intel / universal）；簽章為 `-` ad-hoc，非 Developer ID；沒有 notarization、GitHub Hosted CI、Windows Next installer、正式更新 feed、原生安裝／更新／回滾、P11 UAT。提交本次來源修正後，日後仍需對該新 SHA 執行 clean HEAD 完整包驗證；本記錄不授予 P10 closeout 或公開發佈權限。


## 2026-10-14 recovery: P10 source candidate and installer safety gates

本紀錄用於區分「**P10 source preflight 實作通過**」與「**P10 release/installer readiness 正式驗收**」；UI/UX 尚未凍結、P10 尚不得標示完整 closeout，正式發佈仍暫停。

- Mac-Dev 1.1.0 連線恢復後，檢查 `feature/p10-release-migration` 從乾淨 `8caf0995` 開始。斷線前的三個 Codex run 均為 interrupted / run ID missing，**沒有可採信的原始碼成果**；改以新獨立 worker 完成並由 owner 整合測試。
- Windows Next WPF：新增只讀 `Models/NextWindowsManifestIdentity.cs` 與 `scripts/test/test-next-wpf-runtime.py`，要求明確 `product_variant=next`、`product_id=dev.dropabit.agentdock.next`、`8767`、Next 專屬 root/state/task/run 值並拒絕缺漏、重複、路徑別名及非 Next 內容。**現有 Next WPF 入口仍 fail-closed**，驗證器的存在不解鎖管理／更新或回退 Stable。來源測試 **4/4 PASS**，並非 Windows C# 原生執行驗收。
- Windows Installer：`prepare-next-rc.py --verify-payload` 讀取 Next 專用候選 metadata 與逐檔 SHA-256、PE 架構／命名／link 安全檢查，並執行既有 canonical `next-candidate` clean-HEAD gate。**本機 selected 16/16 PASS**，含一次真實 Go canonical verifier 連接測試；成功路徑仍 `installable=false`。Next `.iss` 首行無條件 `#error`，`--build-installer` 仍在 ISCC 前拒絕。未產生 Windows Setup 或操作 Stable／Next 安裝狀態。
- Release：新增 `go run ./tools/release next-artifacts`，僅對已存在且明確點名的 Next macOS arm64 / Windows 架構產物建立確切 JSON 清單、SHA-256、大小與格式驗證，`release_ready=false`，用 `--verify` byte-exact 核對。native PR `candidate-core` CI 各架構多了一次 Core-only inventory；仍只有 `contents:read`、不傳 release、無 secrets、無 dispatch。Go release 完整套件測試 **PASS**；Ruby YAML／CI read-only policy **PASS**。
- 實體 source-only smoke：從乾淨 `1f59a5b984e06067bc069feefc3d37290c54005b` 編出 macOS arm64 `agentdock-next-core`；`version --json` 回報 `0.9.1-rc.1`、`1f59a5b984e0`、`2026-10-14T05:38:35Z`、`darwin/arm64`，`next-artifacts --verify` **PASS**。實體檔案為 31,677,698 bytes、SHA-256 `0628c7055a7c719daae59e90909100438c2d7403d50a99e53a2bee3a7a8d3345`。這**不等同最新 SHA 的完整 App/ZIP/DMG build**；先前成功的完整 macOS bundle 是其他已提交 SHA 的 ad-hoc arm64 包。
- Source commit：`cb85388e`（Windows identity/payload candidate checks）、`1f59a5b9`（artifact inventory/CI）。兩者皆為本地提交，工作樹 clean，未 Push、Tag、建立 GitHub Release、部署、安裝或啟用 Next update feed。

**阻塞 P10 完整結案的必要條件（不可改列已完成）：**

1. Windows Next 的完整信任 Runtime Manifest 與 Core/WPF/Installer 共用、獨立排程／權限／啟動／移除 owner、完整可驗證的 Windows payload；目前 Next 入口與 Inno 編譯仍故意拒絕。這是 P10 **實作**缺口，不能只歸類為 P11 native UAT。
2. 同一最終 immutable Commit 的 macOS arm64 App／ZIP／DMG 及 Windows Setup 資產／中繼資料／SHA 核對（目前 release CLI 是 source-only available files，不能證明完整 release assets）。
3. GitHub Hosted PR CI（Ubuntu、macOS arm64、Windows amd64/arm64）實際遠端 PASS／產物 trace；工作流程目前僅本地語法及來源測試，未 Push／未觸發。進一步串接正式 Release 發佈 pipeline 須繼續維持 **禁用** 至另行授權。
4. UI/UX 完成後須選定實際版本/Tag/Bundle Build、最終 source SHA 再跑 immutable preflight。依使用者決定，Tag／Release／正式 feed 在此之前暫緩。

**P11 另案處理** Developer ID/Authenticode 簽章及可能的 notarization、更新／回滾／恢復、Windows/macOS 跨平台原生 UAT；不要將 P11 的實測記為 P10 已完成。
