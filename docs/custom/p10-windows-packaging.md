# 2026-10-14 scope revision: source-only Windows feasibility

依使用者最新決定，Windows 理論可行即符合本階段範圍：source tests／交叉編譯／靜態安全審查，不做 Windows host／VM／Installer native UAT，完整 Next Windows Installer 與 WPF 執行仍為未交付功能，**非 macOS P10 closeout gate**。保留 build/install fail-closed，嚴禁宣稱 Windows release-ready。詳 [Windows 驗收決策](p10-windows-source-only-acceptance.md)。以下是決策前的歷史 checkpoint。

---

# P10 Windows RC packaging checkpoint

2026-10-14; reviewed clean `feature/p10-release-migration` at
`66b77426e0e082972196c9f25b32b23e887b8ab0`. This worker's additions are
uncommitted. P10/M9 remains open; there is **no runnable Next Windows installer**.

## Workflow and bounded outcome

- User pain: RC preparation must not label Stable binaries as Next or install over Stable.
- Direct improvement: maintainers can prepare exact canonical Next provenance, Windows
  version resources and optional unsigned Core/arbiter binaries without launching them.
- Simplest implementation: one independent Python wrapper reuses `next-candidate`;
  Stable packaging, installer scripts, workflows, updater and UI remain unchanged.
- Acceptance/non-interruption: canonical verifier and offline Git-clone fixture;
  negative manifest/path/dirty checks; optional Windows crossbuild with read-only
  binary inspection. No app/service, port, consent, profile or scheduled-task actions.
- Exclusions: installer execution, publishing, tag/Release, push/commit, signing,
  UIUX and P11 native update/rollback. This advances P10 metadata/Windows preparation,
  not native installation or milestone acceptance.

## What runs now

`packaging/windows/prepare-next-rc.py` requires Python 3, Git and Go. Run from the
repository root with a canonical P10 manifest. `next-candidate` enforces exact JSON
bytes, clean source including untracked files, HEAD/source timestamp, canonical Next
identity, and a non-symlink repo-local `dist` subdirectory. Output must be fresh.
The wrapper snapshots and re-verifies identical manifest bytes before producing:

- `candidate.json` plus its SHA-256 in `preparation.json` (a derivative record,
  never a replacement canonical manifest).
- `overlay.json` / `next-buildinfo.go`: full RC Version; linker flags bind full
  Commit and UTC BuildDate equally to Core and arbiter.
- Per-executable `.winres.json`: ProductName `AgentDock Next`, numeric FileVersion
  and ProductVersion `M.m.p.N`, Next descriptions, original filenames and full
  source provenance in Comments.
- `next-version.props`: numeric Version/FileVersion/AssemblyVersion, full RC
  InformationalVersion, Next Product/AssemblyTitle/Description. This is build-only
  input for later WPF packaging; it does not isolate the existing WPF runtime.
- `preparation.json`: architecture, provenance, disabled/unpublished channel,
  `release_ready:false`, `installable:false`, explicit installer blocker.

Default mode builds no binaries and makes no claim that PE resources were applied.
`--build-binaries` cross-compiles only `agentdock.exe` and `agentdock-arbiter.exe`
using `CGO_ENABLED=0`, then uses the existing Stable tooling's pinned
`go-winres@v0.3.3` host patch operation with independent Next resource inputs.
Checksums in the record and individual `.exe.sha256` sidecars describe the resulting
patched unsigned binaries. They are **not** a
complete install payload: no tray/shims/WSL/cloudflared/skills archive is produced.
Never launch them: runtime/updater isolation has not been accepted on Windows.
Missing dependencies or build/patch errors fail with a nonzero exit. An incomplete
fresh directory may remain for inspection; it has no successful preparation record.
No existing output is overwritten, no signing secrets are used.

After owner integration into a **clean HEAD**, generate an example manifest
(version/ordinal are illustrative, not a selected release):

