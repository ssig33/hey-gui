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
    private var threadCache: [Int: [Entry]] = [:]
    private var inFlight: [Int: Task<[Entry], Error>] = [:]
    private var loadGeneration = 0

    var selectedRow: Row? {
        guard let selection else { return nil }
        return visibleRows.first { $0.id == selection }
    }

    // MARK: - Loading

    func reload() async {
        loadGeneration += 1
        let generation = loadGeneration
        auth = .checking
        loadError = nil
        threadCache = [:]
        inFlight = [:]
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
        isLoading = true
        do {
            let first = try await cli.imbox(limit: 100)
            guard generation == loadGeneration else { return }
            setRows(first)
            isLoading = false
            isLoadingRest = true
            let all = try await cli.imbox(limit: nil)
            guard generation == loadGeneration else { return }
            setRows(all)
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
        rows = list
        if selection == nil || !visibleRows.contains(where: { $0.id == selection }) {
            selection = visibleRows.first?.id
        }
    }

    func entries(for topicID: Int) async throws -> [Entry] {
        if let cached = threadCache[topicID] { return cached }
        if let task = inFlight[topicID] { return try await task.value }
        guard let cli else { throw HeyError.notFound }
        let task = Task { try await cli.thread(topicID: topicID) }
        inFlight[topicID] = task
        defer { inFlight[topicID] = nil }
        let entries = try await task.value
        threadCache[topicID] = entries
        return entries
    }

    /// Prefetches the threads before and after the selected one.
    func prefetchNeighbors(of topicID: Int) {
        guard let index = visibleRows.firstIndex(where: { $0.id == topicID }) else { return }
        for i in [index + 1, index - 1] where visibleRows.indices.contains(i) {
            let id = visibleRows[i].id
            Task { _ = try? await entries(for: id) }
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
