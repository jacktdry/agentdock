# AgentDock NEXT RC21 — 工作線 G Windows App／DesktopAPI／Config／Plugin HANDOFF

日期：2026-11-12（Asia/Taipei）
Owner：G；只處理 G-owned source/tests，不操作正式服務、使用者瀏覽器、ACP、CI 或遠端 Git。

## 1. 固定基準與範圍

- 規格：`docs/custom/next-rc21-hosted-ci-post-e-windows-parallel-2026-11-12.md`。
- 固定起點：`a2b47000bbbb3fa349d5f2f739496522bf585d9b`；對照 Hosted Full CI `43167819420`，Windows job `131013723150`，原始 CI Head `5af6f00ea25f15d6011378a1debb08e8a6ef69f9`。
- Branch：`fix/next-rc21-ci-g-win-app-20261112`。
- Worktree：`/Users/wei/sideProject/agentdock-next-rc21-ci-g-win-app-20261112`。
- 僅修改 `internal/app/**`、`internal/config/**`、`internal/desktopapi/**`、`internal/httpx/**`、`internal/tool/plugin/**` 與本報告。F 的 `internal/tool/browser/**`、H 的 `internal/desktopruntime/**`、Permission、CI YAML 與共用狀態文件均未修改。

產品阻礙：Windows Hosted Go Test FAIL，導致候選無法安全合流；本線只修已觀測的 Windows fixture 與 Token Bridge 安全功能差異。驗收層級是 macOS Go 1.26.5 focused tests、vet 與 Windows `go test -c`；Windows Native 需由後續整合者在 Hosted runner 執行。

## 2. CI 失敗根因／最小修正

| G 套件／CI 測試 | 分類與原因 | 本線處置 |
|---|---|---|
| `internal/app/TestRuntimeACPBrowserNextSharedWorkspaceOptIn` | Fixture：Windows `os.UserHomeDir()` 使用 `USERPROFILE`，原測試只設 `HOME`，Next isolated-root 拒絕屬正確 | 同步設定 fixture `USERPROFILE`，保留 Next root / restart fence |
| `internal/app/TestRuntimeBrowserDesktopConfiguredAndRetainedRoutes` | Fixture：`/private/profiles/edge` 不是 Windows 原生絕對路徑 | 改用 `t.TempDir()` 下 native absolute profile/workspace paths，不變更 browserpolicy canonical 驗證 |
| `internal/config/TestBrowserWorkspacePoliciesConfig`、`TestBrowserCatalogConfig` | Fixture：catalog JSON 使用 Unix `/configured/edge`；Windows 正確拒絕 | 保留既有常數作 Darwin 專用衝突模板；一般測試依 `filepath.Join` 建出原生 JSON 路徑，保留錯誤欄位/來源/外部 connector 負測 |
| `internal/desktopapi` ACP Probe／Check／Update 共六組 Unix positive fixtures | 平台功能／fixture：Unix `#!/bin/sh` 測試 adapter 不可在 Windows 執行，且 H-owned `desktopruntime/acp_update_promote_other.go` 在 Windows 刻意不授予 target trust 或 promotion | 只在 Windows skip 不適用的 Unix positive cases；新增 Windows-native fail-closed test：可信 release 也不可產生 update plan／promotion，二進位不得被覆寫；保留 Unix positive coverage |
| `internal/desktopapi/TestConnectionTokenBridgeCleanupAndNoLeak` | 真正跨平台私密權限缺口：`os.FileMode.Perm()==0600` 不能證明 Windows ACL；訊息為 `connection_mutation_failed` | 根目錄先 canonical / symlink 拒絕；Unix 繼續 `0600`；Windows 使用既有 `securepath.EnsurePrivate` 對根目錄與暫存 token 安裝 protected owner/SYSTEM/Administrators DACL，再寫入 token，任何失敗須清理；Windows 測試直接讀 DACL 確認 only allowlisted full-control ACE，保留 argv / response 不洩漏與 token cleanup |
| `internal/httpx/TestRuntimePluginDesktopCandidateRouteIsLoopbackOnlyAndSafe` | Fixture：以字串插值生成 Windows 絕對路徑導致 JSON 反斜線無效，403/401 邊界正確、local 400 | 用 `json.Marshal` 產生 request，原有 loopback-only、credential、generic-route denial 等驗證不變 |
| `internal/tool/plugin/TestDecodeDesktopCandidateRequestStrict` | Fixture：合法 source 使用 Unix-only `/private/plugin` | 改為原生 `t.TempDir()` 衍生 absolute picker path 並 JSON marshal；相對路徑、URL、不合法 action/target/review_token 仍必須拒絕 |