```sh
rtk proxy python3 -B - <<'PY'
import datetime, subprocess
from pathlib import Path
sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
stamp = int(subprocess.check_output(['git', 'show', '-s', '--format=%ct', 'HEAD'], text=True))
date = datetime.datetime.fromtimestamp(stamp, datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
data = subprocess.check_output(['go', 'run', '-mod=readonly', './tools/release', 'next-preflight',
    '--version', '0.9.1-rc.1', '--tag', 'next-v0.9.1-rc.1', '--commit', sha,
    '--build-date', date, '--bundle-build', '1'])
Path('/private/tmp/agentdock-p10-windows-candidate.json').write_bytes(data)
PY
rtk proxy python3 -B packaging/windows/prepare-next-rc.py \
  --manifest /private/tmp/agentdock-p10-windows-candidate.json \
  --architecture amd64 --output dist/next-windows-amd64-metadata
# Optional unsigned binary preparation; requires cached/available go-winres dependencies.
# Use another fresh directory and add --build-binaries to the same command.
# Repeat for arm64 with a distinct output directory.
```

On Windows, use `python`/`py -3` and an explicit local manifest path rather than the
macOS `/private/tmp` example. Python preparation works on either host; no PowerShell
or Inno is required for metadata. The modified worker checkout deliberately fails
the clean-source gate. Fixture acceptance cannot bind these new scripts to HEAD
until the owner integrates them; no bypass or worker commit is supplied.

`--build-installer` always exits 2 **before any tool/output/ISCC operation**.
`AgentDock-NextSetup-amd64.exe` and `AgentDock-NextSetup-arm64.exe` remain planned
canonical filenames, not generated artifacts. There is no Next `.iss` file or
silent fallback to `AgentDock.iss` / `build-windows-offline-setup.ps1`.

After an authorized unsigned binary build, inspect without launching on Windows:

```powershell
$root = 'dist/next-windows-amd64-binaries' # explicit fresh build output
$record = Get-Content -Raw "$root/preparation.json" | ConvertFrom-Json
foreach ($name in @('agentdock.exe', 'agentdock-arbiter.exe')) {
    $path = Join-Path $root $name
    $info = [Diagnostics.FileVersionInfo]::GetVersionInfo((Resolve-Path $path).Path)
    if ($info.ProductName -cne 'AgentDock Next' -or
        $info.FileVersion -cne $record.windows_version -or
        $info.ProductVersion -cne $record.windows_version) { throw 'Next PE metadata mismatch' }
    if ((Get-FileHash $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne
        $record.binary_checksums.$name) { throw 'Next binary hash mismatch' }
    go version -m $path # check full linker commit/date and GOOS/GOARCH
}
```

This read-only Windows inspection has not been executed by this macOS worker.

## Specific isolation blockers and P10 work remaining

| Existing source | Why a renamed Setup would be unsafe / required completion |
| --- | --- |
| `packaging/windows/AgentDock.iss` | Stable GUID `{D6788C7A-4104-48D4-B5C3-F4858B5606EA}`, `LocalAppData\AgentDock`, group name, previous install reuse, shortcuts, uninstall deletions and stable AUMID. A reviewed Next-only identity and separate install root/uninstaller are required. |
| `packaging/windows/includes/code.iss` | Extracts/runs Stable `install.ps1`, default port unless silent override, Stable runtime activation/credential handling. Next setup must carry only Next-aware install/launch scripts; neither a port override nor GUID rename suffices. |
| `scripts/install/install.ps1` | Defaults to Stable home/workspace/port/startup names; fixed `AgentDock` scheduled-task start/elevated/restore paths. Next-only install/startup/task/tunnel/credential/recovery ownership must reject Stable paths and no-identity fallback. |
| `desktop/windows/control-panel/App.xaml.cs` | Stable AUMID plus `Local\AgentDock.ControlPanel.Singleton` and Show event can communicate with Stable. Needs Next runtime identities without UIUX redesign. Outside this worker ownership. |
| `desktop/windows/control-panel/Services/RuntimeService.cs` | Stable `LocalAppData\AgentDock`, `.agentdock` Nexus path, 8765 fallback, workspace and task defaults. Next discovery/operations must bind only Next targets and reject Stable fallback. Outside this worker ownership. |
| `internal/selfupdate` and release consumers | Upstream Stable feed remains; canonical Next feed must remain disabled. Separate candidate payload architecture/hash/PE metadata checks and Next channel/target gates are required before any installable artifact. Outside this worker ownership. |

