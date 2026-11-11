#!/usr/bin/env python3
"""Explicit Next-only bundle replacement, SMAppService rebinding, and rollback.

Never targets stable AgentDock, and never touches GUI, user browser, or AGY ACP.
Requires a running, healthy Next Core before attempting an upgrade.
"""
import importlib.util
import json
import os
from pathlib import Path
import pwd
import re
import selectors
import subprocess
import sys
import time
import urllib.request

from next_shared_bundle import NAME, no_symlinks, require, validate_bundle

LABEL_BASE = "dev.dropabit.agentdock.next"
NEXT_HEALTH = "http://127.0.0.1:8767/healthz"
VERSION_OUTPUT_LIMIT = 4096
VERSION_TIMEOUT = 5

INSTALLER = Path(__file__).with_name("install-next-shared.py")
INSTALLER_SPEC = importlib.util.spec_from_file_location("next_shared_installer", INSTALLER)
require(INSTALLER_SPEC is not None and INSTALLER_SPEC.loader is not None, "Next-only installer module missing")
NEXT_INSTALLER = importlib.util.module_from_spec(INSTALLER_SPEC)
INSTALLER_SPEC.loader.exec_module(NEXT_INSTALLER)


def semver_key(version: str) -> tuple:
    """Canonical SemVer precedence; build metadata does not affect ordering."""
    require(isinstance(version, str), "Invalid Core semantic version")
    match = re.fullmatch(
        r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
        r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
        r"(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?", version,
    )
    require(match is not None, "Invalid Core semantic version")
    prerelease = match[4]
    identifiers = []
    for item in prerelease.split(".") if prerelease is not None else []:
        numeric = item.isdecimal()
        require(not numeric or item == "0" or not item.startswith("0"), "Noncanonical Core prerelease")
        identifiers.append((0, int(item)) if numeric else (1, item))
    return (*map(int, match.group(1, 2, 3)), prerelease is None, tuple(identifiers))


def core_version(bundle: Path) -> dict:
    """Run only the validated bundle's read-only CLI, with bounded pipe reads."""
    binary = bundle / "Contents/Helpers/agentdock"
    require(no_symlinks(binary) == binary and binary.is_file() and os.access(binary, os.X_OK),
            "Core version executable path mismatch or unavailable")
    deadline = time.monotonic() + VERSION_TIMEOUT
    output = bytearray()
    with subprocess.Popen([str(binary), "version", "--json"], stdin=subprocess.DEVNULL,
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT) as process:
        try:
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                while True:
                    remaining = deadline - time.monotonic()
                    require(remaining > 0 and selector.select(remaining), "Core version probe timed out")
                    chunk = os.read(process.stdout.fileno(), VERSION_OUTPUT_LIMIT + 1 - len(output))
                    if not chunk:
                        break
                    output.extend(chunk)
                    require(len(output) <= VERSION_OUTPUT_LIMIT, "Core version output exceeds limit")
            require(process.wait(timeout=max(0, deadline - time.monotonic())) == 0,
                    "Core version probe failed")
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()

    def unique_fields(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "Duplicate Core version metadata")
            result[key] = value
        return result

    def invalid_constant(value):
        raise ValueError("Non-JSON constant in Core version metadata")

    metadata = json.loads(output.decode("utf-8"), object_pairs_hook=unique_fields,
                          parse_constant=invalid_constant)
    require(isinstance(metadata, dict), "Invalid Core version metadata")
    semver_key(metadata.get("version"))
    commit = metadata.get("commit")
    require(isinstance(commit, str) and re.fullmatch(r"[0-9a-f]{7,40}", commit),
            "Missing or invalid Core commit identity")
    require(metadata.get("platform") == "darwin/arm64", "Missing or invalid Core platform identity")
    return metadata


def manual_version_gate(source: Path, destination: Path) -> None:
    candidate = core_version(source)
    installed = core_version(destination)
    candidate_key = semver_key(candidate["version"])
    installed_key = semver_key(installed["version"])
    require(candidate_key >= installed_key, "Next manual deploy refused: Core semantic version downgrade")
    require(candidate_key != installed_key or candidate["commit"] == installed["commit"],
            "Next manual deploy refused: equal Core version with different commit is ambiguous")


def active_jobs(list_output: str) -> dict[str, int | None]:
    """Only exact Next-owned launchd labels; unrelated labels are ignored."""
    jobs: dict[str, int | None] = {}
    for line in list_output.splitlines():
        fields = line.split()
        if len(fields) != 3:
            continue
        pid_text, _, label = fields
        if label not in {f"{LABEL_BASE}.core", f"{LABEL_BASE}.tunnel"}:
            continue
        if pid_text == "-":
            jobs[label] = None
        elif pid_text.isdecimal() and int(pid_text) > 0:
            jobs[label] = int(pid_text)
    return jobs


