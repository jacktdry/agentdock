# NEXT RC18 — installed launchd rollback Host UAT (2026-10-24, Asia/Taipei)

## User workflow, scope and verdict

A NEXT-only updater must preserve the working signed App/launchd Core if a deployment fails **after** files have already been replaced, without touching the original Stable AgentDock. **This specific real installed RC18 signed Contents swap -> injected first registration failure -> built-in rollback -> real launchd re-registration / signed Core process restart / health validation path is Host PASS.** Failure at the first registrar is a **single test-injected exception**; the actual rollback contents swap, second registrar invocation, launchd process restart, Core signed identity and health are NOT mocked. This does not prove all updater failures (e.g., power loss mid-write or filesystem damage).

Product target: `~/Applications/AgentDock Next.app`, dedicated Core port 8767 and launchd services `dev.dropabit.agentdock.next.core/.tunnel`. Stable `AgentDock` port 8765 and its processes are strictly outside scope. No company Edge connection, browser profile, GUI foreground changes, or user work files were touched.

## Strict preconditions

- Source Git branch `feature/next-edge-rc-20261023` clean; installed signed NEXT `0.9.2-rc.18` from binary source `60a08929649dfc689448fbad0824fa86fe25c839`; **same exact** candidate `dist/next-shared-arm64.JMEYgA/package/AgentDock Next.app` verified with signed bundle validator and all **24 regular-file SHA256 matches**. Prevented version downgrade or surprise code changes; candidate and installed Core `version --json` matched.
- Separate signed rc.17 rollback remained available at `~/Applications/.agentdock-next-replacement-l2tbj2yz/AgentDock Next.app`. Destination parent App inode captured before test, source/installed/backup all validated.
- NEXT Core trusted real launchd PID **65849**, macOS-owned listener/binary and `8767/healthz` healthy. NEXT Tunnel service was present.
- Browser Broker idle, owners/leases/workers/operations/orphans/authenticated transports 0, Computer active sessions 0, Codex and Antigravity ACP ready/running/loaded 0, NEXT no established Edge TCP 9222 socket. Stable `8765/healthz` confirmed `1.1.0`.

## Real one-shot failure and recovery

- Test script used the **actual** `packaging/macos/deploy-next-shared.py.main` and `install-next-shared.py`; no alternate update implementation. It allowed the genuine signed bundle copy and atomic Contents swap (preserving root App inode).
- One scoped Python `registrar` wrapper deliberately threw `UAT_ONLY_REGISTRAR_FIRST_CALL_FAILURE` on its **first** call, and only after asserting the installed App's Contents still matched the verified rc.18 candidate. This simulates a registrar failure at the exact post-swap boundary. **No OS executable was killed to induce the failure.**
- The built-in `except` path executed the real `NEXT_INSTALLER.rename_swap(backup, installed Contents)`, validated the restored signed bundle, then invoked the original **unmocked** `registrar(destination, "core")` and original `wait_for_core(None, ...) ` which verifies exact new launchd PID, signed binary identity, port 8767 listener and health. It also re-registered the real NEXT Tunnel.
- The tool emitted `NEXT_ONLY_ROLLBACK_RESTORED` and intentionally raised `New Next deployment failed; previous Next bundle recovered`; the UAT harness **expected** that failure and asserted the exact marker.
- Actual NEXT Core launchd PID changed **65849 → 27482**; NEXT Tunnel was observed at PID **27615** after restoration. Installed Core remained `0.9.2-rc.18`; all **24 files restored to original SHA256**, App root inode identical, `codesign --verify --deep --strict` PASS.
- Rechecked both HTTP health endpoints: NEXT 8767 rc.18 PASS; untouched Stable 8765 v1.1.0 PASS. Verified NEXT GitLab OAuth retained and `mcp_manage refresh(name=gitlab-company)` returned **ready, 35 tools**, no reauthorization or Stable token copy.
- After test Browser idle/zero owner/lease/worker/orphan and no NEXT Core TCP socket on Edge 9222, Computer active 0, Antigravity ACP managed/ready/running/loaded 0, Codex ACP ready/running/loaded 0. GUI process PID **68191** continued running; no GUI swap or foreground action performed.
- The one redundant test-created rollback stage `~/Applications/.agentdock-next-replacement-k0akd5ur` was removed **only after its signed 24 files were proven identical to the currently installed RC18**. The separate verified **RC17 recovery bundle** `~/Applications/.agentdock-next-replacement-l2tbj2yz/AgentDock Next.app` remains intact.
- Private machine-only test and data: `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc18-next-real-launchd-rollback.py`, `rc18-next-real-launchd-rollback-result.json`. This UAT intentionally does not write test secrets or product credentials to source Git.

## Acceptance classification / remaining gates

**Host PASS for the one tested updater failure boundary:** correctly rejects a failed post-swap registration and restores the previous signed Contents, genuinely recovers NEXT launchd Core/Tunnel and preserved GitLab OAuth while Stable remains healthy. A separate prior test using signed disposable bundle copies covered rc17→rc18→rc17 atomic swap but stubbed service health; **this installed test closes that service-restart limitation for the specific post-swap registrar failure case**.

**Still OPEN:** unprompted real power-loss/corrupt-disk update failures (outside safe routine RC scope), ACP crash with actual browser/computer leases and whether a pre-crash sibling's existing Run completes, native Tray ownership/readable item/physical menu action, and any requested GitLab REST `read_api` capability. See [actual NEXT-owned Adapter crash/recovery](next-rc18-acp-adapter-crash-host-2026-10-24.md) and [RC18 Tray GUI](next-rc18-tray-host-2026-10-24.md).

**Release decision:** source binary `0.9.2-rc.18` remains `release_ready=false`; don't merge `custom/main`, push/tag or publish solely because this tested rollback path passed. NEXT remains usable.
