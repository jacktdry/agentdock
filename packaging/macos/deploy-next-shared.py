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
import subprocess
import sys
import time
import urllib.request

from next_shared_bundle import NAME, no_symlinks, require, validate_bundle

LABEL_BASE = "dev.dropabit.agentdock.next"
NEXT_HEALTH = "http://127.0.0.1:8767/healthz"

INSTALLER = Path(__file__).with_name("install-next-shared.py")
INSTALLER_SPEC = importlib.util.spec_from_file_location("next_shared_installer", INSTALLER)
require(INSTALLER_SPEC is not None and INSTALLER_SPEC.loader is not None, "Next-only installer module missing")
NEXT_INSTALLER = importlib.util.module_from_spec(INSTALLER_SPEC)
INSTALLER_SPEC.loader.exec_module(NEXT_INSTALLER)


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


def run_checked(args: list[str], *, timeout: int = 90) -> str:
    result = subprocess.run(args, check=False, capture_output=True, text=True, timeout=timeout)
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


def wait_for_core(previous_pid: int | None, *, max_seconds: float = 24) -> bool:
    deadline = time.monotonic() + max_seconds
    while time.monotonic() < deadline:
        current = jobs().get(f"{LABEL_BASE}.core")
        if current is not None and current != previous_pid and healthy_next_core():
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


def main() -> None:
    require(sys.platform == "darwin" and len(sys.argv) == 2, "Usage: python3 deploy-next-shared.py SOURCE_NEXT.app")
    home = no_symlinks(pwd.getpwuid(os.getuid()).pw_dir)
    destination = home / "Applications" / f"{NAME}.app"
    require(destination.is_dir(), "Installed AgentDock Next is required; never install into stable")
    source = no_symlinks(sys.argv[1])
    require(source != destination and not os.path.samefile(source, destination), "Source and installed Next must differ")
    validate_bundle(source, shared=True)
    validate_bundle(destination, shared=True)
    loaded = jobs()
    previous_core = loaded.get(f"{LABEL_BASE}.core")
    require(previous_core is not None and healthy_next_core(), "Existing Next Core must be running and healthy")
    previous_tunnel = loaded.get(f"{LABEL_BASE}.tunnel")
    tunnel_enabled = previous_tunnel is not None

    backup: Path | None = None
    try:
        install_output = run_checked([sys.executable, str(INSTALLER), str(source)], timeout=100)
        backup = backup_from_installer_output(install_output, destination)
        validate_bundle(backup.parent, shared=True)
        validate_bundle(destination, shared=True)
        verify_same_payload(source, destination)
        registrar(destination, "core")
        require(wait_for_core(previous_core), "Updated Next Core failed its PID/health gate")
        if tunnel_enabled:
            registrar(destination, "tunnel")
            updated_tunnel = jobs().get(f"{LABEL_BASE}.tunnel")
            require(updated_tunnel is not None and updated_tunnel != previous_tunnel, "Updated Next tunnel did not start a fresh instance")
        print(f"NEXT_ONLY_DEPLOYED: {destination}")
        print(f"NEXT_ROLLBACK_BUNDLE: {backup.parent}")
        print("NEXT_GUI_RESTART_PENDING: manually quit and reopen the Next app without stealing focus")
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
            validate_bundle(backup.parent, shared=True)
            NEXT_INSTALLER.rename_swap(backup, destination / "Contents")
            validate_bundle(destination, shared=True)
            registrar(destination, "core")
            require(wait_for_core(None), "Rolled-back Next Core health not recovered")
            if tunnel_enabled:
                registrar(destination, "tunnel")
            print("NEXT_ONLY_ROLLBACK_RESTORED", file=sys.stderr)
        except Exception as rollback_error:
            raise RuntimeError(
                "Next deployment failed AND verified Next-only rollback did not complete; recover via stable mac-dev"
            ) from rollback_error
        raise RuntimeError("New Next deployment failed; previous Next bundle recovered") from deployment_error


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        sys.exit(f"Next-only deployment error: {error}")
