import Foundation

enum UILanguagePreference: String, CaseIterable {
    case system
    case simplifiedChinese = "zh-Hans"
    case traditionalChinese = "zh-Hant"
    case english = "en"

    var title: String {
        switch self {
        case .system:
            return L10n.text("Follow system")
        case .simplifiedChinese:
            return L10n.text("Simplified Chinese")
        case .traditionalChinese:
            return L10n.text("Traditional Chinese")
        case .english:
            return L10n.text("English")
        }
    }
}

enum L10n {
    private static let languagePreferenceKey = "AgentDockUILanguage"

    static func languagePreference(defaults: UserDefaults = .standard) -> UILanguagePreference {
        guard let rawValue = defaults.string(forKey: languagePreferenceKey) else {
            return .system
        }
        if let preference = UILanguagePreference(rawValue: rawValue) {
            return preference
        }
        switch rawValue.lowercased() {
        case "zh-tw", "zh-hk", "zh-mo", "zh-hant-tw", "zh-hant-hk", "zh-hant-mo":
            return .traditionalChinese
        case "zh-cn", "zh-sg", "zh-hans-cn", "zh-hans-sg":
            return .simplifiedChinese
        default:
            return .system
        }
    }

    static func setLanguagePreference(_ preference: UILanguagePreference, defaults: UserDefaults = .standard) {
        if preference == .system {
            defaults.removeObject(forKey: languagePreferenceKey)
        } else {
            defaults.set(preference.rawValue, forKey: languagePreferenceKey)
        }
    }

    static func text(_ key: String) -> String {
        NSLocalizedString(
            key,
            tableName: nil,
            bundle: localizationBundle(for: languagePreference()),
            value: key,
            comment: ""
        )
    }

    static func format(_ key: String, _ arguments: CVarArg...) -> String {
        String(format: text(key), locale: Locale.current, arguments: arguments)
    }

    private static func localizationBundle(for preference: UILanguagePreference) -> Bundle {
        guard preference != .system,
              let path = Bundle.main.path(forResource: preference.rawValue, ofType: "lproj"),
              let bundle = Bundle(path: path) else {
            return .main
        }
        return bundle
    }
}
