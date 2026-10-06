#!/usr/bin/env python3
"""Metadata/negative fixtures; optional real bundle inspection, never execution."""
import importlib.util
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "packaging/macos"))
from next_shared_bundle import BUNDLE_ID, validate_bundle

spec = importlib.util.spec_from_file_location("installer", ROOT / "packaging/macos/install-next-shared.py")
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class NextPackageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir="/private/tmp")
        self.addCleanup(self.temp.cleanup)
        self.home = Path(self.temp.name)
        output = self.home / "Applications"
        subprocess.run(["env", "AGENTDOCK_MACOS_APP_VARIANT=next", "zsh", str(ROOT / "packaging/macos/build-app.sh"), "--metadata-only", str(output)], check=True)
        self.app = output / "AgentDock Next.app"
        contents = self.app / "Contents"
        for relative in ("MacOS/AgentDock", "Helpers/agentdock", "Helpers/cloudflared", "Helpers/agentdock-arbiter", "Helpers/AgentDockLoginHelper", "Resources/core-skills/manifest.json"):
            p = contents / relative
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text("fixture")
            p.chmod(0o755)

    def test_next_contract(self):
        validate_bundle(self.app, signatures=False)
        installer.validate_destination(self.app, self.home)

    def test_stable_identity_refused(self):
        info = self.app / "Contents/Info.plist"
        data = plistlib.loads(info.read_bytes())
        data["CFBundleIdentifier"] = "com.uvwt.agentdock"
        info.write_bytes(plistlib.dumps(data))
        with self.assertRaises(ValueError):
            validate_bundle(self.app, signatures=False)

    def test_stable_service_and_helper_path_refused(self):
        p = self.app / f"Contents/Library/LaunchAgents/{BUNDLE_ID}.core.plist"
        original = plistlib.loads(p.read_bytes())
        for key, value in (("Label", "com.uvwt.agentdock.core"), ("BundleProgram", "/Applications/AgentDock.app/Contents/Helpers/agentdock"), ("EnvironmentVariables", {"AGENTDOCK_DESKTOP_VARIANT": "stable"})):
            data = dict(original)
            data[key] = value
            p.write_bytes(plistlib.dumps(data))
            with self.assertRaises(ValueError):
                validate_bundle(self.app, signatures=False)

    def test_destination_allowlist(self):
        for dest in ("/Applications/AgentDock.app", "/Applications/AgentDock Next.app", self.home / "Applications/AgentDock.app", self.home / "elsewhere/AgentDock Next.app"):
            with self.assertRaises(ValueError):
                installer.validate_destination(dest, self.home)

    def test_symlink_destination_and_ancestor(self):
        alias = self.home / "alias"
        alias.symlink_to(self.app.parent, target_is_directory=True)
        with self.assertRaises(ValueError):
            installer.validate_destination(alias / self.app.name, self.home)
        old = self.app.with_name("saved.app")
        self.app.rename(old)
        self.app.symlink_to(old, target_is_directory=True)
        with self.assertRaises(ValueError):
            installer.validate_destination(self.app, self.home)

    def test_missing_or_symlink_helper(self):
        helper = self.app / "Contents/Helpers/agentdock"
        helper.unlink()
        with self.assertRaises(ValueError):
            validate_bundle(self.app, signatures=False)
        helper.symlink_to("cloudflared")
        with self.assertRaises(ValueError):
            validate_bundle(self.app, signatures=False)

    def test_writable_destination_refused(self):
        self.app.parent.chmod(0o777)
        with self.assertRaises(ValueError):
            installer.validate_destination(self.app, self.home)


if __name__ == "__main__":
    bundle = sys.argv.pop(1) if len(sys.argv) > 1 else None
    result = unittest.main(exit=False).result
    if not result.wasSuccessful():
        sys.exit(1)
    if bundle:
        validate_bundle(bundle, shared=True)
        print(f"Verified Next Shared bundle: {bundle}")
