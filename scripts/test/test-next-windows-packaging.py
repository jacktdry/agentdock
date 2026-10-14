#!/usr/bin/env python3
"""Offline fixture: actual canonical Go verifier, no installer or runtime launch."""
import datetime
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "packaging/windows/prepare-next-rc.py"
spec = importlib.util.spec_from_file_location("next_windows", SCRIPT)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class InstallerSourceTests(unittest.TestCase):
    """Fast source-only gates; select this class to avoid the cold Go fixture."""

    def setUp(self):
        self.source = (ROOT / "packaging/windows/NextAgentDock.iss").read_text()
        self.payload = (ROOT / "packaging/windows/includes/next-payload.iss").read_text()
        self.code = (ROOT / "packaging/windows/includes/next-code.iss").read_text()

    def test_wpf_next_entrypoints_fail_before_stable_access(self):
        panel = ROOT / "desktop/windows/control-panel"
        identity = (panel / "Models/WindowsProductIdentity.cs").read_text()
        app = (panel / "App.xaml.cs").read_text()
        service = (panel / "Services/RuntimeService.cs").read_text()
        admin = (panel / "Services/TaskAdminService.cs").read_text()
        for value in (
            "AgentDock Next", "dev.dropabit.agentdock.next.controlpanel",
            ".agentdock-next", "8767", r"Local\AgentDock.Next.ControlPanel.Singleton",
            r"Local\AgentDock.Next.ControlPanel.Show",
        ):
            self.assertIn(value, identity)
        self.assertIn('null or "" or "stable" => new(false)', identity)
        self.assertIn('"next" => new(true)', identity)
        self.assertIn('_ => throw new InvalidOperationException(', identity)
        self.assertIn("if (IsNext)", identity)
        self.assertIn("throw new InvalidOperationException(", identity.split("if (IsNext)", 1)[1])
        self.assertIn('AppUserModelID: "dev.dropabit.agentdock.next.controlpanel"', self.source)
        gate = "WindowsProductIdentity.Current.RequireSupportedRuntime();"
        startup = app.split("protected override void OnStartup", 1)[1]
        for side_effect in (
            "UiText.ConfigureCurrentUICulture()", "SetCurrentProcessExplicitAppUserModelID(AppUserModelId)",
            "base.OnStartup(e)", "TaskAdminService.Run(e.Args)",
            "new Mutex(", "new RuntimeService()",
        ):
            self.assertLess(startup.index(gate), startup.index(side_effect), side_effect)
        self.assertIn("Shutdown(2);", startup[:startup.index("UiText.ConfigureCurrentUICulture()")])
        constructor = service.split("public RuntimeService(string? runtimeRoot = null)", 1)[1]
        self.assertLess(constructor.index(gate), constructor.index("ResolveRuntimeRoot()"))
        task = admin.split("internal static", 1)[1] if "internal static" in admin else admin
        self.assertLess(task.index(gate), task.index("Parse(arguments)"))

    def test_identity_and_architecture(self):
        app_id = re.search(r"^AppId=\{\{([^}]+)\}", self.source, re.M).group(1)
        self.assertEqual(app_id, "725CE7CB-6F4D-4D78-957E-64CF729FF413")
        self.assertNotEqual(app_id, "D6788C7A-4104-48D4-B5C3-F4858B5606EA")
        for line in (
            "AppName=AgentDock Next", "AppPublisher=AgentDock",
            "AppVersion={#NextVersion}", "VersionInfoVersion={#NextWindowsVersion}",
            "VersionInfoProductVersion={#NextWindowsVersion}",
            "OutputBaseFilename=AgentDock-NextSetup-{#NextArchitecture}",
            'NextArchitecture == "amd64"', 'NextArchitecture == "arm64"',
            '"x64compatible and not arm64"', "#error Unsupported NextArchitecture",
        ):
            self.assertIn(line, self.source)
        self.assertIn('AppUserModelID: "dev.dropabit.agentdock.next.controlpanel"', self.source)
        for icon in self.source.split("[Icons]\n")[1].split("#include")[0].strip().splitlines():
            self.assertIn('{userprograms}\\AgentDock Next\\', icon)

    def test_fixed_root_rejects_directory_overrides(self):
        for line in (
            r"DefaultDirName={localappdata}\AgentDock Next", "UsePreviousAppDir=no",
            "UsePreviousGroup=no", "DisableDirPage=yes", "PrivilegesRequired=lowest",
            "CloseApplications=no", "RestartApplications=no",
            r"UninstallFilesDir={app}\installer",
        ):
            self.assertIn(line, self.source)
        self.assertIn("if not IsNextRoot(WizardDirValue) then", self.code)
        self.assertIn("Result := IsNextRoot(ExpandConstant('{app}'))", self.code)
        self.assertIn("CompareText(RemoveBackslash(ExpandFileName(Path))", self.code)
        self.assertIn(r"ExpandConstant('{localappdata}\AgentDock Next')", self.code)

    def test_no_stable_calls_or_implicit_payload(self):
        includes = re.findall(r'^#include "([^"]+)"', self.source, re.M)
        self.assertEqual(includes, [r"includes\next-payload.iss", r"includes\next-code.iss"])
        all_source = "\n".join((self.source, self.payload, self.code)).lower()
        for forbidden in (
            "d6788c7a", "com.uvwt.agentdock.controlpanel", "agentdock.iss",
            "install.ps1", "uninstall-windows.ps1", "includes\\code.iss",
            "[run]", "[uninstallrun]", "[registry]", "[uninstalldelete]",
            "schtasks", "scheduledtask", "dpapi", "powershell", "exec(",
            "shellexec(", "regquery", "regwrite", "8765", "recursesubdirs",
        ):
            self.assertNotIn(forbidden, all_source)
        sources = re.findall(r'^Source: "([^"]+)"', self.payload, re.M)
        self.assertEqual(sources, ["{#NextPayloadDir}\\" + name for name in (
            "agentdock.exe", "agentdock-arbiter.exe", "agentdock-tray.exe", "candidate.json")])
        self.assertNotIn("*", self.payload)

    def test_direct_compilation_cannot_be_enabled_by_define(self):
        directives = [line for line in self.source.splitlines() if line.startswith("#")]
        self.assertTrue(directives[0].startswith("#error Next Windows installer disabled:"))
        self.assertNotIn("SignedBuild", self.source)

    def test_wrapper_blocks_before_manifest_output_and_tools(self):
        with tempfile.TemporaryDirectory(prefix="next-source-gate-") as tmp:
            for arch in ("amd64", "arm64"):
                out = Path(tmp) / arch
                result = subprocess.run([
                    os.sys.executable, "-B", str(SCRIPT), "--manifest", str(Path(tmp) / "missing.json"),
                    "--architecture", arch, "--output", str(out),
                    "--build-installer", "--build-binaries",
                ], cwd=tmp, env=dict(os.environ, PATH=""), text=True, capture_output=True)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("blocked before ISCC", result.stderr)
                self.assertFalse(out.exists())


