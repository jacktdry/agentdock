import Foundation

private struct DesktopUpdateTransactionEnvelope: Decodable {
    let schemaVersion: Int
    let transactionID: String
    let state: String
    let macOS: DesktopUpdateMacOSPlan?

    private enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case transactionID = "transaction_id"
        case state
        case macOS = "macos"
    }
}

private struct DesktopUpdateMacOSPlan: Decodable {
    let sourceArbiterPath: String

    private enum CodingKeys: String, CodingKey {
        case sourceArbiterPath = "source_arbiter_path"
    }
}

enum DesktopUpdateTransactionRecovery {
    static func hasActiveTransaction(paths: AppPaths) -> Bool {
        pendingTransaction(paths: paths) != nil
    }

    static func shouldPreserveResultTrigger(paths: AppPaths) -> Bool {
        guard FileManager.default.fileExists(atPath: paths.updateTransaction.path) else { return false }
        guard let transaction = loadTransaction(paths: paths) else {
            // An unreadable durable journal is a recovery problem, not proof that the
            // one-shot desktop trigger is stale.
            return true
        }
        return transaction.state == "trial" || transaction.state == "rolling_back"
    }

    static func recoverIfNeeded(paths: AppPaths) async -> Bool {
        let transactionFileExists = FileManager.default.fileExists(atPath: paths.updateTransaction.path)
        guard !transactionFileExists || loadTransaction(paths: paths) != nil else {
            NSLog("AgentDock update recovery: durable transaction journal is unreadable.")
            return false
        }
        guard hasActiveTransaction(paths: paths) else { return true }
        return await withCheckedContinuation { continuation in
            DispatchQueue.global(qos: .utility).async {
                continuation.resume(returning: recoverSynchronouslyIfNeeded(paths: paths))
            }
        }
    }

    private static func loadTransaction(paths: AppPaths) -> DesktopUpdateTransactionEnvelope? {
        guard let data = try? Data(contentsOf: paths.updateTransaction),
              let transaction = try? JSONDecoder().decode(DesktopUpdateTransactionEnvelope.self, from: data),
              transaction.schemaVersion == 1 else {
            return nil
        }
        return transaction
    }

    private static func pendingTransaction(paths: AppPaths) -> DesktopUpdateTransactionEnvelope? {
        guard let transaction = loadTransaction(paths: paths),
              transaction.state == "trial" || transaction.state == "rolling_back" else {
            return nil
        }
        return transaction
    }

    private static func recoverSynchronouslyIfNeeded(paths: AppPaths) -> Bool {
        guard let transaction = pendingTransaction(paths: paths) else { return true }
        guard let plan = transaction.macOS else {
            NSLog("AgentDock update recovery: pending macOS transaction has no platform plan.")
            return false
        }

        let allowedRoot = paths.appSupport
            .appendingPathComponent("update/arbiters", isDirectory: true)
            .standardizedFileURL
            .path
        let arbiter = URL(fileURLWithPath: plan.sourceArbiterPath).standardizedFileURL
        let resolvedArbiter = arbiter.resolvingSymlinksInPath().path
        let prefix = allowedRoot.hasSuffix("/") ? allowedRoot : allowedRoot + "/"
        guard resolvedArbiter.hasPrefix(prefix),
              FileManager.default.isExecutableFile(atPath: resolvedArbiter) else {
            NSLog("AgentDock update recovery: source Arbiter path is not trusted: %@", resolvedArbiter)
            return false
        }

        let process = Process()
        process.executableURL = URL(fileURLWithPath: resolvedArbiter)
        process.arguments = [
            "--root", paths.appSupport.path,
            "--transaction-id", transaction.transactionID,
            "--recover-if-unlocked",
        ]
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        do {
            try process.run()
        } catch {
            NSLog("AgentDock update recovery: unable to start source Arbiter: %@", error.localizedDescription)
            return false
        }

        let deadline = Date().addingTimeInterval(90)
        while process.isRunning, Date() < deadline {
            Thread.sleep(forTimeInterval: 0.1)
        }
        if process.isRunning {
            process.terminate()
            NSLog("AgentDock update recovery: source Arbiter exceeded the recovery deadline.")
            return false
        }
        if process.terminationStatus != 0 {
            // Successful rollback intentionally returns the original trial error. In that case
            // the Arbiter launches the source App and this trial process is normally terminated.
            NSLog("AgentDock update recovery: source Arbiter exited with status %d.", process.terminationStatus)
            return false
        }
        return true
    }
}
