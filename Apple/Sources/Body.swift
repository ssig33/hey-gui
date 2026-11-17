import Foundation

// A message body is rendered as a list of blocks, each holding styled spans.
// Both Markdown and HTML bodies are converted into this form.

struct Span: Hashable, Sendable {
    var text: String
    var bold = false
    var italic = false
    var code = false
    var strike = false
    var link: URL?

    func sameStyle(_ o: Span) -> Bool {
        bold == o.bold && italic == o.italic && code == o.code && strike == o.strike && link == o.link
    }
}

struct Block: Hashable, Sendable {
    enum Kind: Hashable, Sendable { case paragraph, heading, code, rule }
    var kind: Kind
    var level = 0 // heading level
    var quote = 0 // blockquote depth
    var indent = 0 // list depth
    var marker = "" // list marker of the first line, such as "•" or "1."
    var spans: [Span] = []
}

enum RenderedBody: Hashable, Sendable {
    case plain(String)
    case blocks([Block])

    static func make(markdown: String, html: String?) -> RenderedBody {
        var blocks: [Block] = []
        if let html, BodyText.isAttachmentOnly(markdown) {
            blocks = HTMLBody.blocks(html)
        } else if BodyText.looksLikeMarkdown(markdown) {
            blocks = MarkdownBody.blocks(markdown)
        }
        return blocks.isEmpty ? .plain(markdown) : .blocks(blocks)
    }
}

enum BodyText {
    private static let markdownHint = try! NSRegularExpression(
        pattern: #"\[[^\]\n]+\]\([^)\s]+\)|^#{1,6} |^ {0,3}[-*+] \S|^ {0,3}\d+[.)] \S|^ {0,3}> |\*\*[^*\n]+\*\*|^ {0,3}```|\\[\\`*_{}\[\]()#+\-.!]"#,
        options: [.anchorsMatchLines])

    /// Whether a body uses Markdown syntax and should be rendered as Markdown.
    static func looksLikeMarkdown(_ s: String) -> Bool {
        markdownHint.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)) != nil
    }

    /// Whether the Markdown body is only an attachment placeholder, which is how HTML
    /// emails come back from `hey thread read`.
    static func isAttachmentOnly(_ body: String) -> Bool {
        body.split(separator: "\n").allSatisfy { line in
            let t = line.trimmingCharacters(in: .whitespaces)
            return t.isEmpty || t.hasPrefix("📎")
        }
    }
}

struct DocBuilder {
    private(set) var blocks: [Block] = []
    private var current: Block?
    var quote = 0
    var indent = 0
    var marker = ""
    var style = Span(text: "")

    mutating func start(_ kind: Block.Kind, level: Int = 0) {
        flush()
        current = Block(kind: kind, level: level, quote: quote, indent: indent, marker: marker)
        marker = ""
    }

    mutating func flush() {
        guard var block = current else { return }
        current = nil
        while let first = block.spans.first, first.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            block.spans.removeFirst()
        }
        while let last = block.spans.last, last.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            block.spans.removeLast()
        }
        guard !block.spans.isEmpty else {
            if !block.marker.isEmpty { marker = block.marker }
            return
        }
        if block.kind == .code {
            block.spans[block.spans.count - 1].text = block.spans[block.spans.count - 1].text.trimmingTrailing("\n")
        } else {
            block.spans[0].text = block.spans[0].text.trimmingLeading(" \t\n")
            block.spans[block.spans.count - 1].text = block.spans[block.spans.count - 1].text.trimmingTrailing(" \t\n")
        }
        blocks.append(block)
    }

    mutating func rule() {
        flush()
        blocks.append(Block(kind: .rule, quote: quote, indent: indent))
    }

    mutating func text(_ s: String) {
        guard !s.isEmpty else { return }
        if current == nil { start(.paragraph) }
        var span = style
        span.text = s
        if let last = current!.spans.last, last.sameStyle(span) {
            current!.spans[current!.spans.count - 1].text += s
        } else {
            current!.spans.append(span)
        }
    }

    /// The last character written to the current block, or a newline at its start.
    var lastCharacter: Character {
        current?.spans.last(where: { !$0.text.isEmpty })?.text.last ?? "\n"
    }

    mutating func finish() -> [Block] {
        flush()
        return blocks
    }
}

