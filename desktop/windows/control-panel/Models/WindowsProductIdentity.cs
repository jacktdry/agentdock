using System.IO;

namespace AgentDock.ControlPanel;

// Selected once per process, before startup helpers or runtime discovery.
internal sealed class WindowsProductIdentity
{
    private static readonly Lazy<WindowsProductIdentity> Selected = new(() =>
        FromVariant(Environment.GetEnvironmentVariable("AGENTDOCK_DESKTOP_VARIANT")));

    internal static WindowsProductIdentity Current => Selected.Value;
    internal bool IsNext { get; }
    internal string ProductName => IsNext ? "AgentDock Next" : "AgentDock";
    internal string MutexName => IsNext
        ? @"Local\AgentDock.Next.ControlPanel.Singleton"
        : @"Local\AgentDock.ControlPanel.Singleton";
    internal string ShowEventName => IsNext
        ? @"Local\AgentDock.Next.ControlPanel.Show"
        : @"Local\AgentDock.ControlPanel.Show";
    internal string AppUserModelId => IsNext
        ? "dev.dropabit.agentdock.next.controlpanel"
        : "com.uvwt.agentdock.controlpanel";
    internal string StateDirectoryName => IsNext ? ".agentdock-next" : ".agentdock";
    internal int DefaultPort => IsNext ? 8767 : 8765;
    internal string DefaultRuntimeRoot => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), ProductName);

    private WindowsProductIdentity(bool isNext) => IsNext = isNext;

    internal static WindowsProductIdentity FromVariant(string? variant) => variant switch
    {
        null or "" or "stable" => new(false),
        "next" => new(true),
        _ => throw new InvalidOperationException(
            "Invalid AGENTDOCK_DESKTOP_VARIANT; expected unset, stable or next (exact lowercase).")
    };

    internal void RequireSupportedRuntime()
    {
        if (IsNext)
        {
            // The current manifest has Stable defaults and no product/state identity.
            // Do not inspect an override, executable ancestor or legacy manifest as Next.
            throw new InvalidOperationException(
                $"{ProductName} Windows runtime is unavailable: a verified Next runtime.json contract " +
                $"for {DefaultRuntimeRoot}, {StateDirectoryName} and port {DefaultPort} is not implemented. " +
                "Startup, task administration, elevated helpers and updates are blocked; " +
                "no Stable runtime fallback is permitted.");
        }
    }
}
