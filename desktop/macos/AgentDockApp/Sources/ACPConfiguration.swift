import Foundation

struct ACPAdapterResolution: Equatable {
    let available: Bool
    let command: String
    let arguments: [String]
    let message: String
}

struct ACPAdapterPackageStatus: Equatable {
    let available: Bool
    let installedVersion: String?
    let latestVersion: String?
    let updateAvailable: Bool?
}

private struct ACPNodePackage {
    let name: String
    let binName: String
}

enum ACPAgentPreset: String, CaseIterable, Codable {
    case codex
    case claude
    case grok
    case antigravity
    case custom

    static let builtInCases: [ACPAgentPreset] = [.codex, .claude, .grok, .antigravity]

    var title: String {
        switch self {
        case .codex: return "Codex"
        case .claude: return "Claude"
        case .grok: return "Grok Build"
        case .antigravity: return "Antigravity"
        case .custom: return L10n.text("Custom")
        }
    }

    var executableNames: [String] {
        switch self {
        case .codex: return ["codex-acp"]
        case .claude: return ["claude-agent-acp"]
        case .grok: return ["grok"]
        case .antigravity: return ["refined-antigravity-acp"]
        case .custom: return []
        }
    }

    var arguments: [String] {
        switch self {
        case .grok: return ["agent", "stdio"]
        case .codex, .claude, .antigravity, .custom: return []
        }
    }

    private var nodePackage: ACPNodePackage? {
        switch self {
        case .codex:
            return ACPNodePackage(name: "@agentclientprotocol/codex-acp", binName: "codex-acp")
        case .claude:
            return ACPNodePackage(name: "@agentclientprotocol/claude-agent-acp", binName: "claude-agent-acp")
        case .antigravity:
            return ACPNodePackage(name: "@simonepri/refined-antigravity-acp", binName: "refined-antigravity-acp")
        case .grok, .custom:
            return nil
        }
    }

    var npmPackageName: String? {
        nodePackage?.name
    }

    var installHint: String {
        switch self {
        case .codex:
            return "npm install -g @agentclientprotocol/codex-acp"
        case .claude:
            return "npm install -g @agentclientprotocol/claude-agent-acp"
        case .antigravity:
            return "pnpm add -g @simonepri/refined-antigravity-acp && refined-antigravity-acp setup"
        case .grok:
            return L10n.text("Install the Grok CLI from x.ai")
        case .custom:
            return L10n.text("Enter the absolute path to an executable ACP Adapter")
        }
    }

    var missingAdapterMessage: String {
        if self == .custom {
            return L10n.text("Enter the absolute path to an executable ACP Adapter")
        }
        guard let nodePackage else {
            return L10n.format("Could not find %@", executableNames[0])
        }
        return L10n.format("Could not find %@ or %@", executableNames[0], nodePackage.name)
    }

    static func parse(_ raw: String) -> ACPAgentPreset? {
        ACPAgentPreset(rawValue: raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased())
    }

    func resolveAdapter(
        configuredCommand: String = "",
        configuredArguments: [String] = [],
        home: URL = FileManager.default.homeDirectoryForCurrentUser,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) -> ACPAdapterResolution {
        let directories = searchDirectories(home: home, environment: environment)
        let configured = resolveConfiguredAdapter(
            command: configuredCommand,
            arguments: configuredArguments,
            directories: directories
        )
        if self == .custom {
            return configured ?? ACPAdapterResolution(
                available: false,
                command: "",
                arguments: [],
                message: L10n.format("Not configured · %@", missingAdapterMessage)
            )
        }

        var discovered: ACPAdapterResolution?
        for directory in directories {
            for executableName in executableNames {
                let candidate = directory.appendingPathComponent(executableName)
                if let executable = executableFile(candidate),
                   let resolution = resolveExecutableAdapter(
                       executable,
                       arguments: arguments,
                       directories: directories
                   ) {
                    discovered = resolution
                    break
                }
            }
            if discovered != nil { break }
        }

        if discovered == nil,
           let nodePackage,
           let node = resolveNodeExecutable(directories: directories) {
            for packageRoot in npmPackageRoots(nodePackage, directories: directories) {
                guard let entry = readNPMBinEntry(
                    packageRoot: packageRoot,
                    binName: nodePackage.binName
                ) else {
                    continue
                }
                let resolvedArguments = [entry.path] + arguments
                discovered = ACPAdapterResolution(
                    available: true,
                    command: node.path,
                    arguments: resolvedArguments,
                    message: L10n.format("Detected · %@ · %@", node.path, entry.path)
                )
                break
            }
        }

        if let configured {
            // 旧版曾把 ~/.local/bin/grok、Homebrew Node 等稳定 symlink 保存成版本化 target。
            // 只有重新发现的入口与既有配置实际指向同一组文件时才换回稳定路径，避免覆盖手工配置。
            if let discovered, equivalentAdapterFiles(configured, discovered) {
                return discovered
            }
            return configured
        }
        if let discovered {
            return discovered
        }

        return ACPAdapterResolution(
            available: false,
            command: "",
            arguments: [],
            message: L10n.format("Not installed · %@", missingAdapterMessage)
        )
    }

