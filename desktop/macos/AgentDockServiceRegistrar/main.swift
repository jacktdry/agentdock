import Darwin
import Foundation
import ServiceManagement

private enum Target: String {
    case core
    case tunnel

    var label: String {
        switch self {
        case .core: return "dev.dropabit.agentdock.next.core"
        case .tunnel: return "dev.dropabit.agentdock.next.tunnel"
        }
    }

    var plistName: String { label + ".plist" }
}

private func fail(_ message: String, code: Int32 = EXIT_FAILURE) -> Never {
    FileHandle.standardError.write(Data((message + "\n").utf8))
    exit(code)
}

private func isUnregistered(_ status: SMAppService.Status) -> Bool {
    status == .notRegistered || status == .notFound
}

private func waitUntilUnregistered(_ service: SMAppService, timeout: TimeInterval = 5) -> Bool {
    let deadline = Date().addingTimeInterval(timeout)
    while Date() < deadline {
        if isUnregistered(service.status) { return true }
        Thread.sleep(forTimeInterval: 0.1)
    }
    return isUnregistered(service.status)
}

private func launchdJobRunning(_ label: String) -> Bool {
    let process = Process()
    process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
    process.arguments = ["print", "gui/\(getuid())/\(label)"]
    let output = Pipe()
    process.standardOutput = output
    process.standardError = Pipe()
    do {
        try process.run()
        process.waitUntilExit()
    } catch {
        return false
    }
    guard process.terminationStatus == 0 else { return false }
    let data = output.fileHandleForReading.readDataToEndOfFile()
    guard let text = String(data: data, encoding: .utf8) else { return false }
    return text.split(whereSeparator: \.isNewline).contains {
        $0.trimmingCharacters(in: .whitespaces) == "state = running"
    }
}

private func waitUntilRunning(_ label: String, timeout: TimeInterval) -> Bool {
    let deadline = Date().addingTimeInterval(timeout)
    while Date() < deadline {
        if launchdJobRunning(label) { return true }
        Thread.sleep(forTimeInterval: 0.2)
    }
    return launchdJobRunning(label)
}

guard #available(macOS 13.0, *) else {
    fail("AgentDock Service Registrar requires macOS 13 or later.")
}

let bundle = Bundle.main
guard bundle.bundleIdentifier == "dev.dropabit.agentdock.next",
      bundle.object(forInfoDictionaryKey: "AgentDockVariant") as? String == "next" else {
    fail("Refusing to manage services outside the AgentDock Next bundle.")
}

let args = Array(CommandLine.arguments.dropFirst())
guard args.count == 2, args[0] == "reregister", let target = Target(rawValue: args[1]) else {
    fail("Usage: AgentDockServiceRegistrar reregister core|tunnel", code: EX_USAGE)
}

let plist = bundle.bundleURL
    .appendingPathComponent("Contents/Library/LaunchAgents", isDirectory: true)
    .appendingPathComponent(target.plistName)
var isDirectory: ObjCBool = false
guard FileManager.default.fileExists(atPath: plist.path, isDirectory: &isDirectory),
      !isDirectory.boolValue else {
    fail("Missing AgentDock Next service definition: \(target.plistName)", code: EX_CONFIG)
}

let service = SMAppService.agent(plistName: target.plistName)
let wasRegistered: Bool
switch service.status {
case .enabled, .requiresApproval:
    wasRegistered = true
case .notRegistered, .notFound:
    wasRegistered = false
@unknown default:
    fail("Unknown background service state.", code: EX_SOFTWARE)
}

func registerAndRequireRunning() throws -> Bool {
    try service.register()
    switch service.status {
    case .enabled:
        return waitUntilRunning(target.label, timeout: 3.0)
    case .requiresApproval:
        print("requires_approval")
        exit(3)
    case .notRegistered, .notFound:
        fail("Background service registration did not become available.", code: EX_UNAVAILABLE)
    @unknown default:
        fail("Unknown background service state after registration.", code: EX_SOFTWARE)
    }
}

do {
    if wasRegistered {
        try service.unregister()
        guard waitUntilUnregistered(service) else {
            fail("Background service did not converge to the unregistered state.", code: EX_TEMPFAIL)
        }
        // SMAppService can report .notRegistered before backgroundtaskmanagementd
        // finishes invalidating the previous launch constraint. Registering again
        // too early may leave launchd with the old helper CDHash.
        Thread.sleep(forTimeInterval: 6.0)
    }

    if try registerAndRequireRunning() {
        print("enabled")
        exit(EXIT_SUCCESS)
    }

    guard wasRegistered else {
        fail("Background service registered but did not start.", code: EX_TEMPFAIL)
    }

    // On an in-place signed bundle update, macOS can accept the first register
    // while launchd still enforces the previous helper CDHash. The failed spawn
    // causes backgroundtaskmanagementd to invalidate that stale launch constraint
    // asynchronously (observed at roughly five seconds). Retry the registration
    // once after that bounded invalidation window instead of reporting a false
    // enabled state to the caller.
    Thread.sleep(forTimeInterval: 6.0)
    switch service.status {
    case .enabled, .requiresApproval:
        try service.unregister()
        guard waitUntilUnregistered(service) else {
            fail("Background service retry did not unregister cleanly.", code: EX_TEMPFAIL)
        }
    case .notRegistered, .notFound:
        break
    @unknown default:
        fail("Unknown background service state before retry.", code: EX_SOFTWARE)
    }
    Thread.sleep(forTimeInterval: 1.0)

    guard try registerAndRequireRunning() else {
        fail("Background service registration remained non-running after retry.", code: EX_TEMPFAIL)
    }
    print("enabled")
    exit(EXIT_SUCCESS)
} catch {
    fail("AgentDock Next service registration failed: \(error.localizedDescription)")
}