private extension String {
    func trimmingLeading(_ chars: String) -> String {
        String(drop(while: { chars.contains($0) }))
    }

    func trimmingTrailing(_ chars: String) -> String {
        var s = self
        while let c = s.last, chars.contains(c) { s.removeLast() }
        return s
    }
}

// MARK: - Markdown

enum MarkdownBody {
    /// Converts Markdown into blocks. Soft line breaks are kept as line breaks, since mail
    /// text is laid out line by line.
    static func blocks(_ markdown: String) -> [Block] {
        guard let attributed = try? AttributedString(
            markdown: markdown,
            options: .init(allowsExtendedAttributes: true, interpretedSyntax: .full, failurePolicy: .returnPartiallyParsedIfPossible)
        ) else { return [] }

        var b = DocBuilder()
        var currentKey: Int?
        var currentCell: Int?
        var markedItems = Set<Int>()
        for run in attributed.runs {
            let components = run.presentationIntent?.components ?? []
            let row = components.first { if case .tableRow = $0.kind { return true }; if case .tableHeaderRow = $0.kind { return true }; return false }
            let key = row?.identity ?? components.first?.identity ?? -1
            if key != currentKey {
                currentKey = key
                currentCell = nil
                b.quote = components.filter { $0.kind == .blockQuote }.count
                let items = components.filter { if case .listItem = $0.kind { return true }; return false }
                b.indent = items.count
                if let item = items.first, !markedItems.contains(item.identity), case let .listItem(ordinal) = item.kind {
                    markedItems.insert(item.identity)
                    let parent = components.drop(while: { $0.identity != item.identity }).dropFirst().first
                    b.marker = parent?.kind == .orderedList ? "\(ordinal)." : "•"
                }
                switch components.first?.kind {
                case let .header(level): b.start(.heading, level: level)
                case .codeBlock: b.start(.code)
                case .thematicBreak:
                    b.rule()
                    continue
                default: b.start(.paragraph)
                }
            }
            if row != nil, let cell = components.first(where: { if case .tableCell = $0.kind { return true }; return false }) {
                if let currentCell, currentCell != cell.identity { b.text("  |  ") }
                currentCell = cell.identity
            }

            let inline = run.inlinePresentationIntent ?? []
            var style = Span(text: "")
            style.bold = inline.contains(.stronglyEmphasized) || row.map { if case .tableHeaderRow = $0.kind { return true }; return false } ?? false
            style.italic = inline.contains(.emphasized)
            style.code = inline.contains(.code)
            style.strike = inline.contains(.strikethrough)
            style.link = run.link
            b.style = style
            if inline.contains(.softBreak) || inline.contains(.lineBreak) {
                b.text("\n")
            } else {
                b.text(String(attributed[run.range].characters))
            }
        }
        return b.finish()
    }
}

// MARK: - HTML

