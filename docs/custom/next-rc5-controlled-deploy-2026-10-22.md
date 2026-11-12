# AgentDock NEXT RC5 — controlled installed deploy and rollback rehearsal (2026-10-22)

> **Later 2026-10-22 GUI evidence:** After the user exposed the NEXT window, the actual RC5 Browser Broker native page and guidance were read and refreshed successfully; the previously unobservable background GUI limitation is now **closed for GUI presentation only**. Company Edge / explicit Chrome Profile authorization remains unverified. See [native Browser UAT](next-rc5-browser-route-host-uat-2026-10-22.md).


**Status: actual NEXT-only upgrade PASS; controlled isolated rollback rehearsal PASS; native GUI Browser route/Edge Host UAT OPEN.** This is an ad-hoc local RC installed acceptance, **not** an official Release, Tag, feed, Developer ID signing or notarization.

## Identity, source and provenance

- Candidate `0.9.2-rc.5` / `CFBundleVersion=6.0.0` built from clean `custom/main` full source commit `3dcdabe29bd83d47c29359fbc79ea2a2bfe3c38e`, 2026-10-22T10:21:25Z. Production package origin `dist/next-shared-arm64.gOwQN9/package/AgentDock Next.app`, canonical private manifest `/private/tmp/agentdock-next-rc-20261022-3dcdabe2/next-candidate.json`, manifest checksum `cc1001d913e4883d8ec4601ecfb6685134ad399008864aeca1e933aa10b1471f`. Package built/verified last turn: real macOS arm64 GUI/Core/Swift, ad-hoc signed, ZIP+DMG archives verified, suite 9/9 PASS. Read-only manual_version_gate accepted rc.4→rc.5 and rejected reverse.
- Previous installed NEXT `0.9.2-rc.4` / `CFBundleVersion=5.0.0`, Core commit `5860a2b46dee`. NEXT GUI background process was already active; Stable App is a separate identity and **must never be touched**.
- Upgrade required preflight: `custom/main` clean, proper Next-only paths and no Skills symlink redirection, candidate hashes match acceptance report, installed signed source and existing Core PID/8767 listener/health bind, Browser Broker zero owners/leasing/queue, NEXT Codex ACP zero ready/running/loaded. All prerequisites satisfied before mutation. Stable Mac-Dev performed the actual deployment.

## Actual installed NEXT-only upgrade

- Executed existing `python3 packaging/macos/deploy-next-shared.py <verified RC5 package/AgentDock Next.app>` **through original Stable Mac-Dev**. This helper validates Core semver, Next-only source/destination signed bundles and owner paths, atomically replaces **Contents only**, re-registers only `dev.dropabit.agentdock.next.core` and `dev.dropabit.agentdock.next.tunnel`, checks the new PID/binary/listener/health, and **only then** executes NEXT Core Skills bootstrap. Script printed `NEXT_ONLY_DEPLOYED` and `NEXT_GUI_RESTART_PENDING`.
- Installed NEXT Core is truly `0.9.2-rc.5` / `3dcdabe29bd8` with matching source UTC and App plist `6.0.0`; binary bytes match candidate. Existing parent `.app` root inode unchanged. Next Core changed PID `50220→29412`, Next tunnel `68273→29630`. Both Next and Stable health endpoints returned HTTP 200 afterward.
- All seven Managed Skills remained present (`acp-adaptive-routing`, `agentdock-user-guide`, `memory-lifecycle`, `multi-agent-discussion`, `plugin-import`, `skill-authoring`, `skill-installation`): **all 26 pre-existing Skill file SHA-256 hashes were identical after deployment**. Thus four bundled Core Skills auto-bootstrap did not disturb the user's working configuration/other three Skills. No credentials/Memory/CBM index were copied or modified.
- NEXT's own connected ChatGPT plugin subsequently returned version `0.9.2-rc.5`, all 7 Skills and successfully called `codebase-memory:list_projects`, confirming the connector recovered. Broker had zero active owners/leases after deployment. Stable port 8765 remained healthy throughout.
- The pre-existing old NEXT GUI PID `68392` was gracefully terminated and the newly installed GUI restarted **in background** as PID `33739` via `open -gj ... --args --background`; macOS foreground bundle ID remained `com.googlecode.iterm2` before and after, no focus steal; stable 8765 and NEXT 8767 stayed healthy. A no-focus Computer Control observation of the newly hidden background GUI found **no observable window**, therefore current installed Browser route **visual** UAT is **not passed** and needs a user-visible Next window in a separate non-interrupting check.

## Rollback safety and limitations

- Verified real old RC4 backup of signed App Contents at `/Users/wei/Applications/.agentdock-next-replacement-5gpv5wmy/AgentDock Next.app/Contents` (same filesystem and retained until remaining Host acceptance). Do not remove or repurpose it until Browser/GUI closeout. Backing up this old Contents is not equivalent to running the script's Core failure branch.
- In an isolated `/private/tmp` fixture cloned from the **actually installed RC5 app** and its **actual retained RC4 backup**, exercised the exact installer `rename_swap` primitive to return to RC4, verified former `.app` root inode preserved and both binaries traded places, then swapped again to return to RC5 and proved original RC5 Core SHA content restored. Test directory automatically removed. **Isolated atomic rollback/restore PASS; no intentional live Core failure/restart/rollback injection.** Full live failing-service automatic recovery was not triggered, must not be claimed as verified.
- Older RC4 backup remains an emergency manual recovery path via Stable Mac-Dev and exact verified Next-only staging contract, not a suggestion to bypass Core version anti-downgrade by running ordinary deployment backwards.

## Remaining gates

1. **NEXT Browser route native GUI and Edge consent UAT:** observed new GUI has zero windows while launched hidden; user can expose the GUI during a safe window. Check Browser route source-only UI features in actual RC5; don't replace company Edge Profile with generic Chrome; retain evidence of authorization and no-focus behavior.
2. **Full live-failure rollback branch:** source tests and isolated real binary atomic swap pass, but in-service failure recovery not deliberately invoked because the upgrade succeeded; test when a genuinely failed deployment occurs or in a sanctioned disposable service fixture.
3. **Core Skills/CBM longer-term:** Post-upgrade Skill state byte-identical and actual MCP read success; native CBM multi-session writer/watch coordination and index refresh remain separate limited items. No forced full repo index while single-writer conflict unknown.
4. **Release governance:** no push, tag, GitHub release, updater activation, notarization, or update feed. Installing an ad-hoc local RC is not approving a public release. P13/P14 remain deferred by user's UX decision.

## Evidence location

Private local file `/private/tmp/agentdock-next-rc-20261022-3dcdabe2/next-deploy-preflight.json` (app inode, previous version, Skill names), `next-installed-skill-files-before.json` (26 checksums), `next-only-deploy.log` (Next-only swap/re-register/bootstrap success), `next-rc5-live-acceptance.json` (new core/tunnel PID, backup path, unchanged Skill hashes), `candidate-acceptance.md` (full ZIP/DMG hashes and source provenance). No secrets added to repository documentation.
