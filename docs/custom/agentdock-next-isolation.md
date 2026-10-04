# AgentDock Next Isolation

> 狀態：Accepted durable decision / implementation and validation pending
>
> 日期：2026-10-05
>
> 適用分支：`custom/main`；M7 code / stress 已於 `d8acb9be` 整合完成

## 決策與 side-by-side 架構

先前對已安裝 pre-M7 AgentDock 的 live activation 嘗試造成 ChatGPT Mac-Dev control channel 斷線。目前連線中的 **AgentDock 是 production control plane**，必須保持可用；客製版以 **AgentDock Next** 作獨立 development plane 並行開發。這取代先前 M7 direct stable activation / Memory registry cutover gate，不推翻 M7 feature code / stress 的完成狀態。

Next 沿用 custom Core / Shared Desktop 架構，但 app identity、service ownership、runtime/state、tunnel / connector 與 update target 均獨立。ChatGPT 現有 Mac-Dev 仍連 stable；未來 `mac-dev-next` 單獨連 Next Core，不重指向原 connector。

唯一規劃中的 shared service 是 Memory HTTP `http://127.0.0.1:8766/mcp`。Next registry/config 固定使用 `protocol_version=2025-11-25`，保存在 Next state；共用 endpoint / DB 不等於共用 AgentDock registry、runtime state 或 daemon lifecycle authority。

## 不可破壞的 invariants

- Next 開發不得檢查、修改、重啟、停止、替換或寫入 `/Applications/AgentDock.app`、目前 stable AgentDock Core、`~/.agentdock`、stable Memory registry 或 live launchd services；stable 不得作 development target。
- Next 的 start / stop / restart、settings、diagnostics、tunnel、login item、ACP / Browser cleanup 及 installer / self-update 只可操作可證明為 Next-owned 的目標。禁止全域 kill、按 app basename 猜測 ownership、清除 stable children 或接管使用者 browser。
- 不從 stable state 原地轉換、搬移或建立可寫 symlink；Next 不因缺少 state/config 而讀寫 stable fallback。明確 Next namespaces 未建立前，不啟動 runtime / service 實驗。
- Bundle identifier、service label、paths、port、connector 必須一致驗證。缺失、衝突、identity 不符或 ownership 不明時 fail closed。
- Installer / self-update 必須 **Next-target-aware**：以 Next identity 驗證來源、destination、runtime/service targets 與 update metadata；永不得 fallback 選擇 `AgentDock.app`。失敗 / rollback 只影響 Next。
- Shared Memory daemon、stable stdio Memory children、stable launchd services 都不是 Next cleanup 目標。Memory endpoint 不可用時停止 Next cutover，不修理或重啟 live shared service。
- 只有 Next 完整開發、測試且能獨立連上 ChatGPT 後，才另行規劃並授權 stable migration / retirement；開發 gate 不包含退休 old AgentDock。

## Namespace matrix（proposed defaults）

以下是待實作 / 驗證的預設契約，非已安裝或正在運行的宣稱。Stable identity 只記錄已知邊界，不以本文件要求現場探查。

| 項目 | Stable production plane | AgentDock Next development plane |
| --- | --- | --- |
| App | `/Applications/AgentDock.app`，禁止操作 | `AgentDock Next.app`；installer destination `/Applications/AgentDock Next.app` |
| Bundle identifier | 保留既有，不探查 / 更改 | `dev.dropabit.agentdock.next` |
| Core LaunchAgent | 保留既有 live service | `dev.dropabit.agentdock.next.core` |
| Tunnel LaunchAgent | 保留既有 live service | `dev.dropabit.agentdock.next.tunnel` |
| Menu login LaunchAgent | 保留既有 live service | `dev.dropabit.agentdock.next.menu-login` |
| Application Support / runtime root | 保留既有，不共用 | `~/Library/Application Support/AgentDock Next` |
| Logs | 保留既有，不共用 | `~/Library/Logs/AgentDock Next` |
| State / `AGENTDOCK_HOME` | `~/.agentdock`，禁止操作 | `~/.agentdock-next` |
| Default work directory | 保留既有，不共用 | `~/AgentDock Next` |
| Core port | `8765`，保持原樣 | `8767`；衝突時 fail closed，不回退 `8765` |
| ChatGPT connector | 原 Mac-Dev control channel | 未來獨立 `mac-dev-next` |
| Memory registry/config | 原 stdio registry，禁止操作 | Next state 內獨立 registry → HTTP `http://127.0.0.1:8766/mcp`，pin `2025-11-25` |
| Installer / self-update | 禁止作開發 target | 僅 Next identity / paths / services；fail closed，無 stable fallback |

