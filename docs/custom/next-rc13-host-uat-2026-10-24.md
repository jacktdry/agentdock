# AgentDock NEXT RC13 — Installed Host checkpoint (2026-10-24)

**Release status: RC13 source/packaging/NEXT-only install PASS; real Edge lease admission NOT VERIFIED because the sole official connect timed out. Native GUI window/screenshot/AX basic observation PASS, full GUI/Tray and failed-update rollback OPEN. No merge, push, tag, public release or feed.**

## Identity, build and rollback

- Worktree: `/Users/wei/sideProject/agentdock-next-edge-rc-20261023`, branch `feature/next-edge-rc-20261023`; **binary source** `02b6e6201cc4b2b90e73f7a97cc19d385c865d45`, Core `0.9.2-rc.13`, macOS bundle `14.0.0`. Manifest: `/private/tmp/agentdock-next-edge-verification-y6b6836m/next-rc13-candidate.json` (release_ready=false).
- Native macOS arm64 bundle built from exact clean HEAD at `dist/next-shared-arm64.Cy895v/package/`. Vue typecheck/Vite PASS; four Go packages **1,692 PASS, 7 opt-in SKIP, 0 FAIL**; targeted Race Detector **52 PASS**; vet PASS. Bundle tests **9 PASS**, ad-hoc deep/strict codesign PASS.
- App / ZIP / read-only DMG **24 files byte-identical by SHA-256**. ZIP SHA-256 `a1b3885fafe7ce80c600fd2c4a82366a7b8523dab64111347533fc8a152bf723`; DMG SHA-256 `00ea529c560d99f5832dc2af6ea38802f163dbf5c3dcd109a43a5b10277344b2`; DMG checksum VALID; private test mount detached.
- Explicit predeployment proof: Browser idle with 0 owners, leases, workers, operations, queue, orphans; Computer 0 active sessions; ACP ready/running/loaded = 0 with no session interactions; independent GitLab MCP ready/35 tools. Existing NEXT-only deploy helper returned `NEXT_ONLY_DEPLOYED`, installed 24 App files SHA-match candidate, installed Core `0.9.2-rc.13` on 8767. **Stable 8765 remains version 1.1.0**, untouched.
- Genuine preserved RC12 rollback bundle: `~/Applications/.agentdock-next-replacement-ik8t5g3g/AgentDock Next.app`, Core `0.9.2-rc.12`. **Live deliberate failed-update/restore injection was not performed**; this release gate stays OPEN.
- After NEXT Core restart, independent GitLab grant survived; one ordinary `mcp_manage(refresh, gitlab-company)` returned ready and **35 tools**, no reauthorization or Stable credential use.

## Exactly one official Edge attempt, incomplete

- Using **installed RC13**, preannounced a single genuine `browser_broker(action=connect_external)` with current company Edge worktree. Edge may show its own native allow-debugging dialog; no programmatic acceptance or second connection attempt.
- Client timeout **65 seconds** expired before Broker connect returned. The script recorded `error: timed out`, `ok=false`, `official_connect_calls=1`; **no lease was acquired, no page action, website, sibling-release, fresh-acquire or negative scope gate executed**. This is neither native Edge peer success nor a repeat of RC12's first-lease rejection: this RC13 run never reached that stage.
- Immediately after timeout, a lone Core-to-9222 TCP connection was still ESTABLISHED while Broker reported idle/0 logical resources: native consent/transport shutdown was still pending. **Do not claim cleanup from logical idle alone.** After allowing the Core's own bound native handshake to finish, passive observation confirmed the connection disappeared, no new connect/probe/restart. NEXT Browser state idle, owners/leases/workers/active/queued/managed/external orphans all 0; original Edge process/Profile and Stable untouched.
- Private proof: `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc13-edge-fresh-result.json` and `rc13-edge-fresh-foreground.jsonl` (151 complete foreground samples, all Chrome; Edge window sets changed across observation, which does not prove consent). No raw credentials or Edge screenshots stored in Git.
- Source-side root-cause candidate is **sequential two-stage attestation incorrectly given one 2s enclosing deadline**, now bounded at 5s per native outer phase with both child 2s limits preserved, and safe finite failure phases; selected and full source tests PASS. This code path is **not yet approved by installed Edge lease Host evidence** because the authorized connection did not complete.

## Native GUI consistency, ScreenCaptureKit, accessibility

- After Core update, old NEXT GUI PID 52626 still mapped the previous RC12 rollback executable. Verified exact NEXT-only old process; foreground was Edge. Sent graceful SIGTERM only to old NEXT GUI and used `open -g -a` on currently installed NEXT. New GUI PID **99046** has image mapped from `~/Applications/AgentDock Next.app/Contents/MacOS/AgentDock` (not rollback bundle). Core 8767 and Stable 8765 unchanged.
- NEXT computer observe session `capability=observe, foreground=forbidden` found exactly one non-minimized, onscreen `AgentDock Next` main window, captured PNG using ScreenCaptureKit, and returned **46 AX elements**, up from RC12's five generic window nodes. The observe calls themselves reported `focus_changed=false`; session released with `cleanup_state=complete`.
- **No-focus qualification:** foreground was Edge before GUI restart and NEXT when observed later. The intervening focus transition was not independently attributed, so **do not mark the entire GUI restart no-focus gate PASS**. Neither native menu bar/Tray interaction nor main UI control activation was exercised. Orca's menubar surface is unsupported; do not simulate it or claim full GUI parity.

## Next exact acceptance path

1. Keep this RC13 installed, retain RC12 rollback. Next time the user is able to handle the Edge native allow-debugging step, make at most **one explicitly authorized Core-owned formal connect**, then reuse the same socket for first/parallel/fresh leases and two isolated negative checks. Do not retry short probes or downgrade company routes. Safe finite `process_epoch` / `held_socket` / other statuses must be recorded if admission rejects again.
2. Real ACP ChatGPT/Codex shared Browser and cancel/crash/cleanup lifecycle; native GUI controls/Tray and noninterruption evidence; RC13 live failed-update rollback. RC11 four approved origin Host UAT remains separate past-binary evidence, not RC13 PASS.
3. No `custom/main` merge, push, release tag, feed or deletion of rollback while gates remain open.
