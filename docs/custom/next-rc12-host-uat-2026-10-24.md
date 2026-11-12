# AgentDock NEXT RC12 — Installed Host UAT (2026-10-24)

**Status: RC12 packaging/install PASS; Edge lease admission FAIL CLOSED; full RC acceptance OPEN.** This is installed Host evidence, distinct from the source-only fixes in [RC12 peer source checkpoint](next-rc12-edge-peer-2026-10-24.md). No merge, push, tag, feed or formal release.

## Candidate and NEXT-only install

- Worktree `feature/next-edge-rc-20261023`, source `4a8abca7307bbb8e893200652b472c66cf65a98b`, built Core `0.9.2-rc.12`, bundle `13.0.0`, candidate `dist/next-shared-arm64.XEDylB/package/`.
- Go source regression 1,687 PASS / 7 opt-in SKIP / 0 FAIL, selected `-race` 85 PASS, `go vet` PASS. Vue TypeScript and Vite production build PASS.
- App, ZIP and read-only DMG each have 24 matching file SHA-256 values. ZIP SHA `2bc2b6005a554a77117be89b68ce3aa155de85917a67d835662e36574ef45e8b`; DMG SHA `1f671ba3e68cae15b7fc8667b7ce78cce1089e482c09fd21ec9876b1f99a13db`; ad-hoc deep strict codesign and DMG checksum PASS.
- Before installation, NEXT Broker idle (zero owners, leases, workers, active/queued, orphans), Computer no active session, ACP ready/running/loaded all 0 and no pending interaction; GitLab independent MCP ready. Existing `packaging/macos/deploy-next-shared.py` returned `NEXT_ONLY_DEPLOYED`. Installed `~/Applications/AgentDock Next.app` matches candidate all 24 file SHA. NEXT 8767 runs rc.12; Stable 8765 remains 1.1.0 unchanged.
- Known-good RC11 rollback bundle preserved at `~/Applications/.agentdock-next-replacement-vl5_9ocg/AgentDock Next.app`, verified Core rc.11 / `c331afe47d2e`. **Live failure-injection rollback was not exercised**, still OPEN.

## Genuine RC12 Edge Host gate

- Exactly one preannounced `browser_broker(connect_external)` on installed RC12 succeeded in **5.649 s**.
- First company Oprueba `direct_acquire` rejected: `BROWSER_REQUIRED_ROUTE_UNAVAILABLE: external peer verification unavailable or mismatched`. No lease, navigation, screenshot or independent negative tests could run in RC12. Source changes are not evidence of resolving this installed failure.
- No retry, replacement Profile, Managed Chrome fallback, automatic prompt click or new socket. Explicit `disconnect_external` PASS; post-check 0 owners/leases/workers/operations/orphans; NEXT had no remaining 9222 socket. Edge user's program and Stable untouched.
- Private evidence: `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc12-edge-fresh-result.json`, `rc12-edge-fresh-foreground.jsonl`. RC11 four-site/sibling/no-focus Host PASS belongs **only to RC11**; see [RC11 Host record](next-rc11-host-uat-2026-10-23.md).

## Native GUI and independent GitLab lifecycle

- Old NEXT GUI PID 33739 still mapped an older backup image. After verifying the process was NEXT-only and Chrome remained foreground, gracefully SIGTERM'd **only** that old GUI, followed by `open -g -a` of installed NEXT. New GUI PID 52626 maps current installed App binary; neither Core nor Stable restarted.
- NEXT Computer observe session with `foreground=forbidden` returned Orca `capabilities`, a real main window title `AgentDock Next` (nonminimized, onscreen, approx. 1147×917) and a ScreenCaptureKit PNG (2294×1834), with Chrome foreground unchanged and `focus_changed=false`. Session released `cleanup_state=complete`. **AX tree had only 5 standard window/title nodes; main controls and native Tray action UAT remain OPEN**. Orca provider reports menubar unsupported.
- After Core restarted, NEXT-only `gitlab-company` connector initially `idle/0 tools`; one normal `mcp_manage(refresh)` using the **existing independent OAuth grant** returned `ready/35 tools` without reauthorization or borrowing Stable credentials. Previous [GitLab MCP read-only Host gate](next-rc11-gitlab-host-uat-2026-10-24.md) already PASS. Separate REST `read_api` not asserted.

## Remaining release gates

1. Explain real macOS Edge `connect` success but first `direct_acquire` peer verification failure without bypassing pinned socket/process/signature/Profile identity; only reattempt one officially consented Host flow after focused fix.
2. Separate negative URL/JavaScript admission checks and cross-session/crash/cancel/abnormal cleanup.
3. GUI application control/accessibility, native Tray; not just visible window.
4. NEXT-only RC12 live failure rollback acceptance; no push or public release while gates remain OPEN.
