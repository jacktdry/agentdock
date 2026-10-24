# NEXT RC16 — Native GUI passive UAT and source crash/cancel checkpoint (2026-10-24)

**Scope:** RC16 installed on the user's Mac. This checkpoint observes only NEXT's own GUI in background and separately exercises isolated, test-owned recovery fixtures. It **does not** count foreground GUI interaction, menu bar/tray interaction, live cross-session crash, or live updater rollback as PASS.

## GUI process version / passive observations

- After NEXT-only Core RC16 deployment, NEXT GUI PID **99046** still had text mapped to the old protected rollback location `~/Applications/.agentdock-next-replacement-isy0dclw/…` while its nominal argv path referred to the current installed App. This was a **stale GUI image**, not an RC16 GUI Host PASS.
- Checked that current frontmost app was Chrome. Sent **SIGTERM only to verified NEXT GUI PID 99046**, waited for it to exit normally, then used `open -g -a ~/Applications/AgentDock Next.app` (explicit background launch). New GUI **PID 1648** had its text mapped to the **current installed `~/Applications/AgentDock Next.app`** rather than the rollback location. The NEXT 8767 `rc.16` and original 8765 `1.1.0` Core services remained healthy. This did **not** touch or restart Edge.
- The during-restart foreground sampler wrote **0 usable records**. Its verification assertion failed solely because the recording was empty; this is **NOT evidence of a focus change**, but it also **does NOT prove restart no-focus**. Separate snapshots **before and after** both observed `com.google.Chrome` foreground; do not interpolate across the unobserved interval.
- RC16 NEXT-only `computer_session(action=acquire, capability=observe, foreground=forbidden)` + `computer_observe(action=list_windows, app=AgentDock Next)`: one non-minimized target main window, `focus_changed=false`. `computer_observe(action=get_app_state, restore_window=false)`: ScreenCaptureKit `screenshotStatus.state=captured`, the exact window title `AgentDock Next`, Accessibility **46 nodes** and 1,153-character tree, `focus_changed=false`. A later `no_screenshot=true` repeat independently reproduced **46 nodes** and focus unchanged. All observation sessions were **released `complete`**, resulting Computer Broker `active_sessions=0`. No native GUI action was dispatched.
- Limitations: the screenshot was provider-reported captured and AX tree returned successfully; **the screenshot pixel contents and every individual control were not manually inspected**. Native Tray/MenuBar surfaces are not supported by this Orca observation flow; **no menu click, focus-required act, interactive button or setting state was exercised**.

## Crash/cancel/recovery source-only verification

- Existing focused browser/app test fixtures rerun with Go Race Detector, including `NativeEdgeRuntimeCloseCancelsPendingConsent`, `DirectBrokerCancellationAndLifetime`, `ExternalAcquireOrphanConnectorRecovery`, `ExternalFailedCleanupRecoveryRetriesConnectorOnly`, `PinnedCDPGateCancellationStopAndGenerationQuarantine`, `SharedBrowserRestartGuard{CrashAndVerifiedRecovery,HardCrashMarkerSurvives}`, `ManagedWorkerHardCrashReturnsOwnedProcessGroupToBaseline`, `NextCodexStartupCrashFenceNeverAutoReclaimsOrphan`, and `RuntimeCloseCancelsForegroundCommandAndRejectsNewStarts`.
- Result **24 PASS / 0 FAIL**, private machine-only JSON log: `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc16-crash-cancel-source-race.jsonl`.
- These run against isolated fixtures and test-owned child processes; **not an installed NEXT multi-session Host fault-injection PASS**. No production Edge transport, Stable process, user document, OAuth grant or external service was deliberately crashed.
- Browser full Host gates stay accepted from [RC15 positive Host](next-rc15-host-uat-2026-10-24.md) plus [RC16 negative Host](next-rc16-host-uat-2026-10-24.md), without conflating different binary test runs.

## Outstanding release gates

1. **Gui/Tray native interaction:** verify menu bar presence/action, user-facing controls, accessibility navigation and focus behavior through an explicitly authorized, monitored foreground-required test, with precise expectations. The current passive flow cannot certify it.
2. **GUI restart no-focus:** repeat under a foreground observer that actually completes/flushed samples, or maintain as OPEN. Do not restart the App merely to create a PASS.
3. **Live cross-session failure paths:** controlled cancel/crash (isolated sessions and disposable work), recovery and exact zero-leak evidence in installed NEXT; do not kill company Edge.
4. **NEXT-only updater failure injection and rollback:** verified RC15 backup is available, but an actual failed-install/restore Host test is still OPEN; protect working RC16 and the separate Stable 8765.
5. Separate GitLab REST `read_api` authorization, if required by product scope; MCP `mcp` scope and 35 GitLab tools remain verified independently.

**Release status:** RC16 `release_ready=false`; do not merge to `custom/main`, push, tag or publish until the remaining gates have evidence and approval. Codebase/candidate source is `d51f1bc4`; subsequent documentation commits do not change that binary.
