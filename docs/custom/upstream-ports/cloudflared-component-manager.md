# Cloudflared Component Manager — 從上游 v1.0.1 選擇性採用

> 狀態：DEFERRED / design gate only（2026-10-09）。不得以本文件視為 cloudflared 遷移授權；不得直接對目前 live Next Tunnel 執行 install/update/restart/uninstall。

## 來源與採用目標

- 上游：`uvwt/agentdock` 正式 `v1.0.1` commit `255000078e399be2fdff986ae65d4c24290425b9`。
- 相關實作：`internal/component/cloudflared.go`、`catalog-v1.json`、`trust_darwin.go`、`trust_windows.go`、`trust_linux.go`、`internal/desktopruntime/cloudflared_component_darwin.go`。
- 採用：catalog revision、版本相容性、`supported/deprecated/revoked`、artifact digest、staging、驗證後 atomic active pointer、repair rollback、明確舊版來源匯入、狀態與進度事件、解除安裝。
- 不採用：直接依賴 `download.nexusdock.co`；放寬 Next identity guard；從 PATH/任意可執行檔自動匯入；以雜湊驗證取代 platform publisher 驗證。

## 安全差異（設計時必須解決）

| 面向 | v1.0.1 上游行為 | Next 所需安全契約 |
| --- | --- | --- |
| 安裝位置 | `<runtime>/components/cloudflared/versions/<version>/cloudflared` + `active.json` | 固定 **Next-owned** 專屬 root、擁有者/目錄權限與非符號連結驗證；不得與 stable 同根 |
| macOS identity | 上游平台 `codesign --verify --strict`，沒有在該函式釘死 Next bundle identifier | 不能只測簽章有效；須審查 publisher/TeamID、CodeDirectory、路徑及權限。既有 `dev.dropabit.agentdock.next.cloudflared` 自簽 Helper 與 Cloudflare-signed component 是兩套不同身分 |
| 啟動鏈 | `prepareUnixCloudflared()` 將已驗證 component path 寫入 manifest | Next `next_identity_darwin.go` 現要求 cloudflared 與 Core 共置 `Contents/Helpers`，必須設計獨立可信解析器與受控 Launcher，保留 Next identity 不變式 |
| App 更新 | App 無須總是內嵌 cloudflared | Next `desktop_update_darwin.go`、`next_shared_bundle.py`、packaging/test 目前要求存在並驗證內嵌 Helper，需經向後相容過渡而非一次刪除 |
| Catalog 信任 | HTTPS、baseline + cached revision monotonic、artifact pinned SHA-256 | Revision 不等於授權：對遠端 manifest 應提供簽章或與 Next 發布來源綁定的完整性授權；避免被替換為惡意高 revision／惡意新 hash；不能僅信任 TLS |
| 來源與平台 | GitHub upstream + NexusDock mirror，macOS codesign、Windows Authenticode、Linux digest | Next 明確定義允許的 vendor/URL、重導向、SHA/簽章驗證及撤銷策略；Linux 不匯入任意 legacy binary |
| 運行一致性 | `active.json` 是唯一 active 選擇點 | 更新後需識別 disk active vs 已執行的 PID/binary digest 與 restart-pending，避免無感替換仍在使用的版本 |
| 操作權限 | `agentdock component install/update/uninstall` | 新增 Desktop API revision/generation/M8 permission/confirmation，明確 timeout、回滾與使用者可見原因；不得帶出 OAuth/Tunnel token |

## 分階段實作（M9 之後獨立 milestone）

**C0：契約/測試基線（唯讀）**

- 定義 Component Store 的 Next 目錄所有權、可信 artifact/metadata 來源、發布撤銷語意、脫離 App Bundle 的 LaunchAgent 執行路徑／驗證決策。
- 凍結 `ComponentStatus`（not_installed/ready/broken/deprecated/revoked、installed/running versions、needs_restart）、`OperationCapability`（install/update/repair/remove）及 failure codes。
- 建立穩定版和 Next 同時安裝、現有 Named Tunnel 不變更的 fixture；baseline 中禁止讀取 stable 私有 state。

**C1：純後端 Store，尚不切換執行路徑**

- 在獨立 worktree 引入 `NextComponentStore`：catalog 驗證與最低 revision、防高 revision 攻擊、pinned artifact digest、OS publisher 校驗、atomic version/active pointer、備援/repair、fixtures。
- 先實作 `inspect` 與 dry-run compatibility，不碰現有 `Contents/Helpers/cloudflared`、不主動 restart，所有實際 install 只用隔離 test-root。

**C2：雙路徑兼容／明確遷移**

- `BundleLegacy` 與 `ManagedComponent` 是兩種可觀測的受信任來源，runtime resolution 使用固定優先級，不能悄悄從 PATH 尋找。
- 僅允許從 **Next 自身已簽 Bundle 的精確 Helper 路徑** 遷移；成功 staging/核對/commit 後才更新 Next-owned registry；原 live Tunnel 不因「元件已安裝」就被 restart。
- 加入執行中版本/PID 的可觀測狀態；一律由 Next-owned launcher/supervisor 控制後續重啟與回復。

**C3：包裝／更新器協調**

- 更新 `internal/desktopruntime/next_identity_darwin.go`、`internal/selfupdate/desktop_update_darwin.go`、`packaging/macos/next_shared_bundle.py` 與跨平台 installer；新增受信任的 Next-owned Component Resolver boundary。只有管理元件已準備且 health/回復 gates 過關，才可產生不再內建 cloudflared 的新格式。
- 舊版 Next package/rollback 要能辨識新格式及故障後的 Legacy fallback，但不能將任意外部檔案誤認為受信任的 Helper。

**C4：Shared UI 與受控導入**

- Connection → Tunnel 顯示元件可用性、安裝中、版本、更新、修復、Restart required，M8 approval 與錯誤/retry 操作明確。
- 每次更新先可逆 stage，具健康檢查與 bounded rollback；以人工確認的 Next-only UAT 驗證 Token/OAuth/Hostname/stable 隔離，最後才考慮預設 external component 模式。

## 測試門檻（至少）

1. 偽造/損壞 digest、無效署名、目錄 symlink、跨 Next/stable root path、版本範圍錯誤、revoked metadata：全都 fail closed。
2. Catalog 過期、equal revision 衝突、網路中斷、攻擊性高 revision、下載中斷、archive 解包 traversal、磁碟空間不足：不改 active pointer。
3. 原子 staging 成功但 active pointer 寫入失敗：保留舊 active 且具可復原檔案；並行 install/update 不會互踩。
4. 故障注入：更新中的進程被關閉、舊進程仍在運作、首次啟動失敗、restore backup 失敗、cleanup 出錯：狀態真實且不重啟 stable。
5. 從現有 Next Bundle 的受信任 Helper 明確匯入：元件可用且線上 tunnel/token 不變；匯入來源不是目前 Next Helper 時拒絕。
6. 新舊 App package update/downgrade/rollback matrix（有/無元件、損壞元件、現有 named tunnel）均要通過；無原生 macOS／Windows evidence 不 closeout。

## Gate／非目標

- **前置 Gate**：Pre-M9 P8、AGY .14 Next-only 整合、必要 native GUI、安全複審與 M9 signed release/update/rollback 完成；若部分被正式延後須記錄風險，不默認放行。
- 不借用官方或 stable runtime root、port、Cloudflare Tunnel token；不變更 `mac-dev.dropabit.dev` 與 Next 既有 `mac-dev-next.dropabit.dev`。
- 最初只支援 cloudflared；Codex/AGY ACP、Skills、Plugins 的安裝/升級 owner 仍維持既有管理域。
