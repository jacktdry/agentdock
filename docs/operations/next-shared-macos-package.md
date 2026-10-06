# Next Shared Desktop macOS package

This repository-only arm64 build puts the Wails/Vue executable at
`AgentDock Next.app/Contents/MacOS/AgentDock`. The native AppKit builder and
sources remain available for native registration and fallback. This is a local
ad-hoc package, not a notarized release or an updater activation artifact.

## Build and inspect

On an arm64 Mac with Go, pnpm, Python 3 and Xcode command-line tools:

```sh
zsh packaging/macos/build-next-shared.sh /absolute/non-symlink/path/to/cloudflared
```

Supply a trusted arm64 cloudflared executable, for example a resolved Homebrew
Cellar path. The builder only copies it; it never runs cloudflared or reads its
configuration. Dependencies follow the committed Go modules and pnpm lockfile.
Core and arbiter are compiled from this checkout; core-skills come from this
repository. Source commit/date are embedded in Core. No installed AgentDock app
or runtime state supplies packaging inputs.

Every invocation uses a fresh `dist/next-shared-arm64.XXXXXX/` directory and prints
the final `package/AgentDock Next.app` path. The existing builder also emits ZIP
and DMG files. Their historical `macos-universal` names are retained for the
metadata contract, but **these artifacts contain arm64 only**. Do not publish
them as universal releases. A failed build directory is diagnostic output, not
an accepted package. Build caches and intermediate payloads stay under that
directory and may be removed after review.

The builder sets both Go product identity and frontend branding to AgentDock
Next, forces `AGENTDOCK_DESKTOP_VARIANT=next` before desktop services are created,
and isolates shell preferences under the Next runtime root. It accepts the
existing login helper's `--background` argument. The metadata and signing
identifiers come from `app-identity.sh`; helper paths and service labels match
the native Next package. The script writes a signed `Resources/desktop-product.json` marker for the Shared/Next identity, verifies signatures, and inspects Go build metadata without launching the app or any bundled helper.

```sh
python3 scripts/test/test-next-shared-package.py
python3 scripts/test/test-next-shared-package.py 'dist/next-shared-arm64.XXXXXX/package/AgentDock Next.app'
```

The first command runs temporary metadata/refusal fixtures. The second also
checks the actual bundle, Wails production build markers, arm64 Mach-O helpers,
signing identifiers, core-skills and strict recursive codesign verification.

## Later replacement, after main-session validation

Do not execute this during repository-only packaging work. The main session
must first validate the package and coordinate Next-only app/service quiescence
and recovery. Replacing a signed bundle can require native SMAppService approval
or re-registration; this helper does not perform either operation.

```sh
python3 packaging/macos/install-next-shared.py '/absolute/path/to/package/AgentDock Next.app'
```

The destination is fixed from the current account's home directory to
`~/Applications/AgentDock Next.app`; there is no destination flag. An existing
Next bundle is required. Stable/system destinations, symlinks, hardlinked bundle
files, wrong metadata/helper/signing identities and writable-by-others
destination directories are refused. The source must also prove it is the Next
Wails production build. To preserve the existing SMAppService parent-bundle
binding, the installer keeps the top-level `AgentDock Next.app` directory (and
its inode) in place and atomically exchanges only its signed `Contents`
directory with a validated private sibling copy using macOS `RENAME_SWAP`.
The previous Next `Contents` directory is retained at the printed backup path.
Post-swap signature/product validation failure automatically swaps the old
contents back. Coordinate writers externally; the helper is not a service
manager or a defense against a malicious process running as the same user.

This command does not launch, stop, restart, register, or unregister anything.
It does not modify runtime configuration, credentials, or service state. Its
installation entrypoint is not executed by the fixture suite.

A Shared package also contains the narrowly scoped native
`Contents/MacOS/AgentDockServiceRegistrar`. It is Next-only: the helper refuses
any bundle whose identifier / variant is not `dev.dropabit.agentdock.next` /
`next`. After replacing `Contents`, an already registered Core may still be
bound to the previous helper code requirement. The authorized live migration
step is therefore:

```sh
"$HOME/Applications/AgentDock Next.app/Contents/MacOS/AgentDockServiceRegistrar" reregister core
```

A successful call prints `enabled`; exit status 3 means macOS requires the user
to approve the background item. Do not run this against stable AgentDock. Tunnel
re-registration is separate and should only be used when the Next-owned Tunnel
was already intended to be enabled.

## Native and live gates

Shared Desktop still does not expose SMAppService registration or install/apply
updates through its normal Vue UI. Existing capability/unavailable responses
remain authoritative. The native AppKit fallback remains in the repository,
and the packaged registrar exists only as a bounded native migration adapter.
Successful build/signature checks do not by themselves establish service
registration continuity, GUI usability, or connector readiness.

Cloudflare provisioning is excluded. The later gate requires a dedicated Next
Named Tunnel, hostname, token and route to port 8767. Follow
[Next isolation](../custom/agentdock-next-isolation.md) and the
[Pre-M9 contract](../custom/pre-m9-wave0-contract.md).
