import Foundation

struct UILanguagePreference: RawRepresentable, CaseIterable, Equatable {
    let rawValue: String

    static let system = UILanguagePreference(rawValue: "system")!

    static var allCases: [UILanguagePreference] {
        [.system] + GeneratedLocales.supported.compactMap { UILanguagePreference(rawValue: $0.code) }
    }

    init?(rawValue: String) {
        if rawValue == "system" || GeneratedLocales.supported.contains(where: { $0.code == rawValue }) {
            self.rawValue = rawValue
            return
        }

        if let locale = GeneratedLocales.supported.first(where: { descriptor in
            descriptor.aliases.contains(where: { $0.caseInsensitiveCompare(rawValue) == .orderedSame })
                || descriptor.macOSResource.caseInsensitiveCompare(rawValue) == .orderedSame
        }) {
            self.rawValue = locale.code
            return
        }

        return nil
    }

    var title: String {
        guard self != .system,
              let descriptor = GeneratedLocales.supported.first(where: { $0.code == rawValue }) else {
            return L10n.text("Follow system")
        }
        return descriptor.nativeName
    }

    var macOSResource: String? {
        GeneratedLocales.supported.first(where: { $0.code == rawValue })?.macOSResource
    }
}

enum L10n {
    private static let languagePreferenceKey = "AgentDockUILanguage"

    static func languagePreference(defaults: UserDefaults = .standard) -> UILanguagePreference {
        guard let rawValue = defaults.string(forKey: languagePreferenceKey),
              let preference = UILanguagePreference(rawValue: rawValue) else {
            return .system
        }
        return preference
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
              let resource = preference.macOSResource,
              let path = Bundle.main.path(forResource: resource, ofType: "lproj"),
              let bundle = Bundle(path: path) else {
            return .main
        }
        return bundle
    }
}