    func localPackageStatus(
        home: URL = FileManager.default.homeDirectoryForCurrentUser,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) -> ACPAdapterPackageStatus {
        let resolution = resolveAdapter(home: home, environment: environment)
        guard resolution.available else {
            return ACPAdapterPackageStatus(
                available: false,
                installedVersion: nil,
                latestVersion: nil,
                updateAvailable: nil
            )
        }

        guard let nodePackage else {
            return ACPAdapterPackageStatus(
                available: true,
                installedVersion: nil,
                latestVersion: nil,
                updateAvailable: nil
            )
        }
        let directories = searchDirectories(home: home, environment: environment)
        let version = npmPackageRoots(nodePackage, directories: directories).compactMap { packageRoot -> String? in
            guard readNPMBinEntry(packageRoot: packageRoot, binName: nodePackage.binName) != nil else {
                return nil
            }
            return readNPMPackageVersion(packageRoot: packageRoot)
        }.first
        return ACPAdapterPackageStatus(
            available: true,
            installedVersion: version,
            latestVersion: nil,
            updateAvailable: nil
        )
    }

    private func equivalentAdapterFiles(_ lhs: ACPAdapterResolution, _ rhs: ACPAdapterResolution) -> Bool {
        guard sameResolvedPath(lhs.command, rhs.command), lhs.arguments.count == rhs.arguments.count else {
            return false
        }
        for (leftArgument, rightArgument) in zip(lhs.arguments, rhs.arguments) {
            if leftArgument == rightArgument {
                continue
            }
            guard leftArgument.hasPrefix("/"), rightArgument.hasPrefix("/"),
                  sameResolvedPath(leftArgument, rightArgument) else {
                return false
            }
        }
        return true
    }

    private func sameResolvedPath(_ lhs: String, _ rhs: String) -> Bool {
        URL(fileURLWithPath: lhs).resolvingSymlinksInPath().standardizedFileURL.path ==
            URL(fileURLWithPath: rhs).resolvingSymlinksInPath().standardizedFileURL.path
    }

    private func resolveConfiguredAdapter(
        command: String,
        arguments: [String],
        directories: [URL]
    ) -> ACPAdapterResolution? {
        let trimmedCommand = command.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmedCommand.hasPrefix("/"),
              let executable = executableFile(URL(fileURLWithPath: trimmedCommand)) else {
            return nil
        }
        return resolveExecutableAdapter(executable, arguments: arguments, directories: directories)
    }

    private func resolveExecutableAdapter(
        _ executable: URL,
        arguments: [String],
        directories: [URL]
    ) -> ACPAdapterResolution? {
        var resolvedArguments = arguments
        if executable.lastPathComponent == "node" {
            guard let firstArgument = resolvedArguments.first,
                  let entry = regularFile(URL(fileURLWithPath: firstArgument)) else {
                return nil
            }
            resolvedArguments[0] = entry.path
        } else if isNodeScript(executable) {
            guard let node = resolveNodeExecutable(directories: directories) else {
                return nil
            }
            return ACPAdapterResolution(
                available: true,
                command: node.path,
                arguments: [executable.path] + resolvedArguments,
                message: L10n.format("Detected · %@ · %@", node.path, executable.path)
            )
        }
        return ACPAdapterResolution(
            available: true,
            command: executable.path,
            arguments: resolvedArguments,
            message: L10n.format("Detected · %@", executable.path)
        )
    }

    private func isNodeScript(_ executable: URL) -> Bool {
        guard let handle = try? FileHandle(forReadingFrom: executable) else {
            return false
        }
        defer { try? handle.close() }
        guard let data = try? handle.read(upToCount: 256),
              let prefix = String(data: data, encoding: .utf8),
              let firstLine = prefix.split(whereSeparator: { $0.isNewline }).first else {
            return false
        }
        let shebang = firstLine.lowercased()
        return shebang.hasPrefix("#!") && shebang.contains("node")
    }

    private func searchDirectories(home: URL, environment: [String: String]) -> [URL] {
        var directories = [
            home.appendingPathComponent(".local/bin", isDirectory: true),
            URL(fileURLWithPath: "/opt/homebrew/bin", isDirectory: true),
            URL(fileURLWithPath: "/usr/local/bin", isDirectory: true),
            URL(fileURLWithPath: "/usr/bin", isDirectory: true),
        ]
        if let path = environment["PATH"] {
            directories += path.split(separator: ":", omittingEmptySubsequences: true)
                .map { URL(fileURLWithPath: String($0), isDirectory: true) }
        }
        return uniqueURLs(directories)
    }

