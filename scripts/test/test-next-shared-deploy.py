#!/usr/bin/env python3
"""Offline Next-only deployment orchestration tests. No launchd or installed App mutation."""
import contextlib
import importlib.util
import io
import json
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
REAL_TRUSTED_NEXT_PID = deploy.trusted_next_core_pid
REAL_CORE_VERSION = deploy.core_version


class CoreVersionTests(unittest.TestCase):
    def probe(self, output, *, returncode=0, ready=True):
        bundle = Path("/fixture/AgentDock Next.app")
        process = mock.MagicMock()
        process.__enter__.return_value = process
        process.wait.return_value = returncode
        process.poll.return_value = 0 if ready else None
        selector = mock.MagicMock()
        selector.__enter__.return_value = selector
        selector.select.return_value = [object()] if ready else []
        with mock.patch.object(deploy.Path, "is_file", return_value=True), \
             mock.patch.object(deploy.os, "access", return_value=True), \
             mock.patch.object(deploy.subprocess, "Popen", return_value=process) as popen, \
             mock.patch.object(deploy.selectors, "DefaultSelector", return_value=selector), \
             mock.patch.object(deploy.os, "read", side_effect=[output, b""]):
            try:
                return REAL_CORE_VERSION(bundle)
            finally:
                self.assertEqual(popen.call_args.args[0],
                                 [str(bundle / "Contents/Helpers/agentdock"), "version", "--json"])
                if not ready:
                    process.kill.assert_called_once()
                    process.wait.assert_called_once()

    def metadata(self, **changes):
        return {"version": "0.9.2-rc.1", "commit": "cceadb0c1234",
                "platform": "darwin/arm64", **changes}

    def test_valid_read_only_probe(self):
        metadata = self.metadata()
        self.assertEqual(self.probe(json.dumps(metadata).encode()), metadata)

    def test_non_json_constants_rejected_even_in_extra_fields(self):
        for constant in ("NaN", "Infinity", "-Infinity"):
            output = json.dumps(self.metadata())[:-1] + ', "extra": ' + constant + '}'
            with self.subTest(constant=constant), self.assertRaises(ValueError):
                self.probe(output.encode())

    def test_malformed_truncated_duplicate_and_oversized_output(self):
        for output in (b"", b"{", b"[]", b"null", b"\xff", b"{} trailing",
                       b'{"version":"0.9.2","version":"0.9.3"}', b"x" * 4097):
            with self.subTest(output=output[:60]), self.assertRaises(ValueError):
                self.probe(output)

    def test_missing_and_invalid_identity(self):
        for key in ("version", "commit", "platform"):
            metadata = self.metadata()
            del metadata[key]
            with self.subTest(missing=key), self.assertRaises(ValueError):
                self.probe(json.dumps(metadata).encode())
        for change in ({"version": "v0.9.2"}, {"version": None}, {"commit": "unknown"},
                       {"commit": ""}, {"commit": 123}, {"commit": "ABCDEF12"},
                       {"platform": "darwin/amd64"}):
            with self.subTest(change=change), self.assertRaises(ValueError):
                self.probe(json.dumps(self.metadata(**change)).encode())

    def test_nonzero_and_timeout_fail_closed(self):
        with self.assertRaisesRegex(ValueError, "probe failed"):
            self.probe(json.dumps(self.metadata()).encode(), returncode=1)
        with self.assertRaisesRegex(ValueError, "timed out"):
            self.probe(b"", ready=False)

    def test_executable_path_mismatch(self):
        with mock.patch.object(deploy, "no_symlinks", return_value=Path("/wrong/agentdock")), \
             mock.patch.object(deploy.subprocess, "Popen") as popen:
            with self.assertRaisesRegex(ValueError, "path mismatch"):
                REAL_CORE_VERSION(Path("/fixture/AgentDock Next.app"))
            popen.assert_not_called()

    def test_canonical_semver_precedence(self):
        versions = ["0.9.1-alpha", "0.9.1-alpha.1", "0.9.1-alpha.beta", "0.9.1-beta",
                    "0.9.1-rc.1", "0.9.1-rc.2", "0.9.1-rc.10", "0.9.1", "0.9.2-rc.1", "0.10.0"]
        keys = [deploy.semver_key(v) for v in versions]
        self.assertEqual(keys, sorted(keys))
        self.assertEqual(deploy.semver_key("0.9.1+build.01"), deploy.semver_key("0.9.1"))
        for version in ("v0.9.1", " 0.9.1", "0.9.1\n", "00.9.1", "0.9", "0.9.1-rc.01",
                        "0.9.1-01", "0.9.1-", "0.9.1+", "0.9.1-rc..1"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                deploy.semver_key(version)


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
            mock.patch.object(deploy, "validate_bundle", side_effect=lambda path, **kwargs: path),
            mock.patch.object(deploy, "core_version", side_effect=lambda path: {
                "version": "0.9.2" if path == self.source else "0.9.1",
                "commit": "cceadb0c1234" if path == self.source else "154f73249671",
            }),
            mock.patch.object(deploy, "healthy_next_core", return_value=True),
            mock.patch.object(deploy, "trusted_next_core_pid", return_value=True),
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

    def test_version_rejection_precedes_jobs_and_installer(self):
        cases = [
            [{"version": "0.9.1-rc.1", "commit": "cceadb0c"},
             {"version": "0.9.1", "commit": "154f73249671"}],
            [{"version": "0.9.1", "commit": "cceadb0c"},
             {"version": "0.9.1", "commit": "154f73249671"}],
            [{"version": "0.9.1+new", "commit": "154f732"},
             {"version": "0.9.1", "commit": "154f73249671"}],
            [ValueError("invalid identity")],
            [{"version": "0.9.2", "commit": "cceadb0c"}, ValueError("installed malformed")],
        ]
        for case in cases:
            with self.subTest(case=case), \
                 mock.patch.object(deploy, "core_version", side_effect=case), \
                 mock.patch.object(deploy, "jobs") as jobs, \
                 mock.patch.object(deploy, "registrar") as registrar, \
                 mock.patch.object(deploy.NEXT_INSTALLER, "rename_swap") as swap:
                with self.assertRaises(ValueError):
                    deploy.main()
                jobs.assert_not_called()
                deploy.run_checked.assert_not_called()
                registrar.assert_not_called()
                swap.assert_not_called()

    def test_equal_version_same_commit_allows_reinstall(self):
        with mock.patch.object(deploy, "core_version", return_value={"version": "0.9.1", "commit": "154f73249671"}), \
             mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "registrar"):
            with contextlib.redirect_stdout(io.StringIO()):
                deploy.main()
            deploy.run_checked.assert_called_once()

    def test_validation_and_path_binding_precede_version_probe(self):
        for result in (ValueError("signature rejected"), self.dest):
            with self.subTest(result=result), \
                 mock.patch.object(deploy, "validate_bundle", side_effect=[result]), \
                 mock.patch.object(deploy, "jobs") as jobs:
                with self.assertRaises(ValueError):
                    deploy.main()
                deploy.core_version.assert_not_called()
                jobs.assert_not_called()
                deploy.run_checked.assert_not_called()

    def test_both_signatures_validated_before_versions_then_jobs(self):
        events = []
        def validated(path, **kwargs):
            events.append(("validate", path))
            return path
        def version(path):
            events.append(("version", path))
            return {"version": "0.9.2" if path == self.source else "0.9.1", "commit": "cceadb0c"}
        with mock.patch.object(deploy, "validate_bundle", side_effect=validated), \
             mock.patch.object(deploy, "core_version", side_effect=version), \
             mock.patch.object(deploy, "jobs", side_effect=ValueError("stop before mutation")):
            with self.assertRaisesRegex(ValueError, "stop before mutation"):
                deploy.main()
        self.assertEqual(events, [("validate", self.source), ("validate", self.dest),
                                  ("version", self.source), ("version", self.dest)])

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

    def test_listener_belongs_to_next_core_and_not_an_unrelated_pid(self):
        expected = self.dest / "Contents/Helpers/agentdock"
        matching = [
            SimpleNamespace(returncode=0, stdout=f"p123\nftxt\nn{expected}\n"),
            SimpleNamespace(returncode=0, stdout="p123\nf7\n"),
        ]
        with mock.patch.object(deploy.subprocess, "run", side_effect=matching) as proc:
            self.assertTrue(REAL_TRUSTED_NEXT_PID(123, self.dest))
            self.assertEqual(proc.call_count, 2)
        wrong_binary = [
            SimpleNamespace(returncode=0, stdout="p123\nftxt\nn/Applications/AgentDock.app/Contents/Helpers/agentdock\n"),
            SimpleNamespace(returncode=0, stdout="p123\nf7\n"),
        ]
        with mock.patch.object(deploy.subprocess, "run", side_effect=wrong_binary):
            self.assertFalse(REAL_TRUSTED_NEXT_PID(123, self.dest))
        wrong_listener = [
            SimpleNamespace(returncode=0, stdout=f"p123\nftxt\nn{expected}\n"),
            SimpleNamespace(returncode=0, stdout="p999\nf7\n"),
        ]
        with mock.patch.object(deploy.subprocess, "run", side_effect=wrong_listener):
            self.assertFalse(REAL_TRUSTED_NEXT_PID(123, self.dest))

    def test_wrong_live_core_executable_rejects_before_swap(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "trusted_next_core_pid", return_value=False):
            with self.assertRaisesRegex(ValueError, "PID, binary, listener"):
                deploy.main()
            deploy.run_checked.assert_not_called()

    def test_unhealthy_core_rejects_before_install(self):
        with mock.patch.object(deploy, "jobs", return_value={"dev.dropabit.agentdock.next.core": 123}), \
             mock.patch.object(deploy, "healthy_next_core", return_value=False), \
             mock.patch.object(deploy, "registrar") as registrar:
            with self.assertRaisesRegex(ValueError, "PID, binary, listener"):
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