def backup_from_installer_output(output: str, destination: Path) -> Path:
    marker = f"Replaced Contents in {destination}; previous Next Contents retained at "
    matches = [line[len(marker):] for line in output.splitlines() if line.startswith(marker)]
    require(len(matches) == 1, "Installer did not return one unambiguous Next rollback location")
    candidate = no_symlinks(matches[0])
    require(candidate.name == "Contents" and candidate.parent.name == f"{NAME}.app", "Invalid rollback bundle shape")
    stage = candidate.parent.parent
    require(stage.parent == destination.parent and re.fullmatch(r"\.agentdock-next-replacement-[a-zA-Z0-9_-]+", stage.name), "Rollback path is outside Next private staging")
    require(candidate.is_dir() and stage.stat().st_uid == os.getuid(), "Rollback directory is unavailable or unowned")
    return candidate


def run_checked(args: list[str], *, timeout: int = 90, env: dict[str, str] | None = None) -> str:
    result = subprocess.run(args, check=False, capture_output=True, text=True, timeout=timeout, env=env)
    if result.returncode:
        # Never echo process configuration, tokens, or unexpected raw stderr.
        raise RuntimeError(f"Next-only tool returned {result.returncode}: {Path(args[0]).name}")
    return result.stdout.strip()


def jobs() -> dict[str, int | None]:
    return active_jobs(run_checked(["/bin/launchctl", "list"], timeout=12))


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, msg, headers, new_url):
        return None


def healthy_next_core() -> bool:
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        with opener.open(NEXT_HEALTH, timeout=2) as response:
            if response.status != 200:
                return False
            payload = json.loads(response.read(4096))
            return payload.get("ok") is True and isinstance(payload.get("version"), str)
    except (OSError, ValueError, TypeError):
        return False


