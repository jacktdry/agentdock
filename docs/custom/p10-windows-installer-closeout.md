# P10 Windows installer pre-publish source closeout

> **較新 Windows 驗收範圍（2026-10-14）：** Windows 現階段 source-only 理論可行，無 Windows 設備、不做原生／Installer 驗收，亦**不阻塞 macOS-focused P10**。本文件其餘內容是當時的實作及未交付事實紀錄；Next Windows 仍 fail-closed 且不可發布。見 [Windows source-only acceptance](p10-windows-source-only-acceptance.md)。

2026-10-14. Worker baseline: clean `8caf0995`. This advances the offline
input-validation gate in [the installer source contract](p10-windows-installer-source.md),
not P10/M9 native acceptance. No installer was produced or executed.

## Workflow and scope

Maintainers need to reject wrong or altered Next inputs before invoking ISCC,
without risking an operational Stable installation. The smallest improvement is
a read-only payload verifier in the existing preparation wrapper, reusing the
canonical Go candidate gate. Observable acceptance is rejection of unsafe paths,
Stable metadata, mismatched candidate bytes/hash/architecture, and retained
unconditional wrapper/Inno build rejection. Non-interruption is source-only:
no app, process, service, profile, task or registry operations.

No lifecycle implementation, WPF changes, Stable packaging changes, CI changes,
commit, push, tag, Release, installation, updater, signing or publication is
authorized by this closeout. Other workers own lifecycle, WPF and release tools.

## Offline input check

```sh
rtk proxy python3 -B packaging/windows/prepare-next-rc.py \
  --manifest /absolute/candidate.json --architecture amd64 \
  --verify-payload /absolute/next-payload --output "$PWD/dist/next-setup-check"
```

The output is a check result on stdout; no directory is created. The prospective
output must be fresh and a non-symlink repository-local `dist` subdirectory.
`--build-installer` still exits 2 before input reads or tools, even when combined
with `--verify-payload`. Direct ISCC still encounters the first unconditional
`#error`; no define can enable compilation.

The explicit expanded payload contains exact canonical `candidate.json` bytes
and an externally supplied `next-payload.json` object with these exact fields:

- `schema`: `agentdock-next-windows-payload-v1`
- `product_name`: `AgentDock Next`; `product_variant`: `next`
- `product_id`: `dev.dropabit.agentdock.next`
- `app_id`: `725CE7CB-6F4D-4D78-957E-64CF729FF413`
- `aumid`: `dev.dropabit.agentdock.next.controlpanel`
- `install_root`: `LocalAppData/AgentDock Next`
- `state_directory`: `.agentdock-next`; `listen_port`: integer `8767`
- `architecture`: the requested `amd64` or `arm64`
- `version`, `windows_version`, `commit`, `build_date`, `channel`, `update_feed`:
  exact candidate values
- `candidate_manifest_sha256`: lowercase SHA-256 of exact candidate bytes
- `files`: object mapping every other payload file's portable relative path to
  its lowercase SHA-256; neither metadata file belongs in this map

Root/state strings above are **build inventory location contracts**, not an
installed `runtime.json`. WPF's NextWindowsManifestIdentity requires native
absolute current-user roots plus its full task/startup/home/workspace contract.
This verifier neither fabricates nor accepts that runtime contract. Product
variant/id/port match the runtime worker's explicit Next identity; no defaults
or Stable discovery are used.

The minimum asset list comes from the existing offline builder: Core, arbiter,
tray, both shims, icon, WSL manifest and both Linux helpers, core-skills manifest,
plus cloudflared. All `.exe` inputs get a PE signature/machine check for the
requested architecture. Additional dependencies must be individually inventoried
and hashed. Missing or unlisted files fail. No archive extraction, recursive
Stable fallback, installer execution or signing verification occurs.

Windows traversal/absolute/drive/UNC/device/ADS spellings, reserved filenames,
trailing dots/spaces, backslash aliases, case aliases, duplicate JSON keys,
symlinks and detected reparse components fail. Inputs must be regular files.
The existing `tools/release next-candidate` checks canonical candidate bytes,
clean HEAD, timestamp and identity; it runs with GOPROXY/GOSUMDB off and local
toolchain only. There is no `candidate-verify` command in the inspected baseline.

## Go / no-go

**Go:** reviewable offline input consistency checks, on an immutable staged
directory and a clean canonical checkout with locally available Go dependencies.
Hash inventories supplied alongside files detect inconsistency, not independent
build provenance. PE machine checks do not verify complete executable validity,
version resources, compiled Next identity, assembly dependencies or authenticity.
This is not an atomic handoff to a compiler; files can change after checking.

**No-go:** compiling or publishing Setup. `next-payload.iss` is still deliberately
incomplete, and no producer/accepted complete generation layout exists here.
WPF runtime and separately owned install/uninstall contracts remain unaccepted;
concurrent source additions alone cannot close their native gates. The check
always reports `installable=false` and `release_ready=false`.

Deferred native Windows gates: ISCC compilation/output inspection for both
architectures; complete same-SHA PE/WPF/dependency/shim/helper/skills provenance;
cloudflared Authenticode; physical-path/reparse/account/ACL and task/process
ownership; invalid `/DIR`/saved-INF cases; side-by-side install/uninstall and
generation recovery; Stable files/registry/tasks/credentials/services/process
preservation; no focus stealing/prompts and truthful Next health. Native Windows
path behavior was not exercised on this macOS host.

P11 remains separate: signing/publication, feed and update-target isolation,
installed update/rollback/recovery, and native release UAT. No hosted CI or
production release evidence is claimed.

## Verification evidence

Fast suite: `InstallerSourceTests PayloadIntegrityTests` exercises declarative
isolation, unavoidable gates and synthetic malicious-input rejection. The
synthetic success test mocks only the canonical Go boundary and is not payload
acceptance. `PackagingFixture.test_payload_uses_real_canonical_verifier` uses a
clean temporary local clone and the actual Go gate, with synthetic PE headers;
it rejects a noncanonical candidate despite internally consistent inventory.
`PackagingFixture.test_metadata_and_provenance` checks existing real canonical
preparation. Crossbuild, installer, native Windows and hosted CI are not run.

Final selected suite: **16/16 PASS**, 36.271s, no selected tests skipped. Selection
was `InstallerSourceTests PayloadIntegrityTests` plus PackagingFixture methods
`test_metadata_and_provenance`, `test_payload_uses_real_canonical_verifier`,
`test_dirty_checkout_rejected`, `test_altered_manifest_rejected`, and
`test_outside_or_symlink_output_rejected`. The logged canonical mismatch/exit 1
is the expected negative case. **2/2 Python AST checks PASS**;
`git diff --check` PASS. Inno files were left unchanged, including the unconditional
compiler gate. This worker changed only the preparation script, its test script
and this closeout; concurrent WPF/release/CI changes were preserved.
The task exceeded the local 4,000-token budget; scope stayed source-only.

No runtime resources were started. The next useful workflow is an accepted
complete Next payload/lifecycle followed by separately authorized Windows UAT.
