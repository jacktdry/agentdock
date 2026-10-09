# Upstream Tracking

> 狀態：Accepted

## 1. Remote 定義

```text
origin
→ jacktdry/agentdock

upstream
→ uvwt/agentdock

workbench
→ A-m-o-r-F-a-t-i/AgentDock-Workbench
```

`upstream` 是唯一 runtime upstream；`workbench` 是 feature radar。

> **2026-10-09 Next 專案決策補充**：第 2 節記載的是歷史上的定期同步程序，**現在不再以追趕 upstream 版本或自動 merge 為開發目標**。本階段僅針對已確認的 upstream release/PR 做一次性差異盤點、明確採用清單與選擇性重作；未經新的合併決策不執行 `upstream/main → custom/main`，不因官方新版本而打斷 Pre-M9/M9。`main` 保留官方相容基準與既有 branch 治理，但不是這一輪的變更目標。

## 2. AgentDock 更新流程

每次官方更新：

1. `git fetch upstream`
2. 閱讀 release notes 與高影響 commits。
3. 確認 `upstream/main..main` 沒有 local-only commit；若有則停止同步並調查。
4. fast-forward `main`。
5. push 前再次確認 `main` SHA 等於 `upstream/main`。
6. 將 `main` merge 進 `custom/main`。
7. 處理 custom integration conflict。
8. 跑 Core / Desktop / i18n / packaging tests。
9. custom release pipeline 完成後，才視需要發布新的 custom revision。

不在 `main` 上解 custom conflict。

目前官方 release workflow 仍限制 release source 必須屬於 `origin/main`，因此 custom-only Release 要等 custom release pipeline / version contract 完成後才能啟用。

## 3. Workbench 更新流程

Workbench 更新不產生自動 merge。

每次只做：

1. 掃描 release / commits / branches。
2. 列出新增或明顯改善的 workflow capability。
3. 與我們現有痛點比對。
4. 確認官方 AgentDock 是否已有等價能力。
5. 分類為 `ADOPT` / `PARTIAL` / `REFERENCE` / `DEFER` / `REJECT`。
6. 需要實作才建立 `port/workbench-*`。

## 4. Port Record

重要 selective port 建議建立：

```text
docs/custom/upstream-ports/<feature>.md
```

至少記錄：

- Source commit / PR / release
- 解決的問題
- 採用範圍
- 被排除的 dependency
- cherry-pick / selective port / reimplementation
- local branch / commit
- 後續維護注意事項

## 5. 優先順序

若官方與 Workbench 都有類似功能：

```text
official AgentDock implementation
        ↓
reuse / extend
        ↓
Workbench concept only if still needed
```

避免維護兩套解決同一問題的 implementation。
