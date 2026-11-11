#!/usr/bin/env bash
# NEXT-only source-to-artifact gate. Never signs for distribution or installs.
set -euo pipefail

[[ "$(uname -sm)" == "Darwin arm64" ]] || { echo "arm64 macOS runner required" >&2; exit 1; }
for required in RUNNER_TEMP GITHUB_WORKSPACE NEXT_VERSION BUNDLE_BUILD SOURCE_SHA CLOUDFLARED_VERSION CLOUDFLARED_ARCHIVE_SHA256; do
  [[ -n "$(printenv "$required" || true)" ]] || { echo "missing $required" >&2; exit 1; }
done
[[ "$SOURCE_SHA" =~ ^[0-9a-f]{40}$ && "$SOURCE_SHA" != 0000000000000000000000000000000000000000 ]] || exit 1
[[ "$NEXT_VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.([1-9][0-9]*)$ ]] || exit 1
[[ "$BUNDLE_BUILD" =~ ^[1-9][0-9]{0,3}$ ]] || exit 1
[[ "$CLOUDFLARED_VERSION" =~ ^[0-9]{4}\.[0-9]{1,2}\.[0-9]+$ ]] || exit 1
[[ "$CLOUDFLARED_ARCHIVE_SHA256" =~ ^[0-9a-f]{64}$ ]] || exit 1
[[ "$(git rev-parse --show-toplevel)" == "$GITHUB_WORKSPACE" ]]
[[ "$(git rev-parse HEAD)" == "$SOURCE_SHA" ]] || { echo "source SHA mismatch" >&2; exit 1; }
[[ -z "$(git status --porcelain --untracked-files=all)" ]] || { echo "dirty source checkout" >&2; exit 1; }

stamp="$(git show -s --format=%ct HEAD)"
build_date="$(date -u -r "$stamp" +%Y-%m-%dT%H:%M:%SZ)"
manifest="$RUNNER_TEMP/next-candidate.json"
go run -mod=readonly ./tools/release next-preflight \
  --version "$NEXT_VERSION" --tag "next-v$NEXT_VERSION" --commit "$SOURCE_SHA" \
  --build-date "$build_date" --bundle-build "$BUNDLE_BUILD" > "$manifest"
go run -mod=readonly ./tools/release next-preflight \
  --version "$NEXT_VERSION" --tag "next-v$NEXT_VERSION" --commit "$SOURCE_SHA" \
  --build-date "$build_date" --bundle-build "$BUNDLE_BUILD" --verify "$manifest" >/dev/null
go run -mod=readonly ./tools/release next-candidate \
  --manifest "$manifest" --check-output "$GITHUB_WORKSPACE/dist/next-candidate-check" >/dev/null
python3 - "$manifest" <<'PY'
import json, pathlib, sys
m = json.loads(pathlib.Path(sys.argv[1]).read_bytes())
assert m['channel'] == 'next-rc' and m['macos_bundle_id'] == 'dev.dropabit.agentdock.next'
assert m['release_ready'] is False and m['update_feed'] == 'disabled/unpublished' and not m['update_url']
PY

# Local workflow smoke: checks full commit, canonical manifest, and fail-closed
# identity with no cloud download/package. The hosted workflow never sets it.
if [[ "${NEXT_CANDIDATE_VERIFY_ONLY:-0}" == 1 ]]; then
  echo "PASS: exact source, canonical NEXT manifest, and no-publication contract"
  exit 0
fi

go test -mod=readonly -count=1 -timeout=3m ./tools/release ./internal/updateidentity
go test -mod=readonly -race -count=10 -timeout=3m \
  -run '^Test(BlockedWriteHonorsRequestContext|QueuedWriteTimeoutPreservesActiveWrite|AlreadyCancelledWriteDoesNotTouchHealthyConnection|WriteFailureClosesFramingAndKeepsClassification|CloseReleasesBlockedAndQueuedWrites|CancelledRequestDoesNotWaitForCancelNotification|BlockedNotifyHonorsWriteDeadline)$' ./internal/acp
python3 scripts/test/test-next-windows-packaging.py -v
pnpm --dir desktop/shared-poc/frontend install --frozen-lockfile
pnpm --dir desktop/shared-poc/frontend typecheck
pnpm --dir desktop/shared-poc/frontend test

archive="$RUNNER_TEMP/cloudflared-darwin-arm64.tgz"
curl -fL --retry 3 --proto '=https' --tlsv1.2 \
  "https://github.com/cloudflare/cloudflared/releases/download/$CLOUDFLARED_VERSION/cloudflared-darwin-arm64.tgz" -o "$archive"
printf '%s  %s\n' "$CLOUDFLARED_ARCHIVE_SHA256" "$archive" | shasum -a 256 -c -
# Do not extract arbitrary paths; reject symlinks, additions and traversal.
python3 - "$archive" "$RUNNER_TEMP/cloudflared-arm64" <<'PY'
import pathlib, shutil, sys, tarfile
source, target = sys.argv[1:]
with tarfile.open(source, 'r:gz') as archive:
    members = archive.getmembers()
    if len(members) != 1 or members[0].name not in ('cloudflared', './cloudflared') or not members[0].isfile():
        raise SystemExit('Unexpected cloudflared archive layout')
    with archive.extractfile(members[0]) as src, open(target, 'xb') as dst:
        shutil.copyfileobj(src, dst)
pathlib.Path(target).chmod(0o755)
PY
[[ "$(file "$RUNNER_TEMP/cloudflared-arm64")" == *arm64* ]]
codesign --verify --deep --strict "$RUNNER_TEMP/cloudflared-arm64"

zsh packaging/macos/build-next-shared.sh "$RUNNER_TEMP/cloudflared-arm64" --candidate-manifest "$manifest"

# build-next-shared.sh produces a fresh, ignored dist directory, not ~/Applications.
package_count="$(find "$GITHUB_WORKSPACE/dist" -maxdepth 2 -type d -path '*/next-shared-arm64.*/package' | wc -l | tr -d ' ')"
[[ "$package_count" == 1 ]] || { echo "Expected exactly one fresh NEXT package" >&2; exit 1; }
package="$(find "$GITHUB_WORKSPACE/dist" -maxdepth 2 -type d -path '*/next-shared-arm64.*/package' -print -quit)"
app="$package/AgentDock Next.app"
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$app/Contents/Info.plist")" == dev.dropabit.agentdock.next ]]
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$app/Contents/Info.plist")" == "$BUNDLE_BUILD.0.0" ]]
codesign --verify --deep --strict "$app"
(cd "$package" && shasum -a 256 -c AgentDock-Next-macos-arm64.zip.sha256 AgentDock-Next-macos-arm64.dmg.sha256)
unzip -tqq "$package/AgentDock-Next-macos-arm64.zip"
hdiutil verify "$package/AgentDock-Next-macos-arm64.dmg"

