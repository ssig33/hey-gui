import SwiftUI

struct ThreadView: View {
    @Environment(InboxModel.self) private var model
    @State private var loaded: (id: Int, entries: [Entry])?
    @State private var error: String?
    @State private var position = ScrollPosition(idType: Int.self)
    @State private var geometry: ScrollGeometry?
    @State private var entryIndex = 0

    var body: some View {
        Group {
            if let row = model.selectedRow {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        HStack(alignment: .firstTextBaseline) {
                            Text(row.subject)
                                .font(.system(size: 15, weight: .semibold))
                                .textSelection(.enabled)
                            Spacer()
                            if let url = row.appURL {
                                OpenInBrowserButton(url: url, label: "Open in Browser")
                            }
                        }
                        .padding(.bottom, 8)
                        if let loaded, loaded.id == row.id {
                            ForEach(loaded.entries) { entry in
                                EntryView(entry: entry)
                                    .id(entry.id)
                            }
                        } else if let error {
                            Text(error).foregroundStyle(.red)
                        } else {
                            ProgressView().controlSize(.small)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                }
                .scrollPosition($position)
                .onScrollGeometryChange(for: ScrollGeometry.self) { $0 } action: { _, new in
                    geometry = new
                }
            } else {
                Text("Select a thread")
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task(id: model.selection) { await load(model.selection) }
        .onChange(of: model.threadCommand) { _, event in
            if let event { perform(event.command) }
        }
    }

    private func load(_ topicID: Int?) async {
        error = nil
        entryIndex = 0
        position.scrollTo(edge: .top)
        guard let topicID else { return }
        if loaded?.id != topicID {
            // Wait briefly so holding j doesn't fetch every thread along the way.
            try? await Task.sleep(for: .milliseconds(80))
            if Task.isCancelled { return }
        }
        do {
            let entries = try await model.entries(for: topicID)
            if Task.isCancelled { return }
            loaded = (topicID, entries)
            model.prefetchNeighbors(of: topicID)
        } catch {
            if !Task.isCancelled { self.error = error.localizedDescription }
        }
    }

    private func perform(_ command: ThreadCommand) {
        switch command {
        case .nextEntry, .previousEntry:
            guard let entries = loaded?.entries, !entries.isEmpty else { return }
            let delta = command == .nextEntry ? 1 : -1
            entryIndex = min(max(entryIndex + delta, 0), entries.count - 1)
            withAnimation(.easeOut(duration: 0.12)) {
                position.scrollTo(id: entries[entryIndex].id, anchor: .top)
            }
        case .pageDown, .pageUp:
            guard let g = geometry else { return }
            let step = g.containerSize.height * 0.9 * (command == .pageDown ? 1 : -1)
            let maxY = max(0, g.contentSize.height - g.containerSize.height)
            withAnimation(.easeOut(duration: 0.12)) {
                position.scrollTo(y: min(max(g.contentOffset.y + step, 0), maxY))
            }
        }
    }
}

struct EntryView: View {
    let entry: Entry

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Divider().padding(.bottom, 4)
            HStack(alignment: .top) {
                Grid(alignment: .leading, horizontalSpacing: 8, verticalSpacing: 1) {
                    header("From", entry.sender, bold: true)
                    if let to = entry.recipients?.to, !to.isEmpty {
                        header("To", to.map(\.display).joined(separator: ", "))
                    }
                    if let cc = entry.recipients?.cc, !cc.isEmpty {
                        header("Cc", cc.map(\.display).joined(separator: ", "))
                    }
                    header("Date", HeyDate.long(entry.date))
                    if let kind = entry.kind, kind != "message" {
                        header("Kind", kind)
                    }
                }
                .font(.system(size: 11))
                Spacer()
                if let url = entry.webURL {
                    OpenInBrowserButton(url: url, label: "Open This Email")
                }
            }
            Text(Self.render(entry.body ?? ""))
                .font(.system(size: 13))
                .lineSpacing(2)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.top, 6)
                .padding(.bottom, 14)
                .foregroundStyle(entry.kind == "message" || entry.kind == nil ? .primary : .secondary)
        }
    }

    private func header(_ label: String, _ value: String, bold: Bool = false) -> some View {
        GridRow {
            Text(verbatim: label).foregroundStyle(.secondary)
            Text(value)
                .fontWeight(bold ? .semibold : .regular)
                .textSelection(.enabled)
        }
    }

    static func render(_ markdown: String) -> AttributedString {
        let options = AttributedString.MarkdownParsingOptions(interpretedSyntax: .inlineOnlyPreservingWhitespace)
        return (try? AttributedString(markdown: markdown, options: options)) ?? AttributedString(markdown)
    }
}

struct OpenInBrowserButton: View {
    let url: URL
    let label: LocalizedStringKey

    var body: some View {
        Button {
            NSWorkspace.shared.open(url)
        } label: {
            Label(label, systemImage: "safari")
                .font(.system(size: 11))
        }
        .buttonStyle(.borderless)
        .help(url.absoluteString)
    }
}
