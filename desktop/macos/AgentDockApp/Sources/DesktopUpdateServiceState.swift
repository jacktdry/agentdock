import Darwin
import Foundation

struct DesktopUpdateServiceState: Codable {
    static let schemaVersion = 1

    let schemaVersion: Int
    let variant: String?
    let coreEnabled: Bool
    let tunnelEnabled: Bool

    private enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case variant
        case coreEnabled = "core_enabled"
        case tunnelEnabled = "tunnel_enabled"
    }

    init(coreEnabled: Bool, tunnelEnabled: Bool, identity: AppIdentity = .stable) {
        variant = identity == .next ? identity.rawValue : nil
        schemaVersion = Self.schemaVersion
        self.coreEnabled = coreEnabled
        self.tunnelEnabled = tunnelEnabled
    }

    static func load(from path: URL, identity: AppIdentity = .stable) throws -> DesktopUpdateServiceState? {
        guard FileManager.default.fileExists(atPath: path.path) else { return nil }
        let data = try Data(contentsOf: path)
        let state = try JSONDecoder().decode(DesktopUpdateServiceState.self, from: data)
        guard state.schemaVersion == Self.schemaVersion,
              state.variant == identity.rawValue || (identity == .stable && state.variant == nil) else {
            throw ValidationError(L10n.text("The AgentDock update service state version is not supported."))
        }
        return state
    }

    func write(to path: URL) throws {
        let fileManager = FileManager.default
        let directory = path.deletingLastPathComponent()
        try fileManager.createDirectory(at: directory, withIntermediateDirectories: true)
        try fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: directory.path)

        var data = try JSONEncoder().encode(self)
        data.append(0x0a)
        let temporary = directory.appendingPathComponent(".\(path.lastPathComponent).tmp.\(UUID().uuidString)")
        defer { try? fileManager.removeItem(at: temporary) }
        guard fileManager.createFile(atPath: temporary.path, contents: data, attributes: [.posixPermissions: 0o600]) else {
            throw ValidationError(L10n.text("Unable to create AgentDock update service state."))
        }
        try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: temporary.path)
        if Darwin.rename(temporary.path, path.path) != 0 {
            throw ValidationError(L10n.format(
                "Unable to save AgentDock update service state: %@",
                String(cString: strerror(errno))
            ))
        }
    }

    static func remove(at path: URL) {
        try? FileManager.default.removeItem(at: path)
    }
}
