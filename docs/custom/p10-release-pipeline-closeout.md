# P10 source release pipeline checkpoint (PRE-PUBLISH)

2026-10-14; clean starting HEAD `8caf0995d5ca773bbe1b0f167cdad273ecc2f95e`.
This completes the available-file inventory source implementation, not P10/M9 acceptance or release readiness. Canonical contracts remain [p10-release-preflight.md](p10-release-preflight.md) and [p10-ci-release-pipeline.md](p10-ci-release-pipeline.md).

## Workflow and bounded acceptance

- Pain: an existing local Next candidate or native CI Core needs a reproducible byte inventory without claiming that missing installers exist.
- Improvement: maintainers can generate and later compare SHA-256, byte size, name, format, platform and the full canonical source manifest for explicitly selected available files.
- Simplest implementation: `next-artifacts` in the existing release tool; reuse `next-candidate` for canonical manifest, full HEAD SHA, RC version, UTC source date, Next identity and clean checkout validation. Only the command switch is added to `main.go`.
- Acceptance/non-interruption: deterministic and negative Go tests, YAML/policy assertions and embedded Python syntax checks. No app, server, browser, service, profile, focus, OS dialog, installation or update operations.
- Excluded: Windows worker files, Stable release workflow, UI, installer build, publishing workflow, workflow dispatch, tag, commit/push, GitHub Release, updater enablement and P11 signing.

## Inventory contract

`agentdock-next-artifacts-v1` / `source-only-available-files` embeds the entire verified candidate, including full source SHA, RC version and source date. Both inventory and candidate keep `release_ready:false`; feeds and remaining gates are unchanged. No timestamp from wall clock is added.

| Platform | Accepted basename | Format check |
| --- | --- | --- |
| darwin/arm64 | `AgentDock-Next-macos-arm64.zip` | ZIP central directory parses |
| darwin/arm64 | `AgentDock-Next-macos-arm64.dmg` | UDIF `koly` trailer |
| darwin/arm64 | `agentdock-next-core`, `agentdock-next-arbiter` | arm64 Mach-O executable |
| windows/amd64, windows/arm64 | `agentdock-next-core.exe`, `agentdock-next-arbiter.exe` | matching PE machine, executable, not DLL |
| windows/amd64, windows/arm64 | `AgentDock-NextSetup-<arch>.exe` | matching PE machine, executable, not DLL |

These Core names follow existing CI staging. Generic Windows payload basenames such as `agentdock.exe` / `agentdock-arbiter.exe` overlap Stable and are intentionally rejected. A separately verified Next payload can be copied into separate source staging under the explicit Next basename before inventory; this tool does not rename files or establish that provenance. Installer availability is optional, never inferred from a planned name.

Supply at least one filename. Only selected files are inventoried, independently of other platform/package availability; unrelated files and overlays in the directory are ignored. Filenames must be exact allowed basenames, unique, and direct children of the supplied canonical directory. Unknown, Stable, traversal, absolute or nested names fail. Symlinked roots (including ancestors), symlinked artifacts, nonregular/empty/missing files and detectable replacement/size/modification during reading fail. Entries are sorted deterministically; SHA-256 is streamed from the same opened file used for format inspection.

`--verify` regenerates from current files and compares exact canonical JSON bytes. Wrong hash, size, type, platform, filename, manifest SHA/version/date, readiness flag, unknown/duplicate fields or formatting fail. It never imports inventory values as authority or fills missing files. Keep the inventory outside selected artifact paths.

Example after owner integration into a clean SHA, using an existing matching manifest and already built files:

```sh
rtk proxy go run ./tools/release next-artifacts \
  --manifest /absolute/path/candidate.json \
  --root /absolute/canonical/path/to/package --platform darwin/arm64 \
  AgentDock-Next-macos-arm64.zip AgentDock-Next-macos-arm64.dmg \
  > /private/tmp/next-artifacts.json
rtk proxy go run ./tools/release next-artifacts \
  --manifest /absolute/path/candidate.json \
  --root /absolute/canonical/path/to/package --platform darwin/arm64 \
  --verify /private/tmp/next-artifacts.json \
  AgentDock-Next-macos-arm64.zip AgentDock-Next-macos-arm64.dmg
```

The manifest binds the checked-out source contract; the inventory binds supplied bytes to that contract as a record. Format/hash checks **do not prove** ZIP/DMG internal architecture, embedded binary version/source SHA, installer payload provenance, signatures, notarization, successful installation or complete release. Existing native Core CI separately validates actual runtime version and embedded Go full-SHA/date metadata before inventory. Local package provenance retains its separate accepted packaging checks; merely renaming a binary never makes it a Next candidate.

## PR CI and local evidence

The existing `candidate-core` jobs now generate and verify a one-file inventory for the actual Core they just built. The selected platform comes from the native matrix. No Setup, arbiter, ZIP or DMG is demanded or presented as built. Inventory is logged and kept in runner temp only; no upload/publish step is added. PR-only triggers, `contents: read`, no persisted checkout credentials and all existing native provenance checks remain.

- Final `GOCACHE=/private/tmp/agentdock-p10-artifact-gocache go test ./tools/release -count=1 -v`: PASS, 19.797s, no skips on this macOS host. Includes deterministic ZIP/DMG/PE/Mach-O fixtures and rejection of changed hash/size/type/source tuple/readiness/JSON, unsafe paths, duplicate or Stable names and wrong executable architecture. Existing overlay/arbiter compilation and plist fixtures also passed. Fixtures are not real package UAT.
- Ruby YAML parse plus PR-only trigger, read permission and credential assertions: PASS. Initial assertion used YAML 1.2 `on` spelling against Ruby's YAML 1.1 boolean key; corrected key handling and reran successfully.
- Embedded workflow Python AST and Core-only inventory/policy assertions: PASS. These do not substitute for actionlint or Hosted Actions execution.
- `git diff --check`: PASS.
- No actual package rebuild, actual historical candidate inventory run, Hosted CI, remote dispatch, Windows native execution or publishing was performed. Historical macOS files must not be relabeled as built from this uncommitted source change or a future integrated SHA.

Other worker changes appeared in the shared working tree during validation; they were preserved. Only release tool inventory/tests, the Next preflight workflow and this document are owned by this checkpoint. There is no existing codebase-memory index for this worktree; this document records the durable contract within the authorized file scope. No shared/stable memory store was rebuilt or mutated.

## External gates and next useful workflow

1. Owner integration/UIUX freeze and a clean immutable source SHA; choose actual RC and monotonic bundle ordinal rather than the illustrative CI version.
2. An actual remote PR run on Ubuntu, native macOS arm64 and Windows amd64/arm64, including runner availability, build metadata, read-only version smoke and inventory comparison. Locally parsed CI is not executed CI.
3. Full macOS arm64 candidate rebuilt from that same SHA and independent embedded payload metadata/architecture validation. Ad-hoc signing and a checksum inventory do not satisfy Developer ID/notarization or native GUI parity.
4. Separately owned isolated Windows Next Core/arbiter/control-panel/Setup integration, metadata/hash evidence and native installer acceptance. A Core-only CI run cannot close this gate, and inventory does not require unsigned Setup to be useful.
5. P11 signing, explicitly authorized release/publish, Next feed activation, native install/update/trial/commit/rollback/interruption/recovery evidence. Updater remains disabled; this checkpoint grants none of these actions.

Next closest workflow: review actual native Core PR evidence for the integrated SHA, then inventory the available independently verified package files. Keep missing platform/install/signing gates visible rather than adding a publishing workflow or declaring release readiness.
