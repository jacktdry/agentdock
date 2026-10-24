# NEXT RC18 — Installed visible Tray label Host checkpoint (2026-10-24, Asia/Taipei)

**Verdict: RC18 candidate/signing/install PASS, background GUI no-focus PASS, installed NEXT main window observe PASS. Native Tray's visible `NEXT` label and ownership/AX accessibility are NOT Host PASS. No further cosmetic RC loop. Release remains blocked by native Tray menu interaction, real ACP cross-session failure recovery, and NEXT-only live rollback injection.**

## User workflow / product result

Users run original AgentDock alongside NEXT and should immediately distinguish their menu-bar service controllers. [RC17 installed Host](next-rc17-installed-gui-uat-2026-10-24.md) showed that macOS returned blank `Title/Description/Help` for a menu-bar proxy despite an explicit accessibility label. The minimal source change `60a08929649dfc689448fbad0824fa86fe25c839` adds a short `NEXT` button title with leading package icon **only when validated bundle identity is Next**, leaving the legacy Stable UI unchanged. Swift identity assertions and native AppKit title/image positioning typecheck PASS. This is a candidate for an observable fix, **not proof of live UI appearance**.

## Reproducible candidate and installation

- Fixed-source manifest `/private/tmp/agentdock-next-edge-verification-y6b6836m/next-rc18-candidate.json` — Core `0.9.2-rc.18`, build `19.0.0`, exact source commit `60a08929649dfc689448fbad0824fa86fe25c839`, `release_ready=false`.
- Mac arm64 candidate `dist/next-shared-arm64.JMEYgA/package/`: Core/Swift native app, Vue production bundle, Core Skill bundle and 9 package tests PASS; macOS App signed ad-hoc with `codesign --verify --deep --strict` PASS.
- App/ZIP/DMG **24 identical regular files by SHA-256**; read-only DMG attached, hash-matched, detached, `hdiutil verify` checksum VALID. ZIP SHA-256 `cf252492fab5907c90ca41b03aee08c2ffd00309efe1069eb95c76b4def56faa`, DMG SHA-256 `25aa7dfaf5a934d0e8b1ec1e3a69ebee28b988adc89c75bb451415540e9b01e2`.
- Immediately before NEXT-only install: Browser Broker idle, 0 leases/owners/workers/orphans/authenticated transports, ACP ready/running/loaded 0, Computer active 0, GitLab MCP ready/35 tools, NEXT Core held **no Edge 9222 socket**. Installer returned `NEXT_ONLY_DEPLOYED`. Installed app 24 file SHA identical and codesign PASS; the saved rollback app `~/Applications/.agentdock-next-replacement-l2tbj2yz/AgentDock Next.app` was verified to contain **rc.17**.
- Installed Core `127.0.0.1:8767/healthz` returns `0.9.2-rc.18`, separate original Stable `127.0.0.1:8765/healthz` returns **1.1.0**, never touched/restarted. NEXT-only GitLab OAuth continued to refresh `ready/35 tools`; no company Edge connect, native debugging grant, Chrome Profile swap or extra service was launched.

## RC18 native installed GUI Host evidence

- Previous GUI PID `41116` mapped specifically from the protected rc.17 rollback and was gracefully signaled only after verifying the executable and PID. RC18 was opened with `open -g`, resulting GUI PID **68191**, mapped to the currently installed app. No Core, Edge or Stable process restart.
- The native foreground sampler was started before the swap and permitted to exit normally: **95 valid samples, all `com.google.Chrome`, 0 NEXT foreground transitions**. Private aggregate `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc18-gui-foreground-result.json`. This particular no-focus interval **PASS**.
- NEXT-only Computer observe with `capability=observe`/`foreground=forbidden` confirmed exactly **one main window**, **46 Accessibility nodes**, ScreenCaptureKit screenshot `captured`, `focus_changed=false` and a `complete` session release. An initial script assertion failed, then a separate diagnostic read established each of these specific fields was correct; do not call the first script a PASS.

## Tray blocker: avoid false identification

- The app-level `AXExtrasMenuBar` proxy exposed one enabled `AXMenuBarItem` advertising `AXPress` but with **empty `AXTitle`, `AXDescription`, `AXHelp`, `AXValue`** even on installed RC18. It had reported x=1007, y=-1437, width=40, height=24 on the upper of two displays.
- A deliberately tiny screenshot of this region visibly matched a **different-looking 'W' status item**. Both the primary and secondary display's narrow menu bar strips were examined privately; **`NEXT` was not confirmed in the inspected strips**. Accessibility menu-bar proxies may reflect shared/global menu items rather than an item uniquely owned by NEXT. Therefore neither AX's `AXPress` nor its empty title can be attributed conclusively to NEXT's own `NSStatusItem` in this test. Private cropped screenshots in the same machine-only `/private/tmp/agentdock-next-edge-verification-y6b6836m/` directory; no complete-screen recording or user web content recorded.
- **Do not claim `NEXT` visual text or accessible Tray name Host PASS**. The item may be hidden by macOS/menu bar layout, or the proxy may be unrelated. Do not disable a user's status-bar customization, click menu icons, enlarge permissions or endlessly ship source variants just to force a synthetic acceptance.
- No physical click, menu dropdown or foreground-required UI control was attempted. Separate installed RC16 Browser positive/negative Host acceptance remains historically proven for its binary; **RC18 has not rerun the Edge lease Host tests**.

## Required decisions / remaining release gates

1. A bounded user-visible menu/tray inspection in a non-disruptive period (correct NEXT item and readable label, menu operations) with explicit focus-change evidence, or record native Tray accessibility as a recognized product limitation rather than making unverifiable claims.
2. True installed **Codex/Antigravity ACP cross-session** cancel/crash and owned resource recovery under disposable work; the RC16 two HTTP-client Computer leases and isolated 24 passing race tests are complementary, not equivalent.
3. **NEXT-only live failed-update rollback** evidence or a justified staging path that cannot disrupt Stable and preserves working NEXT; existence of signed rollback is not execution of fault injection.
4. GitLab REST `read_api` credential scope separately if product requirements demand it; independent GitLab MCP OAuth tools currently ready/35.

**Do not merge `custom/main`, push, tag, publish/update feed.** Candidate remains `release_ready=false`; documents committed after the candidate do not change the installed binary source.
