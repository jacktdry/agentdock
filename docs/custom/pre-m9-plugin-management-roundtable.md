# Pre-M9 Wave 3 — Plugin Management UX / IA + Authority Contract

> 狀態：**Roundtable frozen for implementation（2026-10-08）**
>
> 前置：P4 ACP Manager 與 P5 MCP Management 均已 closeout。此文件定義 Wave 3 第三個 domain「P6 Plugin Management」的 Shared Desktop 產品、Desktop API、安全與 runtime contract；不建立 Plugin marketplace，也不提前進入 Nexus / Platform Essentials 或 M9。

## 1. 目標與邊界

Plugin Management 要讓一般使用者能安全回答：

- AgentDock Next 現在安裝了哪些 Plugin？
- 每個 Plugin 提供哪些 Skills / MCP capabilities？
- Plugin 是否已啟用、是否需要設定 credential / environment？
- 如何從本機資料夾或 ZIP 安裝 Plugin？
- 安裝或更新前，我實際要允許哪些能力與 executable？
- 更新會改變哪些 Skills / MCP / executable？
- 移除時哪些資料會保留，哪些會永久刪除？

不可破壞的邊界：

- stable `mac-dev` 仍是 Next repository / build / install / live-runtime mutation 的唯一開發 control plane；`macbook-air-m3` 只作 Next connector/runtime validation。
- Shared renderer 不直接呼叫 model-facing `plugin_manage`，不直接讀寫 `~/.agentdock-next` Plugin state，不直接執行 package 內容。
- 目前 public/internal Runtime Plugin API 的 GET list/detail 繼續保持 read-only；P6 mutation 使用獨立 Desktop authority。
- Plugin source 只支援 **本機資料夾或 ZIP**。P6 不做 marketplace、遠端 URL install、Git clone、auto-update feed 或「檢查最新版」。
- renderer 不取得絕對本機路徑、raw review token、secret value、raw provider error、stdout/stderr、Plugin private runtime path。
- P5 對 Plugin-owned MCP 維持 read-only；真正的 Plugin lifecycle 與 Plugin-owned MCP environment configuration 由 P6 管理。

## 2. 現況盤點

Core 已有完整 Plugin engine：

- Portable / OpenAI / Claude Plugin format detection / normalization；
- local directory / ZIP staging；
- symlink / traversal / archive size / extracted size / file-count safety gate；
- validate / review token；
- install / update activation transaction；
- enable / disable；
- update activation rollback；
- remove with `keep` / `purge`；
- purge tombstone / retry；
- Plugin-owned MCP reconciliation；
- Plugin Skill data / env、Plugin-owned MCP env / OAuth cleanup。

但既有 contract 不能直接變成 Shared UI mutation API：

1. `plugin_manage` 接受 raw local path，renderer 不應接觸 host absolute path。
2. validate 回傳的 `review_token` 是 package review capability，不應進 renderer state。
3. installed Plugin 尚沒有 Desktop stale-editor revision / incarnation generation。
4. install 預設可 enabled；Shared UI 必須把「安裝 package」和「啟用能力」拆開。
5. update 會保留既有 Enabled 狀態；對 enabled Plugin 會造成 runtime deactivate / activate，UI 必須 truthful。
6. Runtime Plugin GET API 目前不呈現 cleanup/recovery、Plugin-owned env configured state 或 mutation outcome。
7. Plugin-owned MCP environment 不能借用 P5 standalone mutation path；必須驗證 Plugin ownership 與 component scope。
8. remove keep / purge 的資料語意差異很大，不能只做一個「Delete」按鈕。

因此 P6 先建立 Desktop-owned candidate / management authority，再做 Shared UI。

## 3. Page goal / information hierarchy

使用單一 **Plugin Management** page：

1. **Summary**
   - Installed
   - Enabled
   - Needs attention
   - Plugins providing Skills
   - Plugins providing MCP
   - passive **Reload**
2. **Installed Plugins**
   - Primary action：**Install Plugin**
   - row/card：
     - name / description
     - version
     - format
     - Enabled / Disabled
     - Skills count / MCP count
     - warning / recovery state
