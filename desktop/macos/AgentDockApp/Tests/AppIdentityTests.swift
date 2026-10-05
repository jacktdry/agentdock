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
            let manifestData = try InstallerRunner(service: ServiceController(paths: paths)).nextRuntimeManifest()
            let manifest = try JSONSerialization.jsonObject(with: manifestData) as! [String: Any]
            precondition(manifest["schema_version"] as? Int == 1)
            precondition(manifest["service_manager"] as? String == "smappservice")
            precondition(manifest["service_name"] as? String == identity.coreLabel)
            precondition(manifest["tunnel_service_name"] as? String == identity.tunnelLabel)
            precondition(manifest["agentdock_binary"] as? String == paths.binary.path)
            precondition(manifest["cloudflared_binary"] as? String == paths.cloudflared.path)
            precondition(manifest["environment_file"] as? String == paths.environment.path)
            precondition(manifest["tunnel_environment"] as? String == paths.tunnelEnvironment.path)
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

func testNextInstallerMutationLock(root: URL) async throws {
    let fm = FileManager.default
    let runtime = root.resolvingSymlinksInPath().appendingPathComponent("next-lock-fixture")
    let first = try await NextDesktopMutationLock.acquire(root: runtime)
    let directory = runtime.appendingPathComponent(".desktop-mutation.lock")
    let entries = try fm.contentsOfDirectory(atPath: directory.path)
    precondition(entries.count == 1 && entries[0].hasPrefix("owner-") && entries[0].count == 38)
    let owner = directory.appendingPathComponent(entries[0])
    let ownerText = try String(contentsOf: owner, encoding: .utf8)
    precondition(ownerText == "\(getpid())\n")
    let directoryMode = try fm.attributesOfItem(atPath: directory.path)[.posixPermissions] as! NSNumber
    precondition(directoryMode.intValue == 0o700)
    let ownerMode = try fm.attributesOfItem(atPath: owner.path)[.posixPermissions] as! NSNumber
    precondition(ownerMode.intValue == 0o600)
    do {
        _ = try await NextDesktopMutationLock.acquire(root: runtime, timeout: 0.05)
        preconditionFailure("Concurrent installer acquired lock")
    } catch is ValidationError {}
    let after = try fm.contentsOfDirectory(atPath: directory.path)
    precondition(after == entries)
    first.release()
    precondition(!fm.fileExists(atPath: directory.path))
    let second = try await NextDesktopMutationLock.acquire(root: runtime)
    second.release()
    precondition(!fm.fileExists(atPath: directory.path))
    // A Go-shaped owner from the same live PID must also block native mutation.
    try fm.createDirectory(at: directory, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
    let foreign = directory.appendingPathComponent("owner-0123456789abcdef0123456789abcdef")
    try Data("\(getpid())\n".utf8).write(to: foreign)
    try fm.setAttributes([.posixPermissions: 0o600], ofItemAtPath: foreign.path)
    do {
        _ = try await NextDesktopMutationLock.acquire(root: runtime, timeout: 0.05)
        preconditionFailure("Native bypassed Go lock")
    } catch is ValidationError {}
    precondition(fm.fileExists(atPath: foreign.path))
}

func testNextInstallerManifestRollback(root: URL) throws {
    let paths = AppPaths(identity: .next, home: root.appendingPathComponent("manifest-rollback"), appBundle: root.appendingPathComponent("AgentDock Next.app"))
    let runner = InstallerRunner(service: ServiceController(paths: paths))
    try FileManager.default.createDirectory(at: paths.appSupport, withIntermediateDirectories: true)
    let absent = try runner.snapshotManagedFiles()
    precondition(absent.contains { $0.url == paths.desktopRuntimeManifest && $0.data == nil })
    try runner.nextRuntimeManifest().write(to: paths.desktopRuntimeManifest)
    try runner.restoreManagedFiles(absent)
    precondition(!FileManager.default.fileExists(atPath: paths.desktopRuntimeManifest.path))
    let old = Data("old fixture manifest".utf8)
    try old.write(to: paths.desktopRuntimeManifest)
    let present = try runner.snapshotManagedFiles()
    try runner.nextRuntimeManifest().write(to: paths.desktopRuntimeManifest)
    try runner.restoreManagedFiles(present)
    let restored = try Data(contentsOf: paths.desktopRuntimeManifest)
    precondition(restored == old)
    let stablePaths = AppPaths(identity: .stable, home: root.appendingPathComponent("stable-manifest-fixture"), appBundle: root.appendingPathComponent("AgentDock.app"))
    let stable = try InstallerRunner(service: ServiceController(paths: stablePaths)).snapshotManagedFiles()
    precondition(!stable.contains { $0.url == stablePaths.desktopRuntimeManifest })
}
