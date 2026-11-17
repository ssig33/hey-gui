import AppKit
import Observation

enum AuthState: Equatable {
    case checking
    case loggedIn
    case loggedOut
    case cliMissing
    case error(String)
}

enum ThreadCommand {
    case nextEntry, previousEntry, pageDown, pageUp
}

struct ThreadCommandEvent: Equatable {
    let command: ThreadCommand
    let seq: Int
}

@Observable
@MainActor
final class InboxModel {
    var auth: AuthState = .checking
    var isLoading = false
    var isLoadingRest = false
    var loadError: String?

    var rows: [Row] = [] { didSet { refilter() } }
    var serverResults: [Row]? { didSet { refilter() } }
    var isSearchingServer = false
    var query = "" { didSet { if query != oldValue { refilter() } } }
    var newestFirst: Bool = UserDefaults.standard.object(forKey: "newestFirst") as? Bool ?? true {
        didSet {
            UserDefaults.standard.set(newestFirst, forKey: "newestFirst")
            refilter()
        }
    }

    var unreadOnly: Bool = UserDefaults.standard.bool(forKey: "unreadOnly") {
        didSet {
            UserDefaults.standard.set(unreadOnly, forKey: "unreadOnly")
            refilter()
        }
    }
    /// topic_ids of unread threads. Removed when marked as seen.
    private(set) var unread: Set<Int> = []
    private var postingIDs: [Int: Int] = [:]

    private(set) var visibleRows: [Row] = []
    var selection: Int?

    /// Requests from key handling to views. Views react whenever a value changes.
    var threadCommand: ThreadCommandEvent?
    var focusSearchToken = 0
    var focusListToken = 0
    private var seq = 0

    private var cli: HeyCLI?
    private var loadGeneration = 0

    /// Threads in memory, tagged with the list version they were fetched for.
    private var threads: [Int: (version: String?, entries: [Entry])] = [:]
    private var inFlight: [Int: Task<[Entry], Error>] = [:]
    /// Version of each thread in the list: changes when a thread gets new entries.
    private var versions: [Int: String] = [:]

    private var prefetchQueue: [Int] = []
    private var prefetchRunning = 0
    private let prefetchLimit = 3

    var selectedRow: Row? {
        guard let selection else { return nil }
        return visibleRows.first { $0.id == selection }
    }

    // MARK: - Loading

    func reload() async {
        loadGeneration += 1
        let generation = loadGeneration
        loadError = nil
        prefetchQueue = []
        var cached: [Posting] = []
        if rows.isEmpty, let data = await Task.detached(operation: { DiskCache.loadImbox() }).value,
           let postings = try? HeyCLI.postings(from: data) {
            // Show the list from the last launch while checking login and fetching the latest one.
            cached = postings
            setRows(postings)
            auth = .loggedIn
        }
        if rows.isEmpty { auth = .checking }
        let override = UserDefaults.standard.string(forKey: "heyPath") ?? ""
        let url = await Task.detached { HeyLocator.locate(override: override) }.value
        guard let url else {
            auth = .cliMissing
            return
        }
        let cli = HeyCLI(executable: url)
        self.cli = cli
        do {
            let status = try await cli.authStatus()
            guard status.authenticated else {
                auth = .loggedOut
                return
            }
            auth = .loggedIn
        } catch HeyError.notLoggedIn {
            auth = .loggedOut
            return
        } catch {
            auth = .error(error.localizedDescription)
            return
        }

        // --all takes about 20 seconds for 1000 postings, so show the first 100 and load the rest afterwards.
        isLoading = rows.isEmpty
        do {
            let first = try HeyCLI.postings(from: try await cli.imboxData(limit: 100))
            guard generation == loadGeneration else { return }
            // Until the rest arrives, keep the older cached postings below the first 100.
            let firstIDs = Set(first.compactMap(\.topicId))
            setRows(first + cached.filter { !firstIDs.contains($0.topicId ?? 0) })
            prefetchList()
            isLoading = false
            isLoadingRest = true
            let data = try await cli.imboxData(limit: nil)
            let all = try HeyCLI.postings(from: data)
            guard generation == loadGeneration else { return }
            setRows(all)
            prefetchList()
            let ids = Set(rows.map(\.id))
            Task.detached(priority: .background) {
                DiskCache.saveImbox(data)
                DiskCache.pruneThreads(keeping: ids)
            }
        } catch {
            guard generation == loadGeneration else { return }
            loadError = error.localizedDescription
        }
        isLoading = false
        isLoadingRest = false
    }

