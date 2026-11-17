import Foundation

enum HeyError: LocalizedError {
    case notFound
    case notLoggedIn
    case failed(code: Int32, stderr: String)

    var errorDescription: String? {
        switch self {
        case .notFound:
            return String(localized: "hey command not found. Set its location in Settings (⌘,).")
        case .notLoggedIn:
            return String(localized: "Not logged in. Run `hey auth login` in Terminal.")
        case let .failed(code, stderr):
            return String(localized: "hey exited with code \(code): \(stderr)")
        }
    }
}

/// Runs the hey CLI as a child process and decodes its JSON output.
struct HeyCLI: Sendable {
    let executable: URL

    func run(_ arguments: [String]) async throws -> Data {
        let executable = executable
        return try await withCheckedThrowingContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async {
                let process = Process()
                process.executableURL = executable
                process.arguments = arguments
                var env = ProcessInfo.processInfo.environment
                let dir = executable.deletingLastPathComponent().path
                env["PATH"] = ([dir, "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"] + [env["PATH"] ?? ""]).joined(separator: ":")
                process.environment = env
                let out = Pipe()
                let err = Pipe()
                process.standardOutput = out
                process.standardError = err
                process.standardInput = FileHandle.nullDevice
                do {
                    try process.run()
                } catch {
                    continuation.resume(throwing: error)
                    return
                }
                // Large output fills the pipe, so drain stdout and stderr concurrently before waiting for exit.
                var errData = Data()
                let group = DispatchGroup()
                group.enter()
                DispatchQueue.global().async {
                    errData = err.fileHandleForReading.readDataToEndOfFile()
                    group.leave()
                }
                let outData = out.fileHandleForReading.readDataToEndOfFile()
                group.wait()
                process.waitUntilExit()
                let status = process.terminationStatus
                if status == 0 {
                    continuation.resume(returning: outData)
                } else if status == 3 {
                    continuation.resume(throwing: HeyError.notLoggedIn)
                } else {
                    let message = String(data: errData, encoding: .utf8) ?? ""
                    continuation.resume(throwing: HeyError.failed(code: status, stderr: message.trimmingCharacters(in: .whitespacesAndNewlines)))
                }
            }
        }
    }

    func decode<T: Decodable>(_ type: T.Type, _ arguments: [String]) async throws -> T {
        let data = try await run(arguments)
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(Envelope<T>.self, from: data).data
    }

    func authStatus() async throws -> AuthStatus {
        try await decode(AuthStatus.self, ["auth", "status", "--json"])
    }

    func imbox(limit: Int?) async throws -> [Posting] {
        var args = ["box", "view", "imbox", "--json"]
        if let limit {
            args += ["--limit", String(limit)]
        } else {
            args.append("--all")
        }
        return try await decode(BoxView.self, args).postings
    }

    func thread(topicID: Int) async throws -> [Entry] {
        try await decode([Entry].self, ["thread", "read", String(topicID), "--json"])
    }

    func markSeen(postingIDs: [Int]) async throws {
        _ = try await run(["seen"] + postingIDs.map(String.init) + ["--json"])
    }

    func search(_ query: String) async throws -> [SearchHit] {
        try await decode([SearchHit].self, ["search", query, "--in", "imbox", "--json"])
    }
}

/// GUI apps don't inherit the shell's PATH, so find hey ourselves.
enum HeyLocator {
    static func locate(override: String) -> URL? {
        let fm = FileManager.default
        let trimmed = override.trimmingCharacters(in: .whitespaces)
        if !trimmed.isEmpty {
            let path = (trimmed as NSString).expandingTildeInPath
            return fm.isExecutableFile(atPath: path) ? URL(fileURLWithPath: path) : nil
        }
        if let path = shellLookup(), fm.isExecutableFile(atPath: path) {
            return URL(fileURLWithPath: path)
        }
        let home = fm.homeDirectoryForCurrentUser.path
        var candidates = ["/opt/homebrew/bin/hey", "/usr/local/bin/hey", "\(home)/.local/bin/hey"]
        let miseInstalls = "\(home)/.local/share/mise/installs"
        if let dirs = try? fm.contentsOfDirectory(atPath: miseInstalls) {
            for dir in dirs where dir.contains("hey") {
                candidates.append("\(miseInstalls)/\(dir)/latest/hey")
            }
        }
        return candidates.first(where: fm.isExecutableFile(atPath:)).map(URL.init(fileURLWithPath:))
    }

    /// Runs `command -v hey` in a login shell. Tools like mise are often set up only in interactive shells, so -i is passed too.
    private static func shellLookup() -> String? {
        let shell = ProcessInfo.processInfo.environment["SHELL"] ?? "/bin/zsh"
        let process = Process()
        process.executableURL = URL(fileURLWithPath: shell)
        process.arguments = ["-ilc", "command -v hey"]
        let out = Pipe()
        process.standardOutput = out
        process.standardError = FileHandle.nullDevice
        process.standardInput = FileHandle.nullDevice
        do { try process.run() } catch { return nil }
        let data = out.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        guard process.terminationStatus == 0, let text = String(data: data, encoding: .utf8) else { return nil }
        return text.split(separator: "\n").map(String.init).last(where: { $0.hasPrefix("/") })
    }
}
