#!/bin/zsh
# Source only from build-app.sh. All persisted identity comes from this closed set.
APP_VARIANT="${AGENTDOCK_MACOS_APP_VARIANT:-stable}"
case "$APP_VARIANT" in
  stable)
    APP_NAME="AgentDock"
    BUNDLE_ID="com.uvwt.agentdock"
    ARTIFACT_PREFIX="AgentDock"
    STATE_NAME=".agentdock"
    DEFAULT_PORT=8765
    ;;
  next)
    APP_NAME="AgentDock Next"
    BUNDLE_ID="dev.dropabit.agentdock.next"
    ARTIFACT_PREFIX="AgentDock-Next"
    STATE_NAME=".agentdock-next"
    DEFAULT_PORT=8767
    ;;
  *) print -u2 -- "Unknown macOS app variant: $APP_VARIANT"; exit 1 ;;
esac
APP_BUNDLE_NAME="$APP_NAME.app"
APP_SUPPORT_NAME="$APP_NAME"
LOG_NAME="$APP_NAME"
WORK_NAME="$APP_NAME"
CORE_LABEL="$BUNDLE_ID.core"
TUNNEL_LABEL="$BUNDLE_ID.tunnel"
MENU_LABEL="$BUNDLE_ID.menu-login"
LOGIN_SIGN_ID="$BUNDLE_ID.login-helper"
CLOUDFLARED_SIGN_ID="$BUNDLE_ID.cloudflared"
ARBITER_SIGN_ID="$BUNDLE_ID.arbiter"
ZIP_NAME="$ARTIFACT_PREFIX-macos-universal.zip"
DMG_NAME="$ARTIFACT_PREFIX-macos-universal.dmg"

write_app_metadata() {
  local CONTENTS_DIR="$1"
  local LAUNCH_AGENTS_DIR="$CONTENTS_DIR/Library/LaunchAgents"
  mkdir -p "$LAUNCH_AGENTS_DIR"
cat > "$LAUNCH_AGENTS_DIR/${CORE_LABEL}.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${CORE_LABEL}</string>
  <key>BundleProgram</key>
  <string>Contents/Helpers/agentdock</string>
  <key>ProgramArguments</key>
  <array>
    <string>agentdock</string>
    <string>service</string>
    <string>launch-core</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>EnvironmentVariables</key>
  <dict>
    <key>AGENTDOCK_DESKTOP_VARIANT</key>
    <string>$APP_VARIANT</string>
  </dict>
  <key>ProcessType</key>
  <string>Background</string>
  <key>ThrottleInterval</key>
  <integer>5</integer>
</dict>
</plist>
PLIST

cat > "$LAUNCH_AGENTS_DIR/${TUNNEL_LABEL}.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${TUNNEL_LABEL}</string>
  <key>BundleProgram</key>
  <string>Contents/Helpers/agentdock</string>
  <key>ProgramArguments</key>
  <array>
    <string>agentdock</string>
    <string>tunnel</string>
    <string>launch</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>EnvironmentVariables</key>
  <dict>
    <key>AGENTDOCK_DESKTOP_VARIANT</key>
    <string>$APP_VARIANT</string>
  </dict>
  <key>ProcessType</key>
  <string>Background</string>
  <key>ThrottleInterval</key>
  <integer>5</integer>
</dict>
</plist>
PLIST

cat > "$LAUNCH_AGENTS_DIR/${MENU_LABEL}.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${MENU_LABEL}</string>
  <key>BundleProgram</key>
  <string>Contents/Helpers/AgentDockLoginHelper</string>
  <key>ProgramArguments</key>
  <array>
    <string>AgentDockLoginHelper</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>LimitLoadToSessionType</key>
  <string>Aqua</string>
</dict>
</plist>
PLIST
plutil -lint "$LAUNCH_AGENTS_DIR/${CORE_LABEL}.plist" >/dev/null
plutil -lint "$LAUNCH_AGENTS_DIR/${TUNNEL_LABEL}.plist" >/dev/null
plutil -lint "$LAUNCH_AGENTS_DIR/${MENU_LABEL}.plist" >/dev/null

cat > "$CONTENTS_DIR/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>$APP_NAME</string>
  <key>CFBundleExecutable</key>
  <string>AgentDock</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleIconFile</key>
  <string>AgentDock.icns</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>$APP_NAME</string>
  <key>AgentDockVariant</key>
  <string>$APP_VARIANT</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$VERSION</string>
  <key>CFBundleVersion</key>
  <string>$VERSION</string>
  <key>LSMinimumSystemVersion</key>
  <string>$MIN_VERSION</string>
  <key>LSUIElement</key>
  <true/>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>NSAppleEventsUsageDescription</key>
  <string>AgentDock needs to control System Events and Finder to perform desktop automation tasks you request.</string>
  <key>NSHumanReadableCopyright</key>
  <string>Copyright © AgentDock contributors</string>
</dict>
</plist>
PLIST
plutil -lint "$CONTENTS_DIR/Info.plist" >/dev/null

}
