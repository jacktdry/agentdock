#!/usr/bin/env python3
"""Prepare canonical Next RC metadata; optional unsigned Core/arbiter crossbuild.

Never packages, launches or installs a Windows runtime. Setup remains blocked.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import xml.etree.ElementTree as ET


INSTALLER_BLOCKER = (
    "Next Windows Setup blocked before ISCC: Next Inno source is declarative only; "
    "Next runtime, complete verified payload, install/uninstall ownership and "
    "updater isolation gates are not accepted"
)


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
    args = parser.parse_args()
    if args.build_installer:
        parser.exit(2, INSTALLER_BLOCKER + "\n")
    try:
        record = prepare(args.manifest, args.output, args.architecture, args.build_binaries)
    except (OSError, ValueError, subprocess.CalledProcessError) as exc:
        parser.exit(1, str(exc) + "\n")
    print(json.dumps(record, indent=2))


if __name__ == "__main__":
    main()
