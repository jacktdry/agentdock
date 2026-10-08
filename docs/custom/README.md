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
- [m8-permission-approval.md](m8-permission-approval.md)：M8 Core-owned Permission / Approval authority、binding、retry、history 與 implementation checkpoint。
- [pre-m9-feature-parity.md](pre-m9-feature-parity.md)：M8 closeout 後的 feature parity audit、Next Connection/Auth/Tunnel readiness、已完成的 P3 stable/Next side-by-side closeout、Wave 3 與 M9 前工作順序。
- [pre-m9-wave0-contract.md](pre-m9-wave0-contract.md)：Wave 0 roundtable 收斂後凍結的 P1/P2 UX、secret、port ownership、Tunnel state/mutation 與 Wave 1 backend integration contract。
- [pre-m9-acp-manager-roundtable.md](pre-m9-acp-manager-roundtable.md)：Wave 3 P4 ACP Manager 的 UX/IA、設定 authority、revisioned save、Antigravity preset、安全 update contract 與 closeout。
- [pre-m9-mcp-management-roundtable.md](pre-m9-mcp-management-roundtable.md)：Wave 3 P5 MCP Management 的 standalone/plugin ownership、revision/generation、credential/OAuth、reconnect 與 Shared UI authority contract。
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

8. stable `mac-dev` 是 production control plane，也是 **Next 開發與維護唯一允許執行 mutation 的 control plane**；Next 開發不得檢查、修改、重啟、停止或替換 `/Applications/AgentDock.app`、stable Core、`~/.agentdock`、stable Memory registry 或 live launchd services，也不得把 stable runtime 當 development target。Next connector（ChatGPT 顯示 `macbook-air-m3`）只作 Next connector/runtime/side-by-side 驗證，不得修改、編譯、重裝或修復自己。
9. 客製版以 **AgentDock Next** side-by-side 開發，app identity、服務、port、runtime/state/log/work roots 與 connector 必須隔離；installer / self-update 必須 Next-target-aware 並 fail closed，禁止 fallback 到 `AgentDock.app`。
10. M7 code/stress 已於 `d8acb9be` 整合完成；M7.5 AgentDock Next isolation 的 repository/runtime validation 與 handoff 已完成，stable live cutover 仍刻意延後。M8 implementation / available-host validation 已完成：API `652331ed`、Shared Permission UI / authority hardening `3df9a784`；完整證據見 [M8 closeout](m8-permission-approval.md#m8-closeout--2026-10-05)。Pre-M9 P1/P2 live readiness 已完成：Shared UI、Next Core `8767`、獨立 Named Tunnel `mac-dev-next.dropabit.dev`、OAuth/public MCP discovery 皆已部署；connector copy 已以 `a7e92f2c` 改用 Wails native clipboard。使用者已建立 Next ChatGPT connector，顯示名稱為 `macbook-air-m3`；P3 stable/Next side-by-side validation 已於 2026-10-06 完成，確認 Core/endpoint/state/task/registry/service ownership 隔離與 Next-only lifecycle 不影響 stable。Wave 3 第一個 domain **P4 ACP Manager** 已於 2026-10-06 完成、review 並部署至 Next（final implementation checkpoint `c58a6f93`）。**P5 MCP Management** 已於 2026-10-07 完成第一輪 6 HIGH hardening、Gemini 3.1 Pro final independent review（0 BLOCKER / 0 HIGH）、Next-only package/deploy 與 Desktop bridge live UAT，正式 closeout；詳見 [P5 closeout](pre-m9-mcp-management-roundtable.md#21-p5-closeout--2026-10-07)。**P6 Plugin Management** 已於 2026-10-08 凍結 UX / IA + authority contract；Phase A Core/Desktop authority（`f70a4715`）、Phase B opaque candidate staging / security hardening（`55e8be0b`、`30fa143b`）、Phase C Shared UI（`999128d2`）與 Phase D independent review / hardening（`07a1a935`；0 BLOCKER / 0 HIGH）已完成，Phase E Desktop/Core live UAT 已通過、native GUI/keyboard UAT 待補，尚未正式 closeout；詳見 [P6 contract + implementation checkpoints](pre-m9-plugin-management-roundtable.md#171-implementation-checkpoint--2026-10-08)。後續依 Nexus / Platform Essentials → Browser Broker 推進。Developer ID signed updater、Windows/WSL/Linux helper native evidence仍作為 M9/release-native gate。
11. 未來更名是 packaging / identity migration，不是現在共用 runtime state 的理由。完整不可變邊界見 [隔離契約](agentdock-next-isolation.md)。