    private func setRows(_ postings: [Posting]) {
        var seen = Set<Int>()
        let list = postings.compactMap(Row.init(posting:)).filter { seen.insert($0.id).inserted }
        unread = Set(list.filter(\.unread).map(\.id))
        postingIDs = Dictionary(list.compactMap { row in row.postingID.map { (row.id, $0) } }, uniquingKeysWith: { a, _ in a })
        versions = Dictionary(list.map { ($0.id, $0.version) }, uniquingKeysWith: { a, _ in a })
        rows = list
        if selection == nil || !visibleRows.contains(where: { $0.id == selection }) {
            selection = visibleRows.first?.id
        }
    }

    // MARK: - Threads

    /// A thread already in memory, and whether it matches the current list.
    func cachedEntries(for topicID: Int) -> (entries: [Entry], fresh: Bool)? {
        guard let thread = threads[topicID] else { return nil }
        return (thread.entries, isFresh(thread.version, topicID))
    }

    private func isFresh(_ version: String?, _ topicID: Int) -> Bool {
        version != nil && version == versions[topicID]
    }

    /// Reads a thread from the disk cache even if it is out of date, to show while fetching the latest.
    func diskEntries(for topicID: Int) async -> (entries: [Entry], fresh: Bool)? {
        if let memory = cachedEntries(for: topicID) { return memory }
        let stored = await Task.detached(priority: .userInitiated) { () -> (String?, [Entry])? in
            guard let thread = DiskCache.loadThread(topicID),
                  let entries = try? HeyCLI.entries(json: thread.json, html: thread.html) else { return nil }
            return (thread.version, entries)
        }.value
        guard let (version, entries) = stored else { return nil }
        if threads[topicID] == nil { threads[topicID] = (version, entries) }
        return cachedEntries(for: topicID)
    }

    /// The latest entries of a thread: from memory or disk if they match the current list, otherwise from hey.
    func entries(for topicID: Int, priority: TaskPriority = .userInitiated) async throws -> [Entry] {
        if let cached = cachedEntries(for: topicID), cached.fresh { return cached.entries }
        if let task = inFlight[topicID] { return try await task.value }
        let version = versions[topicID]
        let cli = cli
        let task = Task.detached(priority: priority) { () -> [Entry] in
            if let version, let thread = DiskCache.loadThread(topicID), thread.version == version,
               let entries = try? HeyCLI.entries(json: thread.json, html: thread.html) {
                return entries
            }
            guard let cli else { throw HeyError.notFound }
            let (json, html) = try await cli.threadData(topicID: topicID)
            let entries = try HeyCLI.entries(json: json, html: html)
            DiskCache.saveThread(topicID, .init(version: version, json: json, html: html))
            return entries
        }
        inFlight[topicID] = task
        defer { inFlight[topicID] = nil }
        let entries = try await task.value
        threads[topicID] = (version, entries)
        return entries
    }

    /// Prefetches the threads around the selected one ahead of the rest of the queue.
    func prefetchNeighbors(of topicID: Int) {
        guard let index = visibleRows.firstIndex(where: { $0.id == topicID }) else { return }
        let around = [1, 2, 3, 4, 5, -1, -2].map { index + $0 }.filter(visibleRows.indices.contains)
        enqueuePrefetch(around.map { visibleRows[$0].id }, front: true)
    }

    /// Prefetches unread threads and the top of the list in the background.
    private func prefetchList() {
        let top = visibleRows.prefix(30).map(\.id)
        let unreadIDs = visibleRows.filter { unread.contains($0.id) }.map(\.id)
        enqueuePrefetch(top + unreadIDs, front: false)
    }