**可信 Fork/ACP:** 沒有建立 Windows update 的 OS ownership proof 或 promotion 實作；H-owned `desktopruntime` 原來的 `update_target_not_next_owned` 正確 fail closed，不應因單元測試而放寬。沒有將 `Trusted Fork` 字串或版本號當成授權證據，也沒有對非可信 Plugin source 或 Edge Profile 增加例外。

## 3. 安全與副作用檢查

- Browser company route / external profile / Transport Fence 不變；不從其他瀏覽器 profile 回退。
- ACP explicit target 必須經 Next-owned + release provenance 驗證；Windows 不具此授權時拒絕更新。
- Plugin Candidate 仍需 native absolute path、正確 request schema、有效代次與授權；無任意 URL 來源或 loopback 外暴露。
- Token Bridge：先確保 Windows root 與 token 的 protected DACL，避免單靠 Unix 權限位元假裝安全；token 不出現在 argv/response，臨時檔於成功或錯誤時均回收。
- 未觸碰現役 Stable/NEXT/Original、Mac-Dev control、CI/部署及非 G 分支。

## 4. 本機證據

- `GOTOOLCHAIN=go1.26.5 go version`：`go1.26.5 darwin/arm64`（已確認）。
- `GOTOOLCHAIN=go1.26.5 go test ./internal/app ./internal/config ./internal/desktopapi ./internal/httpx ./internal/tool/plugin`：**PASS**，五個 G package；首次建置遇測試 import 遺漏，修正後重跑五套件全 PASS。
- `GOTOOLCHAIN=go1.26.5 go vet ./internal/app ./internal/config ./internal/desktopapi ./internal/httpx ./internal/tool/plugin`：**PASS**。
- `GOOS=windows GOARCH=amd64 GOTOOLCHAIN=go1.26.5 go test -c -o /tmp/next-rc21-g-windows-cross/{app,config,desktopapi,httpx,plugin}.test.exe ...`：**五套件 PASS**。初次因 Windows 測試常數 `windows.FILE_ALL_ACCESS` 不存在而失敗，改用 Windows SDDL `FA` 所對應的 `0x001f01ff` 後重編譯全 PASS；這僅是 Windows source/test compile，**尚未 Windows Native 執行**。
- `GOOS=windows GOARCH=amd64 GOTOOLCHAIN=go1.26.5 go vet ./internal/app ./internal/config ./internal/desktopapi ./internal/httpx ./internal/tool/plugin`：**PASS**（Windows 目標 static vet，非 Native runtime）。
- `git diff --check`：**PASS**（格式無警告）。

## 5. Windows Native 待驗／跨線依賴

1. **Windows Hosted Full CI G 原生重跑**：同樣五個 package，檢查上述 tests，尤其 Windows Token Bridge 的 DACL 檢查（Windows 原生可觀測），以及 `USERPROFILE` 測試環境、private temp root 的 canonical path。
2. **Windows Token Bridge**：確保普通使用者的 `GetNamedSecurityInfo` / protected DACL 操作成功，失敗時不留下 token；在不具 symlink creation privilege 的 runner 記錄跳過 symlink *建立* 子情境，於具該權限 runner 補做。
3. **Windows ACP**：確認 fail-closed test（不可更新，無 update plan）成立；真正 Windows ACP updater / signed trusted release / OS binary ownership / atomic promotion 能力目前不在 G，也不應由 G 聲稱可用。若需要此功能，須由 H owner 評估 `internal/desktopruntime/**`，另有明確產品授權與安全證據。
4. **H**：`internal/desktopruntime` Windows Native Port observer、Next OS identity、Named Tunnel token reason 及 updater tests 由 H 修復與驗證，G 不 cherry-pick 或跨界寫入。G 的 Token Bridge 尚有 H-owned mutating preflight/identity 的實機依賴。
5. **F**：`internal/tool/browser` Windows Edge process signature、Profile、npx 與 lease 由 F owner 處理；G 對 Browser Policy 的修改僅測試路徑。
6. **合流**：整合者核對 F/G/H commit、worktree clean/ownership，再在獨立整合 Worktree 合流並重跑 macOS/Ubuntu/Windows Hosted Full CI；未有該證據不得宣稱 Windows full PASS、推送或部署。

## 6. 出界／未做

沒有執行 Windows Native runner、Windows GUI/Installer 真機驗收、Browser 真實 Edge Profile UAT、CI 修改、Git Push/Merge 或清理分支。保留平台缺失與 Windows 原生待驗，不以交叉編譯替代實測。
