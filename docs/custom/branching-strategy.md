# Branching Strategy

> 狀態：Accepted

## 1. `main`

`main` 是官方 AgentDock upstream mirror。

理想狀態：

```text
jacktdry/agentdock:main
==
uvwt/agentdock:main
```

`main` 不接受：

- custom feature
- Workbench port
- custom branding
- custom release-only change

官方更新流程：

```bash
git fetch upstream &&
git switch main &&

# Reject any local-only commit before syncing.
test "$(git rev-list --count upstream/main..main)" -eq 0 &&

git merge --ff-only upstream/main &&

# Before push, require the mirror SHA to match upstream exactly.
test "$(git rev-parse main)" = "$(git rev-parse upstream/main)" &&
git push origin main
```

第一個 guard 會拒絕任何 local-only commit；第二個 guard 在 push 前要求 SHA 完全一致。若 `main` ahead of `upstream/main`，應視為治理異常並先確認原因，不得 push。

## 2. `custom/main`

`custom/main` 是我們實際整合、測試與發布的正式主線。

```text
upstream/main
      ↓
    main
      ↓
custom/main
```

官方更新進入 `custom/main` 時允許正常 merge，因此衝突只存在於 custom integration 層，不污染 upstream mirror。

正式客製 Release 的**目標來源**是 `custom/main`。

目前官方 release workflow 尚不能直接用於 custom-only commit：

- `.github/workflows/release.yml` 會要求 release source commit 屬於 `origin/main`。
- `tools/release verify-version` 會要求 tag 與官方 `buildinfo.Version` 完全一致。

因此在 M9 完成 custom release pipeline 與 version contract 前，`custom/main` 只作為正式整合主線，不應使用現有官方 workflow 發布 custom Release。

## 3. Branch 類型

### 自研功能

```text
feature/<name>
```

例如：

- `feature/acp-manager`
- `feature/browser-routing`
- `feature/activity-center`

一律從 `custom/main` 建立。

### 架構工作

```text
architecture/<name>
```

例如：

- `architecture/shared-desktop-ui`
- `architecture/desktop-api`
- `architecture/i18n`

一律從 `custom/main` 建立。

### Workbench selective port

```text
port/workbench-<feature>
```

Workbench 不允許直接 merge 到 `main` 或 `custom/main`。每一項採用都先在獨立 branch 分析 dependency、port 或 reimplement，再進入 custom integration。

### 我們自己的修正

```text
custom-fix/<name>
```

只適用於 custom-only 問題，從 `custom/main` 建立。

### 準備貢獻回官方

```text
fix/upstream-<name>
feature/upstream-<name>
```

必須從乾淨的 `main` 建立，不能從 `custom/main` 建立，以避免 upstream PR 夾帶客製內容。

## 4. Release

候選版本格式：

```text
v<upstream-version>-custom.<revision>
```

例如：

```text
v0.9.1-custom.1
v0.9.1-custom.2
```

此格式只是目前偏好的候選方案；正式採用前必須在 M9 一併調整 release source validation、`buildinfo.Version` 契約、update channel 與 installer metadata。未來若 custom distribution 需要獨立品牌版本，再另立 ADR。

## 5. Branch Protection 建議

`main`：

- 禁止 force push
- 禁止 delete
- 不直接開發 custom feature
- CI 可檢查 `main..upstream/main` / `upstream/main..main` 差異

`custom/main`：

- 禁止 force push
- 功能經 feature/architecture/port branch 整合
- Release 前必須通過完整 cross-platform gate

## 6. Worktree 規則

現有 AgentDock 開發大量使用 worktree。為避免未提交工作互相覆蓋：

- 不因建立 `custom/main` 而切換正在工作的舊 worktree。
- 新架構工作使用獨立 worktree。
- 移除 branch / worktree 前先確認 dirty state、未 push commit 與 open upstream PR。
- 官方 PR branch 保留到 upstream PR 有明確結果後再清理。