    private func resolveNodeExecutable(directories: [URL]) -> URL? {
        for directory in directories {
            if let node = executableFile(directory.appendingPathComponent("node")) {
                return node
            }
        }
        return nil
    }

    private func npmPackageRoots(_ package: ACPNodePackage, directories: [URL]) -> [URL] {
        var roots: [URL] = []
        for directory in directories {
            roots.append(appendingPackage(package.name, to: directory.appendingPathComponent("node_modules", isDirectory: true)))
            if directory.lastPathComponent == "bin" {
                let globalModules = directory.deletingLastPathComponent()
                    .appendingPathComponent("lib/node_modules", isDirectory: true)
                roots.append(appendingPackage(package.name, to: globalModules))
            }
        }
        return uniqueURLs(roots)
    }

    private func appendingPackage(_ packageName: String, to base: URL) -> URL {
        packageName.split(separator: "/").reduce(base) { partial, component in
            partial.appendingPathComponent(String(component), isDirectory: true)
        }
    }

    private func readNPMBinEntry(
        packageRoot: URL,
        binName: String
    ) -> URL? {
        let manifest = packageRoot.appendingPathComponent("package.json")
        guard let data = try? Data(contentsOf: manifest),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let bin = object["bin"] else {
            return nil
        }

        let relativeEntry: String?
        if let value = bin as? String {
            relativeEntry = value
        } else if let values = bin as? [String: Any] {
            relativeEntry = values[binName] as? String
        } else {
            relativeEntry = nil
        }
        guard let relativeEntry = relativeEntry?.trimmingCharacters(in: .whitespacesAndNewlines),
              !relativeEntry.isEmpty,
              !relativeEntry.hasPrefix("/") else {
            return nil
        }

        let normalizedRoot = packageRoot.standardizedFileURL
        let candidate = normalizedRoot.appendingPathComponent(relativeEntry).standardizedFileURL
        guard isDescendant(candidate, of: normalizedRoot),
              let resolved = resolvedRegularFile(candidate) else {
            return nil
        }

        // package.json 的 bin 只能指向包目录内部，避免通过符号链接把 Node 入口解析到包外。
        let resolvedRoot = normalizedRoot.resolvingSymlinksInPath().standardizedFileURL
        guard isDescendant(resolved, of: resolvedRoot) else {
            return nil
        }
        return candidate
    }

    private func readNPMPackageVersion(packageRoot: URL) -> String? {
        let manifest = packageRoot.appendingPathComponent("package.json")
        guard let data = try? Data(contentsOf: manifest),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let version = object["version"] as? String else {
            return nil
        }
        let trimmed = version.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }

    private func executableFile(_ candidate: URL) -> URL? {
        let normalized = candidate.standardizedFileURL
        guard FileManager.default.isExecutableFile(atPath: normalized.path) else {
            return nil
        }
        return regularFile(normalized)
    }

    private func regularFile(_ candidate: URL) -> URL? {
        let normalized = candidate.standardizedFileURL
        guard resolvedRegularFile(normalized) != nil else {
            return nil
        }
        // 保留用户安装器提供的稳定符号链接路径。Grok、Homebrew Node 和 npm bin
        // 都可能把稳定入口指向版本化文件；保存真实 target 会在升级清理旧版本后失效。
        return normalized
    }

    private func resolvedRegularFile(_ candidate: URL) -> URL? {
        let resolved = candidate.resolvingSymlinksInPath().standardizedFileURL
        guard let values = try? resolved.resourceValues(forKeys: [.isRegularFileKey]),
              values.isRegularFile == true else {
            return nil
        }
        return resolved
    }

    private func isDescendant(_ candidate: URL, of root: URL) -> Bool {
        let rootPath = root.standardizedFileURL.path
        let candidatePath = candidate.standardizedFileURL.path
        return candidatePath.hasPrefix(rootPath.hasSuffix("/") ? rootPath : rootPath + "/")
    }

    private func uniqueURLs(_ urls: [URL]) -> [URL] {
        var seen = Set<String>()
        return urls.compactMap { url in
            let normalized = url.standardizedFileURL
            return seen.insert(normalized.path).inserted ? normalized : nil
        }
    }
}

final class ACPAdapterUpdateChecker: @unchecked Sendable {
    private let session: URLSession

    init(session: URLSession = .shared) {
        self.session = session
    }

