# Pre-M9 Wave 3 — ACP Manager UX / IA Contract

> 狀態：**Roundtable frozen for implementation（2026-10-06）**
>
> 前置：P3 stable / Next side-by-side validation 已 closeout。此文件只定義 Wave 3 第一個 domain「ACP Manager」的 Shared Desktop 產品 / API contract，不重開 P3，也不進 M9。

## 1. 目標與邊界

ACP Manager 的主要工作不是展示 ACP internals，而是讓使用者能安全回答並完成：

- 現在有哪些 Coding Agent profile？
- 哪個 profile 會作為預設？
- 某個 adapter 是否可用、目前版本與是否可更新？
- 如何新增、修改、停用或移除 profile？
- Runtime/session 為什麼仍在執行或佔用資源？

不可破壞的邊界：

- stable `mac-dev` 仍是 Next repository / build / install / service / tunnel / live-runtime mutation 的唯一開發 control plane。
- ChatGPT 顯示為 `macbook-air-m3` 的 Next connector 只作 connector/runtime validation。
- **上述規則不代表 AgentDock Next 產品 UI 本身必須唯讀。** ACP Manager 的產品功能就是透過可信任 Desktop/Core API 管理 **Next-owned ACP configuration**。
- frontend 不直接執行 shell、不自行解析 executable/package、不直接修改 env/service files。
- secrets、provider tokens、Desktop-control credential、raw adapter stderr/stdout 不得進普通 frontend state。
- M7 session lifecycle / diagnostics 與 M6 Browser/Computer ownership 保持既有 authority；ACP Manager 不重新實作 Browser Broker。

## 2. Page goal / hierarchy

單一 ACP Manager page，資訊層級固定為：

1. **Summary**
   - ACP enabled / disabled
   - default profile
   - configured profile count
   - active / loaded session count
   - attention-required count
   - Refresh
2. **Profiles**
   - 所有 configured profiles，包括 disabled profiles
   - Primary action：**Add profile**
   - profile row/card 顯示：
     - display name
     - preset / adapter type
     - Enabled / Default badge
     - availability / installed state
     - installed version
     - latest-version state
     - attention / error summary
3. **Runtime**
   - 保留既有 M7 session list、adapter process、lifecycle policy、manual close、resource correlation
   - 以 progressive disclosure 顯示 process PID / descendants / RSS、close reason、cleanup failure、Browser/Computer correlation
4. **Diagnostics**
   - Shared Memory health
   - ACP-specific safe diagnostics
   - 不把 Browser Broker 操作搬進此頁

窄視窗改成垂直 stack；320 CSS px 或等價 zoom 下不得要求 page-level horizontal scrolling。

## 3. Profile actions

### Primary

- Add profile

### Secondary

- Edit
- Enable / Disable
- Make default
- Detect / Re-check
- Check version
- Update adapter（僅 backend 宣告可安全更新時）
- Delete

Delete 放在 secondary / danger action，不與 Add / Edit 同層強調。

### 不需要 confirmation 的操作

- Refresh
- non-disruptive Edit/Save
- Make default
- Detect / Check version

成功用 inline status / live region 回饋，不彈 modal。

### 需要 confirmation 或 backend-impact preview

- Delete profile
- Disable / delete current default 且會造成 default invalid
- adapter update
- backend 明確回報會中斷 loaded/running sessions 的 config change
- manual Close session（沿用 M7）

## 4. Add / Edit flow

使用同一個 accessible editor surface；桌面可為 dialog，窄視窗可 full-width。

### Step A — Adapter

新 profile 提供三個主要選項：

- **Codex**
- **Antigravity**
- **Custom adapter**

Wave 3 不把 legacy Claude / Grok 當主要 preset；既有 profile 必須 preserve / render，不可自動轉換或刪除。

### Step B — Configuration

Codex / Antigravity：

- 顯示 backend detection 結果與簡短說明
- 預設隱藏 command / args internals
- 若 backend 允許 explicit override，放在 Advanced
- detection 不得無提示覆寫既有 custom command

Custom：

- display name
- executable
- args 使用 ordered string list，不要求使用者輸入 shell command 或 JSON 字串
- 不在本輪把 raw environment / credential editor 平鋪到一般 UI

### Step C — Save

- Enabled
- Default（只有 enabled profile 才可選）
- save 前顯示 backend validation / impact
- save failure 保留 draft
- conflict 保留 draft，要求 reload/review 後再提交
- 成功 persistence 後才關 editor

