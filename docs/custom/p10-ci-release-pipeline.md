# P10 Next CI candidate Core validation

2026-10-14 worker checkpoint; base `66b77426e0e082972196c9f25b32b23e887b8ab0`.
This advances P10 source-to-candidate evidence only. It does not close P10/M9 or authorize a release.

## Workflow and acceptance

- User pain: Linux source tests cannot prove that a Next RC Core actually compiles and reports the expected version on macOS and Windows.
- Direct improvement: PR CI builds the actual `cmd/agentdock` Core and verifies its native, read-only `version --json` result against the canonical source manifest.
- Smallest implementation: extend `.github/workflows/next-release-preflight.yml`, reuse `next-preflight` and `next-candidate`, and use their existing Next-only Go overlay. Stable source constants remain unchanged.
- Acceptance/non-interruption: clean checkout, canonical manifest, native host match, real compilation, exact metadata checks and SHA-256; no server, GUI, focus, dialogs, service, profile or user-state operations.
- Excluded: UIUX, installer/app packaging, install/update, runtime/server tests, signing, P11, tag, publication, commit/push and Stable changes.

## Jobs

| Job | Host | Evidence |
| --- | --- | --- |
| `preflight` | `ubuntu-latest` | Existing `tools/release` and `internal/buildinfo` tests; offline Windows Next metadata fixtures; source manifest generation and exact-byte verification |
| `candidate-core` darwin/arm64 | `macos-15` | Focused source contract tests, actual native Next candidate Core build and metadata/hash checks |
| `candidate-core` windows/amd64 | `windows-latest` | Same native checks for amd64 `.exe` |
| `candidate-core` windows/arm64 | `windows-11-arm` | Same native checks for arm64 `.exe` |

The native jobs require `GOHOSTOS/GOHOSTARCH` to equal the matrix target. A missing runner, changed architecture, build failure or execution failure is a blocker, not a silent cross-build success. These labels follow [GitHub's hosted runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners); actual scheduling/installed tools still require remote CI execution.

PR path filters cover Core commands/internal packages, Go dependencies, release helpers, canonical macOS identity, `packaging/windows/**`, `scripts/test/test-next-windows-packaging.py`, this workflow and this document. Checkout uses the PR merge snapshot consistently in each job. There is no dispatch or tag trigger, `pull_request_target`, secret, write scope, credential persistence, release API, installer invocation or upload to GitHub Releases. Permissions are only `contents: read`; setup caches are disabled. Dependencies/toolchains can be fetched by normal CI setup/build, but no network application tests or updater calls run.

The Windows worker's Python suite checks canonical metadata/provenance, dirty and altered inputs, unsafe output destinations and installer refusal before any installer tool runs. Its opt-in cross-build case is skipped by default (`NEXT_WINDOWS_BUILD_SMOKE` is unset); this is recorded separately from the actual native Core build jobs. The existing Go suite's macOS plist fixture is skipped on Ubuntu because it requires macOS tools.

## Candidate checks

`0.9.1-rc.1` and bundle ordinal `1` are fixed illustrative CI inputs, not an allocated RC or monotonic bundle build. Each host generates the same deterministic contract from its checked-out full HEAD SHA and UTC commit timestamp, verifies its exact canonical bytes, then revalidates clean HEAD/identity through `next-candidate`. Windows writes manifest bytes without newline conversion. All commands use argument arrays or quoted shell environment variables; PR text is never interpolated into executable shell code.

The generated overlay is exclusively created in fresh ignored `dist/p10-ci-core`; `go build -mod=readonly -buildvcs=true` builds the actual Core with `CGO_ENABLED=0`. P10 Next macOS supports arm64 only: planned assets are `AgentDock-Next-macos-arm64.zip/.dmg`, matching the canonical shell single-architecture identity and disabled update identity. Stable retains `AgentDock-macos-universal.zip`; this CI does not prove packaging. `-trimpath` is intentionally omitted because Go omits linker flags from embedded build settings with that option; this gate checks those settings explicitly.

The binary's `go version -m -json` must contain exact GOOS/GOARCH, disabled CGo, full `vcs.revision`, source `vcs.time`, `vcs.modified=false`, and the exact Commit/BuildDate linker arguments. Native `version --json` must match full RC version, 12-character runtime commit, source build date and platform. The full SHA check compensates for `buildinfo.Current()` truncating runtime commit. A final clean-checkout check rejects build-induced source changes.

CI logs print the canonical candidate, runtime JSON, embedded Go metadata and SHA-256 of the checked binary after success. There is no staging artifact upload; binaries and overlays expire with each runner. Hashes identify these unsigned source-only binaries, not distributable installer sets or release attestations. The source manifest remains `source-only-dry-run`, `release_ready:false`, feed disabled/unpublished, and its remaining gates include `windows-next-installer-not-isolated` and `hosted-ci-native-artifacts-not-verified`, replacing the completed Core overlay/channel guard source gates without granting closeout.

## Validation evidence and remaining gates

Local evidence is recorded below. No actionlint/yamllint is installed locally; generic YAML parsing does not validate GitHub Actions semantics.

- Ruby YAML parse and trigger/permission assertions: PASS. Workflow candidate Python extraction/compilation and policy assertions: PASS. `git diff --check`: PASS.
- `GOCACHE=/private/tmp/agentdock-p10-ci-gocache python3 scripts/test/test-next-windows-packaging.py -v`: PASS, 6 cases in 351.740s; 5 passed, opt-in cross-build skipped. This ran the concurrent Windows worker's source fixture without changing its files.
- `GOCACHE=/private/tmp/agentdock-p10-ci-gocache go test ./tools/release ./internal/buildinfo -count=1 -v`: PASS; release suite 373.602s, including overlay, actual arbiter compilation and macOS plist fixtures. No local Go test skips; `internal/buildinfo` reports no test files. These are source/fixture tests, not installer or native app acceptance.

Hosted Ubuntu/macOS/Windows jobs have not been executed by this worker. Windows execution, arm64 runner scheduling and end-to-end PR CI success require owner integration and a remote PR run. Local macOS smoke uses a clean clone of the base commit to preserve concurrent workers and the candidate gate; it is not evidence for their uncommitted changes or for a future integrated SHA.

No isolated Next installer path is invoked here. In particular, neither Stable `packaging/windows/AgentDock.iss` nor `scripts/install/install.ps1` is presented as Next. No Windows File/Product/Assembly VersionInfo or Authenticode acceptance is claimed for the Core `.exe`. macOS `.app`/DMG, deployment target/native GUI parity, full arm64 packaging acceptance, full installer metadata/hash sets, UIUX freeze and owner integration remain outside this checkpoint. Next updater-channel safety and P11 signing/native install/update/recovery need their separate accepted evidence.

The next closest user workflow is reviewing native Core CI evidence for an integrated immutable SHA, followed by the separately owned isolated Next packaging gate. Existing P10 contracts remain canonical in [p10-release-preflight.md](p10-release-preflight.md).
