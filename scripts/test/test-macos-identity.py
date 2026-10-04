#!/usr/bin/env python3
"""Exercise actual package metadata generation using temporary output only."""
import os
from pathlib import Path
import plistlib
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
BUILD = ROOT / "packaging/macos/build-app.sh"
IDENTITY = ROOT / "packaging/macos/app-identity.sh"
with tempfile.TemporaryDirectory(prefix="agentdock-identity-") as temporary:
    for variant, name, bundle, prefix, state, port in [
        (None, "AgentDock", "com.uvwt.agentdock", "AgentDock", ".agentdock", "8765"),
        ("next", "AgentDock Next", "dev.dropabit.agentdock.next", "AgentDock-Next", ".agentdock-next", "8767"),
    ]:
        env = dict(os.environ)
        env.pop("AGENTDOCK_MACOS_APP_VARIANT", None)
        if variant:
            env["AGENTDOCK_MACOS_APP_VARIANT"] = variant
        output = Path(temporary) / (variant or "stable")
        subprocess.run(["zsh", str(BUILD), "--metadata-only", str(output)], env=env, check=True)
        contents = output / (name + ".app") / "Contents"
        with (contents / "Info.plist").open("rb") as stream:
            info = plistlib.load(stream)
        assert info["CFBundleIdentifier"] == bundle
        assert info["CFBundleName"] == info["CFBundleDisplayName"] == name
        assert info["CFBundleExecutable"] == "AgentDock"
        assert info["AgentDockVariant"] == (variant or "stable")
        agents = contents / "Library/LaunchAgents"
        assert {p.name for p in agents.iterdir()} == {bundle + "." + role + ".plist" for role in ("core", "tunnel", "menu-login")}
        for role, command in [("core", ["agentdock", "service", "launch-core"]),
                              ("tunnel", ["agentdock", "tunnel", "launch"]),
                              ("menu-login", ["AgentDockLoginHelper"])]:
            data = (agents / (bundle + "." + role + ".plist")).read_bytes()
            plist = plistlib.loads(data)
            assert plist["Label"] == bundle + "." + role
            assert plist["ProgramArguments"] == command
            assert plist["BundleProgram"] == "Contents/Helpers/" + command[0]
            assert b"/Users/" not in data
            if role != "menu-login":
                assert plist["EnvironmentVariables"] == {"AGENTDOCK_DESKTOP_VARIANT": variant or "stable"}
        variables = subprocess.check_output([
            "zsh", "-c", 'source "$1"; print -rl -- "$APP_BUNDLE_NAME" "$ZIP_NAME" "$DMG_NAME" "$LOGIN_SIGN_ID" "$CLOUDFLARED_SIGN_ID" "$ARBITER_SIGN_ID" "$STATE_NAME" "$APP_SUPPORT_NAME" "$LOG_NAME" "$WORK_NAME" "$DEFAULT_PORT"',
            "identity-test", str(IDENTITY)], env=env, text=True).splitlines()
        assert variables == [name + ".app", prefix + "-macos-universal.zip", prefix + "-macos-universal.dmg",
                             bundle + ".login-helper", bundle + ".cloudflared", bundle + ".arbiter",
                             state, name, name, name, port]
    build = BUILD.read_text()
    for statement in [
        'APP_DIR="$OUTPUT_DIR/$APP_BUNDLE_NAME"',
        'ZIP_PATH="$OUTPUT_DIR/$ZIP_NAME"', 'DMG_PATH="$OUTPUT_DIR/$DMG_NAME"',
        'sign_macos_code "$LOGIN_SIGN_ID" "$MENU_LOGIN_HELPER"',
        'sign_macos_code "$CORE_LABEL" "$HELPERS_DIR/agentdock"',
        'sign_macos_code "$CLOUDFLARED_SIGN_ID" "$HELPERS_DIR/cloudflared"',
        'sign_macos_code "$ARBITER_SIGN_ID" "$HELPERS_DIR/agentdock-arbiter"',
    ]:
        assert statement in build, statement
    env["AGENTDOCK_MACOS_APP_VARIANT"] = "unknown"
    invalid = Path(temporary) / "invalid"
    result = subprocess.run(["zsh", str(BUILD), "--metadata-only", str(invalid)], env=env, capture_output=True)
    assert result.returncode != 0 and not invalid.exists()
print("macOS stable/Next packaging identity contracts passed")