### Profile identity

- Profile ID 由 backend 建立 / 正規化。
- Edit 時 ID immutable。
- 變更 adapter family 不 silently rebind 原 profile；應新增另一 profile。
- Codex preset 維持既有 built-in invariant。
- Antigravity 在 Wave 3 先作 **first-class UI preset → existing custom runtime kind**，由 backend 回傳 preset/source metadata；不為了 UI 新增新的 Core ACP kind。

## 5. Default / enable invariants

- ACP global disabled 時仍可讀取、編輯 profiles。
- ACP enabled 時至少需要一個 enabled + usable profile。
- default profile 必須指向 enabled profile。
- Disable / Delete current default：
  - 有其他 eligible profile 時，UI 要求使用者明確選 replacement；
  - 沒有 replacement 時，必須 explicit 同時關閉 ACP，不能 silent fallback。
- 不沿用舊 native「自動挑第一個 profile」的隱性行為。

## 6. Desktop API contract

現有 `ACPService.Status()` 保留為 runtime/diagnostics snapshot；**不得拿它當設定 inventory**，因為目前 ACP off / disabled profile 會被省略。

新增獨立 configuration contract。

### 6.1 Read model

建議：

~~~text
ACPManagerSnapshot
  revision
  enabled
  defaultProfile
  profiles[]
  capabilities
  error?

ACPManagedProfile
  id
  displayName
  runtimeKind
  preset               // codex | antigravity | legacy | custom
  source
  enabled
  configuredCommand?   // backend-redacted/safe representation only
  configuredArgs?      // only values explicitly safe for UI round-trip
  availability
  installedVersion?
  latestVersion?
  versionState
  canDetect
  canUpdate
  blockedReason?
  activeSessionCount
~~~

要求：

- 包含 disabled profiles。
- ACP off 仍可讀。
- Core unavailable 時只讓 runtime observation degraded；config inventory 若 Desktop backend 可讀，仍應存在。
- unknown ≠ not installed；lookup failure ≠ current。

### 6.2 Mutation model

Profile CRUD / enable / default 採 **單一 revisioned atomic save**，而不是每個 toggle 一個 persistence endpoint：

~~~text
SaveSettings(expectedRevision, enabled, defaultProfile, profiles[]) -> ACPSettingsMutationResult
~~~

理由：

- default / enabled / delete 是同一組 invariant。
- 可一次驗證 candidate config。
- 可保證 atomic persistence。
- 可做 stale revision reject。
- 可保留未暴露到 UI 的 backend-owned fields。

backend 必須：

- serialization / locking
- validate full candidate
- stale revision fail closed
- preserve unrelated settings
- 對既有 profile 以 immutable ID merge，保留 UI 未管理的 `env_from_env` 等欄位
- write/apply failure rollback
- 回傳 persisted / applied / restartRequired / runtimeImpact 的結構化結果
- 不回傳 raw file content / secret value

### 6.3 Detection / version / update

獨立 operations：

- `ProbeProfile(profileID | draft)`
- `CheckProfileVersion(profileID)`
- `UpdateProfileAdapter(profileID, expectedRevision?)`

責任全部在 Desktop/backend：

- executable resolution
- package/source identity
- installed version
- supported latest version
- update ownership
- update execution
- rollback / failure mapping
- platform differences

Frontend 只 render structured state。

## 7. Update ownership

Update 是本輪最嚴格的安全 gate。

- globally shared adapter 不可因「Next UI 點 Update」就被原地更新，否則可能影響 stable。
- 只有 backend 能證明 **Next-owned installation target** 時才 `canUpdate=true`。
- Codex / Antigravity 的 source/channel 必須明確；「latest」指 AgentDock 支援的 approved channel，不代表任意 upstream 最新版。
- Antigravity 必須辨識：
  - adapter version
  - AGY CLI version
  - approved fork/channel
  這三者不得混成一個 version。
- Update 前 backend 回傳 impact：active sessions、restart need、target path/source/version。
- 不 silently close session，不 silent update shared/global install。
- custom adapter 預設可 Detect；沒有可信 update provider 時顯示 `unsupported`，不是 error。

## 8. Runtime interaction

Configuration save 與 existing session lifecycle 分開。

