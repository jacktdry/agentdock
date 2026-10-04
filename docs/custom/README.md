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
- [agentdock-next-isolation.md](agentdock-next-isolation.md)：AgentDock Next side-by-side 隔離、namespace、rollout 與驗收契約。
- [branching-strategy.md](branching-strategy.md)：`main`、`custom/main`、feature、Workbench port 與 upstream PR 的 Git 規則。
- [upstream-tracking.md](upstream-tracking.md)：AgentDock / Workbench 更新的追蹤與採用流程。
- [workbench-adoption.md](workbench-adoption.md)：Workbench 功能採用、延後與排除清單。
- [desktop-ui.md](desktop-ui.md)：macOS / Windows 共用 Desktop UI 的技術方向與 POC 驗收條件。
- [i18n.md](i18n.md)：跨平台多國語系、志願翻譯與 CI 架構。
- [m3-i18n-foundation.md](m3-i18n-foundation.md)：M3 實作、native bridge、migration boundary 與驗證方式。
- [roundtable-2026-10-02.md](roundtable-2026-10-02.md)：本次 AI 圓桌的分歧、交叉挑戰與最終收斂。
- [roadmap.md](roadmap.md)：Milestone、優先級與 Definition of Done。
- [acp-lifecycle-memory.md](acp-lifecycle-memory.md)：ACP session lifecycle、共享 Memory daemon 與 Adapter process cleanup 的實作交接。
- [browser-cdp-lifecycle.md](browser-cdp-lifecycle.md)：Browser / CDP process ownership、stale cleanup、Edge connector 去重與 ACP browser child lifecycle。
- [computer-use-backends.md](computer-use-backends.md)：Computer Use provider 選型、Orca / OpenAI Sky 實測、no-focus policy 與 ACP Computer Control Broker 邊界。
- [cbm-lifecycle-conflict.md](cbm-lifecycle-conflict.md)：Codebase Memory 更新後 supervisor / worker build 衝突、現場證據與後續 lifecycle 改善方向。

## 不可破壞的原則

1. `main` 保持可與 `uvwt/agentdock:main` fast-forward 對齊。
2. 客製功能只進 `custom/main` 或從它衍生的 branch。
3. 正式客製 Release 的目標來源是 `custom/main`；在 custom release pipeline 完成前不得沿用官方 workflow 強行發布 custom-only commit。
4. Workbench 不整體 merge，只做 selective adoption。
5. 準備回饋 upstream 的修正必須從乾淨的 `main` 建立。
6. 新 Desktop 業務功能原則上只實作一次，不再長期維護 AppKit 與 WPF 兩套平行 UI。
7. 使用者可見字串不得散落硬寫在 Swift、C#、Vue 或驗證腳本中。

8. 目前連線中的 AgentDock 是 production control plane；Next 開發不得檢查、修改、重啟、停止或替換 `/Applications/AgentDock.app`、stable Core、`~/.agentdock`、stable Memory registry 或 live launchd services，也不得把 stable 當 development target。
9. 客製版以 **AgentDock Next** side-by-side 開發，app identity、服務、port、runtime/state/log/work roots 與 connector 必須隔離；installer / self-update 必須 Next-target-aware 並 fail closed，禁止 fallback 到 `AgentDock.app`。
10. M7 code/stress 已於 `d8acb9be` 整合完成；M7.5 Phase 1 identity/runtime isolation 已於 `architecture/agentdock-next-isolation` 的 `b189ee5` 完成並通過 fixture/typecheck/Go 測試與第二模型 review。現在接續 Phase 2 self-update / updateplatform / arbiter target isolation；stable live cutover 仍刻意延後。只有 Next 完整開發、測試並可獨立連上 ChatGPT 後，才可另行規劃與授權舊 AgentDock migration / retirement。
11. 未來更名是 packaging / identity migration，不是現在共用 runtime state 的理由。完整不可變邊界見 [隔離契約](agentdock-next-isolation.md)。
