# AI 圓桌：AgentDock Custom Foundation

> 日期：2026-10-02
>
> 狀態：Concluded

## 參與角色

本輪不是單一模型直接下結論，而是由主 orchestrator 整合兩個獨立、read-only reviewer：

- **Architecture / upstream reviewer**：Codex ACP，GPT-6 Astra，High reasoning。
- **Product / workflow / i18n reviewer**：Antigravity ACP，Gemini 3.1 Pro，High reasoning。

兩個 reviewer 都以目前 `custom/main` 對應的官方 AgentDock source 為基礎分析，不負責寫入 repo；最終決策由 orchestrator 根據使用需求與 repo 現況收斂。

## 共識

### 1. 官方 AgentDock 應維持唯一 primary upstream

Workbench 不應成為第二個 merge upstream。最穩定的長期模式仍是：

```text
uvwt/agentdock
      ↓
main
      ↓
custom/main
```

Workbench 只提供 concept、資料模型與 reference implementation。

### 2. Shared Desktop UI 值得做，但不能 big-bang rewrite

AppKit + WPF 對快速增加的 Agent workflow UI 形成雙倍維護成本。

兩個 reviewer 都支持：

- Go 保持 Core / authority。
- Vue 3 / TypeScript 作 shared business UI。
- OS-specific service / credential / permission / updater 留在 platform adapter。
- 舊 native UI 在 shared UI parity 與安全驗證完成前保留。

### 3. Wails 只是首選 POC，不是已定案依賴

Wails 的 Go integration 最自然，因此先測。

但 framework 決策必須由以下實證決定：

- event / stream reliability
- tray / menu / window
- service lifecycle
- macOS signing / update / TCC
- Windows startup lifecycle / privilege / update
- accessibility
- RAM / CPU
- packaging

Tauri 保留作對照 / fallback。

### 4. Workbench 最值得採用的是 execution semantics

共同高價值項目：

- truthful active state
- Activity / Call observability
- bounded output + continuation
- user insertion
- ACK / bounded retry
- attention semantics

Android、Termux 與為完整 Workbench parity 才需要的 infrastructure 不進目前 roadmap。

### 5. Activity vertical slice 應早於 ACP package CRUD

原因不是 Activity UI 比 ACP Manager 更重要，而是它更早驗證新架構最危險的部分：

- high-frequency event
- streaming
- reconnect
- backpressure
- execution identity
- mid-run insertion
- accessibility of dynamic content

ACP package CRUD 的 UI 相對單純，可以稍後建立。

## 分歧與收斂

### i18n：JSON vs YAML

第一輪：

- Codex 偏好 JSON + JSON Schema，理由是 Go / TypeScript / tooling 一致、隱式解析風險低。
- Gemini 原先也偏向 JSON / ICU，但在第二輪加入「strict parser + validator + deterministic generator」條件後，認為 YAML 作 authoring format 可接受。

最終選擇：

> **Strict YAML authoring + ICU MessageFormat contract + generated runtime resources**

選擇 YAML 是因為本專案明確希望讓志願翻譯者容易在 GitHub 上提交、看註解與 review diff。

為接受 Codex / Gemini 對 YAML 的風險提醒，加入強限制：

- value 必須雙引號。
- 禁止 anchor / alias / tag / flow collection。
- 禁止 block / folded multiline scalar。
- schema + ICU validator。
- build 時生成 JSON / `.strings` / Windows resources。
- 若未來接 TMS，以 JSON / ICU 作 interchange format，不要求 TMS round-trip YAML。

### ICU MessageFormat

收斂為現在就固定 contract，但不要求所有字串使用複雜 ICU expression。

- 簡單字串仍是簡單字串。
- 有 interpolation 時使用 ICU argument。
- 真正需要 plural / select 才使用 ICU plural / select。

這可以避免之後不同平台各發明一套 formatting syntax。

### Locale identity

採用：

- `en`
- `zh-Hant`
- `zh-Hans`

而不是預設 `en-US` / `zh-TW`。沒有實際 dialect requirement 時使用 language / script tag 比 region-specific tag 更符合目前需求；平台需要不同 tag 時由 generator mapping。

## 新增的驗收 Gate

### Accessibility

- VoiceOver
- Narrator
- keyboard-only
- zero focus trap
- dynamic Activity / log 的 live-region 行為

### Performance

不先接受任意絕對數字，而是：

1. 量測目前 native app baseline。
2. POC 建立 idle / active RAM、CPU、event throughput、routing latency 測法。
3. 設定 regression budget。
4. 高頻 event 必須 batching / backpressure。

### Migration safety

- inventory 目前 AppKit / WPF feature。
- 匯入既有 `.strings` / `.resx`，不重新人工翻譯。
- 保留 permission / service / updater regression tests。

## 最終決策

1. `main` = official mirror。
2. `custom/main` = custom integration，並作為 custom release pipeline 完成後的 Release source。
3. Workbench = feature radar / selective port。
4. Cross-platform Desktop = shared Vue UI + Go API + thin native adapter，先做 POC。
5. i18n = strict YAML + ICU + generators。
6. Shared UI foundation 後先做 Activity / insertion vertical slice。
7. Browser Routing 再做 workspace-aware policy。
8. ACP Manager 隨後整合，保持工具型 UI，不做 marketplace。
9. Android / Termux 明確不在目前 roadmap。

## 現況限制：Custom Release

最終文件審查確認官方 release pipeline 目前仍會：

- 要求 release source commit 屬於 `origin/main`。
- 要求 release tag 與官方 `buildinfo.Version` 相符。

因此建立 `custom/main` 不代表現在即可直接發 custom Release。Custom release source validation、version contract 與 update channel 會在 M9 一併完成；在此之前不得為了發布而把 custom commit merge 回 `main`。
