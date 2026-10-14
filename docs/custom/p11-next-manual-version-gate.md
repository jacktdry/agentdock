# P11 — Next manual local deployment version gate

## Workflow and acceptance

The manual deployer must refuse a candidate that would downgrade the installed
Next Core before staging, swapping Contents, or changing service registration.
This advances P11's controlled local replacement safety. The smallest change is
a read-only preflight in `packaging/macos/deploy-next-shared.py`; the existing
installer, signer checks, ownership checks, backups and rollback remain intact.
Offline rejection and orchestration regressions are the acceptance for this
source change. Native install, upgrade, GUI and recovery UAT remain separate.

Owner-provided observations (2026-10-14; not re-probed by this source task):

- Installed `~/Applications/AgentDock Next.app`: Core `0.9.1`, commit
  `154f73249671`, healthy port 8767 PID 1362.
- P10 candidate `/Users/wei/sideProject/agentdock-p10-release-migration/dist/next-shared-arm64.UxHhPF/package/AgentDock Next.app`:
  Core `0.9.1-rc.1`, commit `cceadb0c`.
- Stable port 8765 PID 12127 is outside this task.

The candidate is a semantic version downgrade and must be rejected.

## Exact preflight contract

1. Resolve the account-database home and the fixed installed Next destination;
   reject symlinks, the same source/destination, and an absent installed App.
2. Run the existing actual Next shared bundle validation on both paths,
   including bundle/helper identities, arm64 and signatures, unchanged.
   Require each validator return path to equal its requested path.
3. Execute each verified bundle's exact `Contents/Helpers/agentdock` with only
   `version --json`, disconnected stdin, at most 4096 bytes of combined stdout
   and stderr and a five-second deadline per probe. Read at most one extra byte
   to detect overflow. No shell, server command or GUI is launched. Terminate
   and reap only the probe process if it has not exited when preflight fails.
4. Require successful exit and one complete UTF-8 JSON object. Reject empty,
   malformed, truncated, duplicate-key, trailing non-JSON and oversized output;
   reject execution failure and timeout. Require canonical `version`, a lower
   case hexadecimal Git `commit` of 7–40 characters, and exactly
   `platform: darwin/arm64`. Missing/unknown commit or platform fails closed.
   `build_date` and `go_version` are informational and do not grant identity.
   The CLI does not emit its executable path: path identity comes from the
   verified bundle and the exact non-symlink executable invoked.
5. Compare canonical SemVer precedence: unsigned major/minor/patch integers
   without leading zeros; valid dot-separated ASCII prerelease/build
   identifiers; no leading zeros in numeric prerelease identifiers. Numeric
   prerelease identifiers compare numerically, precede nonnumeric identifiers,
   and shorter equal-prefix prereleases precede longer ones. Releases precede
   higher versions but outrank their own prereleases. Thus
   `0.9.1-rc.1 < 0.9.1-rc.2 < 0.9.1-rc.10 < 0.9.1 < 0.9.2-rc.1`.
   Build metadata does not affect precedence. Reject `v` prefixes, whitespace,
   partial versions and noncanonical numeric identifiers.
6. Reject lower precedence. Equal precedence requires exact commit string
   equality; a different commit or merely matching abbreviation prefix is
   ambiguous and rejected, including versions differing only in build metadata.
   Equal precedence with identical commit allows reinstall. Higher precedence
   allows the manual local deployment to continue through the existing gates.
7. Only after this preflight may the deployer inspect jobs/live Next ownership
   and health, invoke the installer, rebind services or enter rollback. Gate
   failure occurs outside the mutation/rollback block.

The read-only version command returns before Core configuration/server startup
in `cmd/agentdock/main.go`. Core currently abbreviates commits to at most twelve
characters; this gate does not establish full immutable Git provenance from an
abbreviated commit, nor does equal metadata alone establish identical payloads.
Existing signature and payload checks retain their existing responsibilities.

## Verification and limits

Run `python3 -B scripts/test/test-next-shared-deploy.py` for bounded probe,
metadata, SemVer, pre-mutation rejection, permitted replacement/reinstall and
existing service/rollback regressions. Tests mock subprocesses, launchd and
installer operations, use only temporary fixtures and do not run installed or
candidate Core binaries. They are offline source evidence, not native UAT.

No paid Developer ID, notarization, automatic update execution, Windows UAT,
publication, install/deploy, GUI/Core lifecycle changes or user App changes are
part of this work. The certificate-bound Next automatic updater is unchanged;
allowing a higher ad-hoc version here authorizes no automatic updater bypass.
The low-level `install-next-shared.py` remains a content replacement primitive;
this gate protects the manual deploy orchestration entry point only.