3. **Plugin detail**
   - Overview
   - Capabilities
   - MCP settings
   - Package information
   - Remove Plugin

Summary Reload 只能重新讀 authoritative state，不得 enable、reconcile、spawn MCP、執行 executable 或掃描外部 source。

## 4. Install flow

### 4.1 Choose package

Primary CTA：**Install Plugin**。

使用 native picker，提供：

- **Choose folder**
- **Choose ZIP file**

renderer 不取得選取後的 absolute path。native/Desktop layer 立即把來源交給 Core 建立 AgentDock-owned immutable candidate snapshot，renderer只得到：

- opaque `candidateId`
- safe display label（例如 ZIP filename / 「Selected folder」；不可含完整 path）
- expiry
- safe review result

Cancel 不產生 error。

### 4.2 Review package

Review screen 的順序固定：

1. **Plugin**
   - name
   - version
   - description
   - package format
2. **What it adds**
   - Skills：name + description
   - MCP connections：display name、Local process / Remote connection、safe endpoint host/origin 或 safe executable basename/relative path
   - executables：只顯示 relative path / basename
3. **Configuration needed**
   - Plugin-owned MCP required environment / header names
   - 只顯示 variable/header names與 configured/missing；不顯示 value
4. **Package source**
   - safe provenance origin / ref / revision / subdir（若存在且通過 sanitize）
   - package fingerprint（digest放 Advanced；可 copy，但不是 approval secret）
5. **Review findings**
   - warnings
   - blocking issues

Blocking issues 禁止 Install。Warnings 顯示在 confirmation 前，不用使用者勾「我已閱讀」才能前進；Install confirmation 本身就是 review consent。

### 4.3 Install

**Shared UI 的新 Plugin 一律安裝為 Disabled。**

理由：

- 安裝 package 與啟動能力是兩個不同風險層級；
- 使用者可先設定 Plugin-owned MCP credential/environment；
- 不會因完成 install 就啟動 local process 或 remote MCP connection；
- 符合 P5「side effect 必須 explicit」原則。

Install confirmation：

> Install this Plugin?
>
> The package will be added to AgentDock but will stay disabled. Review or configure its connections before enabling it.

成功後 primary next action：**Configure and enable**；沒有 MCP / required configuration 時可顯示 **Enable Plugin**。

## 5. Enable / Disable

### Enable

需要 confirmation。

若含 local-process MCP：

> Enabling this Plugin may run local programs with your user permissions.

若含 remote MCP：

> Enabling this Plugin may connect to external services.

若 required environment 尚未 configured，backend fail closed；UI列出缺少的名稱並導向 MCP settings。

### Disable

需要 confirmation，因為可能停止 Plugin-owned MCP 並讓 Skills / capabilities立即不可用。

文案：

> Disable this Plugin?
>
> Its Plugin-provided capabilities will stop being available. Saved Plugin data and connection settings are kept.

Enable / Disable 不稱作「Start / Stop process」；process只是 runtime implementation。

## 6. Update from local package

P6 **沒有「Check for updates」**。現有 engine沒有 marketplace/version feed。

動作名稱固定為：**Update from local package**。

流程：

1. native picker選 folder / ZIP；
2. backend建立 immutable candidate snapshot；
3. candidate name 必須和 target Plugin相同；
4. 顯示 current → candidate diff；
5. confirmation 後 update。

Diff 至少包含：

- version；
- package fingerprint changed / unchanged；
- format；
- provenance；
- Skills added / removed / changed；
- MCP connections added / removed / changed；
- executables added / removed / changed；
- warnings added / resolved。

若 current Plugin Enabled：

> This Plugin is currently enabled. Updating it will temporarily stop its Plugin-provided connections and activate the reviewed package when the update succeeds.

Core 繼續保留既有 Enabled 狀態；如果使用者不希望 candidate立刻 activation，應先 Disable 再 Update，不新增隱藏的「install update disabled」語意。

rollback success 要顯示：

> Update failed. The previous Plugin version was restored.