Next LaunchAgent plist filenames 以各 label 加 `.plist` 對應；實作前須完成 target-isolation 檢查，不能在準備 namespace 時操作 live launchd services。Tunnel 必須只指向 Next port，credential/config 與 endpoint ownership 留在 Next state，不沿用或覆寫 stable tunnel identity。平台 credential、login item、update channel / artifact metadata 等任何額外 persisted identity，也必須 Next-scoped；不明確時不安裝或啟動。

日後若將 **AgentDock Next** 更名，視為獨立 packaging / identity migration，需完整處理 bundle、service、paths、installer/update metadata 與 connector 的一致性；更名不是現在共用 runtime state 的理由，也不能暗中取得 stable ownership。

## Rollout sequence

1. **文件決策**：本次僅記錄 contract；不改 source、安裝 app、註冊服務、寫 registry 或探查 live runtime。
2. **M7.5 namespace 實作**：後續 bounded task 在 repository 內實作 Next identity / target resolution、runtime/state/log/work roots、port / tunnel 與 installer/self-update fail-closed；先用 isolated fixtures 驗證，不碰 stable。
3. **Next-only package / runtime 驗證**：確認各 mutation surface 的 Next ownership，再於獨立 Next roots 驗證 install、start/stop/restart、settings、login/tunnel、update、failure/rollback 與 cleanup。不得操作 stable service；所需新 Next service 安裝不在本次 docs task 範圍。
4. **Next Memory cutover**：只寫 Next registry/config，以 `2025-11-25` pin 使用既有 shared HTTP endpoint；驗證 health、ACP sessions 與無 per-session stdio Memory child。Stable registry/children 保持不動。
5. **Next regression / independent connector**：重跑 M6/M7 relevant regression、ACP prompt-driven stress 與 Desktop checks，建立獨立 `mac-dev-next` ChatGPT 連線；由原 control channel 持續可用的連線結果證明隔離，不探查 / 操作 stable runtime。
6. **M8/M9 與未來 migration**：M7.5 為 live migration 前置；Next 完整開發 / 測試並可獨立連線後，再另行提出含 recovery / rollback 的 migration 與 old AgentDock retirement 計畫。不存在立即 stable activation 的下一步。

## Acceptance criteria

- Diff / build artifact metadata 可證明 Next app、bundle、三個 service labels、roots、port 與 connector namespace 一致；本文件 defaults 尚未代表 source 已完成。
- Target-resolution / installer / self-update tests 覆蓋 missing Next app、identity mismatch、stable-only installation、port collision 與 rollback；全部 fail closed，沒有選擇 `AgentDock.app` 或 stable state 的 fallback。
- Next Runtime / Connection / Settings / Update / Diagnostics 與 process cleanup 只作用於 Next-owned targets；Next lifecycle / crash / rollback 不使原 Mac-Dev control channel 中斷。
- Next Memory registry 位於 `~/.agentdock-next`，HTTP handshake / health 使用 `2025-11-25`；Next 新 ACP 不 spawn stdio Memory child。不得以 stable stdio child 歸零作 gate。
- Next 中完成 M6/M7 lifecycle / Browser ownership regression 與 Codex、Antigravity 各 20 prompt-driven ephemeral stress；只以 Next-owned resources 回 baseline 驗收。
- Next 可透過獨立 `mac-dev-next` 連上 ChatGPT；獨立 connector 不替換原 Mac-Dev。
- 驗證證據明列 Next targets、實際執行 checks、失敗與未驗證部分；stable app / Core / state / registry / live services 未受操作。Stable migration / retirement 仍是後續另行授權的工作。

相關文件：[Roadmap](roadmap.md)、[Architecture](architecture.md)、[ACP lifecycle / Memory](acp-lifecycle-memory.md)。
