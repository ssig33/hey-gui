import Foundation

/// Raw hey CLI output kept in ~/Library/Caches so the list and threads show instantly on the next launch.
enum DiskCache {
    private static let root: URL = {
        let base = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
        return base.appendingPathComponent(Bundle.main.bundleIdentifier ?? "HeyGUI", isDirectory: true)
    }()

    private static var threadsDir: URL { root.appendingPathComponent("threads", isDirectory: true) }
    private static var imboxURL: URL { root.appendingPathComponent("imbox.json") }

    static func loadImbox() -> Data? {
        try? Data(contentsOf: imboxURL)
    }

    static func saveImbox(_ data: Data) {
        try? FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        try? data.write(to: imboxURL, options: .atomic)
    }

    /// One thread's `hey thread read --json` and `--html` output, tagged with the list version it was fetched for.
    struct Thread: Codable {
        let version: String?
        let json: Data
        let html: Data?
    }

    private static func threadURL(_ topicID: Int) -> URL {
        threadsDir.appendingPathComponent("\(topicID).plist")
    }

    static func loadThread(_ topicID: Int) -> Thread? {
        guard let data = try? Data(contentsOf: threadURL(topicID)) else { return nil }
        return try? PropertyListDecoder().decode(Thread.self, from: data)
    }

    static func saveThread(_ topicID: Int, _ thread: Thread) {
        let encoder = PropertyListEncoder()
        encoder.outputFormat = .binary
        guard let data = try? encoder.encode(thread) else { return }
        try? FileManager.default.createDirectory(at: threadsDir, withIntermediateDirectories: true)
        try? data.write(to: threadURL(topicID), options: .atomic)
    }

    /// Deletes cached threads that are no longer in the Imbox.
    static func pruneThreads(keeping topicIDs: Set<Int>) {
        let files = (try? FileManager.default.contentsOfDirectory(at: threadsDir, includingPropertiesForKeys: nil)) ?? []
        for file in files {
            if let id = Int(file.deletingPathExtension().lastPathComponent), !topicIDs.contains(id) {
                try? FileManager.default.removeItem(at: file)
            }
        }
    }
}
