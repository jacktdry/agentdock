# NEXT 階段一：分支／工作樹／Milestone 整合盤點（2026-11-11，Asia/Taipei）

> **範圍與界線：**回應使用者要求先做不需 Codex ACP 的「階段一：分支與 Milestone 統整」。工作流程痛點是 NEXT RC20 已在本機安裝，但驗收／部署修正分散在多個分支與未提交文件，盲目整合恐遺漏進度或污染 Original。交付是**可追溯的唯讀比對與後續合併順序**，不是 Merge、Rebase、Push、發版、重裝、啟動 Browser/Computer/ACP 或清理任何其他工作樹。此文件中的 Git 身分及遠端差距是盤點時快照，未來操作前須重查。

## 1. 已核對 Git 拓樸與本機現役身分

```text
origin/custom/main       4a6c24af  (git ls-remote origin refs/heads/custom/main 已核對)
  └─ +312 commits
custom/main              7e87605a  (clean；本機整合基準，遠端尚未包含)
  └─ +49 commits
feature/next-edge-rc-20261023 4af20cdb (原 NEXT 開發樹，有未提交修改)
  └─ +1 commit (ACP A1 有界 pipe write／cancel)
feature/next-a1-rc20-20261105 704bb40d (clean，已簽章 RC20 來源)
  └─ +4 commits (2 部署相容性修正、2 Host／backlog 文件)
fix/next-rc19-to-rc20-deploy-compat-20261111 86ff4cf0 (盤點前 clean HEAD)
```

- `git merge-base --is-ancestor` 已證實每段都是祖先／後代關係：從本地 `custom/main` 到 `86ff4cf0` 為**54 個提交**，相對 `custom/main` 有 **164 檔、9,923 insertions／219 deletions**。在這個單一路徑上沒有平行分叉衝突；**理論上可 fast-forward，但尚未執行**。
- 目前 `origin/custom/main` 實際遠端 hash 與本地 remote-tracking ref 同為 `4a6c24af`；本地 `custom/main` 已先行 **312 個提交**，到 `86ff4cf0` 將是遠端之後 **366 個提交**。**絕不可把這視為只需 Push 4 或 54 個提交**。先審核整段發布範圍、GitHub 權限／CI、安全與發行政策，再另行授權。
- 正式安裝 NEXT Core `0.9.2-rc.20`、signed source `704bb40d`、Bundle `21.0.0` 的 Host 與回滾證據見 [RC20 installed Host](next-rc20-installed-host-2026-11-11.md)。此獨立交接文件不改動既有已簽章的 RC20 candidate branch 或已安裝 App。
- 全倉庫查得 **19 個 worktrees**；`main`／`custom/main`／RC20 candidate／compat branch 均 clean。`feature/p10-release-migration` 工作樹顯示 `locked initializing`，本輪不解鎖或清理，以免踩到其他 owner。

## 2. 原 NEXT 開發樹的未提交內容：哪些已涵蓋、哪些要保留

原工作樹：`/Users/wei/sideProject/agentdock-next-edge-rc-20261023`，HEAD `4af20cdb`，共有 **5 個 tracked edits ＋ 7 個 untracked**；不得 reset、stash、rebase、overwrite、清理或以 dirty 樹直接打正式包。

