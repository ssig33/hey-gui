import SwiftUI

struct ContentView: View {
    @Environment(InboxModel.self) private var model
    @FocusState private var searchFocused: Bool
    @FocusState private var listFocused: Bool
    @State private var keyMonitor: Any?

    var body: some View {
        @Bindable var model = model
        VStack(spacing: 0) {
            topBar
            Divider()
            main
        }
        .task { await model.reload() }
        .onAppear(perform: installKeyMonitor)
        .onDisappear {
            if let keyMonitor { NSEvent.removeMonitor(keyMonitor) }
        }
        .onChange(of: model.focusSearchToken) { searchFocused = true }
        .onChange(of: model.focusListToken) { listFocused = true }
    }

    private var topBar: some View {
        @Bindable var model = model
        return HStack(spacing: 10) {
            TextField("Search (/)", text: $model.query)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 280)
                .focused($searchFocused)
                .onSubmit { Task { await model.searchServer() } }
            if model.isSearchingServer {
                ProgressView().controlSize(.small)
            }
            Picker("Sort", selection: $model.newestFirst) {
                Text("Newest first").tag(true)
                Text("Oldest first").tag(false)
            }
            .pickerStyle(.menu)
            .fixedSize()
            Toggle("Unread only (u)", isOn: $model.unreadOnly)
                .toggleStyle(.checkbox)
            Spacer()
            if model.isLoadingRest {
                ProgressView().controlSize(.small)
                Text("Loading the rest…").foregroundStyle(.secondary)
            }
            Text("Unread \(model.unreadCount)").monospacedDigit()
            Text(countLabel).monospacedDigit()
            Text(authLabel).foregroundStyle(.secondary)
        }
        .font(.system(size: 12))
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
    }

    private var countLabel: LocalizedStringKey {
        if model.serverResults != nil {
            return "Server search: \(model.visibleRows.count)"
        }
        if model.query.isEmpty && !model.unreadOnly {
            return "\(model.rows.count) threads"
        }
        return "\(model.visibleRows.count) / \(model.rows.count) threads"
    }

    private var authLabel: LocalizedStringKey {
        switch model.auth {
        case .checking: "Checking"
        case .loggedIn: "Logged in"
        case .loggedOut: "Logged out"
        case .cliMissing: "hey not found"
        case .error: "Error"
        }
    }

    @ViewBuilder
    private var main: some View {
        switch model.auth {
        case .checking:
            message(String(localized: "Checking hey CLI…"), progress: true)
        case .cliMissing:
            message(HeyError.notFound.localizedDescription + "\n\n" + String(localized: "Press r to check again."))
        case .loggedOut:
            message(HeyError.notLoggedIn.localizedDescription + "\n\n" + String(localized: "Press r to check again."))
        case let .error(text):
            message(text + "\n\n" + String(localized: "Press r to check again."))
        case .loggedIn:
            if model.isLoading {
                message(String(localized: "Loading Imbox…"), progress: true)
            } else {
                VStack(spacing: 0) {
                    if let error = model.loadError {
                        Text(error)
                            .font(.system(size: 11))
                            .foregroundStyle(.red)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(6)
                        Divider()
                    }
                    SplitLayout {
                        PostingList(listFocused: $listFocused)
                    } trailing: {
                        ThreadView()
                    }
                }
            }
        }
    }

    /// Shows an already localized message, rendering inline Markdown such as `code`.
    private func message(_ text: String, progress: Bool = false) -> some View {
        VStack(spacing: 12) {
            if progress { ProgressView() }
            Text((try? AttributedString(markdown: text)) ?? AttributedString(text))
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding()
    }

    // MARK: - Key commands

    /// List (NSTableView) consumes letter keys itself, so catch keys before they reach the window.
    private func installKeyMonitor() {
        guard keyMonitor == nil else { return }
        let model = model
        keyMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { event in
            MainActor.assumeIsolated { handle(event, model: model) ? nil : event }
        }
    }

    /// Returns true when the key was handled.
    @MainActor
    private func handle(_ event: NSEvent, model: InboxModel) -> Bool {
        guard let window = event.window, window.identifier?.rawValue.hasPrefix("main") == true else { return false }
        let mods = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
        if !mods.isDisjoint(with: [.command, .control, .option]) { return false }

        // While typing in the search field, pass keys through as text. Esc clears the search, except during IME composition.
        if let editor = window.firstResponder as? NSTextView, editor.isEditable {
            if event.keyCode == 53, !editor.hasMarkedText() {
                model.clearSearch()
                model.focusListToken += 1
                return true
            }
            return false
        }

        switch event.keyCode {
        case 53: // esc
            model.clearSearch()
            return true
        case 36, 76: // return, enter
            model.openInBrowser()
            return true
        case 49: // space
            model.send(mods.contains(.shift) ? .pageUp : .pageDown)
            return true
        default:
            break
        }

        switch event.characters {
        case "j": model.move(1)
        case "k": model.move(-1)
        case "J": model.send(.nextEntry)
        case "K": model.send(.previousEntry)
        case "o": model.newestFirst.toggle()
        case "/": model.focusSearchToken += 1
        case "r": Task { await model.reload() }
        case "g": model.selectFirst()
        case "G": model.selectLast()
        case "O": model.openInBrowser()
        case "A": model.markSeenAndAdvance()
        case "u": model.unreadOnly.toggle()
        case "q": NSApp.terminate(nil)
        default: return false
        }
        return true
    }
}

