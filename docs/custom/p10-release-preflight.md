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
| 預計產物 | `AgentDock-Next-macos-universal.zip/.dmg`（沿用 identity 名稱），`AgentDock-NextSetup-amd64.exe`、`AgentDock-NextSetup-arm64.exe`；Windows 名稱是預定契約，尚無對應 Next installer；後續每項須具備 `.sha256` 與同 commit metadata 證據 |

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
2. Stable `internal/buildinfo.Version` 仍是常數；現有 `build-next-shared.sh` 仍取 Stable version，Core 只注入 Commit/BuildDate。本批 JSON 的 Next Core Version 是要求值，尚未接入真正建置；不得把這支腳本產物稱符合 RC 合約。
3. `app-identity.sh` 現在用同一 `$VERSION` 寫兩個 CFBundle 欄位，需要後續分離 metadata；既有 Next shared builder 僅 arm64，不能因產物名稱含 universal 就視為 universal。Windows Next identity / installer、完整 artifact/hash/metadata verify 亦待實作。
4. **`internal/selfupdate/update.go` 的 `defaultReleaseAPI` 仍指向 upstream `uvwt/agentdock/releases/latest`，尚未 Next channelize**。本批不更改或呼叫 updater；Next feed 必須保持 disabled/unpublished，直到 channel/identity fail-closed gate 完成。
5. P11：Developer ID signed macOS 13 deployment target、certificate-bound update → trial → commit、rollback/interrupted recovery、Windows/WSL/Linux helper native UAT、install/update/recovery。Cross-build、source tests 或本 preflight 都不能取代这些驗收。

最接近的後續使用者流程：UIUX 凍結與 owner 整合後，在 clean immutable SHA 將本契約接到既有 Next build metadata，核對真正 artifact；其後才進 P11。沒有安裝、發布、服務或 Stable retirement 授權。