| 類別 | 檔案／範圍 | 精確核對結論與下一步 |
| --- | --- | --- |
| A1 程式 | `internal/acp/connection.go`、`internal/acp/steering_compatibility_test.go`、`pipe_write_unix.go`、`pipe_write_windows.go`、`write_timeout_test.go` | **5 檔均與 RC20 工作樹相同 bytes**；已由 `704bb40d` 正式提交，不得重複套用舊 dirty patch |
| A1 相關 3 份文件 | `next-a1-staged-core-host-2026-11-05.md`、`next-acp-write-bounded-2026-11-05.md`、`next-original-v111-selective-adoption-2026-11-04.md` | RC20 已有同名檔；新分支比舊工作樹各多 1 個連結／後續證據小節。以較新內容為準，保留舊樹不動 |
| 獨立候選記錄 | `next-a1-rc20-candidate-2026-11-05.md` | **舊工作樹 untracked 獨有**，尚未納入 RC20/compat。是 11/05 歷史 source/Host 證據，不得按其中「installed RC19」文字當作 11/11 現況；之後可逐項保存為歷史 appendix |
| 工作樹進度文件 | `docs/custom/next-active-backlog-2026-10-21.md` | 舊樹新增 11/04–11/05 A1／Original 比較；compat 已有 11/11 installed Host 最新狀態。應採**歷史交接合併、不可整檔覆寫** |
| 工作樹 Roadmap | `docs/custom/roadmap.md` | 舊樹僅在頁首增加 11/04 checkpoint，內容「NEXT 仍 RC19」已過期；不可原封搬入最新總覽 |
| Pre-M9 歷史維護 | `docs/custom/pre-m9-progress-2026-10-09.md` | 舊樹獨有 11/02 NEXT／Original Cloudflared Tunnel 維護與回滾證據；可保存為**帶日期的歷史事件**，但不能把當時 PID／版本當成現役狀態 |

其他未提交、必須維持原狀的工作樹：`agentdock-m6-acp-tests`（8 tracked＋9 untracked）、`agentdock-p5-mcp-oauth-fence`（5 tracked）、`agentdock-original-maintenance-20261021`（1 tracked）。M6／OAuth 的**branch HEAD** 已是 compat 祖先，不代表這些工作樹的**未提交變更**已包含。

## 3. 不可直接一起合併的其他實驗分支

| 分支 | compat 之外獨有提交 | 決策 |
| --- | ---: | --- |
| `feature/next-p13-single-owner-integration-20261022` | 8 | **不合併**：依 [10/22 使用者 P13/P14 決策](p13-p14-ux-scope-decision-2026-10-22.md) 保留研究，除非真實 NEXT Compact Task Card 回歸 |
| `feature/next-mcp-card-ux` | 20 | 另案確認收益與範圍；不可因其為 UI 分支而直接匯入 RC20 |
| `feature/next-skill-cbm-integration` | 2 | 獨立 Skill／Codebase Memory workstream，先確認安全與使用者需求，再決定是否採納 |
| `feature/p11-adhoc-native-uat`、`feature/m6-acp-tests`、`feature/p5-mcp-oauth-fence` | 0 (已是 compat 祖先) | 不重複挑 commit；但保護上方具名工作樹的未提交檔案 |

## 4. Milestone 目前的可驗證狀態（以最新 Host 證據覆蓋舊 Roadmap 日期）

| 項目 | 盤點認列 | 待補實際證據／決策 |
| --- | --- | --- |
| Original/Next 分離、P1–P5、M8 基本 Authority | 先前 Closeout；Original `8765` 與 NEXT `8767` 並存 | 不等於 Original 可退休或 NEXT 已正式發行 |
| P6 Plugin、P7 Platform | 核心／Shared UI 與部分 Host UAT 已有歷史證據 | native GUI 鍵盤／選檔／焦點仍欠完整驗收；**NexusDock 依使用者決策延後** |
| P8 Broker | 10/14 真實 ChatGPT＋Codex＋Antigravity 共享 Headless Chrome 已 Host PASS；RC16 Edge scoped policy 有獨立證據 | 不代表所有指定公司 Edge Profile／授權情境或每次 GUI 操作均 PASS；按現行政策與可核驗憑證驗收 |
| A1／RC20 | `704bb40d` source/race/跨平台建置、RC20 簽章與實際 NEXT 安裝 PASS；**installed Gemini 同 Session completed→cancelled→recovered PASS** | **installed Codex** Prompt 三階段受 provider quota 阻擋；不把 Gemini PASS 當 Codex PASS |
| Connection UX-S1／fail-closed | Source + Vitest/Frontend 回歸已 PASS，實作已隨 RC20 candidate 進入安裝版 | 真實 NEXT GUI「拒絕 mutation→切頁→保留錯誤」Host OPEN |
| Tray MenuBar 身分 | 既有 Swift title／文字及簽章；RC20 GUI 背景新程序＋單一視窗／一次 `focus_changed=false` 已驗證 | 真實 Tray AX 品牌辨識／安全點擊選單及全面 no-focus 時序仍 OPEN |
| Native Codex MCP | signed launcher、Core Catalog、ID-only dispatch、peer provenance、M8 fence 的 source/repository 證據已存在 | 可信 Desktop Operator→M8→真正 Native Backend installed Host UAT OPEN |
| P10／P11 | P10 本機 arm64 prepublish 工程證據；P11 本機 ad-hoc RC20 NEXT-only 安裝及**帶簽章 RC19 回滾副本**已驗證 | RC20 更新發生後**未對本次版本執行實際 rollback**；Windows/WSL 原生 live UAT、Hosted CI、Developer ID／Notarization 按既有範圍／外部條件決定，不得冒稱完成 |
| M9／正式 Release | `release_ready=false`、Update Feed 未發布；正式 release 不可開始 | 完成必要整合、原生 Host、安全／發行治理；必須另外確認推送／Tag／Release 時機 |
| Original 1.1.1 選擇性吸收 A2–A6 | A1 已在 RC20；其餘僅有差距評估 | A2/A3 Windows Tunnel／回滾先確認真正需求；A4 覆蓋缺口再加測；A5/A6 按收益排程，不整包搬移 Original |
| P13／P14 | **Deferred / no active implementation** | 依 10/22 使用者 UX 決策，禁止將舊大型持續輪詢卡當必要產品工作 |

