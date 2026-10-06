#!/bin/zsh
set -euo pipefail

ROOT_DIR="${0:A:h:h:h}"
SOURCE_DIR="$ROOT_DIR/desktop/macos/AgentDockApp/Sources"
LOCALIZATION_DIR="$ROOT_DIR/desktop/macos/AgentDockApp/Resources"
LOGIN_HELPER_SOURCE="$ROOT_DIR/desktop/macos/AgentDockLoginHelper/main.swift"
OUTPUT_DIR="${AGENTDOCK_MACOS_APP_OUTPUT_DIR:-$ROOT_DIR/dist/macos-app}"
ARCH_LIST="${AGENTDOCK_MACOS_ARCHES:-$(uname -m)}"
OFFLINE_PAYLOAD_DIR="${AGENTDOCK_MACOS_OFFLINE_PAYLOAD_DIR:-}"
MIN_VERSION="${AGENTDOCK_MACOS_MIN_VERSION:-13.0}"
# Fixed flavor contract; shared with the metadata-only fixture path.
source "$ROOT_DIR/packaging/macos/app-identity.sh"
# Only the Next product packager may supply a Shared Desktop entrypoint.
SHARED_EXECUTABLE="${AGENTDOCK_MACOS_SHARED_EXECUTABLE:-}"
if [[ -n "$SHARED_EXECUTABLE" ]]; then
  [[ "$APP_VARIANT" == next && "$ARCH_LIST" == arm64 && -f "$SHARED_EXECUTABLE" && ! -L "$SHARED_EXECUTABLE" ]] || {
    print -u2 -- "Shared executable requires Next arm64 and a regular file"; exit 1
  }
  [[ "$(file "$SHARED_EXECUTABLE")" == *"Mach-O 64-bit executable arm64"* ]] || exit 1
