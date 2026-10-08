#!/usr/bin/env python3
"""Offline Next-only deployment orchestration tests. No launchd or installed App mutation."""
import contextlib
import importlib.util
import io
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "packaging/macos"))
SPEC = importlib.util.spec_from_file_location("next_deploy", ROOT / "packaging/macos/deploy-next-shared.py")
deploy = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(deploy)


class NextDeployTests(unittest.TestCase):
    def setUp(self):
        temp_root = "/private/tmp" if Path("/private/tmp").is_dir() else "/tmp"
        self.temp = tempfile.TemporaryDirectory(prefix="next-deploy-test-", dir=temp_root)
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.home = self.root / "user"
        self.apps = self.home / "Applications"
        self.dest = self.apps / "AgentDock Next.app"
        self.source = self.root / "incoming" / "AgentDock Next.app"
        self.backup = self.apps / ".agentdock-next-replacement-abc_123" / "AgentDock Next.app" / "Contents"
        (self.dest / "Contents/MacOS").mkdir(parents=True)
        (self.source / "Contents/MacOS").mkdir(parents=True)
        self.backup.mkdir(parents=True)
        (self.dest / "Contents/MacOS/AgentDockServiceRegistrar").write_text("fake registrar")
        (self.dest / "Contents/MacOS/AgentDockServiceRegistrar").chmod(0o755)
        self.install_output = f"Replaced Contents in {self.dest}; previous Next Contents retained at {self.backup}"
        self.patches = [
            mock.patch.object(deploy.pwd, "getpwuid", return_value=SimpleNamespace(pw_dir=str(self.home))),
            mock.patch.object(deploy.sys, "argv", ["deploy-next-shared.py", str(self.source)]),
            mock.patch.object(deploy, "validate_bundle", return_value=self.dest),
            mock.patch.object(deploy, "healthy_next_core", return_value=True),
            mock.patch.object(deploy, "verify_same_payload"),
            mock.patch.object(deploy, "wait_for_core", return_value=True),
            mock.patch.object(deploy, "run_checked", return_value=self.install_output),
        ]
        for patch in self.patches:
            patch.start()
            self.addCleanup(patch.stop)

    def test_launchd_label_allowlist(self):
        rows = (
            "123\t0\tdev.dropabit.agentdock.next.core\n"
            "-\t0\tdev.dropabit.agentdock.next.tunnel\n"
            "999\t0\tcom.uvwt.agentdock.core\n"
            "222\t0\tdev.dropabit.agentdock.next.malicious\n"
        )
        self.assertEqual(deploy.active_jobs(rows), {
            "dev.dropabit.agentdock.next.core": 123,
            "dev.dropabit.agentdock.next.tunnel": None,
        })

    def test_rollback_path_must_be_exact_next_private_stage(self):
        self.assertEqual(deploy.backup_from_installer_output(self.install_output, self.dest), self.backup)
        with self.assertRaises(ValueError):
            deploy.backup_from_installer_output(self.install_output + "\n" + self.install_output, self.dest)
        with self.assertRaises(ValueError):
            deploy.backup_from_installer_output(
                f"Replaced Contents in {self.dest}; previous Next Contents retained at {self.apps}/not-next.app/Contents",
                self.dest,
            )
        with self.assertRaises(ValueError):
            deploy.backup_from_installer_output(self.install_output.replace(str(self.dest), "/Applications/AgentDock.app"), self.dest)

    def test_disabled_tunnel_is_not_registered_or_started(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "registrar") as registrar:
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf):
                deploy.main()
            registrar.assert_called_once_with(self.dest, "core")
            self.assertIn("NEXT_ONLY_DEPLOYED", buf.getvalue())
            self.assertIn(str(self.backup.parent), buf.getvalue())

    def test_registered_but_inactive_tunnel_is_not_started(self):
        old = {"dev.dropabit.agentdock.next.core": 123, "dev.dropabit.agentdock.next.tunnel": None}
        with mock.patch.object(deploy, "jobs", return_value=old), \
             mock.patch.object(deploy, "registrar") as registrar:
            with contextlib.redirect_stdout(io.StringIO()):
                deploy.main()
            registrar.assert_called_once_with(self.dest, "core")

    def test_enabled_tunnel_is_reregistered_but_never_stable(self):
        old = {"dev.dropabit.agentdock.next.core": 123, "dev.dropabit.agentdock.next.tunnel": 456}
        new = {"dev.dropabit.agentdock.next.core": 789, "dev.dropabit.agentdock.next.tunnel": 999}
        with mock.patch.object(deploy, "jobs", side_effect=[old, new]), \
             mock.patch.object(deploy, "registrar") as registrar:
            deploy.main()
            self.assertEqual(registrar.call_args_list,
                             [mock.call(self.dest, "core"), mock.call(self.dest, "tunnel")])

    def test_unhealthy_core_rejects_before_install(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "healthy_next_core", return_value=False), \
             mock.patch.object(deploy, "registrar") as registrar:
            with self.assertRaisesRegex(ValueError, "running and healthy"):
                deploy.main()
            deploy.run_checked.assert_not_called()
            registrar.assert_not_called()

    def test_registrar_failure_restores_old_contents_and_health(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "registrar", side_effect=[RuntimeError("new rejected"), None]) as registrar, \
             mock.patch.object(deploy.NEXT_INSTALLER, "rename_swap") as swap:
            with self.assertRaisesRegex(RuntimeError, "previous Next bundle recovered"):
                deploy.main()
            swap.assert_called_once_with(self.backup, self.dest / "Contents")
            self.assertEqual(registrar.call_count, 2)

    def test_missing_rollback_location_aborts_without_registering(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "run_checked", return_value="unrecognized installer response"), \
             mock.patch.object(deploy, "registrar") as registrar:
            with self.assertRaisesRegex(RuntimeError, "verified rollback path"):
                deploy.main()
            registrar.assert_not_called()

    def test_failed_rollback_reports_unrecovered_next(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "registrar", side_effect=RuntimeError("constraint unavailable")) as registrar, \
             mock.patch.object(deploy.NEXT_INSTALLER, "rename_swap") as swap:
            with self.assertRaisesRegex(RuntimeError, "rollback did not complete"):
                deploy.main()
            swap.assert_called_once_with(self.backup, self.dest / "Contents")
            self.assertEqual(registrar.call_count, 2)

    def test_invalid_payload_also_enters_rollback(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "verify_same_payload", side_effect=ValueError("payload changed")), \
             mock.patch.object(deploy.NEXT_INSTALLER, "rename_swap") as swap, \
             mock.patch.object(deploy, "registrar") as registrar:
            with self.assertRaisesRegex(RuntimeError, "previous Next bundle recovered"):
                deploy.main()
            swap.assert_called_once_with(self.backup, self.dest / "Contents")
            registrar.assert_called_once_with(self.dest, "core")


if __name__ == "__main__":
    unittest.main()