舊 `roadmap.md`／`pre-m9-progress-2026-10-09.md` 內包含多個曾被後續 RC16–RC20 Host 證據覆蓋的歷史 snapshot，**不能直接當今日 live status**；本盤點只是狀態優先序說明，沒有將所有舊段落改寫。

## 5. 安全整合順序（Plan，非已執行）

1. **凍結證據基線：**保留 RC20 已簽章來源 `704bb40d`，以及已安裝 NEXT/Original 和 RC19 回滾副本；不在原 dirty worktree 製包。
2. **先整理歷史文件：**在另一個乾淨、由任務持有的文件工作樹選擇性採納舊候選紀錄與 11/02 Tunnel 維護紀錄，新增「歷史／已過時」標籤，而非覆蓋 `next-active-backlog` 的 11/11 最新狀態。不得改寫舊工作樹以取得乾淨 Git。
3. **整合提交審查：**從 `custom/main=7e87605a` 先驗證 `custom/main..86ff4cf0` 的 **54 個 commit／164 個檔案**與必要測試，再評估**本機 fast-forward** 到經核准的整合 HEAD（可能包含此狀態文件）。必須把 `custom/main` 和已簽章候選 commit 作為兩個不同識別，不能把新 commit 冒稱 RC20 build 的來源。
4. **遠端另案審查與授權：**本機 `custom/main` 已比 origin 多 312 提交，兼容分支則多 366 提交；遠端 Push 會傳送整段可達提交鏈，不是小規模四檔上傳。除非來源與目標、CI、Release policy 都通過獨立檢查並取得授權，否則**不 Push／Tag／Release**。
5. **實機優先：**切換到階段二 NEXT Connection GUI 與 Tray 的真實但受控 Host 驗收；前景互動另檢查安全窗口。Codex 額度恢復後才補原生 Codex 三階段測試；Native MCP operator／Windows／正式發行分別有獨立 Gate。
6. **保留其他工作線：**P13/P14 實驗預設不合；P5 OAuth、M6 ACP、Original maintenance 有 dirty 證據，在該工作負責人決定前不可清理、重設或強制套用到 NEXT。

**本階段驗收：**Git 工作樹與遠端 ref 已唯讀查證；重要候選關係、dirty 檔案、Milestone OPEN/PASS 分層與後續整合順序明確記錄。沒有啟動 Codex ACP、移動 `custom/main`／RC20 signed candidate 分支、執行任何合併／Push／Release、更新安裝版或操作 Original/Next 服務；僅在獨立 compat 文件分支建立本地交接提交。資料來自本機 Git／工作樹與列出的專案歷史文件，非完整遠端 CI／PR 在線審核。

## R04 — 歷史證據選擇性整合 Appendix（2026-11-11）