    private func enqueuePrefetch(_ ids: [Int], front: Bool) {
        var seen = Set<Int>()
        let ids = ids.filter { seen.insert($0).inserted && cachedEntries(for: $0)?.fresh != true }
        prefetchQueue.removeAll(where: seen.contains)
        if front {
            prefetchQueue.insert(contentsOf: ids, at: 0)
        } else {
            prefetchQueue.append(contentsOf: ids)
        }
        pumpPrefetch()
    }

    private func pumpPrefetch() {
        while prefetchRunning < prefetchLimit, cli != nil, !prefetchQueue.isEmpty {
            let id = prefetchQueue.removeFirst()
            if cachedEntries(for: id)?.fresh == true || inFlight[id] != nil { continue }
            prefetchRunning += 1
            Task {
                _ = try? await entries(for: id, priority: .utility)
                prefetchRunning -= 1
                pumpPrefetch()
            }
        }
    }

    // MARK: - Search and sorting

    private func refilter() {
        var list: [Row]
        if let serverResults {
            list = serverResults
        } else {
            let q = query.trimmingCharacters(in: .whitespaces).lowercased()
            list = q.isEmpty ? rows : rows.filter { $0.searchText.contains(q) }
        }
        if unreadOnly {
            list = list.filter { unread.contains($0.id) }
        }
        let newest = newestFirst
        list.sort { a, b in
            let da = a.date ?? .distantPast, db = b.date ?? .distantPast
            return newest ? da > db : da < db
        }
        visibleRows = list
    }

    func searchServer() async {
        let q = query.trimmingCharacters(in: .whitespaces)
        guard !q.isEmpty, let cli else { return }
        isSearchingServer = true
        defer { isSearchingServer = false }
        do {
            let hits = try await cli.search(q)
            guard q == query.trimmingCharacters(in: .whitespaces) else { return }
            var seen = Set<Int>()
            serverResults = hits.map(Row.init(hit:)).filter { seen.insert($0.id).inserted }
            selection = visibleRows.first?.id
        } catch {
            loadError = error.localizedDescription
        }
    }

    func clearSearch() {
        query = ""
        serverResults = nil
    }

    // MARK: - Key commands

    func move(_ delta: Int) {
        guard !visibleRows.isEmpty else { return }
        let index = visibleRows.firstIndex { $0.id == selection } ?? (delta > 0 ? -1 : visibleRows.count)
        let next = min(max(index + delta, 0), visibleRows.count - 1)
        selection = visibleRows[next].id
    }

    func isUnread(_ row: Row) -> Bool { unread.contains(row.id) }

    var unreadCount: Int { unread.count }

    /// Marks the selected thread as seen and moves to the next one.
    func markSeenAndAdvance() {
        guard let row = selectedRow,
              let index = visibleRows.firstIndex(where: { $0.id == row.id }) else { return }
        let nextID = visibleRows.indices.contains(index + 1) ? visibleRows[index + 1].id
            : (index > 0 ? visibleRows[index - 1].id : nil)
        if unread.contains(row.id), let postingID = row.postingID ?? postingIDs[row.id], let cli {
            unread.remove(row.id)
            if unreadOnly { refilter() }
            Task {
                do {
                    try await cli.markSeen(postingIDs: [postingID])
                } catch {
                    unread.insert(row.id)
                    if unreadOnly { refilter() }
                    loadError = String(localized: "Couldn't mark as seen: \(error.localizedDescription)")
                }
            }
        }
        selection = nextID
    }

    func selectFirst() { selection = visibleRows.first?.id }
    func selectLast() { selection = visibleRows.last?.id }

    func send(_ command: ThreadCommand) {
        seq += 1
        threadCommand = ThreadCommandEvent(command: command, seq: seq)
    }

    func openInBrowser() {
        if let url = selectedRow?.appURL {
            NSWorkspace.shared.open(url)
        }
    }
}