若 runtime / persistence outcome 不確定，不能顯示「更新失敗所以沒變」；必須進 recovery / operation-status state。

## 7. Plugin-owned MCP settings

P5 MCP page：

- 仍列出 Plugin-owned MCP status / tools；
- configuration/lifecycle controls維持 read-only；
- 提供 **Manage in Plugin Management** link。

P6 detail 的 **MCP settings**：

- 依 Plugin component分組；
- 顯示 safe connection summary；
- 顯示 declared required env / header env names；
- value永遠 write-only；
- ordinary snapshot只回 `key + configured/missing/unknown`；
- set / replace / unset 必須走 Plugin-owned scope authority；
- backend以 Plugin name + component identity解析 storage key，renderer不提交 raw storage key；
- 不允許寫入不屬於該 component declaration / existing scoped state 的任意 key。

P6 不重做 OAuth browser flow UI；Plugin-owned MCP若需要 OAuth，狀態可在 P5觀察，P6只負責 Plugin lifecycle與 package-declared configuration。若後續需要 Plugin-owned OAuth lifecycle，另開 contract，不偷偷借用 standalone P5 action。

## 8. Remove flow

入口名稱：**Remove Plugin**。

第一層 dialog先選兩種清楚的結果：

### Remove Plugin, keep data（default）

移除：

- installed Plugin package/state；
- active Plugin capabilities。

保留：

- Plugin data；
- Plugin Skill data / environment；
- Plugin-owned MCP environment；
- Plugin-owned MCP OAuth grants。

文案：

> Remove the Plugin but keep its saved data and connection settings. Reinstalling the same Plugin can use them again.

### Remove Plugin and delete data

這是 destructive secondary option，需第二層 danger confirmation：

> Permanently delete this Plugin and its AgentDock-managed data?
>
> This also removes Plugin Skill data/settings and Plugin-owned MCP connection values and saved authorizations. This cannot be undone.

Purge 進行中 crash/retry時 UI 不把 Plugin重新顯示成正常 installed；顯示 **Finishing removal** / **Cleanup required**，只允許 Retry cleanup / Reload 等 recovery action。

不使用「tombstone」等工程術語。

## 9. Desktop authority contract

新增 Plugin Desktop management boundary；model-facing `plugin_manage` 與 read-only `/internal/runtime/plugins` 保持原用途。

建議 internal route：

~~~text
/internal/runtime/plugin/desktop
~~~

### 9.1 Snapshot

~~~text
PluginManagerSnapshot
  registryRevision
  authoritative
  plugins[]
  recoveryItems[]

PluginManagedItem
  name
  description
  version
  format
  enabled
  generation
  installedAt
  skillsCount
  mcpCount
  warningCount
  provenance?          // safe projection
  packageFingerprint
  recoveryState?
~~~

規則：

- passive snapshot / inspect = zero activation / network / executable side effects；
- Core unavailable 或 state read失敗時 `authoritative=false`，所有 mutation disabled；
- `registryRevision` 必須涵蓋會影響 mutation basis 的 installed/recovery state；
- `generation` 是 Plugin incarnation fence，update / remove / reinstall後改變；enable/disable不改 incarnation；
- 可由 backend安全地以 persisted identity（例如 name + package digest + installedAt）產生 opaque generation，不把演算法當 public contract。

### 9.2 Native candidate

renderer 不提交 source path。

native picker → Desktop/Core candidate staging 後回：

~~~text
PluginCandidate
  candidateId
  kind                 // install | update
  targetName?
  expiresAt
  review               // safe projection, no review token/path
~~~

Core/authority private state保存：

- AgentDock-owned immutable staged snapshot；
- Core review token；
- package digest；
- intended action；
- target name + expected generation（update）；
- expiry。

原始 user directory / ZIP 後續變動不能改變已 review candidate。

candidate：

- 有限 TTL；
- close/cancel 可 explicit discard；
- definitive success後 consume；
- expired / missing candidate fail closed並要求重新選 package；
- renderer永遠看不到 raw review token。

### 9.3 Mutations

至少：

