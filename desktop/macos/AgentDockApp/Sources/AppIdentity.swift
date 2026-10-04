import Foundation

// Identity is selected only by validated bundle metadata, never by ambient environment.
enum AppIdentity: String, CaseIterable {
    case stable, next

    var name: String { self == .stable ? "AgentDock" : "AgentDock Next" }
    var bundleID: String { self == .stable ? "com.uvwt.agentdock" : "dev.dropabit.agentdock.next" }
    var appFilename: String { name + ".app" }
    var coreLabel: String { bundleID + ".core" }
    var tunnelLabel: String { bundleID + ".tunnel" }
    var menuLabel: String { bundleID + ".menu-login" }
    var corePlistName: String { coreLabel + ".plist" }
    var tunnelPlistName: String { tunnelLabel + ".plist" }
    var menuPlistName: String { menuLabel + ".plist" }
    var stateName: String { self == .stable ? ".agentdock" : ".agentdock-next" }
    var defaultPort: Int { self == .stable ? 8765 : 8767 }
    var allowsLegacyMigration: Bool { self == .stable }

    struct InvalidMetadata: Error {}

    static func load(metadata: [String: Any]) throws -> AppIdentity {
        // Older stable bundles have no variant marker. A missing marker is never
        // sufficient to infer stable: the stable bundle identifier must match.
        let marker = metadata["AgentDockVariant"] as? String
        let identity: AppIdentity
        if marker == nil, metadata["AgentDockVariant"] == nil,
           metadata["CFBundleIdentifier"] as? String == AppIdentity.stable.bundleID {
            identity = .stable
        } else if let marker, let parsed = AppIdentity(rawValue: marker) {
            identity = parsed
        } else {
            throw InvalidMetadata()
        }
        guard metadata["CFBundleIdentifier"] as? String == identity.bundleID,
              metadata["CFBundleName"] as? String == identity.name,
              metadata["CFBundleDisplayName"] as? String == identity.name else {
            throw InvalidMetadata()
        }
        return identity
    }

    static let current: AppIdentity = {
        do { return try load(metadata: Bundle.main.infoDictionary ?? [:]) }
        catch { fatalError("Invalid AgentDock bundle identity; refusing runtime access") }
    }()
}