**來源：**只讀 `agentdock-next-edge-rc-20261023` 原 dirty 工作樹；當時 untracked `docs/custom/next-a1-rc20-candidate-2026-11-05.md`、dirty `docs/custom/pre-m9-progress-2026-10-09.md` 與 `next-active-backlog-2026-10-21.md`。以下都是**過去時間點的紀錄，不是 11/11 現役 Installed 狀態**。來源檔仍在原工作樹，**未 reset/stash/移除/修改**；本 Appendix 不整份複製有舊版主張的文件。

### 11/02 Cloudflared 維護／rollback（新增追溯）

- 2026-11-02 授權本機維護時，NEXT 還是 **RC19**（Core 來源 `138c6cb0f123`）；只把 `cloudflared 2026.9.3 → 2026.10.7`，重簽 helper/外層 bundle，未換 Core 或新增 Native Catalog/Launcher。當時 GUI Stop 回報 `runtime_action_failed`，**沒有把錯誤當成功**，改採版本相符的 NEXT-only `deploy-next-shared.py`，保留 App root inode/原子 Contents swap；Original 元件同日有獨立維護，並非 NEXT Source merge。
- 11/02 當時驗收：Next Core/Tunnel launchd/PID、新 `cloudflared --version`、四條 QUIC 連線、`8767/healthz` 200、Bundle codesign 和 Original `8765` 200；**公開端點 health 403 未過**，以及 CLI `ready=false / connected=null / public_endpoint=unchecked / remote_route=manual_route_required`，不補稱 public Host PASS。
- 當次回滾證據在 ``/Library/Application Support/AgentDock-Rollback/tunnel-2026.10.7-su2pnuwe/` 與 ``/Applications/.agentdock-next-replacement-a57nvx7y/AgentDock Next.app`（只記錄當時路徑，**本輪未確認當前仍存在或有效**）。當時 Core PID `92533`／Tunnel `92907`、`92909` 僅歷史，禁止套用至 RC20。

### 11/05 A1 RC20 的未被新 Host 狀態覆寫的歷史差異

- 獨立乾淨工作樹 `feature/next-a1-rc20-20261105` 在 `704bb40d08f6144513791a9e1854cd2c9a799b73` 封存八檔 A1 source/test/doc；Candidate `next-v0.9.2-rc.20` 僅是 **tag 字串**，沒有 Tag／Push。當時 `next-preflight` canonical、ad-hoc arm64 App/ZIP/DMG、Core `version --json`、`deploy-next-shared.py::manual_version_gate` 的唯讀版本檢查通過，**沒有部署**。
- 11/05 有兩種不同 Host 證據不可合併：一是 `staged Core` 真實 ACP completion→cancellation；二是 repository-local、已簽章 RC20 packaged Core 以隔離臨時 Root 與隨機 Loopback Port 執行真實 RPC：同 Session `completed/end_turn → cancelled/cancelled → completed/end_turn`，Session close/delete 後無測試 listener。後者**不是 installed GUI/UAT**，亦不等於 11/11 installed Antigravity Host。當時有一筆**非該輪 owner** 的 direct Computer active Session，因此拒絕 RC20 deploy，這是正確的 ownership blocker，而非部署失敗。
- 11/05 A1 曾有 `-race -count=10` source PASS 紀錄；**11/11 另於相同 signed RC20 源重測，Race 下至少一項 1 秒 Fixture watchdog 3/3 FAIL**，診斷 overlay 七項×十次 PASS。前者是帶日期的歷史觀察，**不取代現在 H04 Source 未修正**；不要以 11/05 記錄覆蓋 11/11 失敗證據。
- 舊 Active Backlog 的 A1 尚未實作、RC19 installed 文字是 11/04～11/05 的歷史快照；當前以 [RC20 Installed Host](next-rc20-installed-host-2026-11-11.md)、[Race/GUI/Tray 根因](next-rc20-h04-h01-h02-root-cause-2026-11-11.md) 和 Active Backlog 頁首為準。原未提交的五個 A1 source/test 與 signed RC20 source 已核實相同 bytes，**不得重新整合舊 dirty patch**。

**R04 決議：完成選擇性歷史補充，不另創重複的 Candidate Full Copy。** 若後續要求稽核逐字原文，仍可對照保留的 dirty source；這不構成回頭更動其 ownership 的授權。