enum HTMLBody {
    /// Splits the output of `hey thread read --html` into the body of each entry. HTML emails
    /// are stored as Trix attachments whose content is escaped HTML wrapped in
    /// <shadow-content><template>; they are expanded back into plain HTML.
    static func entries(_ document: String) -> [Int: String] {
        let article = try! NSRegularExpression(pattern: #"<article\b[^>]*\bdata-entry-id="(\d+)"[^>]*>([\s\S]*?)</article>"#)
        let header = try! NSRegularExpression(pattern: #"<header>[\s\S]*?</header>"#)
        var result: [Int: String] = [:]
        for match in article.matches(in: document, range: NSRange(document.startIndex..., in: document)) {
            guard let idRange = Range(match.range(at: 1), in: document),
                  let bodyRange = Range(match.range(at: 2), in: document),
                  let id = Int(document[idRange]) else { continue }
            var body = String(document[bodyRange])
            body = header.stringByReplacingMatches(in: body, range: NSRange(body.startIndex..., in: body), withTemplate: "")
            result[id] = expand(body)
        }
        return result
    }

    private static let figure = try! NSRegularExpression(pattern: #"<figure\b[^>]*\bdata-trix-attachment="([^"]*)"[^>]*>[\s\S]*?</figure>"#)
    private static let attachment = try! NSRegularExpression(pattern: #"<action-text-attachment\b([^>]*)>(?:\s*</action-text-attachment>)?"#)
    private static let attribute = try! NSRegularExpression(pattern: #"([\w-]+)="([^"]*)""#)
    private static let wrapper = try! NSRegularExpression(pattern: #"</?(shadow-content|template)\b[^>]*>"#)

    static func expand(_ html: String) -> String {
        var s = html
        // Each pass unwraps one level of nested attachments.
        for _ in 0..<10 {
            var changed = false
            s = replace(figure, in: s) { groups in
                changed = true
                struct Attachment: Decodable { let contentType: String?; let content: String?; let url: String? }
                let json = Data(decodeEntities(groups[1]).utf8)
                guard let a = try? JSONDecoder().decode(Attachment.self, from: json) else { return "" }
                return attachmentHTML(type: a.contentType, content: a.content, url: a.url)
            }
            s = replace(attachment, in: s) { groups in
                changed = true
                var attrs: [String: String] = [:]
                let text = groups[1]
                for m in attribute.matches(in: text, range: NSRange(text.startIndex..., in: text)) {
                    if let k = Range(m.range(at: 1), in: text), let v = Range(m.range(at: 2), in: text) {
                        attrs[String(text[k])] = decodeEntities(String(text[v]))
                    }
                }
                return attachmentHTML(type: attrs["content-type"], content: attrs["content"], url: attrs["url"])
            }
            if !changed { break }
        }
        return wrapper.stringByReplacingMatches(in: s, range: NSRange(s.startIndex..., in: s), withTemplate: "")
    }

    private static func attachmentHTML(type: String?, content: String?, url: String?) -> String {
        if type?.hasPrefix("text/html") == true { return content ?? "" }
        if type?.hasPrefix("image") == true, let url { return #"<img src="\#(url)" alt="">"# }
        return ""
    }

    private static func replace(_ regex: NSRegularExpression, in s: String, with transform: ([String]) -> String) -> String {
        var result = ""
        var last = s.startIndex
        for m in regex.matches(in: s, range: NSRange(s.startIndex..., in: s)) {
            guard let whole = Range(m.range, in: s) else { continue }
            let groups = (0..<m.numberOfRanges).map { i in Range(m.range(at: i), in: s).map { String(s[$0]) } ?? "" }
            result += s[last..<whole.lowerBound]
            result += transform(groups)
            last = whole.upperBound
        }
        result += s[last...]
        return result
    }

    private static let entity = try! NSRegularExpression(pattern: "&(#x[0-9a-fA-F]+|#[0-9]+|[a-zA-Z]+);")
    private static let named = ["quot": "\"", "amp": "&", "lt": "<", "gt": ">", "apos": "'", "nbsp": "\u{a0}"]

    static func decodeEntities(_ s: String) -> String {
        replace(entity, in: s) { groups in
            let name = groups[1]
            if name.hasPrefix("#x"), let v = UInt32(name.dropFirst(2), radix: 16), let u = Unicode.Scalar(v) { return String(u) }
            if name.hasPrefix("#"), let v = UInt32(name.dropFirst()), let u = Unicode.Scalar(v) { return String(u) }
            return named[name] ?? groups[0]
        }
    }

    private static let spaceRun = try! NSRegularExpression(pattern: #"\s+"#)
    private static let hidden = try! NSRegularExpression(pattern: #"display:\s*none|visibility:\s*hidden|max-height:\s*0|font-size:\s*0"#)
    private static let boldStyle = try! NSRegularExpression(pattern: #"font-weight:\s*(bold|[6-9]00)"#)
    private static let fontSize = try! NSRegularExpression(pattern: #"font-size:\s*(\d+)px"#)

    /// Converts an HTML body into blocks.
    static func blocks(_ html: String) -> [Block] {
        guard let doc = try? XMLDocument(xmlString: "<html><body>\(html)</body></html>", options: [.documentTidyHTML]),
              let body = try? doc.nodes(forXPath: "//*[local-name()='body']").first else { return [] }
        var b = DocBuilder()
        for child in body.children ?? [] { walk(child, &b, pre: false) }
        return b.finish()
    }

    private static func matches(_ regex: NSRegularExpression, _ s: String) -> Bool {
        regex.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)) != nil
    }

    private static func walk(_ node: XMLNode, _ b: inout DocBuilder, pre: Bool) {
        if node.kind == .text {
            var t = node.stringValue ?? ""
            if !pre {
                t = spaceRun.stringByReplacingMatches(in: t, range: NSRange(t.startIndex..., in: t), withTemplate: " ")
                if b.lastCharacter == " " || b.lastCharacter == "\n" { t = String(t.drop(while: { $0 == " " })) }
            }
            b.text(t)
            return
        }
        guard let element = node as? XMLElement else {
            for child in node.children ?? [] { walk(child, &b, pre: pre) }
            return
        }
        let name = (element.localName ?? element.name ?? "").lowercased()
        let style = element.attribute(forName: "style")?.stringValue?.lowercased() ?? ""
        if matches(hidden, style) { return }
        let saved = b.style
        defer { b.style = saved }
        if matches(boldStyle, style) { b.style.bold = true }
        func children(pre: Bool = pre) {
            for child in element.children ?? [] { walk(child, &b, pre: pre) }
        }

        switch name {
        case "script", "style", "head", "title", "meta", "link":
            return
        case "br":
            b.text("\n")
        case "hr":
            b.rule()
        case "img":
            if let alt = element.attribute(forName: "alt")?.stringValue?.trimmingCharacters(in: .whitespaces), !alt.isEmpty {
                b.text(alt)
            }
        case "a":
            if let href = element.attribute(forName: "href")?.stringValue, !href.hasPrefix("mailto:") {
                b.style.link = URL(string: href)
            }
            children()
        case "b", "strong":
            b.style.bold = true
            children()
        case "i", "em":
            b.style.italic = true
            children()
        case "s", "strike", "del":
            b.style.strike = true
            children()
        case "code", "tt", "kbd":
            b.style.code = true
            children()
        case "h1", "h2", "h3", "h4", "h5", "h6":
            b.start(.heading, level: Int(String(name.last!)) ?? 2)
            children()
            b.flush()
        case "pre":
            b.start(.code)
            children(pre: true)
            b.flush()
        case "blockquote":
            b.flush()
            b.quote += 1
            children()
            b.flush()
            b.quote -= 1
        case "ul", "ol":
            b.flush()
            b.indent += 1
            var ordinal = 1
            for child in element.children ?? [] {
                if (child as? XMLElement)?.localName?.lowercased() == "li" {
                    b.flush()
                    b.marker = name == "ol" ? "\(ordinal)." : "•"
                    ordinal += 1
                }
                walk(child, &b, pre: pre)
            }
            b.flush()
            b.marker = ""
            b.indent -= 1
        case "td", "th":
            if b.lastCharacter != " " && b.lastCharacter != "\n" { b.text(" ") }
            if name == "th" { b.style.bold = true }
            children()
        case "div", "p":
            // Emails often style headings as large divs.
            b.flush()
            if let m = fontSize.firstMatch(in: style, range: NSRange(style.startIndex..., in: style)),
               let r = Range(m.range(at: 1), in: style), let size = Int(style[r]), size >= 20 {
                b.start(.heading, level: 2)
            }
            children()
            b.flush()
        case "section", "article", "header", "footer", "main", "nav", "aside", "table", "tbody", "thead", "tfoot",
             "tr", "li", "dl", "dt", "dd", "figure", "figcaption", "center", "address":
            b.flush()
            children()
            b.flush()
        default:
            children()
        }
    }
}
