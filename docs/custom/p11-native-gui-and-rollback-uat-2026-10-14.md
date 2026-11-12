# P11 — Next macOS GUI / native live-fault rollback UAT (2026-10-14)

**Outcome:** On an actual macOS arm64 host, the Next-only **GUI restart, 12-page navigation, bounded page interactions, manual update and injected real post-swap rollback** have been verified. This is **not full feature-by-feature live end-to-end acceptance**: destructive/authenticated actions, a brand-new OS-user install, macOS menu bar controls and approved public auto-updater were not verified.

## Exact code and live identity

- Source code commit **`fa279cd45f2e4abdedeafa54e4009d51aae1ba43`** (already committed), branch `feature/p11-adhoc-native-uat`. Subsequent documentation-only commits do not change the tested package provenance.
- Real Next installed at `~/Applications/AgentDock Next.app`; its `version --json` is **`0.9.2-rc.2`**, commit `fa279cd45f2e`, UTC `2026-10-14T08:10:02Z`, `darwin/arm64`. ad-hoc `codesign --verify --deep --strict` PASS. This is a **local test RC**, not an approved public release/version.
- Stable `/Applications/AgentDock.app` GUI PID **12001**, Stable Core PID **12127** listening on **127.0.0.1:8765**, both retained across Next-only GUI restarts, manual upgrade and failure-injected rollback. Stable Core executable `st_dev`, `st_ino`, byte size and modification timestamp unchanged across fault injection.
- Installed Next Core after recovery PID **23460** listening only on **127.0.0.1:8767**, Next Tunnel PID **23536**, launchd-owned, healthy. Next GUI PID **23865** after safe reopen. No Stable service/bundle/browser/credential was intentionally changed.

## Computer Use GUI test matrix — actual visibility versus action coverage

`orca computer` macOS provider Accessibility and screenshots permissions were both granted. Refreshed accessibility state before each semantic click. Screenshots captured on actual visible Next GUI. `orca computer` **cannot manipulate native menu bar/menus** in this provider, so menu bar/Tray is not verified. The language select popup appeared, but its options were not exposed by this provider; it was dismissed without persisting a locale change.

| GUI page | Observed and verified, not merely renderable | Outside this safe, non-destructive UAT |
| --- | --- | --- |
| 總覽 | Running/healthy Next Core, Connector summary and disabled update feed reflected; click navigation buttons | No automatic update |
| 執行階段 | Read-only runtime health and status, refresh, buttons present | No stop/restart of Core via GUI |
| 活動中心 | Synchronized/reconnected read-only view, completed/current job sections, no unexpected action failure | No new external task dispatched |
| 連線 → ChatGPT 存取 | Next port 8767 owned, Named Tunnel matches selected radio (fix), local/public MCP URL present; native Core and connector readiness separated | No OAuth password reveal/copy, public endpoint probe, DNS route change, tunnel restart or port mutation |
| 連線 → NexusDock | Explicit **開發中**, readonly unknown/unavailable states not falsely shown as connected | Pairing/replace pairing/restart intentionally unavailable |
| ACP | Profiles codex/antigravity listed; codex editor opens and Cancel leaves unchanged; saved session details visible | No adapter detection, credential prompts, profile install/delete, ACP sessions termination |
| 瀏覽器 Broker | Read-only routing, leases, queue, worker/owner counts and refresh; labels clearly distinguish retained records from live connections | No browser launch, profile login, Core lease allocation; retained diagnostics do not prove active Chrome/Edge |
| MCP 管理 | One `memory` standalone entry, **尚未連線** truthfully displayed; detail opens, Add Server form opens and cancels without mutation, refresh works | Did not force reconnect/discover, invoke tools, OAuth login, disable/delete memory server |
| 外掛管理 | No installed Plugin shown; localized navigation, refresh success | No folder/ZIP install/uninstall or elevated change |
| 權限與批准 | Mode and pending/history sections, policy review dialog opens then Cancel | No permission mode update, approval/rejection or policy commit |
| 設定 | Port 8767 read-only native-managed, log level and autostart states, refresh | No Save, reset running Core or change log level |
| 系統 | Diagnostics read-only, Next update channel correctly **未啟用**, check button disabled, no `update_check_failed` | No public check/apply, no Config/Logs folder writes |
| 開發者／架構 | Explicit unavailable capabilities, Activity event stream probe set to **1 Hz**, started, received **2 events**, stopped successfully | No high-rate load test or unbounded probe |

**ACP note:** The live UI included **45 retained session cards**; many have an `自動關閉失敗` *field* whose actual value is `否`. Do **not** infer 45 failures. 44 close buttons were disabled due recorded state/availability; this is dense retained UI (P13/P14 noise workline) and was **not** cleaned, deleted, or migrated as part of P11.

## GUI defects discovered, fixed and proved on installed version

1. **Connection draft hydration:** Snapshot was `Named Tunnel` but the mode radio could show `僅本機`. `ConnectionPanel.vue` now watches `configRevision` with `immediate: true`; on real installed GUI Named Tunnel radio has **Value 1** and local radio **Value 0**. No tunnel configuration was changed.
2. **Disabled Next updates misreported as generic errors:** Next native update feed remains intentionally unpublished and fail-closed. New `selfupdate.IsNextChannelDisabled` / desktop API `next_update_channel_disabled` reason / frontend store and localized 3-locale message renders **「AgentDock Next 更新通道尚未啟用；自動更新已停用。請使用驗證過的本地部署方式。」**, with disabled Check button, not `update_check_failed`. **No signer guard/feed enablement was weakened.**

