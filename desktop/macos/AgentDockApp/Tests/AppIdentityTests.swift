import Foundation

private final class NoLegacyFileAccess: FileManager, @unchecked Sendable {
    override func fileExists(atPath path: String) -> Bool {
        preconditionFailure("Next consulted legacy filesystem: \(path)")
    }
}

func testAppIdentity(root: URL) throws {
    for (identity, name, bundleID, state, port) in [
        (AppIdentity.stable, "AgentDock", "com.uvwt.agentdock", ".agentdock", 8765),
        (AppIdentity.next, "AgentDock Next", "dev.dropabit.agentdock.next", ".agentdock-next", 8767),
    ] {
        let metadata: [String: Any] = [
            "CFBundleIdentifier": bundleID, "CFBundleName": name,
            "CFBundleDisplayName": name, "AgentDockVariant": identity.rawValue,
        ]
        let loaded = try AppIdentity.load(metadata: metadata)
        precondition(loaded == identity)
        precondition(identity.bundleID == bundleID && identity.appFilename == name + ".app")
        precondition(identity.coreLabel == bundleID + ".core")
        precondition(identity.tunnelLabel == bundleID + ".tunnel")
        precondition(identity.menuLabel == bundleID + ".menu-login")
        precondition(identity.corePlistName == bundleID + ".core.plist")
        precondition(identity.tunnelPlistName == bundleID + ".tunnel.plist")
        precondition(identity.menuPlistName == bundleID + ".menu-login.plist")
        precondition(identity.defaultPort == port)
        let paths = AppPaths(identity: identity, home: root, appBundle: root.appendingPathComponent(name + ".app"))
        precondition(paths.appSupport.path == root.appendingPathComponent("Library/Application Support/" + name).path)
        precondition(paths.logs.path == root.appendingPathComponent("Library/Logs/" + name).path)
        precondition(paths.stateDirectory.path == root.appendingPathComponent(state).path)
        precondition(paths.workDirectory.path == root.appendingPathComponent(name).path)
        try FileManager.default.createDirectory(at: paths.appSupport, withIntermediateDirectories: true)
        try Data().write(to: paths.environment)
        precondition(ServiceConfiguration.load(from: paths.environment, identity: identity)?.port == port)

        let prepared = try InstallerRunner(service: ServiceController(paths: paths)).prepareConfiguration(
            request: InstallRequest(mode: .quick, serverURL: "", tunnelToken: ""),
            serverURL: nil, providedTunnelToken: nil
        )
        try prepared.environment.write(to: paths.environment)
        let env = try ManagedEnvironment.load(from: paths.environment).values
        precondition(env["AGENTDOCK_PORT"] == String(port))
        precondition(String(decoding: prepared.tunnelEnvironment, as: UTF8.self).contains("127.0.0.1:\(port)"))
        if identity == .next {
            precondition(env["AGENTDOCK_HOME"] == paths.stateDirectory.path)
            precondition(env["AGENTDOCK_DEFAULT_DIR"] == paths.workDirectory.path)
            precondition(env["AGENTDOCK_DESKTOP_VARIANT"] == "next")
            precondition(paths.commandEnvironment == [
                "AGENTDOCK_DESKTOP_VARIANT": "next",
                "AGENTDOCK_HOME": paths.stateDirectory.path,
                "AGENTDOCK_DEFAULT_DIR": paths.workDirectory.path,
            ])
            precondition(!identity.allowsLegacyMigration)
            let noProbe: (String) -> Bool = { _ in preconditionFailure("Next consulted legacy launchd") }
            precondition(!LegacyDesktopRuntimeMigration.isPresent(paths: paths, fileManager: NoLegacyFileAccess(), serviceLoaded: noProbe))
            let transaction = try LegacyDesktopRuntimeMigration(paths: paths, fileManager: NoLegacyFileAccess(), serviceLoaded: noProbe).begin()
            precondition(transaction == nil)
            try ServiceController(paths: paths).validateServiceManagementReadiness()
            for key in metadata.keys {
                var incomplete = metadata
                incomplete.removeValue(forKey: key)
                rejectIdentity(incomplete)
            }
            for (key, value) in [("AgentDockVariant", "unknown"), ("AgentDockVariant", "stable"),
                                  ("CFBundleIdentifier", "com.uvwt.agentdock"), ("CFBundleName", "AgentDock")] {
                var invalid = metadata
                invalid[key] = value
                rejectIdentity(invalid)
            }
            var malformed = metadata
            malformed["AgentDockVariant"] = 42
            rejectIdentity(malformed)
        } else {
            var legacy = metadata
            legacy.removeValue(forKey: "AgentDockVariant")
            let loadedLegacy = try AppIdentity.load(metadata: legacy)
            precondition(loadedLegacy == .stable)
        }
    }
    rejectIdentity([:])
}

private func rejectIdentity(_ metadata: [String: Any]) {
    do {
        _ = try AppIdentity.load(metadata: metadata)
        preconditionFailure("Invalid identity was accepted")
    } catch is AppIdentity.InvalidMetadata {} catch {
        preconditionFailure("Unexpected identity error: \(error)")
    }
}

func testNextUpdateBoundary(root: URL) async {
    let paths = AppPaths(identity: .next, home: root.appendingPathComponent("update-test"), appBundle: root.appendingPathComponent("missing-next.app"))
    let controller = ServiceController(paths: paths, legacyServiceLoaded: { _ in
        preconditionFailure("Next updater consulted legacy services")
    })
    do {
        _ = try await controller.checkForUpdates()
        preconditionFailure("Next called the stable updater")
    } catch {
        precondition(error.localizedDescription.contains("updates are not available"))
    }
    do {
        _ = try await controller.applyUpdate { _ in preconditionFailure("Next started updating") }
        preconditionFailure("Next called the stable updater")
    } catch {
        precondition(error.localizedDescription.contains("updates are not available"))
    }
    precondition(!FileManager.default.fileExists(atPath: paths.appSupport.path))
}