~~~text
installCandidate
updateCandidate
setEnabled
removeKeep
removePurge
pluginMCPEnvSnapshot
pluginMCPEnvSet
pluginMCPEnvUnset
operationStatus
discardCandidate
~~~

每個 mutation：

- request ID；
- expected registry revision；
- target mutation需 expected generation；
- env mutation另帶 expected env revision；
- semantic request fingerprint / approval binding沿用 P5 pattern；
- idempotent retry / operation journal；
- stale state fail closed；
- timeout / unavailable後回 `outcomeUnknown`，不得猜測成功或失敗；
- structured `persisted / runtimeApplied / recoveryRequired / runtimeImpact`。

Install 的 backend-owned enabled固定為 `false`；renderer不能繞過。

## 10. Security / privacy projection

不得送 renderer：

- absolute source / package / runtime / data paths；
- raw review token；
- environment / header secret values；
- OAuth access/refresh/client secrets；
- raw executable args若可能含 secret；
- raw stderr/stdout/provider error；
- internal storage key；
- unsanitized URL userinfo/query/fragment。

可顯示：

- safe package filename label；
- safe relative executable path / basename；
- safe HTTPS origin / host；
- environment/header variable names；
- configured boolean；
- safe provenance fields；
- package digest/fingerprint。

所有 error code/message經 Desktop safe allowlist轉換；raw Core cause只留安全 diagnostics boundary。

## 11. Confirmation matrix

不需要 confirmation：

- Reload；
- open detail；
- choose package；
- validate/review candidate；
- discard candidate；
- view configured status。

需要 confirmation：

- Install reviewed Plugin；
- Update from reviewed local package；
- Enable；
- Disable；
- unset saved Plugin-owned MCP value；
- Remove + keep data；
- Remove + purge data。

Purge 使用更強的 danger confirmation；其餘不使用 typed-name confirmation，避免不必要 friction。

## 12. Loading / empty / degraded / conflict / recovery

### Empty

> No Plugins installed.
>
> Install a Plugin from a local folder or ZIP file.

### Loading

- skeleton/list busy state；
- 保留 page heading，不用 full-screen spinner；
- `aria-busy`。

### Candidate validation

- 顯示「Reviewing package…」；
- 可 cancel UI，但若 backend snapshot已建立要 discard candidate；
- candidate expired：「This package review expired. Choose the package again.」

### Degraded

- Core unavailable / non-authoritative snapshot：保留最後安全 read model只作 reference並清楚標 stale，不允許 mutation。

### Conflict

- stale revision / generation：「This Plugin changed since you opened it. Reload and review the latest state before trying again.」
- candidate不自動套到新 generation。

### Recovery

- operation outcome unknown：顯示 Checking result，透過 operation status查詢；
- rollback restored previous：「Previous Plugin restored」；
- purge cleanup pending：「Finishing removal」；
- recovery-required時封鎖會讓狀態更難判斷的 lifecycle mutation。

## 13. Responsive / accessibility

- 320 CSS px 或等價 zoom不得 page-level horizontal scroll；
- desktop list/detail可 split layout；窄視窗改 stack；
- review diff不用大型橫向 table，改 section/card + Added/Removed/Changed badges；
- 所有狀態不只靠顏色；
- dialog有 focus trap、初始 focus、Escape semantics、return focus；
- destructive confirmation focus 不預設落在 danger button；
- async result使用 `aria-live` / common OperationStatus；
- Skills / MCP / executable lists有 semantic heading/list；
- collapsed Advanced仍可鍵盤操作；
- source picker button有明確 accessible name：「Choose Plugin folder」「Choose Plugin ZIP file」。

## 14. User-facing terminology

使用：

- Plugin
- Install Plugin
- Enabled / Disabled
- Update from local package
- Review package
- Plugin capabilities
- Skills
- MCP connections
- Connection settings
- Keep data
- Delete data
- Package fingerprint
- Reload
- Needs attention
- Cleanup required

不要在一般 UI 使用：

- registry revision
- generation / incarnation
- HMAC
- review_token
- storage key
- tombstone
- activation transaction
- reconcile
- lease

## 15. Non-goals

