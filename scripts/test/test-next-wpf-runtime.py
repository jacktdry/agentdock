#!/usr/bin/env python3
"""Source regression checks; --native executes the actual validator on Windows/.NET 8."""
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
PANEL = ROOT / "desktop/windows/control-panel"
VALIDATOR = PANEL / "Models/NextWindowsManifestIdentity.cs"
BASE = "8caf0995d5ca773bbe1b0f167cdad273ecc2f95e"

# No Python reimplementation of admission. These cases execute the production C#.
HARNESS = r'''
using System.Text.Json;
using AgentDock.ControlPanel;
var local = @"C:\Users\NextTest\AppData\Local";
var user = @"C:\Users\NextTest";
var fields = new Dictionary<string, object> {
    ["product_variant"] = "next", ["product_id"] = "dev.dropabit.agentdock.next",
    ["install_root"] = local + @"\AgentDock Next",
    ["state_directory"] = user + @"\.agentdock-next",
    ["agentdock_home"] = user + @"\.agentdock-next",
    ["agentdock_default_dir"] = user + @"\AgentDock Next",
    ["listen_port"] = 8767, ["agentdock_task_name"] = "AgentDockNext",
    ["startup_value_name"] = "AgentDockNext",
    ["tray_startup_value_name"] = "AgentDockNextTray",
    ["cloudflared_startup_value_name"] = "AgentDockNextCloudflared"
};
var checks = 0;
void Check(string json, bool accepted, string? context = null) {
    bool actual;
    try { NextWindowsManifestIdentity.Validate(json, context ?? local, user); actual = true; }
    catch (Exception e) when (e is InvalidOperationException or JsonException or ArgumentException) { actual = false; }
    if (actual != accepted) throw new Exception("Unexpected admission: " + json);
    checks++;
}
var valid = JsonSerializer.Serialize(fields);
Check(valid, true);
foreach (var key in fields.Keys) {
    var changed = new Dictionary<string, object>(fields);
    changed.Remove(key); Check(JsonSerializer.Serialize(changed), false);
    changed[key] = null!; Check(JsonSerializer.Serialize(changed), false);
    changed[key] = key == "listen_port" ? 8765 : "AgentDock";
    Check(JsonSerializer.Serialize(changed), false);
    changed = new(fields);
    changed[key.ToUpperInvariant()] = fields[key]; Check(JsonSerializer.Serialize(changed), false);
}
foreach (var json in new[] { "", "[]", "null", "{}", "{", "\uFEFF" + valid,
    valid[..^1] + ",}", valid[..^1] + ",\"product_variant\":\"next\"}",
    valid[..^1] + ",\"port\":8765}", valid.Replace("8767", "8767.0"),
    valid.Replace("8767", "\"8767\""), valid.Replace("\"next\"", "\"Next\"") })
    Check(json, false);
foreach (var alias in new[] { local + @"\..\Local", local.Replace('\\', '/'),
    @"C:\Users\NEXTTE~1\AppData\Local", local + " ", local + ".", local + @"\",
    @"\\server\Local", @"\\?\C:\Users\NextTest\AppData\Local", local + ":stream" })
    Check(valid, false, alias);
foreach (var key in new[] { "install_root", "state_directory", "agentdock_home", "agentdock_default_dir" }) {
    foreach (var alias in new[] { fields[key] + @"\..", fields[key] + @"\", fields[key] + ".",
        fields[key].ToString()!.Replace('\\', '/'), fields[key].ToString()!.ToUpperInvariant() }) {
        var changed = new Dictionary<string, object>(fields); changed[key] = alias;
        Check(JsonSerializer.Serialize(changed), false);
    }
}
Console.WriteLine($"PASS: {checks} actual C# identity cases (no runtime activation)");
'''


class SourceTests(unittest.TestCase):
    def test_existing_stable_and_early_next_gates_unchanged(self):
        for relative in ("App.xaml.cs", "Models/WindowsProductIdentity.cs",
                         "Services/RuntimeService.cs", "Services/TaskAdminService.cs"):
            path = PANEL / relative
            baseline = subprocess.check_output(
                ["git", "show", f"{BASE}:{path.relative_to(ROOT)}"], cwd=ROOT)
            self.assertEqual(path.read_bytes(), baseline, relative)

    def test_validator_has_no_runtime_or_mutation_dependencies(self):
        source = VALIDATOR.read_text()
        for forbidden in ("RuntimeService(", "RuntimeManifest", "Process.Start(",
                          "HttpClient", "Registry.", "File.Write", "Directory.Create",
                          "GetEnvironmentVariable", "JsonSerializer.Deserialize"):
            # RuntimeManifest appears only in an explanatory comment.
            code = "\n".join(line.split("//")[0] for line in source.splitlines())
            self.assertNotIn(forbidden, code)

    def test_closed_schema_and_no_alias_normalization(self):
        source = VALIDATOR.read_text()
        for required in ('StringComparer.OrdinalIgnoreCase', 'seen.Add(property.Name)',
                         'expected.TryGetValue(property.Name', 'JsonValueKind.String',
                         'GetRawText() != "8767"', 'expected.Count + 1',
                         'FileAttributes.ReparsePoint', 'File.GetAttributes(current)',
                         'RequireCanonicalPath(path)', 'Path.GetFullPath(path), path',
                         'OperatingSystem.IsWindows()'):
            self.assertIn(required, source)

    def test_native_fixture_covers_every_required_identity_field(self):
        source = VALIDATOR.read_text()
        for field in ('product_variant', 'product_id', 'install_root', 'state_directory',
                      'agentdock_home', 'agentdock_default_dir', 'listen_port',
                      'agentdock_task_name', 'startup_value_name', 'tray_startup_value_name',
                      'cloudflared_startup_value_name'):
            self.assertIn(f'"{field}"', source)
            self.assertIn(f'"{field}"', HARNESS)
        self.assertIn('changed.Remove(key)', HARNESS)
        self.assertIn('key.ToUpperInvariant()', HARNESS)


def native():
    if sys.platform != "win32":
        raise SystemExit("--native requires Windows and .NET 8; no tests were run")
    with tempfile.TemporaryDirectory(prefix="agentdock-next-identity-") as directory:
        path = pathlib.Path(directory)
        (path / "Validator.cs").write_bytes(VALIDATOR.read_bytes())
        (path / "Program.cs").write_text(HARNESS)
        (path / "NuGet.Config").write_text('<configuration><packageSources><clear /></packageSources></configuration>')
        (path / "Identity.csproj").write_text('''<Project Sdk="Microsoft.NET.Sdk">
<PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework>
<ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup></Project>''')
        subprocess.run(["dotnet", "run", "--project", str(path / "Identity.csproj")],
                       cwd=path, check=True)


if __name__ == "__main__":
    if sys.argv[1:] == ["--native"]:
        native()
    else:
        unittest.main()