Fixed code commit `fa279cd4`. On installed Next `0.9.2-rc.2`, actual Orca Computer Use proved both changes on Overview/Connection/System; the same states stayed correct after deliberate native fault recovery.

## Real macOS Next-only post-swap injected failure and recovery — PASS

- Created **local test only** canonical `0.9.2-rc.3` candidate from the **same exact source commit** `fa279cd45f2e4abdedeafa54e4009d51aae1ba43`, build ordinal 3, at `dist/next-shared-arm64.VAeNpQ/package/`. Real Wails Next.app arm64 + Core/arbiter + packaged ZIP/DMG, ad-hoc signature and 9/9 bundle tests PASS; `next-artifacts --verify` and both `shasum -a 256 -c` PASS.
- Candidate ZIP `AgentDock-Next-macos-arm64.zip`: **29,903,062 bytes**, SHA-256 `390e229963e96846429bf3062bde6fd606838cbb4ea0e830620591c2b26ce071`.
- Candidate DMG `AgentDock-Next-macos-arm64.dmg`: **35,151,157 bytes**, SHA-256 `deabc8ae7d2166f5930c1c55de4c18c2d321d78f626f30c7493b5124c04cd795`.
- Real Next input admission: verified signed old/new Next identities and SemVer forward `0.9.2-rc.2 → 0.9.2-rc.3`; Next Core PID 8518 and Tunnel PID 8690 healthy/owned, port 8767 had no established external sessions; Stable PID 12127 / executable metadata baselined. Gracefully quit only Next GUI.
- Imported the real `deploy-next-shared.py` in a one-off Python harness and **injected exactly one Core registrar failure only after `install-next-shared.py` had actually atomically exchanged the on-disk signed Next App `Contents`** and read-only `Core version --json` reported `0.9.2-rc.3` in the installed bundle. The real built-in deployment exception/recovery path restored old signed `Contents`, re-registered Next Core and Tunnel, checked Next-only Core listener/binary/PID and health.
- Log: `dist/p11-rollback-injected-live-fa279cd4.log` contains **`INJECTED_FAIL_AFTER_REAL_NEXT_CONTENTS_SWAP`**, **`NEXT_ONLY_ROLLBACK_RESTORED`**, **`REAL_NEXT_ONLY_NATIVE_RECOVERY_PASS 0.9.2-rc.2 new_core_pid 23460 new_tunnel_pid 23536`**, **`STABLE_NEVER_CHANGED_DURING_REAL_ROLLBACK`**; command **exit 0**.
- Reopened only Next GUI from restored App (PID 23865). Actual Orca GUI smoke on **總覽、執行階段、連線、系統**: normal Next Core/Connector, Named Tunnel selected and matches observed mode, Next update feed clear/disabled, no generic error — all PASS.
- No public Tag/Release, no update feed activation, no Developer ID/Notarization, no Stable changes. The intentionally failed `rc.3` is **not installed**; real running Next remains signed/ad-hoc `0.9.2-rc.2`.

## Source and package regressions — PASS

- `GOMAXPROCS=2 GOFLAGS=-p=2 make check` exit **0**, includes i18n canonical 3 locales / 883 keys, Go root tests, script governance, shell checks, vet, build.
- Independent Wails Go module `desktop/shared-poc`: `go test ./... -count=1` exit **0** (macOS SDK object-target warnings only).
- `desktop/shared-poc/frontend`: Vitest **24/24** files, **171/171** tests PASS; Vue TypeScript `vue-tsc --noEmit` and production Vite build exit **0** (JS chunk-size warning nonblocking).
- `scripts/test/test-next-shared-package.py` **9/9** on signed failure candidate; same-source manifest and ZIP/DMG artifact inventory and SHA sidecars exact. All candidate inventories retain `release_ready=false`, feed `disabled/unpublished`.

## Remaining P11 / non-claims

- **Fresh install on a separate empty macOS user/account/environment** was not exercised: live source account already has a functioning Next and installer deliberately requires exact previously installed `~/Applications/AgentDock Next.app`. Must not delete or overwrite the existing user setup to manufacture a first-install proof.
- **Native macOS menu bar icon/menus** could not be driven by the installed Orca Computer Use provider (`supports.surfaces.menubar=false`). A running bundle and login helper do not substitute for an observed menu action.
- Deliberately *not* run: real authentication/MCP tool executions, ACP detection with possible repeated Keychain prompts, service stop/update from GUI, permission Save/reject, actual public endpoint change, plugin installation, manual interruption (hard-kill) recovery. Those are separate sensitive end-to-end UAT, not implied by navigation tests.
- **P11 ad-hoc Next-only manual upgrade, real post-swap failure rollback and non-destructive native GUI UAT now have actual evidence**, but the **entire P11 is not marked 100% complete** until required fresh install/OS menu or other user-accepted scope exceptions are decided. Windows/WSL native waived; official certificate-bound updater, Developer ID and Notarization excluded by user's explicit decision.
- No GitHub Hosted PR CI or formal publication performed. Stable operational environment and all preserved rollback backups remain intact.