P6 不包含：

- marketplace / Plugin catalog；
- remote URL / GitHub install；
- auto update / version feed；
- Plugin publishing / signing ecosystem；
- arbitrary package file browser；
- raw manifest editor；
- raw command/env editor；
- per-Skill permission editor；
- P5 standalone MCP lifecycle；
- Browser Broker；
- Plugin-owned OAuth lifecycle redesign；
- bulk install/update/remove。

## 16. Verification gate

Implementation closeout前至少：

- Desktop Plugin authority targeted Go tests + race tests；
- candidate source path never appears in Desktop API response / frontend bindings / Activity / Diagnostics；
- review token never reaches renderer；
- source modified after selection不改變 reviewed candidate；
- candidate expiry / discard / exact retry；
- stale registry revision / generation fail closed；
- same-name remove/reinstall incarnation fence；
- install always disabled；
- enabled update rollback restores prior runtime/state；
- Plugin-owned MCP env ownership isolation + write-only no-leak；
- remove keep preservation；
- purge crash/retry / tombstone lifecycle；
- passive snapshot/inspect zero runtime side effects；
- approval semantic binding / request-ID idempotency；
- frontend typecheck/tests/build；
- macOS live UAT through stable `mac-dev`；
- `macbook-air-m3` final read-only connector validation；
- Windows/Linux P6 internal package cross-build/compile coverage。

P6 live UAT使用最小、local、可完整清除的 fixture；不得拿使用者既有 Plugin資料作破壞性測試。

## 17. Implementation order

1. ✅ **A — Core/Desktop authority**
   - registry revision / Plugin generation；
   - Desktop request contract；
   - operation journal / safe errors；
   - Plugin-owned MCP env scope。
2. ✅ **B — Native candidate staging**
   - file/folder picker；
   - opaque candidate；
   - private review token；
   - expiry/discard。
3. ✅ **C — Shared UI**
   - navigation/store；
   - summary/list/detail；
   - install review；
   - update diff；
   - enable/disable/remove；
   - MCP settings；
   - i18n/a11y/responsive。
4. ✅ **D — Independent security review + hardening**
5. **E — Next-only package/live UAT：Desktop bridge PASS；native GUI UAT 待補**

在 D 得到 0 BLOCKER / 0 HIGH 前，不 package/reinstall Next 做 P6 live UAT。

### 17.1 Implementation checkpoint — 2026-10-08

Phase A / B backend + native boundary 已完成，尚未進 package/live UAT：

- `f70a4715 feat(plugin): add Desktop management authority`
  - authoritative registry snapshot、persisted 256-bit Plugin generation；
  - strict action-specific request semantics、revision/generation fence；
  - request-id journal / semantic approval fingerprint / exact retry；
  - enable/disable、remove keep/purge、Plugin-owned MCP write-only environment與 recovery outcome。
- `55e8be0b feat(plugin): add opaque Desktop candidates`
  - native folder / ZIP picker；
  - AgentDock-owned immutable staged snapshot；
  - opaque `candidateId` + server-held review state，renderer不取得 source path / raw review token；
  - install強制 disabled、update綁定 exact target generation並保留既有 Enabled；
  - candidate expiry / discard / single-use；
  - current → candidate safe review projection。
- `30fa143b fix(plugin): harden candidate staging boundary`
  - candidate staging HTTP route從 ordinary Runtime dispatch完全拆離；
  - staging要求 direct loopback + **native Desktop control credential**，ordinary Core bearer不可使用；
  - provenance / warning / issue / description中的 absolute Unix / Windows / UNC path fail-closed redaction；
  - generic Runtime handler不能繞過專用 candidate control route。

Validation：

- `go test ./cmd/... ./internal/...`：PASS；
- P6/backend targeted + `go test -race`：PASS；
- backend Windows/Linux compile-only：PASS；
- Shared Desktop macOS tests + Windows compile：PASS；
- Shared Desktop Linux `server` tag compile：PASS；
- native Linux Wails build需要 Linux + CGO + GTK/WebKit toolchain，保留到 release-native UAT，不視為 P6 failure；
- `go vet` / `git diff --check`：PASS；
- candidate source path / raw review token / storage key / runtime name / credential value不進 renderer-facing candidate response。

