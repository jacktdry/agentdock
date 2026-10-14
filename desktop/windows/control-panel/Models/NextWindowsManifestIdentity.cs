using System.IO;
using System.Text.Json;

namespace AgentDock.ControlPanel;

// Declarative preflight only. Success is not permission to construct RuntimeService,
// install, activate, manage tasks, probe health or update this runtime.
internal static class NextWindowsManifestIdentity
{
    internal const string ProductId = "dev.dropabit.agentdock.next";
    internal const string TaskName = "AgentDockNext";
    internal const string TrayStartupName = "AgentDockNextTray";
    internal const string TunnelStartupName = "AgentDockNextCloudflared";

    internal static void Validate(string json, string localAppData, string userProfile)
    {
        if (!OperatingSystem.IsWindows())
            throw new PlatformNotSupportedException("Next Windows identity requires Windows path semantics.");

        var installRoot = Path.Combine(RequireCanonicalPath(localAppData), "AgentDock Next");
        var stateRoot = Path.Combine(RequireCanonicalPath(userProfile), ".agentdock-next");
        var workspace = Path.Combine(userProfile, "AgentDock Next");
        var expected = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["product_variant"] = "next",
            ["product_id"] = ProductId,
            ["install_root"] = installRoot,
            ["state_directory"] = stateRoot,
            ["agentdock_home"] = stateRoot,
            ["agentdock_default_dir"] = workspace,
            ["agentdock_task_name"] = TaskName,
            ["startup_value_name"] = TaskName,
            ["tray_startup_value_name"] = TrayStartupName,
            ["cloudflared_startup_value_name"] = TunnelStartupName
        };

        // JsonDocument rejects comments, trailing commas and malformed JSON. Never
        // deserialize into RuntimeManifest: its defaults/case-insensitivity are Stable.
        using var document = JsonDocument.Parse(json);
        if (document.RootElement.ValueKind != JsonValueKind.Object)
            throw new InvalidOperationException("Next identity must be an object.");
        var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (var property in document.RootElement.EnumerateObject())
        {
            if (!seen.Add(property.Name))
                throw new InvalidOperationException("Duplicate Next identity field: " + property.Name);
            if (property.Name == "listen_port")
            {
                if (property.Value.ValueKind != JsonValueKind.Number ||
                    property.Value.GetRawText() != "8767")
                    throw new InvalidOperationException("Next listen_port must be integer 8767.");
                continue;
            }
            if (!expected.TryGetValue(property.Name, out var value) ||
                property.Value.ValueKind != JsonValueKind.String ||
                !string.Equals(property.Value.GetString(), value, StringComparison.Ordinal))
                throw new InvalidOperationException("Unexpected or conflicting Next identity field: " + property.Name);
        }
        if (seen.Count != expected.Count + 1 || !seen.Contains("listen_port") ||
            expected.Keys.Any(key => !seen.Contains(key)))
            throw new InvalidOperationException("Next identity is incomplete.");
    }

    // Separate filesystem preflight for a future installer. Requires existing paths;
    // it never creates/repairs them or searches ancestors for another runtime.
    // This does not establish ACL ownership, payload provenance or race-free activation.
    internal static void RequireExistingNonAliasedPath(string path, bool directory)
    {
        if (!OperatingSystem.IsWindows())
            throw new PlatformNotSupportedException("Next Windows paths require Windows.");
        RequireCanonicalPath(path);
        var current = path;
        var leaf = true;
        while (true)
        {
            var attributes = File.GetAttributes(current); // Missing/inaccessible paths fail.
            if ((attributes & FileAttributes.ReparsePoint) != 0)
                throw new InvalidOperationException("Next path contains a reparse point.");
            if (((attributes & FileAttributes.Directory) != 0) != (!leaf || directory))
                throw new InvalidOperationException("Unexpected Next path type.");
            var parent = Path.GetDirectoryName(current);
            if (string.IsNullOrEmpty(parent)) break;
            current = parent;
            leaf = false;
        }
    }

    private static string RequireCanonicalPath(string path)
    {
        // Fixed local drive spelling only: no UNC/device paths, ADS, short names,
        // dot segments, slash aliases, trailing separators/spaces/dots or rebasing.
        if (string.IsNullOrEmpty(path) || path.Length < 4 ||
            !char.IsAsciiLetter(path[0]) || path[1] != ':' || path[2] != '\\' ||
            path.Contains('/') || path.Contains('~') || path[3..].Contains(':') ||
            path.Any(character => char.IsControl(character) || "\"<>|?*".Contains(character)) ||
            path[3..].Split('\\').Any(segment => string.IsNullOrEmpty(segment) ||
                segment is "." or ".." || segment.EndsWith(' ') || segment.EndsWith('.')) ||
            !string.Equals(Path.GetFullPath(path), path, StringComparison.Ordinal))
            throw new InvalidOperationException("Next path must use an exact canonical local drive path.");
        return path;
    }
}
