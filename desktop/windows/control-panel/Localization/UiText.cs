using System.Globalization;
using System.IO;
using System.Resources;
using System.Windows.Markup;

namespace AgentDock.ControlPanel;

internal static class UiText
{
    internal const string SystemPreference = "system";

    private static readonly string SystemLocale = NormalizeCultureName(CultureInfo.CurrentUICulture.Name);
    private static readonly ResourceManager Resources = new(
        "AgentDock.ControlPanel.Resources.UiStrings",
        typeof(UiText).Assembly);
    private static CultureInfo _resourceCulture = CultureInfo.GetCultureInfo(SystemLocale);

    private static string PreferencePath => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
        "AgentDock",
        "ui-language");

    public static void ConfigureCurrentUICulture()
    {
        ApplyPreference(ReadPreference());
    }

    internal static string ReadPreference()
    {
        try
        {
            return NormalizePreference(File.ReadAllText(PreferencePath));
        }
        catch (IOException)
        {
            return SystemPreference;
        }
        catch (UnauthorizedAccessException)
        {
            return SystemPreference;
        }
    }

    internal static void SetPreference(string preference)
    {
        var normalized = NormalizePreference(preference);
        if (normalized == SystemPreference)
        {
            File.Delete(PreferencePath);
            ApplyPreference(normalized);
            return;
        }

        var directory = Path.GetDirectoryName(PreferencePath)
            ?? throw new InvalidOperationException("AgentDock UI preference directory is unavailable.");
        Directory.CreateDirectory(directory);
        File.WriteAllText(PreferencePath, normalized);
        ApplyPreference(normalized);
    }

    internal static string NormalizePreference(string? value)
    {
        var candidate = value?.Trim();
        if (string.IsNullOrEmpty(candidate) || string.Equals(candidate, SystemPreference, StringComparison.OrdinalIgnoreCase))
        {
            return SystemPreference;
        }

        return FindLocale(candidate)?.Code ?? SystemPreference;
    }

    internal static string ResolveLocale(string preference, string systemCultureName)
    {
        var normalized = NormalizePreference(preference);
        if (normalized == SystemPreference)
        {
            return NormalizeCultureName(systemCultureName);
        }

        return FindLocale(normalized)?.WindowsResource ?? SourceLocale().WindowsResource;
    }

    internal static string NormalizeCultureName(string? value)
    {
        var candidate = value?.Trim();
        if (string.IsNullOrEmpty(candidate))
        {
            return SourceLocale().WindowsResource;
        }

        return FindLocale(candidate)?.WindowsResource ?? SourceLocale().WindowsResource;
    }

    public static string Get(string key)
    {
        return Resources.GetString(key, _resourceCulture) ?? key;
    }

    public static string Format(string key, params object?[] args)
    {
        return string.Format(CultureInfo.CurrentCulture, Get(key), args);
    }

    private static GeneratedLocaleDescriptor SourceLocale() =>
        GeneratedLocales.Supported.First(locale => locale.Code == GeneratedLocales.SourceLocale);

    private static GeneratedLocaleDescriptor? FindLocale(string value)
    {
        static bool Matches(string candidate, string value) =>
            string.Equals(candidate, value, StringComparison.OrdinalIgnoreCase)
            || value.StartsWith(candidate + "-", StringComparison.OrdinalIgnoreCase);

        return GeneratedLocales.Supported.FirstOrDefault(locale =>
            Matches(locale.Code, value)
            || Matches(locale.WindowsResource, value)
            || locale.Aliases.Any(alias => Matches(alias, value)));
    }

    private static void ApplyPreference(string preference)
    {
        var locale = ResolveLocale(preference, SystemLocale);
        var culture = CultureInfo.GetCultureInfo(locale);
        // Use an explicit culture so async continuations cannot silently restore stale UI-language state.
        _resourceCulture = culture;
        CultureInfo.CurrentUICulture = culture;
        CultureInfo.DefaultThreadCurrentUICulture = culture;
    }
}

[MarkupExtensionReturnType(typeof(string))]
internal sealed class LocExtension : MarkupExtension
{
    public LocExtension(string key)
    {
        Key = key;
    }

    [ConstructorArgument("key")]
    public string Key { get; set; }

    public override object ProvideValue(IServiceProvider serviceProvider)
    {
        return UiText.Get(Key);
    }
}