class PayloadIntegrityTests(unittest.TestCase):
    """Synthetic bytes test rejection rules, never runtime/installer acceptance."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="next-integrity-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.payload = self.root / "payload"
        self.payload.mkdir()
        self.output = self.root / "fresh"
        self.manifest = self.root / "candidate.json"
        self.candidate = dict(version="0.9.1-rc.7", windows_version="0.9.1.7",
                              commit="a" * 40, build_date="2026-10-14T00:00:00Z",
                              channel="next-rc", update_feed="disabled/unpublished")
        data = json.dumps(self.candidate).encode()
        self.manifest.write_bytes(data)
        (self.payload / "candidate.json").write_bytes(data)
        self.record = dict(module.NEXT_IDENTITY, **self.candidate,
                           schema="agentdock-next-windows-payload-v1", architecture="amd64",
                           candidate_manifest_sha256=module.hashlib.sha256(data).hexdigest(), files={})
        for name in module.REQUIRED_PAYLOAD:
            path = self.payload / name
            path.parent.mkdir(parents=True, exist_ok=True)
            blob = b"fixture"
            if name.endswith(".exe"):
                blob = b"MZ" + bytes(58) + (64).to_bytes(4, "little") + b"PE\0\0\x64\x86"
            path.write_bytes(blob)
            self.record["files"][name] = module.hashlib.sha256(blob).hexdigest()
        self.save()

    def save(self):
        (self.payload / "next-payload.json").write_text(json.dumps(self.record))

    def verify(self):
        return module.verify_payload(self.manifest, self.payload, self.output, "amd64")

    def test_integrity_success_still_blocked_and_offline(self):
        with patch.object(module, "run", return_value=str(ROOT)) as run:
            result = self.verify()
        self.assertTrue(result["input_integrity_verified"])
        self.assertFalse(result["installable"])
        self.assertFalse(self.output.exists())
        args, root, env = run.call_args.args
        self.assertIn("next-candidate", args)
        self.assertIn("--check-output", args)
        self.assertEqual(env["GOPROXY"], "off")
        with patch.object(module, "run", side_effect=ValueError("canonical verifier rejection")):
            with self.assertRaisesRegex(ValueError, "canonical verifier rejection"):
                self.verify()

    def test_unsafe_windows_paths(self):
        for name in ("../escape", "/abs", "C:/escape", r"..\escape", "core:ads",
                     "CON.txt", "nul", "dir/..", "file.", "file ", "//host/share",
                     "AGENTDOCK.EXE", "candidate.json"):
            with self.subTest(name=name):
                self.record["files"][name] = "a" * 64
                self.save()
                with self.assertRaises(ValueError):
                    self.verify()
                del self.record["files"][name]
        self.save()

    def test_stable_identity_or_arch_manifest_rejected(self):
        for key, value in dict(product_name="AgentDock", aumid="com.uvwt.agentdock.controlpanel",
                               product_variant="stable", product_id="dev.dropabit.agentdock",
                               listen_port=8765, architecture="arm64", commit="b" * 40,
                               candidate_manifest_sha256="0" * 64).items():
            old = self.record[key]
            self.record[key] = value
            self.save()
            with self.subTest(key=key), self.assertRaises(ValueError):
                self.verify()
            self.record[key] = old
        self.save()
        (self.payload / "candidate.json").write_bytes(b"{}")
        with self.assertRaisesRegex(ValueError, "candidate bytes"):
            self.verify()

    def test_hash_pe_missing_extra_and_links_rejected(self):
        exe = self.payload / "agentdock.exe"
        original = exe.read_bytes()
        exe.write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "SHA-256"):
            self.verify()
        wrong_arch = original[:-2] + b"\x64\xaa"
        exe.write_bytes(wrong_arch)
        self.record["files"][exe.name] = module.hashlib.sha256(wrong_arch).hexdigest()
        self.save()
        with self.assertRaisesRegex(ValueError, "PE architecture"):
            self.verify()
        exe.write_bytes(original)
        self.record["files"][exe.name] = module.hashlib.sha256(original).hexdigest()
        self.save()
        extra = self.payload / "stable.zip"
        extra.write_bytes(b"stable fallback")
        with self.assertRaisesRegex(ValueError, "unlisted"):
            self.verify()
        extra.unlink()
        exe.unlink()
        with self.assertRaisesRegex(ValueError, "missing"):
            self.verify()
        exe.symlink_to(self.manifest)
        with self.assertRaisesRegex(ValueError, "symlink/reparse"):
            self.verify()

    def test_duplicate_json_and_existing_output_rejected(self):
        self.output.mkdir()
        with self.assertRaisesRegex(ValueError, "fresh"):
            self.verify()
        self.output.rmdir()
        (self.payload / "next-payload.json").write_text('{"files":{},"files":{}}')
        with self.assertRaisesRegex(ValueError, "duplicate JSON"):
            self.verify()


class PackagingFixture(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = tempfile.TemporaryDirectory(prefix="next-windows-")
        cls.repo = Path(cls.tmp.name).resolve() / "repo"
        subprocess.run(["git", "clone", "--quiet", "--no-hardlinks", str(ROOT), str(cls.repo)], check=True)
        cls.env = dict(os.environ, GOPROXY="off", GOTOOLCHAIN="local",
                       GOCACHE=os.environ.get("GOCACHE", str(Path(cls.tmp.name).resolve() / "go-cache")))
        commit = cls.command(["git", "rev-parse", "HEAD"]).strip()
        stamp = int(cls.command(["git", "show", "-s", "--format=%ct"]).strip())
        date = datetime.datetime.fromtimestamp(stamp, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        cls.canonical = cls.command(["go", "run", "-mod=readonly", "./tools/release", "next-preflight",
            "--version", "0.9.1-rc.7", "--tag", "next-v0.9.1-rc.7", "--commit", commit,
            "--build-date", date, "--bundle-build", "1"])
        cls.manifest = Path(cls.tmp.name).resolve() / "candidate.json"
        cls.manifest.write_text(cls.canonical, encoding="utf-8")

    @classmethod
    def tearDownClass(cls):
        cls.tmp.cleanup()

    @classmethod
    def command(cls, args):
        return subprocess.check_output(args, cwd=cls.repo, env=cls.env, text=True)

    def invoke(self, output, *args):
        return subprocess.run([os.sys.executable, str(SCRIPT), "--manifest", str(self.manifest),
            "--architecture", "arm64", "--output", str(output), *args],
            cwd=self.repo, env=self.env, text=True, capture_output=True)

    def test_metadata_and_provenance(self):
        out = self.repo / "dist/next-meta"
        result = self.invoke(out)
        self.assertEqual(result.returncode, 0, result.stderr)
        record = json.loads((out / "preparation.json").read_text())
        self.assertFalse(record["installable"])
        self.assertFalse(record["pe_version_info_applied"])
        self.assertEqual(record["windows_version"], "0.9.1.7")
        self.assertEqual(record["binary_checksums"], {})
        self.assertEqual((out / "candidate.json").read_bytes(), self.canonical.encode())
        self.assertEqual(record["candidate_manifest_sha256"], module.hashlib.sha256(self.canonical.encode()).hexdigest())
        for name in ("agentdock.exe", "agentdock-arbiter.exe"):
            resource = json.loads((out / (name + ".winres.json")).read_text())["RT_VERSION"]["#1"]["0000"]
            self.assertEqual(resource["fixed"]["file_version"], "0.9.1.7")
            self.assertEqual(resource["info"]["0409"]["ProductName"], "AgentDock Next")
            self.assertIn(record["commit"], resource["info"]["0409"]["Comments"])
        group = ET.parse(out / "next-version.props").find("PropertyGroup")
        self.assertEqual(group.findtext("AssemblyVersion"), "0.9.1.7")
        self.assertEqual(group.findtext("InformationalVersion"), "0.9.1-rc.7")
        self.assertNotEqual(self.invoke(out).returncode, 0)

    def test_payload_uses_real_canonical_verifier(self):
        fixture = PayloadIntegrityTests()
        fixture.setUp()
        try:
            fixture.manifest.write_text(self.canonical)
            (fixture.payload / "candidate.json").write_text(self.canonical)
            candidate = json.loads(self.canonical)
            for key in fixture.candidate:
                fixture.record[key] = candidate[key]
            fixture.record["candidate_manifest_sha256"] = module.hashlib.sha256(
                self.canonical.encode()).hexdigest()
            fixture.output = self.repo / "dist/verified-payload"
            fixture.save()
            original_run = module.run
            def in_clone(args, root, env=None):
                return original_run(args, self.repo, dict(self.env, **(env or {})))
            with patch.object(module, "run", side_effect=in_clone):
                self.assertTrue(fixture.verify()["input_integrity_verified"])
                fixture.manifest.write_text(json.dumps(candidate))
                (fixture.payload / "candidate.json").write_bytes(fixture.manifest.read_bytes())
                fixture.record["candidate_manifest_sha256"] = module.hashlib.sha256(
                    fixture.manifest.read_bytes()).hexdigest()
                fixture.save()
                with self.assertRaises(subprocess.CalledProcessError):
                    fixture.verify()
            self.assertFalse(fixture.output.exists())
        finally:
            fixture.doCleanups()

    def test_installer_fails_before_any_output_or_tool(self):
        out = self.repo / "dist/blocked"
        result = self.invoke(out, "--build-installer", "--build-binaries")
        self.assertEqual(result.returncode, 2)
        self.assertIn("blocked before ISCC", result.stderr)
        self.assertFalse(out.exists())

    def test_dirty_checkout_rejected(self):
        dirty = self.repo / "untracked-fixture"
        dirty.write_text("fixture")
        try:
            out = self.repo / "dist/dirty"
            result = self.invoke(out)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("clean checkout", result.stderr)
            self.assertFalse(out.exists())
        finally:
            dirty.unlink()

    def test_altered_manifest_rejected(self):
        for mutate in (
            lambda value: value.replace('"windows_version": "0.9.1.7"', '"windows_version": "0.9.1.0"'),
            lambda value: value.replace('"commit": "', '"commit": "f', 1),
            lambda value: value.replace('"release_ready": false', '"release_ready": true'),
            lambda value: value[:-2] + ', "unknown": 1}\n',
            lambda value: json.dumps(json.loads(value)),
        ):
            self.manifest.write_text(mutate(self.canonical))
            try:
                out = self.repo / "dist/invalid"
                self.assertNotEqual(self.invoke(out).returncode, 0)
                self.assertFalse(out.exists())
            finally:
                self.manifest.write_text(self.canonical)

    def test_outside_or_symlink_output_rejected(self):
        outside = Path(self.tmp.name).resolve() / "outside"
        self.assertNotEqual(self.invoke(outside).returncode, 0)
        self.assertFalse(outside.exists())
        (self.repo / "dist").mkdir(exist_ok=True)
        link = self.repo / "dist/link"
        link.symlink_to(Path(self.tmp.name).resolve(), target_is_directory=True)
        self.assertNotEqual(self.invoke(link / "escape").returncode, 0)
        self.assertFalse((Path(self.tmp.name) / "escape").exists())

    @unittest.skipUnless(os.environ.get("NEXT_WINDOWS_BUILD_SMOKE") == "1", "opt-in crossbuild")
    def test_windows_crossbuild_without_launch(self):
        out = self.repo / "dist/build-smoke"
        result = self.invoke(out)
        self.assertEqual(result.returncode, 0, result.stderr)
        manifest = json.loads(self.canonical)
        flags = "-X github.com/uvwt/agentdock/internal/buildinfo.Commit={} -X github.com/uvwt/agentdock/internal/buildinfo.BuildDate={}".format(
            manifest["commit"], manifest["build_date"])
        for architecture in ("amd64", "arm64"):
            env = dict(self.env, GOOS="windows", GOARCH=architecture, CGO_ENABLED="0")
            for name in ("agentdock", "agentdock-arbiter"):
                binary = out / (name + "-" + architecture + ".exe")
                subprocess.run(["go", "build", "-mod=readonly", "-overlay", str(out / "overlay.json"),
                    "-ldflags", flags, "-o", str(binary), "./cmd/" + name], cwd=self.repo, env=env, check=True)
                self.assertEqual(binary.read_bytes()[:2], b"MZ")
                info = self.command(["go", "version", "-m", str(binary)])
                self.assertIn("GOOS=windows", info)
                self.assertIn("GOARCH=" + architecture, info)
                self.assertIn(manifest["commit"], info)
                self.assertIn(manifest["build_date"], info)


if __name__ == "__main__":
    unittest.main()