fi
if [[ "${1:-}" == "--metadata-only" ]]; then
  [[ $# == 2 ]] || { print -u2 -- "Usage: build-app.sh --metadata-only OUTPUT"; exit 1; }
  VERSION="0.0.0"
  write_app_metadata "$2/$APP_BUNDLE_NAME/Contents"
  exit 0
fi
APP_ICON_SOURCE="$ROOT_DIR/packaging/assets/agentdock.png"
CODESIGN_IDENTITY="${AGENTDOCK_CODESIGN_IDENTITY:-"-"}"
CODESIGN_KEYCHAIN="${AGENTDOCK_CODESIGN_KEYCHAIN:-}"
CODESIGN_KEYCHAIN_PASSWORD="${AGENTDOCK_CODESIGN_KEYCHAIN_PASSWORD:-}"
CODESIGN_TIMESTAMP="${AGENTDOCK_CODESIGN_TIMESTAMP:-none}"

usage() {
  cat <<'USAGE'
用法：
  packaging/macos/build-app.sh [版本]

环境变量：
  AGENTDOCK_MACOS_APP_VARIANT   stable（默认）或 next
  AGENTDOCK_MACOS_ARCHES        逗号分隔架构，默认当前架构；Release 使用 arm64,x86_64
  AGENTDOCK_MACOS_APP_OUTPUT_DIR 输出目录，默认 dist/macos-app
  AGENTDOCK_MACOS_OFFLINE_PAYLOAD_DIR
                               双架构离线载荷目录，构建 DMG 时必须提供
  AGENTDOCK_MACOS_MIN_VERSION   最低 macOS 版本，默认 13.0
  AGENTDOCK_CODESIGN_IDENTITY   代码签名身份；默认 -（ad-hoc）
  AGENTDOCK_CODESIGN_KEYCHAIN   可选，指定签名身份所在钥匙串
  AGENTDOCK_CODESIGN_KEYCHAIN_PASSWORD
                               可选，指定钥匙串解锁密码
  AGENTDOCK_CODESIGN_TIMESTAMP  none 或 auto；默认 none
USAGE
}

die() {
  print -u2 -- "ERROR: $*"
  exit 1
}

if (( $# > 1 )); then
  usage
  exit 2
fi

for command_name in codesign ditto file go hdiutil iconutil lipo plutil shasum sips swiftc unzip xcrun; do
  command -v "$command_name" >/dev/null 2>&1 || die "缺少命令：$command_name"
done
case "$CODESIGN_TIMESTAMP" in
  none|auto) ;;
  *) die "AGENTDOCK_CODESIGN_TIMESTAMP 只支持 none 或 auto" ;;
esac
if [[ "$CODESIGN_IDENTITY" != "-" ]]; then
  command -v security >/dev/null 2>&1 || die "缺少命令：security"
  if [[ -n "$CODESIGN_KEYCHAIN" ]]; then
    [[ -f "$CODESIGN_KEYCHAIN" && ! -L "$CODESIGN_KEYCHAIN" ]] || die "代码签名钥匙串不存在或不是普通文件：$CODESIGN_KEYCHAIN"
    security unlock-keychain -p "$CODESIGN_KEYCHAIN_PASSWORD" "$CODESIGN_KEYCHAIN" >/dev/null 2>&1 || \
      die "无法解锁代码签名钥匙串：$CODESIGN_KEYCHAIN"
    identity_output="$(security find-identity -v -p codesigning "$CODESIGN_KEYCHAIN")" || die "无法读取指定钥匙串中的签名身份"
  else
    identity_output="$(security find-identity -v -p codesigning)" || die "无法读取系统钥匙串中的签名身份"
  fi
  [[ "$identity_output" == *"$CODESIGN_IDENTITY"* ]] || die "找不到代码签名身份：$CODESIGN_IDENTITY"
elif [[ -n "$CODESIGN_KEYCHAIN" ]]; then
  die "设置 AGENTDOCK_CODESIGN_KEYCHAIN 时必须同时设置 AGENTDOCK_CODESIGN_IDENTITY"
fi
[[ -d "$SOURCE_DIR" ]] || die "缺少 macOS App 源码：$SOURCE_DIR"
[[ -d "$LOCALIZATION_DIR" ]] || die "缺少 macOS App 本地化资源：$LOCALIZATION_DIR"
[[ -f "$APP_ICON_SOURCE" && ! -L "$APP_ICON_SOURCE" ]] || die "缺少 macOS App 图标：$APP_ICON_SOURCE"
[[ -f "$LOGIN_HELPER_SOURCE" && ! -L "$LOGIN_HELPER_SOURCE" ]] || die "缺少 macOS 登录代理源码：$LOGIN_HELPER_SOURCE"
[[ -n "$OFFLINE_PAYLOAD_DIR" ]] || die "构建 macOS DMG 必须设置 AGENTDOCK_MACOS_OFFLINE_PAYLOAD_DIR"
[[ -d "$OFFLINE_PAYLOAD_DIR" && ! -L "$OFFLINE_PAYLOAD_DIR" ]] || die "离线载荷目录无效：$OFFLINE_PAYLOAD_DIR"

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(go run "$ROOT_DIR/tools/release" version)"
fi
[[ "$VERSION" == <->.<->.<->* ]] || die "无法解析 App 版本：$VERSION"

SDK_PATH="$(xcrun --sdk macosx --show-sdk-path)"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/agentdock-macos-app.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$OUTPUT_DIR"
rm -rf \
  "$OUTPUT_DIR/$APP_BUNDLE_NAME" \
  "$OUTPUT_DIR/$DMG_NAME" \
  "$OUTPUT_DIR/${DMG_NAME}.sha256" \
  "$OUTPUT_DIR/$ZIP_NAME" \
  "$OUTPUT_DIR/${ZIP_NAME}.sha256"

IFS=',' read -rA architectures <<< "$ARCH_LIST"
(( ${#architectures[@]} > 0 )) || die "没有可构建的架构"
compiled_binaries=()
login_helper_binaries=()
release_architectures=()
for architecture in "${architectures[@]}"; do
  architecture="${architecture//[[:space:]]/}"
  case "$architecture" in
    arm64) release_architecture="arm64" ;;
    x86_64) release_architecture="amd64" ;;
    *) die "不支持的 App 架构：$architecture" ;;
  esac
  release_architectures+=("$release_architecture")
  binary="$TMP_DIR/AgentDock-$architecture"
  print -- "==> 编译 AgentDock.app：$architecture"
  if [[ -n "$SHARED_EXECUTABLE" ]]; then
    cp "$SHARED_EXECUTABLE" "$binary"
  else
    xcrun swiftc \
    -swift-version 5 \
    -O \
    -whole-module-optimization \
    -target "$architecture-apple-macosx$MIN_VERSION" \
    -sdk "$SDK_PATH" \
    "$SOURCE_DIR"/*.swift \
    -o "$binary"
  fi
  compiled_binaries+=("$binary")

  login_helper="$TMP_DIR/AgentDockLoginHelper-$architecture"
  xcrun swiftc \
    -swift-version 5 \
    -O \
    -whole-module-optimization \
    -target "$architecture-apple-macosx$MIN_VERSION" \
    -sdk "$SDK_PATH" \
    "$SOURCE_DIR/AppIdentity.swift" \
    "$LOGIN_HELPER_SOURCE" \
    -o "$login_helper"
  login_helper_binaries+=("$login_helper")
done

APP_DIR="$OUTPUT_DIR/$APP_BUNDLE_NAME"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
HELPERS_DIR="$CONTENTS_DIR/Helpers"
LAUNCH_AGENTS_DIR="$CONTENTS_DIR/Library/LaunchAgents"
MENU_LOGIN_HELPER="$HELPERS_DIR/AgentDockLoginHelper"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR" "$HELPERS_DIR" "$LAUNCH_AGENTS_DIR"
localization_count=0
while IFS= read -r -d '' source_lproj; do
  localization="$(basename "$source_lproj" .lproj)"
  [[ -d "$source_lproj" && ! -L "$source_lproj" ]] || die "無效的 macOS 本地化目錄：$source_lproj"
  ditto "$source_lproj" "$RESOURCES_DIR/$localization.lproj"
  ((localization_count += 1))
done < <(find "$LOCALIZATION_DIR" -mindepth 1 -maxdepth 1 -type d -name '*.lproj' -print0 | sort -z)
(( localization_count > 0 )) || die "找不到 macOS 本地化目錄：$LOCALIZATION_DIR"

if (( ${#compiled_binaries[@]} == 1 )); then
  cp -p "$compiled_binaries[1]" "$MACOS_DIR/AgentDock"
else
  lipo -create "${compiled_binaries[@]}" -output "$MACOS_DIR/AgentDock"
fi
chmod 0755 "$MACOS_DIR/AgentDock"

if (( ${#login_helper_binaries[@]} == 1 )); then
  cp -p "$login_helper_binaries[1]" "$MENU_LOGIN_HELPER"
else
  lipo -create "${login_helper_binaries[@]}" -output "$MENU_LOGIN_HELPER"
fi
chmod 0755 "$MENU_LOGIN_HELPER"

ICONSET_DIR="$TMP_DIR/AgentDock.iconset"
mkdir -p "$ICONSET_DIR"
icon_specs=(
  'icon_16x16.png:16'
  'icon_16x16@2x.png:32'
  'icon_32x32.png:32'
  'icon_32x32@2x.png:64'
  'icon_128x128.png:128'
  'icon_128x128@2x.png:256'
  'icon_256x256.png:256'
  'icon_256x256@2x.png:512'
  'icon_512x512.png:512'
  'icon_512x512@2x.png:1024'
)
for icon_spec in "${icon_specs[@]}"; do
  icon_name="${icon_spec%%:*}"
  icon_size="${icon_spec##*:}"
  sips -z "$icon_size" "$icon_size" "$APP_ICON_SOURCE" --out "$ICONSET_DIR/$icon_name" >/dev/null
done
iconutil -c icns "$ICONSET_DIR" -o "$RESOURCES_DIR/AgentDock.icns"
[[ -f "$RESOURCES_DIR/AgentDock.icns" && ! -L "$RESOURCES_DIR/AgentDock.icns" ]] || \
  die "macOS App 图标生成失败"

# 桌面版把 Core、cloudflared 和官方核心 Skill 直接收进 App Bundle。
# 版本更新因此只替换一个 AgentDock.app，不再维护 ~/.local/bin 的第二套生产文件。
helper_core_binaries=()
helper_cloudflared_binaries=()
helper_arbiter_binaries=()
CORE_SKILL_BUNDLE="$RESOURCES_DIR/core-skills"
for release_architecture in "${release_architectures[@]}"; do
  agentdock_archive="agentdock_darwin_${release_architecture}.tar.gz"
  agentdock_checksum="$agentdock_archive.sha256"
  cloudflared_binary="cloudflared_darwin_${release_architecture}"
  cloudflared_checksum="$cloudflared_binary.sha256"

  for payload_name in "$agentdock_archive" "$agentdock_checksum" "$cloudflared_binary" "$cloudflared_checksum"; do
    payload_path="$OFFLINE_PAYLOAD_DIR/$payload_name"
    [[ -f "$payload_path" && ! -L "$payload_path" ]] || die "缺少离线载荷：$payload_path"
  done

  (
    cd "$OFFLINE_PAYLOAD_DIR"
    shasum -a 256 -c "$agentdock_checksum"
    shasum -a 256 -c "$cloudflared_checksum"
  )

  payload_check_dir="$TMP_DIR/payload-check-$release_architecture"
  mkdir -p "$payload_check_dir"
  tar -xzf "$OFFLINE_PAYLOAD_DIR/$agentdock_archive" -C "$payload_check_dir"
  [[ -f "$payload_check_dir/bin/agentdock" && ! -L "$payload_check_dir/bin/agentdock" ]] || die "$agentdock_archive 缺少 bin/agentdock"
  [[ -f "$payload_check_dir/share/agentdock/core-skills/manifest.json" && ! -L "$payload_check_dir/share/agentdock/core-skills/manifest.json" ]] || \
    die "$agentdock_archive 缺少核心 Skill Bundle"

  case "$release_architecture" in
    arm64) expected_file_architecture="arm64" ;;
    amd64) expected_file_architecture="x86_64" ;;
  esac
  # pipefail 下避免让早退的匹配命令截断 file 输出，否则上游可能收到 SIGPIPE(141)。
  agentdock_file_output="$(file "$payload_check_dir/bin/agentdock")"
  [[ "$agentdock_file_output" == *"$expected_file_architecture"* ]] || \
    die "$agentdock_archive 架构不匹配，期望 $expected_file_architecture"
  cloudflared_file_output="$(file "$OFFLINE_PAYLOAD_DIR/$cloudflared_binary")"
  [[ "$cloudflared_file_output" == *"$expected_file_architecture"* ]] || \
    die "$cloudflared_binary 架构不匹配，期望 $expected_file_architecture"

  helper_core_binaries+=("$payload_check_dir/bin/agentdock")
  helper_cloudflared_binaries+=("$OFFLINE_PAYLOAD_DIR/$cloudflared_binary")
  arbiter_binary="$TMP_DIR/agentdock-arbiter-$release_architecture"
  (
    cd "$ROOT_DIR"
    CGO_ENABLED=0 GOOS=darwin GOARCH="$release_architecture" \
      go build -trimpath -ldflags '-s -w' -o "$arbiter_binary" ./cmd/agentdock-arbiter
  )
  arbiter_file_output="$(file "$arbiter_binary")"
  [[ "$arbiter_file_output" == *"$expected_file_architecture"* ]] || \
    die "agentdock-arbiter 架构不匹配，期望 $expected_file_architecture"
  helper_arbiter_binaries+=("$arbiter_binary")
  if [[ ! -d "$CORE_SKILL_BUNDLE" ]]; then
    ditto "$payload_check_dir/share/agentdock/core-skills" "$CORE_SKILL_BUNDLE"
  elif ! diff -qr "$payload_check_dir/share/agentdock/core-skills" "$CORE_SKILL_BUNDLE" >/dev/null; then
    die "不同架构 Release 中的核心 Skill Bundle 不一致"
  fi
done

if (( ${#helper_core_binaries[@]} == 1 )); then
  cp -p "$helper_core_binaries[1]" "$HELPERS_DIR/agentdock"
  cp -p "$helper_cloudflared_binaries[1]" "$HELPERS_DIR/cloudflared"
  cp -p "$helper_arbiter_binaries[1]" "$HELPERS_DIR/agentdock-arbiter"
else
  lipo -create "${helper_core_binaries[@]}" -output "$HELPERS_DIR/agentdock"
  lipo -create "${helper_cloudflared_binaries[@]}" -output "$HELPERS_DIR/cloudflared"
  lipo -create "${helper_arbiter_binaries[@]}" -output "$HELPERS_DIR/agentdock-arbiter"
fi
chmod 0755 "$HELPERS_DIR/agentdock" "$HELPERS_DIR/cloudflared" "$HELPERS_DIR/agentdock-arbiter"
find "$CORE_SKILL_BUNDLE" -type d -exec chmod 0755 {} +
find "$CORE_SKILL_BUNDLE" -type f -exec chmod 0644 {} +
[[ -f "$CORE_SKILL_BUNDLE/manifest.json" && ! -L "$CORE_SKILL_BUNDLE/manifest.json" ]] || \
  die "App Bundle 缺少核心 Skill manifest"

write_app_metadata "$CONTENTS_DIR"

if [[ -n "$SHARED_EXECUTABLE" ]]; then
  cat > "$RESOURCES_DIR/desktop-product.json" <<JSON
{"schema_version":1,"product_name":"$APP_NAME","desktop_variant":"$APP_VARIANT","ui":"shared-wails"}
JSON
  chmod 0644 "$RESOURCES_DIR/desktop-product.json"
fi

sign_macos_code() {
  local identifier="$1"
  local target="$2"
  local args=(--force --sign "$CODESIGN_IDENTITY" --identifier "$identifier")
  if [[ "$CODESIGN_IDENTITY" != "-" ]]; then
    args+=(--options runtime)
    if [[ "$CODESIGN_TIMESTAMP" == "auto" ]]; then
      args+=(--timestamp)
    else
      args+=(--timestamp=none)
    fi
    if [[ -n "$CODESIGN_KEYCHAIN" ]]; then
      args+=(--keychain "$CODESIGN_KEYCHAIN")
    fi
  fi
  codesign "${args[@]}" "$target"
}

if [[ "$CODESIGN_IDENTITY" == "-" ]]; then
  print -- "==> ad-hoc 签名 AgentDock.app"
else
  print -- "==> 使用指定身份签名 AgentDock.app"
fi
sign_macos_code "$LOGIN_SIGN_ID" "$MENU_LOGIN_HELPER"
sign_macos_code "$CORE_LABEL" "$HELPERS_DIR/agentdock"
sign_macos_code "$CLOUDFLARED_SIGN_ID" "$HELPERS_DIR/cloudflared"
sign_macos_code "$ARBITER_SIGN_ID" "$HELPERS_DIR/agentdock-arbiter"
# 嵌套代码先分别签名，再签外层 App。不要用 --deep 做签名操作，否则会重新签
# Core/cloudflared 并破坏它们的稳定代码身份；--deep 只用于最终递归验证。
sign_macos_code "$BUNDLE_ID" "$APP_DIR"
codesign --verify --deep --strict --verbose=2 "$APP_DIR"

ZIP_PATH="$OUTPUT_DIR/$ZIP_NAME"
print -- "==> 创建 AgentDock App 更新 ZIP"
ditto -c -k --keepParent "$APP_DIR" "$ZIP_PATH"
unzip -tq "$ZIP_PATH" >/dev/null
(
  cd "$OUTPUT_DIR"
  shasum -a 256 "${ZIP_PATH:t}" > "${ZIP_PATH:t}.sha256"
)

DMG_STAGE_DIR="$TMP_DIR/dmg-root"
DMG_PATH="$OUTPUT_DIR/$DMG_NAME"
mkdir -p "$DMG_STAGE_DIR"
ditto "$APP_DIR" "$DMG_STAGE_DIR/$APP_BUNDLE_NAME"
ln -s /Applications "$DMG_STAGE_DIR/Applications"

print -- "==> 创建 AgentDock DMG"
hdiutil create \
  -volname "$APP_NAME" \
  -srcfolder "$DMG_STAGE_DIR" \
  -ov \
  -format UDZO \
  "$DMG_PATH" >/dev/null
hdiutil verify "$DMG_PATH" >/dev/null
(
  cd "$OUTPUT_DIR"
  shasum -a 256 "${DMG_PATH:t}" > "${DMG_PATH:t}.sha256"
)

print -- "built: $APP_DIR"
print -- "zip: $ZIP_PATH"
print -- "dmg: $DMG_PATH"
file "$MACOS_DIR/AgentDock"
file "$HELPERS_DIR/agentdock"
file "$HELPERS_DIR/cloudflared"