    func check(
        _ preset: ACPAgentPreset,
        home: URL = FileManager.default.homeDirectoryForCurrentUser,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) async -> ACPAdapterPackageStatus {
        let local = preset.localPackageStatus(home: home, environment: environment)
        guard let packageName = preset.npmPackageName else {
            return local
        }
        let encodedPackageName = packageName.replacingOccurrences(of: "/", with: "%2F")
        guard let url = URL(string: "https://registry.npmjs.org/\(encodedPackageName)/latest") else {
            return local
        }

        do {
            var request = URLRequest(url: url)
            request.timeoutInterval = 5
            request.cachePolicy = .reloadIgnoringLocalCacheData
            let (data, response) = try await session.data(for: request)
            guard (response as? HTTPURLResponse)?.statusCode == 200 else {
                return local
            }
            let latest = try JSONDecoder().decode(NPMRegistryLatest.self, from: data).version
                .trimmingCharacters(in: .whitespacesAndNewlines)
            guard !latest.isEmpty else { return local }
            return ACPAdapterPackageStatus(
                available: local.available,
                installedVersion: local.installedVersion,
                latestVersion: latest,
                updateAvailable: local.installedVersion.map { Self.isVersion(latest, newerThan: $0) }
            )
        } catch {
            return local
        }
    }

    static func isVersion(_ candidate: String, newerThan current: String) -> Bool {
        guard let candidateVersion = parsedVersion(candidate),
              let currentVersion = parsedVersion(current) else {
            return false
        }
        let count = max(candidateVersion.components.count, currentVersion.components.count)
        for index in 0..<count {
            let candidateComponent = index < candidateVersion.components.count ? candidateVersion.components[index] : 0
            let currentComponent = index < currentVersion.components.count ? currentVersion.components[index] : 0
            if candidateComponent != currentComponent {
                return candidateComponent > currentComponent
            }
        }
        if candidateVersion.prerelease != currentVersion.prerelease {
            return currentVersion.prerelease && !candidateVersion.prerelease
        }
        return false
    }

    private static func parsedVersion(_ raw: String) -> (components: [Int], prerelease: Bool)? {
        var value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if value.hasPrefix("v") || value.hasPrefix("V") {
            value.removeFirst()
        }
        let buildStripped = value.split(separator: "+", maxSplits: 1, omittingEmptySubsequences: false)[0]
        let segments = buildStripped.split(separator: "-", maxSplits: 1, omittingEmptySubsequences: false)
        let core = String(segments[0])
        let rawComponents = core.split(separator: ".", omittingEmptySubsequences: false)
        let components = rawComponents.compactMap { Int($0) }
        guard !components.isEmpty, components.count == rawComponents.count else {
            return nil
        }
        return (components, segments.count > 1 && !segments[1].isEmpty)
    }

    private struct NPMRegistryLatest: Decodable {
        let version: String
    }
}

struct ACPProfileConfiguration: Codable, Equatable {
    var id: String
    var displayName: String? = nil
    var kind: ACPAgentPreset
    var command: String
    var args: [String]
    var envFromEnv: [String: String]?
    var enabled: Bool

    enum CodingKeys: String, CodingKey {
        case id
        case displayName = "display_name"
        case kind
        case command
        case args
        case envFromEnv = "env_from_env"
        case enabled
    }
}

struct ACPDesktopConfiguration {
    static func encodeProfiles(_ profiles: [ACPProfileConfiguration]) throws -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let data = try encoder.encode(profiles)
        guard let value = String(data: data, encoding: .utf8) else {
            throw ValidationError(L10n.text("Unable to encode Coding Agent profiles."))
        }
        return value
    }

    static func decodeProfiles(_ raw: String?) throws -> [ACPProfileConfiguration] {
        guard let raw,
              !raw.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return []
        }
        let data = Data(raw.utf8)
        return try JSONDecoder().decode([ACPProfileConfiguration].self, from: data).map { profile in
            var migrated = profile
            if migrated.kind == .custom,
               migrated.id.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == ACPAgentPreset.antigravity.rawValue {
                migrated.kind = .antigravity
                migrated.displayName = nil
            }
            return migrated
        }
    }

    static func encodeArguments(_ arguments: [String]) throws -> String {
        let data = try JSONEncoder().encode(arguments)
        guard let value = String(data: data, encoding: .utf8) else {
            throw ValidationError(L10n.text("Unable to encode Coding Agent startup arguments."))
        }
        return value
    }

    static func decodeArguments(_ raw: String) throws -> [String] {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty else { return [] }
        guard let data = value.data(using: .utf8),
              let arguments = try? JSONDecoder().decode([String].self, from: data) else {
            throw ValidationError(L10n.text("Coding Agent startup arguments must be a JSON string array, for example [\"--flag\",\"value\"]."))
        }
        return arguments
    }
}