下一步：**Phase D independent security review + hardening**。在 reviewer 得到 0 BLOCKER / 0 HIGH 前，仍禁止 package/reinstall Next 做 P6 live UAT。

### 17.2 Phase C review-ready checkpoint — 2026-10-08

Shared Plugin Management 已完成並進入 independent review gate，尚未 package/reinstall Next：

- Shared navigation 新增 **Plugin Management**，並完成 P5 Plugin-owned MCP → P6 Plugin detail deep link；
- summary / installed list / detail / recovery cleanup UX；
- native folder/ZIP install 入口，不提供 renderer path input；
- install review 顯示 identity/version/format/provenance/fingerprint、Skills、MCP、executables、warnings/issues；
- update review 顯示 current → candidate version 與 Skills/MCP/executable/warning semantic diff；
- install 保持 disabled-by-default，enable / disable 均 explicit confirmation 並顯示 local-process / remote side-effect notice；
- Plugin-owned MCP connection values 採 write-only UX，只顯示 key + configured/missing，secret 送出後立即從 frontend state 清除；
- remove 分為 keep data 與 purge data；purge 有第二層 destructive confirmation；
- purge recovery item 提供 Retry cleanup；
- 320px responsive layout 採 list/detail stack、review diff card，不使用寬 table；
- English / 繁體中文 / 简体中文 stable catalog 同步更新並由 generator 產生平台 artifacts。

Validation：

- Shared frontend `vue-tsc --noEmit`：PASS；
- Shared frontend Vitest：**16 files / 110 tests PASS**；
- Shared frontend production build：PASS（169 modules）；
- P6/backend targeted Go tests + Shared Desktop Go tests：PASS；
- `go test -race`（Plugin / tool/plugin / app / runtimeapi / httpx / desktopapi / mcp client）：PASS；
- `go test ./cmd/... ./internal/...`：PASS；
- Windows/Linux P6 backend compile-only：PASS；
- `go vet` / `git diff --check`：PASS；
- Wails Plugin binding 只有 opaque candidate / lifecycle / env actions，沒有任何接收 absolute source path 的 renderer method；
- renderer boundary scan：沒有 `review_token` / storage key / source/package/runtime path / access-refresh token 欄位；
- i18n generator + catalog tests + coverage：PASS，三個 stable locale 均 **742/742 (100%)**；
- repo-wide hard-coded Shared UI scanner 仍會對既有 Permission/ACP literals 回報非零；本次輸出沒有新增 P6 Plugin violation，列為既有 i18n debt，不誤宣稱整體 `i18n-check` 已全綠。

下一步：**Phase E Next-only package/live UAT**。

### 17.3 Phase D independent review closeout — 2026-10-08

單一 Antigravity / Gemini 3.1 Pro reviewer 對 P6 Phase A–C 實作做 independent security + architecture review。

第一輪結果：**0 BLOCKER / 1 HIGH**。

HIGH 根因：candidate safe projection 先刪除 newline/tab/control characters，可能把 prose 與 absolute path 黏成同一 token，繞過 prefix-based absolute-path redaction；同一根因也會讓多行 description 黏字。

Hardening commit：07a1a935 fix(plugin): preserve candidate redaction boundaries

- control characters 改為正規化成空白，不再直接移除；
- Unix / Windows drive / UNC absolute path 保持獨立 token，再由既有 redaction 規則攔截；
- 新增 newline/tab + Unix/Windows/UNC regression test；
- 多行描述同時恢復可讀空白。

修正後 targeted、race、repo-wide regression 與 git diff check 全部 PASS。

同一 reviewer re-review：

**SECURITY VERDICT: PASS — 0 BLOCKER / 0 HIGH**

附帶 reviewer orchestration note：第一輪 reviewer 雖被要求 read-only，仍建立兩個未追蹤 scratch Go 檔做局部實驗；已立即清除，未進 commit、未影響產品狀態。這是 reviewer/tool discipline 問題，不是 P6 product finding。

