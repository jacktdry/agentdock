# P11 — macOS Next ad-hoc native manual upgrade checkpoint (2026-10-14)

**Status:** the Next-only **manual local upgrade/installation over an existing app** succeeded on an actual macOS arm64 host. This is a bounded P11 milestone, **NOT P11 full closeout**: Next GUI process restart/native visual UAT, deliberately injected *live* rollback/interruption recovery and unattended auto-updater acceptance have not passed. No paid Developer ID or Notarization; official auto-update remains disabled.

## Product problem and scope

The already installed **Next** (Core `0.9.1`, commit `154f73249671`, Core PID `1362` / port `8767`) was newer than the P10 demonstrator build (`0.9.1-rc.1`, `cceadb0c`). The P10 candidate is a SemVer **downgrade**; blindly replacing an active Next app could roll back a version unexpectedly. Stable AgentDock is an absolute exclusion: `/Applications/AgentDock.app`, `com.uvwt.agentdock.core` and port `8765`.

- Added source-bundle and version preflight to `packaging/macos/deploy-next-shared.py` at `0c2c5e88`: after exact Next bundle/signature validation, run the verified Core `version --json` only (5s, max 4096 bytes, strict JSON + platform + canonical SemVer + commit) and reject downgrade or equal precedence with conflicting commit **before jobs, install or service registration**. Same-version/same-commit reinstall remains allowed; higher SemVer may proceed. Current certificate-bound Next GUI auto-updater was not modified.
- Offline `scripts/test/test-next-shared-deploy.py`: **23/23 PASS** including same-version ambiguity, Core output corruption/timeouts and existing Next-only rollback. `scripts/test/test-next-shared-package.py`: **9/9 PASS** on actual signed P10 Next App. Full real bundled Next App Contents native `renameatx_np` exchange and reversal (with deliberate post-swap validation failure) succeeded on private auto-cleaned `/private/tmp` clones; **this is an isolated native file-operation test, not live failure injection**.
- Direct read-only preflight against actual installed `0.9.1` and signed P10 `0.9.1-rc.1`: **rejected as downgrade** without modifying Next or Stable. Repeated actual `deploy-next-shared.py` invocation with the older P10 signed App after live upgrade: **rejected before any mutation**; Next/Stable Core PIDs remained unchanged.

## Forward-version test candidate and actual native deploy

Produced a new **local-only** Next candidate **`0.9.2-rc.1`** from a clean committed source:
`0c2c5e888a0c896f14c4af1c8707f891515200e4`,
source UTC `2026-10-14T07:16:49Z`, `bundle-build=2`.
Version number is a **test candidate only**, not a user-approved public release.

Path: `dist/next-shared-arm64.62jk6Q/package/` within P11 worktree. Built actual arm64 Wails Next.app + Next Core, arbiter, Skill Bundle, login helper, registrar, cloudflared, ZIP and DMG. Ad-hoc code signatures, DMG checksum and **9/9 package tests PASS**, without Developer ID, Notarization or bypassing Gatekeeper.

| Local artifact | Size in bytes | SHA-256 |
| --- | ---: | --- |
| `AgentDock-Next-macos-arm64.zip` | 29,902,935 | `fa1b2ca8a04f0a50b9b5e1e3d21018b3e28a98fe02857bb61ada989f79224614` |
| `AgentDock-Next-macos-arm64.dmg` | 34,443,391 | `019e15c05fb4489862220ce4c86862d63b8a9488470a32d8cbe7c29ba56813d8` |

`next-artifacts --verify`, both generated `sha256` sidecars, Bundle IDs/signatures and Core version: **PASS**; candidate `release_ready=false` and feed `disabled/unpublished`.

Native mutation admission used Next exact signed bundle + Core version, healthy trusted Next PID and port `8767`, no established Core sessions, and read-only Stable PID/file snapshot. Invoked only `PYTHONDONTWRITEBYTECODE=1 python3 -B packaging/macos/deploy-next-shared.py <local Next candidate App>`:

- **Command exit 0**; installed `~/Applications/AgentDock Next.app` Core **`0.9.2-rc.1`**, commit `0c2c5e888a0c`; new Next Core PID **`71292`**, port **`8767`**, exact owned binary/listener/health verified; Next Tunnel PID **`71579`**, exact Next launchd label.
- Verified the signed, valid previous Next `0.9.1` App Contents is retained at `~/Applications/.agentdock-next-replacement-xhvwhdol/AgentDock Next.app/Contents` (do **not delete**; retained for recovery).
- **Stable PID `12127` / port `8765` remained identical**, and Stable Core helper `st_dev`, `st_ino`, size and modification time matched pre-install snapshot exactly. No Stable App, Stable state, launchd service, Chrome or user browser was intentionally modified.
- **Next GUI PID `730` was intentionally not restarted** to avoid stealing focus/interrupting desktop work; existing GUI process can retain old in-memory code until Next GUI is explicitly restarted.

## Remaining real P11 acceptance (not PASS)

1. Next GUI **safe, authorized background restart** then native UI/Tray, Core status and ACP/MCP/Connection truthful state smoke (including no focus stealing); active GUI remained running, so do not describe native visual GUI UAT as done.
2. **Live** controlled fault injection of Next-only rollback/interruption and service re-registration recovery with owner/backup/health assertions and Stable PID unchanged. Offline and temporary real file exchange already passed, but do not substitute them for an actual failure recovery test of installed Next.
3. Isolated fresh Next installation/reinstallation where safe and needed; this run exercised versioned replacement of an existing app, not first-time installation.
4. No automatic GUI update with ad-hoc signer: existing Next certificate-bound updater intentionally fails closed; Developer ID/Notarization and Windows native remain explicitly out of P11 scope.
5. Hosted PR CI and formal Tag/Release/feed activation never run or authorized; do not claim public release readiness.

P11 can continue from the installed healthy Next `0.9.2-rc.1` with its **old-version backup preserved**; use stable Mac-Dev to orchestrate any later Next-only live mutation, verify session activity before restarting Next and never affect Stable.
