"""Read-only validation shared by the Next package check and explicit installer."""
import json
import os
from pathlib import Path
import plistlib
import stat
import subprocess

NAME = "AgentDock Next"
BUNDLE_ID = "dev.dropabit.agentdock.next"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def no_symlinks(path):
    path = Path(os.path.abspath(path))
    for item in (path, *path.parents):
        require(not item.is_symlink(), f"Symlink refused: {item}")
    return path


def validate_bundle(path, *, shared=False, signatures=True):
    path = no_symlinks(path)
    require(path.is_dir(), "Bundle must be a directory")
    for item in path.rglob("*"):
        mode = item.lstat().st_mode
        require(stat.S_ISREG(mode) or stat.S_ISDIR(mode), f"Non-regular bundle entry: {item}")
        if stat.S_ISREG(mode):
            require(item.stat().st_nlink == 1, f"Hardlink refused: {item}")
    contents = path / "Contents"
    with (contents / "Info.plist").open("rb") as f:
        info = plistlib.load(f)
    for key, expected in {
        "CFBundleName": NAME, "CFBundleDisplayName": NAME,
        "CFBundleIdentifier": BUNDLE_ID, "AgentDockVariant": "next",
        "CFBundleExecutable": "AgentDock", "CFBundlePackageType": "APPL",
    }.items():
        require(info.get(key) == expected, f"Invalid {key}")
    agents = contents / "Library/LaunchAgents"
    require({p.name for p in agents.iterdir()} == {
        f"{BUNDLE_ID}.{suffix}.plist" for suffix in ("core", "tunnel", "menu-login")
    }, "Unexpected LaunchAgents")
    for suffix, helper, args in (
        ("core", "agentdock", ["agentdock", "service", "launch-core"]),
        ("tunnel", "agentdock", ["agentdock", "tunnel", "launch"]),
        ("menu-login", "AgentDockLoginHelper", ["AgentDockLoginHelper"]),
    ):
        with (agents / f"{BUNDLE_ID}.{suffix}.plist").open("rb") as f:
            agent = plistlib.load(f)
        require(agent.get("Label") == f"{BUNDLE_ID}.{suffix}", "Wrong service label")
        require(agent.get("BundleProgram") == f"Contents/Helpers/{helper}", "Wrong helper path")
        require(agent.get("ProgramArguments") == args, "Wrong helper arguments")
        require("Program" not in agent, "Absolute service program refused")
        if suffix != "menu-login":
            require(agent.get("EnvironmentVariables") == {"AGENTDOCK_DESKTOP_VARIANT": "next"}, "Wrong service environment")
        text = repr(agent)
        require("com.uvwt.agentdock" not in text and "AgentDock.app" not in text, "Stable identity in service")
    require((contents / "Resources/core-skills/manifest.json").is_file(), "Missing core skills")
    targets = {
        "MacOS/AgentDock": BUNDLE_ID,
        "Helpers/agentdock": BUNDLE_ID + ".core",
        "Helpers/cloudflared": BUNDLE_ID + ".cloudflared",
        "Helpers/agentdock-arbiter": BUNDLE_ID + ".arbiter",
        "Helpers/AgentDockLoginHelper": BUNDLE_ID + ".login-helper",
    }
    if shared:
        targets["MacOS/AgentDockServiceRegistrar"] = BUNDLE_ID + ".service-registrar"
    for relative, identifier in targets.items():
        target = contents / relative
        require(target.is_file() and os.access(target, os.X_OK), f"Missing executable: {relative}")
        if signatures:
            description = subprocess.check_output(["/usr/bin/file", str(target)], text=True)
            require("Mach-O" in description and "arm64" in description, f"Wrong executable type: {relative}")
            subprocess.run(["/usr/bin/codesign", "--verify", "--strict", str(target)], check=True)
            result = subprocess.run(["/usr/bin/codesign", "-dv", str(target)], capture_output=True, text=True, check=True)
            require(f"Identifier={identifier}" in result.stderr.splitlines(), f"Wrong signing identity: {relative}")
    if signatures:
        subprocess.run(["/usr/bin/codesign", "--verify", "--deep", "--strict", str(path)], check=True)
    if shared:
        marker_path = contents / "Resources/desktop-product.json"
        require(marker_path.is_file(), "Missing Shared Desktop product marker")
        marker = json.loads(marker_path.read_text())
        require(marker == {
            "schema_version": 1,
            "product_name": NAME,
            "desktop_variant": "next",
            "ui": "shared-wails",
        }, "Invalid Shared Desktop product marker")
        # Inspect Go module/build metadata without launching the bundled executable.
        metadata = subprocess.check_output(["go", "version", "-m", str(contents / "MacOS/AgentDock")], text=True)
        require("github.com/wailsapp/wails/v3" in metadata, "Missing Wails Shared Desktop dependency")
        require("	build	-tags=production" in metadata, "Shared Desktop executable is not a production build")
    return path
