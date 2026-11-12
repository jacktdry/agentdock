# P10 — M9 Release Migration: macOS-first local prepublish engineering closeout

> **後續 P11 使用者決策（2026-10-14）：** 沒有付費 Apple Developer Program 帳號，Developer ID 正式簽章、Notarization、公證票據 stapling 與憑證綁定的 GUI 自動更新驗收均移出 P11 本地測試範圍；P11 仍驗證 Next-only ad-hoc 本地安裝、手動更新／回滾。**Updater certificate gate 不可放寬**；以下舊 P11 簽章描述屬歷史紀錄。見 [P11 scope](p11-adhoc-native-uat-scope.md)。

**Decision / acceptance (2026-10-14):** User confirms required Next UI/UX corrections are completed in a separate workline. P10 integrates that workline and completes **local source, version, package and prepublish CI contract** for macOS arm64. The user separately decided that Windows/WSL requires theoretical source/cross-build feasibility only; it is **non-shipping** and Windows native/VM/UAT is not a gate. This does not grant consent to publish, tag, install, activate a feed, or retire Stable AgentDock.

## Product job and integration ownership

- User can move the completed Shared Desktop UI/UX and Next release source to a single macOS prepublish release candidate without interfering with the installed Stable control channel or the installed Next instance.
- Started from isolated `feature/p10-release-migration` and merged the completed `feature/next-uiux-audit` at `e14a9af1` (UI/UX source head `0c7dbcdd`); no merge conflicts. Experimental `feature/next-mcp-card-ux` commit `d92dd1fe` is **not included** (P13/P14 remain separate).
- Source-build fixes: registered six previously omitted Next Windows Python/Inno scripts in `scripts/governance/inventory.yaml`; their original fail-closed protections are unchanged. The P10 RC manifest dropped already resolved `uiux-unfrozen` / `custom/main-not-integrated` markers while retaining conservative final-asset, hosted-CI, explicitly authorized publication and P11 checks. Fix committed as `cceadb0c`.
- Source code targeted for this P10 artifact build: **`cceadb0cec6b860e0d26399afe6779a949440eed`** (full SHA). The subsequent documentation-only closeout commit, if any, is *not* the binary provenance SHA. Any real public release must regenerate manifests/artifacts for its selected immutable release SHA.

## Same-SHA macOS arm64 Next RC evidence — PASS

From a clean `cceadb0c` checkout, generated and exact-verified canonical `next-preflight` `0.9.1-rc.1` manifest and full Next Wails/Desktop/Core/arbiter/Skills/cloudflared bundle using `build-next-shared.sh --candidate-manifest`. Actual local output:

`dist/next-shared-arm64.UxHhPF/package/`

- `AgentDock Next.app` — actual macOS arm64 Wails executable + Next Core + arbiter + cloudflared + service registrar and login helper; App ID `dev.dropabit.agentdock.next`, `CFBundleShortVersionString=0.9.1`, `CFBundleVersion=1.0.0`, minimum macOS `13.0`.
- `AgentDock-Next-macos-arm64.zip` — **29,902,932 bytes**, SHA-256 `516dbc5c58c759256b6d5cccfc6312060ff659fa2d3e3438fb46089e4e4555e3`.
- `AgentDock-Next-macos-arm64.dmg` — **34,443,389 bytes**, SHA-256 `ae28e0f8a61e957a72dbff962bb5c13ff24dde5c6bd561b9a214384c6e342b5c`.
- Runtime `agentdock version --json`: `0.9.1-rc.1`, 12-character commit `cceadb0cec6b`, source UTC `2026-10-14T06:26:25Z`, `darwin/arm64`. Both Core and arbiter `go version -m` report the **full exact SHA**, exact UTC `vcs.time`, `vcs.modified=false`, `darwin/arm64`. The compiled `-trimpath` binaries omit `-ldflags` in `go version -m`; provenance is checked via the actual full VCS records and read-only Core version CLI, not invented linker-flag output.
- `codesign --verify --deep --strict`: PASS, **ad-hoc only** (not Developer ID/notarized). `hdiutil verify`: valid. ZIP/DMG `shasum -a 256 -c`: both PASS.
- `scripts/test/test-next-shared-package.py`: **9/9 PASS**, full actual Next Shared bundle validated. `tools/release next-artifacts --verify`: same canonical candidate, two macOS files, byte-exact inventory PASS. Candidate and inventory deliberately remain `release_ready=false`, update feed `disabled/unpublished`.

## Integrated code and regressions — PASS

- Shared Desktop frontend `pnpm test`: **168/168** tests, 23/23 files PASS. `pnpm build`: Vue TypeScript check + production Vite build PASS (nonblocking JS chunk-size performance warning).
- Independent `desktop/shared-poc` Go module `go test ./... -count=1`: PASS (macOS SDK object/link-target warning, not a test failure).
- Root `GOMAXPROCS=2 GOFLAGS=-p=2 make check`: **exit 0**; includes Go module tests, three-language 882-key i18n, tracked Script Governance, shell-script test entry, `go vet`, Core build. First integrated run exposed the missing six governance entries; fixed and reran **successfully**. Root `./desktop/...` intentionally does not include the separate Wails module; its test is recorded above.
- Windows feasibility, not native acceptance: existing dual-architecture Core/arbiter PE cross-build evidence; `InstallerSourceTests + PayloadIntegrityTests`: **11/11 PASS**, Next WPF source identity tests **4/4 PASS**. Next WPF and Inno Setup remain fail-closed and no Windows Setup ships.
- Git source worktree was clean at build and after tests; Stable release script/installed App/services and P13/P14 experiments were not changed.

## Local P10 closeout / activation boundaries

**P10 prepublish source/package engineering is complete locally for macOS arm64.** This is **not** a public production release and **not** a P11 sign/update/rollback acceptance. Canonical `next-preflight` intentionally reports conservative post-build `remaining_gates` until formal release attestation; the successful on-disk artifact inventory and tests are separate evidence. Do not silently set `release_ready=true`.

- Read-only `.github/workflows/next-release-preflight.yml` is defined, locally syntax/policy-checked and exercises source and native macOS Core candidate inventory. **GitHub Hosted CI has not executed**, as no branch push/PR was authorized. It is an *explicit required check before any real publication*, not a claimed local PASS.
- Formal release branch/tag/GitHub Release/update channel enablement are **not authorized**. Actual public-version selection, monotonic build ordinal and immutable new release SHA must be confirmed at the release gate; the `0.9.1-rc.1` here is an illustrative local RC.
- **P11** still owns Developer ID signing/notarization (when applicable), updater target/trial/commit/rollback/recovery, and macOS native GUI/install/update UAT. Windows/WSL native UAT is nonmandatory under the user's source-only Windows decision, and Windows installer remains blocked.
- **P12** native UI retirement and **P13/P14** task/card experiments remain independent; no unapproved Stable retirement or Windows release has occurred.
- The local `custom/main` integration is required to present the single accepted source line; do not mistake a worktree merge or locally produced App for an upload to GitHub.

## Verification limitations

Full macOS bundle was locally built from immutable `cceadb0c`, not from a later docs-only commit or a publicly selected final release SHA. File hashes bind only actual local files; they are not notarization, Publisher certificate, hosted attestation or user installation evidence. `hdiutil` emitted a deprecation warning, and tests emitted platform-linked object warnings and a Vite chunk warning; all corresponding commands returned successful exit codes.
