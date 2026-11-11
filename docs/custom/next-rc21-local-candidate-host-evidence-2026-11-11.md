# AgentDock NEXT RC21 — Local candidate and H02 AX Host (2026-11-11)

## Scope and immutable source

Goal: deliver a usable, verifiable NEXT RC21 macOS candidate and demonstrate the real Wails Tray accessible name without interrupting installed NEXT RC20 or Original. No Codex ACP, no install, no Push/Tag/Release, no update-feed activation.

- Source worktree: /Users/wei/sideProject/agentdock-next-h04-h02-r01-20261111, clean branch fix/next-rc20-h04-h02-r01-20261111
- Source commit: 0a0a0ba98412c6f22f8793a6abd184d6db62a27e; contains H04 race fixture, H02 Tray label and R01 unpublished workflow
- Candidate: 0.9.2-rc.21; manifest candidate tag string next-v0.9.2-rc.21 is NOT a created Git tag; Bundle Build 22.0.0; build date 2026-11-11T12:20:23Z
- Identity: dev.dropabit.agentdock.next; canonical Manifest SHA256 44b28731c1eeeefeffb50871b677c81880edfefb0d9bace6e4eaeea331d987fa
- Release gates: release_ready=false, update_feed=disabled/unpublished, no Update URL. No Developer ID or notarization

The signed RC20 cloudflared arm64 helper was copied to the ignored local build input (NOT downloaded from latest). Source and copy SHA256 matched 067f71da9d28777f78054e9db8fbe326df25e4c7d1287d2b93a560020b2baf34 and codesign verification passed.

## Build, provenance and artifacts

The original packaging/macos/build-next-shared.sh was executed with canonical --candidate-manifest. It built actual new Vue, Wails GUI, Go Core, Launcher and Core Skill Bundle, then ad-hoc signed the NEXT-only macOS arm64 App and created ZIP/DMG. Product App is NOT installed.

Package directory in the above source worktree: dist/next-shared-arm64.0v48fc/package/

| Artifact | Size (bytes) | SHA256 |
| --- | ---: | --- |
| AgentDock Next.app | — | Entire bundle codesign --verify --deep --strict PASS; Bundle ID and 22.0.0 PASS |
| AgentDock-Next-macos-arm64.zip | 31,999,724 | 4fb18108a412b78b2e2be12fafd41bc25943c9e5ad70674cdc992b394f880d8d |
| AgentDock-Next-macos-arm64.dmg | 36,750,849 | 725af8692fba7d270fffc1d5cf3857e9892eb6047490d432507651a8371d96bd |

ZIP/DMG SHA256 sidecars PASS, unzip -tqq PASS, hdiutil verify PASS, NEXT canonical artifact inventory generation and exact verification PASS. Inventory at dist/rc21-20261111-local/next-rc21-inventory.json, SHA256 4a2bc790963bfc7160eac58a466b211a27b8e076db2363beccd132123825e5cd.

Actual NEW bundled Core version --json: version 0.9.2-rc.21, commit 0a0a0ba98412, build_date 2026-11-11T12:20:23Z, darwin/arm64, matching canonical manifest. GUI/Core are native arm64 Mach-O.

## H02 real macOS accessibility, isolated host

Read-only Swift ApplicationServices AX queried actual application-specific AXExtrasMenuBar / AXChildren (no AXPress or clicking). Baseline: installed NEXT RC20 PID14144 has unnamed AXMenuBarItem, Original PID38041 has AXDescription=AgentDock.

An isolated copy of the new RC21 App was given separate bundle ID dev.dropabit.agentdock.next.h02uat, re-signed ad-hoc, validated, and launched using open -g -j -n with hidden window and isolated runtime root. This prevents LaunchServices name collision or Core modification. Test-only PID 6383 returned ONE AXMenuBarItem with AXTitle=NEXT, AXEnabled=true; read-only foreground stayed com.openai.codex before/after; Original and RC20 names/PIDs remained unchanged. Only PID 6383 was terminated; zero test processes remained.

H02 exact Wails SetLabel(NEXT) AX result: ISOLATED HOST PASS. Visual icon, notch space, menu click operation and eventual installed NEXT RC21 Host remain OPEN; modified test bundle ID is deliberately NOT claimed as installed full-product UAT.

## Regression results and the important load-sensitive failure

- Vue typecheck PASS, Vitest 25 files/184 tests PASS.
- H04 seven focused ACP tests with -race -count=10 PASSED when no macOS build was concurrent (17.175 s); complete ACP -race PASSED (13.815 s).
- An earlier concurrent native CGO/Swift compile caused focused -race -count=10 to FAIL due 1-second scheduling/watchdog timeouts in TestBlockedNotifyHonorsWriteDeadline, TestCancelledRequestDoesNotWaitForCancelNotification and TestQueuedWriteTimeoutPreservesActiveWrite. This failure is not overwritten by the later success; high-load fixture resilience remains OPEN.
- Release/UpdateIdentity Go tests and Go vet PASS.
- macOS NEXT shared package fixtures 20 PASS, including real RC21 App; expected negative sealed-resource fixture rejection is not a product failure.
- Windows NEXT metadata/negative fixtures 18 tests PASS, 1 opt-in cross-build skipped; no Windows native or installer validation.
- Original 8765 health HTTP 200 version 1.1.1; NEXT 8767 HTTP 200 version 0.9.2-rc.20; original GUI PIDs unchanged; installed NEXT bundle still 21.0.0. Both signed RC20 and RC19 backup exist, but real RC21 rollback not run.
- Candidate source Git checkout remains clean, no ACL/TCC change, no background owned process left.

## Next milestone and blockers

NEXT-only RC21 deployment requires preserving a byte-verified RC20 restore point, checking non-interruption / safe foreground window, limiting replacement to installed NEXT bundle / 8767, verifying rollback, and performing installed H02 Tray visual/AX/menu tests without touching Original or existing user sessions. This task authorized local candidate Build and isolated Host, not actual RC20 replacement.

Hosted NEXT-only CI requires a separate remote branch and hosted run: NONE performed. Developer ID, Apple notarization, formal release approval, public GitHub Release and Update Feed are still BLOCKED/OPEN. Do not treat ad-hoc artifacts as production release ready.

This record is committed on the separate compat documentation branch so the already-built RC21 canonical source SHA remains immutable and exactly reproducible.