Phase E 仍由 stable mac-dev 作唯一 mutation control plane；macbook-air-m3 只作部署後 Next read-only validation。

### 17.4 Phase E Next-only live UAT checkpoint — 2026-10-08

**狀態：installed Next / Desktop bridge live UAT PASS；可見視窗的 native GUI / accessibility UAT 尚未完成。P6 尚未正式 closeout。**

- 部署前確認 Next 已安裝 commit `89e34852`，build date `2026-10-08T10:39:17+08:00`，因此本次**未重複打包或安裝**，直接使用現有 Next Core 做 live UAT。
- stable Core 維持 PID `44832` / `127.0.0.1:8765`；Next Core 為 PID `44682` / `127.0.0.1:8767`，Next Tunnel / GUI 同時存在。本次不重啟 stable 或 Next。
- Next public protected-resource metadata 使用正確路徑 `/.well-known/oauth-protected-resource/mcp` 回應 HTTP `200`；未授權 `/mcp` 回應預期 `401`。
- Next connector `macbook-air-m3` 只作 read-only 驗證：Plugin `[]`；standalone MCP 仍只有既有 `memory`。
- 透過**真正 Next runtime 的 native Desktop bridge**、不是 model-facing `plugin_manage`，使用獨立 disposable `p6-live-uat` 本機 folder/ZIP fixture 完成：
  - authoritative registry snapshot、opaque candidate native picker preview / discard；
  - 安裝版本 `1.0.0`，確認預設 `Enabled=false`；
  - stale generation mutation 正確拒絕；operation journal status 可對帳；
  - 無 MCP 的安全 fixture 完成 enable → disable；
  - local ZIP update `1.0.0 → 2.0.0`，review current/candidate 分離，generation 變更並保留 Disabled；
  - Plugin-owned `remote` MCP env snapshot / TOKEN set / configured-only readback / unset，Desktop result 不回傳寫入的測試值；該 remote MCP 始終 Disabled，未啟動對外連線；
  - remove keep 回報 `data_preserved=true`，重新從 ZIP 安裝成功且 Disabled；
  - remove purge 完成，最終 authoritative registry `0 plugins / 0 recovery items`。
- Next private storage 另外保有兩筆 `grant-plugin.p6-live-uat*` JSON，僅含 `schema_version` / 隨機 `epoch`，**沒有 `grant`、access/refresh token**。這是既有 OAuth revocation epoch tombstone，防止 stale OAuth writer 復活舊授權；不是 credential residue，不應手動刪除。
- native GUI process 正常，但 Orca read-only `list-windows` 回傳空列表；在避免強制前景操作的使用習慣下，**本次未做實際視窗的 keyboard/focus/320px visual/native picker walkthrough**。Frontend typecheck / 110 Vitest / production build / i18n 已有 Phase C 自動化證據，但不能替代 native GUI UAT。

**Remaining P6 gate：**取得可見 Next GUI 視窗後，驗證 Plugin summary/empty state、folder/ZIP picker、install/update review、enable/disable、write-only env、keep/purge confirmations、P5 MCP → P6 deep link、鍵盤焦點與窄視窗；完成才可正式 P6 closeout。後續 P7 Nexus / Platform Essentials 可先進 read-only inventory，不得據此提前放行 M9。

## 18. Roundtable decision

**UX VERDICT: READY TO FREEZE**

**SECURITY VERDICT: READY TO FREEZE**，條件已納入上述 contract：renderer zero absolute path / zero raw review token、opaque immutable candidate、install disabled by default、revision + generation fencing、P5-style semantic approval/idempotency binding、Plugin-owned MCP scope isolation與 truthful recovery outcome。

附帶技術債：2026-10-08 roundtable 期間確認 Antigravity 平行 managed sessions可發生 shared brain artifact污染；Codex ACP亦重現「new 成功、下一操作 ACP_SESSION_NOT_FOUND」。P6 不依賴這些 ACP lifecycle 行為，後續另行追蹤，不把 reviewer orchestration 問題混入 Plugin product contract。
