"""Read-only validation shared by the Next package check and explicit installer."""
import json
import os
from pathlib import Path
import plistlib
import re
import stat
import struct
import subprocess

NAME = "AgentDock Next"
BUNDLE_ID = "dev.dropabit.agentdock.next"
MCP_LAUNCHER = "Helpers/agentdock-next-mcp-launch"
MCP_CATALOG = "Resources/native-mcp-servers.json"
MCP_CATALOG_LIMIT = 64 * 1024
MCP_SERVER_ID = re.compile(r"[a-z][a-z0-9-]{0,62}", re.ASCII)
MCP_INTERPRETERS = {"sh", "bash", "zsh", "dash", "fish", "csh", "tcsh", "env",
                    "python", "python3", "perl", "ruby", "node", "osascript"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def no_symlinks(path):
    path = Path(os.path.abspath(path))
    for item in (path, *path.parents):
        require(not item.is_symlink(), f"Symlink refused: {item}")
    return path


def strict_json(raw):
    require(len(raw) <= MCP_CATALOG_LIMIT, "Oversized native MCP JSON")
    def object_pairs(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "Duplicate native MCP JSON key")
            result[key] = value
        return result
    def invalid_constant(value):
        raise ValueError("Invalid native MCP JSON number")
    value = json.loads(raw.decode("utf-8"), object_pairs_hook=object_pairs,
                       parse_constant=invalid_constant)
    def check(item, depth=0):
        require(depth <= 8, "Nested native MCP JSON")
        if isinstance(item, str):
            item.encode("utf-8", errors="strict")
            require("\ufffd" not in item, "Invalid native MCP Unicode")
        elif isinstance(item, dict):
            for key, value in item.items():
                check(key, depth + 1)
                check(value, depth + 1)
        elif isinstance(item, list):
            for value in item:
                check(value, depth + 1)
    check(value)
    return value


def parse_native_mcp_catalog(raw):
    catalog = strict_json(raw)
    require(isinstance(catalog, dict) and set(catalog) == {"schema_version", "servers"}
            and type(catalog["schema_version"]) is int and catalog["schema_version"] == 1,
            "Invalid native MCP catalog schema")
    servers = catalog["servers"]
    require(isinstance(servers, list) and len(servers) <= 32, "Invalid native MCP servers")
    seen = set()
    for entry in servers:
        require(isinstance(entry, dict) and {"id", "executable"} <= set(entry)
                <= {"id", "executable", "args"}, "Invalid native MCP server fields")
        server_id = entry["id"]
        require(isinstance(server_id, str) and MCP_SERVER_ID.fullmatch(server_id)
                and server_id not in seen and server_id not in MCP_INTERPRETERS,
                "Invalid or duplicate native MCP server ID")
        seen.add(server_id)
        require(entry["executable"] == "Helpers/native-mcp/" + server_id,
                "Invalid native MCP executable path")
        args = entry.get("args", [])
        require(isinstance(args, list) and len(args) <= 64, "Invalid native MCP argv")
        for arg in args:
            require(isinstance(arg, str) and len(arg.encode("utf-8")) <= 1024
                    and not any(c in arg for c in "\x00\r\n"), "Invalid native MCP argument")
    return servers


def native_mcp_private_entry(target, bundle):
    for path in (target, *target.parents):
        info = path.lstat()
        require((stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode))
                and info.st_uid == os.getuid() and info.st_mode & 0o6022 == 0
                and (not stat.S_ISREG(info.st_mode) or info.st_nlink == 1),
                "Unsafe native MCP bundle entry")
        if path == bundle:
            break


def native_mcp_catalog_targets(bundle):
    manifest = bundle / "Contents" / MCP_CATALOG
    require(manifest.is_file(), "Missing native MCP catalog")
    native_mcp_private_entry(manifest, bundle)
    with manifest.open("rb") as file:
        servers = parse_native_mcp_catalog(file.read(MCP_CATALOG_LIMIT + 1))
    targets = {}
    for entry in servers:
        target = bundle / "Contents" / entry["executable"]
        require(target.is_file() and os.access(target, os.X_OK), "Missing native MCP backend")
        native_mcp_private_entry(target, bundle)
        with target.open("rb") as file:
            header = file.read(16)
        require(len(header) == 16 and struct.unpack("<IIII", header)[:2] == (0xfeedfacf, 0x100000c)
                and struct.unpack("<IIII", header)[3] == 2, "Native MCP backend must be arm64 Mach-O executable")
        targets[entry["executable"]] = BUNDLE_ID + ".mcp-server." + entry["id"]
    return targets


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
        targets.update(native_mcp_catalog_targets(path))
    # Existing native/older Next destinations may omit it; every new Shared
    # package requires it, and any included launcher must retain Next identity.
    if shared or (contents / MCP_LAUNCHER).exists():
        targets[MCP_LAUNCHER] = BUNDLE_ID + ".mcp-launch"
    for relative, identifier in targets.items():
        target = contents / relative
        require(target.is_file() and os.access(target, os.X_OK), f"Missing executable: {relative}")
        if signatures:
            description = subprocess.check_output(["/usr/bin/file", str(target)], text=True)
            require("Mach-O" in description and "arm64" in description, f"Wrong executable type: {relative}")
            if relative == MCP_LAUNCHER:
                arches = subprocess.check_output(["/usr/bin/lipo", "-archs", str(target)], text=True)
                require(arches.split() == ["arm64"], "Next MCP launcher must be arm64 only")
            subprocess.run(["/usr/bin/codesign", "--verify", "--strict", str(target)], check=True)
            result = subprocess.run(["/usr/bin/codesign", "-dv", str(target)], capture_output=True, text=True, check=True)
            require(f"Identifier={identifier}" in result.stderr.splitlines(), f"Wrong signing identity: {relative}")
            if relative == MCP_LAUNCHER:
                metadata = subprocess.check_output(["go", "version", "-m", str(target)], text=True)
                for entry in (
                    "\tpath\tgithub.com/uvwt/agentdock/cmd/agentdock-next-mcp-launch",
                    "\tbuild\tGOOS=darwin", "\tbuild\tGOARCH=arm64",
                ):
                    require(entry in metadata.splitlines(), "Wrong Next MCP launcher build")
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
        core_metadata = subprocess.check_output(["go", "version", "-m", str(contents / "Helpers/agentdock")], text=True)
        require("	build	CGO_ENABLED=1" in core_metadata, "AgentDock Next Core must enable cgo for macOS port ownership preflight")
    return path
