#!/bin/zsh
set -euo pipefail

ROOT_DIR="${0:A:h:h:h}"
cd "$ROOT_DIR"

if [[ "${AGENTDOCK_ALLOW_DIRTY_FORK_BUILD:-0}" != "1" ]] && [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
  print -u2 -- "refusing to build a fork app from a dirty worktree"
  print -u2 -- "commit/stash changes first, or set AGENTDOCK_ALLOW_DIRTY_FORK_BUILD=1 for an explicitly non-reproducible build"
  exit 1
fi

case "$(uname -m)" in
  arm64|aarch64)
    goarch="arm64"
    app_arch="arm64"
    ;;
  x86_64|amd64)
    goarch="amd64"
    app_arch="x86_64"
    ;;
  *)
    print -u2 -- "unsupported macOS architecture: $(uname -m)"
    exit 2
    ;;
esac

distribution="${AGENTDOCK_FORK_DISTRIBUTION:-jacktdry/agentdock}"
update_policy="${AGENTDOCK_FORK_UPDATE_POLICY:-check-only}"
output_dir="${AGENTDOCK_FORK_OUTPUT_DIR:-$ROOT_DIR/dist/fork-macos-app}"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/agentdock-fork-build.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT

payload_root="$work_dir/payload-root"
payload_dir="$work_dir/payload"
mkdir -p "$payload_root/bin" "$payload_root/share/agentdock" "$payload_dir"

source_commit="$(git rev-parse HEAD)"
build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ldflags="-s -w -X github.com/uvwt/agentdock/internal/buildinfo.Commit=$source_commit -X github.com/uvwt/agentdock/internal/buildinfo.BuildDate=$build_date -X github.com/uvwt/agentdock/internal/buildinfo.Distribution=$distribution -X github.com/uvwt/agentdock/internal/buildinfo.UpdatePolicy=$update_policy"

print -- "==> build fork Core ($distribution, $update_policy)"
CGO_ENABLED=0 GOOS=darwin GOARCH="$goarch" \
  go build -trimpath -ldflags="$ldflags" -o "$payload_root/bin/agentdock" ./cmd/agentdock
python3 packaging/build-core-skill-bundle.py --output "$payload_root/share/agentdock/core-skills"

asset="agentdock_darwin_${goarch}.tar.gz"
tar -C "$payload_root" -czf "$payload_dir/$asset" bin/agentdock share/agentdock/core-skills
(
  cd "$payload_dir"
  shasum -a 256 "$asset" > "$asset.sha256"
)

cloudflared_source="${AGENTDOCK_CLOUDFLARED_BIN:-/Applications/AgentDock.app/Contents/Helpers/cloudflared}"
[[ -f "$cloudflared_source" && ! -L "$cloudflared_source" ]] || {
  print -u2 -- "cloudflared not found: $cloudflared_source"
  print -u2 -- "set AGENTDOCK_CLOUDFLARED_BIN to an existing trusted cloudflared binary"
  exit 1
}
cloudflared_asset="cloudflared_darwin_${goarch}"
cp -p "$cloudflared_source" "$payload_dir/$cloudflared_asset"
chmod 0755 "$payload_dir/$cloudflared_asset"
(
  cd "$payload_dir"
  shasum -a 256 "$cloudflared_asset" > "$cloudflared_asset.sha256"
)

rm -rf "$output_dir"
AGENTDOCK_MACOS_ARCHES="$app_arch" \
AGENTDOCK_MACOS_APP_OUTPUT_DIR="$output_dir" \
AGENTDOCK_MACOS_OFFLINE_PAYLOAD_DIR="$payload_dir" \
AGENTDOCK_DISTRIBUTION="$distribution" \
AGENTDOCK_UPDATE_POLICY="$update_policy" \
  packaging/macos/build-app.sh

app_path="$output_dir/AgentDock.app"
core_path="$app_path/Contents/Helpers/agentdock"
core_json="$("$core_path" version --json)"
python3 - "$distribution" "$update_policy" "$core_json" <<'PY'
import json
import sys

distribution, policy, raw = sys.argv[1:]
info = json.loads(raw)
if info.get("distribution") != distribution:
    raise SystemExit(f"fork Core distribution mismatch: {info.get('distribution')!r} != {distribution!r}")
if info.get("update_policy") != policy:
    raise SystemExit(f"fork Core update policy mismatch: {info.get('update_policy')!r} != {policy!r}")
PY

app_distribution="$(plutil -extract AgentDockDistribution raw -o - "$app_path/Contents/Info.plist")"
app_update_policy="$(plutil -extract AgentDockUpdatePolicy raw -o - "$app_path/Contents/Info.plist")"
[[ "$app_distribution" == "$distribution" ]] || {
  print -u2 -- "App distribution mismatch: $app_distribution"
  exit 1
}
[[ "$app_update_policy" == "$update_policy" ]] || {
  print -u2 -- "App update policy mismatch: $app_update_policy"
  exit 1
}

print -- ""
print -- "Fork build ready:"
print -- "  $app_path"
print -- "  distribution: $distribution"
print -- "  update policy: $update_policy"