- 一般 config change 預設只影響 future launches。
- 若 backend 需要 restart / reload，必須明確回傳 impact，由 UI confirm。
- Delete / Update 若存在 backend 判定不可安全處理的 loaded/running session，block 並提供「查看 sessions」。
- Close session 仍表示釋放 loaded runtime，不等於刪除 remote transcript。
- Lifecycle UI 建議 plain-language label：
  - Keep open → persistent
  - Close after task finishes → ephemeral
  - Close when idle → idle-managed

技術值仍可在 Details 顯示。

## 9. State / recovery UX

必須處理：

- first load：loading/skeleton，不閃現 empty
- no profiles：解釋 profile + Add profile
- ACP off：profiles 仍 editable，清楚說只是停用 launch
- refresh：保留 prior content / focus / expanded state
- Core unavailable：config 可讀則保留，runtime 標 stale/unavailable
- per-profile partial failure：不隱藏其他 healthy profiles
- version lookup failure：保留 installed version，提供 targeted Retry
- stale revision conflict：保留 draft，要求 reload/review
- mutation outcome unknown：先 reconcile 再允許 retry，避免 duplicate mutation
- repeated click：pending 時 disable duplicate submit

所有 disabled action 需有 nearby reason，不依賴 hover-only tooltip。

## 10. Accessibility

- semantic headings / list / definition list
- switch/checkbox/select/input 有 explicit label
- status 不只靠顏色
- action accessible name 包含 profile，例如「Edit Codex」
- dialogs：initial focus、focus containment、Esc-before-submit、close 後 restore focus
- validation errors 關聯欄位並可 focus first invalid field
- operation result 用 polite live region
- polling refresh 不持續打斷 screen reader
- keyboard-only 可完成所有 CRUD / default / detect / update / retry

## 11. 不從 native UI 原樣搬回來

- hidden click-to-edit labels
- JSON-only args editor
- fixed-size desktop dialogs
- enable preset 時直接隱式建立 profile
- disable default 時 silent pick first profile
- service-wide broad Apply / Restart 當作每次 ACP save 的固定副作用
- platform resolver duplicated into Vue
- raw command / stderr / secret render
- legacy Claude/Grok preset-first IA

## 12. Implementation slices

### Slice A — Profile configuration parity

- configuration snapshot + revision
- atomic save
- disabled/off inventory
- Add/Edit/Delete
- Enable/Disable
- Default
- Codex / Antigravity / Custom editor
- backend preservation of hidden profile fields

### Slice B — Detection / version

- backend-owned resolver
- availability
- installed version
- latest-version state
- Retry / unsupported / unknown states

### Slice C — Safe adapter update

- Next-owned target proof
- approved source/channel
- impact preview
- update + rollback
- post-update re-probe/reconcile

## 13. Acceptance criteria

- Add/Edit/Delete、Enable/Disable、Default 在 ACP off 時仍可管理。
- ACP enabled 不可能保存「沒有 enabled profile」或「default 指向 disabled/missing profile」。
- stale revision 不覆寫另一個 writer 的變更。
- failed save 保留 draft；unknown mutation 先 reconcile。
- Codex、Antigravity、Custom 三種主要 flow 可用；legacy profile 不丟失。
- editing profile 不改 ID；hidden `env_from_env` / backend-owned fields 不因 save 被清掉。
- unknown availability/version 不誤顯示為 absent/current。
- Antigravity preset 不需要新增 Core runtime kind。
- Update 只有在 Next-owned target + approved source 可證明時可執行。
- Custom adapter 沒有 updater 時顯示 unsupported，不假裝可更新。
- existing M7 lifecycle/session/process/resource diagnostics 完整保留。
- Browser Broker 不被 duplicate 到 ACP Manager。
- provider secrets、raw adapter error/command output 不進 ordinary frontend state。
- frontend tests/typecheck/build、Go focused tests、contract tests、i18n validation 與 M7 regression 全數通過。

## 14. Roundtable notes

本次 roundtable 使用 Product/UX、Security/Operator、Cross-platform/Implementation 三類視角收斂。

一個 reviewer 曾把「Next connector 僅可驗證」誤解成「AgentDock Next UI 必須全域唯讀」；此建議已明確駁回。正確界線是：

- connector / 開發 control plane 與產品功能是不同層；
- 本開發 session 的 live mutation 只能由 stable `mac-dev` 執行；
- 完成後的 AgentDock Next ACP Manager 仍必須能透過 backend authority 管理 Next-owned ACP settings。
