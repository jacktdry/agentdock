# AgentDock Custom 開發文件

> 狀態：Active
>
> 建立日期：2026-10-02
>
> 適用分支：`custom/main`

這個目錄記錄 `jacktdry/agentdock` 客製版本的長期開發策略。客製版仍以 `uvwt/agentdock` 為唯一 primary upstream；AgentDock Workbench 僅作為功能與架構參考，不作為第二 upstream。

核心目標不是建立另一個 Workbench，而是在保留官方 AgentDock 更新能力的前提下，只加入對我們實際 AI 開發工作流程有價值的能力。

## 文件索引

- [architecture.md](architecture.md)：整體產品與技術架構。
- [branching-strategy.md](branching-strategy.md)：`main`、`custom/main`、feature、Workbench port 與 upstream PR 的 Git 規則。
- [upstream-tracking.md](upstream-tracking.md)：AgentDock / Workbench 更新的追蹤與採用流程。
- [workbench-adoption.md](workbench-adoption.md)：Workbench 功能採用、延後與排除清單。
- [desktop-ui.md](desktop-ui.md)：macOS / Windows 共用 Desktop UI 的技術方向與 POC 驗收條件。
- [i18n.md](i18n.md)：跨平台多國語系、志願翻譯與 CI 架構。
- [roundtable-2026-10-02.md](roundtable-2026-10-02.md)：本次 AI 圓桌的分歧、交叉挑戰與最終收斂。
- [roadmap.md](roadmap.md)：Milestone、優先級與 Definition of Done。

## 不可破壞的原則

1. `main` 保持可與 `uvwt/agentdock:main` fast-forward 對齊。
2. 客製功能只進 `custom/main` 或從它衍生的 branch。
3. 正式客製 Release 的目標來源是 `custom/main`；在 custom release pipeline 完成前不得沿用官方 workflow 強行發布 custom-only commit。
4. Workbench 不整體 merge，只做 selective adoption。
5. 準備回饋 upstream 的修正必須從乾淨的 `main` 建立。
6. 新 Desktop 業務功能原則上只實作一次，不再長期維護 AppKit 與 WPF 兩套平行 UI。
7. 使用者可見字串不得散落硬寫在 Swift、C#、Vue 或驗證腳本中。
