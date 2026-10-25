#!/usr/bin/env python3
"""Metadata/negative fixtures; optional real bundle inspection, never execution."""
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "packaging/macos"))
from next_shared_bundle import BUNDLE_ID, MCP_LAUNCHER, validate_bundle

spec = importlib.util.spec_from_file_location("installer", ROOT / "packaging/macos/install-next-shared.py")
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class NextPackageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir="/private/tmp")
        self.addCleanup(self.temp.cleanup)
        self.home = Path(self.temp.name)
        output = self.home / "Applications"
        fixture_env = os.environ.copy()
        # Candidate build scripts set this for real bundles; metadata fixtures
        # deliberately exercise the non-candidate path outside repository dist.
        fixture_env.pop("AGENTDOCK_NEXT_CANDIDATE_MANIFEST", None)
        fixture_env.pop("AGENTDOCK_MACOS_SHARED_EXECUTABLE", None)
        fixture_env.pop("AGENTDOCK_MACOS_NEXT_MCP_LAUNCHER", None)
        fixture_env["AGENTDOCK_MACOS_APP_VARIANT"] = "next"
        subprocess.run(["zsh", str(ROOT / "packaging/macos/build-app.sh"), "--metadata-only", str(output)], check=True, env=fixture_env)
        self.app = output / "AgentDock Next.app"
        contents = self.app / "Contents"
        for relative in ("MacOS/AgentDock", "Helpers/agentdock", "Helpers/cloudflared", "Helpers/agentdock-arbiter", "Helpers/AgentDockLoginHelper", "Resources/core-skills/manifest.json"):
            p = contents / relative
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text("fixture")
            p.chmod(0o755)

    def test_architecture_specific_next_artifact_names(self):
        for variant, arches, expected in (
            ("next", "arm64", "AgentDock-Next-macos-arm64"),
            ("next", "arm64,x86_64", "AgentDock-Next-macos-universal"),
            ("stable", "arm64", "AgentDock-macos-universal"),
        ):
            env = os.environ.copy()
            env["AGENTDOCK_MACOS_APP_VARIANT"] = variant
            env["AGENTDOCK_MACOS_ARCHES"] = arches
            values = subprocess.check_output(
                ["zsh", "-c", 'source packaging/macos/app-identity.sh; print -r -- "$ZIP_NAME" "$DMG_NAME"'],
                cwd=ROOT, env=env, text=True,
            ).split()
            self.assertEqual(values, [expected + ".zip", expected + ".dmg"])

    def test_legacy_nonshared_next_destination_without_launcher(self):
        self.assertFalse((self.app / "Contents" / MCP_LAUNCHER).exists())
        validate_bundle(self.app, signatures=False)
        installer.validate_destination(self.app, self.home)

    def shared_fixture(self):
        contents = self.app / "Contents"
        for relative in ("MacOS/AgentDockServiceRegistrar", MCP_LAUNCHER):
            target = contents / relative
            target.write_text("fixture")
            target.chmod(0o755)
        (contents / "Resources/desktop-product.json").write_text(json.dumps({
            "schema_version": 1, "product_name": "AgentDock Next",
            "desktop_variant": "next", "ui": "shared-wails",
        }))

    @staticmethod
    def go_metadata(command, **kwargs):
        if command[-1].endswith("agentdock-next-mcp-launch"):
            return ("\tpath\tgithub.com/uvwt/agentdock/cmd/agentdock-next-mcp-launch\n"
                    "\tbuild\tGOOS=darwin\n\tbuild\tGOARCH=arm64\n")
        return "github.com/wailsapp/wails/v3\n\tbuild\t-tags=production\n\tbuild\tCGO_ENABLED=1\n"

    def test_shared_launcher_required_at_exact_path(self):
        self.shared_fixture()
        launcher = self.app / "Contents" / MCP_LAUNCHER
        launcher.rename(launcher.with_name("other-launcher"))
        with self.assertRaisesRegex(ValueError, "Missing executable: " + MCP_LAUNCHER):
            validate_bundle(self.app, shared=True, signatures=False)

    def test_shared_launcher_symlink_and_nonexecutable_refused(self):
        self.shared_fixture()
        launcher = self.app / "Contents" / MCP_LAUNCHER
        launcher.chmod(0o644)
        with self.assertRaisesRegex(ValueError, "Missing executable: " + MCP_LAUNCHER):
            validate_bundle(self.app, shared=True, signatures=False)
        launcher.unlink()
        launcher.symlink_to("agentdock")
        with self.assertRaisesRegex(ValueError, "Non-regular bundle entry: .*" + MCP_LAUNCHER):
            validate_bundle(self.app, shared=True, signatures=False)

    def test_builder_refuses_launcher_for_stable_or_nonshared(self):
        for variant in ("stable", "next"):
            with self.subTest(variant=variant):
                env = os.environ.copy()
                env.pop("AGENTDOCK_NEXT_CANDIDATE_MANIFEST", None)
                env.pop("AGENTDOCK_MACOS_SHARED_EXECUTABLE", None)
                env.update(AGENTDOCK_MACOS_APP_VARIANT=variant,
                           AGENTDOCK_MACOS_ARCHES="arm64",
                           AGENTDOCK_MACOS_NEXT_MCP_LAUNCHER="/unused-launcher")
                result = subprocess.run(
                    ["zsh", str(ROOT / "packaging/macos/build-app.sh"),
                     "--metadata-only", str(self.home / "refused")],
                    env=env, capture_output=True, text=True,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("MCP launcher requires Next arm64 Shared Desktop", result.stderr)
                self.assertFalse((self.home / "refused").exists())

    def test_stable_metadata_does_not_require_launcher(self):
        env = os.environ.copy()
        for key in ("AGENTDOCK_NEXT_CANDIDATE_MANIFEST", "AGENTDOCK_MACOS_SHARED_EXECUTABLE", "AGENTDOCK_MACOS_NEXT_MCP_LAUNCHER"):
            env.pop(key, None)
        env["AGENTDOCK_MACOS_APP_VARIANT"] = "stable"
        output = self.home / "stable"
        subprocess.run(["zsh", str(ROOT / "packaging/macos/build-app.sh"),
                        "--metadata-only", str(output)], env=env, check=True)
        contents = output / "AgentDock.app/Contents"
        self.assertEqual(plistlib.loads((contents / "Info.plist").read_bytes())["CFBundleIdentifier"], "com.uvwt.agentdock")
        self.assertFalse((contents / MCP_LAUNCHER).exists())

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

    @unittest.skipUnless(sys.platform == "darwin", "renameatx_np is macOS-only")
    def test_contents_swap_preserves_app_root(self):
        left_app = self.home / "left.app"
        right_app = self.home / "right.app"
        (left_app / "Contents").mkdir(parents=True)
        (right_app / "Contents").mkdir(parents=True)
        (left_app / "Contents/marker").write_text("old")
        (right_app / "Contents/marker").write_text("new")
        original = left_app.stat()

        installer.rename_swap(left_app / "Contents", right_app / "Contents")

        current = left_app.stat()
        self.assertEqual((current.st_dev, current.st_ino), (original.st_dev, original.st_ino))
        self.assertEqual((left_app / "Contents/marker").read_text(), "new")
        self.assertEqual((right_app / "Contents/marker").read_text(), "old")

    def test_writable_destination_refused(self):
        self.app.parent.chmod(0o777)
        with self.assertRaises(ValueError):
            installer.validate_destination(self.app, self.home)

    def signed_fixture(self):
        """Real Mach-O/codesign/bundle seal; Go metadata is stubbed by callers."""
        self.shared_fixture()
        stub = self.home / "stub.c"
        stub.write_text("int main(void) { return 0; }\n")
        binary = self.home / "stub"
        subprocess.run(["xcrun", "clang", "-arch", "arm64", str(stub), "-o", str(binary)], check=True)
        targets = {
            "MacOS/AgentDock": BUNDLE_ID,
            "Helpers/agentdock": BUNDLE_ID + ".core",
            "Helpers/cloudflared": BUNDLE_ID + ".cloudflared",
            "Helpers/agentdock-arbiter": BUNDLE_ID + ".arbiter",
            "Helpers/AgentDockLoginHelper": BUNDLE_ID + ".login-helper",
            "MacOS/AgentDockServiceRegistrar": BUNDLE_ID + ".service-registrar",
            MCP_LAUNCHER: BUNDLE_ID + ".mcp-launch",
        }
        for relative, identifier in targets.items():
            target = self.app / "Contents" / relative
            target.write_bytes(binary.read_bytes())
            if relative != "MacOS/AgentDock":
                self.sign(target, identifier)
        self.sign(self.app, BUNDLE_ID)
        return self.app / "Contents" / MCP_LAUNCHER

    @staticmethod
    def sign(target, identifier):
        subprocess.run(["/usr/bin/codesign", "--force", "--sign", "-",
                        "--identifier", identifier, str(target)], check=True, capture_output=True)

    @unittest.skipUnless(sys.platform == "darwin", "Native signature fixtures require macOS")
    def test_signed_launcher_and_seal(self):
        self.signed_fixture()
        real_output = subprocess.check_output
        def inspect(command, **kwargs):
            return self.go_metadata(command, **kwargs) if command[0] == "go" else real_output(command, **kwargs)
        with mock.patch("next_shared_bundle.subprocess.check_output", side_effect=inspect):
            validate_bundle(self.app, shared=True)

    @unittest.skipUnless(sys.platform == "darwin", "Native signature fixtures require macOS")
    def test_launcher_identity_and_architecture_refusals(self):
        launcher = self.signed_fixture()
        original = launcher.read_bytes()
        real_output = subprocess.check_output
        def inspect(command, **kwargs):
            return self.go_metadata(command, **kwargs) if command[0] == "go" else real_output(command, **kwargs)
        for failure in ("stable-identity", "x86_64"):
            with self.subTest(failure=failure):
                launcher.write_bytes(original)
                if failure == "stable-identity":
                    self.sign(launcher, "com.uvwt.agentdock.mcp-launch")
                    expected = "Wrong signing identity: " + MCP_LAUNCHER
                else:
                    subprocess.run(["xcrun", "clang", "-arch", "x86_64", str(self.home / "stub.c"), "-o", str(launcher)], check=True)
                    self.sign(launcher, BUNDLE_ID + ".mcp-launch")
                    expected = "Wrong executable type: " + MCP_LAUNCHER
                # Both negative launchers are signed; reseal the parent so all
                # earlier signature checks pass and reach the launcher check.
                self.sign(self.app, BUNDLE_ID)
                subprocess.run(["/usr/bin/codesign", "--verify", "--deep", "--strict", str(self.app)], check=True, capture_output=True)
                with mock.patch("next_shared_bundle.subprocess.check_output", side_effect=inspect):
                    with self.assertRaisesRegex(ValueError, expected):
                        validate_bundle(self.app, shared=True)


if __name__ == "__main__":
    bundle = sys.argv.pop(1) if len(sys.argv) > 1 else None
    result = unittest.main(exit=False).result
    if not result.wasSuccessful():
        sys.exit(1)
    if bundle:
        validate_bundle(bundle, shared=True)
        print(f"Verified Next Shared bundle: {bundle}")