Next completion sequence: owner implements/reviews these runtime namespaces and
install/uninstall ownership; Next-only payload builder includes all required same-SHA
components; Windows build validates actual PE File/Product metadata and WPF
Assembly/Informational metadata; Next Inno build validates architecture, identity,
manifest/hash binding and output checksum. Only then enable compilation after a
reviewed fail-closed gate. These are P10 source/packaging tasks independent of visual
UIUX changes. Signing and native install/update/rollback remain separately authorized
P11 gates. Existing Stable script behavior must not be changed speculatively.

## Verification evidence

- `rtk proxy python3 -B scripts/test/test-next-windows-packaging.py -v`:
  **5/5 PASS**, 449.443s (initial cold-cache run, before adding opt-in crossbuild).
  Actual canonical Go verifier in a clean local clone; five manifest tamper cases,
  dirty source, outside/symlink/existing destinations, metadata/provenance and
  pre-ISCC rejection. The clone creates no commits and is removed at teardown.
- `rtk proxy python3 -B packaging/windows/prepare-next-rc.py --manifest
  /private/tmp/nonexistent-candidate.json --architecture amd64 --output
  dist/should-not-exist --build-installer`: expected **exit 2**, explicit isolation
  blocker, before reading the nonexistent manifest or producing output.
- Python AST syntax checks: **PASS**. `rtk git diff --check`: **PASS** at checkpoint.
- `rtk proxy env GOCACHE=/private/tmp/agentdock-p10-windows-go-cache GOPROXY=off
  GOTOOLCHAIN=local go test ./tools/release ./internal/buildinfo -count=1`:
  **PASS**, release tests 414.850s; buildinfo has **no test files**.
  This is local source-test evidence, not CI/native installer acceptance.

Further final results are appended below. Source/fixture and crossbuild results do
not prove Windows Inno compilation, native WPF build, installer success, service
health, release readiness or P11 acceptance. `pwsh` and `ISCC.exe` are unavailable
on this macOS host; no PowerShell parser/source lint was run. The pinned go-winres
CLI is not cached here, so the optional full PE-patching path and native PE readback
remain unverified. Metadata resources are prepared, not claimed as applied.

Concurrent changes appeared under CI/selfupdate and their separate checkpoint docs
during verification. This worker neither edits nor reverts them. Worker ownership
is exactly the three new files listed by this checkpoint; no Stable file was changed.

The nearest real workflow is clean-HEAD Next metadata/Core preparation, then owner
integration of Windows runtime isolation. No runtime resources were started.

## 2026-10-14 clean-HEAD source-only Windows dual-architecture crossbuild

從 clean P10 source SHA `3cecb735dd6133c9455cc12a0f4e87be73616587` 的 canonical `next-preflight` manifest (`0.9.1-rc.1` illustrative only)、Next-only Go overlay 及相同 full Commit/UTC BuildDate (`2026-10-14T04:10:32Z`)，以 `CGO_ENABLED=0 GOOS=windows` 分別交叉編譯 `cmd/agentdock` 和 `cmd/agentdock-arbiter` 於 amd64／arm64，輸出僅在忽略版控的 `dist/p10-windows-source-<SHA>/`。四個 binary 的 PE `MZ` magic、`go version -m` 的 GOOS、GOARCH、完整 source SHA 和 UTC BuildDate：**4/4 PASS**。來源工作樹前後皆乾淨，無安裝、啟動、修改服務、Push、Tag 或 Release。

這次沒有使用 `prepare-next-rc.py --build-binaries` 的 `go-winres` patch 路徑，**未套用/讀回 Windows File/Product VersionInfo**；此證據不代表 WPF UI binary、tray/shims、完整 Next payload、獨立 Inno Setup、原生 Windows 啟動或正式 Installer 已完成。完整 P10 Windows installer 隔離門檻與 Windows Hosted CI 仍待後續工作；P11 签章、更新／回滾也未啟動。
