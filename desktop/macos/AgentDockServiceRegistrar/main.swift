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

do {
    switch service.status {
    case .enabled, .requiresApproval:
        try service.unregister()
        guard waitUntilUnregistered(service) else {
            fail("Background service did not converge to the unregistered state.", code: EX_TEMPFAIL)
        }
        // SMAppService can report .notRegistered before backgroundtaskmanagementd
        // finishes invalidating the previous launch constraint. Registering again
        // too early may leave launchd with the old helper CDHash and produce an
        // OS_REASON_CODESIGNING launch-constraint failure. Give that asynchronous
        // invalidation one bounded settle window before registering the new helper.
        Thread.sleep(forTimeInterval: 6.0)
    case .notRegistered, .notFound:
        break
    @unknown default:
        fail("Unknown background service state.", code: EX_SOFTWARE)
    }

    try service.register()

    switch service.status {
    case .enabled:
        print("enabled")
        exit(EXIT_SUCCESS)
    case .requiresApproval:
        print("requires_approval")
        exit(3)
    case .notRegistered, .notFound:
        fail("Background service registration did not become available.", code: EX_UNAVAILABLE)
    @unknown default:
        fail("Unknown background service state after registration.", code: EX_SOFTWARE)
    }
} catch {
    fail("AgentDock Next service registration failed: \(error.localizedDescription)")
}
