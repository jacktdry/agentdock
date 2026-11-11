# AgentDock NEXT RC21 — Installed NEXT-only deploy and H02 Host (2026-11-11, Asia/Taipei)

## Goal and authority boundary

User authorized the next-stage NEXT-only RC21 upgrade and installed Host validation. Real job: make the existing NEXT Tray clearly distinguishable from Original while preserving the separate stable Mac-Dev control channel. No Codex ACP delegation, no Edge/browser profile action, no Original mutation, no public release, Tag, hosted CI, feed or notarization. Source commit and canonical signed RC21 candidates were already built in the separate clean worktree; the production-looking installation below is still a **local ad-hoc release candidate**, NOT Developer-ID-signed production.

- Immutable source: 0a0a0ba98412c6f22f8793a6abd184d6db62a27e, branch fix/next-rc20-h04-h02-r01-20261111.
- Version: 0.9.2-rc.21, Build 22.0.0, bundle dev.dropabit.agentdock.next.
- Candidate package directory: /Users/wei/sideProject/agentdock-next-h04-h02-r01-20261111/dist/next-shared-arm64.0v48fc/package/.
- Canonical manifest: /Users/wei/sideProject/agentdock-next-h04-h02-r01-20261111/dist/rc21-20261111-local/next-rc21-candidate.json; release_ready=false, update_feed=disabled/unpublished.
- See preceding [local build and isolated H02 Host evidence](next-rc21-local-candidate-host-evidence-2026-11-11.md). No fresh source edits for this deployment.

## Protected Core preflight (real authenticated runtime)

Using exact, locally configured NEXT runtime authentication read through the repository's desktopruntime.ReadLocalCoreAccess, the standard one-shot Streamable HTTP MCP client queried only browser_broker status, computer_broker status, and acp_session status for each Codex and Antigravity profile. MCP sessions were closed automatically. No ACP agent prompts, cancellation, session cleanup or creation.

Immediately before the actual swap: Browser Broker owners=0, leases=null, workers=0, authenticated owners=0, active queue=0, queued=0, managed/external orphans=0; Computer active_sessions=null; Codex and Antigravity diagnostics sessions=0. The old Adapter and chrome-devtools-mcp child PIDs were managed idle processes, not proof of an active run. Original Core 8765 returned 1.1.1; installed NEXT Core PID11613 on 8767 returned 0.9.2-rc.20; NEXT Tunnel PID12375 was up. The preflight re-ran immediately in the deploy command and passed.

Source RC21 App, old installed RC20 and the previously created RC20 restore App all passed Next-only bundle strict validators, codesign deep strict, semantic version upgrade gate, exact Core listener ownership and local package SHA256 checks. The restore App was 26/26 regular files byte identical to installed RC20.

## Exact real deploy, services, rollback assets

The **existing** packaging/macos/deploy-next-shared.py was used with the validated candidate App. It performed its original, dedicated NEXT-only atomic Contents replacement, Next Core/Tunnel ServiceRegistrar reregister and Core Skill bootstrap; returned:

- NEXT_ONLY_DEPLOYED: /Users/wei/Applications/AgentDock Next.app
- NEXT_ROLLBACK_BUNDLE: /Users/wei/Applications/.agentdock-next-replacement-6zm2fbh8/AgentDock Next.app
- NEXT_GUI_RESTART_PENDING (then handled separately and verified below)

Temporary 8767 downtime during the controlled Core restart was observed. The installed new Core launched as **PID 20891**, actual listening port 8767, HTTP 200 0.9.2-rc.21, with new verified installed executable. New Tunnel was **PID 21108**. Original Core **PID 38464**, HTTP 200 1.1.1 on 8765 remained unchanged.

Installed Core version --json returned: version=0.9.2-rc.21, commit=0a0a0ba98412, build_date=2026-11-11T12:20:23Z, platform=darwin/arm64. Installed package was ad-hoc signed and codesign --verify --deep --strict passed.

Both the installed RC21 and the original RC21 candidate contain 26 regular files; path, size and SHA256 matched **26/26**. The actual atomic deployment rollback stage at the path above was signed RC20 Bundle 21.0.0 and matched the pre-existing separate signed RC20 backup **26/26**. No live rollback was triggered because deployment succeeded. Previous RC18 rollback-host history must not be presented as an RC21 live rollback test.

## Installed GUI and real AX Host

