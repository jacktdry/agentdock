import Foundation

@main
struct LocalizationPreferenceTests {
    static func main() {
        let suiteName = "AgentDock.LocalizationPreferenceTests.\(UUID().uuidString)"
        guard let defaults = UserDefaults(suiteName: suiteName) else {
            preconditionFailure("failed to create isolated UserDefaults")
        }
        defer { defaults.removePersistentDomain(forName: suiteName) }

        precondition(L10n.languagePreference(defaults: defaults) == .system)

        guard let english = UILanguagePreference(rawValue: "en"),
              let simplifiedChinese = UILanguagePreference(rawValue: "zh-Hans"),
              let traditionalChinese = UILanguagePreference(rawValue: "zh-Hant") else {
            preconditionFailure("generated locale manifest is missing the M3 baseline locales")
        }

        L10n.setLanguagePreference(english, defaults: defaults)
        precondition(L10n.languagePreference(defaults: defaults) == english)

        L10n.setLanguagePreference(simplifiedChinese, defaults: defaults)
        precondition(L10n.languagePreference(defaults: defaults) == simplifiedChinese)

        // Legacy platform tags are normalized through the generated manifest aliases.
        defaults.set("zh-TW", forKey: "AgentDockUILanguage")
        precondition(L10n.languagePreference(defaults: defaults) == traditionalChinese)

        precondition(UILanguagePreference.allCases.count == GeneratedLocales.supported.count + 1)
        precondition(traditionalChinese.title == "繁體中文")

        L10n.setLanguagePreference(english, defaults: defaults)
        L10n.setLanguagePreference(.system, defaults: defaults)
        precondition(L10n.languagePreference(defaults: defaults) == .system)
        precondition(defaults.object(forKey: "AgentDockUILanguage") == nil)

        precondition(L10n.text("Change interface language?") != "")
        precondition(L10n.text("Changing the interface language restarts the AgentDock interface. Any unsaved changes in this window will be lost.") != "")
        precondition(L10n.text("Continue") != "")

        print("localization preference tests passed")
    }
}