python3 - "$app/Contents/Helpers/agentdock" "$manifest" <<'PY'
import json, pathlib, subprocess, sys
binary, path = sys.argv[1:]
manifest = json.loads(pathlib.Path(path).read_bytes())
info = json.loads(subprocess.check_output([binary, 'version', '--json']))
for key, wanted in [('version', manifest['version']), ('commit', manifest['commit'][:12]),
                    ('build_date', manifest['build_date']), ('platform', 'darwin/arm64')]:
    if info.get(key) != wanted:
        raise SystemExit(f'NEXT Core identity mismatch: {key}')
PY

inventory="$RUNNER_TEMP/next-inventory.json"
go run -mod=readonly ./tools/release next-artifacts \
  --manifest "$manifest" --root "$package" --platform darwin/arm64 \
  AgentDock-Next-macos-arm64.zip AgentDock-Next-macos-arm64.dmg > "$inventory"
go run -mod=readonly ./tools/release next-artifacts \
  --manifest "$manifest" --root "$package" --platform darwin/arm64 \
  --verify "$inventory" AgentDock-Next-macos-arm64.zip AgentDock-Next-macos-arm64.dmg >/dev/null

for ext in zip dmg; do
  cp "$package/AgentDock-Next-macos-arm64.$ext" "$RUNNER_TEMP/"
  cp "$package/AgentDock-Next-macos-arm64.$ext.sha256" "$RUNNER_TEMP/"
done
printf 'source_sha=%s\nrelease_ready=false\nupdate_feed=disabled/unpublished\nsignature=ad-hoc (not Developer ID)\n' "$SOURCE_SHA" > "$RUNNER_TEMP/next-status.txt"
[[ -z "$(git status --porcelain --untracked-files=all)" ]]
echo 'NEXT-only unpublished, ad-hoc candidate verified. No installation or release.'