The still-running old GUI PID14144 was explicitly bound using lsof to the **RC20 rollback stage executable**, not guessed by a process name. After signature/version/PID/ownership checks, only that known NEXT GUI received a graceful SIGTERM and exited; **no SIGKILL** and no Original mutation. A normal installed RC21 GUI was started using open -g -a, PID **22247**. lsof -d txt verified its loaded executable was exactly /Users/wei/Applications/AgentDock Next.app/Contents/MacOS/AgentDock (the new RC21 image).

Native macOS ApplicationServices Accessibility, scoped to exact Bundle ID:

| Observed field | Installed NEXT RC21 | Original |
| --- | --- | --- |
| GUI bundle ID | dev.dropabit.agentdock.next | com.uvwt.agentdock |
| App PID | 22247 | 38041 |
| ExtrasMenuBar items | 1 | 1 |
| Status item AXTitle | **NEXT** | empty |
| Status item AXDescription | missing | **AgentDock** |
| AXEnabled | true | true |
| Status item size | 81x24 | 36x24 |
| Status item position | x967 y-1437 | x1046 y-1437 |

The installed NEXT GUI had one AXWindow, title AgentDock Next. A recursive, read-only AX traversal of the GUI returned **67 native AX elements** including **1 AXWebArea**, 6 headings and 19 buttons. The WebView/GUI exists without requiring foreground activation for inspection. No menu item AXPress, menu command, stop, restart, Connection mutation or permission prompt was performed.

**H02 installed AX accessibility identification PASS**; visual icon/notch rendering and actual menu open/Refresh/Open/Quit behavior are **OPEN**, because AXChildren did not expose the closed menu and blindly activating a status item while the user works would disrupt the foreground.

## Foreground exception and follow-up

A separate foreground monitor sampled **367** times and observed only com.google.Chrome, with zero NEXT/Original foreground events **during that monitoring period**. However, that monitoring completed before the GUI restart; it does **not** prove the complete upgrade was no-focus. Despite using open -g -a, the newly launched NEXT became frontmost briefly (native AX inspector observed dev.dropabit.agentdock.next active). The user subsequently changed foreground to com.openai.codex; a safe guard detected this change and **skipped** an attempted Chrome focus restoration. At the end, read-only native AX showed com.openai.codex remained in foreground and NEXT not active.

Therefore **zero unexpected foreground activation is NOT PASS**. Investigate Wails activation and its normal launch path as a bounded next fix before calling GUI startup fully noninterrupting; do not hide or change current installed app speculatively during this acceptance.

## Final runtime, safety, remaining gates

After deployment, standard authorized read-only MCP status returned: Browser Broker owners 0, leases nil, workers 0, queue active/queued/orphans 0; Computer active sessions null; Codex/Antigravity both diagnostics sessions 0. The adapters are not required to remain running when no ACP Sessions exist.

- Installed RC21 Core 8767 HTTP 200; Original 8765 HTTP 200 (PID 38464 unchanged). NEXT Tunnel launched with PID 21108; GUI new PID 22247 is the exact installed binary.
- Installed App Core/GUI and archive payloads were not modified after sign; Source Git worktree remained clean.
- Verified RC20 restore content exists both in actual private atomic swap stage and independent source ignored dist. No real RC21 rollback transaction executed.
- H01 Connection true rejected mutation -> Overview -> Connection retained error remains **OPEN** without a proven UI-safe rejection path; Source tests do not count as Host.
- Full menu visual/action Host and strict no-foreground-launch are **OPEN**.
- R01 unpublished Hosted GitHub CI has not run, native Windows validation incomplete, high-load H04 fixture timeouts retained; no Developer ID signing, Apple notarization, GitHub Release, Tag or public Update Feed. No Push/MR/Merge.
- This evidence lives on the separate documentation worktree branch rather than editing the immutable RC21 source, so the already-verified source SHA stays unchanged.

## Evidence file paths (all ignored within source worktree)

- dist/rc21-20261111-local/deploy-rc21-result.log
- dist/rc21-20261111-local/immediate-deploy-idle.log
- dist/rc21-20261111-local/deploy-foreground.log
- dist/rc21-20261111-local/restart-installed-gui.log
- dist/rc21-20261111-local/installed-tray-menu-readonly.log
- dist/rc21-20261111-local/post-deploy-broker-readonly.log
- dist/rc21-20261111-local/next-rc21-candidate.json
- dist/rc21-20261111-local/next-rc21-inventory.json

No user-facing app/service was deliberately changed except the authorized NEXT App/Core/Tunnel and NEXT-only managed Core Skills.
