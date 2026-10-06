#!/bin/zsh
# Repository-only arm64 product build. Never installs or executes the product.
set -euo pipefail
ROOT_DIR="${0:A:h:h:h}"
[[ $# == 1 && "$(uname -sm)" == "Darwin arm64" ]] || {
  print -u2 -- "Usage: build-next-shared.sh /absolute/path/to/arm64/cloudflared"; exit 2
}
CLOUDFLARED_SOURCE="$1"
[[ "$CLOUDFLARED_SOURCE" == /* && -f "$CLOUDFLARED_SOURCE" && ! -L "$CLOUDFLARED_SOURCE" ]] || exit 1
[[ "$(file "$CLOUDFLARED_SOURCE")" == *"arm64"* ]] || exit 1
cd "$ROOT_DIR"
# Fixed output beneath the repository, canonical and fresh; no live destination override.
[[ ! -L "$ROOT_DIR/dist" ]] || exit 1
mkdir -p "$ROOT_DIR/dist"
BUILD_DIR="$(mktemp -d "$ROOT_DIR/dist/next-shared-arm64.XXXXXX")"
export GOCACHE="$BUILD_DIR/go-cache"
export TMPDIR="$BUILD_DIR/tmp"
export CLANG_MODULE_CACHE_PATH="$BUILD_DIR/clang-cache"
export SWIFT_MODULECACHE_PATH="$BUILD_DIR/swift-cache"
mkdir -p "$TMPDIR" "$BUILD_DIR/payload" "$BUILD_DIR/core/bin"
export npm_config_store_dir="${npm_config_store_dir:-$ROOT_DIR/dist/pnpm-store}"
VERSION="$(go run ./tools/release version)"
SOURCE_COMMIT="$(git rev-parse HEAD)"
SOURCE_DATE="$(git show -s --format=%cI HEAD)"
(
  cd desktop/shared-poc/frontend
  pnpm install --frozen-lockfile --store-dir "$npm_config_store_dir"
  VITE_AGENTDOCK_PRODUCT_NAME='AgentDock Next' pnpm build
)
(
  cd desktop/shared-poc
  CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
    MACOSX_DEPLOYMENT_TARGET=13.0 CGO_CFLAGS='-mmacosx-version-min=13.0' CGO_LDFLAGS='-mmacosx-version-min=13.0' \
    go build -mod=readonly -tags production -trimpath -buildvcs=false \
    -ldflags="-s -w -X 'main.productName=AgentDock Next' -X main.desktopVariant=next" \
    -o "$BUILD_DIR/AgentDock" .
)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=readonly -trimpath \
  -ldflags="-s -w -X github.com/uvwt/agentdock/internal/buildinfo.Commit=$SOURCE_COMMIT -X github.com/uvwt/agentdock/internal/buildinfo.BuildDate=$SOURCE_DATE" \
  -o "$BUILD_DIR/core/bin/agentdock" ./cmd/agentdock
python3 packaging/build-core-skill-bundle.py --output "$BUILD_DIR/core/share/agentdock/core-skills"
tar -C "$BUILD_DIR/core" -czf "$BUILD_DIR/payload/agentdock_darwin_arm64.tar.gz" bin/agentdock share/agentdock/core-skills
cp "$CLOUDFLARED_SOURCE" "$BUILD_DIR/payload/cloudflared_darwin_arm64"
(
  cd "$BUILD_DIR/payload"
  shasum -a 256 agentdock_darwin_arm64.tar.gz > agentdock_darwin_arm64.tar.gz.sha256
  shasum -a 256 cloudflared_darwin_arm64 > cloudflared_darwin_arm64.sha256
)
AGENTDOCK_MACOS_APP_VARIANT=next AGENTDOCK_MACOS_ARCHES=arm64 \
  AGENTDOCK_MACOS_MIN_VERSION=13.0 AGENTDOCK_MACOS_SHARED_EXECUTABLE="$BUILD_DIR/AgentDock" \
  AGENTDOCK_MACOS_APP_OUTPUT_DIR="$BUILD_DIR/package" \
  AGENTDOCK_MACOS_OFFLINE_PAYLOAD_DIR="$BUILD_DIR/payload" \
  AGENTDOCK_CODESIGN_IDENTITY=- AGENTDOCK_CODESIGN_KEYCHAIN='' \
  AGENTDOCK_CODESIGN_KEYCHAIN_PASSWORD='' AGENTDOCK_CODESIGN_TIMESTAMP=none \
  zsh packaging/macos/build-app.sh "$VERSION"
python3 scripts/test/test-next-shared-package.py "$BUILD_DIR/package/AgentDock Next.app"
print -- "Shared Desktop artifact: $BUILD_DIR/package/AgentDock Next.app"
