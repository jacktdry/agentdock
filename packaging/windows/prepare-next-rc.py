#!/usr/bin/env python3
"""Prepare canonical Next RC metadata; optional unsigned Core/arbiter crossbuild.

Never packages, launches or installs a Windows runtime. Setup remains blocked.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import struct
import subprocess
import sys
import xml.etree.ElementTree as ET


INSTALLER_BLOCKER = (
    "Next Windows Setup blocked before ISCC: Next Inno source is declarative only; "
    "Next runtime, complete verified payload, install/uninstall ownership and "
    "updater isolation gates are not accepted"
)

# Known minimum from build-windows-offline-setup.ps1; never copy its Stable ZIP.
# This inventory is an integrity contract, not an accepted runtime/layout contract.
REQUIRED_PAYLOAD = {
    "agentdock.exe", "agentdock-arbiter.exe", "agentdock-tray.exe",
    "agentdock-shim.exe", "agentdock-tray-shim.exe", "agentdock.ico",
    "cloudflared.exe", "wsl-helper/manifest.json",
    "wsl-helper/agentdock-wsl-helper-linux-amd64",
    "wsl-helper/agentdock-wsl-helper-linux-arm64",
    "share/agentdock/core-skills/manifest.json",
}
NEXT_IDENTITY = {
    "product_name": "AgentDock Next",
    "product_variant": "next", "product_id": "dev.dropabit.agentdock.next",
    "install_root": "LocalAppData/AgentDock Next",
    "app_id": "725CE7CB-6F4D-4D78-957E-64CF729FF413",
    "aumid": "dev.dropabit.agentdock.next.controlpanel",
    "state_directory": ".agentdock-next", "listen_port": 8767,
}


def safe_path(path):
    """Reject link/reparse components before reading, including broken links."""
    path = Path(path).absolute()
    for part in (*reversed(path.parents), path):
        info = part.lstat()
        if stat.S_ISLNK(info.st_mode) or getattr(info, "st_file_attributes", 0) & 0x400:
            raise ValueError("symlink/reparse input rejected: " + str(part))
    return path


def read_input(path):
    path = safe_path(path)
    if not stat.S_ISREG(path.stat().st_mode):
        raise ValueError("regular input file required: " + str(path))
    return path.read_bytes()


def strict_json(data):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate JSON key: " + key)
            result[key] = value
        return result
    value = json.loads(data.decode("utf-8"), object_pairs_hook=pairs)
    if not isinstance(value, dict):
        raise ValueError("JSON object required")
    return value


def verify_payload(manifest_path, payload, output, architecture):
    """Read-only pre-ISCC check. Success never grants build/install permission."""
    if architecture not in ("amd64", "arm64"):
        raise ValueError("unsupported architecture")
    manifest_path = safe_path(manifest_path)
    payload = safe_path(payload)
    if not payload.is_dir():
        raise ValueError("explicit Next payload directory required")
    output = Path(os.path.abspath(output))
    if os.path.lexists(output):
        raise ValueError("output must be fresh")
    data = read_input(manifest_path)
    manifest = strict_json(data)
    candidate = safe_path(payload / "candidate.json")
    if read_input(candidate) != data:
        raise ValueError("payload candidate bytes mismatch")
    record = strict_json(read_input(payload / "next-payload.json"))
    if any(k not in manifest for k in
           ("version", "windows_version", "commit", "build_date", "channel", "update_feed")):
        raise ValueError("incomplete candidate manifest")
    expected = dict(NEXT_IDENTITY, schema="agentdock-next-windows-payload-v1",
                    architecture=architecture,
                    candidate_manifest_sha256=hashlib.sha256(data).hexdigest(),
                    **{k: manifest[k] for k in
                       ("version", "windows_version", "commit", "build_date", "channel", "update_feed")})
    if set(record) != set(expected) | {"files"} or any(
            type(record[k]) is not type(v) or record[k] != v for k, v in expected.items()):
        raise ValueError("Next payload identity/manifest/architecture mismatch")
    files = record["files"]
    if not isinstance(files, dict) or not REQUIRED_PAYLOAD <= set(files):
        raise ValueError("missing required Next payload assets")
    for name, digest in files.items():
        # Portable Windows spelling: reject ADS, traversal, drive/UNC, reserved
        # names, case aliases and trailing-dot/space normalization.
        parts = name.split("/")
        if (not all(re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]*", p) and
                    p not in (".", "..") and not p.endswith(".") and
                    not re.fullmatch(r"(?i)(con|prn|aux|nul|com[0-9]|lpt[0-9])(?:\..*)?", p)
                    for p in parts) or name in ("candidate.json", "next-payload.json")):
            raise ValueError("unsafe payload path: " + name)
        if not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError("invalid SHA-256: " + name)
    names = {"candidate.json", "next-payload.json"} | set(files)
    if len({n.casefold() for n in names}) != len(names):
        raise ValueError("case-alias payload path")
    spellings = {}
    for name in names:
        parts = name.split("/")
        for end in range(1, len(parts) + 1):
            prefix = "/".join(parts[:end])
            previous = spellings.setdefault(prefix.casefold(), prefix)
            if previous != prefix:
                raise ValueError("case-alias payload directory")
    actual = set()
    for path in payload.rglob("*"):
        safe_path(path)
        if path.is_dir():
            continue
        if not path.is_file():
            raise ValueError("non-regular payload file")
        actual.add(path.relative_to(payload).as_posix())
    if actual != names:
        raise ValueError("unlisted or missing payload file")
    for name, digest in files.items():
        path = safe_path(payload / name)
        blob = read_input(path)
        if hashlib.sha256(blob).hexdigest() != digest:
            raise ValueError("SHA-256 mismatch: " + name)
        if name.endswith(".exe"):
            offset = struct.unpack_from("<I", blob, 0x3c)[0] if len(blob) >= 64 else len(blob)
            if (blob[:2] != b"MZ" or offset + 6 > len(blob) or
                    blob[offset:offset + 4] != b"PE\0\0" or
                    struct.unpack_from("<H", blob, offset + 4)[0] !=
                    {"amd64": 0x8664, "arm64": 0xaa64}[architecture]):
                raise ValueError("PE architecture mismatch: " + name)
    root = Path(run(["git", "rev-parse", "--show-toplevel"], Path.cwd())).resolve()
    env = dict(os.environ, GOPROXY="off", GOTOOLCHAIN="local", GOSUMDB="off")
    # Existing canonical candidate verifier owns clean HEAD/date/channel checks.
    run(["go", "run", "-mod=readonly", "./tools/release", "next-candidate",
         "--manifest", str(manifest_path), "--check-output", str(output)], root, env)
    if manifest_path.read_bytes() != data or candidate.read_bytes() != data:
        raise ValueError("candidate changed during verification")
    return {"input_integrity_verified": True, "architecture": architecture,
            "installable": False, "release_ready": False,
            "installer_blocker": INSTALLER_BLOCKER}


def run(args, root, env=None):
    return subprocess.check_output(args, cwd=root, env=env, text=True).strip()


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def resource(filename, description, manifest):
    version = manifest["windows_version"]
    return {"RT_VERSION": {"#1": {"0000": {
        "fixed": {"file_version": version, "product_version": version},
        "info": {"0409": {
            "CompanyName": "AgentDock", "ProductName": "AgentDock Next",
            "FileDescription": description, "FileVersion": version,
            "ProductVersion": version, "OriginalFilename": filename,
            "InternalName": Path(filename).stem,
            "LegalCopyright": "Copyright AgentDock contributors",
            "Comments": "{}; commit={}; build_date={}".format(
                manifest["version"], manifest["commit"], manifest["build_date"]),
        }},
    }}}}


def prepare(manifest_path, output, architecture, build_binaries=False):
    root = Path(run(["git", "rev-parse", "--show-toplevel"], Path.cwd())).resolve()
    # Snapshot bytes once: the verifier and all outputs use the identical input.
    data = manifest_path.read_bytes()
    output = Path(os.path.abspath(output))
    if output.exists():
        raise ValueError("output must be a fresh repo-local dist subdirectory")
    # Canonical tool checks HEAD/date, clean source, identity and non-symlink output.
    run(["go", "run", "-mod=readonly", "./tools/release", "next-candidate",
         "--manifest", str(manifest_path.resolve()), "--check-output", str(output)], root)
    output.mkdir(parents=True, exist_ok=False)
    snapshot = output / "candidate.json"
    snapshot.write_bytes(data)
    verified = run(["go", "run", "-mod=readonly", "./tools/release", "next-candidate",
                    "--manifest", str(snapshot), "--output", str(output)], root)
    manifest = json.loads(data)
    if verified.split("\t")[:3] != [manifest[k] for k in ("version", "commit", "build_date")]:
        raise ValueError("verified provenance mismatch")
    executables = {
        "agentdock.exe": ("AgentDock Next Core", "./cmd/agentdock"),
        "agentdock-arbiter.exe": ("AgentDock Next Arbiter", "./cmd/agentdock-arbiter"),
    }
    for filename, (description, _) in executables.items():
        write_json(output / (filename + ".winres.json"), resource(filename, description, manifest))
    # Build-only properties, not permission to launch the still-Stable WPF runtime.
    props = ET.Element("Project")
    group = ET.SubElement(props, "PropertyGroup")
    for key, value in {
        "Version": manifest["windows_version"], "FileVersion": manifest["windows_version"],
        "AssemblyVersion": manifest["windows_version"],
        "InformationalVersion": manifest["version"], "Product": "AgentDock Next",
        "AssemblyTitle": "AgentDock Next", "Description": "AgentDock Next Control Panel",
        "Company": "AgentDock", "IncludeSourceRevisionInInformationalVersion": "false",
    }.items():
        ET.SubElement(group, key).text = value
    ET.ElementTree(props).write(output / "next-version.props", encoding="utf-8", xml_declaration=True)
    record = {
        "schema": "agentdock-next-windows-preparation-v1",
        "candidate_manifest_sha256": hashlib.sha256(data).hexdigest(),
        "version": manifest["version"], "windows_version": manifest["windows_version"],
        "commit": manifest["commit"], "build_date": manifest["build_date"],
        "product_name": "AgentDock Next", "architecture": architecture,
        "channel": manifest["channel"], "update_feed": manifest["update_feed"],
        "release_ready": False, "installable": False,
        "installer_blocker": INSTALLER_BLOCKER,
        "planned_setup_filename": "AgentDock-NextSetup-" + architecture + ".exe",
        "binary_checksums": {}, "pe_version_info_applied": False,
    }
    if build_binaries:
        env = dict(os.environ, GOOS="windows", GOARCH=architecture, CGO_ENABLED="0")
        flags = "-s -w -X github.com/uvwt/agentdock/internal/buildinfo.Commit={} -X github.com/uvwt/agentdock/internal/buildinfo.BuildDate={}".format(
            manifest["commit"], manifest["build_date"])
        for filename, (_, package) in executables.items():
            binary = output / filename
            run(["go", "build", "-mod=readonly", "-trimpath", "-overlay", str(output / "overlay.json"),
                 "-ldflags", flags, "-o", str(binary), package], root, env)
            # Host tool patches the PE; restore host GOOS/GOARCH via separate env.
            host_env = dict(os.environ)
            host_env.pop("GOOS", None)
            host_env.pop("GOARCH", None)
            host_env["CGO_ENABLED"] = "0"
            run(["go", "run", "github.com/tc-hib/go-winres@v0.3.3", "patch",
                 "--in", str(output / (filename + ".winres.json")),
                 "--product-version", manifest["windows_version"],
                 "--file-version", manifest["windows_version"], "--no-backup", str(binary)], root, host_env)
            record["binary_checksums"][filename] = hashlib.sha256(binary.read_bytes()).hexdigest()
            (output / (filename + ".sha256")).write_text(
                record["binary_checksums"][filename] + "  " + filename + "\n", encoding="ascii")
        record["pe_version_info_applied"] = True
    write_json(output / "preparation.json", record)
    return record


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--architecture", required=True, choices=("amd64", "arm64"))
    parser.add_argument("--build-binaries", action="store_true")
    parser.add_argument("--build-installer", action="store_true", help="always fails before ISCC")
    parser.add_argument("--verify-payload", type=Path, help="read-only offline input check; never enables ISCC")
    args = parser.parse_args()
    if args.build_installer:
        parser.exit(2, INSTALLER_BLOCKER + "\n")
    try:
        if args.verify_payload:
            if args.build_binaries:
                raise ValueError("payload verification cannot build binaries")
            record = verify_payload(args.manifest, args.verify_payload, args.output, args.architecture)
        else:
            record = prepare(args.manifest, args.output, args.architecture, args.build_binaries)
    except (OSError, ValueError, subprocess.CalledProcessError) as exc:
        parser.exit(1, str(exc) + "\n")
    print(json.dumps(record, indent=2))


if __name__ == "__main__":
    main()