struct PostingList: View {
    @Environment(InboxModel.self) private var model
    var listFocused: FocusState<Bool>.Binding

    var body: some View {
        @Bindable var model = model
        ScrollViewReader { proxy in
            List(model.visibleRows, selection: $model.selection) { row in
                PostingRow(row: row, unread: model.isUnread(row))
                    .id(row.id)
            }
            .listStyle(.plain)
            .environment(\.defaultMinListRowHeight, 10)
            .focused(listFocused)
            .onChange(of: model.selection) { _, id in
                if let id { proxy.scrollTo(id) }
            }
        }
    }
}

struct PostingRow: View {
    let row: Row
    let unread: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            HStack(spacing: 6) {
                Text(verbatim: unread ? "●" : " ")
                    .foregroundStyle(.blue)
                    .font(.system(size: 7))
                    .frame(width: 8)
                Text(HeyDate.short(row.date))
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
                Text(row.sender)
                    .fontWeight(unread ? .semibold : .regular)
                    .lineLimit(1)
                Spacer(minLength: 4)
                if row.count > 1 {
                    Text(verbatim: "(\(row.count))").foregroundStyle(.secondary)
                }
            }
            HStack(spacing: 6) {
                Spacer().frame(width: 8)
                (Text(row.subject).fontWeight(unread ? .semibold : .regular)
                    + Text(verbatim: row.summary.isEmpty ? "" : "  \(row.summary)").foregroundStyle(.secondary))
                    .lineLimit(1)
                    .truncationMode(.tail)
            }
        }
        .font(.system(size: 12))
        .padding(.vertical, 1)
    }
}

/// Two panes: the list and the thread. The dragged list width is saved and restored on the next launch.
struct SplitLayout<Leading: View, Trailing: View>: View {
    @ViewBuilder var leading: Leading
    @ViewBuilder var trailing: Trailing
    @AppStorage("listWidth") private var listWidth: Double = 400
    @State private var dragStart: Double?

    private let minLeading: Double = 240
    private let minTrailing: Double = 360

    var body: some View {
        GeometryReader { geo in
            let maxLeading = max(minLeading, geo.size.width - minTrailing)
            let width = min(max(listWidth, minLeading), maxLeading)
            HStack(spacing: 0) {
                leading
                    .frame(width: width)
                Rectangle()
                    .fill(Color(nsColor: .separatorColor))
                    .frame(width: 1)
                    .padding(.horizontal, 2)
                    .contentShape(Rectangle())
                    .onHover { inside in
                        if inside { NSCursor.resizeLeftRight.push() } else { NSCursor.pop() }
                    }
                    .gesture(
                        DragGesture(minimumDistance: 1, coordinateSpace: .global)
                            .onChanged { value in
                                let start = dragStart ?? width
                                dragStart = start
                                listWidth = min(max(start + value.translation.width, minLeading), maxLeading)
                            }
                            .onEnded { _ in dragStart = nil }
                    )
                trailing
                    .frame(maxWidth: .infinity)
            }
        }
    }
}
