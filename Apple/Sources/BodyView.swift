import SwiftUI

/// Lays out a message body: rich blocks for Markdown and HTML, or plain text.
struct BodyView: View {
    let rendered: RenderedBody

    var body: some View {
        content
    }

    @ViewBuilder
    private var content: some View {
        switch rendered {
        case let .plain(text):
            Text(verbatim: text)
                .font(.system(size: 13))
                .lineSpacing(2)
                .textSelection(.enabled)
        case let .blocks(blocks):
            VStack(alignment: .leading, spacing: 0) {
                ForEach(Array(blocks.enumerated()), id: \.offset) { i, block in
                    BlockView(block: block)
                        .padding(.top, i == 0 ? 0 : gap(blocks[i - 1], block))
                }
            }
            .textSelection(.enabled)
        }
    }

    private func gap(_ previous: Block, _ block: Block) -> CGFloat {
        !block.marker.isEmpty || (block.indent > 0 && previous.indent > 0) ? 3 : 8
    }
}

private struct BlockView: View {
    let block: Block

    var body: some View {
        HStack(alignment: .top, spacing: 0) {
            // One bar per quote level.
            ForEach(0..<block.quote, id: \.self) { _ in
                Rectangle()
                    .fill(Color(nsColor: .separatorColor))
                    .frame(width: 2)
                    .padding(.leading, 2)
                    .padding(.trailing, 8)
            }
            if block.indent > 0 {
                HStack(alignment: .firstTextBaseline, spacing: 0) {
                    Text(verbatim: block.marker)
                        .font(.system(size: 13))
                        .foregroundStyle(.secondary)
                        .frame(width: 18, alignment: .leading)
                    content
                }
                .padding(.leading, CGFloat(18 * (block.indent - 1)))
            } else {
                content
            }
        }
        .fixedSize(horizontal: false, vertical: true)
    }

    @ViewBuilder
    private var content: some View {
        switch block.kind {
        case .rule:
            Divider().padding(.vertical, 4)
        case .code:
            text(size: 12, mono: true)
                .padding(6)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color(nsColor: .quaternarySystemFill))
        case .heading:
            text(size: [1: 18, 2: 16, 3: 15][block.level] ?? 14, mono: false)
        case .paragraph:
            text(size: 13, mono: false)
        }
    }

    private func text(size: CGFloat, mono: Bool) -> some View {
        var result = AttributedString()
        for span in block.spans {
            var part = AttributedString(span.text)
            let weight: Font.Weight = span.bold || block.kind == .heading ? .bold : .regular
            var font: Font = span.code || mono
                ? .system(size: size - 1, design: .monospaced)
                : .system(size: size, weight: weight)
            if span.italic { font = font.italic() }
            part.font = font
            if span.strike {
                part.strikethroughStyle = .single
                part.foregroundColor = .secondary
            }
            if block.quote > 0 {
                part.foregroundColor = .secondary
            }
            if let link = span.link {
                part.link = link
            }
            result += part
        }
        return Text(result)
            .lineSpacing(2)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