def trusted_next_core_pid(pid: int, destination: Path) -> bool:
    """Bind Next launchd PID, its signed bundle executable and listener.

    A generic HTTP 200 on loopback does NOT establish service ownership.
    These are read-only OS observations; no stable process is inspected.
    """
    if pid <= 0:
        return False
    expected_executable = str(destination / "Contents/Helpers/agentdock")
    try:
        txt = subprocess.run(
            ["/usr/sbin/lsof", "-nP", "-a", "-p", str(pid), "-d", "txt", "-Fn"],
            capture_output=True, text=True, timeout=5, check=False,
        )
        listener = subprocess.run(
            ["/usr/sbin/lsof", "-nP", "-a", "-p", str(pid),
             "-iTCP:8767", "-sTCP:LISTEN", "-Fp"],
            capture_output=True, text=True, timeout=5, check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return False
    return (txt.returncode == 0 and listener.returncode == 0
            and txt.stdout.splitlines().count("n" + expected_executable) == 1
            and ("p" + str(pid)) in txt.stdout.splitlines()
            and ("p" + str(pid)) in listener.stdout.splitlines())


def wait_for_core(previous_pid: int | None, *, destination: Path, max_seconds: float = 24) -> bool:
    deadline = time.monotonic() + max_seconds
    while time.monotonic() < deadline:
        current = jobs().get(f"{LABEL_BASE}.core")
        if (current is not None and current != previous_pid
                and trusted_next_core_pid(current, destination)
                and healthy_next_core()
                and trusted_next_core_pid(current, destination)):
            return True
        time.sleep(0.5)
    return False


def verify_same_payload(source: Path, destination: Path) -> None:
    from hashlib import sha256

    for item in ("MacOS/AgentDock", "Helpers/agentdock", "Helpers/cloudflared", "MacOS/AgentDockServiceRegistrar"):
        left = (source / "Contents" / item).read_bytes()
        right = (destination / "Contents" / item).read_bytes()
        require(sha256(left).digest() == sha256(right).digest(), "Installed Next payload differs from verified source")


def registrar(destination: Path, component: str) -> None:
    require(component in {"core", "tunnel"}, "Only Next Core and Tunnel service registrations are supported")
    binary = destination / "Contents/MacOS/AgentDockServiceRegistrar"
    require(binary.is_file() and os.access(binary, os.X_OK), "Next Registrar binary unavailable")
    output = run_checked([str(binary), "reregister", component], timeout=75)
    require(output.strip() == "enabled", f"Next {component} registration was not confirmed enabled")


def validate_next_skill_paths(home: Path) -> None:
    """Reject redirected Next Skill write roots and any symlinked ancestors."""
    root = home / ".agentdock-next"
    for relative in (".", "skills", "locks/skills", "tmp/skills", "state/skills/transactions"):
        no_symlinks(root / relative)


def bootstrap_next_core_skills(destination: Path, home: Path) -> None:
    """Install the exact Next-bundled Core Skills into Next-owned managed state.

    The shared-Wails replacement path does not execute InstallerRunner.swift,
    which normally bootstraps those Skills during initial Desktop setup.
    Keep this final deploy step Next-only; never use the stable Home.
    """
    validate_next_skill_paths(home)
    binary = destination / "Contents/Helpers/agentdock"
    bundle = destination / "Contents/Resources/core-skills"
    require(binary.is_file() and bundle.joinpath("manifest.json").is_file(),
            "Next Core Skill bootstrap payload is missing")
    env = os.environ.copy()
    env.update({
        "AGENTDOCK_DESKTOP_VARIANT": "next",
        "AGENTDOCK_HOME": str(home / ".agentdock-next"),
        "AGENTDOCK_DEFAULT_DIR": str(home / "AgentDock Next"),
    })
    run_checked([str(binary), "skill", "bootstrap", "--bundle", str(bundle)],
                timeout=90, env=env)


def main() -> None:
    require(sys.platform == "darwin" and len(sys.argv) == 2, "Usage: python3 deploy-next-shared.py SOURCE_NEXT.app")
    home = no_symlinks(pwd.getpwuid(os.getuid()).pw_dir)
    destination = home / "Applications" / f"{NAME}.app"
    require(destination.is_dir(), "Installed AgentDock Next is required; never install into stable")
    source = no_symlinks(sys.argv[1])
    require(source != destination and not os.path.samefile(source, destination), "Source and installed Next must differ")
    require(validate_bundle(source, shared=True) == source, "Validated source bundle path mismatch")
    require(validate_bundle(destination, shared=True, allow_legacy_rc19_shared_omissions=True) == destination, "Validated installed bundle path mismatch")
    manual_version_gate(source, destination)
    validate_next_skill_paths(home)
    loaded = jobs()
    previous_core = loaded.get(f"{LABEL_BASE}.core")
    require(previous_core is not None and trusted_next_core_pid(previous_core, destination)
            and healthy_next_core() and trusted_next_core_pid(previous_core, destination),
            "Existing Next Core PID, binary, listener and health must match")
    previous_tunnel = loaded.get(f"{LABEL_BASE}.tunnel")
    tunnel_enabled = previous_tunnel is not None

    backup: Path | None = None
    try:
        install_output = run_checked([sys.executable, str(INSTALLER), str(source)], timeout=100)
        backup = backup_from_installer_output(install_output, destination)
        validate_bundle(backup.parent, shared=True, allow_legacy_rc19_shared_omissions=True)
        validate_bundle(destination, shared=True)
        verify_same_payload(source, destination)
        registrar(destination, "core")
        require(wait_for_core(previous_core, destination=destination), "Updated Next Core failed its PID/binary/listener/health gate")
        if tunnel_enabled:
            registrar(destination, "tunnel")
            updated_tunnel = jobs().get(f"{LABEL_BASE}.tunnel")
            require(updated_tunnel is not None and updated_tunnel != previous_tunnel, "Updated Next tunnel did not start a fresh instance")
    except Exception as deployment_error:
        if backup is None:
            raise RuntimeError("Next deploy failed before a verified rollback path; inspect Next stage using stable mac-dev") from deployment_error
        try:
            # Existing installer performs the only bundle-content swap. Reuse its
            # exact low-level atomic primitive without touching root/service state.
            # Do not validate the failed candidate: a corrupt new bundle is
            # exactly when restoring known-good Contents must still work.
            require((destination / "Contents").is_dir(), "Installed Next Contents missing")
            no_symlinks(destination / "Contents")
            validate_bundle(backup.parent, shared=True, allow_legacy_rc19_shared_omissions=True)
            NEXT_INSTALLER.rename_swap(backup, destination / "Contents")
            validate_bundle(destination, shared=True)
            registrar(destination, "core")
            require(wait_for_core(None, destination=destination), "Rolled-back Next Core identity/health not recovered")
            if tunnel_enabled:
                registrar(destination, "tunnel")
            print("NEXT_ONLY_ROLLBACK_RESTORED", file=sys.stderr)
        except Exception as rollback_error:
            raise RuntimeError(
                "Next deployment failed AND verified Next-only rollback did not complete; recover via stable mac-dev"
            ) from rollback_error
        raise RuntimeError("New Next deployment failed; previous Next bundle recovered") from deployment_error

    # App activation is complete. A killed bootstrap may leave partial Skill
    # updates, so restoring only App Contents would create mismatched state.
    print(f"NEXT_ROLLBACK_BUNDLE: {backup.parent}")
    try:
        bootstrap_next_core_skills(destination, home)
    except Exception as bootstrap_error:
        raise RuntimeError(
            "Next Core Skills bootstrap incomplete; app update already active and healthy; next-only repair required"
        ) from bootstrap_error
    print(f"NEXT_ONLY_DEPLOYED: {destination}")
    print("NEXT_GUI_RESTART_PENDING: manually quit and reopen the Next app without stealing focus")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        sys.exit(f"Next-only deployment error: {error}")
